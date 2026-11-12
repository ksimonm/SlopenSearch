// Scripting (minimal painless/expression subset). Port of servers/go/painless.go.

use crate::eval::{compare_any, integer_of, QueryCtx};
use crate::resp::format_number;
use crate::store::to_f64;
use serde_json::{Map, Number, Value};

pub struct ScriptEnv<'a, 'b> {
    pub c: &'a QueryCtx<'b>,
    pub params: serde_json::Map<String, Value>,
}

impl<'a, 'b> ScriptEnv<'a, 'b> {
    pub fn resolve_path(&self, path: &str) -> Value {
        if path.starts_with("params[") {
            let mut norm = String::from("params");
            for seg in path["params".len()..].split(']') {
                let seg = seg.trim_start_matches('.').trim_start_matches('[');
                let seg = seg.trim_matches(|c| c == '\'' || c == '"');
                if !seg.is_empty() {
                    norm.push('.');
                    norm.push_str(seg);
                }
            }
            return self.resolve_path(&norm);
        }
        if path.starts_with("doc[") {
            let i = match path.find(']') {
                Some(i) => i,
                None => return Value::Null,
            };
            let field = path[4..i].trim_matches(|c| c == '\'' || c == '"');
            let (mut vals, _) = self.c.values(field);
            if i + 1 == path.len() {
                vals.sort_by(|a, b| compare_any(a, b).cmp(&0));
                return Value::Array(vals);
            }
            if vals.is_empty() {
                return Value::Null;
            }
            return vals[0].clone();
        }
        if let Some(rest) = path.strip_prefix("doc.") {
            let field = rest
                .strip_suffix(".value")
                .or_else(|| rest.strip_suffix(".size()"))
                .unwrap_or(rest);
            let (vals, _) = self.c.values(field);
            if vals.is_empty() {
                return Value::Null;
            }
            return vals[0].clone();
        }
        if path == "_score" || path == "score" {
            return self.params.get("__score").cloned().unwrap_or(Value::Null);
        }
        if path == "ctx._index" {
            return self.params.get("__ctx_index").cloned().unwrap_or(Value::Null);
        }
        if path == "ctx._id" {
            return self.params.get("__ctx_id").cloned().unwrap_or(Value::Null);
        }
        if path == "ctx._version" {
            return self.params.get("__ctx_version").cloned().unwrap_or(Value::Null);
        }
        if path == "ctx._source" {
            return self.params.get("__ctx_source").cloned().unwrap_or(Value::Null);
        }
        if let Some(rest) = path.strip_prefix("ctx._source.") {
            let mut cur = match self.params.get("__ctx_source") {
                Some(v) => v,
                None => return Value::Null,
            };
            for seg in rest.split('.') {
                match cur.get(seg) {
                    Some(v) => cur = v,
                    None => return Value::Null,
                }
            }
            return cur.clone();
        }
        if let Some(key) = path.strip_prefix("params.") {
            let mut segs = key.split('.');
            let mut cur = match self.params.get(segs.next().unwrap_or("")) {
                Some(v) => v,
                None => return Value::Null,
            };
            for seg in segs {
                match cur.get(seg) {
                    Some(v) => cur = v,
                    None => return Value::Null,
                }
            }
            return cur.clone();
        }
        if let Some(key) = path.strip_prefix("params.") {
            let mut cur = match self.params.get(key) {
                Some(v) => v,
                None => return Value::Null,
            };
            let mut rest = key;
            for seg in rest.split('.').skip(1) {
                rest = seg;
                match cur.get(seg) {
                    Some(v) => cur = v,
                    None => return Value::Null,
                }
            }
            let _ = rest;
            return cur.clone();
        }
        if path == "params" {
            return Value::Object(
                self.params
                    .iter()
                    .filter(|(k, _)| !k.starts_with("__"))
                    .map(|(k, v)| (k.clone(), v.clone()))
                    .collect(),
            );
        }
        if let Some(v) = self.params.get(path) {
            return v.clone();
        }
        Value::Null
    }

