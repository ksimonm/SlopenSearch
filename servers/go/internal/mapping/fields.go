// Package mapping implements index mappings and field types: mapping
// validation and merging, dotted names, dynamic mappings and templates, and
// the index-time parsing of document fields into doc values (ParseContext).
package mapping

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

	"slopensearch/internal/analysis"
	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/geohash"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/settings"
)

// ---------------------------------------------------------------------------
// Index-time field value extraction.
//
// OpenSearch stores an indexed (normalized) form of each field value that is
// independent of the original _source. Term/range queries, sorting and
// aggregations run against these values, while _source is echoed verbatim.
// ---------------------------------------------------------------------------

type ParseContext struct {
	Idx            *model.Index
	IndexCoerce    bool
	indexIgnoreMal bool
	docID          string
	Doc            map[string]any
	ignored        []string
	ignoredSet     map[string]bool
	Values         map[string][]any
	totalFields    int
	fieldNameLimit int
	dyn            string
}

func NewParseContext(idx *model.Index, docID string) *ParseContext {
	c := &ParseContext{
		Idx:        idx,
		docID:      docID,
		ignoredSet: map[string]bool{},
		Values:     map[string][]any{},
	}
	c.IndexCoerce = true
	if v, ok := settings.Find(idx.Settings, "mapping.coerce"); ok {
		c.IndexCoerce = settings.ValueString(v) != "false"
	}
	if v, ok := settings.Find(idx.Settings, "mapping.ignore_malformed"); ok {
		c.indexIgnoreMal = settings.ValueString(v) == "true"
	}
	c.totalFields = 1000
	if v, ok := settings.Int(idx.Settings, "mapping.total_fields.limit"); ok {
		c.totalFields = v
	}
	c.fieldNameLimit = 1000
	if v, ok := settings.Int(idx.Settings, "mapping.field_name_length.limit"); ok {
		c.fieldNameLimit = v
	}
	c.dyn = RootDynamicString(idx)
	if enabled, ok := idx.Mappings["enabled"].(bool); ok && !enabled {
		c.dyn = "false"
	}
	return c
}

func mapperErr(typ, reason string, causedType, causedReason string) *apierr.Error {
	e := &apierr.Error{Type: "mapper_parsing_exception", Reason: reason, Status: 400}
	if causedType != "" {
		e.CausedBy = &apierr.Error{Type: causedType, Reason: causedReason}
	} else if causedReason != "" {
		e.CausedBy = &apierr.Error{Type: "illegal_argument_exception", Reason: causedReason}
	}
	return e
}

// fieldValueError builds the standard "failed to parse field [f] of type [t] in
// document with id 'x'" wrapper.
func (c *ParseContext) fieldValueError(name, typ string, v any, causeType, causeReason string) *apierr.Error {
	preview := PreviewValue(v)
	reason := fmt.Sprintf("failed to parse field [%s] of type [%s] in document with id '%s'. Preview of field's value: '%s'", name, typ, c.docID, preview)
	return mapperErr("mapper_parsing_exception", reason, causeType, causeReason)
}

func PreviewValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return jsonv.FormatNumber(t)
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

func IgnoreAboveOf(def map[string]any) (int, bool) {
	v, ok := def["ignore_above"]
	if !ok {
		return 0, false
	}
	n, ok := jsonv.ToFloat(v)
	if !ok || n < 0 {
		return 0, false
	}
	return int(n), true
}

func ignoreAboveExceeded(def map[string]any, v any) bool {
	limit, ok := IgnoreAboveOf(def)
	if !ok {
		return false
	}
	s, ok := jsonv.ScalarString(v)
	if !ok {
		return false
	}
	return len([]rune(s)) > limit
}

