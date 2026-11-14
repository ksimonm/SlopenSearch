//! Port of Lucene's RSLP ("Removedor de Sufixos da Lingua Portuguesa")
//! stemmers: pt/RSLPStemmerBase (rule/step machinery), pt/PortugueseStemmer
//! and gl/GalicianStemmer, gl/GalicianMinimalStemmer. The rule files are
//! converted into `rslp_data.rs` by gen_rslp.py.

use super::rslp_data::*;

/// Exceptions of a rule: whole words (sorted by UTF-16 units, binary searched;
/// Lucene uses a CharArraySet) or suffixes (linear ends-with scan, as Lucene).
pub enum Exceptions {
    None,
    Words(&'static [&'static str]),
    Suffixes(&'static [&'static str]),
}

/// RSLPStemmerBase.Rule / RuleWithSetExceptions / RuleWithSuffixExceptions.
pub struct Rule {
    pub suffix: &'static str,
    /// Length of `suffix` in UTF-16 units.
    pub suffix_len: usize,
    /// Minimum stem length (after removing the suffix).
    pub min: usize,
    pub replacement: &'static str,
    pub exceptions: Exceptions,
}

/// RSLPStemmerBase.Step: an ordered list of rules; the first one that matches fires.
pub struct Step {
    /// Minimum word length to enter the step (already computed when the file says 0).
    pub min: usize,
    /// The word must end with one of these to enter the step (empty: no condition).
    pub suffixes: &'static [&'static str],
    pub rules: &'static [Rule],
}

/// `s[..len]` ends with the BMP-only string `suf` (of `suf_len` UTF-16 units).
fn ends_with_str(s: &[u16], len: usize, suf: &str, suf_len: usize) -> bool {
    suf_len <= len && s[len - suf_len..len].iter().copied().eq(suf.encode_utf16())
}

/// ends_with for strings whose length in UTF-16 units is not precomputed
/// (all RSLP strings are in the BMP, so chars == units).
fn ends_with_any_len(s: &[u16], len: usize, suf: &str) -> bool {
    ends_with_str(s, len, suf, suf.chars().count())
}

impl Rule {
    fn matches(&self, s: &[u16], len: usize) -> bool {
        if !(len >= self.suffix_len + self.min
            && ends_with_str(s, len, self.suffix, self.suffix_len))
        {
            return false;
        }
        match self.exceptions {
            Exceptions::None => true,
            Exceptions::Words(words) => words
                .binary_search_by(|w| w.encode_utf16().cmp(s[..len].iter().copied()))
                .is_err(),
            Exceptions::Suffixes(sufs) => !sufs.iter().any(|e| ends_with_any_len(s, len, e)),
        }
    }

    /// Returns the new length after firing this rule. The buffer must have room
    /// for a replacement longer than the suffix.
    fn replace(&self, s: &mut [u16], len: usize) -> usize {
        let start = len - self.suffix_len;
        let mut end = start;
        for c in self.replacement.encode_utf16() {
            s[end] = c;
            end += 1;
        }
        end
    }
}

impl Step {
    /// Returns the new valid length after applying the entire step.
    pub fn apply(&self, s: &mut [u16], len: usize) -> usize {
        if len < self.min {
            return len;
        }
        if !self.suffixes.is_empty()
            && !self
                .suffixes
                .iter()
                .any(|suf| ends_with_any_len(s, len, suf))
        {
            return len;
        }
        for rule in self.rules {
            if rule.matches(s, len) {
                return rule.replace(s, len);
            }
        }
        len
    }
}

/// Runs an RSLP stem function on a term, with the buffer oversized by one unit
/// (RSLP replacements may be one unit longer than the suffix they replace).
pub fn run(term: &mut Vec<u16>, f: impl FnOnce(&mut [u16], usize) -> usize) {
    let len = term.len();
    term.push(0);
    let n = f(term.as_mut_slice(), len);
    term.truncate(n);
}

// ---------------------------------------------------------------------------
// pt/PortugueseStemmer

/// PortugueseStemFilter: the full RSLP (Orengo) Portuguese stemmer.
pub fn portuguese_rslp_stem(term: &mut Vec<u16>) {
    run(term, portuguese_stem);
}

fn portuguese_stem(s: &mut [u16], mut len: usize) -> usize {
    len = PORTUGUESE_PLURAL.apply(s, len);
    len = PORTUGUESE_ADVERB.apply(s, len);
    len = PORTUGUESE_FEMININE.apply(s, len);
    len = PORTUGUESE_AUGMENTATIVE.apply(s, len);

    let oldlen = len;
    len = PORTUGUESE_NOUN.apply(s, len);

    if len == oldlen {
        // suffix not removed
        let oldlen = len;
        len = PORTUGUESE_VERB.apply(s, len);
        if len == oldlen {
            // suffix not removed
            len = PORTUGUESE_VOWEL.apply(s, len);
        }
    }

    // rslp accent removal
    for c in s[..len].iter_mut() {
        let folded = match char::from_u32(*c as u32) {
            Some('à' | 'á' | 'â' | 'ã' | 'ä' | 'å') => 'a',
            Some('ç') => 'c',
            Some('è' | 'é' | 'ê' | 'ë') => 'e',
            Some('ì' | 'í' | 'î' | 'ï') => 'i',
            Some('ñ') => 'n',
            Some('ò' | 'ó' | 'ô' | 'õ' | 'ö') => 'o',
            Some('ù' | 'ú' | 'û' | 'ü') => 'u',
            Some('ý' | 'ÿ') => 'y',
            _ => continue,
        };
        *c = folded as u16;
    }
    len
}

// ---------------------------------------------------------------------------
// gl/GalicianStemmer

/// GalicianStemFilter: the RSLP-based Galician stemmer ("Regras do lematizador
/// para o galego").
pub fn galician_stem(term: &mut Vec<u16>) {
    run(term, galician);
}

fn galician(s: &mut [u16], mut len: usize) -> usize {
    len = GALICIAN_PLURAL.apply(s, len);
    len = GALICIAN_UNIFICATION.apply(s, len);
    len = GALICIAN_ADVERB.apply(s, len);

    loop {
        let oldlen = len;
        len = GALICIAN_AUGMENTATIVE.apply(s, len);
        if len == oldlen {
            break;
        }
    }

    let oldlen = len;
    len = GALICIAN_NOUN.apply(s, len);
    if len == oldlen {
        // suffix not removed
        len = GALICIAN_VERB.apply(s, len);
    }

    len = GALICIAN_VOWEL.apply(s, len);

    // RSLG accent removal
    for c in s[..len].iter_mut() {
        let folded = match char::from_u32(*c as u32) {
            Some('á') => 'a',
            Some('é' | 'ê') => 'e',
            Some('í') => 'i',
            Some('ó') => 'o',
            Some('ú') => 'u',
            _ => continue,
        };
        *c = folded as u16;
    }
    len
}

// ---------------------------------------------------------------------------
// gl/GalicianMinimalStemmer

/// GalicianMinimalStemFilter: only the "Plural" step of galician.rslp (Lucene
/// has no separate galician-minimal rule file; the minimal stemmer reads
/// galician.rslp too).
pub fn galician_minimal_stem(term: &mut Vec<u16>) {
    run(term, |s, len| GALICIAN_PLURAL.apply(s, len));
}
