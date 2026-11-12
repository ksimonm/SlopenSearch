package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

// The _analyze API (TransportAnalyzeAction).

// jsonKeyPos is the location of a top-level key and of its value in a JSON
// object, as Jackson reports them (1-based line and column).
type jsonKeyPos struct {
	key                string
	line, col          int
	valLine, valColumn int
}

// topLevelKeyPositions scans a JSON object and returns its top-level keys.
func topLevelKeyPositions(body []byte) []jsonKeyPos {
	var out []jsonKeyPos
	line, col := 1, 0
	depth := 0
	expectKey := false
	var pending *jsonKeyPos
	rs := []rune(string(body))
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c == '\n' {
			line++
			col = 0
			continue
		}
		col++
		switch c {
		case '{', '[':
			if depth == 1 && pending != nil {
				pending.valLine, pending.valColumn = line, col
				out = append(out, *pending)
				pending = nil
			}
			depth++
			if c == '{' && depth == 1 {
				expectKey = true
			}
		case '}', ']':
			depth--
		case ',':
			if depth == 1 {
				expectKey = true
			}
		case '"':
			startLine, startCol := line, col
			j := i + 1
			var sb strings.Builder
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' && j+1 < len(rs) {
					sb.WriteRune(rs[j+1])
					j += 2
					col += 2
					continue
				}
				sb.WriteRune(rs[j])
				j++
				col++
			}
			col++
			i = j
			if depth == 1 {
				if expectKey {
					pending = &jsonKeyPos{key: sb.String(), line: startLine, col: startCol}
					expectKey = false
				} else if pending != nil {
					pending.valLine, pending.valColumn = startLine, startCol
					out = append(out, *pending)
					pending = nil
				}
			}
		case ' ', '\t', '\r', ':':
		default:
			if depth == 1 && pending != nil {
				pending.valLine, pending.valColumn = line, col
				out = append(out, *pending)
				pending = nil
			}
		}
	}
	return out
}

func xcontentParseError(reason string, cause *apiError) *apiError {
	return &apiError{Type: "x_content_parse_exception", Status: 400, Reason: reason,
		RootCause: &apiError{Type: "x_content_parse_exception", Reason: reason}, CausedBy: cause}
}

var analyzeRequestFields = map[string]bool{
	"text": true, "analyzer": true, "tokenizer": true, "char_filter": true, "filter": true,
	"token_filter": true, "field": true, "explain": true, "attributes": true, "normalizer": true,
}

type analyzeRequest struct {
	texts       []string
	analyzer    string
	normalizer  string
	field       string
	tokName     string
	tokInline   map[string]any
	hasTok      bool
	filters     []analyzeComponent
	charFilters []analyzeComponent
	explain     bool
	attributes  []string
}

