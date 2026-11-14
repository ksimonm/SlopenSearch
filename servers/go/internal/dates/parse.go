// Package dates parses and formats dates the way OpenSearch mappings and
// queries do: Java format names and patterns, epoch values, date math.
package dates

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/jsonv"
)

// FormatOf returns the date format of a field mapping (OpenSearch default if unset).
func FormatOf(def map[string]any) string {
	if f, ok := def["format"].(string); ok {
		return f
	}
	return "strict_date_optional_time||epoch_millis"
}

// JavaFormatToGo converts the common subset of Java date format letters.
func JavaFormatToGo(f string) string {
	repl := []struct{ from, to string }{
		{"yyyy", "2006"}, {"yy", "06"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"}, {"mm", "04"}, {"ss", "05"},
		{"SSSSSSSSS", "000000000"}, {"SSS", "000"},
	}
	out := f
	if strings.Contains(out, "'T'") {
		out = strings.ReplaceAll(out, "'T'", "T")
	}
	for _, r := range repl {
		out = strings.ReplaceAll(out, r.from, r.to)
	}
	return out
}

func parseEpochNumber(s string, seconds bool) (int64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if seconds {
		return int64(f * 1000), true
	}
	return int64(f), true
}

func parseDateWithNamedFormat(name, s string) (int64, bool) {
	switch name {
	case "epoch_millis":
		return parseEpochNumber(s, false)
	case "epoch_second":
		return parseEpochNumber(s, true)
	case "date_optional_time", "strict_date_optional_time", "date_time",
		"strict_date_optional_time_nanos":
		return parseStrictDateOptionalTime(s)
	case "strict_date":
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "date":
		if t, err := time.Parse("2006-1-2", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "basic_date":
		if t, err := time.Parse("20060102", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "basic_date_time":
		if t, err := time.Parse("20060102T150405.000Z0700", s); err == nil {
			return t.UnixMilli(), true
		}
		if t, err := time.Parse("20060102T150405Z0700", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	}
	if layout := JavaFormatToGo(name); layout != "" {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), true
		}
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}

// ParseAny parses s with the formats of the mapping def into epoch millis.
func ParseAny(def map[string]any, s string) (int64, bool) {
	if f, ok := def["format"].(string); ok && f != "" {
		for _, fmtPart := range strings.Split(f, "||") {
			if ms, ok := parseDateWithNamedFormat(strings.TrimSpace(fmtPart), s); ok {
				return ms, true
			}
		}
		return 0, false
	}
	if ms, ok := parseStrictDateOptionalTime(s); ok {
		return ms, true
	}
	if ms, ok := parseEpochNumber(s, false); ok {
		return ms, true
	}
	return 0, false
}

func parseDateDefault(s string) (int64, bool) {
	return parseStrictDateOptionalTime(s)
}

func parseStrictDateOptionalTime(s string) (int64, bool) {
	sec, nsec, ok := parseStrictDateOptionalTimeParts(s)
	if !ok {
		return 0, false
	}
	return sec*1000 + int64(nsec)/int64(1e6), true
}

func parseStrictDateOptionalTimeParts(s string) (int64, int, bool) {
	i := 0
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	ystart := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == ystart {
		return 0, 0, false
	}
	ydigits := s[ystart:i]
	if !neg && len(ydigits) != 4 {
		return 0, 0, false
	}
	year, err := strconv.ParseInt(ydigits, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if neg {
		year = -year
	}
	month, day := 1, 1
	hour, minute, second, nsec := 0, 0, 0, 0
	loc := time.UTC
	if i < len(s) {
		if s[i] != '-' || i+3 > len(s) {
			return 0, 0, false
		}
		m, err := strconv.Atoi(s[i+1 : i+3])
		if err != nil {
			return 0, 0, false
		}
		month = m
		i += 3
		if i < len(s) && s[i] == '-' {
			if i+3 > len(s) {
				return 0, 0, false
			}
			d, err := strconv.Atoi(s[i+1 : i+3])
			if err != nil {
				return 0, 0, false
			}
			day = d
			i += 3
		}
	}
	if i < len(s) {
		if s[i] != 'T' || i+6 > len(s) {
			return 0, 0, false
		}
		h, err := strconv.Atoi(s[i+1 : i+3])
		if err != nil || s[i+3] != ':' {
			return 0, 0, false
		}
		mi, err := strconv.Atoi(s[i+4 : i+6])
		if err != nil {
			return 0, 0, false
		}
		hour, minute = h, mi
		i += 6
		if i < len(s) && s[i] == ':' {
			if i+3 > len(s) {
				return 0, 0, false
			}
			se, err := strconv.Atoi(s[i+1 : i+3])
			if err != nil {
				return 0, 0, false
			}
			second = se
			i += 3
			if i < len(s) && s[i] == '.' {
				i++
				fstart := i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				if i == fstart {
					return 0, 0, false
				}
				frac := s[fstart:i]
				for len(frac) < 9 {
					frac += "0"
				}
				nsec, _ = strconv.Atoi(frac[:9])
			}
		}
		if i < len(s) {
			switch s[i] {
			case 'Z':
				i++
			case '+', '-':
				sign := 1
				if s[i] == '-' {
					sign = -1
				}
				if i+6 > len(s) {
					return 0, 0, false
				}
				th, err1 := strconv.Atoi(s[i+1 : i+3])
				tm, err2 := strconv.Atoi(s[i+4 : i+6])
				if err1 != nil || err2 != nil || s[i+3] != ':' {
					return 0, 0, false
				}
				loc = time.FixedZone("", sign*(th*3600+tm*60))
				i += 6
			default:
				return 0, 0, false
			}
		}
	}
	if i != len(s) {
		return 0, 0, false
	}
	if month < 1 || month > 12 || day < 1 || day > 31 ||
		hour > 23 || minute > 59 || second > 59 {
		return 0, 0, false
	}
	t := time.Date(int(year), time.Month(month), day, hour, minute, second, nsec, loc)
	if int(t.Month()) != month || t.Day() != day {
		return 0, 0, false
	}
	return t.Unix(), t.Nanosecond(), true
}

// ParseNanosAny parses s for a date_nanos field into epoch nanos; on failure
// the string is the error reason.
func ParseNanosAny(def map[string]any, s string) (int64, string, bool) {
	checkMs := func(ms int64) (int64, string, bool) {
		if ms < 0 {
			return 0, "date[" + s + "] is before the epoch in 1970 and cannot be stored in nanosecond resolution", false
		}
		if ms > math.MaxInt64/int64(1e6) {
			return 0, "date[" + s + "] is after 2262-04-11T23:47:16.854775807 and cannot be stored in nanosecond resolution", false
		}
		return ms * int64(1e6), "", true
	}
	if f, ok := def["format"].(string); ok && f != "" {
		for _, fmtPart := range strings.Split(f, "||") {
			fmtPart = strings.TrimSpace(fmtPart)
			switch fmtPart {
			case "epoch_millis":
				if ms, ok := parseEpochNumber(s, false); ok {
					return checkMs(ms)
				}
				continue
			case "epoch_second":
				if ms, ok := parseEpochNumber(s, true); ok {
					return checkMs(ms)
				}
				continue
			}
			if ms, ok := parseDateWithNamedFormat(fmtPart, s); ok {
				return checkMs(ms)
			}
		}
		return 0, "", false
	}
	if sec, nsec, ok := parseStrictDateOptionalTimeParts(s); ok {
		if sec < 0 {
			return 0, "date[" + s + "] is before the epoch in 1970 and cannot be stored in nanosecond resolution", false
		}
		maxSec := math.MaxInt64 / int64(1e9)
		maxNsec := math.MaxInt64 % int64(1e9)
		if sec > maxSec || (sec == maxSec && int64(nsec) > maxNsec) {
			return 0, "date[" + s + "] is after 2262-04-11T23:47:16.854775807 and cannot be stored in nanosecond resolution", false
		}
		return sec*int64(1e9) + int64(nsec), "", true
	}
	if ms, ok := parseEpochNumber(s, false); ok {
		return checkMs(ms)
	}
	return 0, "", false
}

// Pattern matches ISO-like dates as detected in dynamic mappings.
var Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?)?$`)

// ParseMillis parses a number or a common ISO date string into epoch millis.
func ParseMillis(v any) (float64, bool) {
	if f, ok := jsonv.ToFloat(v); ok {
		return f, true
	}
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.UnixMilli()), true
		}
	}
	return 0, false
}

// IsISOLike reports strings that dynamic date detection treats as ISO dates.
func IsISOLike(s string) bool {
	b := []byte(s)
	if len(b) < 7 || len(b) > 30 {
		return false
	}
	for _, c := range b[0:4] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if b[4] != '-' {
		return false
	}
	for _, c := range b[5:7] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if len(b) == 7 {
		return true
	}
	if b[7] != '-' || len(b) < 10 {
		return false
	}
	for _, c := range b[8:10] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if len(b) == 10 {
		return true
	}
	if b[10] != 'T' {
		return false
	}
	for _, c := range b[11:] {
		if (c >= '0' && c <= '9') || c == ':' || c == '.' || c == 'Z' || c == '+' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// IsSlashLike reports yyyy/MM/dd style dates of the default dynamic formats.
func IsSlashLike(s string) bool {
	b := []byte(s)
	if len(b) < 8 {
		return false
	}
	for _, c := range b[0:4] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return b[4] == '/' && b[5] >= '0' && b[5] <= '9' && b[6] >= '0' && b[6] <= '9' && b[7] == '/'
}

// FormatMatches reports whether s parses with the Java date format.
func FormatMatches(format, s string) bool {
	for _, alt := range strings.Split(format, "||") {
		var pat strings.Builder
		pat.WriteString("^")
		runes := []rune(alt)
		for i := 0; i < len(runes); {
			c := runes[i]
			tok := string(c)
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				j := i + 1
				for j < len(runes) {
					n := runes[j]
					if (n >= 'a' && n <= 'z') || (n >= 'A' && n <= 'Z') {
						j++
						continue
					}
					break
				}
				tok = string(runes[i:j])
				i = j
			} else {
				i++
			}
			switch tok {
			case "yyyy":
				pat.WriteString(`\d{4}`)
			case "MM", "dd", "HH", "mm", "ss":
				pat.WriteString(`\d{2}`)
			case "epoch_millis", "epoch_second":
				pat.WriteString(`\d+`)
			case "T":
				pat.WriteString("T")
			default:
				for _, ch := range tok {
					if strings.ContainsRune(`\.+*?()|[]{}^$`, ch) {
						pat.WriteByte('\\')
					}
					pat.WriteRune(ch)
				}
			}
		}
		pat.WriteString("$")
		if re, err := regexp.Compile(pat.String()); err == nil && re.MatchString(s) {
			return true
		}
	}
	return false
}

// FormatEpochMillis renders epoch millis as strict_date_optional_time in UTC.
func FormatEpochMillis(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

// FormatEpochNanos renders epoch nanos as strict_date_optional_time_nanos in UTC.
func FormatEpochNanos(ns int64) string {
	sec := ns / int64(1e9)
	frac := ns % int64(1e9)
	if frac < 0 {
		sec--
		frac += int64(1e9)
	}
	base := time.Unix(sec, frac).UTC().Format("2006-01-02T15:04:05")
	s := fmt.Sprintf("%09d", frac)
	end := len(s)
	for end > 3 && s[end-1] == '0' {
		end--
	}
	return base + "." + s[:end] + "Z"
}
