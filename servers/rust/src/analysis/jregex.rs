//! A small backtracking engine for `java.util.regex` patterns. Analysis
//! settings carry Java regexes (lookahead, lookbehind, possessive quantifiers,
//! POSIX classes such as `\p{Lower}`) that the `regex` crate does not accept,
//! and errors must read like `PatternSyntaxException` messages.
//!
//! Matching works on `&[char]`; positions are char indices.

use super::uchar;

pub const UNIX_LINES: u32 = 0x01;
pub const CASE_INSENSITIVE: u32 = 0x02;
pub const COMMENTS: u32 = 0x04;
pub const MULTILINE: u32 = 0x08;
pub const LITERAL: u32 = 0x10;
pub const DOTALL: u32 = 0x20;
pub const UNICODE_CASE: u32 = 0x40;
pub const CANON_EQ: u32 = 0x80;
pub const UNICODE_CHARACTER_CLASS: u32 = 0x100;

/// Parses OpenSearch's `flags` setting ("CASE_INSENSITIVE|COMMENTS").
pub fn parse_flags(s: &str) -> Result<u32, String> {
    let mut out = 0;
    for part in s.split('|') {
        let p = part.trim();
        if p.is_empty() {
            continue;
        }
        out |= match p {
            "UNIX_LINES" => UNIX_LINES,
            "CASE_INSENSITIVE" => CASE_INSENSITIVE,
            "COMMENTS" => COMMENTS,
            "MULTILINE" => MULTILINE,
            "LITERAL" => LITERAL,
            "DOTALL" => DOTALL,
            "UNICODE_CASE" => UNICODE_CASE,
            "CANON_EQ" => CANON_EQ,
            "UNICODE_CHARACTER_CLASS" => UNICODE_CHARACTER_CLASS,
            other => return Err(format!("Unknown regex flag [{}]", other)),
        };
    }
    Ok(out)
}

#[derive(Debug, Clone)]
enum Pred {
    Digit,
    Word,
    Space,
    HSpace,
    VSpace,
    Prop(PropKind),
}

#[derive(Debug, Clone, Copy)]
enum PropKind {
    Lower,
    Upper,
    Ascii,
    Alpha,
    PosixDigit,
    Alnum,
    Punct,
    Graph,
    Print,
    Blank,
    Cntrl,
    XDigit,
    PosixSpace,
    Letter,
    LetterUpper,
    LetterLower,
    LetterTitle,
    LetterOther,
    LetterModifier,
    Mark,
    Number,
    DecimalNumber,
    Punctuation,
    Symbol,
    Separator,
    Other,
    Alphabetic,
    Ideographic,
    WhiteSpace,
    Han,
    Hiragana,
    Katakana,
    Hangul,
    Latin,
    Greek,
    Cyrillic,
    Arabic,
    Hebrew,
    Thai,
    Any,
}

#[derive(Debug, Clone)]
enum ClassItem {
    Range(char, char),
    Pred(Pred, bool),
    Sub(Class),
}

#[derive(Debug, Clone)]
struct Class {
    neg: bool,
    items: Vec<ClassItem>,
    and: Vec<Class>,
}

#[derive(Debug, Clone)]
enum Node {
    Empty,
    Char(char, bool),
    Any(bool),
    Class(Box<Class>, bool, bool),
    Start(bool),
    End(bool),
    WordB(bool),
    InputStart,
    InputEnd(bool),
    Group(Option<usize>, Box<Node>),
    Look { behind: bool, neg: bool, node: Box<Node> },
    Repeat { node: Box<Node>, min: u32, max: u32, greedy: bool },
    Atomic(Box<Node>),
    Concat(Vec<Node>),
    Alt(Vec<Node>),
    Backref(usize, bool),
}

#[derive(Debug, Clone)]
pub struct JRegex {
    root: Node,
    pub groups: usize,
    #[allow(dead_code)]
    pub pattern: String,
}

/// A java.util.regex.PatternSyntaxException.
#[derive(Debug, Clone)]
pub struct SyntaxError {
    pub desc: String,
    pub index: i64,
    pub pattern: String,
}

impl SyntaxError {
    pub fn message(&self) -> String {
        let mut s = self.desc.clone();
        if self.index >= 0 {
            s.push_str(&format!(" near index {}", self.index));
        }
        s.push('\n');
        s.push_str(&self.pattern);
        let len = self.pattern.chars().count() as i64;
        if self.index >= 0 && self.index < len {
            s.push('\n');
            for _ in 0..self.index {
                s.push(' ');
            }
            s.push('^');
        }
        s
    }
}

struct Parser<'a> {
    p: &'a [char],
    i: usize,
    flags: u32,
    groups: usize,
    names: Vec<(String, usize)>,
    pattern: String,
}

impl<'a> Parser<'a> {
    fn err(&self, desc: &str, index: usize) -> SyntaxError {
        SyntaxError { desc: desc.to_string(), index: index as i64, pattern: self.pattern.clone() }
    }

    fn peek(&self) -> Option<char> {
        self.p.get(self.i).copied()
    }

    fn ci(&self) -> bool {
        self.flags & CASE_INSENSITIVE != 0
    }

    fn skip_comments(&mut self) {
        if self.flags & COMMENTS == 0 {
            return;
        }
        loop {
            match self.peek() {
                Some(c) if c.is_whitespace() => self.i += 1,
                Some('#') => {
                    while let Some(c) = self.peek() {
                        self.i += 1;
                        if c == '\n' {
                            break;
                        }
                    }
                }
                _ => break,
            }
        }
    }

