package dates

import (
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/zone"
)

// Resolution is the precision of a date anchor, used to round range bounds up.
type Resolution int

const (
	resMillis Resolution = iota
	resSecond
	resMinute
	resHour
	resDay
	resWeek
	resMonth
	resYear
)

func dateMathUnitResolution(unit byte) (Resolution, bool) {
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

// ApplyMath applies a date math expression ("+1d/d") to t, the way
// JavaDateMathParser.parseMath does: only a rounding ("/d") rounds, and with
// roundUp (the upper bound of an inclusive range, ...) it moves to the last
// millisecond of that unit at once. Additions never round.
func ApplyMath(t time.Time, expr string, roundUp bool) (time.Time, bool) {
	i := 0
	for i < len(expr) {
		c := expr[i]
		if c == '/' {
			if i+1 >= len(expr) {
				return t, false
			}
			unit := expr[i+1]
			if _, ok := dateMathUnitResolution(unit); !ok {
				return t, false
			}
			t = truncateDateMathUnit(t, unit)
			if roundUp {
				t = addDateMathUnit(t, unit, 1).Add(-time.Millisecond)
			}
			i += 2
			continue
		}
		if c != '+' && c != '-' {
			return t, false
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
			return t, false
		}
		n, err := strconv.Atoi(expr[start:i])
		if err != nil {
			return t, false
		}
		unit := expr[i]
		if _, ok := dateMathUnitResolution(unit); !ok {
			return t, false
		}
		i++
		t = addDateMathUnit(t, unit, sign*n)
	}
	return t, true
}

// RoundUp moves t to the last millisecond of its unit of resolution res.
func RoundUp(t time.Time, res Resolution) time.Time {
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

func dateResolutionOfString(s string) Resolution {
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

func dateResolutionOfFormat(f string) Resolution {
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

func parseDefaultDateAnchor(s string, loc *time.Location) (time.Time, Resolution, bool) {
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

// ParseAnchor parses the anchor of a range bound ("now" or a date in the
// query or field format).
func ParseAnchor(s, defFormat, queryFormat string, loc *time.Location) (time.Time, Resolution, bool) {
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
				layout := JavaFormatToGo(part)
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

// ParseLocation resolves a time_zone parameter with ZoneId.of ("" is UTC).
func ParseLocation(tz string) (*time.Location, *zone.Error) {
	if tz == "" {
		return time.UTC, nil
	}
	z, err := zone.Cached(tz)
	if err != nil {
		return nil, err
	}
	return z.Location(), nil
}
