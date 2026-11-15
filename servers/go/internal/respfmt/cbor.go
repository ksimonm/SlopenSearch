package respfmt

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/ojson"
)

// RequestFormatHeader carries the XContent type of a YAML or CBOR request
// body after the HTTP layer transcoded it to JSON ("yaml", "cbor", ...).
const RequestFormatHeader = "X-Slop-Request-Xcontent"

// responseFormat picks the response XContent type like
// AbstractRestChannel.newBuilder: the format parameter, then the Accept
// header, then the type of the request body, then JSON.
func responseFormat(r *http.Request) string {
	switch f := strings.ToLower(r.URL.Query().Get("format")); f {
	case "json", "yaml", "cbor", "smile":
		return f
	}
	acc := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(acc, "yaml"):
		return "yaml"
	case strings.Contains(acc, "cbor"):
		return "cbor"
	case strings.Contains(acc, "smile"):
		return "smile"
	case strings.Contains(acc, "json"):
		return "json"
	}
	switch r.Header.Get(RequestFormatHeader) {
	case "yaml", "yaml-undetected":
		return "yaml"
	case "cbor":
		return "cbor"
	}
	return "json"
}

// toCBOR renders an ordered tree the way Jackson's CBORGenerator does for
// XContentBuilder output: indefinite-length maps and arrays, definite
// strings, minimal integers, doubles as 64-bit floats (scores, which are
// Java floats, as 32-bit).
func toCBOR(v any) []byte {
	var buf bytes.Buffer
	writeCBOR(&buf, v, "")
	return buf.Bytes()
}

func cborHead(buf *bytes.Buffer, major byte, n uint64) {
	m := major << 5
	switch {
	case n < 24:
		buf.WriteByte(m | byte(n))
	case n <= math.MaxUint8:
		buf.WriteByte(m | 24)
		buf.WriteByte(byte(n))
	case n <= math.MaxUint16:
		buf.WriteByte(m | 25)
		buf.Write(binary.BigEndian.AppendUint16(nil, uint16(n)))
	case n <= math.MaxUint32:
		buf.WriteByte(m | 26)
		buf.Write(binary.BigEndian.AppendUint32(nil, uint32(n)))
	default:
		buf.WriteByte(m | 27)
		buf.Write(binary.BigEndian.AppendUint64(nil, n))
	}
}

func cborString(buf *bytes.Buffer, s string) {
	cborHead(buf, 3, uint64(len(s)))
	buf.WriteString(s)
}

func cborFloatKey(k string) bool { return k == "_score" || k == "max_score" }

func writeCBOR(buf *bytes.Buffer, v any, key string) {
	switch t := v.(type) {
	case ojson.Object:
		buf.WriteByte(0xbf)
		for _, kv := range t {
			cborString(buf, kv.K)
			writeCBOR(buf, kv.V, kv.K)
		}
		buf.WriteByte(0xff)
	case map[string]any:
		buf.WriteByte(0xbf)
		for _, k := range jsonv.SortedKeys(t) {
			cborString(buf, k)
			writeCBOR(buf, t[k], k)
		}
		buf.WriteByte(0xff)
	case []any:
		buf.WriteByte(0x9f)
		for _, e := range t {
			writeCBOR(buf, e, key)
		}
		buf.WriteByte(0xff)
	case ojson.Verbatim:
		x, err := parseResp(t, false)
		if err != nil {
			buf.WriteByte(0xf6)
			return
		}
		writeCBOR(buf, x, key)
	case string:
		cborString(buf, t)
	case bool:
		if t {
			buf.WriteByte(0xf5)
		} else {
			buf.WriteByte(0xf4)
		}
	case nil:
		buf.WriteByte(0xf6)
	case json.Number:
		writeCBORNumber(buf, string(t), key)
	case float64:
		writeCBORNumber(buf, strconv.FormatFloat(t, 'g', -1, 64), key)
	case int:
		writeCBORNumber(buf, strconv.Itoa(t), key)
	case int64:
		writeCBORNumber(buf, strconv.FormatInt(t, 10), key)
	default:
		writeCBOR(buf, string(ojson.Marshal(t)), key)
	}
}

func writeCBORNumber(buf *bytes.Buffer, s, key string) {
	if !strings.ContainsAny(s, ".eE") && !strings.Contains(s, "Infinity") && s != "NaN" {
		n, ok := new(big.Int).SetString(s, 10)
		if ok {
			if n.Sign() >= 0 && n.IsUint64() {
				cborHead(buf, 0, n.Uint64())
				return
			}
			m := new(big.Int).Neg(n)
			m.Sub(m, big.NewInt(1))
			if m.IsUint64() {
				cborHead(buf, 1, m.Uint64())
				return
			}
			// BigInteger: tagged byte string
			tag, mag := uint64(2), n
			if n.Sign() < 0 {
				tag, mag = 3, m
			}
			cborHead(buf, 6, tag)
			b := mag.Bytes()
			cborHead(buf, 2, uint64(len(b)))
			buf.Write(b)
			return
		}
	}
	var f float64
	switch s {
	case "NaN":
		f = math.NaN()
	case "Infinity":
		f = math.Inf(1)
	case "-Infinity":
		f = math.Inf(-1)
	default:
		f, _ = strconv.ParseFloat(s, 64)
	}
	if cborFloatKey(key) {
		buf.WriteByte(0xfa)
		buf.Write(binary.BigEndian.AppendUint32(nil, math.Float32bits(float32(f))))
		return
	}
	buf.WriteByte(0xfb)
	buf.Write(binary.BigEndian.AppendUint64(nil, math.Float64bits(f)))
}
