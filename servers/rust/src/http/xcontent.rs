//! Non-JSON request bodies and response formats. OpenSearch parses YAML and
//! CBOR request bodies with the matching XContent parser; here they are
//! transcoded to JSON text before routing (keeping key order), and the
//! original format is remembered in a request header so that the response
//! is written in it too (`response_format`, `to_cbor`).

use axum::http::HeaderMap;
use base64::prelude::{Engine as _, BASE64_STANDARD};
use serde_json::Value;
use yaml_rust2::parser::{Event, Parser};
use yaml_rust2::scanner::TScalarStyle;

use crate::errors::ApiError;
use crate::http::Req;
use crate::painless::value::{fmt_double, fmt_float};
use crate::resp::encode_string;

/// The XContent type of a transcoded request body ("yaml", "yaml-undetected",
/// "cbor").
const FORMAT_HEADER: &str = "x-slop-request-xcontent";
/// The parse error of a body that could not be transcoded.
const ERROR_HEADER: &str = "x-slop-request-xcontent-error";

const MAX_DEPTH: usize = 1000;

/// "yaml" or "cbor" for a body Content-Type of that kind.
fn body_kind(headers: &HeaderMap) -> Option<&'static str> {
    let ct = headers.get("content-type").and_then(|v| v.to_str().ok()).unwrap_or("");
    let base = ct.split(';').next().unwrap_or("").trim().to_ascii_lowercase();
    match base.as_str() {
        "application/yaml" | "application/x-yaml" | "text/yaml" | "application/yml"
        | "application/vnd.opensearch+yaml" => Some("yaml"),
        "application/cbor" | "application/vnd.opensearch+cbor" => Some("cbor"),
        _ => None,
    }
}

/// Turns a YAML or CBOR request body into JSON.
pub fn transcode_request(req: &mut Req) {
    req.headers.remove(FORMAT_HEADER);
    req.headers.remove(ERROR_HEADER);
    if req.body.is_empty() {
        return;
    }
    let kind = match body_kind(&req.headers) {
        Some(k) => k,
        None => return,
    };
    let (marker, out) = if kind == "yaml" {
        // XContentFactory only detects YAML by its document start
        let marker = if req.body.starts_with(b"---") { "yaml" } else { "yaml-undetected" };
        (marker, yaml_to_json(&req.body))
    } else {
        ("cbor", cbor_to_json(&req.body))
    };
    req.headers.insert(FORMAT_HEADER, marker.parse().unwrap());
    match out {
        Ok(json) => {
            req.body = json.into_bytes();
            req.headers.insert("content-type", "application/json".parse().unwrap());
        }
        Err(e) => {
            let e = e.replace('\n', "\\n");
            let v = e.parse().unwrap_or_else(|_| "failed to parse".parse().unwrap());
            req.headers.insert(ERROR_HEADER, v);
        }
    }
}

/// The error of a YAML or CBOR document source that cannot be indexed:
/// undetectable YAML, unparsable content, or a root that is not an object.
pub fn doc_error(req: &Req) -> Option<ApiError> {
    let kind = req.headers.get(FORMAT_HEADER)?.to_str().ok()?;
    let failed = || ApiError::new("mapper_parsing_exception", "failed to parse", 400);
    if kind == "yaml-undetected" {
        return Some(failed().caused_by(ApiError::new(
            "not_x_content_exception",
            "Compressor detection can only be called on some xcontent bytes or compressed xcontent bytes",
            400,
        )));
    }
    if let Some(msg) = req.headers.get(ERROR_HEADER).and_then(|v| v.to_str().ok()) {
        return Some(failed().caused_by(ApiError::new("json_parse_exception", &msg.replace("\\n", "\n"), 400)));
    }
    let first = req.body.iter().find(|b| !b.is_ascii_whitespace());
    if first.is_some_and(|b| *b != b'{') {
        return Some(ApiError::new("mapper_parsing_exception", "Malformed content, must start with an object", 400));
    }
    None
}

