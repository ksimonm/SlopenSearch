//! Tokenizers. Offsets are UTF-16 code units of the (char-filtered) input.

use std::collections::HashSet;

use serde_json::{Map, Value};

use super::jregex::{self, JRegex};
use super::uchar::{self, utf16_len};
use super::{bad, p_bool, p_int, p_list, p_str, AResult, Ctx, Stream, Token};
use crate::errors::ApiError;

#[derive(Debug, Clone)]
pub enum CharClass {
    Letter,
    Digit,
    Whitespace,
    Punctuation,
    Symbol,
    Custom,
    Category(String),
}

#[derive(Debug, Clone)]
pub struct TokenChars {
    classes: Vec<CharClass>,
    custom: HashSet<char>,
}

impl TokenChars {
    fn is_token_char(&self, c: char) -> bool {
        self.classes.iter().any(|cl| match cl {
            CharClass::Letter => uchar::is_letter(c) || (c.is_alphabetic() && !uchar::is_mark(c)),
            CharClass::Digit => uchar::is_digit(c),
            CharClass::Whitespace => uchar::is_java_whitespace(c),
            CharClass::Punctuation => uchar::is_punct(c),
            CharClass::Symbol => uchar::is_symbol(c),
            CharClass::Custom => self.custom.contains(&c),
            CharClass::Category(n) => uchar::general_category(c) == n,
        })
    }
}

#[derive(Debug, Clone)]
pub enum Tokenizer {
    Standard { max_len: usize },
    Classic { max_len: usize },
    UaxUrlEmail { max_len: usize },
    Letter { max_len: usize },
    Lowercase { max_len: usize },
    Whitespace { max_len: usize },
    Keyword,
    NGram { min: usize, max: usize, edge: bool, chars: Option<TokenChars> },
    PathHierarchy { delim: char, repl: char, skip: usize, reverse: bool },
    Pattern { re: JRegex, group: i64 },
    SimplePattern { re: JRegex },
    SimplePatternSplit { re: JRegex },
    CharGroup { split: Vec<CharClass>, chars: HashSet<char>, max_len: usize },
    Thai,
    /// A complete language analyzer of `crate::lang` (see builtin_analyzer).
    Language { name: String, params: Map<String, Value> },
}

pub fn compile_regex(pattern: &str, flags: u32) -> AResult<JRegex> {
    JRegex::new(pattern, flags).map_err(|e| ApiError::new("pattern_syntax_exception", &e.message(), 400))
}

const TOKEN_TYPE_NAMES: &str = "symbol, private_use, paragraph_separator, start_punctuation, unassigned, enclosing_mark, connector_punctuation, letter_number, other_number, math_symbol, lowercase_letter, space_separator, surrogate, initial_quote_punctuation, decimal_digit_number, digit, other_punctuation, dash_punctuation, currency_symbol, non_spacing_mark, custom, format, modifier_letter, control, uppercase_letter, other_symbol, end_punctuation, modifier_symbol, other_letter, line_separator, titlecase_letter, letter, punctuation, combining_spacing_mark, final_quote_punctuation, whitespace";

fn parse_token_chars(p: &Map<String, Value>) -> AResult<Option<TokenChars>> {
    let list = match p_list(p, "token_chars") {
        Some(l) => l,
        None => return Ok(None),
    };
    let list: Vec<String> = list.into_iter().flat_map(|s| s.split(',').map(|x| x.trim().to_string()).collect::<Vec<_>>()).filter(|s| !s.is_empty()).collect();
    if list.is_empty() {
        return Ok(None);
    }
    let mut classes = Vec::new();
    for name in &list {
        let cl = match name.to_lowercase().as_str() {
            "letter" => CharClass::Letter,
            "digit" => CharClass::Digit,
            "whitespace" => CharClass::Whitespace,
            "punctuation" => CharClass::Punctuation,
            "symbol" => CharClass::Symbol,
            "custom" => CharClass::Custom,
            other => {
                let known: Vec<&str> = TOKEN_TYPE_NAMES.split(", ").collect();
                if known.contains(&other) {
                    CharClass::Category(other.to_string())
                } else {
                    return Err(bad(&format!("Unknown token type: '{}', must be one of [{}]", other, TOKEN_TYPE_NAMES)));
                }
            }
        };
        classes.push(cl);
    }
    let custom_str = p_str(p, "custom_token_chars");
    let mut custom = HashSet::new();
    if classes.iter().any(|c| matches!(c, CharClass::Custom)) {
        match custom_str {
            Some(s) if !s.is_empty() => custom.extend(s.chars()),
            _ => return Err(bad("Token type: 'custom' requires setting `custom_token_chars`")),
        }
    }
    Ok(Some(TokenChars { classes, custom }))
}

