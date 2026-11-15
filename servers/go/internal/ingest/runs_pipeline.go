package ingest

import (
	"slopensearch/internal/model"
	"slopensearch/internal/settings"
)

// RunsPipeline reports whether indexing into idx goes through an ingest
// pipeline: the requested one (unless "_none"), else the index default
// pipeline, or the index final pipeline.
func RunsPipeline(idx *model.Index, requested string) bool {
	p := requested
	if p == "" {
		if v, ok := settings.Find(idx.Settings, "default_pipeline"); ok {
			p = settings.ValueString(v)
		}
	}
	if p != "" && p != "_none" {
		return true
	}
	if v, ok := settings.Find(idx.Settings, "final_pipeline"); ok {
		fp := settings.ValueString(v)
		return fp != "" && fp != "_none"
	}
	return false
}
