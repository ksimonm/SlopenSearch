// Package javafmt renders numbers the way Java's Double.toString and
// Float.toString do.
package javafmt

import (
	"math"
	"strconv"
	"strings"
)

// Double renders f like Java's Double.toString (JDK 19+: the shortest digits
// that round-trip; 1.0E7 / 1.0E-4 outside [1e-3, 1e7)), which is how
// Jackson writes a double.
func Double(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	return fmtShortest(shortestE(f, 64), math.Abs(f))
}

// shortestE returns the shortest round-tripping digits, with at least two
// significant digits (the closest two-digit decimal), like Double.toString.
func shortestE(f float64, bits int) string {
	e := strconv.FormatFloat(f, 'e', -1, bits)
	if !strings.Contains(e, ".") {
		if two := strconv.FormatFloat(f, 'e', 1, bits); !strings.Contains(two, ".0e") {
			e = two
		}
	}
	return e
}

// Float renders f like Java's Float.toString, which is how Jackson writes a
// float.
func Float(f float32) string {
	d := float64(f)
	switch {
	case math.IsNaN(d):
		return "NaN"
	case math.IsInf(d, 1):
		return "Infinity"
	case math.IsInf(d, -1):
		return "-Infinity"
	case d == 0:
		if math.Signbit(d) {
			return "-0.0"
		}
		return "0.0"
	}
	return fmtShortest(shortestE(d, 32), math.Abs(d))
}

// fmtShortest renders shortest digits ("d.ddde±x") the way
// Double.toString does.
func fmtShortest(e string, abs float64) string {
	neg := strings.HasPrefix(e, "-")
	if neg {
		e = e[1:]
	}
	mant, expS, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expS)
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	if abs >= 1e-3 && abs < 1e7 {
		if exp >= 0 {
			if len(digits) <= exp+1 {
				out = digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
			} else {
				out = digits[:exp+1] + "." + digits[exp+1:]
			}
		} else {
			out = "0." + strings.Repeat("0", -exp-1) + digits
		}
	} else {
		m := digits[:1] + "."
		if len(digits) > 1 {
			m += digits[1:]
		} else {
			m += "0"
		}
		out = m + "E" + strconv.Itoa(exp)
	}
	if neg {
		return "-" + out
	}
	return out
}
