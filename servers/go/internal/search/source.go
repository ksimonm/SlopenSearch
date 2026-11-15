package search

import (
	"strings"

	"slopensearch/internal/jsonv"
)

func SplitSourceList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func StringList(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case string:
		out = append(out, t)
	case []any:
		for _, e := range t {
			if s, ok := jsonv.ToString(e); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func ParseSourceFilter(body map[string]any) (includes, excludes []string, omit bool, ok bool) {
	raw, exists := body["_source"]
	if !exists {
		return nil, nil, false, false
	}
	switch t := raw.(type) {
	case bool:
		return nil, nil, !t, true
	case string:
		if t == "false" {
			return nil, nil, true, true
		}
		if t == "true" || t == "" {
			return nil, nil, false, true
		}
		return SplitSourceList(t), nil, false, true
	case []any:
		return StringList(t), nil, false, true
	case map[string]any:
		inc := StringList(t["includes"])
		if len(inc) == 0 {
			inc = StringList(t["include"])
		}
		exc := StringList(t["excludes"])
		if len(exc) == 0 {
			exc = StringList(t["exclude"])
		}
		return inc, exc, false, true
	}
	return nil, nil, false, false
}

func WildcardMatch(pattern, s string) bool {
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]) {
			pi++
			si++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			star = pi
			mark = si
			pi++
		} else if star != -1 {
			pi = star + 1
			mark++
			si = mark
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func includeSourcePath(dst, src map[string]any, segs []string) {
	if len(segs) == 0 {
		return
	}
	seg := segs[0]
	last := len(segs) == 1
	for key, val := range src {
		if !WildcardMatch(seg, key) {
			continue
		}
		if last {
			dst[key] = jsonv.DeepCopy(val)
			continue
		}
		switch t := val.(type) {
		case map[string]any:
			child, _ := dst[key].(map[string]any)
			if child == nil {
				child = map[string]any{}
				dst[key] = child
			}
			includeSourcePath(child, t, segs[1:])
		case []any:
			arr := []any{}
			for _, elem := range t {
				if em, ok := elem.(map[string]any); ok {
					child := map[string]any{}
					includeSourcePath(child, em, segs[1:])
					if len(child) > 0 {
						arr = append(arr, child)
					}
				}
			}
			if len(arr) > 0 {
				dst[key] = arr
			}
		}
	}
}

func excludeSourcePath(dst map[string]any, segs []string) {
	if len(segs) == 0 {
		return
	}
	seg := segs[0]
	last := len(segs) == 1
	for key, val := range dst {
		if !WildcardMatch(seg, key) {
			continue
		}
		if last {
			delete(dst, key)
			continue
		}
		switch t := val.(type) {
		case map[string]any:
			excludeSourcePath(t, segs[1:])
		case []any:
			for _, elem := range t {
				if em, ok := elem.(map[string]any); ok {
					excludeSourcePath(em, segs[1:])
				}
			}
		}
	}
}

func FilterSource(doc map[string]any, includes, excludes []string) map[string]any {
	if len(includes) == 0 && len(excludes) == 0 {
		return doc
	}
	out := map[string]any{}
	if len(includes) == 0 {
		for k, v := range doc {
			out[k] = jsonv.DeepCopy(v)
		}
	} else {
		for _, inc := range includes {
			segs := strings.Split(inc, ".")
			includeSourcePath(out, doc, segs)
		}
	}
	for _, exc := range excludes {
		excludeSourcePath(out, strings.Split(exc, "."))
	}
	return out
}
