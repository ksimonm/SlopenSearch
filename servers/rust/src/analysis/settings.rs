//! Validation of `index.analysis.*` settings, as OpenSearch builds the
//! analysis registry when an index is created or its analysis settings change.

use serde_json::{json, Map, Value};

use super::tokenizers::Tokenizer;
use super::{analysis_from_settings, bad, p_str, specs_of, Chain, Ctx, Named, Spec};
use crate::errors::ApiError;

fn entries<'a>(analysis: &'a Map<String, Value>, kind: &str) -> Vec<(&'a String, &'a Map<String, Value>)> {
    match analysis.get(kind).and_then(|v| v.as_object()) {
        Some(m) => m.iter().filter_map(|(k, v)| v.as_object().map(|o| (k, o))).collect(),
        None => Vec::new(),
    }
}

/// Checks analysis settings; Ok when they build.
pub fn validate_index_analysis(settings: &Map<String, Value>) -> Result<(), ApiError> {
    let analysis = analysis_from_settings(settings);
    if analysis.is_empty() {
        return Ok(());
    }
    let ctx = Ctx::for_settings(settings);
    let dummy = Chain::new(Named { name: "keyword".to_string(), inner: Tokenizer::Keyword });

    for (name, def) in entries(&analysis, "char_filter") {
        let typ = match p_str(def, "type") {
            Some(t) => t,
            None => return Err(bad(&format!("char_filter [{}] must specify either an analyzer type, or a tokenizer", name))),
        };
        if super::charfilters::build(&typ, def)?.is_none() {
            return Err(bad(&format!("Unknown char_filter type [{}] for [{}]", typ, name)));
        }
    }
    for (name, def) in entries(&analysis, "tokenizer") {
        let typ = match p_str(def, "type") {
            Some(t) => t,
            None => return Err(bad(&format!("tokenizer [{}] must specify either an analyzer type, or a tokenizer", name))),
        };
        if super::tokenizers::build(&typ, def, &ctx)?.is_none() {
            return Err(bad(&format!("Unknown tokenizer type [{}] for [{}]", typ, name)));
        }
    }
    for (name, def) in entries(&analysis, "filter") {
        let typ = match p_str(def, "type") {
            Some(t) => t,
            None => return Err(bad(&format!("filter [{}] must specify either an analyzer type, or a tokenizer", name))),
        };
        // Synonym rules are analyzed with the analyzer chain that uses them;
        // they are checked when an analyzer is built below.
        if typ == "synonym" || typ == "synonym_graph" {
            continue;
        }
        match super::filters::build(&typ, def, &ctx, &dummy) {
            Ok(Some(_)) => {}
            Ok(None) => return Err(bad(&format!("Unknown filter type [{}] for [{}]", typ, name))),
            Err(e) => {
                if e.reason.starts_with("failed to find") {
                    continue;
                }
                return Err(e);
            }
        }
    }
    let mut failed: Vec<String> = Vec::new();
    let mut suppressed: Vec<Value> = Vec::new();
    for (name, def) in entries(&analysis, "analyzer") {
        let typ = p_str(def, "type");
        if typ.is_none() && !def.contains_key("tokenizer") {
            return Err(bad(&format!("analyzer [{}] must specify either an analyzer type, or a tokenizer", name)));
        }
        let typ = typ.unwrap_or_else(|| "custom".to_string());
        if typ != "custom" && super::builtin_analyzer(&typ, &Map::new(), &ctx).ok().flatten().is_none() {
            return Err(bad(&format!("Unknown analyzer type [{}] for [{}]", typ, name)));
        }
        if let Err(e) = super::analyzer_from_def(name, def, &ctx) {
            if typ == "custom" && e.reason.starts_with("Custom Analyzer") {
                failed.push(name.clone());
                suppressed.push(json!({
                    "type": "illegal_argument_exception",
                    "reason": format!("[{}] {}", name, e.reason),
                    "caused_by": {"type": "illegal_argument_exception", "reason": e.reason},
                }));
                continue;
            }
            return Err(e);
        }
    }
    if !failed.is_empty() {
        let mut e = bad(&format!("Failed to build analyzers: [{}]", failed.join(", ")));
        e.top_fields.insert("suppressed".to_string(), Value::Array(suppressed));
        return Err(e);
    }
    for (name, def) in entries(&analysis, "normalizer") {
        let typ = p_str(def, "type").unwrap_or_else(|| "custom".to_string());
        if typ != "custom" {
            return Err(bad(&format!("Unknown normalizer type [{}] for [{}]", typ, name)));
        }
        for s in specs_of(def.get("char_filter")) {
            let n = match &s {
                Spec::Name(n) => n.clone(),
                Spec::Inline(m) => p_str(m, "type").unwrap_or_default(),
            };
            let cf = ctx
                .char_filter(&s)
                .map_err(|_| bad(&format!("Custom normalizer [{}] failed to find char_filter under name [{}]", name, n)))?;
            if matches!(cf.inner, super::charfilters::CharFilter::HtmlStrip { .. }) {
                return Err(bad(&format!("Custom normalizer [{}] may not use char filter [{}]", name, n)));
            }
        }
        for s in specs_of(def.get("filter")) {
            let n = match &s {
                Spec::Name(n) => n.clone(),
                Spec::Inline(m) => p_str(m, "type").unwrap_or_default(),
            };
            let f = ctx
                .filter(&s, &dummy)
                .map_err(|_| bad(&format!("Custom normalizer [{}] failed to find filter under name [{}]", name, n)))?;
            if !f.inner.normalizer_safe() {
                return Err(bad(&format!("Custom normalizer [{}] may not use filter [{}]", name, n)));
            }
        }
    }
    Ok(())
}

