package search

import (
	"math"
	"strings"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
)

func searchSortSpecValue(h MultiHit, sp SortSpec) (any, bool, bool) {
	v, ok, missing := searchSortSpecValueRaw(h, sp)
	if ok && !missing && sp.NumericType != "" {
		v = convertSortNumeric(v, sp.NumericType)
	}
	return v, ok, missing
}

func convertSortNumeric(v any, numericType string) any {
	f, ok := jsonv.ToFloat(v)
	if !ok {
		return v
	}
	switch numericType {
	case "long":
		return int64(f)
	case "double":
		return f
	case "date_nanos":
		return int64(f) * 1000000
	}
	return v
}

func searchSortSpecValueRaw(h MultiHit, sp SortSpec) (any, bool, bool) {
	switch sp.Field {
	case "_score":
		return h.Score, true, false
	case "_id":
		return h.Id, true, false
	case "_index":
		return h.Idx.Name, true, false
	case "_script":
		if sp.ScriptVals != nil {
			v, ok := sp.ScriptVals[h.Idx.Name+"\x00"+h.Id]
			return v, ok, false
		}
		return scriptSortValue(sp, h), true, false
	case "_geo_distance":
		return geoSortDistance(h, sp), true, false
	case "_shard_doc":
		pos := 0
		for i, id := range h.Idx.Order {
			if id == h.Id {
				pos = i
				break
			}
		}
		shard := int64(h.Idx.DocShard(h.Id))
		return shard*1000000000 + int64(pos), true, false
	case "_doc":
		pos := 0
		for i, id := range h.Idx.Order {
			if id == h.Id {
				pos = i
				break
			}
		}
		return int64(pos), true, false
	}
	if fieldUnderNested(h.Idx, sp.Field) && sp.NestedPath == "" {
		return nil, false, true
	}
	if sp.NestedPath != "" {
		vals := nestedSortValues(h, sp)
		if len(vals) == 0 {
			return nil, false, true
		}
		if len(vals) == 1 {
			return vals[0], true, false
		}
		return reduceSortValues(vals, sp.Desc, sp.Mode), true, false
	}
	if sp.Unmapped != "" {
		if !SortFieldMapped(h.Idx, sp.Field) {
			switch sp.Unmapped {
			case "long":
				return int64(9223372036854775807), true, false
			case "integer", "short", "byte":
				return int64(2147483647), true, false
			case "double", "float", "half_float", "scaled_float":
				return math.MaxFloat64, true, false
			}
		}
	}
	v, ok := h.sortValue(sp.Field, sp.Desc, sp.Mode)
	if ok {
		return v, true, false
	}
	if sp.HasMissing {
		switch m := sp.Missing.(type) {
		case string:
			if m == "_first" || m == "_last" {
				return nil, false, true
			}
		}
		return sp.Missing, true, false
	}
	return nil, false, true
}

// compareSortSpec compares two hits by one sort spec.
func compareSortSpec(a, b MultiHit, sp SortSpec) int {
	av, aok, amissing := searchSortSpecValue(a, sp)
	bv, bok, bmissing := searchSortSpecValue(b, sp)
	if amissing || bmissing {
		if amissing && bmissing {
			return 0
		}
		first := false
		if sp.HasMissing {
			if m, ok := sp.Missing.(string); ok && m == "_first" {
				first = true
			}
		}
		if amissing {
			if first {
				if sp.Desc {
					return 1
				}
				return -1
			}
			if sp.Desc {
				return -1
			}
			return 1
		}
		if first {
			if sp.Desc {
				return -1
			}
			return 1
		}
		if sp.Desc {
			return 1
		}
		return -1
	}
	if !aok && !bok {
		return 0
	}
	if !aok {
		return 1
	}
	if !bok {
		return -1
	}
	return compareSortValues(av, bv)
}

