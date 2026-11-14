package main

// Durable state (opt-in): when SLOP_DATA_DIR is set, the server keeps its
// cluster state and documents in that directory and reloads them on start.
//
// Layout: <dir>/snapshot holds the full state as of generation G and
// <dir>/journal-G holds every change made since. After each request that may
// have changed state the server diffs the in-memory state against what it has
// already written, appends the difference to the journal as one checksummed
// frame and fsyncs it before the response leaves, so an acknowledged write
// survives a crash (SIGKILL). A torn frame at the journal's end is dropped on
// load. On start, and whenever the journal grows large, the full state is
// written as snapshot G+1 (temp file + fsync + rename) and the journal restarts.
//
// Not persisted, like in OpenSearch: transient cluster settings, scroll
// contexts, point-in-time contexts and task results. Documents indexed but not
// yet refreshed are visible after a restart (recovery refreshes the shard).

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	persistSnapshotName = "snapshot"
	persistMagic        = "SLOPGO1\n"
	persistMinCompact   = 64 << 20
)

// record kinds
const (
	recGlobal    byte = 'G'
	recPipelines byte = 'P'
	recScripts   byte = 'S'
	recIndexMeta byte = 'I'
	recIndexNew  byte = 'X' // drop any previous index of that name and start empty
	recIndexDrop byte = 'D'
	recDocPut    byte = 'U'
	recDocDel    byte = 'R'
	recOrder     byte = 'O'
)

type docFP struct {
	seq, ver, term int64
	ptr            uintptr
}

type idxSummary struct {
	nextSeq, indexTotal, deleteTotal, deletes int64
	nDocs, nOrder, nSeq                       int
	dvGen                                     uint64
	docsPtr                                   uintptr
}

type idxPersistState struct {
	ptr   *Index
	meta  []byte
	sum   idxSummary
	docs  map[string]docFP
	order []string
}

type persister struct {
	mu       sync.Mutex
	dir      string
	gen      uint64
	journal  *os.File
	jsize    int64
	snapSize int64
	lock     *os.File

	global, pipelines, scripts []byte
	indices                    map[string]*idxPersistState
}

// openPersistence loads the state stored in dir (creating it if needed) and
// starts journaling. An empty dir disables persistence.
func (s *Server) openPersistence(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := &persister{dir: dir, indices: map[string]*idxPersistState{}}
	lf, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lf.Close()
		return fmt.Errorf("data directory %s is in use by another process: %w", dir, err)
	}
	p.lock = lf
	if err := p.load(s); err != nil {
		return err
	}
	s.persist = p
	s.mu.Lock()
	defer s.mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.compactLocked(s); err != nil {
		return err
	}
	go func() {
		// catches changes made outside a request (background tasks)
		for range time.Tick(time.Second) {
			s.persistNow()
		}
	}()
	return nil
}

// persistAfterRequest makes the changes of a finished request durable before
// its response is sent.
func (s *Server) persistAfterRequest(method, path string) {
	if s.persist == nil || method == "GET" || method == "HEAD" || readOnlyPath(path) {
		return
	}
	s.persistNow()
}

func (s *Server) persistNow() {
	p := s.persist
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var buf pbuf
	// pipelines and scripts have their own locks
	s.ingestMu.RLock()
	pipes := encodeNamedMaps(s.ingestPipelines)
	s.ingestMu.RUnlock()
	if !bytes.Equal(pipes, p.pipelines) {
		buf.byte(recPipelines)
		buf.bytes(pipes)
		p.pipelines = pipes
	}
	s.scriptsMu.RLock()
	scripts := encodeNamedMaps(s.scripts)
	s.scriptsMu.RUnlock()
	if !bytes.Equal(scripts, p.scripts) {
		buf.byte(recScripts)
		buf.bytes(scripts)
		p.scripts = scripts
	}
	s.mu.Lock()
	p.diffLocked(s, &buf)
	if len(buf.b) > 0 {
		if err := p.appendFrame(buf.b); err != nil {
			fmt.Fprintln(os.Stderr, "persistence: journal write failed:", err)
		}
	}
	if p.jsize > persistMinCompact && p.jsize > 2*p.snapSize {
		if err := p.compactLocked(s); err != nil {
			fmt.Fprintln(os.Stderr, "persistence: snapshot failed:", err)
		}
	}
	s.mu.Unlock()
}

