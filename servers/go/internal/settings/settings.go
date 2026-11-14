// Package settings reads index settings maps.
package settings

import "slopensearch/internal/jsonv"

// Int returns an integer setting (see Find).
func Int(settings map[string]any, key string) (int, bool) {
	v, ok := Find(settings, key)
	if !ok {
		return 0, false
	}
	f, ok := jsonv.ToFloat(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Find looks key up as "index.<key>", "<key>" and inside a nested "index"
// object, in that order.
func Find(settings map[string]any, key string) (any, bool) {
	if v, ok := settings["index."+key]; ok {
		return v, true
	}
	if v, ok := settings[key]; ok {
		return v, true
	}
	if m, ok := settings["index"].(map[string]any); ok {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}
