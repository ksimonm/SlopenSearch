package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/wildcard"
)

const (
	nodeHost       = "127.0.0.1"
	nodeEphemeral  = "slopensearch-ephemeral-id"
	nodeTransport  = "127.0.0.1:9300"
	luceneVersion  = "10.5.1"
	stateUUIDConst = "slopensearch-state-uuid"
)

func nowMillis() int64 { return time.Now().UnixMilli() }

func nodeRoles() []any {
	return []any{"cluster_manager", "data", "ingest", "remote_cluster_client"}
}

func nodeAttributes() map[string]any {
	return map[string]any{"shard_indexing_pressure_enabled": "true"}
}

func nodeShortRoles() string { return "dimr" }

func paramBool(r *http.Request, name string) bool {
	v, ok := r.URL.Query()[name]
	if !ok {
		return false
	}
	if len(v) == 0 {
		return true
	}
	return v[0] == "" || v[0] == "true"
}

// resolveOpts controls index pattern resolution.
type resolveOpts struct {
	missing404     bool // explicit missing name -> 404
	wildcardClosed bool // wildcards match closed indices
	namedClosed    bool // explicit names match closed indices
}

func (s *Server) resolveIndices(spec string, opts resolveOpts) ([]*model.Index, *apierr.Error) {
	if spec == "" {
		spec = "*"
	}
	seen := map[string]*model.Index{}
	var order []string
	add := func(idx *model.Index) {
		if _, ok := seen[idx.Name]; !ok {
			seen[idx.Name] = idx
			order = append(order, idx.Name)
		}
	}
	for _, token := range strings.Split(spec, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if token == "_all" || token == "*" {
			for _, idx := range s.Indices {
				if idx.Closed && !opts.wildcardClosed {
					continue
				}
				add(idx)
			}
			continue
		}
		if strings.ContainsAny(token, "*?") {
			matched := false
			for _, idx := range s.Indices {
				if wildcard.Match(token, idx.Name) {
					if idx.Closed && !opts.wildcardClosed {
						continue
					}
					add(idx)
					matched = true
				}
			}
			if !matched {
				for _, idx := range s.Indices {
					if idx.Closed && !opts.wildcardClosed {
						continue
					}
					for alias := range idx.Aliases {
						if wildcard.Match(token, alias) {
							add(idx)
							break
						}
					}
				}
			}
			continue
		}
		if idx, ok := s.Indices[token]; ok {
			if idx.Closed && !opts.namedClosed && !opts.wildcardClosed {
				add(idx)
				continue
			}
			add(idx)
			continue
		}
		matchedAlias := false
		for _, idx := range s.Indices {
			if _, ok := idx.Aliases[token]; ok {
				add(idx)
				matchedAlias = true
			}
		}
		if matchedAlias {
			continue
		}
		if opts.missing404 {
			return nil, apierr.IndexNotFound(token)
		}
	}
	out := make([]*model.Index, 0, len(order))
	jsonv.SortStrings(order)
	for _, name := range order {
		out = append(out, seen[name])
	}
	return out, nil
}

func validateKnownParams(r *http.Request, known map[string]bool, reqPath string) *apierr.Error {
	for name := range r.URL.Query() {
		if !known[name] {
			return &apierr.Error{Type: "illegal_argument_exception",
				Reason: fmt.Sprintf("request [%s] contains unrecognized parameter: [%s]", reqPath, name), Status: 400}
		}
	}
	return nil
}

func parseTimeout(v string) (time.Duration, bool) {
	if v == "" {
		return 30 * time.Second, true
	}
	units := []struct {
		suffix string
		mult   time.Duration
	}{{"ms", time.Millisecond}, {"s", time.Second}, {"m", time.Minute}, {"h", time.Hour}, {"d", 24 * time.Hour}}
	for _, u := range units {
		if strings.HasSuffix(v, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(v, u.suffix), 64)
			if err != nil {
				return 0, false
			}
			return time.Duration(n * float64(u.mult)), true
		}
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(n * float64(time.Millisecond)), true
}

// readBodyJSON reads a JSON or YAML request body, enforcing content types.
func readBodyJSON(w http.ResponseWriter, r *http.Request) (map[string]any, *apierr.Error) {
	ctype := r.Header.Get("Content-Type")
	if r.ContentLength == 0 {
		return nil, &apierr.Error{Type: "parse_exception", Reason: "request body is required", Status: 400}
	}
	base := strings.TrimSpace(strings.Split(ctype, ";")[0])
	switch base {
	case "application/json", "application/x-ndjson":
	case "application/yaml", "application/x-yaml", "text/yaml":
		body, apiErr := readAllBody(r)
		if apiErr != nil {
			return nil, apiErr
		}
		js, err := yamlToJSON(body)
		if err != nil {
			return nil, &apierr.Error{Type: "json_parse_exception", Reason: err.Error(), Status: 400}
		}
		var out map[string]any
		if err := jsonv.Unmarshal(js, &out); err != nil {
			return nil, &apierr.Error{Type: "json_parse_exception", Reason: err.Error(), Status: 400}
		}
		return out, nil
	default:
		return nil, nil
	}
	body, apiErr := readAllBody(r)
	if apiErr != nil {
		return nil, apiErr
	}
	var out map[string]any
	if err := jsonv.Unmarshal(body, &out); err != nil {
		return nil, &apierr.Error{Type: "json_parse_exception", Reason: err.Error(), Status: 400}
	}
	return out, nil
}

