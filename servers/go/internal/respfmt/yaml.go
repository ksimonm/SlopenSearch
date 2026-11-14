package respfmt

import (
	"bytes"
	"encoding/json"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/ojson"
)

// YAML renders a decoded JSON value as YAML (format=yaml on plain maps).
func YAML(v any) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	writeYAML(&buf, v, 0)
	return buf.Bytes()
}

func writeYAML(buf *bytes.Buffer, v any, indent int) {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range jsonv.SortedKeys(t) {
			for i := 0; i < indent; i++ {
				buf.WriteString("  ")
			}
			buf.WriteString(k)
			buf.WriteByte(':')
			switch child := t[k].(type) {
			case map[string]any:
				if len(child) == 0 {
					buf.WriteString(" {}\n")
				} else {
					buf.WriteByte('\n')
					writeYAML(buf, child, indent+1)
				}
			case []any:
				if len(child) == 0 {
					buf.WriteString(" []\n")
				} else {
					buf.WriteByte('\n')
					writeYAML(buf, child, indent)
				}
			default:
				buf.WriteByte(' ')
				writeYAMLScalar(buf, child)
				buf.WriteByte('\n')
			}
		}
	case []any:
		for _, e := range t {
			for i := 0; i < indent; i++ {
				buf.WriteString("  ")
			}
			buf.WriteString("- ")
			if m, ok := e.(map[string]any); ok {
				keys := jsonv.SortedKeys(m)
				for i, k := range keys {
					if i > 0 {
						for j := 0; j < indent+1; j++ {
							buf.WriteString("  ")
						}
					}
					buf.WriteString(k)
					buf.WriteString(": ")
					writeYAMLScalar(buf, m[k])
					buf.WriteByte('\n')
				}
			} else {
				writeYAMLScalar(buf, e)
				buf.WriteByte('\n')
			}
		}
	default:
		writeYAMLScalar(buf, v)
		buf.WriteByte('\n')
	}
}

func writeYAMLScalar(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case map[string]any, []any:
		b := string(ojson.Marshal(t))
		buf.WriteString(b)
	case string:
		buf.WriteString(jsonv.EncodeString(t))
	case json.Number:
		buf.WriteString(t.String())
	case float64:
		buf.WriteString(jsonv.FormatNumber(t))
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		buf.WriteString(string(ojson.Marshal(v)))
	}
}
