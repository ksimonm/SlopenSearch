//! Port of Lucene's br/BrazilianStemmer, as driven by BrazilianStemFilter.
//!
//! The Java stemmer works on Strings: it lowercases with
//! `String.toLowerCase(Locale pt-BR)` (Java simple case mapping per code point,
//! except İ -> "i̇" and the context-sensitive final sigma), folds some
//! accents, trims one leading/trailing punctuation char and then strips
//! suffixes. Strings here are UTF-16 `Vec<u16>`, compared unit by unit like
//! Java's `String.endsWith`/`equals`.
//!
//! Final sigma: Java decides "final" with a word `BreakIterator`
//! (ConditionalSpecialCasing.isFinalCased). `word_boundaries` reproduces the
//! JDK's legacy word rules (sun.text BreakIteratorRules), checked against
//! JDK 25: letters (L, Mc) and digits (N) join, `$mid_word`/`$mid_num`
//! punctuation joins two letters/digits (Cf chars may sit in between, Mn/Me
//! may not; likewise after a danda), Mn/Me/Cf attach to the preceding letter
//! or digit, danda ends a word, kana/kanji are not word letters, and a
//! supplementary letter/digit ends its word except at offset 0. These rules
//! were derived from probing the JDK (its supplementary handling is quirky)
//! and match it on all tested strings, but are not a full port of the
//! iterator's state tables. Only terms reaching this stemmer un-lowercased
//! can contain Σ at all.

use super::brazilian_data::*;
use super::case_width::java_to_lower;

type Str = Vec<u16>;

fn in_ranges<T: Copy + Ord>(ranges: &[(T, T)], c: T) -> bool {
    let i = ranges.partition_point(|r| r.1 < c);
    i < ranges.len() && ranges[i].0 <= c
}

// ---------------------------------------------------------------------------
// String.toLowerCase(Locale pt-BR)

/// Java `String.codePointAt` / `codePointBefore`: (code point, unit count).
fn code_point_at(v: &[u16], i: usize) -> (u32, usize) {
    let c = v[i] as u32;
    if (0xD800..0xDC00).contains(&c) && i + 1 < v.len() && (0xDC00..0xE000).contains(&(v[i + 1] as u32)) {
        return (0x10000 + ((c - 0xD800) << 10) + (v[i + 1] as u32 - 0xDC00), 2);
    }
    (c, 1)
}

fn code_point_before(v: &[u16], i: usize) -> (u32, usize) {
    let c = v[i - 1] as u32;
    if (0xDC00..0xE000).contains(&c) && i >= 2 && (0xD800..0xDC00).contains(&(v[i - 2] as u32)) {
        return (0x10000 + ((v[i - 2] as u32 - 0xD800) << 10) + (c - 0xDC00), 2);
    }
    (c, 1)
}

fn push_cp(out: &mut Str, cp: u32) {
    if cp < 0x10000 {
        out.push(cp as u16);
    } else {
        let v = cp - 0x10000;
        out.push(0xD800 + (v >> 10) as u16);
        out.push(0xDC00 + (v & 0x3FF) as u16);
    }
}

#[derive(Clone, Copy, PartialEq)]
enum Wb {
    Let,
    Dgt,
    /// Mn/Me (BMP): transparent after a letter or digit
    Mark,
    /// Cf (BMP): transparent everywhere (also between a mid char and the next letter)
    Format,
    Other,
}

fn wb_class(cp: u32) -> Wb {
    if cp <= 0xFFFF {
        let c = cp as u16;
        if in_ranges(WB_MARK_BMP, c) {
            return Wb::Mark;
        }
        // the soft hyphen is Cf but behaves as $mid_word
        if c != 0xAD && in_ranges(WB_FORMAT_BMP, c) {
            return Wb::Format;
        }
        // $let=[[[:L:][:Mc:]]-[$kanji$kata$hira]]
        let kana_kanji = c == 0x3005
            || (0x4E00..=0x9FA5).contains(&c)
            || (0xF900..=0xFA2D).contains(&c)
            || (0x3099..=0x309C).contains(&c)
            || (0x30A1..=0x30FE).contains(&c)
            || (0x3041..=0x309E).contains(&c);
        if kana_kanji {
            return Wb::Other;
        }
    }
    if in_ranges(WB_LET, cp) {
        return Wb::Let;
    }
    if in_ranges(WB_DGT, cp) {
        return Wb::Dgt;
    }
    Wb::Other
}

