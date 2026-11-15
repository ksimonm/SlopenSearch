package aggs

import (
	"reflect"
	"testing"

	"slopensearch/internal/jsonv"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := jsonv.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad JSON %s: %v", s, err)
	}
	return v
}

func TestFmtMetricNumber(t *testing.T) {
	// "value_as_string" of tests/reference/test_42_aggs_metric.jsonl
	// (test_numeric_format, test_stats_numeric_format, test_percentiles_format,
	// test_bucket_script_format) and the "from_as_string" of
	// test_41_aggs_bucket.jsonl (test_range_format_adds_as_string).
	tests := []struct {
		f      float64
		format string
		want   string
	}{
		{10, "0000.0", "0010.0"},
		{30, "#.00", "30.00"},
		{50, "#", "50"},
		{150, "0.000", "150.000"},
		{10, "0.0", "10.0"},
		{30, "0.00", "30.00"},
		{1, "0.00", "1.00"},
		{1, "#.0", "1.0"},
		{2, "#.0", "2.0"},
		// no format: whole doubles without a fraction
		{30, "", "30"},
		{2.5, "", "2.5"},
	}
	for _, tt := range tests {
		if got := fmtMetricNumber(tt.f, tt.format); got != tt.want {
			t.Errorf("fmtMetricNumber(%v, %q) = %q, want %q", tt.f, tt.format, got, tt.want)
		}
	}
}

func TestRangeAndPercentileKeys(t *testing.T) {
	// range bucket keys like "1.0-2.0" and percentile keys like "50.0"
	tests := []struct {
		f          float64
		rangeKey   string
		percentile string
	}{
		{1, "1.0", "1.0"},
		{2, "2.0", "2.0"},
		{50, "50.0", "50.0"},
		{99.9, "99.9", "99.9"},
		{0, "0.0", "0.0"},
		// Double.toString (test_35 test_percentile_keys_and_double_terms)
		{1e7, "1.0E7", "1.0E7"},
		{1e-4, "1.0E-4", "1.0E-4"},
		{99.99999, "99.99999", "99.99999"},
	}
	for _, tt := range tests {
		if got := rangeKeyNumber(tt.f); got != tt.rangeKey {
			t.Errorf("rangeKeyNumber(%v) = %q, want %q", tt.f, got, tt.rangeKey)
		}
		if got := percentileKey(tt.f); got != tt.percentile {
			t.Errorf("percentileKey(%v) = %q, want %q", tt.f, got, tt.percentile)
		}
	}
}

func TestGeotileKey(t *testing.T) {
	// test_geotile_grid_precision_{0,1,2} in test_41_aggs_bucket.jsonl:
	// Amsterdam, Berlin and Kyiv share a tile, New York is in another.
	tests := []struct {
		lat, lon  float64
		precision int
		want      string
	}{
		{52.37, 4.89, 0, "0/0/0"},
		{40.71, -74.0, 0, "0/0/0"},
		{52.37, 4.89, 1, "1/1/0"},
		{52.52, 13.4, 1, "1/1/0"},
		{50.45, 30.52, 1, "1/1/0"},
		{40.71, -74.0, 1, "1/0/0"},
		{52.37, 4.89, 2, "2/2/1"},
		{50.45, 30.52, 2, "2/2/1"},
		{40.71, -74.0, 2, "2/1/1"},
	}
	for _, tt := range tests {
		if got := geotileKey(tt.lat, tt.lon, tt.precision); got != tt.want {
			t.Errorf("geotileKey(%v, %v, %d) = %q, want %q", tt.lat, tt.lon, tt.precision, got, tt.want)
		}
	}
}

func TestParsePercents(t *testing.T) {
	// Errors from test_percentiles_invalid_params in test_42_aggs_metric.jsonl.
	const top = "Failed to build [percentiles] after last required field arrived"
	errTests := []struct {
		raw, cause string
	}{
		{`[]`, "[percents] must not be empty: [a]"},
		{`[101]`, "percent must be in [0,100], got [101.0]: [a]"},
		{`[-1]`, "percent must be in [0,100], got [-1.0]: [a]"},
	}
	for _, tt := range errTests {
		_, err := parsePercents(decode(t, tt.raw), nil)
		if err == nil {
			t.Errorf("parsePercents(%s) succeeded", tt.raw)
			continue
		}
		if err.Type != "x_content_parse_exception" || err.Reason != top || err.CausedBy == nil ||
			err.CausedBy.Type != "illegal_argument_exception" || err.CausedBy.Reason != tt.cause {
			t.Errorf("parsePercents(%s) = %s %q caused_by %+v, want cause %q", tt.raw, err.Type, err.Reason, err.CausedBy, tt.cause)
		}
	}

	def := []float64{1, 50, 99}
	okTests := []struct {
		raw  string
		want []float64
	}{
		{`null`, def},
		{`[50]`, []float64{50}},
		{`[0,100]`, []float64{0, 100}},
		{`[99,50,50,1]`, []float64{1, 50, 99}}, // sorted, deduplicated
	}
	for _, tt := range okTests {
		got, err := parsePercents(decode(t, tt.raw), def)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parsePercents(%s) = %v, %v, want %v", tt.raw, got, err, tt.want)
		}
	}
}

func TestGapPolicy(t *testing.T) {
	// test_invalid_gap_policy in test_42_aggs_metric.jsonl.
	tests := []struct {
		body, want, reason string
	}{
		{`{}`, "skip", ""},
		{`{"gap_policy":"skip"}`, "skip", ""},
		{`{"gap_policy":"insert_zeros"}`, "insert_zeros", ""},
		{`{"gap_policy":"bogus"}`, "", "Invalid gap policy: [bogus], accepted values: [insert_zeros, skip]"},
	}
	for _, tt := range tests {
		got, err := gapPolicyOf(decode(t, tt.body).(map[string]any))
		if tt.reason != "" {
			if err == nil || err.Type != "parsing_exception" || err.Reason != tt.reason {
				t.Errorf("gapPolicyOf(%s) error = %+v, want parsing_exception %q", tt.body, err, tt.reason)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("gapPolicyOf(%s) = %q, %v, want %q", tt.body, got, err, tt.want)
		}
	}
}
