// Painless engine: java.util.regex Pattern / Matcher and the regex circuit
// breaker.
//
// In "limited" mode Painless wraps the input of every matcher and fails once
// the (backtracking) java.util.regex engine has read more than
// limit-factor * length characters. The regex crate never backtracks, so a
// small backtracking matcher replays the match only to count character
// reads; the match result itself still comes from the regex crate.

use std::rc::Rc;

use regex::{Regex, RegexBuilder};

use crate::pl_value::{RtErr, PV, R};

pub const LIMIT_FACTOR: usize = 6;

pub struct PPattern {
    pub src: String,
    pub flags: String,
    pub re: Regex,
    /// The pattern anchored at both ends, for Matcher.matches().
    pub full: Regex,
}

pub struct PMatcher {
    pub p: Rc<PPattern>,
    pub input: String,
    /// Byte spans of the groups of the last match.
    pub loc: Option<Vec<Option<(usize, usize)>>>,
    pub pos: usize,
}

fn build(expr: &str, flags: &str) -> Result<Regex, regex::Error> {
    let mut b = RegexBuilder::new(expr);
    b.case_insensitive(flags.contains('i'))
        .multi_line(flags.contains('m'))
        .dot_matches_new_line(flags.contains('s'))
        .ignore_whitespace(flags.contains('x'));
    b.build()
}

/// Compiles a regex literal or Pattern.compile argument.
pub fn compile_pattern(src: &str, flags: &str, off: usize) -> R<PV> {
    let expr = if flags.contains('l') { regex::escape(src) } else { src.to_string() };
    let re = build(&expr, flags);
    let full = build(&format!(r"\A(?:{})\z", expr), flags);
    match (re, full) {
        (Ok(re), Ok(full)) => Ok(PV::Pattern(Rc::new(PPattern {
            src: src.to_string(),
            flags: flags.to_string(),
            re,
            full,
        }))),
        (Err(e), _) | (_, Err(e)) => Err(RtErr::new("pattern_syntax_exception", &e.to_string(), off)),
    }
}

fn spans(re: &Regex, input: &str, at: usize) -> Option<Vec<Option<(usize, usize)>>> {
    let caps = re.captures_at(input, at)?;
    Some((0..caps.len()).map(|i| caps.get(i).map(|m| (m.start(), m.end()))).collect())
}

impl PMatcher {
    pub fn new(p: Rc<PPattern>, input: &str) -> Self {
        PMatcher { p, input: input.to_string(), loc: None, pos: 0 }
    }

    /// Enforces Painless' regex circuit breaker.
    fn check_budget(&self, off: usize, full: bool) -> R<()> {
        let chars: Vec<char> = self.input.chars().collect();
        let limit = LIMIT_FACTOR * chars.len();
        let pos = self.input[..self.pos.min(self.input.len())].chars().count();
        if !regex_exceeds(&self.p.src, &self.p.flags, &chars, full, pos, limit) {
            return Ok(());
        }
        Err(RtErr::new(
            "circuit_breaking_exception",
            &format!(
                "[scripting] Regular expression considered too many characters, pattern: [{}], limit factor: [{}], char limit: [{}], count: [{}], wrapped: [{}], this limit can be changed by changed by the [script.painless.regex.limit-factor] setting",
                self.p.src,
                LIMIT_FACTOR,
                limit,
                limit + 1,
                self.input
            ),
            off,
        ))
    }

    pub fn find(&mut self, off: usize) -> R<bool> {
        self.check_budget(off, false)?;
        if self.pos > self.input.len() {
            self.loc = None;
            return Ok(false);
        }
        match spans(&self.p.re, &self.input, self.pos) {
            None => {
                self.loc = None;
                Ok(false)
            }
            Some(loc) => {
                let (s, e) = loc[0].unwrap();
                self.pos = if s == e {
                    e + self.input[e..].chars().next().map(|c| c.len_utf8()).unwrap_or(1)
                } else {
                    e
                };
                self.loc = Some(loc);
                Ok(true)
            }
        }
    }

    pub fn matches(&mut self, off: usize) -> R<bool> {
        self.check_budget(off, true)?;
        self.loc = spans(&self.p.full, &self.input, 0);
        Ok(self.loc.is_some())
    }

    pub fn looking_at(&mut self) -> bool {
        match spans(&self.p.re, &self.input, 0) {
            Some(loc) if loc[0].map(|(s, _)| s) == Some(0) => {
                self.loc = Some(loc);
                true
            }
            _ => false,
        }
    }

