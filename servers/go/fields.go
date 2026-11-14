package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Index-time field value extraction.
//
// OpenSearch stores an indexed (normalized) form of each field value that is
// independent of the original _source. Term/range queries, sorting and
// aggregations run against these values, while _source is echoed verbatim.
// ---------------------------------------------------------------------------

type indexCtx struct {
	idx            *Index
	indexCoerce    bool
	indexIgnoreMal bool
	docID          string
	doc            map[string]any
	ignored        []string
	ignoredSet     map[string]bool
	values         map[string][]any
	totalFields    int
	fieldNameLimit int
	dyn            string
}

func newIndexCtx(idx *Index, docID string) *indexCtx {
	c := &indexCtx{
		idx:        idx,
		docID:      docID,
		ignoredSet: map[string]bool{},
		values:     map[string][]any{},
	}
	c.indexCoerce = true
	if v, ok := findSetting(idx.Settings, "mapping.coerce"); ok {
		c.indexCoerce = settingsValueString(v) != "false"
	}
	if v, ok := findSetting(idx.Settings, "mapping.ignore_malformed"); ok {
		c.indexIgnoreMal = settingsValueString(v) == "true"
	}
	c.totalFields = 1000
	if v, ok := settingInt(idx.Settings, "mapping.total_fields.limit"); ok {
		c.totalFields = v
	}
	c.fieldNameLimit = 1000
	if v, ok := settingInt(idx.Settings, "mapping.field_name_length.limit"); ok {
		c.fieldNameLimit = v
	}
	c.dyn = rootDynamicString(idx)
	if enabled, ok := idx.Mappings["enabled"].(bool); ok && !enabled {
		c.dyn = "false"
	}
	return c
}

func mapperErr(typ, reason string, causedType, causedReason string) *apiError {
	e := &apiError{Type: "mapper_parsing_exception", Reason: reason, Status: 400}
	if causedType != "" {
		e.CausedBy = &apiError{Type: causedType, Reason: causedReason}
	} else if causedReason != "" {
		e.CausedBy = &apiError{Type: "illegal_argument_exception", Reason: causedReason}
	}
	return e
}

// fieldValueError builds the standard "failed to parse field [f] of type [t] in
// document with id 'x'" wrapper.
func (c *indexCtx) fieldValueError(name, typ string, v any, causeType, causeReason string) *apiError {
	preview := previewValue(v)
	reason := fmt.Sprintf("failed to parse field [%s] of type [%s] in document with id '%s'. Preview of field's value: '%s'", name, typ, c.docID, preview)
	return mapperErr("mapper_parsing_exception", reason, causeType, causeReason)
}

func previewValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return formatNumber(t)
	case json.Number:
		return t.String()
	case map[string]any:
		return javaMapDebug(v)
	case []any:
		return "[...]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func ignoreAboveOf(def map[string]any) (int, bool) {
	v, ok := def["ignore_above"]
	if !ok {
		return 0, false
	}
	n, ok := toFloat(v)
	if !ok || n < 0 {
		return 0, false
	}
	return int(n), true
}

func ignoreAboveExceeded(def map[string]any, v any) bool {
	limit, ok := ignoreAboveOf(def)
	if !ok {
		return false
	}
	s, ok := valueScalarString(v)
	if !ok {
		return false
	}
	return len([]rune(s)) > limit
}

// numericKind classifies the numeric field types.
func isIntegerKind(t string) bool {
	switch t {
	case "byte", "short", "integer", "long", "unsigned_long":
		return true
	}
	return false
}

func intBounds(t string) (int64, uint64, bool) {
	switch t {
	case "byte":
		return math.MaxInt8, math.MaxUint8 + 1, true
	case "short":
		return math.MaxInt16, math.MaxUint16 + 1, true
	case "integer":
		return math.MaxInt32, math.MaxUint32 + 1, true
	case "long":
		return math.MaxInt64, 0, false
	}
	return 0, 0, false
}

func typeDisplayName(t string) string {
	switch t {
	case "byte":
		return "byte"
	case "short":
		return "short"
	case "integer":
		return "integer"
	case "long":
		return "long"
	case "unsigned_long":
		return "unsigned long"
	case "float":
		return "float"
	case "double":
		return "double"
	case "half_float":
		return "half_float"
	}
	return t
}

// parseIntValue converts a raw JSON number/string into an int64 value subject
// to OpenSearch coercion rules.
func (c *indexCtx) parseIntValue(name, typ string, def map[string]any, v any) (any, *apiError) {
	coerce := c.indexCoerce
	if b, ok := def["coerce"].(bool); ok {
		coerce = b
	}
	switch raw := v.(type) {
	case json.Number:
		if !coerce {
			f, err := raw.Float64()
			if err != nil {
				return nil, c.fieldValueError(name, typ, raw.String(), "number_format_exception",
					"For input string: \""+raw.String()+"\"")
			}
			return c.parseIntFloat(name, typ, def, f, false)
		}
		s := raw.String()
		return c.parseIntString(name, typ, def, s, coerce, false)
	case float64:
		return c.parseIntFloat(name, typ, def, raw, coerce)
	case int64:
		return c.checkIntRange(name, typ, raw, float64(raw), false)
	case int:
		return c.checkIntRange(name, typ, int64(raw), float64(raw), false)
	case bool:
		return nil, c.fieldValueError(name, typ, v, "input_coercion_exception",
			fmt.Sprintf("Cannot coerce value %v to %s", raw, typ))
	case string:
		return c.parseIntString(name, typ, def, raw, coerce, true)
	}
	return nil, c.fieldValueError(name, typ, v, "input_coercion_exception", "For input string: \""+previewValue(v)+"\"")
}

func (c *indexCtx) parseIntFloat(name, typ string, def map[string]any, f float64, coerce bool) (any, *apiError) {
	if !coerce {
		if f != math.Trunc(f) {
			return nil, c.fieldValueError(name, typ, formatNumber(f), "illegal_argument_exception",
				fmt.Sprintf("%s cannot be converted to Integer without data loss", formatNumber(f)))
		}
	}
	iv := int64(f)
	return c.checkIntRange(name, typ, iv, f, true)
}

func (c *indexCtx) checkIntRange(name, typ string, iv int64, orig float64, fromJSON bool) (any, *apiError) {
	maxI, _, bounded := intBounds(typ)
	switch {
	case typ == "byte":
		if iv > math.MaxInt8 || iv < math.MinInt8 {
			return nil, c.fieldValueError(name, typ, orig, "illegal_argument_exception",
				fmt.Sprintf("Value [%d] is out of range for a byte", iv))
		}
	case typ == "short":
		if iv > math.MaxInt16 || iv < math.MinInt16 {
			cause := "illegal_argument_exception"
			reason := fmt.Sprintf("Value [%d] is out of range for a short", iv)
			if fromJSON {
				cause = "input_coercion_exception"
				reason = fmt.Sprintf("Numeric value (%d) out of range of `short` (-32768 - 32767)", iv)
			}
			return nil, c.fieldValueError(name, typ, orig, cause, reason)
		}
	case typ == "integer":
		if iv > math.MaxInt32 || iv < math.MinInt32 {
			cause := "illegal_argument_exception"
			reason := fmt.Sprintf("Value [%d] is out of range for an integer", iv)
			if fromJSON {
				cause = "input_coercion_exception"
				reason = fmt.Sprintf("Numeric value (%d) out of range of `int` (-2147483648 - 2147483647)", iv)
			}
			return nil, c.fieldValueError(name, typ, orig, cause, reason)
		}
	}
	_ = maxI
	_ = bounded
	if typ == "unsigned_long" {
		if iv < 0 {
			return nil, c.fieldValueError(name, typ, orig, "illegal_argument_exception",
				fmt.Sprintf("Value [%d] is out of range for an unsigned long", iv))
		}
	}
	switch typ {
	case "byte", "short", "integer", "long":
		return iv, nil
	case "unsigned_long":
		return uint64(iv), nil
	}
	return iv, nil
}

