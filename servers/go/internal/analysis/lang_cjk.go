package analysis

// A port of Lucene 10.5.1 CJKBigramFilter. The filter works on a stream with
// position increments; tokens here carry absolute positions, so the stream is
// converted on the way in and out.

const (
	cjkBigramHan = 1 << iota
	cjkBigramHiragana
	cjkBigramKatakana
	cjkBigramHangul
	cjkBigramAll = cjkBigramHan | cjkBigramHiragana | cjkBigramKatakana | cjkBigramHangul
)

type cjkStreamTok struct {
	tok    Token
	posInc int
}

func cjkBigramLucene(toks []Token, flags int, outputUnigrams bool) []Token {
	in := make([]cjkStreamTok, len(toks))
	prev := -1
	for i, t := range toks {
		in[i] = cjkStreamTok{tok: t, posInc: t.Pos - prev}
		prev = t.Pos
	}
	doType := map[string]bool{}
	if flags&cjkBigramHan != 0 {
		doType["<IDEOGRAPHIC>"] = true
	}
	if flags&cjkBigramHiragana != 0 {
		doType["<HIRAGANA>"] = true
	}
	if flags&cjkBigramKatakana != 0 {
		doType["<KATAKANA>"] = true
	}
	if flags&cjkBigramHangul != 0 {
		doType["<HANGUL>"] = true
	}

	var out []cjkStreamTok
	var buffer []rune
	var startOff, endOff []int
	index, lastEndOffset := 0, 0
	ngramState := false
	next := 0
	var lone *cjkStreamTok
	var cur cjkStreamTok

	doNext := func() bool {
		if lone != nil {
			cur = *lone
			lone = nil
			return true
		}
		if next >= len(in) {
			return false
		}
		cur = in[next]
		next++
		return true
	}
	hasBigram := func() bool { return len(buffer)-index > 1 }
	hasUnigram := func() bool {
		if outputUnigrams {
			return len(buffer)-index == 1
		}
		return len(buffer) == 1 && index == 0
	}
	flushBigram := func() {
		t := Token{Term: string(buffer[index]) + string(buffer[index+1]),
			Start: startOff[index], End: endOff[index+1], Type: "<DOUBLE>"}
		inc := 1
		if outputUnigrams {
			inc = 0
			t.PosLen = 2
		}
		out = append(out, cjkStreamTok{tok: t, posInc: inc})
		index++
	}
	flushUnigram := func() {
		t := Token{Term: string(buffer[index]), Start: startOff[index], End: endOff[index], Type: "<SINGLE>"}
		out = append(out, cjkStreamTok{tok: t, posInc: 1})
		index++
	}
	refill := func() {
		if len(buffer) > 64 {
			last := len(buffer) - 1
			buffer = []rune{buffer[last]}
			startOff = []int{startOff[last]}
			endOff = []int{endOff[last]}
			index -= last
		}
		rs := []rune(cur.tok.Term)
		start, end := cur.tok.Start, cur.tok.End
		lastEndOffset = end
		if end-start != UTF16Offset(cur.tok.Term) {
			for _, r := range rs {
				buffer = append(buffer, r)
				startOff = append(startOff, start)
				endOff = append(endOff, end)
			}
		} else {
			for _, r := range rs {
				buffer = append(buffer, r)
				startOff = append(startOff, start)
				start += RuneWidth(r)
				endOff = append(endOff, start)
			}
		}
	}
	for {
		if hasBigram() {
			if outputUnigrams {
				if ngramState {
					flushBigram()
				} else {
					flushUnigram()
					index--
				}
				ngramState = !ngramState
			} else {
				flushBigram()
			}
			continue
		}
		if doNext() {
			if doType[cur.tok.Type] {
				if cur.tok.Start != lastEndOffset {
					if hasUnigram() {
						c := cur
						lone = &c
						flushUnigram()
						continue
					}
					index = 0
					buffer, startOff, endOff = buffer[:0], startOff[:0], endOff[:0]
				}
				refill()
			} else {
				if hasUnigram() {
					c := cur
					lone = &c
					flushUnigram()
					continue
				}
				out = append(out, cur)
			}
			continue
		}
		if hasUnigram() {
			flushUnigram()
			continue
		}
		break
	}
	res := make([]Token, len(out))
	pos := -1
	for i, o := range out {
		pos += o.posInc
		o.tok.Pos = pos
		res[i] = o.tok
	}
	return res
}

// cjkBigramFlags reads the ignored_scripts option of the cjk_bigram filter.
func cjkBigramFlags(p map[string]any) int {
	flags := cjkBigramAll
	for _, s := range fsStrings(p, "ignored_scripts") {
		switch s {
		case "han":
			flags &^= cjkBigramHan
		case "hiragana":
			flags &^= cjkBigramHiragana
		case "katakana":
			flags &^= cjkBigramKatakana
		case "hangul":
			flags &^= cjkBigramHangul
		}
	}
	return flags
}
