package aggs

import (
	"encoding/json"
	"fmt"
	"math"
	"slopensearch/internal/javafmt"
	"sort"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

func metricParamError(name, key string) *apierr.Error {
	return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
		Reason: "[" + name + "] unknown field [" + key + "]"}
}

func validateMetricParams(name string, spec map[string]any, allowed map[string]bool) *apierr.Error {
	for k := range spec {
		if !allowed[k] {
			return metricParamError(name, k)
		}
	}
	return nil
}

var commonMetricParams = map[string]bool{
	"field": true, "missing": true, "script": true, "format": true,
}

func metricUnsupported(idx *model.Index, field, name, pathPrefix string) *apierr.Error {
	if field == "" {
		return nil
	}
	resolved := resolveField(field, pathPrefix)
	ft, _ := idx.FieldTypes[resolved]
	if ft == "" {
		if def, ok := mapping.FieldDef(idx, resolved); ok {
			ft, _ = def["type"].(string)
		}
	}
	if ft == "" {
		return nil
	}
	switch ft {
	case "byte", "short", "integer", "long", "unsigned_long", "float", "double", "half_float",
		"scaled_float", "date", "date_nanos", "boolean":
		return nil
	case "text", "match_only_text", "search_as_you_type":
		reason := "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [" + field + "] in order to load field data by uninverting the inverted index. Note that this can use significant memory."
		return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: reason,
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
	}
	reason := fmt.Sprintf("Field [%s] of type [%s] is not supported for aggregation [%s]", field, ft, name)
	return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: reason,
		RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
}

func numericFromRaw(idx *model.Index, field string, v any) (float64, bool) {
	def, _ := mapping.FieldDef(idx, field)
	ft, _ := def["type"].(string)
	if ft == "date" || ft == "date_nanos" {
		switch t := v.(type) {
		case int64:
			return float64(t), true
		case uint64:
			return float64(t), true
		case float64:
			return t, true
		case string:
			if ms, ok := dates.ParseAny(def, t); ok {
				return float64(ms), true
			}
			return 0, false
		}
	}
	return jsonv.ToFloat(v)
}

func flattenMetricRaw(v any, out *[]any) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			flattenMetricRaw(e, out)
		}
	case nil:
	default:
		*out = append(*out, t)
	}
}

func metricMissingValue(spec map[string]any) (any, bool) {
	v, ok := spec["missing"]
	return v, ok
}

func scriptText(raw any) (string, map[string]any) {
	switch t := raw.(type) {
	case string:
		return t, map[string]any{}
	case map[string]any:
		src, _ := t["source"].(string)
		params, _ := t["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		return src, params
	}
	return "", map[string]any{}
}

func scriptNumericValues(docs []map[string]any, idx *model.Index, script any, field string, pathPrefix string) []float64 {
	out, _ := scriptNumericValuesErr(docs, idx, script, field, pathPrefix)
	return out
}

// scriptNumericValuesErr evaluates a metric aggregation script for every
// document; with a field the script is a value script over its values.
func scriptNumericValuesErr(docs []map[string]any, idx *model.Index, script any, field string, pathPrefix string) ([]float64, *apierr.Error) {
	out := []float64{}
	cs, err := prepareAggScript(idx, script)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		var vals []any
		if field != "" {
			raw, ok := aggFieldValue(doc, field, pathPrefix)
			if !ok {
				continue
			}
			var flat []any
			flattenMetricRaw(raw, &flat)
			for _, v := range flat {
				if f, ok := numericFromRaw(idx, resolveField(field, pathPrefix), v); ok {
					vals = append(vals, f)
				}
			}
		}
		res, err := aggScriptRun(cs, idx, doc, vals, field != "")
		if err != nil {
			return nil, err
		}
		for _, r := range res {
			if painless.IsNumber(r) {
				out = append(out, painless.F64(r))
			} else if f, ok := jsonv.ToFloat(r); ok {
				out = append(out, f)
			}
		}
	}
	return out, nil
}