func (c *indexCtx) parseIntString(name, typ string, def map[string]any, s string, coerce, trim bool) (any, *apiError) {
	coerceFalse := !coerce
	if b, ok := def["coerce"].(bool); ok {
		coerceFalse = !b
	}
	if coerceFalse {
		word := "Integer"
		if !isIntegerKind(typ) {
			word = "Float"
		}
		return nil, c.fieldValueError(name, typ, s, "illegal_argument_exception",
			fmt.Sprintf("%s value passed as String", word))
	}
	orig := s
	if trim && typ != "long" && typ != "unsigned_long" {
		s = strings.TrimSpace(s)
	}
	if typ == "unsigned_long" {
		u, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
				return nil, c.fieldValueError(name, typ, orig, "illegal_argument_exception",
					fmt.Sprintf("Value [%s] is out of range for an unsigned long", orig))
			}
			if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
				return c.checkIntRange(name, typ, int64(f), f, !trim)
			}
			return nil, c.fieldValueError(name, typ, orig, "illegal_argument_exception",
				fmt.Sprintf("For input string: \"%s\"", orig))
		}
		return u, nil
	}
	iv, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			causeType := "illegal_argument_exception"
			if !trim {
				causeType = "input_coercion_exception"
			}
			return nil, c.fieldValueError(name, typ, orig, causeType,
				fmt.Sprintf("Value [%s] is out of range for a %s", orig, typeDisplayName(typ)))
		}
		// integers accept decimal-like strings with coercion; floats parse natively
		if isIntegerKind(typ) {
			if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
				return c.checkIntRange(name, typ, int64(f), f, !trim)
			}
			causeType := "number_format_exception"
			if typ == "long" {
				causeType = "illegal_argument_exception"
			}
			return nil, c.fieldValueError(name, typ, orig, causeType,
				fmt.Sprintf("For input string: \"%s\"", orig))
		}
		return nil, c.fieldValueError(name, typ, orig, "number_format_exception",
			fmt.Sprintf("For input string: \"%s\"", orig))
	}
	return c.checkIntRange(name, typ, iv, float64(iv), !trim)
}

// parseFloatValue implements float/double/half_float/scaled_float coercion.
func (c *indexCtx) parseFloatValue(name, typ string, def map[string]any, v any) (any, *apiError) {
	coerceFalse := !c.indexCoerce
	if b, ok := def["coerce"].(bool); ok {
		coerceFalse = !b
	}
	var f float64
	switch raw := v.(type) {
	case json.Number:
		fv, err := raw.Float64()
		if err != nil {
			return nil, c.fieldValueError(name, typ, raw.String(), "number_format_exception",
				"For input string: \""+raw.String()+"\"")
		}
		f = fv
	case float64:
		f = raw
	case int64:
		f = float64(raw)
	case int:
		f = float64(raw)
	case bool:
		return nil, c.fieldValueError(name, typ, v, "input_coercion_exception", "Cannot coerce boolean")
	case string:
		if coerceFalse {
			return nil, c.fieldValueError(name, typ, raw, "illegal_argument_exception",
				"Float value passed as String")
		}
		lv := strings.ToLower(strings.TrimSpace(raw))
		if lv == "nan" || lv == "infinity" || lv == "-infinity" || lv == "inf" || lv == "-inf" {
			return nil, c.fieldValueError(name, typ, raw, "illegal_argument_exception",
				fmt.Sprintf("[%s] supports only finite values, but got [%s]", typ, raw))
		}
		if strings.Contains(raw, " ") {
			return nil, c.fieldValueError(name, typ, raw, "number_format_exception",
				fmt.Sprintf("For input string: \"%s\"", raw))
		}
		fv, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
				return nil, c.fieldValueError(name, typ, raw, "illegal_argument_exception",
					fmt.Sprintf("[%s] supports only finite values, but got [%s]", typ, raw))
			}
			return nil, c.fieldValueError(name, typ, raw, "number_format_exception",
				fmt.Sprintf("For input string: \"%s\"", raw))
		}
		f = fv
	default:
		return nil, c.fieldValueError(name, typ, v, "input_coercion_exception", "malformed")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, c.fieldValueError(name, typ, v, "illegal_argument_exception",
			fmt.Sprintf("[%s] supports only finite values, but got [%s]", typ, previewValue(v)))
	}
	switch typ {
	case "float":
		if f > math.MaxFloat32 || f < -math.MaxFloat32 {
			return nil, c.fieldValueError(name, typ, v, "input_coercion_exception",
				fmt.Sprintf("Value [%s] is out of range for a float", formatNumber(f)))
		}
		return float64(float32(f)), nil
	case "half_float":
		if f > 65504 || f < -65504 {
			return nil, c.fieldValueError(name, typ, v, "input_coercion_exception",
				fmt.Sprintf("Value [%s] is out of range for a half_float", formatNumber(f)))
		}
		return float64(float32FromHalf(float32Half(f))), nil
	case "scaled_float":
		scale := 1.0
		if s, ok := toFloat(def["scaling_factor"]); ok {
			scale = s
		}
		return math.Round(f*scale) / scale, nil
	default:
		return f, nil
	}
}

func float32Half(f float64) uint16 {
	// round-to-nearest-even conversion to IEEE 754 half precision
	bits := math.Float32bits(float32(f))
	sign := uint16(bits>>31) << 15
	exp := int32((bits>>23)&0xff) - 127
	frac := bits & 0x7fffff
	if exp > 15 {
		return sign | 0x7c00
	}
	if exp < -24 {
		return sign
	}
	if exp < -14 {
		shift := uint32(-exp - 14)
		m := frac | 0x800000
		half := uint16(m >> (13 + shift))
		rem := m & ((1 << (13 + shift)) - 1)
		halfRound := uint32(1) << (12 + shift)
		if rem > halfRound || (rem == halfRound && half&1 == 1) {
			half++
		}
		return sign | half
	}
	half := uint16(exp+15)<<10 | uint16(frac>>13)
	rem := frac & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++
	}
	return sign | half
}

func float32FromHalf(h uint16) float32 {
	sign := uint32(h>>15) << 31
	exp := uint32(h>>10) & 0x1f
	frac := uint32(h & 0x3ff)
	switch {
	case exp == 0:
		if frac == 0 {
			return math.Float32frombits(sign)
		}
		e := uint32(127 - 15 + 1)
		for frac&0x400 == 0 {
			frac <<= 1
			e--
		}
		frac &= 0x3ff
		return math.Float32frombits(sign | e<<23 | frac<<13)
	case exp == 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | frac<<13)
	default:
		return math.Float32frombits(sign | (exp+127-15)<<23 | frac<<13)
	}
}

