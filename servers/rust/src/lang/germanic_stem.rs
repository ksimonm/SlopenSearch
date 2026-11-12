//! Ports of Lucene's German, Swedish and Norwegian light/minimal stemmers, the
//! old GermanStemmer (GermanStemFilter), GermanNormalizationFilter and the
//! Scandinavian normalization/folding filters. Each `pub fn` is what the
//! corresponding TokenFilter does to one term (stem filters skip keyword
//! terms; that check is the caller's).

use super::case_width::java_to_lower;
use super::java_char_data::{is_letter, sigma_class, SigmaClass};
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
// de/GermanLightStemmer

/// GermanLightStemFilter: the UniNE light German stemmer (Savoy).
pub fn german_light_stem(term: &mut Vec<u16>) {
    apply(term, german_light);
}

fn german_light(s: &mut [u16], len: usize) -> usize {
    for c in s[..len].iter_mut() {
        match ch(*c) {
            'ä' | 'à' | 'á' | 'â' => *c = w('a'),
            'ö' | 'ò' | 'ó' | 'ô' => *c = w('o'),
            'ï' | 'ì' | 'í' | 'î' => *c = w('i'),
            'ü' | 'ù' | 'ú' | 'û' => *c = w('u'),
            _ => {}
        }
    }

    let len = german_light_step1(s, len);
    german_light_step2(s, len)
}

fn st_ending(c: u16) -> bool {
    matches!(
        ch(c),
        'b' | 'd' | 'f' | 'g' | 'h' | 'k' | 'l' | 'm' | 'n' | 't'
    )
}

fn german_light_step1(s: &[u16], len: usize) -> usize {
    if len > 5 && s[len - 3] == w('e') && s[len - 2] == w('r') && s[len - 1] == w('n') {
        return len - 3;
    }

    if len > 4 && s[len - 2] == w('e') && matches!(ch(s[len - 1]), 'm' | 'n' | 'r' | 's') {
        return len - 2;
    }

    if len > 3 && s[len - 1] == w('e') {
        return len - 1;
    }

    if len > 3 && s[len - 1] == w('s') && st_ending(s[len - 2]) {
        return len - 1;
    }

    len
}

fn german_light_step2(s: &[u16], len: usize) -> usize {
    if len > 5 && s[len - 3] == w('e') && s[len - 2] == w('s') && s[len - 1] == w('t') {
        return len - 3;
    }

    if len > 4 && s[len - 2] == w('e') && (s[len - 1] == w('r') || s[len - 1] == w('n')) {
        return len - 2;
    }

    if len > 4 && s[len - 2] == w('s') && s[len - 1] == w('t') && st_ending(s[len - 3]) {
        return len - 2;
    }

    len
}

// ---------------------------------------------------------------------------
// de/GermanMinimalStemmer

/// GermanMinimalStemFilter: the minimal German (plural) stemmer (Savoy).
pub fn german_minimal_stem(term: &mut Vec<u16>) {
    apply(term, german_minimal);
}

fn german_minimal(s: &mut [u16], len: usize) -> usize {
    if len < 5 {
        return len;
    }

    for c in s[..len].iter_mut() {
        match ch(*c) {
            'ä' => *c = w('a'),
            'ö' => *c = w('o'),
            'ü' => *c = w('u'),
            _ => {}
        }
    }

    if len > 6 && s[len - 3] == w('n') && s[len - 2] == w('e') && s[len - 1] == w('n') {
        return len - 3;
    }

    if len > 5 {
        let prev = s[len - 2];
        match ch(s[len - 1]) {
            'n' | 's' | 'r' if prev == w('e') => return len - 2,
            'e' if prev == w('s') => return len - 2,
            _ => {}
        }
    }

    match ch(s[len - 1]) {
        'n' | 'e' | 's' | 'r' => len - 1,
        _ => len,
    }
}

// ---------------------------------------------------------------------------
// de/GermanStemmer (+ GermanStemFilter)

/// GermanStemFilter: the old Caumanns German stemmer. Lowercases the term
/// (Java `String.toLowerCase`), and stems it only if every UTF-16 unit is a
/// letter (`Character.isLetter`); otherwise the lowercased term is the result.
pub fn german_stem(term: &mut Vec<u16>) {
    // Use lowercase for medium stemming.
    let mut sb = java_string_to_lower_case(term);
    if sb.iter().all(|&c| is_letter(c)) {
        // Stemming starts here...
        let subst_count = german_substitute(&mut sb);
        german_strip(&mut sb, subst_count);
        german_optimize(&mut sb, subst_count);
        german_resubstitute(&mut sb);
        german_remove_particle_denotion(&mut sb);
    }
    *term = sb;
}