    fn parse_alt(&mut self) -> Result<Node, SyntaxError> {
        let mut alts = vec![self.parse_concat()?];
        while self.peek() == Some('|') {
            self.i += 1;
            alts.push(self.parse_concat()?);
        }
        if alts.len() == 1 {
            Ok(alts.pop().unwrap())
        } else {
            Ok(Node::Alt(alts))
        }
    }

    fn parse_concat(&mut self) -> Result<Node, SyntaxError> {
        let mut items = Vec::new();
        loop {
            self.skip_comments();
            match self.peek() {
                None | Some('|') | Some(')') => break,
                _ => {}
            }
            let atom = match self.parse_atom()? {
                Some(a) => a,
                None => continue,
            };
            let atom = self.parse_quant(atom)?;
            items.push(atom);
        }
        if items.len() == 1 {
            Ok(items.pop().unwrap())
        } else if items.is_empty() {
            Ok(Node::Empty)
        } else {
            Ok(Node::Concat(items))
        }
    }

    fn parse_number(&mut self) -> Option<u32> {
        let start = self.i;
        while let Some(c) = self.peek() {
            if c.is_ascii_digit() {
                self.i += 1;
            } else {
                break;
            }
        }
        if self.i == start {
            return None;
        }
        self.p[start..self.i].iter().collect::<String>().parse().ok()
    }

    fn parse_quant(&mut self, atom: Node) -> Result<Node, SyntaxError> {
        let mut atom = atom;
        loop {
            self.skip_comments();
            let (min, max) = match self.peek() {
                Some('*') => {
                    self.i += 1;
                    (0, u32::MAX)
                }
                Some('+') => {
                    self.i += 1;
                    (1, u32::MAX)
                }
                Some('?') => {
                    self.i += 1;
                    (0, 1)
                }
                Some('{') => {
                    let at = self.i;
                    self.i += 1;
                    let min = match self.parse_number() {
                        Some(n) => n,
                        None => return Err(self.err("Illegal repetition", at)),
                    };
                    let max = if self.peek() == Some(',') {
                        self.i += 1;
                        self.parse_number().unwrap_or(u32::MAX)
                    } else {
                        min
                    };
                    if self.peek() != Some('}') {
                        return Err(self.err("Unclosed counted closure", self.i));
                    }
                    self.i += 1;
                    if max < min {
                        return Err(self.err("Illegal repetition range", at));
                    }
                    (min, max)
                }
                _ => return Ok(atom),
            };
            let mut greedy = true;
            let mut possessive = false;
            match self.peek() {
                Some('?') => {
                    self.i += 1;
                    greedy = false;
                }
                Some('+') => {
                    self.i += 1;
                    possessive = true;
                }
                _ => {}
            }
            let rep = Node::Repeat { node: Box::new(atom), min, max, greedy };
            atom = if possessive { Node::Atomic(Box::new(rep)) } else { rep };
        }
    }

    fn parse_group_flags(&mut self) -> Result<Option<u32>, SyntaxError> {
        // After "(?" : flags like "i-m" followed by ')' or ':'.
        let mut on = true;
        let mut flags = self.flags;
        loop {
            let c = match self.peek() {
                Some(c) => c,
                None => return Err(self.err("Unknown inline modifier", self.i)),
            };
            let f = match c {
                'i' => CASE_INSENSITIVE,
                'm' => MULTILINE,
                's' => DOTALL,
                'x' => COMMENTS,
                'u' => UNICODE_CASE,
                'd' => UNIX_LINES,
                'U' => UNICODE_CHARACTER_CLASS,
                '-' => {
                    on = false;
                    self.i += 1;
                    continue;
                }
                ')' | ':' => return Ok(Some(flags)),
                _ => return Err(self.err("Unknown inline modifier", self.i)),
            };
            if on {
                flags |= f;
            } else {
                flags &= !f;
            }
            self.i += 1;
        }
    }

