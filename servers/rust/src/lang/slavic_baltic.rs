//! Ports of Lucene's light stemmers / normalizers for Czech, Bulgarian,
//! Latvian, Indonesian and Serbian, on UTF-16 terms: cz/CzechStemmer,
//! bg/BulgarianStemmer, lv/LatvianStemmer, id/IndonesianStemmer and
//! sr/SerbianNormalizationFilter.

use super::stem_util::*;

/// One UTF-16 unit of a BMP char literal.
const fn c(ch: char) -> u16 {
    ch as u16
}

// ---------------------------------------------------------------- Czech

/// CzechStemFilter (Lucene skips keyword tokens): removes case endings and
/// possessives, then normalizes the final consonant cluster.
pub fn czech_stem(term: &mut Vec<u16>) {
    apply(term, |s, len| {
        let mut len = cz_remove_case(s, len);
        len = cz_remove_possessives(s, len);
        if len > 0 {
            len = cz_normalize(s, len);
        }
        len
    });
}

fn any_ends_with(s: &[u16], len: usize, sufs: &[&str]) -> bool {
    sufs.iter().any(|suf| ends_with(s, len, suf))
}

fn cz_remove_case(s: &[u16], len: usize) -> usize {
    if len > 7 && ends_with(s, len, "atech") {
        return len - 5;
    }
    if len > 6 && any_ends_with(s, len, &["ětem", "etem", "atům"]) {
        return len - 4;
    }
    if len > 5
        && any_ends_with(
            s,
            len,
            &[
                "ech", "ich", "ích", "ého", "ěmi", "emi", "ému", "ěte", "ete", "ěti", "eti", "ího", "iho", "ími",
                "ímu", "imu", "ách", "ata", "aty", "ých", "ama", "ami", "ové", "ovi", "ými",
            ],
        )
    {
        return len - 3;
    }
    if len > 4 && any_ends_with(s, len, &["em", "es", "ém", "ím", "ům", "at", "ám", "os", "us", "ým", "mi", "ou"]) {
        return len - 2;
    }
    if len > 3 {
        const V: [u16; 12] = [
            c('a'),
            c('e'),
            c('i'),
            c('o'),
            c('u'),
            c('ů'),
            c('y'),
            c('á'),
            c('é'),
            c('í'),
            c('ý'),
            c('ě'),
        ];
        if V.contains(&s[len - 1]) {
            return len - 1;
        }
    }
    len
}

fn cz_remove_possessives(s: &[u16], len: usize) -> usize {
    if len > 5 && any_ends_with(s, len, &["ov", "in", "ův"]) {
        return len - 2;
    }
    len
}

fn cz_normalize(s: &mut [u16], len: usize) -> usize {
    if ends_with(s, len, "čt") {
        // čt -> ck
        s[len - 2] = c('c');
        s[len - 1] = c('k');
        return len;
    }
    if ends_with(s, len, "št") {
        // št -> sk
        s[len - 2] = c('s');
        s[len - 1] = c('k');
        return len;
    }
    let last = s[len - 1];
    if last == c('c') || last == c('č') {
        // [cč] -> k
        s[len - 1] = c('k');
        return len;
    }
    if last == c('z') || last == c('ž') {
        // [zž] -> h
        s[len - 1] = c('h');
        return len;
    }
    if len > 1 && s[len - 2] == c('e') {
        s[len - 2] = s[len - 1]; // e* > *
        return len - 1;
    }
    if len > 2 && s[len - 2] == c('ů') {
        s[len - 2] = c('o'); // *ů* -> *o*
        return len;
    }
    len
}

// ---------------------------------------------------------------- Bulgarian

/// BulgarianStemFilter (Lucene skips keyword tokens): terms shorter than 4
/// chars are kept; removes articles, plurals and final vowels.
pub fn bulgarian_stem(term: &mut Vec<u16>) {
    apply(term, bg_stem);
}

fn bg_stem(s: &mut [u16], mut len: usize) -> usize {
    if len < 4 {
        // do not stem
        return len;
    }
    if len > 5 && ends_with(s, len, "ища") {
        return len - 3;
    }
    len = bg_remove_article(s, len);
    len = bg_remove_plural(s, len);
    if len > 3 {
        if ends_with(s, len, "я") {
            len -= 1;
        }
        if ends_with(s, len, "а") || ends_with(s, len, "о") || ends_with(s, len, "е") {
            len -= 1;
        }
    }
    // the rule to rewrite ен -> н is duplicated in the paper.
    // in the perl implementation referenced by the paper, this is fixed.
    // (it is fixed here as well)
    if len > 4 && ends_with(s, len, "ен") {
        s[len - 2] = c('н'); // replace with н
        len -= 1;
    }
    if len > 5 && s[len - 2] == c('ъ') {
        s[len - 2] = s[len - 1]; // replace ъN with N
        len -= 1;
    }
    len
}

