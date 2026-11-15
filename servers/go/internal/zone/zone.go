// Package zone is java.time.ZoneId as OpenSearch uses it for `time_zone` /
// `timezone` parameters: `ZoneId.of` parsing with Java's rules and error texts
// (fixed offsets, `UTC+h`/`GMT+h`/`UT+h` prefixes, case-sensitive tz database
// regions), and the zone-rule queries the date code needs (offset at an
// instant, valid offsets of a local time, gaps and transitions).
package zone

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"slopensearch/internal/apierr" // the Docker image has no zoneinfo
)

// Error is a failed `ZoneId.of`: the OpenSearch type of the Java exception
// (zone_rules_exception or date_time_exception), its message and the error
// it wraps (`GMT+19`: the invalid offset).
type Error struct {
	Kind    string
	Message string
	Cause   *Error
}

func (e *Error) Error() string { return e.Message }

// JavaClass is the Java exception class, which OpenSearch puts in front of
// some messages.
func (e *Error) JavaClass() string {
	if e.Kind == "zone_rules_exception" {
		return "java.time.zone.ZoneRulesException"
	}
	return "java.time.DateTimeException"
}

// Qualified is "<java class>: <message>", Throwable.toString.
func (e *Error) Qualified() string { return e.JavaClass() + ": " + e.Message }

// API is the exception as OpenSearch renders it (with its cause).
func (e *Error) API() *apierr.Error {
	out := &apierr.Error{Type: e.Kind, Reason: e.Message, Status: 400}
	if e.Cause != nil {
		out.CausedBy = e.Cause.API()
	}
	return out
}

// Wrapped is the IllegalArgumentException OpenSearch wraps the exception in
// ("java.time.zone.ZoneRulesException: Unknown time-zone ID: X").
func (e *Error) Wrapped() *apierr.Error {
	return &apierr.Error{Type: "illegal_argument_exception", Reason: e.Qualified(), Status: 400, CausedBy: e.API()}
}

func unknown(id string) *Error {
	return &Error{Kind: "zone_rules_exception", Message: "Unknown time-zone ID: " + id}
}

func invalid(msg string) *Error { return &Error{Kind: "date_time_exception", Message: msg} }

// Zone is a java.time.ZoneId: a fixed offset or a region with tz rules.
type Zone struct {
	ID    string
	loc   *time.Location
	fixed bool
	off   int // seconds, when fixed
}

// UTC is the zero offset.
var UTC = &Zone{ID: "Z", loc: time.UTC, fixed: true}

// notInJava are IDs the tz database has but Java's tzdb.dat leaves out.
var notInJava = map[string]bool{"EST": true, "MST": true, "HST": true, "EST5EDT": true,
	"CST6CDT": true, "MST7MDT": true, "PST8PDT": true, "ROC": true, "Local": true}

// ParseOffset is ZoneOffset.of: `Z`, `±h`, `±hh`, `±hh:mm`, `±hhmm`,
// `±hh:mm:ss`, `±hhmmss`; it returns the total seconds.
func ParseOffset(id string) (int, *Error) {
	if id == "Z" {
		return 0, nil
	}
	if id == "" || (id[0] != '+' && id[0] != '-') {
		return 0, invalid("Invalid ID for ZoneOffset, invalid format: " + id)
	}
	neg := id[0] == '-'
	p := id
	if len(p) == 2 { // `+h` is read as `+0h`
		p = p[:1] + "0" + p[1:]
	}
	var err *Error
	two := func(at int, colon bool) int {
		if err != nil {
			return 0
		}
		if colon && p[at-1] != ':' {
			err = invalid("Invalid ID for ZoneOffset, colon not found when expected: " + p)
			return 0
		}
		x, y := p[at], p[at+1]
		if x < '0' || x > '9' || y < '0' || y > '9' {
			err = invalid("Invalid ID for ZoneOffset, non numeric characters found: " + p)
			return 0
		}
		return int(x-'0')*10 + int(y-'0')
	}
	var h, m, s int
	switch len(p) {
	case 3:
		h = two(1, false)
	case 5:
		h, m = two(1, false), two(3, false)
	case 6:
		h, m = two(1, false), two(4, true)
	case 7:
		h, m, s = two(1, false), two(3, false), two(5, false)
	case 9:
		h, m, s = two(1, false), two(4, true), two(7, true)
	default:
		return 0, invalid("Invalid ID for ZoneOffset, invalid format: " + p)
	}
	if err != nil {
		return 0, err
	}
	sign := 1
	if neg {
		sign = -1
	}
	if h > 18 {
		return 0, invalid(fmt.Sprintf("Zone offset hours not in valid range: value %d is not in the range -18 to 18", sign*h))
	}
	if m > 59 {
		return 0, invalid(fmt.Sprintf("Zone offset minutes not in valid range: value %d is not in the range -59 to 59", sign*m))
	}
	if s > 59 {
		return 0, invalid(fmt.Sprintf("Zone offset seconds not in valid range: value %d is not in the range -59 to 59", sign*s))
	}
	if h == 18 && (m != 0 || s != 0) {
		return 0, invalid("Zone offset not in valid range: -18:00 to +18:00")
	}
	return sign * (h*3600 + m*60 + s), nil
}