// parseBooleanValue implements boolean coercion.
func (c *indexCtx) parseBooleanValue(name string, def map[string]any, v any) (any, *apiError) {
	switch raw := v.(type) {
	case bool:
		return raw, nil
	case string:
		if raw == "" {
			return false, nil
		}
		if raw == "true" || raw == "false" {
			return raw == "true", nil
		}
		return nil, c.fieldValueError(name, "boolean", v, "illegal_argument_exception",
			fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", raw))
	case json.Number:
		s := raw.String()
		if s == "" {
			return false, nil
		}
		return nil, c.fieldValueError(name, "boolean", v, "input_coercion_exception",
			"Current token (VALUE_NUMBER_INT) not of boolean type")
	default:
		return nil, c.fieldValueError(name, "boolean", v, "illegal_argument_exception",
			fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", previewValue(v)))
	}
}

// parseDateValue stores epoch millis (date) or epoch nanos (date_nanos).
func (c *indexCtx) parseDateValue(name, typ string, def map[string]any, v any) (any, *apiError) {
	s, isStr := v.(string)
	if isStr && s == "" {
		return nil, c.fieldValueError(name, typ, v, "illegal_argument_exception", "cannot parse empty date")
	}
	if !isStr {
		switch t := v.(type) {
		case json.Number:
			s = t.String()
		case float64:
			s = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			s = strconv.FormatBool(t)
		default:
			s = previewValue(v)
		}
	}
	if typ == "date_nanos" {
		if ns, rangeErr, ok := parseDateNanosAny(def, s); ok {
			return ns, nil
		} else if rangeErr != "" {
			return nil, c.fieldValueError(name, typ, v, "illegal_argument_exception", rangeErr)
		}
	} else if ms, ok := parseDateAny(def, s); ok {
		return ms, nil
	}
	reason := fmt.Sprintf("failed to parse date field [%s] with format [%s]", s, dateFormatOf(def))
	e := c.fieldValueError(name, typ, v, "illegal_argument_exception", reason)
	e.CausedBy.CausedBy = &apiError{Type: "date_time_parse_exception", Status: 400,
		Reason: "Failed to parse with all enclosed parsers"}
	return nil, e
}

func dateFormatOf(def map[string]any) string {
	if f, ok := def["format"].(string); ok {
		return f
	}
	return "strict_date_optional_time||epoch_millis"
}

// javaDateFormatToGo converts the common subset of Java date format letters.
func javaDateFormatToGo(f string) string {
	repl := []struct{ from, to string }{
		{"yyyy", "2006"}, {"yy", "06"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"}, {"mm", "04"}, {"ss", "05"},
		{"SSSSSSSSS", "000000000"}, {"SSS", "000"},
	}
	out := f
	if strings.Contains(out, "'T'") {
		out = strings.ReplaceAll(out, "'T'", "T")
	}
	for _, r := range repl {
		out = strings.ReplaceAll(out, r.from, r.to)
	}
	return out
}

func parseEpochNumber(s string, seconds bool) (int64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if seconds {
		return int64(f * 1000), true
	}
	return int64(f), true
}

func parseDateWithNamedFormat(name, s string) (int64, bool) {
	switch name {
	case "epoch_millis":
		return parseEpochNumber(s, false)
	case "epoch_second":
		return parseEpochNumber(s, true)
	case "date_optional_time", "strict_date_optional_time", "date_time",
		"strict_date_optional_time_nanos":
		return parseStrictDateOptionalTime(s)
	case "strict_date":
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "date":
		if t, err := time.Parse("2006-1-2", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "basic_date":
		if t, err := time.Parse("20060102", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	case "basic_date_time":
		if t, err := time.Parse("20060102T150405.000Z0700", s); err == nil {
			return t.UnixMilli(), true
		}
		if t, err := time.Parse("20060102T150405Z0700", s); err == nil {
			return t.UnixMilli(), true
		}
		return 0, false
	}
	if layout := javaDateFormatToGo(name); layout != "" {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), true
		}
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}

func parseDateAny(def map[string]any, s string) (int64, bool) {
	if f, ok := def["format"].(string); ok && f != "" {
		for _, fmtPart := range strings.Split(f, "||") {
			if ms, ok := parseDateWithNamedFormat(strings.TrimSpace(fmtPart), s); ok {
				return ms, true
			}
		}
		return 0, false
	}
	if ms, ok := parseStrictDateOptionalTime(s); ok {
		return ms, true
	}
	if ms, ok := parseEpochNumber(s, false); ok {
		return ms, true
	}
	return 0, false
}

func parseDateDefault(s string) (int64, bool) {
	return parseStrictDateOptionalTime(s)
}

func parseStrictDateOptionalTime(s string) (int64, bool) {
	sec, nsec, ok := parseStrictDateOptionalTimeParts(s)
	if !ok {
		return 0, false
	}
	return sec*1000 + int64(nsec)/int64(1e6), true
}

func parseStrictDateOptionalTimeParts(s string) (int64, int, bool) {
	i := 0
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	ystart := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == ystart {
		return 0, 0, false
	}
	ydigits := s[ystart:i]
	if !neg && len(ydigits) != 4 {
		return 0, 0, false
	}
	year, err := strconv.ParseInt(ydigits, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if neg {
		year = -year
	}
	month, day := 1, 1
	hour, minute, second, nsec := 0, 0, 0, 0
	loc := time.UTC
	if i < len(s) {
		if s[i] != '-' || i+3 > len(s) {
			return 0, 0, false
		}
		m, err := strconv.Atoi(s[i+1 : i+3])
		if err != nil {
			return 0, 0, false
		}
		month = m
		i += 3
		if i < len(s) && s[i] == '-' {
			if i+3 > len(s) {
				return 0, 0, false
			}
			d, err := strconv.Atoi(s[i+1 : i+3])
			if err != nil {
				return 0, 0, false
			}
			day = d
			i += 3
		}
	}
	if i < len(s) {
		if s[i] != 'T' || i+6 > len(s) {
			return 0, 0, false
		}
		h, err := strconv.Atoi(s[i+1 : i+3])
		if err != nil || s[i+3] != ':' {
			return 0, 0, false
		}
		mi, err := strconv.Atoi(s[i+4 : i+6])
		if err != nil {
			return 0, 0, false
		}
		hour, minute = h, mi
		i += 6
		if i < len(s) && s[i] == ':' {
			if i+3 > len(s) {
				return 0, 0, false
			}
			se, err := strconv.Atoi(s[i+1 : i+3])
			if err != nil {
				return 0, 0, false
			}
			second = se
			i += 3
			if i < len(s) && s[i] == '.' {
				i++
				fstart := i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				if i == fstart {
					return 0, 0, false
				}
				frac := s[fstart:i]
				for len(frac) < 9 {
					frac += "0"
				}
				nsec, _ = strconv.Atoi(frac[:9])
			}
		}
		if i < len(s) {
			switch s[i] {
			case 'Z':
				i++
			case '+', '-':
				sign := 1
				if s[i] == '-' {
					sign = -1
				}
				if i+6 > len(s) {
					return 0, 0, false
				}
				th, err1 := strconv.Atoi(s[i+1 : i+3])
				tm, err2 := strconv.Atoi(s[i+4 : i+6])
				if err1 != nil || err2 != nil || s[i+3] != ':' {
					return 0, 0, false
				}
				loc = time.FixedZone("", sign*(th*3600+tm*60))
				i += 6
			default:
				return 0, 0, false
			}
		}
	}
	if i != len(s) {
		return 0, 0, false
	}
	if month < 1 || month > 12 || day < 1 || day > 31 ||
		hour > 23 || minute > 59 || second > 59 {
		return 0, 0, false
	}
	t := time.Date(int(year), time.Month(month), day, hour, minute, second, nsec, loc)
	if int(t.Month()) != month || t.Day() != day {
		return 0, 0, false
	}
	return t.Unix(), t.Nanosecond(), true
}

func parseDateNanosAny(def map[string]any, s string) (int64, string, bool) {
	checkMs := func(ms int64) (int64, string, bool) {
		if ms < 0 {
			return 0, "date[" + s + "] is before the epoch in 1970 and cannot be stored in nanosecond resolution", false
		}
		if ms > math.MaxInt64/int64(1e6) {
			return 0, "date[" + s + "] is after 2262-04-11T23:47:16.854775807 and cannot be stored in nanosecond resolution", false
		}
		return ms * int64(1e6), "", true
	}
	if f, ok := def["format"].(string); ok && f != "" {
		for _, fmtPart := range strings.Split(f, "||") {
			fmtPart = strings.TrimSpace(fmtPart)
			switch fmtPart {
			case "epoch_millis":
				if ms, ok := parseEpochNumber(s, false); ok {
					return checkMs(ms)
				}
				continue
			case "epoch_second":
				if ms, ok := parseEpochNumber(s, true); ok {
					return checkMs(ms)
				}
				continue
			}
			if ms, ok := parseDateWithNamedFormat(fmtPart, s); ok {
				return checkMs(ms)
			}
		}
		return 0, "", false
	}
	if sec, nsec, ok := parseStrictDateOptionalTimeParts(s); ok {
		if sec < 0 {
			return 0, "date[" + s + "] is before the epoch in 1970 and cannot be stored in nanosecond resolution", false
		}
		maxSec := math.MaxInt64 / int64(1e9)
		maxNsec := math.MaxInt64 % int64(1e9)
		if sec > maxSec || (sec == maxSec && int64(nsec) > maxNsec) {
			return 0, "date[" + s + "] is after 2262-04-11T23:47:16.854775807 and cannot be stored in nanosecond resolution", false
		}
		return sec*int64(1e9) + int64(nsec), "", true
	}
	if ms, ok := parseEpochNumber(s, false); ok {
		return checkMs(ms)
	}
	return 0, "", false
}

// parseIPValue canonicalizes an IPv4/IPv6 string or CIDR.
func (c *indexCtx) parseIPValue(name string, v any) (any, *apiError) {
	s, ok := toString(v)
	if !ok {
		return nil, c.fieldValueError(name, "ip", v, "illegal_argument_exception", "failed to parse ip")
	}
	if strings.Contains(s, "/") {
		return nil, c.fieldValueError(name, "ip", s, "illegal_argument_exception",
			fmt.Sprintf("'%s' is not an IP string literal.", s))
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, c.fieldValueError(name, "ip", s, "illegal_argument_exception",
			fmt.Sprintf("'%s' is not an IP string literal.", s))
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), nil
	}
	return ip.String(), nil
}

// parseIPRangeString expands a CIDR (e.g. 192.168.0.0/24) into the network
// and broadcast addresses, canonicalized for storage.
func (c *indexCtx) parseIPRangeString(name, s string) (string, string, *apiError) {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		ones := 0
		if i := strings.Index(s, "/"); i >= 0 {
			ones, _ = strconv.Atoi(s[i+1:])
		}
		return "", "", c.fieldValueError(name, "ip_range", s, "illegal_argument_exception",
			fmt.Sprintf("Illegal prefix length [%d] in [%s]. Must be 0-32 for IPv4 ranges, 0-128 for IPv6 ranges", ones, s))
	}
	lower := ipnet.IP
	upper := make(net.IP, len(lower))
	copy(upper, lower)
	for i := range upper {
		upper[i] |= ^ipnet.Mask[i]
	}
	canon := func(ip net.IP) string {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}
	return canon(lower), canon(upper), nil
}

// indexRawField handles a single declared field definition with raw values.
func (c *indexCtx) indexRawField(path string, def map[string]any, raw any) *apiError {
	typ, _ := def["type"].(string)
	if typ == "" {
		if _, hasProps := def["properties"].(map[string]any); hasProps {
			typ = "object"
		} else {
			typ = "object"
		}
	}
	if typ == "object" || typ == "nested" {
		return c.indexObject(path, def, raw)
	}
	if typ == "alias" {
		if raw == nil {
			return nil
		}
		return mapperErr("mapper_parsing_exception", "failed to parse", "illegal_argument_exception",
			"Cannot write to a field alias ["+path+"].")
	}
	if typ == "flat_object" {
		m, ok := raw.(map[string]any)
		if !ok {
			return c.fieldValueError(path, typ, raw, "illegal_argument_exception", "malformed")
		}
		var walk func(prefix string, v any)
		walk = func(prefix string, v any) {
			switch t := v.(type) {
			case map[string]any:
				for k, child := range t {
					p := k
					if prefix != "" {
						p = prefix + "." + k
					}
					walk(p, child)
				}
			case []any:
				for _, child := range t {
					walk(prefix, child)
				}
			case nil:
			default:
				if s, ok := valueScalarString(v); ok {
					c.values[path+"."+prefix] = append(c.values[path+"."+prefix], s)
					c.values[path] = append(c.values[path], s)
				}
			}
		}
		walk("", m)
		return nil
	}
	if typ == "rank_features" {
		m, ok := raw.(map[string]any)
		if !ok {
			return mapperErr("mapper_parsing_exception", "failed to parse", "illegal_argument_exception",
				"[rank_features] fields must be json objects, expected a START_OBJECT but got: "+jsonValueKind(raw))
		}
		for k, fv := range m {
			f, ok := toFloat(fv)
			if !ok {
				if s, ok := valueScalarString(fv); ok {
					if fv2, err := strconv.ParseFloat(s, 64); err == nil {
						f = fv2
						ok = true
					}
				}
			}
			if !ok || f <= 0 || float64(float32(f)) < 1.1754944e-38 {
				shown := f
				if !ok {
					shown = 0
				} else {
					shown = float64(float32(f))
				}
				return mapperErr("mapper_parsing_exception", "failed to parse", "illegal_argument_exception",
					fmt.Sprintf("featureValue must be a positive normal float, got: %.1f for feature %s on field %s which is less than the minimum positive normal float: 1.1754944E-38", shown, k, path))
			}
			sub := path + "." + k
			c.values[sub] = append(c.values[sub], f)
		}
		return nil
	}
	var values []any
	if typ == "rank_feature" {
		if arr, ok := raw.([]any); ok {
			if len(arr) > 1 {
				return c.fieldValueError(path, "rank_feature", arr[len(arr)-1], "illegal_argument_exception",
					"[rank_feature] fields do not support indexing multiple values for the same field ["+path+"] in the same document")
			}
			values = arr
		}
	}
	if typ == "geo_point" {
		if arr, ok := raw.([]any); ok && len(arr) >= 2 {
			if _, ok1 := toFloat(arr[0]); ok1 {
				if _, ok2 := toFloat(arr[1]); ok2 {
					values = []any{raw}
				}
			}
		}
	}
	if values == nil {
		var err *apiError
		values, err = c.flattenScalars(path, raw)
		if err != nil {
			return err
		}
	}
	out := []any{}
	ignoreMalformed := c.indexIgnoreMal
	if b, ok := def["ignore_malformed"].(bool); ok {
		ignoreMalformed = b
	}
	nullValue, hasNull := def["null_value"]
	for _, v := range values {
		if v == nil {
			if hasNull {
				nv, err := c.coerce(path, def, typ, nullValue)
				if err != nil {
					return err
				}
				if nv != nil {
					out = append(out, nv)
				}
			}
			continue
		}
		if typ == "object" || typ == "nested" {
			continue
		}
		nv, err := c.coerce(path, def, typ, v)
		if err != nil {
			if ignoreMalformed {
				if _, isObj := v.(map[string]any); !isObj {
					c.addIgnored(path)
				}
				continue
			}
			return err
		}
		if nv != nil {
			if ignoreAboveExceeded(def, nv) {
				continue
			}
			out = append(out, nv)
		}
	}
	if len(out) > 0 || hasNull {
		c.values[path] = append(c.values[path], out...)
	}
	if typ == "text" {
		var prefixes []any
		if ip, ok := def["index_prefixes"].(map[string]any); ok {
			minChars := 2
			if n, ok := toFloat(ip["min_chars"]); ok {
				minChars = int(n)
			}
			maxChars := 5
			if n, ok := toFloat(ip["max_chars"]); ok {
				maxChars = int(n)
			}
			for _, v := range out {
				sv, ok := valueScalarString(v)
				if !ok {
					continue
				}
				for _, tok := range analyze(sv) {
					chars := []rune(tok)
					limit := maxChars
					if limit > len(chars) {
						limit = len(chars)
					}
					for n := minChars; n <= limit; n++ {
						prefixes = append(prefixes, string(chars[:n]))
					}
				}
			}
		}
		if len(prefixes) > 0 {
			c.values[path+"._index_prefix"] = append(c.values[path+"._index_prefix"], prefixes...)
		}
		if b, ok := def["index_phrases"].(bool); ok && b {
			var phrases []any
			for _, v := range out {
				sv, ok := valueScalarString(v)
				if !ok {
					continue
				}
				toks := analyze(sv)
				for i := 0; i+1 < len(toks); i++ {
					phrases = append(phrases, toks[i]+" "+toks[i+1])
				}
			}
			if len(phrases) > 0 {
				c.values[path+"._index_phrase"] = append(c.values[path+"._index_phrase"], phrases...)
			}
		}
	}
	if typ == "search_as_you_type" {
		maxShingle := 3
		if n, ok := toFloat(def["max_shingle_size"]); ok {
			maxShingle = int(n)
		}
		if maxShingle < 2 {
			maxShingle = 2
		}
		if maxShingle > 4 {
			maxShingle = 4
		}
		grams := map[int][]any{}
		var prefixes []any
		for _, v := range out {
			sv, ok := valueScalarString(v)
			if !ok {
				continue
			}
			toks := analyze(sv)
			for n := 2; n <= maxShingle; n++ {
				for i := 0; i+n <= len(toks); i++ {
					grams[n] = append(grams[n], strings.Join(toks[i:i+n], " "))
				}
			}
			for i := 0; i < len(toks); i++ {
				joined := strings.Join(toks[i:], " ")
				chars := []rune(joined)
				for end := 1; end <= len(chars); end++ {
					prefixes = append(prefixes, string(chars[:end]))
				}
			}
		}
		for n, list := range grams {
			if len(list) > 0 {
				key := fmt.Sprintf("%s._%dgram", path, n)
				c.values[key] = append(c.values[key], list...)
			}
		}
		if len(prefixes) > 0 {
			c.values[path+"._index_prefix"] = append(c.values[path+"._index_prefix"], prefixes...)
		}
	}
	// multi-fields
	if fields, ok := def["fields"].(map[string]any); ok {
		for fname, fraw := range fields {
			fdef, ok := fraw.(map[string]any)
			if !ok {
				continue
			}
			sub := path + "." + fname
			subVals := []any{}
			for _, v := range values {
				if v == nil {
					continue
				}
				nv, err := c.coerce(sub, fdef, strOf(fdef["type"]), v)
				if err != nil {
					return err
				}
				if nv != nil {
					if ignoreAboveExceeded(fdef, nv) {
						continue
					}
					subVals = append(subVals, nv)
				}
			}
			if len(subVals) > 0 {
				c.values[sub] = append(c.values[sub], subVals...)
			}
		}
	}
	// copy_to
	if ct, ok := def["copy_to"].(string); ok && ct != "" {
		if err := c.copyToField(ct, values); err != nil {
			return err
		}
	} else if cts, ok := def["copy_to"].([]any); ok {
		for _, e := range cts {
			if s, ok := e.(string); ok {
				if err := c.copyToField(s, values); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *indexCtx) copyToField(target string, vals []any) *apiError {
	tdef, mapped := mappingFieldDef(c.idx, target)
	ttype := ""
	if mapped {
		ttype, _ = tdef["type"].(string)
	}
	for _, v := range vals {
		if v == nil {
			continue
		}
		if mapped && ttype != "" {
			nv, err := c.coerce(target, tdef, ttype, v)
			if err != nil {
				return err
			}
			if nv == nil {
				continue
			}
			c.values[target] = append(c.values[target], nv)
			continue
		}
		c.values[target] = append(c.values[target], v)
	}
	return nil
}

func (c *indexCtx) addIgnored(path string) {
	if !c.ignoredSet[path] {
		c.ignoredSet[path] = true
		c.ignored = append(c.ignored, path)
	}
}

func (c *indexCtx) coerce(path string, def map[string]any, typ string, v any) (any, *apiError) {
	switch typ {
	case "text", "match_only_text", "search_as_you_type", "wildcard":
		if _, isMap := v.(map[string]any); isMap {
			return nil, c.fieldValueError(path, typ, v, "illegal_state_exception", "Can't get text on a START_OBJECT at 1:6")
		}
		s, ok := valueScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		if typ == "wildcard" {
			if norm, ok := def["normalizer"].(string); ok && norm != "" {
				s = applyNormalizerWithIndex(c.idx, norm, s)
			}
			return s, nil
		}
		return s, nil
	case "keyword", "constant_keyword":
		if _, isMap := v.(map[string]any); isMap {
			return nil, c.fieldValueError(path, typ, v, "illegal_state_exception", "Can't get text on a START_OBJECT at 1:6")
		}
		s, ok := valueScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		if typ == "constant_keyword" {
			if cv, has := def["value"]; has {
				cs, _ := valueScalarString(cv)
				if s != cs {
					return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception",
						"constant keyword field ["+path+"] must have a value of ["+cs+"]")
				}
			}
		}
		if norm, ok := def["normalizer"].(string); ok && norm != "" {
			s = applyNormalizerWithIndex(c.idx, norm, s)
		}
		return s, nil
	case "integer", "long", "short", "byte", "unsigned_long":
		return c.parseIntValue(path, typ, def, v)
	case "float", "double", "half_float", "scaled_float":
		return c.parseFloatValue(path, typ, def, v)
	case "boolean":
		return c.parseBooleanValue(path, def, v)
	case "date", "date_nanos":
		return c.parseDateValue(path, typ, def, v)
	case "ip":
		return c.parseIPValue(path, v)
	case "binary":
		s, ok := valueScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		dv, _ := def["doc_values"].(bool)
		store, _ := def["store"].(bool)
		if store || dv {
			if _, err := base64.StdEncoding.DecodeString(s); err != nil {
				reason := "Unexpected end of base64-encoded String: base64 variant 'MIME-NO-LINEFEEDS' expects padding (one or more '=' characters) at the end. This Base64Variant might have been incorrectly configured"
				return nil, mapperErr("mapper_parsing_exception",
					fmt.Sprintf("failed to parse field [%s] of type [binary] in document with id '%s'. Preview of field's value: '%s'", path, c.docID, previewValue(v)),
					"json_parse_exception", reason)
			}
		}
		if !dv {
			return nil, nil
		}
		return s, nil
	case "token_count":
		return c.parseTokenCount(path, def, v)
	case "flat_object":
		s, ok := valueScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		return s, nil
	case "integer_range", "float_range", "long_range", "double_range", "date_range", "ip_range":
		return c.parseRangeValue(path, typ, def, v)
	case "geo_point":
		return c.parseGeoPoint(path, def, v)
	case "geo_shape":
		return c.parseGeoShape(path, v)
	case "completion":
		return c.parseCompletion(path, def, v)
	case "rank_feature":
		return c.parseRankFeature(path, v)
	case "rank_features":
		return c.parseRankFeature(path, v)
	case "constant_keyword_default":
		return nil, nil
	case "alias":
		return nil, nil
	case "join":
		return c.parseJoin(path, def, v)
	case "percolator":
		return v, nil
	case "derived":
		return nil, nil
	}
	if isNumericType(typ) {
		return c.parseFloatValue(path, typ, def, v)
	}
	if s, ok := valueScalarString(v); ok {
		return s, nil
	}
	return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}

func valueScalarString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return formatNumber(t), true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}

func base64DecodeLenient(s string) ([]byte, error) {
	return nil, nil
}

// flattenScalars flattens arrays into individual scalar values. An empty array
// yields no values; nested arrays are flattened recursively.
func (c *indexCtx) flattenScalars(path string, raw any) ([]any, *apiError) {
	switch t := raw.(type) {
	case []any:
		out := []any{}
		for _, e := range t {
			sub, err := c.flattenScalars(path, e)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		return out, nil
	case map[string]any:
		return []any{t}, nil
	default:
		return []any{raw}, nil
	}
}

func (c *indexCtx) indexObject(path string, def map[string]any, raw any) *apiError {
	props, _ := def["properties"].(map[string]any)
	attachProps := false
	if props == nil {
		props = map[string]any{}
		attachProps = true
	}
	if d, ok := def["dynamic"]; ok {
		saved := c.dyn
		c.dyn = dynamicString(d)
		defer func() { c.dyn = saved }()
	}
	enabled := true
	if b, ok := def["enabled"].(bool); ok {
		enabled = b
	}
	if !enabled {
		return nil
	}
	if raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case []any:
		for _, e := range t {
			if err := c.indexObject(path, def, e); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		if err := c.indexObjectMap(path, props, t); err != nil {
			return err
		}
		if attachProps && len(props) > 0 {
			def["properties"] = props
		}
		return nil
	default:
		return nil
	}
}

func (c *indexCtx) indexObjectMap(path string, props map[string]any, src map[string]any) *apiError {
	for rawK, v := range src {
		k := strings.TrimRight(rawK, ".")
		if k == "" {
			k = rawK
		}
		if isDocumentMetadataField(k) {
			if err := c.rejectMetadataField(k); err != nil {
				return err
			}
			continue
		}
		childPath := k
		if path != "" {
			childPath = path + "." + k
		}
		def, declared := props[k].(map[string]any)
		if !declared {
			nd, matched, derr := dynamicDefFor(c.idx, childPath, k, v)
			if derr != nil {
				return derr
			}
			allow := true
			switch c.dyn {
			case "false", "strict":
				allow = false
			case "strict_allow_templates", "false_allow_templates":
				allow = matched
			}
			if !allow {
				if c.dyn == "false" || c.dyn == "false_allow_templates" {
					continue
				}
				name := "_doc"
				if path != "" {
					name = path
				}
				return &apiError{Type: "strict_dynamic_mapping_exception", Status: 400,
					Reason: fmt.Sprintf("mapping set to %s, dynamic introduction of [%s] within [%s] is not allowed", c.dyn, k, name)}
			}
			if nd == nil {
				continue
			}
			def = nd
			props[k] = def
			c.bumpTotalFields()
		}
		if err := c.indexRawField(childPath, def, v); err != nil {
			return err
		}
		if cts, ok := def["copy_to"]; ok {
			var targets []string
			switch t := cts.(type) {
			case string:
				targets = []string{t}
			case []any:
				for _, e := range t {
					if s, ok := e.(string); ok {
						targets = append(targets, s)
					}
				}
			}
			for _, target := range targets {
				if _, exists := props[target]; exists {
					continue
				}
				if c.dyn == "strict" || c.dyn == "strict_allow_templates" {
					return &apiError{Type: "strict_dynamic_mapping_exception", Status: 400,
						Reason: "mapping set to strict, dynamic introduction of [" + target + "] within [_doc] is not allowed"}
				}
				props[target] = map[string]any{"type": "text"}
				c.bumpTotalFields()
			}
		}
	}
	return nil
}

func isDocumentMetadataField(name string) bool {
	switch name {
	case "_id", "_index", "_source", "_routing", "_field_names", "_ignored":
		return true
	}
	return false
}

func (c *indexCtx) rejectMetadataField(name string) *apiError {
	return mapperErr("mapper_parsing_exception",
		fmt.Sprintf("Field [%s] is a metadata field and cannot be added inside a document. Use the index API request parameters.", name),
		"mapper_parsing_exception",
		fmt.Sprintf("Field [%s] is a metadata field and cannot be added inside a document. Use the index API request parameters.", name))
}

func (c *indexCtx) bumpTotalFields() {}

// constantKeywordDefault fills the fixed value for docs missing the field.
func (c *indexCtx) fillConstants(props map[string]any, src map[string]any) {
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "constant_keyword" {
			if _, present := src[name]; !present {
				if v, ok := def["value"]; ok {
					c.values[name] = []any{fmt.Sprint(v)}
				}
			}
		}
	}
}

// buildDocValues computes indexed values for a document.
func (c *indexCtx) buildDocValues() *apiError {
	props, _ := c.idx.Mappings["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		c.idx.Mappings["properties"] = props
	}
	if err := c.indexObjectMap("", props, c.doc); err != nil {
		return err
	}
	c.fillConstants(props, c.doc)
	if len(c.ignored) > 0 {
		sort.Strings(c.ignored)
		c.values["_ignored"] = []any{}
		for _, f := range c.ignored {
			c.values["_ignored"] = append(c.values["_ignored"], f)
		}
	}
	return nil
}

func invalidDocFieldName(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if k == "" {
				return "field name cannot be an empty string"
			}
			if k == "." {
				return "field name cannot contain only the character [.]"
			}
			if strings.HasPrefix(k, ".") || strings.Contains(k, "..") {
				return "object field starting or ending with a [.] makes object resolution ambiguous: [" + k + "]"
			}
			if r := invalidDocFieldName(child); r != "" {
				return r
			}
		}
	case []any:
		for _, e := range t {
			if r := invalidDocFieldName(e); r != "" {
				return r
			}
		}
	}
	return ""
}

func quotedKey(src, marker string) (string, bool) {
	i := strings.Index(src, marker)
	if i < 0 {
		return "", false
	}
	rest := strings.TrimLeft(src[i+len(marker):], " \t\r\n")
	if rest == "" {
		return "", false
	}
	q := rest[0]
	if q != '\'' && q != '"' {
		return "", false
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return "", false
	}
	return rest[1 : 1+end], true
}

func evalDerived(def map[string]any, doc map[string]any) (any, bool) {
	script, _ := def["script"].(map[string]any)
	src := ""
	if script != nil {
		src, _ = script["source"].(string)
	} else if s, ok := def["script"].(string); ok {
		src = s
	}
	if src == "" {
		return nil, false
	}
	body := src
	if i := strings.Index(src, "emit("); i >= 0 {
		body = src[i+len("emit("):]
	}
	if strings.Contains(body, "toUpperCase") {
		key, ok := quotedKey(body, "params._source[")
		if !ok {
			return nil, false
		}
		v, ok := fieldValue(doc, key)
		if !ok {
			return nil, false
		}
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		return strings.ToUpper(s), true
	}
	key, ok := quotedKey(body, "doc[")
	if !ok {
		key, ok = quotedKey(body, "params._source[")
	}
	if !ok {
		return nil, false
	}
	base, ok := fieldValue(doc, key)
	if !ok {
		return nil, false
	}
	if strings.Contains(body, ".value") {
		rest := ""
		if i := strings.Index(body, ".value"); i >= 0 {
			rest = body[i+len(".value"):]
		}
		rest = strings.TrimRight(strings.TrimSpace(rest), ")")
		rest = strings.TrimSpace(rest)
		if rest != "" {
			op := rest[0]
			if op == '*' || op == '+' || op == '-' || op == '/' {
				rhs, ok := strconv.ParseFloat(strings.TrimSpace(rest[1:]), 64)
				if ok == nil {
					lhs, lok := toFloat(base)
					if !lok {
						if s, sok := base.(string); sok {
							if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
								lhs, lok = f, true
							}
						}
					}
					if lok {
						var out float64
						switch op {
						case '*':
							out = lhs * rhs
						case '+':
							out = lhs + rhs
						case '-':
							out = lhs - rhs
						case '/':
							out = lhs / rhs
						}
						if out == math.Trunc(out) {
							return int64(out), true
						}
						return out, true
					}
				}
			}
		}
	}
	return base, true
}

func addDerivedValues(idx *Index, c *indexCtx) {
	defs, _ := idx.Mappings["derived"].(map[string]any)
	for name, raw := range defs {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := evalDerived(def, c.doc); ok {
			c.values[name] = []any{v}
		}
	}
}

// normalizeDerivedMappings gives derived scripts their default lang.
func normalizeDerivedMappings(mappings map[string]any) {
	derived, ok := mappings["derived"].(map[string]any)
	if !ok {
		return
	}
	for _, raw := range derived {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch s := def["script"].(type) {
		case string:
			def["script"] = map[string]any{"source": s, "lang": "painless"}
		case map[string]any:
			if _, has := s["lang"]; !has {
				s["lang"] = "painless"
			}
		}
	}
}

// applyRequestDerived installs request-level derived field definitions.
func applyRequestDerived(idx *Index, defs any) *apiError {
	src, ok := defs.(map[string]any)
	if !ok {
		return &apiError{Type: "parsing_exception", Status: 400, Reason: "expected derived to be an object"}
	}
	norm := map[string]any{}
	for name, raw := range src {
		def, _ := raw.(map[string]any)
		if def == nil {
			def = map[string]any{}
		}
		cp := map[string]any{}
		for k, v := range def {
			cp[k] = v
		}
		if s, ok := cp["script"].(string); ok {
			cp["script"] = map[string]any{"source": s, "lang": "painless"}
		}
		norm[name] = cp
	}
	idx.Mappings["derived"] = norm
	idx.recomputeAllDocValues()
	ensureFieldTypes(idx)
	return nil
}

// computeDocValues validates and indexes a document, storing normalized field
// values used by search, sort, aggregations and docvalue_fields.
func (idx *Index) computeDocValues(id string, doc map[string]any) *apiError {
	if reason := invalidDocFieldName(doc); reason != "" {
		return mapperErr("mapper_parsing_exception", reason, "illegal_argument_exception", reason)
	}
	if derr := checkObjectDepth(doc, 0, mappingIntSetting(idx, "mapping.depth.limit", 20)); derr != nil {
		return derr
	}
	if derr := checkDocFieldNameLengths(doc, mappingIntSetting(idx, "mapping.field_name_length.limit", 1000)); derr != nil {
		return derr
	}
	if props, ok := idx.Mappings["properties"].(map[string]any); ok {
		limit := mappingIntSetting(idx, "mapping.nested_objects.limit", 10000)
		if countNestedDocuments(props, doc) > limit {
			return nestedObjectsLimitError(limit)
		}
	}
	c := newIndexCtx(idx, id)
	c.doc = doc
	if err := c.buildDocValues(); err != nil {
		return err
	}
	addDerivedValues(idx, c)
	if idx.DocValues == nil {
		idx.DocValues = map[string]map[string][]any{}
	}
	idx.DocValues[id] = c.values
	idx.dvGen++
	if len(c.ignored) > 0 {
		if idx.DocIgnored == nil {
			idx.DocIgnored = map[string][]string{}
		}
		idx.DocIgnored[id] = c.ignored
		doc["_ignored"] = ignoredValues(c.ignored)
	} else if idx.DocIgnored != nil {
		delete(idx.DocIgnored, id)
		delete(doc, "_ignored")
	}
	return nil
}

func ignoredValues(fields []string) []any {
	out := make([]any, 0, len(fields))
	for _, f := range fields {
		out = append(out, f)
	}
	return out
}

func (idx *Index) removeDocValues(id string) {
	delete(idx.DocValues, id)
	delete(idx.DocIgnored, id)
	idx.dvGen++
}

// recomputeAllDocValues rebuilds doc values for every document (used when
// mappings change).
func (idx *Index) recomputeAllDocValues() {
	idx.dvGen++
	idx.DocValues = map[string]map[string][]any{}
	idx.DocIgnored = map[string][]string{}
	for _, id := range idx.Order {
		doc, live := idx.Docs[id]
		if !live {
			continue
		}
		_ = idx.computeDocValues(id, doc)
	}
}

func applyNormalizer(name, s string) string {
	switch name {
	case "lowercase":
		return strings.ToLower(s)
	}
	return s
}

func asciiLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func applyNormalizerWithIndex(idx *Index, name, s string) string {
	if idx != nil {
		if rawFilters, ok := findSetting(idx.Settings, "analysis.normalizer."+name+".filter"); ok {
			for _, f := range filterNames(rawFilters) {
				switch f {
				case "lowercase":
					s = strings.ToLower(s)
				case "uppercase":
					s = strings.ToUpper(s)
				case "asciifolding":
					s = asciiFoldReplacer.Replace(s)
				}
			}
			return s
		}
	}
	return applyNormalizer(name, s)
}

func normalizeQueryValue(idx *Index, field, s string) string {
	def, ok := mappingFieldDef(idx, field)
	if !ok {
		return s
	}
	norm, _ := def["normalizer"].(string)
	if norm == "" {
		return s
	}
	return applyNormalizerWithIndex(idx, norm, s)
}

func (c *indexCtx) parseTokenCount(path string, def map[string]any, v any) (any, *apiError) {
	s, ok := valueScalarString(v)
	if !ok {
		return nil, c.fieldValueError(path, "token_count", v, "illegal_argument_exception", "malformed")
	}
	analyzer, _ := def["analyzer"].(string)
	tokens := analyzeTokensWithIndex(c.idx, analyzer, s)
	epi := true
	if b, ok := def["enable_position_increments"].(bool); ok {
		epi = b
	}
	if epi {
		if len(tokens) == 0 {
			return int64(0), nil
		}
		return int64(tokens[len(tokens)-1].pos + 1), nil
	}
	return int64(len(tokens)), nil
}

const geohashBase32 = "0123456789bcdefghjkmnpqrstuvwxyz"

func decodeGeohash(s string) (float64, float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, 0, false
	}
	latMin, latMax := -90.0, 90.0
	lonMin, lonMax := -180.0, 180.0
	even := true
	for _, c := range s {
		idx := strings.IndexRune(geohashBase32, c)
		if idx < 0 {
			return 0, 0, false
		}
		for bit := 4; bit >= 0; bit-- {
			b := (idx >> uint(bit)) & 1
			if even {
				mid := (lonMin + lonMax) / 2
				if b == 1 {
					lonMin = mid
				} else {
					lonMax = mid
				}
			} else {
				mid := (latMin + latMax) / 2
				if b == 1 {
					latMin = mid
				} else {
					latMax = mid
				}
			}
			even = !even
		}
	}
	return (latMin + latMax) / 2, (lonMin + lonMax) / 2, true
}

