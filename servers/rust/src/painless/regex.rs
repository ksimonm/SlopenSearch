// Painless engine: java.util.regex Pattern / Matcher and the regex circuit
// breaker.
//
// Patterns run on `crate::jregex`, a java.util.regex emulation: lookaround,
// backreferences, possessive quantifiers, named groups, the replacement
// syntax and PatternSyntaxException texts behave like Java. Positions are
// char indices internally and UTF-16 offsets in scripts.
//
// In "limited" mode Painless wraps the input of every matcher and fails once
// the backtracking java.util.regex engine has read more than
// limit-factor * length characters. A small backtracking matcher replays the
// match only to count character reads; on top of that a step cap stops
// patterns the replay cannot model (lookaround, backreferences).

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use crate::jregex::{self, Caps, Exceeded, JRegex, SyntaxError};
use crate::painless::interp::Interp;
use crate::painless::lex::CompileErr;
use crate::painless::value::{i64_of, jstr, RtErr, PV, R};

pub const LIMIT_FACTOR: usize = 6;

/// Matcher steps one call may take before the circuit breaker trips.
const STEP_CAP: usize = 5_000_000;

pub struct PPattern {
    pub src: String,
    pub flags: String,
    pub re: Rc<JRegex>,
}

pub struct PMatcher {
    pub p: Rc<PPattern>,
    pub input: String,
    chars: Vec<char>,
    /// Char spans of the groups of the last match.
    pub loc: Option<Caps>,
    /// Char index where the next find() starts.
    pos: usize,
    /// No search yet (or reset).
    first: bool,
}

/// The Pattern flags of the regex literal flags (ERegex); the bits are
/// `Pattern.flags()`.
pub fn java_flags(flags: &str) -> u32 {
    let mut f = 0;
    for c in flags.chars() {
        f |= match c {
            'c' => jregex::CANON_EQ,
            'i' => jregex::CASE_INSENSITIVE,
            'l' => jregex::LITERAL,
            'm' => jregex::MULTILINE,
            's' => jregex::DOTALL,
            'U' => jregex::UNICODE_CHARACTER_CLASS,
            'u' => jregex::UNICODE_CASE,
            'x' => jregex::COMMENTS,
            _ => 0,
        };
    }
    f
}

type Compiled = Result<Rc<JRegex>, SyntaxError>;

thread_local! {
    static CACHE: RefCell<HashMap<(String, String), Compiled>> = RefCell::new(HashMap::new());
}

fn compile_java(src: &str, flags: &str) -> Compiled {
    let key = (flags.to_string(), src.to_string());
    if let Some(c) = CACHE.with(|c| c.borrow().get(&key).cloned()) {
        return c;
    }
    let c = JRegex::new(src, java_flags(flags)).map(Rc::new);
    CACHE.with(|cache| {
        let mut cache = cache.borrow_mut();
        if cache.len() > 1000 {
            cache.clear();
        }
        cache.insert(key, c.clone());
    });
    c
}

/// The compile error of an invalid regex constant (`/a(/`) at `off`.
pub fn constant_error(src: &str, flags: &str, off: usize) -> Option<CompileErr> {
    let e = compile_java(src, flags).err()?;
    let at = if e.index >= 0 { off + 1 + e.index as usize } else { off };
    let mut ce = CompileErr::new(
        "illegal_argument_exception",
        &format!("invalid regular expression: could not compile regex constant [{}] with flags [{}]", src, flags),
        at,
    );
    ce.cause = Some(Box::new(CompileErr::new("pattern_syntax_exception", &e.message(), at)));
    Some(ce)
}

/// Compiles a regex literal.
pub fn compile_pattern(src: &str, flags: &str, off: usize) -> R<PV> {
    match compile_java(src, flags) {
        Ok(re) => Ok(PV::Pattern(Rc::new(PPattern { src: src.to_string(), flags: flags.to_string(), re }))),
        Err(e) => Err(RtErr::new("pattern_syntax_exception", &e.message(), off)),
    }
}

fn utf16_len(cs: &[char]) -> usize {
    cs.iter().map(|c| c.len_utf16()).sum()
}

