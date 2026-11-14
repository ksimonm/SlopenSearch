//! Lucene 10.5.1 `th.ThaiTokenizer` (on `util.SegmentingTokenizerBase` and
//! `util.CharArrayIterator`), together with the JDK 25 break iterators it runs:
//! `BreakIterator.getSentenceInstance(Locale.ROOT)` (`sun.text.RuleBasedBreakIterator`
//! over SentenceBreakIteratorData) and `BreakIterator.getWordInstance(th)`
//! (`sun.text.DictionaryBasedBreakIterator` over WordBreakIteratorData_th and the
//! `thai_dict` `sun.text.BreakDictionary`). The compiled rule and dictionary
//! resources are the JDK's own (`thai_data.bin`), parsed like the JDK parses them.
//!
//! Only the forward iteration the tokenizer uses (`setText`, `current`, `next`)
//! is ported; positions and characters are UTF-16 code units, like Java.
//! `CharArrayIterator.HAS_BUGGY_BREAKITERATORS` is false on JDK 25, so the
//! wrappers pass characters through unchanged.

use std::sync::OnceLock;

use super::thai_data::{BREAK_DATA, LETTER_OR_DIGIT};
use super::Tok;

/// `BreakIterator.DONE`.
const DONE: i32 = -1;
/// `CharacterIterator.DONE` ('￿'). The JDK iterators compare code points
/// against it, so a real U+FFFF in the text also ends a scan, as in Java.
const CI_DONE: i32 = 0xFFFF;
/// `RuleBasedBreakIterator.IGNORE`.
const IGNORE: i32 = -1;
const START_STATE: i32 = 1;
const STOP_STATE: i32 = 0;
const BMP_INDICES_LENGTH: usize = 512;

fn is_high_surrogate(c: u16) -> bool {
    (0xD800..0xDC00).contains(&c)
}

fn is_low_surrogate(c: u16) -> bool {
    (0xDC00..0xE000).contains(&c)
}

fn to_code_point(c1: u16, c2: u16) -> i32 {
    0x10000 + (((c1 as i32) - 0xD800) << 10) + ((c2 as i32) - 0xDC00)
}

/// Java `Character.isLetterOrDigit(int)`.
fn is_letter_or_digit(cp: u32) -> bool {
    LETTER_OR_DIGIT
        .binary_search_by(|&(lo, hi)| {
            if hi < cp {
                std::cmp::Ordering::Less
            } else if lo > cp {
                std::cmp::Ordering::Greater
            } else {
                std::cmp::Ordering::Equal
            }
        })
        .is_ok()
}

/// Java `Character.codePointAt(char[] a, int index, int limit)`.
fn code_point_at(a: &[u16], index: usize, limit: usize) -> u32 {
    let c1 = a[index];
    if is_high_surrogate(c1) && index + 1 < limit && is_low_surrogate(a[index + 1]) {
        return to_code_point(c1, a[index + 1]) as u32;
    }
    c1 as u32
}

// ---------------------------------------------------------------------------
// Resource parsing (java.nio.ByteBuffer, big-endian)

struct ByteBuffer<'a> {
    b: &'a [u8],
    pos: usize,
}

impl ByteBuffer<'_> {
    fn get(&mut self) -> i8 {
        self.pos += 1;
        self.b[self.pos - 1] as i8
    }
    fn get_bytes(&mut self, n: usize) -> &[u8] {
        self.pos += n;
        &self.b[self.pos - n..self.pos]
    }
    fn get_short(&mut self) -> i16 {
        i16::from_be_bytes(self.get_bytes(2).try_into().unwrap())
    }
    fn get_int(&mut self) -> i32 {
        i32::from_be_bytes(self.get_bytes(4).try_into().unwrap())
    }
    fn get_long(&mut self) -> i64 {
        i64::from_be_bytes(self.get_bytes(8).try_into().unwrap())
    }
    fn get_shorts(&mut self, n: usize) -> Vec<i16> {
        (0..n).map(|_| self.get_short()).collect()
    }
    fn get_ints(&mut self, n: usize) -> Vec<i32> {
        (0..n).map(|_| self.get_int()).collect()
    }
    fn get_len(&mut self) -> usize {
        self.get_int() as usize
    }
}

