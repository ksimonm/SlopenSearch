package respfmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/ojson"
)

// Process applies filter_path, human, error_trace, pretty and the
// yaml format to a JSON response body.
//
// h is the response header (Content-Type is adjusted), ordered reports a body
// the handler already rendered in its final form, version is the node version
// for the human readable settings.
func Process(h http.Header, ordered bool, r *http.Request, status int, body []byte, version string) []byte {
	q := r.URL.Query()
	fp := q.Get("filter_path")
	_, hasPretty := q["pretty"]
	pretty := (hasPretty && q.Get("pretty") != "false") || q.Get("pretty") == "true"
	trace := status >= 400 && q.Get("error_trace") == "true"
	format := responseFormat(r)
	yaml := format == "yaml"
	cborOut := format == "cbor"
	human := q.Get("human") == "true" || (q.Has("human") && q.Get("human") == "")
	filter := fp != "" && status < 400
	if format == "smile" {
		h.Set("Content-Type", "application/smile")
	}
	if cborOut {
		tree, err := parseResp(body, false)
		if err != nil {
			return bytes.TrimRight(body, "\n")
		}
		if human && status < 400 {
			tree = humanize(tree, r, version)
		}
		if filter {
			tree = reorderLike(tree, FilterPath(toPlain(tree), fp))
		}
		if trace {
			tree = addStackTraces(tree)
		}
		h.Set("Content-Type", "application/cbor")
		return toCBOR(tree)
	}
	if ordered && !filter && !trace && !yaml {
		return body // already rendered in its final form
	}
	if !filter && !trace && !yaml && !pretty && !human {
		return bytes.TrimRight(body, "\n")
	}
	tree, err := parseResp(body, pretty && !filter && !yaml)
	if err != nil {
		return bytes.TrimRight(body, "\n")
	}
	if human && status < 400 {
		tree = humanize(tree, r, version)
	}
	if filter {
		tree = reorderLike(tree, FilterPath(toPlain(tree), fp))
	}
	if trace {
		tree = addStackTraces(tree)
	}
	if yaml {
		h.Set("Content-Type", "application/yaml")
		return toYAMLOrdered(tree)
	}
	if pretty {
		return prettyJSON(tree)
	}
	var buf bytes.Buffer
	writeCompactOrdered(&buf, tree)
	return buf.Bytes()
}

// Response post-processing (filter_path, human, error_trace, pretty, yaml)
// on an order-preserving view of the handler's JSON, so the key order the
// handler wrote survives every transformation.

// parseResp decodes JSON keeping object key order (objects become
// ojson.Object, numbers json.Number). With rawSource, a "_source" object is
// kept as its verbatim bytes, the way OpenSearch copies stored sources
// into pretty output.
func parseResp(b []byte, rawSource bool) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := parseRespValue(dec, b, rawSource)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data")
	}
	return v, nil
}

func parseRespValue(dec *json.Decoder, src []byte, rawSource bool) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch d {
	case '{':
		o := ojson.Object{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := kt.(string)
			if rawSource && k == "_source" {
				start := dec.InputOffset()
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return nil, err
				}
				if len(raw) > 0 && raw[0] == '{' {
					// keep the bytes exactly as written (after the colon)
					seg := bytes.TrimLeft(src[start:dec.InputOffset()], " \t\r\n:")
					o = append(o, ojson.KV{k, ojson.Verbatim(append([]byte(nil), seg...))})
					continue
				}
				v, err := parseResp(raw, false)
				if err != nil {
					return nil, err
				}
				o = append(o, ojson.KV{k, v})
				continue
			}
			v, err := parseRespValue(dec, src, rawSource)
			if err != nil {
				return nil, err
			}
			o = append(o, ojson.KV{k, v})
		}
		_, err := dec.Token()
		return o, err
	default: // '['
		a := []any{}
		for dec.More() {
			v, err := parseRespValue(dec, src, rawSource)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err := dec.Token()
		return a, err
	}
}

// toPlain converts an ordered tree to plain maps (for filter_path).
func toPlain(v any) any {
	switch t := v.(type) {
	case ojson.Object:
		m := make(map[string]any, len(t))
		for _, kv := range t {
			m[kv.K] = toPlain(kv.V)
		}
		return m
	case []any:
		a := make([]any, len(t))
		for i, e := range t {
			a[i] = toPlain(e)
		}
		return a
	case ojson.Verbatim:
		var x any
		_ = jsonv.Unmarshal(t, &x)
		return x
	}
	return v
}

// reorderLike gives a filtered plain tree the key order of the original.
func reorderLike(orig, filtered any) any {
	switch f := filtered.(type) {
	case map[string]any:
		out := ojson.Object{}
		if o, ok := orig.(ojson.Object); ok {
			seen := map[string]bool{}
			for _, kv := range o {
				if v, has := f[kv.K]; has && !seen[kv.K] {
					seen[kv.K] = true
					out = append(out, ojson.KV{kv.K, reorderLike(kv.V, v)})
				}
			}
			for _, k := range jsonv.SortedKeys(f) {
				if !seen[k] {
					out = append(out, ojson.KV{k, reorderLike(nil, f[k])})
				}
			}
			return out
		}
		for _, k := range jsonv.SortedKeys(f) {
			out = append(out, ojson.KV{k, reorderLike(nil, f[k])})
		}
		return out
	case []any:
		o, _ := orig.([]any)
		out := make([]any, len(f))
		for i, e := range f {
			var oe any
			if len(o) == len(f) {
				oe = o[i]
			}
			out[i] = reorderLike(oe, e)
		}
		return out
	}
	return filtered
}

