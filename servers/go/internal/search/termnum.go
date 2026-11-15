package search

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
)

// javaDecimalFloat is the decimal syntax of Java's Double.parseDouble (after
// trimming): an optional sign, NaN, Infinity or digits with an optional
// fraction and exponent, and an optional float/double suffix.
var javaDecimalFloat = regexp.MustCompile(`^[+-]?(NaN|Infinity|(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?[fFdD]?)$`)

// javaParseDouble parses s like Double.parseDouble: leading and trailing
// characters up to ' ' are ignored. A failure is the NumberFormatException
// Java throws ("empty String" or `For input string: "<trimmed>"`).
func javaParseDouble(s string) (float64, *apierr.Error) {
	t := strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
	if t == "" {
		return 0, &apierr.Error{Type: "number_format_exception", Status: 400, Reason: "empty String"}
	}
	if javaDecimalFloat.MatchString(t) {
		if f, err := strconv.ParseFloat(strings.TrimRight(t, "fFdD"), 64); err == nil || strings.Contains(err.Error(), "range") {
			return f, nil
		}
	}
	return 0, &apierr.Error{Type: "number_format_exception", Status: 400, Reason: `For input string: "` + t + `"`}
}

func isQueryNumberType(t string) bool {
	switch t {
	case "integer", "long", "short", "byte", "unsigned_long", "float", "double", "half_float", "scaled_float":
		return true
	}
	return false
}

// bytesRefString renders a string the way Lucene's BytesRef.toString does
// ("[61 62]"): a term query value reaches the number field types as a BytesRef.
func bytesRefString(s string) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < len(s); i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%x", s[i])
	}
	sb.WriteByte(']')
	return sb.String()
}

// checkQueryNumber reproduces the checks NumberFieldMapper's types make on a
// string query value before converting it (objectToDouble, then the range of
// the type the value is parsed with), returning the error OpenSearch reports.
// Values it accepts are converted by the index-time parser.
func checkQueryNumber(ftype, s string) *apierr.Error {
	d, err := javaParseDouble(s)
	if err != nil {
		return err
	}
	var lo, hi float64
	var name string
	switch ftype {
	case "long":
		lo, hi, name = -9223372036854775808, 9223372036854775807, "a long"
	case "integer", "short", "byte":
		lo, hi, name = -2147483648, 2147483647, "an integer"
	case "unsigned_long":
		lo, hi, name = 0, 18446744073709551615, "an unsigned long"
	default:
		return nil
	}
	if d < lo || d > hi {
		return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "Value [" + bytesRefString(s) + "] is out of range for " + name}
	}
	return nil
}