/// `$mid_word=[[:Pd:][:Pc:]­‧\"\'\.]`
fn is_mid_word(cp: u32) -> bool {
    cp == 0xAD
        || cp == 0x2027
        || cp == '"' as u32
        || cp == '\'' as u32
        || cp == '.' as u32
        || (cp <= 0xFFFF && in_ranges(WB_PD_PC_BMP, cp as u16))
}

/// `$mid_num=[\"\'\,٫\.]`
fn is_mid_num(cp: u32) -> bool {
    matches!(cp, 0x22 | 0x27 | 0x2C | 0x66B | 0x2E)
}

/// Word boundary flags for every UTF-16 offset 0..=len.
///
/// Supplementary letters/digits (observed JDK behaviour): one is always the
/// last character of its word, except at offset 0 where it starts a word
/// normally.
fn word_boundaries(v: &[u16]) -> Vec<bool> {
    let n = v.len();
    let mut b = vec![false; n + 1];
    b[0] = true;
    b[n] = true;
    let mut p = 0;
    while p < n {
        let (cp, w) = code_point_at(v, p);
        let mut i = p + w;
        let mut state = wb_class(cp);
        if (state == Wb::Let || state == Wb::Dgt) && w == 2 && p > 0 {
            // a supplementary letter/digit ends its word (unless the text starts with it)
        } else if state != Wb::Let && state != Wb::Dgt {
            // any other single char, with the marks that follow it
            while i < n {
                let (c, w) = code_point_at(v, i);
                match wb_class(c) {
                    Wb::Mark | Wb::Format => i += w,
                    _ => break,
                }
            }
        } else {
            // {$word}($number$word)*{$number}, $word = $let+($mid_word$let+)*$danda?,
            // $number = $dgt+($mid_num$dgt+)*; `after_danda` allows only a number next.
            let mut after_danda = false;
            while i < n {
                let (c, w) = code_point_at(v, i);
                match wb_class(c) {
                    cls @ (Wb::Let | Wb::Dgt) if !(after_danda && cls == Wb::Let) => {
                        state = cls;
                        after_danda = false;
                        i += w;
                        if w == 2 {
                            break;
                        }
                    }
                    // (after a danda, as after a mid char, only Cf is transparent)
                    Wb::Format => i += w,
                    Wb::Mark if !after_danda => i += w,
                    _ if !after_danda && state == Wb::Let && (c == 0x964 || c == 0x965) => {
                        after_danda = true;
                        i += w;
                    }
                    Wb::Other if !after_danda => {
                        // a mid char joins if (after Cf chars) the same kind follows
                        let mut k = i + w;
                        while k < n && wb_class(code_point_at(v, k).0) == Wb::Format {
                            k += code_point_at(v, k).1;
                        }
                        if k >= n {
                            break;
                        }
                        let (next, nw) = code_point_at(v, k);
                        let joins = match state {
                            Wb::Let => is_mid_word(c) && wb_class(next) == Wb::Let,
                            _ => is_mid_num(c) && wb_class(next) == Wb::Dgt,
                        };
                        if !joins {
                            break;
                        }
                        i = k + nw;
                        if nw == 2 {
                            break;
                        }
                    }
                    _ => break,
                }
            }
        }
        b[i] = true;
        p = i;
    }
    b
}

fn is_cased(cp: u32) -> bool {
    in_ranges(CASED, cp)
}

/// ConditionalSpecialCasing.isFinalCased(src, index, locale)
fn is_final_cased(src: &[u16], index: usize, boundary: &[bool]) -> bool {
    // Look for a preceding 'cased' letter
    let mut i = index;
    while !boundary[i] {
        let (ch, w) = code_point_before(src, i);
        if is_cased(ch) {
            // Check that there is no 'cased' letter after the index
            let mut j = index + code_point_at(src, index).1;
            while j < src.len() && !boundary[j] {
                let (ch, w) = code_point_at(src, j);
                if is_cased(ch) {
                    return false;
                }
                j += w;
            }
            return true;
        }
        i -= w;
    }
    false
}

