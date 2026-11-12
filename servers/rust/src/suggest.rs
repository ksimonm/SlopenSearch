use crate::errors::ApiError;
use crate::store::*;
use serde_json::{json, Map, Value};
use std::collections::{HashMap, HashSet};

fn collect_field_terms(idx: &Index, field: &str) -> (HashMap<String, usize>, Vec<String>) {
    let mut freq: HashMap<String, usize> = HashMap::new();
    let mut order: Vec<String> = vec![];
    for id in &idx.order {
        let doc = match idx.docs.get(id) { Some(d) => d, None => continue };
        if let Some(v) = field_value(doc, field) {
            for s in doc_strings(v) {
                for tok in analyze(&s) {
                    if !freq.contains_key(&tok) {
                        order.push(tok.clone());
                    }
                    *freq.entry(tok).or_insert(0) += 1;
                }
            }
        }
    }
    (freq, order)
}

fn suggest_text(spec: &Map<String, Value>) -> String {
    if let Some(v) = spec.get("text") {
        return value_to_string(v).unwrap_or_default();
    }
    if let Some(v) = spec.get("prefix") {
        return value_to_string(v).unwrap_or_default();
    }
    String::new()
}

fn f32v(x: f32) -> Value {
    format!("{}", x).parse::<f64>().map(|f| json!(f)).unwrap_or(json!(0))
}

fn term_shard_error(idx: &Index, reason: &str) -> ApiError {
    let o = json!({"type": "illegal_argument_exception", "reason": reason});
    crate::script_ctx::shard_failure(&idx.name, 400, o.clone(), o.clone(), o, "query")
}

const TERM_OPTIONS: &[&str] = &[
    "field", "analyzer", "size", "shard_size", "suggest_mode", "accuracy", "sort", "string_distance",
    "max_edits", "max_inspections", "max_term_freq", "prefix_length", "prefix_len", "min_word_length",
    "min_word_len", "min_doc_freq", "lowercase_terms",
];

pub fn eval_suggest(suggest: &Map<String, Value>, indices: &[&Index]) -> Result<Value, ApiError> {
    let global_text = suggest.get("text").and_then(value_to_string);
    let mut out = Map::new();
    let mut names: Vec<&String> = suggest.keys().filter(|k| k.as_str() != "text").collect();
    names.sort();
    for name in names {
        let raw = &suggest[name];
        let spec = match raw.as_object() {
            Some(s) => s,
            None => continue,
        };
        if let Some(t) = spec.get("term").and_then(|v| v.as_object()) {
            out.insert(name.clone(), term_suggester(spec, t, indices, global_text.as_deref())?);
        } else if let Some(t) = spec.get("phrase").and_then(|v| v.as_object()) {
            if let Some(idx) = indices.first() {
                out.insert(name.clone(), json!([phrase_suggester(spec, t, idx)]));
            }
        } else if let Some(t) = spec.get("completion").and_then(|v| v.as_object()) {
            if let Some(idx) = indices.first() {
                out.insert(name.clone(), json!([completion_suggester(spec, t, idx)]));
            }
        }
    }
    Ok(Value::Object(out))
}

/// Optimal string alignment distance (Damerau-Levenshtein with adjacent
/// transpositions), over code points.
fn osa(a: &[char], b: &[char]) -> usize {
    let (n, m) = (a.len(), b.len());
    let mut d = vec![vec![0usize; m + 1]; n + 1];
    for (i, row) in d.iter_mut().enumerate() {
        row[0] = i;
    }
    for j in 0..=m {
        d[0][j] = j;
    }
    for i in 1..=n {
        for j in 1..=m {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            let mut v = (d[i - 1][j] + 1).min(d[i][j - 1] + 1).min(d[i - 1][j - 1] + cost);
            if i > 1 && j > 1 && a[i - 1] == b[j - 2] && a[i - 2] == b[j - 1] {
                v = v.min(d[i - 2][j - 2] + 1);
            }
            d[i][j] = v;
        }
    }
    d[n][m]
}

