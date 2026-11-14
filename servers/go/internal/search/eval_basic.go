package search

import (
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/analysis"
	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
)

type MultiHit struct {
	Idx   *model.Index
	Id    string
	Doc   map[string]any
	Score float64
}

func CheckDocValuesLoadable(idx *model.Index, field string) *apierr.Error {
	def, ok := mapping.FieldDef(idx, field)
	if !ok {
		return nil
	}
	ftype, _ := def["type"].(string)
	if dv, ok := def["doc_values"].(bool); ok && !dv {
		reason := "Can't load fielddata on [" + field + "] because fielddata is unsupported on fields of type [" + ftype + "]. Use doc values instead."
		return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
			CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
	}
	if ftype == "binary" {
		if dv, ok := def["doc_values"].(bool); !ok || !dv {
			reason := "Can't load fielddata on [" + field + "] because fielddata is unsupported on fields of type [binary]. Use doc values instead."
			return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
				RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
				CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
		}
	}
	if ftype == "text" {
		if fd, ok := def["fielddata"].(bool); !ok || !fd {
			reason := "Fielddata is disabled on text fields by default. Set fielddata=true on [" + field + "] in order to load field data by uninverting the inverted index. Note that this can use significant memory."
			return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
				RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
				CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
		}
	}
	return nil
}

var TextLikeTypes = map[string]bool{
	"text": true, "match_only_text": true, "search_as_you_type": true,
	"completion": true, "annotated_text": true,
}

func FormatDocValue(idx *model.Index, field, ftype string, v any, format string) any {
	if ftype == "geo_point" && format == "wkt" {
		if m, ok := v.(map[string]any); ok {
			lat, _ := jsonv.ToFloat(m["lat"])
			lon, _ := jsonv.ToFloat(m["lon"])
			return "POINT (" + strconv.FormatFloat(lon, 'f', 1, 64) + " " + strconv.FormatFloat(lat, 'f', 1, 64) + ")"
		}
	}
	switch ftype {
	case "date", "date_nanos":
		var raw int64
		switch t := v.(type) {
		case int64:
			raw = t
		case uint64:
			raw = int64(t)
		default:
			if n, ok := IntegerOf(v); ok {
				raw = n
			} else {
				return v
			}
		}
		eff := format
		if eff == "" || eff == "use_field_mapping" {
			if def, ok := mapping.FieldDef(idx, field); ok {
				if mf, ok := def["format"].(string); ok && mf != "" {
					eff = strings.TrimSpace(strings.Split(mf, "||")[0])
				}
			}
		}
		nanosValue := ftype == "date_nanos"
		ms := raw
		if nanosValue {
			ms = raw / int64(1e6)
		}
		switch eff {
		case "", "strict_date_optional_time", "date_optional_time":
			return dates.FormatEpochMillis(ms)
		case "strict_date_optional_time_nanos":
			if nanosValue {
				return dates.FormatEpochNanos(raw)
			}
			return dates.FormatEpochMillis(ms)
		case "epoch_millis":
			return strconv.FormatInt(ms, 10)
		case "epoch_second":
			return strconv.FormatInt(ms/1000, 10)
		default:
			if layout := dates.JavaFormatToGo(eff); layout != "" {
				if nanosValue {
					return time.Unix(0, raw).UTC().Format(layout)
				}
				return time.UnixMilli(ms).UTC().Format(layout)
			}
			return strconv.FormatInt(ms, 10)
		}
	}
	if format != "" && strings.Contains(format, ".") {
		if n, ok := jsonv.ToFloat(v); ok {
			dec := len(format) - strings.Index(format, ".") - 1
			if dec < 0 {
				dec = 0
			}
			return strconv.FormatFloat(n, 'f', dec, 64)
		}
	}
	return v
}

func (h MultiHit) SortValue(field string, desc bool, mode string) (any, bool) {
	switch field {
	case "_score":
		return h.Score, true
	case "_id":
		return h.Id, true
	}
	def, mapped := mapping.FieldDef(h.Idx, field)
	if mapped {
		if t, _ := def["type"].(string); t == "alias" {
			field = mapping.ResolveAliasField(h.Idx, field)
			def, mapped = mapping.FieldDef(h.Idx, field)
		}
	}
	vals := h.Idx.DocValues[h.Id]
	if vals == nil {
		return nil, false
	}
	list, ok := vals[field]
	if !ok || len(list) == 0 {
		return nil, false
	}
	if mapped {
		ft, _ := def["type"].(string)
		if ft == "ip" {
			if len(list) == 1 {
				if s, ok := jsonv.ScalarString(list[0]); ok {
					if ip := net.ParseIP(s); ip != nil {
						return IpSortKey{key: string(ip.To16()), Out: CanonicalIP(s)}, true
					}
				}
			}
		}
		if TextLikeTypes[ft] {
			if fd, _ := def["fielddata"].(bool); fd {
				analyzer, _ := def["analyzer"].(string)
				toks := []string{}
				for _, v := range list {
					s, _ := jsonv.ScalarString(v)
					for _, t := range analysis.Tokens(mapping.AnalysisIndex(h.Idx), analyzer, s) {
						toks = append(toks, t.Term)
					}
				}
				if len(toks) == 0 {
					return nil, false
				}
				sort.Strings(toks)
				if desc {
					return toks[len(toks)-1], true
				}
				return toks[0], true
			}
		}
	}
	if len(list) == 1 {
		return list[0], true
	}
	return ReduceSortValues(list, desc, mode), true
}