/// Suffix stripping of the seven "base" suffixes "e", "s", "n", "t", "em",
/// "er" and "nd".
fn german_strip(buffer: &mut Vec<u16>, subst_count: usize) {
    loop {
        let len = buffer.len();
        if len <= 3 {
            return;
        }
        let tail = &buffer[len - 2..];
        if len + subst_count > 5 && tail == [w('n'), w('d')] {
            buffer.truncate(len - 2);
        } else if len + subst_count > 4 && (tail == [w('e'), w('m')] || tail == [w('e'), w('r')]) {
            buffer.truncate(len - 2);
        } else if matches!(ch(buffer[len - 1]), 'e' | 's' | 'n' | 't') {
            // "t" occurs only as suffix of verbs.
            buffer.truncate(len - 1);
        } else {
            return;
        }
    }
}

/// Contextual optimizations: female plurals of professions and inhabitants,
/// irregular plurals like "Matrizen -> Matrix".
fn german_optimize(buffer: &mut Vec<u16>, subst_count: usize) {
    let len = buffer.len();
    if len > 5 && buffer[len - 5..] == u("erin*")[..] {
        buffer.truncate(len - 1);
        german_strip(buffer, subst_count);
    }
    if let Some(last) = buffer.last_mut() {
        if *last == w('z') {
            *last = w('x');
        }
    }
}

/// Removes a particle denotion ("ge") from a term.
fn german_remove_particle_denotion(buffer: &mut Vec<u16>) {
    let gege = u("gege");
    if buffer.len() > 4 {
        if let Some(c) = buffer.windows(4).position(|x| x == &gege[..]) {
            buffer.drain(c..c + 2);
        }
    }
}

/// Umlauts -> vowels, "ß" -> "ss", second of a pair of equal chars -> '*',
/// sch/ch/ei/ie/ig/st -> $/§/%/&/#/!. Returns the number of removed chars
/// (substCount).
fn german_substitute(buffer: &mut Vec<u16>) -> usize {
    let mut subst_count = 0;
    let mut c = 0;
    while c < buffer.len() {
        // Replace the second char of a pair of the equal characters with an asterisk
        if c > 0 && buffer[c] == buffer[c - 1] {
            buffer[c] = w('*');
        }
        // Substitute Umlauts.
        else if buffer[c] == w('ä') {
            buffer[c] = w('a');
        } else if buffer[c] == w('ö') {
            buffer[c] = w('o');
        } else if buffer[c] == w('ü') {
            buffer[c] = w('u');
        }
        // Fix bug so that 'ß' at the end of a word is replaced.
        else if buffer[c] == w('ß') {
            buffer[c] = w('s');
            buffer.insert(c + 1, w('s'));
            subst_count += 1;
        }
        // Take care that at least one character is left left side from the current one
        if c + 1 < buffer.len() {
            let (x, y) = (buffer[c], buffer[c + 1]);
            // Masking several common character combinations with an token
            if c + 2 < buffer.len() && x == w('s') && y == w('c') && buffer[c + 2] == w('h') {
                buffer[c] = w('$');
                buffer.drain(c + 1..c + 3);
                subst_count += 2;
            } else {
                let token = match (ch(x), ch(y)) {
                    ('c', 'h') => Some('§'),
                    ('e', 'i') => Some('%'),
                    ('i', 'e') => Some('&'),
                    ('i', 'g') => Some('#'),
                    ('s', 't') => Some('!'),
                    _ => None,
                };
                if let Some(t) = token {
                    buffer[c] = w(t);
                    buffer.remove(c + 1);
                    subst_count += 1;
                }
            }
        }
        c += 1;
    }
    subst_count
}

/// Undoes substitute(): character pairs and character combinations (umlauts
/// stay plain vowels, "ß" stays "ss").
fn german_resubstitute(buffer: &mut Vec<u16>) {
    let mut c = 0;
    while c < buffer.len() {
        match ch(buffer[c]) {
            // '*' is never first: substitute() only writes it at c > 0.
            '*' => buffer[c] = buffer[c - 1],
            '$' => {
                buffer[c] = w('s');
                buffer.splice(c + 1..c + 1, [w('c'), w('h')]);
            }
            '§' => {
                buffer[c] = w('c');
                buffer.insert(c + 1, w('h'));
            }
            '%' => {
                buffer[c] = w('e');
                buffer.insert(c + 1, w('i'));
            }
            '&' => {
                buffer[c] = w('i');
                buffer.insert(c + 1, w('e'));
            }
            '#' => {
                buffer[c] = w('i');
                buffer.insert(c + 1, w('g'));
            }
            '!' => {
                buffer[c] = w('s');
                buffer.insert(c + 1, w('t'));
            }
            _ => {}
        }
        c += 1;
    }
}