fn plain_lev(a: &[char], b: &[char]) -> usize {
    let mut prev: Vec<usize> = (0..=b.len()).collect();
    for i in 1..=a.len() {
        let mut cur = vec![i; b.len() + 1];
        for j in 1..=b.len() {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            cur[j] = (prev[j] + 1).min(cur[j - 1] + 1).min(prev[j - 1] + cost);
        }
        prev = cur;
    }
    prev[b.len()]
}

/// Lucene's JaroWinklerDistance (threshold 0.7, unbounded prefix).
fn jaro_winkler(s1: &[char], s2: &[char]) -> f32 {
    let (max, min) = if s1.len() > s2.len() { (s1, s2) } else { (s2, s1) };
    let range = (max.len() / 2).saturating_sub(1);
    let mut match_idx = vec![false; min.len()];
    let mut match_flags = vec![false; max.len()];
    let mut matches = 0usize;
    for (mi, c1) in min.iter().enumerate() {
        let lo = mi.saturating_sub(range);
        let hi = (mi + range + 1).min(max.len());
        for xi in lo..hi {
            if !match_flags[xi] && *c1 == max[xi] {
                match_idx[mi] = true;
                match_flags[xi] = true;
                matches += 1;
                break;
            }
        }
    }
    let ms1: Vec<char> = min.iter().zip(match_idx.iter()).filter(|(_, m)| **m).map(|(c, _)| *c).collect();
    let ms2: Vec<char> = max.iter().zip(match_flags.iter()).filter(|(_, m)| **m).map(|(c, _)| *c).collect();
    let transpositions = ms1.iter().zip(ms2.iter()).filter(|(a, b)| a != b).count() / 2;
    let prefix = min.iter().zip(max.iter()).take_while(|(a, b)| a == b).count();
    if matches == 0 {
        return 0.0;
    }
    let m = matches as f32;
    let j = (m / s1.len() as f32 + m / s2.len() as f32 + (m - transpositions as f32) / m) / 3.0;
    if j < 0.7 {
        j
    } else {
        j + (0.1f32).min(1.0 / max.len() as f32) * prefix as f32 * (1.0 - j)
    }
}

fn num_opt(body: &Map<String, Value>, keys: &[&str]) -> Option<f64> {
    keys.iter().find_map(|k| body.get(*k).and_then(to_f64))
}

type TermEntry = (String, usize, usize, Vec<(String, f32, usize)>);

