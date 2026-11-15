package dates

import (
	"testing"
	"time"
)

// Expected values come from tests/reference (OpenSearch 3.9 answers), mostly
// test_71_field_types.jsonl (test_date_custom_formats_*, test_date_nanos_*,
// test_default_date_detection, test_dynamic_date_formats,
// test_date_range_timezones_and_rounding), test_32_queries.jsonl
// (test_range_dates, test_range_date_format) and test_41_aggs_bucket.jsonl
// (test_date_range_*).

func TestParseAny(t *testing.T) {
	cases := []struct {
		name   string
		format string // "" = no format in the mapping (default)
		in     string
		want   string // strict_date_optional_time rendering; "" = rejected
	}{
		// test_date_custom_formats_accept (numbers reach ParseAny as their JSON text)
		{"epoch_millis negative number", "epoch_millis", "-86400000", "1969-12-31T00:00:00.000Z"},
		{"epoch_millis fractional truncates", "epoch_millis", "1.5", "1970-01-01T00:00:00.001Z"},
		{"epoch_millis fractional string", "epoch_millis", "1705312800000.5", "2024-01-15T10:00:00.000Z"},
		{"epoch_second", "epoch_second", "1705312800", "2024-01-15T10:00:00.000Z"},
		{"epoch_second negative", "epoch_second", "-1", "1969-12-31T23:59:59.000Z"},
		{"epoch_second fractional keeps millis", "epoch_second", "1705312800.5", "2024-01-15T10:00:00.500Z"},
		{"custom yyyy/MM/dd", "yyyy/MM/dd", "2024/01/15", "2024-01-15T00:00:00.000Z"},
		{"alternatives second wins", "yyyy/MM/dd||epoch_millis", "86400000", "1970-01-02T00:00:00.000Z"},
		{"alternatives first wins", "yyyy/MM/dd||epoch_millis", "2024/01/15", "2024-01-15T00:00:00.000Z"},
		{"strict_date", "strict_date", "2024-01-15", "2024-01-15T00:00:00.000Z"},
		{"date allows single digits", "date", "2024-1-5", "2024-01-05T00:00:00.000Z"},
		{"basic_date", "basic_date", "20240115", "2024-01-15T00:00:00.000Z"},
		{"custom with time", "dd.MM.yyyy HH:mm", "15.01.2024 13:45", "2024-01-15T13:45:00.000Z"},
		// test_date_custom_formats_reject
		{"epoch_millis rejects ISO", "epoch_millis", "2024-01-01", ""},
		{"epoch_second rejects ISO", "epoch_second", "2024-01-01", ""},
		{"custom rejects other separator", "yyyy/MM/dd", "2024-01-15", ""},
		{"custom rejects single digits", "yyyy/MM/dd", "2024/1/5", ""},
		{"custom rejects bare number", "yyyy/MM/dd", "20240115", ""},
		{"no alternative matches", "yyyy/MM/dd||epoch_millis", "2024-01-15", ""},
		{"strict_date rejects single digit month", "strict_date", "2024-1-15", ""},
		{"strict_date rejects time", "strict_date", "2024-01-15T00:00:00Z", ""},
		{"basic_date rejects dashes", "basic_date", "2024-01-15", ""},
		{"custom rejects missing time", "dd.MM.yyyy HH:mm", "15.01.2024", ""},
		// default strict_date_optional_time||epoch_millis
		{"default ISO with zone offset", "", "2024-01-15T10:00:00+05:00", "2024-01-15T05:00:00.000Z"},
		{"default millis truncates nanos", "", "2024-01-15T10:00:00.123456789Z", "2024-01-15T10:00:00.123Z"},
		{"default epoch millis", "", "946684800000", "2000-01-01T00:00:00.000Z"},
		{"default date only", "", "2000-01-03", "2000-01-03T00:00:00.000Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := map[string]any{"type": "date"}
			if c.format != "" {
				def["format"] = c.format
			}
			ms, ok := ParseAny(def, c.in)
			if c.want == "" {
				if ok {
					t.Fatalf("ParseAny(%q, %q) = %d, want rejection", c.format, c.in, ms)
				}
				return
			}
			if !ok {
				t.Fatalf("ParseAny(%q, %q) rejected, want %s", c.format, c.in, c.want)
			}
			if got := FormatEpochMillis(ms); got != c.want {
				t.Fatalf("ParseAny(%q, %q) = %s, want %s", c.format, c.in, got, c.want)
			}
		})
	}
}

func TestFormatOfDefault(t *testing.T) {
	// failed to parse date field [...] with format [strict_date_optional_time||epoch_millis]
	if got := FormatOf(map[string]any{"type": "date"}); got != "strict_date_optional_time||epoch_millis" {
		t.Fatalf("FormatOf(no format) = %q", got)
	}
	if got := FormatOf(map[string]any{"format": "yyyy/MM/dd"}); got != "yyyy/MM/dd" {
		t.Fatalf("FormatOf(yyyy/MM/dd) = %q", got)
	}
}