    pub fn eval_script_expression(&self, src: &str) -> Result<Value, String> {
        let mut src = src.trim().to_string();
        if src.ends_with(';') {
            src = src.trim_end_matches(';').trim().to_string();
        }
        if let Some(rest) = src.strip_prefix("return ") {
            src = rest.trim().to_string();
        }
        if src.ends_with(';') {
            src = src.trim_end_matches(';').trim().to_string();
        }
        let mut e2 = ExprEval {
            src: src.into_bytes(),
            pos: 0,
            env: self,
            skip: 0,
        };
        let v = e2.parse_ternary()?;
        e2.skip_space();
        if e2.pos < e2.src.len() {
            return Err(format!("unexpected token at {}", e2.pos));
        }
        Ok(v)
    }
}

/// Message of the illegal_state_exception raised by doc[...].value on a
/// document without a value.
pub const MISSING_DOC_VALUE: &str = "A document doesn't have a value for a field! Use doc[<field>].size()==0 to check if a document is missing a field!";

pub fn script_float(v: &Value) -> f64 {
    if let Some(n) = integer_of(v) {
        return n as f64;
    }
    to_f64(v).unwrap_or(0.0)
}

pub fn truthy(v: &Value) -> bool {
    match v {
        Value::Null => false,
        Value::Bool(b) => *b,
        Value::Number(n) => n.as_f64().unwrap_or(0.0) != 0.0,
        Value::String(s) => !s.is_empty(),
        _ => true,
    }
}

fn int_or_zero(v: &Value) -> i64 {
    if let Some(n) = integer_of(v) {
        return n;
    }
    script_float(v) as i64
}

fn value_to_script_string(v: &Value) -> Option<String> {
    match v {
        Value::String(s) => Some(s.clone()),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Some(i.to_string())
            } else if let Some(u) = n.as_u64() {
                Some(u.to_string())
            } else {
                Some(format_number(n.as_f64().unwrap_or(0.0)))
            }
        }
        Value::Bool(b) => Some(b.to_string()),
        Value::Null => Some("null".to_string()),
        _ => None,
    }
}

fn values_equal(a: &Value, b: &Value) -> bool {
    if a.is_null() || b.is_null() {
        return a.is_null() && b.is_null();
    }
    if let Value::String(sa) = a {
        let bs = value_to_script_string(b).unwrap_or_default();
        return *sa == bs;
    }
    script_float(a) == script_float(b)
}

/// Both operands as integers (Painless integer arithmetic), if they are.
fn int_pair(a: &Value, b: &Value) -> Option<(i64, i64)> {
    match (a, b) {
        (Value::Number(x), Value::Number(y)) if !x.is_f64() && !y.is_f64() => Some((x.as_i64()?, y.as_i64()?)),
        _ => None,
    }
}

fn num_f64(f: f64) -> Value {
    match Number::from_f64(f) {
        Some(n) => Value::Number(n),
        None if f.is_nan() => Value::String("NaN".to_string()),
        None if f > 0.0 => Value::String("Infinity".to_string()),
        None => Value::String("-Infinity".to_string()),
    }
}

struct ExprEval<'a, 'b, 'c> {
    src: Vec<u8>,
    pos: usize,
    env: &'c ScriptEnv<'a, 'b>,
    /// > 0 while parsing a branch that is not taken (no runtime errors).
    skip: u32,
}

impl<'a, 'b, 'c> ExprEval<'a, 'b, 'c> {
    fn skip_space(&mut self) {
        while self.pos < self.src.len() {
            let c = self.src[self.pos];
            if c == b' ' || c == b'\t' || c == b'\n' || c == b'\r' {
                self.pos += 1;
            } else {
                break;
            }
        }
    }

    fn peek(&mut self, prefix: &str) -> bool {
        self.skip_space();
        self.src[self.pos..].starts_with(prefix.as_bytes())
    }

