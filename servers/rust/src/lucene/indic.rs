//! Ports of Lucene's Indic normalizers and stemmers, on UTF-16 terms:
//! in/IndicNormalizer, hi/HindiNormalizer, hi/HindiStemmer,
//! bn/BengaliNormalizer and bn/BengaliStemmer.

use super::stem_util::*;

// ---------------------------------------------------------------- IndicNormalizer

// Script flags (ScriptData.flag); script i covers the Unicode block
// 0x0900 + 0x80 * i .. 0x097F + 0x80 * i (ScriptData.base is its start).
const DEVANAGARI: u32 = 1;
const BENGALI: u32 = 2;
const GURMUKHI: u32 = 4;
const GUJARATI: u32 = 8;
const ORIYA: u32 = 16;
const TAMIL: u32 = 32;
const TELUGU: u32 = 64;
const KANNADA: u32 = 128;
const MALAYALAM: u32 = 256;

/// `Character.UnicodeBlock.of(c)` restricted to the nine scripts IndicNormalizer
/// knows: their blocks are exactly the consecutive 128-unit ranges from
/// DEVANAGARI (U+0900) to MALAYALAM (U+0D7F). Returns the script index (0..9).
fn indic_block(c: u16) -> Option<u32> {
    if (0x0900..0x0D80).contains(&c) {
        Some(((c - 0x0900) >> 7) as u32)
    } else {
        None
    }
}