/// The response XContent type, like AbstractRestChannel.newBuilder: the
/// format parameter, then the Accept header, then the type of the request
/// body, then JSON.
pub fn response_format(req: &Req) -> &'static str {
    match req.param("format").map(|f| f.to_ascii_lowercase()).as_deref() {
        Some("json") => return "json",
        Some("yaml") => return "yaml",
        Some("cbor") => return "cbor",
        Some("smile") => return "smile",
        _ => {}
    }
    let acc = req.headers.get("accept").and_then(|v| v.to_str().ok()).unwrap_or("").to_ascii_lowercase();
    for f in ["yaml", "cbor", "smile", "json"] {
        if acc.contains(f) {
            return f;
        }
    }
    match req.headers.get(FORMAT_HEADER).and_then(|v| v.to_str().ok()) {
        Some("yaml") | Some("yaml-undetected") => "yaml",
        Some("cbor") => "cbor",
        _ => "json",
    }
}

// ---------------------------------------------------------------- YAML

/// Converts the first YAML document to JSON text, resolving plain scalars
/// the way Jackson's YAML parser (SnakeYAML, YAML 1.1) does.
pub fn yaml_to_json(src: &[u8]) -> Result<String, String> {
    let text = std::str::from_utf8(src).map_err(|e| e.to_string())?;
    let mut c = YamlConv { p: Parser::new_from_str(text), anchors: Vec::new(), out: String::new() };
    loop {
        match c.next()? {
            Event::StreamStart | Event::DocumentStart => continue,
            Event::StreamEnd | Event::DocumentEnd => return Err("no content".to_string()),
            ev => {
                c.node(ev, 0)?;
                return Ok(c.out);
            }
        }
    }
}

struct YamlConv<'a> {
    p: Parser<std::str::Chars<'a>>,
    anchors: Vec<(usize, String)>,
    out: String,
}

impl YamlConv<'_> {
    fn next(&mut self) -> Result<Event, String> {
        self.p.next_token().map(|(ev, _)| ev).map_err(|e| e.to_string())
    }

    fn anchored(&mut self, anchor: usize, start: usize) {
        if anchor > 0 {
            let text = self.out[start..].to_string();
            self.anchors.push((anchor, text));
        }
    }

    fn node(&mut self, ev: Event, depth: usize) -> Result<(), String> {
        if depth > MAX_DEPTH {
            return Err("document nested too deeply".to_string());
        }
        let start = self.out.len();
        match ev {
            Event::Scalar(v, style, anchor, tag) => {
                let as_str = tag.as_ref().is_some_and(|t| t.suffix == "str" || t.suffix == "tag:yaml.org,2002:str");
                if as_str || style != TScalarStyle::Plain {
                    self.out.push_str(&encode_string(&v));
                } else {
                    self.out.push_str(&yaml_plain_scalar(&v));
                }
                self.anchored(anchor, start);
            }
            Event::SequenceStart(anchor, _) => {
                self.out.push('[');
                let mut first = true;
                loop {
                    let ev = self.next()?;
                    if matches!(ev, Event::SequenceEnd) {
                        break;
                    }
                    if !first {
                        self.out.push(',');
                    }
                    first = false;
                    self.node(ev, depth + 1)?;
                }
                self.out.push(']');
                self.anchored(anchor, start);
            }
            Event::MappingStart(anchor, _) => {
                self.out.push('{');
                let mut seen: Vec<String> = Vec::new();
                loop {
                    let key = match self.next()? {
                        Event::MappingEnd => break,
                        Event::Scalar(k, ..) => k,
                        Event::Alias(id) => {
                            let t = self.alias(id)?;
                            serde_json::from_str::<Value>(&t)
                                .ok()
                                .and_then(|v| v.as_str().map(|s| s.to_string()))
                                .unwrap_or(t)
                        }
                        _ => return Err("only scalar mapping keys are supported".to_string()),
                    };
                    if seen.contains(&key) {
                        return Err(format!("Duplicate field '{}'", key));
                    }
                    if !seen.is_empty() {
                        self.out.push(',');
                    }
                    self.out.push_str(&encode_string(&key));
                    self.out.push(':');
                    seen.push(key);
                    let ev = self.next()?;
                    self.node(ev, depth + 1)?;
                }
                self.out.push('}');
                self.anchored(anchor, start);
            }
            Event::Alias(id) => {
                let t = self.alias(id)?;
                self.out.push_str(&t);
            }
            _ => return Err("unexpected YAML event".to_string()),
        }
        Ok(())
    }

    fn alias(&self, id: usize) -> Result<String, String> {
        self.anchors
            .iter()
            .rev()
            .find(|(a, _)| *a == id)
            .map(|(_, t)| t.clone())
            .ok_or_else(|| "found undefined alias".to_string())
    }
}