/// The char index of a UTF-16 offset (None when out of range).
fn char_index(cs: &[char], u: usize) -> Option<usize> {
    let mut n = 0;
    for (i, c) in cs.iter().enumerate() {
        if n >= u {
            return Some(i);
        }
        n += c.len_utf16();
    }
    if n == u {
        Some(cs.len())
    } else {
        None
    }
}

fn matcher_err(typ: &str, reason: &str, off: usize, frames: &[&str]) -> RtErr {
    let mut e = RtErr::new(typ, reason, off);
    e.frames = frames.iter().map(|f| format!("java.base/java.util.regex.Matcher.{}", f)).collect();
    e
}

/// The Matcher frames of a replacement string error.
fn replacement_frames(reason: &str) -> &'static [&'static str] {
    if reason.starts_with("No group with name") {
        &["appendExpandedReplacement(Matcher.java:1103)"]
    } else if reason.starts_with("No group ") {
        &["checkGroup(Matcher.java:1845)", "start(Matcher.java:523)", "appendExpandedReplacement(Matcher.java:1134)"]
    } else if reason == "character to be escaped is missing" {
        &["appendExpandedReplacement(Matcher.java:1063)"]
    } else if reason == "Illegal group reference: group index is missing" {
        &["appendExpandedReplacement(Matcher.java:1073)"]
    } else if reason == "Illegal group reference" {
        &["appendExpandedReplacement(Matcher.java:1111)"]
    } else if reason == "named capturing group is missing trailing '}'" {
        &["appendExpandedReplacement(Matcher.java:1094)"]
    } else if reason.ends_with("starts with digit character") {
        &["appendExpandedReplacement(Matcher.java:1098)"]
    } else {
        &[]
    }
}

/// Calling an int-taking Matcher method with a String through def (the
/// whitelist has no String overloads).
fn wrong_method_type(ret: &str, off: usize) -> RtErr {
    RtErr::new(
        "wrong_method_type_exception",
        &format!("cannot convert MethodHandle(Matcher,int){} to (Object,String)Object", ret),
        off,
    )
}

impl PMatcher {
    pub fn new(p: Rc<PPattern>, input: &str) -> Self {
        PMatcher { p, input: input.to_string(), chars: input.chars().collect(), loc: None, pos: 0, first: true }
    }

    fn breaker_error(&self, off: usize) -> RtErr {
        let limit = LIMIT_FACTOR * self.chars.len();
        RtErr::new(
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
        )
    }

    /// Enforces Painless' regex circuit breaker.
    fn check_budget(&self, off: usize, full: bool, pos: usize) -> R<()> {
        let limit = LIMIT_FACTOR * self.chars.len();
        if !regex_exceeds(&self.p.src, &self.p.flags, &self.chars, full, pos, limit) {
            return Ok(());
        }
        Err(self.breaker_error(off))
    }

    fn set_match(&mut self, loc: Option<Caps>) -> bool {
        self.first = false;
        if let Some(l) = &loc {
            let (s, e) = l[0].unwrap();
            self.pos = if s == e { e + 1 } else { e };
        }
        self.loc = loc;
        self.loc.is_some()
    }

    fn exceeded<T>(&self, r: Result<T, Exceeded>, off: usize) -> R<T> {
        r.map_err(|_| self.breaker_error(off))
    }

    fn search(&mut self, from: usize, off: usize) -> R<bool> {
        self.check_budget(off, false, from)?;
        if from > self.chars.len() {
            self.loc = None;
            self.first = false;
            return Ok(false);
        }
        let loc = self.exceeded(self.p.re.find_from(&self.chars, from, STEP_CAP), off)?;
        Ok(self.set_match(loc))
    }

    pub fn find(&mut self, off: usize) -> R<bool> {
        let from = if self.first { 0 } else { self.pos };
        self.search(from, off)
    }

    pub fn matches(&mut self, off: usize) -> R<bool> {
        self.check_budget(off, true, 0)?;
        let loc = self.exceeded(self.p.re.match_start(&self.chars, true, STEP_CAP), off)?;
        Ok(self.set_match(loc))
    }

    fn looking_at(&mut self, off: usize) -> R<bool> {
        let loc = self.exceeded(self.p.re.match_start(&self.chars, false, STEP_CAP), off)?;
        Ok(self.set_match(loc))
    }