// readOnlyPath reports requests that never change durable state.
func readOnlyPath(path string) bool {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i < len(segs) && i < 2; i++ {
		switch segs[i] {
		case "_search", "_msearch", "_count", "_analyze", "_mget", "_explain",
			"_field_caps", "_validate", "_termvectors", "_mtermvectors", "_rank_eval",
			"_render", "_refresh", "_cache", "_search_shards":
			// /_search/pipeline/<id> stores a search pipeline
			if segs[i] == "_search" && i+1 < len(segs) && segs[i+1] == "pipeline" {
				return false
			}
			return true
		}
	}
	return false
}

// diffLocked appends the records that bring the stored state up to date with
// the in-memory state. Caller holds s.mu and p.mu.
func (p *persister) diffLocked(s *Server, buf *pbuf) {
	g := encodeGlobal(s)
	if !bytes.Equal(g, p.global) {
		buf.byte(recGlobal)
		buf.bytes(g)
		p.global = g
	}
	for name := range p.indices {
		if _, ok := s.indices[name]; !ok {
			buf.byte(recIndexDrop)
			buf.str(name)
			delete(p.indices, name)
		}
	}
	names := make([]string, 0, len(s.indices))
	for name := range s.indices {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		idx := s.indices[name]
		st := p.indices[name]
		if st == nil || st.ptr != idx {
			st = &idxPersistState{ptr: idx, docs: map[string]docFP{}}
			p.indices[name] = st
			buf.byte(recIndexNew)
			buf.str(name)
		}
		meta := encodeIndexMeta(idx)
		if !bytes.Equal(meta, st.meta) {
			buf.byte(recIndexMeta)
			buf.str(name)
			buf.bytes(meta)
			st.meta = meta
		}
		sum := summarize(idx)
		if sum == st.sum && st.meta != nil && len(st.docs) == len(idx.Docs) {
			continue
		}
		st.sum = sum
		ids := make([]string, 0)
		for id, doc := range idx.Docs {
			fp := docFP{seq: idx.SeqNos[id], ver: idx.Versions[id], term: idx.PrimaryTerms[id], ptr: mapPtr(doc)}
			if old, ok := st.docs[id]; ok && old == fp {
				continue
			}
			st.docs[id] = fp
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			encodeDocPut(buf, idx, name, id)
		}
		for id := range st.docs {
			if _, ok := idx.Docs[id]; !ok {
				buf.byte(recDocDel)
				buf.str(name)
				buf.str(id)
				delete(st.docs, id)
			}
		}
		p.diffOrder(buf, name, st, idx.Order)
	}
}