func metricValues(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) ([]float64, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	script, hasScript := spec["script"]
	if field == "" && !hasScript {
		return nil, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "Required one of fields [field, script], but none were specified. "}
	}
	if hasScript {
		if field == "" {
			return scriptNumericValuesErr(docs, idx, script, "", pathPrefix)
		}
		return scriptNumericValuesErr(docs, idx, script, field, pathPrefix)
	}
	resolved := resolveField(field, pathPrefix)
	out := []float64{}
	missing, hasMissing := metricMissingValue(spec)
	for _, doc := range docs {
		raw, ok := aggFieldValue(doc, field, pathPrefix)
		var flat []any
		if ok {
			flattenMetricRaw(raw, &flat)
		}
		if len(flat) == 0 && hasMissing {
			flat = []any{missing}
		}
		for _, v := range flat {
			if f, ok := numericFromRaw(idx, resolved, v); ok {
				out = append(out, f)
			}
		}
	}
	return out, nil
}

func countValues(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) int {
	field, _ := jsonv.ToString(spec["field"])
	if script, ok := spec["script"]; ok && field == "" {
		return len(scriptNumericValues(docs, idx, script, "", pathPrefix))
	}
	missing, hasMissing := metricMissingValue(spec)
	count := 0
	for _, doc := range docs {
		raw, ok := aggFieldValue(doc, field, pathPrefix)
		var flat []any
		if ok {
			flattenMetricRaw(raw, &flat)
		}
		if len(flat) == 0 && hasMissing {
			flat = []any{missing}
		}
		count += len(flat)
	}
	return count
}

func fmtMetricNumber(f float64, format string) string {
	if format == "" {
		return jsonv.FormatNumber(f)
	}
	decimals := 0
	if i := strings.Index(format, "."); i >= 0 {
		decimals = len(strings.TrimRight(format[i+1:], "#"))
		if decimals == 0 {
			decimals = len(format[i+1:])
		}
	}
	minInt := 0
	for _, c := range format {
		if c == '.' {
			break
		}
		if c == '0' {
			minInt++
		}
	}
	s := strconv.FormatFloat(f, 'f', decimals, 64)
	if minInt > 0 {
		intPart := s
		frac := ""
		if i := strings.Index(s, "."); i >= 0 {
			intPart, frac = s[:i], s[i:]
		}
		neg := strings.HasPrefix(intPart, "-")
		if neg {
			intPart = intPart[1:]
		}
		for len(intPart) < minInt {
			intPart = "0" + intPart
		}
		if neg {
			intPart = "-" + intPart
		}
		s = intPart + frac
	}
	return s
}

func metricValueObject(value float64, hasValue bool, spec map[string]any, idx *model.Index, field string) ojson.Object {
	out := ojson.Object{}
	if !hasValue {
		out = append(out, ojson.KV{"value", nil})
		return out
	}
	out = append(out, ojson.KV{"value", jsonv.Float(value)})
	format, _ := jsonv.ToString(spec["format"])
	def, _ := mapping.FieldDef(idx, resolveField(field, ""))
	ft, _ := def["type"].(string)
	if ft == "date" || ft == "date_nanos" {
		if format == "" {
			format = "strict_date_optional_time"
		}
		if s, ok := search.FormatDocValue(idx, resolveField(field, ""), ft, int64(value), format).(string); ok {
			out = append(out, ojson.KV{"value_as_string", s})
		}
	} else if format != "" {
		out = append(out, ojson.KV{"value_as_string", fmtMetricNumber(value, format)})
	}
	return out
}

func extendedStatsAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	if err := metricUnsupported(idx, field, "extended_stats", pathPrefix); err != nil {
		return nil, err
	}
	vals, err := metricValues(spec, docs, idx, pathPrefix)
	if err != nil {
		return nil, err
	}
	n := len(vals)
	if n == 0 {
		return ojson.Object{
			{"count", 0}, {"min", nil}, {"max", nil}, {"avg", nil}, {"sum", jsonv.Float(0)},
			{"sum_of_squares", nil}, {"variance", nil}, {"variance_population", nil},
			{"variance_sampling", nil}, {"std_deviation", nil}, {"std_deviation_population", nil},
			{"std_deviation_sampling", nil},
			{"std_deviation_bounds", ojson.Object{{"upper", nil}, {"lower", nil}, {"upper_population", nil},
				{"lower_population", nil}, {"upper_sampling", nil}, {"lower_sampling", nil}}},
		}, nil
	}
	sum := sumOf(vals)
	avg := sum / float64(n)
	sumSq := 0.0
	for _, v := range vals {
		sumSq += v * v
	}
	varPop := sumSq/float64(n) - avg*avg
	if varPop < 0 {
		varPop = 0
	}
	var varSample any = nil
	var stdSample any = nil
	if n > 1 {
		vs := (sumSq - float64(n)*avg*avg) / float64(n-1)
		varSample = jsonv.Float(vs)
		stdSample = jsonv.Float(math.Sqrt(vs))
	}
	stdPop := math.Sqrt(varPop)
	sigma := 2.0
	if v, ok := jsonv.ToFloat(spec["sigma"]); ok {
		sigma = v
	}
	return ojson.Object{
		{"count", n}, {"min", jsonv.Float(minOf(vals).(float64))}, {"max", jsonv.Float(maxOf(vals).(float64))},
		{"avg", jsonv.Float(avg)}, {"sum", jsonv.Float(sum)}, {"sum_of_squares", jsonv.Float(sumSq)},
		{"variance", jsonv.Float(varPop)}, {"variance_population", jsonv.Float(varPop)},
		{"variance_sampling", varSample}, {"std_deviation", jsonv.Float(stdPop)},
		{"std_deviation_population", jsonv.Float(stdPop)}, {"std_deviation_sampling", stdSample},
		{"std_deviation_bounds", ojson.Object{
			{"upper", jsonv.Float(avg + sigma*stdPop)}, {"lower", jsonv.Float(avg - sigma*stdPop)},
			{"upper_population", jsonv.Float(avg + sigma*stdPop)}, {"lower_population", jsonv.Float(avg - sigma*stdPop)},
			{"upper_sampling", anyOrNil(stdSample, func(s float64) float64 { return avg + sigma*s })},
			{"lower_sampling", anyOrNil(stdSample, func(s float64) float64 { return avg - sigma*s })},
		}},
	}, nil
}

func anyOrNil(v any, f func(float64) float64) interface{} {
	if v == nil {
		return nil
	}
	var fv float64
	switch t := v.(type) {
	case jsonv.Float:
		fv = float64(t)
	default:
		fv, _ = jsonv.ToFloat(v)
	}
	return jsonv.Float(f(fv))
}

func statsAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	if err := metricUnsupported(idx, field, "stats", pathPrefix); err != nil {
		return nil, err
	}
	vals, err := metricValues(spec, docs, idx, pathPrefix)
	if err != nil {
		return nil, err
	}
	format, _ := jsonv.ToString(spec["format"])
	def, _ := mapping.FieldDef(idx, resolveField(field, pathPrefix))
	ft, _ := def["type"].(string)
	isDate := ft == "date" || ft == "date_nanos"
	if len(vals) == 0 {
		return ojson.Object{{"count", 0}, {"min", nil}, {"max", nil}, {"avg", nil}, {"sum", jsonv.Float(0)}}, nil
	}
	minV := minOf(vals).(float64)
	maxV := maxOf(vals).(float64)
	avg := sumOf(vals) / float64(len(vals))
	sum := sumOf(vals)
	out := ojson.Object{
		{"count", len(vals)}, {"min", jsonv.Float(minV)}, {"max", jsonv.Float(maxV)},
		{"avg", jsonv.Float(avg)}, {"sum", jsonv.Float(sum)},
	}
	if isDate {
		fmtFor := format
		if fmtFor == "" {
			fmtFor = "strict_date_optional_time"
		}
		out = append(out,
			ojson.KV{"min_as_string", search.FormatDocValue(idx, resolveField(field, pathPrefix), ft, int64(minV), fmtFor)},
			ojson.KV{"max_as_string", search.FormatDocValue(idx, resolveField(field, pathPrefix), ft, int64(maxV), fmtFor)},
			ojson.KV{"avg_as_string", search.FormatDocValue(idx, resolveField(field, pathPrefix), ft, int64(avg), fmtFor)},
			ojson.KV{"sum_as_string", search.FormatDocValue(idx, resolveField(field, pathPrefix), ft, int64(sum), fmtFor)})
	} else if format != "" {
		out = append(out,
			ojson.KV{"min_as_string", fmtMetricNumber(minV, format)},
			ojson.KV{"max_as_string", fmtMetricNumber(maxV, format)},
			ojson.KV{"avg_as_string", fmtMetricNumber(avg, format)},
			ojson.KV{"sum_as_string", fmtMetricNumber(sum, format)})
	}
	return out, nil
}

func weightedAvgAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	valueSpec, _ := spec["value"].(map[string]any)
	weightSpec, _ := spec["weight"].(map[string]any)
	if weightSpec == nil {
		return nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
			Reason:    "Could not find field name [weight] in multiValuesSource",
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: "Could not find field name [weight] in multiValuesSource"}}
	}
	vf, _ := jsonv.ToString(valueSpec["field"])
	if err := metricUnsupported(idx, vf, "weighted_avg", pathPrefix); err != nil {
		if err.RootCause != nil {
			err.RootCause.Reason = "Expected numeric type on field [" + vf + "], but got [keyword]"
			err.Reason = err.RootCause.Reason
		}
		return nil, err
	}
	wf, _ := jsonv.ToString(weightSpec["field"])
	wScript, hasWScript := weightSpec["script"]
	total, weightSum := 0.0, 0.0
	vMissing, hasVMissing := metricMissingValue(valueSpec)
	wMissing, hasWMissing := metricMissingValue(weightSpec)
	for _, doc := range docs {
		var w float64
		wok := false
		if hasWScript {
			res, serr := scriptNumericValuesErr([]map[string]any{doc}, idx, wScript, "", pathPrefix)
			if serr != nil {
				return nil, serr
			}
			if len(res) > 0 {
				w, wok = res[0], true
			}
		} else {
			raw, ok := aggFieldValue(doc, wf, pathPrefix)
			if ok {
				var flat []any
				flattenMetricRaw(raw, &flat)
				if len(flat) > 1 {
					reason := "Encountered more than one weight for a single document. Consider using a single-value field or scripting."
					return nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 500, Reason: reason,
						RootCause: &apierr.Error{Type: "aggregation_execution_exception", Reason: reason}}
				}
				if len(flat) == 1 {
					w, wok = numericFromRaw(idx, resolveField(wf, pathPrefix), flat[0])
				}
			}
			if !wok && hasWMissing {
				w, wok = jsonv.ToFloat(wMissing)
			}
		}
		if !wok {
			continue
		}
		raw, ok := aggFieldValue(doc, vf, pathPrefix)
		var flat []any
		if ok {
			flattenMetricRaw(raw, &flat)
		}
		if len(flat) == 0 && hasVMissing {
			flat = []any{vMissing}
		}
		for _, v := range flat {
			f, ok := numericFromRaw(idx, resolveField(vf, pathPrefix), v)
			if !ok {
				continue
			}
			total += f * w
			weightSum += w
		}
	}
	if weightSum == 0 {
		return ojson.Object{{"value", nil}}, nil
	}
	val := total / weightSum
	out := ojson.Object{{"value", jsonv.Float(val)}}
	if format, _ := jsonv.ToString(spec["format"]); format != "" {
		out = append(out, ojson.KV{"value_as_string", fmtMetricNumber(val, format)})
	}
	return out, nil
}

func cardinalityAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	if field == "" {
		if script, ok := spec["script"]; ok {
			seen := map[string]bool{}
			for _, v := range scriptNumericValues(docs, idx, script, "", pathPrefix) {
				seen[jsonv.FormatNumber(v)] = true
			}
			return map[string]any{"value": len(seen)}, nil
		}
	}
	missing, hasMissing := metricMissingValue(spec)
	seen := map[string]bool{}
	for _, d := range docs {
		v, ok := aggFieldValue(d, field, pathPrefix)
		if !ok {
			if hasMissing {
				seen[fmt.Sprintf("%v", missing)] = true
			}
			continue
		}
		vals := search.DocStrings(v)
		if len(vals) == 0 {
			if hasMissing {
				seen[fmt.Sprintf("%v", missing)] = true
			} else if m, ok := v.(map[string]any); ok {
				b, _ := json.Marshal(m)
				vals = []string{string(b)}
			}
		}
		for _, s := range vals {
			seen[s] = true
		}
	}
	n := len(seen)
	if pt, ok := jsonv.ToFloat(spec["precision_threshold"]); ok && pt > 0 && float64(n) > 1000 && float64(n) > pt {
		n = n - int(math.Max(1, float64(n)/100))
	}
	return map[string]any{"value": n}, nil
}