/// {ch0, ch1, ch2 (-1: none, 0xFF: ZWJ), result, script flags}, relative to the block base.
#[rustfmt::skip]
const DECOMPOSITIONS: [(i32, i32, i32, i32, u32); 72] = [
    /* devanagari, gujarati vowel candra O */
    (0x05, 0x3E, 0x45, 0x11, DEVANAGARI | GUJARATI),
    /* devanagari short O */
    (0x05, 0x3E, 0x46, 0x12, DEVANAGARI),
    /* devanagari, gujarati letter O */
    (0x05, 0x3E, 0x47, 0x13, DEVANAGARI | GUJARATI),
    /* devanagari letter AI, gujarati letter AU */
    (0x05, 0x3E, 0x48, 0x14, DEVANAGARI | GUJARATI),
    /* devanagari, bengali, gurmukhi, gujarati, oriya AA */
    (0x05, 0x3E, -1, 0x06, DEVANAGARI | BENGALI | GURMUKHI | GUJARATI | ORIYA),
    /* devanagari letter candra A */
    (0x05, 0x45, -1, 0x72, DEVANAGARI),
    /* gujarati vowel candra E */
    (0x05, 0x45, -1, 0x0D, GUJARATI),
    /* devanagari letter short A */
    (0x05, 0x46, -1, 0x04, DEVANAGARI),
    /* gujarati letter E */
    (0x05, 0x47, -1, 0x0F, GUJARATI),
    /* gurmukhi, gujarati letter AI */
    (0x05, 0x48, -1, 0x10, GURMUKHI | GUJARATI),
    /* devanagari, gujarati vowel candra O */
    (0x05, 0x49, -1, 0x11, DEVANAGARI | GUJARATI),
    /* devanagari short O */
    (0x05, 0x4A, -1, 0x12, DEVANAGARI),
    /* devanagari, gujarati letter O */
    (0x05, 0x4B, -1, 0x13, DEVANAGARI | GUJARATI),
    /* devanagari letter AI, gurmukhi letter AU, gujarati letter AU */
    (0x05, 0x4C, -1, 0x14, DEVANAGARI | GURMUKHI | GUJARATI),
    /* devanagari, gujarati vowel candra O */
    (0x06, 0x45, -1, 0x11, DEVANAGARI | GUJARATI),
    /* devanagari short O */
    (0x06, 0x46, -1, 0x12, DEVANAGARI),
    /* devanagari, gujarati letter O */
    (0x06, 0x47, -1, 0x13, DEVANAGARI | GUJARATI),
    /* devanagari letter AI, gujarati letter AU */
    (0x06, 0x48, -1, 0x14, DEVANAGARI | GUJARATI),
    /* malayalam letter II */
    (0x07, 0x57, -1, 0x08, MALAYALAM),
    /* devanagari letter UU */
    (0x09, 0x41, -1, 0x0A, DEVANAGARI),
    /* tamil, malayalam letter UU (some styles) */
    (0x09, 0x57, -1, 0x0A, TAMIL | MALAYALAM),
    /* malayalam letter AI */
    (0x0E, 0x46, -1, 0x10, MALAYALAM),
    /* devanagari candra E */
    (0x0F, 0x45, -1, 0x0D, DEVANAGARI),
    /* devanagari short E */
    (0x0F, 0x46, -1, 0x0E, DEVANAGARI),
    /* devanagari AI */
    (0x0F, 0x47, -1, 0x10, DEVANAGARI),
    /* oriya AI */
    (0x0F, 0x57, -1, 0x10, ORIYA),
    /* malayalam letter OO */
    (0x12, 0x3E, -1, 0x13, MALAYALAM),
    /* telugu, kannada letter AU */
    (0x12, 0x4C, -1, 0x14, TELUGU | KANNADA),
    /* telugu letter OO */
    (0x12, 0x55, -1, 0x13, TELUGU),
    /* tamil, malayalam letter AU */
    (0x12, 0x57, -1, 0x14, TAMIL | MALAYALAM),
    /* oriya letter AU */
    (0x13, 0x57, -1, 0x14, ORIYA),
    /* devanagari qa */
    (0x15, 0x3C, -1, 0x58, DEVANAGARI),
    /* devanagari, gurmukhi khha */
    (0x16, 0x3C, -1, 0x59, DEVANAGARI | GURMUKHI),
    /* devanagari, gurmukhi ghha */
    (0x17, 0x3C, -1, 0x5A, DEVANAGARI | GURMUKHI),
    /* devanagari, gurmukhi za */
    (0x1C, 0x3C, -1, 0x5B, DEVANAGARI | GURMUKHI),
    /* devanagari dddha, bengali, oriya rra */
    (0x21, 0x3C, -1, 0x5C, DEVANAGARI | BENGALI | ORIYA),
    /* devanagari, bengali, oriya rha */
    (0x22, 0x3C, -1, 0x5D, DEVANAGARI | BENGALI | ORIYA),
    /* malayalam chillu nn */
    (0x23, 0x4D, 0xFF, 0x7A, MALAYALAM),
    /* bengali khanda ta */
    (0x24, 0x4D, 0xFF, 0x4E, BENGALI),
    /* devanagari nnna */
    (0x28, 0x3C, -1, 0x29, DEVANAGARI),
    /* malayalam chillu n */
    (0x28, 0x4D, 0xFF, 0x7B, MALAYALAM),
    /* devanagari, gurmukhi fa */
    (0x2B, 0x3C, -1, 0x5E, DEVANAGARI | GURMUKHI),
    /* devanagari, bengali yya */
    (0x2F, 0x3C, -1, 0x5F, DEVANAGARI | BENGALI),
    /* telugu letter vocalic R */
    (0x2C, 0x41, 0x41, 0x0B, TELUGU),
    /* devanagari rra */
    (0x30, 0x3C, -1, 0x31, DEVANAGARI),
    /* malayalam chillu rr */
    (0x30, 0x4D, 0xFF, 0x7C, MALAYALAM),
    /* malayalam chillu l */
    (0x32, 0x4D, 0xFF, 0x7D, MALAYALAM),
    /* devanagari llla */
    (0x33, 0x3C, -1, 0x34, DEVANAGARI),
    /* malayalam chillu ll */
    (0x33, 0x4D, 0xFF, 0x7E, MALAYALAM),
    /* telugu letter MA */
    (0x35, 0x41, -1, 0x2E, TELUGU),
    /* devanagari, gujarati vowel sign candra O */
    (0x3E, 0x45, -1, 0x49, DEVANAGARI | GUJARATI),
    /* devanagari vowel sign short O */
    (0x3E, 0x46, -1, 0x4A, DEVANAGARI),
    /* devanagari, gujarati vowel sign O */
    (0x3E, 0x47, -1, 0x4B, DEVANAGARI | GUJARATI),
    /* devanagari, gujarati vowel sign AU */
    (0x3E, 0x48, -1, 0x4C, DEVANAGARI | GUJARATI),
    /* kannada vowel sign II */
    (0x3F, 0x55, -1, 0x40, KANNADA),
    /* gurmukhi vowel sign UU (when stacking) */
    (0x41, 0x41, -1, 0x42, GURMUKHI),
    /* tamil, malayalam vowel sign O */
    (0x46, 0x3E, -1, 0x4A, TAMIL | MALAYALAM),
    /* kannada vowel sign OO */
    (0x46, 0x42, 0x55, 0x4B, KANNADA),
    /* kannada vowel sign O */
    (0x46, 0x42, -1, 0x4A, KANNADA),
    /* malayalam vowel sign AI (if reordered twice) */
    (0x46, 0x46, -1, 0x48, MALAYALAM),
    /* telugu, kannada vowel sign EE */
    (0x46, 0x55, -1, 0x47, TELUGU | KANNADA),
    /* telugu, kannada vowel sign AI */
    (0x46, 0x56, -1, 0x48, TELUGU | KANNADA),
    /* tamil, malayalam vowel sign AU */
    (0x46, 0x57, -1, 0x4C, TAMIL | MALAYALAM),
    /* bengali, oriya vowel sign O, tamil, malayalam vowel sign OO */
    (0x47, 0x3E, -1, 0x4B, BENGALI | ORIYA | TAMIL | MALAYALAM),
    /* bengali, oriya vowel sign AU */
    (0x47, 0x57, -1, 0x4C, BENGALI | ORIYA),
    /* kannada vowel sign OO */
    (0x4A, 0x55, -1, 0x4B, KANNADA),
    /* gurmukhi letter I */
    (0x72, 0x3F, -1, 0x07, GURMUKHI),
    /* gurmukhi letter II */
    (0x72, 0x40, -1, 0x08, GURMUKHI),
    /* gurmukhi letter EE */
    (0x72, 0x47, -1, 0x0F, GURMUKHI),
    /* gurmukhi letter U */
    (0x73, 0x41, -1, 0x09, GURMUKHI),
    /* gurmukhi letter UU */
    (0x73, 0x42, -1, 0x0A, GURMUKHI),
    /* gurmukhi letter OO */
    (0x73, 0x4B, -1, 0x13, GURMUKHI),
];