func (c *indexCtx) parseGeoPoint(path string, def map[string]any, v any) (any, *apiError) {
	ignoreZ := true
	if b, ok := def["ignore_z_value"].(bool); ok {
		ignoreZ = b
	}
	ignoreMal := c.indexIgnoreMal
	if b, ok := def["ignore_malformed"].(bool); ok {
		ignoreMal = b
	}
	errf := func(cause, reason string) *apiError {
		return mapperErr("mapper_parsing_exception", "failed to parse field ["+path+"] of type [geo_point]", cause, reason)
	}
	normalize := func(lat, lon float64) (float64, float64) {
		if !ignoreMal {
			return lat, lon
		}
		la, lo := lat, lon
		for la > 90 {
			la = 180 - la
			lo += 180
		}
		for la < -90 {
			la = -180 - la
			lo += 180
		}
		lo = math.Mod(lo, 360)
		if lo > 180 {
			lo -= 360
		} else if lo < -180 {
			lo += 360
		}
		return la, lo
	}
	check := func(lat, lon float64) (any, *apiError) {
		lat, lon = normalize(lat, lon)
		if !ignoreMal && (lat > 90 || lat < -90) {
			return nil, errf("illegal_argument_exception", fmt.Sprintf("illegal latitude value [%.1f] for %s", lat, path))
		}
		if !ignoreMal && (lon > 180 || lon < -180) {
			return nil, errf("illegal_argument_exception", fmt.Sprintf("illegal longitude value [%.1f] for %s", lon, path))
		}
		return map[string]any{"lat": lat, "lon": lon}, nil
	}
	switch t := v.(type) {
	case string:
		trimmed := strings.TrimSpace(t)
		up := strings.ToUpper(trimmed)
		if strings.HasPrefix(up, "POINT") {
			body := trimmed
			if i := strings.Index(body, "("); i >= 0 {
				body = body[i+1:]
			}
			if j := strings.LastIndex(body, ")"); j >= 0 {
				body = body[:j]
			}
			fs := strings.Fields(body)
			nums := []float64{}
			for _, f := range fs {
				if n, err := strconv.ParseFloat(f, 64); err == nil {
					nums = append(nums, n)
				}
			}
			if len(nums) >= 3 && !ignoreZ {
				return nil, errf("parse_exception", "Invalid WKT format")
			}
			if len(nums) >= 2 {
				return check(nums[1], nums[0])
			}
			return nil, errf("parse_exception", "Invalid WKT format")
		}
		parts := strings.Split(trimmed, ",")
		if len(parts) >= 2 {
			nums := []float64{}
			allNums := true
			for _, p := range parts {
				n, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					allNums = false
					break
				}
				nums = append(nums, n)
			}
			if allNums && len(nums) >= 2 {
				if len(nums) >= 3 && !ignoreZ {
					return nil, errf("parse_exception", fmt.Sprintf("Exception parsing coordinates: found Z value [%.1f] but [ignore_z_value] parameter is [false]", nums[2]))
				}
				return check(nums[0], nums[1])
			}
		}
		if lat, lon, ok := decodeGeohash(trimmed); ok {
			return check(lat, lon)
		}
		bad := ' '
		for _, ch := range trimmed {
			if !strings.ContainsRune("0123456789bcdefghjkmnpqrstuvwxyzBCDEFGHJKMNPQRSTUVWXYZ", ch) {
				bad = ch
				break
			}
		}
		return nil, errf("parse_exception", fmt.Sprintf("unsupported symbol [%c] in geohash [%s]", bad, trimmed))
	case map[string]any:
		_, hasLat := t["lat"]
		_, hasLon := t["lon"]
		if hasLat || hasLon {
			for k := range t {
				if k != "lat" && k != "lon" {
					return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
				}
			}
			lat, ok := toFloat(t["lat"])
			if !ok {
				return nil, errf("parse_exception", "field [lat] missing")
			}
			lon, ok := toFloat(t["lon"])
			if !ok {
				return nil, errf("parse_exception", "field [lon] missing")
			}
			return check(lat, lon)
		}
		typ, _ := t["type"].(string)
		if strings.EqualFold(typ, "point") {
			if coords, ok := t["coordinates"].([]any); ok && len(coords) >= 2 {
				lon, ok1 := toFloat(coords[0])
				lat, ok2 := toFloat(coords[1])
				if ok1 && ok2 {
					return check(lat, lon)
				}
			}
		}
		return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
	case []any:
		if len(t) >= 3 && !ignoreZ {
			z, _ := toFloat(t[2])
			return nil, errf("parse_exception", fmt.Sprintf("Exception parsing coordinates: found Z value [%.1f] but [ignore_z_value] parameter is [false]", z))
		}
		if len(t) >= 2 {
			lon, ok1 := toFloat(t[0])
			lat, ok2 := toFloat(t[1])
			if ok1 && ok2 {
				return check(lat, lon)
			}
		}
		return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
	}
	return nil, errf("parse_exception", "failed to parse geo_point")
}