func parseAnalyzeRequest(body map[string]any, raw []byte) (*analyzeRequest, *apiError) {
	positions := topLevelKeyPositions(raw)
	pos := func(key string, value bool) string {
		for _, p := range positions {
			if p.key == key {
				if value {
					return "[" + strconv.Itoa(p.valLine) + ":" + strconv.Itoa(p.valColumn) + "]"
				}
				return "[" + strconv.Itoa(p.line) + ":" + strconv.Itoa(p.col) + "]"
			}
		}
		return "[1:2]"
	}
	for _, p := range positions {
		if !analyzeRequestFields[p.key] {
			return nil, xcontentParseError(pos(p.key, false)+" [analyze_request] unknown field ["+p.key+"]", nil)
		}
	}
	for k := range body {
		if !analyzeRequestFields[k] {
			return nil, xcontentParseError("[1:2] [analyze_request] unknown field ["+k+"]", nil)
		}
	}
	req := &analyzeRequest{}
	req.texts, _ = analyzeStringValues(body["text"])
	req.analyzer, _ = body["analyzer"].(string)
	req.normalizer, _ = body["normalizer"].(string)
	req.field, _ = body["field"].(string)
	if raw, ok := body["tokenizer"]; ok && raw != nil {
		switch t := raw.(type) {
		case string:
			req.tokName, req.hasTok = t, true
		case map[string]any:
			req.tokInline, req.hasTok = t, true
		}
	}
	var err *apiError
	filterRaw := body["filter"]
	if filterRaw == nil {
		filterRaw = body["token_filter"]
	}
	if req.filters, err = parseAnalyzeComponents(filterRaw); err != nil {
		return nil, err
	}
	if req.charFilters, err = parseAnalyzeComponents(body["char_filter"]); err != nil {
		return nil, err
	}
	if req.charFilters != nil {
		for _, c := range req.charFilters {
			if c.inline != nil {
				if _, ok := c.inline["type"].(string); !ok {
					return nil, analyzeRequestError("illegal_argument_exception", "Missing [type] setting for anonymous char_filter: "+anonymousJSON(c.inline))
				}
			}
		}
	}
	switch v := body["explain"].(type) {
	case nil:
	case bool:
		req.explain = v
	case string:
		switch v {
		case "true":
			req.explain = true
		case "false":
		default:
			return nil, xcontentParseError(pos("explain", true)+" [analyze_request] failed to parse field [explain]",
				&apiError{Type: "illegal_argument_exception", Reason: "Failed to parse value [" + v + "] as only [true] or [false] are allowed."})
		}
	default:
		return nil, xcontentParseError(pos("explain", true)+" [analyze_request] failed to parse field [explain]",
			&apiError{Type: "illegal_argument_exception", Reason: "Failed to parse value [" + anonymousJSON(map[string]any{"v": v}) + "] as only [true] or [false] are allowed."})
	}
	req.attributes = filterNames(body["attributes"])
	return req, nil
}

// validate mirrors AnalyzeAction.Request.validate().
func (req *analyzeRequest) validate(hasIndex bool) *apiError {
	var msgs []string
	if len(req.texts) == 0 {
		msgs = append(msgs, "text is missing")
	}
	if !hasIndex && req.normalizer != "" {
		msgs = append(msgs, "index is required if normalizer is specified")
	}
	if req.normalizer != "" && (req.hasTok || req.analyzer != "") {
		msgs = append(msgs, "tokenizer/analyze should be null if normalizer is specified")
	}
	if req.analyzer != "" && (req.hasTok || len(req.filters) > 0 || len(req.charFilters) > 0) {
		msgs = append(msgs, "cannot define extra components on a named analyzer")
	}
	if req.normalizer != "" && (req.hasTok || len(req.charFilters) > 0) {
		msgs = append(msgs, "cannot define extra components on a named normalizer")
	}
	if len(msgs) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("Validation Failed: ")
	for i, m := range msgs {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(": ")
		b.WriteString(m)
		b.WriteString(";")
	}
	return analyzeRequestError("action_request_validation_exception", b.String())
}

// resolveAnalyzeChain picks the analyzer for a request, in the order of
// TransportAnalyzeAction.
func resolveAnalyzeChain(sc analysisScope, req *analyzeRequest) (*analysisChain, *apiError) {
	if req.hasTok {
		return sc.buildRequestChain(req.tokName, req.tokInline, req.charFilters, req.filters)
	}
	if len(req.filters) > 0 || len(req.charFilters) > 0 {
		c, err := sc.buildRequestChain("keyword", nil, req.charFilters, req.filters)
		if err != nil {
			return nil, err
		}
		for _, f := range c.filters {
			if !normalizerFilterAllowed[f.spec.typ] {
				return nil, analyzeRequestError("illegal_argument_exception", "Custom normalizer may not use filter ["+f.name+"]")
			}
		}
		return c, nil
	}
	if req.analyzer != "" {
		return sc.analyzerChain(req.analyzer)
	}
	if req.normalizer != "" {
		if sc.global() {
			return nil, analyzeRequestError("illegal_argument_exception", "analysis based on a normalizer requires an index")
		}
		return sc.normalizerChain(req.normalizer)
	}
	if req.field != "" {
		if sc.global() {
			return nil, analyzeRequestError("illegal_argument_exception", "analysis based on a specific field requires an index")
		}
		if def, ok := mappingFieldDef(sc.idx, req.field); ok && def != nil {
			typ, _ := def["type"].(string)
			switch typ {
			case "text", "match_only_text", "search_as_you_type", "":
				var c *analysisChain
				var err *apiError
				if a, ok := def["analyzer"].(string); ok && a != "" {
					c, err = sc.analyzerChain(a)
				} else {
					c = sc.defaultIndexChain()
				}
				if err != nil {
					return nil, err
				}
				if n, ok := toFloat(def["position_increment_gap"]); ok {
					c.gap = int(n)
				}
				return c, nil
			case "keyword":
				if n, ok := def["normalizer"].(string); ok && n != "" {
					return sc.normalizerChain(n)
				}
				return sc.analyzerChain("keyword")
			default:
				return nil, analyzeRequestError("illegal_argument_exception",
					"Can't process field ["+req.field+"], Analysis requests are only supported on tokenized fields")
			}
		}
	}
	return sc.defaultIndexChain(), nil
}

