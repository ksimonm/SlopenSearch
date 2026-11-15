// Package ojson has the JSON building blocks of the server's responses:
// objects that keep their key order and pre-encoded values.
package ojson

import (
	"bytes"
	"encoding/json"
)

// KV is one member of an Object. It is an alias of an unnamed struct type so
// that other packages can write unkeyed Object literals.
type KV = struct {
	K string
	V any
}

// Object is a JSON object that keeps the order of its members.
type Object []KV

// Verbatim is already encoded JSON, written as is.
type Verbatim []byte

func (v Verbatim) MarshalJSON() ([]byte, error) { return v, nil }

// Index returns the position of the first member named key, or -1.
func (o Object) Index(key string) int {
	for i, kv := range o {
		if kv.K == key {
			return i
		}
	}
	return -1
}

func (o Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(Marshal(kv.K))
		b.WriteByte(':')
		switch t := kv.V.(type) {
		case Verbatim:
			b.Write(t)
		case json.RawMessage:
			b.Write(t)
		default:
			var vb bytes.Buffer
			enc := json.NewEncoder(&vb)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(kv.V); err != nil {
				return nil, err
			}
			b.Write(bytes.TrimRight(vb.Bytes(), "\n"))
		}
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Marshal encodes v like encoding/json without HTML escaping and without the
// trailing newline; errors yield whatever was encoded.
func Marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// JacksonEscapes rewrites the string escapes of JSON text encoded by
// encoding/json to the ones Jackson writes: \u escapes with upper-case hex
// (\u001F), and U+2028 / U+2029 unescaped. Use it only on text that passes
// nothing through verbatim (a _source is returned as it was sent).
func JacksonEscapes(b []byte) []byte {
	i := bytes.IndexByte(b, '\\')
	if i < 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	out = append(out, b[:i]...)
	for i < len(b) {
		j := bytes.IndexByte(b[i:], '\\')
		if j < 0 {
			break
		}
		out = append(out, b[i:i+j]...)
		i += j
		if i+1 >= len(b) {
			break
		}
		if b[i+1] != 'u' || i+6 > len(b) {
			out = append(out, b[i], b[i+1])
			i += 2
			continue
		}
		switch string(b[i+2 : i+6]) {
		case "2028":
			out = append(out, " "...)
		case "2029":
			out = append(out, " "...)
		default:
			out = append(out, '\\', 'u')
			out = append(out, bytes.ToUpper(b[i+2:i+6])...)
		}
		i += 6
	}
	return append(out, b[i:]...)
}
