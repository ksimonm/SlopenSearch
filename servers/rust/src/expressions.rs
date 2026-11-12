// Lucene expressions ("lang": "expression"): a JavaScript-like numeric
// language. Everything is a double; variables are bound at link time to doc
// values, _score, _value or numeric params. Port of servers/go/pl_expr.go.

use crate::eval::{mapping_field_def, QueryCtx};
use crate::script_ctx::ScriptErr;
use crate::store::Index;
use serde_json::{Map, Value};

#[derive(Debug, Clone)]
enum Node {
    Num(f64),
    Var(usize), // index into Expr::vars
    Unary(char, Box<Node>),
    Binary(&'static str, Box<Node>, Box<Node>),
    Cond(Box<Node>, Box<Node>, Box<Node>),
    Call(String, Vec<Node>, usize),
}

#[derive(Debug, Clone)]
struct Var {
    name: String,
    pos: usize,
}

/// A compiled expression.
#[derive(Debug, Clone)]
pub struct Expr {
    root: Node,
    vars: Vec<Var>,
}

/// A compile or link error of an expression.
#[derive(Debug, Clone)]
pub struct ExprErr {
    pub link: bool,
    pub reason: String,
    pub offset: usize,
    pub eof: bool,
}

impl ExprErr {
    pub fn script_err(&self, src: &str, name: &str) -> ScriptErr {
        let mut se = ScriptErr::new(!self.link, "parse_exception", &self.reason);
        se.link = self.link;
        se.lang = "expression".to_string();
        se.script = name.to_string();
        if self.eof {
            se.inner = Some(("no_viable_alt_exception".to_string(), None));
        }
        let off = self.offset.min(src.len());
        se.stack = vec![src.to_string(), format!("{}^---- HERE", " ".repeat(off))];
        se
    }
}

const LEVELS: &[&[&str]] = &[
    &["||"],
    &["&&"],
    &["|"],
    &["^"],
    &["&"],
    &["==", "!="],
    &["<=", ">=", "<", ">"],
    &[">>>", "<<", ">>"],
    &["+", "-"],
    &["*", "/", "%"],
];

struct Parser<'a> {
    src: &'a str,
    b: &'a [u8],
    i: usize,
    vars: Vec<Var>,
}

fn ident_start(c: u8) -> bool {
    c == b'_' || c == b'$' || c.is_ascii_alphabetic()
}

fn ident_char(c: u8) -> bool {
    ident_start(c) || c.is_ascii_digit()
}

impl<'a> Parser<'a> {
    fn skip(&mut self) {
        while self.i < self.b.len() && matches!(self.b[self.i], b' ' | b'\t' | b'\n' | b'\r') {
            self.i += 1;
        }
    }

