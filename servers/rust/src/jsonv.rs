//! Helpers over decoded JSON values (serde_json `Value`): numbers, scalar
//! strings, dotted paths.

use serde_json::Value;
use std::collections::HashMap;

use crate::resp::format_number;

pub fn stringify_scalar(v: &Value) -> Value {
    match v {
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Value::String(i.to_string())
            } else if let Some(u) = n.as_u64() {
                Value::String(u.to_string())
            } else if let Some(f) = n.as_f64() {
                Value::String(format_number(f))
            } else {
                v.clone()
            }
        }
        Value::Bool(b) => Value::String(b.to_string()),
        other => other.clone(),
    }
}

pub fn to_f64(v: &Value) -> Option<f64> {
    match v {
        Value::Number(n) => n.as_f64(),
        Value::String(s) => s.trim().parse::<f64>().ok(),
        Value::Bool(b) => Some(if *b { 1.0 } else { 0.0 }),
        _ => None,
    }
}

pub fn value_to_string(v: &Value) -> Option<String> {
    match v {
        Value::String(s) => Some(s.clone()),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Some(i.to_string())
            } else if let Some(u) = n.as_u64() {
                Some(u.to_string())
            } else if let Some(f) = n.as_f64() {
                Some(format_number(f))
            } else {
                Some(n.to_string())
            }
        }
        Value::Bool(b) => Some(b.to_string()),
        _ => None,
    }
}

pub fn doc_strings(v: &Value) -> Vec<String> {
    match v {
        Value::Array(a) => a.iter().filter_map(value_to_string).collect(),
        Value::Null => vec![],
        other => value_to_string(other).into_iter().collect(),
    }
}

fn value_at_path<'a>(doc: &'a Value, path: &str) -> Option<&'a Value> {
    let mut cur = doc;
    for part in path.split('.') {
        cur = cur.as_object()?.get(part)?;
    }
    Some(cur)
}

pub fn field_value<'a>(doc: &'a Value, path: &str) -> Option<&'a Value> {
    if let Some(v) = value_at_path(doc, path) {
        return Some(v);
    }
    let parts: Vec<&str> = path.split('.').collect();
    for i in (1..parts.len()).rev() {
        if let Some(v) = value_at_path(doc, &parts[..i].join(".")) {
            return Some(v);
        }
    }
    None
}

/// serde_json normalizes the exponent of a number while parsing (`1E2` and
/// `1e2` read `1e+2`). Go's decoder (UseNumber) keeps the literal text, so
/// documents indexed with `1E+2` echo back verbatim. Re-apply the original
/// spelling for numbers whose exponent was normalized.
pub(crate) fn fix_exponent_literals(s: &str, v: &mut Value) {
    let bytes = s.as_bytes();
    let mut map: HashMap<String, String> = HashMap::new();
    let mut i = 0;
    let mut in_str = false;
    let mut esc = false;
    while i < bytes.len() {
        let c = bytes[i];
        if in_str {
            if esc {
                esc = false;
            } else if c == b'\\' {
                esc = true;
            } else if c == b'"' {
                in_str = false;
            }
            i += 1;
            continue;
        }
        if c == b'"' {
            in_str = true;
            i += 1;
            continue;
        }
        if c == b'-' || c.is_ascii_digit() {
            let start = i;
            if c == b'-' {
                i += 1;
            }
            while i < bytes.len() {
                let d = bytes[i];
                if d.is_ascii_digit() || matches!(d, b'.' | b'e' | b'E' | b'+' | b'-') {
                    i += 1;
                } else {
                    break;
                }
            }
            let tok = &s[start..i];
            // serde_json also adds a `+` to an unsigned exponent (1e7 reads 1e+7)
            if tok.contains(['e', 'E']) {
                if let Ok(Value::Number(n)) = serde_json::from_str::<Value>(tok) {
                    if n.to_string() != tok {
                        map.insert(n.to_string(), tok.to_string());
                    }
                }
            }
            continue;
        }
        i += 1;
    }
    if map.is_empty() {
        return;
    }
    fn walk(v: &mut Value, map: &HashMap<String, String>) {
        match v {
            Value::Number(n) => {
                let key = n.to_string();
                if let Some(raw) = map.get(&key) {
                    *n = serde_json::Number::from_string_unchecked(raw.clone());
                }
            }
            Value::Array(a) => {
                for e in a.iter_mut() {
                    walk(e, map);
                }
            }
            Value::Object(m) => {
                for (_, e) in m.iter_mut() {
                    walk(e, map);
                }
            }
            _ => {}
        }
    }
    walk(v, &map);
}

