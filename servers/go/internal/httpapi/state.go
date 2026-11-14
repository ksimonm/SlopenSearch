package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/ingest"
	"slopensearch/internal/jsonv"
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

func analyzeWith(analyzer, text string) []string {
	switch analyzer {
	case "keyword":
		return []string{text}
	case "whitespace":
		return strings.Fields(text)
	case "lowercase":
		return []string{strings.ToLower(text)}
	default:
		return mapping.Analyze(text)
	}
}

func validateMapping(mappings map[string]any) error {
	props, ok := mappings["properties"].(map[string]any)
	if !ok {
		return nil
	}
	return mapping.ValidateProps(props)
}

func validateDoc(props map[string]any, src map[string]any) error {
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		val, present := src[name]
		if !present {
			continue
		}
		t, _ := def["type"].(string)
		switch t {
		case "integer", "long", "short", "byte", "float", "double", "half_float", "scaled_float":
			if _, ok := jsonv.ToFloat(val); !ok {
				return fmt.Errorf("failed to parse field [%s] of type [%s]", name, t)
			}
		case "boolean":
			switch b := val.(type) {
			case bool:
			case string:
				if b != "true" && b != "false" {
					return fmt.Errorf("failed to parse field [%s] of type [boolean]", name)
				}
			default:
				return fmt.Errorf("failed to parse field [%s] of type [boolean]", name)
			}
		case "nested", "object":
			if sub, ok := def["properties"].(map[string]any); ok {
				switch v := val.(type) {
				case []any:
					for _, el := range v {
						if m, ok := el.(map[string]any); ok {
							if err := validateDoc(sub, m); err != nil {
								return err
							}
						}
					}
				case map[string]any:
					if err := validateDoc(sub, v); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func normalizeSettings(settings map[string]any) map[string]any {
	index := map[string]any{}
	for k, v := range settings {
		if k == "index" {
			if m, ok := v.(map[string]any); ok {
				for ik, iv := range m {
					index[ik] = stringifyScalar(iv)
				}
			}
			continue
		}
		index[k] = stringifyScalar(v)
	}
	return map[string]any{"index": index}
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

func inferType(v any) map[string]any {
	switch t := v.(type) {
	case bool:
		return map[string]any{"type": "boolean"}
	case float64:
		if t == math.Trunc(t) {
			return map[string]any{"type": "long"}
		}
		return map[string]any{"type": "float"}
	case json.Number:
		if strings.ContainsAny(t.String(), ".eE") {
			return map[string]any{"type": "float"}
		}
		return map[string]any{"type": "long"}
	case string:
		if dates.Pattern.MatchString(t) {
			return map[string]any{"type": "date"}
		}
		return map[string]any{
			"type": "text",
			"fields": map[string]any{
				"keyword": map[string]any{"type": "keyword", "ignore_above": 256},
			},
		}
	case []any:
		for _, e := range t {
			if e != nil {
				return inferType(e)
			}
		}
		return nil
	case map[string]any:
		props := map[string]any{}
		for k, val := range t {
			if sub := inferType(val); sub != nil {
				props[k] = sub
			}
		}
		return map[string]any{"properties": props}
	}
	return nil
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