    fn peek_op(&mut self, ops: &[&'static str]) -> Option<&'static str> {
        self.skip();
        let rest = &self.src[self.i..];
        for op in ops {
            if let Some(after) = rest.strip_prefix(op) {
                if (*op == "<" || *op == ">")
                    && (after.starts_with('<') || after.starts_with('>') || after.starts_with('='))
                {
                    continue;
                }
                if (*op == "&" && after.starts_with('&')) || (*op == "|" && after.starts_with('|')) {
                    continue;
                }
                if *op == "!" && after.starts_with('=') {
                    continue;
                }
                return Some(op);
            }
        }
        None
    }

    fn err_here(&mut self) -> ExprErr {
        self.skip();
        if self.i >= self.b.len() {
            return ExprErr { link: false, reason: "unexpected end of expression".into(), offset: self.b.len(), eof: true };
        }
        let c = self.src[self.i..].chars().next().unwrap_or(' ');
        ExprErr {
            link: false,
            reason: format!("unexpected character '{}' on line (1) position ({})", c, self.i),
            offset: self.i,
            eof: false,
        }
    }

    fn cond(&mut self) -> Result<Node, ExprErr> {
        let c = self.binary(0)?;
        if self.peek_op(&["?"]).is_some() {
            self.i += 1;
            let a = self.cond()?;
            if self.peek_op(&[":"]).is_none() {
                return Err(self.err_here());
            }
            self.i += 1;
            let b = self.cond()?;
            return Ok(Node::Cond(Box::new(c), Box::new(a), Box::new(b)));
        }
        Ok(c)
    }

    fn binary(&mut self, level: usize) -> Result<Node, ExprErr> {
        if level >= LEVELS.len() {
            return self.unary();
        }
        let mut l = self.binary(level + 1)?;
        while let Some(op) = self.peek_op(LEVELS[level]) {
            self.i += op.len();
            let r = self.binary(level + 1)?;
            l = Node::Binary(op, Box::new(l), Box::new(r));
        }
        Ok(l)
    }

    fn unary(&mut self) -> Result<Node, ExprErr> {
        if let Some(op) = self.peek_op(&["!", "~", "-", "+"]) {
            self.i += 1;
            let x = self.unary()?;
            if op == "+" {
                return Ok(x);
            }
            return Ok(Node::Unary(op.chars().next().unwrap(), Box::new(x)));
        }
        self.primary()
    }

    fn primary(&mut self) -> Result<Node, ExprErr> {
        self.skip();
        if self.i >= self.b.len() {
            return Err(self.err_here());
        }
        let c = self.b[self.i];
        if c == b'(' {
            self.i += 1;
            let x = self.cond()?;
            if self.peek_op(&[")"]).is_none() {
                return Err(self.err_here());
            }
            self.i += 1;
            return Ok(x);
        }
        if c.is_ascii_digit() || c == b'.' {
            let mut j = self.i;
            let rest = &self.src[j..];
            if rest.starts_with("0x") || rest.starts_with("0X") {
                j += 2;
                while j < self.b.len() && self.b[j].is_ascii_hexdigit() {
                    j += 1;
                }
                let v = i64::from_str_radix(&self.src[self.i + 2..j], 16).unwrap_or(0);
                self.i = j;
                return Ok(Node::Num(v as f64));
            }
            while j < self.b.len() && (self.b[j].is_ascii_digit() || self.b[j] == b'.') {
                j += 1;
            }
            if j < self.b.len() && (self.b[j] == b'e' || self.b[j] == b'E') {
                j += 1;
                if j < self.b.len() && (self.b[j] == b'+' || self.b[j] == b'-') {
                    j += 1;
                }
                while j < self.b.len() && self.b[j].is_ascii_digit() {
                    j += 1;
                }
            }
            let text = &self.src[self.i..j];
            let mut v: f64 = match text.parse() {
                Ok(v) => v,
                Err(_) => {
                    return Err(ExprErr {
                        link: false,
                        reason: format!("unexpected character '{}'", text),
                        offset: self.i,
                        eof: false,
                    })
                }
            };
            if text.len() > 1 && text.starts_with('0') && !text.contains(['.', 'e', 'E']) {
                v = i64::from_str_radix(text, 8).unwrap_or(0) as f64;
            }
            self.i = j;
            return Ok(Node::Num(v));
        }
        if ident_start(c) {
            let start = self.i;
            let mut j = self.i;
            while j < self.b.len() && ident_char(self.b[j]) {
                j += 1;
            }
            let ident = self.src[self.i..j].to_string();
            let mut k = j;
            while k < self.b.len() && self.b[k] == b' ' {
                k += 1;
            }
            if k < self.b.len() && self.b[k] == b'(' {
                self.i = k + 1;
                let mut args = Vec::new();
                if self.peek_op(&[")"]).is_some() {
                    self.i += 1;
                    return Ok(Node::Call(ident, args, start));
                }
                loop {
                    args.push(self.cond()?);
                    if self.peek_op(&[","]).is_some() {
                        self.i += 1;
                        continue;
                    }
                    if self.peek_op(&[")"]).is_none() {
                        return Err(self.err_here());
                    }
                    self.i += 1;
                    return Ok(Node::Call(ident, args, start));
                }
            }
            // variable: ID ([...] | .ID)* with an optional trailing ()
            while j < self.b.len() {
                if self.b[j] == b'[' {
                    match self.src[j..].find(']') {
                        Some(end) => {
                            j += end + 1;
                            continue;
                        }
                        None => {
                            self.i = self.b.len();
                            return Err(self.err_here());
                        }
                    }
                }
                if self.b[j] == b'.' && j + 1 < self.b.len() && ident_start(self.b[j + 1]) {
                    j += 1;
                    while j < self.b.len() && ident_char(self.b[j]) {
                        j += 1;
                    }
                    if self.src[j..].starts_with("()") {
                        j += 2;
                    }
                    continue;
                }
                break;
            }
            self.vars.push(Var { name: self.src[start..j].to_string(), pos: start });
            self.i = j;
            return Ok(Node::Var(self.vars.len() - 1));
        }
        Err(self.err_here())
    }
}

fn func_arity(f: &str) -> Option<usize> {
    Some(match f {
        "abs" | "acos" | "acosh" | "asin" | "asinh" | "atan" | "atanh" | "ceil" | "cos" | "cosh"
        | "exp" | "floor" | "ln" | "log10" | "sin" | "sinh" | "sqrt" | "tan" | "tanh" => 1,
        "atan2" | "logn" | "max" | "min" | "pow" => 2,
        "haversin" => 4,
        _ => return None,
    })
}

fn check_funcs(n: &Node, src: &str) -> Result<(), ExprErr> {
    match n {
        Node::Call(f, args, pos) => {
            let ar = match func_arity(f) {
                Some(a) => a,
                None => {
                    return Err(ExprErr {
                        link: false,
                        reason: format!("Invalid expression '{}': Unrecognized function call ({}).", src, f),
                        offset: *pos,
                        eof: false,
                    })
                }
            };
            if ar != args.len() {
                return Err(ExprErr {
                    link: false,
                    reason: format!(
                        "Invalid expression '{}': Expected ({}) arguments for function call ({}), but found ({}).",
                        src,
                        ar,
                        f,
                        args.len()
                    ),
                    offset: *pos,
                    eof: false,
                });
            }
            for a in args {
                check_funcs(a, src)?;
            }
        }
        Node::Unary(_, x) => check_funcs(x, src)?,
        Node::Binary(_, l, r) => {
            check_funcs(l, src)?;
            check_funcs(r, src)?;
        }
        Node::Cond(c, a, b) => {
            check_funcs(c, src)?;
            check_funcs(a, src)?;
            check_funcs(b, src)?;
        }
        _ => {}
    }
    Ok(())
}

/// Compiles an expression and checks its function calls.
pub fn compile(src: &str) -> Result<Expr, ExprErr> {
    let mut p = Parser { src, b: src.as_bytes(), i: 0, vars: Vec::new() };
    let root = p.cond()?;
    p.skip();
    if p.i < p.b.len() {
        return Err(p.err_here());
    }
    check_funcs(&root, src)?;
    Ok(Expr { root, vars: p.vars })
}

/// Contexts that the expression engine supports.
pub fn supports_context(ctx: &str) -> bool {
    matches!(
        ctx,
        "aggregation_selector" | "aggs" | "bucket_aggregation" | "field" | "filter" | "number_sort" | "score"
            | "terms_set"
    )
}

// ---- linking ----

#[derive(Debug, Clone, Copy, PartialEq)]
enum Kind {
    Num,
    Date,
    Geo,
}

#[derive(Debug, Clone)]
enum Agg {
    First,
    Max,
    Sum,
    Avg,
    Median,
    Count,
    Empty,
    DatePart(&'static str),
    GeoLat,
    GeoLon,
}

#[derive(Debug, Clone)]
enum Binding {
    Score,
    Value,
    Const(f64),
    Doc { field: String, kind: Kind, agg: Agg },
}

/// An expression bound to an index and its parameters.
#[derive(Debug, Clone)]
pub struct Linked {
    expr: Expr,
    binds: Vec<Binding>,
}

/// Splits doc['field'].member into (field, member, offset of the field name).
fn parse_doc_var(name: &str) -> Option<(String, String, usize)> {
    let rest = name.strip_prefix("doc[")?;
    let end = rest.find(']')?;
    let mut inner = &rest[..end];
    let mut off = 4;
    if inner.len() >= 2 && (inner.starts_with('\'') || inner.starts_with('"')) {
        inner = &inner[1..inner.len() - 1];
        off = 5;
    }
    let member = rest[end + 1..].strip_prefix('.').unwrap_or(&rest[end + 1..]);
    Some((inner.to_string(), member.to_string(), off))
}

fn is_numeric_type(t: &str) -> bool {
    matches!(
        t,
        "long" | "integer" | "short" | "byte" | "double" | "float" | "half_float" | "scaled_float"
            | "unsigned_long" | "boolean"
    )
}

fn link_err(reason: String, offset: usize) -> ExprErr {
    ExprErr { link: true, reason, offset, eof: false }
}

fn doc_binding(field: &str, kind: Kind, member: &str, pos: usize) -> Result<Binding, ExprErr> {
    let mk = |agg| Ok(Binding::Doc { field: field.to_string(), kind, agg });
    if kind == Kind::Geo {
        return match member {
            "lat" => mk(Agg::GeoLat),
            "lon" => mk(Agg::GeoLon),
            "empty" => mk(Agg::Empty),
            _ => Err(link_err(
                format!("Member variable [{}] does not exist for geo field [{}].", member, field),
                pos,
            )),
        };
    }
    match member {
        "" | "value" | "min()" => return mk(Agg::First),
        "empty" => return mk(Agg::Empty),
        "length" | "count()" => return mk(Agg::Count),
        "max()" => return mk(Agg::Max),
        "sum()" => return mk(Agg::Sum),
        "avg()" => return mk(Agg::Avg),
        "median()" => return mk(Agg::Median),
        _ => {}
    }
    if kind == Kind::Date && (member.starts_with("date.") || member.starts_with("get")) {
        let part = member.strip_prefix("date.").unwrap_or(member);
        let part = part.strip_prefix("get").unwrap_or(part);
        let part = part.strip_suffix("()").unwrap_or(part);
        let mut p = part.to_string();
        if let Some(c) = p.get(..1) {
            p = c.to_lowercase() + &p[1..];
        }
        let known: &[&'static str] = &[
            "year",
            "month",
            "monthOfYear",
            "dayOfMonth",
            "dayOfYear",
            "dayOfWeek",
            "hourOfDay",
            "minutes",
            "minuteOfHour",
            "seconds",
            "secondOfMinute",
            "millis",
        ];
        if let Some(k) = known.iter().find(|k| **k == p) {
            return mk(Agg::DatePart(k));
        }
    }
    Err(link_err(
        format!("Member method [{}] does not exist for numeric field [{}].", member, field),
        pos,
    ))
}

/// Binds the variables of an expression for an index. `params` holds the
/// numeric parameters (or bucket path values).
pub fn link(
    e: &Expr,
    idx: Option<&Index>,
    params: &Map<String, Value>,
    allow_value: bool,
    allow_score: bool,
) -> Result<Linked, ExprErr> {
    let mut binds = Vec::with_capacity(e.vars.len());
    for v in &e.vars {
        let name = v.name.as_str();
        if name == "_score" && allow_score {
            binds.push(Binding::Score);
            continue;
        }
        if name == "_value" && allow_value {
            binds.push(Binding::Value);
            continue;
        }
        if let Some((field, member, foff)) = parse_doc_var(name) {
            let idx = match idx {
                Some(i) => i,
                None => return Err(link_err("Unknown variable [doc]".into(), v.pos)),
            };
            let def = match mapping_field_def(idx, &field) {
                Some(d) => d,
                None => {
                    return Err(link_err(
                        format!("Field [{}] does not exist in mappings", field),
                        v.pos + foff,
                    ))
                }
            };
            let ft = def.get("type").and_then(|t| t.as_str()).unwrap_or("");
            let kind = if ft == "date" || ft == "date_nanos" {
                Kind::Date
            } else if is_numeric_type(ft) {
                Kind::Num
            } else if ft == "geo_point" {
                Kind::Geo
            } else {
                return Err(link_err(
                    format!("Field [{}] must be numeric, date, or geopoint", field),
                    v.pos + foff,
                ));
            };
            binds.push(doc_binding(&field, kind, &member, v.pos)?);
            continue;
        }
        let base = match name.find(['.', '[']) {
            Some(i) => &name[..i],
            None => name,
        };
        if base == name {
            if let Some(pv) = params.get(base) {
                match pv {
                    Value::Number(n) => {
                        binds.push(Binding::Const(n.as_f64().unwrap_or(0.0)));
                        continue;
                    }
                    _ => {
                        return Err(link_err(format!("Parameter [{}] must be a numeric type", base), v.pos));
                    }
                }
            }
        }
        return Err(link_err(format!("Unknown variable [{}]", base), v.pos));
    }
    Ok(Linked { expr: e.clone(), binds })
}

fn numeric_values(qc: &QueryCtx, field: &str) -> Vec<f64> {
    let (vals, _) = qc.values(field);
    let mut out: Vec<f64> = vals
        .iter()
        .filter_map(|v| match v {
            Value::Bool(b) => Some(if *b { 1.0 } else { 0.0 }),
            Value::Number(n) => n.as_f64(),
            Value::String(s) => s.parse().ok(),
            _ => None,
        })
        .collect();
    out.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
    out
}

fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146097 + doe - 719468
}

fn date_part(part: &str, ms: i64) -> f64 {
    let days = ms.div_euclid(86_400_000);
    let rem = ms.rem_euclid(86_400_000);
    let (y, m, d) = crate::search::civil_from_days(days);
    let jan1 = days_from_civil(y, 1, 1);
    match part {
        "year" => y as f64,
        "month" => (m - 1) as f64,
        "monthOfYear" => m as f64,
        "dayOfMonth" => d as f64,
        "dayOfYear" => (days - jan1 + 1) as f64,
        "dayOfWeek" => ((days + 3).rem_euclid(7) + 1) as f64,
        "hourOfDay" => (rem / 3_600_000) as f64,
        "minutes" | "minuteOfHour" => (rem / 60_000 % 60) as f64,
        "seconds" | "secondOfMinute" => (rem / 1000 % 60) as f64,
        "millis" => ms as f64,
        _ => 0.0,
    }
}

fn bind_value(b: &Binding, qc: Option<&QueryCtx>, score: f64, value: &Value) -> f64 {
    match b {
        Binding::Score => score,
        Binding::Value => match value {
            Value::Number(n) => n.as_f64().unwrap_or(0.0),
            Value::Bool(b) => {
                if *b {
                    1.0
                } else {
                    0.0
                }
            }
            Value::String(s) => s.parse().unwrap_or(0.0),
            _ => 0.0,
        },
        Binding::Const(f) => *f,
        Binding::Doc { field, kind, agg } => {
            let qc = match qc {
                Some(q) => q,
                None => return 0.0,
            };
            if *kind == Kind::Geo {
                let (vals, _) = qc.values(field);
                let first = vals.first();
                return match agg {
                    Agg::Empty => {
                        if vals.is_empty() {
                            1.0
                        } else {
                            0.0
                        }
                    }
                    Agg::GeoLat | Agg::GeoLon => {
                        let key = if matches!(agg, Agg::GeoLat) { "lat" } else { "lon" };
                        first
                            .and_then(|v| v.get(key))
                            .and_then(|v| v.as_f64())
                            .unwrap_or(0.0)
                    }
                    _ => 0.0,
                };
            }
            let v = numeric_values(qc, field);
            match agg {
                Agg::Empty => return if v.is_empty() { 1.0 } else { 0.0 },
                Agg::Count => return v.len() as f64,
                _ => {}
            }
            if v.is_empty() {
                return 0.0;
            }
            match agg {
                Agg::First => v[0],
                Agg::Max => v[v.len() - 1],
                Agg::Sum => v.iter().sum(),
                Agg::Avg => v.iter().sum::<f64>() / v.len() as f64,
                Agg::Median => {
                    let n = v.len();
                    if n % 2 == 1 {
                        v[n / 2]
                    } else {
                        (v[n / 2 - 1] + v[n / 2]) / 2.0
                    }
                }
                Agg::DatePart(p) => date_part(p, v[0] as i64),
                _ => 0.0,
            }
        }
    }
}

fn b2f(b: bool) -> f64 {
    if b {
        1.0
    } else {
        0.0
    }
}

fn call(f: &str, a: &[f64]) -> f64 {
    match f {
        "abs" => a[0].abs(),
        "acos" => a[0].acos(),
        "acosh" => a[0].acosh(),
        "asin" => a[0].asin(),
        "asinh" => a[0].asinh(),
        "atan" => a[0].atan(),
        "atan2" => a[0].atan2(a[1]),
        "atanh" => a[0].atanh(),
        "ceil" => a[0].ceil(),
        "cos" => a[0].cos(),
        "cosh" => a[0].cosh(),
        "exp" => a[0].exp(),
        "floor" => a[0].floor(),
        "ln" => a[0].ln(),
        "log10" => a[0].log10(),
        "logn" => a[1].ln() / a[0].ln(),
        "max" => a[0].max(a[1]),
        "min" => a[0].min(a[1]),
        "pow" => a[0].powf(a[1]),
        "sin" => a[0].sin(),
        "sinh" => a[0].sinh(),
        "sqrt" => a[0].sqrt(),
        "tan" => a[0].tan(),
        "tanh" => a[0].tanh(),
        "haversin" => {
            let r = std::f64::consts::PI / 180.0;
            let (lat1, lon1, lat2, lon2) = (a[0] * r, a[1] * r, a[2] * r, a[3] * r);
            let h = ((lat2 - lat1) / 2.0).sin().powi(2)
                + lat1.cos() * lat2.cos() * ((lon2 - lon1) / 2.0).sin().powi(2);
            2.0 * 6371.0087714 * h.sqrt().asin()
        }
        _ => 0.0,
    }
}

impl Linked {
    /// Computes the expression for one document.
    pub fn eval(&self, qc: Option<&QueryCtx>, score: f64, value: &Value) -> f64 {
        self.ev(&self.expr.root, qc, score, value)
    }

    fn ev(&self, n: &Node, qc: Option<&QueryCtx>, score: f64, value: &Value) -> f64 {
        match n {
            Node::Num(v) => *v,
            Node::Var(i) => bind_value(&self.binds[*i], qc, score, value),
            Node::Unary(op, x) => {
                let x = self.ev(x, qc, score, value);
                match op {
                    '-' => -x,
                    '!' => b2f(x == 0.0),
                    '~' => !(x as i64) as f64,
                    _ => x,
                }
            }
            Node::Cond(c, a, b) => {
                if self.ev(c, qc, score, value) != 0.0 {
                    self.ev(a, qc, score, value)
                } else {
                    self.ev(b, qc, score, value)
                }
            }
            Node::Binary(op, l, r) => {
                if *op == "&&" {
                    return b2f(self.ev(l, qc, score, value) != 0.0 && self.ev(r, qc, score, value) != 0.0);
                }
                if *op == "||" {
                    return b2f(self.ev(l, qc, score, value) != 0.0 || self.ev(r, qc, score, value) != 0.0);
                }
                let a = self.ev(l, qc, score, value);
                let b = self.ev(r, qc, score, value);
                let sh = (b as i64 & 63) as u32;
                match *op {
                    "+" => a + b,
                    "-" => a - b,
                    "*" => a * b,
                    "/" => a / b,
                    "%" => a % b,
                    "==" => b2f(a == b),
                    "!=" => b2f(a != b),
                    "<" => b2f(a < b),
                    "<=" => b2f(a <= b),
                    ">" => b2f(a > b),
                    ">=" => b2f(a >= b),
                    "&" => ((a as i64) & (b as i64)) as f64,
                    "|" => ((a as i64) | (b as i64)) as f64,
                    "^" => ((a as i64) ^ (b as i64)) as f64,
                    "<<" => (a as i64).wrapping_shl(sh) as f64,
                    ">>" => (a as i64).wrapping_shr(sh) as f64,
                    ">>>" => ((a as i64 as u64) >> sh) as f64,
                    _ => 0.0,
                }
            }
            Node::Call(f, args, _) => {
                let vals: Vec<f64> = args.iter().map(|a| self.ev(a, qc, score, value)).collect();
                call(f, &vals)
            }
        }
    }
}
