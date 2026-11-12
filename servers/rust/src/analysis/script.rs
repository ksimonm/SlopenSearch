//! The Painless subset used by `condition` and `predicate_token_filter`
//! scripts: expressions over the `token` object (getTerm(), getPosition(),
//! getType(), ...), with literals, arithmetic, comparisons and boolean logic.

use super::uchar::utf16_len;
use super::Token;

#[derive(Debug, Clone, PartialEq)]
enum V {
    Int(i64),
    Str(String),
    Bool(bool),
}

#[derive(Debug, Clone)]
enum Tok {
    Num(i64),
    Str(String),
    Ident(String),
    Op(String),
}

fn lex(src: &str) -> Result<Vec<Tok>, String> {
    let c: Vec<char> = src.chars().collect();
    let mut i = 0;
    let mut out = Vec::new();
    while i < c.len() {
        let ch = c[i];
        if ch.is_whitespace() || ch == ';' {
            i += 1;
            continue;
        }
        if ch.is_ascii_digit() {
            let s = i;
            while i < c.len() && c[i].is_ascii_digit() {
                i += 1;
            }
            let n: String = c[s..i].iter().collect();
            out.push(Tok::Num(n.parse().map_err(|_| "bad number")?));
            // Allow an 'L' suffix.
            if i < c.len() && (c[i] == 'L' || c[i] == 'l') {
                i += 1;
            }
            continue;
        }
        if ch == '\'' || ch == '"' {
            let q = ch;
            i += 1;
            let mut s = String::new();
            while i < c.len() && c[i] != q {
                if c[i] == '\\' && i + 1 < c.len() {
                    i += 1;
                }
                s.push(c[i]);
                i += 1;
            }
            i += 1;
            out.push(Tok::Str(s));
            continue;
        }
        if ch.is_alphabetic() || ch == '_' {
            let s = i;
            while i < c.len() && (c[i].is_alphanumeric() || c[i] == '_') {
                i += 1;
            }
            out.push(Tok::Ident(c[s..i].iter().collect()));
            continue;
        }
        let two: String = c[i..(i + 2).min(c.len())].iter().collect();
        if ["==", "!=", "<=", ">=", "&&", "||"].contains(&two.as_str()) {
            out.push(Tok::Op(two));
            i += 2;
            continue;
        }
        if "()<>+-*/%!.,?:".contains(ch) {
            out.push(Tok::Op(ch.to_string()));
            i += 1;
            continue;
        }
        return Err(format!("unexpected character [{}]", ch));
    }
    Ok(out)
}

#[derive(Debug, Clone)]
pub struct Script {
    toks: Vec<Tok>,
}

impl Script {
    pub fn parse(src: &str) -> Result<Script, String> {
        let mut toks = lex(src)?;
        if let Some(Tok::Ident(r)) = toks.first() {
            if r == "return" {
                toks.remove(0);
            }
        }
        let s = Script { toks };
        // Validate by evaluating against a dummy token.
        let t = Token::new("x".to_string(), 0, 1, "word", 0);
        s.eval(&t, 1)?;
        Ok(s)
    }

    /// Evaluates the script for a token; `pos_inc` is its position increment.
    pub fn test(&self, t: &Token, pos_inc: usize) -> bool {
        matches!(self.eval(t, pos_inc), Ok(V::Bool(true)))
    }

    fn eval(&self, t: &Token, pos_inc: usize) -> Result<V, String> {
        let mut p = Ev { toks: &self.toks, i: 0, t, pos_inc };
        let v = p.ternary()?;
        if p.i < self.toks.len() {
            return Err("trailing tokens".to_string());
        }
        Ok(v)
    }
}

struct Ev<'a> {
    toks: &'a [Tok],
    i: usize,
    t: &'a Token,
    pos_inc: usize,
}

impl<'a> Ev<'a> {
    fn peek_op(&self, op: &str) -> bool {
        matches!(self.toks.get(self.i), Some(Tok::Op(o)) if o == op)
    }

    fn eat(&mut self, op: &str) -> bool {
        if self.peek_op(op) {
            self.i += 1;
            true
        } else {
            false
        }
    }

    fn ternary(&mut self) -> Result<V, String> {
        let c = self.or()?;
        if self.eat("?") {
            let a = self.ternary()?;
            if !self.eat(":") {
                return Err("expected :".to_string());
            }
            let b = self.ternary()?;
            return Ok(if c == V::Bool(true) { a } else { b });
        }
        Ok(c)
    }

    fn or(&mut self) -> Result<V, String> {
        let mut l = self.and()?;
        while self.eat("||") {
            let r = self.and()?;
            l = V::Bool(l == V::Bool(true) || r == V::Bool(true));
        }
        Ok(l)
    }

    fn and(&mut self) -> Result<V, String> {
        let mut l = self.cmp()?;
        while self.eat("&&") {
            let r = self.cmp()?;
            l = V::Bool(l == V::Bool(true) && r == V::Bool(true));
        }
        Ok(l)
    }

    fn cmp(&mut self) -> Result<V, String> {
        let l = self.add()?;
        for op in ["==", "!=", "<=", ">=", "<", ">"] {
            if self.eat(op) {
                let r = self.add()?;
                return Ok(V::Bool(match op {
                    "==" => l == r,
                    "!=" => l != r,
                    _ => {
                        let (a, b) = match (&l, &r) {
                            (V::Int(a), V::Int(b)) => (*a, *b),
                            _ => return Err("cannot compare".to_string()),
                        };
                        match op {
                            "<=" => a <= b,
                            ">=" => a >= b,
                            "<" => a < b,
                            _ => a > b,
                        }
                    }
                }));
            }
        }
        Ok(l)
    }

