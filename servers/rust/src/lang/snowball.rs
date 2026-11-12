//! Port of Lucene's Snowball runtime (org.tartarus.snowball.SnowballProgram)
//! used by the stemmers in `snowball_gen.rs`. Works on UTF-16 code units,
//! exactly like the Java original.

pub struct Among<T: 'static> {
    pub s: &'static [u16],
    pub substring_i: i32,
    pub result: i32,
    pub method: Option<fn(&mut T) -> bool>,
}

#[derive(Default)]
pub struct Env {
    pub current: Vec<u16>,
    pub cursor: i32,
    pub length: i32,
    pub limit: i32,
    pub limit_backward: i32,
    pub bra: i32,
    pub ket: i32,
}

pub trait Stemmer {
    fn env(&mut self) -> &mut Env;
    fn run(&mut self) -> bool;
}

/// Stems one term (UTF-16 units) in place, like SnowballFilter does.
pub fn stem_units(st: &mut dyn Stemmer, term: &[u16]) -> Vec<u16> {
    st.env().set_current(term);
    st.run();
    let e = st.env();
    e.current[..e.length as usize].to_vec()
}

impl Env {
    pub fn set_current(&mut self, text: &[u16]) {
        self.current = text.to_vec();
        self.cursor = 0;
        self.length = text.len() as i32;
        self.limit = self.length;
        self.limit_backward = 0;
        self.bra = self.cursor;
        self.ket = self.limit;
    }

    #[inline]
    fn ch(&self, i: i32) -> i32 {
        self.current[i as usize] as i32
    }

    fn in_set(s: &[u16], ch: i32, min: i32) -> bool {
        let c = ch - min;
        (s[(c >> 3) as usize] as i32 & (1 << (c & 7))) != 0
    }

    pub fn in_grouping(&mut self, s: &[u16], min: i32, max: i32) -> bool {
        if self.cursor >= self.limit {
            return false;
        }
        let ch = self.ch(self.cursor);
        if ch > max || ch < min || !Self::in_set(s, ch, min) {
            return false;
        }
        self.cursor += 1;
        true
    }

    pub fn in_grouping_b(&mut self, s: &[u16], min: i32, max: i32) -> bool {
        if self.cursor <= self.limit_backward {
            return false;
        }
        let ch = self.ch(self.cursor - 1);
        if ch > max || ch < min || !Self::in_set(s, ch, min) {
            return false;
        }
        self.cursor -= 1;
        true
    }

    pub fn out_grouping(&mut self, s: &[u16], min: i32, max: i32) -> bool {
        if self.cursor >= self.limit {
            return false;
        }
        let ch = self.ch(self.cursor);
        if ch > max || ch < min || !Self::in_set(s, ch, min) {
            self.cursor += 1;
            return true;
        }
        false
    }

    pub fn out_grouping_b(&mut self, s: &[u16], min: i32, max: i32) -> bool {
        if self.cursor <= self.limit_backward {
            return false;
        }
        let ch = self.ch(self.cursor - 1);
        if ch > max || ch < min || !Self::in_set(s, ch, min) {
            self.cursor -= 1;
            return true;
        }
        false
    }

    pub fn eq_s(&mut self, s: &[u16]) -> bool {
        let n = s.len() as i32;
        if self.limit - self.cursor < n {
            return false;
        }
        for i in 0..n {
            if self.current[(self.cursor + i) as usize] != s[i as usize] {
                return false;
            }
        }
        self.cursor += n;
        true
    }

    pub fn eq_s_b(&mut self, s: &[u16]) -> bool {
        let n = s.len() as i32;
        if self.cursor - self.limit_backward < n {
            return false;
        }
        for i in 0..n {
            if self.current[(self.cursor - n + i) as usize] != s[i as usize] {
                return false;
            }
        }
        self.cursor -= n;
        true
    }

    pub fn replace_s(&mut self, c_bra: i32, c_ket: i32, s: &[u16]) -> i32 {
        let n = s.len() as i32;
        let adjustment = n - (c_ket - c_bra);
        let new_length = self.length + adjustment;
        if new_length as usize > self.current.len() {
            self.current.resize(new_length as usize, 0);
        }
        if adjustment != 0 && c_ket < self.length {
            self.current
                .copy_within(c_ket as usize..self.length as usize, (c_bra + n) as usize);
        }
        for (i, &u) in s.iter().enumerate() {
            self.current[c_bra as usize + i] = u;
        }
        self.length += adjustment;
        self.limit += adjustment;
        if self.cursor >= c_ket {
            self.cursor += adjustment;
        } else if self.cursor > c_bra {
            self.cursor = c_bra;
        }
        adjustment
    }

