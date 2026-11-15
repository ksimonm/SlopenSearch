package zone

import (
	"testing"
	"time"
)

// expected texts: tests/reference/test_37_parity_time_hash.jsonl
func TestOfErrors(t *testing.T) {
	cases := []struct{ id, kind, msg, cause string }{
		{"Mars/Olympus", "zone_rules_exception", "Unknown time-zone ID: Mars/Olympus", ""},
		{"+25:00", "date_time_exception", "Zone offset hours not in valid range: value 25 is not in the range -18 to 18", ""},
		{"EST", "zone_rules_exception", "Unknown time-zone ID: EST", ""},
		{"asia/kolkata", "zone_rules_exception", "Unknown time-zone ID: asia/kolkata", ""},
		{"+5:30", "date_time_exception", "Invalid ID for ZoneOffset, non numeric characters found: +5:30", ""},
		{"GMT+19", "date_time_exception", "Invalid ID for offset-based ZoneId: GMT+19",
			"Zone offset hours not in valid range: value 19 is not in the range -18 to 18"},
		{"1Bad", "date_time_exception", "Invalid ID for region-based ZoneId, invalid format: 1Bad", ""},
		{"Europe/", "zone_rules_exception", "Unknown time-zone ID: Europe/", ""},
		{"Local", "zone_rules_exception", "Unknown time-zone ID: Local", ""},
	}
	for _, c := range cases {
		z, err := Of(c.id)
		if err == nil {
			t.Errorf("%s: got zone %v", c.id, z.ID)
			continue
		}
		if err.Kind != c.kind || err.Message != c.msg {
			t.Errorf("%s: %s %q", c.id, err.Kind, err.Message)
		}
		if c.cause != "" && (err.Cause == nil || err.Cause.Message != c.cause) {
			t.Errorf("%s: cause %v", c.id, err.Cause)
		}
	}
}

func TestOfValid(t *testing.T) {
	cases := []struct {
		id    string
		fixed bool
		off   int
	}{
		{"Z", true, 0}, {"UTC", true, 0}, {"GMT+2", true, 7200}, {"UTC-03:30", true, -12600},
		{"+3", true, 10800}, {"-05:30", true, -19800}, {"Etc/GMT+5", true, -18000},
		{"Asia/Kolkata", false, 0}, {"America/New_York", false, 0}, {"US/Eastern", false, 0},
	}
	for _, c := range cases {
		z, err := Of(c.id)
		if err != nil {
			t.Errorf("%s: %v", c.id, err)
			continue
		}
		off, fixed := z.FixedOffset()
		if fixed != c.fixed || (fixed && off != c.off) {
			t.Errorf("%s: fixed=%v off=%d", c.id, fixed, off)
		}
	}
}

func local(s string) int64 {
	t, _ := time.Parse("2006-01-02T15:04:05", s)
	return t.UnixMilli()
}

func TestLocalResolution(t *testing.T) {
	ny, _ := Of("America/New_York")
	// gap: 02:30 moves forward to 03:30 EDT
	if got, want := ny.LocalToEpochMs(local("2024-03-10T02:30:00")), local("2024-03-10T07:30:00"); got != want {
		t.Errorf("gap: %d want %d", got, want)
	}
	if offs := ny.ValidOffsets(local("2024-03-10T02:30:00")); len(offs) != 0 {
		t.Errorf("gap offsets: %v", offs)
	}
	if tr, before := ny.GapTransition(local("2024-03-10T02:30:00")); tr != local("2024-03-10T07:00:00") || before != -5*3600_000 {
		t.Errorf("gap transition: %d %d", tr, before)
	}
	// overlap: 01:30 takes the earlier offset (EDT)
	if offs := ny.ValidOffsets(local("2024-11-03T01:30:00")); len(offs) != 2 || offs[0] != -4*3600_000 {
		t.Errorf("overlap offsets: %v", offs)
	}
	if got, want := ny.LocalToEpochMs(local("2024-11-03T01:30:00")), local("2024-11-03T05:30:00"); got != want {
		t.Errorf("overlap: %d want %d", got, want)
	}
	if tr, ok := ny.TransitionAtOrBefore(local("2024-06-01T00:00:00")); !ok || tr != local("2024-03-10T07:00:00") {
		t.Errorf("transition: %d %v", tr, ok)
	}
}
