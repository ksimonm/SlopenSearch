package search

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
)

type SortSpec struct {
	Field        string
	Desc         bool
	Missing      any
	HasMissing   bool
	Mode         string
	Unmapped     string
	format       string
	NestedPath   string
	NestedFilter map[string]any
	NumericType  string
	scriptSource string
	scriptParams any
	ScriptType   string
	ScriptRaw    any
	ScriptVals   map[string]any
	GeoField     string
	GeoPoint     any
	GeoUnit      string
	GeoMode      string
	GeoIgnore    bool
}

func ParseSortSpecs(bodySort any, uriSort string) ([]SortSpec, *apierr.Error) {
	var out []SortSpec
	add := func(raw any, uri bool) *apierr.Error {
		switch t := raw.(type) {
		case string:
			if t == "" {
				return nil
			}
			out = append(out, SortSpec{Field: t})
		case map[string]any:
			for field, v := range t {
				sp := SortSpec{Field: field}
				switch ov := v.(type) {
				case string:
					if !validSortOrder(ov) {
						if uri {
							return nil
						}
						return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
							Reason: "No enum constant org.opensearch.search.sort.SortOrder." + strings.ToUpper(ov)}
					}
					sp.Desc = strings.EqualFold(ov, "desc")
				case map[string]any:
					if o, ok := ov["order"].(string); ok {
						if !validSortOrder(o) {
							if uri {
								return nil
							}
							return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
								Reason: "No enum constant org.opensearch.search.sort.SortOrder." + strings.ToUpper(o)}
						}
						sp.Desc = strings.EqualFold(o, "desc")
					}
					if m, ok := ov["missing"]; ok {
						sp.Missing = m
						sp.HasMissing = true
					}
					if m, ok := ov["mode"].(string); ok {
						m = strings.ToLower(m) // SortMode.fromString
						switch m {
						case "min", "max", "avg", "sum", "median":
							sp.Mode = m
						default:
							return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
								Reason: "Failed to parse sort mode [" + m + "]",
								CausedBy: &apierr.Error{Type: "illegal_argument_exception",
									Reason: "Unknown SortMode [" + m + "]"}}
						}
					}
					if m, ok := ov["unmapped_type"].(string); ok {
						sp.Unmapped = m
					}
					if _, ok := ov["format"]; ok {
						return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
							Reason: "Failed to parse sort [field_sort] unknown field [format]"}
					}
					if nt, ok := ov["numeric_type"].(string); ok {
						sp.NumericType = nt
					}
					if st, ok := ov["type"].(string); ok {
						sp.ScriptType = st
					}
					if sp.Field == "_geo_distance" {
						for k, v := range ov {
							switch k {
							case "order", "unit", "mode", "distance_type", "validation_method", "ignore_unmapped", "nested", "nested_path":
							default:
								sp.GeoField = k
								sp.GeoPoint = v
							}
						}
						if u, ok := ov["unit"].(string); ok {
							sp.GeoUnit = u
						}
						if m, ok := ov["mode"].(string); ok {
							sp.GeoMode = m
						}
						if b, ok := ov["ignore_unmapped"].(bool); ok {
							sp.GeoIgnore = b
						}
						if sp.GeoUnit != "" {
							if _, ok := geoUnitMeters(sp.GeoUnit); !ok {
								return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
									Reason: "No distance unit match [" + sp.GeoUnit + "]"}
							}
						}
					}
					if sp.Field == "_script" {
						sp.ScriptRaw = ov["script"]
						st, hasType := ov["type"].(string)
						if !hasType {
							return apierr.BadRequest("Required [type]")
						}
						if st != "number" && st != "string" && st != "version" {
							return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
								Reason:   "[_script] failed to parse field [type]",
								CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "Unknown ScriptSortType [" + st + "]"}}
						}
					}
					switch sc := ov["script"].(type) {
					case string:
						sp.scriptSource = sc
					case map[string]any:
						if src, ok := sc["source"].(string); ok {
							sp.scriptSource = src
						} else if src, ok := sc["inline"].(string); ok {
							sp.scriptSource = src
						}
						if pr, ok := sc["params"]; ok {
							sp.scriptParams = pr
						}
					}
					if np, ok := ov["nested_path"].(string); ok {
						sp.NestedPath = np
					}
					if nm, ok := ov["nested"].(map[string]any); ok {
						if np, ok := nm["path"].(string); ok {
							sp.NestedPath = np
						}
						if f, ok := nm["filter"].(map[string]any); ok {
							sp.NestedFilter = f
						}
					}
					if m, ok := ov["format"].(string); ok {
						sp.format = m
					}
				}
				out = append(out, sp)
			}
		}
		return nil
	}
	switch t := bodySort.(type) {
	case []any:
		for _, raw := range t {
			if err := add(raw, false); err != nil {
				return nil, err
			}
		}
	case string, map[string]any:
		if err := add(t, false); err != nil {
			return nil, err
		}
	}
	if uriSort != "" {
		valid := true
		var uriSpecs []SortSpec
		for _, part := range strings.Split(uriSort, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			field := part
			desc := false
			if at := strings.LastIndex(part, ":"); at >= 0 {
				field = part[:at]
				ord := part[at+1:]
				if ord != "asc" && ord != "desc" {
					valid = false
					break
				}
				desc = ord == "desc"
			}
			uriSpecs = append(uriSpecs, SortSpec{Field: field, Desc: desc})
		}
		if valid {
			out = append(out, uriSpecs...)
		}
	}
	return out, nil
}

func validSortOrder(s string) bool {
	return strings.EqualFold(s, "asc") || strings.EqualFold(s, "desc") // SortOrder.fromString
}

func singleEntry(m map[string]any) (string, any, bool) {
	for k, v := range m {
		return k, v, true
	}
	return "", nil, false
}

func QueryText(raw any) (string, any) {
	if m, ok := raw.(map[string]any); ok {
		if q, ok := m["query"]; ok {
			s, _ := jsonv.ToString(q)
			return s, m
		}
	}
	s, _ := jsonv.ToString(raw)
	return s, nil
}

func AsQueryList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		return []any{t}
	}
	return nil
}

var wildcardCache = map[string]*regexp.Regexp{}

func WildcardRegex(pattern string) *regexp.Regexp {
	if re, ok := wildcardCache[pattern]; ok {
		return re
	}
	var b strings.Builder
	b.WriteString("^")
	escaped := false
	for _, ch := range pattern {
		if escaped {
			b.WriteString(regexp.QuoteMeta(string(ch)))
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	b.WriteString("$")
	re := regexp.MustCompile(b.String())
	wildcardCache[pattern] = re
	return re
}

func ScoreJSON(v float64) any {
	switch {
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	case math.IsNaN(v):
		return "NaN"
	}
	if v == math.Trunc(v) {
		return json.Number(strconv.FormatFloat(v, 'f', 1, 64))
	}
	if f32 := float32(v); float64(f32) == v && !math.IsInf(float64(f32), 0) {
		return json.Number(strconv.FormatFloat(v, 'g', -1, 32))
	}
	return v
}

func geoUnitMeters(unit string) (float64, bool) {
	switch unit {
	case "m", "meters":
		return 1, true
	case "km", "kilometers":
		return 1000, true
	case "mi", "miles":
		return 1609.344, true
	case "yd", "yards":
		return 0.9144, true
	case "ft", "feet":
		return 0.3048, true
	case "in", "inch":
		return 0.0254, true
	case "cm", "centimeters":
		return 0.01, true
	case "mm", "millimeters":
		return 0.001, true
	case "nmi", "NM", "nauticalmiles":
		return 1852, true
	}
	return 0, false
}
