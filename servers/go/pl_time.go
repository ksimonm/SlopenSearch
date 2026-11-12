package main

import (
	"fmt"
	"strings"
	"time"
)

// Painless engine: the java.time subset.

type plZDT struct{ t time.Time }
type plLocalDate struct{ t time.Time } // midnight UTC of the date
type plInstant struct {
	ms    int64
	nanos int64 // nanos within the millisecond
}
type plDuration struct{ d time.Duration }
type plDayOfWeek int // 1 = MONDAY
type plChronoUnit string
type plZoneID string
type plDTF struct {
	pattern string
	iso     bool
}

var plDayNames = []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}
var plMonthNames = []string{"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY",
	"AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER"}

var plChronoUnits = map[string]time.Duration{
	"NANOS": time.Nanosecond, "MICROS": time.Microsecond, "MILLIS": time.Millisecond,
	"SECONDS": time.Second, "MINUTES": time.Minute, "HOURS": time.Hour,
	"HALF_DAYS": 12 * time.Hour, "DAYS": 24 * time.Hour, "WEEKS": 7 * 24 * time.Hour,
	"MONTHS": 0, "YEARS": 0, "DECADES": 0, "CENTURIES": 0, "MILLENNIA": 0,
}

func (d plDayOfWeek) javaString() string { return plDayNames[int(d)-1] }
func (u plChronoUnit) javaString() string {
	return strings.ToUpper(string(u)[:1]) + strings.ToLower(string(u)[1:])
}
func (z plZoneID) javaString() string     { return string(z) }
func (z *plZDT) javaString() string       { return plFmtZDT(z.t) }
func (d *plLocalDate) javaString() string { return d.t.Format("2006-01-02") }
func (i *plInstant) javaString() string {
	t := time.UnixMilli(i.ms).UTC().Add(time.Duration(i.nanos))
	return t.Format("2006-01-02T15:04:05") + plFmtNanos(t.Nanosecond()) + "Z"
}
func (d *plDuration) javaString() string { return plFmtDuration(d.d) }
func (f *plDTF) javaString() string {
	if f.iso {
		return "ParseCaseSensitive(false)(Value(Year,4,10,EXCEEDS_PAD)'-'Value(MonthOfYear,2)'-'Value(DayOfMonth,2))'T'(Value(HourOfDay,2)':'Value(MinuteOfHour,2)[':'Value(SecondOfMinute,2)[Fraction(NanoOfSecond,0,9,DecimalPoint)]])"
	}
	return f.pattern
}

func plFmtNanos(n int) string {
	switch {
	case n == 0:
		return ""
	case n%1000000 == 0:
		return fmt.Sprintf(".%03d", n/1000000)
	case n%1000 == 0:
		return fmt.Sprintf(".%06d", n/1000)
	}
	return fmt.Sprintf(".%09d", n)
}

// plFmtZDT renders ZonedDateTime.toString(): seconds only when non-zero.
func plFmtZDT(t time.Time) string {
	s := t.Format("2006-01-02T15:04")
	if t.Second() != 0 || t.Nanosecond() != 0 {
		s += fmt.Sprintf(":%02d", t.Second()) + plFmtNanos(t.Nanosecond())
	}
	_, off := t.Zone()
	if off == 0 {
		s += "Z"
		if name := t.Location().String(); name != "UTC" && name != "" && !strings.HasPrefix(name, "+") {
			s += "[" + name + "]"
		}
		return s
	}
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	s += fmt.Sprintf("%s%02d:%02d", sign, off/3600, (off%3600)/60)
	if name := t.Location().String(); name != "" && !strings.HasPrefix(name, "+") && !strings.HasPrefix(name, "-") && name != "Local" {
		s += "[" + name + "]"
	}
	return s
}