fn unescape_char_setting(s: &str) -> AResult<char> {
    let chars: Vec<char> = s.chars().collect();
    if chars.len() == 1 {
        return Ok(chars[0]);
    }
    if chars.len() >= 2 && chars[0] == '\\' {
        match chars[1] {
            'n' if chars.len() == 2 => return Ok('\n'),
            't' if chars.len() == 2 => return Ok('\t'),
            'r' if chars.len() == 2 => return Ok('\r'),
            'f' if chars.len() == 2 => return Ok('\u{0C}'),
            'b' if chars.len() == 2 => return Ok('\u{08}'),
            '\\' if chars.len() == 2 => return Ok('\\'),
            '\'' if chars.len() == 2 => return Ok('\''),
            '"' if chars.len() == 2 => return Ok('"'),
            'u' if chars.len() == 6 => {
                let h: String = chars[2..].iter().collect();
                if let Ok(v) = u32::from_str_radix(&h, 16) {
                    if let Some(c) = char::from_u32(v) {
                        return Ok(c);
                    }
                }
            }
            _ => {}
        }
        return Err(bad(&format!("Invalid escaped char in [{}]", s)));
    }
    Err(bad(&format!("Invalid tokenize_on_chars [{}]", s)))
}

/// Builds a tokenizer of the given type. Ok(None) means the type is unknown.
pub fn build(typ: &str, p: &Map<String, Value>, ctx: &Ctx) -> AResult<Option<Tokenizer>> {
    let max_len = || -> AResult<usize> {
        let n = p_int(p, "max_token_length", 255);
        if n < 1 || n > 1024 * 1024 {
            return Err(bad(&format!("maxTokenLength must be greater than 0 and less than 1048576 passed: {}", n)));
        }
        Ok(n as usize)
    };
    Ok(Some(match typ {
        "standard" => Tokenizer::Standard { max_len: max_len()? },
        "classic" => Tokenizer::Classic { max_len: max_len()? },
        "uax_url_email" => Tokenizer::UaxUrlEmail { max_len: max_len()? },
        "letter" => Tokenizer::Letter { max_len: max_len()? },
        "lowercase" => Tokenizer::Lowercase { max_len: max_len()? },
        "whitespace" => Tokenizer::Whitespace { max_len: max_len()? },
        "keyword" => Tokenizer::Keyword,
        "ngram" | "nGram" | "edge_ngram" | "edgeNGram" => {
            let edge = typ.starts_with('e');
            let min = p_int(p, "min_gram", 1);
            let max = p_int(p, "max_gram", 2);
            if !edge && max - min > ctx.max_ngram_diff {
                return Err(bad(&format!(
                    "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [{}] but was [{}]. This limit can be set by changing the [index.max_ngram_diff] index level setting.",
                    ctx.max_ngram_diff,
                    max - min
                )));
            }
            if min < 1 {
                return Err(bad("minGram must be greater than zero"));
            }
            if min > max {
                return Err(bad("minGram must not be greater than maxGram"));
            }
            let chars = parse_token_chars(p)?;
            Tokenizer::NGram { min: min as usize, max: max as usize, edge, chars }
        }
        "path_hierarchy" | "PathHierarchy" => {
            let delim = p_str(p, "delimiter").unwrap_or_else(|| "/".to_string());
            let dc: Vec<char> = delim.chars().collect();
            if dc.len() != 1 {
                return Err(bad("delimiter must be a one char value"));
            }
            let repl = match p_str(p, "replacement") {
                Some(r) => {
                    let rc: Vec<char> = r.chars().collect();
                    if rc.len() > 1 {
                        return Err(bad("replacement must be a one char value"));
                    }
                    rc.first().copied().unwrap_or(dc[0])
                }
                None => dc[0],
            };
            Tokenizer::PathHierarchy {
                delim: dc[0],
                repl,
                skip: p_int(p, "skip", 0).max(0) as usize,
                reverse: p_bool(p, "reverse", false),
            }
        }
        "pattern" => {
            let pat = p_str(p, "pattern").unwrap_or_else(|| "\\W+".to_string());
            let flags = jregex::parse_flags(&p_str(p, "flags").unwrap_or_default()).map_err(|e| bad(&e))?;
            Tokenizer::Pattern { re: compile_regex(&pat, flags)?, group: p_int(p, "group", -1) }
        }
        "simple_pattern" => Tokenizer::SimplePattern { re: compile_regex(&p_str(p, "pattern").unwrap_or_default(), 0)? },
        "simple_pattern_split" => Tokenizer::SimplePatternSplit { re: compile_regex(&p_str(p, "pattern").unwrap_or_default(), 0)? },
        "char_group" => {
            let mut split = Vec::new();
            let mut chars = HashSet::new();
            for s in p_list(p, "tokenize_on_chars").unwrap_or_default() {
                match s.as_str() {
                    "letter" => split.push(CharClass::Letter),
                    "digit" => split.push(CharClass::Digit),
                    "whitespace" => split.push(CharClass::Whitespace),
                    "punctuation" => split.push(CharClass::Punctuation),
                    "symbol" => split.push(CharClass::Symbol),
                    _ => {
                        chars.insert(unescape_char_setting(&s)?);
                    }
                }
            }
            Tokenizer::CharGroup { split, chars, max_len: max_len()? }
        }
        "thai" => Tokenizer::Thai,
        _ => return Ok(None),
    }))
}

