package painless

import "time"

// ZDTTime returns the instant of a ZonedDateTime value (doc['date'].value),
// for the score functions bound from outside the package.
func ZDTTime(v any) (time.Time, bool) {
	if z, ok := v.(*plZDT); ok {
		return z.t, true
	}
	return time.Time{}, false
}

// JavaClassName is the Java class name of a script value (for messages).
func JavaClassName(v any) string { return plClassName(v) }