fn term_suggester(
    spec: &Map<String, Value>,
    body: &Map<String, Value>,
    indices: &[&Index],
    global_text: Option<&str>,
) -> Result<Value, ApiError> {
    for k in body.keys() {
        if !TERM_OPTIONS.contains(&k.as_str()) {
            return Err(ApiError::parsing(&format!("suggester[term] doesn't support field [{}]", k)));
        }
    }
    let field = match body.get("field").and_then(value_to_string) {
        Some(f) => f,
        None => return Err(ApiError::new("parse_exception", "the required field option [field] is missing", 400)),
    };
    let sort = body.get("sort").and_then(value_to_string).unwrap_or_else(|| "score".into());
    let by_freq = match sort.as_str() {
        "score" => false,
        "frequency" => true,
        s => {
            return Err(ApiError::new(
                "illegal_argument_exception",
                &format!("No enum constant org.opensearch.search.suggest.SortBy.{}", s.to_uppercase()),
                400,
            ))
        }
    };
    let mode = body.get("suggest_mode").and_then(value_to_string).unwrap_or_else(|| "missing".into());
    if !["missing", "popular", "always"].contains(&mode.as_str()) {
        return Err(ApiError::new(
            "illegal_argument_exception",
            &format!(
                "No enum constant org.opensearch.search.suggest.term.TermSuggestionBuilder.SuggestMode.{}",
                mode.to_uppercase()
            ),
            400,
        ));
    }
    let distance = body.get("string_distance").and_then(value_to_string).unwrap_or_else(|| "internal".into());
    if !["internal", "damerau_levenshtein", "levenshtein", "jaro_winkler", "ngram"].contains(&distance.as_str()) {
        return Err(ApiError::new("illegal_argument_exception", &format!("Illegal distance option {}", distance), 400));
    }
    let max_edits = num_opt(body, &["max_edits"]).unwrap_or(2.0) as i64;
    if !(1..=2).contains(&max_edits) {
        return Err(ApiError::new("illegal_argument_exception", "maxEdits must be between 1 and 2", 400));
    }
    let size = num_opt(body, &["size"]).unwrap_or(5.0) as i64;
    if size <= 0 {
        return Err(ApiError::new("illegal_argument_exception", "size must be positive", 400));
    }
    let size = size as usize;
    let shard_size = num_opt(body, &["shard_size"]).map(|v| v as usize).unwrap_or(size.max(5));
    let accuracy = num_opt(body, &["accuracy"]).unwrap_or(0.5) as f32;
    let max_term_freq = num_opt(body, &["max_term_freq"]).unwrap_or(0.01) as f32;
    let min_doc_freq = num_opt(body, &["min_doc_freq"]).unwrap_or(0.0) as f32;
    let prefix_len = num_opt(body, &["prefix_length", "prefix_len"]).unwrap_or(1.0) as usize;
    let min_word = num_opt(body, &["min_word_length", "min_word_len"]).unwrap_or(4.0) as usize;
    let text = match spec.get("text").and_then(value_to_string).or_else(|| global_text.map(|s| s.to_string())) {
        Some(t) => t,
        None => {
            return Err(match indices.first() {
                Some(i) => term_shard_error(i, "The required text option is missing"),
                None => ApiError::bad("The required text option is missing"),
            })
        }
    };

    let mut entries: Vec<TermEntry> = vec![];
    for (ii, idx) in indices.iter().enumerate() {
        let def = match crate::eval::mapping_field_def(idx, &field) {
            Some(d) => d.clone(),
            None => return Err(term_shard_error(idx, &format!("no mapping found for field [{}]", field))),
        };
        let ctx = crate::analysis::Ctx::for_index(idx);
        let index_an = def.get("analyzer").and_then(|v| v.as_str()).unwrap_or("standard").to_string();
        let search_an = body
            .get("analyzer")
            .and_then(|v| v.as_str())
            .map(|s| s.to_string())
            .or_else(|| def.get("search_analyzer").and_then(|v| v.as_str()).map(|s| s.to_string()))
            .unwrap_or_else(|| index_an.clone());
        let sa = match ctx.analyzer(&search_an) {
            Ok(Some(a)) => a,
            _ => return Err(term_shard_error(idx, &format!("analyzer [{}] doesn't exists", search_an))),
        };
        let ia = ctx.analyzer(&index_an).ok().flatten();
        let mut df: HashMap<String, usize> = HashMap::new();
        let mut max_doc = 0usize;
        for id in &idx.order {
            let doc = match idx.docs.get(id) {
                Some(d) => d,
                None => continue,
            };
            max_doc += 1;
            let mut seen: HashSet<String> = HashSet::new();
            if let Some(v) = field_value(doc, &field) {
                for s in doc_strings(v) {
                    let toks = match &ia {
                        Some(a) => a.terms(&s),
                        None => analyze(&s),
                    };
                    seen.extend(toks);
                }
            }
            for t in seen {
                *df.entry(t).or_insert(0) += 1;
            }
        }
        let mut terms: Vec<(&String, &usize)> = df.iter().collect();
        terms.sort();
        let tokens = sa.run(&text).tokens;
        for (ti, tok) in tokens.iter().enumerate() {
            if ii == 0 {
                entries.push((tok.term.clone(), tok.start, tok.end - tok.start, vec![]));
            }
            let qc: Vec<char> = tok.term.chars().collect();
            if qc.len() < min_word {
                continue;
            }
            let input_df = df.get(&tok.term).copied().unwrap_or(0);
            if mode == "missing" && input_df > 0 {
                continue;
            }
            if max_term_freq >= 1.0 {
                if input_df as f32 > max_term_freq {
                    continue;
                }
            } else if input_df as f64 > (max_term_freq as f64 * max_doc as f64).ceil() {
                continue;
            }
            let mut floor = if mode == "popular" { input_df } else { 0 };
            if min_doc_freq >= 1.0 {
                floor = floor.max(min_doc_freq as usize);
            } else if min_doc_freq > 0.0 {
                floor = floor.max(((min_doc_freq * max_doc as f32) as i64 - 1).max(0) as usize);
            }
            let mut cands: Vec<(String, f32, usize)> = vec![];
            for (cand, &cdf) in &terms {
                if **cand == tok.term || cdf <= floor {
                    continue;
                }
                let cc: Vec<char> = cand.chars().collect();
                let ed = osa(&qc, &cc);
                if ed == 0 || ed as i64 > max_edits {
                    continue;
                }
                let need = if ed == 1 { prefix_len } else { prefix_len.max(ed - 1) };
                if qc.len() < need || cc.len() < need || qc[..need] != cc[..need] {
                    continue;
                }
                let score = match distance.as_str() {
                    "levenshtein" => 1.0 - plain_lev(&qc, &cc) as f32 / qc.len().max(cc.len()) as f32,
                    "jaro_winkler" => jaro_winkler(&qc, &cc),
                    _ => 1.0 - ed as f32 / qc.len().min(cc.len()) as f32,
                };
                if score < accuracy {
                    continue;
                }
                cands.push(((*cand).clone(), score, cdf));
            }
            sort_term_options(&mut cands, by_freq);
            cands.truncate(shard_size);
            if let Some(e) = entries.get_mut(ti) {
                for (t, s, f) in cands {
                    if let Some(o) = e.3.iter_mut().find(|o| o.0 == t) {
                        o.2 += f;
                        o.1 = o.1.max(s);
                    } else {
                        e.3.push((t, s, f));
                    }
                }
            }
        }
    }
    let out: Vec<Value> = entries
        .into_iter()
        .map(|(t, off, len, mut opts)| {
            sort_term_options(&mut opts, by_freq);
            opts.truncate(size);
            let options: Vec<Value> =
                opts.into_iter().map(|(t, s, f)| json!({"text": t, "score": f32v(s), "freq": f})).collect();
            json!({"text": t, "offset": off, "length": len, "options": options})
        })
        .collect();
    Ok(Value::Array(out))
}