/// Text as chars with their UTF-16 start offsets (`off[len]` is the total).
struct Text {
    c: Vec<char>,
    off: Vec<usize>,
}

impl Text {
    fn new(s: &str) -> Text {
        let c: Vec<char> = s.chars().collect();
        let mut off = Vec::with_capacity(c.len() + 1);
        let mut o = 0;
        for ch in &c {
            off.push(o);
            o += ch.len_utf16();
        }
        off.push(o);
        Text { c, off }
    }
    fn slice(&self, a: usize, b: usize) -> String {
        self.c[a..b].iter().collect()
    }
}

impl Tokenizer {
    pub fn tokenize(&self, text: &str) -> Stream {
        let t = Text::new(text);
        let mut s = match self {
            Tokenizer::Standard { max_len } => standard(&t, *max_len, false),
            Tokenizer::UaxUrlEmail { max_len } => standard(&t, *max_len, true),
            Tokenizer::Classic { max_len } => classic(&t, *max_len),
            Tokenizer::Letter { max_len } => char_tokens(&t, *max_len, |c| uchar::is_letter(c) || (c.is_alphabetic() && !c.is_ascii()), false),
            Tokenizer::Lowercase { max_len } => char_tokens(&t, *max_len, |c| uchar::is_letter(c) || (c.is_alphabetic() && !c.is_ascii()), true),
            Tokenizer::Whitespace { max_len } => char_tokens(&t, *max_len, |c| !uchar::is_java_whitespace(c), false),
            Tokenizer::CharGroup { split, chars, max_len } => {
                let tc = TokenChars { classes: split.clone(), custom: HashSet::new() };
                char_tokens(&t, *max_len, |c| !(chars.contains(&c) || tc.is_token_char(c)), false)
            }
            Tokenizer::Keyword => Stream {
                tokens: vec![Token::new(text.to_string(), 0, t.off[t.c.len()], "word", 0)],
                end_pos: 1,
            },
            Tokenizer::NGram { min, max, edge, chars } => ngram(&t, *min, *max, *edge, chars.as_ref()),
            Tokenizer::PathHierarchy { delim, repl, skip, reverse } => {
                if *reverse {
                    reverse_path(&t, *delim, *repl, *skip)
                } else {
                    path(&t, *delim, *repl, *skip)
                }
            }
            Tokenizer::Pattern { re, group } => pattern(&t, re, *group),
            Tokenizer::SimplePattern { re } => simple_pattern(&t, re, false),
            Tokenizer::SimplePatternSplit { re } => simple_pattern(&t, re, true),
            Tokenizer::Thai => thai(text),
            Tokenizer::Language { name, params } => language_analyzer(name, params, text),
        };
        s.fix_end();
        s
    }
}

/// A whole Lucene language analyzer chain from `crate::lang` (it runs as
/// the tokenizer of a filter-less chain).
fn language_analyzer(name: &str, params: &Map<String, Value>, text: &str) -> Stream {
    let toks = crate::lang::chains::language_analyzer(name, params, text).unwrap_or_default();
    let mut pos: isize = -1;
    let mut end_pos = 0usize;
    let mut out = Vec::with_capacity(toks.len());
    for t in toks {
        pos += t.pos_inc as isize;
        let p = pos.max(0) as usize;
        let mut tok = Token::new(t.term, t.start, t.end, &t.typ, p);
        tok.pos_len = t.pos_len;
        end_pos = end_pos.max(p + t.pos_len);
        out.push(tok);
    }
    Stream { tokens: out, end_pos }
}

// ------------------------------------------------------------ char tokenizers

/// Lucene CharTokenizer: maximal runs of token chars, cut at `max_len`
/// UTF-16 units.
fn char_tokens(t: &Text, max_len: usize, is_tok: impl Fn(char) -> bool, lower: bool) -> Stream {
    let mut out = Vec::new();
    let mut i = 0;
    let n = t.c.len();
    while i < n {
        if !is_tok(t.c[i]) {
            i += 1;
            continue;
        }
        let start = i;
        let mut len = 0;
        while i < n && is_tok(t.c[i]) {
            let w = t.c[i].len_utf16();
            if len + w > max_len && len > 0 {
                break;
            }
            len += w;
            i += 1;
            if len >= max_len {
                break;
            }
        }
        let mut term = t.slice(start, i);
        if lower {
            term = uchar::lower_str(&term);
        }
        let pos = out.len();
        out.push(Token::new(term, t.off[start], t.off[i], "word", pos));
    }
    let end_pos = out.len();
    Stream { tokens: out, end_pos }
}

// ---------------------------------------------------------------- standard

#[derive(Debug, Clone, Copy, PartialEq)]
enum Wb {
    ALetter,
    Numeric,
    MidLetter,
    MidNum,
    MidNumLet,
    SingleQuote,
    ExtNumLet,
    Extend,
    Zwj,
    Katakana,
    Hiragana,
    Ideo,
    Hangul,
    Sea,
    Pict,
    Ri,
    Other,
}