    /// Matcher.checkMatch / checkGroup; `site` are the frames of the public
    /// method.
    fn check_group(&self, g: i64, off: usize, site: &[&str]) -> R<()> {
        if self.loc.is_none() {
            let mut fs = vec!["checkMatch(Matcher.java:1850)"];
            fs.extend_from_slice(site);
            return Err(matcher_err("illegal_state_exception", "No match found", off, &fs));
        }
        if g < 0 || g as usize > self.p.re.groups {
            let mut fs = vec!["checkGroup(Matcher.java:1845)"];
            fs.extend_from_slice(site);
            return Err(matcher_err("index_out_of_bounds_exception", &format!("No group {}", g), off, &fs));
        }
        Ok(())
    }

    fn group(&self, g: i64, off: usize, site: &[&str]) -> R<PV> {
        self.check_group(g, off, site)?;
        Ok(match self.loc.as_ref().unwrap()[g as usize] {
            None => PV::Null,
            Some((s, e)) => PV::str(&self.chars[s..e].iter().collect::<String>()),
        })
    }

    fn offset(&self, g: i64, end: bool, off: usize, site: &str) -> R<PV> {
        self.check_group(g, off, &[site])?;
        Ok(match self.loc.as_ref().unwrap()[g as usize] {
            None => PV::Int(-1),
            Some((s, e)) => PV::Int(utf16_len(&self.chars[..if end { e } else { s }]) as i32),
        })
    }

    /// Matcher.replaceAll / replaceFirst(String).
    fn replace(&mut self, repl: &str, all: bool, off: usize) -> R<PV> {
        self.check_budget(off, false, 0)?;
        let r = self.exceeded(self.p.re.replace_java(&self.input, repl, all, STEP_CAP), off)?;
        self.loc = None;
        self.first = true;
        self.pos = 0;
        match r {
            Ok(s) => Ok(PV::str(&s)),
            Err((typ, reason)) => {
                let mut fs: Vec<&str> = replacement_frames(&reason).to_vec();
                fs.push("appendReplacement(Matcher.java:1041)");
                fs.push(if all { "replaceAll(Matcher.java:1227)" } else { "replaceFirst(Matcher.java:1444)" });
                Err(matcher_err(typ, &reason, off, &fs))
            }
        }
    }
}

/// The java.util.regex.Matcher methods.
pub fn matcher_method(m: &Rc<RefCell<PMatcher>>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    let int_arg = |i: usize| -> Option<i64> {
        match &args[i] {
            PV::Str(_) => None,
            v => Some(i64_of(v)),
        }
    };
    Some(match (name, n) {
        ("find", 0) => m.borrow_mut().find(off).map(PV::Bool),
        ("find", 1) => {
            let mut mm = m.borrow_mut();
            let u = int_arg(0).unwrap_or(0);
            let idx = if u < 0 { None } else { char_index(&mm.chars, u as usize) };
            let idx = match idx {
                Some(i) => i,
                None => {
                    return Some(Err(matcher_err(
                        "index_out_of_bounds_exception",
                        "Illegal start index",
                        off,
                        &["find(Matcher.java:810)"],
                    )))
                }
            };
            mm.loc = None;
            mm.first = false;
            mm.search(idx, off).map(PV::Bool)
        }
        ("matches", 0) => m.borrow_mut().matches(off).map(PV::Bool),
        ("lookingAt", 0) => m.borrow_mut().looking_at(off).map(PV::Bool),
        ("group", 0) => m.borrow().group(0, off, &["group(Matcher.java:685)", "group(Matcher.java:643)"]),
        ("group", 1) => {
            let g = match int_arg(0) {
                Some(g) => g,
                None => return Some(Err(wrong_method_type("String", off))),
            };
            let mm = m.borrow();
            let site = if mm.loc.is_none() { "group(Matcher.java:685)" } else { "group(Matcher.java:686)" };
            mm.group(g, off, &[site])
        }
        ("namedGroup", 1) => {
            let mm = m.borrow();
            let name = jstr(&args[0]);
            match mm.p.re.group_index(&name) {
                Some(g) => mm.group(g as i64, off, &["group(Matcher.java:720)"]),
                None => Err(matcher_err(
                    "illegal_argument_exception",
                    &format!("No group with name <{}>", name),
                    off,
                    &["getMatchedGroupIndex(Matcher.java:1839)", "group(Matcher.java:720)"],
                )),
            }
        }
        ("groupCount", 0) => Ok(PV::Int(m.borrow().p.re.groups as i32)),
        ("start", _) | ("end", _) if n <= 1 => {
            let g = if n == 1 {
                match int_arg(0) {
                    Some(g) => g,
                    None => return Some(Err(wrong_method_type("int", off))),
                }
            } else {
                0
            };
            let site = match (name, n) {
                ("end", 0) => "end(Matcher.java:557)",
                ("end", _) => "end(Matcher.java:591)",
                (_, 0) => "start(Matcher.java:493)",
                _ => "start(Matcher.java:523)",
            };
            m.borrow().offset(g, name == "end", off, site)
        }
        ("replaceAll", 1) | ("replaceFirst", 1) => {
            let repl = jstr(&args[0]);
            m.borrow_mut().replace(&repl, name == "replaceAll", off)
        }
        ("reset", 0) => {
            let mut mm = m.borrow_mut();
            mm.pos = 0;
            mm.loc = None;
            mm.first = true;
            drop(mm);
            Ok(PV::Matcher(m.clone()))
        }
        ("pattern", 0) => Ok(PV::Pattern(m.borrow().p.clone())),
        ("hitEnd", 0) => {
            let mm = m.borrow();
            Ok(PV::Bool(mm.pos >= mm.chars.len()))
        }
        ("regionStart", 0) => Ok(PV::Int(0)),
        ("regionEnd", 0) => Ok(PV::Int(utf16_len(&m.borrow().chars) as i32)),
        ("hasAnchoringBounds", 0) => Ok(PV::Bool(true)),
        ("hasTransparentBounds", 0) => Ok(PV::Bool(false)),
        _ => return None,
    })
}