func (c *indexCtx) parseGeoShape(path string, v any) (any, *apiError) {
	errf := func(cause, reason string) *apiError {
		return mapperErr("mapper_parsing_exception", "failed to parse field ["+path+"] of type [geo_shape]", cause, reason)
	}
	knownTypes := map[string]bool{
		"point": true, "multipoint": true, "linestring": true,
		"multilinestring": true, "polygon": true, "multipolygon": true,
		"envelope": true, "geometrycollection": true,
	}
	switch t := v.(type) {
	case string:
		up := strings.ToUpper(strings.TrimSpace(t))
		for _, k := range []string{"POINT", "POLYGON", "LINESTRING", "MULTIPOINT", "MULTILINESTRING", "MULTIPOLYGON", "BBOX", "ENVELOPE", "GEOMETRYCOLLECTION"} {
			if strings.HasPrefix(up, k) {
				if _, ok := parseWktGeoShape(t); ok {
					return v, nil
				}
				return nil, errf("parse_exception", "Invalid WKT format")
			}
		}
		return nil, errf("illegal_argument_exception", "Unknown geometry type ["+t+"]")
	case map[string]any:
		typ, _ := t["type"].(string)
		typ = strings.ToLower(typ)
		if typ == "circle" {
			return nil, errf("unsupported_operation_exception", "circle is not supported")
		}
		if !knownTypes[typ] {
			return nil, errf("x_content_parse_exception", "Unknown geometry type ["+typ+"]")
		}
		if typ == "polygon" {
			rings, ok := t["coordinates"].([]any)
			if !ok {
				return nil, errf("x_content_parse_exception", "Invalid polygon")
			}
			for _, ring := range rings {
				pts, ok := ring.([]any)
				if !ok {
					return nil, errf("x_content_parse_exception", "Invalid polygon")
				}
				if len(pts) < 4 {
					return nil, errf("x_content_parse_exception", "Invalid polygon: ring has fewer than 4 points")
				}
				if !reflect.DeepEqual(pts[0], pts[len(pts)-1]) {
					return nil, errf("x_content_parse_exception", "Invalid polygon: ring is not closed")
				}
			}
		}
		if typ == "geometrycollection" {
			if _, ok := t["geometries"].([]any); !ok {
				return nil, errf("x_content_parse_exception", "Invalid geometry collection")
			}
		}
		return v, nil
	default:
		return nil, errf("x_content_parse_exception", "Invalid shape")
	}
}

