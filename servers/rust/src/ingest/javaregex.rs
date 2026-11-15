//! Regular expressions of the ingest processors. OpenSearch compiles gsub,
//! split, kv and split_to_fields patterns with java.util.regex and grok
//! patterns with Joni (Oniguruma, Ruby syntax); both run here on
//! `crate::jregex`.

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use crate::errors::ApiError;
use crate::jregex::{self, JRegex, SyntaxError};

/// Matcher steps of one ingest regex call (a pattern that backtracks for
/// longer finds nothing).
const STEP_CAP: usize = 5_000_000;

type Compiled = Result<Rc<JRegex>, SyntaxError>;

thread_local! {
    static JAVA_CACHE: RefCell<HashMap<String, Compiled>> = RefCell::new(HashMap::new());
    static GROK_CACHE: RefCell<HashMap<String, Result<Rc<GrokRegex>, String>>> = RefCell::new(HashMap::new());
}

fn cached<T: Clone>(
    cache: &'static std::thread::LocalKey<RefCell<HashMap<String, T>>>,
    key: &str,
    f: impl FnOnce() -> T,
) -> T {
    if let Some(v) = cache.with(|c| c.borrow().get(key).cloned()) {
        return v;
    }
    let v = f();
    cache.with(|c| {
        let mut c = c.borrow_mut();
        if c.len() > 1000 {
            c.clear();
        }
        c.insert(key.to_string(), v.clone());
    });
    v
}

/// `Pattern.compile(pattern)`, cached.
pub fn compile_java(pattern: &str) -> Compiled {
    cached(&JAVA_CACHE, pattern, || JRegex::new(pattern, 0).map(Rc::new))
}

/// A pattern compiled when a document is processed: an invalid one fails the
/// document with the PatternSyntaxException.
pub fn java_pattern(pattern: &str) -> Result<Rc<JRegex>, ApiError> {
    compile_java(pattern).map_err(|e| ApiError::new("pattern_syntax_exception", &e.message(), 400))
}

/// `String.split(regex, limit)`.
pub fn java_split(s: &str, sep: &str, limit: i64) -> Result<Vec<String>, ApiError> {
    Ok(java_pattern(sep)?.split(s, limit))
}

/// `Matcher.replaceAll(replacement)`.
pub fn java_replace_all(re: &JRegex, s: &str, repl: &str) -> Result<String, ApiError> {
    match re.replace_java(s, repl, true, STEP_CAP) {
        Ok(Ok(out)) => Ok(out),
        Ok(Err((typ, reason))) => Err(ApiError::new(typ, &reason, 400)),
        Err(_) => Ok(s.to_string()),
    }
}

// ---------------------------------------------------------------- grok

/// A compiled grok expression: named groups are renumbered (Oniguruma names
/// allow characters Java names do not); `names[g]` is the original name of
/// group `g` (index 0 unused).
pub struct GrokRegex {
    re: JRegex,
    pub names: Vec<String>,
}

