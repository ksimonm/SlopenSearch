package main

// Port of the OpenJDK 25 sun.text break iterators used by Lucene's
// ThaiTokenizer: RuleBasedBreakIterator (sentence instance for Locale.ROOT),
// DictionaryBasedBreakIterator + BreakDictionary (word instance for "th"),
// together with CompactByteArray / SupplementaryCharacterData lookups and
// Lucene's CharArrayIterator (non-buggy variant; HAS_BUGGY_BREAKITERATORS is
// false on JDK 25). The compiled rule tables and the Thai dictionary are the
// JDK's own binary resources (see thai_data.go), parsed exactly like
// RuleBasedBreakIterator.setupTables and BreakDictionary.setupDictionary.

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"io"
	"sync"
)

// thaiCIDone is java.text.CharacterIterator.DONE.
const thaiCIDone = 0xFFFF

// thaiIgnore is RuleBasedBreakIterator.IGNORE.
const thaiIgnore = -1

// thaiCharIter ports org.apache.lucene.analysis.util.CharArrayIterator.
type thaiCharIter struct {
	array  []uint16
	start  int
	index  int
	length int
	limit  int
}

func (t *thaiCharIter) setText(array []uint16, start, length int) {
	t.array = array
	t.start = start
	t.index = start
	t.length = length
	t.limit = start + length
}

func (t *thaiCharIter) current() int {
	if t.index == t.limit {
		return thaiCIDone
	}
	return int(t.array[t.index])
}

func (t *thaiCharIter) first() int {
	t.index = t.start
	return t.current()
}

func (t *thaiCharIter) getBeginIndex() int { return 0 }
func (t *thaiCharIter) getEndIndex() int   { return t.length }
func (t *thaiCharIter) getIndex() int      { return t.index - t.start }

func (t *thaiCharIter) next() int {
	t.index++
	if t.index >= t.limit {
		t.index = t.limit
		return thaiCIDone
	}
	return t.current()
}

func (t *thaiCharIter) previous() int {
	t.index--
	if t.index < t.start {
		t.index = t.start
		return thaiCIDone
	}
	return t.current()
}

func (t *thaiCharIter) setIndex(position int) int {
	if position < t.getBeginIndex() || position > t.getEndIndex() {
		panic("Illegal Position")
	}
	t.index = t.start + position
	return t.current()
}

// thaiCompactByteArray ports sun.text.CompactByteArray (lookup only).
type thaiCompactByteArray struct {
	indices []uint16
	values  []int8
}

func (c *thaiCompactByteArray) elementAt(ch int) int {
	return int(c.values[int(c.indices[ch>>7])+(ch&0x7F)])
}

// thaiSuppData ports sun.text.SupplementaryCharacterData.
type thaiSuppData []int32

func (d thaiSuppData) getValue(index int) int {
	i := 0
	j := len(d) - 1
	for {
		k := (i + j) / 2
		start := int(d[k] >> 8)
		end := int(d[k+1] >> 8)
		if index < start {
			j = k
		} else if index > end-1 {
			i = k
		} else {
			v := int(d[k] & 0xFF)
			if v == 0xFF {
				return thaiIgnore
			}
			return v
		}
	}
}

// thaiReader mirrors java.nio.ByteBuffer big-endian reads.
type thaiReader struct {
	b   []byte
	pos int
}

func (r *thaiReader) u8() byte {
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *thaiReader) i16() int16 {
	v := int16(binary.BigEndian.Uint16(r.b[r.pos:]))
	r.pos += 2
	return v
}

