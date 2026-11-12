//! `GET|POST [/{index}]/_analyze`.

use axum::response::Response;
use serde_json::{json, Map, Value};

use super::tokenizers::Tokenizer;
use super::uchar::utf16_len;
use super::{bad, specs_of, Analyzer, Chain, Ctx, Named, Spec, Stream, Token};
use crate::errors::{err_response, json_response, ApiError};
use crate::store::{Index, Store};
use crate::Req;

fn err400(typ: &str, reason: &str) -> ApiError {
    ApiError::new(typ, reason, 400)
}

/// 1-based (line, column) of every top-level key and value of a JSON object,
/// as Jackson reports token locations.
fn top_level_locations(src: &str) -> Vec<(String, (usize, usize), (usize, usize))> {
    let c: Vec<char> = src.chars().collect();
    let mut out = Vec::new();
    let mut line = 1;
    let mut col = 1;
    let mut i = 0;
    let mut depth = 0i32;
    let mut expect_key = false;
    let mut pending_key: Option<(String, (usize, usize))> = None;
    let advance = |i: &mut usize, line: &mut usize, col: &mut usize, c: &[char]| {
        if c[*i] == '\n' {
            *line += 1;
            *col = 1;
        } else {
            *col += 1;
        }
        *i += 1;
    };
    while i < c.len() {
        let ch = c[i];
        match ch {
            '{' | '[' => {
                if depth == 1 {
                    if let Some((k, kl)) = pending_key.take() {
                        out.push((k, kl, (line, col)));
                    }
                }
                depth += 1;
                expect_key = ch == '{' && depth == 1;
                advance(&mut i, &mut line, &mut col, &c);
            }
            '}' | ']' => {
                depth -= 1;
                advance(&mut i, &mut line, &mut col, &c);
            }
            ',' => {
                if depth == 1 {
                    expect_key = true;
                }
                advance(&mut i, &mut line, &mut col, &c);
            }
            '"' => {
                let start = (line, col);
                let mut s = String::new();
                advance(&mut i, &mut line, &mut col, &c);
                while i < c.len() && c[i] != '"' {
                    if c[i] == '\\' {
                        advance(&mut i, &mut line, &mut col, &c);
                    }
                    if i < c.len() {
                        s.push(c[i]);
                        advance(&mut i, &mut line, &mut col, &c);
                    }
                }
                if i < c.len() {
                    advance(&mut i, &mut line, &mut col, &c);
                }
                if depth == 1 {
                    if expect_key {
                        pending_key = Some((s, start));
                        expect_key = false;
                    } else if let Some((k, kl)) = pending_key.take() {
                        out.push((k, kl, start));
                    }
                }
            }
            ':' => advance(&mut i, &mut line, &mut col, &c),
            c0 if c0.is_whitespace() => advance(&mut i, &mut line, &mut col, &c),
            _ => {
                if depth == 1 {
                    if let Some((k, kl)) = pending_key.take() {
                        out.push((k, kl, (line, col)));
                    }
                }
                advance(&mut i, &mut line, &mut col, &c);
            }
        }
    }
    out
}

fn bytes_hex(s: &str) -> String {
    let parts: Vec<String> = s.as_bytes().iter().map(|b| format!("{:02x}", b)).collect();
    format!("[{}]", parts.join(" "))
}

struct Rendered {
    tokens: Vec<Token>,
}

/// Concatenates the streams of several values: positions continue after
/// `end_pos + gap`, offsets after each value's length plus one.
fn accumulate(streams: Vec<Stream>, texts: &[String], gap: usize) -> Rendered {
    let mut out = Vec::new();
    let mut base_pos = 0usize;
    let mut base_off = 0usize;
    for (s, text) in streams.into_iter().zip(texts) {
        for mut t in s.tokens {
            t.pos += base_pos;
            t.start += base_off;
            t.end += base_off;
            out.push(t);
        }
        base_pos += s.end_pos + gap;
        base_off += utf16_len(text) + 1;
    }
    Rendered { tokens: out }
}

fn token_json(t: &Token, explain: bool, keyword_attr: bool, attrs: Option<&Vec<String>>) -> Value {
    let mut m = Map::new();
    m.insert("token".into(), json!(t.term));
    m.insert("start_offset".into(), json!(t.start));
    m.insert("end_offset".into(), json!(t.end));
    m.insert("type".into(), json!(t.typ));
    m.insert("position".into(), json!(t.pos));
    if t.pos_len > 1 {
        m.insert("positionLength".into(), json!(t.pos_len));
    }
    if explain {
        let want = |k: &str| attrs.map(|a| a.iter().any(|x| x == k)).unwrap_or(true);
        if want("bytes") {
            m.insert("bytes".into(), json!(bytes_hex(&t.term)));
        }
        if keyword_attr && want("keyword") {
            m.insert("keyword".into(), json!(t.keyword));
        }
        if want("positionLength") {
            m.insert("positionLength".into(), json!(t.pos_len));
        }
        if want("termFrequency") {
            m.insert("termFrequency".into(), json!(1));
        }
    }
    Value::Object(m)
}

