//! Ports of Lucene's light/minimal stemmers for French, Spanish, Italian and
//! Portuguese (org.apache.lucene.analysis.{fr,es,it,pt}). Each `pub fn` is what
//! the corresponding `*StemFilter` does to one non-keyword term.

use super::java_char_data::is_letter;
use super::rslp;
use super::rslp_data::PORTUGUESE_PLURAL;
use super::stem_util::*;

/// A UTF-16 unit as a `char` for matching against literals (surrogates, which
/// never match a literal, become U+FFFD).
fn ch(c: u16) -> char {
    char::from_u32(c as u32).unwrap_or('\u{FFFD}')
}

const fn w(c: char) -> u16 {
    c as u16
}

// ---------------------------------------------------------------------------
// fr/FrenchLightStemmer

/// FrenchLightStemFilter: the UniNE light French stemmer (Savoy).
pub fn french_light_stem(term: &mut Vec<u16>) {
    apply(term, french_light);
}

fn french_light(s: &mut [u16], mut len: usize) -> usize {
    if len > 5 && s[len - 1] == w('x') {
        if s[len - 3] == w('a') && s[len - 2] == w('u') && s[len - 4] != w('e') {
            s[len - 2] = w('l');
        }
        len -= 1;
    }

    if len > 3 && s[len - 1] == w('x') {
        len -= 1;
    }

    if len > 3 && s[len - 1] == w('s') {
        len -= 1;
    }

    if len > 9 && ends_with(s, len, "issement") {
        len -= 6;
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 8 && ends_with(s, len, "issant") {
        len -= 4;
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 6 && ends_with(s, len, "ement") {
        len -= 4;
        if len > 3 && ends_with(s, len, "ive") {
            len -= 1;
            s[len - 1] = w('f');
        }
        return french_norm(s, len);
    }

    if len > 11 && ends_with(s, len, "ficatrice") {
        len -= 5;
        s[len - 2] = w('e');
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 10 && ends_with(s, len, "ficateur") {
        len -= 4;
        s[len - 2] = w('e');
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 9 && ends_with(s, len, "catrice") {
        len -= 3;
        s[len - 4] = w('q');
        s[len - 3] = w('u');
        s[len - 2] = w('e');
        // s[len-1] = 'r' <-- unnecessary, already 'r'.
        return french_norm(s, len);
    }

    if len > 8 && ends_with(s, len, "cateur") {
        len -= 2;
        s[len - 4] = w('q');
        s[len - 3] = w('u');
        s[len - 2] = w('e');
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 8 && ends_with(s, len, "atrice") {
        len -= 4;
        s[len - 2] = w('e');
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 7 && ends_with(s, len, "ateur") {
        len -= 3;
        s[len - 2] = w('e');
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 6 && ends_with(s, len, "trice") {
        len -= 1;
        s[len - 3] = w('e');
        s[len - 2] = w('u');
        s[len - 1] = w('r');
    }

    if len > 5 && ends_with(s, len, "ième") {
        return french_norm(s, len - 4);
    }

    if len > 7 && ends_with(s, len, "teuse") {
        len -= 2;
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 6 && ends_with(s, len, "teur") {
        len -= 1;
        s[len - 1] = w('r');
        return french_norm(s, len);
    }

    if len > 5 && ends_with(s, len, "euse") {
        return french_norm(s, len - 2);
    }

    if len > 8 && ends_with(s, len, "ère") {
        len -= 1;
        s[len - 2] = w('e');
        return french_norm(s, len);
    }

    if len > 7 && ends_with(s, len, "ive") {
        len -= 1;
        s[len - 1] = w('f');
        return french_norm(s, len);
    }

    if len > 4 && (ends_with(s, len, "folle") || ends_with(s, len, "molle")) {
        len -= 2;
        s[len - 1] = w('u');
        return french_norm(s, len);
    }

    if len > 9 && ends_with(s, len, "nnelle") {
        return french_norm(s, len - 5);
    }

    if len > 9 && ends_with(s, len, "nnel") {
        return french_norm(s, len - 3);
    }

    if len > 4 && ends_with(s, len, "ète") {
        len -= 1;
        s[len - 2] = w('e');
    }

    if len > 8 && ends_with(s, len, "ique") {
        len -= 4;
    }

    if len > 8 && ends_with(s, len, "esse") {
        return french_norm(s, len - 3);
    }

    if len > 7 && ends_with(s, len, "inage") {
        return french_norm(s, len - 3);
    }

    if len > 9 && ends_with(s, len, "isation") {
        len -= 7;
        if len > 5 && ends_with(s, len, "ual") {
            s[len - 2] = w('e');
        }
        return french_norm(s, len);
    }

    if len > 9 && ends_with(s, len, "isateur") {
        return french_norm(s, len - 7);
    }

    if len > 8 && ends_with(s, len, "ation") {
        return french_norm(s, len - 5);
    }

    if len > 8 && ends_with(s, len, "ition") {
        return french_norm(s, len - 5);
    }

    french_norm(s, len)
}

/// FrenchLightStemmer.norm: accent removal, squeezing of doubled letters and
/// final -ie / -r / -e / doubled-letter removal.
fn french_norm(s: &mut [u16], mut len: usize) -> usize {
    if len > 4 {
        for c in s[..len].iter_mut() {
            match ch(*c) {
                'à' | 'á' | 'â' => *c = w('a'),
                'ô' => *c = w('o'),
                'è' | 'é' | 'ê' => *c = w('e'),
                'ù' | 'û' => *c = w('u'),
                'î' => *c = w('i'),
                'ç' => *c = w('c'),
                _ => {}
            }
        }

        // Collapse runs of the same letter (Character.isLetter on the UTF-16 unit).
        let mut prev = s[0];
        let mut i = 1;
        while i < len {
            if s[i] == prev && is_letter(prev) {
                len = delete(s, i, len);
            } else {
                prev = s[i];
                i += 1;
            }
        }
    }

    if len > 4 && ends_with(s, len, "ie") {
        len -= 2;
    }

    if len > 4 {
        if s[len - 1] == w('r') {
            len -= 1;
        }
        if s[len - 1] == w('e') {
            len -= 1;
        }
        if s[len - 1] == w('e') {
            len -= 1;
        }
        if s[len - 1] == s[len - 2] && is_letter(s[len - 1]) {
            len -= 1;
        }
    }
    len
}

// ---------------------------------------------------------------------------
// fr/FrenchMinimalStemmer

/// FrenchMinimalStemFilter: Savoy's minimal French (plural) stemmer.
pub fn french_minimal_stem(term: &mut Vec<u16>) {
    apply(term, french_minimal);
}

fn french_minimal(s: &mut [u16], mut len: usize) -> usize {
    if len < 6 {
        return len;
    }

    if s[len - 1] == w('x') {
        if s[len - 3] == w('a') && s[len - 2] == w('u') {
            s[len - 2] = w('l');
        }
        return len - 1;
    }

    if s[len - 1] == w('s') {
        len -= 1;
    }
    if s[len - 1] == w('r') {
        len -= 1;
    }
    if s[len - 1] == w('e') {
        len -= 1;
    }
    if s[len - 1] == w('é') {
        len -= 1;
    }
    if s[len - 1] == s[len - 2] && is_letter(s[len - 1]) {
        len -= 1;
    }
    len
}

// ---------------------------------------------------------------------------
// es/SpanishLightStemmer

/// SpanishLightStemFilter: Savoy's light Spanish stemmer.
pub fn spanish_light_stem(term: &mut Vec<u16>) {
    apply(term, spanish_light);
}

fn spanish_light(s: &mut [u16], len: usize) -> usize {
    if len < 5 {
        return len;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'à' | 'á' | 'â' | 'ä' => *c = w('a'),
            'ò' | 'ó' | 'ô' | 'ö' => *c = w('o'),
            'è' | 'é' | 'ê' | 'ë' => *c = w('e'),
            'ù' | 'ú' | 'û' | 'ü' => *c = w('u'),
            'ì' | 'í' | 'î' | 'ï' => *c = w('i'),
            _ => {}
        }
    }

    match ch(s[len - 1]) {
        'o' | 'a' | 'e' => return len - 1,
        's' => {
            if s[len - 2] == w('e') && s[len - 3] == w('s') && s[len - 4] == w('e') {
                return len - 2;
            }
            if s[len - 2] == w('e') && s[len - 3] == w('c') {
                s[len - 3] = w('z');
                return len - 2;
            }
            if s[len - 2] == w('o') || s[len - 2] == w('a') || s[len - 2] == w('e') {
                return len - 2;
            }
        }
        _ => {}
    }

    len
}

// ---------------------------------------------------------------------------
// es/SpanishMinimalStemmer

/// SpanishMinimalStemFilter (deprecated in Lucene): minimal Spanish plural stemmer.
pub fn spanish_minimal_stem(term: &mut Vec<u16>) {
    apply(term, spanish_minimal);
}

fn spanish_minimal(s: &mut [u16], len: usize) -> usize {
    if len < 4 || s[len - 1] != w('s') {
        return len;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'à' | 'á' | 'â' | 'ä' => *c = w('a'),
            'ò' | 'ó' | 'ô' | 'ö' => *c = w('o'),
            'è' | 'é' | 'ê' | 'ë' => *c = w('e'),
            'ù' | 'ú' | 'û' | 'ü' => *c = w('u'),
            'ì' | 'í' | 'î' | 'ï' => *c = w('i'),
            'ñ' => *c = w('n'),
            _ => {}
        }
    }

    // s[len - 1] is 's' here.
    if s[len - 2] == w('a') || s[len - 2] == w('o') {
        return len - 1;
    }
    if s[len - 2] == w('e') {
        if s[len - 3] == w('s') && s[len - 4] == w('e') {
            return len - 2;
        }
        if s[len - 3] == w('c') {
            s[len - 3] = w('z');
        }
        len - 2
    } else {
        len - 1
    }
}

// ---------------------------------------------------------------------------
// it/ItalianLightStemmer

/// ItalianLightStemFilter: Savoy's light Italian stemmer.
pub fn italian_light_stem(term: &mut Vec<u16>) {
    apply(term, italian_light);
}

fn italian_light(s: &mut [u16], len: usize) -> usize {
    if len < 6 {
        return len;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'à' | 'á' | 'â' | 'ä' => *c = w('a'),
            'ò' | 'ó' | 'ô' | 'ö' => *c = w('o'),
            'è' | 'é' | 'ê' | 'ë' => *c = w('e'),
            'ù' | 'ú' | 'û' | 'ü' => *c = w('u'),
            'ì' | 'í' | 'î' | 'ï' => *c = w('i'),
            _ => {}
        }
    }

    let prev = s[len - 2];
    match ch(s[len - 1]) {
        'e' => {
            if prev == w('i') || prev == w('h') {
                len - 2
            } else {
                len - 1
            }
        }
        'i' => {
            if prev == w('h') || prev == w('i') {
                len - 2
            } else {
                len - 1
            }
        }
        'a' | 'o' => {
            if prev == w('i') {
                len - 2
            } else {
                len - 1
            }
        }
        _ => len,
    }
}

// ---------------------------------------------------------------------------
// pt/PortugueseLightStemmer

/// PortugueseLightStemFilter: the UniNE light Portuguese stemmer (Savoy).
pub fn portuguese_light_stem(term: &mut Vec<u16>) {
    apply(term, portuguese_light);
}

fn portuguese_light(s: &mut [u16], mut len: usize) -> usize {
    if len < 4 {
        return len;
    }

    len = pt_remove_suffix(s, len);

    if len > 3 && s[len - 1] == w('a') {
        len = pt_norm_feminine(s, len);
    }

    if len > 4 && matches!(ch(s[len - 1]), 'e' | 'a' | 'o') {
        len -= 1;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'à' | 'á' | 'â' | 'ä' | 'ã' => *c = w('a'),
            'ò' | 'ó' | 'ô' | 'ö' | 'õ' => *c = w('o'),
            'è' | 'é' | 'ê' | 'ë' => *c = w('e'),
            'ù' | 'ú' | 'û' | 'ü' => *c = w('u'),
            'ì' | 'í' | 'î' | 'ï' => *c = w('i'),
            'ç' => *c = w('c'),
            _ => {}
        }
    }

    len
}

fn pt_remove_suffix(s: &mut [u16], mut len: usize) -> usize {
    if len > 4 && ends_with(s, len, "es") && matches!(ch(s[len - 3]), 'r' | 's' | 'l' | 'z') {
        return len - 2;
    }

    if len > 3 && ends_with(s, len, "ns") {
        s[len - 2] = w('m');
        return len - 1;
    }

    if len > 4 && (ends_with(s, len, "eis") || ends_with(s, len, "éis")) {
        s[len - 3] = w('e');
        s[len - 2] = w('l');
        return len - 1;
    }

    if len > 4 && ends_with(s, len, "ais") {
        s[len - 2] = w('l');
        return len - 1;
    }

    if len > 4 && ends_with(s, len, "óis") {
        s[len - 3] = w('o');
        s[len - 2] = w('l');
        return len - 1;
    }

    if len > 4 && ends_with(s, len, "is") {
        s[len - 1] = w('l');
        return len;
    }

    if len > 3 && (ends_with(s, len, "ões") || ends_with(s, len, "ães")) {
        len -= 1;
        s[len - 2] = w('ã');
        s[len - 1] = w('o');
        return len;
    }

    if len > 6 && ends_with(s, len, "mente") {
        return len - 5;
    }

    if len > 3 && s[len - 1] == w('s') {
        return len - 1;
    }
    len
}

fn pt_norm_feminine(s: &mut [u16], len: usize) -> usize {
    if len > 7
        && (ends_with(s, len, "inha") || ends_with(s, len, "iaca") || ends_with(s, len, "eira"))
    {
        s[len - 1] = w('o');
        return len;
    }

    if len > 6 {
        if ends_with(s, len, "osa")
            || ends_with(s, len, "ica")
            || ends_with(s, len, "ida")
            || ends_with(s, len, "ada")
            || ends_with(s, len, "iva")
            || ends_with(s, len, "ama")
        {
            s[len - 1] = w('o');
            return len;
        }

        if ends_with(s, len, "ona") {
            s[len - 3] = w('ã');
            s[len - 2] = w('o');
            return len - 1;
        }

        if ends_with(s, len, "ora") {
            return len - 1;
        }

        if ends_with(s, len, "esa") {
            s[len - 3] = w('ê');
            return len - 1;
        }

        if ends_with(s, len, "na") {
            s[len - 1] = w('o');
            return len;
        }
    }
    len
}

// ---------------------------------------------------------------------------
// pt/PortugueseMinimalStemmer

/// PortugueseMinimalStemFilter: RSLP-S, i.e. only the "Plural" step of
/// portuguese.rslp.
pub fn portuguese_minimal_stem(term: &mut Vec<u16>) {
    rslp::run(term, |s, len| PORTUGUESE_PLURAL.apply(s, len));
}