fn bg_remove_article(s: &[u16], len: usize) -> usize {
    if len > 6 && ends_with(s, len, "ият") {
        return len - 3;
    }
    if len > 5 && any_ends_with(s, len, &["ът", "то", "те", "та", "ия"]) {
        return len - 2;
    }
    if len > 4 && ends_with(s, len, "ят") {
        return len - 2;
    }
    len
}

fn bg_remove_plural(s: &mut [u16], len: usize) -> usize {
    if len > 6 {
        if ends_with(s, len, "овци") {
            return len - 3; // replace with о
        }
        if ends_with(s, len, "ове") {
            return len - 3;
        }
        if ends_with(s, len, "еве") {
            s[len - 3] = c('й'); // replace with й
            return len - 2;
        }
    }
    if len > 5 {
        if ends_with(s, len, "ища") {
            return len - 3;
        }
        if ends_with(s, len, "та") {
            return len - 2;
        }
        if ends_with(s, len, "ци") {
            s[len - 2] = c('к'); // replace with к
            return len - 1;
        }
        if ends_with(s, len, "зи") {
            s[len - 2] = c('г'); // replace with г
            return len - 1;
        }
        if s[len - 3] == c('е') && s[len - 1] == c('и') {
            s[len - 3] = c('я'); // replace е with я, remove и
            return len - 1;
        }
    }
    if len > 4 {
        if ends_with(s, len, "си") {
            s[len - 2] = c('х'); // replace with х
            return len - 1;
        }
        if ends_with(s, len, "и") {
            return len - 1;
        }
    }
    len
}

// ---------------------------------------------------------------- Latvian

/// (suffix, vowel count of the suffix, fires palatalization rules)
const LV_AFFIXES: [(&str, usize, bool); 38] = [
    ("ajiem", 3, false),
    ("ajai", 3, false),
    ("ajam", 2, false),
    ("ajām", 2, false),
    ("ajos", 2, false),
    ("ajās", 2, false),
    ("iem", 2, true),
    ("ajā", 2, false),
    ("ais", 2, false),
    ("ai", 2, false),
    ("ei", 2, false),
    ("ām", 1, false),
    ("am", 1, false),
    ("ēm", 1, false),
    ("īm", 1, false),
    ("im", 1, false),
    ("um", 1, false),
    ("us", 1, true),
    ("as", 1, false),
    ("ās", 1, false),
    ("es", 1, false),
    ("os", 1, true),
    ("ij", 1, false),
    ("īs", 1, false),
    ("ēs", 1, false),
    ("is", 1, false),
    ("ie", 1, false),
    ("u", 1, true),
    ("a", 1, true),
    ("i", 1, true),
    ("e", 1, false),
    ("ā", 1, false),
    ("ē", 1, false),
    ("ī", 1, false),
    ("ū", 1, false),
    ("o", 1, false),
    ("s", 0, false),
    ("š", 0, false),
];

/// LatvianStemFilter (Lucene skips keyword tokens): removes the first matching
/// affix (if the word has more vowels than it and keeps >= 3 chars), then
/// undoes palatalization for the affixes that cause it.
pub fn latvian_stem(term: &mut Vec<u16>) {
    apply(term, |s, mut len| {
        let num_vowels = lv_num_vowels(s, len);
        for &(affix, vc, palatalizes) in LV_AFFIXES.iter() {
            let alen = affix.encode_utf16().count();
            if num_vowels > vc && len >= alen + 3 && ends_with(s, len, affix) {
                len -= alen;
                return if palatalizes { lv_unpalatalize(s, len) } else { len };
            }
        }
        len
    });
}

fn lv_unpalatalize(s: &mut [u16], mut len: usize) -> usize {
    // we check the character removed: if it's -u then
    // it's 2,5, or 6 gen pl., and these two can only apply then.
    if s[len] == c('u') {
        // kš -> kst
        if ends_with(s, len, "kš") {
            len += 1;
            s[len - 2] = c('s');
            s[len - 1] = c('t');
            return len;
        }
        // ņņ -> nn
        if ends_with(s, len, "ņņ") {
            s[len - 2] = c('n');
            s[len - 1] = c('n');
            return len;
        }
    }

    // otherwise all other rules
    if ends_with(s, len, "pj") || ends_with(s, len, "bj") || ends_with(s, len, "mj") || ends_with(s, len, "vj") {
        // labial consonant
        return len - 1;
    }
    const PAIRS: [(&str, char, char); 6] = [
        ("šņ", 's', 'n'),
        ("žņ", 'z', 'n'),
        ("šļ", 's', 'l'),
        ("žļ", 'z', 'l'),
        ("ļņ", 'l', 'n'),
        ("ļļ", 'l', 'l'),
    ];
    for (suf, a, b) in PAIRS {
        if ends_with(s, len, suf) {
            s[len - 2] = c(a);
            s[len - 1] = c(b);
            return len;
        }
    }
    let last = s[len - 1];
    if last == c('č') {
        s[len - 1] = c('c');
    } else if last == c('ļ') {
        s[len - 1] = c('l');
    } else if last == c('ņ') {
        s[len - 1] = c('n');
    }
    len
}

