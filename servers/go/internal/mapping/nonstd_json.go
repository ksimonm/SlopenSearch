package mapping

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/model"
)

// Jackson (as configured by OpenSearch) rejects the non-numeric number tokens
// below. They are only recognised when they start a value and are not followed
// by another identifier character (otherwise Jackson reports an unrecognized
// token instead).
var jacksonNonStandardTokens = []string{"NaN", "Infinity", "-Infinity", "+Infinity", "-INF", "+INF"}

// nonStdFrame is one open container in the valid JSON prefix before the token.
type nonStdFrame struct {
	array bool
	key   string // object: the current key; array: the key the array is the value of
	elems [][]byte
}

// NonStandardTokenError reports the error OpenSearch returns when a document
// body is valid JSON up to one of Jackson's non-standard number tokens (NaN,
// Infinity, ...). Jackson fails while the document parser reads that token: a
// knn_vector field reading its array elements wraps it in its field error with
// the usual value preview; anywhere else the document parser wraps it in
// "failed to parse". It returns nil when the body fails for any other reason.
func NonStandardTokenError(idx *model.Index, docID string, raw []byte) *apierr.Error {
	var v any
	var se *json.SyntaxError
	if err := json.Unmarshal(raw, &v); !errors.As(err, &se) {
		return nil
	}
	off := int(se.Offset) - 1 // the offending byte
	if off < 0 || off >= len(raw) {
		return nil
	}
	start := off
	if raw[off] == 'I' && off > 0 && raw[off-1] == '-' {
		start = off - 1 // "-I": Go rejects the byte after the minus sign
	}
	tok := ""
	for _, t := range jacksonNonStandardTokens {
		if bytes.HasPrefix(raw[start:], []byte(t)) {
			tok = t
			break
		}
	}
	end := start + len(tok)
	if tok == "" || (end < len(raw) && isJavaIdentPart(raw[end])) {
		return nil
	}
	frames, ok := scanJSONPrefix(raw[:start])
	if !ok {
		return nil
	}
	reason := fmt.Sprintf("Non-standard token '%s': enable `JsonReadFeature.ALLOW_NON_NUMERIC_NUMBERS` to allow\n"+
		" at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); byte offset: #%d]", tok, end)
	cause := &apierr.Error{Type: "json_parse_exception", Reason: reason,
		CausedBy: &apierr.Error{Type: "stream_read_exception", Reason: reason}}
	if e := knnNonStandardTokenError(idx, docID, frames, raw[end:]); e != nil {
		e.CausedBy = cause
		return e
	}
	return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse", CausedBy: cause}
}

// knnNonStandardTokenError builds the knn_vector field error when the token is
// an element of an array that is the value of a knn_vector field whose earlier
// elements are all numbers. The preview is what OpenSearch reads from the
// parser after the failure: the previous element, or the rest of the array
// when the token was the first element.
func knnNonStandardTokenError(idx *model.Index, docID string, frames []nonStdFrame, rest []byte) *apierr.Error {
	n := len(frames)
	if idx == nil || n < 2 || !frames[n-1].array || frames[n-2].array {
		return nil
	}
	var path []string
	for _, f := range frames[:n-1] {
		if !f.array {
			path = append(path, f.key)
		}
	}
	def, ok := FieldDef(idx, strings.Join(path, "."))
	if !ok || def["type"] != "knn_vector" {
		return nil
	}
	elems := frames[n-1].elems
	for _, e := range elems {
		if _, err := strconv.ParseFloat(string(e), 64); err != nil {
			return nil
		}
	}
	var preview string
	if len(elems) > 0 {
		preview = javaNumberString(elems[len(elems)-1])
	} else {
		r := bytes.TrimLeft(rest, " \t\r\n")
		r = bytes.TrimPrefix(r, []byte(","))
		dec := json.NewDecoder(bytes.NewReader(append([]byte("["), r...)))
		dec.UseNumber()
		var list []any
		if dec.Decode(&list) != nil {
			return nil
		}
		parts := make([]string, len(list))
		for i, e := range list {
			if num, ok := e.(json.Number); ok {
				parts[i] = javaNumberString([]byte(num))
			} else {
				parts[i] = javaMapDebug(e)
			}
		}
		preview = "[" + strings.Join(parts, ", ") + "]"
	}
	c := &ParseContext{docID: docID}
	return knnValueError(c, strings.Join(path, "."), preview, "", "")
}

// scanJSONPrefix walks a valid JSON prefix that ends where a value starts and
// returns the open containers.
func scanJSONPrefix(b []byte) ([]nonStdFrame, bool) {
	var frames []nonStdFrame
	expectKey := false
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',' || c == ':':
			i++
			if c == ',' && len(frames) > 0 && !frames[len(frames)-1].array {
				expectKey = true
			}
		case c == '{' || c == '[':
			key := ""
			if n := len(frames); n > 0 {
				key = frames[n-1].key
			}
			frames = append(frames, nonStdFrame{array: c == '[', key: key})
			expectKey = c == '{'
			i++
		case c == '}' || c == ']':
			if len(frames) == 0 {
				return nil, false
			}
			frames = frames[:len(frames)-1]
			i++
			markElem(frames, b[i-1:i])
		default:
			j := scanJSONScalar(b, i)
			if j <= i {
				return nil, false
			}
			if expectKey {
				var k string
				if json.Unmarshal(b[i:j], &k) != nil {
					return nil, false
				}
				frames[len(frames)-1].key = k
				expectKey = false
			} else {
				markElem(frames, b[i:j])
			}
			i = j
		}
	}
	return frames, len(frames) > 0
}

// markElem records a finished value in the enclosing array.
func markElem(frames []nonStdFrame, v []byte) {
	if n := len(frames); n > 0 && frames[n-1].array {
		frames[n-1].elems = append(frames[n-1].elems, v)
	}
}

// scanJSONScalar returns the end of the string or literal starting at i.
func scanJSONScalar(b []byte, i int) int {
	if b[i] == '"' {
		for j := i + 1; j < len(b); j++ {
			switch b[j] {
			case '\\':
				j++
			case '"':
				return j + 1
			}
		}
		return -1
	}
	j := i
	for j < len(b) && !strings.ContainsRune(" \t\r\n,:]}", rune(b[j])) {
		j++
	}
	return j
}

func isJavaIdentPart(c byte) bool {
	return c >= 0x80 || c == '_' || c == '$' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// javaNumberString renders a JSON number the way Java prints the parsed
// Integer/Long (as written) or Double (Double.toString).
func javaNumberString(num []byte) string {
	s := string(num)
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) {
		return s
	}
	a := math.Abs(f)
	if a != 0 && (a < 1e-3 || a >= 1e7) {
		mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'E', -1, 64), "E")
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		e, _ := strconv.Atoi(exp)
		return mant + "E" + strconv.Itoa(e)
	}
	out := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(out, ".") {
		out += ".0"
	}
	return out
}