fn sort_term_options(v: &mut [(String, f32, usize)], by_freq: bool) {
    v.sort_by(|a, b| {
        let sc = b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal);
        let fr = b.2.cmp(&a.2);
        let primary = if by_freq { fr.then(sc) } else { sc.then(fr) };
        primary.then(a.0.cmp(&b.0))
    });
}

fn phrase_suggester(spec: &Map<String, Value>, body: &Map<String, Value>, idx: &Index) -> Value {
    let field = body.get("field").and_then(value_to_string).unwrap_or_default();
    let input = suggest_text(spec);
    let (_, order) = collect_field_terms(idx, &field);

    let tokens = analyze(&input);
    let mut corrected: Vec<String> = vec![];
    let mut changed = false;
    for tok in &tokens {
        let mut best = tok.clone();
        let mut best_dist = usize::MAX;
        for c in &order {
            let d = levenshtein(tok, c);
            if d == 0 {
                best = tok.clone();
                break;
            }
            if d <= 2 && d < best_dist {
                best_dist = d;
                best = c.clone();
            }
        }
        if &best != tok {
            changed = true;
        }
        corrected.push(best);
    }

    let mut options: Vec<Value> = vec![];
    if changed {
        options.push(json!({"text": corrected.join(" "), "score": 0.9}));
    }
    json!({"text": input, "offset": 0, "length": input.len(), "options": options})
}