    fn parse_atom(&mut self) -> Result<Option<Node>, SyntaxError> {
        let c = self.peek().unwrap();
        let start = self.i;
        match c {
            '(' => {
                self.i += 1;
                let mut capture = None;
                let mut look: Option<(bool, bool)> = None;
                let mut atomic = false;
                let saved_flags = self.flags;
                let mut restore_flags = true;
                if self.peek() == Some('?') {
                    self.i += 1;
                    match self.peek() {
                        Some(':') => self.i += 1,
                        Some('=') => {
                            self.i += 1;
                            look = Some((false, false));
                        }
                        Some('!') => {
                            self.i += 1;
                            look = Some((false, true));
                        }
                        Some('>') => {
                            self.i += 1;
                            atomic = true;
                        }
                        Some('<') => {
                            self.i += 1;
                            match self.peek() {
                                Some('=') => {
                                    self.i += 1;
                                    look = Some((true, false));
                                }
                                Some('!') => {
                                    self.i += 1;
                                    look = Some((true, true));
                                }
                                _ => {
                                    let ns = self.i;
                                    while let Some(ch) = self.peek() {
                                        if ch.is_ascii_alphanumeric() {
                                            self.i += 1;
                                        } else {
                                            break;
                                        }
                                    }
                                    if self.peek() != Some('>') || self.i == ns {
                                        return Err(self.err("named capturing group is missing trailing '>'", self.i));
                                    }
                                    let name: String = self.p[ns..self.i].iter().collect();
                                    self.i += 1;
                                    self.groups += 1;
                                    capture = Some(self.groups);
                                    self.names.push((name, self.groups));
                                }
                            }
                        }
                        _ => {
                            let f = self.parse_group_flags()?.unwrap();
                            if self.peek() == Some(')') {
                                // (?i) applies to the rest of the enclosing group.
                                self.i += 1;
                                self.flags = f;
                                return Ok(None);
                            }
                            self.i += 1; // ':'
                            self.flags = f;
                        }
                    }
                } else {
                    self.groups += 1;
                    capture = Some(self.groups);
                }
                let inner = self.parse_alt()?;
                if self.peek() != Some(')') {
                    return Err(self.err("Unclosed group", self.p.len()));
                }
                self.i += 1;
                if restore_flags {
                    self.flags = saved_flags;
                }
                restore_flags = false;
                let _ = restore_flags;
                let node = if let Some((behind, neg)) = look {
                    Node::Look { behind, neg, node: Box::new(inner) }
                } else if atomic {
                    Node::Atomic(Box::new(inner))
                } else {
                    Node::Group(capture, Box::new(inner))
                };
                Ok(Some(node))
            }
            '[' => {
                self.i += 1;
                let cls = self.parse_class(start)?;
                Ok(Some(Node::Class(Box::new(cls), self.ci(), false)))
            }
            '.' => {
                self.i += 1;
                Ok(Some(Node::Any(self.flags & DOTALL != 0)))
            }
            '^' => {
                self.i += 1;
                Ok(Some(Node::Start(self.flags & MULTILINE != 0)))
            }
            '$' => {
                self.i += 1;
                Ok(Some(Node::End(self.flags & MULTILINE != 0)))
            }
            '*' | '+' | '?' => Err(self.err(&format!("Dangling meta character '{}'", c), start)),
            '{' => Err(self.err("Illegal repetition", start)),
            '\\' => {
                self.i += 1;
                self.parse_escape(start).map(Some)
            }
            _ => {
                self.i += 1;
                Ok(Some(Node::Char(c, self.ci())))
            }
        }
    }

    fn parse_hex(&mut self, n: usize, at: usize) -> Result<char, SyntaxError> {
        let mut v = 0u32;
        for _ in 0..n {
            let d = self.peek().and_then(|c| c.to_digit(16));
            match d {
                Some(d) => {
                    v = v * 16 + d;
                    self.i += 1;
                }
                None => return Err(self.err("Illegal hexadecimal escape sequence", at)),
            }
        }
        char::from_u32(v).ok_or_else(|| self.err("Illegal hexadecimal escape sequence", at))
    }

    /// Parses the escape after a backslash: either a single char or a
    /// predefined class.
    fn parse_escape_item(&mut self, start: usize) -> Result<Result<char, (Pred, bool)>, SyntaxError> {
        let c = match self.peek() {
            Some(c) => c,
            None => return Err(self.err("Unexpected internal error", self.p.len())),
        };
        self.i += 1;
        let u = self.flags & UNICODE_CHARACTER_CLASS != 0;
        let _ = u;
        Ok(match c {
            'd' => Err((Pred::Digit, false)),
            'D' => Err((Pred::Digit, true)),
            'w' => Err((Pred::Word, false)),
            'W' => Err((Pred::Word, true)),
            's' => Err((Pred::Space, false)),
            'S' => Err((Pred::Space, true)),
            'h' => Err((Pred::HSpace, false)),
            'H' => Err((Pred::HSpace, true)),
            'v' => Err((Pred::VSpace, false)),
            'V' => Err((Pred::VSpace, true)),
            'p' | 'P' => {
                let neg = c == 'P';
                let name: String = if self.peek() == Some('{') {
                    self.i += 1;
                    let ns = self.i;
                    while let Some(ch) = self.peek() {
                        if ch == '}' {
                            break;
                        }
                        self.i += 1;
                    }
                    if self.peek() != Some('}') {
                        return Err(self.err("Unclosed character family", self.p.len()));
                    }
                    let n: String = self.p[ns..self.i].iter().collect();
                    self.i += 1;
                    n
                } else {
                    match self.peek() {
                        Some(ch) => {
                            self.i += 1;
                            ch.to_string()
                        }
                        None => return Err(self.err("Illegal character family", self.i)),
                    }
                };
                match prop_kind(&name) {
                    Some(k) => Err((Pred::Prop(k), neg)),
                    None => {
                        return Err(self.err(
                            &format!("Unknown character property name {{{}}}", name),
                            self.i,
                        ))
                    }
                }
            }
            't' => Ok('\t'),
            'n' => Ok('\n'),
            'r' => Ok('\r'),
            'f' => Ok('\u{0C}'),
            'a' => Ok('\u{07}'),
            'e' => Ok('\u{1B}'),
            '0' => {
                let mut v = 0u32;
                let mut n = 0;
                while n < 3 {
                    match self.peek().and_then(|c| c.to_digit(8)) {
                        Some(d) if v * 8 + d <= 0o377 => {
                            v = v * 8 + d;
                            self.i += 1;
                            n += 1;
                        }
                        _ => break,
                    }
                }
                if n == 0 {
                    return Err(self.err("Illegal octal escape sequence", self.i));
                }
                Ok(char::from_u32(v).unwrap())
            }
            'x' => {
                if self.peek() == Some('{') {
                    self.i += 1;
                    let mut v = 0u32;
                    while let Some(d) = self.peek().and_then(|c| c.to_digit(16)) {
                        v = v * 16 + d;
                        self.i += 1;
                    }
                    if self.peek() != Some('}') {
                        return Err(self.err("Unclosed hexadecimal escape sequence", self.i));
                    }
                    self.i += 1;
                    Ok(char::from_u32(v).ok_or_else(|| self.err("Hexadecimal codepoint is too big", self.i))?)
                } else {
                    Ok(self.parse_hex(2, start)?)
                }
            }
            'u' => Ok(self.parse_hex(4, start)?),
            'c' => match self.peek() {
                Some(ch) => {
                    self.i += 1;
                    Ok(char::from_u32((ch as u32) ^ 64).unwrap_or(ch))
                }
                None => return Err(self.err("Illegal control escape sequence", self.i)),
            },
            c if c.is_ascii_alphabetic() => {
                return Err(self.err("Illegal/unsupported escape sequence", self.i - 1));
            }
            c => Ok(c),
        })
    }

