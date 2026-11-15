//! Ports of Lucene's case and width folding filters, on UTF-16 terms:
//! LowerCaseFilter / UpperCaseFilter (core, via CharacterUtils),
//! ga/IrishLowerCaseFilter, tr/TurkishLowerCaseFilter, cjk/CJKWidthFilter and
//! core/DecimalDigitFilter. The java.lang.Character facts they need (simple case
//! mappings, digit values, non-spacing marks) come from `char_data`.

use super::char_data::{DECIMAL_DIGITS, LOWER_EXCEPTIONS, NON_SPACING_MARKS, UPPER_EXCEPTIONS};

/// The single char a full Rust case mapping yields, else `c` unchanged.
fn simple(c: char, mut it: impl Iterator<Item = char>) -> u32 {
    match (it.next(), it.next()) {
        (Some(x), None) => x as u32,
        _ => c as u32,
    }
}

/// Java `Character.toLowerCase(int)` (simple mapping; surrogates unchanged).
pub fn java_to_lower(cp: u32) -> u32 {
    if cp < 0x80 {
        return (cp as u8).to_ascii_lowercase() as u32;
    }
    if let Ok(i) = LOWER_EXCEPTIONS.binary_search_by_key(&cp, |e| e.0) {
        return LOWER_EXCEPTIONS[i].1;
    }
    match char::from_u32(cp) {
        Some(c) => simple(c, c.to_lowercase()),
        None => cp,
    }
}

/// Java `Character.toUpperCase(int)` (simple mapping; surrogates unchanged).
pub fn java_to_upper(cp: u32) -> u32 {
    if cp < 0x80 {
        return (cp as u8).to_ascii_uppercase() as u32;
    }
    if let Ok(i) = UPPER_EXCEPTIONS.binary_search_by_key(&cp, |e| e.0) {
        return UPPER_EXCEPTIONS[i].1;
    }
    match char::from_u32(cp) {
        Some(c) => simple(c, c.to_uppercase()),
        None => cp,
    }
}

/// Java `Character.getType(cp) == NON_SPACING_MARK`.
pub fn is_non_spacing_mark(cp: u32) -> bool {
    in_ranges(NON_SPACING_MARKS, cp)
}

fn in_ranges(ranges: &[(u32, u32)], cp: u32) -> bool {
    // index of the first range whose end is >= cp
    let i = ranges.partition_point(|r| r.1 < cp);
    i < ranges.len() && ranges[i].0 <= cp
}

/// `Character.getNumericValue(cp)` when `Character.isDigit(cp)` (DECIMAL_DIGIT_NUMBER).
pub fn decimal_digit_value(cp: u32) -> Option<u32> {
    let i = DECIMAL_DIGITS.partition_point(|r| r.1 < cp);
    match DECIMAL_DIGITS.get(i) {
        Some(&(first, _, v)) if first <= cp => Some(v + (cp - first)),
        _ => None,
    }
}

/// Java `Character.codePointAt(char[] a, int index, int limit)`: (code point, unit count).
fn code_point_at(s: &[u16], i: usize, limit: usize) -> (u32, usize) {
    let c = s[i] as u32;
    if (0xD800..0xDC00).contains(&c) && i + 1 < limit {
        let d = s[i + 1] as u32;
        if (0xDC00..0xE000).contains(&d) {
            return (0x10000 + ((c - 0xD800) << 10) + (d - 0xDC00), 2);
        }
    }
    (c, 1)
}

/// Java `Character.toChars(int cp, char[] dst, int index)`: returns the unit count.
fn to_chars(cp: u32, s: &mut [u16], i: usize) -> usize {
    if cp < 0x10000 {
        s[i] = cp as u16;
        1
    } else {
        let v = cp - 0x10000;
        s[i] = 0xD800 + (v >> 10) as u16;
        s[i + 1] = 0xDC00 + (v & 0x3FF) as u16;
        2
    }
}

/// CharacterUtils.toLowerCase/toUpperCase: maps code point by code point in place.
/// (Java's simple mappings never change a code point's UTF-16 length.)
fn map_code_points(s: &mut [u16], f: fn(u32) -> u32) {
    let limit = s.len();
    let mut i = 0;
    while i < limit {
        let (cp, _) = code_point_at(s, i, limit);
        i += to_chars(f(cp), s, i);
    }
}