func (p *persister) diffOrder(buf *pbuf, name string, st *idxPersistState, order []string) {
	old := st.order
	if len(order) == len(old) && sameStrings(order, old) {
		return
	}
	if len(order) > len(old) && sameStrings(order[:len(old)], old) {
		suffix := order[len(old):]
		buf.byte(recOrder)
		buf.str(name)
		buf.byte(1)
		buf.strs(suffix)
		st.order = append(st.order, suffix...)
		return
	}
	buf.byte(recOrder)
	buf.str(name)
	buf.byte(0)
	buf.strs(order)
	st.order = append([]string(nil), order...)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mapPtr(m map[string]any) uintptr {
	if m == nil {
		return 0
	}
	return reflect.ValueOf(m).Pointer()
}

func summarize(idx *Index) idxSummary {
	return idxSummary{
		nextSeq: idx.NextSeq, indexTotal: idx.IndexTotal, deleteTotal: idx.DeleteTotal,
		deletes: idx.Deletes, nDocs: len(idx.Docs), nOrder: len(idx.Order), nSeq: len(idx.SeqNos),
		dvGen: idx.dvGen, docsPtr: reflect.ValueOf(idx.Docs).Pointer(),
	}
}

func encodeDocPut(buf *pbuf, idx *Index, name, id string) {
	buf.byte(recDocPut)
	buf.str(name)
	buf.str(id)
	buf.varint(idx.Versions[id])
	buf.varint(idx.SeqNos[id])
	buf.varint(idx.PrimaryTerms[id])
	if r, ok := idx.Routing[id]; ok {
		buf.byte(1)
		buf.str(r)
	} else {
		buf.byte(0)
	}
	if raw, ok := idx.RawDocs[id]; ok && raw != nil {
		buf.byte(1)
		buf.bytes(raw)
	} else {
		buf.byte(0)
	}
	buf.val(idx.Docs[id])
}

func encodeGlobal(s *Server) []byte {
	var b pbuf
	b.varint(s.counter)
	b.varint(s.stateVersion)
	b.val(s.clusterPersistent)
	b.bytes(encodeNamedMaps(s.legacyTemplates))
	b.bytes(encodeNamedMaps(s.composableTemplates))
	b.bytes(encodeNamedMaps(s.componentTemplates))
	b.bytes(encodeNamedMaps(s.searchPipelines))
	names := persistKeys(s.dataStreams)
	b.uvarint(uint64(len(names)))
	for _, n := range names {
		ds := s.dataStreams[n]
		b.str(n)
		b.str(ds.Name)
		b.strs(ds.BackingIndices)
		b.varint(ds.Generation)
		b.str(ds.TimestampField)
		b.str(ds.Template)
	}
	return b.b
}

func decodeGlobal(s *Server, data []byte) error {
	r := &preader{b: data}
	s.counter = r.varint()
	s.stateVersion = r.varint()
	s.clusterPersistent = persistMap(r.val())
	s.legacyTemplates = decodeNamedMaps(r.bytes())
	s.composableTemplates = decodeNamedMaps(r.bytes())
	s.componentTemplates = decodeNamedMaps(r.bytes())
	s.searchPipelines = decodeNamedMaps(r.bytes())
	n := int(r.uvarint())
	s.dataStreams = map[string]*dataStreamInfo{}
	for i := 0; i < n && r.err == nil; i++ {
		key := r.str()
		ds := &dataStreamInfo{}
		ds.Name = r.str()
		ds.BackingIndices = r.strs()
		ds.Generation = r.varint()
		ds.TimestampField = r.str()
		ds.Template = r.str()
		s.dataStreams[key] = ds
	}
	return r.err
}

func encodeIndexMeta(idx *Index) []byte {
	var b pbuf
	b.str(idx.UUID)
	b.varint(idx.CreationDate)
	b.val(idx.Mappings)
	b.val(idx.Settings)
	b.val(idx.FieldTypes)
	b.bool(idx.Closed)
	b.val(idx.Aliases)
	b.varint(idx.MappingVersion)
	b.varint(idx.SegGen)
	b.varint(idx.Deletes)
	b.str(idx.DataStream)
	b.bool(idx.Verified)
	b.varint(idx.IndexTotal)
	b.varint(idx.DeleteTotal)
	b.bool(idx.ForceMerged)
	b.varint(idx.NextSeq)
	return b.b
}

func decodeIndexMeta(idx *Index, data []byte) error {
	r := &preader{b: data}
	idx.UUID = r.str()
	idx.CreationDate = r.varint()
	idx.Mappings = persistMap(r.val())
	idx.Settings = persistMap(r.val())
	idx.FieldTypes = map[string]string{}
	switch ft := r.val().(type) {
	case map[string]string:
		idx.FieldTypes = ft
	case map[string]any:
		for k, v := range ft {
			idx.FieldTypes[k], _ = v.(string)
		}
	}
	idx.Closed = r.bool()
	idx.Aliases = map[string]map[string]any{}
	switch a := r.val().(type) {
	case map[string]map[string]any:
		idx.Aliases = a
	case map[string]any:
		for k, v := range a {
			idx.Aliases[k] = persistMap(v)
		}
	}
	idx.MappingVersion = r.varint()
	idx.SegGen = r.varint()
	idx.Deletes = r.varint()
	idx.DataStream = r.str()
	idx.Verified = r.bool()
	idx.IndexTotal = r.varint()
	idx.DeleteTotal = r.varint()
	idx.ForceMerged = r.bool()
	idx.NextSeq = r.varint()
	return r.err
}

func persistMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func persistKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func encodeNamedMaps(m map[string]map[string]any) []byte {
	var b pbuf
	b.val(m)
	return b.b
}

func decodeNamedMaps(data []byte) map[string]map[string]any {
	r := &preader{b: data}
	out := map[string]map[string]any{}
	switch v := r.val().(type) {
	case map[string]map[string]any:
		out = v
	case map[string]any:
		for k, x := range v {
			out[k] = persistMap(x)
		}
	}
	return out
}

func newEmptyIndex(name string) *Index {
	idx := &Index{
		Name:       name,
		Mappings:   map[string]any{},
		Settings:   map[string]any{},
		Docs:       map[string]map[string]any{},
		FieldTypes: map[string]string{},
		Aliases:    map[string]map[string]any{},
	}
	ensureDocState(idx)
	return idx
}

// apply replays one frame of records into s.
func applyFrame(s *Server, data []byte) error {
	r := &preader{b: data}
	get := func(name string) *Index {
		idx := s.indices[name]
		if idx == nil {
			idx = newEmptyIndex(name)
			s.indices[name] = idx
		}
		return idx
	}
	for r.err == nil && r.pos < len(r.b) {
		switch kind := r.byte(); kind {
		case recGlobal:
			if err := decodeGlobal(s, r.bytes()); err != nil {
				return err
			}
		case recPipelines:
			s.ingestPipelines = decodeNamedMaps(r.bytes())
		case recScripts:
			s.scripts = decodeNamedMaps(r.bytes())
		case recIndexNew:
			name := r.str()
			s.indices[name] = newEmptyIndex(name)
		case recIndexDrop:
			delete(s.indices, r.str())
		case recIndexMeta:
			idx := get(r.str())
			if err := decodeIndexMeta(idx, r.bytes()); err != nil {
				return err
			}
		case recDocPut:
			idx := get(r.str())
			id := r.str()
			idx.Versions[id] = r.varint()
			idx.SeqNos[id] = r.varint()
			idx.PrimaryTerms[id] = r.varint()
			if r.byte() == 1 {
				idx.Routing[id] = r.str()
			} else {
				delete(idx.Routing, id)
			}
			if r.byte() == 1 {
				idx.RawDocs[id] = r.bytes()
			} else {
				delete(idx.RawDocs, id)
			}
			idx.Docs[id] = persistMap(r.val())
		case recDocDel:
			idx := get(r.str())
			id := r.str()
			delete(idx.Docs, id)
			delete(idx.RawDocs, id)
			delete(idx.Versions, id)
			delete(idx.SeqNos, id)
			delete(idx.PrimaryTerms, id)
			delete(idx.Routing, id)
		case recOrder:
			idx := get(r.str())
			mode := r.byte()
			ids := r.strs()
			if mode == 1 {
				idx.Order = append(idx.Order, ids...)
			} else {
				idx.Order = ids
			}
		default:
			return fmt.Errorf("unknown record kind %q", kind)
		}
	}
	return r.err
}

// load reads the snapshot and replays its journal.
func (p *persister) load(s *Server) error {
	snap, err := os.ReadFile(filepath.Join(p.dir, persistSnapshotName))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if len(snap) < len(persistMagic)+8 || string(snap[:len(persistMagic)]) != persistMagic {
			return fmt.Errorf("persistence: %s is not a snapshot", filepath.Join(p.dir, persistSnapshotName))
		}
		p.gen = binary.LittleEndian.Uint64(snap[len(persistMagic):])
		frames, _, err := readFrames(snap[len(persistMagic)+8:])
		if err != nil {
			return fmt.Errorf("persistence: corrupt snapshot: %w", err)
		}
		for _, f := range frames {
			if err := applyFrame(s, f); err != nil {
				return fmt.Errorf("persistence: snapshot: %w", err)
			}
		}
	}
	jpath := p.journalPath(p.gen)
	jdata, err := os.ReadFile(jpath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	frames, _, _ := readFrames(jdata) // a torn tail is an unacknowledged write
	for _, f := range frames {
		if err := applyFrame(s, f); err != nil {
			return fmt.Errorf("persistence: journal: %w", err)
		}
	}
	for _, idx := range s.indices {
		ensureDocState(idx)
		if idx.Docs == nil {
			idx.Docs = map[string]map[string]any{}
		}
		// recovery refreshes the shard: translog operations become searchable
		idx.Pending = map[string]map[string]any{}
		idx.recomputeAllDocValues()
	}
	return nil
}

func (p *persister) journalPath(gen uint64) string {
	return filepath.Join(p.dir, "journal-"+strconv.FormatUint(gen, 10))
}

// readFrames splits data into checksummed frames, stopping at the first
// incomplete or corrupt one; it returns the frames and the valid length.
func readFrames(data []byte) ([][]byte, int, error) {
	var out [][]byte
	pos := 0
	for pos < len(data) {
		if len(data)-pos < 12 {
			return out, pos, io.ErrUnexpectedEOF
		}
		n := binary.LittleEndian.Uint64(data[pos:])
		sum := binary.LittleEndian.Uint32(data[pos+8:])
		if n > uint64(len(data)-pos-12) {
			return out, pos, io.ErrUnexpectedEOF
		}
		body := data[pos+12 : pos+12+int(n)]
		if crc32.ChecksumIEEE(body) != sum {
			return out, pos, errors.New("checksum mismatch")
		}
		out = append(out, body)
		pos += 12 + int(n)
	}
	return out, pos, nil
}

func frame(body []byte) []byte {
	out := make([]byte, 12, 12+len(body))
	binary.LittleEndian.PutUint64(out, uint64(len(body)))
	binary.LittleEndian.PutUint32(out[8:], crc32.ChecksumIEEE(body))
	return append(out, body...)
}

func (p *persister) appendFrame(body []byte) error {
	f := frame(body)
	if _, err := p.journal.Write(f); err != nil {
		return err
	}
	p.jsize += int64(len(f))
	return p.journal.Sync()
}

// compactLocked writes the whole state as the next snapshot generation and
// starts a new journal. Caller holds s.mu and p.mu.
func (p *persister) compactLocked(s *Server) error {
	p.global, p.pipelines, p.scripts = nil, nil, nil
	p.indices = map[string]*idxPersistState{}
	var buf pbuf
	s.ingestMu.RLock()
	buf.byte(recPipelines)
	p.pipelines = encodeNamedMaps(s.ingestPipelines)
	buf.bytes(p.pipelines)
	s.ingestMu.RUnlock()
	s.scriptsMu.RLock()
	buf.byte(recScripts)
	p.scripts = encodeNamedMaps(s.scripts)
	buf.bytes(p.scripts)
	s.scriptsMu.RUnlock()
	p.diffLocked(s, &buf)

	next := p.gen + 1
	jf, err := os.OpenFile(p.journalPath(next), os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	header := make([]byte, len(persistMagic)+8)
	copy(header, persistMagic)
	binary.LittleEndian.PutUint64(header[len(persistMagic):], next)
	tmp := filepath.Join(p.dir, persistSnapshotName+".tmp")
	content := append(header, frame(buf.b)...)
	if err := writeFileSync(tmp, content); err != nil {
		jf.Close()
		return err
	}
	if err := syncDir(p.dir); err != nil {
		jf.Close()
		return err
	}
	if err := os.Rename(tmp, filepath.Join(p.dir, persistSnapshotName)); err != nil {
		jf.Close()
		return err
	}
	if err := syncDir(p.dir); err != nil {
		jf.Close()
		return err
	}
	if p.journal != nil {
		p.journal.Close()
	}
	p.journal, p.jsize, p.snapSize, p.gen = jf, 0, int64(len(content)), next
	// drop journals of older generations
	if entries, err := os.ReadDir(p.dir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "journal-") && e.Name() != filepath.Base(p.journalPath(next)) {
				os.Remove(filepath.Join(p.dir, e.Name()))
			}
		}
	}
	return nil
}