    fn parse_escape(&mut self, start: usize) -> Result<Node, SyntaxError> {
        let ci = self.ci();
        match self.peek() {
            Some('b') => {
                self.i += 1;
                return Ok(Node::WordB(true));
            }
            Some('B') => {
                self.i += 1;
                return Ok(Node::WordB(false));
            }
            Some('A') => {
                self.i += 1;
                return Ok(Node::InputStart);
            }
            Some('z') => {
                self.i += 1;
                return Ok(Node::InputEnd(true));
            }
            Some('Z') => {
                self.i += 1;
                return Ok(Node::InputEnd(false));
            }
            Some('G') => {
                self.i += 1;
                return Ok(Node::Empty);
            }
            Some('R') => {
                self.i += 1;
                return Ok(Node::Alt(vec![
                    Node::Concat(vec![Node::Char('\r', false), Node::Char('\n', false)]),
                    Node::Class(
                        Box::new(Class {
                            neg: false,
                            items: vec![
                                ClassItem::Range('\n', '\r'),
                                ClassItem::Range('\u{85}', '\u{85}'),
                                ClassItem::Range('\u{2028}', '\u{2029}'),
                            ],
                            and: vec![],
                        }),
                        false,
                        false,
                    ),
                ]));
            }
            Some('Q') => {
                self.i += 1;
                let mut items = Vec::new();
                while self.i < self.p.len() {
                    if self.p[self.i] == '\\' && self.p.get(self.i + 1) == Some(&'E') {
                        self.i += 2;
                        break;
                    }
                    items.push(Node::Char(self.p[self.i], ci));
                    self.i += 1;
                }
                return Ok(Node::Concat(items));
            }
            Some('k') => {
                self.i += 1;
                if self.peek() != Some('<') {
                    return Err(self.err("\\k is not followed by '<' for named capturing group", self.i));
                }
                self.i += 1;
                let ns = self.i;
                while let Some(ch) = self.peek() {
                    if ch == '>' {
                        break;
                    }
                    self.i += 1;
                }
                let name: String = self.p[ns..self.i].iter().collect();
                self.i += 1;
                return match self.names.iter().find(|(n, _)| *n == name) {
                    Some((_, g)) => Ok(Node::Backref(*g, ci)),
                    None => Err(self.err(&format!("named capturing group <{}> does not exist", name), self.i - 1)),
                };
            }
            Some(d) if d.is_ascii_digit() && d != '0' => {
                self.i += 1;
                let mut g = d.to_digit(10).unwrap() as usize;
                while let Some(n) = self.peek().and_then(|c| c.to_digit(10)) {
                    let ng = g * 10 + n as usize;
                    if ng > self.groups {
                        break;
                    }
                    g = ng;
                    self.i += 1;
                }
                return Ok(Node::Backref(g, ci));
            }
            _ => {}
        }
        match self.parse_escape_item(start)? {
            Ok(c) => Ok(Node::Char(c, ci)),
            Err((pred, neg)) => Ok(Node::Class(
                Box::new(Class { neg: false, items: vec![ClassItem::Pred(pred, neg)], and: vec![] }),
                ci,
                self.flags & UNICODE_CHARACTER_CLASS != 0,
            )),
        }
    }

    fn parse_class(&mut self, open: usize) -> Result<Class, SyntaxError> {
        let mut cls = Class { neg: false, items: Vec::new(), and: Vec::new() };
        if self.peek() == Some('^') {
            self.i += 1;
            cls.neg = true;
        }
        let mut first = true;
        loop {
            let c = match self.peek() {
                Some(c) => c,
                None => return Err(self.err("Unclosed character class", self.p.len() - 1)),
            };
            if c == ']' && !first {
                self.i += 1;
                break;
            }
            first = false;
            if c == '[' {
                self.i += 1;
                let sub = self.parse_class(self.i - 1)?;
                cls.items.push(ClassItem::Sub(sub));
                continue;
            }
            if c == '&' && self.p.get(self.i + 1) == Some(&'&') {
                self.i += 2;
                // Intersection with the rest of the class.
                let mut rest = Class { neg: false, items: Vec::new(), and: Vec::new() };
                if self.peek() == Some('[') {
                    self.i += 1;
                    rest = self.parse_class(self.i - 1)?;
                    // Continue parsing remaining items into the outer set.
                    cls.and.push(rest);
                    continue;
                }
                // Parse until ']' as an anonymous class.
                loop {
                    match self.peek() {
                        None => return Err(self.err("Unclosed character class", self.p.len() - 1)),
                        Some(']') => break,
                        _ => {}
                    }
                    let item = self.parse_class_item()?;
                    rest.items.push(item);
                }
                cls.and.push(rest);
                continue;
            }
            if self.flags & COMMENTS != 0 && c.is_whitespace() {
                self.i += 1;
                continue;
            }
            let item = self.parse_class_item()?;
            cls.items.push(item);
        }
        let _ = open;
        Ok(cls)
    }