// tokenListCreator accumulates tokens of several texts the way
// TransportAnalyzeAction.TokenListCreator does.
type tokenListCreator struct {
	lastPosition int
	lastOffset   int
	tokens       []analyzedToken
}

func newTokenListCreator() *tokenListCreator {
	return &tokenListCreator{lastPosition: -1}
}

func (t *tokenListCreator) add(toks []analyzedToken, posCount, finalOffset, gap int) {
	base := t.lastPosition + 1
	for _, tok := range toks {
		tok.pos += base
		tok.start += t.lastOffset
		tok.end += t.lastOffset
		t.tokens = append(t.tokens, tok)
	}
	t.lastPosition = base + posCount - 1
	t.lastOffset += finalOffset
	t.lastPosition += gap
	t.lastOffset++
}

type tokenAttrs struct {
	keyword bool
	include map[string]bool
}

func tokenJSON(t analyzedToken, attrs *tokenAttrs) orderedJSON {
	typ := t.typ
	if typ == "" {
		typ = "<ALPHANUM>"
	}
	o := orderedJSON{{"token", t.term}, {"start_offset", t.start}, {"end_offset", t.end}, {"type", typ}, {"position", t.pos}}
	posLen := t.posLen
	if posLen < 1 {
		posLen = 1
	}
	if attrs == nil {
		if posLen > 1 {
			o = append(o, orderedKV{"positionLength", posLen})
		}
		return o
	}
	want := func(k string) bool {
		return len(attrs.include) == 0 || attrs.include[strings.ToLower(k)]
	}
	if want("bytes") {
		o = append(o, orderedKV{"bytes", tokenBytes(t.term)})
	}
	if attrs.keyword && want("keyword") {
		o = append(o, orderedKV{"keyword", t.keyword})
	}
	if want("positionLength") {
		o = append(o, orderedKV{"positionLength", posLen})
	}
	if want("termFrequency") {
		o = append(o, orderedKV{"termFrequency", 1})
	}
	return o
}

func tokensJSON(toks []analyzedToken, attrs *tokenAttrs) []any {
	out := make([]any, 0, len(toks))
	for _, t := range toks {
		out = append(out, tokenJSON(t, attrs))
	}
	return out
}