// OffsetID is the ZoneOffset ID of an offset in seconds: `Z`, `+05:30`,
// `-00:00:30`.
func OffsetID(secs int) string {
	if secs == 0 {
		return "Z"
	}
	sign := '+'
	a := secs
	if a < 0 {
		sign, a = '-', -a
	}
	h, m, s := a/3600, a/60%60, a%60
	if s == 0 {
		return fmt.Sprintf("%c%02d:%02d", sign, h, m)
	}
	return fmt.Sprintf("%c%02d:%02d:%02d", sign, h, m, s)
}

// Fixed is the zone of a fixed offset in seconds.
func Fixed(secs int) *Zone {
	if secs == 0 {
		return UTC
	}
	id := OffsetID(secs)
	return &Zone{ID: id, loc: time.FixedZone(id, secs), fixed: true, off: secs}
}

func wellFormedRegion(id string) bool {
	if len(id) < 2 {
		return false
	}
	c := id[0]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	for i := 1; i < len(id); i++ {
		c := id[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("~/._+-", c) >= 0) {
			return false
		}
	}
	return true
}

// Of is ZoneId.of(id).
func Of(id string) (*Zone, *Error) {
	if id == "Z" || strings.HasPrefix(id, "+") || strings.HasPrefix(id, "-") {
		secs, err := ParseOffset(id)
		if err != nil {
			return nil, err
		}
		return Fixed(secs), nil
	}
	for _, prefix := range []string{"UTC", "GMT", "UT"} {
		if id == prefix {
			return &Zone{ID: id, loc: time.UTC, fixed: true}, nil
		}
		if rest, ok := strings.CutPrefix(id, prefix); ok && (strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-")) {
			secs, err := ParseOffset(rest)
			if err != nil {
				e := invalid("Invalid ID for offset-based ZoneId: " + id)
				e.Cause = err
				return nil, e
			}
			z := Fixed(secs)
			if secs != 0 {
				z = &Zone{ID: prefix + z.ID, loc: time.FixedZone(prefix+z.ID, secs), fixed: true, off: secs}
			} else {
				z = &Zone{ID: prefix, loc: time.UTC, fixed: true}
			}
			return z, nil
		}
	}
	if !wellFormedRegion(id) {
		return nil, invalid("Invalid ID for region-based ZoneId, invalid format: " + id)
	}
	if notInJava[id] || strings.Contains(id, "..") || strings.HasSuffix(id, "/") {
		return nil, unknown(id)
	}
	loc, err := time.LoadLocation(id)
	if err != nil || loc.String() != id {
		return nil, unknown(id)
	}
	z := &Zone{ID: id, loc: loc}
	// a region without transitions (Etc/GMT+5) is a fixed offset
	t := time.Unix(0, 0).In(loc)
	if start, end := t.ZoneBounds(); start.IsZero() && end.IsZero() {
		_, off := t.Zone()
		z.fixed, z.off = true, off
	}
	return z, nil
}

var cache sync.Map // id -> cached

type cached struct {
	z   *Zone
	err *Error
}

// Cached is Of through a process-wide cache (hot loops resolve the same IDs).
func Cached(id string) (*Zone, *Error) {
	if v, ok := cache.Load(id); ok {
		c := v.(cached)
		return c.z, c.err
	}
	z, err := Of(id)
	cache.Store(id, cached{z, err})
	return z, err
}

// Location is the zone as a time.Location (for formatting).
func (z *Zone) Location() *time.Location { return z.loc }

// FixedOffset is the offset in seconds when the zone has one fixed offset.
func (z *Zone) FixedOffset() (int, bool) { return z.off, z.fixed }