    fn parse_class_char(&mut self) -> Result<Result<char, (Pred, bool)>, SyntaxError> {
        let c = self.peek().unwrap();
        self.i += 1;
        if c == '\\' {
            let start = self.i - 1;
            if self.peek() == Some('Q') {
                // Rare inside classes; treat the next char literally.
                self.i += 1;
            }
            return self.parse_escape_item(start);
        }
        Ok(Ok(c))
    }

    fn parse_class_item(&mut self) -> Result<ClassItem, SyntaxError> {
        let lo = self.parse_class_char()?;
        match lo {
            Err((p, neg)) => Ok(ClassItem::Pred(p, neg)),
            Ok(lo) => {
                if self.peek() == Some('-')
                    && self.p.get(self.i + 1).map(|c| *c != ']').unwrap_or(false)
                    && self.p.get(self.i + 1) != Some(&'[')
                {
                    let at = self.i;
                    self.i += 1;
                    match self.parse_class_char()? {
                        Ok(hi) => {
                            if hi < lo {
                                return Err(self.err("Illegal character range", self.i - 1));
                            }
                            Ok(ClassItem::Range(lo, hi))
                        }
                        Err(_) => Err(self.err("Illegal character range", at)),
                    }
                } else {
                    Ok(ClassItem::Range(lo, lo))
                }
            }
        }
    }
}

fn prop_kind(name: &str) -> Option<PropKind> {
    let n = name.strip_prefix("Is").unwrap_or(name);
    let n = n.strip_prefix("gc=").or_else(|| n.strip_prefix("general_category=")).unwrap_or(n);
    let n = n.strip_prefix("sc=").or_else(|| n.strip_prefix("script=")).unwrap_or(n);
    Some(match n {
        "Lower" => PropKind::Lower,
        "Upper" => PropKind::Upper,
        "ASCII" => PropKind::Ascii,
        "Alpha" => PropKind::Alpha,
        "Digit" => PropKind::PosixDigit,
        "Alnum" => PropKind::Alnum,
        "Punct" => PropKind::Punct,
        "Graph" => PropKind::Graph,
        "Print" => PropKind::Print,
        "Blank" => PropKind::Blank,
        "Cntrl" => PropKind::Cntrl,
        "XDigit" => PropKind::XDigit,
        "Space" => PropKind::PosixSpace,
        "L" | "Letter" | "javaLetter" => PropKind::Letter,
        "Lu" | "Uppercase_Letter" | "javaUpperCase" | "Uppercase" => PropKind::LetterUpper,
        "Ll" | "Lowercase_Letter" | "javaLowerCase" | "Lowercase" => PropKind::LetterLower,
        "Lt" | "Titlecase_Letter" | "TitleCase" | "javaTitleCase" => PropKind::LetterTitle,
        "Lo" | "Other_Letter" => PropKind::LetterOther,
        "Lm" | "Modifier_Letter" => PropKind::LetterModifier,
        "M" | "Mn" | "Mc" | "Me" | "Mark" => PropKind::Mark,
        "N" | "Number" => PropKind::Number,
        "Nd" | "javaDigit" => PropKind::DecimalNumber,
        "P" | "Punctuation" | "Pc" | "Pd" | "Ps" | "Pe" | "Pi" | "Pf" | "Po" => PropKind::Punctuation,
        "S" | "Symbol" | "Sm" | "Sc" | "Sk" | "So" => PropKind::Symbol,
        "Z" | "Zs" | "Zl" | "Zp" | "Separator" | "javaSpaceChar" => PropKind::Separator,
        "C" | "Cc" | "Cf" | "Co" | "Cn" | "Other" => PropKind::Other,
        "Alphabetic" | "javaAlphabetic" => PropKind::Alphabetic,
        "Ideographic" | "javaIdeographic" => PropKind::Ideographic,
        "White_Space" | "WhiteSpace" | "javaWhitespace" => PropKind::WhiteSpace,
        "Han" | "InCJKUnifiedIdeographs" | "InCJK_Unified_Ideographs" => PropKind::Han,
        "Hiragana" | "InHiragana" => PropKind::Hiragana,
        "Katakana" | "InKatakana" => PropKind::Katakana,
        "Hangul" | "InHangulSyllables" => PropKind::Hangul,
        "Latin" | "InBasicLatin" => PropKind::Latin,
        "Greek" | "InGreek" => PropKind::Greek,
        "Cyrillic" | "InCyrillic" => PropKind::Cyrillic,
        "Arabic" | "InArabic" => PropKind::Arabic,
        "Hebrew" | "InHebrew" => PropKind::Hebrew,
        "Thai" | "InThai" => PropKind::Thai,
        "all" => PropKind::Any,
        "javaLetterOrDigit" => PropKind::Alnum,
        _ => return None,
    })
}

