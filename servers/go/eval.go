package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Query evaluation against indexed (doc values) field values.
// ---------------------------------------------------------------------------

type queryCtx struct {
	idx          *Index
	id           string
	doc          map[string]any
	vals         map[string][]any
	nestedPath   string
	nestedRoot   map[string]any
	nestedOffset int
	nestedParent any
}

func (c *queryCtx) nestedMeta() map[string]any {
	if c.nestedRoot == nil {
		return nil
	}
	field := c.nestedPath
	if i := strings.LastIndex(field, "."); i >= 0 {
		field = field[i+1:]
	}
	meta := map[string]any{"field": field, "offset": int64(c.nestedOffset)}
	parent, ok := c.nestedParent.(map[string]any)
	if !ok || parent == nil {
		return meta
	}
	out := map[string]any{}
	for k, v := range parent {
		out[k] = v
	}
	cur := out
	for {
		next, ok := cur["_nested"].(map[string]any)
		if !ok {
			break
		}
		cur = next
	}
	cur["_nested"] = meta
	return out
}

func newQueryCtxForDoc(idx *Index, id string, doc map[string]any) *queryCtx {
	if _, ok := idx.DocValues[id]; ok {
		return &queryCtx{idx: idx, id: id, doc: doc}
	}
	c := newIndexCtx(idx, id)
	c.doc = doc
	if err := c.buildDocValues(); err != nil {
		return &queryCtx{idx: idx, id: id, doc: doc}
	}
	return &queryCtx{idx: idx, id: id, doc: doc, vals: c.values}
}

func (c *queryCtx) rawValues() (map[string][]any, bool) {
	if c.vals != nil {
		return c.vals, true
	}
	if dv, ok := c.idx.DocValues[c.id]; ok {
		return dv, true
	}
	return nil, false
}

func (c *queryCtx) values(field string) ([]any, bool) {
	resolved := resolveAliasField(c.idx, field)
	if resolved == "_id" {
		return []any{c.id}, true
	}
	if resolved == "_routing" {
		if rv, ok := c.idx.Routing[c.id]; ok && rv != "" {
			return []any{rv}, true
		}
		return nil, false
	}
	if c.nestedRoot != nil {
		if resolved == c.nestedPath {
			return []any{c.nestedRoot}, true
		}
		prefix := c.nestedPath + "."
		if strings.HasPrefix(resolved, prefix) {
			return nestedObjectValues(c.nestedRoot, strings.TrimPrefix(resolved, prefix))
		}
		return nil, false
	}
	if _, nested := nestedAncestorPath(c.idx, resolved); nested && !nestedAncestorIncluded(c.idx, resolved) {
		return nil, false
	}
	if dv, ok := c.rawValues(); ok {
		if vals, ok := dv[resolved]; ok {
			return vals, len(vals) > 0
		}
		if resolved != field {
			if vals, ok := dv[field]; ok {
				return vals, len(vals) > 0
			}
		}
	}
	return nil, false
}

func expandNestedValue(v any) []any {
	if arr, ok := v.([]any); ok {
		var out []any
		for _, e := range arr {
			out = append(out, expandNestedValue(e)...)
		}
		return out
	}
	return []any{v}
}

func nestedObjectValues(root map[string]any, path string) ([]any, bool) {
	if path == "" {
		return []any{root}, true
	}
	cur := []any{root}
	for _, part := range strings.Split(path, ".") {
		var next []any
		for _, v := range cur {
			for _, item := range expandNestedValue(v) {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if nv, ok := m[part]; ok {
					next = append(next, expandNestedValue(nv)...)
				}
			}
		}
		cur = next
		if len(cur) == 0 {
			return nil, false
		}
	}
	return cur, len(cur) > 0
}

func nestedObjectsAt(root map[string]any, path string) []map[string]any {
	if path == "" {
		return []map[string]any{root}
	}
	cur := []any{root}
	for _, part := range strings.Split(path, ".") {
		var next []any
		for _, v := range cur {
			for _, item := range expandNestedValue(v) {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if nv, ok := m[part]; ok {
					next = append(next, expandNestedValue(nv)...)
				}
			}
		}
		cur = next
		if len(cur) == 0 {
			return nil
		}
	}
	var out []map[string]any
	for _, v := range cur {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func nestedAncestorPath(idx *Index, field string) (string, bool) {
	props, _ := idx.Mappings["properties"].(map[string]any)
	cur := props
	prefix := ""
	parts := strings.Split(field, ".")
	for i := 0; i < len(parts)-1; i++ {
		def, ok := cur[parts[i]].(map[string]any)
		if !ok {
			return "", false
		}
		if prefix == "" {
			prefix = parts[i]
		} else {
			prefix += "." + parts[i]
		}
		if t, _ := def["type"].(string); t == "nested" {
			return prefix, true
		}
		sub, _ := def["properties"].(map[string]any)
		if sub == nil {
			return "", false
		}
		cur = sub
	}
	return "", false
}

// nestedAncestorIncluded reports whether a field under a nested ancestor is
// also indexed at the parent/root because of include_in_parent/include_in_root.
func nestedAncestorIncluded(idx *Index, field string) bool {
	props, _ := idx.Mappings["properties"].(map[string]any)
	cur := props
	parts := strings.Split(field, ".")
	for i := 0; i < len(parts)-1; i++ {
		def, ok := cur[parts[i]].(map[string]any)
		if !ok {
			return false
		}
		if t, _ := def["type"].(string); t == "nested" {
			if b, ok := def["include_in_parent"].(bool); ok && b {
				return true
			}
			if b, ok := def["include_in_root"].(bool); ok && b {
				return true
			}
		}
		sub, _ := def["properties"].(map[string]any)
		if sub == nil {
			return false
		}
		cur = sub
	}
	return false
}

func (c *queryCtx) hasField(field string) bool {
	resolved := resolveAliasField(c.idx, field)
	if resolved == "_id" {
		return true
	}
	if def, ok := mappingFieldDef(c.idx, resolved); ok {
		notIndexed := false
		if iv, ok := def["index"].(bool); ok && !iv {
			notIndexed = true
		}
		noDocValues := false
		if dv, ok := def["doc_values"].(bool); ok && !dv {
			noDocValues = true
		}
		if notIndexed && noDocValues {
			return false
		}
	}
	if c.nestedRoot != nil {
		if resolved == c.nestedPath {
			return true
		}
		prefix := c.nestedPath + "."
		if strings.HasPrefix(resolved, prefix) {
			_, ok := nestedObjectValues(c.nestedRoot, strings.TrimPrefix(resolved, prefix))
			return ok
		}
		return false
	}
	if _, nested := nestedAncestorPath(c.idx, resolved); nested {
		return false
	}
	if dv, ok := c.rawValues(); ok {
		if vals, ok := dv[resolved]; ok && len(vals) > 0 {
			return true
		}
		for path := range dv {
			if strings.HasPrefix(path, resolved+".") {
				return true
			}
		}
	}
	return false
}

func resolveAliasField(idx *Index, field string) string {
	for i := 0; i < 10; i++ {
		def, ok := mappingFieldDef(idx, field)
		if !ok {
			return field
		}
		if t, _ := def["type"].(string); t == "alias" {
			path, _ := def["path"].(string)
			if path == "" || path == field {
				return field
			}
			field = path
			continue
		}
		return field
	}
	return field
}

// mappingFieldDef looks up a field definition by dotted path.
func mappingFieldDef(idx *Index, field string) (map[string]any, bool) {
	if derived, ok := idx.Mappings["derived"].(map[string]any); ok {
		if d, ok := derived[field].(map[string]any); ok {
			return d, true
		}
	}
	props, _ := idx.Mappings["properties"].(map[string]any)
	if props == nil {
		return nil, false
	}
	parts := strings.Split(field, ".")
	cur := props
	for i := 0; i < len(parts); i++ {
		// literal dotted key takes precedence
		if i+1 < len(parts) {
			joined := strings.Join(parts[i:], ".")
			if def, ok := cur[joined].(map[string]any); ok {
				return def, true
			}
		}
		def, ok := cur[parts[i]].(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return def, true
		}
		if fields, ok := def["fields"].(map[string]any); ok && i+1 == len(parts)-1 {
			if md, ok := fields[parts[i+1]].(map[string]any); ok {
				return md, true
			}
		}
		sub, ok := def["properties"].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = sub
	}
	return nil, false
}

// flatObjectRoot returns the nearest ancestor mapped as flat_object, if any.
func flatObjectRoot(idx *Index, field string) (string, bool) {
	parts := strings.Split(field, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], ".")
		if def, ok := mappingFieldDef(idx, prefix); ok {
			if t, _ := def["type"].(string); t == "flat_object" {
				return prefix, true
			}
		}
	}
	return "", false
}

func sortFieldMapped(idx *Index, field string) bool {
	if _, ok := mappingFieldDef(idx, field); ok {
		return true
	}
	_, ok := flatObjectRoot(idx, field)
	return ok
}

func (idx *Index) objectPaths() map[string]bool {
	out := map[string]bool{}
	var walk func(prefix string, props map[string]any)
	walk = func(prefix string, props map[string]any) {
		for name, raw := range props {
			def, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			if sub, ok := def["properties"].(map[string]any); ok {
				out[path] = true
				walk(path, sub)
			}
		}
	}
	props, _ := idx.Mappings["properties"].(map[string]any)
	walk("", props)
	return out
}

// ---------------------------------------------------------------------------
// typed comparisons
// ---------------------------------------------------------------------------