/// A plain scalar under YAML 1.1 rules (null, booleans, 0x/0b/0 integers,
/// floats); anything else is a string.
fn yaml_plain_scalar(t: &str) -> String {
    match t {
        "" | "~" | "null" | "Null" | "NULL" => return "null".to_string(),
        "true" | "True" | "TRUE" | "yes" | "Yes" | "YES" | "on" | "On" | "ON" => return "true".to_string(),
        "false" | "False" | "FALSE" | "no" | "No" | "NO" | "off" | "Off" | "OFF" => return "false".to_string(),
        _ => {}
    }
    if let Some(i) = yaml_int(t) {
        return i;
    }
    if yaml_float(t) {
        if let Ok(f) = t.parse::<f64>() {
            return fmt_double(f);
        }
    }
    encode_string(t)
}

/// A YAML 1.1 integer (decimal, 0 octal, 0x hex, 0b binary).
fn yaml_int(t: &str) -> Option<String> {
    let (neg, body) = match t.as_bytes().first() {
        Some(b'-') => (true, &t[1..]),
        Some(b'+') => (false, &t[1..]),
        _ => (false, t),
    };
    let all = |s: &str, f: fn(u8) -> bool| !s.is_empty() && s.bytes().all(f);
    let (digits, radix) = if body == "0" || (all(body, |b| b.is_ascii_digit()) && !body.starts_with('0')) {
        (body, 10)
    } else if let Some(h) = body.strip_prefix("0x").filter(|h| all(h, |b| b.is_ascii_hexdigit())) {
        (h, 16)
    } else if let Some(b) = body.strip_prefix("0b").filter(|b| all(b, |c| c == b'0' || c == b'1')) {
        (b, 2)
    } else if let Some(o) = body.strip_prefix('0').filter(|o| all(o, |b| (b'0'..=b'7').contains(&b))) {
        (o, 8)
    } else {
        return None;
    };
    let mag = if radix == 10 {
        digits.trim_start_matches('0').to_string()
    } else {
        u128::from_str_radix(digits, radix).ok()?.to_string()
    };
    let mag = if mag.is_empty() { "0".to_string() } else { mag };
    Some(if neg && mag != "0" { format!("-{}", mag) } else { mag })
}

/// `[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?` with a '.' or an
/// exponent.
fn yaml_float(t: &str) -> bool {
    let b = t.as_bytes();
    let mut i = 0;
    if i < b.len() && (b[i] == b'-' || b[i] == b'+') {
        i += 1;
    }
    let digits = |i: &mut usize| {
        let s = *i;
        while *i < b.len() && b[*i].is_ascii_digit() {
            *i += 1;
        }
        *i - s
    };
    let int = digits(&mut i);
    let mut frac_dot = false;
    if i < b.len() && b[i] == b'.' {
        frac_dot = true;
        i += 1;
        let f = digits(&mut i);
        if int == 0 && f == 0 {
            return false;
        }
    } else if int == 0 {
        return false;
    }
    let mut exp = false;
    if i < b.len() && (b[i] == b'e' || b[i] == b'E') {
        exp = true;
        i += 1;
        if i < b.len() && (b[i] == b'-' || b[i] == b'+') {
            i += 1;
        }
        if digits(&mut i) == 0 {
            return false;
        }
    }
    i == b.len() && (frac_dot || exp)
}