fn wb(c: char) -> Wb {
    let v = c as u32;
    if v == 0x200D {
        return Wb::Zwj;
    }
    if uchar::is_mark(c) || (0xFE00..=0xFE0F).contains(&v) || v == 0x200C || uchar::is_emoji_modifier(c) || uchar::is_format(c) && v != 0xAD {
        if !uchar::is_southeast_asian(c) {
            return Wb::Extend;
        }
    }
    if v == 0xAD {
        return Wb::Extend;
    }
    if uchar::is_han(c) {
        return Wb::Ideo;
    }
    if uchar::is_hiragana(c) {
        return Wb::Hiragana;
    }
    if uchar::is_katakana(c) {
        return Wb::Katakana;
    }
    if uchar::is_hangul(c) {
        return Wb::Hangul;
    }
    if uchar::is_southeast_asian(c) {
        return Wb::Sea;
    }
    if uchar::is_regional_indicator(c) {
        return Wb::Ri;
    }
    if uchar::is_pictographic(c) {
        return Wb::Pict;
    }
    if c.is_ascii_digit() || (!c.is_ascii() && uchar::is_digit(c)) {
        return Wb::Numeric;
    }
    if c.is_ascii_alphabetic() || (!c.is_ascii() && c.is_alphabetic()) {
        return Wb::ALetter;
    }
    match v {
        0x27 => Wb::SingleQuote,
        0x2E | 0x2018 | 0x2019 | 0x2024 | 0xFE52 | 0xFF07 | 0xFF0E => Wb::MidNumLet,
        0x3A | 0xB7 | 0x387 | 0x55F | 0x5F4 | 0x2027 | 0xFE13 | 0xFE55 | 0xFF1A => Wb::MidLetter,
        0x2C | 0x3B | 0x37E | 0x589 | 0x60C | 0x60D | 0x66C | 0x7F8 | 0x2044 | 0xFE10 | 0xFE14 | 0xFE50 | 0xFE54 | 0xFF0C | 0xFF1B => Wb::MidNum,
        0x5F | 0x202F | 0x203F | 0x2040 | 0x2054 | 0xFE33 | 0xFE34 | 0xFE4D..=0xFE4F | 0xFF3F => Wb::ExtNumLet,
        _ => Wb::Other,
    }
}

const TLDS: &[&str] = &[
    "com", "org", "net", "edu", "gov", "mil", "int", "info", "biz", "name", "pro", "aero", "coop", "museum",
    "mobi", "jobs", "travel", "cat", "tel", "asia", "xxx", "post", "arpa", "app", "dev", "online", "site",
    "tech", "store", "blog", "cloud", "shop", "xyz", "top", "club", "live", "news", "email", "global",
    "page", "web", "wiki", "io", "ai", "co", "me", "tv", "cc", "ly", "fm", "gg", "localhost",
];

fn is_tld(s: &str) -> bool {
    let l = s.to_ascii_lowercase();
    if l.len() == 2 && l.chars().all(|c| c.is_ascii_alphabetic()) {
        return true;
    }
    TLDS.contains(&l.as_str())
}

fn is_label_char(c: char) -> bool {
    c.is_ascii_alphanumeric() || c == '-'
}

/// Length (in chars) of a domain name with a known TLD starting at `i`, the
/// longest such prefix.
fn domain_len(c: &[char], i: usize) -> Option<usize> {
    let mut j = i;
    let mut labels: Vec<(usize, usize)> = Vec::new();
    loop {
        let s = j;
        while j < c.len() && is_label_char(c[j]) {
            j += 1;
        }
        if j == s || c[s] == '-' || c[j - 1] == '-' {
            break;
        }
        labels.push((s, j));
        if j + 1 < c.len() && c[j] == '.' && is_label_char(c[j + 1]) {
            j += 1;
        } else {
            break;
        }
    }
    // Longest prefix of labels (at least two) whose last label is a TLD.
    while labels.len() >= 2 {
        let (s, e) = *labels.last().unwrap();
        let last: String = c[s..e].iter().collect();
        if is_tld(&last) && !last.chars().all(|x| x.is_ascii_digit()) {
            return Some(e - i);
        }
        labels.pop();
    }
    // IPv4
    let mut k = i;
    let mut parts = 0;
    loop {
        let s = k;
        while k < c.len() && c[k].is_ascii_digit() && k - s < 3 {
            k += 1;
        }
        if k == s {
            break;
        }
        parts += 1;
        if parts == 4 {
            return Some(k - i);
        }
        if k < c.len() && c[k] == '.' {
            k += 1;
        } else {
            break;
        }
    }
    None
}

fn email_len(c: &[char], i: usize) -> Option<usize> {
    let local = |ch: char| ch.is_ascii_alphanumeric() || "!#$%&'*+/=?^_`{|}~-".contains(ch);
    let mut j = i;
    loop {
        let s = j;
        while j < c.len() && local(c[j]) {
            j += 1;
        }
        if j == s {
            return None;
        }
        if j + 1 < c.len() && c[j] == '.' && local(c[j + 1]) {
            j += 1;
            continue;
        }
        break;
    }
    if j >= c.len() || c[j] != '@' {
        return None;
    }
    let d = domain_len(c, j + 1)?;
    Some(j + 1 + d - i)
}

