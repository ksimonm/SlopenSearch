package model

import (
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/settings"
)

// Scheduled refreshes. OpenSearch refreshes every shard each
// index.refresh_interval (1s by default; a search-idle shard refreshes
// before it is searched), so an unrefreshed write is searchable at the
// latest one interval after it happened. The server calls TickRefresh
// before and after every request: the first call that sees unrefreshed
// writes starts their clock, a later one past the interval refreshes.

// refreshInterval is index.refresh_interval; ok is false when periodic
// refreshes are off (-1).
func (idx *Index) refreshInterval() (time.Duration, bool) {
	v, found := settings.Find(idx.Settings, "refresh_interval")
	if !found {
		return time.Second, true
	}
	s := strings.TrimSpace(settings.ValueString(v))
	if strings.HasPrefix(s, "-") {
		return 0, false
	}
	i := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if i < 0 {
		i = len(s)
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return time.Second, true
	}
	unit := map[string]time.Duration{"nanos": time.Nanosecond, "micros": time.Microsecond, "ms": time.Millisecond,
		"": time.Millisecond, "s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}
	u, known := unit[s[i:]]
	if !known {
		return time.Second, true
	}
	return time.Duration(n) * u, true
}

// RefreshTickDue reports whether TickRefresh would change the index.
func (idx *Index) RefreshTickDue(now time.Time) bool {
	if len(idx.Pending) == 0 && len(idx.Stale) == 0 {
		return !idx.pendingSince.IsZero()
	}
	if idx.pendingSince.IsZero() {
		return true
	}
	if idx.Closed {
		return false
	}
	d, on := idx.refreshInterval()
	return on && now.Sub(idx.pendingSince) >= d
}

// TickRefresh starts the clock of unrefreshed writes or refreshes the index
// once its refresh interval has passed since they were first seen.
func (idx *Index) TickRefresh(now time.Time) {
	switch {
	case len(idx.Pending) == 0 && len(idx.Stale) == 0:
		idx.pendingSince = time.Time{}
	case idx.pendingSince.IsZero():
		idx.pendingSince = now
	case idx.RefreshTickDue(now):
		idx.Refresh()
		idx.pendingSince = time.Time{}
	}
}
