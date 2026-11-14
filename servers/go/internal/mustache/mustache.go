// Package mustache renders Mustache templates with the semantics of
// OpenSearch's mustache.java integration: JSON escaping for {{x}}, raw
// {{{x}}}, lists exposed as maps by index, and the toJson / join / url
// functions.
package mustache

import (
	"bytes"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"slopensearch/internal/painless"
)

// Error is a template error (a mustache_exception in OpenSearch).
type Error struct{ Reason string }

func (e *Error) Error() string { return e.Reason }

type node struct {
	kind     byte // 't' text, 'v' escaped var, 'u' raw var, '#' section, '^' inverted
	text     string
	children []*node
	fn       string // toJson, join, url
	delim    string // join delimiter
	inner    string // raw text of the section body (for functions)
}

func parse(tpl string) ([]*node, *Error) {
	open, close := "{{", "}}"
	type frame struct {
		node  *node
		start int
	}
	root := &node{}
	stack := []frame{{node: root}}
	i := 0
	for i < len(tpl) {
		j := strings.Index(tpl[i:], open)
		if j < 0 {
			top := stack[len(stack)-1].node
			top.children = append(top.children, &node{kind: 't', text: tpl[i:]})
			break
		}
		top := stack[len(stack)-1].node
		if j > 0 {
			top.children = append(top.children, &node{kind: 't', text: tpl[i : i+j]})
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
			return nil, &Error{Reason: "Improperly closed variable in query-template:1"}
		}
		tag := tpl[i : i+end]
		i += end + len(close)
		if triple {
			if strings.HasPrefix(tpl[i:], "}") {
				i++
			}
			top.children = append(top.children, &node{kind: 'u', text: strings.TrimSpace(tag)})
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
			top.children = append(top.children, &node{kind: 'u', text: strings.TrimSpace(tag[1:])})
		case '#', '^':
			name := strings.TrimSpace(tag[1:])
			n := &node{kind: tag[0], text: name}
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
				return nil, &Error{Reason: "Mismatched start/end tags: " + name + " in query-template:1"}
			}
			f := stack[len(stack)-1]
			f.node.inner = tpl[f.start:tagStart]
			stack = stack[:len(stack)-1]
		case '>':
			continue
		default:
			top.children = append(top.children, &node{kind: 'v', text: strings.TrimSpace(tag)})
		}
	}
	if len(stack) > 1 {
		name := stack[len(stack)-1].node.text
		if f := strings.Fields(name); len(f) > 0 {
			name = f[0]
		}
		return nil, &Error{Reason: "Failed to close '" + name + "' tag @[query-template:1]"}
	}
	return root.children, nil
}

// Render renders a template against params.
func Render(tpl string, params map[string]any) (string, *Error) {
	nodes, err := parse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	renderNodes(&sb, nodes, []any{params})
	return sb.String(), nil
}

func lookup(stack []any, name string) (any, bool) {
	if name == "." {
		return stack[len(stack)-1], true
	}
	parts := strings.Split(name, ".")
	for i := len(stack) - 1; i >= 0; i-- {
		v, ok := get(stack[i], parts[0])
		if !ok {
			continue
		}
		for _, p := range parts[1:] {
			v, ok = get(v, p)
			if !ok {
				return nil, false
			}
		}
		return v, true
	}
	return nil, false
}

func get(v any, key string) (any, bool) {
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

func truthy(v any) bool {
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

// toString renders a value like Object.toString(); lists are exposed
// to templates as maps keyed by index.
func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case []any:
		m := painless.NewMap(painless.Linked)
		for i, e := range t {
			m.Put(strconv.Itoa(i), painless.FromJSON(e, painless.Linked))
		}
		return painless.ToString(m)
	}
	return painless.ToString(painless.FromJSON(v, painless.Hash))
}

func jsonEscape(s string) string {
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

// toJSON serializes a value compactly (Jackson style).
func toJSON(v any) string {
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

func renderNodes(sb *strings.Builder, nodes []*node, stack []any) {
	for _, n := range nodes {
		switch n.kind {
		case 't':
			sb.WriteString(n.text)
		case 'v':
			if v, ok := lookup(stack, n.text); ok {
				sb.WriteString(jsonEscape(toString(v)))
			}
		case 'u':
			if v, ok := lookup(stack, n.text); ok {
				sb.WriteString(toString(v))
			}
		case '#':
			switch n.fn {
			case "toJson":
				if v, ok := lookup(stack, strings.TrimSpace(n.inner)); ok {
					sb.WriteString(toJSON(v))
				}
				continue
			case "join":
				if v, ok := lookup(stack, strings.TrimSpace(n.inner)); ok {
					if list, ok := v.([]any); ok {
						parts := make([]string, len(list))
						for i, e := range list {
							parts[i] = toString(e)
						}
						sb.WriteString(strings.Join(parts, n.delim))
					} else {
						sb.WriteString(toString(v))
					}
				}
				continue
			case "url":
				var inner strings.Builder
				renderNodes(&inner, n.children, stack)
				sb.WriteString(url.QueryEscape(inner.String()))
				continue
			}
			v, _ := lookup(stack, n.text)
			if !truthy(v) {
				continue
			}
			if list, ok := v.([]any); ok {
				for _, e := range list {
					renderNodes(sb, n.children, append(stack, e))
				}
				continue
			}
			renderNodes(sb, n.children, append(stack, v))
		case '^':
			v, _ := lookup(stack, n.text)
			if !truthy(v) {
				renderNodes(sb, n.children, stack)
			}
		}
	}
}

// TemplateText returns the template text of a source: strings are
// used as is, objects are serialized to JSON first.
func TemplateText(source any) (string, bool) {
	switch t := source.(type) {
	case string:
		return t, true
	case map[string]any, []any:
		return toJSON(t), true
	}
	return "", false
}
