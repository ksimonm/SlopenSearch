package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/painless"
	"slopensearch/internal/respfmt"
)

// Non-JSON request bodies. OpenSearch parses YAML and CBOR request bodies
// with the matching XContent parser; here they are transcoded to JSON before
// routing (keeping key order), and the original format is remembered so the
// response is written in it too (see respfmt). Two headers carry that state;
// incoming copies are dropped first.

const (
	xcontentHeader    = respfmt.RequestFormatHeader
	xcontentErrHeader = "X-Slop-Request-Xcontent-Error"
)

// requestXContent returns the base media type of a body Content-Type that
// is YAML or CBOR ("yaml", "cbor"), or "".
func requestXContent(ct string) string {
	base := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch base {
	case "application/yaml", "application/x-yaml", "text/yaml", "application/yml", "application/vnd.opensearch+yaml":
		return "yaml"
	case "application/cbor", "application/vnd.opensearch+cbor":
		return "cbor"
	}
	return ""
}

// transcodeRequestBody turns a YAML or CBOR request body into JSON.
func transcodeRequestBody(r *http.Request) {
	r.Header.Del(xcontentHeader)
	r.Header.Del(xcontentErrHeader)
	if r.ContentLength == 0 || r.Body == nil {
		return
	}
	kind := requestXContent(r.Header.Get("Content-Type"))
	if kind == "" {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	var out []byte
	var terr error
	marker := kind
	if kind == "yaml" {
		if !bytes.HasPrefix(raw, []byte("---")) {
			// XContentFactory only detects YAML by its document start
			marker = "yaml-undetected"
		}
		out, terr = yamlToJSON(raw)
	} else {
		out, terr = cborToJSON(raw)
	}
	r.Header.Set(xcontentHeader, marker)
	if terr != nil {
		r.Header.Set(xcontentErrHeader, strings.ReplaceAll(terr.Error(), "\n", "\\n"))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(out))
	r.ContentLength = int64(len(out))
	r.Header.Set("Content-Length", strconv.Itoa(len(out)))
	r.Header.Set("Content-Type", "application/json")
}

// xcontentDocError is the error of a document source in YAML or CBOR that
// cannot be indexed: undetectable YAML, unparsable content, or a root that
// is not an object.
func xcontentDocError(r *http.Request, body []byte) *apierr.Error {
	kind := r.Header.Get(xcontentHeader)
	if kind == "" {
		return nil
	}
	if kind == "yaml-undetected" {
		return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
			CausedBy: &apierr.Error{Type: "not_x_content_exception", Reason: "Compressor detection can only be called on some xcontent bytes or compressed xcontent bytes"}}
	}
	if msg := r.Header.Get(xcontentErrHeader); msg != "" {
		return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
			CausedBy: &apierr.Error{Type: "json_parse_exception", Reason: strings.ReplaceAll(msg, "\\n", "\n")}}
	}
	if t := bytes.TrimSpace(body); len(t) > 0 && t[0] != '{' {
		return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "Malformed content, must start with an object"}
	}
	return nil
}

// ---------------------------------------------------------------- YAML

var (
	yamlIntDec = regexp.MustCompile(`^[-+]?(0|[1-9][0-9]*)$`)
	yamlIntOct = regexp.MustCompile(`^[-+]?0[0-7]+$`)
	yamlIntHex = regexp.MustCompile(`^[-+]?0x[0-9a-fA-F]+$`)
	yamlIntBin = regexp.MustCompile(`^[-+]?0b[01]+$`)
	yamlFloat  = regexp.MustCompile(`^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$`)
)