/// Rewrites an Oniguruma (Ruby syntax) pattern for jregex: named groups
/// become `(?<gN>...)`, `\k<name>` follows, the Ruby `(?m)` flag is Java's
/// DOTALL and `\h` is a hex digit. Invalid names give the Oniguruma message.
fn grok_translate(p: &str) -> Result<(String, Vec<String>), String> {
    let src: Vec<char> = p.chars().collect();
    let mut b = String::new();
    let mut names = vec![String::new()];
    let mut by_name: HashMap<String, usize> = HashMap::new();
    let mut in_class = false;
    let mut i = 0;
    while i < src.len() {
        let c = src[i];
        if c == '\\' && i + 1 < src.len() {
            let n = src[i + 1];
            match n {
                'h' if !in_class => b.push_str("[0-9a-fA-F]"),
                'h' => b.push_str("0-9a-fA-F"),
                'H' if !in_class => b.push_str("[^0-9a-fA-F]"),
                'k' if !in_class && i + 2 < src.len() && src[i + 2] == '<' => {
                    let mut end = i + 3;
                    while end < src.len() && src[end] != '>' {
                        end += 1;
                    }
                    let name: String = src[i + 3..end.min(src.len())].iter().collect();
                    let first = name.chars().next();
                    if first.is_none_or(|f| f.is_ascii_digit() || f == '-') {
                        return Err("invalid backref number/name".to_string());
                    }
                    let g = match by_name.get(&name) {
                        Some(g) => *g,
                        None => return Err(format!("undefined name <{}> reference", name)),
                    };
                    b.push_str(&format!("\\k<g{}>", g));
                    i = end + 1;
                    continue;
                }
                _ => {
                    b.push(c);
                    b.push(n);
                }
            }
            i += 2;
            continue;
        }
        if in_class {
            if c == ']' {
                in_class = false;
            }
            b.push(c);
            i += 1;
            continue;
        }
        if c == '[' {
            in_class = true;
            b.push(c);
            // a leading ']' (after an optional '^') is a literal
            if src.get(i + 1) == Some(&'^') {
                b.push('^');
                i += 1;
            }
            if src.get(i + 1) == Some(&']') {
                b.push(']');
                i += 1;
            }
            i += 1;
            continue;
        }
        if c == '('
            && src.get(i + 1) == Some(&'?')
            && src.get(i + 2) == Some(&'<')
            && !matches!(src.get(i + 3), Some('=') | Some('!'))
        {
            let mut end = i + 3;
            while end < src.len() && src[end] != '>' {
                end += 1;
            }
            if end >= src.len() {
                let rest: String = src[(i + 3).min(src.len())..].iter().collect();
                return Err(format!("invalid group name <{}>", rest));
            }
            let name: String = src[i + 3..end].iter().collect();
            if name.is_empty() {
                return Err("group name is empty".to_string());
            }
            if name.starts_with(|c: char| c.is_ascii_digit()) {
                return Err(format!("invalid group name <{}>", name));
            }
            names.push(name.clone());
            let g = names.len() - 1;
            by_name.entry(name).or_insert(g);
            b.push_str(&format!("(?<g{}>", g));
            i = end + 1;
            continue;
        }
        if c == '(' && src.get(i + 1) == Some(&'?') {
            // inline options (?imx-imx) / (?imx-imx:...): Ruby m is DOTALL
            let mut j = i + 2;
            while j < src.len() && "imx-".contains(src[j]) {
                j += 1;
            }
            if j > i + 2 && j < src.len() && (src[j] == ')' || src[j] == ':') {
                b.push_str("(?");
                for &f in &src[i + 2..j] {
                    b.push(if f == 'm' { 's' } else { f });
                }
                i = j;
                continue;
            }
        }
        b.push(c);
        i += 1;
    }
    Ok((b, names))
}

/// The Oniguruma message of a java.util.regex error.
fn onig_message(e: &SyntaxError) -> String {
    let d = e.desc.as_str();
    match d {
        "Unclosed group" => "end pattern with unmatched parenthesis".to_string(),
        "Unmatched closing ')'" => "unmatched close parenthesis".to_string(),
        "Unclosed character class" => "premature end of char-class".to_string(),
        "Illegal repetition range" => "upper is smaller than lower in repeat range".to_string(),
        "Illegal character range" => "empty range in char class".to_string(),
        _ if d.starts_with("Dangling meta character") => "target of repeat operator is not specified".to_string(),
        _ if d.starts_with("Unknown character property name {") => {
            let name = d.trim_start_matches("Unknown character property name {").trim_end_matches('}');
            format!("invalid character property name <{}>", name)
        }
        _ => d.to_string(),
    }
}

/// Compiles an expanded grok expression; the error is the Oniguruma message.
pub fn grok_compile(expanded: &str) -> Result<Rc<GrokRegex>, String> {
    cached(&GROK_CACHE, expanded, || {
        let (tr, names) = grok_translate(expanded)?;
        let re = JRegex::new(&tr, jregex::MULTILINE).map_err(|e| onig_message(&e))?;
        if re.variable_lookbehind() {
            return Err("invalid pattern in look-behind".to_string());
        }
        Ok(Rc::new(GrokRegex { re, names }))
    })
}

impl GrokRegex {
    /// The value of every named group of the first match (None for groups
    /// that did not participate), or None without a match.
    pub fn find(&self, s: &str) -> Option<Vec<Option<String>>> {
        let text: Vec<char> = s.chars().collect();
        let caps = self.re.find_from(&text, 0, STEP_CAP).ok()??;
        let mut out = vec![None; self.names.len()];
        for (i, slot) in out.iter_mut().enumerate().skip(1) {
            if let Some(Some((a, b))) = self.re.group_index(&format!("g{}", i)).map(|g| caps[g]) {
                *slot = Some(text[a..b].iter().collect());
            }
        }
        Some(out)
    }
}
