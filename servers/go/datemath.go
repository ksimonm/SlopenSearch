package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type dateResolution int

const (
	resMillis dateResolution = iota
	resSecond
	resMinute
	resHour
	resDay
	resWeek
	resMonth
	resYear
)

func dateMathUnitResolution(unit byte) (dateResolution, bool) {
	switch unit {
	case 's':
		return resSecond, true
	case 'm':
		return resMinute, true
	case 'h', 'H':
		return resHour, true
	case 'd':
		return resDay, true
	case 'w':
		return resWeek, true
	case 'M':
		return resMonth, true
	case 'y':
		return resYear, true
	}
	return resMillis, false
}

func addDateMathUnit(t time.Time, unit byte, n int) time.Time {
	switch unit {
	case 'y':
		return t.AddDate(n, 0, 0)
	case 'M':
		return t.AddDate(0, n, 0)
	case 'w':
		return t.AddDate(0, 0, 7*n)
	case 'd':
		return t.AddDate(0, 0, n)
	case 'h', 'H':
		return t.Add(time.Duration(n) * time.Hour)
	case 'm':
		return t.Add(time.Duration(n) * time.Minute)
	case 's':
		return t.Add(time.Duration(n) * time.Second)
	}
	return t
}

func truncateDateMathUnit(t time.Time, unit byte) time.Time {
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	loc := t.Location()
	switch unit {
	case 'y':
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	case 'M':
		return time.Date(y, mo, 1, 0, 0, 0, 0, loc)
	case 'w':
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		day := time.Date(y, mo, d, 0, 0, 0, 0, loc)
		return day.AddDate(0, 0, -(wd - 1))
	case 'd':
		return time.Date(y, mo, d, 0, 0, 0, 0, loc)
	case 'h', 'H':
		return time.Date(y, mo, d, h, 0, 0, 0, loc)
	case 'm':
		return time.Date(y, mo, d, h, mi, 0, 0, loc)
	case 's':
		return time.Date(y, mo, d, h, mi, s, 0, loc)
	}
	return t
}

func applyRangeDateMath(t time.Time, expr string) (time.Time, dateResolution, bool) {
	res := resMillis
	i := 0
	for i < len(expr) {
		c := expr[i]
		if c == '/' {
			if i+1 >= len(expr) {
				return t, res, false
			}
			unit := expr[i+1]
			ures, ok := dateMathUnitResolution(unit)
			if !ok {
				return t, res, false
			}
			t = truncateDateMathUnit(t, unit)
			res = ures
			i += 2
			continue
		}
		if c != '+' && c != '-' {
			return t, res, false
		}
		sign := 1
		if c == '-' {
			sign = -1
		}
		i++
		start := i
		for i < len(expr) && expr[i] >= '0' && expr[i] <= '9' {
			i++
		}
		if start == i || i >= len(expr) {
			return t, res, false
		}
		n, err := strconv.Atoi(expr[start:i])
		if err != nil {
			return t, res, false
		}
		unit := expr[i]
		ures, ok := dateMathUnitResolution(unit)
		if !ok {
			return t, res, false
		}
		i++
		t = addDateMathUnit(t, unit, sign*n)
		if ures > res {
			res = ures
		}
	}
	return t, res, true
}

func roundUpDateMath(t time.Time, res dateResolution) time.Time {
	var unit byte
	switch res {
	case resSecond:
		unit = 's'
	case resMinute:
		unit = 'm'
	case resHour:
		unit = 'h'
	case resDay:
		unit = 'd'
	case resWeek:
		unit = 'w'
	case resMonth:
		unit = 'M'
	case resYear:
		unit = 'y'
	default:
		return t
	}
	trunc := truncateDateMathUnit(t, unit)
	return addDateMathUnit(trunc, unit, 1).Add(-time.Millisecond)
}

func dateResolutionOfString(s string) dateResolution {
	body := s
	if i := strings.Index(body, "||"); i >= 0 {
		body = body[:i]
	}
	if i := strings.IndexAny(body, "T "); i >= 0 {
		tp := body[i+1:]
		if strings.HasSuffix(tp, "Z") {
			tp = tp[:len(tp)-1]
		}
		if len(tp) > 6 {
			if idx := strings.IndexAny(tp[6:], "+-"); idx >= 0 {
				tp = tp[:6+idx]
			}
		}
		if strings.Contains(tp, ".") {
			return resMillis
		}
		switch strings.Count(tp, ":") {
		case 0:
			return resHour
		case 1:
			return resMinute
		default:
			return resSecond
		}
	}
	switch strings.Count(body, "-") {
	case 0:
		return resYear
	case 1:
		return resMonth
	default:
		return resDay
	}
}

