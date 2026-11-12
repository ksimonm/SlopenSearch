//! Ports of Lucene's Arabic-script normalizers and stemmers, on UTF-16 terms:
//! ar/ArabicNormalizer, ar/ArabicStemmer, fa/PersianNormalizer,
//! fa/PersianStemmer, ckb/SoraniNormalizer and ckb/SoraniStemmer.

use super::char_data::FORMAT_BMP;
use super::stem_util::*;

// ---------------------------------------------------------------- Arabic

const ALEF: u16 = 0x0627;
const ALEF_MADDA: u16 = 0x0622;
const ALEF_HAMZA_ABOVE: u16 = 0x0623;
const ALEF_HAMZA_BELOW: u16 = 0x0625;
const BEH: u16 = 0x0628;
const YEH: u16 = 0x064A;
const DOTLESS_YEH: u16 = 0x0649;
const TEH_MARBUTA: u16 = 0x0629;
const TEH: u16 = 0x062A;
const REH: u16 = 0x0631;
const FEH: u16 = 0x0641;
const KAF: u16 = 0x0643;
const LAM: u16 = 0x0644;
const NOON: u16 = 0x0646;
const HEH: u16 = 0x0647;
const WAW: u16 = 0x0648;
const TATWEEL: u16 = 0x0640;
const FATHATAN: u16 = 0x064B;
const DAMMATAN: u16 = 0x064C;
const KASRATAN: u16 = 0x064D;
const FATHA: u16 = 0x064E;
const DAMMA: u16 = 0x064F;
const KASRA: u16 = 0x0650;
const SHADDA: u16 = 0x0651;
const SUKUN: u16 = 0x0652;
const ZWNJ: u16 = 0x200C;

/// ArabicNormalizationFilter: hamza-carrying alefs -> alef, dotless yeh -> yeh,
/// teh marbuta -> heh; removes tatweel and harakat.
pub fn arabic_normalize(term: &mut Vec<u16>) {
    apply(term, arabic_normalize_len);
}

fn arabic_normalize_len(s: &mut [u16], mut len: usize) -> usize {
    let mut i = 0;
    while i < len {
        match s[i] {
            ALEF_MADDA | ALEF_HAMZA_ABOVE | ALEF_HAMZA_BELOW => s[i] = ALEF,
            DOTLESS_YEH => s[i] = YEH,
            TEH_MARBUTA => s[i] = HEH,
            TATWEEL | KASRATAN | DAMMATAN | FATHATAN | FATHA | DAMMA | KASRA | SHADDA | SUKUN => {
                len = delete(s, i, len);
                continue; // Java: i--, then the loop's i++
            }
            _ => {}
        }
        i += 1;
    }
    len
}

const AR_PREFIXES: [&[u16]; 7] = [
    &[ALEF, LAM],
    &[WAW, ALEF, LAM],
    &[BEH, ALEF, LAM],
    &[KAF, ALEF, LAM],
    &[FEH, ALEF, LAM],
    &[LAM, LAM],
    &[WAW],
];

const AR_SUFFIXES: [&[u16]; 10] = [
    &[HEH, ALEF],
    &[ALEF, NOON],
    &[ALEF, TEH],
    &[WAW, NOON],
    &[YEH, NOON],
    &[YEH, HEH],
    &[YEH, TEH_MARBUTA],
    &[HEH],
    &[TEH_MARBUTA],
    &[YEH],
];

/// ArabicStemFilter (light10 stemmer): strips one prefix, then each listed
/// suffix in turn, keeping at least 2 chars (3 for the wa- prefix).
pub fn arabic_stem(term: &mut Vec<u16>) {
    apply(term, |s, len| {
        let len = arabic_stem_prefix(s, len);
        arabic_stem_suffix(s, len)
    });
}

fn arabic_stem_prefix(s: &mut [u16], len: usize) -> usize {
    for p in AR_PREFIXES {
        if arabic_starts_with_check_length(s, len, p) {
            return delete_n(s, 0, len, p.len());
        }
    }
    len
}

fn arabic_stem_suffix(s: &mut [u16], mut len: usize) -> usize {
    for suf in AR_SUFFIXES {
        if ends_with_check_length(s, len, suf) {
            len = delete_n(s, len - suf.len(), len, suf.len());
        }
    }
    len
}

fn arabic_starts_with_check_length(s: &[u16], len: usize, prefix: &[u16]) -> bool {
    if prefix.len() == 1 && len < 4 {
        // wa- prefix requires at least 3 characters
        false
    } else if len < prefix.len() + 2 {
        // other prefixes require only 2.
        false
    } else {
        s[..prefix.len()] == *prefix
    }
}

/// all suffixes require at least 2 characters after stemming (Arabic and Persian)
fn ends_with_check_length(s: &[u16], len: usize, suffix: &[u16]) -> bool {
    len >= suffix.len() + 2 && ends_with_units(s, len, suffix)
}

// ---------------------------------------------------------------- Persian

const FARSI_YEH: u16 = 0x06CC;
const YEH_BARREE: u16 = 0x06D2;
const KEHEH: u16 = 0x06A9;
const HAMZA_ABOVE: u16 = 0x0654;
const HEH_YEH: u16 = 0x06C0;
const HEH_GOAL: u16 = 0x06C1;

