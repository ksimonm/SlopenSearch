package main

import "unicode/utf16"

// Port of Lucene 10.5.1 org.apache.lucene.analysis.standard.StandardTokenizer and its
// JFlex-generated scanner StandardTokenizerImpl (UAX#29 word breaking). The DFA tables
// live in uax_standard_tables.go and were extracted from lucene-core-10.5.1.jar. The scanner
// works on UTF-16 code units exactly like the Java one, including its fixed-size buffer
// (whose size is the max token length) and the way that buffer splits long tokens.

// uaxToken is one token produced by uaxStandardTokens; start/end are UTF-16 offsets.
type uaxToken struct {
	term, typ          string
	start, end, posInc int
}

// uaxStandardTypes mirrors StandardTokenizer.TOKEN_TYPES.
var uaxStandardTypes = [...]string{
	"<ALPHANUM>",
	"<NUM>",
	"<SOUTHEAST_ASIAN>",
	"<IDEOGRAPHIC>",
	"<HIRAGANA>",
	"<KATAKANA>",
	"<HANGUL>",
	"<EMOJI>",
}

const (
	uaxYYEOF               = -1
	uaxDefaultMaxTokenLen  = 255
	uaxMaxTokenLengthLimit = 1024 * 1024
)

func uaxUnpack(packed []uint16, n int, bias int) []int32 {
	res := make([]int32, n)
	j := 0
	for i := 0; i+1 < len(packed); i += 2 {
		cnt, v := int(packed[i]), int32(int(packed[i+1])-bias)
		for k := 0; k < cnt; k++ {
			res[j] = v
			j++
		}
	}
	if j != n {
		panic("uaxUnpack: bad table length")
	}
	return res
}

// uaxStringReader mimics java.io.StringReader over UTF-16 units.
type uaxStringReader struct {
	s    []uint16
	next int
}

func (r *uaxStringReader) read(buf []uint16, off, n int) int {
	if n == 0 {
		return 0
	}
	if r.next >= len(r.s) {
		return -1
	}
	k := len(r.s) - r.next
	if n < k {
		k = n
	}
	copy(buf[off:off+k], r.s[r.next:r.next+k])
	r.next += k
	return k
}

func (r *uaxStringReader) readOne() int {
	if r.next >= len(r.s) {
		return -1
	}
	c := r.s[r.next]
	r.next++
	return int(c)
}

// uaxScanner is the state of StandardTokenizerImpl.
type uaxScanner struct {
	reader             *uaxStringReader
	buf                []uint16
	markedPos          int
	currentPos         int
	startRead          int
	endRead            int
	atEOF              bool
	finalHighSurrogate int
	yychar             int
}

func uaxIsHigh(c uint16) bool { return c >= 0xD800 && c <= 0xDBFF }
func uaxIsLow(c uint16) bool  { return c >= 0xDC00 && c <= 0xDFFF }

// uaxCodePointAt is Character.codePointAt(char[], index, limit).
func uaxCodePointAt(b []uint16, i, limit int) (int, int) {
	c := b[i]
	if uaxIsHigh(c) && i+1 < limit && uaxIsLow(b[i+1]) {
		return int(utf16.DecodeRune(rune(c), rune(b[i+1]))), 2
	}
	return int(c), 1
}

func uaxCMap(input int) int {
	offset := input & 255
	if offset == input {
		return int(uaxCmapBlocks[offset])
	}
	return int(uaxCmapBlocks[int(uaxCmapTop[input>>8])|offset])
}

func (s *uaxScanner) refill() bool {
	if s.startRead > 0 {
		s.endRead += s.finalHighSurrogate
		s.finalHighSurrogate = 0
		copy(s.buf[0:], s.buf[s.startRead:s.endRead])
		s.endRead -= s.startRead
		s.currentPos -= s.startRead
		s.markedPos -= s.startRead
		s.startRead = 0
	}
	requested := len(s.buf) - s.endRead - s.finalHighSurrogate
	if requested == 0 {
		return true
	}
	numRead := s.reader.read(s.buf, s.endRead, requested)
	if numRead > 0 {
		s.endRead += numRead
		if uaxIsHigh(s.buf[s.endRead-1]) {
			if numRead == requested {
				s.endRead--
				s.finalHighSurrogate = 1
				if numRead == 1 {
					return true
				}
			} else {
				c := s.reader.readOne()
				if c == -1 {
					return true
				}
				s.buf[s.endRead] = uint16(c)
				s.endRead++
			}
		}
		return false
	}
	return true
}