// yamlToJSON converts the first YAML document to JSON, resolving plain
// scalars the way Jackson's YAML parser (SnakeYAML, YAML 1.1) does.
func yamlToJSON(src []byte) ([]byte, error) {
	f, err := parser.ParseBytes(src, 0)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if len(f.Docs) == 0 || f.Docs[0].Body == nil {
		return nil, errors.New("no content")
	}
	c := &yamlConv{anchors: map[string]ast.Node{}}
	if err := c.write(&buf, f.Docs[0].Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type yamlConv struct {
	anchors map[string]ast.Node
	depth   int
}

func (c *yamlConv) write(buf *bytes.Buffer, n ast.Node) error {
	c.depth++
	defer func() { c.depth-- }()
	if c.depth > 1000 {
		return errors.New("document nested too deeply")
	}
	switch t := n.(type) {
	case nil:
		buf.WriteString("null")
	case *ast.DocumentNode:
		return c.write(buf, t.Body)
	case *ast.MappingNode:
		return c.writeMapping(buf, t.Values)
	case *ast.MappingValueNode:
		return c.writeMapping(buf, []*ast.MappingValueNode{t})
	case *ast.SequenceNode:
		buf.WriteByte('[')
		for i, v := range t.Values {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := c.write(buf, v); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case *ast.LiteralNode:
		buf.WriteString(jsonv.EncodeString(t.Value.Value))
	case *ast.AnchorNode:
		if name := t.Name.String(); name != "" {
			c.anchors[name] = t.Value
		}
		return c.write(buf, t.Value)
	case *ast.AliasNode:
		target, ok := c.anchors[t.Value.String()]
		if !ok {
			return errors.New("found undefined alias " + t.Value.String())
		}
		return c.write(buf, target)
	case *ast.TagNode:
		if t.Start != nil && (t.Start.Value == "!!str" || t.Start.Value == "!str") {
			if s, ok := t.Value.(ast.ScalarNode); ok {
				buf.WriteString(jsonv.EncodeString(yamlScalarText(s)))
				return nil
			}
		}
		return c.write(buf, t.Value)
	case *ast.CommentGroupNode, *ast.CommentNode:
		buf.WriteString("null")
	case ast.ScalarNode:
		writeYAMLScalarJSON(buf, t)
	default:
		return errors.New("unsupported YAML node " + n.Type().String())
	}
	return nil
}

func (c *yamlConv) writeMapping(buf *bytes.Buffer, values []*ast.MappingValueNode) error {
	buf.WriteByte('{')
	seen := map[string]bool{}
	for i, mv := range values {
		key := ""
		if s, ok := mv.Key.(ast.ScalarNode); ok {
			key = yamlScalarText(s)
		} else if mv.Key != nil {
			key = mv.Key.String()
		}
		if seen[key] {
			return errors.New("Duplicate Object property \"" + key + "\"")
		}
		seen[key] = true
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(jsonv.EncodeString(key))
		buf.WriteByte(':')
		if err := c.write(buf, mv.Value); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// yamlScalarText is the text of a scalar (quoted strings unescaped).
func yamlScalarText(s ast.ScalarNode) string {
	if sn, ok := s.(*ast.StringNode); ok {
		return sn.Value
	}
	if tk := s.GetToken(); tk != nil {
		return tk.Value
	}
	return s.String()
}

func writeYAMLScalarJSON(buf *bytes.Buffer, s ast.ScalarNode) {
	tk := s.GetToken()
	if tk != nil && (tk.Type == token.SingleQuoteType || tk.Type == token.DoubleQuoteType) {
		buf.WriteString(jsonv.EncodeString(yamlScalarText(s)))
		return
	}
	text := yamlScalarText(s)
	if _, ok := s.(*ast.NullNode); ok && text == "" {
		buf.WriteString("null")
		return
	}
	switch text {
	case "", "~", "null", "Null", "NULL":
		buf.WriteString("null")
		return
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		buf.WriteString("true")
		return
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		buf.WriteString("false")
		return
	}
	if v, ok := yamlInt(text); ok {
		buf.WriteString(v)
		return
	}
	if yamlFloat.MatchString(text) && strings.ContainsAny(text, ".eE") {
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			buf.WriteString(painless.FormatDouble(f))
			return
		}
	}
	buf.WriteString(jsonv.EncodeString(text))
}

// yamlInt resolves a YAML 1.1 integer (decimal, 0 octal, 0x hex, 0b binary).
func yamlInt(text string) (string, bool) {
	neg := strings.HasPrefix(text, "-")
	digits := strings.TrimLeft(text, "+-")
	base := 0
	switch {
	case yamlIntDec.MatchString(text):
		base = 10
	case yamlIntHex.MatchString(text):
		base, digits = 16, digits[2:]
	case yamlIntBin.MatchString(text):
		base, digits = 2, digits[2:]
	case yamlIntOct.MatchString(text):
		base, digits = 8, digits[1:]
	default:
		return "", false
	}
	n, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return "", false
	}
	if neg {
		n.Neg(n)
	}
	return n.String(), true
}

// ---------------------------------------------------------------- CBOR

// cborToJSON converts one CBOR data item to JSON, keeping map order.
func cborToJSON(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	rest, err := writeCBORItem(&buf, src, 0)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(rest)) > 0 {
		return nil, errors.New("trailing data after CBOR content")
	}
	return buf.Bytes(), nil
}

// cborHeader decodes the argument of an item head; count -1 means
// indefinite length.
func cborHeader(b []byte) (major byte, count int64, n int, err error) {
	if len(b) == 0 {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	major, ai := b[0]>>5, b[0]&0x1f
	switch {
	case ai < 24:
		return major, int64(ai), 1, nil
	case ai == 31:
		return major, -1, 1, nil
	case ai > 27:
		return 0, 0, 0, errors.New("invalid additional information")
	}
	size := 1 << (ai - 24)
	if len(b) < 1+size {
		return 0, 0, 0, io.ErrUnexpectedEOF
	}
	var v uint64
	switch size {
	case 1:
		v = uint64(b[1])
	case 2:
		v = uint64(binary.BigEndian.Uint16(b[1:]))
	case 4:
		v = uint64(binary.BigEndian.Uint32(b[1:]))
	default:
		v = binary.BigEndian.Uint64(b[1:])
	}
	if v > math.MaxInt32 {
		return 0, 0, 0, errors.New("container too large")
	}
	return major, int64(v), 1 + size, nil
}

func writeCBORItem(buf *bytes.Buffer, b []byte, depth int) ([]byte, error) {
	if depth > 1000 {
		return nil, errors.New("document nested too deeply")
	}
	if len(b) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	major := b[0] >> 5
	if major == 4 || major == 5 {
		_, count, n, err := cborHeader(b)
		if err != nil {
			return nil, err
		}
		b = b[n:]
		open, close := byte('['), byte(']')
		if major == 5 {
			open, close = '{', '}'
		}
		buf.WriteByte(open)
		for i := int64(0); count < 0 || i < count; i++ {
			if count < 0 {
				if len(b) == 0 {
					return nil, io.ErrUnexpectedEOF
				}
				if b[0] == 0xff {
					b = b[1:]
					break
				}
			}
			if i > 0 {
				buf.WriteByte(',')
			}
			if major == 5 {
				var key any
				rest, err := cbor.UnmarshalFirst(b, &key)
				if err != nil {
					return nil, err
				}
				buf.WriteString(jsonv.EncodeString(cborKeyString(key)))
				buf.WriteByte(':')
				b = rest
			}
			rest, err := writeCBORItem(buf, b, depth+1)
			if err != nil {
				return nil, err
			}
			b = rest
		}
		buf.WriteByte(close)
		return b, nil
	}
	if major == 6 { // tag: bignums keep their value, other tags their content
		var raw cbor.RawTag
		rest, err := cbor.UnmarshalFirst(b, &raw)
		if err != nil {
			return nil, err
		}
		if raw.Number == 2 || raw.Number == 3 {
			var n big.Int
			if err := cbor.Unmarshal(b[:len(b)-len(rest)], &n); err != nil {
				return nil, err
			}
			buf.WriteString(n.String())
			return rest, nil
		}
		if _, err := writeCBORItem(buf, raw.Content, depth+1); err != nil {
			return nil, err
		}
		return rest, nil
	}
	var v any
	rest, err := cbor.UnmarshalFirst(b, &v)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(t))
	case uint64:
		buf.WriteString(strconv.FormatUint(t, 10))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case big.Int:
		buf.WriteString(t.String())
	case float64:
		if b[0] == 0xfb {
			buf.WriteString(painless.FormatDouble(t))
		} else {
			buf.WriteString(javaFloatText(float32(t)))
		}
	case string:
		buf.WriteString(jsonv.EncodeString(t))
	case []byte:
		buf.WriteString(jsonv.EncodeString(base64.StdEncoding.EncodeToString(t)))
	case cbor.SimpleValue:
		buf.WriteString("null")
	default:
		return nil, errors.New("unsupported CBOR value")
	}
	return rest, nil
}

func cborKeyString(k any) string {
	switch t := k.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case uint64:
		return strconv.FormatUint(t, 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case nil:
		return "null"
	}
	return ""
}

// javaFloatText renders a float32 like Float.toString.
func javaFloatText(f float32) string {
	d := float64(f)
	switch {
	case math.IsNaN(d):
		return "NaN"
	case math.IsInf(d, 1):
		return "Infinity"
	case math.IsInf(d, -1):
		return "-Infinity"
	case d == 0:
		if math.Signbit(d) {
			return "-0.0"
		}
		return "0.0"
	}
	a := math.Abs(d)
	if a < 1e-3 || a >= 1e7 {
		s := strconv.FormatFloat(d, 'E', -1, 32)
		mant, exp, _ := strings.Cut(s, "E")
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		e, _ := strconv.Atoi(exp)
		return mant + "E" + strconv.Itoa(e)
	}
	s := strconv.FormatFloat(d, 'f', -1, 32)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
