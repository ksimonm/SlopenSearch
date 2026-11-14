package aggs

import (
	"fmt"
	"math"
	"sort"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/search"
)

func matrixStatsAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	fields := search.StringList(spec["fields"])
	cols := map[string][]float64{}
	for _, f := range fields {
		cols[f] = []float64{}
	}
	selected := [][]float64{}
	for _, d := range docs {
		row := []float64{}
		ok := true
		for _, f := range fields {
			v, exists := aggFieldValue(d, f, pathPrefix)
			if !exists {
				ok = false
				break
			}
			n, valid := jsonv.ToFloat(v)
			if !valid {
				ok = false
				break
			}
			row = append(row, n)
		}
		if !ok {
			continue
		}
		selected = append(selected, row)
		for i, f := range fields {
			cols[f] = append(cols[f], row[i])
		}
	}
	n := len(selected)
	entries := []any{}
	for _, f := range fields {
		vals := cols[f]
		mean := meanF(vals)
		variance := 0.0
		if len(vals) > 1 {
			s := 0.0
			for _, v := range vals {
				s += (v - mean) * (v - mean)
			}
			variance = s / float64(len(vals)-1)
		}
		m2, m3, m4 := 0.0, 0.0, 0.0
		for _, v := range vals {
			d := v - mean
			m2 += d * d
			m3 += d * d * d
			m4 += d * d * d * d
		}
		skew := 0.0
		kurt := 0.0
		if len(vals) > 0 && m2 != 0 {
			skew = (m3 / float64(len(vals))) / math.Pow(m2/float64(len(vals)), 1.5)
			kurt = (m4 / float64(len(vals))) / math.Pow(m2/float64(len(vals)), 2)
		}
		cov := ojson.Object{}
		corr := ojson.Object{}
		for _, g := range fields {
			if n == 0 {
				cov = append(cov, ojson.KV{g, jsonv.Float(0)})
				corr = append(corr, ojson.KV{g, jsonv.Float(1)})
				continue
			}
			c := 0.0
			gi := indexOfString(fields, g)
			fi := indexOfString(fields, f)
			for _, row := range selected {
				c += (row[fi] - mean) * (row[gi] - meanF(cols[g]))
			}
			if n > 1 {
				c /= float64(n - 1)
			}
			cov = append(cov, ojson.KV{g, jsonv.Float(c)})
			vg := 0.0
			if len(cols[g]) > 1 {
				for _, v := range cols[g] {
					vg += (v - meanF(cols[g])) * (v - meanF(cols[g]))
				}
				vg /= float64(len(cols[g]) - 1)
			}
			cc := 1.0
			if variance > 0 && vg > 0 {
				cc = c / math.Sqrt(variance*vg)
			}
			corr = append(corr, ojson.KV{g, jsonv.Float(cc)})
		}
		entries = append(entries, ojson.Object{
			{"name", f}, {"count", len(vals)}, {"mean", jsonv.Float(mean)}, {"variance", jsonv.Float(variance)},
			{"skewness", jsonv.Float(skew)}, {"kurtosis", jsonv.Float(kurt)},
			{"covariance", cov}, {"correlation", corr},
		})
	}
	return ojson.Object{{"doc_count", n}, {"fields", entries}}, nil
}

func indexOfString(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

func geoBoundsAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	lats, lons := []float64{}, []float64{}
	for _, d := range docs {
		v, ok := aggFieldValue(d, field, pathPrefix)
		if !ok {
			continue
		}
		lat, lon, ok := search.GeoPointOf(v)
		if !ok {
			continue
		}
		lats = append(lats, lat)
		lons = append(lons, lon)
	}
	if len(lats) == 0 {
		return map[string]any{}, nil
	}
	wrap := true
	if w, ok := spec["wrap_longitude"].(bool); ok {
		wrap = w
	}
	minLat, maxLat := lats[0], lats[0]
	minLon, maxLon := lons[0], lons[0]
	for i := range lats {
		if lats[i] < minLat {
			minLat = lats[i]
		}
		if lats[i] > maxLat {
			maxLat = lats[i]
		}
		if lons[i] < minLon {
			minLon = lons[i]
		}
		if lons[i] > maxLon {
			maxLon = lons[i]
		}
	}
	if wrap && maxLon-minLon > 180 {
		shifted := make([]float64, len(lons))
		for i, lon := range lons {
			if lon < 0 {
				shifted[i] = lon + 360
			} else {
				shifted[i] = lon
			}
		}
		sm, sx := shifted[0], shifted[0]
		for _, lon := range shifted {
			if lon < sm {
				sm = lon
			}
			if lon > sx {
				sx = lon
			}
		}
		if sx > 180 {
			sx -= 360
		}
		minLon, maxLon = sm, sx
	}
	return ojson.Object{{"bounds", ojson.Object{
		{"top_left", ojson.Object{{"lat", jsonv.Float(maxLat)}, {"lon", jsonv.Float(minLon)}}},
		{"bottom_right", ojson.Object{{"lat", jsonv.Float(minLat)}, {"lon", jsonv.Float(maxLon)}}},
	}}}, nil
}

func geoCentroidAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	latSum, lonSum := 0.0, 0.0
	count := 0
	for _, d := range docs {
		v, ok := aggFieldValue(d, field, pathPrefix)
		if !ok {
			continue
		}
		lat, lon, ok := search.GeoPointOf(v)
		if !ok {
			continue
		}
		latSum += lat
		lonSum += lon
		count++
	}
	out := ojson.Object{{"count", count}}
	if count > 0 {
		out = append(out, ojson.KV{"location", ojson.Object{{"lat", jsonv.Float(latSum / float64(count))}, {"lon", jsonv.Float(lonSum / float64(count))}}})
	}
	return out, nil
}

func topHitsAgg(spec map[string]any, docs []map[string]any, idx *model.Index) (any, *apierr.Error) {
	if sub := subAggsOf(spec); sub != nil {
		return nil, &apierr.Error{Type: "aggregation_initialization_exception", Status: 500,
			Reason: "Aggregator [a] of type [top_hits] cannot accept sub-aggregations"}
	}
	from := jsonv.Int(spec, "from", 0)
	size := jsonv.Int(spec, "size", 3)
	if from+size > 100 {
		reason := fmt.Sprintf("Top hits result window is too large, the top hits aggregator [a]'s from + size must be less than or equal to: [100] but was [%d]. This limit can be set by changing the [index.max_inner_result_window] index level setting.", from+size)
		return nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: reason,
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
	}
	sorts := parseSortList(spec["sort"])
	if len(sorts) > 0 {
		sort.SliceStable(docs, func(i, j int) bool {
			for _, s := range sorts {
				av, aok := sortValue(docs[i], s.field, idx)
				bv, bok := sortValue(docs[j], s.field, idx)
				if !aok || !bok {
					if aok != bok {
						return aok
					}
					continue
				}
				cmp := compareAggSortValues(av, bv)
				if cmp != 0 {
					if s.desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
	}
	total := len(docs)
	start := from
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	trackScores, _ := spec["track_scores"].(bool)
	includeSource, sourceIncludes, sourceExcludes := parseTopHitsSource(spec["_source"])
	storedNone := false
	_, sourceExplicit := spec["_source"]
	if sf, ok := spec["stored_fields"]; ok {
		names := search.StringList(sf)
		for _, n := range names {
			if n == "_none_" {
				storedNone = true
			}
		}
		if storedNone && !sourceExplicit {
			includeSource = false
		}
	}
	hits := []any{}
	maxScore := 0.0
	hasScores := len(sorts) == 0 || trackScores
	for _, d := range docs {
		sc, _ := jsonv.ToFloat(d["__slop_score"])
		if sc > maxScore {
			maxScore = sc
		}
	}
	for i := start; i < end; i++ {
		d := docs[i]
		id, _ := d["__slop_id"].(string)
		sc, _ := jsonv.ToFloat(d["__slop_score"])
		h := ojson.Object{}
		if !storedNone {
			h = append(h, ojson.KV{"_index", idx.Name}, ojson.KV{"_id", id})
		}
		if hasScores {
			h = append(h, ojson.KV{"_score", jsonv.Float(sc)})
		} else {
			h = append(h, ojson.KV{"_score", nil})
		}
		if includeSource {
			src := search.FilterSource(d, sourceIncludes, sourceExcludes)
			delete(src, "__slop_id")
			delete(src, "__slop_score")
			h = append(h, ojson.KV{"_source", jsonv.NormalizeNumbers(src)})
		}
		if v, ok := spec["version"].(bool); ok && v {
			ver := int64(1)
			if idx.Versions != nil {
				if x, ok := idx.Versions[id]; ok {
					ver = x
				}
			}
			h = append(h, ojson.KV{"_version", ver})
		}
		if v, ok := spec["seq_no_primary_term"].(bool); ok && v {
			seq := int64(0)
			if idx.SeqNos != nil {
				if x, ok := idx.SeqNos[id]; ok {
					seq = x
				}
			}
			pt := int64(1)
			if idx.PrimaryTerms != nil {
				if x, ok := idx.PrimaryTerms[id]; ok {
					pt = x
				}
			}
			h = append(h, ojson.KV{"_seq_no", seq}, ojson.KV{"_primary_term", pt})
		}
		if len(sorts) > 0 {
			sv := []any{}
			for _, s := range sorts {
				v, _ := sortValue(d, s.field, idx)
				sv = append(sv, jsonv.NormalizeNumbers(v))
			}
			h = append(h, ojson.KV{"sort", sv})
		}
		fields := ojson.Object{}
		if dv, ok := spec["docvalue_fields"]; ok {
			for _, f := range search.StringList(dv) {
				if v, ok := mapping.FieldValue(d, f); ok {
					fields = append(fields, ojson.KV{f, []any{jsonv.NormalizeNumbers(v)}})
				}
			}
		}
		if sf, ok := spec["script_fields"].(map[string]any); ok {
			got, serr := search.EvalScriptFields(search.MultiHit{Idx: idx, Id: id, Doc: d}, sf)
			if serr != nil {
				return nil, serr
			}
			for _, name := range jsonv.SortedKeys(got) {
				fields = append(fields, ojson.KV{name, got[name]})
			}
		}
		if len(fields) > 0 {
			h = append(h, ojson.KV{"fields", fields})
		}
		if hm, ok := spec["highlight"].(map[string]any); ok {
			if hf, ok := hm["fields"].(map[string]any); ok {
				terms, _ := d["__slop_query_terms"].([]string)
				hl := ojson.Object{}
				for fname := range hf {
					txt, ok := jsonv.ToString(d[fname])
					if !ok {
						continue
					}
					hl = append(hl, ojson.KV{fname, []any{search.HighlightText(txt, terms)}})
				}
				if len(hl) > 0 {
					h = append(h, ojson.KV{"highlight", hl})
				}
			}
		}
		hits = append(hits, h)
	}
	var max any
	if hasScores && total > 0 {
		max = jsonv.Float(maxScore)
	}
	return ojson.Object{{"hits", ojson.Object{
		{"total", ojson.Object{{"value", total}, {"relation", "eq"}}},
		{"max_score", max},
		{"hits", hits},
	}}}, nil
}

type sortSpec struct {
	field string
	desc  bool
}

func parseSortList(raw any) []sortSpec {
	out := []sortSpec{}
	switch t := raw.(type) {
	case []any:
		for _, e := range t {
			out = append(out, parseSortEntry(e)...)
		}
	default:
		out = append(out, parseSortEntry(raw)...)
	}
	return out
}

func parseSortEntry(raw any) []sortSpec {
	switch t := raw.(type) {
	case string:
		return []sortSpec{{field: t}}
	case map[string]any:
		out := []sortSpec{}
		for k, v := range t {
			s := sortSpec{field: k}
			switch vv := v.(type) {
			case string:
				s.desc = vv == "desc"
			case map[string]any:
				if o, _ := jsonv.ToString(vv["order"]); o == "desc" {
					s.desc = true
				}
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

func sortValue(doc map[string]any, field string, idx *model.Index) (any, bool) {
	if field == "_score" {
		return doc["__slop_score"], true
	}
	v, ok := mapping.FieldValue(doc, field)
	if !ok {
		return nil, false
	}
	def, _ := mapping.FieldDef(idx, field)
	ft, _ := def["type"].(string)
	if ft == "date" || ft == "date_nanos" {
		if ms, ok := mapping.DateMillis(idx, field, ft, v); ok {
			return float64(ms), true
		}
	}
	return v, true
}

func compareAggSortValues(a, b any) int {
	af, aok := jsonv.ToFloat(a)
	bf, bok := jsonv.ToFloat(b)
	if aok && bok {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	as, _ := jsonv.ToString(a)
	bs, _ := jsonv.ToString(b)
	if as < bs {
		return -1
	}
	if as > bs {
		return 1
	}
	return 0
}

func parseTopHitsSource(raw any) (bool, []string, []string) {
	if raw == nil {
		return true, nil, nil
	}
	switch t := raw.(type) {
	case bool:
		return t, nil, nil
	case string:
		if t == "false" {
			return false, nil, nil
		}
		if t == "true" || t == "" {
			return true, nil, nil
		}
		return true, search.SplitSourceList(t), nil
	case []any:
		return true, search.StringList(t), nil
	case map[string]any:
		inc := search.StringList(t["includes"])
		if len(inc) == 0 {
			inc = search.StringList(t["include"])
		}
		exc := search.StringList(t["excludes"])
		if len(exc) == 0 {
			exc = search.StringList(t["exclude"])
		}
		return true, inc, exc
	}
	return true, nil, nil
}
