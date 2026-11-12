package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Mustache templates with the semantics of OpenSearch's mustache.java
// integration: JSON escaping for {{x}}, raw {{{x}}}, lists exposed as maps
// by index, and the toJson / join / url functions.

type mustacheErr struct{ reason string }

type mNode struct {
	kind     byte // 't' text, 'v' escaped var, 'u' raw var, '#' section, '^' inverted
	text     string
	children []*mNode
	fn       string // toJson, join, url
	delim    string // join delimiter
	inner    string // raw text of the section body (for functions)
}

func mustacheParse(tpl string) ([]*mNode, *mustacheErr) {
	open, close := "{{", "}}"
	type frame struct {
		node  *mNode
		start int
	}
	root := &mNode{}
	stack := []frame{{node: root}}
	i := 0
	for i < len(tpl) {
		j := strings.Index(tpl[i:], open)
		if j < 0 {
			top := stack[len(stack)-1].node
			top.children = append(top.children, &mNode{kind: 't', text: tpl[i:]})
			break
		}
		top := stack[len(stack)-1].node
		if j > 0 {
			top.children = append(top.children, &mNode{kind: 't', text: tpl[i : i+j]})
		}
		tagStart := i + j
		i = tagStart + len(open)
		triple := false
		if open == "{{" && strings.HasPrefix(tpl[i:], "{") {
			triple = true
			i++
		}
		end := strings.Index(tpl[i:], close)
		if end < 0 {
			return nil, &mustacheErr{reason: "Improperly closed variable in query-template:1"}
		}
		tag := tpl[i : i+end]
		i += end + len(close)
		if triple {
			if strings.HasPrefix(tpl[i:], "}") {
				i++
			}
			top.children = append(top.children, &mNode{kind: 'u', text: strings.TrimSpace(tag)})
			continue
		}
		if tag == "" {
			continue
		}
		switch tag[0] {
		case '!':
			continue
		case '=':
			d := strings.TrimSpace(strings.TrimSuffix(tag[1:], "="))
			parts := strings.Fields(d)
			if len(parts) == 2 {
				open, close = parts[0], parts[1]
			}
			continue
		case '&':
			top.children = append(top.children, &mNode{kind: 'u', text: strings.TrimSpace(tag[1:])})
		case '#', '^':
			name := strings.TrimSpace(tag[1:])
			n := &mNode{kind: tag[0], text: name}
			fields := strings.Fields(name)
			if len(fields) > 0 {
				switch fields[0] {
				case "toJson", "url":
					if tag[0] == '#' && len(fields) == 1 {
						n.fn = fields[0]
					}
				case "join":
					if tag[0] == '#' {
						n.fn = "join"
						n.delim = ","
						if len(fields) > 1 {
							if k, v, ok := strings.Cut(fields[1], "="); ok && k == "delimiter" {
								n.delim = strings.Trim(v, "'\"")
							}
						}
					}
				}
			}
			top.children = append(top.children, n)
			stack = append(stack, frame{node: n, start: i})
		case '/':
			name := strings.TrimSpace(tag[1:])
			if len(stack) == 1 || stack[len(stack)-1].node.text != name {
				return nil, &mustacheErr{reason: "Mismatched start/end tags: " + name + " in query-template:1"}
			}
			f := stack[len(stack)-1]
			f.node.inner = tpl[f.start:tagStart]
			stack = stack[:len(stack)-1]
		case '>':
			continue
		default:
			top.children = append(top.children, &mNode{kind: 'v', text: strings.TrimSpace(tag)})
		}
	}
	if len(stack) > 1 {
		name := stack[len(stack)-1].node.text
		if f := strings.Fields(name); len(f) > 0 {
			name = f[0]
		}
		return nil, &mustacheErr{reason: "Failed to close '" + name + "' tag @[query-template:1]"}
	}
	return root.children, nil
}

// mustacheRender renders a template against params.
func mustacheRender(tpl string, params map[string]any) (string, *mustacheErr) {
	nodes, err := mustacheParse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	mustacheRenderNodes(&sb, nodes, []any{params})
	return sb.String(), nil
}

func mustacheLookup(stack []any, name string) (any, bool) {
	if name == "." {
		return stack[len(stack)-1], true
	}
	parts := strings.Split(name, ".")
	for i := len(stack) - 1; i >= 0; i-- {
		v, ok := mustacheGet(stack[i], parts[0])
		if !ok {
			continue
		}
		for _, p := range parts[1:] {
			v, ok = mustacheGet(v, p)
			if !ok {
				return nil, false
			}
		}
		return v, true
	}
	return nil, false
}