// ---------------------------------------------------------------- CBOR

/// Converts one CBOR data item to JSON text, keeping map order.
pub fn cbor_to_json(src: &[u8]) -> Result<String, String> {
    let mut d = CborDec { b: src, i: 0, out: String::new() };
    d.item(0)?;
    if src[d.i..].iter().any(|b| !b.is_ascii_whitespace()) {
        return Err("Trailing data after CBOR content".to_string());
    }
    Ok(d.out)
}

const EOF: &str = "Unexpected end-of-input";

struct CborDec<'a> {
    b: &'a [u8],
    i: usize,
    out: String,
}

impl CborDec<'_> {
    fn byte(&mut self) -> Result<u8, String> {
        let c = *self.b.get(self.i).ok_or(EOF)?;
        self.i += 1;
        Ok(c)
    }

    fn take(&mut self, n: usize) -> Result<&[u8], String> {
        if self.b.len() - self.i < n {
            return Err(EOF.to_string());
        }
        let s = &self.b[self.i..self.i + n];
        self.i += n;
        Ok(s)
    }

    /// The argument of an item head (None: indefinite length).
    fn arg(&mut self, ai: u8) -> Result<Option<u64>, String> {
        Ok(Some(match ai {
            0..=23 => ai as u64,
            24..=27 => {
                let n = 1usize << (ai - 24);
                self.take(n)?.iter().fold(0u64, |a, b| a << 8 | *b as u64)
            }
            31 => return Ok(None),
            _ => return Err(format!("Invalid CBOR additional information [{}]", ai)),
        }))
    }

    /// A definite or indefinite byte or text string.
    fn string(&mut self, major: u8, ai: u8) -> Result<Vec<u8>, String> {
        match self.arg(ai)? {
            Some(n) => {
                let n = usize::try_from(n).map_err(|_| EOF.to_string())?;
                Ok(self.take(n)?.to_vec())
            }
            None => {
                let mut out = Vec::new();
                loop {
                    let h = self.byte()?;
                    if h == 0xff {
                        return Ok(out);
                    }
                    if h >> 5 != major || h & 0x1f == 31 {
                        return Err("Invalid chunk in indefinite-length string".to_string());
                    }
                    out.extend(self.string(major, h & 0x1f)?);
                }
            }
        }
    }

    fn text(&mut self, ai: u8) -> Result<String, String> {
        String::from_utf8(self.string(3, ai)?).map_err(|_| "Invalid UTF-8 in CBOR text string".to_string())
    }

    fn item(&mut self, depth: usize) -> Result<(), String> {
        if depth > MAX_DEPTH {
            return Err("document nested too deeply".to_string());
        }
        let h = self.byte()?;
        let (major, ai) = (h >> 5, h & 0x1f);
        match major {
            0 => {
                let n = self.arg(ai)?.ok_or("Invalid indefinite-length integer")?;
                self.out.push_str(&n.to_string());
            }
            1 => {
                let n = self.arg(ai)?.ok_or("Invalid indefinite-length integer")?;
                self.out.push_str(&(-1 - n as i128).to_string());
            }
            2 => {
                let b = self.string(2, ai)?;
                self.out.push_str(&encode_string(&BASE64_STANDARD.encode(b)));
            }
            3 => {
                let s = self.text(ai)?;
                self.out.push_str(&encode_string(&s));
            }
            4 | 5 => {
                let count = self.arg(ai)?;
                let (open, close) = if major == 4 { ('[', ']') } else { ('{', '}') };
                self.out.push(open);
                let mut k = 0u64;
                loop {
                    match count {
                        Some(n) if k >= n => break,
                        None => {
                            if *self.b.get(self.i).ok_or(EOF)? == 0xff {
                                self.i += 1;
                                break;
                            }
                        }
                        _ => {}
                    }
                    if k > 0 {
                        self.out.push(',');
                    }
                    if major == 5 {
                        let key = self.key()?;
                        self.out.push_str(&encode_string(&key));
                        self.out.push(':');
                    }
                    self.item(depth + 1)?;
                    k += 1;
                }
                self.out.push(close);
            }
            6 => {
                let tag = self.arg(ai)?.ok_or("Invalid indefinite-length tag")?;
                if tag == 2 || tag == 3 {
                    let h = self.byte()?;
                    if h >> 5 != 2 {
                        return Err("Invalid bignum content".to_string());
                    }
                    let mag = self.string(2, h & 0x1f)?;
                    let mut s = big_decimal(&mag);
                    if tag == 3 {
                        // -1 - n
                        s = format!("-{}", decimal_add_one(&s));
                    }
                    self.out.push_str(&s);
                } else {
                    self.item(depth + 1)?;
                }
            }
            _ => match ai {
                20 => self.out.push_str("false"),
                21 => self.out.push_str("true"),
                25 => {
                    let b = self.take(2)?;
                    let f = half_to_f32(u16::from_be_bytes([b[0], b[1]]));
                    self.out.push_str(&fmt_float(f));
                }
                26 => {
                    let b = self.take(4)?;
                    let f = f32::from_be_bytes([b[0], b[1], b[2], b[3]]);
                    self.out.push_str(&fmt_float(f));
                }
                27 => {
                    let b = self.take(8)?;
                    let f = f64::from_be_bytes(b.try_into().unwrap());
                    self.out.push_str(&fmt_double(f));
                }
                24 => {
                    self.take(1)?;
                    self.out.push_str("null");
                }
                31 => return Err("Unexpected break".to_string()),
                28..=30 => return Err(format!("Invalid CBOR additional information [{}]", ai)),
                _ => self.out.push_str("null"),
            },
        }
        Ok(())
    }

    /// A map key: text (or a byte string, number or null rendered as text).
    fn key(&mut self) -> Result<String, String> {
        let h = *self.b.get(self.i).ok_or(EOF)?;
        let (major, ai) = (h >> 5, h & 0x1f);
        match major {
            3 => {
                self.i += 1;
                self.text(ai)
            }
            2 => {
                self.i += 1;
                Ok(String::from_utf8_lossy(&self.string(2, ai)?).to_string())
            }
            _ => {
                let start = self.out.len();
                self.item(MAX_DEPTH)?;
                let t = self.out.split_off(start);
                Ok(serde_json::from_str::<Value>(&t).ok().and_then(|v| v.as_str().map(|s| s.to_string())).unwrap_or(t))
            }
        }
    }
}

