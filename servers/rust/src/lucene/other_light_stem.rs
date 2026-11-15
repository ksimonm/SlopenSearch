//! Ports of Lucene's light stemmers for Finnish, Hungarian and Russian
//! (org.apache.lucene.analysis.{fi,hu,ru}). Each `pub fn` is what the
//! corresponding `*LightStemFilter` does to one non-keyword term.

use super::stem_util::*;

/// A UTF-16 unit as a `char` for matching against literals (surrogates, which
/// never match a literal, become U+FFFD).
fn ch(c: u16) -> char {
    char::from_u32(c as u32).unwrap_or('\u{FFFD}')
}

const fn w(c: char) -> u16 {
    c as u16
}

fn ends_with_any(s: &[u16], len: usize, suffixes: &[&str]) -> bool {
    suffixes.iter().any(|x| ends_with(s, len, x))
}

fn is_vowel(c: u16) -> bool {
    matches!(ch(c), 'a' | 'e' | 'i' | 'o' | 'u' | 'y')
}

// ---------------------------------------------------------------------------
// fi/FinnishLightStemmer

/// FinnishLightStemFilter: the light Finnish stemmer (Savoy).
pub fn finnish_light_stem(term: &mut Vec<u16>) {
    apply(term, finnish_light);
}

fn finnish_light(s: &mut [u16], mut len: usize) -> usize {
    if len < 4 {
        return len;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'ä' | 'å' => *c = w('a'),
            'ö' => *c = w('o'),
            _ => {}
        }
    }

    len = fi_step1(s, len);
    len = fi_step2(s, len);
    len = fi_step3(s, len);
    len = fi_norm1(s, len);
    fi_norm2(s, len)
}

fn fi_step1(s: &[u16], len: usize) -> usize {
    if len > 8 {
        if ends_with(s, len, "kin") {
            return fi_step1(s, len - 3);
        }
        if ends_with(s, len, "ko") {
            return fi_step1(s, len - 2);
        }
    }

    if len > 11 {
        if ends_with(s, len, "dellinen") {
            return len - 8;
        }
        if ends_with(s, len, "dellisuus") {
            return len - 9;
        }
    }
    len
}

fn fi_step2(s: &[u16], len: usize) -> usize {
    if len > 5 {
        if ends_with_any(s, len, &["lla", "tse", "sti"]) {
            return len - 3;
        }

        if ends_with(s, len, "ni") {
            return len - 2;
        }

        if ends_with(s, len, "aa") {
            return len - 1; // aa -> a
        }
    }

    len
}

fn fi_step3(s: &mut [u16], len: usize) -> usize {
    if len > 8 {
        if ends_with(s, len, "nnen") {
            s[len - 4] = w('s');
            return len - 3;
        }

        if ends_with(s, len, "ntena") {
            s[len - 5] = w('s');
            return len - 4;
        }

        if ends_with(s, len, "tten") {
            return len - 4;
        }

        if ends_with(s, len, "eiden") {
            return len - 5;
        }
    }

    if len > 6 {
        if ends_with_any(s, len, &["neen", "niin", "seen", "teen", "inen"]) {
            return len - 4;
        }

        if s[len - 3] == w('h') && is_vowel(s[len - 2]) && s[len - 1] == w('n') {
            return len - 3;
        }

        if ends_with(s, len, "den") {
            s[len - 3] = w('s');
            return len - 2;
        }

        if ends_with(s, len, "ksen") {
            s[len - 4] = w('s');
            return len - 3;
        }

        if ends_with_any(s, len, &["ssa", "sta", "lla", "lta", "tta", "ksi", "lle"]) {
            return len - 3;
        }
    }

    if len > 5 {
        if ends_with_any(s, len, &["na", "ne"]) {
            return len - 2;
        }

        if ends_with(s, len, "nei") {
            return len - 3;
        }
    }

    if len > 4 {
        if ends_with_any(s, len, &["ja", "ta"]) {
            return len - 2;
        }

        if s[len - 1] == w('a') {
            return len - 1;
        }

        if s[len - 1] == w('n') && is_vowel(s[len - 2]) {
            return len - 2;
        }

        if s[len - 1] == w('n') {
            return len - 1;
        }
    }

    len
}

fn fi_norm1(s: &mut [u16], len: usize) -> usize {
    if len > 5 && ends_with(s, len, "hde") {
        s[len - 3] = w('k');
        s[len - 2] = w('s');
        s[len - 1] = w('i');
    }

    if len > 4 && ends_with_any(s, len, &["ei", "at"]) {
        return len - 2;
    }

    if len > 3 && matches!(ch(s[len - 1]), 't' | 's' | 'j' | 'e' | 'a' | 'i') {
        return len - 1;
    }

    len
}

fn fi_norm2(s: &mut [u16], mut len: usize) -> usize {
    if len > 8 && matches!(ch(s[len - 1]), 'e' | 'o' | 'u') {
        len -= 1;
    }

    if len > 4 {
        if s[len - 1] == w('i') {
            len -= 1;
        }

        if len > 4 {
            // Squeeze doubled k, p and t.
            let mut prev = s[0];
            let mut i = 1;
            while i < len {
                if s[i] == prev && matches!(ch(prev), 'k' | 'p' | 't') {
                    len = delete(s, i, len);
                } else {
                    prev = s[i];
                    i += 1;
                }
            }
        }
    }

    len
}

// ---------------------------------------------------------------------------
// hu/HungarianLightStemmer

/// HungarianLightStemFilter: the UniNE light Hungarian stemmer (Savoy).
pub fn hungarian_light_stem(term: &mut Vec<u16>) {
    apply(term, hungarian_light);
}

