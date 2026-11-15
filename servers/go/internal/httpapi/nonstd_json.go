package httpapi

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/ingest"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
)

// nonStdDocError is the error for a document source that is valid JSON up
// to one of Jackson's non-standard number tokens (NaN, Infinity, ...); nil
// when raw fails for any other reason. Through an ingest pipeline the source
// fails while it is read into a map; otherwise while the document parser
// reads it.
func nonStdDocError(idx *model.Index, id, pipeline string, raw []byte) *apierr.Error {
	t := mapping.FindNonStandardToken(raw)
	if t == nil {
		return nil
	}
	if ingest.RunsPipeline(idx, pipeline) {
		return t.IngestError()
	}
	return mapping.NonStandardTokenError(idx, id, raw)
}