/// `sun.text.CompactByteArray` (read-only, compact form).
struct CompactByteArray {
    indices: Vec<i16>,
    values: Vec<i8>,
}

impl CompactByteArray {
    const BLOCKSHIFT: u32 = 7;
    const BLOCKMASK: usize = (1 << Self::BLOCKSHIFT) - 1;

    fn element_at(&self, index: u16) -> i8 {
        let index = index as usize;
        self.values
            [(self.indices[index >> Self::BLOCKSHIFT] as u16 as usize) + (index & Self::BLOCKMASK)]
    }
}

/// `sun.text.SupplementaryCharacterData`: sorted (start << 8 | value) entries.
struct SupplementaryCharacterData {
    data_table: Vec<i32>,
}

impl SupplementaryCharacterData {
    fn get_value(&self, index: i32) -> i32 {
        let mut i = 0usize;
        let mut j = self.data_table.len() - 1;
        loop {
            let k = (i + j) / 2;
            let start = self.data_table[k] >> 8;
            let end = self.data_table[k + 1] >> 8;
            if index < start {
                j = k;
            } else if index > end - 1 {
                i = k;
            } else {
                let v = self.data_table[k] & 0xFF;
                return if v == 0xFF { IGNORE } else { v };
            }
        }
    }
}

/// The tables of a `RuleBasedBreakIterator` (`setupTables`).
struct RuleData {
    char_category_table: CompactByteArray,
    supplementary_char_category_table: SupplementaryCharacterData,
    state_table: Vec<i16>,
    end_states: Vec<bool>,
    lookahead_states: Vec<bool>,
    additional_data: Vec<u8>,
    num_categories: i32,
}

impl RuleData {
    /// `validateRuleData` + `setupTables`. The backwards state table is only
    /// used by `previous()`/`following()`, which the tokenizer never calls.
    fn parse(data: &[u8]) -> RuleData {
        let mut bb = ByteBuffer { b: data, pos: 0 };
        assert_eq!(bb.get_bytes(7), b"BIdata\0", "Wrong magic number");
        assert_eq!(bb.get(), 1, "Unsupported version");
        let len = bb.get_len();
        assert_eq!(bb.pos + len, data.len(), "Wrong data length");

        let state_table_length = bb.get_len();
        let backwards_state_table_length = bb.get_len();
        let end_states_length = bb.get_len();
        let lookahead_states_length = bb.get_len();
        let bmp_data_length = bb.get_len();
        let non_bmp_data_length = bb.get_len();
        let additional_data_length = bb.get_len();
        let _checksum = bb.get_long();

        let state_table = bb.get_shorts(state_table_length);
        let _backwards_state_table = bb.get_shorts(backwards_state_table_length);
        let end_states = (0..end_states_length).map(|_| bb.get() == 1).collect::<Vec<_>>();
        let lookahead_states = (0..lookahead_states_length).map(|_| bb.get() == 1).collect();
        let indices = bb.get_shorts(BMP_INDICES_LENGTH);
        let values = bb.get_bytes(bmp_data_length).iter().map(|&b| b as i8).collect();
        let supp = bb.get_ints(non_bmp_data_length);
        let additional_data = bb.get_bytes(additional_data_length).to_vec();
        assert_eq!(bb.pos, data.len());
        let num_categories = (state_table.len() / end_states.len()) as i32;
        RuleData {
            char_category_table: CompactByteArray { indices, values },
            supplementary_char_category_table: SupplementaryCharacterData { data_table: supp },
            state_table,
            end_states,
            lookahead_states,
            additional_data,
            num_categories,
        }
    }
}

/// `sun.text.BreakDictionary`: the Thai word trie as a compressed state table.
struct BreakDictionary {
    column_map: CompactByteArray,
    supplementary_char_column_map: SupplementaryCharacterData,
    num_cols: i32,
    table: Vec<i16>,
    row_index: Vec<i16>,
    row_index_flags: Vec<i32>,
    row_index_flags_index: Vec<i16>,
    row_index_shifts: Vec<i8>,
}