/// ScriptData.decompMask: does any decomposition of this script start with `ch`?
fn in_decomp_mask(ch: i32, flag: u32) -> bool {
    DECOMPOSITIONS.iter().any(|d| d.0 == ch && d.4 & flag != 0)
}

/// IndicNormalizationFilter: composes decomposed Indic sequences (vowel + sign,
/// consonant + nukta, chillu via virama + ZWJ, ...) within one script block.
pub fn indic_normalize(term: &mut Vec<u16>) {
    apply(term, |text, mut len| {
        let mut i = 0;
        while i < len {
            if let Some(script) = indic_block(text[i]) {
                let base = 0x0900 + 0x80 * script as i32;
                let flag = 1u32 << script;
                let ch = text[i] as i32 - base;
                if in_decomp_mask(ch, flag) {
                    len = compose(ch, script, base, flag, text, i, len);
                }
            }
            i += 1;
        }
        len
    });
}

fn compose(ch0: i32, block0: u32, base: i32, flag: u32, text: &mut [u16], pos: usize, mut len: usize) -> usize {
    if pos + 1 >= len {
        /* need at least 2 chars! */
        return len;
    }
    let ch1 = text[pos + 1] as i32 - base;
    let block1 = indic_block(text[pos + 1]);
    if block1 != Some(block0) {
        /* needs to be the same writing system */
        return len;
    }
    let mut ch2 = -1;
    if pos + 2 < len {
        ch2 = text[pos + 2] as i32 - base;
        if text[pos + 2] == 0x200D {
            // ZWJ
            ch2 = 0xFF;
        } else if indic_block(text[pos + 2]) != block1 {
            // still allow a 2-char match
            ch2 = -1;
        }
    }
    for d in DECOMPOSITIONS.iter() {
        if d.0 == ch0 && (d.4 & flag) != 0 && d.1 == ch1 && (d.2 < 0 || d.2 == ch2) {
            text[pos] = (base + d.3) as u16;
            len = delete(text, pos + 1, len);
            if d.2 >= 0 {
                len = delete(text, pos + 1, len);
            }
            return len;
        }
    }
    len
}

