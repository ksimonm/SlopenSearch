package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// normalizeQuery re-parses the raw query string the way OpenSearch does:
// empty pairs are skipped, a pair with an empty name uses its value as the
// name, and a repeated parameter keeps its last value.
func normalizeQuery(r *http.Request) {
	raw := r.URL.RawQuery
	if raw == "" {
		return
	}
	out := url.Values{}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		n, err := url.QueryUnescape(name)
		if err != nil {
			n = name
		}
		v, err := url.QueryUnescape(value)
		if err != nil {
			v = value
		}
		if n == "" {
			if v == "" {
				continue
			}
			n, v = v, ""
		}
		out[n] = []string{v}
	}
	r.URL.RawQuery = out.Encode()
}

// strictBoolParam validates a boolean query parameter that must be exactly
// "true" or "false" (empty means true).
func strictBoolParam(r *http.Request, name string) *apiError {
	v, ok := r.URL.Query()[name]
	if !ok || len(v) == 0 {
		return nil
	}
	switch v[0] {
	case "", "true", "false":
		return nil
	}
	return &apiError{Type: "illegal_argument_exception", Status: 400,
		Reason: "Failed to parse value [" + v[0] + "] as only [true] or [false] are allowed."}
}

// checkContentType answers 406/400 for request bodies OpenSearch does not accept.
// It returns true when the request was rejected.
func checkContentType(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	base := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	if ct != "" && !strings.Contains(base, "/") {
		inner := "invalid Content-Type header [" + ct + "]"
		writeError(w, &apiError{Type: "content_type_header_exception", Status: 400,
			Reason:   "java.lang.IllegalArgumentException: " + inner,
			CausedBy: &apiError{Type: "illegal_argument_exception", Reason: inner}})
		return true
	}
	switch base {
	case "application/json", "application/x-ndjson", "application/yaml", "application/x-yaml",
		"text/yaml", "application/yml", "application/smile", "application/cbor",
		"application/vnd.opensearch+json", "application/vnd.opensearch+x-ndjson",
		"application/vnd.opensearch+yaml", "application/vnd.opensearch+smile",
		"application/vnd.opensearch+cbor":
		return false
	}
	writeSimpleError(w, 406, "Content-Type header ["+ct+"] is not supported")
	return true
}

// decodeRequestBody undoes a gzip or deflate Content-Encoding.
func decodeRequestBody(r *http.Request) *apiError {
	enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return nil
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return errBad("failed to read body")
	}
	var rd io.Reader
	switch enc {
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return errBad("failed to decompress request body")
		}
		rd = zr
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return errBad("failed to decompress request body")
		}
		rd = zr
	default:
		r.Body = io.NopCloser(bytes.NewReader(raw))
		return nil
	}
	plain, err := io.ReadAll(io.LimitReader(rd, maxContentLength+1))
	if err != nil {
		return errBad("failed to decompress request body")
	}
	r.Body = io.NopCloser(bytes.NewReader(plain))
	r.Header.Del("Content-Encoding")
	return nil
}

// responseEncoding picks the response Content-Encoding the way Netty's
// HttpContentCompressor does (brotli and zstd are not available).
func responseEncoding(accept string) string {
	if accept == "" {
		return ""
	}
	gz, df, sn, star := -1.0, -1.0, -1.0, -1.0
	for _, tok := range strings.Split(accept, ",") {
		name, params, _ := strings.Cut(tok, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		q := 1.0
		if i := strings.Index(params, "q="); i >= 0 {
			if f, err := strconv.ParseFloat(strings.TrimSpace(params[i+2:]), 64); err == nil {
				q = f
			} else {
				q = 0
			}
		}
		switch name {
		case "gzip", "x-gzip":
			gz = q
		case "deflate":
			df = q
		case "snappy":
			sn = q
		case "*":
			star = q
		}
	}
	switch {
	case sn > 0 && sn >= gz && sn >= df:
		return "snappy"
	case gz > 0 && gz >= df:
		return "gzip"
	case df > 0:
		return "deflate"
	}
	if star > 0 {
		switch {
		case sn == -1:
			return "snappy"
		case gz == -1:
			return "gzip"
		case df == -1:
			return "deflate"
		}
	}
	return ""
}

var crc32c = crc32.MakeTable(crc32.Castagnoli)

// snappyFrame wraps data in the snappy framing format using uncompressed chunks.
func snappyFrame(data []byte) []byte {
	var out bytes.Buffer
	out.Write([]byte{0xff, 0x06, 0x00, 0x00, 's', 'N', 'a', 'P', 'p', 'Y'})
	for len(data) > 0 {
		n := len(data)
		if n > 65536 {
			n = 65536
		}
		chunk := data[:n]
		data = data[n:]
		c := crc32.Checksum(chunk, crc32c)
		masked := ((c >> 15) | (c << 17)) + 0xa282ead8
		l := len(chunk) + 4
		out.Write([]byte{0x01, byte(l), byte(l >> 8), byte(l >> 16)})
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], masked)
		out.Write(b[:])
		out.Write(chunk)
	}
	return out.Bytes()
}