impl BreakDictionary {
    fn parse(data: &[u8]) -> BreakDictionary {
        let mut bb = ByteBuffer { b: data, pos: 0 };
        assert_eq!(bb.get_int(), 1, "Dictionary version is unsupported");
        let len = bb.get_len();
        assert_eq!(bb.pos + len, data.len(), "Dictionary size is wrong");
        let len = bb.get_len();
        let indices = bb.get_shorts(len);
        let len = bb.get_len();
        let values = bb.get_bytes(len).iter().map(|&b| b as i8).collect();
        let num_cols = bb.get_int();
        let _num_col_groups = bb.get_int();
        let len = bb.get_len();
        let row_index = bb.get_shorts(len);
        let len = bb.get_len();
        let row_index_flags_index = bb.get_shorts(len);
        let len = bb.get_len();
        let row_index_flags = bb.get_ints(len);
        let len = bb.get_len();
        let row_index_shifts = bb.get_bytes(len).iter().map(|&b| b as i8).collect();
        let len = bb.get_len();
        let table = bb.get_shorts(len);
        let len = bb.get_len();
        let supp = bb.get_ints(len);
        assert_eq!(bb.pos, data.len());
        BreakDictionary {
            column_map: CompactByteArray { indices, values },
            supplementary_char_column_map: SupplementaryCharacterData { data_table: supp },
            num_cols,
            table,
            row_index,
            row_index_flags,
            row_index_flags_index,
            row_index_shifts,
        }
    }

    fn get_next_state_from_character(&self, row: i32, ch: i32) -> i32 {
        let col = if ch < 0x10000 {
            self.column_map.element_at(ch as u16) as i32
        } else {
            self.supplementary_char_column_map.get_value(ch)
        };
        self.get_next_state(row, col)
    }

    fn get_next_state(&self, row: i32, col: i32) -> i32 {
        if self.cell_is_populated(row, col) {
            self.internal_at(
                self.row_index[row as usize] as i32,
                col + self.row_index_shifts[row as usize] as i32,
            )
        } else {
            0
        }
    }

    fn cell_is_populated(&self, row: i32, col: i32) -> bool {
        let flags_index = self.row_index_flags_index[row as usize] as i32;
        if flags_index < 0 {
            col == -flags_index
        } else {
            // Java would throw on a negative index here; so do we (usize cast panics on lookup).
            let flags = self.row_index_flags[usize::try_from(flags_index + (col >> 5)).unwrap()];
            (flags & (1i32 << (col & 0x1f))) != 0
        }
    }

    fn internal_at(&self, row: i32, col: i32) -> i32 {
        self.table[usize::try_from(row * self.num_cols + col).unwrap()] as i32
    }
}

struct Tables {
    sentence: RuleData,
    word: RuleData,
    /// DictionaryBasedBreakIterator.categoryFlags (from the word rules' additional data).
    category_flags: Vec<bool>,
    dictionary: BreakDictionary,
}

fn tables() -> &'static Tables {
    static TABLES: OnceLock<Tables> = OnceLock::new();
    TABLES.get_or_init(|| {
        let mut parts = Vec::new();
        let mut p = 0;
        while p < BREAK_DATA.len() {
            let n = u32::from_be_bytes(BREAK_DATA[p..p + 4].try_into().unwrap()) as usize;
            parts.push(&BREAK_DATA[p + 4..p + 4 + n]);
            p += 4 + n;
        }
        let sentence = RuleData::parse(parts[0]);
        let word = RuleData::parse(parts[1]);
        let category_flags = word.additional_data.iter().map(|&b| b == 1).collect();
        Tables { sentence, word, category_flags, dictionary: BreakDictionary::parse(parts[2]) }
    })
}

// ---------------------------------------------------------------------------
// CharArrayIterator: a CharacterIterator over buffer[start..start+length],
// with indices relative to `start`. The array itself is passed to each call.

#[derive(Default)]
struct CharArrayIterator {
    start: i32,
    index: i32,
    length: i32,
    limit: i32,
}