func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// ---- binary encoding ----

type pbuf struct{ b []byte }

func (w *pbuf) byte(c byte) { w.b = append(w.b, c) }
func (w *pbuf) bool(v bool) {
	if v {
		w.byte(1)
	} else {
		w.byte(0)
	}
}
func (w *pbuf) uvarint(v uint64) { w.b = binary.AppendUvarint(w.b, v) }
func (w *pbuf) varint(v int64)   { w.b = binary.AppendVarint(w.b, v) }
func (w *pbuf) str(s string) {
	w.uvarint(uint64(len(s)))
	w.b = append(w.b, s...)
}
func (w *pbuf) bytes(b []byte) {
	w.uvarint(uint64(len(b)))
	w.b = append(w.b, b...)
}
func (w *pbuf) strs(ss []string) {
	w.uvarint(uint64(len(ss)))
	for _, s := range ss {
		w.str(s)
	}
}

// value tags
const (
	tNil byte = iota
	tFalse
	tTrue
	tString
	tNumber
	tFloat64
	tInt64
	tInt
	tMap
	tSlice
	tStrings
	tStrMap
	tJSON
	tFloat32
	tInt32
	tUint64
	tMapSlice
	tNamedMaps
	tBytes
)

var persistWarned sync.Map

// val encodes the JSON-like values the server keeps in its maps, preserving
// their Go types. Maps are written with sorted keys so equal states encode to
// equal bytes.
func (w *pbuf) val(v any) {
	switch t := v.(type) {
	case nil:
		w.byte(tNil)
	case bool:
		if t {
			w.byte(tTrue)
		} else {
			w.byte(tFalse)
		}
	case string:
		w.byte(tString)
		w.str(t)
	case json.Number:
		w.byte(tNumber)
		w.str(string(t))
	case float64:
		w.byte(tFloat64)
		w.b = binary.LittleEndian.AppendUint64(w.b, math.Float64bits(t))
	case float32:
		w.byte(tFloat32)
		w.b = binary.LittleEndian.AppendUint32(w.b, math.Float32bits(t))
	case int64:
		w.byte(tInt64)
		w.varint(t)
	case int:
		w.byte(tInt)
		w.varint(int64(t))
	case int32:
		w.byte(tInt32)
		w.varint(int64(t))
	case uint64:
		w.byte(tUint64)
		w.uvarint(t)
	case []byte:
		w.byte(tBytes)
		w.bytes(t)
	case map[string]any:
		if t == nil {
			w.byte(tNil)
			return
		}
		w.byte(tMap)
		keys := persistKeys(t)
		w.uvarint(uint64(len(keys)))
		for _, k := range keys {
			w.str(k)
			w.val(t[k])
		}
	case []any:
		if t == nil {
			w.byte(tNil)
			return
		}
		w.byte(tSlice)
		w.uvarint(uint64(len(t)))
		for _, x := range t {
			w.val(x)
		}
	case []string:
		w.byte(tStrings)
		w.strs(t)
	case map[string]string:
		w.byte(tStrMap)
		keys := persistKeys(t)
		w.uvarint(uint64(len(keys)))
		for _, k := range keys {
			w.str(k)
			w.str(t[k])
		}
	case []map[string]any:
		w.byte(tMapSlice)
		w.uvarint(uint64(len(t)))
		for _, x := range t {
			w.val(x)
		}
	case map[string]map[string]any:
		w.byte(tNamedMaps)
		keys := persistKeys(t)
		w.uvarint(uint64(len(keys)))
		for _, k := range keys {
			w.str(k)
			w.val(t[k])
		}
	default:
		typ := fmt.Sprintf("%T", v)
		if _, seen := persistWarned.LoadOrStore(typ, true); !seen {
			fmt.Fprintln(os.Stderr, "persistence: storing", typ, "as JSON")
		}
		data, _ := json.Marshal(v)
		w.byte(tJSON)
		w.bytes(data)
	}
}

