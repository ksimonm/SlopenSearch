package search

import (
	"fmt"
	"strings"
	"time"

	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/mapping"
)

func rangeDateParseError(value, queryFormat string, def map[string]any) *apierr.Error {
	format := queryFormat
	if format == "" {
		format = dates.FormatOf(def)
	}
	return &apierr.Error{Type: "parse_exception", Status: 400,
		Reason: fmt.Sprintf("failed to parse date field [%s] with format [%s]", value, format)}
}

func parseRangeDateBound(def map[string]any, v any, tz, queryFormat string, roundUp bool) (any, *apierr.Error) {
	if m, ok := v.(map[string]any); ok {
		if mv, ok := m["value"]; ok {
			v = mv
		}
	}
	if s, ok := v.(string); ok {
		isNanos := false
		if t, _ := def["type"].(string); t == "date_nanos" {
			isNanos = true
		}
		anchor := s
		mathExpr := ""
		if strings.HasPrefix(s, "now") {
			anchor = "now"
			mathExpr = s[3:]
		} else if i := strings.Index(s, "||"); i >= 0 {
			anchor = s[:i]
			mathExpr = s[i+2:]
		}
		loc := time.UTC
		if tz != "" {
			if l, err := dates.ParseLocation(tz); err == nil {
				loc = l
			}
		}
		t, res, ok := dates.ParseAnchor(anchor, dates.FormatOf(def), queryFormat, loc)
		if !ok {
			return nil, rangeDateParseError(s, queryFormat, def)
		}
		if mathExpr != "" {
			// JavaDateMathParser: the anchor of a math expression is never
			// rounded up; only a "/unit" in the expression rounds.
			var mok bool
			if t, mok = dates.ApplyMath(t, mathExpr, roundUp); !mok {
				return nil, rangeDateParseError(s, queryFormat, def)
			}
		} else if roundUp {
			t = dates.RoundUp(t, res)
		}
		if isNanos {
			return t.Unix()*int64(1e9) + int64(t.Nanosecond()), nil
		}
		return t.UnixMilli(), nil
	}
	ctx := &mapping.ParseContext{IndexCoerce: true}
	typ := "date"
	if t, _ := def["type"].(string); t == "date_nanos" {
		typ = "date_nanos"
	}
	parsed, err := ctx.ParseDateValue("_value", typ, def, v)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}