fn url_len(c: &[char], i: usize) -> Option<usize> {
    let mut j = i;
    // scheme://
    let mut k = i;
    if k < c.len() && c[k].is_ascii_alphabetic() {
        while k < c.len() && (c[k].is_ascii_alphanumeric() || "+-.".contains(c[k])) {
            k += 1;
        }
        if k + 2 < c.len() && c[k] == ':' && c[k + 1] == '/' && c[k + 2] == '/' {
            j = k + 3;
        }
    }
    let has_scheme = j > i;
    let host = if has_scheme {
        match domain_len(c, j) {
            Some(d) => d,
            None => {
                let s = j;
                while j < c.len() && is_label_char(c[j]) {
                    j += 1;
                }
                if j == s {
                    return None;
                }
                j - s - (j - s)
            }
        }
    } else {
        domain_len(c, j)?
    };
    j += host;
    if j < c.len() && c[j] == ':' && j + 1 < c.len() && c[j + 1].is_ascii_digit() {
        j += 1;
        while j < c.len() && c[j].is_ascii_digit() {
            j += 1;
        }
    }
    let path_char = |ch: char| ch.is_ascii_alphanumeric() || "-._~!$&'()*+,;=:@/%?#".contains(ch);
    if j < c.len() && (c[j] == '/' || c[j] == '?' || c[j] == '#') {
        while j < c.len() && path_char(c[j]) {
            j += 1;
        }
        // Trailing punctuation is not part of the URL.
        while j > i && ".,;:!?'()".contains(c[j - 1]) && c[j - 1] != '/' {
            j -= 1;
        }
    }
    Some(j - i)
}

/// UAX#29 word segmentation as Lucene's StandardTokenizer applies it, plus
/// URL/email recognition for uax_url_email.
fn standard(t: &Text, max_len: usize, urls: bool) -> Stream {
    let c = &t.c;
    let n = c.len();
    let cls: Vec<Wb> = c.iter().map(|&ch| wb(ch)).collect();
    let mut out: Vec<Token> = Vec::new();
    let mut pos = 0usize;
    let width = |a: usize, b: usize| t.off[b] - t.off[a];
    let mut i = 0;
    let emit = |out: &mut Vec<Token>, a: usize, b: usize, typ: &str, pos: &mut usize| {
        out.push(Token::new(t.slice(a, b), t.off[a], t.off[b], typ, *pos));
        *pos += 1;
    };
    // Extends/format chars attach to the preceding char.
    let skip_ext = |mut j: usize, start: usize| {
        while j < n && matches!(cls[j], Wb::Extend | Wb::Zwj) && width(start, j + 1) <= max_len {
            j += 1;
        }
        j
    };
    while i < n {
        if urls {
            let mut best: Option<(usize, &str)> = None;
            if let Some(l) = url_len(c, i) {
                if l > 0 && width(i, i + l) <= max_len {
                    best = Some((l, "<URL>"));
                }
            }
            if let Some(l) = email_len(c, i) {
                if width(i, i + l) <= max_len && best.map(|b| l >= b.0).unwrap_or(true) {
                    best = Some((l, "<EMAIL>"));
                }
            }
            if let Some((l, typ)) = best {
                // A plain word scan must not be longer (it never is for valid matches
                // except bare domains, where URL wins ties).
                if l > 1 || typ == "<EMAIL>" {
                    let wl = word_len(&cls, i, max_len, &t.off);
                    if wl.map(|(w, _)| w <= l).unwrap_or(true) {
                        emit(&mut out, i, i + l, typ, &mut pos);
                        i += l;
                        continue;
                    }
                }
            }
        }
        match cls[i] {
            Wb::Ideo => {
                let j = skip_ext(i + 1, i);
                emit(&mut out, i, j, "<IDEOGRAPHIC>", &mut pos);
                i = j;
            }
            Wb::Hiragana => {
                let j = skip_ext(i + 1, i);
                emit(&mut out, i, j, "<HIRAGANA>", &mut pos);
                i = j;
            }
            Wb::Katakana | Wb::Hangul | Wb::Sea => {
                let k = cls[i];
                let mut j = i + 1;
                while j < n && width(i, j + 1) <= max_len && (cls[j] == k || matches!(cls[j], Wb::Extend | Wb::Zwj) || (k == Wb::Katakana && cls[j] == Wb::ExtNumLet)) {
                    j += 1;
                }
                let typ = match k {
                    Wb::Katakana => "<KATAKANA>",
                    Wb::Hangul => "<HANGUL>",
                    _ => "<SOUTHEAST_ASIAN>",
                };
                emit(&mut out, i, j, typ, &mut pos);
                i = j;
            }
            Wb::Pict => {
                let mut j = i + 1;
                loop {
                    if j < n && matches!(cls[j], Wb::Extend) {
                        j += 1;
                    } else if j + 1 < n && cls[j] == Wb::Zwj && cls[j + 1] == Wb::Pict {
                        j += 2;
                    } else if j < n && cls[j] == Wb::Zwj {
                        j += 1;
                    } else {
                        break;
                    }
                }
                emit(&mut out, i, j, "<EMOJI>", &mut pos);
                i = j;
            }
            Wb::Ri => {
                let mut j = i + 1;
                if j < n && cls[j] == Wb::Ri {
                    j += 1;
                }
                let j = skip_ext(j, i);
                emit(&mut out, i, j, "<EMOJI>", &mut pos);
                i = j;
            }
            Wb::ALetter | Wb::Numeric | Wb::ExtNumLet => {
                // Keycap sequences (digit FE0F 20E3) are emoji.
                if cls[i] == Wb::Numeric && i + 1 < n {
                    let mut j = i + 1;
                    if c[j] == '\u{FE0F}' {
                        j += 1;
                    }
                    if j < n && c[j] == '\u{20E3}' {
                        emit(&mut out, i, j + 1, "<EMOJI>", &mut pos);
                        i = j + 1;
                        continue;
                    }
                }
                match word_len(&cls, i, max_len, &t.off) {
                    Some((l, typ)) => {
                        emit(&mut out, i, i + l, typ, &mut pos);
                        i += l;
                    }
                    None => {
                        // Underscores alone are not a token.
                        let mut j = i + 1;
                        while j < n && matches!(cls[j], Wb::ExtNumLet | Wb::Extend | Wb::Zwj) {
                            j += 1;
                        }
                        i = j;
                    }
                }
            }
            _ => {
                if (c[i] == '#' || c[i] == '*') && i + 1 < n {
                    let mut j = i + 1;
                    if c[j] == '\u{FE0F}' {
                        j += 1;
                    }
                    if j < n && c[j] == '\u{20E3}' {
                        emit(&mut out, i, j + 1, "<EMOJI>", &mut pos);
                        i = j + 1;
                        continue;
                    }
                }
                i += 1;
            }
        }
    }
    Stream { tokens: out, end_pos: pos }
}

