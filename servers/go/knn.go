package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// knn_vector mappings (index.knn = true)
// ---------------------------------------------------------------------------

var knnSpaceTypes = []string{"l2", "cosinesimil", "l1", "linf", "innerproduct", "hamming"}

var knnFieldParams = map[string]bool{
	"type": true, "dimension": true, "data_type": true, "space_type": true, "method": true,
	"mode": true, "compression_level": true, "doc_values": true, "model_id": true,
}

func knnValidation(msg string) *apiError {
	r := "Validation Failed: 1: " + msg + ";"
	return wrappedMappingError(r, "validation_exception")
}

// knnIndexMappings validates and normalizes knn_vector fields of an index
// that has index.knn enabled.
func knnIndexMappings(props map[string]any) *apiError {
	for _, name := range sortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := knnIndexMappings(sub); err != nil {
				return err
			}
		}
		if t, _ := def["type"].(string); t != "knn_vector" {
			continue
		}
		if err := knnValidateField(name, def); err != nil {
			return err
		}
	}
	return nil
}

func knnValidateField(name string, def map[string]any) *apiError {
	for _, k := range sortedAnyKeys(def) {
		if !knnFieldParams[k] {
			return wrappedMappingError(fmt.Sprintf("unknown parameter [%s] on mapper [%s] of type [knn_vector]", k, name), "mapper_parsing_exception")
		}
	}
	var dim int64
	switch d := def["dimension"].(type) {
	case float64:
		dim = int64(d)
	case int64:
		dim = d
	case int:
		dim = int64(d)
	case string:
		n, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			return wrappedMappingError(fmt.Sprintf("Unable to parse [dimension] from provided value [%s] for vector [%s]", d, name), "illegal_argument_exception")
		}
		dim = n
	default:
		if f, ok := toFloat(d); ok {
			dim = int64(f)
		}
	}
	if dim <= 0 {
		return wrappedMappingError("Dimension value must be greater than 0 for vector: "+name, "illegal_argument_exception")
	}
	if m, ok := def["mode"]; ok {
		s, _ := m.(string)
		if s != "in_memory" && s != "on_disk" {
			return wrappedMappingError(fmt.Sprintf("Unknown value [%v] for field [mode] - accepted values are [null, in_memory, on_disk]", m), "mapper_parsing_exception")
		}
	}
	if c, ok := def["compression_level"]; ok {
		s, _ := c.(string)
		switch s {
		case "1x", "2x", "4x", "8x", "16x", "32x":
		default:
			return wrappedMappingError(fmt.Sprintf("Unknown value [%v] for field [compression_level] - accepted values are [, 1x, 2x, 4x, 8x, 16x, 32x]", c), "mapper_parsing_exception")
		}
	}
	dataType := "float"
	if dt, ok := def["data_type"]; ok {
		s, _ := dt.(string)
		s = strings.ToLower(s)
		switch s {
		case "binary", "byte", "float", "half_float":
		default:
			return wrappedMappingError("Invalid value provided for [data_type] field. Supported values are [binary,byte,float,half_float]", "illegal_argument_exception")
		}
		dataType = s
		def["data_type"] = strings.ToUpper(s)
	}
	topSpace, _ := def["space_type"].(string)
	engine, methodName, methodSpace := "faiss", "hnsw", ""
	var params map[string]any
	method, hasMethod := def["method"].(map[string]any)
	if hasMethod {
		n, ok := method["name"].(string)
		if !ok {
			return wrappedMappingError("name needs to be set", "mapper_parsing_exception")
		}
		methodName = n
		if e, ok := method["engine"].(string); ok {
			engine = e
		}
		switch engine {
		case "faiss", "lucene":
		case "nmslib":
			return wrappedMappingError("nmslib engine is deprecated in OpenSearch  and cannot be used for new index creation in OpenSearch from  3.0.0.", "illegal_argument_exception")
		default:
			return wrappedMappingError("Invalid engine: "+engine, "mapper_parsing_exception")
		}
		methodSpace, _ = method["space_type"].(string)
		if topSpace != "" && methodSpace != "" && topSpace != methodSpace {
			return wrappedMappingError(fmt.Sprintf(`Cannot specify conflicting space types: "[%s]" "[%s]"`, methodSpace, topSpace), "mapper_parsing_exception")
		}
		params, _ = method["parameters"].(map[string]any)
	}
	space := topSpace
	if space == "" {
		space = methodSpace
	}
	if space != "" {
		found := false
		for _, s := range knnSpaceTypes {
			if s == space {
				found = true
			}
		}
		if !found {
			return wrappedMappingError("Unable to find space: "+space+" . Valid values are: [l2, cosinesimil, l1, linf, innerproduct, hamming]", "illegal_argument_exception")
		}
		if space == "hamming" && dataType != "binary" {
			return wrappedMappingError(fmt.Sprintf("Space type [hamming] is not supported with [%s] data type", dataType), "illegal_argument_exception")
		}
		if dataType == "binary" && space != "hamming" {
			return wrappedMappingError(fmt.Sprintf("Space type [%s] is not supported with [binary] data type", space), "illegal_argument_exception")
		}
	}
	if hasMethod {
		switch {
		case methodName == "hnsw":
		case methodName == "ivf" && engine == "faiss":
			return knnValidation(`"method" requires training.`)
		default:
			return wrappedMappingError("Invalid method name: "+methodName, "illegal_argument_exception")
		}
	}
	if dim > 16000 {
		return knnValidation("Dimension value cannot be greater than 16000 for vector with engine: " + engine)
	}
	if dataType == "binary" && dim%8 != 0 {
		return knnValidation("Dimension should be multiply of 8 for binary vector data type")
	}
	if space == "l1" || space == "linf" {
		return knnValidation(fmt.Sprintf(`"%s" with "%s" configuration does not support space type: "%s".`, methodName, engine, space))
	}
	for _, k := range sortedAnyKeys(params) {
		switch k {
		case "m", "ef_construction", "ef_search":
			if n, isNum := params[k].(json.Number); isNum && !strings.ContainsAny(n.String(), ".eE") {
				continue
			}
			if _, ok := params[k].(float64); !ok {
				if _, ok := params[k].(int64); !ok {
					return knnValidation(fmt.Sprintf("value is not an instance of Integer for Integer parameter [%s].", k))
				}
			}
		case "encoder":
		default:
			return knnValidation(fmt.Sprintf("Unknown parameter '%s' found", k))
		}
	}
	if dv, ok := def["doc_values"].(bool); ok && dv {
		delete(def, "doc_values")
	}
	if hasMethod {
		ms := space
		if ms == "" {
			ms = "l2"
			if dataType == "binary" {
				ms = "hamming"
			}
		}
		if params == nil {
			params = map[string]any{}
		}
		method["engine"] = engine
		method["space_type"] = ms
		method["parameters"] = params
	}
	return nil
}

