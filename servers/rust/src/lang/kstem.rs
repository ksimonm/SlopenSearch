//! Port of Lucene's org.apache.lucene.analysis.en.KStemmer (Bob Krovetz'
//! KStem, Lucid Imagination / CIIR version), as driven by KStemFilter.
//!
//! The Java code works on an OpenStringBuilder whose `setLength` only moves
//! the length (characters past it stay in the buffer and are read back by
//! `charAt`/`setCharAt`); `Word` reproduces that with a fixed buffer.
//! Indices are `isize` like the Java ints (`j` can become -1).

use super::kstem_data::*;
use std::collections::HashMap;
use std::sync::OnceLock;

const MAX_WORD_LEN: usize = 50;

/// KStemmer.DictEntry: `root == None` means the word itself is the root.
struct DictEntry {
    exception: bool,
    root: Option<Vec<u16>>,
}

/// KStemmer.dict_ht (a case-sensitive CharArrayMap), built once.
fn dict() -> &'static HashMap<Vec<u16>, DictEntry> {
    static DICT: OnceLock<HashMap<Vec<u16>, DictEntry>> = OnceLock::new();
    DICT.get_or_init(|| {
        let mut d = HashMap::with_capacity(30000);
        let enc = |s: &str| s.encode_utf16().collect::<Vec<u16>>();
        // (Java throws on duplicate entries; the generator checks there are none.)
        for w in EXCEPTION_WORDS {
            d.insert(enc(w), DictEntry { exception: true, root: Some(enc(w)) });
        }
        for (w, root) in DIRECT_CONFLATIONS.iter().chain(COUNTRY_NATIONALITY) {
            d.insert(enc(w), DictEntry { exception: false, root: Some(enc(root)) });
        }
        for w in HEAD_WORDS.iter().chain(SUPPLEMENT_DICT).chain(PROPER_NOUNS) {
            d.insert(enc(w), DictEntry { exception: false, root: None });
        }
        d
    })
}

/// The subset of OpenStringBuilder that KStemmer uses.
struct Word {
    buf: [u16; MAX_WORD_LEN + 16],
    len: usize,
}

impl Word {
    fn char_at(&self, i: isize) -> u16 {
        self.buf[i as usize]
    }
    fn set_char_at(&mut self, i: isize, ch: char) {
        self.buf[i as usize] = ch as u16;
    }
    fn set_length(&mut self, len: isize) {
        self.len = len as usize;
    }
    fn unsafe_write(&mut self, ch: u16) {
        self.buf[self.len] = ch;
        self.len += 1;
    }
    fn write_char(&mut self, ch: char) {
        self.unsafe_write(ch as u16);
    }
    fn append(&mut self, s: &str) {
        for b in s.bytes() {
            self.unsafe_write(b as u16);
        }
    }
    fn as_slice(&self) -> &[u16] {
        &self.buf[..self.len]
    }
    fn length(&self) -> isize {
        self.len as isize
    }
}

struct KStemmer {
    dict: &'static HashMap<Vec<u16>, DictEntry>,
    word: Word,
    /// index of final letter in stem (within word)
    j: isize,
    /// INDEX of final letter in word (length is k+1)
    k: isize,
    matched_entry: Option<&'static DictEntry>,
}

fn is_alpha(ch: u16) -> bool {
    (0x61..=0x7A).contains(&ch) // terms must be lowercased already
}

const fn c(ch: char) -> u16 {
    ch as u16
}

impl KStemmer {
    fn penult_char(&self) -> u16 {
        self.word.char_at(self.k - 1)
    }

    fn is_vowel(&self, index: isize) -> bool {
        !self.is_cons(index)
    }

    fn is_cons(&self, index: isize) -> bool {
        let ch = self.word.char_at(index);
        if ch == c('a') || ch == c('e') || ch == c('i') || ch == c('o') || ch == c('u') {
            return false;
        }
        if ch != c('y') || index == 0 {
            true
        } else {
            !self.is_cons(index - 1)
        }
    }

    /// length of stem within word
    fn stem_length(&self) -> isize {
        self.j + 1
    }