fn prop_match(k: PropKind, c: char, unicode: bool) -> bool {
    let v = c as u32;
    match k {
        PropKind::Lower => {
            if unicode {
                c.is_lowercase()
            } else {
                c.is_ascii_lowercase()
            }
        }
        PropKind::Upper => {
            if unicode {
                c.is_uppercase()
            } else {
                c.is_ascii_uppercase()
            }
        }
        PropKind::Ascii => c.is_ascii(),
        PropKind::Alpha => {
            if unicode {
                c.is_alphabetic()
            } else {
                c.is_ascii_alphabetic()
            }
        }
        PropKind::PosixDigit => {
            if unicode {
                uchar::is_digit(c)
            } else {
                c.is_ascii_digit()
            }
        }
        PropKind::Alnum => {
            if unicode {
                c.is_alphabetic() || uchar::is_digit(c)
            } else {
                c.is_ascii_alphanumeric()
            }
        }
        PropKind::Punct => {
            if unicode {
                uchar::is_punct(c)
            } else {
                c.is_ascii_punctuation()
            }
        }
        PropKind::Graph => c.is_ascii_graphic(),
        PropKind::Print => c.is_ascii_graphic() || c == ' ',
        PropKind::Blank => c == ' ' || c == '\t',
        PropKind::Cntrl => v < 0x20 || v == 0x7F,
        PropKind::XDigit => c.is_ascii_hexdigit(),
        PropKind::PosixSpace => matches!(c, ' ' | '\t' | '\n' | '\u{0B}' | '\u{0C}' | '\r'),
        PropKind::Letter => uchar::is_letter(c),
        PropKind::LetterUpper => c.is_uppercase() && uchar::is_letter(c),
        PropKind::LetterLower => c.is_lowercase() && uchar::is_letter(c),
        PropKind::LetterTitle => matches!(v, 0x01C5 | 0x01C8 | 0x01CB | 0x01F2) || (0x1F88..=0x1FAF).contains(&v) && v % 16 >= 8,
        PropKind::LetterOther => uchar::is_letter(c) && !c.is_uppercase() && !c.is_lowercase(),
        PropKind::LetterModifier => (0x02B0..=0x02FF).contains(&v) && uchar::is_letter(c),
        PropKind::Mark => uchar::is_mark(c),
        PropKind::Number => c.is_numeric(),
        PropKind::DecimalNumber => uchar::is_digit(c),
        PropKind::Punctuation => uchar::is_punct(c),
        PropKind::Symbol => uchar::is_symbol(c),
        PropKind::Separator => uchar::is_space_separator(c) || v == 0x2028 || v == 0x2029,
        PropKind::Other => v < 0x20 || (0x7F..=0x9F).contains(&v) || uchar::is_format(c),
        PropKind::Alphabetic => c.is_alphabetic(),
        PropKind::Ideographic => uchar::is_han(c),
        PropKind::WhiteSpace => c.is_whitespace(),
        PropKind::Han => uchar::is_han(c),
        PropKind::Hiragana => uchar::is_hiragana(c),
        PropKind::Katakana => uchar::is_katakana(c),
        PropKind::Hangul => uchar::is_hangul(c),
        PropKind::Latin => {
            c.is_ascii_alphabetic() || (0xC0..=0x24F).contains(&v) && v != 0xD7 && v != 0xF7 || (0x1E00..=0x1EFF).contains(&v)
        }
        PropKind::Greek => (0x0370..=0x03FF).contains(&v) || (0x1F00..=0x1FFF).contains(&v),
        PropKind::Cyrillic => (0x0400..=0x052F).contains(&v),
        PropKind::Arabic => (0x0600..=0x06FF).contains(&v) || (0x0750..=0x077F).contains(&v),
        PropKind::Hebrew => (0x0590..=0x05FF).contains(&v),
        PropKind::Thai => (0x0E00..=0x0E7F).contains(&v),
        PropKind::Any => true,
    }
}

fn pred_match(p: &Pred, c: char, unicode: bool) -> bool {
    match p {
        Pred::Digit => {
            if unicode {
                uchar::is_digit(c)
            } else {
                c.is_ascii_digit()
            }
        }
        Pred::Word => {
            if unicode {
                c.is_alphabetic() || uchar::is_digit(c) || uchar::is_mark(c) || c == '_'
            } else {
                c.is_ascii_alphanumeric() || c == '_'
            }
        }
        Pred::Space => {
            if unicode {
                c.is_whitespace()
            } else {
                matches!(c, ' ' | '\t' | '\n' | '\u{0B}' | '\u{0C}' | '\r')
            }
        }
        Pred::HSpace => matches!(c, ' ' | '\t' | '\u{A0}' | '\u{1680}' | '\u{180E}' | '\u{2000}'..='\u{200A}' | '\u{202F}' | '\u{205F}' | '\u{3000}'),
        Pred::VSpace => matches!(c, '\n' | '\u{0B}' | '\u{0C}' | '\r' | '\u{85}' | '\u{2028}' | '\u{2029}'),
        Pred::Prop(k) => prop_match(*k, c, unicode),
    }
}

fn class_match(cls: &Class, c: char, ci: bool, unicode: bool) -> bool {
    let mut m = class_items_match(cls, c, unicode);
    if !m && ci {
        let l = uchar::lower_char(c);
        let u = uchar::upper_char(c);
        m = (l != c && class_items_match(cls, l, unicode)) || (u != c && class_items_match(cls, u, unicode));
    }
    m != cls.neg
}