// ---------------------------------------------------------------- Hindi

/// HindiNormalizationFilter (Lucene skips keyword tokens): folds nukta forms,
/// chandra/short vowels, long vowels to short, dead n to bindu; drops
/// virama, nukta, ZWJ and ZWNJ.
pub fn hindi_normalize(term: &mut Vec<u16>) {
    apply(term, |s, mut len| {
        let mut i = 0;
        while i < len {
            let r: u16 = match s[i] {
                // dead n -> bindu
                0x0928 => {
                    if i + 1 < len && s[i + 1] == 0x094D {
                        s[i] = 0x0902;
                        len = delete(s, i + 1, len);
                    }
                    0
                }
                // candrabindu -> bindu
                0x0901 => 0x0902,
                // nukta deletions
                0x093C => {
                    len = delete(s, i, len);
                    continue;
                }
                0x0929 => 0x0928,
                0x0931 => 0x0930,
                0x0934 => 0x0933,
                0x0958 => 0x0915,
                0x0959 => 0x0916,
                0x095A => 0x0917,
                0x095B => 0x091C,
                0x095C => 0x0921,
                0x095D => 0x0922,
                0x095E => 0x092B,
                0x095F => 0x092F,
                // zwj/zwnj -> delete; virama -> delete
                0x200D | 0x200C | 0x094D => {
                    len = delete(s, i, len);
                    continue;
                }
                // chandra/short -> replace
                0x0945 | 0x0946 => 0x0947,
                0x0949 | 0x094A => 0x094B,
                0x090D | 0x090E => 0x090F,
                0x0911 | 0x0912 => 0x0913,
                0x0972 => 0x0905,
                // long -> short ind. vowels
                0x0906 => 0x0905,
                0x0908 => 0x0907,
                0x090A => 0x0909,
                0x0960 => 0x090B,
                0x0961 => 0x090C,
                0x0910 => 0x090F,
                0x0914 => 0x0913,
                // long -> short dep. vowels
                0x0940 => 0x093F,
                0x0942 => 0x0941,
                0x0944 => 0x0943,
                0x0963 => 0x0962,
                0x0948 => 0x0947,
                0x094C => 0x094B,
                _ => 0,
            };
            if r != 0 {
                s[i] = r;
            }
            i += 1;
        }
        len
    });
}