func plFmtDuration(d time.Duration) string {
	if d == 0 {
		return "PT0S"
	}
	var sb strings.Builder
	sb.WriteString("PT")
	neg := d < 0
	if neg {
		d = -d
	}
	sign := ""
	if neg {
		sign = "-"
	}
	h := int64(d / time.Hour)
	m := int64((d % time.Hour) / time.Minute)
	sec := d % time.Minute
	if h != 0 {
		sb.WriteString(fmt.Sprintf("%s%dH", sign, h))
	}
	if m != 0 {
		sb.WriteString(fmt.Sprintf("%s%dM", sign, m))
	}
	if sec != 0 {
		whole := int64(sec / time.Second)
		frac := int(sec % time.Second)
		s := fmt.Sprintf("%s%d", sign, whole) + strings.TrimRight(plFmtNanos(frac), "0")
		sb.WriteString(s + "S")
	}
	return sb.String()
}

func plParseZDT(s string, off int) (*plZDT, *plRtErr) {
	zone := ""
	if i := strings.IndexByte(s, '['); i >= 0 && strings.HasSuffix(s, "]") {
		zone = s[i+1 : len(s)-1]
		s = s[:i]
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04Z07:00", s)
	}
	if err != nil {
		return nil, &plRtErr{typ: "date_time_parse_exception", offset: off,
			reason: "Text '" + s + "' could not be parsed at index 0"}
	}
	if zone != "" {
		if loc, lerr := time.LoadLocation(zone); lerr == nil {
			t = t.In(loc)
		}
	} else if _, o := t.Zone(); o == 0 {
		t = t.UTC()
	}
	return &plZDT{t: t}, nil
}

func plDateOf(y, m, d int) *plLocalDate {
	return &plLocalDate{t: time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)}
}

func (in *plInterp) callStaticTime(class, name string, args []any, off int) (any, *plRtErr, bool) {
	i := func(k int) int64 { return plI64(args[k]) }
	switch class {
	case "Instant":
		switch name {
		case "ofEpochMilli":
			return &plInstant{ms: i(0)}, nil, true
		case "ofEpochSecond":
			return &plInstant{ms: i(0) * 1000}, nil, true
		case "parse":
			z, err := plParseZDT(plToString(args[0]), off)
			if err != nil {
				return nil, err, true
			}
			return &plInstant{ms: z.t.UnixMilli(), nanos: int64(z.t.Nanosecond() % 1000000)}, nil, true
		}
	case "ZonedDateTime":
		switch name {
		case "parse":
			z, err := plParseZDT(plToString(args[0]), off)
			return z, err, true
		case "ofInstant":
			inst, _ := args[0].(*plInstant)
			if inst == nil {
				return nil, plNPE(off), true
			}
			t := time.UnixMilli(inst.ms).UTC().Add(time.Duration(inst.nanos))
			if z, ok := args[1].(plZoneID); ok && z != "Z" && z != "UTC" {
				if loc, err := time.LoadLocation(string(z)); err == nil {
					t = t.In(loc)
				}
			}
			return &plZDT{t: t}, nil, true
		case "of":
			t := time.Date(int(i(0)), time.Month(i(1)), int(i(2)), int(i(3)), int(i(4)), int(i(5)), int(i(6)), time.UTC)
			return &plZDT{t: t}, nil, true
		}
	case "LocalDate":
		switch name {
		case "of":
			return plDateOf(int(i(0)), int(i(1)), int(i(2))), nil, true
		case "parse":
			t, err := time.Parse("2006-01-02", plToString(args[0]))
			if err != nil {
				return nil, &plRtErr{typ: "date_time_parse_exception", offset: off,
					reason: "Text '" + plToString(args[0]) + "' could not be parsed at index 0"}, true
			}
			return &plLocalDate{t: t}, nil, true
		case "ofEpochDay":
			return &plLocalDate{t: time.Unix(i(0)*86400, 0).UTC()}, nil, true
		}
	case "ZoneId":
		if name == "of" {
			return plZoneID(plToString(args[0])), nil, true
		}
	case "Duration":
		unit := map[string]time.Duration{"ofDays": 24 * time.Hour, "ofHours": time.Hour, "ofMinutes": time.Minute,
			"ofSeconds": time.Second, "ofMillis": time.Millisecond, "ofNanos": time.Nanosecond}
		if u, ok := unit[name]; ok {
			return &plDuration{d: time.Duration(i(0)) * u}, nil, true
		}
		if name == "between" {
			a, aok := plTemporalTime(args[0])
			b, bok := plTemporalTime(args[1])
			if !aok || !bok {
				return nil, plIAE(off, "Unsupported temporal"), true
			}
			return &plDuration{d: b.Sub(a)}, nil, true
		}
	case "DayOfWeek":
		if name == "of" {
			return plDayOfWeek(i(0)), nil, true
		}
		if name == "valueOf" {
			for k, d := range plDayNames {
				if d == plToString(args[0]) {
					return plDayOfWeek(k + 1), nil, true
				}
			}
		}
	case "DateTimeFormatter":
		if name == "ofPattern" {
			return &plDTF{pattern: plToString(args[0])}, nil, true
		}
	}
	return nil, nil, false
}

func plTemporalTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case *plZDT:
		return t.t, true
	case *plLocalDate:
		return t.t, true
	case *plInstant:
		return time.UnixMilli(t.ms).UTC().Add(time.Duration(t.nanos)), true
	}
	return time.Time{}, false
}

func plMonthsBetween(a, b time.Time) int64 {
	months := int64(b.Year()-a.Year())*12 + int64(b.Month()-a.Month())
	// a partial month does not count
	aRest := time.Date(2000, 1, a.Day(), a.Hour(), a.Minute(), a.Second(), a.Nanosecond(), time.UTC)
	bRest := time.Date(2000, 1, b.Day(), b.Hour(), b.Minute(), b.Second(), b.Nanosecond(), time.UTC)
	if months > 0 && bRest.Before(aRest) {
		months--
	} else if months < 0 && bRest.After(aRest) {
		months++
	}
	return months
}

func plAddUnit(t time.Time, unit string, n int64) time.Time {
	switch unit {
	case "YEARS":
		return t.AddDate(int(n), 0, 0)
	case "MONTHS":
		y, m, d := t.Date()
		target := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if d > last {
			d = last
		}
		return time.Date(target.Year(), target.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	case "WEEKS":
		return t.AddDate(0, 0, int(7*n))
	case "DAYS":
		return t.AddDate(0, 0, int(n))
	}
	return t.Add(time.Duration(n) * plChronoUnits[unit])
}

// plFormatPattern renders a time with a java.time DateTimeFormatter pattern.
func plFormatPattern(p string, t time.Time) string {
	var sb strings.Builder
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\'' {
			j := strings.IndexByte(p[i+1:], '\'')
			if j < 0 {
				sb.WriteString(p[i+1:])
				break
			}
			if j == 0 {
				sb.WriteByte('\'')
			} else {
				sb.WriteString(p[i+1 : i+1+j])
			}
			i += j + 2
			continue
		}
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			sb.WriteByte(c)
			i++
			continue
		}
		j := i
		for j < len(p) && p[j] == c {
			j++
		}
		n := j - i
		pad := func(v int) string { return fmt.Sprintf("%0*d", n, v) }
		switch c {
		case 'y', 'u':
			if n == 2 {
				sb.WriteString(fmt.Sprintf("%02d", t.Year()%100))
			} else {
				sb.WriteString(pad(t.Year()))
			}
		case 'M', 'L':
			switch {
			case n >= 4:
				name := plMonthNames[t.Month()-1]
				sb.WriteString(name[:1] + strings.ToLower(name[1:]))
			case n == 3:
				name := plMonthNames[t.Month()-1]
				sb.WriteString(name[:1] + strings.ToLower(name[1:3]))
			default:
				sb.WriteString(pad(int(t.Month())))
			}
		case 'd':
			sb.WriteString(pad(t.Day()))
		case 'D':
			sb.WriteString(pad(t.YearDay()))
		case 'H':
			sb.WriteString(pad(t.Hour()))
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			sb.WriteString(pad(h))
		case 'm':
			sb.WriteString(pad(t.Minute()))
		case 's':
			sb.WriteString(pad(t.Second()))
		case 'S':
			frac := fmt.Sprintf("%09d", t.Nanosecond())
			if n <= 9 {
				sb.WriteString(frac[:n])
			} else {
				sb.WriteString(frac)
			}
		case 'a':
			if t.Hour() < 12 {
				sb.WriteString("AM")
			} else {
				sb.WriteString("PM")
			}
		case 'E':
			name := plDayNames[(int(t.Weekday())+6)%7]
			if n >= 4 {
				sb.WriteString(name[:1] + strings.ToLower(name[1:]))
			} else {
				sb.WriteString(name[:1] + strings.ToLower(name[1:3]))
			}
		case 'X', 'x', 'Z':
			_, o := t.Zone()
			if o == 0 && c == 'X' {
				sb.WriteString("Z")
			} else {
				sign := "+"
				if o < 0 {
					sign = "-"
					o = -o
				}
				sb.WriteString(fmt.Sprintf("%s%02d%02d", sign, o/3600, (o%3600)/60))
			}
		case 'z', 'V':
			sb.WriteString(t.Location().String())
		default:
			sb.WriteString(p[i:j])
		}
		i = j
	}
	return sb.String()
}