/// LowerCaseFilter (`lowercase`): Java `Character.toLowerCase` per code point.
pub fn lowercase(term: &mut Vec<u16>) {
    map_code_points(term, java_to_lower);
}

/// UpperCaseFilter (`uppercase`): Java `Character.toUpperCase` per code point.
pub fn uppercase(term: &mut Vec<u16>) {
    map_code_points(term, java_to_upper);
}

fn is_upper_vowel(v: u16) -> bool {
    matches!(
        v,
        0x41 | 0x45 | 0x49 | 0x4F | 0x55 // A E I O U
            // vowels with acute accent (fada)
            | 0xC1 | 0xC9 | 0xCD | 0xD3 | 0xDA
    )
}

/// IrishLowerCaseFilter: "nA"/"tE"... become "n-a"/"t-e"..., then
/// `Character.toLowerCase(char)` per UTF-16 unit (surrogates stay as they are).
pub fn irish_lowercase(term: &mut Vec<u16>) {
    let mut idx = 0;
    if term.len() > 1 && (term[0] == b'n' as u16 || term[0] == b't' as u16) && is_upper_vowel(term[1]) {
        term.insert(1, b'-' as u16);
        idx = 2;
    }
    for c in &mut term[idx..] {
        // BMP lowercase mappings stay in the BMP (checked by gen_chars.py)
        *c = java_to_lower(*c as u32) as u16;
    }
}

const LATIN_CAPITAL_LETTER_I: u32 = 0x49;
const LATIN_SMALL_LETTER_I: u16 = 0x69;
const LATIN_SMALL_LETTER_DOTLESS_I: u16 = 0x131;
const COMBINING_DOT_ABOVE: u32 = 0x307;

/// TurkishLowerCaseFilter: I -> dotless i unless a COMBINING DOT ABOVE follows
/// (after other non-spacing marks), in which case I + dot -> i; then Java
/// lowercase per code point.
pub fn turkish_lowercase(term: &mut Vec<u16>) {
    let buffer = term.as_mut_slice();
    let mut length = buffer.len();
    let mut i_or_after = false;
    let mut i = 0;
    while i < length {
        let (ch, _) = code_point_at(buffer, i, length);
        i_or_after = ch == LATIN_CAPITAL_LETTER_I || (i_or_after && is_non_spacing_mark(ch));
        if i_or_after {
            // all the special I turkish handling happens here.
            match ch {
                // remove COMBINING_DOT_ABOVE to mimic composed lowercase
                COMBINING_DOT_ABOVE => {
                    length = turkish_delete(buffer, i, length);
                    continue;
                }
                // i itself, it depends if it is followed by COMBINING_DOT_ABOVE
                LATIN_CAPITAL_LETTER_I => {
                    if is_before_dot(buffer, i + 1, length) {
                        buffer[i] = LATIN_SMALL_LETTER_I;
                    } else {
                        buffer[i] = LATIN_SMALL_LETTER_DOTLESS_I;
                        i_or_after = false;
                    }
                    i += 1;
                    continue;
                }
                _ => {}
            }
        }
        i += to_chars(java_to_lower(ch), buffer, i);
    }
    term.truncate(length);
}

fn is_before_dot(s: &[u16], pos: usize, len: usize) -> bool {
    let mut i = pos;
    while i < len {
        let (ch, n) = code_point_at(s, i, len);
        if !is_non_spacing_mark(ch) {
            return false;
        }
        if ch == COMBINING_DOT_ABOVE {
            return true;
        }
        i += n;
    }
    false
}

fn turkish_delete(s: &mut [u16], pos: usize, len: usize) -> usize {
    if pos < len {
        s.copy_within(pos + 1..len, pos);
    }
    len - 1
}

