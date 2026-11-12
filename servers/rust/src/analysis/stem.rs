//! Stemmers behind the `stemmer`, `snowball`, `porter_stem`, `kstem` and
//! `*_stem` filters. Languages without an implementation here are accepted
//! and leave terms unchanged; language support plugs in at [`stemmer_for`].

use crate::errors::ApiError;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Stemmer {
    Porter,
    Porter2,
    MinimalEnglish,
    PossessiveEnglish,
    /// A known language whose algorithm is not implemented yet.
    Unimplemented,
    /// A Lucene language stemmer from `crate::lang`.
    Lang(crate::lang::chains::TermStem),
}

impl Stemmer {
    pub fn stem(&self, term: &str) -> String {
        match self {
            Stemmer::Porter => porter(term),
            Stemmer::Porter2 => porter2(term),
            Stemmer::MinimalEnglish => minimal_english(term),
            Stemmer::PossessiveEnglish => possessive_english(term),
            Stemmer::Unimplemented => term.to_string(),
            Stemmer::Lang(t) => t.stem(term),
        }
    }
}

/// Snowball stemmer class names Lucene ships (`org.tartarus.snowball.ext.*Stemmer`).
const SNOWBALL: &[&str] = &[
    "Arabic", "Armenian", "Basque", "Catalan", "Danish", "Dutch", "DutchPorter", "English", "Esperanto",
    "Estonian", "Finnish", "French", "German", "German2", "Greek", "Hindi", "Hungarian", "Indonesian",
    "Irish", "Italian", "Kp", "Lithuanian", "Nepali", "Norwegian", "Porter", "Portuguese", "Romanian",
    "Russian", "Serbian", "Spanish", "Swedish", "Tamil", "Turkish", "Yiddish",
];

fn invalid_class(lang: &str) -> ApiError {
    let mut cls: String = String::new();
    let mut chars = lang.chars();
    if let Some(f) = chars.next() {
        cls.extend(f.to_uppercase());
        cls.push_str(chars.as_str());
    }
    ApiError::bad(&format!("Invalid stemmer class specified: {}", cls)).caused_by(ApiError::new(
        "class_not_found_exception",
        &format!("org.tartarus.snowball.ext.{}Stemmer", cls),
        400,
    ))
}

/// The stemmer for a `stemmer` filter `language` (OpenSearch's
/// StemmerTokenFilterFactory names).
pub fn stemmer_for(language: &str) -> Result<Stemmer, ApiError> {
    if let Some(t) = crate::lang::chains::term_stemmer(language) {
        return Ok(Stemmer::Lang(t));
    }
    let l = language.to_lowercase();
    Ok(match l.as_str() {
        "english" | "porter" => Stemmer::Porter,
        "minimal_english" => Stemmer::MinimalEnglish,
        "possessive_english" => Stemmer::PossessiveEnglish,
        "lovins" => return Err(invalid_class(language)),
        "porter2" => Stemmer::Porter2,
        "light_english" | "kstem" => Stemmer::Unimplemented,
        "arabic" | "armenian" | "basque" | "bengali" | "brazilian" | "bulgarian" | "catalan" | "czech"
        | "danish" | "dutch" | "estonian" | "finnish" | "light_finnish" | "french"
        | "light_french" | "minimal_french" | "galician" | "minimal_galician" | "german" | "german2"
        | "light_german" | "minimal_german" | "greek" | "hindi" | "hungarian" | "light_hungarian"
        | "indonesian" | "irish" | "italian" | "light_italian" | "sorani" | "latvian" | "lithuanian"
        | "norwegian" | "light_norwegian" | "minimal_norwegian" | "light_nynorsk" | "minimal_nynorsk"
        | "portuguese" | "light_portuguese" | "minimal_portuguese" | "portuguese_rslp" | "romanian"
        | "russian" | "light_russian" | "serbian" | "spanish" | "light_spanish" | "spanish_plural"
        | "swedish" | "light_swedish" | "turkish" => Stemmer::Unimplemented,
        _ => {
            // Unknown names are looked up as Snowball classes.
            let mut cls = String::new();
            let mut chars = language.chars();
            if let Some(f) = chars.next() {
                cls.extend(f.to_uppercase());
                cls.push_str(chars.as_str());
            }
            if SNOWBALL.contains(&cls.as_str()) {
                if cls == "English" {
                    return Ok(Stemmer::Porter2);
                }
                if cls == "Porter" {
                    return Ok(Stemmer::Porter);
                }
                return Ok(lang_snowball(&cls));
            }
            return Err(invalid_class(language));
        }
    })
}