/// `String.toLowerCase(Locale pt-BR)`.
fn to_lower_case(v: &[u16]) -> Str {
    let mut out = Vec::with_capacity(v.len() + 1);
    let mut boundary: Option<Vec<bool>> = None;
    let mut i = 0;
    while i < v.len() {
        let (cp, w) = code_point_at(v, i);
        match cp {
            0x03A3 => {
                let b = boundary.get_or_insert_with(|| word_boundaries(v));
                out.push(if is_final_cased(v, i, b) { 0x03C2 } else { 0x03C3 });
            }
            0x0130 => out.extend_from_slice(&[0x0069, 0x0307]),
            _ => push_cp(&mut out, java_to_lower(cp)),
        }
        i += w;
    }
    out
}

// ---------------------------------------------------------------------------
// BrazilianStemmer

fn is_vowel(value: u16) -> bool {
    matches!(value, 0x61 | 0x65 | 0x69 | 0x6F | 0x75) // a e i o u
}

/// Discard terms that contain non-letter characters (`Character.isLetter(char)`).
fn is_stemmable(term: &[u16]) -> bool {
    term.iter().all(|&c| in_ranges(LETTER_BMP, c))
}

fn is_indexable(term: &[u16]) -> bool {
    term.len() < 30 && term.len() > 2
}

fn get_r1(value: Option<&Str>) -> Option<Str> {
    let value = value?;
    // find 1st vowel
    let i = value.len() as isize - 1;
    let mut j: isize = 0;
    while j < i {
        if is_vowel(value[j as usize]) {
            break;
        }
        j += 1;
    }
    if j >= i {
        return None;
    }
    // find 1st non-vowel
    while j < i {
        if !is_vowel(value[j as usize]) {
            break;
        }
        j += 1;
    }
    if j >= i {
        return None;
    }
    Some(value[(j + 1) as usize..].to_vec())
}

fn get_rv(value: &Str) -> Option<Str> {
    let i = value.len() as isize - 1;
    // RV - IF the second letter is a consonant, RV is the region after the next following vowel,
    if i > 0 && !is_vowel(value[1]) {
        let mut j = 2;
        while j < i {
            if is_vowel(value[j as usize]) {
                break;
            }
            j += 1;
        }
        if j < i {
            return Some(value[(j + 1) as usize..].to_vec());
        }
    }
    // RV - OR if the first two letters are vowels, RV is the region after the next consonant,
    if i > 1 && is_vowel(value[0]) && is_vowel(value[1]) {
        let mut j = 2;
        while j < i {
            if !is_vowel(value[j as usize]) {
                break;
            }
            j += 1;
        }
        if j < i {
            return Some(value[(j + 1) as usize..].to_vec());
        }
    }
    // RV - AND otherwise (consonant-vowel case) RV is the region after the third letter.
    if i > 2 {
        return Some(value[3..].to_vec());
    }
    None
}

/// changeTerm: lowercase (pt-BR) and fold some accents.
fn change_term(value: &[u16]) -> Str {
    to_lower_case(value)
        .into_iter()
        .map(|c| match c {
            0xE1 | 0xE2 | 0xE3 => 0x61,        // á â ã -> a
            0xE9 | 0xEA => 0x65,               // é ê -> e
            0xED => 0x69,                      // í -> i
            0xF3 | 0xF4 | 0xF5 => 0x6F,        // ó ô õ -> o
            0xFA | 0xFC => 0x75,               // ú ü -> u
            0xE7 => 0x63,                      // ç -> c
            0xF1 => 0x6E,                      // ñ -> n
            _ => c,
        })
        .collect()
}

fn suffix(value: Option<&Str>, suffix: &str) -> bool {
    // all suffixes are ASCII
    match value {
        Some(v) => {
            let suf = suffix.as_bytes();
            suf.len() <= v.len() && v[v.len() - suf.len()..].iter().zip(suf).all(|(&a, &b)| a == b as u16)
        }
        None => false,
    }
}