    fn ends_in(&mut self, s: &str) -> bool {
        let s = s.as_bytes();
        if s.len() as isize > self.k {
            return false;
        }
        let r = self.word.length() - s.len() as isize; // length of word before this suffix
        self.j = self.k;
        for (i, &ch) in s.iter().enumerate() {
            if self.word.char_at(r + i as isize) != ch as u16 {
                return false;
            }
        }
        self.j = r - 1; // index of the character BEFORE the posfix
        true
    }

    fn ends_in2(&mut self, a: char, b: char) -> bool {
        if 2 > self.k {
            return false;
        }
        if self.word.char_at(self.k - 1) == c(a) && self.word.char_at(self.k) == c(b) {
            self.j = self.k - 2;
            return true;
        }
        false
    }

    fn ends_in3(&mut self, a: char, b: char, cc: char) -> bool {
        if 3 > self.k {
            return false;
        }
        let w = &self.word;
        if w.char_at(self.k - 2) == c(a) && w.char_at(self.k - 1) == c(b) && w.char_at(self.k) == c(cc) {
            self.j = self.k - 3;
            return true;
        }
        false
    }

    fn ends_in4(&mut self, a: char, b: char, cc: char, d: char) -> bool {
        if 4 > self.k {
            return false;
        }
        let w = &self.word;
        if w.char_at(self.k - 3) == c(a)
            && w.char_at(self.k - 2) == c(b)
            && w.char_at(self.k - 1) == c(cc)
            && w.char_at(self.k) == c(d)
        {
            self.j = self.k - 4;
            return true;
        }
        false
    }