fn lv_num_vowels(s: &[u16], len: usize) -> usize {
    const V: [u16; 9] = [c('a'), c('e'), c('i'), c('o'), c('u'), c('ā'), c('ī'), c('ē'), c('ū')];
    s[..len].iter().filter(|ch| V.contains(ch)).count()
}

// ---------------------------------------------------------------- Indonesian

const REMOVED_KE: u32 = 1;
const REMOVED_PENG: u32 = 2;
const REMOVED_DI: u32 = 4;
const REMOVED_MENG: u32 = 8;
const REMOVED_TER: u32 = 16;
const REMOVED_BER: u32 = 32;
const REMOVED_PE: u32 = 64;

/// State of one IndonesianStemmer.stem call.
struct IdStemmer {
    num_syllables: usize,
    flags: u32,
}

fn id_is_vowel(ch: u16) -> bool {
    matches!(ch, 0x61 | 0x65 | 0x69 | 0x6F | 0x75) // a e i o u
}

/// IndonesianStemFilter with stemDerivational=true (Lucene skips keyword tokens):
/// removes particles, possessive pronouns, then first/second order prefixes and
/// derivational suffixes, each only while the word has more than 2 syllables.
pub fn indonesian_stem(term: &mut Vec<u16>) {
    apply(term, |text, mut length| {
        let mut st = IdStemmer { num_syllables: 0, flags: 0 };
        st.num_syllables = text[..length].iter().filter(|&&ch| id_is_vowel(ch)).count();
        if st.num_syllables > 2 {
            length = st.remove_particle(text, length);
        }
        if st.num_syllables > 2 {
            length = st.remove_possessive_pronoun(text, length);
        }
        st.stem_derivational(text, length)
    });
}

impl IdStemmer {
    fn stem_derivational(&mut self, text: &mut [u16], mut length: usize) -> usize {
        let mut old_length = length;
        if self.num_syllables > 2 {
            length = self.remove_first_order_prefix(text, length);
        }
        if old_length != length {
            // a rule is fired
            old_length = length;
            if self.num_syllables > 2 {
                length = self.remove_suffix(text, length);
            }
            if old_length != length && self.num_syllables > 2 {
                // a rule is fired
                length = self.remove_second_order_prefix(text, length);
            }
        } else {
            // fail
            if self.num_syllables > 2 {
                length = self.remove_second_order_prefix(text, length);
            }
            if self.num_syllables > 2 {
                length = self.remove_suffix(text, length);
            }
        }
        length
    }

    fn remove_particle(&mut self, text: &[u16], length: usize) -> usize {
        if ends_with(text, length, "kah") || ends_with(text, length, "lah") || ends_with(text, length, "pun") {
            self.num_syllables -= 1;
            return length - 3;
        }
        length
    }

    fn remove_possessive_pronoun(&mut self, text: &[u16], length: usize) -> usize {
        if ends_with(text, length, "ku") || ends_with(text, length, "mu") {
            self.num_syllables -= 1;
            return length - 2;
        }
        if ends_with(text, length, "nya") {
            self.num_syllables -= 1;
            return length - 3;
        }
        length
    }

    /// Records `flag`, drops one syllable and deletes the first `n` chars.
    fn strip(&mut self, flag: u32, text: &mut [u16], length: usize, n: usize) -> usize {
        self.flags |= flag;
        self.num_syllables -= 1;
        delete_n(text, 0, length, n)
    }