fn remove_suffix(value: &Str, to_remove: &str) -> Str {
    if !suffix(Some(value), to_remove) {
        return value.clone();
    }
    value[..value.len() - to_remove.len()].to_vec()
}

fn replace_suffix(value: &Str, to_replace: &str, change_to: &str) -> Str {
    let vvalue = remove_suffix(value, to_replace);
    if *value == vvalue {
        value.clone()
    } else {
        let mut r = vvalue;
        r.extend(change_to.encode_utf16());
        r
    }
}

fn suffix_preceded(value: Option<&Str>, suf: &str, preceded: &str) -> bool {
    match value {
        Some(v) if suffix(Some(v), suf) => suffix(Some(&remove_suffix(v, suf)), preceded),
        _ => false,
    }
}

#[derive(Clone, Copy)]
enum Region {
    R1,
    R2,
    RV,
}

#[derive(Clone, Copy)]
enum Op {
    Remove,
    Replace(&'static str),
    /// `replaceSuffix(CT, "logias", "log")` whose result Java discards
    ReplaceDiscarded,
    /// replace if CT's suffix is preceded by "e" (the -ira/-iras rules)
    ReplaceAfterE(&'static str),
}

use Op::*;
use Region::*;

/// step1 rules: (minimum CT length of the group, suffix, region, operation).
const STEP1: &[(usize, &str, Region, Op)] = &[
    // suffix length = 7
    (0, "uciones", R2, Replace("u")),
    // suffix length = 6
    (6, "imentos", R2, Remove),
    (6, "amentos", R2, Remove),
    (6, "adores", R2, Remove),
    (6, "adoras", R2, Remove),
    (6, "logias", R2, ReplaceDiscarded),
    (6, "encias", R2, Replace("ente")),
    (6, "amente", R1, Remove),
    (6, "idades", R2, Remove),
    // suffix length = 5
    (5, "acoes", R2, Remove),
    (5, "imento", R2, Remove),
    (5, "amento", R2, Remove),
    (5, "adora", R2, Remove),
    (5, "ismos", R2, Remove),
    (5, "istas", R2, Remove),
    (5, "logia", R2, Replace("log")),
    (5, "ucion", R2, Replace("u")),
    (5, "encia", R2, Replace("ente")),
    (5, "mente", R2, Remove),
    (5, "idade", R2, Remove),
    // suffix length = 4
    (4, "acao", R2, Remove),
    (4, "ezas", R2, Remove),
    (4, "icos", R2, Remove),
    (4, "icas", R2, Remove),
    (4, "ismo", R2, Remove),
    (4, "avel", R2, Remove),
    (4, "ivel", R2, Remove),
    (4, "ista", R2, Remove),
    (4, "osos", R2, Remove),
    (4, "osas", R2, Remove),
    (4, "ador", R2, Remove),
    (4, "ivas", R2, Remove),
    (4, "ivos", R2, Remove),
    (4, "iras", RV, ReplaceAfterE("ir")),
    // suffix length = 3
    (3, "eza", R2, Remove),
    (3, "ico", R2, Remove),
    (3, "ica", R2, Remove),
    (3, "oso", R2, Remove),
    (3, "osa", R2, Remove),
    (3, "iva", R2, Remove),
    (3, "ivo", R2, Remove),
    (3, "ira", RV, ReplaceAfterE("ir")),
];

/// step2 rules: (minimum RV length of the group, RV suffix, suffix removed from CT).
/// (Duplicates and the "ira"/"ava" mismatch are as in the Java code.)
const STEP2: &[(usize, &str, &str)] = &[
    // suffix length = 7
    (7, "issemos", "issemos"),
    (7, "essemos", "essemos"),
    (7, "assemos", "assemos"),
    (7, "ariamos", "ariamos"),
    (7, "eriamos", "eriamos"),
    (7, "iriamos", "iriamos"),
    // suffix length = 6
    (6, "iremos", "iremos"),
    (6, "eremos", "eremos"),
    (6, "aremos", "aremos"),
    (6, "avamos", "avamos"),
    (6, "iramos", "iramos"),
    (6, "eramos", "eramos"),
    (6, "aramos", "aramos"),
    (6, "asseis", "asseis"),
    (6, "esseis", "esseis"),
    (6, "isseis", "isseis"),
    (6, "arieis", "arieis"),
    (6, "erieis", "erieis"),
    (6, "irieis", "irieis"),
    // suffix length = 5
    (5, "irmos", "irmos"),
    (5, "iamos", "iamos"),
    (5, "armos", "armos"),
    (5, "ermos", "ermos"),
    (5, "areis", "areis"),
    (5, "ereis", "ereis"),
    (5, "ireis", "ireis"),
    (5, "asses", "asses"),
    (5, "esses", "esses"),
    (5, "isses", "isses"),
    (5, "astes", "astes"),
    (5, "assem", "assem"),
    (5, "essem", "essem"),
    (5, "issem", "issem"),
    (5, "ardes", "ardes"),
    (5, "erdes", "erdes"),
    (5, "irdes", "irdes"),
    (5, "ariam", "ariam"),
    (5, "eriam", "eriam"),
    (5, "iriam", "iriam"),
    (5, "arias", "arias"),
    (5, "erias", "erias"),
    (5, "irias", "irias"),
    (5, "estes", "estes"),
    (5, "istes", "istes"),
    (5, "areis", "areis"),
    (5, "aveis", "aveis"),
    // suffix length = 4
    (4, "aria", "aria"),
    (4, "eria", "eria"),
    (4, "iria", "iria"),
    (4, "asse", "asse"),
    (4, "esse", "esse"),
    (4, "isse", "isse"),
    (4, "aste", "aste"),
    (4, "este", "este"),
    (4, "iste", "iste"),
    (4, "arei", "arei"),
    (4, "erei", "erei"),
    (4, "irei", "irei"),
    (4, "aram", "aram"),
    (4, "eram", "eram"),
    (4, "iram", "iram"),
    (4, "avam", "avam"),
    (4, "arem", "arem"),
    (4, "erem", "erem"),
    (4, "irem", "irem"),
    (4, "ando", "ando"),
    (4, "endo", "endo"),
    (4, "indo", "indo"),
    (4, "arao", "arao"),
    (4, "erao", "erao"),
    (4, "irao", "irao"),
    (4, "adas", "adas"),
    (4, "idas", "idas"),
    (4, "aras", "aras"),
    (4, "eras", "eras"),
    (4, "iras", "iras"),
    (4, "avas", "avas"),
    (4, "ares", "ares"),
    (4, "eres", "eres"),
    (4, "ires", "ires"),
    (4, "ados", "ados"),
    (4, "idos", "idos"),
    (4, "amos", "amos"),
    (4, "emos", "emos"),
    (4, "imos", "imos"),
    (4, "iras", "iras"),
    (4, "ieis", "ieis"),
    // suffix length = 3
    (3, "ada", "ada"),
    (3, "ida", "ida"),
    (3, "ara", "ara"),
    (3, "era", "era"),
    (3, "ira", "ava"),
    (3, "iam", "iam"),
    (3, "ado", "ado"),
    (3, "ido", "ido"),
    (3, "ias", "ias"),
    (3, "ais", "ais"),
    (3, "eis", "eis"),
    (3, "ira", "ira"),
    (3, "ear", "ear"),
    // suffix length = 2
    (2, "ia", "ia"),
    (2, "ei", "ei"),
    (2, "am", "am"),
    (2, "em", "em"),
    (2, "ar", "ar"),
    (2, "er", "er"),
    (2, "ir", "ir"),
    (2, "as", "as"),
    (2, "es", "es"),
    (2, "is", "is"),
    (2, "eu", "eu"),
    (2, "iu", "iu"),
    (2, "iu", "iu"),
    (2, "ou", "ou"),
];

struct BrazilianStemmer {
    ct: Str,
    r1: Option<Str>,
    r2: Option<Str>,
    rv: Option<Str>,
}

impl BrazilianStemmer {
    fn region(&self, r: Region) -> Option<&Str> {
        match r {
            R1 => self.r1.as_ref(),
            R2 => self.r2.as_ref(),
            RV => self.rv.as_ref(),
        }
    }

    fn step1(&mut self) -> bool {
        for &(min_len, suf, region, op) in STEP1 {
            if self.ct.len() < min_len {
                continue;
            }
            if !(suffix(Some(&self.ct), suf) && suffix(self.region(region), suf)) {
                continue;
            }
            match op {
                Remove => self.ct = remove_suffix(&self.ct, suf),
                Replace(to) => self.ct = replace_suffix(&self.ct, suf, to),
                ReplaceDiscarded => {}
                ReplaceAfterE(to) => {
                    if !suffix_preceded(Some(&self.ct), suf, "e") {
                        continue;
                    }
                    self.ct = replace_suffix(&self.ct, suf, to);
                }
            }
            return true;
        }
        // no ending was removed by step1
        false
    }

    fn step2(&mut self) -> bool {
        let Some(rv) = &self.rv else { return false };
        for &(min_len, rv_suffix, ct_suffix) in STEP2 {
            if rv.len() >= min_len && suffix(Some(rv), rv_suffix) {
                self.ct = remove_suffix(&self.ct, ct_suffix);
                return true;
            }
        }
        // no ending was removed by step2
        false
    }

    fn step3(&mut self) {
        if suffix(self.rv.as_ref(), "i") && suffix_preceded(self.rv.as_ref(), "i", "c") {
            self.ct = remove_suffix(&self.ct, "i");
        }
    }

    fn step4(&mut self) {
        for suf in ["os", "a", "i", "o"] {
            if suffix(self.rv.as_ref(), suf) {
                self.ct = remove_suffix(&self.ct, suf);
                return;
            }
        }
    }

    fn step5(&mut self) {
        if suffix(self.rv.as_ref(), "e") {
            if suffix_preceded(self.rv.as_ref(), "e", "gu") {
                self.ct = remove_suffix(&self.ct, "e");
                self.ct = remove_suffix(&self.ct, "u");
                return;
            }
            if suffix_preceded(self.rv.as_ref(), "e", "ci") {
                self.ct = remove_suffix(&self.ct, "e");
                self.ct = remove_suffix(&self.ct, "i");
                return;
            }
            self.ct = remove_suffix(&self.ct, "e");
        }
    }
}

/// createCT: changeTerm, then drop one leading and one trailing punctuation char.
fn create_ct(term: &[u16]) -> Str {
    let mut ct = change_term(term);
    if ct.len() < 2 {
        return ct;
    }
    // if the first character is ... , remove it
    if matches!(ct[0], 0x22 | 0x27 | 0x2D | 0x2C | 0x3B | 0x2E | 0x3F | 0x21) {
        ct.remove(0);
    }
    if ct.len() < 2 {
        return ct;
    }
    // if the last character is ... , remove it
    if matches!(ct[ct.len() - 1], 0x2D | 0x2C | 0x3B | 0x2E | 0x3F | 0x21 | 0x27 | 0x22) {
        ct.pop();
    }
    ct
}

/// BrazilianStemmer.stem: `None` when the term is not indexable (Java null).
fn stem(term: &[u16]) -> Option<Str> {
    let ct = create_ct(term);
    if !is_indexable(&ct) {
        return None;
    }
    if !is_stemmable(&ct) {
        return Some(ct);
    }
    let r1 = get_r1(Some(&ct));
    let r2 = get_r1(r1.as_ref());
    let rv = get_rv(&ct);
    let mut st = BrazilianStemmer { ct, r1, r2, rv };
    let mut altered = st.step1();
    if !altered {
        altered = st.step2();
    }
    if altered {
        st.step3();
    } else {
        st.step4();
    }
    st.step5();
    Some(st.ct)
}

/// BrazilianStemFilter: the term is replaced by the stemmer's result when it
/// is non-null and differs from the term. Terms whose normalized form (pt-BR
/// lowercase, accent folding, one leading/trailing punctuation char trimmed)
/// is not 3..=29 units long are kept; terms with non-letters get only that
/// normalization.
pub fn brazilian_stem(term: &mut Vec<u16>) {
    if let Some(s) = stem(term) {
        if s != *term {
            *term = s;
        }
    }
}