    pub fn group(&self, i: usize, off: usize) -> R<PV> {
        let loc = match &self.loc {
            None => return Err(RtErr::new("illegal_state_exception", "No match found", off)),
            Some(l) => l,
        };
        match loc.get(i) {
            None => Err(RtErr::new("index_out_of_bounds_exception", &format!("No group {}", i), off)),
            Some(None) => Ok(PV::Null),
            Some(Some((s, e))) => Ok(PV::str(&self.input[*s..*e])),
        }
    }

    pub fn group_index(&self, name: &str) -> Option<usize> {
        self.p.re.capture_names().position(|n| n == Some(name))
    }
}

/// Converts a java.util.regex replacement string to the regex crate syntax.
pub fn java_replacement(r: &str) -> String {
    let b = r.as_bytes();
    let mut out = String::new();
    let mut i = 0;
    while i < b.len() {
        let c = b[i];
        if c == b'\\' && i + 1 < b.len() {
            if b[i + 1] == b'$' {
                out.push_str("$$");
            } else {
                let ch = r[i + 1..].chars().next().unwrap();
                out.push(ch);
                i += ch.len_utf8() - 1;
            }
            i += 2;
            continue;
        }
        if c == b'$' {
            let mut j = i + 1;
            while j < b.len() && b[j].is_ascii_digit() {
                j += 1;
            }
            if j > i + 1 {
                out.push_str(&format!("${{{}}}", &r[i + 1..j]));
                i = j;
                continue;
            }
            if j < b.len() && b[j] == b'{' {
                if let Some(k) = r[j..].find('}') {
                    out.push_str(&format!("${{{}}}", &r[j + 1..j + k]));
                    i = j + k + 1;
                    continue;
                }
            }
        }
        let ch = r[i..].chars().next().unwrap();
        out.push(ch);
        i += ch.len_utf8();
    }
    out
}

// ---- circuit breaker: a counting backtracking matcher ----

enum RxKind {
    Set(Box<dyn Fn(char) -> bool>),
    Group(Vec<Vec<RxNode>>),
    Begin,
    End,
    WordBoundary,
}

struct RxNode {
    kind: RxKind,
    min: usize,
    /// None = unbounded.
    max: Option<usize>,
    lazy: bool,
}

struct RxParser {
    src: Vec<char>,
    i: usize,
    icase: bool,
    bad: bool,
}

fn fold(icase: bool, f: Box<dyn Fn(char) -> bool>) -> Box<dyn Fn(char) -> bool> {
    if !icase {
        return f;
    }
    Box::new(move |r: char| {
        f(r) || r.to_lowercase().any(|c| f(c)) || r.to_uppercase().any(|c| f(c))
    })
}

fn is_word_ascii(r: char) -> bool {
    r == '_' || r.is_ascii_alphanumeric()
}

fn is_space_ascii(r: char) -> bool {
    matches!(r, ' ' | '\t' | '\n' | '\r' | '\x0c' | '\x0b')
}

fn class_escape(c: char) -> Option<Box<dyn Fn(char) -> bool>> {
    Some(match c {
        'd' => Box::new(|r: char| r.is_ascii_digit()),
        'D' => Box::new(|r: char| !r.is_ascii_digit()),
        'w' => Box::new(is_word_ascii),
        'W' => Box::new(|r: char| !is_word_ascii(r)),
        's' => Box::new(is_space_ascii),
        'S' => Box::new(|r: char| !is_space_ascii(r)),
        _ => return None,
    })
}

fn literal_escape(c: char) -> char {
    match c {
        't' => '\t',
        'n' => '\n',
        'r' => '\r',
        'f' => '\x0c',
        _ => c,
    }
}

impl RxParser {
    fn peek(&self) -> Option<char> {
        self.src.get(self.i).copied()
    }

    fn alternation(&mut self) -> Vec<RxNode> {
        let mut alts = vec![self.sequence()];
        while self.peek() == Some('|') {
            self.i += 1;
            alts.push(self.sequence());
        }
        if alts.len() == 1 {
            return alts.pop().unwrap();
        }
        vec![RxNode { kind: RxKind::Group(alts), min: 1, max: Some(1), lazy: false }]
    }