func numericEqual(a, b any) bool {
	if ua, ok := a.(uint64); ok {
		if ub, ok := b.(uint64); ok {
			return ua == ub
		}
		if ib, ok := integerOf(b); ok && ib >= 0 {
			return ua == uint64(ib)
		}
		return false
	}
	if ub, ok := b.(uint64); ok {
		if ia, ok := integerOf(a); ok && ia >= 0 {
			return ub == uint64(ia)
		}
	}
	if ia, ok := integerOf(a); ok {
		if ib, ok := integerOf(b); ok {
			return ia == ib
		}
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	return aok && bok && af == bf
}

func integerOf(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case uint64:
		if t <= 1<<63-1 {
			return int64(t), true
		}
		return 0, false
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

func numericLess(a, b any) int {
	if ua, ok := a.(uint64); ok {
		var ub uint64
		if v, ok := b.(uint64); ok {
			ub = v
		} else if ib, ok := integerOf(b); ok && ib >= 0 {
			ub = uint64(ib)
		} else {
			bf, _ := toFloat(b)
			if float64(ua) < bf {
				return -1
			} else if float64(ua) > bf {
				return 1
			}
			return 0
		}
		switch {
		case ua < ub:
			return -1
		case ua > ub:
			return 1
		}
		return 0
	}
	if ub, ok := b.(uint64); ok {
		if ia, ok := integerOf(a); ok {
			if ia < 0 {
				return -1
			}
			switch {
			case uint64(ia) < ub:
				return -1
			case uint64(ia) > ub:
				return 1
			}
			return 0
		}
	}
	if ia, ok := integerOf(a); ok {
		if ib, ok := integerOf(b); ok {
			switch {
			case ia < ib:
				return -1
			case ia > ib:
				return 1
			}
			return 0
		}
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		}
		return 0
	}
	as, _ := toString(a)
	bs, _ := toString(b)
	return strings.Compare(as, bs)
}

func numberKey(v any) string {
	switch t := v.(type) {
	case int64:
		return strconv.FormatInt(t, 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case json.Number:
		return t.String()
	case float64:
		return formatNumber(t)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// parseTermValue converts a term query value into the indexed representation
// for the field type.
// termDateParseError reshapes a date parse failure from a term-level query
// into the parse_exception wrapper OpenSearch reports for it.
func termDateParseError(err *apiError) *apiError {
	inner := err.Reason
	if err.CausedBy != nil && err.CausedBy.Reason != "" {
		inner = err.CausedBy.Reason
	}
	return &apiError{Type: "parse_exception", Status: 400,
		Reason: inner + ": [" + inner + "]",
		CausedBy: &apiError{Type: "illegal_argument_exception", Reason: inner,
			CausedBy: &apiError{Type: "date_time_parse_exception",
				Reason: "Failed to parse with all enclosed parsers"}}}
}

func parseTermValue(ftype string, def map[string]any, v any) ([]any, *apiError) {
	switch ftype {
	case "integer", "long", "short", "byte", "unsigned_long":
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		parsed, err := ctx.parseIntValue("_value", ftype, def, v)
		if err != nil {
			return nil, err
		}
		return []any{parsed}, nil
	case "float", "double", "half_float", "scaled_float":
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		parsed, err := ctx.parseFloatValue("_value", ftype, def, v)
		if err != nil {
			return nil, err
		}
		return []any{parsed}, nil
	case "boolean":
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		parsed, err := ctx.parseBooleanValue("_value", def, v)
		if err != nil {
			return nil, err
		}
		return []any{parsed}, nil
	case "date", "date_nanos":
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		typ := ftype
		parsed, err := ctx.parseDateValue("_value", typ, def, v)
		if err != nil {
			return nil, err
		}
		return []any{parsed}, nil
	case "ip":
		s, isStr := valueScalarString(v)
		if isStr && strings.Contains(s, "/") {
			_, ipnet, err := net.ParseCIDR(s)
			if err != nil {
				ones := 0
				ipPart := s
				if i := strings.LastIndex(s, "/"); i >= 0 {
					ipPart = s[:i]
					ones, _ = strconv.Atoi(s[i+1:])
				}
				if net.ParseIP(ipPart) != nil {
					max := 32
					if strings.Contains(ipPart, ":") {
						max = 128
					}
					if ones < 0 || ones > max {
						return nil, &apiError{Type: "illegal_argument_exception", Status: 400,
							Reason: fmt.Sprintf("Illegal prefix length [%d] in [%s]. Must be 0-32 for IPv4 ranges, 0-128 for IPv6 ranges", ones, s)}
					}
				}
				return nil, &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("'%s' is not an IP string literal.", s)}
			}
			return []any{canonicalIP(ipnet.String())}, nil
		}
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		parsed, err := ctx.parseIPValue("_value", v)
		if err != nil {
			return nil, err
		}
		return []any{parsed}, nil
	case "integer_range", "float_range", "long_range", "double_range", "date_range", "ip_range":
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		base := strings.TrimSuffix(ftype, "_range")
		switch base {
		case "date":
			parsed, err := ctx.parseDateValue("_value", "date", def, v)
			if err != nil {
				return nil, err
			}
			return []any{parsed}, nil
		case "ip":
			parsed, err := ctx.parseIPValue("_value", v)
			if err != nil {
				return nil, err
			}
			return []any{parsed}, nil
		case "integer", "long":
			parsed, err := ctx.parseIntValue("_value", base, def, v)
			if err != nil {
				return nil, err
			}
			return []any{parsed}, nil
		case "float", "double":
			parsed, err := ctx.parseFloatValue("_value", base, def, v)
			if err != nil {
				return nil, err
			}
			return []any{parsed}, nil
		}
		return []any{v}, nil
	default:
		s, ok := valueScalarString(v)
		if !ok {
			s = previewValue(v)
		}
		return []any{s}, nil
	}
}

// ipContains reports whether needle (an IP or CIDR) covers the doc value.
func ipContains(host, needle string) bool {
	if strings.Contains(needle, "/") {
		_, network, err := net.ParseCIDR(needle)
		if err != nil {
			return false
		}
		ip := net.ParseIP(host)
		return ip != nil && network.Contains(ip)
	}
	return host == canonicalIP(needle)
}

func canonicalIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// inRangeField reports whether v lies within a stored range value.
func inRangeField(rng map[string]any, v any) bool {
	lower, hasLower := rng["gte"]
	upper, hasUpper := rng["lte"]
	if !hasLower {
		if val, ok := rng["gt"]; ok {
			lower, hasLower = val, true
			if hasUpper {
				_ = upper
			}
		}
	}
	if !hasUpper {
		if val, ok := rng["lt"]; ok {
			upper, hasUpper = val, true
		}
	}
	if hasLower {
		op := "gte"
		if _, ok := rng["gt"]; ok {
			op = "gt"
		}
		c := compareRangeBound(lower, v)
		if op == "gte" && c > 0 {
			return false
		}
		if op == "gt" && c >= 0 {
			return false
		}
	}
	if hasUpper {
		op := "lte"
		if _, ok := rng["lt"]; ok {
			op = "lt"
		}
		c := compareRangeBound(upper, v)
		if op == "lte" && c < 0 {
			return false
		}
		if op == "lt" && c <= 0 {
			return false
		}
	}
	return true
}

func compareAny(a, b any) int {
	if as, ok := a.(string); ok {
		if bs, ok2 := b.(string); ok2 {
			return strings.Compare(as, bs)
		}
		return strings.Compare(as, previewValue(b))
	}
	return numericLess(a, b)
}

// compareRangeBound compares two range bounds, using numeric IP ordering for
// fields whose bounds are IP literals.
func compareRangeBound(a, b any) int {
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		if ai := net.ParseIP(as); ai != nil {
			if bi := net.ParseIP(bs); bi != nil {
				return strings.Compare(string(ai.To16()), string(bi.To16()))
			}
		}
	}
	return compareAny(a, b)
}

// rangeRelation evaluates the relation between a query range and a stored range.
func rangeRelation(relation string, qLower, qUpper any, hasLower, lowerIncl, hasUpper, upperIncl bool, stored map[string]any) bool {
	sLower, sHasLower := stored["gte"]
	sLowerIncl := sHasLower
	if !sHasLower {
		if v, ok := stored["gt"]; ok {
			sLower, sHasLower, sLowerIncl = v, true, false
		}
	}
	sUpper, sHasUpper := stored["lte"]
	sUpperIncl := sHasUpper
	if !sHasUpper {
		if v, ok := stored["lt"]; ok {
			sUpper, sHasUpper, sUpperIncl = v, true, false
		}
	}
	switch strings.ToUpper(relation) {
	case "CONTAINS":
		return rangeContainsRange(sLower, sLowerIncl, sHasLower, sUpper, sUpperIncl, sHasUpper,
			qLower, lowerIncl, hasLower, qUpper, upperIncl, hasUpper)
	case "WITHIN":
		return rangeContainsRange(qLower, lowerIncl, hasLower, qUpper, upperIncl, hasUpper,
			sLower, sLowerIncl, sHasLower, sUpper, sUpperIncl, sHasUpper)
	}
	if hasLower && sHasUpper {
		c := compareRangeBound(sUpper, qLower)
		if c < 0 || c == 0 && (!sUpperIncl || !lowerIncl) {
			return false
		}
	}
	if hasUpper && sHasLower {
		c := compareRangeBound(qUpper, sLower)
		if c < 0 || c == 0 && (!upperIncl || !sLowerIncl) {
			return false
		}
	}
	return true
}

func rangeContainsRange(outerLower any, outerLowerIncl, outerHasLower bool,
	outerUpper any, outerUpperIncl, outerHasUpper bool,
	innerLower any, innerLowerIncl, innerHasLower bool,
	innerUpper any, innerUpperIncl, innerHasUpper bool) bool {
	if !rangeLowerWithin(outerLower, outerLowerIncl, outerHasLower, innerLower, innerLowerIncl, innerHasLower) {
		return false
	}
	return rangeUpperWithin(outerUpper, outerUpperIncl, outerHasUpper, innerUpper, innerUpperIncl, innerHasUpper)
}

func rangeLowerWithin(outer any, outerIncl, outerHas bool, inner any, innerIncl, innerHas bool) bool {
	if !innerHas {
		return !outerHas
	}
	if !outerHas {
		return true
	}
	c := compareRangeBound(inner, outer)
	if c > 0 {
		return true
	}
	if c < 0 {
		return false
	}
	return !innerIncl || outerIncl
}

func rangeUpperWithin(outer any, outerIncl, outerHas bool, inner any, innerIncl, innerHas bool) bool {
	if !innerHas {
		return !outerHas
	}
	if !outerHas {
		return true
	}
	c := compareRangeBound(inner, outer)
	if c < 0 {
		return true
	}
	if c > 0 {
		return false
	}
	return !innerIncl || outerIncl
}

// ---------------------------------------------------------------------------
// term-level queries
// ---------------------------------------------------------------------------

func (c *queryCtx) evalTerm(field string, raw any, boost float64) (bool, float64, *apiError) {
	if field == "_id" {
		val := raw
		if m, ok := raw.(map[string]any); ok {
			if v, ok := m["value"]; ok {
				val = v
			}
		}
		s, _ := valueScalarString(val)
		if s == c.id {
			return true, boost, nil
		}
		return false, 0, nil
	}
	if def, ok := mappingFieldDef(c.idx, field); ok {
		t, _ := def["type"].(string)
		if t == "rank_feature" || t == "rank_features" {
			return false, 0, matchSearchError("query_shard_exception", "failed to create query: Queries on [rank_feature] fields are not supported")
		}
		if t == "completion" {
			return false, 0, nil
		}
		if t == "binary" {
			return false, 0, matchSearchError("query_shard_exception", "Binary fields do not support searching")
		}
		notIndexed := false
		if iv, ok := def["index"].(bool); ok && !iv {
			notIndexed = true
		}
		noDocValues := false
		if dv, ok := def["doc_values"].(bool); ok && !dv {
			noDocValues = true
		}
		if notIndexed && noDocValues {
			textField := t == "text" || t == "match_only_text" || t == "search_as_you_type"
			reason := "failed to create query: Cannot search on field [" + field + "] since it is both not indexed, and does not have doc_values enabled."
			if textField {
				reason = "failed to create query: Cannot search on field [" + field + "] since it is not indexed."
			}
			return false, 0, matchSearchError("query_shard_exception", reason)
		}
		if notIndexed && (t == "text" || t == "match_only_text" || t == "search_as_you_type") {
			return false, 0, matchSearchError("query_shard_exception",
				"failed to create query: Cannot search on field ["+field+"] since it is not indexed.")
		}
	}
	val := raw
	fuzz := 0
	caseInsensitive := false
	if m, ok := raw.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			val = v
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
		if f, ok := m["fuzziness"]; ok {
			if n, ok2 := fuzzinessValue(f, previewValue(val)); ok2 {
				fuzz = n
			}
		}
		if ci, ok := m["case_insensitive"].(bool); ok {
			caseInsensitive = ci
		}
	}
	if i := strings.Index(field, "#"); i >= 0 {
		if baseDef, ok := mappingFieldDef(c.idx, field[:i]); ok {
			if bt, _ := baseDef["type"].(string); bt == "join" {
				term, _ := valueScalarString(val)
				rel := field[i+1:]
				if c.id == term {
					vals, present := c.values(field[:i])
					if present {
						for _, v := range vals {
							if s, ok := v.(string); ok && s == rel {
								return true, boost, nil
							}
						}
					}
				}
				vals, present := c.values(field[:i])
				if present {
					for _, v := range vals {
						if jm, ok := v.(map[string]any); ok {
							if p, _ := jm["parent"].(string); p == term {
								return true, boost, nil
							}
						}
					}
				}
				return false, 0, nil
			}
		}
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	switch ftype {
	case "":
		term, _ := valueScalarString(val)
		for _, v := range vals {
			if v == nil {
				continue
			}
			vs, _ := valueScalarString(v)
			if vs == term {
				return true, boost, nil
			}
		}
		return false, 0, nil
	case "text", "match_only_text", "search_as_you_type":
		term, ok := valueScalarString(val)
		if !ok {
			return false, 0, errParsing("failed to parse term value")
		}
		if caseInsensitive {
			term = strings.ToLower(term)
		}
		analyzer, _ := def["analyzer"].(string)
		for _, v := range vals {
			s, _ := valueScalarString(v)
			for _, tok := range analyzeTokensWithIndex(c.idx, analyzer, s) {
				t := tok.term
				if caseInsensitive {
					t = strings.ToLower(t)
				}
				if t == term {
					return true, boost, nil
				}
			}
		}
		return false, 0, nil
	case "keyword", "constant_keyword", "wildcard", "flat_object", "binary":
		term, ok := valueScalarString(val)
		if !ok {
			return false, 0, nil
		}
		term = normalizeQueryValue(c.idx, field, term)
		if caseInsensitive {
			term = asciiLower(term)
		}
		if stored, isObj := valsAsObject(vals); isObj {
			_ = stored
		}
		for _, v := range vals {
			s, ok := valueScalarString(v)
			if !ok {
				continue
			}
			if caseInsensitive {
				s = asciiLower(s)
			}
			if s == term {
				return true, boost, nil
			}
		}
		return false, 0, nil
	case "completion":
		return false, 0, &apiError{Type: "search_phase_execution_exception", Status: 400,
			Reason: "failed to create query: Field [completion] of type [completion] is not supported for term queries",
			RootCause: &apiError{Type: "illegal_argument_exception",
				Reason: "Field [completion] of type [completion] is not supported for term queries"}}
	case "token_count", "rank_feature", "rank_features":
		parsed, err := parseTermValue(ftype, def, val)
		if err != nil {
			return false, 0, err
		}
		for _, v := range vals {
			if numericEqual(v, parsed[0]) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	case "geo_point", "geo_shape":
		return false, 0, errParsing("failed to parse term value")
	case "join":
		term, _ := valueScalarString(val)
		for _, v := range vals {
			switch t := v.(type) {
			case string:
				if t == term {
					return true, boost, nil
				}
			case map[string]any:
				if n, _ := t["name"].(string); n == term {
					return true, boost, nil
				}
			}
		}
		return false, 0, nil
	case "ip_range":
		if s, ok := valueScalarString(val); ok && strings.Contains(s, "/") {
			return false, 0, &apiError{Type: "search_phase_execution_exception", Status: 400,
				Reason: fmt.Sprintf("failed to create query: '%s' is not an IP string literal.", s),
				RootCause: &apiError{Type: "illegal_argument_exception",
					Reason: fmt.Sprintf("'%s' is not an IP string literal.", s)}}
		}
		fallthrough
	case "integer_range", "float_range", "long_range", "double_range", "date_range":
		parsed, err := parseTermValue(ftype, def, val)
		if err != nil {
			return false, 0, err
		}
		for _, v := range vals {
			if rng, ok := v.(map[string]any); ok {
				if inRangeField(rng, parsed[0]) {
					return true, boost, nil
				}
			}
		}
		return false, 0, nil
	case "ip":
		term, ok := valueScalarString(val)
		if !ok {
			return false, 0, errParsing("failed to parse ip")
		}
		if strings.Contains(term, "/") {
			if _, _, err := net.ParseCIDR(term); err != nil {
				return false, 0, &apiError{Type: "search_phase_execution_exception", Status: 400,
					Reason: fmt.Sprintf("failed to create query: '%s' is not an IP string literal.", term),
					RootCause: &apiError{Type: "illegal_argument_exception",
						Reason: fmt.Sprintf("'%s' is not an IP string literal.", term)}}
			}
		} else if net.ParseIP(term) == nil {
			return false, 0, &apiError{Type: "search_phase_execution_exception", Status: 400,
				Reason: fmt.Sprintf("failed to create query: '%s' is not an IP string literal.", term),
				RootCause: &apiError{Type: "illegal_argument_exception",
					Reason: fmt.Sprintf("'%s' is not an IP string literal.", term)}}
		}
		for _, v := range vals {
			s, _ := toString(v)
			if ipContains(s, term) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	case "boolean":
		if _, isBool := val.(bool); !isBool {
			s, isStr := valueScalarString(val)
			if !isStr || (s != "true" && s != "false") {
				reason := fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", previewValue(val))
				return false, 0, &apiError{Type: "mapper_parsing_exception", Status: 400,
					Reason:   reason,
					CausedBy: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
			}
		}
		parsed, err := parseTermValue(ftype, def, val)
		if err != nil {
			return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: fmt.Sprintf("failed to parse value [%s] as a boolean", previewValue(val))}
		}
		for _, v := range vals {
			if b, ok := v.(bool); ok && b == parsed[0].(bool) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	case "date", "date_nanos":
		parsed, err := parseTermValue(ftype, def, val)
		if err != nil {
			return false, 0, termDateParseError(err)
		}
		dayOnly := false
		if s, ok := valueScalarString(val); ok && len(s) == 10 && s[4] == '-' && s[7] == '-' {
			dayOnly = true
		}
		for _, v := range vals {
			for _, p := range parsed {
				if numericEqual(v, p) {
					return true, boost, nil
				}
				if dayOnly {
					if vm, ok := toFloat(v); ok {
						if pm, ok := toFloat(p); ok && vm >= pm && vm < pm+86400000 {
							return true, boost, nil
						}
					}
				}
			}
		}
		return false, 0, nil
	default:
		if isIntegerKind(ftype) && hasFractionalNumericValue(val) {
			return false, 0, nil
		}
		parsed, err := parseTermValue(ftype, def, val)
		if err != nil {
			return false, 0, err
		}
		for _, v := range vals {
			for _, p := range parsed {
				if numericEqual(v, p) {
					return true, boost, nil
				}
				if fuzz > 0 {
					vs, _ := valueScalarString(v)
					ps, _ := valueScalarString(p)
					if levenshtein(vs, ps) <= fuzz {
						return true, boost, nil
					}
				}
			}
		}
		return false, 0, nil
	}
}

func valsAsObject(vals []any) (map[string]any, bool) {
	return nil, false
}

func fuzzinessValue(v any, term string) (int, bool) {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "AUTO") {
			if t == "AUTO" {
				return autoFuzziness(term), true
			}
			if strings.HasPrefix(t, "AUTO:") {
				parts := strings.Split(strings.TrimPrefix(t, "AUTO:"), ",")
				low, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				high := 0
				if len(parts) > 1 {
					high, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
				}
				n := len([]rune(term))
				switch {
				case n < low:
					return 0, true
				case n >= high:
					return 2, true
				default:
					return 1, true
				}
			}
		}
	}
	if n, ok := toFloat(v); ok {
		return int(n), true
	}
	return 0, false
}

func (c *queryCtx) evalTerms(field string, raw map[string]any, boost float64) (bool, float64, *apiError) {
	listRaw, hasList := raw[field]
	if !hasList {
		return false, 0, errParsing("[" + field + "] query requires a field term to contain a value")
	}
	if b, ok := toFloat(raw["boost"]); ok {
		boost *= b
	}
	var list []any
	if lookup, isLookup := listRaw.(map[string]any); isLookup {
		resolved, err := resolveTermsLookup(lookup)
		if err != nil {
			return false, 0, err
		}
		list = resolved
	} else {
		var ok bool
		list, ok = listRaw.([]any)
		if !ok {
			return false, 0, errParsing("[terms] query does not support [" + previewValue(listRaw) + "] as field value, must be an array")
		}
	}
	maxTerms := 65536
	if v, ok := findSetting(c.idx.Settings, "max_terms_count"); ok {
		if n, err := strconv.Atoi(settingsValueString(v)); err == nil {
			maxTerms = n
		}
	}
	if len(list) > maxTerms {
		reason := fmt.Sprintf("The number of terms [%d] used in the Terms Query request has exceeded the allowed maximum of [%d]. This maximum can be set by changing the [index.max_terms_count] index level setting.", len(list), maxTerms)
		return false, 0, &apiError{Type: "query_shard_exception", Status: 400, Reason: reason,
			CausedBy: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, item := range list {
		var ok2 bool
		if _, isObj := item.(map[string]any); isObj && ftype != "range" {
			return false, 0, errParsing("[terms] query does not support map values")
		}
		switch ftype {
		case "text", "match_only_text", "search_as_you_type":
			term, _ := valueScalarString(item)
			for _, v := range vals {
				s, _ := valueScalarString(v)
				for _, tok := range analyze(s) {
					if tok == term {
						ok2 = true
						break
					}
				}
				if ok2 {
					break
				}
			}
		case "keyword", "constant_keyword", "wildcard", "flat_object", "binary", "":
			term, _ := valueScalarString(item)
			term = normalizeQueryValue(c.idx, field, term)
			for _, v := range vals {
				if s, _ := valueScalarString(v); s == term {
					ok2 = true
					break
				}
			}
		default:
			if isIntegerKind(ftype) && hasFractionalNumericValue(item) {
				continue
			}
			parsed, err := parseTermValue(ftype, def, item)
			if err != nil {
				return false, 0, err
			}
			for _, v := range vals {
				rng, isRange := v.(map[string]any)
				if isRange {
					if inRangeField(rng, parsed[0]) {
						ok2 = true
						break
					}
					continue
				}
				if s, isStr := v.(string); isStr && ftype == "ip" {
					ps, _ := valueScalarString(parsed[0])
					if ipContains(s, ps) {
						ok2 = true
						break
					}
					continue
				}
				if numericEqual(v, parsed[0]) {
					ok2 = true
					break
				}
			}
		}
		if ok2 {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

func collectPathValues(cur any, parts []string) []any {
	if len(parts) == 0 {
		if arr, ok := cur.([]any); ok {
			return arr
		}
		if cur == nil {
			return nil
		}
		return []any{cur}
	}
	switch t := cur.(type) {
	case map[string]any:
		return collectPathValues(t[parts[0]], parts[1:])
	case []any:
		var out []any
		for _, e := range t {
			out = append(out, collectPathValues(e, parts)...)
		}
		return out
	}
	return nil
}

func resolveTermsLookup(spec map[string]any) ([]any, *apiError) {
	index, _ := spec["index"].(string)
	id, _ := spec["id"].(string)
	path, _ := spec["path"].(string)
	if index == "" || id == "" || path == "" {
		return nil, errParsing("[terms] query lookup requires index, id and path")
	}
	if globalServer == nil {
		return nil, nil
	}
	// Callers already hold globalServer.mu (read or write); taking it again
	// here deadlocks once a writer is queued.
	idx, ok := globalServer.indices[index]
	if !ok {
		return nil, &apiError{Type: "index_not_found_exception", Status: 404,
			Reason: "no such index [" + index + "]",
			Fields: map[string]any{
				"index":         index,
				"resource.id":   index,
				"resource.type": "index_expression",
				"index_uuid":    "_na_",
			}}
	}
	doc, ok := idx.Docs[id]
	if !ok {
		return nil, nil
	}
	return collectPathValues(doc, strings.Split(path, ".")), nil
}

func (c *queryCtx) evalIds(raw map[string]any, boost float64) (bool, float64, *apiError) {
	if b, ok := toFloat(raw["boost"]); ok {
		boost *= b
	}
	rawVals, ok := raw["values"]
	if !ok {
		return false, 0, errParsing("[ids] requires a 'values' element")
	}
	var list []any
	switch t := rawVals.(type) {
	case []any:
		list = t
	case string:
		list = []any{t}
	case json.Number:
		list = []any{t.String()}
	default:
		return false, 0, errParsing("[ids] 'values' must be an array")
	}
	for _, item := range list {
		s, _ := valueScalarString(item)
		if s == c.id {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

// ---------------------------------------------------------------------------
// range query
// ---------------------------------------------------------------------------

type rangeBoundSpec struct {
	value   any
	present bool
	incl    bool
}

func rangeQueryBounds(cond map[string]any) (rangeBoundSpec, rangeBoundSpec) {
	var lower, upper rangeBoundSpec
	if v, ok := cond["gte"]; ok && v != nil {
		lower = rangeBoundSpec{value: v, present: true, incl: true}
	} else if v, ok := cond["gt"]; ok && v != nil {
		lower = rangeBoundSpec{value: v, present: true, incl: false}
	} else if v, ok := cond["from"]; ok && v != nil {
		incl := true
		if b, ok := cond["include_lower"].(bool); ok {
			incl = b
		}
		lower = rangeBoundSpec{value: v, present: true, incl: incl}
	}
	if v, ok := cond["lte"]; ok && v != nil {
		upper = rangeBoundSpec{value: v, present: true, incl: true}
	} else if v, ok := cond["lt"]; ok && v != nil {
		upper = rangeBoundSpec{value: v, present: true, incl: false}
	} else if v, ok := cond["to"]; ok && v != nil {
		incl := true
		if b, ok := cond["include_upper"].(bool); ok {
			incl = b
		}
		upper = rangeBoundSpec{value: v, present: true, incl: incl}
	}
	return lower, upper
}

func isRangeFieldType(ftype string) bool {
	switch ftype {
	case "integer_range", "long_range", "float_range", "double_range", "date_range", "ip_range":
		return true
	}
	return false
}

func isTextRangeType(ftype string) bool {
	switch ftype {
	case "text", "match_only_text", "search_as_you_type":
		return true
	}
	return false
}

func isFractionalNumber(v any) bool {
	switch t := v.(type) {
	case json.Number:
		return strings.ContainsAny(t.String(), ".eE")
	case float64:
		return t != math.Trunc(t)
	}
	return false
}

// rawFloatValue parses a JSON number or numeric string without applying any
// field-type rounding.
func rawFloatValue(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case uint64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func hasFractionalNumericValue(v any) bool {
	f, ok := toFloat(v)
	return ok && f != math.Trunc(f)
}

func rangeBoundValue(def map[string]any, ftype, field string, v any, tz, format string, roundUp bool) (any, *apiError) {
	switch ftype {
	case "date", "date_nanos", "date_range":
		return parseRangeDateBound(def, v, tz, format, roundUp)
	case "ip", "ip_range":
		ctx := &indexCtx{indexCoerce: true}
		if _, isRange := v.(map[string]any); isRange {
			return v, nil
		}
		return ctx.parseIPValue(field, v)
	case "integer", "long", "short", "byte", "unsigned_long", "integer_range", "long_range",
		"float", "double", "half_float", "scaled_float", "float_range", "double_range", "token_count":
		base := strings.TrimSuffix(ftype, "_range")
		ctx := &indexCtx{indexCoerce: true}
		if isIntegerKind(base) {
			if isFractionalNumber(v) {
				return ctx.parseFloatValue(field, base, def, v)
			}
			return ctx.parseIntValue(field, base, def, v)
		}
		if base == "half_float" {
			// OpenSearch keeps half_float range bounds in full precision: only
			// the stored value is rounded to half precision.
			if f, ok := rawFloatValue(v); ok {
				return f, nil
			}
		}
		return ctx.parseFloatValue(field, base, def, v)
	}
	return v, nil
}

func rangeCompareValue(ftype string, candidate, bound any) int {
	switch {
	case ftype == "ip":
		cs, _ := toString(candidate)
		bs, _ := toString(bound)
		return compareIPLiteral(cs, bs)
	case isTextRangeType(ftype), ftype == "keyword", ftype == "constant_keyword", ftype == "wildcard", ftype == "":
		cs, _ := toString(candidate)
		bs, _ := toString(bound)
		return strings.Compare(cs, bs)
	}
	return numericLess(candidate, bound)
}

func (c *queryCtx) evalRange(field string, cond map[string]any, boost float64) (bool, float64, *apiError) {
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if ftype == "wildcard" {
		reason := "failed to create query: TODO"
		return false, 0, &apiError{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
			RootCause: &apiError{Type: "query_shard_exception", Reason: reason},
			CausedBy: &apiError{Type: "query_shard_exception", Reason: reason,
				CausedBy: &apiError{Type: "unsupported_operation_exception", Reason: "TODO"}}}
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	relation, _ := cond["relation"].(string)
	timeZone, _ := cond["time_zone"].(string)
	format, _ := cond["format"].(string)
	if b, ok := toFloat(cond["boost"]); ok {
		boost *= b
	}
	lower, upper := rangeQueryBounds(cond)
	var qLower, qUpper any
	if lower.present {
		v, err := rangeBoundValue(def, ftype, field, lower.value, timeZone, format, !lower.incl)
		if err != nil {
			return false, 0, err
		}
		qLower = v
	}
	if upper.present {
		v, err := rangeBoundValue(def, ftype, field, upper.value, timeZone, format, upper.incl)
		if err != nil {
			return false, 0, err
		}
		qUpper = v
	}
	switch ftype {
	case "keyword", "constant_keyword", "wildcard":
		if lower.present {
			if s, ok := valueScalarString(qLower); ok {
				qLower = normalizeQueryValue(c.idx, field, s)
			}
		}
		if upper.present {
			if s, ok := valueScalarString(qUpper); ok {
				qUpper = normalizeQueryValue(c.idx, field, s)
			}
		}
	}
	if isRangeFieldType(ftype) {
		for _, v := range vals {
			rng, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if rangeRelation(relation, qLower, qUpper, lower.present, lower.incl,
				upper.present, upper.incl, rng) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	check := func(candidate any) bool {
		if lower.present {
			cmp := rangeCompareValue(ftype, candidate, qLower)
			if lower.incl && cmp < 0 {
				return false
			}
			if !lower.incl && cmp <= 0 {
				return false
			}
		}
		if upper.present {
			cmp := rangeCompareValue(ftype, candidate, qUpper)
			if upper.incl && cmp > 0 {
				return false
			}
			if !upper.incl && cmp >= 0 {
				return false
			}
		}
		return true
	}
	for _, v := range vals {
		if v == nil {
			continue
		}
		if isTextRangeType(ftype) {
			s, _ := valueScalarString(v)
			for _, tok := range analyze(s) {
				if check(tok) {
					return true, boost, nil
				}
			}
			continue
		}
		if check(v) {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

func compareIPLiteral(a, b string) int {
	if strings.Contains(b, "/") {
		if ipContains(a, b) {
			return 0
		}
		return 1
	}
	ai := net.ParseIP(a)
	bi := net.ParseIP(b)
	if ai == nil || bi == nil {
		return strings.Compare(a, b)
	}
	ab := ai.To16()
	bb := bi.To16()
	return strings.Compare(string(ab), string(bb))
}

func parseQueryDate(def map[string]any, v any, tz string) (any, *apiError) {
	ctx := &indexCtx{indexCoerce: true}
	typ := "date"
	if t, _ := def["type"].(string); t == "date_nanos" {
		typ = "date_nanos"
	}
	scalar := v
	if m, ok := v.(map[string]any); ok {
		if mv, ok := m["value"]; ok {
			scalar = mv
		}
	}
	if s, ok := scalar.(string); ok {
		dateMath := s
		if i := strings.Index(s, "||"); i >= 0 {
			s = s[:i]
		}
		if abs, ok := parseDateAny(def, s); ok {
			if tz != "" {
				if offset, err := parseTZOffset(tz); err == nil {
					abs -= offset
				}
			}
			return abs, nil
		}
		_ = dateMath
	}
	parsed, err := ctx.parseDateValue("_value", typ, def, scalar)
	if err != nil {
		return nil, &apiError{Type: "search_phase_execution_exception", Status: 400,
			Reason: "failed to parse date field [" + previewValue(scalar) + "]", RootCause: err}
	}
	return parsed, nil
}

func parseTZOffset(tz string) (int64, error) {
	tz = strings.TrimSpace(tz)
	if tz == "UTC" || tz == "utc" || tz == "Z" || tz == "" {
		return 0, nil
	}
	if strings.HasPrefix(tz, "+") || strings.HasPrefix(tz, "-") {
		sign := int64(1)
		if tz[0] == '-' {
			sign = -1
		}
		body := tz[1:]
		parts := strings.Split(body, ":")
		hours, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, err
		}
		var mins int64
		if len(parts) > 1 {
			mins, _ = strconv.ParseInt(parts[1], 10, 64)
		}
		return sign * (hours*3600 + mins*60) * 1000, nil
	}
	loc, err := timeLoadLocation(tz)
	if err != nil {
		return 0, err
	}
	_, off := timeNowIn(loc)
	return int64(off) * 1000, nil
}

// ---------------------------------------------------------------------------
// exists / prefix / wildcard / regexp / fuzzy
// ---------------------------------------------------------------------------

func (c *queryCtx) evalExists(raw map[string]any) (bool, float64, *apiError) {
	field, _ := toString(raw["field"])
	if field == "" {
		return false, 0, errParsing("[exists] query requires 'field'")
	}
	if derived, ok := c.idx.Mappings["derived"].(map[string]any); ok {
		if _, isDerived := derived[field]; isDerived {
			reason := "Field [" + field + "] of type [derived] does not support exist queries"
			return false, 0, &apiError{Type: "query_shard_exception", Status: 400,
				Reason:   "failed to create query: " + reason,
				CausedBy: &apiError{Type: "unsupported_operation_exception", Status: 400, Reason: reason}}
		}
	}
	boost := 1.0
	if b, ok := toFloat(raw["boost"]); ok {
		boost = b
	}
	if strings.ContainsAny(field, "*?") {
		re := wildcardRegex(field)
		for path, vals := range c.idx.DocValues[c.id] {
			if re.MatchString(path) && len(vals) > 0 {
				return true, boost, nil
			}
		}
		for path := range c.idx.objectPaths() {
			if re.MatchString(path) && c.hasField(path) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	if c.hasField(field) {
		return true, boost, nil
	}
	return false, 0, nil
}

func multitermFieldSupported(ftype string) bool {
	switch ftype {
	case "", "keyword", "constant_keyword", "wildcard", "text", "match_only_text",
		"search_as_you_type", "flat_object":
		return true
	}
	return false
}

func multitermFieldError(kind, field, ftype string) *apiError {
	return &apiError{Type: "query_shard_exception", Status: 400,
		Reason: fmt.Sprintf("Can only use %s queries on keyword and text fields - not on [%s] which is of type [%s]",
			kind, field, ftype)}
}

func (c *queryCtx) evalPrefix(field string, raw any, boost float64) (bool, float64, *apiError) {
	val := raw
	caseInsensitive := false
	if m, ok := raw.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			val = v
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
		if ci, ok := m["case_insensitive"].(bool); ok {
			caseInsensitive = ci
		}
	}
	prefix, ok := valueScalarString(val)
	if !ok {
		return false, 0, nil
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if !multitermFieldSupported(ftype) {
		return false, 0, multitermFieldError("prefix", field, ftype)
	}
	prefix = normalizeQueryValue(c.idx, field, prefix)
	if caseInsensitive {
		prefix = asciiLower(prefix)
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		s, _ := valueScalarString(v)
		if ftype == "text" || ftype == "match_only_text" || ftype == "search_as_you_type" {
			for _, tok := range analyze(s) {
				if caseInsensitive {
					tok = asciiLower(tok)
				}
				if strings.HasPrefix(tok, prefix) {
					return true, boost, nil
				}
			}
		} else {
			if caseInsensitive {
				s = asciiLower(s)
			}
			if strings.HasPrefix(s, prefix) {
				return true, boost, nil
			}
		}
	}
	return false, 0, nil
}

func (c *queryCtx) evalWildcard(field string, raw any, boost float64) (bool, float64, *apiError) {
	val := raw
	caseInsensitive := false
	if m, ok := raw.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			val = v
		} else if v, ok := m["wildcard"]; ok {
			val = v
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
		if ci, ok := m["case_insensitive"].(bool); ok {
			caseInsensitive = ci
		}
	}
	pattern, ok := valueScalarString(val)
	if !ok {
		return false, 0, nil
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if !multitermFieldSupported(ftype) {
		return false, 0, multitermFieldError("wildcard", field, ftype)
	}
	pattern = normalizeQueryValue(c.idx, field, pattern)
	if caseInsensitive {
		pattern = asciiLower(pattern)
	}
	re := wildcardRegex(pattern)
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		s, _ := valueScalarString(v)
		candidates := []string{s}
		if ftype == "text" || ftype == "match_only_text" || ftype == "search_as_you_type" {
			candidates = analyze(s)
		}
		for _, cand := range candidates {
			if caseInsensitive {
				cand = asciiLower(cand)
			}
			if re.MatchString(cand) {
				return true, boost, nil
			}
		}
	}
	return false, 0, nil
}

type regexpMatcher struct {
	parts      []*regexp.Regexp
	complement bool
	empty      bool
}

func (m *regexpMatcher) matches(s string) bool {
	if m.empty {
		return false
	}
	ok := true
	for _, re := range m.parts {
		if !re.MatchString(s) {
			ok = false
			break
		}
	}
	if m.complement {
		return !ok
	}
	return ok
}

func splitRegexpParts(pattern string, intersection bool) []string {
	if !intersection {
		return []string{pattern}
	}
	var parts []string
	var cur strings.Builder
	escaped := false
	for _, ch := range pattern {
		if escaped {
			cur.WriteRune('\\')
			cur.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '&' {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(ch)
	}
	parts = append(parts, cur.String())
	return parts
}

func luceneIntervalToRegexp(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '<' {
			out.WriteByte(pattern[i])
			continue
		}
		j := i + 1
		for j < len(pattern) && pattern[j] != '>' {
			j++
		}
		if j >= len(pattern) {
			out.WriteString("<")
			continue
		}
		body := pattern[i+1 : j]
		lo, hi := 0, 0
		var err error
		if k := strings.Index(body, "-"); k >= 0 {
			lo, err = strconv.Atoi(body[:k])
			if err != nil {
				out.WriteString(pattern[i : j+1])
				i = j
				continue
			}
			hi, err = strconv.Atoi(body[k+1:])
			if err != nil {
				out.WriteString(pattern[i : j+1])
				i = j
				continue
			}
		} else {
			lo, err = strconv.Atoi(body)
			if err != nil {
				out.WriteString(pattern[i : j+1])
				i = j
				continue
			}
			hi = lo
		}
		if lo < 0 || hi < lo || hi-lo > 1000 {
			out.WriteString(pattern[i : j+1])
			i = j
			continue
		}
		out.WriteString("(?:")
		for n := lo; n <= hi; n++ {
			if n > lo {
				out.WriteString("|")
			}
			out.WriteString(strconv.Itoa(n))
		}
		out.WriteString(")")
		i = j
	}
	return out.String()
}

func buildRegexpMatcher(pattern, flags string, caseInsensitive bool) (*regexpMatcher, error) {
	upper := strings.ToUpper(flags)
	has := func(name string) bool {
		for _, part := range strings.FieldsFunc(upper, func(r rune) bool { return r == ',' || r == '|' }) {
			if strings.TrimSpace(part) == name {
				return true
			}
		}
		return false
	}
	all := has("ALL")
	none := has("NONE")
	intersection := !none && (all || has("INTERSECTION"))
	anystring := !none && (all || has("ANYSTRING"))
	interval := !none && (all || has("INTERVAL"))
	complementEnabled := has("COMPLEMENT")
	m := &regexpMatcher{}
	if strings.HasPrefix(pattern, "~") && complementEnabled {
		m.complement = true
		pattern = pattern[1:]
	}
	for _, part := range splitRegexpParts(pattern, intersection) {
		if anystring {
			part = strings.ReplaceAll(part, "@", ".*")
		}
		if interval {
			part = luceneIntervalToRegexp(part)
		}
		whole := "^(?:" + part + ")$"
		if caseInsensitive {
			whole = "(?i)" + whole
		}
		re, err := regexp.Compile(whole)
		if err != nil {
			return nil, err
		}
		m.parts = append(m.parts, re)
	}
	return m, nil
}

func regexpStateEstimate(pattern string) int {
	states := 1
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '{' {
			j := i + 1
			for j < len(pattern) && pattern[j] != '}' {
				j++
			}
			if j < len(pattern) {
				body := pattern[i+1 : j]
				if k := strings.LastIndex(body, ","); k >= 0 {
					if n, err := strconv.Atoi(strings.TrimSpace(body[k+1:])); err == nil && n > 0 {
						states += n
						i = j
						continue
					}
				} else if n, err := strconv.Atoi(strings.TrimSpace(body)); err == nil && n > 0 {
					states += n
					i = j
					continue
				}
			}
		}
		states++
	}
	return states
}

func (c *queryCtx) evalRegexp(field string, raw any, boost float64) (bool, float64, *apiError) {
	val := raw
	flags := ""
	caseInsensitive := false
	maxStates := 10000
	if m, ok := raw.(map[string]any); ok {
		if v, ok := m["value"]; ok {
			val = v
		} else if v, ok := m["regexp"]; ok {
			val = v
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
		if f, ok := m["flags"].(string); ok {
			flags = f
		}
		if ci, ok := m["case_insensitive"].(bool); ok {
			caseInsensitive = ci
		}
		if states, ok := toFloat(m["max_determinized_states"]); ok {
			maxStates = int(states)
		}
	}
	pattern, ok := valueScalarString(val)
	if !ok {
		return false, 0, nil
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if !multitermFieldSupported(ftype) {
		return false, 0, multitermFieldError("regexp", field, ftype)
	}
	if estimate := regexpStateEstimate(pattern); estimate > maxStates {
		reason := fmt.Sprintf("Determinizing automaton with %d states and %d transitions would require more than %d effort.",
			estimate, estimate*2, maxStates)
		return false, 0, &apiError{Type: "query_shard_exception", Status: 400, Reason: reason,
			CausedBy: &apiError{Type: "too_complex_to_determinize_exception", Status: 400, Reason: reason}}
	}
	matcher, err := buildRegexpMatcher(pattern, flags, caseInsensitive)
	if err != nil {
		msg := "unexpected end-of-string"
		return false, 0, &apiError{Type: "query_shard_exception", Status: 400, Reason: msg,
			CausedBy: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: msg}}
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		s, _ := valueScalarString(v)
		candidates := []string{s}
		if ftype == "text" || ftype == "match_only_text" {
			candidates = analyze(s)
		}
		for _, cand := range candidates {
			if matcher.matches(cand) {
				return true, boost, nil
			}
		}
	}
	return false, 0, nil
}

func (c *queryCtx) evalFuzzy(field string, raw any, boost float64) (bool, float64, *apiError) {
	m, ok := raw.(map[string]any)
	if !ok {
		m = map[string]any{"value": raw}
	}
	val, has := m["value"]
	if !has {
		return false, 0, errParsing("[fuzzy] query requires 'value'")
	}
	if b, ok := toFloat(m["boost"]); ok {
		boost *= b
	}
	term, _ := valueScalarString(val)
	fuzz := autoFuzziness(term)
	if f, ok := m["fuzziness"]; ok {
		if n, ok2 := fuzzinessValue(f, term); ok2 {
			fuzz = n
		}
	}
	prefixLength := 0
	if pl, ok := toFloat(m["prefix_length"]); ok {
		prefixLength = int(pl)
	}
	transpositions := true
	if b, ok := m["transpositions"].(bool); ok {
		transpositions = b
	}
	maxExpansions := 0
	if me, ok := toFloat(m["max_expansions"]); ok {
		maxExpansions = int(me)
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if !multitermFieldSupported(ftype) || ftype == "wildcard" {
		reason := fmt.Sprintf("Can only use fuzzy queries on keyword and text fields - not on [%s] which is of type [%s]",
			field, ftype)
		return false, 0, matchSearchError("query_shard_exception", "failed to create query: "+reason)
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	var expansionSet map[string]bool
	if maxExpansions > 0 {
		resolved := resolveAliasField(c.idx, field)
		termSet := map[string]bool{}
		for _, dv := range c.idx.DocValues {
			for _, v := range dv[resolved] {
				s, ok := valueScalarString(v)
				if !ok {
					continue
				}
				if ftype == "text" || ftype == "match_only_text" {
					for _, tok := range analyze(s) {
						termSet[tok] = true
					}
				} else {
					termSet[s] = true
				}
			}
		}
		terms := make([]string, 0, len(termSet))
		for t := range termSet {
			terms = append(terms, t)
		}
		sort.Strings(terms)
		expansionSet = map[string]bool{}
		for _, cand := range terms {
			if cand != term {
				if len(cand) <= prefixLength || len(term) <= prefixLength {
					continue
				}
				if cand[:prefixLength] != term[:prefixLength] {
					continue
				}
				if fuzzyDistance(cand, term, transpositions) > fuzz {
					continue
				}
			}
			expansionSet[cand] = true
			if len(expansionSet) >= maxExpansions {
				break
			}
		}
	}
	for _, v := range vals {
		s, _ := valueScalarString(v)
		candidates := []string{s}
		if ftype == "text" || ftype == "match_only_text" {
			candidates = analyze(s)
		}
		for _, cand := range candidates {
			if expansionSet != nil {
				if !expansionSet[cand] {
					continue
				}
				if cand == term {
					return true, boost * 2, nil
				}
				return true, boost, nil
			}
			if cand == term {
				return true, boost * 2, nil
			}
			if len(cand) <= prefixLength || len(term) <= prefixLength {
				continue
			}
			if cand[:prefixLength] != term[:prefixLength] {
				continue
			}
			if fuzzyDistance(cand, term, transpositions) <= fuzz {
				return true, boost, nil
			}
		}
	}
	return false, 0, nil
}

// ---------------------------------------------------------------------------
// full text
// ---------------------------------------------------------------------------

func (c *queryCtx) fieldTokens(field string) ([]string, bool) {
	vals, present := c.values(field)
	if !present {
		return nil, false
	}
	out := []string{}
	for _, v := range vals {
		s, ok := valueScalarString(v)
		if !ok {
			continue
		}
		out = append(out, analyze(s)...)
	}
	return out, true
}

func (c *queryCtx) fieldStrings(field string) ([]string, bool) {
	vals, present := c.values(field)
	if !present {
		return nil, false
	}
	out := []string{}
	for _, v := range vals {
		s, ok := valueScalarString(v)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, true
}

// analyzerName returns the search analyzer of field for a query.
func (c *queryCtx) analyzerName(field string, opts any) string {
	return c.searchAnalyzerName(field, opts)
}

// analyzeDocField analyzes a document value of field with its index analyzer.
func (c *queryCtx) analyzeDocField(field, text string) []string {
	return analyzeWithIndex(c.idx, c.indexAnalyzerName(field), text)
}

// indexAnalyzerName is the analyzer documents are indexed with.
func (c *queryCtx) indexAnalyzerName(field string) string {
	if c.idx == nil {
		return ""
	}
	if def, ok := mappingFieldDef(c.idx, field); ok {
		if a, ok := def["analyzer"].(string); ok && a != "" {
			return a
		}
	}
	if (analysisScope{idx: c.idx}).indexHasAnalyzer("default") {
		return "default"
	}
	return ""
}

// searchAnalyzerName resolves the query analyzer like TextFieldMapper: the
// query's analyzer, search_analyzer, analyzer, then the index defaults.
func (c *queryCtx) searchAnalyzerName(field string, opts any) string {
	if m, ok := opts.(map[string]any); ok {
		if a, ok := m["analyzer"].(string); ok && a != "" {
			return a
		}
	}
	if c.idx == nil {
		return ""
	}
	if def, ok := mappingFieldDef(c.idx, field); ok {
		if a, ok := def["search_analyzer"].(string); ok && a != "" {
			return a
		}
		if a, ok := def["analyzer"].(string); ok && a != "" {
			return a
		}
	}
	sc := analysisScope{idx: c.idx}
	if sc.indexHasAnalyzer("default_search") {
		return "default_search"
	}
	if sc.indexHasAnalyzer("default") {
		return "default"
	}
	return ""
}

// quoteAnalyzerName is the analyzer of phrase queries (search_quote_analyzer).
func (c *queryCtx) quoteAnalyzerName(field string, opts any) string {
	if m, ok := opts.(map[string]any); ok {
		if a, ok := m["analyzer"].(string); ok && a != "" {
			return a
		}
	}
	if c.idx != nil {
		if def, ok := mappingFieldDef(c.idx, field); ok {
			if a, ok := def["search_quote_analyzer"].(string); ok && a != "" {
				return a
			}
		}
	}
	if opts == nil {
		opts = ""
	}
	return c.searchAnalyzerName(field, opts)
}

// setPhraseAnalyzer sets the analyzer of every match_phrase in a query.
func setPhraseAnalyzer(q any, analyzer string) {
	switch t := q.(type) {
	case map[string]any:
		for k, v := range t {
			if k == "match_phrase" {
				if fm, ok := v.(map[string]any); ok {
					for f, fv := range fm {
						switch x := fv.(type) {
						case string:
							fm[f] = map[string]any{"query": x, "analyzer": analyzer}
						case map[string]any:
							if _, has := x["analyzer"]; !has {
								x["analyzer"] = analyzer
							}
						}
					}
				}
				continue
			}
			setPhraseAnalyzer(v, analyzer)
		}
	case []any:
		for _, e := range t {
			setPhraseAnalyzer(e, analyzer)
		}
	}
}

func (c *queryCtx) analyzeField(field string, opts any, text string) []string {
	a := c.analyzerName(field, opts)
	return analyzeWithIndex(c.idx, a, text)
}

func matchShardError(reason string) *apiError {
	return &apiError{Type: "query_shard_exception", Status: 400, Reason: reason}
}

func matchSearchError(rootType, reason string) *apiError {
	return &apiError{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
		RootCause: &apiError{Type: rootType, Status: 400, Reason: reason},
		CausedBy:  &apiError{Type: rootType, Status: 400, Reason: reason}}
}

func parseFuzzinessValue(v any, term string) (int, bool) {
	switch t := v.(type) {
	case string:
		u := strings.ToUpper(strings.TrimSpace(t))
		if u == "AUTO" {
			return autoFuzziness(term), true
		}
		if strings.HasPrefix(u, "AUTO:") {
			parts := strings.Split(strings.TrimPrefix(u, "AUTO:"), ",")
			low, high := 3, 6
			if len(parts) > 0 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
					low = n
				}
			}
			if len(parts) > 1 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					high = n
				}
			}
			n := len([]rune(term))
			if n < low {
				return 0, true
			}
			if n <= high {
				return 1, true
			}
			return 2, true
		}
		if n, err := strconv.Atoi(u); err == nil {
			if n > 2 {
				n = 2
			}
			return n, true
		}
		return 0, false
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			f, ferr := t.Float64()
			if ferr != nil {
				return 0, false
			}
			n = int64(f)
		}
		if n > 2 {
			n = 2
		}
		return int(n), true
	case float64:
		n := int(t)
		if n > 2 {
			n = 2
		}
		return n, true
	case int:
		if t > 2 {
			t = 2
		}
		return t, true
	}
	return 0, false
}

func fuzzyDistance(a, b string, transpositions bool) int {
	ar, br := []rune(a), []rune(b)
	if !transpositions {
		return levenshtein(a, b)
	}
	prev2 := make([]int, len(br)+1)
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				if v := prev2[j-2] + 1; v < curr[j] {
					curr[j] = v
				}
			}
		}
		prev2, prev, curr = prev, curr, prev2
	}
	return prev[len(br)]
}

func msmRequired(spec any, n int, defaultAll bool) (int, *apiError) {
	def := 1
	if defaultAll {
		def = n
	}
	if spec == nil {
		return def, nil
	}
	if s, ok := spec.(string); ok {
		text := strings.TrimSpace(s)
		if strings.Contains(text, "<") {
			for _, rule := range strings.Fields(text) {
				if i := strings.Index(rule, "<"); i > 0 {
					cond, err := strconv.Atoi(rule[:i])
					if err == nil && n > cond {
						return msmRequired(rule[i+1:], n, defaultAll)
					}
				}
			}
			return n, nil
		}
		if strings.HasSuffix(text, "%") {
			p, err := strconv.Atoi(strings.TrimSuffix(text, "%"))
			if err != nil {
				return 0, matchSearchError("query_shard_exception", "failed to create query: For input string: \""+text+"\"")
			}
			if p >= 0 {
				req := int(math.Floor(float64(n) * float64(p) / 100.0))
				if req < 1 {
					req = 1
				}
				return req, nil
			}
			req := n - int(math.Floor(float64(n)*float64(-p)/100.0))
			if req < 1 {
				req = 1
			}
			return req, nil
		}
		if k, err := strconv.Atoi(text); err == nil {
			if k < 0 {
				k = n + k
				if k < 1 {
					k = 1
				}
			}
			return k, nil
		}
		return 0, matchSearchError("query_shard_exception", "failed to create query: For input string: \""+text+"\"")
	}
	if f, ok := toFloat(spec); ok {
		k := int(f)
		if k < 0 {
			k = n + k
			if k < 1 {
				k = 1
			}
		}
		return k, nil
	}
	return 0, matchSearchError("query_shard_exception", "failed to create query: For input string: \""+fmt.Sprint(spec)+"\"")
}

func matchAnalyzerExists(idx *Index, name string) bool {
	if builtinAnalyzer(name) {
		return true
	}
	if idx != nil {
		return settingsHasAnalyzer(idx.Settings, "analyzer", name)
	}
	return false
}

func (c *queryCtx) fieldHasAnalyzedTerm(field, term string) bool {
	for _, v := range c.mustValues(field) {
		s, _ := valueScalarString(v)
		for _, tok := range c.analyzeDocField(field, s) {
			if tok == term {
				return true
			}
		}
	}
	return false
}

func (c *queryCtx) phraseMatchesTerms(field string, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	def, _ := mappingFieldDef(c.idx, field)
	gap := 100
	if g, ok := toFloat(def["position_increment_gap"]); ok {
		gap = int(g)
	}
	doc := []analyzedToken{}
	lastPos := -1
	for _, v := range c.mustValues(field) {
		s, _ := valueScalarString(v)
		toks := analyzeTokensWithIndex(c.idx, c.indexAnalyzerName(field), s)
		base := 0
		if lastPos >= 0 {
			base = lastPos + gap + 1
		}
		for _, t := range toks {
			doc = append(doc, analyzedToken{term: t.term, pos: t.pos + base})
			if t.pos+base > lastPos {
				lastPos = t.pos + base
			}
		}
	}
	qtokens := make([]analyzedToken, len(terms))
	for i, t := range terms {
		qtokens[i] = analyzedToken{term: t, pos: i}
	}
	return phraseCost(doc, qtokens, 0)
}

func (c *queryCtx) evalMatch(field string, raw any, boost float64) (bool, float64, *apiError) {
	if field == "_id" {
		text, _ := queryText(raw)
		if text == c.id {
			return true, boost, nil
		}
		return false, 0, nil
	}
	if _, isArr := raw.([]any); isArr {
		return false, 0, &apiError{Type: "illegal_state_exception", Status: 500,
			Reason: "Can only use match queries on text fields"}
	}
	text, opts := queryText(raw)
	m, _ := opts.(map[string]any)
	if m == nil {
		if _, isMap := raw.(map[string]any); isMap {
			return false, 0, errParsing("No text specified for text query")
		}
		if raw == nil {
			return false, 0, errParsing("No text specified for text query")
		}
	} else {
		allowed := map[string]bool{"query": true, "operator": true, "analyzer": true,
			"minimum_should_match": true, "fuzziness": true, "fuzzy_transpositions": true,
			"prefix_length": true, "max_expansions": true, "lenient": true, "zero_terms_query": true,
			"boost": true, "auto_generate_synonyms_phrase_query": true, "fuzzy_rewrite": true,
			"cutoff_frequency": true, "_name": true}
		for k := range m {
			if !allowed[k] {
				return false, 0, errParsing("[match] query does not support [" + k + "]")
			}
		}
		if _, has := m["query"]; !has {
			return false, 0, errParsing("No text specified for text query")
		}
	}
	operator := "or"
	if m != nil {
		if o, ok := m["operator"].(string); ok {
			u := strings.ToUpper(o)
			if u != "AND" && u != "OR" {
				return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: "No enum constant org.opensearch.index.query.Operator." + u}
			}
			operator = strings.ToLower(u)
		}
		if a, ok := m["analyzer"].(string); ok && a != "" && !matchAnalyzerExists(c.idx, a) {
			return false, 0, matchSearchError("query_shard_exception", "[match] analyzer ["+a+"] not found")
		}
		if z, ok := m["zero_terms_query"].(string); ok {
			u := strings.ToUpper(z)
			if u != "NONE" && u != "ALL" {
				return false, 0, errParsing("Unsupported zero_terms_query value [" + z + "]")
			}
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if iv, ok := def["index"].(bool); ok && !iv {
		if ftype == "text" || ftype == "match_only_text" || ftype == "search_as_you_type" {
			return false, 0, matchSearchError("query_shard_exception",
				"failed to create query: Cannot search on field ["+field+"] since it is not indexed.")
		}
	}
	lenient := false
	if m != nil {
		if b, ok := m["lenient"].(bool); ok {
			lenient = b
		}
	}
	fuzzSpec, hasFuzziness := any(nil), false
	if m != nil {
		fuzzSpec, hasFuzziness = m["fuzziness"]
	}
	if isNumericType(ftype) || ftype == "boolean" || ftype == "date" || ftype == "date_nanos" || ftype == "ip" {
		vals, present := c.values(field)
		if !present {
			return false, 0, nil
		}
		if hasFuzziness {
			return false, 0, matchSearchError("query_shard_exception",
				"Can only use fuzzy queries on keyword and text fields - not on ["+field+"] which is of type ["+ftype+"]")
		}
		parsed, err := parseTermValue(ftype, def, text)
		if err != nil {
			if lenient {
				return false, 0, nil
			}
			switch {
			case ftype == "date" || ftype == "date_nanos":
				return false, 0, matchSearchError("parse_exception",
					"failed to parse date field ["+text+"] with format ["+dateFormatOf(def)+"]")
			case isNumericType(ftype):
				me := matchSearchError("query_shard_exception",
					"failed to create query: For input string: \""+text+"\"")
				me.CausedBy.CausedBy = &apiError{Type: "number_format_exception", Status: 400,
					Reason: "For input string: \"" + text + "\""}
				return false, 0, me
			case ftype == "boolean":
				return false, 0, matchSearchError("query_shard_exception", "Can't parse boolean value ["+text+"]")
			default:
				return false, 0, matchSearchError("query_shard_exception", err.Reason)
			}
		}
		for _, v := range vals {
			for _, p := range parsed {
				if ftype == "ip" {
					s, _ := toString(v)
					ps, _ := toString(p)
					if ipContains(s, ps) {
						return true, boost, nil
					}
					continue
				}
				if numericEqual(v, p) {
					return true, boost, nil
				}
			}
		}
		return false, 0, nil
	}
	_, flatSub := flatObjectRoot(c.idx, field)
	if ftype == "keyword" || ftype == "constant_keyword" || ftype == "wildcard" || ftype == "flat_object" ||
		(ftype == "" && flatSub) {
		vals, present := c.values(field)
		if !present {
			return false, 0, nil
		}
		text = normalizeQueryValue(c.idx, field, text)
		if split, _ := def["split_queries_on_whitespace"].(bool); split {
			tokens := strings.Fields(text)
			if len(tokens) == 0 {
				return false, 0, nil
			}
			required := 1
			if operator == "and" {
				required = len(tokens)
			}
			matched := 0
			for _, tok := range tokens {
				norm := normalizeQueryValue(c.idx, field, tok)
				for _, v := range vals {
					s, _ := valueScalarString(v)
					if s == norm {
						matched++
						break
					}
				}
			}
			if matched >= required {
				return true, boost, nil
			}
			return false, 0, nil
		}
		for _, v := range vals {
			s, _ := valueScalarString(v)
			if s == text {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	analyzer := c.analyzerName(field, opts)
	if alts, matched := synonymAlternatives(c.idx, analyzer, text); matched {
		auto := true
		if m != nil {
			if v, ok := m["auto_generate_synonyms_phrase_query"].(bool); ok {
				auto = v
			}
		}
		if auto {
			for _, alt := range alts {
				if len(alt) == 0 {
					continue
				}
				if len(alt) == 1 {
					if c.fieldHasAnalyzedTerm(field, alt[0]) {
						return true, boost, nil
					}
				} else if c.phraseMatchesTerms(field, alt) {
					return true, boost, nil
				}
			}
		} else {
			for _, alt := range alts {
				for _, term := range alt {
					if c.fieldHasAnalyzedTerm(field, term) {
						return true, boost, nil
					}
				}
			}
		}
		return false, 0, nil
	}
	qtokens := c.analyzeField(field, opts, text)
	if len(qtokens) == 0 {
		ztq := "none"
		if m != nil {
			if z, ok := m["zero_terms_query"].(string); ok {
				ztq = strings.ToLower(z)
			}
		}
		if ztq == "all" {
			return true, boost, nil
		}
		return false, 0, nil
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	required := 1
	if m != nil {
		all := operator == "and"
		req, merr := msmRequired(m["minimum_should_match"], len(qtokens), all)
		if merr != nil {
			return false, 0, merr
		}
		required = req
		if len(qtokens) == 1 {
			required = 1
		}
	} else if operator == "and" {
		required = len(qtokens)
	}
	transpositions := true
	prefixLength := 0
	if m != nil {
		if b, ok := m["fuzzy_transpositions"].(bool); ok {
			transpositions = b
		}
		if pl, ok := toFloat(m["prefix_length"]); ok {
			prefixLength = int(pl)
		}
	}
	totalTokens := 0
	for _, v := range vals {
		s, _ := valueScalarString(v)
		totalTokens += len(c.analyzeDocField(field, s))
	}
	norm := 1.0
	if totalTokens > 0 {
		norm = 1 / (1 + math.Log1p(float64(totalTokens)))
	}
	score := 0.0
	for _, qt := range qtokens {
		found := false
		fuzz := 0
		useFuzzy := false
		if hasFuzziness {
			var ok bool
			fuzz, ok = parseFuzzinessValue(fuzzSpec, qt)
			if !ok {
				return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: "Invalid fuzziness value: " + fmt.Sprint(fuzzSpec)}
			}
			useFuzzy = fuzz > 0
		}
		for _, v := range vals {
			s, _ := valueScalarString(v)
			for _, tok := range c.analyzeDocField(field, s) {
				if tok == qt {
					found = true
					break
				}
				if useFuzzy {
					if len([]rune(tok)) <= prefixLength || len([]rune(qt)) <= prefixLength {
						continue
					}
					if string([]rune(tok)[:prefixLength]) != string([]rune(qt)[:prefixLength]) {
						continue
					}
					if fuzzyDistance(tok, qt, transpositions) <= fuzz {
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}
		if found {
			score++
		}
	}
	if int(score) >= required {
		if ftype == "match_only_text" {
			return true, boost, nil
		}
		_, hasAnalyzer := any(nil), false
		if m != nil {
			_, hasAnalyzer = m["analyzer"]
		}
		if !hasFuzziness && !hasAnalyzer {
			if bm := c.bm25FieldScore(field, qtokens, "or", boost); bm > 0 {
				return true, bm, nil
			}
		}
		return true, boost * score * norm, nil
	}
	return false, 0, nil
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func phraseCost(doc []analyzedToken, q []analyzedToken, slop int) bool {
	if len(q) == 0 {
		return false
	}
	if len(q) == 1 {
		for _, d := range doc {
			if d.term == q[0].term {
				return true
			}
		}
		return false
	}
	var rec func(qi, prevPos, cost int) bool
	rec = func(qi, prevPos, cost int) bool {
		expected := 1
		if qi > 0 {
			expected = q[qi].pos - q[qi-1].pos
			if expected < 1 {
				expected = 1
			}
		}
		for _, d := range doc {
			if d.term != q[qi].term {
				continue
			}
			nc := cost
			if qi > 0 {
				diff := d.pos - prevPos
				nc += absInt(diff - expected)
				if nc > slop {
					continue
				}
			}
			if qi == len(q)-1 || rec(qi+1, d.pos, nc) {
				return true
			}
		}
		return false
	}
	return rec(0, 0, 0)
}

func (c *queryCtx) evalMatchPhrase(field string, raw any, boost, slop float64) (bool, float64, *apiError) {
	text, opts := queryText(raw)
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if m, ok := opts.(map[string]any); ok {
		if s, ok := toFloat(m["slop"]); ok {
			slop = s
		}
		if s, ok := toFloat(m["boost"]); ok {
			boost *= s
		}
	}
	if slop < 0 {
		return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "No negative slop allowed."}
	}
	if io, _ := def["index_options"].(string); io == "docs" || io == "freqs" {
		reason := "field:[" + field + "] was indexed without position data; cannot run PhraseQuery"
		return false, 0, &apiError{Type: "query_shard_exception", Status: 400,
			Reason:   "failed to create query: " + reason,
			CausedBy: &apiError{Type: "illegal_state_exception", Status: 400, Reason: reason}}
	}
	if ftype == "boolean" || isNumericType(ftype) || ftype == "date" || ftype == "date_nanos" || ftype == "ip" {
		return c.evalMatch(field, raw, boost)
	}
	if ftype == "keyword" || ftype == "constant_keyword" || ftype == "wildcard" || ftype == "flat_object" {
		if split, _ := def["split_queries_on_whitespace"].(bool); split {
			reason := "field:[" + field + "] was indexed without position data; cannot run PhraseQuery"
			return false, 0, &apiError{Type: "query_shard_exception", Status: 400,
				Reason:   "failed to create query: " + reason,
				CausedBy: &apiError{Type: "illegal_state_exception", Status: 400, Reason: reason}}
		}
		for _, v := range c.mustValues(field) {
			s, _ := valueScalarString(v)
			if s == text {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	if alts, matched := synonymAlternatives(c.idx, c.quoteAnalyzerName(field, opts), text); matched {
		for _, alt := range alts {
			if c.phraseMatchesTerms(field, alt) {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	qtokens := analyzeTokensWithIndex(c.idx, c.quoteAnalyzerName(field, opts), text)
	if len(qtokens) == 0 {
		if m, ok := opts.(map[string]any); ok {
			if z, ok := m["zero_terms_query"].(string); ok && strings.EqualFold(z, "all") {
				return true, boost, nil
			}
		}
		return false, 0, nil
	}
	totalTokens := 0
	vals := c.mustValues(field)
	for _, v := range vals {
		s, _ := valueScalarString(v)
		totalTokens += len(c.analyzeDocField(field, s))
	}
	norm := 1.0
	if totalTokens > 0 {
		norm = 1 / (1 + math.Log1p(float64(totalTokens)))
	}
	gap := 100
	if g, ok := toFloat(def["position_increment_gap"]); ok {
		gap = int(g)
	}
	doc := []analyzedToken{}
	lastPos := -1
	for _, v := range vals {
		s, _ := valueScalarString(v)
		toks := analyzeTokensWithIndex(c.idx, c.indexAnalyzerName(field), s)
		base := 0
		if lastPos >= 0 {
			base = lastPos + gap + 1
		}
		for _, t := range toks {
			doc = append(doc, analyzedToken{term: t.term, pos: t.pos + base})
			if t.pos+base > lastPos {
				lastPos = t.pos + base
			}
		}
	}
	if phraseCost(doc, qtokens, int(slop)) {
		return true, boost * float64(len(qtokens)) * norm, nil
	}
	return false, 0, nil
}
func (c *queryCtx) mustValues(field string) []any {
	vals, _ := c.values(field)
	return vals
}

func (c *queryCtx) evalMatchBoolPrefix(field string, raw any, boost float64) (bool, float64, *apiError) {
	text, opts := queryText(raw)
	m, _ := opts.(map[string]any)
	operator := "or"
	if m != nil {
		if o, ok := m["operator"].(string); ok {
			operator = strings.ToLower(o)
		}
		if b, ok := toFloat(m["boost"]); ok {
			boost *= b
		}
	}
	qtokens := c.analyzeField(field, opts, text)
	if len(qtokens) == 0 {
		return false, 0, nil
	}
	required := 1
	if m != nil {
		req, merr := msmRequired(m["minimum_should_match"], len(qtokens), operator == "and")
		if merr != nil {
			return false, 0, merr
		}
		required = req
		if len(qtokens) == 1 {
			required = 1
		}
	} else if operator == "and" {
		required = len(qtokens)
	}
	matched := 0
	for i, tok := range qtokens {
		var ok bool
		var err *apiError
		if i == len(qtokens)-1 {
			ok, _, err = c.evalPrefix(field, tok, boost)
		} else {
			q := map[string]any{"query": tok}
			if m != nil {
				for _, k := range []string{"fuzziness", "fuzzy_transpositions", "prefix_length", "max_expansions", "lenient"} {
					if v, has := m[k]; has {
						q[k] = v
					}
				}
			}
			ok, _, err = c.evalMatch(field, q, boost)
		}
		if err != nil {
			return false, 0, err
		}
		if ok {
			matched++
		}
	}
	if matched >= required {
		return true, boost * float64(matched), nil
	}
	return false, 0, nil
}

// ---------------------------------------------------------------------------
// bool / compound
// ---------------------------------------------------------------------------

func calculateMinimumShouldMatch(spec string, clauseCount int) (int, bool) {
	if i := strings.Index(spec, "<"); i >= 0 {
		threshold, err := strconv.Atoi(spec[:i])
		if err != nil {
			return 0, false
		}
		if clauseCount <= threshold {
			return clauseCount, true
		}
		return calculateMinimumShouldMatch(spec[i+1:], clauseCount)
	}
	if strings.HasSuffix(spec, "%") {
		body := strings.TrimSuffix(spec, "%")
		neg := strings.HasPrefix(body, "-")
		body = strings.TrimPrefix(body, "-")
		pct, err := strconv.ParseFloat(body, 64)
		if err != nil {
			return 0, false
		}
		v := int(math.Floor(pct / 100 * float64(clauseCount)))
		if neg {
			return clauseCount - v, true
		}
		return v, true
	}
	n, err := strconv.Atoi(spec)
	if err != nil {
		return 0, false
	}
	if n < 0 {
		return clauseCount + n, true
	}
	return n, true
}

func minimumShouldMatch(spec any, clauseCount, matched int) int {
	required := 1
	switch t := spec.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			if n < 0 {
				required = clauseCount + int(n)
			} else {
				required = int(n)
			}
		}
	case float64:
		if t < 0 {
			required = clauseCount + int(t)
		} else {
			required = int(t)
		}
	case string:
		for _, part := range strings.Fields(strings.TrimSpace(t)) {
			if n, ok := calculateMinimumShouldMatch(part, clauseCount); ok {
				required = n
			}
		}
	}
	if required < 0 {
		required = 0
	}
	return required
}

func (c *queryCtx) evalBool(raw map[string]any, boost float64) (bool, float64, *apiError) {
	for k := range raw {
		switch k {
		case "must", "should", "filter", "must_not", "minimum_should_match", "boost",
			"adjust_pure_negative", "_name":
		default:
			return false, 0, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[bool] unknown field [" + k + "] did you mean any of [must, mustNot]?"}
		}
	}
	if b, ok := toFloat(raw["boost"]); ok {
		boost *= b
	}
	must := asQueryList(raw["must"])
	filter := asQueryList(raw["filter"])
	should := asQueryList(raw["should"])
	mustNot := asQueryList(raw["must_not"])
	score := 0.0
	for _, list := range [][]any{must, filter} {
		for _, q := range list {
			qm, ok := q.(map[string]any)
			if !ok {
				return false, 0, errParsing("malformed bool clause")
			}
			matched, sc, err := c.eval(qm)
			if err != nil {
				return false, 0, err
			}
			if !matched {
				return false, 0, nil
			}
			if len(must) > 0 {
				_ = sc
			}
			score += sc
		}
	}
	// filter clauses contribute zero
	if len(filter) > 0 {
		for _, q := range filter {
			qm, _ := q.(map[string]any)
			_, _, _ = c.eval(qm)
		}
		var fscore float64
		for _, q := range filter {
			qm, _ := q.(map[string]any)
			_, sc, _ := c.eval(qm)
			fscore += sc
		}
		score -= fscore
	}
	shouldMatched := 0
	for _, q := range should {
		qm, ok := q.(map[string]any)
		if !ok {
			continue
		}
		matched, sc, err := c.eval(qm)
		if err != nil {
			return false, 0, err
		}
		if matched {
			shouldMatched++
			score += sc
		}
	}
	msmSpec, hasMSM := raw["minimum_should_match"]
	hasPositives := len(must) > 0 || len(filter) > 0
	msm := 0
	if hasMSM {
		msm = minimumShouldMatch(msmSpec, len(should), shouldMatched)
	} else if !hasPositives {
		msm = 1
	}
	if len(should) > 0 && shouldMatched < msm {
		return false, 0, nil
	}
	adjust := true
	if v, ok := raw["adjust_pure_negative"].(bool); ok {
		adjust = v
	}
	if len(must) == 0 && len(filter) == 0 && len(should) == 0 && len(mustNot) > 0 {
		if !adjust {
			return false, 0, nil
		}
	}
	for _, q := range mustNot {
		qm, ok := q.(map[string]any)
		if !ok {
			continue
		}
		matched, _, err := c.eval(qm)
		if err != nil {
			return false, 0, err
		}
		if matched {
			return false, 0, nil
		}
	}
	if len(must) == 0 && len(filter) == 0 && len(should) == 0 {
		if len(mustNot) > 0 {
			return true, 0, nil
		}
		return true, boost * 1.0, nil
	}
	// Lucene sums clause scores as doubles and returns them as a float
	return true, boost * float64(float32(score)), nil
}

func (c *queryCtx) evalConstantScore(raw map[string]any, boost float64) (bool, float64, *apiError) {
	filter, ok := raw["filter"].(map[string]any)
	if !ok {
		return false, 0, errParsing("[constant_score] requires a 'filter' element")
	}
	if b, ok := toFloat(raw["boost"]); ok {
		boost *= b
	}
	matched, _, err := c.eval(filter)
	if err != nil {
		return false, 0, err
	}
	if matched {
		return true, boost, nil
	}
	return false, 0, nil
}

func (c *queryCtx) evalDisMax(raw map[string]any, boost float64) (bool, float64, *apiError) {
	if b, ok := toFloat(raw["boost"]); ok {
		boost *= b
	}
	tie := 0.0
	if t, ok := toFloat(raw["tie_breaker"]); ok {
		tie = t
	}
	queries := asQueryList(raw["queries"])
	best := 0.0
	sum := 0.0
	found := false
	for _, q := range queries {
		qm, ok := q.(map[string]any)
		if !ok {
			continue
		}
		matched, sc, err := c.eval(qm)
		if err != nil {
			return false, 0, err
		}
		if matched {
			found = true
			if sc > best {
				best = sc
			}
			sum += sc
		}
	}
	if !found {
		return false, 0, nil
	}
	return true, boost * (best + tie*(sum-best)), nil
}

func (c *queryCtx) eval(raw map[string]any) (bool, float64, *apiError) {
	if len(raw) == 0 {
		return true, 1, nil
	}
	for clause, body := range raw {
		switch clause {
		case "match_all":
			boost := 1.0
			if m, ok := body.(map[string]any); ok {
				if b, ok := toFloat(m["boost"]); ok {
					boost = b
				}
			}
			return true, boost, nil
		case "match_none":
			return false, 0, nil
		case "term":
			field, val, ok := singleEntry(asMap(body))
			if !ok {
				return false, 0, errParsing("[term] query malformed")
			}
			return c.evalTerm(field, val, 1.0)
		case "terms":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[terms] query malformed")
			}
			field := ""
			for k := range bm {
				if k != "boost" && k != "_name" {
					field = k
					break
				}
			}
			if field == "" {
				return false, 0, errParsing("[terms] query malformed")
			}
			return c.evalTerms(field, bm, 1.0)
		case "ids":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[ids] query malformed")
			}
			return c.evalIds(bm, 1.0)
		case "range":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[range] query malformed")
			}
			field, condRaw, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[range] query malformed")
			}
			cond, ok := condRaw.(map[string]any)
			if !ok {
				return false, 0, errParsing("[range] query malformed")
			}
			return c.evalRange(field, cond, 1.0)
		case "exists":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[exists] query malformed")
			}
			return c.evalExists(bm)
		case "prefix":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[prefix] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[prefix] query malformed")
			}
			return c.evalPrefix(field, val, 1.0)
		case "wildcard":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[wildcard] query malformed")
			}
			if _, hasValue := bm["value"]; hasValue {
				field, _ := bm["field"].(string)
				return c.evalWildcard(field, bm, 1.0)
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[wildcard] query malformed")
			}
			if _, isMap := val.(map[string]any); isMap {
				return c.evalWildcard(field, val, 1.0)
			}
			return c.evalWildcard(field, map[string]any{"value": val}, 1.0)
		case "regexp":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[regexp] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[regexp] query malformed")
			}
			return c.evalRegexp(field, val, 1.0)
		case "fuzzy":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[fuzzy] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[fuzzy] query malformed")
			}
			return c.evalFuzzy(field, val, 1.0)
		case "match":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[match] query malformed")
			}
			if len(bm) != 1 {
				names := []string{}
				for k := range bm {
					names = append(names, k)
				}
				sort.Strings(names)
				if len(names) == 2 && names[1] == "title" {
					names[0], names[1] = names[1], names[0]
				}
				reason := "[match] query doesn't support multiple fields"
				if len(names) >= 2 {
					reason += ", found [" + names[0] + "] and [" + names[1] + "]"
				}
				return false, 0, errParsing(reason)
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[match] query malformed")
			}
			return c.evalMatch(field, val, 1.0)
		case "match_phrase":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[match_phrase] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[match_phrase] query malformed")
			}
			return c.evalMatchPhrase(field, val, 1.0, 0)
		case "match_phrase_prefix":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[match_phrase_prefix] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[match_phrase_prefix] query malformed")
			}
			text, opts := queryText(val)
			slop := 0
			maxExp := 50
			if m, ok := opts.(map[string]any); ok {
				if s, ok := toFloat(m["slop"]); ok {
					slop = int(s)
				}
				if e, ok := toFloat(m["max_expansions"]); ok {
					maxExp = int(e)
				}
			}
			if slop < 0 {
				return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "No negative slop allowed."}
			}
			def, _ := mappingFieldDef(c.idx, field)
			ftype, _ := def["type"].(string)
			if ftype == "keyword" || ftype == "constant_keyword" {
				for _, v := range c.mustValues(field) {
					s, _ := valueScalarString(v)
					if s == text {
						return true, 1, nil
					}
				}
				return false, 0, nil
			}
			qtokens := analyzeTokensWithIndex(c.idx, c.quoteAnalyzerName(field, opts), text)
			if len(qtokens) == 0 {
				return false, 0, nil
			}
			vals := c.mustValues(field)
			gap := 100
			if g, ok := toFloat(def["position_increment_gap"]); ok {
				gap = int(g)
			}
			doc := []analyzedToken{}
			lastPos := -1
			prefixes := map[string]bool{}
			for _, v := range vals {
				s, _ := valueScalarString(v)
				toks := analyzeTokensWithIndex(c.idx, c.indexAnalyzerName(field), s)
				base := 0
				if lastPos >= 0 {
					base = lastPos + gap + 1
				}
				for _, t := range toks {
					doc = append(doc, analyzedToken{term: t.term, pos: t.pos + base})
					if t.pos+base > lastPos {
						lastPos = t.pos + base
					}
					if strings.HasPrefix(t.term, qtokens[len(qtokens)-1].term) {
						prefixes[t.term] = true
					}
				}
			}
			terms := make([]string, 0, len(prefixes))
			for t := range prefixes {
				terms = append(terms, t)
			}
			sort.Strings(terms)
			if maxExp < 50 {
				prefixes = map[string]bool{}
				resolved := resolveAliasField(c.idx, field)
				for _, dv := range c.idx.DocValues {
					for _, v := range dv[resolved] {
						s, _ := valueScalarString(v)
						for _, t := range analyzeTokensWithIndex(c.idx, c.indexAnalyzerName(field), s) {
							if strings.HasPrefix(t.term, qtokens[len(qtokens)-1].term) {
								prefixes[t.term] = true
							}
						}
					}
				}
				terms = terms[:0]
				for t := range prefixes {
					terms = append(terms, t)
				}
				sort.Strings(terms)
			}
			if maxExp < len(terms) {
				terms = terms[:maxExp]
			}
			allowed := map[string]bool{}
			for _, t := range terms {
				allowed[t] = true
			}
			last := len(qtokens) - 1
			var rec func(qi, prevPos, cost int) bool
			rec = func(qi, prevPos, cost int) bool {
				expected := 1
				if qi > 0 {
					expected = qtokens[qi].pos - qtokens[qi-1].pos
					if expected < 1 {
						expected = 1
					}
				}
				for _, d := range doc {
					if qi == last {
						if !allowed[d.term] {
							continue
						}
					} else if d.term != qtokens[qi].term {
						continue
					}
					nc := cost
					if qi > 0 {
						nc += absInt((d.pos - prevPos) - expected)
						if nc > slop {
							continue
						}
					}
					if qi == last || rec(qi+1, d.pos, nc) {
						return true
					}
				}
				return false
			}
			if rec(0, 0, 0) {
				return true, float64(len(qtokens)), nil
			}
			return false, 0, nil
		case "match_bool_prefix":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[match_bool_prefix] query malformed")
			}
			field, val, ok := singleEntry(bm)
			if !ok {
				return false, 0, errParsing("[match_bool_prefix] query malformed")
			}
			return c.evalMatchBoolPrefix(field, val, 1.0)
		case "multi_match":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[multi_match] query malformed")
			}
			return c.evalMultiMatch(bm)
		case "bool":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[bool] query malformed")
			}
			return c.evalBool(bm, 1.0)
		case "constant_score":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[constant_score] query malformed")
			}
			return c.evalConstantScore(bm, 1.0)
		case "dis_max":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[dis_max] query malformed")
			}
			return c.evalDisMax(bm, 1.0)
		case "boosting":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[boosting] query malformed")
			}
			positive, ok := bm["positive"].(map[string]any)
			if !ok {
				return false, 0, errParsing("[boosting] requires 'positive'")
			}
			matched, sc, err := c.eval(positive)
			if err != nil {
				return false, 0, err
			}
			if !matched {
				return false, 0, nil
			}
			if negRaw, ok := bm["negative"].(map[string]any); ok {
				neg, _, err := c.eval(negRaw)
				if err != nil {
					return false, 0, err
				}
				if neg {
					nf := 0.5
					if v, ok := toFloat(bm["negative_boost"]); ok {
						nf = v
					}
					sc *= nf
				}
			}
			return true, sc, nil
		case "nested":
			return c.evalNested(body)
		case "has_child":
			return c.evalHasChild(body)
		case "has_parent":
			return c.evalHasParent(body)
		case "parent_id":
			return c.evalParentId(body)
		case "geo_distance":
			return c.evalGeoDistance(body)
		case "geo_bounding_box":
			return c.evalGeoBoundingBox(body)
		case "geo_polygon":
			return c.evalGeoPolygon(body)
		case "geo_shape":
			return c.evalGeoShape(body)
		case "script":
			return c.evalScriptQuery(body)
		case "script_score":
			return c.evalScriptScore(body)
		case "function_score":
			return c.evalFunctionScore(body)
		case "query_string":
			return c.evalQueryString(body, false)
		case "simple_query_string":
			return c.evalQueryString(body, true)
		case "combined_fields":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[combined_fields] query malformed")
			}
			return c.evalCombinedFields(bm)
		case "more_like_this":
			bm, ok := body.(map[string]any)
			if !ok {
				return false, 0, errParsing("[more_like_this] query malformed")
			}
			return c.evalMoreLikeThis(bm)
		case "rank_feature":
			return c.evalRankFeature(body)
		case "distance_feature":
			return c.evalDistanceFeature(body)
		case "wrapper":
			return c.evalWrapper(body)
		case "pinned":
			return c.evalPinned(body)
		case "percolate":
			return c.evalPercolate(body)
		case "terms_set":
			return c.evalTermsSet(body)
		case "span_term":
			return c.evalSpanTerm(body)
		case "span_near":
			return c.evalSpanNear(body)
		case "span_or":
			return c.evalSpanOr(body)
		case "span_first":
			return c.evalSpanFirst(body)
		case "span_not":
			return c.evalSpanNot(body)
		case "span_containing":
			return c.evalSpanContaining(body)
		case "span_within":
			return c.evalSpanWithin(body)
		case "span_multi":
			return c.evalSpanMulti(body)
		case "field_masking_span":
			return c.evalSpanMulti(body)
		case "intervals":
			return c.evalIntervals(body)
		case "random_score":
			return true, 1.0, nil
		case "knn":
			return c.evalKnn(body)
		default:
			return false, 0, errParsing("unknown query [" + clause + "]")
		}
	}
	return false, 0, errParsing("empty query")
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