func (c *indexCtx) parseCompletion(path string, def map[string]any, v any) (any, *apiError) {
	checkReserved := func(s string) *apiError {
		pos := 0
		for _, ch := range s {
			cp := int(ch)
			if cp < 0x20 || cp == 0x7f {
				return completionParseError("illegal_argument_exception",
					fmt.Sprintf("Illegal input [%s] UTF-16 codepoint [0x%x] at position %d is a reserved character", s, cp, pos))
			}
			if ch > 0xffff {
				pos += 2
			} else {
				pos++
			}
		}
		return nil
	}
	var out []any
	var parseOne func(v any) *apiError
	parseOne = func(v any) *apiError {
		switch t := v.(type) {
		case string:
			if err := checkReserved(t); err != nil {
				return err
			}
			out = append(out, t)
			return nil
		case map[string]any:
			for k := range t {
				if k != "input" && k != "weight" && k != "contexts" {
					return completionParseError("illegal_argument_exception",
						"unknown field name ["+k+"], must be one of [input, weight, contexts]")
				}
			}
			if w, has := t["weight"]; has {
				wi, ok := int64(0), false
				switch wt := w.(type) {
				case json.Number:
					if n, err := wt.Int64(); err == nil {
						wi, ok = n, true
					}
				case float64:
					if wt == float64(int64(wt)) {
						wi, ok = int64(wt), true
					}
				case string:
					if n, err := strconv.ParseInt(wt, 10, 64); err == nil && strings.TrimSpace(wt) == wt {
						wi, ok = n, true
					}
				}
				if !ok {
					return completionParseError("illegal_argument_exception",
						"weight must be an integer, but was ["+previewValue(w)+"]")
				}
				if wi < 0 || wi > 2147483647 {
					return completionParseError("illegal_argument_exception",
						fmt.Sprintf("weight must be in the interval [0..2147483647], but was [%d]", wi))
				}
			}
			if inp, has := t["input"]; has {
				switch it := inp.(type) {
				case string:
					if err := checkReserved(it); err != nil {
						return err
					}
					out = append(out, it)
				case []any:
					for _, e := range it {
						switch et := e.(type) {
						case string:
							if err := checkReserved(et); err != nil {
								return err
							}
							out = append(out, et)
						case map[string]any:
							iv, hasIv := et["input"]
							if !hasIv {
								iv = e
							}
							switch ivt := iv.(type) {
							case string:
								if err := checkReserved(ivt); err != nil {
									return err
								}
								out = append(out, ivt)
							case []any:
								for _, e2 := range ivt {
									if s, ok := valueScalarString(e2); ok {
										if err := checkReserved(s); err != nil {
											return err
										}
										out = append(out, s)
									}
								}
							default:
								return completionParseError("parsing_exception",
									fmt.Sprintf("failed to parse [%s]: expected text or object, but got %s", path, jsonValueKind(iv)))
							}
						default:
							return completionParseError("parsing_exception",
								fmt.Sprintf("failed to parse [%s]: expected text or object, but got %s", path, jsonValueKind(e)))
						}
					}
				default:
					return completionParseError("parsing_exception",
						fmt.Sprintf("failed to parse [%s]: expected text or object, but got %s", path, jsonValueKind(inp)))
				}
			}
			if len(out) == 0 {
				out = append(out, v)
			}
			return nil
		case []any:
			for _, e := range t {
				if err := parseOne(e); err != nil {
					return err
				}
			}
			return nil
		default:
			return completionParseError("parsing_exception",
				fmt.Sprintf("failed to parse [%s]: expected text or object, but got %s", path, jsonValueKind(v)))
		}
	}
	if err := parseOne(v); err != nil {
		return nil, err
	}
	if err := c.checkCompletionContexts(path, def, v); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *indexCtx) parseRankFeature(path string, v any) (any, *apiError) {
	f, ok := toFloat(v)
	if !ok {
		if s, ok := valueScalarString(v); ok {
			if fv, err := strconv.ParseFloat(s, 64); err == nil {
				f = fv
				ok = true
			}
		}
	}
	if !ok {
		return nil, c.fieldValueError(path, "rank_feature", v, "illegal_argument_exception", "failed to parse rank_feature")
	}
	shown := float64(float32(f))
	if shown <= 0 || shown < 1.1754944e-38 {
		return nil, c.fieldValueError(path, "rank_feature", v, "illegal_argument_exception",
			fmt.Sprintf("featureValue must be a positive normal float, got: %.1f for feature %s on field %s which is less than the minimum positive normal float: 1.1754944E-38", shown, path, path))
	}
	return f, nil
}