func compressBody(enc string, body []byte) []byte {
	var buf bytes.Buffer
	switch enc {
	case "gzip":
		zw := gzip.NewWriter(&buf)
		zw.Write(body)
		zw.Close()
	case "deflate":
		zw := zlib.NewWriter(&buf)
		zw.Write(body)
		zw.Close()
	case "snappy":
		return snappyFrame(body)
	default:
		return body
	}
	return buf.Bytes()
}

// writeFinal writes the response head and the (possibly compressed) body.
// A HEAD response carries the Content-Length of the matching GET body.
func writeFinal(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	if r.Method == http.MethodHead {
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		return
	}
	if len(body) > 0 {
		if enc := responseEncoding(r.Header.Get("Accept-Encoding")); enc != "" {
			body = compressBody(enc, body)
			h.Set("Content-Encoding", enc)
		}
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	w.Write(body)
}

// javaURLEncode mirrors java.net.URLEncoder.encode(s, UTF-8).
func javaURLEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var sb strings.Builder
	for _, b := range []byte(s) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9',
			b == '.', b == '-', b == '*', b == '_':
			sb.WriteByte(b)
		case b == ' ':
			sb.WriteByte('+')
		default:
			sb.WriteByte('%')
			sb.WriteByte(hex[b>>4])
			sb.WriteByte(hex[b&15])
		}
	}
	return sb.String()
}

// levenshteinSimilarity mirrors Lucene's LevenshteinDistance.getDistance.
func levenshteinSimilarity(a, b string) float64 {
	s, t := []rune(a), []rune(b)
	n, m := len(s), len(t)
	if n == 0 || m == 0 {
		if n == m {
			return 1
		}
		return 0
	}
	p := make([]int, n+1)
	d := make([]int, n+1)
	for i := range p {
		p[i] = i
	}
	for j := 1; j <= m; j++ {
		d[0] = j
		for i := 1; i <= n; i++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i] = min(d[i-1]+1, p[i]+1, p[i-1]+cost)
		}
		p, d = d, p
	}
	mx := max(n, m)
	return 1 - float64(p[n])/float64(mx)
}

// unrecognizedParamsReason formats OpenSearch's "unrecognized parameter" message.
func unrecognizedParamsReason(path string, bad []string, allowed map[string]bool) string {
	sort.Strings(bad)
	parts := make([]string, 0, len(bad))
	for _, k := range bad {
		type cand struct {
			score float64
			name  string
		}
		var cands []cand
		for a := range allowed {
			if sc := levenshteinSimilarity(k, a); sc > 0.5 {
				cands = append(cands, cand{sc, a})
			}
		}
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].score != cands[j].score {
				return cands[i].score > cands[j].score
			}
			return cands[i].name < cands[j].name
		})
		s := "[" + k + "]"
		if len(cands) == 1 {
			s += " -> did you mean [" + cands[0].name + "]?"
		} else if len(cands) > 1 {
			names := make([]string, len(cands))
			for i, c := range cands {
				names[i] = c.name
			}
			s += " -> did you mean any of [" + strings.Join(names, ", ") + "]?"
		}
		parts = append(parts, s)
	}
	word := "parameter"
	if len(bad) > 1 {
		word = "parameters"
	}
	return "request [" + path + "] contains unrecognized " + word + ": " + strings.Join(parts, ", ")
}