func (r *thaiReader) i32() int32 {
	v := int32(binary.BigEndian.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v
}

func (r *thaiReader) bytes(n int) []byte {
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return v
}

func (r *thaiReader) shorts(n int) []int16 {
	v := make([]int16, n)
	for i := range v {
		v[i] = r.i16()
	}
	return v
}

func (r *thaiReader) ints(n int) []int32 {
	v := make([]int32, n)
	for i := range v {
		v[i] = r.i32()
	}
	return v
}

func thaiSignedBytes(b []byte) []int8 {
	v := make([]int8, len(b))
	for i, x := range b {
		v[i] = int8(x)
	}
	return v
}

func thaiUnsignedShorts(s []int16) []uint16 {
	v := make([]uint16, len(s))
	for i, x := range s {
		v[i] = uint16(x)
	}
	return v
}

// thaiRuleTables holds the parsed tables of a RuleBasedBreakIterator.
type thaiRuleTables struct {
	charCategoryTable   thaiCompactByteArray
	suppCategoryTable   thaiSuppData
	stateTable          []int16
	backwardsStateTable []int16
	endStates           []bool
	lookaheadStates     []bool
	additionalData      []byte
	numCategories       int
}

func thaiParseRules(data []byte) *thaiRuleTables {
	r := &thaiReader{b: data}
	label := []byte{'B', 'I', 'd', 'a', 't', 'a', 0}
	if !bytes.Equal(r.bytes(len(label)), label) {
		panic("thai: wrong magic number")
	}
	if r.u8() != 1 {
		panic("thai: unsupported rule data version")
	}
	if l := int(r.i32()); r.pos+l != len(data) {
		panic("thai: wrong rule data length")
	}
	stateTableLength := int(r.i32())
	backwardsStateTableLength := int(r.i32())
	endStatesLength := int(r.i32())
	lookaheadStatesLength := int(r.i32())
	bmpDataLength := int(r.i32())
	nonBMPDataLength := int(r.i32())
	additionalDataLength := int(r.i32())
	r.bytes(8) // checksum
	t := &thaiRuleTables{}
	t.stateTable = r.shorts(stateTableLength)
	t.backwardsStateTable = r.shorts(backwardsStateTableLength)
	t.endStates = make([]bool, endStatesLength)
	for i := range t.endStates {
		t.endStates[i] = r.u8() == 1
	}
	t.lookaheadStates = make([]bool, lookaheadStatesLength)
	for i := range t.lookaheadStates {
		t.lookaheadStates[i] = r.u8() == 1
	}
	t.charCategoryTable.indices = thaiUnsignedShorts(r.shorts(512))
	t.charCategoryTable.values = thaiSignedBytes(r.bytes(bmpDataLength))
	t.suppCategoryTable = thaiSuppData(r.ints(nonBMPDataLength))
	if additionalDataLength > 0 {
		t.additionalData = r.bytes(additionalDataLength)
	}
	if r.pos != len(data) {
		panic("thai: trailing rule data")
	}
	t.numCategories = len(t.stateTable) / len(t.endStates)
	return t
}

// thaiDictionary ports sun.text.BreakDictionary.
type thaiDictionary struct {
	columnMap          thaiCompactByteArray
	suppColumnMap      thaiSuppData
	numCols            int
	table              []int16
	rowIndex           []int16
	rowIndexFlags      []int32
	rowIndexFlagsIndex []int16
	rowIndexShifts     []int8
}

func thaiParseDictionary(data []byte) *thaiDictionary {
	r := &thaiReader{b: data}
	if r.i32() != 1 {
		panic("thai: unsupported dictionary version")
	}
	if l := int(r.i32()); r.pos+l != len(data) {
		panic("thai: wrong dictionary size")
	}
	d := &thaiDictionary{}
	d.columnMap.indices = thaiUnsignedShorts(r.shorts(int(r.i32())))
	d.columnMap.values = thaiSignedBytes(r.bytes(int(r.i32())))
	d.numCols = int(r.i32())
	r.i32() // numColGroups
	d.rowIndex = r.shorts(int(r.i32()))
	d.rowIndexFlagsIndex = r.shorts(int(r.i32()))
	d.rowIndexFlags = r.ints(int(r.i32()))
	d.rowIndexShifts = thaiSignedBytes(r.bytes(int(r.i32())))
	d.table = r.shorts(int(r.i32()))
	d.suppColumnMap = thaiSuppData(r.ints(int(r.i32())))
	if r.pos != len(data) {
		panic("thai: trailing dictionary data")
	}
	return d
}

func (d *thaiDictionary) getNextStateFromCharacter(row, ch int) int {
	var col int
	if ch < 0x10000 {
		col = d.columnMap.elementAt(ch)
	} else {
		col = d.suppColumnMap.getValue(ch)
	}
	return d.getNextState(row, col)
}

func (d *thaiDictionary) getNextState(row, col int) int {
	if d.cellIsPopulated(row, col) {
		return int(d.table[int(d.rowIndex[row])*d.numCols+col+int(d.rowIndexShifts[row])])
	}
	return 0
}

func (d *thaiDictionary) cellIsPopulated(row, col int) bool {
	fi := int(d.rowIndexFlagsIndex[row])
	if fi < 0 {
		return col == -fi
	}
	flags := d.rowIndexFlags[fi+(col>>5)]
	return flags&(int32(1)<<uint(col&0x1f)) != 0
}

var (
	thaiTablesOnce     sync.Once
	thaiSentenceTables *thaiRuleTables
	thaiWordTables     *thaiRuleTables
	thaiWordCatFlags   []bool
	thaiDict           *thaiDictionary
)

func thaiUnpack(s string) []byte {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		panic(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		panic(err)
	}
	return out
}

func thaiLoadTables() {
	thaiTablesOnce.Do(func() {
		thaiSentenceTables = thaiParseRules(thaiUnpack(thaiSentenceRulesGz))
		thaiWordTables = thaiParseRules(thaiUnpack(thaiWordRulesGz))
		// DictionaryBasedBreakIterator.prepareCategoryFlags
		add := thaiWordTables.additionalData
		thaiWordCatFlags = make([]bool, len(add))
		for i, b := range add {
			thaiWordCatFlags[i] = b == 1
		}
		thaiDict = thaiParseDictionary(thaiUnpack(thaiDictGz))
	})
}

// thaiBreakIter is a RuleBasedBreakIterator, optionally with the
// DictionaryBasedBreakIterator behaviour (dict != nil).
type thaiBreakIter struct {
	tab                  *thaiRuleTables
	text                 *thaiCharIter
	cachedLastKnownBreak int

	dict                 *thaiDictionary
	categoryFlags        []bool
	dictionaryCharCount  int
	cachedBreakPositions []int
	positionInCache      int
}

func thaiNewSentenceIter() *thaiBreakIter {
	thaiLoadTables()
	return &thaiBreakIter{tab: thaiSentenceTables, cachedLastKnownBreak: -1}
}

func thaiNewWordIter() *thaiBreakIter {
	thaiLoadTables()
	return &thaiBreakIter{tab: thaiWordTables, cachedLastKnownBreak: -1,
		dict: thaiDict, categoryFlags: thaiWordCatFlags}
}

func (b *thaiBreakIter) setText(t *thaiCharIter) {
	// CharArrayIterator.setIndex(getEndIndex()) always works, so the
	// iterator is used directly (no SafeCharIterator).
	b.text = t
	t.first()
	b.cachedLastKnownBreak = -1
	b.cachedBreakPositions = nil
	b.dictionaryCharCount = 0
	b.positionInCache = 0
}

func (b *thaiBreakIter) current() int { return b.text.getIndex() }

func thaiIsHigh(c int) bool { return c >= 0xD800 && c <= 0xDBFF }
func thaiIsLow(c int) bool  { return c >= 0xDC00 && c <= 0xDFFF }

func (b *thaiBreakIter) getCurrent() int {
	t := b.text
	c1 := t.current()
	if thaiIsHigh(c1) && t.getIndex() < t.getEndIndex() {
		c2 := t.next()
		t.previous()
		if thaiIsLow(c2) {
			return ((c1 - 0xD800) << 10) + (c2 - 0xDC00) + 0x10000
		}
	}
	return c1
}

func (b *thaiBreakIter) getCurrentCodePointCount() int {
	t := b.text
	c1 := t.current()
	if thaiIsHigh(c1) && t.getIndex() < t.getEndIndex() {
		c2 := t.next()
		t.previous()
		if thaiIsLow(c2) {
			return 2
		}
	}
	return 1
}

func (b *thaiBreakIter) getNext() int {
	t := b.text
	index := t.getIndex()
	endIndex := t.getEndIndex()
	if index == endIndex {
		return thaiCIDone
	}
	index += b.getCurrentCodePointCount()
	if index >= endIndex {
		return thaiCIDone
	}
	t.setIndex(index)
	return b.getCurrent()
}

func (b *thaiBreakIter) getNextIndex() int {
	index := b.text.getIndex() + b.getCurrentCodePointCount()
	endIndex := b.text.getEndIndex()
	if index > endIndex {
		return endIndex
	}
	return index
}

func (b *thaiBreakIter) lookupCategory(c int) int {
	var result int
	if c < 0x10000 {
		result = b.tab.charCategoryTable.elementAt(c)
	} else {
		result = b.tab.suppCategoryTable.getValue(c)
	}
	if b.dict != nil && result != thaiIgnore && b.categoryFlags[result] {
		b.dictionaryCharCount++
	}
	return result
}

// ruleHandleNext is RuleBasedBreakIterator.handleNext.
func (b *thaiBreakIter) ruleHandleNext() int {
	t := b.text
	if t.getIndex() == t.getEndIndex() {
		return -1
	}
	result := b.getNextIndex()
	lookaheadResult := 0
	state := 1
	tab := b.tab
	c := b.getCurrent()
	for c != thaiCIDone && state != 0 {
		category := b.lookupCategory(c)
		if category != thaiIgnore {
			state = int(tab.stateTable[state*tab.numCategories+category])
		}
		if tab.lookaheadStates[state] {
			if tab.endStates[state] {
				result = lookaheadResult
			} else {
				lookaheadResult = b.getNextIndex()
			}
		} else if tab.endStates[state] {
			result = b.getNextIndex()
		}
		c = b.getNext()
	}
	if c == thaiCIDone && lookaheadResult == t.getEndIndex() {
		result = lookaheadResult
	}
	t.setIndex(result)
	return result
}

// next is BreakIterator.next(); returns -1 for DONE.
func (b *thaiBreakIter) next() int {
	if b.dict == nil {
		return b.ruleHandleNext()
	}
	// DictionaryBasedBreakIterator.handleNext
	t := b.text
	if b.cachedBreakPositions == nil || b.positionInCache == len(b.cachedBreakPositions)-1 {
		startPos := t.getIndex()
		b.dictionaryCharCount = 0
		result := b.ruleHandleNext()
		if b.dictionaryCharCount > 1 && result-startPos > 1 {
			b.divideUpDictionaryRange(startPos, result)
		} else {
			b.cachedBreakPositions = nil
			return result
		}
	}
	if b.cachedBreakPositions != nil {
		b.positionInCache++
		t.setIndex(b.cachedBreakPositions[b.positionInCache])
		return b.cachedBreakPositions[b.positionInCache]
	}
	return -9999
}

func thaiContains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func thaiPush(s *[]int, v int) { *s = append(*s, v) }

func thaiPop(s *[]int) int {
	v := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return v
}

func thaiPeek(s *[]int) int { return (*s)[len(*s)-1] }

// divideUpDictionaryRange ports DictionaryBasedBreakIterator's method of the
// same name. Stacks are pointers so that aliasing (currentBreakPositions =
// bestBreakPositions) behaves as with the Java objects.
func (b *thaiBreakIter) divideUpDictionaryRange(startPos, endPos int) {
	t := b.text
	dict := b.dict
	t.setIndex(startPos)
	c := b.getCurrent()
	category := b.lookupCategory(c)
	for category == thaiIgnore || !b.categoryFlags[category] {
		if c == thaiCIDone && t.getIndex() == t.getEndIndex() {
			// Java would loop forever here; never reached with real data.
			break
		}
		c = b.getNext()
		category = b.lookupCategory(c)
	}

	currentBreakPositions := &[]int{}
	possibleBreakPositions := &[]int{}
	var wrongBreakPositions []int
	state := 0
	farthestEndPoint := t.getIndex()
	var bestBreakPositions *[]int

	c = b.getCurrent()
	for {
		if dict.getNextState(state, 0) == -1 {
			thaiPush(possibleBreakPositions, t.getIndex())
		}
		state = dict.getNextStateFromCharacter(state, c)
		if state == -1 {
			thaiPush(currentBreakPositions, t.getIndex())
			break
		} else if state == 0 || t.getIndex() >= endPos {
			if t.getIndex() > farthestEndPoint {
				farthestEndPoint = t.getIndex()
				cp := append([]int(nil), (*currentBreakPositions)...)
				bestBreakPositions = &cp
			}
			for len(*possibleBreakPositions) > 0 &&
				thaiContains(wrongBreakPositions, thaiPeek(possibleBreakPositions)) {
				thaiPop(possibleBreakPositions)
			}
			if len(*possibleBreakPositions) == 0 {
				if bestBreakPositions != nil {
					currentBreakPositions = bestBreakPositions
					if farthestEndPoint < endPos {
						t.setIndex(farthestEndPoint + 1)
					} else {
						break
					}
				} else {
					if (len(*currentBreakPositions) == 0 ||
						thaiPeek(currentBreakPositions) != t.getIndex()) &&
						t.getIndex() != startPos {
						thaiPush(currentBreakPositions, t.getIndex())
					}
					b.getNext()
					thaiPush(currentBreakPositions, t.getIndex())
				}
			} else {
				temp := thaiPop(possibleBreakPositions)
				for len(*currentBreakPositions) > 0 && temp < thaiPeek(currentBreakPositions) {
					wrongBreakPositions = append(wrongBreakPositions, thaiPop(currentBreakPositions))
				}
				thaiPush(currentBreakPositions, temp)
				t.setIndex(thaiPeek(currentBreakPositions))
			}
			c = b.getCurrent()
			if t.getIndex() >= endPos {
				break
			}
		} else {
			c = b.getNext()
		}
	}

	if len(*currentBreakPositions) > 0 {
		thaiPop(currentBreakPositions)
	}
	thaiPush(currentBreakPositions, endPos)

	cache := make([]int, len(*currentBreakPositions)+1)
	cache[0] = startPos
	copy(cache[1:], *currentBreakPositions)
	b.cachedBreakPositions = cache
	b.positionInCache = 0
}