func TestParseNanosAny(t *testing.T) {
	cases := []struct {
		in     string
		want   int64
		reason string
	}{
		{"2024-01-15T10:00:00.123456789Z", 1705312800123456789, ""},
		{"2024-01-15T10:00:00.123456788Z", 1705312800123456788, ""},
		{"2024-01-15T10:00:00.123Z", 1705312800123000000, ""},
		{"1705312800123", 1705312800123000000, ""},
		{"2262-04-11T23:47:16.854775807Z", 9223372036854775807, ""},
		{"1969-12-31T00:00:00Z", 0, "date[1969-12-31T00:00:00Z] is before the epoch in 1970 and cannot be stored in nanosecond resolution"},
		{"2262-04-12T00:00:00Z", 0, "date[2262-04-12T00:00:00Z] is after 2262-04-11T23:47:16.854775807 and cannot be stored in nanosecond resolution"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			ns, reason, ok := ParseNanosAny(map[string]any{"type": "date_nanos"}, c.in)
			if c.reason != "" {
				if ok || reason != c.reason {
					t.Fatalf("ParseNanosAny(%q) = %d, %q, %v; want reason %q", c.in, ns, reason, ok, c.reason)
				}
				return
			}
			if !ok || ns != c.want {
				t.Fatalf("ParseNanosAny(%q) = %d, %q, %v; want %d", c.in, ns, reason, ok, c.want)
			}
		})
	}
}

func TestFormatEpoch(t *testing.T) {
	if got := FormatEpochMillis(1706745600000); got != "2024-02-01T00:00:00.000Z" {
		t.Errorf("FormatEpochMillis = %s", got)
	}
	if got := FormatEpochMillis(-1000); got != "1969-12-31T23:59:59.000Z" {
		t.Errorf("FormatEpochMillis(-1000) = %s", got)
	}
	// test_date_nanos_docvalue_formats: strict_date_optional_time_nanos
	if got := FormatEpochNanos(1705312800123456789); got != "2024-01-15T10:00:00.123456789Z" {
		t.Errorf("FormatEpochNanos = %s", got)
	}
}

func TestDynamicDateDetectionShapes(t *testing.T) {
	// test_default_date_detection: which strings the default dynamic formats
	// (strict_date_optional_time, yyyy/MM/dd HH:mm:ss||yyyy/MM/dd) accept.
	cases := []struct {
		in         string
		iso, slash bool
	}{
		{"2024-01-15T10:00:00Z", true, false},
		{"2024-01", true, false},
		{"2024-01-01", true, false},
		{"2024/01/15", false, true},
		{"2024", false, false},
		{"2024-01-15 10:00:00", false, false},
		{"1705312800000", false, false},
		{"hello", false, false},
	}
	for _, c := range cases {
		if got := IsISOLike(c.in); got != c.iso {
			t.Errorf("IsISOLike(%q) = %v, want %v", c.in, got, c.iso)
		}
		if got := IsSlashLike(c.in); got != c.slash {
			t.Errorf("IsSlashLike(%q) = %v, want %v", c.in, got, c.slash)
		}
	}
}

func TestFormatMatches(t *testing.T) {
	// test_dynamic_date_formats: dynamic_date_formats ["yyyy/MM/dd","dd.MM.yyyy"]
	cases := []struct {
		format, in string
		want       bool
	}{
		{"yyyy/MM/dd", "2024/01/15", true},
		{"yyyy/MM/dd", "2024-01-15", false},
		{"dd.MM.yyyy", "15.01.2024", true},
		{"dd.MM.yyyy", "2024/01/15", false},
		{"dd.MM.yyyy", "15x01x2024", false}, // '.' is literal
		{"yyyy/MM/dd HH:mm:ss||yyyy/MM/dd", "2024/01/15", true},
	}
	for _, c := range cases {
		if got := FormatMatches(c.format, c.in); got != c.want {
			t.Errorf("FormatMatches(%q, %q) = %v, want %v", c.format, c.in, got, c.want)
		}
	}
}

// rangeBound resolves a range bound the way search.parseRangeDateBound does:
// "anchor||math", parsed in tz, rounded up for gt/lte.
func rangeBound(t *testing.T, s, fieldFormat, queryFormat, tz string, roundUp bool) (int64, bool) {
	t.Helper()
	anchor, expr := s, ""
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '|' && s[i+1] == '|' {
			anchor, expr = s[:i], s[i+2:]
			break
		}
	}
	loc := time.UTC
	if tz != "" {
		l, err := ParseLocation(tz)
		if err != nil {
			t.Fatalf("ParseLocation(%q): %v", tz, err)
		}
		loc = l
	}
	tm, res, ok := ParseAnchor(anchor, fieldFormat, queryFormat, loc)
	if !ok {
		return 0, false
	}
	if expr != "" {
		if tm, ok = ApplyMath(tm, expr, roundUp); !ok {
			return 0, false
		}
	} else if roundUp {
		tm = RoundUp(tm, res)
	}
	return tm.UnixMilli(), true
}