impl CharArrayIterator {
    fn set_text(&mut self, start: i32, length: i32) {
        self.start = start;
        self.index = start;
        self.length = length;
        self.limit = start + length;
    }
    fn current(&self, a: &[u16]) -> u16 {
        if self.index == self.limit {
            CI_DONE as u16
        } else {
            a[self.index as usize]
        }
    }
    fn first(&mut self, a: &[u16]) -> u16 {
        self.index = self.start;
        self.current(a)
    }
    fn get_begin_index(&self) -> i32 {
        0
    }
    fn get_end_index(&self) -> i32 {
        self.length
    }
    fn get_index(&self) -> i32 {
        self.index - self.start
    }
    fn next(&mut self, a: &[u16]) -> u16 {
        self.index += 1;
        if self.index >= self.limit {
            self.index = self.limit;
            CI_DONE as u16
        } else {
            self.current(a)
        }
    }
    fn previous(&mut self, a: &[u16]) -> u16 {
        self.index -= 1;
        if self.index < self.start {
            self.index = self.start;
            CI_DONE as u16
        } else {
            self.current(a)
        }
    }
    fn set_index(&mut self, a: &[u16], position: i32) -> u16 {
        assert!(
            position >= self.get_begin_index() && position <= self.get_end_index(),
            "Illegal Position: {position}"
        );
        self.index = self.start + position;
        self.current(a)
    }
}

// ---------------------------------------------------------------------------
// RuleBasedBreakIterator / DictionaryBasedBreakIterator (forward iteration)

/// Dictionary state of a `DictionaryBasedBreakIterator`.
#[derive(Default)]
struct DictState {
    dictionary_char_count: i32,
    cached_break_positions: Option<Vec<i32>>,
    position_in_cache: i32,
}

struct BreakIterator {
    rules: &'static RuleData,
    /// Some for a DictionaryBasedBreakIterator.
    dict: Option<DictState>,
    text: CharArrayIterator,
}

impl BreakIterator {
    /// `setText(CharacterIterator)`: the CharArrayIterator is a "good" iterator
    /// (setIndex(end) works), so it is used directly.
    fn set_text(&mut self, a: &[u16], start: i32, length: i32) {
        self.text.set_text(start, length);
        let end = self.text.get_end_index();
        self.text.set_index(a, end);
        self.text.first(a);
        // (cachedLastKnownBreak = DONE: only used by previous()/following().)
        if let Some(d) = &mut self.dict {
            d.cached_break_positions = None;
            d.dictionary_char_count = 0;
            d.position_in_cache = 0;
        }
    }

    fn current(&self) -> i32 {
        self.text.get_index()
    }

    fn next(&mut self, a: &[u16]) -> i32 {
        if self.dict.is_some() {
            self.dict_handle_next(a)
        } else {
            self.handle_next(a)
        }
    }

    fn get_current(&mut self, a: &[u16]) -> i32 {
        let c1 = self.text.current(a);
        if is_high_surrogate(c1) && self.text.get_index() < self.text.get_end_index() {
            let c2 = self.text.next(a);
            self.text.previous(a);
            if is_low_surrogate(c2) {
                return to_code_point(c1, c2);
            }
        }
        c1 as i32
    }

    fn get_current_code_point_count(&mut self, a: &[u16]) -> i32 {
        let c1 = self.text.current(a);
        if is_high_surrogate(c1) && self.text.get_index() < self.text.get_end_index() {
            let c2 = self.text.next(a);
            self.text.previous(a);
            if is_low_surrogate(c2) {
                return 2;
            }
        }
        1
    }

    fn get_next(&mut self, a: &[u16]) -> i32 {
        let mut index = self.text.get_index();
        let end_index = self.text.get_end_index();
        if index == end_index || {
            index += self.get_current_code_point_count(a);
            index >= end_index
        } {
            return CI_DONE;
        }
        self.text.set_index(a, index);
        self.get_current(a)
    }

    fn get_next_index(&mut self, a: &[u16]) -> i32 {
        let index = self.text.get_index() + self.get_current_code_point_count(a);
        let end_index = self.text.get_end_index();
        if index > end_index {
            end_index
        } else {
            index
        }
    }