fn class_items_match(cls: &Class, c: char, unicode: bool) -> bool {
    let mut m = cls.items.iter().any(|it| match it {
        ClassItem::Range(a, b) => *a <= c && c <= *b,
        ClassItem::Pred(p, neg) => pred_match(p, c, unicode) != *neg,
        ClassItem::Sub(sub) => class_match(sub, c, false, unicode),
    });
    if cls.items.is_empty() && !cls.and.is_empty() {
        m = true;
    }
    if m {
        for a in &cls.and {
            if !class_match(a, c, false, unicode) {
                return false;
            }
        }
    }
    m
}

fn chars_eq(a: char, b: char, ci: bool) -> bool {
    a == b || (ci && (uchar::lower_char(a) == uchar::lower_char(b) || uchar::upper_char(a) == uchar::upper_char(b)))
}

fn is_line_term(c: char) -> bool {
    matches!(c, '\n' | '\r' | '\u{85}' | '\u{2028}' | '\u{2029}')
}

fn is_word_char(c: char) -> bool {
    uchar::is_letter_or_digit(c) || c == '_' || uchar::is_mark(c)
}

type Caps = Vec<Option<(usize, usize)>>;

struct Matcher<'a> {
    text: &'a [char],
    caps: Caps,
    steps: usize,
}

const STEP_LIMIT: usize = 5_000_000;

impl<'a> Matcher<'a> {
    fn m(&mut self, n: &Node, i: usize, k: &mut dyn FnMut(&mut Matcher<'a>, usize) -> bool) -> bool {
        self.steps += 1;
        if self.steps > STEP_LIMIT {
            return false;
        }
        let t = self.text;
        match n {
            Node::Empty => k(self, i),
            Node::Char(c, ci) => i < t.len() && chars_eq(t[i], *c, *ci) && k(self, i + 1),
            Node::Any(dotall) => i < t.len() && (*dotall || !is_line_term(t[i])) && k(self, i + 1),
            Node::Class(cls, ci, uni) => i < t.len() && class_match(cls, t[i], *ci, *uni) && k(self, i + 1),
            Node::Start(multi) => {
                let ok = i == 0 || (*multi && is_line_term(t[i - 1]) && i < t.len());
                ok && k(self, i)
            }
            Node::End(multi) => {
                let ok = if *multi {
                    i == t.len() || is_line_term(t[i])
                } else {
                    i == t.len() || (i + 1 == t.len() && is_line_term(t[i]))
                };
                ok && k(self, i)
            }
            Node::WordB(want) => {
                let before = i > 0 && is_word_char(t[i - 1]);
                let after = i < t.len() && is_word_char(t[i]);
                ((before != after) == *want) && k(self, i)
            }
            Node::InputStart => i == 0 && k(self, i),
            Node::InputEnd(strict) => {
                let ok = i == t.len() || (!*strict && i + 1 == t.len() && is_line_term(t[i]));
                ok && k(self, i)
            }
            Node::Group(g, inner) => match g {
                None => self.m(inner, i, k),
                Some(g) => {
                    let g = *g;
                    self.m(inner, i, &mut |s: &mut Matcher<'a>, j| {
                        let old = s.caps[g];
                        s.caps[g] = Some((i, j));
                        if k(s, j) {
                            true
                        } else {
                            s.caps[g] = old;
                            false
                        }
                    })
                }
            },
            Node::Look { behind, neg, node } => {
                let saved = self.caps.clone();
                let found = if *behind {
                    let mut ok = false;
                    for s in (0..=i).rev() {
                        if self.m(node, s, &mut |_, j| j == i) {
                            ok = true;
                            break;
                        }
                    }
                    ok
                } else {
                    self.m(node, i, &mut |_, _| true)
                };
                if *neg {
                    self.caps = saved;
                    if found {
                        false
                    } else {
                        k(self, i)
                    }
                } else if found {
                    if k(self, i) {
                        true
                    } else {
                        self.caps = saved;
                        false
                    }
                } else {
                    self.caps = saved;
                    false
                }
            }
            Node::Atomic(inner) => {
                let saved = self.caps.clone();
                let mut end = None;
                if self.m(inner, i, &mut |_, j| {
                    end = Some(j);
                    true
                }) {
                    if k(self, end.unwrap()) {
                        return true;
                    }
                }
                self.caps = saved;
                false
            }
            Node::Concat(items) => self.seq(items, i, k),
            Node::Alt(alts) => {
                for a in alts {
                    if self.m(a, i, k) {
                        return true;
                    }
                }
                false
            }
            Node::Repeat { node, min, max, greedy } => self.rep(node, *min, *max, *greedy, 0, i, k),
            Node::Backref(g, ci) => {
                let (s, e) = match self.caps.get(*g).copied().flatten() {
                    Some(r) => r,
                    None => return false,
                };
                let len = e - s;
                if i + len > t.len() {
                    return false;
                }
                for x in 0..len {
                    if !chars_eq(t[s + x], t[i + x], *ci) {
                        return false;
                    }
                }
                k(self, i + len)
            }
        }
    }

