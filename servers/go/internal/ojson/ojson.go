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