fn hungarian_light(s: &mut [u16], mut len: usize) -> usize {
    for c in s[..len].iter_mut() {
        match ch(*c) {
            'á' => *c = w('a'),
            'ë' | 'é' => *c = w('e'),
            'í' => *c = w('i'),
            'ó' | 'ő' | 'õ' | 'ö' => *c = w('o'),
            'ú' | 'ű' | 'ũ' | 'û' | 'ü' => *c = w('u'),
            _ => {}
        }
    }

    len = hu_remove_case(s, len);
    len = hu_remove_possessive(s, len);
    len = hu_remove_plural(s, len);
    hu_normalize(s, len)
}

fn hu_remove_case(s: &[u16], len: usize) -> usize {
    if len > 6 && ends_with(s, len, "kent") {
        return len - 4;
    }

    if len > 5 {
        if ends_with_any(
            s,
            len,
            &[
                "nak", "nek", "val", "vel", "ert", "rol", "ban", "ben", "bol", "nal", "nel", "hoz",
                "hez", "tol",
            ],
        ) {
            return len - 3;
        }

        if ends_with_any(s, len, &["al", "el"]) && !is_vowel(s[len - 3]) && s[len - 3] == s[len - 4]
        {
            return len - 3;
        }
    }

    if len > 4 {
        if ends_with_any(
            s,
            len,
            &[
                "at", "et", "ot", "va", "ve", "ra", "re", "ba", "be", "ul", "ig",
            ],
        ) {
            return len - 2;
        }

        if ends_with_any(s, len, &["on", "en"]) && !is_vowel(s[len - 3]) {
            return len - 2;
        }

        match ch(s[len - 1]) {
            't' | 'n' => return len - 1,
            'a' | 'e' if s[len - 2] == s[len - 3] && !is_vowel(s[len - 2]) => return len - 2,
            _ => {}
        }
    }

    len
}

fn hu_remove_possessive(s: &[u16], len: usize) -> usize {
    if len > 6 {
        if !is_vowel(s[len - 5]) && ends_with_any(s, len, &["atok", "otok", "etek"]) {
            return len - 4;
        }

        if ends_with_any(s, len, &["itek", "itok"]) {
            return len - 4;
        }
    }

    if len > 5 {
        if !is_vowel(s[len - 4]) && ends_with_any(s, len, &["unk", "tok", "tek"]) {
            return len - 3;
        }

        if is_vowel(s[len - 4]) && ends_with(s, len, "juk") {
            return len - 3;
        }

        if ends_with(s, len, "ink") {
            return len - 3;
        }
    }

    if len > 4 {
        if !is_vowel(s[len - 3])
            && ends_with_any(s, len, &["am", "em", "om", "ad", "ed", "od", "uk"])
        {
            return len - 2;
        }

        if is_vowel(s[len - 3]) && ends_with_any(s, len, &["nk", "ja", "je"]) {
            return len - 2;
        }

        if ends_with_any(s, len, &["im", "id", "ik"]) {
            return len - 2;
        }
    }

    if len > 3 {
        match ch(s[len - 1]) {
            'a' | 'e' if !is_vowel(s[len - 2]) => return len - 1,
            'm' | 'd' if is_vowel(s[len - 2]) => return len - 1,
            'i' => return len - 1,
            _ => {}
        }
    }

    len
}

fn hu_remove_plural(s: &[u16], len: usize) -> usize {
    if len > 3 && s[len - 1] == w('k') {
        // -ak/-ok/-ek lose two chars when long enough, any other -k one.
        return if matches!(ch(s[len - 2]), 'a' | 'o' | 'e') && len > 4 {
            len - 2
        } else {
            len - 1
        };
    }
    len
}

fn hu_normalize(s: &[u16], len: usize) -> usize {
    if len > 3 && matches!(ch(s[len - 1]), 'a' | 'e' | 'i' | 'o') {
        return len - 1;
    }
    len
}

// ---------------------------------------------------------------------------
// ru/RussianLightStemmer

/// RussianLightStemFilter: the light Russian stemmer (Savoy).
pub fn russian_light_stem(term: &mut Vec<u16>) {
    apply(term, |s, len| {
        let len = ru_remove_case(s, len);
        ru_normalize(s, len)
    });
}

fn ru_normalize(s: &[u16], len: usize) -> usize {
    if len > 3 {
        match ch(s[len - 1]) {
            'ь' | 'и' => return len - 1,
            'н' if s[len - 2] == w('н') => return len - 1,
            _ => {}
        }
    }
    len
}

fn ru_remove_case(s: &[u16], len: usize) -> usize {
    if len > 6 && ends_with_any(s, len, &["иями", "оями"]) {
        return len - 4;
    }

    if len > 5
        && ends_with_any(
            s,
            len,
            &[
                "иям", "иях", "оях", "ями", "оям", "оьв", "ами", "его", "ему", "ери", "ими", "ого",
                "ому", "ыми", "оев",
            ],
        )
    {
        return len - 3;
    }

    if len > 4
        && ends_with_any(
            s,
            len,
            &[
                "ая", "яя", "ях", "юю", "ах", "ею", "их", "ия", "ию", "ьв", "ою", "ую", "ям", "ых",
                "ея", "ам", "ем", "ей", "ём", "ев", "ий", "им", "ое", "ой", "ом", "ов", "ые", "ый",
                "ым", "ми",
            ],
        )
    {
        return len - 2;
    }

    if len > 3
        && matches!(
            ch(s[len - 1]),
            'а' | 'е' | 'и' | 'о' | 'у' | 'й' | 'ы' | 'я' | 'ь'
        )
    {
        return len - 1;
    }

    len
}
