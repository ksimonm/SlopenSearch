//! Port of Lucene's org.apache.lucene.analysis.en.PorterStemmer (Release 3 of
//! Martin Porter's original algorithm, Brian Goetz's version), as driven by
//! PorterStemFilter. This is Lucene's own implementation, not the Snowball
//! "porter" stemmer.
//!
//! Indices are `isize` because the Java code lets `j` reach -1 (see "Bug 2").

struct PorterStemmer {
    b: Vec<u16>,
    /// offset into b (length of the word)
    i: isize,
    j: isize,
    k: isize,
    k0: isize,
    dirty: bool,
}

const fn c(ch: char) -> u16 {
    ch as u16
}

impl PorterStemmer {
    fn at(&self, i: isize) -> u16 {
        self.b[i as usize]
    }

    /// cons(i) is true <=> b[i] is a consonant.
    fn cons(&self, i: isize) -> bool {
        match self.at(i) {
            0x61 | 0x65 | 0x69 | 0x6F | 0x75 => false, // a e i o u
            0x79 => {
                // y
                if i == self.k0 {
                    true
                } else {
                    !self.cons(i - 1)
                }
            }
            _ => true,
        }
    }

    /// m() measures the number of consonant sequences between k0 and j.
    fn m(&self) -> i32 {
        let mut n = 0;
        let mut i = self.k0;
        loop {
            if i > self.j {
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
                if i > self.j {
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
                if i > self.j {
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

    /// vowelinstem() is true <=> k0,...j contains a vowel
    fn vowelinstem(&self) -> bool {
        (self.k0..=self.j).any(|i| !self.cons(i))
    }

    /// doublec(j) is true <=> j,(j-1) contain a double consonant.
    fn doublec(&self, j: isize) -> bool {
        if j < self.k0 + 1 {
            return false;
        }
        if self.at(j) != self.at(j - 1) {
            return false;
        }
        self.cons(j)
    }

    /// cvc(i) is true <=> i-2,i-1,i has the form consonant - vowel - consonant
    /// and also if the second c is not w,x or y.
    fn cvc(&self, i: isize) -> bool {
        if i < self.k0 + 2 || !self.cons(i) || self.cons(i - 1) || !self.cons(i - 2) {
            return false;
        }
        let ch = self.at(i);
        !(ch == c('w') || ch == c('x') || ch == c('y'))
    }

    fn ends(&mut self, s: &str) -> bool {
        let s = s.as_bytes();
        let l = s.len() as isize;
        let o = self.k - l + 1;
        if o < self.k0 {
            return false;
        }
        for (i, &ch) in s.iter().enumerate() {
            if self.at(o + i as isize) != ch as u16 {
                return false;
            }
        }
        self.j = self.k - l;
        true
    }

    /// setto(s) sets (j+1),...k to the characters in the string s, readjusting k.
    fn setto(&mut self, s: &str) {
        let o = (self.j + 1) as usize;
        for (i, ch) in s.bytes().enumerate() {
            self.b[o + i] = ch as u16;
        }
        self.k = self.j + s.len() as isize;
        self.dirty = true;
    }

    fn r(&mut self, s: &str) {
        if self.m() > 0 {
            self.setto(s);
        }
    }

    /// step1() gets rid of plurals and -ed or -ing.
    fn step1(&mut self) {
        if self.at(self.k) == c('s') {
            if self.ends("sses") {
                self.k -= 2;
            } else if self.ends("ies") {
                self.setto("i");
            } else if self.at(self.k - 1) != c('s') {
                self.k -= 1;
            }
        }
        if self.ends("eed") {
            if self.m() > 0 {
                self.k -= 1;
            }
        } else if (self.ends("ed") || self.ends("ing")) && self.vowelinstem() {
            self.k = self.j;
            if self.ends("at") {
                self.setto("ate");
            } else if self.ends("bl") {
                self.setto("ble");
            } else if self.ends("iz") {
                self.setto("ize");
            } else if self.doublec(self.k) {
                let ch = self.at(self.k);
                self.k -= 1;
                if ch == c('l') || ch == c('s') || ch == c('z') {
                    self.k += 1;
                }
            } else if self.m() == 1 && self.cvc(self.k) {
                self.setto("e");
            }
        }
    }

    /// step2() turns terminal y to i when there is another vowel in the stem.
    fn step2(&mut self) {
        if self.ends("y") && self.vowelinstem() {
            let k = self.k as usize;
            self.b[k] = c('i');
            self.dirty = true;
        }
    }

    /// step3() maps double suffices to single ones (the Java switch with
    /// `break` after the first matching ending becomes an `if`/`else if` chain).
    fn step3(&mut self) {
        if self.k == self.k0 {
            return; // For Bug 1
        }
        let rules: &[(&str, &str)] = match self.at(self.k - 1) {
            0x61 => &[("ational", "ate"), ("tional", "tion")], // a
            0x63 => &[("enci", "ence"), ("anci", "ance")],      // c
            0x65 => &[("izer", "ize")],                         // e
            0x6C => &[
                // l
                ("bli", "ble"),
                ("alli", "al"),
                ("entli", "ent"),
                ("eli", "e"),
                ("ousli", "ous"),
            ],
            0x6F => &[("ization", "ize"), ("ation", "ate"), ("ator", "ate")], // o
            0x73 => &[
                // s
                ("alism", "al"),
                ("iveness", "ive"),
                ("fulness", "ful"),
                ("ousness", "ous"),
            ],
            0x74 => &[("aliti", "al"), ("iviti", "ive"), ("biliti", "ble")], // t
            0x67 => &[("logi", "log")],                                       // g
            _ => &[],
        };
        self.first_rule(rules);
    }

    /// Applies r(to) for the first rule whose ending matches.
    fn first_rule(&mut self, rules: &[(&str, &str)]) {
        for &(from, to) in rules {
            if self.ends(from) {
                self.r(to);
                return;
            }
        }
    }

    /// step4() deals with -ic-, -full, -ness etc. similar strategy to step3.
    fn step4(&mut self) {
        let rules: &[(&str, &str)] = match self.at(self.k) {
            0x65 => &[("icate", "ic"), ("ative", ""), ("alize", "al")], // e
            0x69 => &[("iciti", "ic")],                                  // i
            0x6C => &[("ical", "ic"), ("ful", "")],                      // l
            0x73 => &[("ness", "")],                                     // s
            _ => &[],
        };
        self.first_rule(rules);
    }

    /// step5() takes off -ant, -ence etc., in context <c>vcvc<v>.
    fn step5(&mut self) {
        if self.k == self.k0 {
            return; // for Bug 1
        }
        let found = match self.at(self.k - 1) {
            0x61 => self.ends("al"),                       // a
            0x63 => self.ends("ance") || self.ends("ence"), // c
            0x65 => self.ends("er"),                       // e
            0x69 => self.ends("ic"),                       // i
            0x6C => self.ends("able") || self.ends("ible"), // l
            // n; element etc. not stripped before the m
            0x6E => self.ends("ant") || self.ends("ement") || self.ends("ment") || self.ends("ent"),
            0x6F => {
                // o; j >= 0 fixes Bug 2; "ou" takes care of -ous
                (self.ends("ion") && self.j >= 0 && (self.at(self.j) == c('s') || self.at(self.j) == c('t')))
                    || self.ends("ou")
            }
            0x73 => self.ends("ism"),                     // s
            0x74 => self.ends("ate") || self.ends("iti"), // t
            0x75 => self.ends("ous"),                     // u
            0x76 => self.ends("ive"),                     // v
            0x7A => self.ends("ize"),                     // z
            _ => false,
        };
        if found && self.m() > 1 {
            self.k = self.j;
        }
    }

    /// step6() removes a final -e if m() > 1.
    fn step6(&mut self) {
        self.j = self.k;
        if self.at(self.k) == c('e') {
            let a = self.m();
            if a > 1 || a == 1 && !self.cvc(self.k - 1) {
                self.k -= 1;
            }
        }
        if self.at(self.k) == c('l') && self.doublec(self.k) && self.m() > 1 {
            self.k -= 1;
        }
    }

    /// stem(0): returns whether the word changed ("dirty").
    fn stem(&mut self) -> bool {
        self.k = self.i - 1;
        self.k0 = 0;
        if self.k > self.k0 + 1 {
            self.step1();
            self.step2();
            self.step3();
            self.step4();
            self.step5();
            self.step6();
        }
        // Also, a word is considered dirty if we lopped off letters
        if self.i != self.k + 1 {
            self.dirty = true;
        }
        self.i = self.k + 1;
        self.dirty
    }
}

/// PorterStemFilter's per-term transformation: the term is replaced by the
/// stemmer's result buffer when the stemmer reports a change.
pub fn porter_stem(term: &mut Vec<u16>) {
    let mut st = PorterStemmer { b: term.clone(), i: term.len() as isize, j: 0, k: 0, k0: 0, dirty: false };
    if st.stem() {
        st.b.truncate(st.i as usize);
        *term = st.b;
    }
}