/// Java's `String.toLowerCase(Locale)` for a non-Turkic/Lithuanian locale:
/// `Character.toLowerCase` per code point, except U+0130 -> "i̇" and the
/// context-dependent final sigma (U+03A3 -> U+03C2 at the end of a word).
fn java_string_to_lower_case(s: &[u16]) -> Vec<u16> {
    let cps: Vec<u32> = char::decode_utf16(s.iter().copied())
        .map(|r| {
            r.map(|c| c as u32)
                .unwrap_or_else(|e| e.unpaired_surrogate() as u32)
        })
        .collect();
    let mut out = Vec::with_capacity(s.len() + 1);
    for (i, &cp) in cps.iter().enumerate() {
        let lower = match cp {
            0x130 => {
                out.extend_from_slice(&[w('i'), 0x307]);
                continue;
            }
            0x3A3 => {
                if is_final_cased(&cps, i) {
                    0x3C2
                } else {
                    0x3C3
                }
            }
            _ => java_to_lower(cp),
        };
        match char::from_u32(lower) {
            Some(c) => out.extend_from_slice(c.encode_utf16(&mut [0; 2])),
            None => out.push(lower as u16), // unpaired surrogate, kept as is
        }
    }
    out
}

/// ConditionalSpecialCasing.isFinalCased: a cased letter precedes the sigma
/// and none follows it, both within the same word.
fn is_final_cased(cps: &[u32], index: usize) -> bool {
    let boundary = |i: usize| is_word_boundary(cps, i);
    // U+0345 (combining ypogegrammeni) is a cased mark
    let is_cased = |i: usize| sigma_class_at(cps, i) == SigmaClass::Cased || cps[i] == 0x345;

    // Look for a preceding 'cased' letter
    let mut i = index;
    while !boundary(i) {
        if is_cased(i - 1) {
            // Check that there is no 'cased' letter after the index
            let mut j = index + 1;
            while !boundary(j) {
                if is_cased(j) {
                    return false;
                }
                j += 1;
            }
            return true;
        }
        i -= 1;
    }
    false
}

fn sigma_class_at(cps: &[u32], i: usize) -> SigmaClass {
    match (cps[i], char::from_u32(cps[i])) {
        (cp, _) if cp < 0x10000 => sigma_class(cp as u16),
        (_, Some(c)) if c.is_lowercase() || c.is_uppercase() => SigmaClass::Cased,
        (_, Some(c)) if c.is_alphabetic() => SigmaClass::Letter,
        _ => SigmaClass::Break,
    }
}

/// Java's word `BreakIterator.isBoundary(i)` as seen by isFinalCased, modelled
/// with per-char classes measured on JDK 25 (see java_char_data.rs): marks and
/// format chars stick to the char before them, word chars join, a single Mid*
/// char joins the two letters (or digits) right around it, everything else is
/// a boundary. Supplementary letters (not measured; classed by Rust's Unicode
/// data) join the word chars before them, but the ones after them only when
/// they start the term, as observed on JDK 25. This is a model: exotic mixes
/// of marks, punctuation and supplementary letters around a capital sigma can
/// still differ from Java.
fn is_word_boundary(cps: &[u32], i: usize) -> bool {
    use SigmaClass::*;
    if i == 0 || i == cps.len() {
        return true;
    }
    let class = |j: usize| sigma_class_at(cps, j);
    let word = |c: SigmaClass| matches!(c, Cased | Letter | Digit);
    let letter = |c: SigmaClass| matches!(c, Cased | Letter);
    // a single `mid` char between the chars of classes `x` and `y` joins them
    let mid_joins = |x: SigmaClass, mid: SigmaClass, y: SigmaClass| match mid {
        MidLetter => letter(x) && letter(y),
        MidLetterNum => (letter(x) && letter(y)) || (x == Digit && y == Digit),
        MidNum => x == Digit && y == Digit,
        _ => false,
    };
    let attached = |c: SigmaClass| matches!(c, Extend | Format);
    // the last non-mark, non-format char before position j
    let prev_base = |j: usize| (0..j).rev().find(|&k| !attached(class(k)));

    let Some(a_at) = prev_base(i) else {
        return true; // only marks before: they form a segment of their own
    };
    if attached(class(i)) {
        return false;
    }
    if cps[a_at] >= 0x10000 && a_at > 0 {
        return true;
    }
    let (a, b) = (class(a_at), class(i));
    // after a Mid* char only format chars are skipped (a mark breaks the join)
    let after_mid = (i + 1..cps.len()).find(|&k| class(k) != Format);
    let mid_right_before_b = (a_at + 1..i).all(|k| class(k) == Format);
    let joined = (word(a) && word(b))
        || after_mid.is_some_and(|n| mid_joins(a, b, class(n)))
        || (mid_right_before_b && prev_base(a_at).is_some_and(|p| mid_joins(class(p), a, b)));
    !joined
}