/// The stemmer of a `snowball` filter/analyzer `language` (a Snowball class name).
pub fn snowball_for(language: &str) -> Result<Stemmer, ApiError> {
    let mut cls = String::new();
    let mut chars = language.chars();
    if let Some(f) = chars.next() {
        cls.extend(f.to_uppercase());
        cls.push_str(chars.as_str());
    }
    if !SNOWBALL.contains(&cls.as_str()) {
        return Err(invalid_class(language));
    }
    Ok(match cls.as_str() {
        "English" => Stemmer::Porter2,
        "Porter" => Stemmer::Porter,
        other => lang_snowball(other),
    })
}

fn lang_snowball(cls: &str) -> Stemmer {
    // Snowball folded "German2" into "German".
    let cls = if cls == "German2" { "German" } else { cls };
    match crate::lang::chains::snowball_class(cls) {
        Some(c) => Stemmer::Lang(crate::lang::chains::TermStem::S(c)),
        None => Stemmer::Unimplemented,
    }
}

// ------------------------------------------- Snowball English (Porter2)

fn p2_vowel(c: char) -> bool {
    matches!(c, 'a' | 'e' | 'i' | 'o' | 'u' | 'y')
}

struct P2 {
    w: Vec<char>,
    p1: usize,
    p2: usize,
}

impl P2 {
    fn ends(&self, s: &str) -> bool {
        let sc: Vec<char> = s.chars().collect();
        self.w.len() >= sc.len() && self.w[self.w.len() - sc.len()..] == sc[..]
    }

    fn cut(&mut self, n: usize) {
        let l = self.w.len();
        self.w.truncate(l - n);
    }

    fn replace(&mut self, suf_len: usize, rep: &str) {
        self.cut(suf_len);
        self.w.extend(rep.chars());
    }

    fn short_syllable_at(&self, end: usize) -> bool {
        // The syllable ending at `end` (exclusive).
        let w = &self.w;
        if end == 2 {
            return p2_vowel(w[0]) && !p2_vowel(w[1]);
        }
        if end >= 3 {
            let (a, b, c) = (w[end - 3], w[end - 2], w[end - 1]);
            return !p2_vowel(a) && p2_vowel(b) && !p2_vowel(c) && !matches!(c, 'w' | 'x' | 'Y');
        }
        false
    }

    fn is_short(&self) -> bool {
        self.p1 >= self.w.len() && self.short_syllable_at(self.w.len())
    }

    fn has_vowel(&self, upto: usize) -> bool {
        self.w[..upto].iter().any(|c| p2_vowel(*c))
    }
}

fn region_after(w: &[char], from: usize) -> usize {
    let mut i = from;
    while i < w.len() && !p2_vowel(w[i]) {
        i += 1;
    }
    while i < w.len() && p2_vowel(w[i]) {
        i += 1;
    }
    (i + 1).min(w.len()).max(from.min(w.len()))
}