fn half_to_f32(h: u16) -> f32 {
    let sign = if h & 0x8000 != 0 { -1.0f32 } else { 1.0 };
    let exp = (h >> 10) & 0x1f;
    let frac = (h & 0x3ff) as f32;
    sign * match exp {
        0 => frac * 2f32.powi(-24),
        31 => {
            if frac == 0.0 {
                f32::INFINITY
            } else {
                f32::NAN
            }
        }
        e => (1.0 + frac / 1024.0) * 2f32.powi(e as i32 - 15),
    }
}

/// The decimal text of a big-endian unsigned magnitude.
fn big_decimal(mag: &[u8]) -> String {
    // base 10^9 limbs, least significant first
    let mut limbs: Vec<u64> = vec![0];
    for &byte in mag {
        let mut carry = byte as u64;
        for l in limbs.iter_mut() {
            let v = *l * 256 + carry;
            *l = v % 1_000_000_000;
            carry = v / 1_000_000_000;
        }
        while carry > 0 {
            limbs.push(carry % 1_000_000_000);
            carry /= 1_000_000_000;
        }
    }
    let mut s = limbs.last().unwrap().to_string();
    for l in limbs.iter().rev().skip(1) {
        s.push_str(&format!("{:09}", l));
    }
    s
}

fn decimal_add_one(s: &str) -> String {
    let mut d: Vec<u8> = s.bytes().collect();
    let mut i = d.len();
    loop {
        if i == 0 {
            d.insert(0, b'1');
            break;
        }
        i -= 1;
        if d[i] == b'9' {
            d[i] = b'0';
        } else {
            d[i] += 1;
            break;
        }
    }
    String::from_utf8(d).unwrap()
}