/// PersianNormalizationFilter: farsi yeh / yeh barree -> yeh, keheh -> kaf,
/// heh yeh / heh goal -> heh; removes hamza above.
pub fn persian_normalize(term: &mut Vec<u16>) {
    apply(term, |s, mut len| {
        let mut i = 0;
        while i < len {
            match s[i] {
                FARSI_YEH | YEH_BARREE => s[i] = YEH,
                KEHEH => s[i] = KAF,
                HEH_YEH | HEH_GOAL => s[i] = HEH,
                HAMZA_ABOVE => {
                    // necessary for HEH + HAMZA
                    len = delete(s, i, len);
                    continue;
                }
                _ => {}
            }
            i += 1;
        }
        len
    });
}

const FA_SUFFIXES: [&[u16]; 8] = [
    &[ALEF, TEH],
    &[ALEF, NOON],
    &[TEH, REH, YEH, NOON],
    &[TEH, REH],
    &[YEH, YEH],
    &[YEH],
    &[HEH, ALEF],
    &[ZWNJ],
];

/// PersianStemFilter: strips each listed suffix in turn, keeping >= 2 chars.
pub fn persian_stem(term: &mut Vec<u16>) {
    apply(term, |s, mut len| {
        for suf in FA_SUFFIXES {
            if ends_with_check_length(s, len, suf) {
                len = delete_n(s, len - suf.len(), len, suf.len());
            }
        }
        len
    });
}

// ---------------------------------------------------------------- Sorani

const AE: u16 = 0x06D5;
const HEH_DOACHASHMEE: u16 = 0x06BE;
const RREH: u16 = 0x0695;
const RREH_ABOVE: u16 = 0x0692;

/// Java `Character.getType(char) == Character.FORMAT`.
fn is_format(c: u16) -> bool {
    let i = FORMAT_BMP.partition_point(|r| r.1 < c);
    i < FORMAT_BMP.len() && FORMAT_BMP[i].0 <= c
}

/// SoraniNormalizationFilter: unifies yeh/kaf/heh/reh variants, final heh -> ae,
/// heh+ZWNJ -> ae; removes ZWNJ, tatweel, harakat and other FORMAT chars.
pub fn sorani_normalize(term: &mut Vec<u16>) {
    apply(term, |s, mut len| {
        let mut i = 0;
        while i < len {
            match s[i] {
                YEH | DOTLESS_YEH => s[i] = FARSI_YEH,
                KAF => s[i] = KEHEH,
                ZWNJ => {
                    if i > 0 && s[i - 1] == HEH {
                        s[i - 1] = AE;
                    }
                    len = delete(s, i, len);
                    continue;
                }
                HEH => {
                    if i == len - 1 {
                        s[i] = AE;
                    }
                }
                TEH_MARBUTA => s[i] = AE,
                HEH_DOACHASHMEE => s[i] = HEH,
                REH => {
                    if i == 0 {
                        s[i] = RREH;
                    }
                }
                RREH_ABOVE => s[i] = RREH,
                TATWEEL | KASRATAN | DAMMATAN | FATHATAN | FATHA | DAMMA | KASRA | SHADDA | SUKUN => {
                    len = delete(s, i, len);
                    continue;
                }
                c => {
                    if is_format(c) {
                        len = delete(s, i, len);
                        continue;
                    }
                }
            }
            i += 1;
        }
        len
    });
}

/// SoraniStemFilter: light stemmer for postpositions, possessives, definiteness
/// and plural endings (minimum lengths as in Lucene).
pub fn sorani_stem(term: &mut Vec<u16>) {
    apply(term, sorani_stem_len);
}

fn sorani_stem_len(s: &mut [u16], mut len: usize) -> usize {
    let ew = |len: usize, suf: &str| ends_with(s, len, suf);
    // postposition
    if len > 5 && ew(len, "دا") {
        len -= 2;
    } else if len > 4 && ew(len, "نا") {
        len -= 1;
    } else if len > 6 && ew(len, "ەوە") {
        len -= 3;
    }

    // possessive pronoun
    if len > 6 && (ew(len, "مان") || ew(len, "یان") || ew(len, "تان")) {
        len -= 3;
    }

    // indefinite singular ezafe
    if len > 6 && ew(len, "ێکی") {
        return len - 3;
    } else if len > 7 && ew(len, "یەکی") {
        return len - 4;
    }
    // indefinite singular
    if len > 5 && ew(len, "ێک") {
        return len - 2;
    } else if len > 6 && ew(len, "یەک") {
        return len - 3;
    }
    // definite singular
    else if len > 6 && ew(len, "ەکە") {
        return len - 3;
    } else if len > 5 && ew(len, "کە") {
        return len - 2;
    }
    // definite plural
    else if len > 7 && ew(len, "ەکان") {
        return len - 4;
    } else if len > 6 && ew(len, "کان") {
        return len - 3;
    }
    // indefinite plural ezafe
    else if len > 7 && ew(len, "یانی") {
        return len - 4;
    } else if len > 6 && ew(len, "انی") {
        return len - 3;
    }
    // indefinite plural
    else if len > 6 && ew(len, "یان") {
        return len - 3;
    } else if len > 5 && ew(len, "ان") {
        return len - 2;
    }
    // demonstrative plural
    else if len > 7 && ew(len, "یانە") {
        return len - 4;
    } else if len > 6 && ew(len, "انە") {
        return len - 3;
    }
    // demonstrative singular
    else if len > 5 && (ew(len, "ایە") || ew(len, "ەیە")) {
        return len - 2;
    } else if len > 4 && ew(len, "ە") {
        return len - 1;
    }
    // absolute singular ezafe
    else if len > 4 && ew(len, "ی") {
        return len - 1;
    }
    len
}