    fn add(&mut self) -> Result<V, String> {
        let mut l = self.mul()?;
        loop {
            if self.eat("+") {
                let r = self.mul()?;
                l = match (l, r) {
                    (V::Int(a), V::Int(b)) => V::Int(a + b),
                    (a, b) => V::Str(format!("{}{}", show(&a), show(&b))),
                };
            } else if self.eat("-") {
                let r = self.mul()?;
                l = V::Int(int(&l)? - int(&r)?);
            } else {
                return Ok(l);
            }
        }
    }

    fn mul(&mut self) -> Result<V, String> {
        let mut l = self.unary()?;
        loop {
            let op = if self.peek_op("*") {
                "*"
            } else if self.peek_op("/") {
                "/"
            } else if self.peek_op("%") {
                "%"
            } else {
                return Ok(l);
            };
            self.i += 1;
            let r = self.unary()?;
            let (a, b) = (int(&l)?, int(&r)?);
            l = V::Int(match op {
                "*" => a * b,
                "/" => {
                    if b == 0 {
                        return Err("/ by zero".to_string());
                    }
                    a / b
                }
                _ => {
                    if b == 0 {
                        return Err("/ by zero".to_string());
                    }
                    a % b
                }
            });
        }
    }

    fn unary(&mut self) -> Result<V, String> {
        if self.eat("!") {
            let v = self.unary()?;
            return Ok(V::Bool(v != V::Bool(true)));
        }
        if self.eat("-") {
            let v = self.unary()?;
            return Ok(V::Int(-int(&v)?));
        }
        let mut v = self.primary()?;
        while self.eat(".") {
            let name = match self.toks.get(self.i) {
                Some(Tok::Ident(n)) => n.clone(),
                _ => return Err("expected method".to_string()),
            };
            self.i += 1;
            let args = self.args()?;
            v = method(v, &name, &args)?;
        }
        Ok(v)
    }

    fn args(&mut self) -> Result<Vec<V>, String> {
        let mut out = Vec::new();
        if !self.eat("(") {
            return Err("expected (".to_string());
        }
        if self.eat(")") {
            return Ok(out);
        }
        loop {
            out.push(self.ternary()?);
            if self.eat(")") {
                return Ok(out);
            }
            if !self.eat(",") {
                return Err("expected ,".to_string());
            }
        }
    }

    fn primary(&mut self) -> Result<V, String> {
        let tok = self.toks.get(self.i).cloned().ok_or("unexpected end")?;
        self.i += 1;
        match tok {
            Tok::Num(n) => Ok(V::Int(n)),
            Tok::Str(s) => Ok(V::Str(s)),
            Tok::Op(o) if o == "(" => {
                let v = self.ternary()?;
                if !self.eat(")") {
                    return Err("expected )".to_string());
                }
                Ok(v)
            }
            Tok::Ident(id) => match id.as_str() {
                "true" => Ok(V::Bool(true)),
                "false" => Ok(V::Bool(false)),
                "token" => {
                    if !self.eat(".") {
                        return Err("expected token method".to_string());
                    }
                    let name = match self.toks.get(self.i) {
                        Some(Tok::Ident(n)) => n.clone(),
                        _ => return Err("expected method".to_string()),
                    };
                    self.i += 1;
                    let _ = self.args()?;
                    let t = self.t;
                    Ok(match name.as_str() {
                        "getTerm" => V::Str(t.term.clone()),
                        "getPosition" => V::Int(t.pos as i64),
                        "getPositionIncrement" => V::Int(self.pos_inc as i64),
                        "getPositionLength" => V::Int(t.pos_len as i64),
                        "getStartOffset" => V::Int(t.start as i64),
                        "getEndOffset" => V::Int(t.end as i64),
                        "getType" => V::Str(t.typ.clone()),
                        "isKeyword" => V::Bool(t.keyword),
                        _ => return Err(format!("unknown token method [{}]", name)),
                    })
                }
                _ => Err(format!("unknown variable [{}]", id)),
            },
            _ => Err("unexpected token".to_string()),
        }
    }
}

fn int(v: &V) -> Result<i64, String> {
    match v {
        V::Int(i) => Ok(*i),
        _ => Err("not a number".to_string()),
    }
}

fn show(v: &V) -> String {
    match v {
        V::Int(i) => i.to_string(),
        V::Str(s) => s.clone(),
        V::Bool(b) => b.to_string(),
    }
}

fn method(v: V, name: &str, args: &[V]) -> Result<V, String> {
    let s = match &v {
        V::Str(s) => s.clone(),
        _ => return Err(format!("no method [{}]", name)),
    };
    let arg = |i: usize| -> String { args.get(i).map(show).unwrap_or_default() };
    Ok(match name {
        "length" => V::Int(utf16_len(&s) as i64),
        "toString" => V::Str(s),
        "isEmpty" => V::Bool(s.is_empty()),
        "toLowerCase" => V::Str(s.to_lowercase()),
        "toUpperCase" => V::Str(s.to_uppercase()),
        "equals" => V::Bool(s == arg(0)),
        "startsWith" => V::Bool(s.starts_with(&arg(0))),
        "endsWith" => V::Bool(s.ends_with(&arg(0))),
        "contains" => V::Bool(s.contains(&arg(0))),
        "charAt" => {
            let i = int(args.first().unwrap_or(&V::Int(0)))? as usize;
            V::Str(s.chars().nth(i).map(|c| c.to_string()).unwrap_or_default())
        }
        _ => return Err(format!("no method [{}]", name)),
    })
}