fn too_many(limit: usize) -> ApiError {
    ApiError::new(
        "illegal_state_exception",
        &format!(
            "The number of tokens produced by calling _analyze has exceeded the allowed maximum of [{}]. This limit can be set by changing the [index.analyze.max_token_count] index level setting.",
            limit
        ),
        500,
    )
}

const FIELDS: &[&str] = &[
    "text", "analyzer", "tokenizer", "char_filter", "filter", "token_filter", "normalizer", "field", "explain", "attributes",
];

fn spec_list(v: Option<&Value>, kind: &str) -> Result<Vec<Spec>, ApiError> {
    let specs = specs_of(v);
    for s in &specs {
        if let Spec::Inline(m) = s {
            if super::p_str(m, "type").map(|t| t.is_empty()).unwrap_or(true) {
                let enc = serde_json::to_string(&Value::Object(m.clone())).unwrap_or_default();
                return Err(bad(&format!("Missing [type] setting for anonymous {}: {}", kind, enc)));
            }
        }
    }
    Ok(specs)
}

pub fn handle_analyze(store: &Store, req: &Req, name: &str) -> Response {
    if req.method != "POST" && req.method != "GET" {
        return crate::errors::method_not_allowed(req, &["GET", "POST"]);
    }
    let idx: Option<&Index> = if name.is_empty() {
        None
    } else {
        match crate::shared::resolve_read_target(store, name) {
            Ok(i) => Some(i),
            Err(e) => return err_response(e),
        }
    };
    match analyze(idx, req) {
        Ok(v) => json_response(200, v),
        Err(e) => err_response(e),
    }
}

