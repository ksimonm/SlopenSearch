//! Jackson's non-standard number tokens (NaN, Infinity, ...) in document bodies.
//!
//! OpenSearch's Jackson parser rejects them while the document parser reads
//! the token: a knn_vector field reading its array elements wraps the failure
//! in its field error with the usual value preview; anywhere else the document
//! parser wraps it in "failed to parse".

use crate::errors::ApiError;
use crate::model::Index;
use serde_json::Value;

const TOKENS: [&str; 6] = ["NaN", "Infinity", "-Infinity", "+Infinity", "-INF", "+INF"];

/// One open container before the token.
struct Frame {
    array: bool,
    /// object: the current key; array: the key the array is the value of.
    key: String,
    /// object: byte offset of the current key.
    key_start: usize,
    /// array: raw text of the finished elements.
    elems: Vec<(usize, usize)>,
}

/// One of Jackson's non-standard number tokens in a body that is valid JSON
/// up to it. Offsets are counted from the start of that body.
pub struct NonStdToken {
    pub tok: &'static str,
    /// Byte offset just past the token (what Jackson reports).
    pub end: usize,
    frames: Vec<Frame>,
}

type Found = NonStdToken;

enum Stop {
    Found(Found),
    Invalid,
}

/// The non-standard number token Jackson fails on when `raw` is valid JSON
/// up to it; None when it parses or fails for any other reason.
pub fn find(raw: &[u8]) -> Option<NonStdToken> {
    let mut sc = Scanner { b: raw, i: 0, frames: Vec::new() };
    match sc.value() {
        Err(Stop::Found(f)) => Some(f),
        _ => None,
    }
}

impl NonStdToken {
    /// Jackson's json_parse_exception for the token, as OpenSearch reports it
    /// when nothing wraps it.
    pub fn cause(&self) -> ApiError {
        let reason = format!(
            "Non-standard token '{}': enable `JsonReadFeature.ALLOW_NON_NUMERIC_NUMBERS` to allow\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); byte offset: #{}]",
            self.tok, self.end
        );
        ApiError::new("json_parse_exception", &reason, 400)
            .caused_by(ApiError::new("stream_read_exception", &reason, 400))
    }

    /// The error of an ingest pipeline reading the document into a map
    /// (indexing through a pipeline, ingest simulate).
    pub fn ingest_error(&self) -> ApiError {
        ApiError::new("parse_exception", "Failed to parse content to map", 400).caused_by(self.cause())
    }

    /// The error of the OpenSearch ObjectParser named `parser` (e.g.
    /// "UpdateRequest") failing in the value of a top-level field: it names
    /// that field and the location of the parser's current token (the field
    /// name when the token is an object value, the token itself in an array).
    /// A token that is the direct value of a top-level field fails before the
    /// field is dispatched and is reported unwrapped.
    pub fn object_parser_error(&self, parser: &str, raw: &[u8]) -> ApiError {
        let fr = &self.frames;
        if fr.len() < 2 || fr[0].array {
            return self.cause();
        }
        let last = &fr[fr.len() - 1];
        let pos = if last.array { self.end - self.tok.len() } else { last.key_start };
        let before = &raw[..pos.min(raw.len())];
        let line = 1 + before.iter().filter(|&&c| c == b'\n').count();
        let col = pos - before.iter().rposition(|&c| c == b'\n').map_or(0, |nl| nl + 1) + 1;
        ApiError::new(
            "x_content_parse_exception",
            &format!("[{}:{}] [{}] failed to parse field [{}]", line, col, parser, fr[0].key),
            400,
        )
        .caused_by(self.cause())
    }
}

struct Scanner<'a> {
    b: &'a [u8],
    i: usize,
    frames: Vec<Frame>,
}

impl<'a> Scanner<'a> {
    fn ws(&mut self) {
        while self.i < self.b.len() && matches!(self.b[self.i], b' ' | b'\t' | b'\r' | b'\n') {
            self.i += 1;
        }
    }

    fn eat(&mut self, c: u8) -> Result<(), Stop> {
        self.ws();
        if self.b.get(self.i) == Some(&c) {
            self.i += 1;
            Ok(())
        } else {
            Err(Stop::Invalid)
        }
    }

    fn value(&mut self) -> Result<(), Stop> {
        self.ws();
        let start = self.i;
        let rest = &self.b[self.i..];
        for t in TOKENS {
            if rest.starts_with(t.as_bytes()) {
                let end = self.i + t.len();
                if self.b.get(end).map_or(false, |&c| is_java_ident_part(c)) {
                    return Err(Stop::Invalid);
                }
                let frames = std::mem::take(&mut self.frames);
                return Err(Stop::Found(Found { tok: t, end, frames }));
            }
        }
        match rest.first() {
            Some(b'{') => {
                self.i += 1;
                let key = self.frames.last().map(|f| f.key.clone()).unwrap_or_default();
                self.frames.push(Frame { array: false, key, key_start: 0, elems: Vec::new() });
                self.ws();
                if self.b.get(self.i) == Some(&b'}') {
                    self.i += 1;
                } else {
                    loop {
                        self.ws();
                        let ks = self.i;
                        self.string()?;
                        let k: String = serde_json::from_slice(&self.b[ks..self.i]).map_err(|_| Stop::Invalid)?;
                        let f = self.frames.last_mut().unwrap();
                        f.key = k;
                        f.key_start = ks;
                        self.eat(b':')?;
                        self.value()?;
                        self.ws();
                        match self.b.get(self.i) {
                            Some(b',') => self.i += 1,
                            Some(b'}') => {
                                self.i += 1;
                                break;
                            }
                            _ => return Err(Stop::Invalid),
                        }
                    }
                }
                self.frames.pop();
            }
            Some(b'[') => {
                self.i += 1;
                let key = self.frames.last().map(|f| f.key.clone()).unwrap_or_default();
                self.frames.push(Frame { array: true, key, key_start: 0, elems: Vec::new() });
                self.ws();
                if self.b.get(self.i) == Some(&b']') {
                    self.i += 1;
                } else {
                    loop {
                        self.value()?;
                        self.ws();
                        match self.b.get(self.i) {
                            Some(b',') => self.i += 1,
                            Some(b']') => {
                                self.i += 1;
                                break;
                            }
                            _ => return Err(Stop::Invalid),
                        }
                    }
                }
                self.frames.pop();
            }
            Some(b'"') => self.string()?,
            Some(_) => {
                let mut j = self.i;
                while j < self.b.len() && !b" \t\r\n,:]}".contains(&self.b[j]) {
                    j += 1;
                }
                serde_json::from_slice::<Value>(&self.b[self.i..j]).map_err(|_| Stop::Invalid)?;
                self.i = j;
            }
            None => return Err(Stop::Invalid),
        }
        if let Some(f) = self.frames.last_mut() {
            if f.array {
                f.elems.push((start, self.i));
            }
        }
        Ok(())
    }