// ---------------------------------------------------------------------------
// knn query
// ---------------------------------------------------------------------------

type knnFieldInfo struct {
	space, engine, dataType string
	dim                     int
	nested                  bool
}

func knnFieldDef(idx *Index, path string) (map[string]any, bool) {
	props, _ := idx.Mappings["properties"].(map[string]any)
	parts := strings.Split(path, ".")
	nested := false
	for i, p := range parts {
		def, ok := props[p].(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return def, nested
		}
		if t, _ := def["type"].(string); t == "nested" {
			nested = true
		}
		props, _ = def["properties"].(map[string]any)
	}
	return nil, false
}

func knnInfo(def map[string]any) knnFieldInfo {
	fi := knnFieldInfo{space: "l2", engine: "faiss", dataType: "float"}
	if f, ok := toFloat(def["dimension"]); ok {
		fi.dim = int(f)
	}
	if dt, ok := def["data_type"].(string); ok {
		fi.dataType = strings.ToLower(dt)
	}
	if fi.dataType == "binary" {
		fi.space = "hamming"
	}
	if m, ok := def["method"].(map[string]any); ok {
		if e, ok := m["engine"].(string); ok {
			fi.engine = e
		}
		if s, ok := m["space_type"].(string); ok {
			fi.space = s
		}
	}
	if s, ok := def["space_type"].(string); ok && s != "" {
		fi.space = s
	}
	return fi
}

// knnDistanceScore returns the raw distance-like value and the OpenSearch score.
func knnDistanceScore(space string, q, d []float64) (float64, float64) {
	switch space {
	case "cosinesimil":
		dot, na, nb := 0.0, 0.0, 0.0
		for i := range q {
			dot += q[i] * d[i]
			na += q[i] * q[i]
			nb += d[i] * d[i]
		}
		cos := 0.0
		if na > 0 && nb > 0 {
			cos = dot / math.Sqrt(na*nb)
		}
		return 1 - cos, float64(float32((1 + cos) / 2))
	case "innerproduct":
		ip := 0.0
		for i := range q {
			ip += q[i] * d[i]
		}
		if ip >= 0 {
			return -ip, float64(float32(1 + ip))
		}
		return -ip, float64(float32(1 / (1 - ip)))
	case "hamming":
		bits := 0
		for i := range q {
			x := uint8(int8(q[i])) ^ uint8(int8(d[i]))
			for x != 0 {
				bits += int(x & 1)
				x >>= 1
			}
		}
		return float64(bits), float64(float32(1 / (1 + float64(bits))))
	case "l1":
		s := 0.0
		for i := range q {
			s += math.Abs(q[i] - d[i])
		}
		return s, float64(float32(1 / (1 + s)))
	case "linf":
		s := 0.0
		for i := range q {
			s = math.Max(s, math.Abs(q[i]-d[i]))
		}
		return s, float64(float32(1 / (1 + s)))
	default:
		s := 0.0
		for i := range q {
			s += (q[i] - d[i]) * (q[i] - d[i])
		}
		return s, float64(float32(1 / (1 + s)))
	}
}

