package main

// Port of Lucene 10.5.1 org.apache.lucene.analysis.th.ThaiTokenizer and its
// base class org.apache.lucene.analysis.util.SegmentingTokenizerBase
// (1024-char buffer, refills cut at the last unambiguous line break),
// running on the JDK 25 break iterators ported in thai_breakiter.go.

import (
	"sort"
	"unicode/utf16"
)

// thaiToken is one ThaiTokenizer token: start/end are UTF-16 offsets into
// the input; positions are consecutive (increment 1), type is "word".
type thaiToken struct {
	term       string
	start, end int
}

const thaiBufferMax = 1024

// thaiIsLetterOrDigit is java.lang.Character.isLetterOrDigit(int) (JDK 25).
func thaiIsLetterOrDigit(cp int) bool {
	r := int32(cp)
	i := sort.Search(len(thaiLetterOrDigitRanges), func(i int) bool {
		return thaiLetterOrDigitRanges[i][1] >= r
	})
	return i < len(thaiLetterOrDigitRanges) && thaiLetterOrDigitRanges[i][0] <= r
}

// thaiCodePointAt is Character.codePointAt(char[], index, limit).
func thaiCodePointAt(a []uint16, index, limit int) int {
	c1 := int(a[index])
	if thaiIsHigh(c1) && index+1 < limit {
		c2 := int(a[index+1])
		if thaiIsLow(c2) {
			return ((c1 - 0xD800) << 10) + (c2 - 0xDC00) + 0x10000
		}
	}
	return c1
}

type thaiTokenizerState struct {
	input    []uint16
	inputPos int

	buffer       [thaiBufferMax]uint16
	length       int
	usableLength int
	offset       int

	sentenceIter    *thaiBreakIter
	sentenceWrapper thaiCharIter

	wordBreaker   *thaiBreakIter
	wordWrapper   thaiCharIter
	sentenceStart int
	sentenceEnd   int
}

func thaiIsSafeEnd(ch uint16) bool {
	switch ch {
	case 0x000D, 0x000A, 0x0085, 0x2028, 0x2029:
		return true
	}
	return false
}

func (s *thaiTokenizerState) findSafeEnd() int {
	for i := s.length - 1; i >= 0; i-- {
		if thaiIsSafeEnd(s.buffer[i]) {
			return i + 1
		}
	}
	return -1
}

func (s *thaiTokenizerState) refill() {
	s.offset += s.usableLength
	leftover := s.length - s.usableLength
	copy(s.buffer[:], s.buffer[s.usableLength:s.length])
	requested := len(s.buffer) - leftover
	returned := copy(s.buffer[leftover:], s.input[s.inputPos:])
	s.inputPos += returned
	s.length = returned + leftover
	if returned < requested {
		s.usableLength = s.length
	} else {
		s.usableLength = s.findSafeEnd()
		if s.usableLength < 0 {
			s.usableLength = s.length
		}
	}
	n := s.usableLength
	if n < 0 {
		n = 0
	}
	s.sentenceWrapper.setText(s.buffer[:], 0, n)
	s.sentenceIter.setText(&s.sentenceWrapper)
}

func (s *thaiTokenizerState) setNextSentence(sentenceStart, sentenceEnd int) {
	s.sentenceStart = sentenceStart
	s.sentenceEnd = sentenceEnd
	s.wordWrapper.setText(s.buffer[:], sentenceStart, sentenceEnd-sentenceStart)
	s.wordBreaker.setText(&s.wordWrapper)
}

func (s *thaiTokenizerState) incrementWord(out *[]thaiToken) bool {
	start := s.wordBreaker.current()
	if start == -1 {
		return false
	}
	end := s.wordBreaker.next()
	for end != -1 && !thaiIsLetterOrDigit(thaiCodePointAt(s.buffer[:], s.sentenceStart+start, s.sentenceEnd)) {
		start = end
		end = s.wordBreaker.next()
	}
	if end == -1 {
		return false
	}
	a := s.sentenceStart + start
	z := s.sentenceStart + end
	*out = append(*out, thaiToken{
		term:  string(utf16.Decode(s.buffer[a:z])),
		start: s.offset + a,
		end:   s.offset + z,
	})
	return true
}

func (s *thaiTokenizerState) incrementSentence(out *[]thaiToken) bool {
	if s.length == 0 {
		return false
	}
	for {
		start := s.sentenceIter.current()
		if start == -1 {
			return false
		}
		end := s.sentenceIter.next()
		if end == -1 {
			return false
		}
		s.setNextSentence(start, end)
		if s.incrementWord(out) {
			return true
		}
	}
}

// thaiTokenize runs Lucene's ThaiTokenizer over text.
func thaiTokenize(text string) []thaiToken {
	s := &thaiTokenizerState{
		input:        utf16.Encode([]rune(text)),
		sentenceIter: thaiNewSentenceIter(),
		wordBreaker:  thaiNewWordIter(),
	}
	// reset()
	s.sentenceWrapper.setText(s.buffer[:], 0, 0)
	s.sentenceIter.setText(&s.sentenceWrapper)

	var out []thaiToken
	for {
		// incrementToken()
		if s.length == 0 || !s.incrementWord(&out) {
			done := false
			for !s.incrementSentence(&out) {
				s.refill()
				if s.length <= 0 {
					done = true
					break
				}
			}
			if done {
				return out
			}
		}
	}
}