// ---------------------------------------------------------------------------
// de/GermanNormalizationFilter

/// GermanNormalizationFilter (applies to every term, keyword or not):
/// ä/ö/ü -> a/o/u, ß -> ss, and 'e' after a/o/u (umlaut spelled "ae", "oe",
/// "ue", but not after "qu") is removed.
pub fn german_normalization(term: &mut Vec<u16>) {
    // FSM with 3 states:
    const N: u8 = 0; // ordinary state
    const V: u8 = 1; // stops 'u' from entering umlaut state
    const U: u8 = 2; // umlaut state, allows e-deletion
    let mut state = N;
    let mut i = 0;
    while i < term.len() {
        match ch(term[i]) {
            'a' | 'o' => state = U,
            'u' => state = if state == N { U } else { V },
            'e' => {
                let deleted = state == U;
                state = V;
                if deleted {
                    term.remove(i);
                    continue;
                }
            }
            'i' | 'q' | 'y' => state = V,
            'ä' => {
                term[i] = w('a');
                state = V;
            }
            'ö' => {
                term[i] = w('o');
                state = V;
            }
            'ü' => {
                term[i] = w('u');
                state = V;
            }
            'ß' => {
                term[i] = w('s');
                i += 1;
                term.insert(i, w('s'));
                state = N;
            }
            _ => state = N,
        }
        i += 1;
    }
}

// ---------------------------------------------------------------------------
// sv/SwedishLightStemmer

/// SwedishLightStemFilter: the light Swedish stemmer (Savoy).
pub fn swedish_light_stem(term: &mut Vec<u16>) {
    apply(term, swedish_light);
}

fn swedish_light(s: &mut [u16], mut len: usize) -> usize {
    if len > 4 && s[len - 1] == w('s') {
        len -= 1;
    }

    let ew = |suffixes: &[&str], len: usize| suffixes.iter().any(|x| ends_with(s, len, x));

    if len > 7 && ew(&["elser", "heten"], len) {
        return len - 5;
    }

    if len > 6
        && ew(
            &["arne", "erna", "ande", "else", "aste", "orna", "aren"],
            len,
        )
    {
        return len - 4;
    }

    if len > 5 && ew(&["are", "ast", "het"], len) {
        return len - 3;
    }

    if len > 4 && ew(&["ar", "er", "or", "en", "at", "te", "et"], len) {
        return len - 2;
    }

    if len > 3 && matches!(ch(s[len - 1]), 't' | 'a' | 'e' | 'n') {
        return len - 1;
    }

    len
}

// ---------------------------------------------------------------------------
// sv/SwedishMinimalStemmer

/// SwedishMinimalStemFilter: the minimal Swedish (plural) stemmer.
pub fn swedish_minimal_stem(term: &mut Vec<u16>) {
    apply(term, swedish_minimal);
}

fn swedish_minimal(s: &mut [u16], mut len: usize) -> usize {
    if len > 4 && s[len - 1] == w('s') {
        len -= 1;
    }

    let ew = |suffixes: &[&str], len: usize| suffixes.iter().any(|x| ends_with(s, len, x));

    if len > 6 && ew(&["arne", "erna", "arna", "orna", "aren"], len) {
        return len - 4;
    }

    if len > 5 && ew(&["are"], len) {
        return len - 3;
    }

    if len > 4 && ew(&["ar", "at", "er", "et", "or", "en"], len) {
        return len - 2;
    }

    if len > 3 && matches!(ch(s[len - 1]), 'a' | 'e' | 'n') {
        return len - 1;
    }

    len
}

// ---------------------------------------------------------------------------
// no/NorwegianLightStemmer, no/NorwegianMinimalStemmer