fn analyze(idx: Option<&Index>, req: &Req) -> Result<Value, ApiError> {
    let raw = String::from_utf8_lossy(&req.body).to_string();
    let src = if raw.trim().is_empty() {
        match req.param("source") {
            Some(s) => s.to_string(),
            None => return Err(err400("parse_exception", "request body or source parameter is required")),
        }
    } else {
        raw
    };
    let body = match serde_json::from_str::<Value>(&src) {
        Ok(Value::Object(m)) => m,
        _ => return Err(err400("parse_exception", "Failed to derive xcontent")),
    };
    let locs = top_level_locations(&src);
    let loc = |key: &str, value: bool| -> String {
        match locs.iter().find(|(k, _, _)| k == key) {
            Some((_, kl, vl)) => {
                let (l, c) = if value { *vl } else { *kl };
                format!("[{}:{}]", l, c)
            }
            None => "[1:1]".to_string(),
        }
    };
    for k in body.keys() {
        if !FIELDS.contains(&k.as_str()) {
            return Err(err400("x_content_parse_exception", &format!("{} [analyze_request] unknown field [{}]", loc(k, false), k)));
        }
    }
    let explain = match body.get("explain") {
        None | Some(Value::Null) => false,
        Some(Value::Bool(b)) => *b,
        Some(Value::String(s)) if s == "true" || s == "false" => s == "true",
        Some(other) => {
            let shown = match other {
                Value::String(s) => s.clone(),
                o => o.to_string(),
            };
            return Err(err400("x_content_parse_exception", &format!("{} [analyze_request] failed to parse field [explain]", loc("explain", true)))
                .caused_by(bad(&format!("Failed to parse value [{}] as only [true] or [false] are allowed.", shown))));
        }
    };
    let attrs: Option<Vec<String>> = body.get("attributes").and_then(super::value_list);
    let texts: Vec<String> = match body.get("text") {
        Some(Value::String(s)) => vec![s.clone()],
        Some(Value::Array(a)) => a.iter().map(|v| crate::store::value_to_string(v).unwrap_or_default()).collect(),
        Some(v @ (Value::Number(_) | Value::Bool(_))) => vec![crate::store::value_to_string(v).unwrap_or_default()],
        _ => Vec::new(),
    };
    let analyzer_name = body.get("analyzer").and_then(|v| v.as_str()).map(|s| s.to_string());
    let normalizer = body.get("normalizer").and_then(|v| v.as_str()).map(|s| s.to_string());
    let field = body.get("field").and_then(|v| v.as_str()).map(|s| s.to_string());
    let tokenizer_spec: Option<Spec> = match body.get("tokenizer") {
        Some(Value::String(s)) => Some(Spec::Name(s.clone())),
        Some(Value::Object(m)) => Some(Spec::Inline(m.clone())),
        _ => None,
    };
    let filter_v = body.get("filter").or_else(|| body.get("token_filter"));
    let has_filters = filter_v.map(|v| !matches!(v, Value::Array(a) if a.is_empty())).unwrap_or(false);
    let has_cfs = body.get("char_filter").map(|v| !matches!(v, Value::Array(a) if a.is_empty())).unwrap_or(false);

    // Request validation (AnalyzeAction.Request#validate).
    let mut problems: Vec<&str> = Vec::new();
    if texts.is_empty() {
        problems.push("text is missing");
    }
    if idx.is_none() && normalizer.is_some() {
        problems.push("index is required if normalizer is specified");
    }
    if normalizer.is_some() && (tokenizer_spec.is_some() || analyzer_name.is_some()) {
        problems.push("tokenizer/analyze should be null if normalizer is specified");
    }
    if analyzer_name.is_some() && (tokenizer_spec.is_some() || has_filters || has_cfs) {
        problems.push("cannot define extra components on a named analyzer");
    }
    if normalizer.is_some() && (tokenizer_spec.is_some() || has_filters || has_cfs) {
        problems.push("cannot define extra components on a named normalizer");
    }
    if !problems.is_empty() {
        let mut reason = "Validation Failed: ".to_string();
        for (i, p) in problems.iter().enumerate() {
            reason.push_str(&format!("{}: {};", i + 1, p));
        }
        return Err(err400("action_request_validation_exception", &reason));
    }

    let ctx = match idx {
        Some(i) => Ctx::for_index(i),
        None => Ctx::global(),
    };
    let limit = idx
        .and_then(|i| crate::store::find_setting(&i.settings, "analyze.max_token_count"))
        .and_then(|v| match v {
            Value::Number(n) => n.as_u64(),
            Value::String(s) => s.parse().ok(),
            _ => None,
        })
        .unwrap_or(10000) as usize;

    let not_found_analyzer = |n: &str| {
        if idx.is_some() {
            bad(&format!("failed to find analyzer [{}]", n))
        } else {
            bad(&format!("failed to find global analyzer [{}]", n))
        }
    };

    // Resolve what to run.
    let analyzer: Analyzer;
    let mut request_chain = false;
    if let Some(n) = &analyzer_name {
        analyzer = match ctx.analyzer(n)? {
            Some(a) => a,
            // Interim until language analyzers are implemented: standard
            // tokenizer + lowercase, as the previous simple analysis did.
            None if super::LANGUAGE_ANALYZERS.contains(&n.as_str()) => {
                let mut a = super::builtin_analyzer("standard", &Map::new(), &ctx)?.unwrap();
                if n == "thai" {
                    a.chain.tokenizer = Named { name: "thai".to_string(), inner: Tokenizer::Thai };
                }
                a.name = n.clone();
                a
            }
            None => return Err(not_found_analyzer(n)),
        };
    } else if tokenizer_spec.is_some() || has_filters || has_cfs {
        let tok = match &tokenizer_spec {
            Some(Spec::Inline(m)) if super::p_str(m, "type").map(|t| t.is_empty()).unwrap_or(true) => {
                let enc = serde_json::to_string(&Value::Object(m.clone())).unwrap_or_default();
                return Err(bad(&format!("Missing [type] setting for anonymous tokenizer: {}", enc)));
            }
            Some(s) => ctx.tokenizer(s)?,
            None => Named { name: "keyword".to_string(), inner: Tokenizer::Keyword },
        };
        let mut chain = Chain::new(tok);
        for s in spec_list(body.get("char_filter"), "char_filter")? {
            chain.char_filters.push(ctx.char_filter(&s)?);
        }
        for s in spec_list(filter_v, "filter")? {
            let f = ctx.filter(&s, &chain)?;
            chain.filters.push(f);
        }
        analyzer = Analyzer { name: String::new(), custom: true, chain, gap: 100 };
        request_chain = true;
    } else if let Some(n) = &normalizer {
        analyzer = ctx.normalizer(n)?.ok_or_else(|| bad(&format!("failed to find normalizer under [{}]", n)))?;
    } else if let Some(f) = &field {
        let i = match idx {
            Some(i) => i,
            None => return Err(bad("analysis based on a specific field requires an index")),
        };
        analyzer = field_analyzer(i, &ctx, f)?;
    } else {
        analyzer = match ctx.analyzer("default")? {
            Some(a) if ctx.section("analyzer", "default").is_some() => a,
            _ => super::builtin_analyzer("standard", &Map::new(), &ctx)?.unwrap(),
        };
    }

    if !explain {
        let streams: Vec<Stream> = texts.iter().map(|t| analyzer.run(t)).collect();
        let r = accumulate(streams, &texts, analyzer.gap);
        if r.tokens.len() > limit {
            return Err(too_many(limit));
        }
        let toks: Vec<Value> = r.tokens.iter().map(|t| token_json(t, false, false, None)).collect();
        return Ok(json!({"tokens": toks}));
    }

    let _ = request_chain;
    if !analyzer.custom {
        let kw = analyzer.chain.has_keyword_attr(analyzer.chain.filters.len());
        let streams: Vec<Stream> = texts.iter().map(|t| analyzer.run(t)).collect();
        let r = accumulate(streams, &texts, 0);
        if r.tokens.len() > limit {
            return Err(too_many(limit));
        }
        let toks: Vec<Value> = r.tokens.iter().map(|t| token_json(t, true, kw, attrs.as_ref())).collect();
        return Ok(json!({"detail": {"custom_analyzer": false, "analyzer": {"name": analyzer.name, "tokens": toks}}}));
    }
    let chain = &analyzer.chain;
    let details: Vec<super::Detail> = texts.iter().map(|t| chain.run_detailed(t)).collect();
    let mut charfilters = Vec::new();
    for (ci, cf) in chain.char_filters.iter().enumerate() {
        let ft: Vec<Value> = details.iter().map(|d| json!(d.filtered_texts[ci])).collect();
        charfilters.push(json!({"name": cf.name, "filtered_text": ft}));
    }
    let tok_streams: Vec<Stream> = details.iter().map(|d| d.tokenizer.clone()).collect();
    let tr = accumulate(tok_streams, &texts, 0);
    if tr.tokens.len() > limit {
        return Err(too_many(limit));
    }
    let tok_json: Vec<Value> = tr.tokens.iter().map(|t| token_json(t, true, false, attrs.as_ref())).collect();
    let mut tokenfilters = Vec::new();
    for (fi, f) in chain.filters.iter().enumerate() {
        let streams: Vec<Stream> = details.iter().map(|d| d.stages[fi].clone()).collect();
        let r = accumulate(streams, &texts, 0);
        let kw = chain.has_keyword_attr(fi + 1);
        let tj: Vec<Value> = r.tokens.iter().map(|t| token_json(t, true, kw, attrs.as_ref())).collect();
        tokenfilters.push(json!({"name": f.name, "tokens": tj}));
    }
    Ok(json!({"detail": {
        "custom_analyzer": true,
        "charfilters": charfilters,
        "tokenizer": {"name": chain.tokenizer.name, "tokens": tok_json},
        "tokenfilters": tokenfilters,
    }}))
}