func mustacheGet(v any, key string) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		x, ok := t[key]
		return x, ok
	case []any:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= len(t) {
			return nil, false
		}
		return t[i], true
	}
	return nil, false
}

func mustacheTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	}
	return true
}

// mustacheString renders a value like Object.toString(); lists are exposed
// to templates as maps keyed by index.
func mustacheString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case []any:
		m := newPlMap(plLinked)
		for i, e := range t {
			m.put(itoa(i), plFromJSON(e, plLinked))
		}
		return plToString(m)
	}
	return plToString(plFromJSON(v, plHash))
}

func mustacheJSONEscape(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				sb.WriteString(`\u00`)
				sb.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)>>4, 16)))
				sb.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)&15, 16)))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

// mustacheToJSON serializes a value compactly (Jackson style).
func mustacheToJSON(v any) string {
	var buf bytes.Buffer
	var write func(v any)
	write = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			buf.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					buf.WriteByte(',')
				}
				kb, _ := json.Marshal(k)
				buf.Write(kb)
				buf.WriteByte(':')
				write(t[k])
			}
			buf.WriteByte('}')
		case []any:
			buf.WriteByte('[')
			for i, e := range t {
				if i > 0 {
					buf.WriteByte(',')
				}
				write(e)
			}
			buf.WriteByte(']')
		case json.Number:
			buf.WriteString(t.String())
		case string:
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			_ = enc.Encode(t)
			buf.Truncate(buf.Len() - 1)
		default:
			b, _ := json.Marshal(t)
			buf.Write(b)
		}
	}
	write(v)
	return buf.String()
}

func mustacheRenderNodes(sb *strings.Builder, nodes []*mNode, stack []any) {
	for _, n := range nodes {
		switch n.kind {
		case 't':
			sb.WriteString(n.text)
		case 'v':
			if v, ok := mustacheLookup(stack, n.text); ok {
				sb.WriteString(mustacheJSONEscape(mustacheString(v)))
			}
		case 'u':
			if v, ok := mustacheLookup(stack, n.text); ok {
				sb.WriteString(mustacheString(v))
			}
		case '#':
			switch n.fn {
			case "toJson":
				if v, ok := mustacheLookup(stack, strings.TrimSpace(n.inner)); ok {
					sb.WriteString(mustacheToJSON(v))
				}
				continue
			case "join":
				if v, ok := mustacheLookup(stack, strings.TrimSpace(n.inner)); ok {
					if list, ok := v.([]any); ok {
						parts := make([]string, len(list))
						for i, e := range list {
							parts[i] = mustacheString(e)
						}
						sb.WriteString(strings.Join(parts, n.delim))
					} else {
						sb.WriteString(mustacheString(v))
					}
				}
				continue
			case "url":
				var inner strings.Builder
				mustacheRenderNodes(&inner, n.children, stack)
				sb.WriteString(url.QueryEscape(inner.String()))
				continue
			}
			v, _ := mustacheLookup(stack, n.text)
			if !mustacheTruthy(v) {
				continue
			}
			if list, ok := v.([]any); ok {
				for _, e := range list {
					mustacheRenderNodes(sb, n.children, append(stack, e))
				}
				continue
			}
			mustacheRenderNodes(sb, n.children, append(stack, v))
		case '^':
			v, _ := mustacheLookup(stack, n.text)
			if !mustacheTruthy(v) {
				mustacheRenderNodes(sb, n.children, stack)
			}
		}
	}
}

// mustacheTemplateText returns the template text of a source: strings are
// used as is, objects are serialized to JSON first.
func mustacheTemplateText(source any) (string, bool) {
	switch t := source.(type) {
	case string:
		return t, true
	case map[string]any, []any:
		return mustacheToJSON(t), true
	}
	return "", false
}

// mustacheScriptError is the script_exception of a failed template.
func mustacheScriptError(src string, e *mustacheErr) *apiError {
	body := orderedJSON{{"error", orderedJSON{
		{"root_cause", []any{orderedJSON{{"type", "script_exception"}, {"reason", e.reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"}}}},
		{"type", "script_exception"}, {"reason", e.reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"},
		{"caused_by", orderedJSON{{"type", "mustache_exception"}, {"reason", e.reason}}},
	}}, {"status", 400}}
	return &apiError{Type: "script_exception", Status: 400, Reason: e.reason, Fields: map[string]any{"lang": "mustache"},
		CausedBy: &apiError{Type: "mustache_exception", Reason: e.reason},
		Shard:    &shardFailure{raw: body, status: 400}}
}