// writeCompactOrdered renders an ordered tree as compact JSON.
func writeCompactOrdered(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case ojson.Object:
		buf.WriteByte('{')
		for i, kv := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(jsonv.EncodeString(kv.K))
			buf.WriteByte(':')
			writeCompactOrdered(buf, kv.V)
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCompactOrdered(buf, e)
		}
		buf.WriteByte(']')
	case ojson.Verbatim:
		buf.Write(t)
	case string:
		buf.WriteString(jsonv.EncodeString(t))
	case json.Number:
		buf.WriteString(t.String())
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		buf.Write(ojson.Marshal(t))
	}
}

func writePrettyOrdered(buf *bytes.Buffer, o ojson.Object, indent int) {
	if len(o) == 0 {
		buf.WriteString("{ }")
		return
	}
	buf.WriteString("{\n")
	for i, kv := range o {
		writeIndent(buf, indent+1)
		buf.WriteString(jsonv.EncodeString(kv.K))
		if raw, ok := kv.V.(ojson.Verbatim); ok {
			buf.WriteByte(':')
			buf.Write(raw)
		} else {
			buf.WriteString(" : ")
			WritePretty(buf, kv.V, indent+1)
		}
		if i < len(o)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	writeIndent(buf, indent)
	buf.WriteByte('}')
}

// javaClassName turns an exception type (index_not_found_exception) into
// its Java simple class name (IndexNotFoundException).
func javaClassName(typ string) string {
	var b strings.Builder
	for _, p := range strings.Split(typ, "_") {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}

var javaLangExceptions = map[string]bool{
	"illegal_argument_exception": true, "illegal_state_exception": true,
	"number_format_exception": true, "null_pointer_exception": true,
	"array_index_out_of_bounds_exception": true, "unsupported_operation_exception": true,
	"arithmetic_exception": true, "class_cast_exception": true,
}

// stackTraceFor renders a Java-like stack trace for an error object.
func stackTraceFor(e ojson.Object) string {
	get := func(k string) string {
		if i := e.Index(k); i >= 0 {
			if s, ok := e[i].V.(string); ok {
				return s
			}
		}
		return ""
	}
	typ, reason := get("type"), get("reason")
	head := javaClassName(typ) + "[" + reason + "]"
	if javaLangExceptions[typ] {
		head = "OpenSearchException[" + reason + "]; nested: " + head + ";"
	} else if idx := get("index"); idx != "" {
		head = "[" + idx + "] " + head
	}
	return head + "\n\tat org.opensearch.rest.BytesRestResponse.build(BytesRestResponse.java:188)" +
		"\n\tat org.opensearch.rest.BytesRestResponse.<init>(BytesRestResponse.java:148)" +
		"\n\tat org.opensearch.rest.RestController.dispatchRequest(RestController.java:370)"
}

// addStackTraces appends stack_trace to an error body's error object and
// its root causes, as error_trace=true does.
func addStackTraces(tree any) any {
	top, ok := tree.(ojson.Object)
	if !ok {
		return tree
	}
	i := top.Index("error")
	if i < 0 {
		return tree
	}
	errObj, ok := top[i].V.(ojson.Object)
	if !ok {
		return tree
	}
	if rc := errObj.Index("root_cause"); rc >= 0 {
		if list, ok := errObj[rc].V.([]any); ok {
			for j, c := range list {
				if co, ok := c.(ojson.Object); ok {
					list[j] = append(co, ojson.KV{"stack_trace", stackTraceFor(co)})
				}
			}
		}
	}
	errObj = append(errObj, ojson.KV{"stack_trace", stackTraceFor(errObj)})
	top[i].V = errObj
	return top
}

// humanize adds the human readable twins OpenSearch writes with
// human=true for index stats (sizes) and index settings (dates, versions).
func humanize(tree any, r *http.Request, version string) any {
	path := strings.Trim(r.URL.Path, "/")
	segs := strings.Split(path, "/")
	hasSeg := func(s string) bool {
		for _, x := range segs {
			if x == s {
				return true
			}
		}
		return false
	}
	switch {
	case hasSeg("_stats") && !strings.HasPrefix(path, "_nodes") && !strings.HasPrefix(path, "_cluster"):
		return humanizeStats(tree)
	case hasSeg("_settings") && !strings.HasPrefix(path, "_cluster") && !strings.HasPrefix(path, "_nodes"):
		return humanizeSettings(tree, false, version)
	}
	return tree
}

func humanizeStats(v any) any {
	switch t := v.(type) {
	case ojson.Object:
		out := make(ojson.Object, 0, len(t))
		for _, kv := range t {
			if strings.HasSuffix(kv.K, "_in_bytes") {
				base := strings.TrimSuffix(kv.K, "_in_bytes")
				if t.Index(base) < 0 {
					if n, ok := jsonv.ToFloat(kv.V); ok {
						out = append(out, ojson.KV{base, humanBytes(int64(n))})
					}
				}
			}
			out = append(out, ojson.KV{kv.K, humanizeStats(kv.V)})
		}
		return out
	case []any:
		for i, e := range t {
			t[i] = humanizeStats(e)
		}
	}
	return v
}

// humanBytes formats a byte count like ByteSizeValue.toString.
func humanBytes(n int64) string {
	units := []struct {
		suffix string
		size   float64
	}{{"pb", 1 << 50}, {"tb", 1 << 40}, {"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}}
	f := float64(n)
	for _, u := range units {
		if f >= u.size {
			s := fmt.Sprintf("%.1f", f/u.size)
			s = strings.TrimSuffix(s, ".0")
			return s + u.suffix
		}
	}
	return fmt.Sprintf("%db", n)
}

func humanizeSettings(v any, inIndex bool, version string) any {
	t, ok := v.(ojson.Object)
	if !ok {
		return v
	}
	out := make(ojson.Object, 0, len(t))
	for _, kv := range t {
		val := kv.V
		switch {
		case inIndex && kv.K == "creation_date":
			if s, ok := val.(string); ok {
				var ms int64
				if _, err := fmt.Sscan(s, &ms); err == nil {
					out = append(out, ojson.KV{"creation_date_string",
						time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")})
				}
			}
		case inIndex && kv.K == "version":
			if vo, ok := val.(ojson.Object); ok {
				nv := make(ojson.Object, 0, len(vo)+2)
				for _, x := range vo {
					if x.K == "created" || x.K == "upgraded" {
						nv = append(nv, ojson.KV{x.K + "_string", version})
					}
					nv = append(nv, x)
				}
				val = nv
			}
		default:
			val = humanizeSettings(val, kv.K == "index", version)
		}
		out = append(out, ojson.KV{kv.K, val})
	}
	return out
}

// yamlNeedsQuotes reports whether a YAML key must be quoted the way
// Jackson's YAML generator quotes it.
func yamlNeedsQuotes(k string) bool {
	switch strings.ToLower(k) {
	case "y", "n", "yes", "no", "true", "false", "on", "off", "null", "~", "":
		return true
	}
	return strings.ContainsAny(k, ":#{}[],&*!|>'\"%@`") || strings.HasPrefix(k, "-") || strings.HasPrefix(k, " ")
}

// toYAMLOrdered renders an ordered tree as Jackson's YAML generator does.
func toYAMLOrdered(v any) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	writeYAMLNode(&buf, v, 0)
	return buf.Bytes()
}

func yamlKey(k string) string {
	if yamlNeedsQuotes(k) {
		return jsonv.EncodeString(k)
	}
	return k
}

func asOrdered(v any) (ojson.Object, bool) {
	switch t := v.(type) {
	case ojson.Object:
		return t, true
	case map[string]any:
		o := ojson.Object{}
		for _, k := range jsonv.SortedKeys(t) {
			o = append(o, ojson.KV{k, t[k]})
		}
		return o, true
	case ojson.Verbatim:
		x, err := parseResp(t, false)
		if err == nil {
			return asOrdered(x)
		}
	}
	return nil, false
}

func writeYAMLNode(buf *bytes.Buffer, v any, indent int) {
	if o, ok := asOrdered(v); ok {
		if len(o) == 0 {
			writeIndent(buf, indent)
			buf.WriteString("{}\n")
			return
		}
		for _, kv := range o {
			writeIndent(buf, indent)
			buf.WriteString(yamlKey(kv.K))
			buf.WriteByte(':')
			writeYAMLValue(buf, kv.V, indent)
		}
		return
	}
	if a, ok := v.([]any); ok {
		if len(a) == 0 {
			writeIndent(buf, indent)
			buf.WriteString("[]\n")
			return
		}
		for _, e := range a {
			var item bytes.Buffer
			writeYAMLNode(&item, e, indent+1)
			b := item.Bytes()
			b = bytes.TrimPrefix(b, bytes.Repeat([]byte("  "), indent+1))
			writeIndent(buf, indent)
			buf.WriteString("- ")
			buf.Write(b)
		}
		return
	}
	writeIndent(buf, indent)
	writeYAMLScalar(buf, v)
	buf.WriteByte('\n')
}

func writeYAMLValue(buf *bytes.Buffer, v any, indent int) {
	if o, ok := asOrdered(v); ok {
		if len(o) == 0 {
			buf.WriteString(" {}\n")
			return
		}
		buf.WriteByte('\n')
		writeYAMLNode(buf, o, indent+1)
		return
	}
	if a, ok := v.([]any); ok {
		if len(a) == 0 {
			buf.WriteString(" []\n")
			return
		}
		buf.WriteByte('\n')
		writeYAMLNode(buf, a, indent)
		return
	}
	buf.WriteByte(' ')
	writeYAMLScalar(buf, v)
	buf.WriteByte('\n')
}