// SortOutputValue is the value a hit reports in "sort" for one sort spec.
func SortOutputValue(h MultiHit, sp SortSpec) any {
	v, ok, missing := searchSortSpecValue(h, sp)
	if ik, isIP := v.(ipSortKey); isIP {
		return ik.Out
	}
	if f, isF := v.(float64); isF && math.IsInf(f, 0) {
		if f > 0 {
			return "Infinity"
		}
		return "-Infinity"
	}
	if missing {
		numeric := false
		if def, ok := mapping.FieldDef(h.Idx, sp.Field); ok {
			t, _ := def["type"].(string)
			numeric = mapping.IsNumericType(t)
		}
		if numeric && !sp.HasMissing {
			switch sp.NumericType {
			case "long", "date_nanos":
				if sp.Desc {
					return int64(-9223372036854775808)
				}
				return int64(9223372036854775807)
			case "integer", "short", "byte":
				if sp.Desc {
					return int64(-2147483648)
				}
				return int64(2147483647)
			}
			if def, ok := mapping.FieldDef(h.Idx, sp.Field); ok {
				t, _ := def["type"].(string)
				if t == "integer" || t == "long" || t == "short" || t == "byte" {
					if sp.Desc {
						return int64(-2147483648)
					}
					return int64(2147483647)
				}
			}
		}
		if !numeric {
			if sp.HasMissing {
				if m, ok := sp.Missing.(string); ok && (m == "_first" || m == "_last") {
					return nil
				}
				return sp.Missing
			}
			return nil
		}
		if sp.HasMissing {
			if m, ok := sp.Missing.(string); ok && (m == "_first" || m == "_last") {
				if m == "_first" {
					return "-Infinity"
				}
				if sp.Desc {
					return "-Infinity"
				}
				return "Infinity"
			}
			return sp.Missing
		}
		if sp.Desc {
			return "-Infinity"
		}
		return "Infinity"
	}
	if ok {
		return javaSortNumber(h, sp, v)
	}
	return nil
}

// javaSortNumber renders a numeric sort value as the Java type of its sort
// field: Double for double / scaled_float fields and number scripts, Float
// for float / half_float fields.
func javaSortNumber(h MultiHit, sp SortSpec, v any) any {
	if f, ok := v.(float64); ok && sp.Field == "_score" {
		return ScoreJSON(f)
	}
	if sp.Field == "_script" {
		if sp.ScriptType == "number" {
			return JavaDoubles(v)
		}
		return v
	}
	switch sp.NumericType {
	case "long", "date", "date_nanos":
		return v
	}
	def, ok := mapping.FieldDef(h.Idx, sp.Field)
	if !ok {
		return v
	}
	t, _ := def["type"].(string)
	return JavaFieldNumber(t, v, sp.NumericType == "double")
}

func nestedSortValues(h MultiHit, sp SortSpec) []any {
	pathParts := strings.Split(sp.NestedPath, ".")
	fieldParts := strings.Split(sp.Field, ".")
	rel := fieldParts
	if len(pathParts) <= len(fieldParts) {
		rel = fieldParts[len(pathParts):]
	}
	var elems []any
	cur := any(h.Doc)
	for _, part := range pathParts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	switch t := cur.(type) {
	case []any:
		elems = t
	case map[string]any:
		elems = []any{t}
	default:
		return nil
	}
	out := []any{}
	for _, elem := range elems {
		em, ok := elem.(map[string]any)
		if !ok {
			continue
		}
		if sp.NestedFilter != nil {
			fake := map[string]any{sp.NestedPath: []any{em}}
			qc := NewQueryCtxForDoc(h.Idx, h.Id+"__nested_sort", fake)
			qc.NestedPath = sp.NestedPath
			qc.NestedRoot = em
			ok, _, err := qc.Eval(sp.NestedFilter)
			if err != nil || !ok {
				continue
			}
		}
		var v any = em
		for _, part := range rel {
			m, ok := v.(map[string]any)
			if !ok {
				v = nil
				break
			}
			v = m[part]
		}
		if v != nil {
			out = append(out, v)
		}
	}
	return out
}