type preader struct {
	b   []byte
	pos int
	err error
}

func (r *preader) fail() {
	if r.err == nil {
		r.err = io.ErrUnexpectedEOF
	}
	r.pos = len(r.b)
}

func (r *preader) byte() byte {
	if r.pos >= len(r.b) {
		r.fail()
		return 0
	}
	c := r.b[r.pos]
	r.pos++
	return c
}

func (r *preader) bool() bool { return r.byte() == 1 }

func (r *preader) uvarint() uint64 {
	v, n := binary.Uvarint(r.b[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *preader) varint() int64 {
	v, n := binary.Varint(r.b[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *preader) bytes() []byte {
	n := r.uvarint()
	if n > uint64(len(r.b)-r.pos) {
		r.fail()
		return nil
	}
	out := append([]byte(nil), r.b[r.pos:r.pos+int(n)]...)
	r.pos += int(n)
	return out
}

func (r *preader) str() string {
	n := r.uvarint()
	if n > uint64(len(r.b)-r.pos) {
		r.fail()
		return ""
	}
	s := string(r.b[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s
}

func (r *preader) strs() []string {
	n := r.uvarint()
	if n > uint64(len(r.b)-r.pos) {
		r.fail()
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = r.str()
	}
	return out
}

func (r *preader) count() int {
	n := r.uvarint()
	if n > uint64(len(r.b)-r.pos) {
		r.fail()
		return 0
	}
	return int(n)
}

func (r *preader) val() any {
	switch tag := r.byte(); tag {
	case tNil:
		return nil
	case tFalse:
		return false
	case tTrue:
		return true
	case tString:
		return r.str()
	case tNumber:
		return json.Number(r.str())
	case tFloat64:
		if len(r.b)-r.pos < 8 {
			r.fail()
			return 0.0
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(r.b[r.pos:]))
		r.pos += 8
		return v
	case tFloat32:
		if len(r.b)-r.pos < 4 {
			r.fail()
			return float32(0)
		}
		v := math.Float32frombits(binary.LittleEndian.Uint32(r.b[r.pos:]))
		r.pos += 4
		return v
	case tInt64:
		return r.varint()
	case tInt:
		return int(r.varint())
	case tInt32:
		return int32(r.varint())
	case tUint64:
		return r.uvarint()
	case tBytes:
		return r.bytes()
	case tMap:
		n := r.count()
		m := make(map[string]any, n)
		for i := 0; i < n && r.err == nil; i++ {
			k := r.str()
			m[k] = r.val()
		}
		return m
	case tSlice:
		n := r.count()
		out := make([]any, 0, n)
		for i := 0; i < n && r.err == nil; i++ {
			out = append(out, r.val())
		}
		return out
	case tStrings:
		return r.strs()
	case tStrMap:
		n := r.count()
		m := make(map[string]string, n)
		for i := 0; i < n && r.err == nil; i++ {
			k := r.str()
			m[k] = r.str()
		}
		return m
	case tMapSlice:
		n := r.count()
		out := make([]map[string]any, 0, n)
		for i := 0; i < n && r.err == nil; i++ {
			out = append(out, asMapOrNil(r.val()))
		}
		return out
	case tNamedMaps:
		n := r.count()
		m := make(map[string]map[string]any, n)
		for i := 0; i < n && r.err == nil; i++ {
			k := r.str()
			m[k] = asMapOrNil(r.val())
		}
		return m
	case tJSON:
		data := r.bytes()
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		var v any
		_ = dec.Decode(&v)
		return v
	default:
		r.err = fmt.Errorf("unknown value tag %d", tag)
		r.pos = len(r.b)
		return nil
	}
}

func asMapOrNil(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