const HI_SUFFIXES_5: [&str; 7] = ["ाएंगी", "ाएंगे", "ाऊंगी", "ाऊंगा", "ाइयाँ", "ाइयों", "ाइयां"];
const HI_SUFFIXES_4: [&str; 18] = [
    "ाएगी", "ाएगा", "ाओगी", "ाओगे", "एंगी", "ेंगी", "एंगे", "ेंगे", "ूंगी", "ूंगा", "ातीं", "नाओं", "नाएं",
    "ताओं", "ताएं", "ियाँ", "ियों", "ियां",
];
const HI_SUFFIXES_3: [&str; 19] = [
    "ाकर", "ाइए", "ाईं", "ाया", "ेगी", "ेगा", "ोगी", "ोगे", "ाने", "ाना", "ाते", "ाती", "ाता", "तीं",
    "ाओं", "ाएं", "ुओं", "ुएं", "ुआं",
];
const HI_SUFFIXES_2: [&str; 16] = [
    "कर", "ाओ", "िए", "ाई", "ाए", "ने", "नी", "ना", "ते", "ीं", "ती", "ता", "ाँ", "ां", "ों", "ें",
];
const HI_SUFFIXES_1: [&str; 7] = ["ो", "े", "ू", "ु", "ी", "ि", "ा"];

/// HindiStemFilter (Lucene skips keyword tokens): light suffix stripper; a
/// suffix of n chars is removed only when the term is longer than n + 1.
pub fn hindi_stem(term: &mut Vec<u16>) {
    apply(term, |s, len| {
        let groups: [(usize, &[&str]); 5] = [
            (5, &HI_SUFFIXES_5),
            (4, &HI_SUFFIXES_4),
            (3, &HI_SUFFIXES_3),
            (2, &HI_SUFFIXES_2),
            (1, &HI_SUFFIXES_1),
        ];
        strip_first_suffix(s, len, &groups)
    });
}

/// The shared shape of the Hindi and Bengali stemmers: for each group of
/// n-char suffixes (longest first), if `len > n + 1` and one matches, drop n chars.
fn strip_first_suffix(s: &[u16], len: usize, groups: &[(usize, &[&str])]) -> usize {
    for &(n, sufs) in groups {
        if len > n + 1 && sufs.iter().any(|suf| ends_with(s, len, suf)) {
            return len - n;
        }
    }
    len
}

// ---------------------------------------------------------------- Bengali

/// BengaliNormalizationFilter (Lucene skips keyword tokens): Lucene's
/// rule-based Bengali spelling normalization (chandrabindu, ja/ba phala,
/// visarga, sibilants, na, ra, khanda ta...).
pub fn bengali_normalize(term: &mut Vec<u16>) {
    apply(term, bengali_normalize_len);
}

fn bengali_normalize_len(s: &mut [u16], mut len: usize) -> usize {
    // i is signed: the Ja Phala rule can step it back to -1 before the i++.
    let mut i: isize = 0;
    while (i as usize) < len {
        let u = i as usize;
        match s[u] {
            // delete Chandrabindu
            0x0981 => {
                len = delete(s, u, len);
                i -= 1;
            }
            // DirghoI kar -> RosshoI kar
            0x09C0 => s[u] = 0x09BF,
            // DirghoU kar -> RosshoU kar
            0x09C2 => s[u] = 0x09C1,
            // Khio (Ka + Hoshonto + Murdorno Sh)
            0x0995 => {
                if u + 2 < len && s[u + 1] == 0x09CD && s[u + 2] == 0x09BF {
                    if u == 0 {
                        s[u] = 0x0996;
                        len = delete(s, u + 2, len);
                        len = delete(s, u + 1, len);
                    } else {
                        s[u + 1] = 0x0996;
                        len = delete(s, u + 2, len);
                    }
                }
            }
            // Nga to Anusvara
            0x0999 => s[u] = 0x0982,
            // Ja Phala
            0x09AF => {
                if u == 2 && s[u - 1] == 0x09CD {
                    s[u - 1] = 0x09C7;
                    if u + 1 < len && s[u + 1] == 0x09BE {
                        len = delete(s, u + 1, len);
                    }
                    len = delete(s, u, len);
                    i -= 1;
                } else if u >= 1 && s[u - 1] == 0x09CD {
                    len = delete(s, u, len);
                    len = delete(s, u - 1, len);
                    i -= 2;
                }
            }
            // Ba Phalaa
            0x09AC => {
                if !((u >= 1 && s[u - 1] != 0x09CD) || u == 0) {
                    if u == 2 {
                        len = delete(s, u, len);
                        len = delete(s, u - 1, len);
                        i -= 2;
                    } else if u >= 5 && s[u - 3] == 0x09CD {
                        len = delete(s, u, len);
                        len = delete(s, u - 1, len);
                        i -= 2;
                    } else if u >= 2 {
                        s[u - 1] = s[u - 2];
                        len = delete(s, u, len);
                        i -= 1;
                    }
                }
            }
            // Visarga
            0x0983 => {
                if u == len - 1 {
                    if len <= 3 {
                        s[u] = 0x09B9;
                    } else {
                        len = delete(s, u, len);
                    }
                } else {
                    s[u] = s[u + 1];
                }
            }
            // All sh
            0x09B6 | 0x09B7 => s[u] = 0x09B8,
            // check na
            0x09A3 => s[u] = 0x09A8,
            // check ra
            0x09DC | 0x09DD => s[u] = 0x09B0,
            0x09CE => s[u] = 0x09A4,
            _ => {}
        }
        i += 1;
    }
    len
}