    fn sequence(&mut self) -> Vec<RxNode> {
        let mut seq = Vec::new();
        while let Some(c) = self.peek() {
            if c == '|' || c == ')' {
                break;
            }
            let n = match self.atom() {
                Some(n) => n,
                None => {
                    self.bad = true;
                    return Vec::new();
                }
            };
            let n = self.quantifier(n);
            seq.push(n);
        }
        seq
    }

    fn quantifier(&mut self, mut n: RxNode) -> RxNode {
        n.min = 1;
        n.max = Some(1);
        match self.peek() {
            Some('*') => {
                n.min = 0;
                n.max = None;
                self.i += 1;
            }
            Some('+') => {
                n.min = 1;
                n.max = None;
                self.i += 1;
            }
            Some('?') => {
                n.min = 0;
                n.max = Some(1);
                self.i += 1;
            }
            Some('{') => {
                let rest: String = self.src[self.i..].iter().collect();
                let end = match rest.find('}') {
                    Some(e) => e,
                    None => return n,
                };
                let body = &rest[1..end];
                let (lo, hi) = match body.split_once(',') {
                    Some((l, h)) => (l, Some(h)),
                    None => (body, None),
                };
                let a: usize = match lo.parse() {
                    Ok(a) => a,
                    Err(_) => return n,
                };
                n.min = a;
                n.max = Some(a);
                if let Some(h) = hi {
                    n.max = None;
                    if !h.is_empty() {
                        match h.parse() {
                            Ok(b) => n.max = Some(b),
                            Err(_) => return n,
                        }
                    }
                }
                self.i += rest[..=end].chars().count();
            }
            _ => return n,
        }
        if self.peek() == Some('?') {
            n.lazy = true;
            self.i += 1;
        } else if self.peek() == Some('+') {
            self.i += 1; // possessive: treated as greedy
        }
        n
    }

    fn node(kind: RxKind) -> RxNode {
        RxNode { kind, min: 1, max: Some(1), lazy: false }
    }

    fn atom(&mut self) -> Option<RxNode> {
        let c = self.peek()?;
        self.i += 1;
        let icase = self.icase;
        match c {
            '.' => Some(Self::node(RxKind::Set(Box::new(|r: char| r != '\n')))),
            '^' => Some(Self::node(RxKind::Begin)),
            '$' => Some(Self::node(RxKind::End)),
            '(' => {
                if self.peek() == Some('?') {
                    // only non-capturing and named groups are modelled
                    let n1 = self.src.get(self.i + 1).copied();
                    let n2 = self.src.get(self.i + 2).copied();
                    if n1 == Some(':') {
                        self.i += 2;
                    } else if n1 == Some('<') && n2 != Some('=') && n2 != Some('!') {
                        let end = self.src[self.i..].iter().position(|&c| c == '>')?;
                        self.i += end + 1;
                    } else {
                        return None;
                    }
                }
                let inner = self.alternation();
                if self.bad || self.peek() != Some(')') {
                    return None;
                }
                self.i += 1;
                Some(Self::node(RxKind::Group(vec![inner])))
            }
            '[' => self.class(),
            '\\' => {
                let e = self.peek()?;
                self.i += 1;
                if e == 'b' {
                    return Some(Self::node(RxKind::WordBoundary));
                }
                if let Some(f) = class_escape(e) {
                    return Some(Self::node(RxKind::Set(f)));
                }
                if e.is_ascii_digit() && e != '0' || matches!(e, 'k' | 'p' | 'P' | 'Q') {
                    return None;
                }
                let lit = literal_escape(e);
                Some(Self::node(RxKind::Set(fold(icase, Box::new(move |r: char| r == lit)))))
            }
            _ => Some(Self::node(RxKind::Set(fold(icase, Box::new(move |r: char| r == c))))),
        }
    }

    fn class(&mut self) -> Option<RxNode> {
        let mut neg = false;
        if self.peek() == Some('^') {
            neg = true;
            self.i += 1;
        }
        let mut parts: Vec<Box<dyn Fn(char) -> bool>> = Vec::new();
        let mut first = true;
        while let Some(mut c) = self.peek() {
            if c == ']' && !first {
                break;
            }
            first = false;
            self.i += 1;
            if c == '\\' {
                if let Some(e) = self.peek() {
                    self.i += 1;
                    if let Some(f) = class_escape(e) {
                        parts.push(f);
                        continue;
                    }
                    c = literal_escape(e);
                }
            }
            if self.peek() == Some('-') && self.src.get(self.i + 1).map(|&x| x != ']').unwrap_or(false) {
                let hi = self.src[self.i + 1];
                self.i += 2;
                let lo = c;
                parts.push(Box::new(move |r: char| r >= lo && r <= hi));
                continue;
            }
            parts.push(Box::new(move |r: char| r == c));
        }
        self.peek()?;
        self.i += 1;
        let set: Box<dyn Fn(char) -> bool> = Box::new(move |r: char| parts.iter().any(|f| f(r)));
        let set = fold(self.icase, set);
        let set: Box<dyn Fn(char) -> bool> = if neg { Box::new(move |r: char| !set(r)) } else { set };
        Some(Self::node(RxKind::Set(set)))
    }
}

