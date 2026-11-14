//! Ports of Lucene's en/EnglishMinimalStemmer (Harman's "S-Stemmer", as used by
//! EnglishMinimalStemFilter) and OpenSearch's
//! EnglishPluralStemFilter.EnglishPluralStemmer (analysis-common).

use super::stem_util::apply;

const fn c(ch: char) -> u16 {
    ch as u16
}

fn minimal_stem(s: &mut [u16], len: usize) -> usize {
    if len < 3 || s[len - 1] != c('s') {
        return len;
    }
    match s[len - 2] {
        0x75 | 0x73 => len, // u, s
        0x65 => {
            // e
            if len > 3 && s[len - 3] == c('i') && s[len - 4] != c('a') && s[len - 4] != c('e') {
                s[len - 3] = c('y');
                return len - 2;
            }
            if s[len - 3] == c('i') || s[len - 3] == c('a') || s[len - 3] == c('o') || s[len - 3] == c('e') {
                return len;
            }
            len - 1 // intentional fallthrough to default
        }
        _ => len - 1,
    }
}

/// EnglishMinimalStemFilter: strips plural -s/-es/-ies (-ies -> -y).
pub fn english_minimal_stem(term: &mut Vec<u16>) {
    apply(term, minimal_stem);
}

/// Words ending in oes that retain the e when stemmed
const OES_EXCEPTIONS: &[&str] = &["shoes", "canoes", "oboes"];
/// Words ending in ches that retain the e when stemmed
const CHES_EXCEPTIONS: &[&str] = &[
    "cliches",
    "avalanches",
    "mustaches",
    "moustaches",
    "quiches",
    "headaches",
    "heartaches",
    "porsches",
    "tranches",
    "caches",
];

/// EnglishPluralStemmer.isException: compares from the end until either side
/// runs out, so a term that is a suffix of an exception word also matches.
fn is_exception(s: &[u16], len: usize, exceptions: &[&str]) -> bool {
    exceptions.iter().any(|rule| rule.bytes().rev().zip(s[..len].iter().rev()).all(|(r, &ch)| r as u16 == ch))
}

fn plural_stem(s: &mut [u16], len: usize) -> usize {
    if len < 3 || s[len - 1] != c('s') {
        return len;
    }
    match s[len - 2] {
        0x75 | 0x73 => len, // u, s
        0x65 => {
            // e: ies->y only on strings > 4 (spies -> spy, but pies -> pie)
            if len > 4 && s[len - 3] == c('i') {
                s[len - 3] = c('y');
                return len - 2;
            }
            // Suffix rules to remove any dangling "e"
            if len > 3 {
                // xes (but >1 prefix so we can stem "boxes->box" but keep "axes->axe")
                if len > 4 && s[len - 3] == c('x') {
                    return len - 2;
                }
                // oes
                if s[len - 3] == c('o') {
                    if is_exception(s, len, OES_EXCEPTIONS) {
                        return len - 1; // Only remove the S
                    }
                    return len - 2; // Remove the es
                }
                if len > 4 {
                    // shes/sses
                    if s[len - 4] == c('s') && (s[len - 3] == c('h') || s[len - 3] == c('s')) {
                        return len - 2;
                    }
                    // ches
                    if s[len - 4] == c('c') && s[len - 3] == c('h') {
                        if is_exception(s, len, CHES_EXCEPTIONS) {
                            return len - 1; // Only remove the S
                        }
                        return len - 2; // Remove the es
                    }
                }
            }
            len - 1 // fallthrough to default
        }
        _ => len - 1,
    }
}

/// OpenSearch EnglishPluralStemFilter (`stemmer` language "plural_english"):
/// the minimal stemmer plus -xes/-sses/-shes/-ches/-oes rules (with exception
/// words) and -ies -> -y only for terms longer than 4.
pub fn english_plural_stem(term: &mut Vec<u16>) {
    apply(term, plural_stem);
}