func docRawValue(doc map[string]any, field string) any {
	var cur any = doc
	for _, part := range strings.Split(field, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func parseGeoPoint(v any) (float64, float64, bool) {
	return GeoPointOf(v)
}

func geoDistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func parseGeoOrigins(v any) [][2]float64 {
	if arr, isArr := v.([]any); isArr {
		if len(arr) == 2 {
			if _, ok1 := jsonv.ToFloat(arr[0]); ok1 {
				if _, ok2 := jsonv.ToFloat(arr[1]); ok2 {
					if lat, lon, ok := parseGeoPoint(arr); ok {
						return [][2]float64{{lat, lon}}
					}
				}
			}
		}
		var out [][2]float64
		for _, item := range arr {
			if lat, lon, ok := parseGeoPoint(item); ok {
				out = append(out, [2]float64{lat, lon})
			}
		}
		return out
	}
	if lat, lon, ok := parseGeoPoint(v); ok {
		return [][2]float64{{lat, lon}}
	}
	return nil
}

func geoSortDistance(h MultiHit, sp SortSpec) any {
	origins := parseGeoOrigins(sp.GeoPoint)
	if len(origins) == 0 {
		return math.Inf(1)
	}
	raw := docRawValue(h.Doc, sp.GeoField)
	if raw == nil {
		return math.Inf(1)
	}
	var candidates []any
	if arr, isArr := raw.([]any); isArr {
		if len(arr) == 2 {
			if _, ok1 := jsonv.ToFloat(arr[0]); ok1 {
				if _, ok2 := jsonv.ToFloat(arr[1]); ok2 {
					candidates = []any{raw}
				}
			}
		}
		if candidates == nil {
			candidates = arr
		}
	} else {
		candidates = []any{raw}
	}
	mult, _ := geoUnitMeters(sp.GeoUnit)
	if mult == 0 {
		mult = 1
	}
	best := math.Inf(1)
	sum := 0.0
	count := 0
	for _, c := range candidates {
		lat, lon, pok := parseGeoPoint(c)
		if !pok {
			continue
		}
		for _, o := range origins {
			d := geoDistanceMeters(o[0], o[1], lat, lon) / mult
			switch sp.GeoMode {
			case "max":
				if math.IsInf(best, 1) || d > best {
					best = d
				}
			case "avg":
				sum += d
				count++
				best = sum / float64(count)
			default:
				if d < best {
					best = d
				}
			}
		}
	}
	if math.IsInf(best, 1) {
		return math.Inf(1)
	}
	return best
}

// IsAfter reports whether a hit sorts after the search_after values.
func IsAfter(h MultiHit, specs []SortSpec, after []any) bool {
	for i, sp := range specs {
		if i >= len(after) {
			break
		}
		av, _, _ := searchSortSpecValue(h, sp)
		cmp := compareSearchAfterValue(h, sp, av, after[i])
		if cmp == 0 {
			continue
		}
		if sp.Desc {
			return cmp < 0
		}
		return cmp > 0
	}
	return false
}

func compareSearchAfterValue(h MultiHit, sp SortSpec, av, bv any) int {
	def, _ := mapping.FieldDef(h.Idx, sp.Field)
	ft, _ := def["type"].(string)
	if ft == "date" || ft == "date_nanos" {
		am, ok1 := mapping.DateMillis(h.Idx, sp.Field, ft, av)
		bm, ok2 := mapping.DateMillis(h.Idx, sp.Field, ft, bv)
		if ok1 && ok2 {
			switch {
			case am < bm:
				return -1
			case am > bm:
				return 1
			}
			return 0
		}
	}
	if sp.NumericType == "long" || sp.NumericType == "date_nanos" {
		af, _ := jsonv.ToFloat(av)
		bf, _ := jsonv.ToFloat(bv)
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		}
		return 0
	}
	return compareSortValues(av, bv)
}

// HashString is the 32-bit FNV-1a hash of s.
func HashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