type IpSortKey struct {
	key string
	Out string
}

func CompareSortValues(a, b any) int {
	if aip, ok := a.(IpSortKey); ok {
		if bip, ok2 := b.(IpSortKey); ok2 {
			return strings.Compare(aip.key, bip.key)
		}
	}
	if as, ok := a.(string); ok {
		bs, _ := jsonv.ToString(b)
		if _, bok := b.(string); bok {
			return strings.Compare(as, bs)
		}
		return strings.Compare(as, bs)
	}
	if af, ok := jsonv.ToFloat(a); ok {
		if bf, ok := jsonv.ToFloat(b); ok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			}
			return 0
		}
	}
	return compareValues(a, b)
}

func IntField(body map[string]any, key string, def int) int {
	if v, ok := body[key]; ok {
		if f, ok := jsonv.ToFloat(v); ok {
			return int(f)
		}
	}
	return def
}

func compareValues(a, b any) int {
	af, aok := jsonv.ToFloat(a)
	bf, bok := jsonv.ToFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as, _ := jsonv.ToString(a)
	bs, _ := jsonv.ToString(b)
	return strings.Compare(as, bs)
}

func FieldType(idx *model.Index, field string) string {
	if t, ok := idx.FieldTypes[field]; ok {
		return t
	}
	return "text"
}

func DocStrings(v any) []string {
	switch t := v.(type) {
	case []any:
		out := []string{}
		for _, e := range t {
			if s, ok := jsonv.ToString(e); ok {
				out = append(out, s)
			}
		}
		return out
	case nil:
		return nil
	default:
		if s, ok := jsonv.ToString(t); ok {
			return []string{s}
		}
	}
	return nil
}

func EvalQuery(idx *model.Index, query map[string]any, doc map[string]any) (bool, float64, *apierr.Error) {
	if len(query) == 0 {
		return true, 1, nil
	}
	for clause, raw := range query {
		switch clause {
		case "match_all":
			return true, 1, nil
		case "match_none":
			return false, 0, nil
		case "match":
			return evalMatch(idx, raw, doc)
		case "term":
			return evalTerm(idx, raw, doc, 0)
		case "terms":
			return evalTerms(idx, raw, doc)
		case "range":
			return evalRange(idx, raw, doc)
		case "bool":
			return evalBool(idx, raw, doc)
		case "match_phrase":
			return evalMatchPhrase(idx, raw, doc)
		case "multi_match":
			return evalMultiMatch(idx, raw, doc)
		case "wildcard":
			return evalWildcard(idx, raw, doc)
		case "fuzzy":
			return evalFuzzy(idx, raw, doc)
		case "exists":
			return evalExists(raw, doc)
		case "prefix":
			return evalPrefix(idx, raw, doc)
		case "script":
			id, _ := doc["__slop_id"].(string)
			return NewQueryCtxForDoc(idx, id, doc).EvalScriptQuery(raw)
		default:
			return false, 0, apierr.Parsing("unknown query [" + clause + "]")
		}
	}
	return false, 0, apierr.Parsing("empty query")
}

func evalMatch(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty match")
	}
	text, opts := QueryText(val)
	operator := ""
	if om, ok := opts.(map[string]any); ok {
		if o, ok := om["operator"].(string); ok {
			operator = o
		}
	}
	ft := FieldType(idx, field)
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	if mapping.IsNumericType(ft) || ft == "keyword" || ft == "boolean" {
		for _, s := range DocStrings(v) {
			if s == text {
				return true, 1, nil
			}
		}
		return false, 0, nil
	}
	qTokens := mapping.Analyze(text)
	var docTokens []string
	for _, s := range DocStrings(v) {
		docTokens = append(docTokens, mapping.Analyze(s)...)
	}
	score := 0.0
	for _, qt := range qTokens {
		for _, dt := range docTokens {
			if qt == dt {
				score++
				break
			}
		}
	}
	if operator == "and" {
		return len(qTokens) > 0 && score == float64(len(qTokens)), score, nil
	}
	return score > 0, score, nil
}