/// Length of the word starting at `i` (letters, digits, connectors and the
/// mid-word punctuation UAX#29 allows) and its token type. None for runs
/// of underscores only.
fn word_len(cls: &[Wb], i: usize, max_len: usize, off: &[usize]) -> Option<(usize, &'static str)> {
    let n = cls.len();
    let fits = |j: usize| off[j] - off[i] <= max_len;
    let is_word = |k: Wb| matches!(k, Wb::ALetter | Wb::Numeric | Wb::ExtNumLet);
    if !is_word(cls[i]) {
        return None;
    }
    let mut has_letter = cls[i] == Wb::ALetter;
    let mut has_num = cls[i] == Wb::Numeric;
    let mut last = cls[i];
    let mut j = i + 1;
    // Index after the last base char (excludes trailing extends beyond limit).
    loop {
        if j >= n || !fits(j + 1) {
            break;
        }
        let k = cls[j];
        if matches!(k, Wb::Extend | Wb::Zwj) {
            j += 1;
            continue;
        }
        if is_word(k) && is_word(last) {
            has_letter |= k == Wb::ALetter;
            has_num |= k == Wb::Numeric;
            last = k;
            j += 1;
            continue;
        }
        // Mid punctuation needs a matching char after it.
        let mut nx = j + 1;
        while nx < n && matches!(cls[nx], Wb::Extend | Wb::Zwj) {
            nx += 1;
        }
        if nx < n && fits(nx + 1) {
            let after = cls[nx];
            let mid_letter = matches!(k, Wb::MidLetter | Wb::MidNumLet | Wb::SingleQuote);
            let mid_num = matches!(k, Wb::MidNum | Wb::MidNumLet | Wb::SingleQuote);
            if mid_letter && last == Wb::ALetter && after == Wb::ALetter {
                j = nx + 1;
                last = after;
                continue;
            }
            if mid_num && last == Wb::Numeric && after == Wb::Numeric {
                j = nx + 1;
                last = after;
                continue;
            }
        }
        break;
    }
    if has_letter {
        Some((j - i, "<ALPHANUM>"))
    } else if has_num {
        Some((j - i, "<NUM>"))
    } else {
        None
    }
}

// ----------------------------------------------------------------- classic