// OffsetMs is the offset in milliseconds at an instant (epoch ms).
func (z *Zone) OffsetMs(epochMs int64) int64 {
	if z.fixed {
		return int64(z.off) * 1000
	}
	_, off := time.UnixMilli(epochMs).In(z.loc).Zone()
	return int64(off) * 1000
}

// TransitionAtOrBefore is the latest offset transition at or before an
// instant, in epoch ms (transitions that only rename the zone are skipped).
func (z *Zone) TransitionAtOrBefore(epochMs int64) (int64, bool) {
	if z.fixed {
		return 0, false
	}
	t := time.UnixMilli(epochMs).In(z.loc)
	for range 64 {
		start, _ := t.ZoneBounds()
		if start.IsZero() {
			return 0, false
		}
		_, after := start.Zone()
		_, before := start.Add(-time.Second).Zone()
		if after != before {
			return start.UnixMilli(), true
		}
		t = start.Add(-time.Second)
	}
	return 0, false
}

// candidates are the offsets in force around an instant: the ones of its
// period and of the periods on either side.
func (z *Zone) candidates(epochMs int64) []int64 {
	t := time.UnixMilli(epochMs).In(z.loc)
	_, off := t.Zone()
	out := []int64{int64(off) * 1000}
	start, end := t.ZoneBounds()
	if !start.IsZero() {
		_, o := start.Add(-time.Second).Zone()
		out = append(out, int64(o)*1000)
	}
	if !end.IsZero() {
		_, o := end.Zone()
		out = append(out, int64(o)*1000)
	}
	return out
}

// ValidOffsets is ZoneRules.getValidOffsets of a local date-time (ms since
// 1970-01-01T00:00 local), in milliseconds: one offset, none in a gap, the
// earlier and the later one in an overlap.
func (z *Zone) ValidOffsets(localMs int64) []int64 {
	if z.fixed {
		return []int64{int64(z.off) * 1000}
	}
	seen := map[int64]bool{}
	var out []int64
	for _, probe := range []int64{localMs - z.OffsetMs(localMs), localMs - 18*3600_000, localMs + 18*3600_000} {
		for _, o := range z.candidates(probe) {
			if seen[o] {
				continue
			}
			seen[o] = true
			if z.OffsetMs(localMs-o) == o {
				out = append(out, o)
			}
		}
	}
	// the earlier instant (larger offset) first, as Java lists them
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out
}

// GapTransition is ZoneRules.getTransition(local) for a local time in a gap:
// the instant of the transition that skipped it, and the offset before it,
// both in milliseconds.
func (z *Zone) GapTransition(localMs int64) (int64, int64) {
	probe := localMs - z.OffsetMs(localMs)
	for _, p := range []int64{probe, localMs - 18*3600_000, localMs + 18*3600_000, probe - 86400_000} {
		t := time.UnixMilli(p).In(z.loc)
		for range 4 {
			start, end := t.ZoneBounds()
			for _, s := range []time.Time{start, end} {
				if s.IsZero() {
					continue
				}
				_, after := s.Zone()
				_, before := s.Add(-time.Millisecond).Zone()
				sm := s.UnixMilli()
				if localMs-int64(before)*1000 >= sm && localMs-int64(after)*1000 < sm {
					return sm, int64(before) * 1000
				}
			}
			if end.IsZero() {
				break
			}
			t = end
		}
	}
	return probe, z.OffsetMs(probe)
}

// LocalToEpochMs is the instant of a local date-time (ms since
// 1970-01-01T00:00 local), resolved like ZonedDateTime.ofLocal: a time in a
// gap moves forward by the gap's length, an overlap takes the earlier offset.
func (z *Zone) LocalToEpochMs(localMs int64) int64 {
	if z.fixed {
		return localMs - int64(z.off)*1000
	}
	if offs := z.ValidOffsets(localMs); len(offs) > 0 {
		return localMs - offs[0]
	}
	_, before := z.GapTransition(localMs)
	return localMs - before
}

// LocalToTime resolves the wall clock of t (its date and time, ignoring its
// location) in the zone, like LocalToEpochMs.
func (z *Zone) LocalToTime(t time.Time) time.Time {
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	local := time.Date(y, mo, d, h, mi, s, t.Nanosecond(), time.UTC)
	ms := local.UnixMilli()
	sub := local.Sub(time.UnixMilli(ms))
	return time.UnixMilli(z.LocalToEpochMs(ms)).Add(sub).In(z.loc)
}