func evalPrefix(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty prefix")
	}
	prefix := ""
	if m, ok := val.(map[string]any); ok {
		prefix, _ = jsonv.ToString(m["value"])
	} else {
		prefix, _ = jsonv.ToString(val)
	}
	ft := FieldType(idx, field)
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	for _, s := range DocStrings(v) {
		candidates := []string{s}
		if ft == "text" {
			candidates = mapping.Analyze(s)
		}
		for _, c := range candidates {
			if strings.HasPrefix(c, prefix) {
				return true, 1, nil
			}
		}
	}
	return false, 0, nil
}

func evalTerm(idx *model.Index, raw any, doc map[string]any, fuzz int) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty term")
	}
	if m, ok := val.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			val = v
		}
		if f, ok := m["fuzziness"]; ok {
			if n, ok := jsonv.ToFloat(f); ok {
				fuzz = int(n)
			}
		}
	}
	term, _ := jsonv.ToString(val)
	ft := FieldType(idx, field)
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	for _, s := range DocStrings(v) {
		candidates := []string{s}
		if ft == "text" {
			candidates = mapping.Analyze(s)
		}
		for _, c := range candidates {
			if c == term {
				return true, 1, nil
			}
			if fuzz > 0 && Levenshtein(c, term) <= fuzz {
				return true, 1, nil
			}
		}
	}
	return false, 0, nil
}

func evalFuzzy(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, _ := SingleEntry(raw.(map[string]any))
	fuzz := AutoFuzziness(field)
	if m, ok := val.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			text, _ := jsonv.ToString(v)
			fuzz = AutoFuzziness(text)
		}
	}
	return evalTerm(idx, raw, doc, fuzz)
}

func evalTerms(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty terms")
	}
	list, ok := val.([]any)
	if !ok {
		return false, 0, apierr.Parsing("terms expects array")
	}
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	ft := FieldType(idx, field)
	for _, item := range list {
		want, _ := jsonv.ToString(item)
		for _, s := range DocStrings(v) {
			candidates := []string{s}
			if ft == "text" {
				candidates = mapping.Analyze(s)
			}
			for _, c := range candidates {
				if c == want {
					return true, 1, nil
				}
			}
		}
	}
	return false, 0, nil
}

func compareRange(dv float64, cond map[string]any, field string, parse func(any) (float64, bool)) (bool, float64, *apierr.Error) {
	for op, bound := range cond {
		bv, ok := parse(bound)
		if !ok {
			return false, 0, apierr.Parsing("failed to parse [" + field + "]")
		}
		switch op {
		case "gt":
			if !(dv > bv) {
				return false, 0, nil
			}
		case "gte":
			if !(dv >= bv) {
				return false, 0, nil
			}
		case "lt":
			if !(dv < bv) {
				return false, 0, nil
			}
		case "lte":
			if !(dv <= bv) {
				return false, 0, nil
			}
		}
	}
	return true, 1, nil
}

func evalRange(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad range")
	}
	field, condRaw, ok := SingleEntry(m)
	if !ok {
		return false, 0, apierr.Parsing("empty range")
	}
	cond, ok := condRaw.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad range condition")
	}
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	ft := FieldType(idx, field)
	if ft == "date" {
		dv, ok := dates.ParseMillis(v)
		if !ok {
			return false, 0, nil
		}
		return compareRange(dv, cond, field, dates.ParseMillis)
	}
	if mapping.IsNumericType(ft) {
		dv, ok := jsonv.ToFloat(v)
		if !ok {
			return false, 0, nil
		}
		return compareRange(dv, cond, field, jsonv.ToFloat)
	}
	ds, _ := jsonv.ToString(v)
	for op, bound := range cond {
		bs, _ := jsonv.ToString(bound)
		c := strings.Compare(ds, bs)
		switch op {
		case "gt":
			if !(c > 0) {
				return false, 0, nil
			}
		case "gte":
			if !(c >= 0) {
				return false, 0, nil
			}
		case "lt":
			if !(c < 0) {
				return false, 0, nil
			}
		case "lte":
			if !(c <= 0) {
				return false, 0, nil
			}
		}
	}
	return true, 1, nil
}

