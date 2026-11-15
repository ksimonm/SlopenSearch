// Package jbase64 is java.util.Base64.getDecoder(): padding is optional, the
// unused bits of a final partial unit are ignored, and malformed input fails
// with the IllegalArgumentException message Java gives for the first problem.
package jbase64

import (
	"errors"
	"strconv"
)

func value(c byte) int {
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 26
	case c >= '0' && c <= '9':
		return int(c-'0') + 52
	case c == '+':
		return 62
	case c == '/':
		return 63
	}
	return -1
}

// DecodeString is Base64.getDecoder().decode(String): the string is read as
// ISO-8859-1 bytes (other characters become '?').
func DecodeString(s string) ([]byte, error) {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xff {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return Decode(b)
}

// Decode is Base64.getDecoder().decode(byte[]).
func Decode(src []byte) ([]byte, error) {
	n := len(src)
	if n == 0 {
		return []byte{}, nil
	}
	if n < 2 {
		return nil, errors.New("Input byte[] should at least have 2 bytes for base64 bytes")
	}
	out := make([]byte, 0, n*3/4+2)
	bits, shift, sp := 0, 18, 0
	for sp < n {
		c := src[sp]
		sp++
		if c == '=' {
			if shift == 6 && (sp == n || src[sp] != '=') || shift == 18 {
				return nil, errors.New("Input byte array has wrong 4-byte ending unit")
			}
			if shift == 6 {
				sp++
			}
			break
		}
		v := value(c)
		if v < 0 {
			// Integer.toString of the signed byte, in hex
			return nil, errors.New("Illegal base64 character " + strconv.FormatInt(int64(int8(c)), 16))
		}
		bits |= v << shift
		shift -= 6
		if shift < 0 {
			out = append(out, byte(bits>>16), byte(bits>>8), byte(bits))
			bits, shift = 0, 18
		}
	}
	switch shift {
	case 6:
		out = append(out, byte(bits>>16))
	case 0:
		out = append(out, byte(bits>>16), byte(bits>>8))
	case 12:
		return nil, errors.New("Last unit does not have enough valid bits")
	}
	if sp < n {
		return nil, errors.New("Input byte array has incorrect ending byte at " + strconv.Itoa(sp))
	}
	return out, nil
}