    /// RuleBasedBreakIterator.handleNext.
    fn handle_next(&mut self, a: &[u16]) -> i32 {
        if self.text.get_index() == self.text.get_end_index() {
            return DONE;
        }
        let rules = self.rules;
        let mut result = self.get_next_index(a);
        let mut lookahead_result = 0;
        let mut state = START_STATE;
        let mut c = self.get_current(a);
        while c != CI_DONE && state != STOP_STATE {
            let category = self.lookup_category(c);
            if category != IGNORE {
                state =
                    rules.state_table[(state * rules.num_categories + category) as usize] as i32;
            }
            if rules.lookahead_states[state as usize] {
                if rules.end_states[state as usize] {
                    result = lookahead_result;
                } else {
                    lookahead_result = self.get_next_index(a);
                }
            } else if rules.end_states[state as usize] {
                result = self.get_next_index(a);
            }
            c = self.get_next(a);
        }
        if c == CI_DONE && lookahead_result == self.text.get_end_index() {
            result = lookahead_result;
        }
        self.text.set_index(a, result);
        result
    }

    /// RuleBasedBreakIterator.lookupCategory, plus DictionaryBasedBreakIterator's
    /// count of dictionary characters passed over.
    fn lookup_category(&mut self, c: i32) -> i32 {
        let result = if c < 0x10000 {
            self.rules.char_category_table.element_at(c as u16) as i32
        } else {
            self.rules.supplementary_char_category_table.get_value(c)
        };
        if let Some(d) = &mut self.dict {
            if result != IGNORE && tables().category_flags[result as usize] {
                d.dictionary_char_count += 1;
            }
        }
        result
    }

    fn is_dictionary_category(category: i32) -> bool {
        category != IGNORE && tables().category_flags[category as usize]
    }

    /// DictionaryBasedBreakIterator.handleNext.
    fn dict_handle_next(&mut self, a: &[u16]) -> i32 {
        let d = self.dict.as_ref().unwrap();
        let exhausted = match &d.cached_break_positions {
            None => true,
            Some(c) => d.position_in_cache == c.len() as i32 - 1,
        };
        if exhausted {
            // use the rules for a tentative boundary; if more than one dictionary
            // character was passed over, divide the range up with the dictionary
            let start_pos = self.text.get_index();
            self.dict.as_mut().unwrap().dictionary_char_count = 0;
            let result = self.handle_next(a);
            if self.dict.as_ref().unwrap().dictionary_char_count > 1 && result - start_pos > 1 {
                self.divide_up_dictionary_range(a, start_pos, result);
            } else {
                self.dict.as_mut().unwrap().cached_break_positions = None;
                return result;
            }
        }
        let d = self.dict.as_mut().unwrap();
        if let Some(cache) = &d.cached_break_positions {
            d.position_in_cache += 1;
            let pos = cache[d.position_in_cache as usize];
            self.text.set_index(a, pos);
            return pos;
        }
        -9999 // SHOULD NEVER GET HERE!
    }