func jsonValueKind(v any) string {
	switch v.(type) {
	case nil:
		return "VALUE_NULL"
	case bool:
		return "VALUE_BOOLEAN"
	case json.Number, float64, float32, int, int64, uint64:
		return "VALUE_NUMBER"
	case string:
		return "VALUE_STRING"
	case []any:
		return "START_ARRAY"
	case map[string]any:
		return "START_OBJECT"
	default:
		return "VALUE_NULL"
	}
}

func (c *indexCtx) parseJoin(path string, def map[string]any, v any) (any, *apiError) {
	relations, _ := def["relations"].(map[string]any)
	switch t := v.(type) {
	case string:
		if _, ok := relations[t]; ok {
			return t, nil
		}
		return nil, c.fieldValueError(path, "join", v, "illegal_argument_exception",
			fmt.Sprintf("[%s] is not a valid join name", t))
	case map[string]any:
		name, _ := t["name"].(string)
		parent, _ := t["parent"].(string)
		if name == "" {
			return nil, c.fieldValueError(path, "join", v, "illegal_argument_exception", "join name is missing")
		}
		valid := false
		for parentName, children := range relations {
			if parentName == name {
				valid = true
				break
			}
			switch list := children.(type) {
			case string:
				if list == name {
					valid = true
				}
			case []any:
				for _, e := range list {
					if s, ok := e.(string); ok && s == name {
						valid = true
					}
				}
			}
			if valid {
				break
			}
		}
		if !valid {
			return nil, c.fieldValueError(path, "join", v, "illegal_argument_exception",
				fmt.Sprintf("[%s] is not a valid join name", name))
		}
		return map[string]any{"name": name, "parent": parent}, nil
	}
	return nil, c.fieldValueError(path, "join", v, "illegal_argument_exception", "join name is missing")
}