/// Renders a response the way Jackson's CBORGenerator does for
/// XContentBuilder output: indefinite-length maps and arrays, definite
/// strings, minimal integers, doubles as 64-bit floats (scores, which are
/// Java floats, as 32-bit).
pub fn to_cbor(v: &Value) -> Vec<u8> {
    let mut out = Vec::new();
    write_cbor(&mut out, v, "");
    out
}

fn cbor_head(out: &mut Vec<u8>, major: u8, n: u64) {
    let m = major << 5;
    if n < 24 {
        out.push(m | n as u8);
    } else if n <= u8::MAX as u64 {
        out.push(m | 24);
        out.push(n as u8);
    } else if n <= u16::MAX as u64 {
        out.push(m | 25);
        out.extend((n as u16).to_be_bytes());
    } else if n <= u32::MAX as u64 {
        out.push(m | 26);
        out.extend((n as u32).to_be_bytes());
    } else {
        out.push(m | 27);
        out.extend(n.to_be_bytes());
    }
}

fn cbor_str(out: &mut Vec<u8>, s: &str) {
    cbor_head(out, 3, s.len() as u64);
    out.extend(s.as_bytes());
}

fn write_cbor(out: &mut Vec<u8>, v: &Value, key: &str) {
    match v {
        Value::Object(m) => {
            out.push(0xbf);
            for (k, x) in m {
                cbor_str(out, k);
                write_cbor(out, x, k);
            }
            out.push(0xff);
        }
        Value::Array(a) => {
            out.push(0x9f);
            for x in a {
                write_cbor(out, x, key);
            }
            out.push(0xff);
        }
        Value::String(s) => cbor_str(out, s),
        Value::Bool(b) => out.push(if *b { 0xf5 } else { 0xf4 }),
        Value::Null => out.push(0xf6),
        Value::Number(n) => {
            let s = n.to_string();
            if !s.contains(['.', 'e', 'E']) {
                if let Ok(i) = s.parse::<i128>() {
                    if i >= 0 && i <= u64::MAX as i128 {
                        cbor_head(out, 0, i as u64);
                        return;
                    }
                    if i < 0 && -1 - i <= u64::MAX as i128 {
                        cbor_head(out, 1, (-1 - i) as u64);
                        return;
                    }
                    // BigInteger: a tagged byte string
                    let (tag, mag) = if i < 0 { (3, (-1 - i) as u128) } else { (2, i as u128) };
                    cbor_head(out, 6, tag);
                    let bytes = mag.to_be_bytes();
                    let skip = bytes.iter().take_while(|b| **b == 0).count();
                    cbor_head(out, 2, (16 - skip) as u64);
                    out.extend(&bytes[skip..]);
                    return;
                }
            }
            let f: f64 = s.parse().unwrap_or(f64::NAN);
            if key == "_score" || key == "max_score" {
                out.push(0xfa);
                out.extend((f as f32).to_be_bytes());
            } else {
                out.push(0xfb);
                out.extend(f.to_be_bytes());
            }
        }
    }
}