func tokenLimitError(limit int) *apiError {
	reason := "The number of tokens produced by calling _analyze has exceeded the allowed maximum of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.analyze.max_token_count] index level setting."
	return &apiError{Type: "illegal_state_exception", Status: 500, Reason: reason,
		RootCause: &apiError{Type: "illegal_state_exception", Status: 500, Reason: reason}}
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request, name string) {
	var idx *Index
	if name != "" {
		s.mu.RLock()
		var apiErr *apiError
		idx, apiErr = s.getIndex(name)
		s.mu.RUnlock()
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, errBad("failed to read body"))
		return
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		src := r.URL.Query().Get("source")
		if src == "" {
			writeError(w, &apiError{Type: "parse_exception", Status: 400, Reason: "request body or source parameter is required"})
			return
		}
		raw = []byte(src)
	}
	var body map[string]any
	if uerr := unmarshalJSON(raw, &body); uerr != nil {
		writeError(w, &apiError{Type: "json_parse_exception", Status: 400, Reason: "Failed to parse request body: " + uerr.Error()})
		return
	}
	req, apiErr := parseAnalyzeRequest(body, raw)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if verr := req.validate(idx != nil); verr != nil {
		writeError(w, verr)
		return
	}
	sc := analysisScope{idx: idx}
	chain, apiErr := resolveAnalyzeChain(sc, req)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	limit := 10000
	if idx != nil {
		if n, ok := settingInt(idx.Settings, "analyze.max_token_count"); ok {
			limit = n
		}
	}
	if req.explain {
		s.writeAnalyzeExplain(w, idx, chain, req, limit)
		return
	}
	tl := newTokenListCreator()
	for _, text := range req.texts {
		res, rerr := chain.run(idx, text, false)
		if rerr != nil {
			writeError(w, rerr)
			return
		}
		tl.add(res.toks, res.posCount, res.finalOffset, chain.gap)
		if len(tl.tokens) > limit {
			writeError(w, tokenLimitError(limit))
			return
		}
	}
	writeJSON(w, 200, orderedJSON{{"tokens", tokensJSON(tl.tokens, nil)}})
}

func (s *Server) writeAnalyzeExplain(w http.ResponseWriter, idx *Index, chain *analysisChain, req *analyzeRequest, limit int) {
	include := map[string]bool{}
	for _, a := range req.attributes {
		include[strings.ToLower(a)] = true
	}
	if !chain.custom {
		tl := newTokenListCreator()
		for _, text := range req.texts {
			res, err := chain.run(idx, text, false)
			if err != nil {
				writeError(w, err)
				return
			}
			tl.add(res.toks, res.posCount, res.finalOffset, chain.gap)
			if len(tl.tokens) > limit {
				writeError(w, tokenLimitError(limit))
				return
			}
		}
		kw := chain.keywordAttr
		for _, f := range chain.filters {
			if filterHasKeywordAttr(f.spec.typ) {
				kw = true
			}
		}
		name := chain.name
		writeJSON(w, 200, orderedJSON{{"detail", orderedJSON{
			{"custom_analyzer", false},
			{"analyzer", orderedJSON{{"name", name}, {"tokens", tokensJSON(tl.tokens, &tokenAttrs{keyword: kw, include: include})}}},
		}}})
		return
	}
	charTexts := make([][]string, len(chain.chars))
	stages := make([]*tokenListCreator, len(chain.filters)+1)
	for i := range stages {
		stages[i] = newTokenListCreator()
	}
	for _, text := range req.texts {
		res, err := chain.run(idx, text, true)
		if err != nil {
			writeError(w, err)
			return
		}
		for i := range chain.chars {
			charTexts[i] = append(charTexts[i], res.charTexts[i])
		}
		for i := range stages {
			stages[i].add(res.stages[i], res.stageCounts[i], res.finalOffset, 0)
			if len(stages[i].tokens) > limit {
				writeError(w, tokenLimitError(limit))
				return
			}
		}
	}
	cfs := make([]any, 0, len(chain.chars))
	for i, cf := range chain.chars {
		texts := make([]any, 0, len(charTexts[i]))
		for _, t := range charTexts[i] {
			texts = append(texts, t)
		}
		cfs = append(cfs, orderedJSON{{"name", cf.name}, {"filtered_text", texts}})
	}
	tfs := make([]any, 0, len(chain.filters))
	kw := false
	for i, f := range chain.filters {
		if filterHasKeywordAttr(f.spec.typ) {
			kw = true
		}
		tfs = append(tfs, orderedJSON{{"name", f.name}, {"tokens", tokensJSON(stages[i+1].tokens, &tokenAttrs{keyword: kw, include: include})}})
	}
	writeJSON(w, 200, orderedJSON{{"detail", orderedJSON{
		{"custom_analyzer", true},
		{"charfilters", cfs},
		{"tokenizer", orderedJSON{{"name", chain.tokName}, {"tokens", tokensJSON(stages[0].tokens, &tokenAttrs{include: include})}}},
		{"tokenfilters", tfs},
	}}})
}