    fn seq(&mut self, items: &[Node], i: usize, k: &mut dyn FnMut(&mut Matcher<'a>, usize) -> bool) -> bool {
        if items.is_empty() {
            return k(self, i);
        }
        let (first, rest) = items.split_first().unwrap();
        if rest.is_empty() {
            return self.m(first, i, k);
        }
        self.m(first, i, &mut |s: &mut Matcher<'a>, j| s.seq(rest, j, &mut *k))
    }

    #[allow(clippy::too_many_arguments)]
    fn rep(
        &mut self,
        node: &Node,
        min: u32,
        max: u32,
        greedy: bool,
        count: u32,
        i: usize,
        k: &mut dyn FnMut(&mut Matcher<'a>, usize) -> bool,
    ) -> bool {
        if greedy {
            if count < max {
                let ok = self.m(node, i, &mut |s: &mut Matcher<'a>, j| {
                    if j == i && count >= min {
                        return false;
                    }
                    s.rep(node, min, max, greedy, count + 1, j, &mut *k)
                });
                if ok {
                    return true;
                }
            }
            count >= min && k(self, i)
        } else {
            if count >= min && k(self, i) {
                return true;
            }
            count < max
                && self.m(node, i, &mut |s: &mut Matcher<'a>, j| {
                    if j == i && count >= min {
                        return false;
                    }
                    s.rep(node, min, max, greedy, count + 1, j, &mut *k)
                })
        }
    }
}

impl JRegex {
    pub fn new(pattern: &str, flags: u32) -> Result<JRegex, SyntaxError> {
        let chars: Vec<char> = pattern.chars().collect();
        if flags & LITERAL != 0 {
            let ci = flags & CASE_INSENSITIVE != 0;
            let items = chars.iter().map(|c| Node::Char(*c, ci)).collect();
            return Ok(JRegex { root: Node::Concat(items), groups: 0, pattern: pattern.to_string() });
        }
        let mut p = Parser { p: &chars, i: 0, flags, groups: 0, names: Vec::new(), pattern: pattern.to_string() };
        let root = p.parse_alt()?;
        if p.i < chars.len() {
            // A stray ')'.
            return Err(p.err("Unmatched closing ')'", p.i.saturating_sub(1)));
        }
        Ok(JRegex { root, groups: p.groups, pattern: pattern.to_string() })
    }

    /// Finds the first match starting at or after `start`. Returns the
    /// capture groups (group 0 is the whole match).
    pub fn find_at(&self, text: &[char], start: usize) -> Option<Caps> {
        let mut m = Matcher { text, caps: vec![None; self.groups + 1], steps: 0 };
        for s in start..=text.len() {
            for c in m.caps.iter_mut() {
                *c = None;
            }
            let mut end = None;
            if m.m(&self.root, s, &mut |_, j| {
                end = Some(j);
                true
            }) {
                m.caps[0] = Some((s, end.unwrap()));
                return Some(m.caps);
            }
            if m.steps > STEP_LIMIT {
                return None;
            }
        }
        None
    }

    /// Whole-input match (`Matcher.matches()`).
    pub fn matches(&self, text: &[char]) -> bool {
        let mut m = Matcher { text, caps: vec![None; self.groups + 1], steps: 0 };
        let n = text.len();
        m.m(&self.root, 0, &mut |_, j| j == n)
    }

    /// All successive matches with Java's `find()` semantics.
    pub fn find_all(&self, text: &[char]) -> Vec<Caps> {
        let mut out = Vec::new();
        let mut from = 0usize;
        while from <= text.len() {
            let caps = match self.find_at(text, from) {
                Some(c) => c,
                None => break,
            };
            let (s, e) = caps[0].unwrap();
            out.push(caps);
            from = if e == s { e + 1 } else { e };
        }
        out
    }

    /// Expands a Java replacement string (`$1`, `${name}` is not supported,
    /// `\` escapes) for one match.
    pub fn expand(&self, repl: &str, text: &[char], caps: &Caps) -> Result<String, String> {
        let r: Vec<char> = repl.chars().collect();
        let mut out = String::new();
        let mut i = 0;
        while i < r.len() {
            let c = r[i];
            if c == '\\' {
                i += 1;
                if i < r.len() {
                    out.push(r[i]);
                    i += 1;
                    continue;
                }
                return Err("character to be escaped is missing".to_string());
            }
            if c == '$' {
                i += 1;
                if i >= r.len() || !r[i].is_ascii_digit() {
                    return Err("Illegal group reference".to_string());
                }
                let mut g = r[i].to_digit(10).unwrap() as usize;
                i += 1;
                while i < r.len() && r[i].is_ascii_digit() {
                    let ng = g * 10 + r[i].to_digit(10).unwrap() as usize;
                    if ng > self.groups {
                        break;
                    }
                    g = ng;
                    i += 1;
                }
                if g > self.groups {
                    return Err(format!("No group {}", g));
                }
                if let Some((s, e)) = caps[g] {
                    out.extend(&text[s..e]);
                }
                continue;
            }
            out.push(c);
            i += 1;
        }
        Ok(out)
    }

    /// `String.replaceAll` / `replaceFirst`.
    pub fn replace(&self, s: &str, repl: &str, all: bool) -> String {
        let text: Vec<char> = s.chars().collect();
        let matches = if all {
            self.find_all(&text)
        } else {
            self.find_at(&text, 0).into_iter().collect()
        };
        let mut out = String::new();
        let mut last = 0;
        for caps in &matches {
            let (ms, me) = caps[0].unwrap();
            out.extend(&text[last..ms]);
            out.push_str(&self.expand(repl, &text, caps).unwrap_or_default());
            last = me;
        }
        out.extend(&text[last..]);
        out
    }
}