/// The Snowball English (Porter2) stemmer.
pub fn porter2(term: &str) -> String {
    let mut w: Vec<char> = term.chars().collect();
    if w.len() <= 2 {
        return term.to_string();
    }
    let exceptions: &[(&str, &str)] = &[
        ("skis", "ski"), ("skies", "sky"), ("dying", "die"), ("lying", "lie"), ("tying", "tie"),
        ("idly", "idl"), ("gently", "gentl"), ("ugly", "ugli"), ("early", "earli"), ("only", "onli"),
        ("singly", "singl"), ("sky", "sky"), ("news", "news"), ("howe", "howe"), ("atlas", "atlas"),
        ("cosmos", "cosmos"), ("bias", "bias"), ("andes", "andes"),
    ];
    if let Some((_, r)) = exceptions.iter().find(|(e, _)| *e == term) {
        return r.to_string();
    }
    if w[0] == '\'' {
        w.remove(0);
    }
    if w.first() == Some(&'y') {
        w[0] = 'Y';
    }
    for i in 1..w.len() {
        if w[i] == 'y' && p2_vowel(w[i - 1]) {
            w[i] = 'Y';
        }
    }
    let s: String = w.iter().collect();
    let p1 = if s.starts_with("gener") || s.starts_with("arsen") {
        5
    } else if s.starts_with("commun") {
        6
    } else {
        region_after(&w, 0)
    };
    let p2 = region_after(&w, p1);
    let mut st = P2 { w, p1, p2 };
    // Step 0
    for suf in ["'s'", "'s", "'"] {
        if st.ends(suf) {
            st.cut(suf.len());
            break;
        }
    }
    // Step 1a
    if st.ends("sses") {
        st.replace(4, "ss");
    } else if st.ends("ied") || st.ends("ies") {
        if st.w.len() > 4 {
            st.replace(3, "i");
        } else {
            st.replace(3, "ie");
        }
    } else if st.ends("us") || st.ends("ss") {
    } else if st.ends("s") {
        let l = st.w.len();
        if l >= 2 && st.has_vowel(l - 2) {
            st.cut(1);
        }
    }
    let s: String = st.w.iter().collect();
    if ["inning", "outing", "canning", "herring", "earring", "proceed", "exceed", "succeed"].contains(&s.as_str()) {
        return s;
    }
    // Step 1b
    let l = st.w.len();
    if st.ends("eedly") || st.ends("eed") {
        let n = if st.ends("eedly") { 5 } else { 3 };
        if l - n >= st.p1 {
            st.replace(n, "ee");
        }
    } else {
        let mut found = 0;
        for suf in ["ingly", "edly", "ing", "ed"] {
            if st.ends(suf) {
                found = suf.len();
                break;
            }
        }
        if found > 0 && st.has_vowel(l - found) {
            st.cut(found);
            if st.ends("at") || st.ends("bl") || st.ends("iz") {
                st.w.push('e');
            } else if ["bb", "dd", "ff", "gg", "mm", "nn", "pp", "rr", "tt"].iter().any(|d| st.ends(d)) {
                st.cut(1);
            } else if st.is_short() {
                st.w.push('e');
            }
        }
    }
    // Step 1c
    let l = st.w.len();
    if l > 2 && (st.w[l - 1] == 'y' || st.w[l - 1] == 'Y') && !p2_vowel(st.w[l - 2]) {
        st.w[l - 1] = 'i';
    }
    // Step 2
    let step2: &[(&str, &str)] = &[
        ("ization", "ize"), ("ational", "ate"), ("fulness", "ful"), ("ousness", "ous"), ("iveness", "ive"),
        ("tional", "tion"), ("biliti", "ble"), ("lessli", "less"), ("entli", "ent"), ("ation", "ate"),
        ("alism", "al"), ("aliti", "al"), ("ousli", "ous"), ("iviti", "ive"), ("fulli", "ful"),
        ("enci", "ence"), ("anci", "ance"), ("abli", "able"), ("izer", "ize"), ("ator", "ate"),
        ("alli", "al"), ("bli", "ble"), ("ogi", "og"), ("li", ""),
    ];
    let l = st.w.len();
    for (suf, rep) in step2 {
        if st.ends(suf) {
            let n = suf.chars().count();
            if l - n >= st.p1 {
                if *suf == "ogi" {
                    if l > 3 && st.w[l - 4] == 'l' {
                        st.replace(n, rep);
                    }
                } else if *suf == "li" {
                    if l > 2 && "cdeghkmnrt".contains(st.w[l - 3]) {
                        st.cut(2);
                    }
                } else {
                    st.replace(n, rep);
                }
            }
            break;
        }
    }
    // Step 3
    let step3: &[(&str, &str)] = &[
        ("ational", "ate"), ("tional", "tion"), ("alize", "al"), ("icate", "ic"), ("iciti", "ic"),
        ("ative", ""), ("ical", "ic"), ("ness", ""), ("ful", ""),
    ];
    let l = st.w.len();
    for (suf, rep) in step3 {
        if st.ends(suf) {
            let n = suf.chars().count();
            if l - n >= st.p1 {
                if *suf == "ative" {
                    if l - n >= st.p2 {
                        st.cut(n);
                    }
                } else {
                    st.replace(n, rep);
                }
            }
            break;
        }
    }
    // Step 4
    let step4 = [
        "ement", "ance", "ence", "able", "ible", "ment", "ant", "ent", "ism", "ate", "iti", "ous", "ive",
        "ize", "ion", "al", "er", "ic",
    ];
    let l = st.w.len();
    for suf in step4 {
        if st.ends(suf) {
            let n = suf.len();
            if l - n >= st.p2 {
                if suf == "ion" {
                    if l > 3 && (st.w[l - 4] == 's' || st.w[l - 4] == 't') {
                        st.cut(3);
                    }
                } else {
                    st.cut(n);
                }
            }
            break;
        }
    }
    // Step 5
    let l = st.w.len();
    if st.ends("e") {
        if l - 1 >= st.p2 || (l - 1 >= st.p1 && !st.short_syllable_at(l - 1)) {
            st.cut(1);
        }
    } else if st.ends("l") && l - 1 >= st.p2 && l >= 2 && st.w[l - 2] == 'l' {
        st.cut(1);
    }
    st.w.iter().map(|c| if *c == 'Y' { 'y' } else { *c }).collect()
}