func dateResolutionOfFormat(f string) dateResolution {
	switch {
	case strings.Contains(f, "S"):
		return resMillis
	case strings.Contains(f, "ss"):
		return resSecond
	case strings.Contains(f, "mm"):
		return resMinute
	case strings.Contains(f, "H"):
		return resHour
	case strings.Contains(f, "dd"):
		return resDay
	case strings.Contains(f, "MM"):
		return resMonth
	}
	return resYear
}

func parseDefaultDateAnchor(s string, loc *time.Location) (time.Time, dateResolution, bool) {
	res := dateResolutionOfString(s)
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.000-07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, res, true
		}
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006-01",
		"2006",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, res, true
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(n).In(loc), resMillis, true
	}
	return time.Time{}, resMillis, false
}

func parseRangeDateAnchor(s, defFormat, queryFormat string, loc *time.Location) (time.Time, dateResolution, bool) {
	if s == "now" {
		return time.Now().In(loc), resMillis, true
	}
	format := queryFormat
	if format == "" {
		format = defFormat
	}
	if format != "" {
		for _, part := range strings.Split(format, "||") {
			part = strings.TrimSpace(part)
			switch part {
			case "epoch_millis":
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return time.UnixMilli(n).In(loc), resMillis, true
				}
			case "epoch_second":
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return time.Unix(n, 0).In(loc), resMillis, true
				}
			case "date_optional_time", "strict_date_optional_time", "date_time",
				"basic_date_time", "strict_date_optional_time_nanos":
				if t, res, ok := parseDefaultDateAnchor(s, loc); ok {
					return t, res, true
				}
			default:
				layout := javaDateFormatToGo(part)
				if layout == "" {
					continue
				}
				if t, err := time.ParseInLocation(layout, s, loc); err == nil {
					return t, dateResolutionOfFormat(part), true
				}
			}
		}
		return time.Time{}, resMillis, false
	}
	return parseDefaultDateAnchor(s, loc)
}

func parseRangeLocation(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" || tz == "UTC" || tz == "utc" || tz == "Z" {
		return time.UTC, nil
	}
	if strings.HasPrefix(tz, "+") || strings.HasPrefix(tz, "-") {
		sign := 1
		if tz[0] == '-' {
			sign = -1
		}
		parts := strings.Split(tz[1:], ":")
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		m := 0
		if len(parts) > 1 {
			m, _ = strconv.Atoi(parts[1])
		}
		return time.FixedZone(tz, sign*(h*3600+m*60)), nil
	}
	return time.LoadLocation(tz)
}

func rangeDateParseError(value, queryFormat string, def map[string]any) *apiError {
	format := queryFormat
	if format == "" {
		format = dateFormatOf(def)
	}
	return &apiError{Type: "parse_exception", Status: 400,
		Reason: fmt.Sprintf("failed to parse date field [%s] with format [%s]", value, format)}
}

func parseRangeDateBound(def map[string]any, v any, tz, queryFormat string, roundUp bool) (any, *apiError) {
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
			if l, err := parseRangeLocation(tz); err == nil {
				loc = l
			}
		}
		t, res, ok := parseRangeDateAnchor(anchor, dateFormatOf(def), queryFormat, loc)
		if !ok {
			return nil, rangeDateParseError(s, queryFormat, def)
		}
		if mathExpr != "" {
			var mok bool
			t, res, mok = applyRangeDateMath(t, mathExpr)
			if !mok {
				return nil, rangeDateParseError(s, queryFormat, def)
			}
		}
		if roundUp {
			t = roundUpDateMath(t, res)
		}
		if isNanos {
			return t.Unix()*int64(1e9) + int64(t.Nanosecond()), nil
		}
		return t.UnixMilli(), nil
	}
	ctx := &indexCtx{indexCoerce: true}
	typ := "date"
	if t, _ := def["type"].(string); t == "date_nanos" {
		typ = "date_nanos"
	}
	parsed, err := ctx.parseDateValue("_value", typ, def, v)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}