pub fn duplicate_object_key(s: &str) -> Option<String> {
    use std::collections::HashSet;
    let b = s.as_bytes();
    let mut i = 0usize;
    let mut stack: Vec<HashSet<String>> = vec![];
    let mut in_str = false;
    let mut esc = false;
    let mut cur = String::new();
    let mut last_str: Option<String> = None;
    let mut expect_key = false;
    while i < b.len() {
        let c = b[i];
        if in_str {
            if esc {
                cur.push(c as char);
                esc = false;
            } else if c == b'\\' {
                esc = true;
            } else if c == b'"' {
                in_str = false;
                last_str = Some(std::mem::take(&mut cur));
            } else {
                cur.push(c as char);
            }
            i += 1;
            continue;
        }
        match c {
            b'"' => {
                in_str = true;
                cur.clear();
            }
            b'{' => {
                stack.push(HashSet::new());
                expect_key = true;
                last_str = None;
            }
            b'}' => {
                stack.pop();
                expect_key = false;
                last_str = None;
            }
            b',' => {
                expect_key = true;
                last_str = None;
            }
            b':' => {
                if expect_key {
                    if let Some(k) = last_str.clone() {
                        if let Some(top) = stack.last_mut() {
                            if !top.insert(k.clone()) {
                                return Some(k);
                            }
                        }
                    }
                    expect_key = false;
                }
                last_str = None;
            }
            b'[' | b']' => {
                expect_key = false;
                last_str = None;
            }
            _ => {
                if !c.is_ascii_whitespace() {
                    last_str = None;
                }
            }
        }
        i += 1;
    }
    None
}

// ------------------------------------------------------- body positions

/// Finds the line and column (1-based) of the value at `path` in a JSON
/// document, the way Jackson reports token locations.
pub fn locate(raw: &str, path: &[&str]) -> Option<(usize, usize)> {
    let chars: Vec<char> = raw.chars().collect();
    let mut sc = Scan { s: &chars, i: 0, line: 1, col: 1, key_mode: false };
    sc.value(path)
}

/// Like `locate`, but reports the position of the last member's name.
pub fn locate_key(raw: &str, path: &[&str]) -> Option<(usize, usize)> {
    let chars: Vec<char> = raw.chars().collect();
    let mut sc = Scan { s: &chars, i: 0, line: 1, col: 1, key_mode: true };
    sc.value(path)
}

struct Scan<'a> {
    s: &'a [char],
    i: usize,
    line: usize,
    col: usize,
    key_mode: bool,
}

impl<'a> Scan<'a> {
    fn peek(&self) -> Option<char> {
        self.s.get(self.i).copied()
    }
    fn bump(&mut self) {
        if let Some(c) = self.peek() {
            self.i += 1;
            if c == '\n' {
                self.line += 1;
                self.col = 1;
            } else {
                self.col += 1;
            }
        }
    }
    fn ws(&mut self) {
        while matches!(self.peek(), Some(c) if c.is_whitespace()) {
            self.bump();
        }
    }
    fn string(&mut self) -> String {
        let mut out = String::new();
        self.bump();
        while let Some(c) = self.peek() {
            self.bump();
            if c == '\\' {
                if let Some(n) = self.peek() {
                    out.push(n);
                    self.bump();
                }
            } else if c == '"' {
                break;
            } else {
                out.push(c);
            }
        }
        out
    }
    /// Skips a value; when `path` is non-empty and the value is an object,
    /// descends into the member named `path[0]`.
    fn value(&mut self, path: &[&str]) -> Option<(usize, usize)> {
        self.ws();
        match self.peek()? {
            '{' => {
                self.bump();
                loop {
                    self.ws();
                    match self.peek()? {
                        '}' => {
                            self.bump();
                            return None;
                        }
                        ',' => {
                            self.bump();
                            continue;
                        }
                        '"' => {}
                        _ => return None,
                    }
                    let key_pos = (self.line, self.col);
                    let key = self.string();
                    self.ws();
                    if self.peek()? == ':' {
                        self.bump();
                    }
                    self.ws();
                    if !path.is_empty() && key == path[0] {
                        if path.len() == 1 {
                            return Some(if self.key_mode { key_pos } else { (self.line, self.col) });
                        }
                        if let Some(p) = self.value(&path[1..]) {
                            return Some(p);
                        }
                    } else {
                        self.value(&[]);
                    }
                }
            }
            '[' => {
                self.bump();
                let mut n = 0usize;
                loop {
                    self.ws();
                    match self.peek()? {
                        ']' => {
                            self.bump();
                            return None;
                        }
                        ',' => {
                            self.bump();
                            n += 1;
                        }
                        _ => {
                            if !path.is_empty() && path[0] == n.to_string() {
                                if path.len() == 1 {
                                    return Some((self.line, self.col));
                                }
                                if let Some(p) = self.value(&path[1..]) {
                                    return Some(p);
                                }
                            } else {
                                self.value(&[]);
                            }
                        }
                    }
                }
            }
            '"' => {
                self.string();
                None
            }
            _ => {
                while matches!(self.peek(), Some(c) if !matches!(c, ',' | '}' | ']') && !c.is_whitespace()) {
                    self.bump();
                }
                None
            }
        }
    }
}