func evalBool(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad bool")
	}
	score := 0.0
	must := AsQueryList(m["must"])
	filter := AsQueryList(m["filter"])
	should := AsQueryList(m["should"])
	mustNot := AsQueryList(m["must_not"])

	for _, q := range append(append([]any{}, must...), filter...) {
		ok, sc, err := EvalQuery(idx, q.(map[string]any), doc)
		if err != nil {
			return false, 0, err
		}
		if !ok {
			return false, 0, nil
		}
		score += sc
	}
	if len(should) > 0 {
		anyMatched := false
		for _, q := range should {
			ok, sc, err := EvalQuery(idx, q.(map[string]any), doc)
			if err != nil {
				return false, 0, err
			}
			if ok {
				anyMatched = true
				score += sc
			}
		}
		if !anyMatched && len(must) == 0 && len(filter) == 0 {
			return false, 0, nil
		}
	}
	for _, q := range mustNot {
		ok, _, err := EvalQuery(idx, q.(map[string]any), doc)
		if err != nil {
			return false, 0, err
		}
		if ok {
			return false, 0, nil
		}
	}
	if len(must) == 0 && len(filter) == 0 && len(should) == 0 {
		return true, 1, nil
	}
	return true, score, nil
}

func evalMatchPhrase(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty match_phrase")
	}
	text, _ := QueryText(val)
	qTokens := mapping.Analyze(text)
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	for _, s := range DocStrings(v) {
		dTokens := mapping.Analyze(s)
		if containsSubsequence(dTokens, qTokens) {
			return true, float64(len(qTokens)), nil
		}
	}
	return false, 0, nil
}

func containsSubsequence(hay, needle []string) bool {
	if len(needle) == 0 {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func evalMultiMatch(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad multi_match")
	}
	q, _ := jsonv.ToString(m["query"])
	fields, _ := m["fields"].([]any)
	total := 0.0
	for _, f := range fields {
		fs, _ := jsonv.ToString(f)
		ok, sc, err := evalMatch(idx, map[string]any{fs: q}, doc)
		if err != nil {
			return false, 0, err
		}
		if ok {
			total += sc
		}
	}
	return total > 0, total, nil
}

func evalWildcard(idx *model.Index, raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	field, val, ok := SingleEntry(raw.(map[string]any))
	if !ok {
		return false, 0, apierr.Parsing("empty wildcard")
	}
	m, ok := val.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad wildcard body")
	}
	pattern, _ := jsonv.ToString(m["value"])
	re := WildcardRegex(pattern)
	ft := FieldType(idx, field)
	v, present := mapping.FieldValue(doc, field)
	if !present {
		return false, 0, nil
	}
	for _, s := range DocStrings(v) {
		candidates := []string{s}
		if ft == "text" {
			candidates = mapping.Analyze(s)
		}
		for _, c := range candidates {
			if re.MatchString(c) {
				return true, 1, nil
			}
		}
	}
	return false, 0, nil
}

func evalExists(raw any, doc map[string]any) (bool, float64, *apierr.Error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("bad exists")
	}
	field, _ := jsonv.ToString(m["field"])
	_, present := mapping.FieldValue(doc, field)
	return present, 1, nil
}

func HighlightText(text string, terms []string) string {
	for _, term := range terms {
		if term == "" {
			continue
		}
		lower := strings.ToLower(text)
		lt := strings.ToLower(term)
		idx := 0
		for {
			i := strings.Index(lower[idx:], lt)
			if i < 0 {
				break
			}
			i += idx
			text = text[:i] + "<em>" + text[i:i+len(term)] + "</em>" + text[i+len(term):]
			lower = strings.ToLower(text)
			idx = i + len(term) + 9
		}
	}
	return text
}

func ReduceSortValues(list []any, desc bool, mode string) any {
	sorted := append([]any{}, list...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return CompareSortValues(sorted[i], sorted[j]) < 0
	})
	switch mode {
	case "min":
		return sorted[0]
	case "max":
		return sorted[len(sorted)-1]
	case "avg", "sum", "median":
		floats := make([]float64, 0, len(sorted))
		for _, v := range sorted {
			if f, ok := jsonv.ToFloat(v); ok {
				floats = append(floats, f)
			}
		}
		if len(floats) == 0 {
			return sorted[0]
		}
		sort.Float64s(floats)
		switch mode {
		case "avg":
			sum := 0.0
			for _, f := range floats {
				sum += f
			}
			return math.Round(sum / float64(len(floats)))
		case "sum":
			sum := 0.0
			for _, f := range floats {
				sum += f
			}
			return sum
		default:
			mid := len(floats) / 2
			if len(floats)%2 == 0 {
				return math.Round((floats[mid-1] + floats[mid]) / 2)
			}
			return floats[mid]
		}
	}
	if desc {
		return sorted[len(sorted)-1]
	}
	return sorted[0]
}