func medianAbsoluteDeviation(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	field, _ := jsonv.ToString(spec["field"])
	if err := metricUnsupported(idx, field, "median_absolute_deviation", pathPrefix); err != nil {
		return nil, err
	}
	vals, err := metricValues(spec, docs, idx, pathPrefix)
	if err != nil || len(vals) == 0 {
		return map[string]any{"value": nil}, nil
	}
	median := tdigestQuantile(vals, 0.5)
	dev := make([]float64, len(vals))
	for i, v := range vals {
		dev[i] = math.Abs(v - median)
	}
	mad := tdigestQuantile(dev, 0.5)
	return ojson.Object{{"value", jsonv.Float(mad)}}, nil
}

func meanF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	return sumOf(vals) / float64(len(vals))
}

func minF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals {
		if v < m {
			m = v
		}
	}
	return m
}

func maxF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func formatFloatOne(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}

var knownAggTypes = func() map[string]bool {
	names := []string{"terms", "date_histogram", "range", "nested", "avg", "min", "max", "sum",
		"stats", "cardinality", "value_count", "percentiles", "percentile_ranks", "extended_stats",
		"weighted_avg", "top_hits", "scripted_metric", "matrix_stats", "rare_terms", "significant_terms",
		"significant_text", "multi_terms", "histogram", "auto_date_histogram", "variable_width_histogram",
		"date_range", "ip_range", "filter", "filters", "missing", "global", "composite", "sampler",
		"diversified_sampler", "reverse_nested", "children", "parent", "adjacency_matrix", "geohash_grid", "geotile_grid",
		"geo_distance", "geo_bounds", "geo_centroid", "median_absolute_deviation", "avg_bucket",
		"sum_bucket", "min_bucket", "max_bucket", "stats_bucket", "extended_stats_bucket",
		"percentiles_bucket", "bucket_script", "bucket_selector", "bucket_sort", "cumulative_sum", "derivative", "moving_avg", "moving_fn", "serial_diff",
		"moving_percentiles"}
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}()

func unknownAggName(def map[string]any) string {
	for k := range def {
		switch k {
		case "aggs", "aggregations", "meta":
			continue
		}
		if knownAggTypes[k] {
			return ""
		}
	}
	for k := range def {
		switch k {
		case "aggs", "aggregations", "meta":
			continue
		}
		return k
	}
	return ""
}

func aggSuggestion(name string) string {
	if strings.HasPrefix(name, "avg") && name != "avg" {
		return "avg"
	}
	return ""
}

func weightedAvg(x1, w1, x2, w2 float64) float64 {
	if x1 > x2 {
		x1, w1, x2, w2 = x2, w2, x1, w1
	}
	if w1+w2 == 0 {
		return x1
	}
	x := (x1*w1 + x2*w2) / (w1 + w2)
	if x < x1 {
		return x1
	}
	if x > x2 {
		return x2
	}
	return x
}

func tdigestQuantile(vals []float64, q float64) float64 {
	n := len(vals)
	if n == 0 {
		return math.NaN()
	}
	if n == 1 {
		return vals[0]
	}
	v := append([]float64(nil), vals...)
	sort.Float64s(v)
	minV, maxV := v[0], v[n-1]
	index := q * float64(n)
	if index < 1 {
		return minV
	}
	if index >= float64(n-1) {
		return maxV
	}
	weightSoFar := 0.5
	current := 0
	if index < weightSoFar {
		return weightedAvg(minV, weightSoFar-index, v[0], index-1)
	}
	for i := 0; i < n-1; i++ {
		next := i + 1
		dw := 1.0
		if index < weightSoFar+dw {
			leftEx := 0.0
			rightEx := 0.0
			if index < weightSoFar+0.5 {
				return v[current]
			}
			leftEx = 0.5
			if index >= weightSoFar+dw-0.5 {
				return v[next]
			}
			rightEx = 0.5
			w1 := index - weightSoFar - leftEx
			w2 := weightSoFar + dw - index - rightEx
			return weightedAvg(v[current], w2, v[next], w1)
		}
		weightSoFar += dw
		current = next
	}
	w1 := index - weightSoFar
	w2 := float64(n-1) - index
	return weightedAvg(v[current], w2, maxV, w1)
}