    fn remove_first_order_prefix(&mut self, text: &mut [u16], length: usize) -> usize {
        if starts_with(text, length, "meng") {
            return self.strip(REMOVED_MENG, text, length, 4);
        }
        if starts_with(text, length, "meny") && length > 4 && id_is_vowel(text[4]) {
            text[3] = c('s');
            return self.strip(REMOVED_MENG, text, length, 3);
        }
        if starts_with(text, length, "men") {
            return self.strip(REMOVED_MENG, text, length, 3);
        }
        if starts_with(text, length, "mem") {
            return self.strip(REMOVED_MENG, text, length, 3);
        }
        if starts_with(text, length, "me") {
            return self.strip(REMOVED_MENG, text, length, 2);
        }
        if starts_with(text, length, "peng") {
            return self.strip(REMOVED_PENG, text, length, 4);
        }
        if starts_with(text, length, "peny") && length > 4 && id_is_vowel(text[4]) {
            text[3] = c('s');
            return self.strip(REMOVED_PENG, text, length, 3);
        }
        if starts_with(text, length, "peny") {
            return self.strip(REMOVED_PENG, text, length, 4);
        }
        if starts_with(text, length, "pen") && length > 3 && id_is_vowel(text[3]) {
            text[2] = c('t');
            return self.strip(REMOVED_PENG, text, length, 2);
        }
        if starts_with(text, length, "pen") {
            return self.strip(REMOVED_PENG, text, length, 3);
        }
        if starts_with(text, length, "pem") {
            return self.strip(REMOVED_PENG, text, length, 3);
        }
        if starts_with(text, length, "di") {
            return self.strip(REMOVED_DI, text, length, 2);
        }
        if starts_with(text, length, "ter") {
            return self.strip(REMOVED_TER, text, length, 3);
        }
        if starts_with(text, length, "ke") {
            return self.strip(REMOVED_KE, text, length, 2);
        }
        length
    }

    fn remove_second_order_prefix(&mut self, text: &mut [u16], length: usize) -> usize {
        if starts_with(text, length, "ber") {
            return self.strip(REMOVED_BER, text, length, 3);
        }
        if length == 7 && starts_with(text, length, "belajar") {
            return self.strip(REMOVED_BER, text, length, 3);
        }
        if starts_with(text, length, "be")
            && length > 4
            && !id_is_vowel(text[2])
            && text[3] == c('e')
            && text[4] == c('r')
        {
            return self.strip(REMOVED_BER, text, length, 2);
        }
        // "per" and "pelajar" set no flag
        if starts_with(text, length, "per") {
            return self.strip(0, text, length, 3);
        }
        if length == 7 && starts_with(text, length, "pelajar") {
            return self.strip(0, text, length, 3);
        }
        if starts_with(text, length, "pe") {
            return self.strip(REMOVED_PE, text, length, 2);
        }
        length
    }

    fn remove_suffix(&mut self, text: &[u16], length: usize) -> usize {
        let f = self.flags;
        if ends_with(text, length, "kan") && f & (REMOVED_KE | REMOVED_PENG | REMOVED_PE) == 0 {
            self.num_syllables -= 1;
            return length - 3;
        }
        if ends_with(text, length, "an") && f & (REMOVED_DI | REMOVED_MENG | REMOVED_TER) == 0 {
            self.num_syllables -= 1;
            return length - 2;
        }
        if ends_with(text, length, "i")
            && !ends_with(text, length, "si")
            && f & (REMOVED_BER | REMOVED_KE | REMOVED_PENG) == 0
        {
            self.num_syllables -= 1;
            return length - 1;
        }
        length
    }
}

// ---------------------------------------------------------------- Serbian

/// SerbianNormalizationFilter (applies to keyword tokens too): Cyrillic to
/// Latin, and both scripts folded to ASCII (đ -> dj, ž -> z, č/ć -> c, š -> s ...).
pub fn serbian_normalize(term: &mut Vec<u16>) {
    let mut out = Vec::with_capacity(term.len() + 4);
    for &ch in term.iter() {
        let m: &str = match char::from_u32(ch as u32) {
            Some(x) => match x {
                'а' => "a",
                'б' => "b",
                'в' => "v",
                'г' => "g",
                'д' => "d",
                'ђ' | 'đ' => "dj",
                'е' => "e",
                'ж' | 'з' | 'ž' => "z",
                'и' => "i",
                'ј' => "j",
                'к' => "k",
                'л' => "l",
                'љ' => "lj",
                'м' => "m",
                'н' => "n",
                'њ' => "nj",
                'о' => "o",
                'п' => "p",
                'р' => "r",
                'с' => "s",
                'т' => "t",
                'ћ' | 'ц' | 'ч' | 'č' | 'ć' => "c",
                'у' => "u",
                'ф' => "f",
                'х' => "h",
                'џ' => "dz",
                'ш' | 'š' => "s",
                _ => "",
            },
            None => "", // a lone surrogate unit
        };
        if m.is_empty() {
            out.push(ch);
        } else {
            out.extend(m.bytes().map(|b| b as u16));
        }
    }
    *term = out;
}
