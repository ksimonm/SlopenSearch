package search

import (
	"encoding/json"
	"fmt"
	"strings"

	"slopensearch/internal/apierr"
)

// ---------------------------------------------------------------------------
// sort: parse-time (x_content) checks of field_sort / _score objects, with the
// [line:column] of the offending token in the request body
// ---------------------------------------------------------------------------

// sortParamKinds lists the parameters of FieldSortBuilder's and
// ScoreSortBuilder's object parsers with the kind of value each accepts.
var sortParamKinds = map[string]map[string]string{
	"field_sort": {"order": "string", "mode": "string", "unmapped_type": "string", "numeric_type": "string",
		"nested_path": "string", "missing": "value", "nested_filter": "object", "nested": "object"},
	"_score": {"order": "string"},
}

// jsonTokenName names the token starting at raw[i] the way XContentParser.Token does.
func jsonTokenName(raw []byte, i int) string {
	if i >= len(raw) {
		return "END_OBJECT"
	}
	switch c := raw[i]; {
	case c == '{':
		return "START_OBJECT"
	case c == '[':
		return "START_ARRAY"
	case c == '"':
		return "VALUE_STRING"
	case c == 't' || c == 'f':
		return "VALUE_BOOLEAN"
	case c == 'n':
		return "VALUE_NULL"
	}
	return "VALUE_NUMBER"
}

func sortXContentErr(raw []byte, off int, msg string, cause *apierr.Error) *apierr.Error {
	line, col := jsonLineCol(raw, off)
	return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
		Reason: fmt.Sprintf("[%d:%d] %s", line, col, msg), CausedBy: cause}
}

// SortParsedFirst reports whether the sort of a search body is parsed before
// anything that can fail to parse: OpenSearch parses the body in order, and
// the keys before "sort" are simple values.
func SortParsedFirst(raw []byte) bool {
	for _, e := range jsonObjectEntries(raw, 0) {
		switch e.key {
		case "sort":
			return true
		case "from", "size", "terminate_after", "min_score":
			if jsonTokenName(raw, e.valPos) != "VALUE_NUMBER" {
				return false
			}
		case "track_total_hits", "_source", "timeout", "version", "seq_no_primary_term",
			"explain", "track_scores", "stored_fields", "docvalue_fields",
			"fields", "profile", "indices_boost", "stats":
		default:
			return false
		}
	}
	return false
}

// CheckSortClauses validates the field_sort and _score objects of the
// top-level sort of a search body the way their object parsers do.
func CheckSortClauses(raw []byte) *apierr.Error {
	if len(raw) == 0 {
		return nil
	}
	for _, e := range jsonObjectEntries(raw, 0) {
		if e.key != "sort" {
			continue
		}
		i := jsonSkipWS(raw, e.valPos)
		if i >= len(raw) {
			return nil
		}
		if raw[i] == '{' {
			return checkSortElement(raw, i)
		}
		if raw[i] != '[' {
			return nil
		}
		j := i + 1
		for {
			j = jsonSkipWS(raw, j)
			if j >= len(raw) || raw[j] == ']' {
				return nil
			}
			if raw[j] == ',' {
				j++
				continue
			}
			if raw[j] == '{' {
				if err := checkSortElement(raw, j); err != nil {
					return err
				}
			}
			j = jsonSkipValue(raw, j)
		}
	}
	return nil
}

func checkSortElement(raw []byte, i int) *apierr.Error {
	for _, f := range jsonObjectEntries(raw, i) {
		if raw[jsonSkipWS(raw, f.valPos)] != '{' {
			continue
		}
		name := "field_sort"
		switch f.key {
		case "_geo_distance", "_geoDistance", "_script":
			continue
		case "_score":
			name = "_score"
		}
		kinds := sortParamKinds[name]
		for _, p := range jsonObjectEntries(raw, f.valPos) {
			kind, known := kinds[p.key]
			if !known {
				return sortXContentErr(raw, p.keyPos, "["+name+"] unknown field ["+p.key+"]", nil)
			}
			tok := jsonTokenName(raw, p.valPos)
			ok := false
			switch kind {
			case "string":
				ok = tok == "VALUE_STRING"
			case "value":
				ok = tok != "START_OBJECT" && tok != "START_ARRAY"
			case "object":
				ok = tok == "START_OBJECT"
			}
			if !ok {
				return sortXContentErr(raw, p.valPos, "["+name+"] "+p.key+" doesn't support values of type: "+tok, nil)
			}
			if tok != "VALUE_STRING" {
				continue
			}
			v, _ := jsonStringValue(raw[p.valPos:p.valEnd])
			var reason string
			switch p.key {
			case "order":
				if !validSortOrder(v) {
					reason = "No enum constant org.opensearch.search.sort.SortOrder." + strings.ToUpper(v)
				}
			case "mode":
				if !validSortMode(v) {
					reason = "Unknown SortMode [" + v + "]"
				}
			case "numeric_type":
				switch strings.ToLower(v) {
				case "long", "double", "date", "date_nanos":
				default:
					reason = "invalid value for [numeric_type], must be [long, double, date, date_nanos], got " + v
				}
			}
			if reason != "" {
				return sortXContentErr(raw, p.valPos, "["+name+"] failed to parse field ["+p.key+"]",
					&apierr.Error{Type: "illegal_argument_exception", Reason: reason})
			}
		}
	}
	return nil
}

func jsonStringValue(b []byte) (string, bool) {
	var s string
	err := json.Unmarshal(b, &s)
	return s, err == nil
}

func validSortMode(s string) bool {
	switch strings.ToLower(s) {
	case "min", "max", "sum", "avg", "median":
		return true
	}
	return false
}