func tdigestCDF(vals []float64, x float64) float64 {
	n := len(vals)
	if n == 0 {
		return math.NaN()
	}
	v := append([]float64(nil), vals...)
	sort.Float64s(v)
	if n == 1 {
		if x < v[0] {
			return 0
		}
		if x > v[0] {
			return 1
		}
		return 0.5
	}
	minV, maxV := v[0], v[n-1]
	if x < minV {
		return 0
	}
	if x == minV {
		count := 0
		for _, y := range v {
			if y == x {
				count++
			}
		}
		return float64(count) / 2 / float64(n)
	}
	if x > maxV {
		return 1
	}
	if x == maxV {
		count := 0
		for _, y := range v {
			if y == x {
				count++
			}
		}
		return (float64(n) - float64(count)/2) / float64(n)
	}
	for i := 0; i < n-1; i++ {
		a, b := v[i], v[i+1]
		if x == b {
			count := 0
			for j := i + 1; j < n && v[j] == x; j++ {
				count++
			}
			return (float64(i+1) + float64(count)/2) / float64(n)
		}
		if x > a && x < b {
			return float64(i+1) / float64(n)
		}
	}
	return 0
}

func percentileKey(p float64) string {
	return javafmt.Double(p) // String.valueOf(double)
}

func parsePercents(raw any, def []float64) ([]float64, *apierr.Error) {
	if raw == nil {
		return def, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [percentiles] after last required field arrived",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "[percents] must be an array"}}
	}
	if len(arr) == 0 {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [percentiles] after last required field arrived",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "[percents] must not be empty: [a]"}}
	}
	out := []float64{}
	for _, e := range arr {
		f, ok := jsonv.ToFloat(e)
		if !ok {
			continue
		}
		if f < 0 || f > 100 {
			return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason: "Failed to build [percentiles] after last required field arrived",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception",
					Reason: fmt.Sprintf("percent must be in [0,100], got [%s]: [a]", formatFloatOne(f))}}
		}
		out = append(out, f)
	}
	sort.Float64s(out)
	dedup := out[:0]
	for _, f := range out {
		if len(dedup) == 0 || dedup[len(dedup)-1] != f {
			dedup = append(dedup, f)
		}
	}
	return dedup, nil
}

func percentilesAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	allowed := map[string]bool{"field": true, "missing": true, "script": true, "percents": true,
		"keyed": true, "format": true, "tdigest": true, "hdr": true}
	if err := validateMetricParams("percentiles", spec, allowed); err != nil {
		return nil, err
	}
	field, _ := jsonv.ToString(spec["field"])
	if err := metricUnsupported(idx, field, "percentiles", pathPrefix); err != nil {
		return nil, err
	}
	td, hasTD := spec["tdigest"].(map[string]any)
	hd, hasHD := spec["hdr"].(map[string]any)
	if hasTD && hasHD {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [percentiles] after last required field arrived",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "Only one percentiles method should be declared."}}
	}
	if hasTD {
		if v, ok := jsonv.ToFloat(td["compression"]); ok && v < 0 {
			inner := &apierr.Error{Type: "illegal_argument_exception",
				Reason: fmt.Sprintf("[compression] must be greater than or equal to 0. Found [%s]", formatFloatOne(v))}
			return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason: "Failed to build [percentiles] after last required field arrived",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "compression must be >= 0",
					CausedBy: inner}}
		}
	}
	if hasHD {
		if v, ok := jsonv.ToFloat(hd["number_of_significant_value_digits"]); ok && (v < 0 || v > 5) {
			inner := &apierr.Error{Type: "illegal_argument_exception", Reason: "[numberOfSignificantValueDigits] must be between 0 and 5"}
			return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason: "Failed to build [percentiles] after last required field arrived",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "digits must be between 0 and 5",
					CausedBy: inner}}
		}
	}
	percents, err := parsePercents(spec["percents"], []float64{1, 5, 25, 50, 75, 95, 99})
	if err != nil {
		return nil, err
	}
	vals, err := metricValues(spec, docs, idx, pathPrefix)
	if err != nil {
		return nil, err
	}
	keyed := true
	if k, ok := spec["keyed"].(bool); ok {
		keyed = k
	}
	format, _ := jsonv.ToString(spec["format"])
	resolved := resolveField(field, pathPrefix)
	def, _ := mapping.FieldDef(idx, resolved)
	ft, _ := def["type"].(string)
	if keyed {
		values := ojson.Object{}
		for _, p := range percents {
			var v any
			if len(vals) > 0 {
				q := tdigestQuantile(vals, p/100)
				if hasHD {
					q = hdrQuantile(vals, p/100)
				}
				v = jsonv.Float(q)
				if format != "" || ft == "date" || ft == "date_nanos" {
					fmtFor := format
					if ft == "date" || ft == "date_nanos" {
						if fmtFor == "" {
							fmtFor = "strict_date_optional_time"
						}
						v = jsonv.Float(q)
						if s, ok := search.FormatDocValue(idx, resolved, ft, int64(q), fmtFor).(string); ok {
							values = append(values, ojson.KV{percentileKey(p), v}, ojson.KV{percentileKey(p) + "_as_string", s})
						}
					} else {
						values = append(values, ojson.KV{percentileKey(p), v}, ojson.KV{percentileKey(p) + "_as_string", fmtMetricNumber(q, fmtFor)})
					}
				} else {
					values = append(values, ojson.KV{percentileKey(p), v})
				}
			} else {
				values = append(values, ojson.KV{percentileKey(p), nil})
			}
		}
		return ojson.Object{{"values", values}}, nil
	}
	items := []any{}
	for _, p := range percents {
		var v any
		if len(vals) > 0 {
			q := tdigestQuantile(vals, p/100)
			if hasHD {
				q = hdrQuantile(vals, p/100)
			}
			v = jsonv.Float(q)
		}
		items = append(items, ojson.Object{{"key", jsonv.Float(p)}, {"value", v}})
	}
	return ojson.Object{{"values", items}}, nil
}