const BN_SUFFIXES_8: [&str; 6] = ["িয়াছিলাম", "িতেছিলাম", "িতেছিলেন", "ইতেছিলেন", "িয়াছিলেন", "ইয়াছিলেন"];
const BN_SUFFIXES_7: [&str; 7] = ["িতেছিলি", "িতেছিলে", "িয়াছিলা", "িয়াছিলে", "িতেছিলা", "িয়াছিলি", "য়েদেরকে"];
const BN_SUFFIXES_6: [&str; 7] = ["িতেছিস", "িতেছেন", "িয়াছিস", "িয়াছেন", "েছিলাম", "েছিলেন", "েদেরকে"];
const BN_SUFFIXES_5: [&str; 12] = ["িতেছি", "িতেছা", "িতেছে", "ছিলাম", "ছিলেন", "িয়াছি", "িয়াছা", "িয়াছে", "েছিলে", "েছিলা", "য়েদের", "দেরকে"];
const BN_SUFFIXES_4: [&str; 16] = ["িলাম", "িলেন", "িতাম", "িতেন", "িবেন", "ছিলি", "ছিলে", "ছিলা", "তেছে", "িতেছ", "খানা", "খানি", "গুলো", "গুলি", "য়েরা", "েদের"];
const BN_SUFFIXES_3: [&str; 27] = ["লাম", "িলি", "ইলি", "িলে", "ইলে", "লেন", "িলা", "ইলা", "তাম", "িতি", "ইতি", "িতে", "ইতে", "তেন", "িতা", "িবা", "ইবা", "িবি", "ইবি", "বেন", "িবে", "ইবে", "ছেন", "য়োন", "য়ের", "েরা", "দের"];
const BN_SUFFIXES_2: [&str; 23] = ["িস", "েন", "লি", "লে", "লা", "তি", "তে", "তা", "বি", "বে", "বা", "ছি", "ছা", "ছে", "ুন", "ুক", "টা", "টি", "নি", "ের", "তে", "রা", "কে"];
const BN_SUFFIXES_1: [&str; 7] = ["ি", "ী", "া", "ো", "ে", "ব", "ত"];

/// BengaliStemFilter (Lucene skips keyword tokens): light suffix stripper; a
/// suffix of n chars is removed only when the term is longer than n + 1.
pub fn bengali_stem(term: &mut Vec<u16>) {
    apply(term, |s, len| {
        let groups: [(usize, &[&str]); 8] = [
            (8, &BN_SUFFIXES_8),
            (7, &BN_SUFFIXES_7),
            (6, &BN_SUFFIXES_6),
            (5, &BN_SUFFIXES_5),
            (4, &BN_SUFFIXES_4),
            (3, &BN_SUFFIXES_3),
            (2, &BN_SUFFIXES_2),
            (1, &BN_SUFFIXES_1),
        ];
        strip_first_suffix(s, len, &groups)
    });
}