func readAllBody(r *http.Request) ([]byte, *apierr.Error) {
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// -------------------------------------------------------------- close / open

func (s *Server) handleCloseOpen(w http.ResponseWriter, r *http.Request, indexSpec string, close bool) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, "POST")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o := specOpts{expand: map[string]bool{"open": true, "closed": true}}
	o = s.parseResolveParams(r, o)
	indices, apiErr := s.resolveSpec(indexSpec, o)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	out := map[string]any{}
	changed := 0
	for _, idx := range indices {
		if close {
			if idx.Closed {
				continue
			}
			idx.Closed = true
			idx.Settings["index.verified_before_close"] = "true"
			changed++
			out[idx.Name] = map[string]any{"closed": true}
		} else {
			if !idx.Closed {
				continue
			}
			idx.Closed = false
			delete(idx.Settings, "index.verified_before_close")
			changed++
		}
	}
	s.bumpState()
	if close {
		writeJSON(w, 200, map[string]any{
			"acknowledged": true, "shards_acknowledged": changed > 0, "indices": out,
		})
		return
	}
	writeJSON(w, 200, map[string]any{"acknowledged": true, "shards_acknowledged": true})
}

// -------------------------------------------------------------- server routes

func (s *Server) routeServerEndpoints(w http.ResponseWriter, r *http.Request, segs []string) bool {
	switch segs[0] {
	case "_cluster":
		if len(segs) == 1 {
			writeNoHandler(w, r)
			return true
		}
		sub := segs[1]
		switch sub {
		case "health":
			spec := ""
			if len(segs) == 3 {
				spec = segs[2]
			} else if len(segs) > 3 {
				writeNoHandler(w, r)
				return true
			}
			s.handleHealth(w, r, spec)
		case "state":
			metrics, spec := "", ""
			if len(segs) == 3 {
				metrics = segs[2]
			} else if len(segs) == 4 {
				metrics, spec = segs[2], segs[3]
			} else if len(segs) > 4 {
				writeNoHandler(w, r)
				return true
			}
			s.handleClusterState(w, r, metrics, spec)
		case "stats":
			spec := ""
			if len(segs) == 3 {
				spec = segs[2]
			} else if len(segs) == 4 && segs[2] == "nodes" {
				spec = segs[3]
			} else if len(segs) > 3 {
				writeNoHandler(w, r)
				return true
			}
			s.handleClusterStats(w, r, spec)
		case "settings":
			if len(segs) > 2 {
				writeNoHandler(w, r)
				return true
			}
			s.handleClusterSettings(w, r)
		case "allocation":
			if len(segs) == 3 && segs[2] == "explain" {
				s.handleAllocationExplain(w, r)
			} else {
				writeNoHandler(w, r)
			}
		case "pending_tasks":
			s.handlePendingTasks(w, r)
		case "voting_config_exclusions":
			writeJSON(w, 200, map[string]any{})
		case "reroute":
			writeJSON(w, 200, map[string]any{"acknowledged": true, "state": s.snapshotState()})
		default:
			writeNoHandler(w, r)
		}
		return true
	case "_nodes":
		s.handleNodes(w, r, segs[1:])
		return true
	case "_tasks":
		s.handleTasks(w, r, segs[1:])
		return true
	case "_remote":
		if len(segs) == 2 && segs[1] == "info" {
			s.handleRemoteInfo(w, r)
		} else {
			writeNoHandler(w, r)
		}
		return true
	case "_template":
		s.handleLegacyTemplate(w, r, segs[1:])
		return true
	case "_index_template":
		if len(segs) >= 2 && (segs[1] == "_simulate" || segs[1] == "_simulate_index") {
			s.handleIndexTemplateSimulate(w, r, segs[1:])
			return true
		}
		s.handleComposableTemplate(w, r, segs[1:])
		return true
	case "_component_template":
		s.handleComponentTemplate(w, r, segs[1:])
		return true
	case "_data_stream":
		if len(segs) == 1 {
			s.handleDataStream(w, r, "*")
			return true
		}
		if len(segs) == 2 {
			s.handleDataStream(w, r, segs[1])
			return true
		}
		if len(segs) == 3 && segs[2] == "_stats" {
			s.handleDataStreamStats(w, r, segs[1])
			return true
		}
		writeNoHandler(w, r)
		return true
	case "_resolve":
		if len(segs) == 3 && segs[1] == "index" {
			s.handleResolveIndex(w, r, segs[2])
			return true
		}
		writeNoHandler(w, r)
		return true
	case "_reindex":
		s.handleReindex(w, r)
		return true
	}
	return false
}

func (s *Server) snapshotState() map[string]any {
	return map[string]any{}
}