// IsIntegerKind reports whether t is an integer field type.
func IsIntegerKind(t string) bool {
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

// ParseIntValue converts a raw JSON number/string into an int64 value subject
// to OpenSearch coercion rules.
func (c *ParseContext) ParseIntValue(name, typ string, def map[string]any, v any) (any, *apierr.Error) {
	coerce := c.IndexCoerce
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
	return nil, c.fieldValueError(name, typ, v, "input_coercion_exception", "For input string: \""+PreviewValue(v)+"\"")
}

func (c *ParseContext) parseIntFloat(name, typ string, def map[string]any, f float64, coerce bool) (any, *apierr.Error) {
	if !coerce {
		if f != math.Trunc(f) {
			return nil, c.fieldValueError(name, typ, jsonv.FormatNumber(f), "illegal_argument_exception",
				fmt.Sprintf("%s cannot be converted to Integer without data loss", jsonv.FormatNumber(f)))
		}
	}
	iv := int64(f)
	return c.checkIntRange(name, typ, iv, f, true)
}

func (c *ParseContext) checkIntRange(name, typ string, iv int64, orig float64, fromJSON bool) (any, *apierr.Error) {
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

func (c *ParseContext) parseIntString(name, typ string, def map[string]any, s string, coerce, trim bool) (any, *apierr.Error) {
	coerceFalse := !coerce
	if b, ok := def["coerce"].(bool); ok {
		coerceFalse = !b
	}
	if coerceFalse {
		word := "Integer"
		if !IsIntegerKind(typ) {
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
		if IsIntegerKind(typ) {
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

// ParseFloatValue implements float/double/half_float/scaled_float coercion.
func (c *ParseContext) ParseFloatValue(name, typ string, def map[string]any, v any) (any, *apierr.Error) {
	coerceFalse := !c.IndexCoerce
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
			fmt.Sprintf("[%s] supports only finite values, but got [%s]", typ, PreviewValue(v)))
	}
	switch typ {
	case "float":
		if f > math.MaxFloat32 || f < -math.MaxFloat32 {
			return nil, c.fieldValueError(name, typ, v, "input_coercion_exception",
				fmt.Sprintf("Value [%s] is out of range for a float", jsonv.FormatNumber(f)))
		}
		return float64(float32(f)), nil
	case "half_float":
		if f > 65504 || f < -65504 {
			return nil, c.fieldValueError(name, typ, v, "input_coercion_exception",
				fmt.Sprintf("Value [%s] is out of range for a half_float", jsonv.FormatNumber(f)))
		}
		return float64(float32FromHalf(float32Half(f))), nil
	case "scaled_float":
		scale := 1.0
		if s, ok := jsonv.ToFloat(def["scaling_factor"]); ok {
			scale = s
		}
		// ScaledFloatFieldMapper: Math.round(value * factor) is indexed and
		// read back multiplied by 1 / factor
		return math.Floor(f*scale+0.5) * (1 / scale), nil
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

// ParseBooleanValue implements boolean coercion.
func (c *ParseContext) ParseBooleanValue(name string, def map[string]any, v any) (any, *apierr.Error) {
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
			fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", PreviewValue(v)))
	}
}

// ParseDateValue stores epoch millis (date) or epoch nanos (date_nanos).
func (c *ParseContext) ParseDateValue(name, typ string, def map[string]any, v any) (any, *apierr.Error) {
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
			s = PreviewValue(v)
		}
	}
	if typ == "date_nanos" {
		if ns, rangeErr, ok := dates.ParseNanosAny(def, s); ok {
			return ns, nil
		} else if rangeErr != "" {
			return nil, c.fieldValueError(name, typ, v, "illegal_argument_exception", rangeErr)
		}
	} else if ms, ok := dates.ParseAny(def, s); ok {
		return ms, nil
	}
	reason := fmt.Sprintf("failed to parse date field [%s] with format [%s]", s, dates.FormatOf(def))
	e := c.fieldValueError(name, typ, v, "illegal_argument_exception", reason)
	e.CausedBy.CausedBy = &apierr.Error{Type: "date_time_parse_exception", Status: 400,
		Reason: "Failed to parse with all enclosed parsers"}
	return nil, e
}

// ParseIPValue canonicalizes an IPv4/IPv6 string or CIDR.
func (c *ParseContext) ParseIPValue(name string, v any) (any, *apierr.Error) {
	s, ok := jsonv.ToString(v)
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
func (c *ParseContext) parseIPRangeString(name, s string) (string, string, *apierr.Error) {
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
func (c *ParseContext) indexRawField(path string, def map[string]any, raw any) *apierr.Error {
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
	if typ == "knn_vector" {
		return c.indexKnnVector(path, def, raw)
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
				if s, ok := jsonv.ScalarString(v); ok {
					c.Values[path+"."+prefix] = append(c.Values[path+"."+prefix], s)
					c.Values[path] = append(c.Values[path], s)
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
			f, ok := jsonv.ToFloat(fv)
			if !ok {
				if s, ok := jsonv.ScalarString(fv); ok {
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
			c.Values[sub] = append(c.Values[sub], f)
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
			if _, ok1 := jsonv.ToFloat(arr[0]); ok1 {
				if _, ok2 := jsonv.ToFloat(arr[1]); ok2 {
					values = []any{raw}
				}
			}
		}
	}
	if values == nil {
		var err *apierr.Error
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
		c.Values[path] = append(c.Values[path], out...)
	}
	if typ == "text" {
		var prefixes []any
		if ip, ok := def["index_prefixes"].(map[string]any); ok {
			minChars := 2
			if n, ok := jsonv.ToFloat(ip["min_chars"]); ok {
				minChars = int(n)
			}
			maxChars := 5
			if n, ok := jsonv.ToFloat(ip["max_chars"]); ok {
				maxChars = int(n)
			}
			for _, v := range out {
				sv, ok := jsonv.ScalarString(v)
				if !ok {
					continue
				}
				for _, tok := range Analyze(sv) {
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
			c.Values[path+"._index_prefix"] = append(c.Values[path+"._index_prefix"], prefixes...)
		}
		if b, ok := def["index_phrases"].(bool); ok && b {
			var phrases []any
			for _, v := range out {
				sv, ok := jsonv.ScalarString(v)
				if !ok {
					continue
				}
				toks := Analyze(sv)
				for i := 0; i+1 < len(toks); i++ {
					phrases = append(phrases, toks[i]+" "+toks[i+1])
				}
			}
			if len(phrases) > 0 {
				c.Values[path+"._index_phrase"] = append(c.Values[path+"._index_phrase"], phrases...)
			}
		}
	}
	if typ == "search_as_you_type" {
		maxShingle := 3
		if n, ok := jsonv.ToFloat(def["max_shingle_size"]); ok {
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
			sv, ok := jsonv.ScalarString(v)
			if !ok {
				continue
			}
			toks := Analyze(sv)
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
				c.Values[key] = append(c.Values[key], list...)
			}
		}
		if len(prefixes) > 0 {
			c.Values[path+"._index_prefix"] = append(c.Values[path+"._index_prefix"], prefixes...)
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
				c.Values[sub] = append(c.Values[sub], subVals...)
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

func (c *ParseContext) copyToField(target string, vals []any) *apierr.Error {
	tdef, mapped := FieldDef(c.Idx, target)
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
			c.Values[target] = append(c.Values[target], nv)
			continue
		}
		c.Values[target] = append(c.Values[target], v)
	}
	return nil
}

func (c *ParseContext) addIgnored(path string) {
	if !c.ignoredSet[path] {
		c.ignoredSet[path] = true
		c.ignored = append(c.ignored, path)
	}
}

func (c *ParseContext) coerce(path string, def map[string]any, typ string, v any) (any, *apierr.Error) {
	switch typ {
	case "text", "match_only_text", "search_as_you_type", "wildcard":
		if _, isMap := v.(map[string]any); isMap {
			return nil, c.fieldValueError(path, typ, v, "illegal_state_exception", "Can't get text on a START_OBJECT at 1:6")
		}
		s, ok := jsonv.ScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		if typ == "wildcard" {
			if norm, ok := def["normalizer"].(string); ok && norm != "" {
				s = applyNormalizerWithIndex(c.Idx, norm, s)
			}
			return s, nil
		}
		return s, nil
	case "keyword", "constant_keyword":
		if _, isMap := v.(map[string]any); isMap {
			return nil, c.fieldValueError(path, typ, v, "illegal_state_exception", "Can't get text on a START_OBJECT at 1:6")
		}
		s, ok := jsonv.ScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		if typ == "constant_keyword" {
			if cv, has := def["value"]; has {
				cs, _ := jsonv.ScalarString(cv)
				if s != cs {
					return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception",
						"constant keyword field ["+path+"] must have a value of ["+cs+"]")
				}
			}
		}
		if norm, ok := def["normalizer"].(string); ok && norm != "" {
			s = applyNormalizerWithIndex(c.Idx, norm, s)
		}
		return s, nil
	case "integer", "long", "short", "byte", "unsigned_long":
		return c.ParseIntValue(path, typ, def, v)
	case "float", "double", "half_float", "scaled_float":
		return c.ParseFloatValue(path, typ, def, v)
	case "boolean":
		return c.ParseBooleanValue(path, def, v)
	case "date", "date_nanos":
		return c.ParseDateValue(path, typ, def, v)
	case "ip":
		return c.ParseIPValue(path, v)
	case "binary":
		s, ok := jsonv.ScalarString(v)
		if !ok {
			return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
		}
		dv, _ := def["doc_values"].(bool)
		store, _ := def["store"].(bool)
		if store || dv {
			if _, err := base64.StdEncoding.DecodeString(s); err != nil {
				reason := "Unexpected end of base64-encoded String: base64 variant 'MIME-NO-LINEFEEDS' expects padding (one or more '=' characters) at the end. This Base64Variant might have been incorrectly configured"
				return nil, mapperErr("mapper_parsing_exception",
					fmt.Sprintf("failed to parse field [%s] of type [binary] in document with id '%s'. Preview of field's value: '%s'", path, c.docID, PreviewValue(v)),
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
		s, ok := jsonv.ScalarString(v)
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
	if IsNumericType(typ) {
		return c.ParseFloatValue(path, typ, def, v)
	}
	if s, ok := jsonv.ScalarString(v); ok {
		return s, nil
	}
	return nil, c.fieldValueError(path, typ, v, "illegal_argument_exception", "malformed")
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}

// flattenScalars flattens arrays into individual scalar values. An empty array
// yields no values; nested arrays are flattened recursively.
func (c *ParseContext) flattenScalars(path string, raw any) ([]any, *apierr.Error) {
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

func (c *ParseContext) indexObject(path string, def map[string]any, raw any) *apierr.Error {
	props, _ := def["properties"].(map[string]any)
	attachProps := false
	if props == nil {
		props = map[string]any{}
		attachProps = true
	}
	if d, ok := def["dynamic"]; ok {
		saved := c.dyn
		c.dyn = DynamicString(d)
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

func (c *ParseContext) indexObjectMap(path string, props map[string]any, src map[string]any) *apierr.Error {
	for rawK, v := range src {
		k := strings.TrimRight(rawK, ".")
		if k == "" {
			k = rawK
		}
		if IsMetadataField(k) {
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
			nd, matched, derr := DynamicDefFor(c.Idx, childPath, k, v)
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
				return &apierr.Error{Type: "strict_dynamic_mapping_exception", Status: 400,
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
					return &apierr.Error{Type: "strict_dynamic_mapping_exception", Status: 400,
						Reason: "mapping set to strict, dynamic introduction of [" + target + "] within [_doc] is not allowed"}
				}
				props[target] = map[string]any{"type": "text"}
				c.bumpTotalFields()
			}
		}
	}
	return nil
}

func IsMetadataField(name string) bool {
	switch name {
	case "_id", "_index", "_source", "_routing", "_field_names", "_ignored":
		return true
	}
	return false
}

func (c *ParseContext) rejectMetadataField(name string) *apierr.Error {
	return mapperErr("mapper_parsing_exception",
		fmt.Sprintf("Field [%s] is a metadata field and cannot be added inside a document. Use the index API request parameters.", name),
		"mapper_parsing_exception",
		fmt.Sprintf("Field [%s] is a metadata field and cannot be added inside a document. Use the index API request parameters.", name))
}

func (c *ParseContext) bumpTotalFields() {}

// fillConstants fills the fixed constant_keyword value for docs missing the field.
func (c *ParseContext) fillConstants(props map[string]any, src map[string]any) {
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "constant_keyword" {
			if _, present := src[name]; !present {
				if v, ok := def["value"]; ok {
					c.Values[name] = []any{fmt.Sprint(v)}
				}
			}
		}
	}
}

// BuildDocValues computes indexed values for a document.
func (c *ParseContext) BuildDocValues() *apierr.Error {
	props, _ := c.Idx.Mappings["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		c.Idx.Mappings["properties"] = props
	}
	if err := c.indexObjectMap("", props, c.Doc); err != nil {
		return err
	}
	c.fillConstants(props, c.Doc)
	if len(c.ignored) > 0 {
		sort.Strings(c.ignored)
		c.Values["_ignored"] = []any{}
		for _, f := range c.ignored {
			c.Values["_ignored"] = append(c.Values["_ignored"], f)
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
		v, ok := FieldValue(doc, key)
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
	base, ok := FieldValue(doc, key)
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
					lhs, lok := jsonv.ToFloat(base)
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

func addDerivedValues(idx *model.Index, c *ParseContext) {
	defs, _ := idx.Mappings["derived"].(map[string]any)
	for name, raw := range defs {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := evalDerived(def, c.Doc); ok {
			c.Values[name] = []any{v}
		}
	}
}

// NormalizeDerivedMappings gives derived scripts their default lang.
func NormalizeDerivedMappings(mappings map[string]any) {
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

// ApplyRequestDerived installs request-level derived field definitions.
func ApplyRequestDerived(idx *model.Index, defs any) *apierr.Error {
	src, ok := defs.(map[string]any)
	if !ok {
		return &apierr.Error{Type: "parsing_exception", Status: 400, Reason: "expected derived to be an object"}
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
	RecomputeAllDocValues(idx)
	EnsureFieldTypes(idx)
	return nil
}

// ComputeDocValues validates and indexes a document, storing normalized field
// values used by search, sort, aggregations and docvalue_fields.
func ComputeDocValues(idx *model.Index, id string, doc map[string]any) *apierr.Error {
	// the document is about to change: searches keep its refreshed version
	idx.NoteWrite(id)
	return computeDocValues(idx, id, doc)
}

func computeDocValues(idx *model.Index, id string, doc map[string]any) *apierr.Error {
	if reason := invalidDocFieldName(doc); reason != "" {
		return mapperErr("mapper_parsing_exception", reason, "illegal_argument_exception", reason)
	}
	if derr := checkObjectDepth(doc, 0, mappingIntSetting(idx, "mapping.depth.limit", 20)); derr != nil {
		return derr
	}
	if derr := checkArrayDepth(doc, 0, mappingIntSetting(idx, "mapping.depth.limit", 20)); derr != nil {
		return derr
	}
	if derr := checkDocFieldNameLengths(doc, mappingIntSetting(idx, "mapping.field_name_length.limit", 0)); derr != nil {
		return derr
	}
	if props, ok := idx.Mappings["properties"].(map[string]any); ok {
		limit := mappingIntSetting(idx, "mapping.nested_objects.limit", 10000)
		if countNestedDocuments(props, doc) > limit {
			return nestedObjectsLimitError(limit)
		}
	}
	c := NewParseContext(idx, id)
	c.Doc = doc
	if err := c.BuildDocValues(); err != nil {
		return err
	}
	addDerivedValues(idx, c)
	KnnDeriveSource(idx, doc)
	if idx.DocValues == nil {
		idx.DocValues = map[string]map[string][]any{}
	}
	idx.DocValues[id] = c.Values
	idx.DVGen++
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

func RemoveDocValues(idx *model.Index, id string) {
	delete(idx.DocValues, id)
	delete(idx.DocIgnored, id)
	idx.DVGen++
}

// RecomputeAllDocValues rebuilds doc values for every document (used when
// mappings change).
func RecomputeAllDocValues(idx *model.Index) {
	idx.DVGen++
	idx.DocValues = map[string]map[string][]any{}
	idx.DocIgnored = map[string][]string{}
	for _, id := range idx.Order {
		doc, live := idx.Docs[id]
		if !live {
			continue
		}
		_ = computeDocValues(idx, id, doc)
	}
}

func applyNormalizer(name, s string) string {
	switch name {
	case "lowercase":
		return strings.ToLower(s)
	}
	return s
}

func ASCIILower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func applyNormalizerWithIndex(idx *model.Index, name, s string) string {
	if idx != nil {
		if rawFilters, ok := settings.Find(idx.Settings, "analysis.normalizer."+name+".filter"); ok {
			for _, f := range analysis.FilterNames(rawFilters) {
				switch f {
				case "lowercase":
					s = strings.ToLower(s)
				case "uppercase":
					s = strings.ToUpper(s)
				case "asciifolding":
					s = analysis.ASCIIFoldReplacer.Replace(s)
				}
			}
			return s
		}
	}
	return applyNormalizer(name, s)
}

func NormalizeQueryValue(idx *model.Index, field, s string) string {
	def, ok := FieldDef(idx, field)
	if !ok {
		return s
	}
	norm, _ := def["normalizer"].(string)
	if norm == "" {
		return s
	}
	return applyNormalizerWithIndex(idx, norm, s)
}

func (c *ParseContext) parseTokenCount(path string, def map[string]any, v any) (any, *apierr.Error) {
	s, ok := jsonv.ScalarString(v)
	if !ok {
		return nil, c.fieldValueError(path, "token_count", v, "illegal_argument_exception", "malformed")
	}
	analyzer, _ := def["analyzer"].(string)
	tokens := analysis.Tokens(AnalysisIndex(c.Idx), analyzer, s)
	epi := true
	if b, ok := def["enable_position_increments"].(bool); ok {
		epi = b
	}
	if epi {
		if len(tokens) == 0 {
			return int64(0), nil
		}
		return int64(tokens[len(tokens)-1].Pos + 1), nil
	}
	return int64(len(tokens)), nil
}

func (c *ParseContext) parseGeoPoint(path string, def map[string]any, v any) (any, *apierr.Error) {
	ignoreZ := true
	if b, ok := def["ignore_z_value"].(bool); ok {
		ignoreZ = b
	}
	ignoreMal := c.indexIgnoreMal
	if b, ok := def["ignore_malformed"].(bool); ok {
		ignoreMal = b
	}
	errf := func(cause, reason string) *apierr.Error {
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
	check := func(lat, lon float64) (any, *apierr.Error) {
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
		lat, lon, gherr := geohash.Point(trimmed)
		if gherr != "" {
			return nil, errf("parse_exception", gherr)
		}
		return check(lat, lon)
	case map[string]any:
		_, hasLat := t["lat"]
		_, hasLon := t["lon"]
		if hasLat || hasLon {
			for k := range t {
				if k != "lat" && k != "lon" {
					return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
				}
			}
			lat, ok := jsonv.ToFloat(t["lat"])
			if !ok {
				return nil, errf("parse_exception", "field [lat] missing")
			}
			lon, ok := jsonv.ToFloat(t["lon"])
			if !ok {
				return nil, errf("parse_exception", "field [lon] missing")
			}
			return check(lat, lon)
		}
		typ, _ := t["type"].(string)
		if strings.EqualFold(typ, "point") {
			if coords, ok := t["coordinates"].([]any); ok && len(coords) >= 2 {
				lon, ok1 := jsonv.ToFloat(coords[0])
				lat, ok2 := jsonv.ToFloat(coords[1])
				if ok1 && ok2 {
					return check(lat, lon)
				}
			}
		}
		return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
	case []any:
		if len(t) >= 3 && !ignoreZ {
			z, _ := jsonv.ToFloat(t[2])
			return nil, errf("parse_exception", fmt.Sprintf("Exception parsing coordinates: found Z value [%.1f] but [ignore_z_value] parameter is [false]", z))
		}
		if len(t) >= 2 {
			lon, ok1 := jsonv.ToFloat(t[0])
			lat, ok2 := jsonv.ToFloat(t[1])
			if ok1 && ok2 {
				return check(lat, lon)
			}
		}
		return nil, errf("parse_exception", "field must be either [lon|lat], [type|coordinates], or [geohash]")
	}
	return nil, errf("parse_exception", "failed to parse geo_point")
}

func (c *ParseContext) parseGeoShape(path string, v any) (any, *apierr.Error) {
	errf := func(cause, reason string) *apierr.Error {
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
				if _, ok := ParseWktGeoShape(t); ok {
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

func (c *ParseContext) parseCompletion(path string, def map[string]any, v any) (any, *apierr.Error) {
	checkReserved := func(s string) *apierr.Error {
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
	var parseOne func(v any) *apierr.Error
	parseOne = func(v any) *apierr.Error {
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
						"weight must be an integer, but was ["+PreviewValue(w)+"]")
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
									if s, ok := jsonv.ScalarString(e2); ok {
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

func (c *ParseContext) parseRankFeature(path string, v any) (any, *apierr.Error) {
	f, ok := jsonv.ToFloat(v)
	if !ok {
		if s, ok := jsonv.ScalarString(v); ok {
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

func (c *ParseContext) parseJoin(path string, def map[string]any, v any) (any, *apierr.Error) {
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

func (c *ParseContext) parseRangeValue(path, typ string, def map[string]any, v any) (any, *apierr.Error) {
	if s, ok := v.(string); ok {
		if typ == "ip_range" {
			if strings.Contains(s, "/") {
				lower, upper, err := c.parseIPRangeString(path, s)
				if err != nil {
					return nil, err
				}
				return map[string]any{"gte": lower, "lte": upper}, nil
			}
			vals, err := c.ParseIPValue(path, s)
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
	parse := func(raw any) (any, bool, *apierr.Error) {
		switch typ {
		case "date_range":
			rv, err := c.ParseDateValue(path, "date", def, raw)
			if err != nil {
				reason := fmt.Sprintf("failed to parse date field [%s] with format [%s]", PreviewValue(raw), dates.FormatOf(def))
				if err.CausedBy != nil && err.CausedBy.Reason != "" {
					reason += ": [" + err.CausedBy.Reason + "]"
				}
				return nil, false, c.fieldValueError(path, typ, v, "parse_exception", reason)
			}
			return rv, true, nil
		case "ip_range":
			rv, err := c.ParseIPValue(path, raw)
			if err != nil {
				return nil, false, err
			}
			return rv, true, nil
		default:
			isFloat := typ == "float_range" || typ == "double_range"
			if isFloat {
				rv, err := c.ParseFloatValue(path, strings.TrimSuffix(typ, "_range"), def, raw)
				if err != nil {
					if IsIntegerKind(strings.TrimSuffix(typ, "_range")) {
						return nil, false, err
					}
					return nil, false, err
				}
				return rv, true, nil
			}
			rv, err := c.ParseIntValue(path, strings.TrimSuffix(typ, "_range"), def, raw)
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
		return jsonv.FormatNumber(t)
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
		af, _ := jsonv.ToFloat(a)
		bf, _ := jsonv.ToFloat(b)
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