fn classic_rules() -> &'static Vec<(regex::Regex, &'static str)> {
    use std::sync::OnceLock;
    static RULES: OnceLock<Vec<(regex::Regex, &'static str)>> = OnceLock::new();
    RULES.get_or_init(|| {
        let h = "[a0]*0[a0]*";
        let p = "[_\\-/.,]";
        let an = "[a0]+";
        let num = format!(
            "(?:{an}{p}{h}|{h}{p}{an}|{an}(?:{p}{h}{p}{an})+|{h}(?:{p}{an}{p}{h})+|{an}{p}{h}(?:{p}{an}{p}{h})+|{h}{p}{an}(?:{p}{h}{p}{an})+)",
            an = an,
            p = p,
            h = h
        );
        let rules: Vec<(String, &str)> = vec![
            ("[a0]+".to_string(), "<ALPHANUM>"),
            ("a+(?:'a+)+".to_string(), "<APOSTROPHE>"),
            ("a\\.(?:a\\.)+".to_string(), "<ACRONYM>"),
            ("a+[&@]a+".to_string(), "<COMPANY>"),
            ("[a0]+(?:[._\\-][a0]+)*@[a0]+(?:[.\\-][a0]+)+".to_string(), "<EMAIL>"),
            ("[a0]+(?:\\.[a0]+)+".to_string(), "<HOST>"),
            (num, "<NUM>"),
            ("[a0]+\\.(?:[a0]+\\.)+".to_string(), "ACRONYM_DEP"),
        ];
        rules
            .into_iter()
            .map(|(r, t)| (regex::Regex::new(&format!("^(?:{})$", r)).unwrap(), t))
            .collect()
    })
}

fn is_cj(c: char) -> bool {
    let v = c as u32;
    (0x3100..=0x312F).contains(&v) || (0x3040..=0x309F).contains(&v) || (0x30A0..=0x30FF).contains(&v)
        || (0x31F0..=0x31FF).contains(&v) || (0x3300..=0x337F).contains(&v) || (0x3400..=0x4DBF).contains(&v)
        || (0x4E00..=0x9FFF).contains(&v) || (0xF900..=0xFAFF).contains(&v) || (0xFF65..=0xFF9F).contains(&v)
}

fn classic(t: &Text, max_len: usize) -> Stream {
    let n = t.c.len();
    let mapped: Vec<char> = t
        .c
        .iter()
        .map(|&ch| {
            if is_cj(ch) {
                'c'
            } else if uchar::is_digit(ch) {
                '0'
            } else if uchar::is_letter(ch) || (ch.is_alphabetic() && !ch.is_ascii()) {
                'a'
            } else if ".'&@-_/,".contains(ch) {
                ch
            } else {
                ' '
            }
        })
        .collect();
    let rules = classic_rules();
    let mut out = Vec::new();
    let mut pos = 0usize;
    let mut i = 0;
    while i < n {
        let m = mapped[i];
        if m == 'c' {
            if 1 <= max_len {
                out.push(Token::new(t.slice(i, i + 1), t.off[i], t.off[i + 1], "<CJ>", pos));
            }
            pos += 1;
            i += 1;
            continue;
        }
        if m == ' ' {
            i += 1;
            continue;
        }
        let mut run_end = i;
        while run_end < n && mapped[run_end] != ' ' && mapped[run_end] != 'c' {
            run_end += 1;
        }
        let mut found: Option<(usize, &str)> = None;
        'len: for e in (i + 1..=run_end).rev() {
            let cand: String = mapped[i..e].iter().collect();
            for (re, typ) in rules.iter() {
                if re.is_match(&cand) {
                    found = Some((e - i, typ));
                    break 'len;
                }
            }
        }
        match found {
            None => i += 1,
            Some((l, typ)) => {
                let mut term = t.slice(i, i + l);
                let mut typ = typ.to_string();
                let mut end = t.off[i + l];
                if typ == "ACRONYM_DEP" {
                    typ = "<HOST>".to_string();
                    term.pop();
                    end -= 1;
                    let _ = end;
                    end = t.off[i + l];
                }
                if utf16_len(&term) <= max_len {
                    out.push(Token::new(term, t.off[i], end, &typ, pos));
                }
                pos += 1;
                i += l;
            }
        }
    }
    Stream { tokens: out, end_pos: pos }
}

// ------------------------------------------------------------------- ngram

fn ngram(t: &Text, min: usize, max: usize, edge: bool, chars: Option<&TokenChars>) -> Stream {
    let n = t.c.len();
    let is_tok = |c: char| chars.map(|tc| tc.is_token_char(c)).unwrap_or(true);
    let mut out = Vec::new();
    let mut i = 0;
    while i < n {
        if !is_tok(t.c[i]) {
            i += 1;
            continue;
        }
        let s = i;
        while i < n && is_tok(t.c[i]) {
            i += 1;
        }
        let e = i;
        let starts: Vec<usize> = if edge { vec![s] } else { (s..e).collect() };
        for st in starts {
            for size in min..=max {
                if st + size > e {
                    break;
                }
                let pos = out.len();
                out.push(Token::new(t.slice(st, st + size), t.off[st], t.off[st + size], "word", pos));
            }
        }
    }
    let end_pos = out.len();
    Stream { tokens: out, end_pos }
}

// ------------------------------------------------------------ path hierarchy