// ----------------------------------------------------------- English

fn possessive_english(t: &str) -> String {
    for suf in ["'s", "\u{2019}s", "\u{FF07}s", "'S", "\u{2019}S", "\u{FF07}S"] {
        if let Some(b) = t.strip_suffix(suf) {
            return b.to_string();
        }
    }
    t.to_string()
}

fn minimal_english(t: &str) -> String {
    let s: Vec<char> = t.chars().collect();
    let len = s.len();
    if len < 3 || s[len - 1] != 's' {
        return t.to_string();
    }
    match s[len - 2] {
        'u' | 's' => t.to_string(),
        'e' => {
            if len > 3 && s[len - 3] == 'i' && s[len - 4] != 'a' && s[len - 4] != 'e' {
                let mut o: String = s[..len - 3].iter().collect();
                o.push('y');
                return o;
            }
            if matches!(s[len - 3], 'i' | 'a' | 'o' | 'e') {
                return t.to_string();
            }
            s[..len - 1].iter().collect()
        }
        _ => s[..len - 1].iter().collect(),
    }
}

// ------------------------------------------------- Porter (Lucene PorterStemmer)

struct P {
    b: Vec<char>,
    k: usize,
    j: usize,
}

impl P {
    fn cons(&self, i: usize) -> bool {
        match self.b[i] {
            'a' | 'e' | 'i' | 'o' | 'u' => false,
            'y' => i == 0 || !self.cons(i - 1),
            _ => true,
        }
    }

    /// Number of consonant sequences between 0 and j.
    fn m(&self) -> usize {
        let mut n = 0;
        let mut i = 0;
        let j = self.j;
        loop {
            if i > j {
                return n;
            }
            if !self.cons(i) {
                break;
            }
            i += 1;
        }
        i += 1;
        loop {
            loop {
                if i > j {
                    return n;
                }
                if self.cons(i) {
                    break;
                }
                i += 1;
            }
            i += 1;
            n += 1;
            loop {
                if i > j {
                    return n;
                }
                if !self.cons(i) {
                    break;
                }
                i += 1;
            }
            i += 1;
        }
    }

    fn vowel_in_stem(&self) -> bool {
        (0..=self.j).any(|i| !self.cons(i))
    }

    fn doublec(&self, j: usize) -> bool {
        j >= 1 && self.b[j] == self.b[j - 1] && self.cons(j)
    }

    fn cvc(&self, i: usize) -> bool {
        if i < 2 || !self.cons(i) || self.cons(i - 1) || !self.cons(i - 2) {
            return false;
        }
        !matches!(self.b[i], 'w' | 'x' | 'y')
    }

    fn ends(&mut self, s: &str) -> bool {
        let sc: Vec<char> = s.chars().collect();
        let l = sc.len();
        if l > self.k + 1 {
            return false;
        }
        let o = self.k + 1 - l;
        if self.b[o..=self.k] != sc[..] {
            return false;
        }
        self.j = if o == 0 { usize::MAX } else { o - 1 };
        true
    }

    fn jp1(&self) -> usize {
        self.j.wrapping_add(1)
    }

    fn setto(&mut self, s: &str) {
        let start = self.jp1();
        self.b.truncate(start);
        self.b.extend(s.chars());
        self.k = self.b.len() - 1;
    }

    fn m_ok(&self) -> usize {
        if self.j == usize::MAX {
            0
        } else {
            self.m()
        }
    }

    fn r(&mut self, s: &str) {
        if self.m_ok() > 0 {
            self.setto(s);
        }
    }

    fn step1(&mut self) {
        if self.b[self.k] == 's' {
            if self.ends("sses") {
                self.k -= 2;
            } else if self.ends("ies") {
                self.setto("i");
            } else if self.k >= 1 && self.b[self.k - 1] != 's' {
                self.k -= 1;
            }
            self.b.truncate(self.k + 1);
        }
        if self.ends("eed") {
            if self.m_ok() > 0 {
                self.k -= 1;
                self.b.truncate(self.k + 1);
            }
        } else if (self.ends("ed") || self.ends("ing")) && self.j != usize::MAX && self.vowel_in_stem() {
            self.k = self.j;
            self.b.truncate(self.k + 1);
            if self.ends("at") {
                self.setto("ate");
            } else if self.ends("bl") {
                self.setto("ble");
            } else if self.ends("iz") {
                self.setto("ize");
            } else if self.doublec(self.k) {
                let ch = self.b[self.k];
                if !matches!(ch, 'l' | 's' | 'z') {
                    self.k -= 1;
                    self.b.truncate(self.k + 1);
                }
            } else {
                self.j = self.k;
                if self.m() == 1 && self.cvc(self.k) {
                    self.setto_at_end("e");
                }
            }
        }
    }