func TestRangeBoundDateMath(t *testing.T) {
	const def = "strict_date_optional_time||epoch_millis"
	cases := []struct {
		name        string
		in          string
		queryFormat string
		tz          string
		roundUp     bool   // gt / lte
		want        string // "" = parse error
	}{
		// test_range_dates: docs at 2000-01-01T00, 2000-01-02T12, 2000-01-03, 1999-12-31T23:59:59
		{"gte rounds down to day", "2000-01-02||/d", "", "", false, "2000-01-02T00:00:00.000Z"},
		{"gt rounds up to end of day", "2000-01-02||/d", "", "", true, "2000-01-02T23:59:59.999Z"},
		{"minus one day", "2000-01-01||-1d", "", "", false, "1999-12-31T00:00:00.000Z"},
		{"plus month then round month", "2000-01-01||+1M/M", "", "", false, "2000-02-01T00:00:00.000Z"},
		{"year rounding gte", "2000||/y", "", "", false, "2000-01-01T00:00:00.000Z"},
		{"year rounding lte", "2000||/y", "", "", true, "2000-12-31T23:59:59.999Z"},
		// test_date_range_date_math (date_range agg, exact from/to)
		{"agg from +1M/M", "2024-01-01||+1M/M", "", "", false, "2024-02-01T00:00:00.000Z"},
		{"agg to -1d", "2025-01-01||-1d", "", "", false, "2024-12-31T00:00:00.000Z"},
		// test_date_range_timezones_and_rounding
		{"time_zone shifts date", "2024-01-16", "", "+01:00", false, "2024-01-15T23:00:00.000Z"},
		{"explicit offset", "2024-01-16T00:00:00+01:00", "", "", false, "2024-01-15T23:00:00.000Z"},
		{"query format", "2024/01/16", "yyyy/MM/dd", "", false, "2024-01-16T00:00:00.000Z"},
		{"plus one day", "2024-01-15||+1d", "", "", false, "2024-01-16T00:00:00.000Z"},
		{"round down datetime", "2024-01-15T23:30:00Z||/d", "", "", false, "2024-01-15T00:00:00.000Z"},
		{"lte day precision rounds up", "2024-01-15", "", "", true, "2024-01-15T23:59:59.999Z"},
		{"lt day precision stays", "2024-01-15", "", "", false, "2024-01-15T00:00:00.000Z"},
		{"exact seconds", "2024-01-15T10:00:00+05:00", "", "", false, "2024-01-15T05:00:00.000Z"},
		// test_date_range_time_zone (from 1704063600000)
		{"agg time_zone +02:00", "2024-01-01T01:00:00", "", "+02:00", false, "2023-12-31T23:00:00.000Z"},
		// test_date_range_format: format yyyy-MM, "2024-02" -> 1706745600000
		{"agg format yyyy-MM", "2024-02", "yyyy-MM", "", false, "2024-02-01T00:00:00.000Z"},
		// test_range_date_format
		{"dd/MM/yyyy", "02/01/2000", "dd/MM/yyyy", "", false, "2000-01-02T00:00:00.000Z"},
		{"query format applies to every bound", "2001-01-01", "dd/MM/yyyy", "", false, ""},
		// test_range_date_math_rounds_only_with_unit: additions never round
		{"lte plus day exact", "2024-01-02T12:00:00Z||+1d", "", "", true, "2024-01-03T12:00:00.000Z"},
		{"lte plus day then round", "2024-01-02T12:00:00Z||+1d/d", "", "", true, "2024-01-03T23:59:59.999Z"},
		{"lte day anchor plus hour exact", "2024-01-02||+1h", "", "", true, "2024-01-02T01:00:00.000Z"},
		{"lte round up then add", "2024-01-02||/d+1h", "", "", true, "2024-01-03T00:59:59.999Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms, ok := rangeBound(t, c.in, def, c.queryFormat, c.tz, c.roundUp)
			if c.want == "" {
				if ok {
					t.Fatalf("bound %q = %s, want parse error", c.in, FormatEpochMillis(ms))
				}
				return
			}
			if !ok {
				t.Fatalf("bound %q failed to parse, want %s", c.in, c.want)
			}
			if got := FormatEpochMillis(ms); got != c.want {
				t.Fatalf("bound %q = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestApplyMathRejectsGarbage(t *testing.T) {
	anchor := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	for _, expr := range []string{"+1", "+d", "/", "/q", "*1d", "+1x"} {
		if _, ok := ApplyMath(anchor, expr, false); ok {
			t.Errorf("ApplyMath(%q) accepted", expr)
		}
	}
	// "now-1d/d" style chains on a fixed anchor.
	got, ok := ApplyMath(anchor, "-1d/d", false)
	if !ok || !got.Equal(time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ApplyMath(-1d/d) = %v, %v", got, ok)
	}
}