    fn word_in_dict(&mut self) -> Option<&'static DictEntry> {
        if self.matched_entry.is_some() {
            return self.matched_entry;
        }
        let e = self.dict.get(self.word.as_slice());
        if let Some(e) = e {
            if !e.exception {
                self.matched_entry = Some(e); // only cache if it's not an exception.
            }
        }
        e
    }

    /// Returns true if the word is found in the dictionary (and remembers the match).
    fn lookup(&mut self) -> bool {
        self.matched_entry = self.dict.get(self.word.as_slice());
        self.matched_entry.is_some()
    }

    fn matched(&self) -> bool {
        self.matched_entry.is_some()
    }

    fn set_suffix(&mut self, s: &str) {
        self.word.set_length(self.j + 1);
        self.word.append(s);
        self.k = self.j + s.len() as isize;
    }

    /// Convert plurals to singular form, and '-ies' to 'y'
    fn plural(&mut self) {
        if self.word.char_at(self.k) == c('s') {
            if self.ends_in3('i', 'e', 's') {
                self.word.set_length(self.j + 3);
                self.k -= 1;
                if self.lookup() {
                    return; // ensure calories -> calorie
                }
                self.k += 1;
                self.word.write_char('s');
                self.set_suffix("y");
                self.lookup();
            } else if self.ends_in2('e', 's') {
                // try just removing the "s"
                self.word.set_length(self.j + 2);
                self.k -= 1;
                // don't check for exceptions here (`aides' -> `aide'); the double
                // s check prevents crosses -> crosse.
                let try_e = self.j > 0
                    && !(self.word.char_at(self.j) == c('s') && self.word.char_at(self.j - 1) == c('s'));
                if try_e && self.lookup() {
                    return;
                }
                // try removing the "es"
                self.word.set_length(self.j + 1);
                self.k -= 1;
                if self.lookup() {
                    return;
                }
                // the default is to retain the "e"
                self.word.write_char('e');
                self.k += 1;
                if !try_e {
                    self.lookup(); // if we didn't try the "e" ending before
                }
            } else if self.word.length() > 3 && self.penult_char() != c('s') && !self.ends_in3('o', 'u', 's') {
                // unless the word ends in "ous" or a double "s", remove the final "s"
                self.word.set_length(self.k);
                self.k -= 1;
                self.lookup();
            }
        }
    }

    /// convert past tense (-ed) to present, and `-ied' to `y'
    fn past_tense(&mut self) {
        // Handle words less than 5 letters with a direct mapping (prevents fled -> fl).
        if self.word.length() <= 4 {
            return;
        }
        if self.ends_in3('i', 'e', 'd') {
            self.word.set_length(self.j + 3);
            self.k -= 1;
            if self.lookup() {
                return; // this isn't true for short words (died->die)
            }
            self.k += 1;
            self.word.write_char('d');
            self.set_suffix("y");
            self.lookup();
            return;
        }
        // the vowelInStem() is necessary so we don't stem acronyms
        if self.ends_in2('e', 'd') && self.vowel_in_stem() {
            // see if the root ends in `e'
            self.word.set_length(self.j + 2);
            self.k = self.j + 1;
            if let Some(entry) = self.word_in_dict() {
                if !entry.exception {
                    return; // in the dictionary and not an exception
                }
            }
            // try removing the "ed"
            self.word.set_length(self.j + 1);
            self.k = self.j;
            if self.lookup() {
                return;
            }
            // try removing a doubled consonant; if the root isn't found, leave it doubled
            if self.double_c(self.k) {
                self.word.set_length(self.k);
                self.k -= 1;
                if self.lookup() {
                    return;
                }
                let ch = self.word.char_at(self.k);
                self.word.unsafe_write(ch);
                self.k += 1;
                self.lookup();
                return;
            }
            // if we have a `un-' prefix, then leave the word alone
            if self.word.char_at(0) == c('u') && self.word.char_at(1) == c('n') {
                self.word.write_char('e');
                self.word.write_char('d');
                self.k += 2;
                return;
            }
            // prefer to end with an `e' (e.g., `microcoded' -> `microcode')
            self.word.set_length(self.j + 1);
            self.word.write_char('e');
            self.k = self.j + 1;
        }
    }

    /// return TRUE if word ends with a double consonant
    fn double_c(&self, i: isize) -> bool {
        if i < 1 {
            return false;
        }
        if self.word.char_at(i) != self.word.char_at(i - 1) {
            return false;
        }
        self.is_cons(i)
    }

    fn vowel_in_stem(&self) -> bool {
        (0..self.stem_length()).any(|i| self.is_vowel(i))
    }

    /// handle `-ing' endings
    fn aspect(&mut self) {
        // handle short words (aging -> age) via a direct mapping
        if self.word.length() <= 5 {
            return;
        }
        // the vowelinstem() is necessary so we don't stem acronyms
        if self.ends_in3('i', 'n', 'g') && self.vowel_in_stem() {
            // try adding an `e' to the stem and check against the dictionary
            self.word.set_char_at(self.j + 1, 'e');
            self.word.set_length(self.j + 2);
            self.k = self.j + 1;
            if let Some(entry) = self.word_in_dict() {
                if !entry.exception {
                    return;
                }
            }
            // adding on the `e' didn't work, so remove it
            self.word.set_length(self.k);
            self.k -= 1; // note that `ing' has also been removed
            if self.lookup() {
                return;
            }
            // if I can remove a doubled consonant and get a word, then do so
            if self.double_c(self.k) {
                self.k -= 1;
                self.word.set_length(self.k + 1);
                if self.lookup() {
                    return;
                }
                let ch = self.word.char_at(self.k);
                self.word.unsafe_write(ch); // restore the doubled consonant
                // the default is to leave the consonant doubled
                self.k += 1;
                self.lookup();
                return;
            }
            // default: add an `e' unless the word ends in two consonants
            if self.j > 0 && self.is_cons(self.j) && self.is_cons(self.j - 1) {
                self.k = self.j;
                self.word.set_length(self.k + 1);
                return;
            }
            self.word.set_length(self.j + 1);
            self.word.write_char('e');
            self.k = self.j + 1;
        }
    }

    /// -ity endings; -ability, -ibility, -ivity and -ality are accepted without
    /// checking the dictionary.
    fn ity_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in3('i', 't', 'y') {
            self.word.set_length(self.j + 1); // try just removing -ity
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.write_char('e'); // try removing -ity and adding -e
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_char_at(self.j + 1, 'i');
            self.word.append("ty");
            self.k = old_k;
            let j = self.j;
            if j > 0 && self.word.char_at(j - 1) == c('i') && self.word.char_at(j) == c('l') {
                self.word.set_length(j - 1);
                self.word.append("le"); // convert to -ble
                self.k = j;
                self.lookup();
                return;
            }
            // ditto for -ivity
            if j > 0 && self.word.char_at(j - 1) == c('i') && self.word.char_at(j) == c('v') {
                self.word.set_length(j + 1);
                self.word.write_char('e'); // convert to -ive
                self.k = j + 1;
                self.lookup();
                return;
            }
            // ditto for -ality
            if j > 0 && self.word.char_at(j - 1) == c('a') && self.word.char_at(j) == c('l') {
                self.word.set_length(j + 1);
                self.k = j;
                self.lookup();
                return;
            }
            // if the root isn't in the dictionary, and the variant *is* there, use the variant
            if self.lookup() {
                return;
            }
            // the default is to remove -ity altogether
            self.word.set_length(j + 1);
            self.k = j;
        }
    }

    /// handle -ence and -ance
    fn nce_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in3('n', 'c', 'e') {
            let word_char = self.word.char_at(self.j);
            if !(word_char == c('e') || word_char == c('a')) {
                return;
            }
            self.word.set_length(self.j);
            self.word.write_char('e'); // try converting -e/ance to -e (adherance/adhere)
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j); // try removing -e/ance altogether
            self.k = self.j - 1;
            if self.lookup() {
                return;
            }
            self.word.unsafe_write(word_char); // restore the original ending
            self.word.append("nce");
            self.k = old_k;
        }
    }

    /// handle -ness
    fn ness_endings(&mut self) {
        if self.ends_in4('n', 'e', 's', 's') {
            // this is a very productive ending, so just accept it
            self.word.set_length(self.j + 1);
            self.k = self.j;
            if self.word.char_at(self.j) == c('i') {
                self.word.set_char_at(self.j, 'y');
            }
            self.lookup();
        }
    }

    /// handle -ism
    fn ism_endings(&mut self) {
        if self.ends_in3('i', 's', 'm') {
            // this is a very productive ending, so just accept it
            self.word.set_length(self.j + 1);
            self.k = self.j;
            self.lookup();
        }
    }

    /// -ment endings
    fn ment_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in4('m', 'e', 'n', 't') {
            self.word.set_length(self.j + 1);
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.append("ment");
            self.k = old_k;
        }
    }

    /// -ize endings
    fn ize_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in3('i', 'z', 'e') {
            self.word.set_length(self.j + 1); // try removing -ize entirely
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.write_char('i');
            if self.double_c(self.j) {
                // allow for a doubled consonant
                self.word.set_length(self.j);
                self.k = self.j - 1;
                if self.lookup() {
                    return;
                }
                let ch = self.word.char_at(self.j - 1);
                self.word.unsafe_write(ch);
            }
            self.word.set_length(self.j + 1);
            self.word.write_char('e'); // try removing -ize and adding -e
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j + 1);
            self.word.append("ize");
            self.k = old_k;
        }
    }

    /// handle -ency and -ancy
    fn ncy_endings(&mut self) {
        if self.ends_in3('n', 'c', 'y') {
            if !(self.word.char_at(self.j) == c('e') || self.word.char_at(self.j) == c('a')) {
                return;
            }
            self.word.set_char_at(self.j + 2, 't'); // try converting -ncy to -nt
            self.word.set_length(self.j + 3);
            self.k = self.j + 2;
            if self.lookup() {
                return;
            }
            self.word.set_char_at(self.j + 2, 'c'); // the default is to convert it to -nce
            self.word.write_char('e');
            self.k = self.j + 3;
            self.lookup();
        }
    }

    /// handle -able and -ible
    fn ble_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in3('b', 'l', 'e') {
            if !(self.word.char_at(self.j) == c('a') || self.word.char_at(self.j) == c('i')) {
                return;
            }
            let word_char = self.word.char_at(self.j);
            self.word.set_length(self.j); // try just removing the ending
            self.k = self.j - 1;
            if self.lookup() {
                return;
            }
            if self.double_c(self.k) {
                // allow for a doubled consonant
                self.word.set_length(self.k);
                self.k -= 1;
                if self.lookup() {
                    return;
                }
                self.k += 1;
                let ch = self.word.char_at(self.k - 1);
                self.word.unsafe_write(ch);
            }
            self.word.set_length(self.j);
            self.word.write_char('e'); // try removing -a/ible and adding -e
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j);
            self.word.append("ate"); // try removing -able and adding -ate
            self.k = self.j + 2;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j);
            self.word.unsafe_write(word_char); // restore the original values
            self.word.append("ble");
            self.k = old_k;
        }
    }

    /// -ic endings; the only place an ending is *expanded* (-ic -> -ical).
    fn ic_endings(&mut self) {
        if self.ends_in2('i', 'c') {
            self.word.set_length(self.j + 3);
            self.word.append("al"); // try converting -ic to -ical
            self.k = self.j + 4;
            if self.lookup() {
                return;
            }
            self.word.set_char_at(self.j + 1, 'y'); // try converting -ic to -y
            self.word.set_length(self.j + 2);
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_char_at(self.j + 1, 'e'); // try converting -ic to -e
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j + 1); // try removing -ic altogether
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.append("ic"); // restore the original ending
            self.k = self.j + 2;
        }
    }

    /// -ion, -ition, -ation, -ization, and -ication (-ization always -> -ize)
    fn ion_endings(&mut self) {
        let old_k = self.k;
        if !self.ends_in3('i', 'o', 'n') {
            return;
        }
        if self.ends_in("ization") {
            // the -ize ending is very productive, so simply accept it as the root
            self.word.set_length(self.j + 3);
            self.word.write_char('e');
            self.k = self.j + 3;
            self.lookup();
            return;
        }
        if self.ends_in("ition") {
            self.word.set_length(self.j + 1);
            self.word.write_char('e');
            self.k = self.j + 1;
            if self.lookup() {
                return; // (e.g., definition->define, opposition->oppose)
            }
            // restore original values
            self.word.set_length(self.j + 1);
            self.word.append("ition");
            self.k = old_k;
        } else if self.ends_in("ation") {
            self.word.set_length(self.j + 3);
            self.word.write_char('e');
            self.k = self.j + 3;
            if self.lookup() {
                return; // (elmination -> eliminate)
            }
            self.word.set_length(self.j + 1);
            self.word.write_char('e'); // remove -ation and add `e'
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j + 1); // just remove -ation (resignation->resign)
            self.k = self.j;
            if self.lookup() {
                return;
            }
            // restore original values
            self.word.set_length(self.j + 1);
            self.word.append("ation");
            self.k = old_k;
        }
        // test -ication after -ation is attempted (complication->complicate, not comply)
        if self.ends_in("ication") {
            self.word.set_length(self.j + 1);
            self.word.write_char('y');
            self.k = self.j + 1;
            if self.lookup() {
                return; // (e.g., amplification -> amplify)
            }
            // restore original values
            self.word.set_length(self.j + 1);
            self.word.append("ication");
            self.k = old_k;
        }
        // we checked for -ion earlier... just need to set "j"
        self.j = self.k - 3;
        self.word.set_length(self.j + 1);
        self.word.write_char('e');
        self.k = self.j + 1;
        if self.lookup() {
            return; // remove -ion and add `e'
        }
        self.word.set_length(self.j + 1);
        self.k = self.j;
        if self.lookup() {
            return; // remove -ion, and if it's found, treat that as the root
        }
        // restore original values
        self.word.set_length(self.j + 1);
        self.word.append("ion");
        self.k = old_k;
    }

    /// -er, -or, -ier, and -eer (-izer is always converted to -ize)
    fn er_and_or_endings(&mut self) {
        let old_k = self.k;
        if self.word.char_at(self.k) != c('r') {
            return; // YCS
        }
        if self.ends_in4('i', 'z', 'e', 'r') {
            // -ize is very productive, so accept it as the root
            self.word.set_length(self.j + 4);
            self.k = self.j + 3;
            self.lookup();
            return;
        }
        if self.ends_in2('e', 'r') || self.ends_in2('o', 'r') {
            let word_char = self.word.char_at(self.j + 1); // remember if it was -er or -or
            if self.double_c(self.j) {
                self.word.set_length(self.j);
                self.k = self.j - 1;
                if self.lookup() {
                    return;
                }
                let ch = self.word.char_at(self.j - 1);
                self.word.unsafe_write(ch); // restore the doubled consonant
            }
            if self.word.char_at(self.j) == c('i') {
                // do we have a -ier ending?
                self.word.set_char_at(self.j, 'y');
                self.word.set_length(self.j + 1);
                self.k = self.j;
                if self.lookup() {
                    return;
                }
                self.word.set_char_at(self.j, 'i'); // restore the endings
                self.word.write_char('e');
            }
            if self.word.char_at(self.j) == c('e') {
                // handle -eer
                self.word.set_length(self.j);
                self.k = self.j - 1;
                if self.lookup() {
                    return;
                }
                self.word.write_char('e');
            }
            self.word.set_length(self.j + 2); // remove the -r ending
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j + 1); // try removing -er/-or
            self.k = self.j;
            if self.lookup() {
                return;
            }
            self.word.write_char('e'); // try removing -or and adding -e
            self.k = self.j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(self.j + 1);
            self.word.unsafe_write(word_char);
            self.word.write_char('r'); // restore the word to the way it was
            self.k = old_k;
        }
    }

    /// -ly endings (-ally is always converted to -al)
    fn ly_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in2('l', 'y') {
            self.word.set_char_at(self.j + 2, 'e'); // try converting -ly to -le
            if self.lookup() {
                return;
            }
            self.word.set_char_at(self.j + 2, 'y');
            self.word.set_length(self.j + 1); // try just removing the -ly
            self.k = self.j;
            if self.lookup() {
                return;
            }
            let j = self.j;
            if j > 0 && self.word.char_at(j - 1) == c('a') && self.word.char_at(j) == c('l') {
                return; // always convert -ally to -al
            }
            self.word.append("ly");
            self.k = old_k;
            if j > 0 && self.word.char_at(j - 1) == c('a') && self.word.char_at(j) == c('b') {
                // always convert -ably to -able
                self.word.set_char_at(j + 2, 'e');
                self.k = j + 2;
                return;
            }
            if self.word.char_at(j) == c('i') {
                // e.g., militarily -> military
                self.word.set_length(j);
                self.word.write_char('y');
                self.k = j;
                if self.lookup() {
                    return;
                }
                self.word.set_length(j);
                self.word.append("ily");
                self.k = old_k;
            }
            self.word.set_length(j + 1); // the default is to remove -ly
            self.k = j;
        }
    }

    /// -al endings
    fn al_endings(&mut self) {
        let old_k = self.k;
        if self.word.length() < 4 {
            return;
        }
        if self.ends_in2('a', 'l') {
            let j = self.j;
            self.word.set_length(j + 1);
            self.k = j;
            if self.lookup() {
                return; // try just removing the -al
            }
            if self.double_c(j) {
                // allow for a doubled consonant
                self.word.set_length(j);
                self.k = j - 1;
                if self.lookup() {
                    return;
                }
                let ch = self.word.char_at(j - 1);
                self.word.unsafe_write(ch);
            }
            self.word.set_length(j + 1);
            self.word.write_char('e'); // try removing the -al and adding -e
            self.k = j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(j + 1);
            self.word.append("um"); // try converting -al to -um (optimal -> optimum)
            self.k = j + 2;
            if self.lookup() {
                return;
            }
            self.word.set_length(j + 1);
            self.word.append("al"); // restore the ending to the way it was
            self.k = old_k;
            if j > 0 && self.word.char_at(j - 1) == c('i') && self.word.char_at(j) == c('c') {
                self.word.set_length(j - 1); // try removing -ical
                self.k = j - 2;
                if self.lookup() {
                    return;
                }
                self.word.set_length(j - 1);
                self.word.write_char('y'); // try turning -ical to -y (e.g., bibliographical)
                self.k = j - 1;
                if self.lookup() {
                    return;
                }
                self.word.set_length(j - 1);
                self.word.append("ic"); // the default is to convert -ical to -ic
                self.k = j;
                self.lookup();
                return;
            }
            if self.word.char_at(j) == c('i') {
                // sometimes -ial endings should be removed
                self.word.set_length(j);
                self.k = j - 1;
                if self.lookup() {
                    return;
                }
                self.word.append("ial");
                self.k = old_k;
                self.lookup();
            }
        }
    }

    /// -ive endings (normalizes some -ative endings, maps some -ive to -ion)
    fn ive_endings(&mut self) {
        let old_k = self.k;
        if self.ends_in3('i', 'v', 'e') {
            let j = self.j;
            self.word.set_length(j + 1); // try removing -ive entirely
            self.k = j;
            if self.lookup() {
                return;
            }
            self.word.write_char('e'); // try removing -ive and adding -e
            self.k = j + 1;
            if self.lookup() {
                return;
            }
            self.word.set_length(j + 1);
            self.word.append("ive");
            if j > 0 && self.word.char_at(j - 1) == c('a') && self.word.char_at(j) == c('t') {
                self.word.set_char_at(j - 1, 'e'); // try removing -ative and adding -e
                self.word.set_length(j); // (e.g., determinative -> determine)
                self.k = j - 1;
                if self.lookup() {
                    return;
                }
                self.word.set_length(j - 1); // try just removing -ative
                if self.lookup() {
                    return;
                }
                self.word.append("ative");
                self.k = old_k;
            }
            // try mapping -ive to -ion (e.g., injunctive/injunction)
            self.word.set_char_at(j + 2, 'o');
            self.word.set_char_at(j + 3, 'n');
            if self.lookup() {
                return;
            }
            self.word.set_char_at(j + 2, 'v'); // restore the original values
            self.word.set_char_at(j + 3, 'e');
            self.k = old_k;
        }
    }

    /// KStemmer.stem(char[], int): `Some(stem)` if the term changes (Java returns
    /// true; the stem may equal the input), `None` to keep the term.
    fn stem(term: &[u16]) -> Option<Vec<u16>> {
        let len = term.len();
        if len <= 2 || len >= MAX_WORD_LEN {
            return None; // don't stem (k <= 1 || k >= MaxWordLen - 1)
        }
        let dict = dict();
        // first check the stemmer dictionaries
        if let Some(entry) = dict.get(term) {
            return entry.root.clone();
        }
        if !term.iter().all(|&ch| is_alpha(ch)) {
            return None; // don't stem
        }
        let mut st = KStemmer {
            dict,
            word: Word { buf: [0; MAX_WORD_LEN + 16], len },
            j: 0,
            k: len as isize - 1,
            matched_entry: None,
        };
        st.word.buf[..len].copy_from_slice(term);

        // Each step runs only if no earlier one matched a dictionary word.
        // (`entry = wordInDict()` after alEndings may set matchedEntry, which
        // iveEndings can then overwrite through its own lookups.)
        let steps: [fn(&mut KStemmer); 17] = [
            KStemmer::plural,
            KStemmer::past_tense,
            KStemmer::aspect,
            KStemmer::ity_endings,
            KStemmer::ness_endings,
            KStemmer::ion_endings,
            KStemmer::er_and_or_endings,
            KStemmer::ly_endings,
            KStemmer::al_endings,
            |s| {
                s.word_in_dict();
                s.ive_endings()
            },
            KStemmer::ize_endings,
            KStemmer::ment_endings,
            KStemmer::ble_endings,
            KStemmer::ism_endings,
            KStemmer::ic_endings,
            KStemmer::ncy_endings,
            KStemmer::nce_endings,
        ];
        for step in steps {
            step(&mut st);
            if st.matched() {
                break;
            }
        }

        // try for a direct mapping (`Italian'->`Italy' and `Italians'->`Italy');
        // a null root means that "word" is the stem
        if let Some(DictEntry { root: Some(root), .. }) = st.matched_entry {
            return Some(root.clone());
        }
        Some(st.word.as_slice().to_vec())
    }
}

/// KStemFilter's per-term transformation: replaces the term with the KStem
/// stem whenever KStemmer.stem() returns true (terms of length 3..=49; only
/// all-[a-z] terms are stemmed, other terms change only if they are
/// dictionary words with a fixed root).
pub fn kstem(term: &mut Vec<u16>) {
    if let Some(s) = KStemmer::stem(term) {
        *term = s;
    }
}
