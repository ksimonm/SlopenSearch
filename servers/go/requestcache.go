package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Shard request cache (IndicesRequestCache). OpenSearch caches the result
// of a search keyed by the request and the shard's reader: by default only
// requests with size 0, or every request with request_cache=true, unless
// index.requests.cache.enable is false or the request is not deterministic
// ("now" date math, random scores or scripts), uses a scroll, dfs or
// profiling. A cached answer is served until the index changes (a refresh
// in OpenSearch; here every visible change of any index or the cluster
// state, which is at least as strict). Stored scripts are not part of the
// key, so replacing a stored script does not change a cached answer.

type reqCacheEntry struct {
	done    chan struct{}
	ok      bool
	status  int
	header  http.Header
	body    []byte
	ordered bool
}

var reqCache = struct {
	sync.Mutex
	fp      string
	entries map[string]*reqCacheEntry
}{entries: map[string]*reqCacheEntry{}}

const reqCacheMaxEntries = 2000

var nowDateMath = regexp.MustCompile(`"now[-+/|"]|"now\|\||(?i)random`)

// dataFingerprint identifies the searchable state of every index and the
// cluster state; the caller holds s.mu (read).
func (s *Server) dataFingerprint() string {
	names := make([]string, 0, len(s.indices))
	for n := range s.indices {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "%d", s.stateVersion)
	for _, n := range names {
		idx := s.indices[n]
		var pp uintptr
		if idx.Pending != nil {
			pp = reflect.ValueOf(idx.Pending).Pointer()
		}
		fmt.Fprintf(&b, "|%s:%p:%d:%d:%d:%d:%x:%d:%d:%d:%d:%t:%d", n, idx, idx.dvGen, idx.NextSeq, len(idx.Docs),
			len(idx.Pending), pp, idx.MappingVersion, idx.IndexTotal, idx.DeleteTotal, idx.Deletes, idx.Closed, len(idx.Order))
	}
	return b.String()
}

// requestCacheKey returns the cache key of a cacheable search, or "".
func (s *Server) requestCacheKey(r *http.Request, raw []byte) string {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return ""
	}
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	target := ""
	switch {
	case len(segs) == 1 && segs[0] == "_search":
	case len(segs) == 2 && segs[1] == "_search" && segs[0] != "" && !strings.HasPrefix(segs[0], "_"):
		target = segs[0]
	default:
		return ""
	}
	q := r.URL.Query()
	if _, ok := q["scroll"]; ok {
		return ""
	}
	if q.Get("search_type") == "dfs_query_then_fetch" || q.Get("search_pipeline") != "" {
		return ""
	}
	explicit := q.Get("request_cache")
	if explicit == "false" || (explicit != "" && explicit != "true") {
		return ""
	}
	var body map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := unmarshalJSON(raw, &body); err != nil {
			return ""
		}
	}
	for _, k := range []string{"pit", "profile", "scroll", "search_pipeline"} {
		if _, ok := body[k]; ok {
			return ""
		}
	}
	if nowDateMath.Match(raw) || nowDateMath.MatchString(r.URL.RawQuery) {
		return ""
	}
	size := int64(10)
	if v, ok := toFloat(body["size"]); ok {
		size = int64(v)
	}
	if v := q.Get("size"); v != "" {
		size = -1
		if v == "0" {
			size = 0
		}
	}
	s.mu.RLock()
	indices, e := s.resolveSpec2(target, resolveOpts2{})
	s.mu.RUnlock()
	if e != nil || len(indices) == 0 {
		return ""
	}
	for _, idx := range indices {
		if v, ok := findSetting(idx.Settings, "search.default_pipeline"); ok && settingsValueString(v) != "_none" {
			return ""
		}
		if explicit == "" {
			if v, ok := findSetting(idx.Settings, "requests.cache.enable"); ok && settingsValueString(v) == "false" {
				return ""
			}
		}
	}
	if explicit == "" && size != 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		if !formatParams[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(r.URL.Path)
	for _, k := range keys {
		b.WriteString("&" + k + "=" + strings.Join(q[k], ","))
	}
	b.WriteByte('\n')
	b.Write(raw)
	return b.String()
}

// serveRequestCache answers a cacheable search from the request cache,
// computing and storing the answer on a miss. It returns false when the
// request is not cacheable.
func (s *Server) serveRequestCache(cw *captureWriter, r *http.Request) bool {
	if r.Body == nil || !strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/_search") {
		return false
	}
	raw, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	key := s.requestCacheKey(r, raw)
	if key == "" {
		return false
	}
	for attempt := 0; attempt < 3; attempt++ {
		s.mu.RLock()
		fp := s.dataFingerprint()
		s.mu.RUnlock()
		reqCache.Lock()
		if reqCache.fp != fp {
			reqCache.fp = fp
			reqCache.entries = map[string]*reqCacheEntry{}
		}
		if e := reqCache.entries[key]; e != nil {
			reqCache.Unlock()
			<-e.done
			if e.ok {
				for k, v := range e.header {
					cw.h[k] = v
				}
				cw.WriteHeader(e.status)
				cw.buf.Write(e.body)
				cw.ordered = e.ordered
				return true
			}
			continue
		}
		if len(reqCache.entries) >= reqCacheMaxEntries {
			reqCache.entries = map[string]*reqCacheEntry{}
		}
		e := &reqCacheEntry{done: make(chan struct{})}
		reqCache.entries[key] = e
		reqCache.Unlock()

		rec := &captureWriter{h: http.Header{}}
		r2 := r.Clone(r.Context())
		r2.Body = io.NopCloser(bytes.NewReader(raw))
		s.route(rec, r2)
		status := rec.status
		if status == 0 {
			status = 200
		}
		s.mu.RLock()
		same := s.dataFingerprint() == fp
		s.mu.RUnlock()
		reqCache.Lock()
		if status == 200 && same {
			e.ok, e.status, e.header, e.body, e.ordered = true, status, rec.h.Clone(), rec.buf.Bytes(), rec.ordered
		} else if reqCache.entries[key] == e {
			delete(reqCache.entries, key)
		}
		close(e.done)
		reqCache.Unlock()
		for k, v := range rec.h {
			cw.h[k] = v
		}
		cw.WriteHeader(status)
		cw.buf.Write(rec.buf.Bytes())
		cw.ordered = rec.ordered
		return true
	}
	return false
}