    fn eat(&mut self, prefix: &str) -> bool {
        if self.peek(prefix) {
            self.pos += prefix.len();
            true
        } else {
            false
        }
    }

    /// Parses a sub-expression whose value is not used.
    fn skipped<T>(&mut self, f: impl FnOnce(&mut Self) -> Result<T, String>) -> Result<T, String> {
        self.skip += 1;
        let r = f(self);
        self.skip -= 1;
        r
    }

    fn parse_ternary(&mut self) -> Result<Value, String> {
        let cond = self.parse_or()?;
        if self.eat("?") {
            let take = truthy(&cond);
            let a = if take { self.parse_ternary()? } else { self.skipped(|p| p.parse_ternary())? };
            if !self.eat(":") {
                return Err("expected ':'".to_string());
            }
            let b = if take { self.skipped(|p| p.parse_ternary())? } else { self.parse_ternary()? };
            if take {
                return Ok(a);
            }
            return Ok(b);
        }
        Ok(cond)
    }

    fn parse_or(&mut self) -> Result<Value, String> {
        let mut left = self.parse_and()?;
        loop {
            self.skip_space();
            if self.peek("||") {
                self.pos += 2;
                let right = if truthy(&left) { self.skipped(|p| p.parse_and())? } else { self.parse_and()? };
                left = Value::Bool(truthy(&left) || truthy(&right));
                continue;
            }
            if self.peek("|") && !self.peek("||") {
                self.pos += 1;
                let right = self.parse_and()?;
                left = Value::Number(Number::from(int_or_zero(&left) | int_or_zero(&right)));
                continue;
            }
            return Ok(left);
        }
    }

    fn parse_and(&mut self) -> Result<Value, String> {
        let mut left = self.parse_equality()?;
        loop {
            self.skip_space();
            if self.peek("&&") {
                self.pos += 2;
                let right = if truthy(&left) { self.parse_equality()? } else { self.skipped(|p| p.parse_equality())? };
                left = Value::Bool(truthy(&left) && truthy(&right));
                continue;
            }
            return Ok(left);
        }
    }

    fn parse_equality(&mut self) -> Result<Value, String> {
        let mut left = self.parse_comparison()?;
        loop {
            self.skip_space();
            if self.peek("==") {
                self.pos += 2;
                let right = self.parse_comparison()?;
                left = Value::Bool(values_equal(&left, &right));
            } else if self.peek("!=") {
                self.pos += 2;
                let right = self.parse_comparison()?;
                left = Value::Bool(!values_equal(&left, &right));
            } else {
                return Ok(left);
            }
        }
    }

    fn parse_comparison(&mut self) -> Result<Value, String> {
        let mut left = self.parse_additive()?;
        loop {
            self.skip_space();
            if self.peek("<=") {
                self.pos += 2;
                let right = self.parse_additive()?;
                left = Value::Bool(compare_any(&left, &right) <= 0);
            } else if self.peek(">=") {
                self.pos += 2;
                let right = self.parse_additive()?;
                left = Value::Bool(compare_any(&left, &right) >= 0);
            } else if self.peek("<") {
                self.pos += 1;
                let right = self.parse_additive()?;
                left = Value::Bool(compare_any(&left, &right) < 0);
            } else if self.peek(">") {
                self.pos += 1;
                let right = self.parse_additive()?;
                left = Value::Bool(compare_any(&left, &right) > 0);
            } else {
                return Ok(left);
            }
        }
    }

