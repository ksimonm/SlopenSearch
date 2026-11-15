package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/search"
)

type shardFilter struct {
	active  bool
	allowed map[*model.Index]map[int]bool
	total   int
}

func newShardFilter(r *http.Request, indices []*model.Index) *shardFilter {
	routing := r.URL.Query().Get("routing")
	pref := r.URL.Query().Get("preference")
	f := &shardFilter{active: false, allowed: map[*model.Index]map[int]bool{}}
	if routing != "" {
		f.active = true
		for _, idx := range indices {
			set := map[int]bool{}
			for _, part := range strings.Split(routing, ",") {
				if part == "" {
					continue
				}
				for _, sh := range idx.RoutingShards(part) {
					set[sh] = true
				}
			}
			f.allowed[idx] = set
		}
	} else if strings.HasPrefix(pref, "_shards:") {
		f.active = true
		spec := strings.TrimPrefix(pref, "_shards:")
		for _, idx := range indices {
			set := map[int]bool{}
			for _, part := range strings.Split(spec, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(part))
				if err == nil && n >= 0 && n < idx.NumShards() {
					set[n] = true
				}
			}
			f.allowed[idx] = set
		}
	}
	if !f.active {
		return nil
	}
	for _, idx := range indices {
		f.total += len(f.allowed[idx])
	}
	return f
}

func (f *shardFilter) allowsShard(idx *model.Index, shard int) bool {
	if f == nil {
		return true
	}
	return f.allowed[idx][shard]
}

func (f *shardFilter) allows(idx *model.Index, id string) bool {
	if f == nil {
		return true
	}
	return f.allowsShard(idx, idx.DocShard(id))
}

func searchShardMultiplier(idx *model.Index, id string, query map[string]any, dfs bool) float64 {
	if idx.NumShards() <= 1 {
		return 1
	}
	field, text := "", ""
	if m, ok := query["match"].(map[string]any); ok {
		for f, raw := range m {
			field = f
			text, _ = search.QueryText(raw)
		}
	} else if m, ok := query["match_phrase"].(map[string]any); ok {
		for f, raw := range m {
			field = f
			text, _ = search.QueryText(raw)
		}
	}
	if field == "" {
		return 1
	}
	tokens := mapping.Analyze(text)
	if len(tokens) == 0 {
		return 1
	}
	docShard := idx.DocShard(id)

	n, df := 0, 0
	for _, did := range idx.Order {
		doc, live := idx.Docs[did]
		if !live {
			continue
		}
		ds := idx.DocShard(did)
		if !dfs && ds != docShard {
			continue
		}
		n++
		qc := search.NewQueryCtxForDoc(idx, did, doc)
		vals, present := qc.Values(field)
		if !present {
			continue
		}
		found := false
		for _, v := range vals {
			s, _ := jsonv.ScalarString(v)
			for _, tok := range qc.AnalyzeDocField(field, s) {
				for _, want := range tokens {
					if tok == want {
						found = true
					}
				}
			}
		}
		if found {
			df++
		}
	}
	if n == 0 || df == 0 {
		return 1
	}
	return float64(n) / float64(df)
}