/// Replays a backtracking match and counts character reads.
struct RxCounter<'a> {
    input: &'a [char],
    reads: usize,
    limit: usize,
    aborted: bool,
}

type Cont<'c, 'a> = &'c mut dyn FnMut(&mut RxCounter<'a>, usize) -> bool;

impl<'a> RxCounter<'a> {
    fn read(&mut self, i: usize) -> Option<char> {
        self.reads += 1;
        if self.reads > self.limit {
            self.aborted = true;
            return None;
        }
        Some(self.input[i])
    }

    fn is_word(&mut self, i: isize) -> Option<bool> {
        if i < 0 || i as usize >= self.input.len() {
            return Some(false);
        }
        let r = self.read(i as usize)?;
        Some(r == '_' || r.is_alphanumeric())
    }

    /// Matches nodes[k..] at position i and then calls cont.
    fn seq(&mut self, nodes: &[RxNode], k: usize, i: usize, cont: Cont<'_, 'a>) -> bool {
        if self.aborted {
            return true;
        }
        if k == nodes.len() {
            return cont(self, i);
        }
        let n = &nodes[k];
        self.rep(n, 0, i, None, &mut |rx: &mut RxCounter<'a>, j: usize| rx.seq(nodes, k + 1, j, cont))
    }

    /// Matches n repeated (count so far) starting at i.
    fn rep(&mut self, n: &RxNode, count: usize, i: usize, last: Option<usize>, cont: Cont<'_, 'a>) -> bool {
        if self.aborted {
            return true;
        }
        let can_more = n.max.map(|m| count < m).unwrap_or(true);
        if count >= n.min && n.lazy && cont(self, i) {
            return true;
        }
        if can_more && !(count >= n.min && last == Some(i)) {
            let hit = self.once(n, i, &mut |rx: &mut RxCounter<'a>, j: usize| rx.rep(n, count + 1, j, Some(i), &mut *cont));
            if hit {
                return true;
            }
        }
        if count >= n.min && !n.lazy {
            return cont(self, i);
        }
        false
    }

    fn once(&mut self, n: &RxNode, i: usize, cont: Cont<'_, 'a>) -> bool {
        if self.aborted {
            return true;
        }
        match &n.kind {
            RxKind::Set(f) => {
                if i >= self.input.len() {
                    return false;
                }
                match self.read(i) {
                    None => true,
                    Some(r) => f(r) && cont(self, i + 1),
                }
            }
            RxKind::Begin => i == 0 && cont(self, i),
            RxKind::End => i == self.input.len() && cont(self, i),
            RxKind::WordBoundary => {
                let a = match self.is_word(i as isize - 1) {
                    Some(a) => a,
                    None => return true,
                };
                let b = match self.is_word(i as isize) {
                    Some(b) => b,
                    None => return true,
                };
                a != b && cont(self, i)
            }
            RxKind::Group(alts) => {
                for alt in alts {
                    if self.seq(alt, 0, i, &mut *cont) {
                        return true;
                    }
                }
                false
            }
        }
    }
}

/// Whether matching (or finding from pos) would read more than limit chars.
fn regex_exceeds(pattern: &str, flags: &str, input: &[char], full: bool, pos: usize, limit: usize) -> bool {
    let mut p = RxParser { src: pattern.chars().collect(), i: 0, icase: flags.contains('i'), bad: false };
    let nodes = p.alternation();
    if p.bad || p.i < p.src.len() {
        return false;
    }
    let mut c = RxCounter { input, reads: 0, limit, aborted: false };
    let len = input.len();
    if full {
        c.seq(&nodes, 0, 0, &mut |_: &mut RxCounter, j: usize| j == len);
        return c.aborted;
    }
    for start in pos..=len {
        if c.seq(&nodes, 0, start, &mut |_: &mut RxCounter, _| true) {
            break;
        }
    }
    c.aborted
}