    fn parse_additive(&mut self) -> Result<Value, String> {
        let mut left = self.parse_multiplicative()?;
        loop {
            self.skip_space();
            if self.peek("+") {
                self.pos += 1;
                let right = self.parse_multiplicative()?;
                if let Value::String(ls) = &left {
                    let rs = value_to_script_string(&right).unwrap_or_default();
                    left = Value::String(format!("{}{}", ls, rs));
                } else if let Some((a, b)) = int_pair(&left, &right) {
                    left = Value::Number(Number::from(a.wrapping_add(b)));
                } else {
                    left = num_f64(script_float(&left) + script_float(&right));
                }
            } else if self.peek("-") {
                self.pos += 1;
                let right = self.parse_multiplicative()?;
                left = match int_pair(&left, &right) {
                    Some((a, b)) => Value::Number(Number::from(a.wrapping_sub(b))),
                    None => num_f64(script_float(&left) - script_float(&right)),
                };
            } else {
                return Ok(left);
            }
        }
    }

    fn parse_multiplicative(&mut self) -> Result<Value, String> {
        let mut left = self.parse_unary()?;
        loop {
            self.skip_space();
            if self.peek("*") {
                self.pos += 1;
                let right = self.parse_unary()?;
                left = match int_pair(&left, &right) {
                    Some((a, b)) => Value::Number(Number::from(a.wrapping_mul(b))),
                    None => num_f64(script_float(&left) * script_float(&right)),
                };
            } else if self.peek("/") {
                self.pos += 1;
                let right = self.parse_unary()?;
                left = match int_pair(&left, &right) {
                    Some((_, 0)) => return Err("/ by zero".to_string()),
                    Some((a, b)) => Value::Number(Number::from(a.wrapping_div(b))),
                    None => num_f64(script_float(&left) / script_float(&right)),
                };
            } else if self.peek("%") {
                self.pos += 1;
                let right = self.parse_unary()?;
                left = match int_pair(&left, &right) {
                    Some((_, 0)) => return Err("/ by zero".to_string()),
                    Some((a, b)) => Value::Number(Number::from(a.wrapping_rem(b))),
                    None => num_f64(script_float(&left) % script_float(&right)),
                };
            } else {
                return Ok(left);
            }
        }
    }

    fn parse_unary(&mut self) -> Result<Value, String> {
        self.skip_space();
        if self.eat("!") && !self.peek("=") {
            let v = self.parse_unary()?;
            return Ok(Value::Bool(!truthy(&v)));
        }
        if self.peek("-") {
            self.pos += 1;
            let v = self.parse_unary()?;
            return Ok(num_f64(-script_float(&v)));
        }
        self.parse_primary()
    }