fn mapping_error(reason: &str, cause_type: &str, root_plain: bool) -> ApiError {
    let full = format!("Failed to parse mapping [_doc]: {}", reason);
    let mut e = ApiError::new("mapper_parsing_exception", &full, 400).caused_by(ApiError::new(cause_type, reason, 400));
    if root_plain {
        e.root_cause = Some(Box::new(ApiError::new("mapper_parsing_exception", reason, 400)));
    }
    e
}

/// Checks the analyzers and normalizers fields refer to (index creation).
pub fn validate_mapping_analysis(props: &Map<String, Value>, settings: &Map<String, Value>) -> Result<(), ApiError> {
    let ctx = Ctx::for_settings(settings);
    fn walk(props: &Map<String, Value>, ctx: &Ctx) -> Result<(), ApiError> {
        for (name, raw) in props {
            let Some(def) = raw.as_object() else { continue };
            let typ = def.get("type").and_then(|v| v.as_str()).unwrap_or("");
            match typ {
                "text" | "match_only_text" | "search_as_you_type" | "token_count" => {
                    for key in ["analyzer", "search_analyzer", "search_quote_analyzer"] {
                        if let Some(a) = def.get(key).and_then(|v| v.as_str()) {
                            let known = a == "default"
                                || super::LANGUAGE_ANALYZERS.contains(&a)
                                || matches!(ctx.analyzer(a), Ok(Some(_)));
                            if !known {
                                return Err(mapping_error(
                                    &format!("analyzer [{}] has not been configured in mappings", a),
                                    "illegal_argument_exception",
                                    false,
                                ));
                            }
                        }
                    }
                }
                "keyword" => {
                    for key in ["analyzer", "search_analyzer", "search_quote_analyzer"] {
                        if def.contains_key(key) {
                            return Err(mapping_error(
                                &format!("unknown parameter [{}] on mapper [{}] of type [keyword]", key, name),
                                "mapper_parsing_exception",
                                true,
                            ));
                        }
                    }
                    if let Some(n) = def.get("normalizer").and_then(|v| v.as_str()) {
                        if !matches!(ctx.normalizer(n), Ok(Some(_))) {
                            return Err(mapping_error(
                                &format!("normalizer [{}] not found for field [{}]", n, name),
                                "mapper_parsing_exception",
                                true,
                            ));
                        }
                    }
                }
                _ => {}
            }
            if let Some(sub) = def.get("properties").and_then(|p| p.as_object()) {
                walk(sub, ctx)?;
            }
            if let Some(fields) = def.get("fields").and_then(|f| f.as_object()) {
                walk(fields, ctx)?;
            }
        }
        Ok(())
    }
    walk(props, &ctx)
}