// next is StandardTokenizerImpl.getNextToken; it returns a token type index or uaxYYEOF.
func (s *uaxScanner) next() int {
	endReadL := s.endRead
	bufL := s.buf
	for {
		markedPosL := s.markedPos
		s.yychar += markedPosL - s.startRead
		action := -1
		s.currentPos = markedPosL
		s.startRead = markedPosL
		currentPosL := markedPosL
		state := 0
		if uaxAttribute[state]&1 == 1 {
			action = state
		}
		var input int
		for {
			if currentPosL < endReadL {
				var n int
				input, n = uaxCodePointAt(bufL, currentPosL, endReadL)
				currentPosL += n
			} else if s.atEOF {
				input = uaxYYEOF
				break
			} else {
				s.currentPos = currentPosL
				s.markedPos = markedPosL
				eof := s.refill()
				currentPosL = s.currentPos
				markedPosL = s.markedPos
				bufL = s.buf
				endReadL = s.endRead
				if eof {
					input = uaxYYEOF
					break
				}
				var n int
				input, n = uaxCodePointAt(bufL, currentPosL, endReadL)
				currentPosL += n
			}
			nxt := int(uaxTrans[int(uaxRowMap[state])+uaxCMap(input)])
			if nxt == -1 {
				break
			}
			state = nxt
			attr := uaxAttribute[state]
			if attr&1 == 1 {
				action = state
				markedPosL = currentPosL
				if attr&8 == 8 {
					break
				}
			}
		}
		s.markedPos = markedPosL
		if input == uaxYYEOF && s.startRead == s.currentPos {
			s.atEOF = true
			return uaxYYEOF
		}
		a := action
		if a >= 0 {
			a = int(uaxAction[a])
		}
		switch a {
		case 1, 10, 11, 12, 13, 14, 15, 16, 17, 18:
			// ignore: not numeric, word, ideographic, hiragana, emoji or SE Asian
		case 2:
			return 1 // NUMERIC_TYPE
		case 3:
			return 0 // WORD_TYPE
		case 4:
			return 7 // EMOJI_TYPE
		case 5:
			return 2 // SOUTH_EAST_ASIAN_TYPE
		case 6:
			return 6 // HANGUL_TYPE
		case 7:
			return 3 // IDEOGRAPHIC_TYPE
		case 8:
			return 5 // KATAKANA_TYPE
		case 9:
			return 4 // HIRAGANA_TYPE
		default:
			panic("Error: could not match input")
		}
	}
}

// uaxStandardTokens tokenizes text like Lucene's StandardTokenizer with the given
// maxTokenLength (values < 1 or > 1024*1024 are rejected by Lucene; here they fall back
// to the default 255 / are clamped).
func uaxStandardTokens(text string, maxTokenLength int) []uaxToken {
	toks, _, _ := uaxStandardTokensUTF16(utf16.Encode([]rune(text)), maxTokenLength)
	return toks
}

// uaxStandardTokensUTF16 also returns what StandardTokenizer.end() reports: the final
// offset (yychar+yylength) and the final position increment (skipped too-long tokens).
func uaxStandardTokensUTF16(text []uint16, maxTokenLength int) ([]uaxToken, int, int) {
	if maxTokenLength < 1 {
		maxTokenLength = uaxDefaultMaxTokenLen
	}
	if maxTokenLength > uaxMaxTokenLengthLimit {
		maxTokenLength = uaxMaxTokenLengthLimit
	}
	s := &uaxScanner{reader: &uaxStringReader{s: text}, buf: make([]uint16, maxTokenLength)}
	var out []uaxToken
	skipped := 0
	for {
		typ := s.next()
		if typ == uaxYYEOF {
			return out, s.yychar + s.markedPos - s.startRead, skipped
		}
		length := s.markedPos - s.startRead
		if length <= maxTokenLength {
			term := string(utf16.Decode(s.buf[s.startRead:s.markedPos]))
			out = append(out, uaxToken{
				term:   term,
				typ:    uaxStandardTypes[typ],
				start:  s.yychar,
				end:    s.yychar + length,
				posInc: skipped + 1,
			})
			skipped = 0
		} else {
			skipped++
		}
	}
}