fn path(t: &Text, delim: char, repl: char, skip: usize) -> Stream {
    // A port of Lucene's PathHierarchyTokenizer.incrementToken.
    let c = &t.c;
    let n = c.len();
    let mut out: Vec<(Vec<char>, usize, usize)> = Vec::new();
    let mut result: Vec<char> = Vec::new();
    let mut idx = 0usize;
    let mut skipped = 0usize;
    let mut start_position = 0usize;
    let mut end_delimiter = false;
    'tokens: loop {
        let mut term = result.clone();
        let mut added = false;
        if end_delimiter {
            term.push(repl);
            added = true;
        }
        loop {
            if idx >= n {
                if skipped > skip && added {
                    let len = term.len();
                    out.push((term, start_position, start_position + len));
                }
                break 'tokens;
            }
            let ch = c[idx];
            idx += 1;
            if !added {
                added = true;
                skipped += 1;
                if skipped > skip {
                    term.push(if ch == delim { repl } else { ch });
                } else {
                    start_position += 1;
                }
            } else if ch == delim {
                if skipped > skip {
                    end_delimiter = true;
                    break;
                }
                skipped += 1;
                if skipped > skip {
                    term.push(repl);
                } else {
                    start_position += 1;
                }
            } else if skipped > skip {
                term.push(ch);
            } else {
                start_position += 1;
            }
        }
        let len = term.len();
        out.push((term.clone(), start_position, start_position + len));
        result = term;
    }
    let tokens: Vec<Token> = out
        .into_iter()
        .enumerate()
        .map(|(i, (term, s, e))| Token::new(term.into_iter().collect(), t.off[s.min(n)], t.off[e.min(n)], "word", i))
        .collect();
    let end_pos = tokens.len();
    Stream { tokens, end_pos }
}

fn reverse_path(t: &Text, delim: char, repl: char, skip: usize) -> Stream {
    // A port of Lucene's ReversePathHierarchyTokenizer.
    let c = &t.c;
    let mut positions = vec![0usize];
    let result: Vec<char> = c.iter().map(|&ch| if ch == delim { repl } else { ch }).collect();
    for (i, &ch) in c.iter().enumerate() {
        if ch == delim {
            positions.push(i + 1);
        }
    }
    let length = c.len();
    if *positions.last().unwrap() < length {
        positions.push(length);
    }
    let count = positions.len() as i64;
    let mut out = Vec::new();
    let idx = count - 1 - skip as i64;
    if idx >= 0 {
        let end = positions[idx as usize];
        let mut skipped = 0i64;
        while skipped < count - skip as i64 - 1 {
            let start = positions[skipped as usize];
            let term: String = result[start..end].iter().collect();
            let pos = out.len();
            out.push(Token::new(term, t.off[start], t.off[end], "word", pos));
            skipped += 1;
        }
    }
    let end_pos = out.len();
    Stream { tokens: out, end_pos }
}

// ----------------------------------------------------------------- pattern

fn pattern(t: &Text, re: &JRegex, group: i64) -> Stream {
    let mut out = Vec::new();
    let n = t.c.len();
    if group < 0 {
        let mut index = 0usize;
        for caps in re.find_all(&t.c) {
            let (s, e) = caps[0].unwrap();
            if s > index {
                let pos = out.len();
                out.push(Token::new(t.slice(index, s), t.off[index], t.off[s], "word", pos));
            }
            index = e;
        }
        if n > index {
            let pos = out.len();
            out.push(Token::new(t.slice(index, n), t.off[index], t.off[n], "word", pos));
        }
    } else {
        let g = group as usize;
        for caps in re.find_all(&t.c) {
            if let Some(Some((s, e))) = caps.get(g) {
                if s != e {
                    let pos = out.len();
                    out.push(Token::new(t.slice(*s, *e), t.off[*s], t.off[*e], "word", pos));
                }
            }
        }
    }
    let end_pos = out.len();
    Stream { tokens: out, end_pos }
}

fn simple_pattern(t: &Text, re: &JRegex, split: bool) -> Stream {
    let mut out = Vec::new();
    let n = t.c.len();
    let mut matches: Vec<(usize, usize)> = Vec::new();
    for caps in re.find_all(&t.c) {
        let (s, e) = caps[0].unwrap();
        if e > s {
            matches.push((s, e));
        }
    }
    if split {
        let mut index = 0;
        for (s, e) in matches {
            if s > index {
                let pos = out.len();
                out.push(Token::new(t.slice(index, s), t.off[index], t.off[s], "word", pos));
            }
            index = e;
        }
        if n > index {
            let pos = out.len();
            out.push(Token::new(t.slice(index, n), t.off[index], t.off[n], "word", pos));
        }
    } else {
        for (s, e) in matches {
            let pos = out.len();
            out.push(Token::new(t.slice(s, e), t.off[s], t.off[e], "word", pos));
        }
    }
    let end_pos = out.len();
    Stream { tokens: out, end_pos }
}

/// Thai needs a dictionary-based word break; without one, runs of Thai
/// letters and other words are emitted whole.
/// Lucene ThaiTokenizer (JDK dictionary-based word breaks), from `crate::lang`.
fn thai(text: &str) -> Stream {
    let toks = crate::lang::chains::thai_tokens(text);
    let n = toks.len();
    let tokens = toks.into_iter().enumerate().map(|(i, (term, s, e))| Token::new(term, s, e, "word", i)).collect();
    Stream { tokens, end_pos: n }
}
