// Package analysis implements OpenSearch text analysis: tokenizers, token
// filters, char filters, synonyms, the built-in and language analyzers and
// analysis chains built from index settings (analysis.analyzer.*, ...).
//
// It depends only on the settings of an index (Index), never on the server.
package analysis

import (
	"slopensearch/internal/jsonv"
	"slopensearch/internal/settings"
)

// Index is what analysis needs from an index: its settings. A nil *Index
// means the global scope (built-in components only).
type Index struct {
	Settings map[string]any
}

func findSetting(m map[string]any, key string) (any, bool) { return settings.Find(m, key) }

func settingInt(m map[string]any, key string) (int, bool) { return settings.Int(m, key) }

func toFloat(v any) (float64, bool) { return jsonv.ToFloat(v) }