    pub fn slice_check(&self) {}

    pub fn slice_from(&mut self, s: &[u16]) {
        let (b, k) = (self.bra, self.ket);
        self.replace_s(b, k, s);
    }

    pub fn slice_del(&mut self) {
        self.slice_from(&[]);
    }

    pub fn insert(&mut self, c_bra: i32, c_ket: i32, s: &[u16]) {
        let adjustment = self.replace_s(c_bra, c_ket, s);
        if c_bra <= self.bra {
            self.bra += adjustment;
        }
        if c_bra <= self.ket {
            self.ket += adjustment;
        }
    }

    pub fn slice_to(&self) -> Vec<u16> {
        self.current[self.bra as usize..self.ket as usize].to_vec()
    }
}

pub fn find_among<T: Stemmer>(t: &mut T, v: &[Among<T>]) -> i32 {
    let mut i: i32 = 0;
    let mut j: i32 = v.len() as i32;
    let c = t.env().cursor;
    let l = t.env().limit;
    let mut common_i = 0i32;
    let mut common_j = 0i32;
    let mut first_key_inspected = false;
    loop {
        let k = i + ((j - i) >> 1);
        let mut diff: i32 = 0;
        let mut common = common_i.min(common_j);
        let w = &v[k as usize];
        let e = t.env();
        let mut i2 = common;
        while i2 < w.s.len() as i32 {
            if c + common == l {
                diff = -1;
                break;
            }
            diff = e.current[(c + common) as usize] as i32 - w.s[i2 as usize] as i32;
            if diff != 0 {
                break;
            }
            common += 1;
            i2 += 1;
        }
        if diff < 0 {
            j = k;
            common_j = common;
        } else {
            i = k;
            common_i = common;
        }
        if j - i <= 1 {
            if i > 0 || j == i || first_key_inspected {
                break;
            }
            first_key_inspected = true;
        }
    }
    loop {
        let w = &v[i as usize];
        let n = w.s.len() as i32;
        if common_i >= n {
            t.env().cursor = c + n;
            match w.method {
                None => return w.result,
                Some(m) => {
                    let res = m(t);
                    t.env().cursor = c + n;
                    if res {
                        return w.result;
                    }
                }
            }
        }
        i = w.substring_i;
        if i < 0 {
            return 0;
        }
    }
}

pub fn find_among_b<T: Stemmer>(t: &mut T, v: &[Among<T>]) -> i32 {
    let mut i: i32 = 0;
    let mut j: i32 = v.len() as i32;
    let c = t.env().cursor;
    let lb = t.env().limit_backward;
    let mut common_i = 0i32;
    let mut common_j = 0i32;
    let mut first_key_inspected = false;
    loop {
        let k = i + ((j - i) >> 1);
        let mut diff: i32 = 0;
        let mut common = common_i.min(common_j);
        let w = &v[k as usize];
        let e = t.env();
        let mut i2 = w.s.len() as i32 - 1 - common;
        while i2 >= 0 {
            if c - common == lb {
                diff = -1;
                break;
            }
            diff = e.current[(c - 1 - common) as usize] as i32 - w.s[i2 as usize] as i32;
            if diff != 0 {
                break;
            }
            common += 1;
            i2 -= 1;
        }
        if diff < 0 {
            j = k;
            common_j = common;
        } else {
            i = k;
            common_i = common;
        }
        if j - i <= 1 {
            if i > 0 || j == i || first_key_inspected {
                break;
            }
            first_key_inspected = true;
        }
    }
    loop {
        let w = &v[i as usize];
        let n = w.s.len() as i32;
        if common_i >= n {
            t.env().cursor = c - n;
            match w.method {
                None => return w.result,
                Some(m) => {
                    let res = m(t);
                    t.env().cursor = c - n;
                    if res {
                        return w.result;
                    }
                }
            }
        }
        i = w.substring_i;
        if i < 0 {
            return 0;
        }
    }
}