// ---------------------------------------------------------------------------
// Range field types
// ---------------------------------------------------------------------------

func (c *indexCtx) parseRangeValue(path, typ string, def map[string]any, v any) (any, *apiError) {
	if s, ok := v.(string); ok {
		if typ == "ip_range" {
			if strings.Contains(s, "/") {
				lower, upper, err := c.parseIPRangeString(path, s)
				if err != nil {
					return nil, err
				}
				return map[string]any{"gte": lower, "lte": upper}, nil
			}
			vals, err := c.parseIPValue(path, s)
			if err != nil {
				return nil, err
			}
			return map[string]any{"gte": vals, "lte": vals}, nil
		}
		return nil, c.fieldValueError(path, typ, v, "mapper_parsing_exception",
			fmt.Sprintf("error parsing field [%s], expected an object but got %s", path, s))
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, c.fieldValueError(path, typ, v, "mapper_parsing_exception",
			fmt.Sprintf("error parsing field [%s], expected an object but got %s", path, path))
	}
	hasGt, hasGte := m["gt"] != nil, m["gte"] != nil
	if hasGt && hasGte {
		return nil, c.fieldValueError(path, typ, v, "mapper_parsing_exception",
			fmt.Sprintf("error parsing field [%s], invalid lower bound (gt/gte)", path))
	}
	hasLt, hasLte := m["lt"] != nil, m["lte"] != nil
	if hasLt && hasLte {
		return nil, c.fieldValueError(path, typ, v, "mapper_parsing_exception",
			fmt.Sprintf("error parsing field [%s], invalid upper bound (lt/lte)", path))
	}
	parse := func(raw any) (any, bool, *apiError) {
		switch typ {
		case "date_range":
			rv, err := c.parseDateValue(path, "date", def, raw)
			if err != nil {
				reason := fmt.Sprintf("failed to parse date field [%s] with format [%s]", previewValue(raw), dateFormatOf(def))
				if err.CausedBy != nil && err.CausedBy.Reason != "" {
					reason += ": [" + err.CausedBy.Reason + "]"
				}
				return nil, false, c.fieldValueError(path, typ, v, "parse_exception", reason)
			}
			return rv, true, nil
		case "ip_range":
			rv, err := c.parseIPValue(path, raw)
			if err != nil {
				return nil, false, err
			}
			return rv, true, nil
		default:
			isFloat := typ == "float_range" || typ == "double_range"
			if isFloat {
				rv, err := c.parseFloatValue(path, strings.TrimSuffix(typ, "_range"), def, raw)
				if err != nil {
					if isIntegerKind(strings.TrimSuffix(typ, "_range")) {
						return nil, false, err
					}
					return nil, false, err
				}
				return rv, true, nil
			}
			rv, err := c.parseIntValue(path, strings.TrimSuffix(typ, "_range"), def, raw)
			if err != nil {
				return nil, false, err
			}
			return rv, true, nil
		}
	}
	out := map[string]any{}
	for _, key := range []string{"gt", "gte", "lt", "lte"} {
		raw, present := m[key]
		if !present {
			continue
		}
		pv, _, err := parse(raw)
		if err != nil {
			return nil, err
		}
		out[key] = pv
	}
	for k := range m {
		if k != "gt" && k != "gte" && k != "lt" && k != "lte" {
			return nil, c.fieldValueError(path, typ, v, "mapper_parsing_exception",
				fmt.Sprintf("error parsing field [%s], with unknown parameter [%s]", path, k))
		}
	}
	gte, hasLower := out["gte"]
	if !hasLower {
		gte = out["gt"]
	}
	lte, hasUpper := out["lte"]
	if !hasUpper {
		lte = out["lt"]
	}
	if hasLower && hasUpper {
		if compareValuesForType(typ, gte, lte) > 0 {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception",
				fmt.Sprintf("min value (%v) is greater than max value (%v)", boundsValue(gte), boundsValue(lte)))
		}
	}
	return out, nil
}

func boundsValue(v any) string {
	switch t := v.(type) {
	case float64:
		return formatNumber(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}

func compareValuesForType(typ string, a, b any) int {
	switch typ {
	case "ip_range":
		return compareIPs(a.(string), b.(string))
	default:
		af, _ := toFloat(a)
		bf, _ := toFloat(b)
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		}
		return 0
	}
}

func compareIPs(a, b string) int {
	_, an, _ := net.ParseCIDR(a)
	if an == nil {
		ip := net.ParseIP(a)
		if ip == nil {
			return 0
		}
		return 0
	}
	_, bn, _ := net.ParseCIDR(b)
	if bn == nil {
		return 0
	}
	return strings.Compare(an.String(), bn.String())
}
