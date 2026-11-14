package main

import (
	"fmt"
	"unicode/utf8"
)

// invalidUTF8Error reports the first invalid UTF-8 sequence of a document
// body the way Jackson's UTF8StreamJsonParser does while parsing it, or nil.
func invalidUTF8Error(raw []byte) *apiError {
	if utf8.Valid(raw) {
		return nil
	}
	msg := ""
	for i := 0; i < len(raw) && msg == ""; {
		b := raw[i]
		if b < 0x80 {
			i++
			continue
		}
		var need int
		switch {
		case b&0xE0 == 0xC0:
			need = 1
		case b&0xF0 == 0xE0:
			need = 2
		case b&0xF8 == 0xF0:
			need = 3
		default:
			msg = fmt.Sprintf("Invalid UTF-8 start byte 0x%02x", b) + jacksonOffset(i+1)
			continue
		}
		j := i + 1
		for ; j <= i+need; j++ {
			if j >= len(raw) {
				msg = "Unexpected end-of-input in UTF-8 character" + jacksonOffset(j)
				break
			}
			if raw[j]&0xC0 != 0x80 {
				msg = fmt.Sprintf("Invalid UTF-8 middle byte 0x%02x", raw[j]) + jacksonOffset(j+1)
				break
			}
		}
		i = j
	}
	if msg == "" {
		return nil
	}
	return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
		CausedBy: &apiError{Type: "json_parse_exception", Reason: msg,
			CausedBy: &apiError{Type: "stream_read_exception", Reason: msg}}}
}

func jacksonOffset(n int) string {
	return fmt.Sprintf("\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); byte offset: #%d]", n)
}