/// halfwidth kana mappings: 0xFF65-0xFF9F
const KANA_NORM: [u16; 59] = [
    0x30fb, 0x30f2, 0x30a1, 0x30a3, 0x30a5, 0x30a7, 0x30a9, 0x30e3, 0x30e5, 0x30e7, 0x30c3, 0x30fc,
    0x30a2, 0x30a4, 0x30a6, 0x30a8, 0x30aa, 0x30ab, 0x30ad, 0x30af, 0x30b1, 0x30b3, 0x30b5, 0x30b7,
    0x30b9, 0x30bb, 0x30bd, 0x30bf, 0x30c1, 0x30c4, 0x30c6, 0x30c8, 0x30ca, 0x30cb, 0x30cc, 0x30cd,
    0x30ce, 0x30cf, 0x30d2, 0x30d5, 0x30d8, 0x30db, 0x30de, 0x30df, 0x30e0, 0x30e1, 0x30e2, 0x30e4,
    0x30e6, 0x30e8, 0x30e9, 0x30ea, 0x30eb, 0x30ec, 0x30ed, 0x30ef, 0x30f3, 0x3099, 0x309A,
];

/// kana combining diffs: 0x30A6-0x30FD
const KANA_COMBINE_VOICED: [u8; 88] = [
    78, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1,
    0, 1, 0, 1, 0, 1, 0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0,
    1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 8, 8, 8, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
];

const KANA_COMBINE_HALF_VOICED: [u8; 88] = [
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0,
    2, 0, 0, 2, 0, 0, 2, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
];

/// CJKWidthFilter: fullwidth ASCII -> ASCII, halfwidth katakana -> fullwidth
/// (a halfwidth (semi-)voiced sound mark combines with the preceding kana).
pub fn cjk_width(term: &mut Vec<u16>) {
    let mut length = term.len();
    let text = term.as_mut_slice();
    let mut i = 0;
    while i < length {
        let ch = text[i];
        if (0xFF01..=0xFF5E).contains(&ch) {
            // Fullwidth ASCII variants
            text[i] -= 0xFEE0;
        } else if (0xFF65..=0xFF9F).contains(&ch) {
            // Halfwidth Katakana variants
            if (ch == 0xFF9E || ch == 0xFF9F) && i > 0 && combine(text, i, ch) {
                length = super::stem_util::delete(text, i, length);
                // Java: delete(text, i--, length); the loop's i++ revisits i
                i -= 1;
            } else {
                text[i] = KANA_NORM[(ch - 0xFF65) as usize];
            }
        }
        i += 1;
    }
    term.truncate(length);
}

fn combine(text: &mut [u16], pos: usize, ch: u16) -> bool {
    let prev = text[pos - 1];
    if (0x30A6..=0x30FD).contains(&prev) {
        let k = (prev - 0x30A6) as usize;
        text[pos - 1] += if ch == 0xFF9F { KANA_COMBINE_HALF_VOICED[k] } else { KANA_COMBINE_VOICED[k] } as u16;
        return text[pos - 1] != prev;
    }
    false
}

/// DecimalDigitFilter: every non-ASCII decimal digit (DECIMAL_DIGIT_NUMBER, any
/// plane) becomes the ASCII digit of its numeric value.
pub fn decimal_digit(term: &mut Vec<u16>) {
    let mut length = term.len();
    let buffer = term.as_mut_slice();
    let mut i = 0;
    while i < length {
        let (ch, _) = code_point_at(buffer, i, length);
        // look for digits outside of basic latin
        if ch > 0x7F {
            if let Some(v) = decimal_digit_value(ch) {
                // replace with equivalent basic latin digit
                buffer[i] = b'0' as u16 + v as u16;
                // if the original was supplementary, shrink the string
                if ch > 0xFFFF {
                    length = super::stem_util::delete(buffer, i + 1, length);
                }
            }
        }
        i += 1;
    }
    term.truncate(length);
}

#[cfg(test)]
mod tests {
    /// LOWER/UPPER_EXCEPTIONS are relative to Rust's case tables: regenerate
    /// char_data.rs (gen_chars.py) when the toolchain's Unicode version changes.
    #[test]
    fn char_data_matches_rust_unicode_version() {
        assert_eq!(char::UNICODE_VERSION, (17, 0, 0));
    }
}