func (in *plInterp) timeMethod(recv any, name string, args []any, off int) (any, *plRtErr, bool) {
	n := len(args)
	switch t := recv.(type) {
	case *plZDT:
		tt := t.t
		switch name + "/" + itoa(n) {
		case "getYear/0":
			return int32(tt.Year()), nil, true
		case "getMonthValue/0":
			return int32(tt.Month()), nil, true
		case "getMonth/0":
			return plStringEnum(plMonthNames[tt.Month()-1]), nil, true
		case "getDayOfMonth/0":
			return int32(tt.Day()), nil, true
		case "getDayOfYear/0":
			return int32(tt.YearDay()), nil, true
		case "getDayOfWeek/0":
			return plDayOfWeek((int(tt.Weekday())+6)%7 + 1), nil, true
		case "getHour/0":
			return int32(tt.Hour()), nil, true
		case "getMinute/0":
			return int32(tt.Minute()), nil, true
		case "getSecond/0":
			return int32(tt.Second()), nil, true
		case "getNano/0":
			return int32(tt.Nanosecond()), nil, true
		case "toEpochSecond/0":
			return tt.Unix(), nil, true
		case "toInstant/0":
			return &plInstant{ms: tt.UnixMilli(), nanos: int64(tt.Nanosecond() % 1000000)}, nil, true
		case "toLocalDate/0":
			return plDateOf(tt.Year(), int(tt.Month()), tt.Day()), nil, true
		case "getZone/0":
			if _, o := tt.Zone(); o == 0 {
				return plZoneID("Z"), nil, true
			}
			return plZoneID(tt.Location().String()), nil, true
		case "isBefore/1", "isAfter/1", "isEqual/1":
			o, ok := plTemporalTime(args[0])
			if !ok {
				return nil, plNPE(off), true
			}
			switch name {
			case "isBefore":
				return tt.Before(o), nil, true
			case "isAfter":
				return tt.After(o), nil, true
			}
			return tt.Equal(o), nil, true
		case "format/1":
			f, ok := args[0].(*plDTF)
			if !ok {
				return nil, plNPE(off), true
			}
			if f.iso {
				return plFmtZDT(tt), nil, true
			}
			return plFormatPattern(f.pattern, tt), nil, true
		case "truncatedTo/1":
			u := string(plToChrono(args[0]))
			d := plChronoUnits[u]
			if d > 0 {
				return &plZDT{t: tt.Truncate(d)}, nil, true
			}
			return t, nil, true
		case "withZoneSameInstant/1":
			if z, ok := args[0].(plZoneID); ok {
				if loc, err := time.LoadLocation(string(z)); err == nil && z != "Z" {
					return &plZDT{t: tt.In(loc)}, nil, true
				}
				return &plZDT{t: tt.UTC()}, nil, true
			}
		case "plus/2", "minus/2":
			k := plI64(args[0])
			if name == "minus" {
				k = -k
			}
			return &plZDT{t: plAddUnit(tt, string(plToChrono(args[1])), k)}, nil, true
		case "plus/1", "minus/1":
			if d, ok := args[0].(*plDuration); ok {
				if name == "minus" {
					return &plZDT{t: tt.Add(-d.d)}, nil, true
				}
				return &plZDT{t: tt.Add(d.d)}, nil, true
			}
		case "compareTo/1":
			o, _ := plTemporalTime(args[0])
			return int32(tt.Compare(o)), nil, true
		case "withDayOfMonth/1":
			return &plZDT{t: time.Date(tt.Year(), tt.Month(), int(plI64(args[0])), tt.Hour(), tt.Minute(), tt.Second(), tt.Nanosecond(), tt.Location())}, nil, true
		case "withHour/1":
			return &plZDT{t: time.Date(tt.Year(), tt.Month(), tt.Day(), int(plI64(args[0])), tt.Minute(), tt.Second(), tt.Nanosecond(), tt.Location())}, nil, true
		}
		if n == 1 && (strings.HasPrefix(name, "plus") || strings.HasPrefix(name, "minus")) {
			unit := strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(name, "plus"), "minus"))
			if _, ok := plChronoUnits[unit]; ok {
				k := plI64(args[0])
				if strings.HasPrefix(name, "minus") {
					k = -k
				}
				return &plZDT{t: plAddUnit(tt, unit, k)}, nil, true
			}
		}
	case *plLocalDate:
		tt := t.t
		switch name + "/" + itoa(n) {
		case "getYear/0":
			return int32(tt.Year()), nil, true
		case "getMonthValue/0":
			return int32(tt.Month()), nil, true
		case "getMonth/0":
			return plStringEnum(plMonthNames[tt.Month()-1]), nil, true
		case "getDayOfMonth/0":
			return int32(tt.Day()), nil, true
		case "getDayOfYear/0":
			return int32(tt.YearDay()), nil, true
		case "getDayOfWeek/0":
			return plDayOfWeek((int(tt.Weekday())+6)%7 + 1), nil, true
		case "lengthOfMonth/0":
			return int32(time.Date(tt.Year(), tt.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()), nil, true
		case "isLeapYear/0":
			y := tt.Year()
			return y%4 == 0 && (y%100 != 0 || y%400 == 0), nil, true
		case "toEpochDay/0":
			return tt.Unix() / 86400, nil, true
		case "isBefore/1", "isAfter/1", "isEqual/1":
			o, ok := plTemporalTime(args[0])
			if !ok {
				return nil, plNPE(off), true
			}
			switch name {
			case "isBefore":
				return tt.Before(o), nil, true
			case "isAfter":
				return tt.After(o), nil, true
			}
			return tt.Equal(o), nil, true
		case "format/1":
			if f, ok := args[0].(*plDTF); ok {
				if f.iso {
					return tt.Format("2006-01-02"), nil, true
				}
				return plFormatPattern(f.pattern, tt), nil, true
			}
		case "atStartOfDay/0", "atStartOfDay/1":
			return &plZDT{t: tt}, nil, true
		case "plus/2", "minus/2":
			k := plI64(args[0])
			if name == "minus" {
				k = -k
			}
			return &plLocalDate{t: plAddUnit(tt, string(plToChrono(args[1])), k)}, nil, true
		case "compareTo/1":
			o, _ := plTemporalTime(args[0])
			return int32(tt.Compare(o)), nil, true
		}
		if n == 1 && (strings.HasPrefix(name, "plus") || strings.HasPrefix(name, "minus")) {
			unit := strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(name, "plus"), "minus"))
			switch unit {
			case "DAYS", "WEEKS", "MONTHS", "YEARS":
				k := plI64(args[0])
				if strings.HasPrefix(name, "minus") {
					k = -k
				}
				return &plLocalDate{t: plAddUnit(tt, unit, k)}, nil, true
			}
		}
	case *plInstant:
		switch name + "/" + itoa(n) {
		case "toEpochMilli/0":
			return t.ms, nil, true
		case "getEpochSecond/0":
			return floorDiv(t.ms, 1000), nil, true
		case "getNano/0":
			return int32((t.ms-floorDiv(t.ms, 1000)*1000)*1000000 + t.nanos), nil, true
		case "plusMillis/1":
			return &plInstant{ms: t.ms + plI64(args[0]), nanos: t.nanos}, nil, true
		case "plusSeconds/1":
			return &plInstant{ms: t.ms + 1000*plI64(args[0]), nanos: t.nanos}, nil, true
		case "isBefore/1", "isAfter/1":
			o, _ := args[0].(*plInstant)
			if o == nil {
				return nil, plNPE(off), true
			}
			if name == "isBefore" {
				return t.ms < o.ms, nil, true
			}
			return t.ms > o.ms, nil, true
		case "atZone/1":
			tt := time.UnixMilli(t.ms).UTC().Add(time.Duration(t.nanos))
			return &plZDT{t: tt}, nil, true
		}
	case *plDuration:
		switch name + "/" + itoa(n) {
		case "toMinutes/0":
			return int64(t.d / time.Minute), nil, true
		case "toHours/0":
			return int64(t.d / time.Hour), nil, true
		case "toDays/0":
			return int64(t.d / (24 * time.Hour)), nil, true
		case "toMillis/0":
			return t.d.Milliseconds(), nil, true
		case "toNanos/0":
			return t.d.Nanoseconds(), nil, true
		case "getSeconds/0", "toSeconds/0":
			return int64(t.d / time.Second), nil, true
		case "isNegative/0":
			return t.d < 0, nil, true
		case "isZero/0":
			return t.d == 0, nil, true
		case "plus/1":
			if o, ok := args[0].(*plDuration); ok {
				return &plDuration{d: t.d + o.d}, nil, true
			}
		case "minus/1":
			if o, ok := args[0].(*plDuration); ok {
				return &plDuration{d: t.d - o.d}, nil, true
			}
		}
	case plDayOfWeek:
		switch name {
		case "getValue":
			return int32(t), nil, true
		case "name", "toString":
			return t.javaString(), nil, true
		case "plus":
			return plDayOfWeek((int(t)-1+int(plI64(args[0]))%7+7)%7 + 1), nil, true
		}
	case plStringEnum:
		switch name {
		case "name", "toString":
			return string(t), nil, true
		case "getValue":
			for i, m := range plMonthNames {
				if m == string(t) {
					return int32(i + 1), nil, true
				}
			}
		}
	case plChronoUnit:
		if name == "between" && n == 2 {
			a, aok := plTemporalTime(args[0])
			b, bok := plTemporalTime(args[1])
			if !aok || !bok {
				return nil, plNPE(off), true
			}
			u := string(t)
			switch u {
			case "MONTHS":
				return plMonthsBetween(a, b), nil, true
			case "YEARS":
				return plMonthsBetween(a, b) / 12, nil, true
			case "DECADES":
				return plMonthsBetween(a, b) / 120, nil, true
			case "CENTURIES":
				return plMonthsBetween(a, b) / 1200, nil, true
			}
			return int64(b.Sub(a) / plChronoUnits[u]), nil, true
		}
		if name == "name" || name == "toString" {
			if name == "name" {
				return string(t), nil, true
			}
			return t.javaString(), nil, true
		}
	case *plDTF:
		if name == "format" && n == 1 {
			tt, ok := plTemporalTime(args[0])
			if !ok {
				return nil, plNPE(off), true
			}
			if t.iso {
				if _, isDate := args[0].(*plLocalDate); isDate {
					return tt.Format("2006-01-02"), nil, true
				}
				return plFmtZDT(tt), nil, true
			}
			return plFormatPattern(t.pattern, tt), nil, true
		}
		if name == "withZone" {
			return t, nil, true
		}
	}
	return nil, nil, false
}

type plStringEnum string

func (s plStringEnum) javaString() string { return string(s) }

func plToChrono(v any) plChronoUnit {
	if u, ok := v.(plChronoUnit); ok {
		return u
	}
	return plChronoUnit(strings.ToUpper(plToString(v)))
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func plTimeFromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