    fn parse_primary(&mut self) -> Result<Value, String> {
        self.skip_space();
        if self.pos >= self.src.len() {
            return Err("unexpected end of expression".to_string());
        }
        if self.src[self.pos] == b'(' {
            self.pos += 1;
            let v = self.parse_ternary()?;
            if !self.eat(")") {
                return Err("expected ')'".to_string());
            }
            return Ok(v);
        }
        let c = self.src[self.pos];
        if c == b'[' {
            // list literal [a, b] or map literal [k: v, ...] / [:]
            self.pos += 1;
            if self.eat(":") {
                if !self.eat("]") {
                    return Err("expected ']'".to_string());
                }
                return Ok(Value::Object(Map::new()));
            }
            let mut list = Vec::new();
            let mut map = Map::new();
            if self.eat("]") {
                return Ok(Value::Array(list));
            }
            loop {
                let v = self.parse_ternary()?;
                if self.eat(":") {
                    let val = self.parse_ternary()?;
                    map.insert(value_to_script_string(&v).unwrap_or_default(), val);
                } else {
                    list.push(v);
                }
                if self.eat(",") {
                    continue;
                }
                if !self.eat("]") {
                    return Err("expected ']'".to_string());
                }
                break;
            }
            if !map.is_empty() {
                return Ok(Value::Object(map));
            }
            return Ok(Value::Array(list));
        }
        if c == b'\'' || c == b'"' {
            self.pos += 1;
            let start = self.pos;
            while self.pos < self.src.len() && self.src[self.pos] != c {
                self.pos += 1;
            }
            let s = String::from_utf8_lossy(&self.src[start..self.pos]).to_string();
            if self.pos < self.src.len() {
                self.pos += 1;
            }
            return Ok(Value::String(s));
        }
        if c.is_ascii_digit() {
            let start = self.pos;
            while self.pos < self.src.len()
                && (self.src[self.pos].is_ascii_digit() || self.src[self.pos] == b'.')
            {
                self.pos += 1;
            }
            if self.pos < self.src.len()
                && (self.src[self.pos] == b'L' || self.src[self.pos] == b'l')
            {
                self.pos += 1;
                let text = String::from_utf8_lossy(&self.src[start..self.pos - 1]).to_string();
                let n: i64 = text.parse().unwrap_or(0);
                return Ok(Value::Number(Number::from(n)));
            }
            let text = String::from_utf8_lossy(&self.src[start..self.pos]).to_string();
            if !text.contains('.') {
                if let Ok(n) = text.parse::<i64>() {
                    return Ok(Value::Number(Number::from(n)));
                }
            }
            let f: f64 = text.parse().unwrap_or(0.0);
            return Ok(num_f64(f));
        }
        let start = self.pos;
        let mut depth: i32 = 0;
        while self.pos < self.src.len() {
            let ch = self.src[self.pos];
            if ch == b'[' {
                depth += 1;
                self.pos += 1;
                continue;
            }
            if ch == b']' {
                depth -= 1;
                self.pos += 1;
                continue;
            }
            if ch == b'.' && depth == 0 {
                break;
            }
            if ch == b'.'
                || ch == b'_'
                || ch == b'\''
                || ch == b'"'
                || ch.is_ascii_alphabetic()
                || ch.is_ascii_digit()
            {
                self.pos += 1;
                continue;
            }
            break;
        }
        let mut token = String::from_utf8_lossy(&self.src[start..self.pos]).to_string();
        loop {
            let save = self.pos;
            self.skip_space();
            if self.pos < self.src.len() && self.src[self.pos] == b'.' {
                self.pos += 1;
                let fstart = self.pos;
                while self.pos < self.src.len()
                    && (self.src[self.pos].is_ascii_alphabetic()
                        || self.src[self.pos].is_ascii_digit()
                        || self.src[self.pos] == b'_')
                {
                    self.pos += 1;
                }
                let member = String::from_utf8_lossy(&self.src[fstart..self.pos]).to_string();
                token = format!("{}.{}", token, member);
                if token.starts_with("Math.")
                    && self.src[self.pos..].starts_with(b"(")
                    && !self.src[self.pos..].starts_with(b"()")
                {
                    break;
                }
                if self.pos < self.src.len() && self.src[self.pos] == b'(' {
                    self.pos += 1;
                    if !self.eat(")") {
                        return Err("method args unsupported".to_string());
                    }
                    token.push_str("()");
                }
                continue;
            }
            self.pos = save;
            break;
        }
        if token == "true" {
            return Ok(Value::Bool(true));
        }
        if token == "false" {
            return Ok(Value::Bool(false));
        }
        if token == "null" {
            return Ok(Value::Null);
        }
        if self.pos < self.src.len() && self.src[self.pos] == b'(' {
            return self.eval_call(&token);
        }
        if self.skip == 0 && token.starts_with("doc[") && token.ends_with("].value") {
            let field = Self::doc_field_of(&token);
            if self.env.c.values(&field).0.is_empty() {
                return Err(MISSING_DOC_VALUE.to_string());
            }
        }
        let v = self.env.resolve_path(&token);
        if let Some(base) = token.strip_suffix(".size()") {
            let field = Self::doc_field_of(base);
            let (vals, _) = self.env.c.values(&field);
            return Ok(Value::Number(Number::from(vals.len() as i64)));
        }
        if let Some(base) = token.strip_suffix(".toUpperCase()") {
            if let Some(s) = value_to_script_string(&self.env.resolve_path(base)) {
                return Ok(Value::String(s.to_uppercase()));
            }
        }
        if let Some(base) = token.strip_suffix(".toLowerCase()") {
            if let Some(s) = value_to_script_string(&self.env.resolve_path(base)) {
                return Ok(Value::String(s.to_lowercase()));
            }
        }
        Ok(v)
    }