/// Flag: remove Bokmål-specific endings.
pub const BOKMAAL: u32 = 1;
/// Flag: remove Nynorsk-specific endings.
pub const NYNORSK: u32 = 2;

/// NorwegianLightStemFilter(input, flags): the light Norwegian stemmer.
/// `flags` is BOKMAAL, NYNORSK or both (Lucene rejects anything else).
pub fn norwegian_light_stem(term: &mut Vec<u16>, flags: u32) {
    let bokmaal = flags & BOKMAAL != 0;
    let nynorsk = flags & NYNORSK != 0;
    apply(term, |s, len| norwegian_light(s, len, bokmaal, nynorsk));
}

fn norwegian_light(s: &[u16], mut len: usize, use_bokmaal: bool, use_nynorsk: bool) -> usize {
    // Remove posessive -s (bilens -> bilen) and continue checking
    if len > 4 && s[len - 1] == w('s') {
        len -= 1;
    }

    let ew = |suffix: &str| ends_with(s, len, suffix);

    // Remove common endings, single-pass
    if len > 7
        && ((ew("heter") && use_bokmaal) // general ending (hemmelig-heter -> hemmelig)
            || (ew("heten") && use_bokmaal) // general ending (hemmelig-heten -> hemmelig)
            || (ew("heita") && use_nynorsk))
    // general ending (hemmeleg-heita -> hemmeleg)
    {
        return len - 5;
    }

    // Remove Nynorsk common endings, single-pass
    if len > 8
        && use_nynorsk
        && (ew("heiter") // general ending (hemmeleg-heiter -> hemmeleg)
            || ew("leiken") // general ending (trygg-leiken -> trygg)
            || ew("leikar"))
    // general ending (trygg-leikar -> trygg)
    {
        return len - 6;
    }

    if len > 5
        && (ew("dom") // general ending (kristen-dom -> kristen)
            || (ew("het") && use_bokmaal))
    // general ending (hemmelig-het -> hemmelig)
    {
        return len - 3;
    }

    if len > 6
        && use_nynorsk
        && (ew("heit") // general ending (hemmeleg-heit -> hemmeleg)
            || ew("semd") // general ending (verk-semd -> verk)
            || ew("leik"))
    // general ending (trygg-leik -> trygg)
    {
        return len - 4;
    }

    if len > 7
        && (ew("elser") // general ending (føl-elser -> føl)
            || ew("elsen"))
    // general ending (føl-elsen -> føl)
    {
        return len - 5;
    }

    if len > 6
        && ((ew("ende") && use_bokmaal) // (sov-ende -> sov)
            || (ew("ande") && use_nynorsk) // (sov-ande -> sov)
            || ew("else") // general ending (føl-else -> føl)
            || (ew("este") && use_bokmaal) // adj (fin-este -> fin)
            || (ew("aste") && use_nynorsk) // adj (fin-aste -> fin)
            || (ew("eren") && use_bokmaal) // masc
            || (ew("aren") && use_nynorsk))
    // masc
    {
        return len - 4;
    }

    if len > 5
        && ((ew("ere") && use_bokmaal) // adj (fin-ere -> fin)
            || (ew("are") && use_nynorsk) // adj (fin-are -> fin)
            || (ew("est") && use_bokmaal) // adj (fin-est -> fin)
            || (ew("ast") && use_nynorsk) // adj (fin-ast -> fin)
            || ew("ene") // masc/fem/neutr pl definite (hus-ene)
            || (ew("ane") && use_nynorsk))
    // masc pl definite (gut-ane)
    {
        return len - 3;
    }

    if len > 4
        && (ew("er") // masc/fem indefinite
            || ew("en") // masc/fem definite
            || ew("et") // neutr definite
            || (ew("ar") && use_nynorsk) // masc pl indefinite
            || (ew("st") && use_bokmaal) // adj (billig-st -> billig)
            || ew("te"))
    {
        return len - 2;
    }

    // 'a': fem definite; 'e': to get correct stem for nouns ending in -e
    // (kake -> kak, kaker -> kak)
    if len > 3 && matches!(ch(s[len - 1]), 'a' | 'e' | 'n') {
        return len - 1;
    }

    len
}

/// NorwegianMinimalStemFilter(input, flags): the minimal Norwegian stemmer.
/// `flags` is BOKMAAL, NYNORSK or both (Lucene rejects anything else).
pub fn norwegian_minimal_stem(term: &mut Vec<u16>, flags: u32) {
    let nynorsk = flags & NYNORSK != 0;
    apply(term, |s, len| norwegian_minimal(s, len, nynorsk));
}