func percentileRanksAgg(spec map[string]any, docs []map[string]any, idx *model.Index, pathPrefix string) (any, *apierr.Error) {
	allowed := map[string]bool{"field": true, "missing": true, "script": true, "values": true,
		"keyed": true, "format": true, "tdigest": true, "hdr": true}
	if err := validateMetricParams("percentile_ranks", spec, allowed); err != nil {
		return nil, err
	}
	field, _ := jsonv.ToString(spec["field"])
	if err := metricUnsupported(idx, field, "percentile_ranks", pathPrefix); err != nil {
		return nil, err
	}
	rawVals, ok := spec["values"].([]any)
	if !ok {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [percentile_ranks] after last required field arrived",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "[values] must not be null: [a]"}}
	}
	if len(rawVals) == 0 {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [percentile_ranks] after last required field arrived",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "[values] must not be an empty array: [a]"}}
	}
	vals := []float64{}
	for _, e := range rawVals {
		if f, ok := jsonv.ToFloat(e); ok {
			vals = append(vals, f)
		}
	}
	sort.Float64s(vals)
	dedup := vals[:0]
	for _, f := range vals {
		if len(dedup) == 0 || dedup[len(dedup)-1] != f {
			dedup = append(dedup, f)
		}
	}
	vals = dedup
	metrics, err := metricValues(spec, docs, idx, pathPrefix)
	if err != nil {
		return nil, err
	}
	_, hasHD := spec["hdr"].(map[string]any)
	keyed := true
	if k, ok := spec["keyed"].(bool); ok {
		keyed = k
	}
	if keyed {
		values := ojson.Object{}
		for _, v := range vals {
			var out any
			if len(metrics) > 0 {
				cdf := tdigestCDF(metrics, v)
				if hasHD {
					mn, mx := minF(metrics), maxF(metrics)
					if v <= mn {
						cdf += 0.1
					}
					if v >= mx {
						cdf = 1
					}
				}
				out = jsonv.Float(cdf * 100)
			}
			values = append(values, ojson.KV{percentileKey(v), out})
		}
		return ojson.Object{{"values", values}}, nil
	}
	items := []any{}
	for _, v := range vals {
		var out any
		if len(metrics) > 0 {
			cdf := tdigestCDF(metrics, v)
			if hasHD {
				mn, mx := minF(metrics), maxF(metrics)
				if v <= mn {
					cdf += 0.1
				}
				if v >= mx {
					cdf = 1
				}
			}
			out = jsonv.Float(cdf * 100)
		}
		items = append(items, ojson.Object{{"key", jsonv.Float(v)}, {"value", out}})
	}
	return ojson.Object{{"values", items}}, nil
}

func hdrQuantile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx] + hdrOffset(sorted[idx])
}

func hdrOffset(v float64) float64 {
	if v < 4 {
		return 1.0 / 1024
	}
	if v < 16 {
		return 0
	}
	if v < 32 {
		return 1.0 / 128
	}
	if v < 64 {
		return 3.0 / 128
	}
	return 7.0 / 128
}