    /// DictionaryBasedBreakIterator.divideUpDictionaryRange: splits
    /// [start_pos, end_pos) into dictionary words by backtracking over the trie.
    fn divide_up_dictionary_range(&mut self, a: &[u16], start_pos: i32, end_pos: i32) {
        let dictionary = &tables().dictionary;

        // seek to the first dictionary character
        self.text.set_index(a, start_pos);
        let mut c = self.get_current(a);
        let mut category = self.lookup_category(c);
        while !Self::is_dictionary_category(category) {
            c = self.get_next(a);
            category = self.lookup_category(c);
        }

        // Java's Stack objects, by identity: `currentBreakPositions =
        // bestBreakPositions` makes both names refer to the same stack, so later
        // pushes through one are seen through the other. stacks[cur] /
        // stacks[best] reproduce that aliasing.
        let mut stacks: Vec<Vec<i32>> = vec![Vec::new()];
        let mut cur = 0usize;
        let mut best: Option<usize> = None;
        let mut possible_break_positions: Vec<i32> = Vec::new();
        let mut wrong_break_positions: Vec<i32> = Vec::new();

        let mut state = 0;
        let mut farthest_end_point = self.text.get_index();

        c = self.get_current(a);
        loop {
            // on the last character of a legal word: remember the possible break
            if dictionary.get_next_state(state, 0) == -1 {
                possible_break_positions.push(self.text.get_index());
            }
            state = dictionary.get_next_state_from_character(state, c);

            if state == -1 {
                // a non-dictionary character: the whole range is traversed
                stacks[cur].push(self.text.get_index());
                break;
            } else if state == 0 || self.text.get_index() >= end_pos {
                // error state or off the end of the range
                if self.text.get_index() > farthest_end_point {
                    farthest_end_point = self.text.get_index();
                    stacks.push(stacks[cur].clone());
                    best = Some(stacks.len() - 1);
                }

                while possible_break_positions
                    .last()
                    .is_some_and(|p| wrong_break_positions.contains(p))
                {
                    possible_break_positions.pop();
                }

                if possible_break_positions.is_empty() {
                    if let Some(b) = best {
                        // "bless" the breaks that got us farthest, restart after it
                        cur = b;
                        if farthest_end_point < end_pos {
                            self.text.set_index(a, farthest_end_point + 1);
                        } else {
                            break;
                        }
                    } else {
                        let idx = self.text.get_index();
                        if (stacks[cur].is_empty() || *stacks[cur].last().unwrap() != idx)
                            && idx != start_pos
                        {
                            stacks[cur].push(idx);
                        }
                        self.get_next(a);
                        let idx = self.text.get_index();
                        stacks[cur].push(idx);
                    }
                } else {
                    // promote the last possible break, dropping later current breaks
                    let temp = possible_break_positions.pop().unwrap();
                    while stacks[cur].last().is_some_and(|&p| temp < p) {
                        let temp2 = stacks[cur].pop().unwrap();
                        wrong_break_positions.push(temp2);
                    }
                    stacks[cur].push(temp);
                    let top = *stacks[cur].last().unwrap();
                    self.text.set_index(a, top);
                }

                c = self.get_current(a);
                if self.text.get_index() >= end_pos {
                    break;
                }
            } else {
                c = self.get_next(a);
            }
        }

        // replace the last break with the real end of the range
        let current = &mut stacks[cur];
        current.pop();
        current.push(end_pos);

        let mut cache = Vec::with_capacity(current.len() + 1);
        cache.push(start_pos);
        cache.extend_from_slice(current);
        let d = self.dict.as_mut().unwrap();
        d.cached_break_positions = Some(cache);
        d.position_in_cache = 0;
    }
}

// ---------------------------------------------------------------------------
// SegmentingTokenizerBase + ThaiTokenizer

const BUFFERMAX: usize = 1024;

struct ThaiTokenizer {
    /// The text, read through `read` like a java.io.StringReader.
    input: Vec<u16>,
    input_pos: usize,

    buffer: [u16; BUFFERMAX],
    length: i32,
    usable_length: i32,
    offset: i32,
    /// The sentence iterator (`SegmentingTokenizerBase.iterator`).
    iterator: BreakIterator,

    /// The Thai word iterator (`ThaiTokenizer.wordBreaker`).
    word_breaker: BreakIterator,
    sentence_start: i32,
    sentence_end: i32,

    // the current token
    term_start: usize,
    term_end: usize,
    start_offset: i32,
    end_offset: i32,
}

impl ThaiTokenizer {
    fn new(input: Vec<u16>) -> ThaiTokenizer {
        let t = tables();
        let mut tok = ThaiTokenizer {
            input,
            input_pos: 0,
            buffer: [0; BUFFERMAX],
            length: 0,
            usable_length: 0,
            offset: 0,
            iterator: BreakIterator { rules: &t.sentence, dict: None, text: Default::default() },
            word_breaker: BreakIterator {
                rules: &t.word,
                dict: Some(DictState::default()),
                text: Default::default(),
            },
            sentence_start: 0,
            sentence_end: 0,
            term_start: 0,
            term_end: 0,
            start_offset: 0,
            end_offset: 0,
        };
        tok.reset();
        tok
    }

    fn reset(&mut self) {
        self.iterator.set_text(&self.buffer, 0, 0);
        self.length = 0;
        self.usable_length = 0;
        self.offset = 0;
    }

    fn increment_token(&mut self) -> bool {
        if self.length == 0 || !self.increment_word() {
            while !self.increment_sentence() {
                self.refill();
                if self.length <= 0 {
                    // no more bytes to read
                    return false;
                }
            }
        }
        true
    }

    fn find_safe_end(&self) -> i32 {
        for i in (0..self.length).rev() {
            if Self::is_safe_end(self.buffer[i as usize]) {
                return i + 1;
            }
        }
        -1
    }