    fn setto_at_end(&mut self, s: &str) {
        self.b.truncate(self.k + 1);
        self.b.extend(s.chars());
        self.k = self.b.len() - 1;
    }

    fn step2(&mut self) {
        if self.ends("y") && self.j != usize::MAX && self.vowel_in_stem() {
            self.b[self.k] = 'i';
        }
    }

    fn step3(&mut self) {
        if self.k == 0 {
            return;
        }
        let rules: &[(&str, &str)] = match self.b[self.k - 1] {
            'a' => &[("ational", "ate"), ("tional", "tion")],
            'c' => &[("enci", "ence"), ("anci", "ance")],
            'e' => &[("izer", "ize")],
            'l' => &[("bli", "ble"), ("alli", "al"), ("entli", "ent"), ("eli", "e"), ("ousli", "ous")],
            'o' => &[("ization", "ize"), ("ation", "ate"), ("ator", "ate")],
            's' => &[("alism", "al"), ("iveness", "ive"), ("fulness", "ful"), ("ousness", "ous")],
            't' => &[("aliti", "al"), ("iviti", "ive"), ("biliti", "ble")],
            'g' => &[("logi", "log")],
            _ => &[],
        };
        for (s, r) in rules {
            if self.ends(s) {
                self.r(r);
                return;
            }
        }
    }

    fn step4(&mut self) {
        let rules: &[(&str, &str)] = match self.b[self.k] {
            'e' => &[("icate", "ic"), ("ative", ""), ("alize", "al")],
            'i' => &[("iciti", "ic")],
            'l' => &[("ical", "ic"), ("ful", "")],
            's' => &[("ness", "")],
            _ => &[],
        };
        for (s, r) in rules {
            if self.ends(s) {
                self.r(r);
                return;
            }
        }
    }

    fn step5(&mut self) {
        if self.k == 0 {
            return;
        }
        let found = match self.b[self.k - 1] {
            'a' => self.ends("al"),
            'c' => self.ends("ance") || self.ends("ence"),
            'e' => self.ends("er"),
            'i' => self.ends("ic"),
            'l' => self.ends("able") || self.ends("ible"),
            'n' => self.ends("ant") || self.ends("ement") || self.ends("ment") || self.ends("ent"),
            'o' => {
                if self.ends("ion") && self.j != usize::MAX && (self.b[self.j] == 's' || self.b[self.j] == 't') {
                    true
                } else {
                    self.ends("ou")
                }
            }
            's' => self.ends("ism"),
            't' => self.ends("ate") || self.ends("iti"),
            'u' => self.ends("ous"),
            'v' => self.ends("ive"),
            'z' => self.ends("ize"),
            _ => false,
        };
        if found && self.m_ok() > 1 {
            self.k = self.j;
            self.b.truncate(self.k + 1);
        }
    }

    fn step6(&mut self) {
        self.j = self.k;
        if self.b[self.k] == 'e' {
            let a = self.m();
            if a > 1 || (a == 1 && !self.cvc(self.k - 1)) {
                self.k -= 1;
                self.b.truncate(self.k + 1);
            }
        }
        if self.b[self.k] == 'l' && self.doublec(self.k) {
            self.j = self.k;
            if self.m() > 1 {
                self.k -= 1;
                self.b.truncate(self.k + 1);
            }
        }
    }
}

/// The original Porter algorithm, as Lucene's PorterStemFilter applies it
/// (case-sensitive: only lowercase ASCII suffixes are recognized).
pub fn porter(term: &str) -> String {
    let b: Vec<char> = term.chars().collect();
    if b.len() <= 2 {
        return term.to_string();
    }
    let k = b.len() - 1;
    let mut p = P { b, k, j: 0 };
    p.step1();
    if p.k > 0 {
        p.step2();
        p.step3();
        p.step4();
        p.step5();
        p.step6();
    }
    p.b.truncate(p.k + 1);
    p.b.into_iter().collect()
}