type knnCacheEntry struct {
	body   map[string]any
	idx    *Index
	scores map[string]float64
}

var (
	knnCacheMu sync.Mutex
	knnCache   = map[uintptr][]*knnCacheEntry{}
	knnCacheN  int
)

func floatList(v any) ([]float64, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		f, ok := toFloat(e)
		if !ok {
			return nil, false
		}
		out = append(out, f)
	}
	return out, true
}

func (c *queryCtx) docVector(field string) ([]float64, bool) {
	vals, present := c.values(field)
	if !present || len(vals) == 0 {
		return nil, false
	}
	if len(vals) == 1 {
		if arr, ok := floatList(vals[0]); ok {
			return arr, true
		}
	}
	var dv []float64
	for _, e := range vals {
		f, ok := toFloat(e)
		if !ok {
			return nil, false
		}
		dv = append(dv, f)
	}
	return dv, true
}

func knnParseErr(reason string) *apiError {
	return &apiError{Type: "parsing_exception", Reason: reason, Status: 400}
}

func (c *queryCtx) evalKnnField(field string, p map[string]any) (bool, float64, *apiError) {
	vec, ok := floatList(p["vector"])
	if !ok {
		return false, 0, knnParseErr("[knn] failed to parse field [vector]")
	}
	_, hasK := p["k"]
	_, hasMin := p["min_score"]
	_, hasMax := p["max_distance"]
	set := 0
	for _, b := range []bool{hasK, hasMin, hasMax} {
		if b {
			set++
		}
	}
	if set != 1 {
		return false, 0, errBad("[knn] requires exactly one of k, distance or score to be set")
	}
	boost := 1.0
	if b, ok := toFloat(p["boost"]); ok {
		boost = b
	}
	def, _ := knnFieldDef(c.idx, field)
	fi := knnInfo(def)
	score := func(qc *queryCtx) (bool, float64, float64) {
		dv, ok := qc.docVector(field)
		if !ok || len(dv) != len(vec) {
			return false, 0, 0
		}
		d, s := knnDistanceScore(fi.space, vec, dv)
		return true, d, s
	}
	filter, _ := p["filter"].(map[string]any)
	if hasK {
		kf, _ := toFloat(p["k"])
		k := int(kf)
		if k <= 0 || k > 10000 {
			return false, 0, errBad("[knn] requires k to be in the range (0, 10000]")
		}
		if c.nestedRoot != nil {
			return c.knnNestedTopK(p, k, filter, score, boost)
		}
		scores, err := c.knnTopK(p, field, k, filter, score)
		if err != nil {
			return false, 0, err
		}
		s, ok := scores[c.id]
		if !ok {
			return false, 0, nil
		}
		return true, s * boost, nil
	}
	if filter != nil {
		m, _, err := c.eval(filter)
		if err != nil || !m {
			return false, 0, err
		}
	}
	ok, d, s := score(c)
	if !ok {
		return false, 0, nil
	}
	if hasMin {
		ms, _ := toFloat(p["min_score"])
		if ms <= 0 {
			return false, 0, errBad("[knn] requires minScore to be greater than 0")
		}
		return s >= ms, s * boost, nil
	}
	md, _ := toFloat(p["max_distance"])
	if filter == nil && fi.engine == "faiss" {
		// faiss range search excludes the boundary; exact (filtered) search keeps it
		return d < md, s * boost, nil
	}
	return d <= md, s * boost, nil
}

func (c *queryCtx) knnTopK(p map[string]any, field string, k int, filter map[string]any,
	score func(*queryCtx) (bool, float64, float64)) (map[string]float64, *apiError) {
	key := reflect.ValueOf(p).Pointer()
	knnCacheMu.Lock()
	for _, e := range knnCache[key] {
		if e.idx == c.idx {
			knnCacheMu.Unlock()
			return e.scores, nil
		}
	}
	knnCacheMu.Unlock()
	type cand struct {
		id    string
		score float64
		ord   int
	}
	var cands []cand
	for ord, id := range c.idx.Order {
		doc, ok := c.idx.Docs[id]
		if !ok {
			continue
		}
		qc := &queryCtx{idx: c.idx, id: id, doc: doc}
		if filter != nil {
			m, _, err := qc.eval(filter)
			if err != nil {
				return nil, err
			}
			if !m {
				continue
			}
		}
		if ok, _, s := score(qc); ok {
			cands = append(cands, cand{id, s, ord})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > k {
		cands = cands[:k]
	}
	out := map[string]float64{}
	for _, cd := range cands {
		out[cd.id] = cd.score
	}
	knnCacheMu.Lock()
	if knnCacheN > 512 {
		knnCache = map[uintptr][]*knnCacheEntry{}
		knnCacheN = 0
	}
	knnCache[key] = append(knnCache[key], &knnCacheEntry{body: p, idx: c.idx, scores: out})
	knnCacheN++
	knnCacheMu.Unlock()
	return out, nil
}