    fn is_safe_end(ch: u16) -> bool {
        matches!(ch, 0x000D | 0x000A | 0x0085 | 0x2028 | 0x2029)
    }

    fn refill(&mut self) {
        self.offset += self.usable_length;
        let leftover = self.length - self.usable_length;
        self.buffer.copy_within(
            self.usable_length as usize..(self.usable_length + leftover) as usize,
            0,
        );
        let requested = BUFFERMAX as i32 - leftover;
        let returned = self.read(leftover, requested);
        self.length = if returned < 0 { leftover } else { returned + leftover };
        if returned < requested {
            // reader has been emptied, process the rest
            self.usable_length = self.length;
        } else {
            // still more data to be read, find a safe-stopping place
            self.usable_length = self.find_safe_end();
            if self.usable_length < 0 {
                // more than IOBUFFER of text without breaks, gonna possibly
                // truncate tokens
                self.usable_length = self.length;
            }
        }
        self.iterator.set_text(&self.buffer, 0, self.usable_length.max(0));
    }

    /// SegmentingTokenizerBase.read over a StringReader.
    fn read(&mut self, offset: i32, length: i32) -> i32 {
        let mut remaining = length;
        while remaining > 0 {
            let location = length - remaining;
            // StringReader.read(cbuf, off, len)
            let avail = (self.input.len() - self.input_pos) as i32;
            if avail == 0 {
                break; // EOF
            }
            let count = remaining.min(avail);
            let dst = (offset + location) as usize;
            self.buffer[dst..dst + count as usize]
                .copy_from_slice(&self.input[self.input_pos..self.input_pos + count as usize]);
            self.input_pos += count as usize;
            remaining -= count;
        }
        length - remaining
    }

    fn increment_sentence(&mut self) -> bool {
        if self.length == 0 {
            // we must refill the buffer
            return false;
        }
        loop {
            let start = self.iterator.current();
            if start == DONE {
                return false;
            }
            let end = self.iterator.next(&self.buffer);
            if end == DONE {
                return false;
            }
            self.set_next_sentence(start, end);
            if self.increment_word() {
                return true;
            }
        }
    }

    /// ThaiTokenizer.setNextSentence.
    fn set_next_sentence(&mut self, sentence_start: i32, sentence_end: i32) {
        self.sentence_start = sentence_start;
        self.sentence_end = sentence_end;
        self.word_breaker.set_text(&self.buffer, sentence_start, sentence_end - sentence_start);
    }

    /// ThaiTokenizer.incrementWord: the next word segment whose first code
    /// point is a letter or digit.
    fn increment_word(&mut self) -> bool {
        let mut start = self.word_breaker.current();
        if start == DONE {
            return false;
        }
        let mut end = self.word_breaker.next(&self.buffer);
        while end != DONE
            && !is_letter_or_digit(code_point_at(
                &self.buffer,
                (self.sentence_start + start) as usize,
                self.sentence_end as usize,
            ))
        {
            start = end;
            end = self.word_breaker.next(&self.buffer);
        }
        if end == DONE {
            return false;
        }
        self.term_start = (self.sentence_start + start) as usize;
        self.term_end = (self.sentence_start + end) as usize;
        self.start_offset = self.offset + self.sentence_start + start;
        self.end_offset = self.offset + self.sentence_start + end;
        true
    }
}

/// Lucene's ThaiTokenizer (the `thai` tokenizer): sentences by the JDK's root
/// sentence BreakIterator, words by the JDK's Thai dictionary-based word
/// BreakIterator, keeping the segments that start with a letter or digit.
/// Terms are not lowercased; type "word"; offsets in UTF-16 units.
pub fn thai_tokenize(text: &str) -> Vec<Tok> {
    let mut tok = ThaiTokenizer::new(text.encode_utf16().collect());
    let mut out = Vec::new();
    while tok.increment_token() {
        out.push(Tok {
            term: String::from_utf16_lossy(&tok.buffer[tok.term_start..tok.term_end]),
            start: tok.start_offset as usize,
            end: tok.end_offset as usize,
            typ: "word".to_string(),
            pos_inc: 1,
            pos_len: 1,
            keyword: false,
        });
    }
    out
}
