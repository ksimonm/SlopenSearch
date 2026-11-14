package httpapi

import (
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

// Glue between the scripting engines and the APIs that run scripts
// (search, aggregations, update, ingest): script specs, stored-script
// resolution, language checks and the shard-failure envelopes.

// ---- shard failure envelopes ----

// ---- bindings ----

// ---- compiled scripts ----

// scriptToString renders org.opensearch.script.Script#toString.
func scriptToString(raw any) string {
	spec, err := search.ParseScriptSpec(raw)
	if err != nil {
		return ""
	}
	params := painless.ToString(search.ScriptParams(spec.Params))
	if spec.Id != "" {
		return "Script{type=stored, lang='null', idOrCode='" + spec.Id + "', options=null, params=" + params + "}"
	}
	return "Script{type=inline, lang='" + spec.Lang + "', idOrCode='" + spec.Src + "', options={}, params=" + params + "}"
}

// ---- aggregation scripts ----