/// The analyzer `_analyze` uses for a field: its index analyzer (or
/// normalizer for keywords), with the field's position_increment_gap.
fn field_analyzer(idx: &Index, ctx: &Ctx, field: &str) -> Result<Analyzer, ApiError> {
    let def = match crate::eval::mapping_field_def(idx, field) {
        Some(d) => d,
        None => {
            return Ok(match ctx.analyzer("default")? {
                Some(a) if ctx.section("analyzer", "default").is_some() => a,
                _ => super::builtin_analyzer("standard", &Map::new(), ctx)?.unwrap(),
            })
        }
    };
    let typ = def.get("type").and_then(|v| v.as_str()).unwrap_or("object");
    match typ {
        "text" | "match_only_text" | "search_as_you_type" => {
            let name = def.get("analyzer").and_then(|v| v.as_str()).unwrap_or("default");
            let mut a = match ctx.analyzer(name)? {
                Some(a) if name != "default" || ctx.section("analyzer", "default").is_some() => a,
                _ => super::builtin_analyzer("standard", &Map::new(), ctx)?.unwrap(),
            };
            let explicit = ctx.section("analyzer", name).map(|d| d.contains_key("position_increment_gap")).unwrap_or(false);
            a.gap = match def.get("position_increment_gap") {
                Some(v) => super::p_int(&{
                    let mut m = Map::new();
                    m.insert("g".to_string(), v.clone());
                    m
                }, "g", 100)
                .max(0) as usize,
                None => {
                    if explicit {
                        a.gap
                    } else {
                        100
                    }
                }
            };
            Ok(a)
        }
        "keyword" => {
            if let Some(n) = def.get("normalizer").and_then(|v| v.as_str()) {
                if let Some(a) = ctx.normalizer(n)? {
                    return Ok(a);
                }
            }
            Ok(super::builtin_analyzer("keyword", &Map::new(), ctx)?.unwrap())
        }
        _ => Err(bad(&format!("Can't process field [{}], Analysis requests are only supported on tokenized fields", field))),
    }
}
