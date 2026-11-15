package aggs

import (
	"math"
	"time"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/zone"
)

// aggZone resolves the time_zone of an aggregation with ZoneId.of; an invalid
// ID fails like the aggregation parser does ("[date_histogram] failed to parse
// field [time_zone]", the HTTP layer puts the position in front).
func aggZone(spec map[string]any, aggType string) (*zone.Zone, *apierr.Error) {
	raw, ok := spec["time_zone"]
	if !ok || raw == nil {
		return zone.UTC, nil
	}
	tz, _ := jsonv.ToString(raw)
	z, err := zone.Cached(tz)
	if err != nil {
		return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "[" + aggType + "] failed to parse field [time_zone]",
			CausedBy: err.API()}
	}
	return z, nil
}

// roundLocalMs rounds a local time (ms since 1970-01-01T00:00 local) down to a
// fixed interval or a calendar unit.
func roundLocalMs(local, fixedMs int64, unit string) int64 {
	if fixedMs > 0 {
		return int64(math.Floor(float64(local)/float64(fixedMs))) * fixedMs
	}
	return calendarBucketStart(local, unit, time.UTC)
}

// tzBucketStart rounds ms down to its bucket in z the way OpenSearch's
// Rounding does: calendar units of a day or more start at the first instant of
// the local day (JavaTimeToMidnightRounding); hour and minute units
// (JavaTimeNotToMidnightRounding) and fixed intervals (TimeIntervalRounding)
// round the local time and step back over an offset transition the rounding
// would cross.
func tzBucketStart(ms, fixedMs int64, unit string, z *zone.Zone) int64 {
	if off, ok := z.FixedOffset(); ok {
		o := int64(off) * 1000
		return roundLocalMs(ms+o, fixedMs, unit) - o
	}
	switch {
	case fixedMs > 0:
		return timeIntervalRound(ms, fixedMs, z)
	case unit == "hour" || unit == "minute" || unit == "second":
		return notToMidnightRound(ms, unit, z)
	}
	midnight := roundLocalMs(ms+z.OffsetMs(ms), 0, unit)
	if offs := z.ValidOffsets(midnight); len(offs) > 0 {
		return midnight - offs[0]
	}
	t, _ := z.GapTransition(midnight)
	return t
}

// timeIntervalRound is TimeIntervalRounding.JavaTimeRounding.round.
func timeIntervalRound(ms, interval int64, z *zone.Zone) int64 {
	for range 64 {
		local := roundLocalMs(ms+z.OffsetMs(ms), interval, "")
		offs := z.ValidOffsets(local)
		if len(offs) == 0 {
			// the rounded local time is in a gap: the gap's start
			t, _ := z.GapTransition(local)
			return t
		}
		prev, hasPrev := z.TransitionAtOrBefore(ms)
		stepBack := false
		for i := len(offs) - 1; i >= 0; i-- {
			inst := local - offs[i]
			if hasPrev && inst < prev {
				stepBack = true
				break
			}
			if inst <= ms {
				return inst
			}
		}
		if !stepBack {
			return local - offs[0]
		}
		ms = prev - 1
	}
	return ms
}

// notToMidnightRound is JavaTimeNotToMidnightRounding.round.
func notToMidnightRound(ms int64, unit string, z *zone.Zone) int64 {
	for range 64 {
		local := roundLocalMs(ms+z.OffsetMs(ms), 0, unit)
		truncated, ok := int64(0), false
		offs := z.ValidOffsets(local)
		for i := len(offs) - 1; i >= 0; i-- {
			if inst := local - offs[i]; inst <= ms {
				truncated, ok = inst, true
				break
			}
		}
		// a transition at the instant itself counts (as in OpenSearch's
		// LocalTimeOffset lookup): rounding must not cross it
		prev, hasPrev := z.TransitionAtOrBefore(ms)
		if !hasPrev {
			if ok {
				return truncated
			}
			return local - z.OffsetMs(ms)
		}
		if ok && prev <= truncated {
			return truncated
		}
		ms = prev - 1
	}
	return ms
}

// tzNextKey is the key of the bucket after the one starting at k.
func tzNextKey(k, fixedMs int64, unit string, z *zone.Zone) int64 {
	var next int64
	if fixedMs > 0 {
		// TimeIntervalRounding.nextRoundingValue: the local time one interval
		// later, resolved in the zone
		next = z.LocalToEpochMs(k + z.OffsetMs(k) + fixedMs)
		if next <= k {
			next = k + fixedMs
		}
		return next
	}
	next = addCalendarUnit(k, unit, z.Location())
	if _, fixed := z.FixedOffset(); !fixed {
		if r := tzBucketStart(next, 0, unit, z); r > k {
			next = r
		}
	}
	return next
}
