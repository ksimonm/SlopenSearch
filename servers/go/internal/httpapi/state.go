package httpapi

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"slopensearch/internal/apierr"
	"slopensearch/internal/ingest"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/search"
	"slopensearch/internal/storage"
)

const versionCreated = "137307827"

// Server is the node: its cluster state, the locks guarding it and the
// request-scoped contexts (scroll, point in time).
type Server struct {
	// mu guards State except IngestPipelines (ingestMu) and Scripts (scriptsMu).
	mu sync.RWMutex
	model.State
	ingestMu      sync.RWMutex
	scriptsMu     sync.RWMutex
	scrolls       map[string]*scrollContext
	scrollMu      sync.Mutex
	scrollCounter int64
	pits          map[string]*pitContext
	pitMu         sync.Mutex
	pitCounter    int64
	persist       *storage.Store // nil in memory-only mode (SLOP_IN_MEMORY=true)
	ingest        *ingest.Engine
}

// NewServer returns a node with an empty cluster state.
func NewServer() *Server {
	s := &Server{
		State:   model.NewState(),
		scrolls: map[string]*scrollContext{},
		pits:    map[string]*pitContext{},
	}
	s.ingest = ingest.NewEngine(&s.State, &s.ingestMu, ingestScripts{})
	search.Cluster = clusterLookup{s}
	return s
}

func (s *Server) bumpState() { s.StateVersion++ }

var randState uint64 = 88172645463325252

func simpleRand() uint64 {
	randState ^= randState << 13
	randState ^= randState >> 7
	randState ^= randState << 17
	return randState
}

func stringifyScalar(v any) any {
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return v
	}
}

func mergeMappingForIndex(idx *model.Index, props, src map[string]any) *apierr.Error {
	if enabled, ok := idx.Mappings["enabled"].(bool); ok && !enabled {
		return nil
	}
	dyn := mapping.RootDynamicString(idx)
	if dyn == "false" || dyn == "false_allow_templates" {
		return nil
	}
	return mergeMapping(idx, props, "", dyn, src)
}

func mergeMapping(idx *model.Index, props map[string]any, path, dyn string, src map[string]any) *apierr.Error {
	if dyn == "false" {
		return nil
	}
	expanded, derr := mapping.ExpandDottedObject(src)
	if derr != nil {
		return derr
	}
	for rawK, v := range expanded {
		k := strings.TrimRight(rawK, ".")
		if k == "" {
			k = rawK
		}
		if mapping.IsDocumentMetadataField(k) {
			continue
		}
		childPath := k
		if path != "" {
			childPath = path + "." + k
		}
		existing, declared := props[k].(map[string]any)
		if !declared {
			def, matched, err := mapping.DynamicDefFor(idx, childPath, k, v)
			if err != nil {
				return err
			}
			allow := true
			switch dyn {
			case "false", "strict":
				allow = false
			case "strict_allow_templates", "false_allow_templates":
				allow = matched
			}
			if !allow || def == nil {
				continue
			}
			props[k] = def
			continue
		}
		childDyn := dyn
		if d, ok := existing["dynamic"]; ok {
			childDyn = mapping.DynamicString(d)
		}
		if sub, ok := existing["properties"].(map[string]any); ok {
			switch t := v.(type) {
			case map[string]any:
				if err := mergeMapping(idx, sub, childPath, childDyn, t); err != nil {
					return err
				}
			case []any:
				for _, e := range t {
					if m, ok := e.(map[string]any); ok {
						if err := mergeMapping(idx, sub, childPath, childDyn, m); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func httpPortNum() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9200"
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 9200
	}
	return n
}

// clusterLookup gives query evaluation access to other indices and stored
// scripts.
type clusterLookup struct{ s *Server }

func (c clusterLookup) Index(name string) (*model.Index, bool) {
	idx, ok := c.s.Indices[name]
	return idx, ok
}

func (c clusterLookup) StoredScript(id string) (map[string]any, bool) {
	c.s.scriptsMu.RLock()
	defer c.s.scriptsMu.RUnlock()
	sc, ok := c.s.Scripts[id]
	return sc, ok
}