    fn string(&mut self) -> Result<(), Stop> {
        if self.b.get(self.i) != Some(&b'"') {
            return Err(Stop::Invalid);
        }
        let mut j = self.i + 1;
        while j < self.b.len() {
            match self.b[j] {
                b'\\' => j += 1,
                b'"' => {
                    self.i = j + 1;
                    return Ok(());
                }
                _ => {}
            }
            j += 1;
        }
        Err(Stop::Invalid)
    }
}

fn is_java_ident_part(c: u8) -> bool {
    c >= 0x80 || c == b'_' || c == b'$' || c.is_ascii_alphanumeric()
}

/// The error OpenSearch returns for a document body that is valid JSON up to
/// one of Jackson's non-standard number tokens; None when the body fails for
/// any other reason.
pub fn non_standard_token_error(idx: Option<&Index>, doc_id: &str, raw: &[u8]) -> Option<ApiError> {
    let found = find(raw)?;
    let cause = found.cause();
    let top = idx
        .and_then(|idx| knn_error(idx, doc_id, &found, raw))
        .unwrap_or_else(|| ApiError::new("mapper_parsing_exception", "failed to parse", 400));
    Some(top.caused_by(cause))
}

/// The knn_vector field error when the token is an element of the array value
/// of a knn_vector field whose earlier elements are all numbers. The preview is
/// what OpenSearch reads after the failure: the previous element, or the rest
/// of the array when the token was the first element.
fn knn_error(idx: &Index, doc_id: &str, found: &Found, raw: &[u8]) -> Option<ApiError> {
    let fr = &found.frames;
    let n = fr.len();
    if n < 2 || !fr[n - 1].array || fr[n - 2].array {
        return None;
    }
    let path: Vec<&str> = fr[..n - 1].iter().filter(|f| !f.array).map(|f| f.key.as_str()).collect();
    let path = path.join(".");
    let def = crate::mapping::mapping_field_def(idx, &path)?;
    if def.get("type").and_then(|t| t.as_str()) != Some("knn_vector") {
        return None;
    }
    let mut nums = Vec::new();
    for &(s, e) in &fr[n - 1].elems {
        match serde_json::from_slice::<Value>(&raw[s..e]) {
            Ok(Value::Number(_)) => nums.push(std::str::from_utf8(&raw[s..e]).ok()?),
            _ => return None,
        }
    }
    let preview = if let Some(last) = nums.last() {
        java_number_string(last)
    } else {
        let rest = std::str::from_utf8(&raw[found.end..]).ok()?.trim_start();
        let rest = rest.strip_prefix(',').unwrap_or(rest);
        let text = format!("[{}", rest);
        let mut de = serde_json::Deserializer::from_str(&text).into_iter::<Value>();
        let list = match de.next() {
            Some(Ok(Value::Array(a))) => a,
            _ => return None,
        };
        let parts: Vec<String> = list
            .iter()
            .map(|v| match v {
                Value::Number(n) => java_number_string(&n.to_string()),
                Value::String(s) => s.clone(),
                Value::Null => "null".to_string(),
                other => other.to_string(),
            })
            .collect();
        format!("[{}]", parts.join(", "))
    };
    Some(ApiError::new(
        "mapper_parsing_exception",
        &format!(
            "failed to parse field [{}] of type [knn_vector] in document with id '{}'. Preview of field's value: '{}'",
            path, doc_id, preview
        ),
        400,
    ))
}

/// A JSON number as Java prints the parsed Integer/Long (as written) or
/// Double (Double.toString).
fn java_number_string(s: &str) -> String {
    if !s.contains(['.', 'e', 'E']) {
        return s.to_string();
    }
    let f: f64 = match s.parse() {
        Ok(f) if f64::is_finite(f) => f,
        _ => return s.to_string(),
    };
    let a = f.abs();
    if a != 0.0 && !(1e-3..1e7).contains(&a) {
        let e = format!("{:E}", f);
        let (mant, exp) = e.split_once('E').unwrap_or((&e, "0"));
        let mant = if mant.contains('.') { mant.to_string() } else { format!("{}.0", mant) };
        return format!("{}E{}", mant, exp);
    }
    let out = format!("{}", f);
    if out.contains('.') {
        out
    } else {
        format!("{}.0", out)
    }
}
