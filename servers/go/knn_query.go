package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/search"
)

// ---------------------------------------------------------------------------
// knn query: parse-time (x_content) and shard-level (query creation) checks
// ---------------------------------------------------------------------------

var knnQueryParams = map[string]bool{
	"vector": true, "k": true, "boost": true, "_name": true, "filter": true, "ignore_unmapped": true,
	"max_distance": true, "min_score": true, "method_parameters": true, "rescore": true,
	"expand_nested_docs": true,
}

var knnMethodParams = map[string]bool{"ef_search": true, "nprobes": true}

// jsonEntry is one key/value of a JSON object located in a raw body.
type jsonEntry struct {
	key            string
	keyPos, valPos int
	valEnd         int // offset just past the value
}

func jsonSkipWS(raw []byte, i int) int {
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' || raw[i] == '\n') {
		i++
	}
	return i
}

// jsonSkipValue returns the offset just past the JSON value starting at i.
func jsonSkipValue(raw []byte, i int) int {
	i = jsonSkipWS(raw, i)
	if i >= len(raw) {
		return i
	}
	switch raw[i] {
	case '"':
		i++
		for i < len(raw) {
			if raw[i] == '\\' {
				i += 2
				continue
			}
			if raw[i] == '"' {
				return i + 1
			}
			i++
		}
		return i
	case '{', '[':
		depth := 0
		for i < len(raw) {
			switch raw[i] {
			case '"':
				i = jsonSkipValue(raw, i)
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
			i++
		}
		return i
	default:
		for i < len(raw) && !strings.ContainsRune(",}] \t\r\n", rune(raw[i])) {
			i++
		}
		return i
	}
}

// jsonObjectEntries lists the entries of the object starting at i.
func jsonObjectEntries(raw []byte, i int) []jsonEntry {
	i = jsonSkipWS(raw, i)
	if i >= len(raw) || raw[i] != '{' {
		return nil
	}
	i++
	var out []jsonEntry
	for {
		i = jsonSkipWS(raw, i)
		if i >= len(raw) || raw[i] == '}' {
			return out
		}
		if raw[i] == ',' {
			i++
			continue
		}
		kp := i
		ke := jsonSkipValue(raw, i)
		key := strings.Trim(string(raw[kp:ke]), `"`)
		i = jsonSkipWS(raw, ke)
		if i < len(raw) && raw[i] == ':' {
			i++
		}
		vp := jsonSkipWS(raw, i)
		ve := jsonSkipValue(raw, vp)
		out = append(out, jsonEntry{key: key, keyPos: kp, valPos: vp, valEnd: ve})
		i = ve
	}
}

func jsonLineCol(raw []byte, off int) (int, int) {
	line, col := 1, 1
	for i := 0; i < off && i < len(raw); i++ {
		if raw[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// jsonFindKnn locates the value offsets of every "knn" key in the raw body.
func jsonFindKnn(raw []byte) []int {
	var out []int
	var walk func(i int)
	walk = func(i int) {
		i = jsonSkipWS(raw, i)
		if i >= len(raw) {
			return
		}
		switch raw[i] {
		case '{':
			for _, e := range jsonObjectEntries(raw, i) {
				if e.key == "knn" {
					out = append(out, e.valPos)
				}
				walk(e.valPos)
			}
		case '[':
			j := i + 1
			for {
				j = jsonSkipWS(raw, j)
				if j >= len(raw) || raw[j] == ']' {
					return
				}
				if raw[j] == ',' {
					j++
					continue
				}
				walk(j)
				j = jsonSkipValue(raw, j)
			}
		}
	}
	walk(0)
	return out
}

func knnXContentErr(raw []byte, off int, msg string, cause ojson.Object) *apierr.Error {
	line, col := jsonLineCol(raw, off)
	reason := fmt.Sprintf("[%d:%d] %s", line, col, msg)
	top := ojson.Object{{"type", "x_content_parse_exception"}, {"reason", reason}}
	root := top
	if cause != nil {
		top = append(top, ojson.KV{"caused_by", cause})
	}
	body := ojson.Object{{"error", append(ojson.Object{{"root_cause", []any{root}}}, top...)}, {"status", 400}}
	return &apierr.Error{Type: "x_content_parse_exception", Status: 400, Reason: reason,
		Shard: &search.ShardFailure{Raw: body, Status: 400}}
}

// knnParseCheck validates every knn clause of a search body the way the
// query parser does, before any shard is involved.
func knnParseCheck(raw []byte) *apierr.Error {
	if len(raw) == 0 {
		return nil
	}
	for _, kp := range jsonFindKnn(raw) {
		if err := knnParseClause(raw, kp); err != nil {
			return err
		}
	}
	return nil
}

func knnParseClause(raw []byte, kp int) *apierr.Error {
	entries := jsonObjectEntries(raw, kp)
	if raw[jsonSkipWS(raw, kp)] != '{' {
		return nil
	}
	// only objects shaped like a knn query ({field: {vector/k/...}}); a
	// "knn" key elsewhere (a field or aggregation name) is left alone
	for _, fe := range entries {
		looksKnn := false
		for _, e := range jsonObjectEntries(raw, fe.valPos) {
			if knnQueryParams[e.key] {
				looksKnn = true
			}
		}
		if !looksKnn {
			return nil
		}
	}
	if len(entries) == 0 {
		return apierr.BadRequest("[knn] field 'vector' requires to be non-null and non-empty")
	}
	if len(entries) > 1 {
		line, col := jsonLineCol(raw, entries[1].valPos)
		reason := fmt.Sprintf("[knn] query doesn't support multiple fields, found [%s] and [%s]", entries[0].key, entries[1].key)
		obj := ojson.Object{{"type", "parsing_exception"}, {"reason", reason}, {"line", line}, {"col", col}}
		body := ojson.Object{{"error", append(ojson.Object{{"root_cause", []any{obj}}}, obj...)}, {"status", 400}}
		return &apierr.Error{Type: "parsing_exception", Status: 400, Reason: reason, Shard: &search.ShardFailure{Raw: body, Status: 400}}
	}
	fe := entries[0]
	if raw[fe.valPos] != '{' {
		return nil
	}
	hasVector := false
	for _, e := range jsonObjectEntries(raw, fe.valPos) {
		if !knnQueryParams[e.key] {
			return knnXContentErr(raw, e.keyPos, "[knn] unknown field ["+e.key+"]", nil)
		}
		switch e.key {
		case "vector":
			hasVector = true
			if err := knnParseVector(raw, e); err != nil {
				return err
			}
		case "method_parameters":
			if raw[e.valPos] == '{' {
				for _, me := range jsonObjectEntries(raw, e.valPos) {
					if !knnMethodParams[me.key] {
						line, col := jsonLineCol(raw, e.valEnd-1)
						reason := "[knn] unknown method parameter found [" + me.key + "]"
						inner := ojson.Object{{"type", "parsing_exception"}, {"reason", reason}, {"line", line}, {"col", col}}
						top := ojson.Object{{"type", "x_content_parse_exception"},
							{"reason", fmt.Sprintf("[%d:%d] [knn] failed to parse field [method_parameters]", line, col)},
							{"caused_by", inner}}
						body := ojson.Object{{"error", append(ojson.Object{{"root_cause", []any{inner}}}, top...)}, {"status", 400}}
						return &apierr.Error{Type: "x_content_parse_exception", Status: 400, Reason: reason,
							Shard: &search.ShardFailure{Raw: body, Status: 400}}
					}
				}
			}
		}
	}
	if !hasVector {
		return apierr.BadRequest("[knn] requires query vector")
	}
	return nil
}

func knnParseVector(raw []byte, e jsonEntry) *apierr.Error {
	fail := func(off int, typ, reason string) *apierr.Error {
		return knnXContentErr(raw, off, "[knn] failed to parse field [vector]",
			ojson.Object{{"type", typ}, {"reason", reason}})
	}
	c := raw[e.valPos]
	if c == '"' {
		s := strings.Trim(string(raw[e.valPos:e.valEnd]), `"`)
		return fail(e.valPos, "number_format_exception", `For input string: "`+s+`"`)
	}
	if c != '[' {
		return nil
	}
	j := e.valPos + 1
	n := 0
	for {
		j = jsonSkipWS(raw, j)
		if j >= len(raw) {
			return nil
		}
		if raw[j] == ']' {
			if n == 0 {
				return fail(j, "illegal_argument_exception", "[knn] field 'vector' requires to be non-null and non-empty")
			}
			return nil
		}
		if raw[j] == ',' {
			j++
			continue
		}
		end := jsonSkipValue(raw, j)
		if raw[j] == '"' {
			s := strings.Trim(string(raw[j:end]), `"`)
			var f float64
			if _, err := fmt.Sscan(s, &f); err != nil {
				return fail(j, "number_format_exception", `For input string: "`+s+`"`)
			}
		}
		n++
		j = end
	}
}

func knnIsVectorField(idx *model.Index, field string) bool {
	def, _ := mapping.FieldDef(idx, field)
	t, _ := def["type"].(string)
	return t == "knn_vector"
}

// knnFieldOutput renders a vector for the fields option (the float array) or
// for docvalue_fields (the stored little-endian bytes, base64 encoded).
func knnFieldOutput(idx *model.Index, id, field string, def map[string]any, fields bool) ([]any, bool, *apierr.Error) {
	vals := idx.DocValues[id][field]
	if len(vals) == 0 {
		return nil, false, nil
	}
	dt := mapping.KnnInfo(def).DataType
	if fields {
		out := make([]any, 0, len(vals))
		for _, v := range vals {
			f, _ := jsonv.ToFloat(v)
			if dt == "byte" || dt == "binary" {
				out = append(out, int64(f))
			} else {
				out = append(out, jsonv.Float(float32(f)))
			}
		}
		return out, true, nil
	}
	var b []byte
	for _, v := range vals {
		f, _ := jsonv.ToFloat(v)
		if dt == "byte" || dt == "binary" {
			b = append(b, byte(int8(f)))
		} else {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(float32(f)))
		}
	}
	return []any{base64.StdEncoding.EncodeToString(b)}, true, nil
}

// knnTermCheck rejects term queries on knn_vector fields.
func knnTermCheck(idx *model.Index, raw any) *apierr.Error {
	bm, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	for field := range bm {
		def, _ := mapping.FieldDef(idx, field)
		if t, _ := def["type"].(string); t != "knn_vector" {
			continue
		}
		reason := "KNN vector do not support exact searching, use KNN queries instead: [" + field + "]"
		obj := ojson.Object{{"type", "query_shard_exception"}, {"reason", reason},
			{"index", idx.Name}, {"index_uuid", idx.UUID}}
		f := &search.ShardFailure{Status: 400, Index: idx.Name, Root: obj, Shard: obj, Top: obj}
		return &apierr.Error{Type: "query_shard_exception", Status: 400, Reason: reason, Shard: f}
	}
	return nil
}

// validateKnnQuery runs the shard-level checks of a knn query.
func validateKnnQuery(idx *model.Index, raw any) *apierr.Error {
	bm, ok := raw.(map[string]any)
	if !ok || len(bm) != 1 {
		return nil
	}
	if _, old := bm["field"]; old {
		return nil
	}
	for field, v := range bm {
		p, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		vec, ok := search.FloatList(p["vector"])
		if !ok || len(vec) == 0 {
			return nil
		}
		def, nested := search.KnnFieldDef(idx, field)
		if t, _ := def["type"].(string); t != "knn_vector" {
			if ig, _ := p["ignore_unmapped"].(bool); ig && def == nil {
				return nil
			}
			return search.KnnShardFail(idx, fmt.Sprintf("Field '%s' is not knn_vector type.", field))
		}
		if !mapping.IndexKnnEnabled(idx) {
			return search.KnnShardFail(idx, fmt.Sprintf("Field '%s' is not built for ANN search.", field))
		}
		fi := mapping.KnnInfo(def)
		n := len(vec)
		if fi.DataType == "binary" {
			n *= 8
		}
		if n != fi.Dim {
			return search.KnnShardFail(idx, fmt.Sprintf("Query vector has invalid dimension: %d. Dimension should be: %d", n, fi.Dim))
		}
		if fi.DataType == "byte" || fi.DataType == "binary" {
			for _, f := range vec {
				if f != math.Trunc(f) {
					return search.KnnShardFail(idx, fmt.Sprintf("[data_type] field was set as [%s] in index mapping. But, KNN vector values are floats instead of byte integers", fi.DataType))
				}
			}
			for _, f := range vec {
				if f < -128 || f > 127 {
					return search.KnnShardFail(idx, fmt.Sprintf("[data_type] field was set as [%s] in index mapping. But, KNN vector values are not within in the byte range [-128, 127]", fi.DataType))
				}
			}
		}
		if md, ok := jsonv.ToFloat(p["max_distance"]); ok && md < 0 && fi.Space != "innerproduct" {
			return search.KnnShardFail(idx, "[knn] requires distance to be non-negative for space type: "+strings.ToUpper(fi.Space))
		}
		if en, _ := p["expand_nested_docs"].(bool); en && !nested {
			return search.KnnShardFail(idx, "Invalid value provided for the [expand_nested_docs] field. [expand_nested_docs] is only supported with a nested field.")
		}
		if fi.Space == "cosinesimil" {
			zero := true
			for _, f := range vec {
				if f != 0 {
					zero = false
				}
			}
			if zero {
				return search.KnnShardFail(idx, "Cannot normalize a zero-length vector")
			}
		}
	}
	return nil
}