fn completion_inputs(v: &Value) -> Vec<(String, f64)> {
    let mut out = vec![];
    match v {
        Value::String(s) => out.push((s.clone(), 0.0)),
        Value::Object(m) => {
            let w = m.get("weight").and_then(to_f64).unwrap_or(0.0);
            if let Some(inp) = m.get("input") {
                match inp {
                    Value::String(s) => out.push((s.clone(), w)),
                    Value::Array(a) => {
                        for e in a {
                            if let Some(s) = value_to_string(e) {
                                out.push((s, w));
                            }
                        }
                    }
                    _ => {}
                }
            }
        }
        Value::Array(a) => {
            for e in a {
                match e {
                    Value::String(s) => out.push((s.clone(), 0.0)),
                    Value::Object(m) => {
                        let w = m.get("weight").and_then(to_f64).unwrap_or(0.0);
                        if let Some(inp) = m.get("input") {
                            match inp {
                                Value::String(s) => out.push((s.clone(), w)),
                                Value::Array(ia) => {
                                    for x in ia {
                                        if let Some(s) = value_to_string(x) {
                                            out.push((s, w));
                                        }
                                    }
                                }
                                _ => {}
                            }
                        }
                    }
                    _ => {}
                }
            }
        }
        _ => {}
    }
    out
}

fn completion_suggester(spec: &Map<String, Value>, body: &Map<String, Value>, idx: &Index) -> Value {
    let field = body.get("field").and_then(value_to_string).unwrap_or_default();
    let prefix = suggest_text(spec);
    let lower = prefix.to_lowercase();
    let def = crate::eval::mapping_field_def(idx, &field).cloned().unwrap_or_default();
    let preserve = def
        .get("preserve_separators")
        .and_then(|v| v.as_bool())
        .unwrap_or(true);
    let max_len = def
        .get("max_input_length")
        .and_then(|v| v.as_i64())
        .unwrap_or(50)
        .max(1) as usize;
    let fuzzy = body
        .get("fuzzy")
        .and_then(|v| v.as_object())
        .and_then(|f| f.get("fuzziness"))
        .and_then(to_f64)
        .or_else(|| {
            spec.get("fuzzy")
                .and_then(|v| v.as_object())
                .and_then(|f| f.get("fuzziness"))
                .and_then(to_f64)
        })
        .unwrap_or(0.0);
    let size = body
        .get("size")
        .and_then(|v| v.as_i64())
        .unwrap_or(10)
        .max(0) as usize;
    let norm = |s: &str| -> String {
        let lower = s.to_lowercase();
        if preserve {
            lower
        } else {
            lower.chars().filter(|c| c.is_alphanumeric()).collect()
        }
    };
    let want = norm(&lower);

    let mut entries: Vec<(String, f64, String)> = vec![];
    for id in &idx.order {
        let doc = match idx.docs.get(id) { Some(d) => d, None => continue };
        if let Some(v) = field_value(doc, &field) {
            for (raw_text, weight) in completion_inputs(v) {
                let text: String = raw_text.chars().take(max_len).collect();
                if norm(&text).starts_with(&want) {
                    let mut score = if weight > 0.0 { weight } else { 1.0 };
                    if fuzzy > 0.0 {
                        score *= 1.0 + fuzzy * 2.0;
                    }
                    entries.push((text, score, id.clone()));
                }
            }
        }
    }
    entries.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

    let options: Vec<Value> = entries
        .iter()
        .take(size)
        .map(|(text, weight, id)| json!({"text": text, "_score": weight, "_id": id}))
        .collect();
    json!({"text": prefix, "offset": 0, "length": prefix.len(), "options": options})
}