/// The java.util.regex.Pattern methods.
pub fn pattern_method(p: &Rc<PPattern>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let _ = off;
    let n = args.len();
    Some(match (name, n) {
        ("matcher", 1) => Ok(PV::Matcher(Rc::new(RefCell::new(PMatcher::new(p.clone(), &jstr(&args[0])))))),
        ("pattern", 0) | ("toString", 0) => Ok(PV::str(&p.src)),
        ("flags", 0) => Ok(PV::Int(java_flags(&p.flags) as i32)),
        ("split", 1) | ("split", 2) => {
            let limit = if n == 2 { i64_of(&args[1]) } else { 0 };
            let parts = p.re.split(&jstr(&args[0]), limit);
            Ok(PV::array("String", parts.iter().map(|s| PV::str(s)).collect()))
        }
        ("splitAsStream", 1) => {
            let input = jstr(&args[0]);
            let mut parts = p.re.split(&input, 0);
            if parts.len() == 1 && parts[0].is_empty() {
                parts.clear();
            }
            Ok(PV::stream("obj", parts.iter().map(|s| PV::str(s)).collect()))
        }
        _ => return None,
    })
}

/// `String.replaceAll/replaceFirst(Pattern, Function)`: the function
/// receives the live matcher.
pub fn replace_with(inp: &Interp<'_>, p: &Rc<PPattern>, s: &str, f: &PV, first: bool, off: usize) -> R<PV> {
    let m = Rc::new(RefCell::new(PMatcher::new(p.clone(), s)));
    let mut out = String::new();
    let mut last = 0;
    loop {
        if !m.borrow_mut().find(off)? {
            break;
        }
        let (ms, me) = m.borrow().loc.as_ref().unwrap()[0].unwrap();
        let r = inp.call_fn(f, vec![PV::Matcher(m.clone())], off)?;
        let mm = m.borrow();
        out.extend(&mm.chars[last..ms]);
        out.push_str(&jstr(&r));
        last = me;
        if first {
            break;
        }
    }
    let mm = m.borrow();
    out.extend(&mm.chars[last..]);
    Ok(PV::str(&out))
}

/// `Pattern.asPredicate()`: whether the pattern occurs in the string.
pub fn find_in(p: &PPattern, s: &str) -> bool {
    let chars: Vec<char> = s.chars().collect();
    matches!(p.re.find_from(&chars, 0, STEP_CAP), Ok(Some(_)))
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