fn norwegian_minimal(s: &[u16], mut len: usize, use_nynorsk: bool) -> usize {
    // Remove genitiv s
    if len > 4 && s[len - 1] == w('s') {
        len -= 1;
    }

    let ew = |suffix: &str| ends_with(s, len, suffix);

    if len > 5
        && (ew("ene") // masc/fem/neutr pl definite (hus-ene)
            || (ew("ane") && use_nynorsk))
    // masc pl definite (gut-ane)
    {
        return len - 3;
    }

    if len > 4
        && (ew("er") // masc/fem indefinite
            || ew("en") // masc/fem definite
            || ew("et") // neutr definite
            || (ew("ar") && use_nynorsk))
    // masc pl indefinite
    {
        return len - 2;
    }

    // 'a': fem definite; 'e': nouns ending in -e (kake -> kak, kaker -> kak)
    if len > 3 && matches!(ch(s[len - 1]), 'a' | 'e') {
        return len - 1;
    }

    len
}

// ---------------------------------------------------------------------------
// miscellaneous/ScandinavianNormalizationFilter (ScandinavianNormalizer with
// ALL_FOLDINGS)

/// ScandinavianNormalizationFilter (every term): ä/Ä/ö/Ö -> æ/Æ/ø/Ø and
/// aa/ao -> å, ae -> æ, oe/oo -> ø (case-insensitive second letter, case of the
/// first letter kept).
pub fn scandinavian_normalization(term: &mut Vec<u16>) {
    const AA: u16 = 0xC5; // Å
    const AA_LOWER: u16 = 0xE5; // å
    const AE: u16 = 0xC6; // Æ
    const AE_LOWER: u16 = 0xE6; // æ
    const AE_SE: u16 = 0xC4; // Ä
    const AE_SE_LOWER: u16 = 0xE4; // ä
    const OE: u16 = 0xD8; // Ø
    const OE_LOWER: u16 = 0xF8; // ø
    const OE_SE: u16 = 0xD6; // Ö
    const OE_SE_LOWER: u16 = 0xF6; // ö

    let mut i = 0;
    while i < term.len() {
        let c = term[i];
        if c == AE_SE_LOWER {
            term[i] = AE_LOWER;
        } else if c == AE_SE {
            term[i] = AE;
        } else if c == OE_SE_LOWER {
            term[i] = OE_LOWER;
        } else if c == OE_SE {
            term[i] = OE;
        } else if term.len() - 1 > i {
            let next = ch(term[i + 1]);
            let folded = match (ch(c), next) {
                ('a', 'a' | 'A' | 'o' | 'O') => Some(AA_LOWER),
                ('A', 'a' | 'A' | 'o' | 'O') => Some(AA),
                ('a', 'e' | 'E') => Some(AE_LOWER),
                ('A', 'e' | 'E') => Some(AE),
                ('o', 'e' | 'E' | 'o' | 'O') => Some(OE_LOWER),
                ('O', 'e' | 'E' | 'o' | 'O') => Some(OE),
                _ => None,
            };
            if let Some(f) = folded {
                term.remove(i + 1);
                term[i] = f;
            }
        }
        i += 1;
    }
}

// ---------------------------------------------------------------------------
// miscellaneous/ScandinavianFoldingFilter

/// ScandinavianFoldingFilter (every term): å/ä/æ -> a, ö/ø -> o (same for
/// uppercase), and aa/ae/ao -> a, oe/oo -> o (case-insensitive, first letter
/// kept).
pub fn scandinavian_folding(term: &mut Vec<u16>) {
    let mut i = 0;
    while i < term.len() {
        let c = ch(term[i]);
        match c {
            'å' | 'ä' | 'æ' => term[i] = w('a'),
            'Å' | 'Ä' | 'Æ' => term[i] = w('A'),
            'ø' | 'ö' => term[i] = w('o'),
            'Ø' | 'Ö' => term[i] = w('O'),
            _ if term.len() - 1 > i => {
                let next = ch(term[i + 1]);
                let delete_next = match c {
                    'a' | 'A' => matches!(next, 'a' | 'A' | 'e' | 'E' | 'o' | 'O'),
                    'o' | 'O' => matches!(next, 'e' | 'E' | 'o' | 'O'),
                    _ => false,
                };
                if delete_next {
                    term.remove(i + 1);
                }
            }
            _ => {}
        }
        i += 1;
    }
}