    fn doc_field_of(path: &str) -> String {
        let p = path.trim();
        if let Some(rest) = p.strip_prefix("doc[") {
            if let Some(i) = rest.find(']') {
                return rest[..i].trim_matches(|c| c == '\'' || c == '"').to_string();
            }
        }
        if let Some(rest) = p.strip_prefix("doc.") {
            let f = rest
                .strip_suffix(".value")
                .or_else(|| rest.strip_suffix(".size()"))
                .unwrap_or(rest);
            return f.to_string();
        }
        p.to_string()
    }

    fn eval_call(&mut self, fn_name: &str) -> Result<Value, String> {
        self.skip_space();
        if self.pos >= self.src.len() || self.src[self.pos] != b'(' {
            return Err("expected '('".to_string());
        }
        self.pos += 1;
        let mut args: Vec<Value> = Vec::new();
        self.skip_space();
        if self.pos < self.src.len() && self.src[self.pos] == b')' {
            self.pos += 1;
        } else {
            loop {
                let v = self.parse_ternary()?;
                args.push(v);
                self.skip_space();
                if self.pos < self.src.len() && self.src[self.pos] == b',' {
                    self.pos += 1;
                    continue;
                }
                if self.pos < self.src.len() && self.src[self.pos] == b')' {
                    self.pos += 1;
                    break;
                }
                return Err("bad call args".to_string());
            }
        }
        match fn_name {
            "Math.max" if args.len() == 2 => {
                Ok(num_f64(script_float(&args[0]).max(script_float(&args[1]))))
            }
            "Math.min" if args.len() == 2 => {
                Ok(num_f64(script_float(&args[0]).min(script_float(&args[1]))))
            }
            "Math.abs" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).abs())),
            "Math.sqrt" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).sqrt())),
            "Math.floor" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).floor())),
            "Math.ceil" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).ceil())),
            "Math.round" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).round())),
            "Math.log" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).ln())),
            "Math.exp" if args.len() == 1 => Ok(num_f64(script_float(&args[0]).exp())),
            "Math.pow" if args.len() == 2 => Ok(num_f64(
                script_float(&args[0]).powf(script_float(&args[1])),
            )),
            _ => match crate::script_ctx::score_function(fn_name, &args, self.env.c) {
                Some(f) => Ok(num_f64(f)),
                None => Err(format!("unknown function [{}]", fn_name)),
            },
        }
    }
}

#[allow(dead_code)] // no longer used by _execute, which runs on the pl_* engine
pub fn format_script_value(v: &Value) -> String {
    match v {
        Value::Null => "null".to_string(),
        Value::Bool(b) => b.to_string(),
        Value::String(s) => s.clone(),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                i.to_string()
            } else if let Some(u) = n.as_u64() {
                u.to_string()
            } else {
                let f = n.as_f64().unwrap_or(0.0);
                if f.is_infinite() {
                    if f.is_sign_negative() { "-Infinity".to_string() } else { "Infinity".to_string() }
                } else if f.is_nan() {
                    "NaN".to_string()
                } else {
                    format_number(f)
                }
            }
        }
        Value::Array(a) => format!(
            "[{}]",
            a.iter().map(format_script_value).collect::<Vec<_>>().join(", ")
        ),
        Value::Object(m) => format!(
            "{{{}}}",
            m.iter()
                .map(|(k, v)| format!("{}={}", k, format_script_value(v)))
                .collect::<Vec<_>>()
                .join(", ")
        ),
    }
}

pub fn script_params_of(bm: &serde_json::Map<String, Value>) -> serde_json::Map<String, Value> {
    bm.get("params")
        .and_then(|p| p.as_object())
        .cloned()
        .unwrap_or_default()
}

pub fn _value_to_script_string_public(v: &Value) -> Option<String> {
    value_to_script_string(v)
}
