package httpapi

import (
	"net/url"

	"slopensearch/internal/model"
	"slopensearch/internal/search"
)

// mappingSourceSpec returns the index-level _source filter and whether source
// storage is enabled.
func mappingSourceSpec(idx *model.Index) (includes, excludes []string, enabled bool) {
	enabled = true
	raw, ok := idx.Mappings["_source"]
	if !ok {
		return nil, nil, true
	}
	switch t := raw.(type) {
	case bool:
		return nil, nil, t
	case string:
		return nil, nil, t != "false"
	case map[string]any:
		if b, ok := t["enabled"].(bool); ok {
			enabled = b
		}
		includes = search.AnyStringList(t["includes"])
		if len(includes) == 0 {
			includes = search.AnyStringList(t["include"])
		}
		excludes = search.AnyStringList(t["excludes"])
		if len(excludes) == 0 {
			excludes = search.AnyStringList(t["exclude"])
		}
		return includes, excludes, enabled
	}
	return nil, nil, true
}

func sourceFilterFromQuery(q url.Values) (includes, excludes []string, omit bool, ok bool) {
	if v := q.Get("_source"); v != "" {
		if v == "false" {
			return nil, nil, true, true
		}
		if v == "true" {
			return nil, nil, false, true
		}
		includes = search.SplitSourceList(v)
	}
	if v := q.Get("_source_includes"); v != "" {
		includes = search.SplitSourceList(v)
	}
	if v := q.Get("_source_excludes"); v != "" {
		excludes = search.SplitSourceList(v)
	}
	return includes, excludes, false, len(includes) > 0 || len(excludes) > 0
}
