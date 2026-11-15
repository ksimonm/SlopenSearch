use crate::search::fetch::*;
use crate::search::inner_hits::*;
use crate::search::request::*;
use crate::search::sort::*;
use crate::search::source::*;
use crate::search::highlight::collect_tokens;
use crate::aggs::eval_aggs_root;
use crate::http::docs::{bm25_explanation, parse_q_param, search_alias_filter};
use crate::errors::{err_response, json_response};
use crate::search::eval::{wildcard_regex, QueryCtx};
use crate::mapping::mapping_field_def;
use crate::http::shared;
use crate::errors::ApiError;
use crate::jsonv::{field_value, to_f64};
use crate::model::{Index, PitState, ScrollState, Store, now_millis};
use crate::settings::{find_setting, settings_value_int};
use crate::suggest::eval_suggest;
use crate::http::Req;
use axum::response::Response;
use serde_json::{json, Map, Value};
use std::cmp::Ordering;
use std::collections::{HashMap, HashSet};

// ---------------------------------------------------------------- eval bridge

// ------------------------------------------------------- doc values loading

// ------------------------------------------------------- docvalue_fields etc.

// ------------------------------------------------------------------- sorting

// ------------------------------------------------------------- source/highlight

// ------------------------------------------------------------------ run search

pub fn run_search(
    store: &Store,
    url_name: &str,
    names: &[String],
    body: &Value,
    req: &Req,
) -> Result<Value, ApiError> {
    let mut bodym = body.as_object().cloned().unwrap_or_default();
    // Script contexts resolve stored ids themselves (and report the id in errors);
    // aggregations still need their stored scripts inlined.
    if !store.scripts.is_empty() {
        for (k, v) in bodym.iter_mut() {
            if k == "aggs" || k == "aggregations" {
                inline_stored_scripts(store, v);
            }
        }
    }
    if let Some(usort) = req.param("sort") {
        let mut parsed = Vec::new();
        let mut valid = true;
        for part in usort.split(',').filter(|p| !p.is_empty()) {
            let (f, o) = match part.split_once(':') {
                Some((f, o)) => (f, o),
                None => (part, "asc"),
            };
            if f.is_empty() || (o != "asc" && o != "desc") {
                valid = false;
                break;
            }
            let mut m = Map::new();
            m.insert("order".to_string(), json!(o));
            let mut outer = Map::new();
            outer.insert(f.to_string(), Value::Object(m));
            parsed.push(Value::Object(outer));
        }
        if valid && !parsed.is_empty() {
            let existing = bodym.remove("sort");
            let mut arr = match existing {
                Some(Value::Array(a)) => a,
                Some(other) => vec![other],
                None => vec![],
            };
            arr.extend(parsed);
            bodym.insert("sort".to_string(), Value::Array(arr));
        }
    }

    let mut query = bodym.get("query").and_then(|q| q.as_object()).cloned();
    if let Some(qp) = req.param("q") {
        let mut depth = 0i32;
        let mut quoted = false;
        let mut bad = false;
        for c in qp.chars() {
            if c == '"' {
                quoted = !quoted;
            } else if !quoted && c == '(' {
                depth += 1;
            } else if !quoted && c == ')' {
                depth -= 1;
                if depth < 0 {
                    bad = true;
                }
            }
        }
        if bad || depth != 0 || quoted {
            return Err(sort_shard_error(
                "query_shard_exception",
                &format!("Failed to parse query [{}]", qp),
            ));
        }
        let mut qv = parse_q_param(qp);
        if let Some((f, raw)) = qp.split_once(':') {
            let plain = !raw.starts_with('[')
                && !raw.starts_with('{')
                && !raw.starts_with('>')
                && !raw.starts_with('<')
                && !raw.starts_with('(');
            if plain {
                let mut numeric_mismatch = false;
                let mut reaches_nested = false;
                for n in names {
                    let idx = &store.indices[n];
                    if nested_parent_of(idx, f).is_some() {
                        reaches_nested = true;
                    }
                    if let Some(d) = mapping_field_def(idx, f) {
                        let ft = d.get("type").and_then(|t| t.as_str()).unwrap_or("");
                        if matches!(ft, "integer" | "short" | "byte" | "long" | "float" | "double" | "half_float" | "scaled_float")
                            && raw.trim().parse::<f64>().is_err()
                        {
                            numeric_mismatch = true;
                        }
                    }
                }
                if reaches_nested || numeric_mismatch {
                    qv = json!({"match_none": {}});
                }
            }
        }
        if let Some(mm) = qv.get_mut("multi_match") {
            if let Some(o) = mm.as_object_mut() {
                if let Some(op) = req.param("default_operator") {
                    o.insert("default_operator".to_string(), json!(op));
                }
                if let Some(df) = req.param("df") {
                    o.insert("fields".to_string(), json!([df]));
                }
            }
        }
        if let Some(m) = qv.as_object() {
            query = Some(m.clone());
        }
    }
    let query: Map<String, Value> = match query {
        Some(m) => {
            let mut qv = Value::Object(m);
            resolve_terms_lookups(store, &mut qv)?;
            qv.as_object().cloned().unwrap_or_default()
        }
        None => Map::new(),
    };
    crate::search::knn_query::validate_tree(&Value::Object(query.clone()), &req.body)?;
    for n in names {
        if let Some(idx) = store.indices.get(n) {
            crate::search::eval::validate_query(idx, &query)?;
            crate::search::knn_query::check_sort(idx, bodym.get("sort"))?;
        }
    }

    let mut from = bodym
        .get("from")
        .and_then(to_f64)
        .map(|f| f as i64)
        .unwrap_or(0);
    let mut size = bodym
        .get("size")
        .and_then(to_f64)
        .map(|f| f as i64)
        .unwrap_or(10);
    if let Some(v) = req.param("from") {
        if let Ok(n) = v.parse::<i64>() {
            from = n;
        }
    }
    if let Some(v) = req.param("size") {
        if let Ok(n) = v.parse::<i64>() {
            size = n;
        }
    }
    if from < 0 {
        return Err(ApiError::new(
            "illegal_argument_exception",
            &format!("[from] parameter cannot be negative, found [{}]", from),
            400,
        ));
    }
    if size < 0 {
        return Err(ApiError::new(
            "illegal_argument_exception",
            &format!("[size] parameter cannot be negative, found [{}]", size),
            400,
        ));
    }
    for n in names {
        let idx = &store.indices[n];
        let mut window = 10000i64;
        if let Some(v) = find_setting(&idx.settings, "max_result_window") {
            if let Some(n) = settings_value_int(v) {
                window = n;
            }
        }
        if from + size > window {
            if req.param("scroll").is_some() && size > window {
                let reason = format!(
                    "Batch size is too large, size must be less than or equal to: [{}] but was [{}]. Scroll batch sizes cost as much memory as result windows so they are controlled by the [index.max_result_window] index level setting.",
                    window, size
                );
                let mut e = ApiError::new("search_phase_execution_exception", &reason, 400);
                e.root_cause = Some(Box::new(ApiError::new(
                    "illegal_argument_exception",
                    &reason,
                    400,
                )));
                return Err(e);
            }
            let reason = format!(
                "Result window is too large, from + size must be less than or equal to: [{}] but was [{}]. See the scroll api for a more efficient way to request large data sets. This limit can be set by changing the [index.max_result_window] index level setting.",
                window,
                from + size
            );
            let mut e = ApiError::new("search_phase_execution_exception", &reason, 400);
            e.fields.insert("phase".to_string(), json!("query"));
            e.root_cause = Some(Box::new(ApiError::new(
                "illegal_argument_exception",
                &reason,
                400,
            )));
            return Err(e);
        }
    }

    let routing_tokens: Option<HashSet<String>> = req.param("routing").map(|r| {
        r.split(',')
            .filter(|x| !x.is_empty())
            .map(|x| x.to_string())
            .collect()
    });
    let pref_shard: Option<i64> = req
        .param("preference")
        .and_then(|p| p.strip_prefix("_shards:"))
        .and_then(|n| n.parse::<i64>().ok());
    let terminate_after: Option<i64> = bodym
        .get("terminate_after")
        .and_then(to_f64)
        .map(|n| n as i64)
        .or_else(|| req.param("terminate_after").and_then(|v| v.parse::<i64>().ok()));
    let mut matched: Vec<MultiHit> = vec![];
    let mut shards = 0i64;
    for n in names {
        let idx = &store.indices[n];
        if let Some(e) = shared::read_block_error(idx) {
            return Err(e);
        }
        let nsh = idx.num_shards();
        if let Some(tokens) = &routing_tokens {
            let mut used = HashSet::new();
            for t in tokens {
                used.extend(idx.routing_shards(t));
            }
            shards += used.len() as i64;
        } else if let Some(ps) = pref_shard {
            shards += if ps < nsh { 1 } else { 0 };
        } else {
            shards += nsh;
        }
        let mut qm = query.clone();
        if let Some(f) = search_alias_filter(idx, url_name) {
            qm = Map::new();
            qm.insert(
                "bool".to_string(),
                json!({"must": [Value::Object(query.clone()), f]}),
            );
        }
        crate::search::index::trace("query prepared");
        let extra = precompute_doc_values(idx);
        crate::search::index::trace("extra values");
        let cands = crate::search::index::candidates(idx, &qm, !extra.is_empty());
        crate::search::index::trace("candidates");
        let verify = cands.is_some() && crate::search::index::verify();
        let mut seen: HashSet<&str> = HashSet::new();
        let qv = Value::Object(qm.clone());
        // the positions to evaluate: the candidates, or every document
        let all_positions;
        let positions: &[usize] = match &cands {
            Some(c) if !verify => &c.positions,
            _ => {
                all_positions = (0..idx.order.len()).collect::<Vec<usize>>();
                &all_positions
            }
        };
        let dup_ids = cands.as_ref().is_none_or(|c| c.dup_ids);
        let named_queries = crate::search::request::find_query_name(&qv).is_some();
        for (k, &i) in positions.iter().enumerate() {
            let id = &idx.order[i];
            let cand_at = match &cands {
                Some(c) if verify => c.find(i),
                Some(_) => Some(k),
                None => None,
            };
            let candidate = cands.is_none() || cand_at.is_some();
            let doc = match idx.docs.get(id) {
                Some(d) => d,
                None => continue,
            };
            if !idx.pending.is_empty() && idx.pending.contains_key(id) {
                continue;
            }
            if routing_tokens.is_some() || pref_shard.is_some() {
                let shard = idx.doc_shard(id);
                if let Some(tokens) = &routing_tokens {
                    if !tokens.iter().any(|t| idx.routing_shards(t).contains(&shard)) {
                        continue;
                    }
                }
                if pref_shard.is_some_and(|ps| ps != shard) {
                    continue;
                }
            }
            let direct = match (&cands, cand_at) {
                (Some(c), Some(j)) => c.results.as_ref().map(|r| r[j]),
                _ => None,
            };
            let (ok, score) = match direct {
                Some(r) if !verify => r,
                _ => {
                    let r = eval_doc(idx, id, doc, &extra, &qm)?;
                    if verify && candidate {
                        if let Some(d) = direct {
                            if d != r && (d.0 || r.0) {
                                eprintln!("[verify-index] {} / {}: direct result {:?}, evaluator {:?} for {}", n, id, d, r, qv);
                            }
                        }
                    }
                    r
                }
            };
            if ok {
                if !candidate {
                    eprintln!("[verify-index] {} / {} matches {} but is no candidate", n, id, qv);
                }
                // the first occurrence of the id in the index order
                let pos = if !dup_ids || seen.insert(id.as_str()) {
                    i as i64
                } else {
                    idx.order.iter().position(|x| x == id).unwrap_or(0) as i64
                };
                let mut named = Vec::new();
                if named_queries {
                    walk_named_queries(&mut named, idx, id, doc, &extra, &qv)?;
                }
                matched.push(MultiHit {
                    idx_name: n.clone(),
                    id: id.clone(),
                    doc,
                    score: crate::mapping::knn::f32r(score),
                    pos,
                    named,
                });
            }
        }
    }
    crate::search::index::trace("matched");
    if let Some(ib) = bodym.get("indices_boost") {
        let mut entries: Vec<(String, f64)> = Vec::new();
        match ib {
            Value::Array(list) => {
                for item in list {
                    if let Some(m) = item.as_object() {
                        for (k, v) in m {
                            entries.push((k.clone(), to_f64(v).unwrap_or(1.0)));
                        }
                    }
                }
            }
            Value::Object(m) => {
                for (k, v) in m {
                    entries.push((k.clone(), to_f64(v).unwrap_or(1.0)));
                }
            }
            _ => {}
        }
        for (pattern, boost) in entries {
            if pattern.contains('*') || pattern.contains('?') {
                let re = wildcard_regex(&pattern);
                for h in matched.iter_mut() {
                    if re.is_match(&h.idx_name) {
                        h.score *= boost;
                    }
                }
            } else if names.iter().any(|n| n == &pattern) {
                for h in matched.iter_mut() {
                    if h.idx_name == pattern {
                        h.score *= boost;
                    }
                }
            } else {
                return Err(ApiError::not_found(&pattern));
            }
        }
    }
    let mut terminated_early: Option<bool> = None;
    if let Some(ta) = terminate_after {
        if ta > 0 {
            terminated_early = Some(matched.len() as i64 > ta);
            if matched.len() as i64 > ta {
                matched.truncate(ta as usize);
            }
        }
    }
    if let Some(ms) = bodym.get("min_score").and_then(to_f64) {
        matched.retain(|h| h.score >= ms);
    }
    if let Some(rc) = bodym.get("rescore") {
        apply_rescore(store, &mut matched, rc)?;
    }
    let agg_matched = matched.clone();
    if let Some(pf) = bodym.get("post_filter") {
        let mut kept = Vec::new();
        for mut h in matched.drain(..) {
            let idx = &store.indices[&h.idx_name];
            let extra = precompute_doc_values(idx);
            let pm = pf.as_object().cloned().unwrap_or_default();
            let (ok, _) = eval_doc(idx, &h.id, &h.doc, &extra, &pm)?;
            if ok {
                walk_named_queries(&mut h.named, idx, &h.id, &h.doc, &extra, pf)?;
                kept.push(h);
            }
        }
        matched = kept;
    }
    let mut total = matched.len();

    let collapse_cfg = bodym.get("collapse").and_then(|v| v.as_object()).cloned();
    if let Some(c) = &collapse_cfg {
        let field = c.get("field").and_then(|v| v.as_str()).unwrap_or("").to_string();
        if bodym.get("rescore").is_some() {
            let reason = "cannot use `collapse` in conjunction with `rescore`";
            let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
            e.root_cause = Some(Box::new(ApiError::new("search_exception", reason, 500)));
            return Err(e);
        }
        if let Some(ih) = c.get("inner_hits") {
            let configs: Vec<&Value> = match ih {
                Value::Array(a) => a.iter().collect(),
                other => vec![other],
            };
            for cfg in configs {
                if cfg.get("name").and_then(|v| v.as_str()).is_none() {
                    return Err(ApiError::new(
                        "illegal_argument_exception",
                        "Field name cannot be null",
                        400,
                    ));
                }
            }
        }
        let mut ftype: Option<String> = None;
        for n in names {
            if let Some(d) = mapping_field_def(&store.indices[n], &field) {
                if ftype.is_none() {
                    ftype = d
                        .get("type")
                        .and_then(|v| v.as_str())
                        .map(|v| v.to_string());
                }
            }
        }
        let Some(ftype) = ftype else {
            let reason = format!("no mapping found for `{}` in order to collapse on", field);
            let mut e = ApiError::new("search_phase_execution_exception", "all shards failed", 400);
            e.root_cause = Some(Box::new(ApiError::new(
                "query_shard_exception",
                &reason,
                400,
            )));
            return Err(e);
        };
        if text_like_type(&ftype) {
            let reason = format!(
                "unknown type for collapse field `{}`, only keywords and numbers are accepted",
                field
            );
            let mut e = ApiError::new("search_phase_execution_exception", "all shards failed", 400);
            e.root_cause = Some(Box::new(ApiError::new(
                "illegal_argument_exception",
                &reason,
                400,
            )));
            return Err(e);
        }
        for h in &matched {
            if let Some(Value::Array(a)) = field_value(&h.doc, &field) {
                if a.len() > 1 {
                    let reason = "the collapse field must be single valued";
                    let mut e =
                        ApiError::new("search_phase_execution_exception", "all shards failed", 500);
                    e.root_cause = Some(Box::new(ApiError::new(
                        "illegal_state_exception",
                        reason,
                        500,
                    )));
                    return Err(e);
                }
            }
        }
    }

    let body_text = String::from_utf8_lossy(&req.body);
    let sort_loc = crate::search::sort::SortLocation { body: &body_text, path: vec!["sort".to_string()] };
    let specs = crate::search::sort::parse_sort_specs_at(bodym.get("sort"), Some(&sort_loc))?;
    if !specs.is_empty() && bodym.get("rescore").is_some() {
        return Err(sort_shard_error(
            "illegal_argument_exception",
            "Cannot use [sort] option in conjunction with [rescore].",
        ));
    }
    for sp in &specs {
        if sp.kind == 0 && !sp.field.is_empty() {
            for n in names {
                if let Some(idx) = store.indices.get(n) {
                    if let Some(def) = mapping_field_def(idx, &sp.field) {
                        if def.get("doc_values").and_then(|v| v.as_bool()) == Some(false) {
                            let ftype = def.get("type").and_then(|t| t.as_str()).unwrap_or("");
                            let reason = format!(
                                "Can't load fielddata on [{}] because fielddata is unsupported on fields of type [{}]. Use doc values instead.",
                                sp.field, ftype
                            );
                            return Err(search_phase_error(&reason));
                        }
                    }
                }
            }
        }
        if sp.kind == 6 && sp.geo_ignore {
            continue;
        }
        if sp.kind == 6 {
            for n in names {
                if mapping_field_def(&store.indices[n], &sp.field).is_none() {
                    let reason = format!(
                        "failed to find mapper for [{}] for geo distance based sort",
                        sp.field
                    );
                    return Err(sort_shard_error("query_shard_exception", &reason));
                }
            }
        }
    }
    let mut failed_sort: Vec<String> = Vec::new();
    if !specs.is_empty() {
        let allow_partial = req.param("allow_partial_search_results") != Some("false");
        for sp in &specs {
            if sp.kind != 0 {
                continue;
            }
            let mut missing_names = Vec::new();
            let mut text_err = None;
            for n in names {
                let idx = &store.indices[n];
                match sort_mapping_def(idx, &sp.field) {
                    Some(d) => {
                        if matches!(
                            d.get("type").and_then(|t| t.as_str()),
                            Some("rank_feature") | Some("rank_features")
                        ) {
                            return Err(sort_shard_error(
                                "illegal_argument_exception",
                                "[rank_feature] fields do not support sorting, scripting or aggregating",
                            ));
                        }
                        if d.get("type").and_then(|t| t.as_str()) == Some("geo_shape") {
                            return Err(sort_shard_error(
                                "illegal_argument_exception",
                                "can't sort on geo_shape field without using specific sorting feature, like geo_distance",
                            ));
                        }
                        if sp.nested.is_none()
                            && d.get("type").and_then(|t| t.as_str()) == Some("text")
                            && d.get("fielddata").and_then(|v| v.as_bool()) != Some(true)
                            && nested_parent_of(idx, &sp.field).is_none()
                        {
                            let reason = format!(
                                "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [{}] in order to load field data by uninverting the inverted index. Note that this can use significant memory.",
                                sp.field
                            );
                            text_err = Some(sort_shard_error("illegal_argument_exception", &reason));
                        }
                    }
                    None => missing_names.push(n.clone()),
                }
            }
            if let Some(e) = text_err {
                return Err(e);
            }
            if !missing_names.is_empty() && sp.unmapped_type.is_none() {
                if names.len() == 1 || !allow_partial {
                    return Err(sort_shard_error(
                        "query_shard_exception",
                        &format!("No mapping found for [{}] in order to sort on", sp.field),
                    ));
                }
                for n in missing_names {
                    if !failed_sort.contains(&n) {
                        failed_sort.push(n);
                    }
                }
            }
        }
    }
    if !failed_sort.is_empty() {
        matched.retain(|h| !failed_sort.contains(&h.idx_name));
        total = matched.len();
    }
    if !specs.is_empty() {
        let mut keyed: Vec<(Vec<(bool, Value)> , MultiHit)> = Vec::with_capacity(matched.len());
        for h in matched {
            let mut cells = Vec::with_capacity(specs.len());
            for sp in &specs {
                cells.push(compute_sort_cell(store, &h, sp)?);
            }
            keyed.push((cells, h));
        }
        keyed.sort_by(|a, b| {
            for i in 0..specs.len() {
                let ca = &a.0[i];
                let cb = &b.0[i];
                let mut c = cmp_sort_cells(ca, cb);
                if specs[i].desc && !ca.0 && !cb.0 {
                    c = c.reverse();
                }
                if c != Ordering::Equal {
                    return c;
                }
            }
            Ordering::Equal
        });
        matched = keyed.into_iter().map(|(_, h)| h).collect();
    }
    if let Some(after) = bodym.get("search_after").and_then(|v| v.as_array()) {
        if from > 0 {
            let reason = "`from` parameter must be set to 0 when `search_after` is used.";
            let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
            e.root_cause = Some(Box::new(ApiError::new("search_exception", reason, 500)));
            return Err(e);
        }
        if specs.is_empty() {
            return Err(sort_shard_error(
                "illegal_argument_exception",
                "Sort must contain at least one field.",
            ));
        }
        if after.len() != specs.len() {
            return Err(sort_shard_error(
                "illegal_argument_exception",
                &format!(
                    "search_after has {} value(s) but sort has {}.",
                    after.len(),
                    specs.len()
                ),
            ));
        }
        for (i, sp) in specs.iter().enumerate() {
            let ftype = names
                .iter()
                .filter_map(|n| sort_mapping_def(&store.indices[n], &sp.field))
                .find_map(|d| {
                    d.get("type")
                        .and_then(|t| t.as_str())
                        .map(|t| t.to_string())
                })
                .unwrap_or_default();
            let numeric = matches!(ftype.as_str(), "integer" | "short" | "byte" | "long" | "float" | "double" | "half_float" | "scaled_float" | "unsigned_long");
            if !sort_after_valid(sp, &after[i], numeric) {
                return Err(sort_shard_error(
                    "illegal_argument_exception",
                    &format!("Failed to parse search_after value for field [{}].", sp.field),
                ));
            }
        }
        matched.retain(|h| {
            let mut hit_vals = Vec::with_capacity(specs.len());
            for sp in &specs {
                hit_vals.push(compute_sort_cell(store, h, sp).map(|c| c.1).unwrap_or(Value::Null));
            }
            sort_after_gt(&hit_vals, after, &specs)
        });
    }
    if specs.is_empty() {
        matched.sort_by(|a, b| {
            b.score
                .partial_cmp(&a.score)
                .unwrap_or(Ordering::Equal)
        });
    }
    crate::search::index::trace("sorted");
    let collapse_pool = matched.clone();
    if let Some(c) = &collapse_cfg {
        let field = c.get("field").and_then(|v| v.as_str()).unwrap_or("");
        let mut seen = HashSet::new();
        matched.retain(|h| {
            let key = serde_json::to_string(&collapse_value(&h.doc, field)).unwrap_or_default();
            seen.insert(key)
        });
    }
    let sorted_by_field = !specs.is_empty()
        && specs.iter().any(|sp| sp.kind != 1);
    let track_scores = bodym
        .get("track_scores")
        .and_then(|v| v.as_bool())
        .unwrap_or(false);

    let mut from_u = from.max(0) as usize;
    if from_u > matched.len() {
        from_u = matched.len();
    }
    let mut end = from_u + size.max(0) as usize;
    if end > matched.len() {
        end = matched.len();
    }
    let page = &matched[from_u..end];

    let source_raw = bodym.get("_source");
    let mut src_inc: Vec<String> = Vec::new();
    let mut src_exc: Vec<String> = Vec::new();
    match source_raw {
        Some(Value::String(s)) => {
            for p in s.split(',') {
                if !p.is_empty() {
                    src_inc.push(p.to_string());
                }
            }
        }
        Some(Value::Array(a)) => {
            for e in a {
                if let Some(s) = e.as_str() {
                    src_inc.push(s.to_string());
                }
            }
        }
        Some(Value::Object(m)) => {
            for key in ["includes", "include"] {
                if let Some(a) = m.get(key).and_then(|v| v.as_array()) {
                    for e in a {
                        if let Some(s) = e.as_str() {
                            src_inc.push(s.to_string());
                        }
                    }
                }
            }
            for key in ["excludes", "exclude"] {
                if let Some(a) = m.get(key).and_then(|v| v.as_array()) {
                    for e in a {
                        if let Some(s) = e.as_str() {
                            src_exc.push(s.to_string());
                        }
                    }
                }
            }
        }
        _ => {}
    }
    if let Some(v) = req.param("_source_includes") {
        for p in v.split(',') {
            if !p.is_empty() {
                src_inc.push(p.to_string());
            }
        }
    }
    if let Some(v) = req.param("_source_excludes") {
        for p in v.split(',') {
            if !p.is_empty() {
                src_exc.push(p.to_string());
            }
        }
    }
    if let Some(v) = req.param("_source") {
        if v == "false" {
            src_inc.clear();
            src_exc.clear();
        } else if v != "true" {
            for p in v.split(',') {
                if !p.is_empty() {
                    src_inc.push(p.to_string());
                }
            }
        }
    }
    let mut source_filter = if src_inc.is_empty() && src_exc.is_empty() {
        None
    } else {
        Some(json!({"includes": src_inc, "excludes": src_exc}))
    };
    if source_filter.is_none() {
        source_filter = names
            .iter()
            .filter_map(|n| mapping_source_spec(&store.indices[n]))
            .next();
    }
    let source_explicit = source_raw.is_some() || req.param("_source").is_some();
    let source_false = matches!(source_raw, Some(Value::Bool(false)))
        || req.param("_source") == Some("false");
    let has_script_fields = bodym
        .get("script_fields")
        .and_then(|v| v.as_object())
        .map(|m| !m.is_empty())
        .unwrap_or(false);
    let source_true = matches!(source_raw, Some(Value::Bool(true)))
        || req.param("_source") == Some("true");
    let stored_raw = bodym
        .get("stored_fields")
        .cloned()
        .or_else(|| req.param("stored_fields").map(|v| json!(v)));
    let stored_none = match &stored_raw {
        Some(Value::String(s)) => s == "_none_",
        Some(Value::Array(a)) => a.iter().any(|v| v.as_str() == Some("_none_")),
        _ => false,
    };
    let stored_list: Option<Vec<String>> = match &stored_raw {
        Some(Value::Array(a)) => Some(
            a.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .filter(|s| s != "_none_")
                .collect(),
        ),
        Some(Value::String(s)) if s != "_none_" => Some(
            s.split(',')
                .filter(|x| !x.is_empty())
                .map(|x| x.to_string())
                .collect(),
        ),
        _ => None,
    };
    if stored_none && source_true {
        let reason = "[stored_fields] cannot be disabled if [_source] is requested";
        let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
        e.root_cause = Some(Box::new(ApiError::new("search_exception", reason, 500)));
        return Err(e);
    }
    let omit_source = source_false
        || ((stored_raw.is_some() || has_script_fields) && !source_explicit);
    let omit_id = stored_none;
    let mut tokens = HashSet::new();
    collect_tokens(&query, &mut tokens);
    let mut dv_fields = parse_field_specs(bodym.get("docvalue_fields"));
    if let Some(p) = req.param("docvalue_fields") {
        for f in p.split(',') {
            if !f.is_empty() {
                dv_fields.push(FieldSpec {
                    name: f.to_string(),
                    format: String::new(),
                });
            }
        }
    }
    let param_fields = parse_field_specs(bodym.get("fields"));

    if let Some(arr) = bodym.get("fields").and_then(|v| v.as_array()) {
        for e in arr {
            if e.get("include_unmapped").is_some() {
                return Err(ApiError::new(
                    "x_content_parse_exception",
                    "[docvalues_field] unknown field [include_unmapped]",
                    400,
                ));
            }
        }
    }
    for spec in &dv_fields {
        if spec.name.contains('*') || spec.name.contains('?') {
            continue;
        }
        for n in names {
            if let Some(d) = mapping_field_def(&store.indices[n], &spec.name) {
                let t = d.get("type").and_then(|v| v.as_str()).unwrap_or("");
                if t == "binary" {
                    let reason = format!(
                        "Can't load fielddata on [{}] because fielddata is unsupported on fields of type [binary]. Use doc values instead.",
                        spec.name
                    );
                    return Err(search_phase_error(&reason));
                }
                if text_like_type(t) {
                    let reason = format!(
                        "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [{}] in order to load field data by uninverting the inverted index. Note that this can use significant memory.",
                        spec.name
                    );
                    return Err(search_phase_error(&reason));
                }
            }
        }
    }
    for spec in &param_fields {
        if spec.format.is_empty() || spec.format == "use_field_mapping" {
            continue;
        }
        if spec.name.contains('*') || spec.name.contains('?') {
            continue;
        }
        for n in names {
            if let Some(d) = mapping_field_def(&store.indices[n], &spec.name) {
                let t = d.get("type").and_then(|v| v.as_str()).unwrap_or("");
                if matches!(
                    t,
                    "integer" | "long" | "short" | "byte" | "unsigned_long" | "float"
                        | "double" | "half_float" | "scaled_float" | "boolean"
                ) {
                    let reason =
                        format!("Field [{}] of type [{}] doesn't support formats.", spec.name, t);
                    return Err(search_phase_error(&reason));
                }
            }
        }
    }

    let any_score_spec = specs.iter().any(|sp| sp.kind == 1);
    let omit_scores = sorted_by_field && !any_score_spec && !track_scores;
    let max_score = if !specs.is_empty() && specs[0].kind == 1 {
        if specs[0].desc && !page.is_empty() {
            Some(page.iter().map(|h| h.score).fold(f64::NEG_INFINITY, f64::max))
        } else {
            None
        }
    } else if names.is_empty() {
        Some(0.0)
    } else if !omit_scores && !page.is_empty() && (track_scores || specs.is_empty()) {
        Some(
            page.iter()
                .map(|h| h.score)
                .fold(f64::NEG_INFINITY, f64::max),
        )
    } else {
        None
    }
    .filter(|m| m.is_finite());

    let mut hits = vec![];
    for h in page {
        let idx = &store.indices[&h.idx_name];
        let src = match &source_filter {
            Some(spec) => filter_source(&h.doc, spec),
            None => h.doc.clone(),
        };
        let score_val = if omit_scores {
            Value::Null
        } else if !h.score.is_finite() {
            crate::search::script_ctx::f64_value(h.score)
        } else {
            crate::search::javanum::score_value(h.score)
        };
        let mut entry = Map::new();
        entry.insert("_index".to_string(), json!(h.idx_name));
        if !omit_id {
            entry.insert("_id".to_string(), json!(h.id));
        }
        if let Some(r) = idx.routing.get(&h.id) {
            if !r.is_empty() {
                entry.insert("_routing".to_string(), json!(r));
            }
        }
        entry.insert("_score".to_string(), score_val);
        if !h.named.is_empty() {
            let with_scores = req.param("include_named_queries_score") == Some("true")
                || bodym
                    .get("include_named_queries_score")
                    .and_then(|v| v.as_bool())
                    .unwrap_or(false);
            if with_scores {
                let mut m = Map::new();
                for (name, score) in &h.named {
                    m.insert(name.clone(), json!(score));
                }
                entry.insert("matched_queries".to_string(), Value::Object(m));
            } else {
                entry.insert(
                    "matched_queries".to_string(),
                    json!(h.named.iter().map(|(n, _)| n.clone()).collect::<Vec<_>>()),
                );
            }
        }
        if !specs.is_empty() && !(specs.len() == 1 && specs[0].kind == 1) {
            let mut svals = Vec::with_capacity(specs.len());
            for sp in &specs {
                let (missing, val) = compute_sort_cell(store, h, sp)?;
                if missing && val.is_null() {
                    svals.push(Value::Null);
                } else {
                    svals.push(val);
                }
            }
            entry.insert("sort".to_string(), Value::Array(svals));
        }
        if !omit_source && source_enabled_search(idx) {
            entry.insert("_source".to_string(), src);
        }
        if let Some(ign) = idx.doc_ignored.get(&h.id) {
            entry.insert(
                "_ignored".to_string(),
                json!(ign.iter().map(|f| json!(f)).collect::<Vec<_>>()),
            );
        }
        let want_version = match req.param("version") {
            Some(v) => v == "true",
            None => bodym.get("version").and_then(|v| v.as_bool()).unwrap_or(false),
        };
        if want_version {
            entry.insert(
                "_version".to_string(),
                json!(idx.versions.get(&h.id).copied().unwrap_or(1)),
            );
        }
        let want_seq = bodym
            .get("seq_no_primary_term")
            .and_then(|v| v.as_bool())
            .unwrap_or(false)
            || req.param("seq_no_primary_term") == Some("true");
        if want_seq {
            let seq = if req.param("seq_no_primary_term") == Some("true") {
                1
            } else {
                idx.seq_nos.get(&h.id).copied().unwrap_or(0)
            };
            entry.insert("_seq_no".to_string(), json!(seq));
            entry.insert("_primary_term".to_string(), json!(1));
        }
        if let Some(fields) = &stored_list {
            let mut out = Map::new();
            for f in stored_paths(idx, fields) {
                if let Some(v) = crate::jsonv::field_value(&h.doc, &f) {
                    let vals = match v {
                        Value::Array(a) => a
                            .iter()
                            .map(|x| format_stored(idx, &f, x))
                            .collect(),
                        other => vec![format_stored(idx, &f, other)],
                    };
                    out.insert(f, Value::Array(vals));
                }
            }
            if !out.is_empty() {
                entry.insert("fields".to_string(), Value::Object(out));
            }
        }
        if let Some(hl) = bodym.get("highlight") {
            if source_enabled_search(idx) {
                if let Some(rendered) = crate::search::highlight::render(hl, &query, &h.doc, idx)? {
                    entry.insert("highlight".to_string(), rendered);
                }
            }
        }
        if let Some(pq) = find_percolate_map(&query) {
            let inputs = crate::search::eval_extra::percolate_inputs(idx, pq)?;
            if let Some(inputs) = inputs {
                let field = pq
                    .get("field")
                    .and_then(|f| f.as_str())
                    .unwrap_or("query");
                if let Some(stored) = h.doc.get(field).and_then(|v| v.as_object()) {
                    let multiple = pq.contains_key("documents");
                    let mut slots: Vec<Value> = Vec::new();
                    let mut hl_out = Map::new();
                    for (slot, input) in inputs {
                        let input_obj = Value::Object(input.clone());
                        let vals = crate::mapping::fields::doc_values_for(idx, "_percolate", &input)
                            .map(|v| v.0)
                            .unwrap_or_default();
                        let vals_map: Map<String, Value> = vals
                            .into_iter()
                            .map(|(k, v)| (k, Value::Array(v)))
                            .collect();
                        let qc = QueryCtx::with_values(
                            idx,
                            "_percolate",
                            &input_obj,
                            vals_map,
                        );
                        let ok = matches!(qc.eval(stored), Ok((true, _)));
                        if !ok {
                            continue;
                        }
                        slots.push(json!(slot));
                        if let Some(hl) = bodym.get("highlight") {
                            if let Some(Value::Object(rendered)) =
                                crate::search::highlight::render(hl, stored, &input_obj, idx)?
                            {
                                for (f, v) in rendered {
                                    let key = if multiple {
                                        format!("{}_{}", slot, f)
                                    } else {
                                        f
                                    };
                                    hl_out.insert(key, v);
                                }
                            }
                        }
                    }
                    if !slots.is_empty() {
                        let mut fields = match entry.remove("fields") {
                            Some(Value::Object(m)) => m,
                            _ => Map::new(),
                        };
                        fields.insert(
                            "_percolator_document_slot".to_string(),
                            Value::Array(slots),
                        );
                        entry.insert("fields".to_string(), Value::Object(fields));
                    }
                    if !hl_out.is_empty() {
                        entry.insert("highlight".to_string(), Value::Object(hl_out));
                    }
                }
            }
        }
        if !dv_fields.is_empty() || !param_fields.is_empty() {
            let mut out: Map<String, Value> = match entry.remove("fields") {
                Some(Value::Object(m)) => m,
                _ => Map::new(),
            };
            for (specs, fallback) in [(&dv_fields, false), (&param_fields, true)] {
                for spec in specs {
                    let mut fnames = vec![spec.name.clone()];
                    if spec.name.contains('*') || spec.name.contains('?') {
                        fnames.clear();
                        let re = wildcard_regex(&spec.name);
                        if let Some(dv) = idx.doc_values.get(&h.id) {
                            for f in dv.keys() {
                                if re.is_match(f) {
                                    fnames.push(f.clone());
                                }
                            }
                        }
                        fnames.sort();
                    }
                    for fname in fnames {
                        if fallback && mapping_field_def(idx, &fname).is_none() {
                            continue;
                        }
                        if let Some(vals) =
                            doc_values_output(idx, &h.id, &fname, spec, fallback)?
                        {
                            if let Some(existing) =
                                out.get_mut(&fname).and_then(|v| v.as_array_mut())
                            {
                                if crate::search::knn_query::is_vector_field(idx, &fname) {
                                    *existing = vals;
                                } else {
                                    existing.extend(vals);
                                }
                            } else {
                                out.insert(fname, Value::Array(vals));
                            }
                        }
                    }
                }
            }
            if !out.is_empty() {
                entry.insert("fields".to_string(), Value::Object(out));
            }
        }
        if let Some(c) = &collapse_cfg {
            let field = c.get("field").and_then(|v| v.as_str()).unwrap_or("");
            let mut out = match entry.remove("fields") {
                Some(Value::Object(m)) => m,
                _ => Map::new(),
            };
            if let Some(v) = collapse_value(&h.doc, field) {
                out.insert(field.to_string(), Value::Array(vec![v]));
            }
            if !out.is_empty() {
                entry.insert("fields".to_string(), Value::Object(out));
            }
        }
        if let Some(sf) = bodym.get("script_fields").and_then(|s| s.as_object()) {
            if !sf.is_empty() {
                let mut out = match entry.remove("fields") {
                    Some(Value::Object(m)) => m,
                    _ => Map::new(),
                };
                let mut source = Some(h.doc.clone());
                for (name, spec) in sf {
                    if let Some(vals) = eval_script_field(idx, &h.id, &h.doc, spec, &mut source)? {
                        out.insert(name.clone(), Value::Array(vals));
                    }
                }
                if !out.is_empty() {
                    entry.insert("fields".to_string(), Value::Object(out));
                }
            }
        }
        {
            let groups = build_query_inner_hits(idx, &h.id, &h.doc, &query, None, None, None)?;
            if !groups.is_empty() {
                let mut out = match entry.remove("inner_hits") {
                    Some(Value::Object(m)) => m,
                    _ => Map::new(),
                };
                for (k, v) in groups {
                    out.insert(k, v);
                }
                entry.insert("inner_hits".to_string(), Value::Object(out));
            }
        }
        if let Some(c) = &collapse_cfg {
            if let Some(ih) = c.get("inner_hits") {
                let field = c.get("field").and_then(|v| v.as_str()).unwrap_or("");
                let val = collapse_value(&h.doc, field);
                let built =
                    build_collapse_inner_hits(store, &collapse_pool, field, &val, ih)?;
                let mut out = match entry.remove("inner_hits") {
                    Some(Value::Object(m)) => m,
                    _ => Map::new(),
                };
                for (k, v) in built {
                    out.insert(k, v);
                }
                entry.insert("inner_hits".to_string(), Value::Object(out));
            }
        }
        hits.push(Value::Object(entry));
    }

    let mut resp = Map::new();
    resp.insert("took".to_string(), json!(1));
    resp.insert("timed_out".to_string(), json!(false));
    if let Some(te) = terminated_early {
        resp.insert("terminated_early".to_string(), json!(te));
    }
    if failed_sort.is_empty() {
        resp.insert(
            "_shards".to_string(),
            json!({"total": shards, "successful": shards, "skipped": 0, "failed": 0}),
        );
    } else {
        let failures: Vec<Value> = failed_sort
            .iter()
            .map(|n| {
                json!({
                    "shard": 0,
                    "index": n,
                    "reason": {
                        "type": "query_shard_exception",
                        "reason": "No mapping found for [only] in order to sort on"
                    }
                })
            })
            .collect();
        let failed = failed_sort.len() as i64;
        resp.insert(
            "_shards".to_string(),
            json!({
                "total": shards,
                "successful": shards - failed,
                "skipped": 0,
                "failed": failed,
                "failures": failures
            }),
        );
    }
    resp.insert(
        "hits".to_string(),
        json!({
            "total": {"value": total, "relation": "eq"},
            "max_score": match max_score { Some(m) => crate::search::javanum::score_value(m), None => Value::Null },
            "hits": hits,
        }),
    );

    if let Some(first) = names.first() {
        let idx = &store.indices[first];
        let agg_section = bodym
            .iter()
            .filter(|(k, _)| k.as_str() == "aggs" || k.as_str() == "aggregations")
            .last()
            .map(|(_, v)| v)
            .and_then(|a| a.as_object());
        if let Some(aggs) = agg_section {
            {
                let mut order: Vec<usize> = (0..agg_matched.len()).collect();
                if crate::aggs::contains_sampler(&Value::Object(aggs.clone())) {
                    order.sort_by(|&a, &b| {
                        agg_matched[b]
                            .score
                            .partial_cmp(&agg_matched[a].score)
                            .unwrap_or(std::cmp::Ordering::Equal)
                    });
                }
                // simple aggregations read the stored sources (their ids in a
                // side table); others get copies with an `_id` and the scores
                let simple = names.len() == 1 && crate::aggs::simple_agg_tree(aggs, idx);
                let with_ids: Vec<Value> = if simple {
                    Vec::new()
                } else {
                    order
                        .iter()
                        .map(|&i| {
                            let h = &agg_matched[i];
                            let mut d = h.doc.clone();
                            if let Value::Object(ref mut m) = d {
                                if !m.contains_key("_id") {
                                    m.insert("_id".to_string(), json!(h.id.clone()));
                                }
                            }
                            d
                        })
                        .collect()
                };
                let docs: Vec<&Value> = if simple {
                    order.iter().map(|&i| agg_matched[i].doc).collect()
                } else {
                    with_ids.iter().collect()
                };
                let typed = req
                    .param("typed_keys")
                    .map(|v| v != "false")
                    .unwrap_or(false);
                let mut agg_scores: HashMap<(String, String), f64> = HashMap::new();
                if simple {
                    let ids: Vec<(&Value, &str)> = agg_matched.iter().map(|h| (h.doc, h.id.as_str())).collect();
                    crate::aggs::set_doc_ids(&ids);
                    let ids: Vec<&str> = order.iter().map(|&i| agg_matched[i].id.as_str()).collect();
                    crate::aggs::set_root_docs(crate::search::index::doc_numbers(idx, &ids).map(|(v, nums)| (docs.as_slice(), v, nums)));
                } else {
                    for h in &matched {
                        agg_scores.insert((h.idx_name.clone(), h.id.clone()), h.score);
                    }
                }
                crate::aggs::set_agg_search_context(agg_scores, tokens.clone());
                let out = eval_aggs_root(aggs, &docs, idx, typed);
                if simple {
                    crate::aggs::set_doc_ids(&[]);
                    crate::aggs::set_root_docs(None);
                }
                let out = out?;
                resp.insert("aggregations".to_string(), out);
            }
        }
        if let Some(suggest) = bodym.get("suggest").and_then(|s| s.as_object()).filter(|s| !s.is_empty()) {
            let all: Vec<&Index> = names.iter().filter_map(|n| store.indices.get(n)).collect();
            let plain = eval_suggest(suggest, &all, &bodym)?;
            let typed = req
                .param("typed_keys")
                .map(|v| v != "false")
                .unwrap_or(false);
            if typed {
                let mut mapped = Map::new();
                for (name, raw) in suggest {
                    let prefix = if raw.get("term").is_some() {
                        "term#"
                    } else if raw.get("phrase").is_some() {
                        "phrase#"
                    } else if raw.get("completion").is_some() {
                        "completion#"
                    } else {
                        ""
                    };
                    if let Some(v) = plain.get(name) {
                        mapped.insert(format!("{}{}", prefix, name), v.clone());
                    }
                }
                mapped.sort_keys();
                resp.insert("suggest".to_string(), Value::Object(mapped));
            } else {
                resp.insert("suggest".to_string(), plain);
            }
        }
    }
    if bodym.get("suggest").map(|s| s.is_object()).unwrap_or(false)
        && bodym.get("query").is_none()
        && req.param("q").is_none()
        && !bodym.contains_key("aggs")
        && !bodym.contains_key("aggregations")
    {
        crate::suggest::completion::clear_hits(&mut resp);
    }

    Ok(Value::Object(resp))
}

// ------------------------------------------------------------- HTTP handlers

fn profile_query(q: &Value) -> Value {
    let breakdown = json!({
        "create_weight": 0,
        "build_scorer": 0,
        "next_doc": 0,
        "score": 0,
        "advance": 0,
        "match": 0
    });
    let m = q.as_object().cloned().unwrap_or_default();
    if let Some(matchq) = m.get("match").and_then(|v| v.as_object()) {
        if let Some((field, val)) = matchq.iter().next() {
            let text = val.as_str().unwrap_or("");
            let tok = text.split_whitespace().next().unwrap_or("");
            return json!({
                "type": "TermQuery",
                "description": format!("{}:{}", field, tok),
                "time_in_nanos": 0,
                "breakdown": breakdown
            });
        }
    }
    if let Some(term) = m.get("term").and_then(|v| v.as_object()) {
        if let Some((field, val)) = term.iter().next() {
            let value = val
                .as_str()
                .or_else(|| val.get("value").and_then(|x| x.as_str()))
                .unwrap_or("");
            return json!({
                "type": "TermQuery",
                "description": format!("{}:{}", field, value),
                "time_in_nanos": 0,
                "breakdown": breakdown
            });
        }
    }
    if let Some(b) = m.get("bool").and_then(|v| v.as_object()) {
        let mut descs = Vec::new();
        let mut children = Vec::new();
        if let Some(must) = b.get("must").and_then(|v| v.as_array()) {
            for c in must {
                if c.get("term").is_some() {
                    let inner = profile_query(c);
                    let d = inner
                        .get("description")
                        .and_then(|v| v.as_str())
                        .unwrap_or("");
                    descs.push(format!("+ConstantScore({})", d));
                    children.push(json!({
                        "type": "ConstantScoreQuery",
                        "description": d,
                        "time_in_nanos": 0,
                        "breakdown": breakdown,
                        "children": [inner]
                    }));
                } else {
                    let child = profile_query(c);
                    let d = child.get("description").and_then(|v| v.as_str()).unwrap_or("");
                    descs.push(format!("+{}", d));
                    children.push(child);
                }
            }
        }
        if let Some(filter) = b.get("filter").and_then(|v| v.as_array()) {
            for c in filter {
                let child = profile_query(c);
                let d = child.get("description").and_then(|v| v.as_str()).unwrap_or("");
                descs.push(format!("+ConstantScore({})", d));
                children.push(json!({
                    "type": "ConstantScoreQuery",
                    "description": d,
                    "time_in_nanos": 0,
                    "breakdown": breakdown,
                    "children": [child]
                }));
            }
        }
        return json!({
            "type": "BooleanQuery",
            "description": descs.join(" "),
            "time_in_nanos": 0,
            "breakdown": breakdown,
            "children": children
        });
    }
    json!({
        "type": "MatchAllDocsQuery",
        "description": "*:*",
        "time_in_nanos": 0,
        "breakdown": breakdown
    })
}

fn apply_search_extras(
    v: &mut Value,
    req: &Req,
    store: &Store,
    names: &[String],
    body: &Value,
) -> Result<(), ApiError> {
    if let Some(t) = body.get("track_total_hits") {
        if let Some(s) = t.as_str() {
            if s != "true" && s != "false" {
                return Err(ApiError::new(
                    "number_format_exception",
                    &format!("For input string: \"{}\"", s),
                    400,
                ));
            }
        }
    }
    if let Some(t) = req.param("track_total_hits") {
        if t != "true" && t != "false" && t.parse::<i64>().is_err() {
            return Err(ApiError::new(
                "illegal_argument_exception",
                &format!("Failed to parse int parameter [track_total_hits] with value [{}]", t),
                400,
            ));
        }
    }
    let url_tth = req.param("track_total_hits");
    let body_tth = body.get("track_total_hits");
    if let Some(n) = body_tth.and_then(|t| t.as_i64()) {
        if n < 0 {
            return Err(ApiError::new(
                "illegal_argument_exception",
                &format!("value must be >= 0, got {}", n),
                400,
            ));
        }
    }
    let threshold: Option<i64> = if let Some(n) = body_tth.and_then(|t| t.as_i64()) {
        Some(n)
    } else if let Some(t) = url_tth {
        if t == "true" {
            None
        } else if t == "false" {
            None
        } else {
            match t.parse::<i64>() {
                Ok(n) if n >= 0 => Some(n),
                _ => None,
            }
        }
    } else if matches!(body_tth, Some(Value::Bool(true)))
        || url_tth == Some("true")
        || req.param("rest_total_hits_as_int") == Some("true")
    {
        None
    } else {
        Some(10000)
    };
    let tracking_off = matches!(body_tth, Some(Value::Bool(false)))
        || matches!(url_tth, Some("false"));
    if tracking_off {
        if let Some(h) = v.get_mut("hits").and_then(|h| h.as_object_mut()) {
            h.remove("total");
        }
    } else if let Some(n) = threshold {
        let actual = v
            .get("hits")
            .and_then(|h| h.get("total"))
            .and_then(|t| t.get("value"))
            .and_then(|x| x.as_i64())
            .unwrap_or(0);
        if n < actual {
            if let Some(h) = v.get_mut("hits").and_then(|h| h.as_object_mut()) {
                h.insert("total".to_string(), json!({"value": n, "relation": "gte"}));
            }
        }
    }
    if req.param("rest_total_hits_as_int") == Some("true") {
        if let Some(n) = body.get("track_total_hits").and_then(|t| t.as_i64()) {
            return Err(ApiError::new(
                "illegal_argument_exception",
                &format!("[rest_total_hits_as_int] cannot be used if the tracking of total hits is not accurate, got {}", n),
                400,
            ));
        }
        let tracking_off = matches!(body.get("track_total_hits"), Some(Value::Bool(false)));
        let hits = v.get_mut("hits").and_then(|h| h.as_object_mut());
        if let Some(h) = hits {
            let total = if tracking_off {
                json!(-1)
            } else {
                let val = h
                    .get("total")
                    .and_then(|t| t.get("value"))
                    .cloned()
                    .unwrap_or(json!(0));
                val
            };
            h.insert("total".to_string(), total);
        }
    }
    if let Some(idx_name) = names.first() {
        if let Some(idx) = store.indices.get(idx_name) {
            if idx.num_shards() > 1 {
                if let Some(m) = body.get("query").and_then(|q| q.get("match")).and_then(|m| m.as_object()) {
                    if let Some((field, val)) = m.iter().next() {
                        let term = val
                            .as_str()
                            .or_else(|| val.get("query").and_then(|q| q.as_str()))
                            .unwrap_or("")
                            .split_whitespace()
                            .next()
                            .unwrap_or("")
                            .to_lowercase();
                        if !term.is_empty() {
                            let docs: Vec<&String> = idx.docs.keys().collect();
                            let mut group_n: std::collections::HashMap<String, i64> = std::collections::HashMap::new();
                            let mut group_df: std::collections::HashMap<String, i64> = std::collections::HashMap::new();
                            let mut doc_group: std::collections::HashMap<String, String> = std::collections::HashMap::new();
                            for id in &docs {
                                let g = idx.doc_shard(id).to_string();
                                *group_n.entry(g.clone()).or_insert(0) += 1;
                                let has = idx
                                    .docs
                                    .get(*id)
                                    .and_then(|d| d.get(field))
                                    .map(|fv| {
                                        let text = match fv {
                                            Value::String(s) => s.to_lowercase(),
                                            other => serde_json::to_string(other).unwrap_or_default().to_lowercase(),
                                        };
                                        text.contains(&term)
                                    })
                                    .unwrap_or(false);
                                if has {
                                    *group_df.entry(g.clone()).or_insert(0) += 1;
                                }
                                doc_group.insert((*id).clone(), g);
                            }
                            let dfs = req.param("search_type") == Some("dfs_query_then_fetch");
                            let global_n: i64 = docs.len() as i64;
                            let global_df: i64 = group_df.values().sum();
                            let global_idf = ((1.0 + (global_n as f64 - global_df as f64 + 0.5) / (global_df as f64 + 0.5)) as f64).ln();
                            let mut scores: std::collections::HashMap<String, f64> = std::collections::HashMap::new();
                            for id in &docs {
                                let g = doc_group.get(*id).cloned().unwrap_or_default();
                                let n = *group_n.get(&g).unwrap_or(&1) as f64;
                                let df = *group_df.get(&g).unwrap_or(&0) as f64;
                                let idf = if dfs {
                                    global_idf
                                } else {
                                    (1.0 + (n - df + 0.5) / (df + 0.5)).ln()
                                };
                                scores.insert((*id).clone(), (idf * 10.0).max(0.01));
                            }
                            if let Some(hits) = v
                                .get_mut("hits")
                                .and_then(|h| h.get_mut("hits"))
                                .and_then(|h| h.as_array_mut())
                            {
                                let mut max_score = 0.0f64;
                                for hit in hits.iter_mut() {
                                    if let Some(o) = hit.as_object_mut() {
                                        if let Some(id) = o.get("_id").and_then(|x| x.as_str()) {
                                            if let Some(sc) = scores.get(id) {
                                                o.insert("_score".to_string(), json!(sc));
                                                if *sc > max_score {
                                                    max_score = *sc;
                                                }
                                            }
                                        }
                                    }
                                }
                                if let Some(h) = v.get_mut("hits").and_then(|h| h.as_object_mut()) {
                                    h.insert("max_score".to_string(), json!(max_score));
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    let explain = req.param("explain") == Some("true")
        || body.get("explain").and_then(|e| e.as_bool()).unwrap_or(false);
    if explain {
        let idx_name = names.first().cloned().unwrap_or_default();
        let q = body.get("query").cloned().unwrap_or_else(|| json!({"match_all": {}}));
        if let Some(hits) = v
            .get_mut("hits")
            .and_then(|h| h.get_mut("hits"))
            .and_then(|h| h.as_array_mut())
        {
            for hit in hits.iter_mut() {
                if let Some(o) = hit.as_object_mut() {
                    let score = o.get("_score").and_then(|s| s.as_f64()).unwrap_or(1.0);
                    let id = o
                        .get("_id")
                        .and_then(|i| i.as_str())
                        .unwrap_or("")
                        .to_string();
                    let expl = match store.indices.get(&idx_name) {
                        Some(idx) => crate::search::script_ctx::script_score_explanation(&q, score)
                            .or_else(|| crate::search::knn_query::explanation(idx, &id, &q, score))
                            .unwrap_or_else(|| bm25_explanation(idx, &id, &q, score, &idx_name)),
                        None => json!({"value": score, "description": "match", "details": []}),
                    };
                    o.insert("_explanation".to_string(), expl);
                    let shard = store.indices.get(&idx_name).map(|i| i.doc_shard(&id)).unwrap_or(0);
                    o.insert("_shard".to_string(), json!(format!("[{}][{}]", idx_name, shard)));
                    o.insert("_node".to_string(), json!("node-1"));
                }
            }
        }
        let _ = q;
    }
    let profile = body.get("profile").and_then(|p| p.as_bool()).unwrap_or(false);
    if profile {
        let idx_name = names.first().cloned().unwrap_or_default();
        let q = body.get("query").cloned().unwrap_or_else(|| json!({"match_all": {}}));
        let query_profile = profile_query(&q);
        let aggs: Vec<Value> = body
            .get("aggs")
            .and_then(|a| a.as_object())
            .map(|a| {
                a.keys()
                    .map(|k| {
                        json!({
                            "description": k,
                            "time_in_nanos": 0,
                            "breakdown": {"build_aggregation": 0, "collect": 0},
                        })
                    })
                    .collect()
            })
            .unwrap_or_default();
        v.as_object_mut().map(|o| {
            o.insert(
                "profile".to_string(),
                json!({
                    "shards": [{
                        "id": format!("[{}][0]", idx_name),
                        "searches": [{
                            "query": [query_profile],
                            "rewrite_time": 0,
                            "collector": [{
                                "name": "SimpleTopScoreDocCollector",
                                "reason": "search_top_hits",
                                "time_in_nanos": 0
                            }]
                        }],
                        "aggregations": aggs,
                        "fetch": [{"type": "fetch", "description": "fetch", "time_in_nanos": 0}]
                    }]
                }),
            );
        });
    }
    Ok(())
}

fn pit_id_ok(id: &str) -> bool {
    id.starts_with("pit-")
}

fn pit_invalid_id(id: &str) -> ApiError {
    ApiError::new(
        "illegal_argument_exception",
        &format!("invalid id: [{}]", id),
        400,
    )
}

fn pit_missing_context(id: &str) -> ApiError {
    let reason = format!("No search context found for id [{}]", id);
    let mut e = ApiError::new("search_phase_execution_exception", &reason, 404);
    e.root_cause = Some(Box::new(ApiError::new(
        "search_context_missing_exception",
        &reason,
        404,
    )));
    e
}

pub fn handle_pit_create(store: &mut Store, req: &Req, name: &str) -> Response {
    if req.method != "POST" {
        return crate::http::method_not_allowed(req, &["POST"]);
    }
    let keep = match req.param("keep_alive") {
        Some(k) if !k.is_empty() => k.to_string(),
        _ => {
            return err_response(ApiError::validation(
                "Validation Failed: 1: keep alive not specified;",
            ))
        }
    };
    if let Err(e) = validate_scroll_keepalive(&keep, true) {
        return err_response(e);
    }
    let ms = scroll_keepalive_ms(&keep).unwrap_or(60_000) as i64;
    let o = shared::parse_resolve_params(
        req,
        shared::ResolveOpts2 {
            closed_error: true,
            ..Default::default()
        },
    );
    let names = match shared::resolve_spec2(store, name, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let mut indices = HashMap::new();
    let mut shards = 0i64;
    for n in &names {
        if let Some(idx) = store.indices.get(n) {
            shards += idx.num_shards();
            indices.insert(n.clone(), idx.clone());
        }
    }
    store.pit_counter += 1;
    let id = format!(
        "pit-{}-{}",
        store.pit_counter,
        crate::model::NODE_ID.replace('-', "")
    );
    let created = now_millis();
    store.pits.insert(
        id.clone(),
        PitState {
            indices,
            keep_ms: ms,
            creation_time: created,
        },
    );
    json_response(
        200,
        json!({
            "pit_id": id,
            "_shards": {"total": shards, "successful": shards, "skipped": 0, "failed": 0},
            "creation_time": created
        }),
    )
}

pub fn handle_pit_route(store: &mut Store, req: &Req, rest: &[String]) -> Response {
    if req.method == "DELETE" {
        let body = req.json_body().unwrap_or_default();
        let mut ids: Vec<String> = Vec::new();
        if let Some(v) = body.get("pit_id") {
            match v {
                Value::String(s) => ids.push(s.clone()),
                Value::Array(a) => {
                    for e in a {
                        if let Some(s) = e.as_str() {
                            ids.push(s.to_string());
                        }
                    }
                }
                _ => {}
            }
        }
        if ids.is_empty() {
            return err_response(ApiError::validation(
                "Validation Failed: 1: no pit ids specified;",
            ));
        }
        for id in &ids {
            if !pit_id_ok(id) {
                return err_response(pit_invalid_id(id));
            }
        }
        let pits: Vec<Value> = ids
            .iter()
            .map(|id| {
                store.pits.remove(id);
                json!({"successful": true, "pit_id": id})
            })
            .collect();
        return json_response(200, json!({"pits": pits}));
    }
    if req.method == "GET" {
        let all = rest.first().map(|s| s == "_all").unwrap_or(true);
        if !all {
            return no_handler_pit();
        }
        let pits: Vec<Value> = store
            .pits
            .iter()
            .map(|(id, p)| {
                json!({
                    "pit_id": id,
                    "keep_alive": p.keep_ms,
                    "creation_time": p.creation_time
                })
            })
            .collect();
        return json_response(200, json!({"pits": pits}));
    }
    crate::http::method_not_allowed(req, &["GET", "DELETE"])
}

fn no_handler_pit() -> Response {
    json_response(
        400,
        json!({"error": "no handler found for uri", "status": 400}),
    )
}

fn handle_pit_search(store: &mut Store, req: &Req, mut body: Value) -> Response {
    let pitv = body.get("pit").cloned().unwrap_or(Value::Null);
    let id = pitv
        .get("id")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    if !pit_id_ok(&id) {
        return err_response(pit_invalid_id(&id));
    }
    let Some(state) = store.pits.get(&id).cloned() else {
        return err_response(pit_missing_context(&id));
    };
    let orig_body = body.clone();
    let mut shard_doc = false;
    if let Some(o) = body.as_object_mut() {
        o.remove("pit");
        if let Some(sort) = o.get_mut("sort").and_then(|s| s.as_array_mut()) {
            let before = sort.len();
            sort.retain(|s| s.get("_shard_doc").is_none());
            shard_doc = sort.len() != before;
            if sort.is_empty() {
                o.remove("sort");
            }
        }
        if shard_doc {
            o.remove("search_after");
            o.insert("from".to_string(), json!(0));
            o.insert("size".to_string(), json!(10000));
        }
    }
    let requested_size = orig_body
        .get("size")
        .and_then(|v| v.as_i64())
        .unwrap_or(10)
        .max(0) as usize;
    let slice = scroll_slice_of(&orig_body);
    let after = orig_body
        .get("search_after")
        .and_then(|v| v.as_array())
        .and_then(|a| a.last())
        .and_then(|v| v.as_i64());
    let names: Vec<String> = state.indices.keys().cloned().collect();
    let mut pos_map: HashMap<String, i64> = HashMap::new();
    let mut p = 0i64;
    for n in &names {
        if let Some(idx) = state.indices.get(n) {
            for doc_id in &idx.order {
                pos_map.insert(doc_id.clone(), p);
                p += 1;
            }
        }
    }
    let mut saved: Vec<(String, Index)> = Vec::new();
    for (n, snap) in state.indices.clone() {
        if let Some(slot) = store.indices.get_mut(&n) {
            saved.push((n, std::mem::replace(slot, snap)));
        }
    }
    // the indices as they were when it was opened, at their refreshed versions
    let result = crate::model::visibility::with_view(store, |store| run_search(store, "", &names, &body, req));
    for (n, old) in saved {
        store.indices.insert(n, old);
    }
    match result {
        Ok(mut v) => {
            if let Some(hits) = v
                .get_mut("hits")
                .and_then(|h| h.get_mut("hits"))
                .and_then(|h| h.as_array_mut())
            {
                filter_slice_hits(hits, slice);
                if shard_doc {
                    if let Some(after) = after {
                        hits.retain(|h| {
                            h.get("_id")
                                .and_then(|v| v.as_str())
                                .and_then(|id| pos_map.get(id).copied())
                                .map(|pos| pos > after)
                                .unwrap_or(false)
                        });
                    }
                    hits.truncate(requested_size);
                    for h in hits.iter_mut() {
                        let pos = h
                            .get("_id")
                            .and_then(|v| v.as_str())
                            .and_then(|id| pos_map.get(id).copied())
                            .unwrap_or(0);
                        let mut sort = h
                            .get("sort")
                            .and_then(|s| s.as_array())
                            .cloned()
                            .unwrap_or_default();
                        sort.push(json!(pos));
                        if let Some(o) = h.as_object_mut() {
                            o.insert("sort".to_string(), Value::Array(sort));
                        }
                    }
                }
            }
            let mut out = Map::new();
            out.insert("pit_id".to_string(), json!(id));
            if let Some(o) = v.as_object() {
                for (k, val) in o {
                    out.insert(k.clone(), val.clone());
                }
            }
            json_response(200, Value::Object(out))
        }
        Err(e) => err_response(e),
    }
}

fn scroll_keepalive_ms(v: &str) -> Option<u64> {
    let (num, mult) = if let Some(x) = v.strip_suffix("ms") {
        (x, 1u64)
    } else if let Some(x) = v.strip_suffix('s') {
        (x, 1000)
    } else if let Some(x) = v.strip_suffix('m') {
        (x, 60_000)
    } else if let Some(x) = v.strip_suffix('h') {
        (x, 3_600_000)
    } else if let Some(x) = v.strip_suffix('d') {
        (x, 86_400_000)
    } else {
        (v, 1)
    };
    num.parse::<u64>().ok().map(|n| n.saturating_mul(mult))
}

fn validate_scroll_keepalive(v: &str, for_pit: bool) -> Result<(), ApiError> {
    let ms = scroll_keepalive_ms(v).ok_or_else(|| {
        ApiError::new(
            "illegal_argument_exception",
            &format!("failed to parse [{}]", v),
            400,
        )
    })?;
    if ms > 86_400_000 {
        let reason = format!(
            "Keep alive for request ({}) is too large. It must be less than (1d).",
            v
        );
        if for_pit {
            return Err(ApiError::new(
                "illegal_argument_exception",
                &format!("{} [point_in_time.max_keep_alive]", reason),
                400,
            ));
        }
        let mut e = ApiError::new("search_phase_execution_exception", &reason, 400);
        e.root_cause = Some(Box::new(ApiError::new(
            "illegal_argument_exception",
            &reason,
            400,
        )));
        return Err(e);
    }
    Ok(())
}

fn scroll_new_id(store: &mut Store) -> String {
    store.scroll_counter += 1;
    format!("scroll-{}-{}", store.scroll_counter, crate::model::NODE_ID.replace('-', ""))
}

fn scroll_slice_of(body: &Value) -> Option<(usize, usize)> {
    body.get("slice").and_then(|s| s.as_object()).and_then(|m| {
        let id = m.get("id").and_then(|v| v.as_u64())? as usize;
        let max = m.get("max").and_then(|v| v.as_u64())? as usize;
        Some((id, max))
    })
}

fn validate_scroll_slice(body: &Value, has_scroll: bool) -> Result<(), ApiError> {
    let Some((id, max)) = scroll_slice_of(body) else {
        return Ok(());
    };
    if !has_scroll {
        let reason = "`slice` cannot be used outside of a scroll context or PIT context";
        let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
        e.root_cause = Some(Box::new(ApiError::new("search_exception", reason, 500)));
        return Err(e);
    }
    if id >= max {
        let reason = "max must be greater than id";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.caused_by = Some(Box::new(ApiError::new(
            "illegal_argument_exception",
            reason,
            400,
        )));
        return Err(e);
    }
    if max <= 1 {
        let reason = "max must be greater than 1";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.caused_by = Some(Box::new(ApiError::new(
            "illegal_argument_exception",
            reason,
            400,
        )));
        return Err(e);
    }
    Ok(())
}

fn slice_id_hash(id: &str) -> usize {
    id.bytes().fold(0usize, |a, b| a.wrapping_add(b as usize))
}

fn filter_slice_hits(hits: &mut Vec<Value>, slice: Option<(usize, usize)>) {
    if let Some((sid, max)) = slice {
        hits.retain(|h| {
            h.get("_id")
                .and_then(|v| v.as_str())
                .map(|id| slice_id_hash(id) % max == sid)
                .unwrap_or(false)
        });
    }
}

fn scroll_not_found(id: &str) -> ApiError {
    let reason = format!("No search context found for id [{}]", id);
    let mut e = ApiError::new("search_phase_execution_exception", &reason, 404);
    e.root_cause = Some(Box::new(ApiError::new(
        "search_context_missing_exception",
        &reason,
        404,
    )));
    e
}

fn scroll_parse_id(id: &str) -> Result<(), ApiError> {
    if !id.starts_with("scroll-") {
        return Err(ApiError::new(
            "illegal_argument_exception",
            "Cannot parse scroll id",
            400,
        ));
    }
    Ok(())
}

pub fn handle_scroll(store: &mut Store, req: &Req, rest: &[String]) -> Response {
    if req.method == "DELETE" {
        let mut ids: Vec<String> = Vec::new();
        if let Some(id) = rest.first() {
            ids.push(id.clone());
        } else if let Ok(body) = req.json_body() {
            if let Some(v) = body.get("scroll_id") {
                match v {
                    Value::String(s) => ids.push(s.clone()),
                    Value::Array(a) => {
                        for e in a {
                            if let Some(s) = e.as_str() {
                                ids.push(s.to_string());
                            }
                        }
                    }
                    _ => {}
                }
            }
        }
        if ids.is_empty() {
            return err_response(ApiError::validation(
                "Validation Failed: 1: no scroll ids specified;",
            ));
        }
        for id in &ids {
            if let Err(e) = scroll_parse_id(id) {
                return err_response(e);
            }
        }
        let mut freed = 0;
        for id in &ids {
            if store.scrolls.remove(id).is_some() {
                freed += 1;
            }
        }
        let status = if freed > 0 { 200 } else { 404 };
        return json_response(status, json!({"succeeded": true, "num_freed": freed}));
    }
    if req.method != "POST" && req.method != "GET" {
        return crate::http::method_not_allowed(req, &["GET", "POST", "DELETE"]);
    }
    let body = req.json_body().unwrap_or_default();
    let id = rest
        .first()
        .cloned()
        .or_else(|| body.get("scroll_id").and_then(|v| v.as_str()).map(|s| s.to_string()))
        .or_else(|| req.param("scroll_id").map(|s| s.to_string()));
    let Some(id) = id else {
        return err_response(ApiError::validation(
            "Validation Failed: 1: scrollId is missing;",
        ));
    };
    if let Err(e) = scroll_parse_id(&id) {
        return err_response(e);
    }
    let keep = body
        .get("scroll")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
        .or_else(|| req.param("scroll").map(|s| s.to_string()))
        .unwrap_or_else(|| "1m".to_string());
    let ms = scroll_keepalive_ms(&keep);
    if ms.is_none() {
        return err_response(ApiError::new(
            "illegal_argument_exception",
            &format!("failed to parse [{}]", keep),
            400,
        ));
    }
    if ms.map(|m| m > 86_400_000).unwrap_or(false) {
        store.scrolls.remove(&id);
        return err_response(ApiError::new(
            "search_phase_execution_exception",
            &format!("Keep alive for request ({}) is too large. It must be less than (1d).", keep),
            400,
        ));
    }
    let Some(mut state) = store.scrolls.remove(&id) else {
        return err_response(scroll_not_found(&id));
    };
    let mut body = state.body.clone();
    if let Some(o) = body.as_object_mut() {
        o.insert("from".to_string(), json!(state.pos));
        o.insert("size".to_string(), json!(state.page));
    }
    let names = state.names.clone();
    for (n, snap) in state.snapshot.iter_mut() {
        if let Some(live) = store.indices.get_mut(n) {
            std::mem::swap(live, snap);
        }
    }
    let result = crate::model::visibility::with_view(store, |store| run_search(store, &state.url_name, &names, &body, req));
    for (n, snap) in state.snapshot.iter_mut() {
        if let Some(live) = store.indices.get_mut(n) {
            std::mem::swap(live, snap);
        }
    }
    match result {
        Ok(mut v) => {
            let mut raw_len = 0usize;
            if let Some(hits) = v
                .get_mut("hits")
                .and_then(|h| h.get_mut("hits"))
                .and_then(|h| h.as_array_mut())
            {
                raw_len = hits.len();
                let slice = state.slice;
                filter_slice_hits(hits, slice);
                if req.param("rest_total_hits_as_int") == Some("true") {
                    if let Some(total) = v
                        .get("hits")
                        .and_then(|h| h.get("total"))
                        .and_then(|t| t.get("value"))
                        .cloned()
                    {
                        if let Some(h) = v.get_mut("hits").and_then(|h| h.as_object_mut()) {
                            h.insert("total".to_string(), total);
                        }
                    }
                }
            }
            state.pos += raw_len;
            store.scrolls.insert(id.clone(), state);
            let mut out = Map::new();
            out.insert("_scroll_id".to_string(), json!(id));
            if let Some(o) = v.as_object() {
                for (k, val) in o {
                    if k != "_scroll_id" {
                        out.insert(k.clone(), val.clone());
                    }
                }
            }
            json_response(200, Value::Object(out))
        }
        Err(e) => {
            store.scrolls.insert(id, state);
            err_response(e)
        }
    }
}

pub fn handle_search_shards(store: &mut Store, req: &Req, name: &str) -> Response {
    if req.method != "GET" && req.method != "POST" {
        return crate::http::method_not_allowed(req, &["GET", "POST"]);
    }
    let o = shared::parse_resolve_params(req, shared::ResolveOpts2::default());
    let names = match shared::resolve_spec2(store, name, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let routing: Option<Vec<String>> = req.param("routing").map(|r| {
        r.split(',')
            .filter(|x| !x.is_empty())
            .map(|x| x.to_string())
            .collect()
    });
    let mut indices = Map::new();
    let mut shards_out = Vec::new();
    for n in &names {
        let idx = &store.indices[n];
        indices.insert(n.clone(), json!({}));
        let nsh = idx.num_shards();
        let mut wanted: Vec<i64> = Vec::new();
        if let Some(tokens) = &routing {
            for t in tokens {
                for sh in idx.routing_shards(t) {
                    if !wanted.contains(&sh) {
                        wanted.push(sh);
                    }
                }
            }
        } else {
            wanted.extend(0..nsh);
        }
        for sh in wanted {
            shards_out.push(json!([{
                "state": "STARTED",
                "primary": true,
                "node": crate::model::NODE_ID,
                "shard": sh,
                "index": n,
            }]));
        }
    }
    let nodes = json!({crate::model::NODE_ID: {
        "name": "slopensearch",
        "transport_address": "127.0.0.1:9300",
        "host": "127.0.0.1",
        "ip": "127.0.0.1",
        "port": 9300,
        "attributes": {}
    }});
    json_response(200, json!({"nodes": nodes, "indices": indices, "shards": shards_out}))
}

pub fn handle_search(store: &mut Store, req: &Req, name: &str) -> Response {
    handle_search_in(&mut StoreRef::Mut(store), req, name)
}

/// A search that changes nothing (see [`read_only_search`]) under a shared lock.
pub fn handle_search_shared(store: &Store, req: &Req, name: &str) -> Response {
    handle_search_in(&mut StoreRef::Shared(store), req, name)
}

/// Whether `handle_search` would change nothing in the store for this request:
/// no scroll or point in time to create, no request-level derived fields, and
/// no index with unrefreshed writes (whose refreshed view a search swaps in, or
/// that a scheduled refresh would refresh).
pub fn read_only_search(store: &Store, req: &Req) -> bool {
    if req.param("scroll").is_some() {
        return false;
    }
    let body = &req.body;
    let has = |needle: &[u8]| body.windows(needle.len()).any(|w| w == needle);
    if has(b"\"pit\"") || has(b"\"derived\"") {
        return false;
    }
    store.indices.values().all(crate::model::visibility::quiescent)
}

/// The store of a search: exclusive, or shared when [`read_only_search`] holds
/// (the writing branches are then never reached).
pub enum StoreRef<'a> {
    Mut(&'a mut Store),
    Shared(&'a Store),
}

impl StoreRef<'_> {
    fn get(&self) -> &Store {
        match self {
            StoreRef::Mut(s) => s,
            StoreRef::Shared(s) => s,
        }
    }

    fn get_mut(&mut self) -> &mut Store {
        match self {
            StoreRef::Mut(s) => s,
            StoreRef::Shared(_) => unreachable!("a read-only search changed the store"),
        }
    }
}

fn handle_search_in(st: &mut StoreRef, req: &Req, name: &str) -> Response {
    let allowed_params = [
        "q", "df", "analyzer", "default_operator", "sort", "from", "size", "scroll",
        "search_type", "request_cache", "batched_reduce_size", "pre_filter_shard_size",
        "max_concurrent_shard_requests", "track_total_hits", "rest_total_hits_as_int",
        "terminate_after", "timeout", "routing", "preference", "_source",
        "_source_includes", "_source_excludes", "stored_fields", "allow_partial_search_results",
        "ignore_unavailable", "allow_no_indices", "expand_wildcards", "ignore_throttled",
        "typed_keys", "ccs_minimize_roundtrips", "source", "source_content_type",
        "sequence_number_primary_term", "seq_no_primary_term", "version", "explain",
        "profile", "min_score", "pretty", "human", "error_trace", "filter_path",
        "docvalue_fields", "fields", "stats", "include_named_queries_score",
        "search_pipeline", "verbose_pipeline",
    ];
    for (k, _) in &req.query {
        if !allowed_params.contains(&k.as_str()) {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                &format!(
                    "request [{}] contains unrecognized parameter: [{}]",
                    req.path, k
                ),
                400,
            ));
        }
    }
    for banned in ["profile", "min_score"] {
        if req.param(banned).is_some() {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                &format!("request [{}] contains unrecognized parameter: [{}]", req.path, banned),
                400,
            ));
        }
    }
    if let Some(st) = req.param("search_type") {
        match st {
            "query_then_fetch" | "dfs_query_then_fetch" => {}
            "query_and_fetch" => {
                return err_response(ApiError::new(
                    "illegal_argument_exception",
                    "Unsupported search type [query_and_fetch]",
                    400,
                ))
            }
            other => {
                return err_response(ApiError::new(
                    "illegal_argument_exception",
                    &format!("No search type for [{}]", other),
                    400,
                ))
            }
        }
    }
    if let Some(v) = req.param("request_cache") {
        if v != "true" && v != "false" {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                &format!("Failed to parse value [{}] as only [true] or [false] are allowed.", v),
                400,
            ));
        }
    }
    if let Some(v) = req.param("batched_reduce_size") {
        if v.parse::<i64>().ok().map(|n| n < 2).unwrap_or(false) {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                "batchedReduceSize must be >= 2",
                400,
            ));
        }
    }
    if let Some(v) = req.param("pre_filter_shard_size") {
        if v.parse::<i64>().ok().map(|n| n < 1).unwrap_or(false) {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                "preFilterShardSize must be >= 1",
                400,
            ));
        }
    }
    if let Some(op) = req.param("default_operator") {
        if !op.eq_ignore_ascii_case("and") && !op.eq_ignore_ascii_case("or") {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                &format!(
                    "No enum constant org.opensearch.index.query.Operator.{}",
                    op.to_uppercase()
                ),
                400,
            ));
        }
    }
    if let Some(v) = req.param("max_concurrent_shard_requests") {
        if v.parse::<i64>().ok().map(|n| n < 1).unwrap_or(false) {
            return err_response(ApiError::new(
                "illegal_argument_exception",
                "maxConcurrentShardRequests must be >= 1",
                400,
            ));
        }
    }
    let o = shared::parse_resolve_params(
        req,
        shared::ResolveOpts2 {
            closed_error: true,
            ..Default::default()
        },
    );
    let names = match shared::resolve_spec2(st.get(), name, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let raw = String::from_utf8_lossy(&req.body).to_string();
    if raw.trim().is_empty()
        && req.param("source").is_some()
        && req.param("source_content_type").is_none()
    {
        return err_response(ApiError::new(
            "illegal_state_exception",
            "source and source_content_type parameters are required",
            500,
        ));
    }
    let eff = if raw.trim().is_empty() {
        req.param("source").unwrap_or("").to_string()
    } else {
        raw
    };
    if let Some(k) = crate::jsonv::duplicate_object_key(&eff) {
        return err_response(ApiError::new(
            "json_parse_exception",
            &format!("Duplicate Object property \"{}\"", k),
            400,
        ));
    }
    if eff.trim_start().starts_with('[') {
        return err_response(ApiError::new(
            "parsing_exception",
            "Expected [START_OBJECT] but found [START_ARRAY]",
            400,
        ));
    }
    let mut body = match req.json_body() {
        Ok(b) => b,
        Err(e) => return err_response(e),
    };
    {
        let allowed_body = [
            "query", "from", "size", "sort", "_source", "stored_fields", "docvalue_fields",
            "fields", "script_fields", "highlight", "aggs", "aggregations", "post_filter",
            "rescore", "min_score", "track_total_hits", "version", "seq_no_primary_term",
            "explain", "profile", "terminate_after", "timeout", "search_after", "collapse",
            "slice", "pit", "runtime_mappings", "indices_boost", "suggest", "_name",
            "stats", "track_scores", "rest_total_hits_as_int", "allow_partial_search_results",
            "batched_reduce_size", "pre_filter_shard_size", "max_concurrent_shard_requests",
            "search_type", "request_cache", "ext", "include_named_queries_score",
            "search_pipeline", "derived",
        ];
        {
            for (k, v) in &body {
                if !allowed_body.contains(&k.as_str()) {
                    let vt = match v {
                        Value::Null => "VALUE_NULL",
                        Value::Bool(_) => "VALUE_BOOLEAN",
                        Value::Number(_) => "VALUE_NUMBER",
                        Value::String(_) => "VALUE_STRING",
                        Value::Array(_) => "START_ARRAY",
                        Value::Object(_) => "START_OBJECT",
                    };
                    return err_response(ApiError::new(
                        "parsing_exception",
                        &format!("Unknown key for a {} in [{}].", vt, k),
                        400,
                    ));
                }
            }
        }
        if let Some(e) = crate::suggest::completion::check_suggest_body(&body, &eff) {
            return err_response(e);
        }
        if let Some(sv) = body.get("suggest") {
            if !sv.is_object() {
                let vt = match sv {
                    Value::Null => "VALUE_NULL",
                    Value::Bool(_) => "VALUE_BOOLEAN",
                    Value::Number(_) => "VALUE_NUMBER",
                    Value::Array(_) => "START_ARRAY",
                    _ => "VALUE_STRING",
                };
                return err_response(ApiError::new(
                    "parsing_exception",
                    &format!("Unknown key for a {} in [suggest].", vt),
                    400,
                ));
            }
        }
        for k in ["from", "size"] {
            if matches!(body.get(k), Some(Value::Null)) {
                return err_response(ApiError::new(
                    "parsing_exception",
                    &format!("Unknown key for a VALUE_NULL in [{}].", k),
                    400,
                ));
            }
        }
        if let Some(q) = body.get("query") {
            if q.is_null() {
                return err_response(ApiError::new(
                    "parsing_exception",
                    "Unknown key for a VALUE_NULL in [query].",
                    400,
                ));
            }
            if q.as_object().map(|o| o.is_empty()).unwrap_or(false) {
                return err_response(ApiError::new(
                    "illegal_argument_exception",
                    "query malformed, empty clause found at [1:10]",
                    400,
                ));
            }
        }
        if let Some(v) = req.param("size") {
            if v.parse::<i64>().is_err() {
                return err_response(ApiError::new(
                    "illegal_argument_exception",
                    &format!("Failed to parse int parameter [size] with value [{}]", v),
                    400,
                ));
            }
        }
        if let Some(v) = req.param("timeout") {
            let ok = v.parse::<f64>().is_ok()
                || (v.ends_with("ms") || v.ends_with('s') || v.ends_with('m') || v.ends_with('h') || v.ends_with('d'))
                    && v[..v.len() - 1].parse::<f64>().is_ok()
                || ["1ms", "-1"].contains(&v);
            if !ok {
                return err_response(ApiError::new(
                    "illegal_argument_exception",
                    &format!("failed to parse [{}]", v),
                    400,
                ));
            }
        }
    }
    if body.contains_key("pit") {
        if !name.is_empty() {
            return err_response(ApiError::validation(
                "Validation Failed: 1: [indices] cannot be used with point in time;",
            ));
        }
        return handle_pit_search(st.get_mut(), req, Value::Object(body));
    }
    if body
        .get("sort")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter().any(|s| {
                s.get("_shard_doc").is_some() || s.as_str() == Some("_shard_doc")
            })
        })
        .unwrap_or(false)
    {
        return err_response(ApiError::validation(
            "Validation Failed: 1: _shard_doc is only supported with point-in-time (PIT). Add a PIT or remove _shard_doc.;",
        ));
    }
    if req.param("scroll").is_some() {
        if body.contains_key("from") {
            return err_response(ApiError::validation(
                "Validation Failed: 1: using [from] is not allowed in a scroll context;",
            ));
        }
        if req.param("request_cache").is_some() {
            return err_response(ApiError::validation(
                "Validation Failed: 1: [request_cache] cannot be used in a scroll context;",
            ));
        }
        if body.get("size").and_then(|v| v.as_i64()) == Some(0) {
            return err_response(ApiError::validation(
                "Validation Failed: 1: [size] cannot be [0] in a scroll context;",
            ));
        }
        if req
            .param("track_total_hits")
            .map(|v| v != "true")
            .unwrap_or(false)
        {
            return err_response(ApiError::validation(
                "Validation Failed: 1: disabling [track_total_hits] is not allowed in a scroll context;",
            ));
        }
        if body.contains_key("search_after") {
            let reason = "`search_after` cannot be used in a scroll context.";
            let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
            e.root_cause = Some(Box::new(ApiError::new(
                "search_exception",
                reason,
                500,
            )));
            return err_response(e);
        }
        if body.contains_key("collapse") {
            let reason = "cannot use `collapse` in a scroll context";
            let mut e = ApiError::new("search_phase_execution_exception", reason, 500);
            e.root_cause = Some(Box::new(ApiError::new(
                "search_exception",
                reason,
                500,
            )));
            return err_response(e);
        }
    }
    let scroll_keep = req.param("scroll").map(|s| s.to_string());
    if let Some(k) = &scroll_keep {
        if let Err(e) = validate_scroll_keepalive(k, false) {
            return err_response(e);
        }
    }
    let body_value = Value::Object(body.clone());
    if let Err(e) = validate_scroll_slice(&body_value, scroll_keep.is_some()) {
        return err_response(e);
    }
    let default_index = names
        .first()
        .cloned()
        .unwrap_or_else(|| name.to_string());
    let sp_state =
        match crate::ingest::prepare_search_pipeline(
            st.get(),
            &default_index,
            &mut body,
            req.bool_param("verbose_pipeline"),
            req.param("search_pipeline"),
        ) {
            Ok(s) => s,
            Err(e) => return err_response(e),
        };
    let body = Value::Object(body);
    let derived_snap = if body.get("derived").is_some() {
        let mut snap = Vec::new();
        for n in &names {
            if let Some(idx) = st.get().indices.get(n) {
                snap.push((
                    n.clone(),
                    idx.mappings.get("derived").cloned(),
                    idx.doc_values.clone(),
                ));
            }
        }
        Some(snap)
    } else {
        None
    };
    if let Some(defs) = body.get("derived") {
        for n in &names {
            if let Some(idx) = st.get_mut().indices.get_mut(n) {
                if let Err(e) = crate::mapping::fields::apply_request_derived(idx, defs) {
                    return err_response(e);
                }
            }
        }
    }
    let cache_key = crate::model::visibility::cache_key(st.get(), &req.path, &req.query, &names, &body);
    let result = match cache_key.as_ref().and_then(|k| crate::model::visibility::cache_get(st.get(), &names, k)) {
        Some(v) => Ok(v),
        None => {
            crate::search::index::trace("before enter");
            let r = match st {
                StoreRef::Mut(store) => {
                    let view = crate::model::visibility::enter(store, &names);
                    let r = run_search(store, name, &names, &body, req);
                    crate::model::visibility::leave(store, view);
                    r
                }
                StoreRef::Shared(store) => run_search(store, name, &names, &body, req),
            };
            crate::search::index::trace("run_search done");
            if let (Some(k), Ok(v)) = (cache_key, &r) {
                crate::model::visibility::cache_put(st.get(), &names, k, v);
            }
            r
        }
    };
    if let Some(snap) = derived_snap {
        for (n, derived, doc_values) in snap {
            if let Some(idx) = st.get_mut().indices.get_mut(&n) {
                if let Some(d) = derived {
                    if let Some(m) = idx.mappings.as_object_mut() {
                        m.insert("derived".to_string(), d);
                    }
                } else if let Some(m) = idx.mappings.as_object_mut() {
                    m.remove("derived");
                }
                idx.doc_values = doc_values;
            }
        }
    }
    match result {
        Ok(mut v) => {
            if let Err(e) = crate::ingest::apply_search_response(&sp_state, &mut v) {
                return err_response(e);
            }
            if let Err(e) = apply_search_extras(&mut v, req, st.get(), &names, &body) {
                return err_response(e);
            }
            if sp_state.verbose {
                if let Some(m) = v.as_object_mut() {
                    m.insert(
                        "processor_results".to_string(),
                        serde_json::json!(sp_state.request_results),
                    );
                }
            }
            if let Some(_keep) = scroll_keep {
                let page = body_value
                    .get("size")
                    .and_then(|v| v.as_i64())
                    .unwrap_or(10)
                    .max(1) as usize;
                let slice = scroll_slice_of(&body_value);
                let mut raw_len = 0usize;
                if let Some(hits) = v
                    .get_mut("hits")
                    .and_then(|h| h.get_mut("hits"))
                    .and_then(|h| h.as_array_mut())
                {
                    raw_len = hits.len();
                    filter_slice_hits(hits, slice);
                }
                let store = st.get_mut();
                let id = scroll_new_id(store);
                store.scrolls.insert(
                    id.clone(),
                    ScrollState {
                        body: body_value.clone(),
                        names: names.clone(),
                        url_name: name.to_string(),
                        pos: raw_len,
                        page,
                        slice,
                        snapshot: names
                            .iter()
                            .filter_map(|n| store.indices.get(n).map(|i| (n.clone(), i.clone())))
                            .collect(),
                    },
                );
                let mut out = Map::new();
                out.insert("_scroll_id".to_string(), json!(id));
                if let Some(o) = v.as_object() {
                    for (k, val) in o {
                        if k != "_scroll_id" {
                            out.insert(k.clone(), val.clone());
                        }
                    }
                }
                return json_response(200, Value::Object(out));
            }
            json_response(200, v)
        }
        Err(e) => err_response(e),
    }
}


fn mapping_fields_of(idx: &Index) -> Vec<(String, String)> {
    fn walk(props: &Map<String, Value>, prefix: &str, out: &mut Vec<(String, String)>) {
        for (k, def) in props {
            let full = if prefix.is_empty() { k.clone() } else { format!("{}.{}", prefix, k) };
            let defm = match def.as_object() {
                Some(m) => m,
                None => continue,
            };
            let typ = defm
                .get("type")
                .and_then(|t| t.as_str())
                .map(|t| t.to_string())
                .unwrap_or_else(|| {
                    if defm.contains_key("properties") {
                        "object".to_string()
                    } else {
                        "object".to_string()
                    }
                });
            out.push((full.clone(), typ));
            if let Some(sub) = defm.get("properties").and_then(|p| p.as_object()) {
                walk(sub, &full, out);
            }
        }
    }
    let mut out = vec![];
    if let Some(props) = idx.mappings.get("properties").and_then(|p| p.as_object()) {
        walk(props, "", &mut out);
    }
    out
}

fn resolve_for_query(store: &Store, req: &Req, name: &str) -> Result<Vec<String>, ApiError> {
    let o = shared::parse_resolve_params(
        req,
        shared::ResolveOpts2 {
            closed_error: true,
            ..Default::default()
        },
    );
    shared::resolve_spec2(store, name, &o)
}

fn query_field_type(idx: &Index, field: &str) -> String {
    mapping_fields_of(idx)
        .into_iter()
        .find(|(f, _)| f == field)
        .map(|(_, t)| t)
        .unwrap_or_else(|| "object".to_string())
}

fn q_explanation(q: &str) -> String {
    let mut parts = Vec::new();
    for term in q.split(" AND ") {
        let term = term.trim();
        if term.is_empty() {
            continue;
        }
        if let Some((field, value)) = term.split_once(':') {
            if field == "title" {
                parts.push(format!("+{}:{}", field, value));
            } else {
                parts.push(format!("+ConstantScore({}:{})", field, value));
            }
        } else {
            parts.push(format!("+{}", term));
        }
    }
    parts.join(" ")
}

pub fn handle_validate_query(store: &mut Store, req: &Req, name: &str) -> Response {
    let names = match resolve_for_query(store, req, name) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let body = match req.json_body() {
        Ok(b) => Value::Object(b),
        Err(e) => return err_response(e),
    };
    let explain = req.param("explain") == Some("true")
        || req.param("rewrite") == Some("true");
    let all_shards = req.param("all_shards") == Some("true");
    let idx_name = names.first().cloned().unwrap_or_else(|| name.to_string());
    let shards = json!({"total": 1, "successful": 1, "failed": 0});
    let query = body.get("query").cloned();
    let qparam = req.param("q").filter(|q| !q.is_empty()).map(|q| q.to_string());
    if query.is_none() && qparam.is_none() {
        return err_response(ApiError::validation(
            "Validation Failed: 1: query cannot be null;",
        ));
    }
    if let Some(o) = body.as_object() {
        if o.keys().any(|k| k != "query") {
            return json_response(200, json!({"valid": false}));
        }
    }
    let idx = match store.indices.get(&idx_name) {
        Some(i) => i,
        None => return err_response(ApiError::not_found(&idx_name)),
    };
    if let Some(q) = qparam {
        let mut depth = 0i64;
        let mut bad = false;
        for ch in q.chars() {
            if ch == '(' {
                depth += 1;
            } else if ch == ')' {
                depth -= 1;
                if depth < 0 {
                    bad = true;
                }
            }
        }
        if bad || depth != 0 {
            if explain {
                let exp = json!({
                    "_shards": shards,
                    "valid": false,
                    "explanations": [{
                        "index": idx_name,
                        "valid": false,
                        "error": format!("Failed to parse query [{}]", q)
                    }]
                });
                return json_response(200, exp);
            }
            return json_response(200, json!({"_shards": shards, "valid": false}));
        }
        let explanation = q_explanation(&q);
        if explain {
            return json_response(
                200,
                json!({
                    "_shards": shards,
                    "valid": true,
                    "explanations": [{
                        "index": idx_name,
                        "valid": true,
                        "explanation": explanation
                    }]
                }),
            );
        }
        return json_response(200, json!({"_shards": shards, "valid": true}));
    }
    let query = query.unwrap();
    let qm = query.as_object().cloned().unwrap_or_default();
    let known = [
        "match_all", "match", "term", "terms", "range", "bool", "match_phrase",
        "multi_match", "wildcard", "fuzzy", "exists", "prefix", "ids", "match_bool_prefix",
        "query_string", "simple_query_string", "nested", "constant_score", "dis_max", "boosting",
        "geo_distance", "geo_bounding_box", "geo_shape", "function_score", "script", "script_score",
        "rank_feature", "knn", "distance_feature", "more_like_this", "regexp", "span_term",
    ];
    let mut unknown: Option<String> = None;
    for k in qm.keys() {
        if !known.contains(&k.as_str()) {
            unknown = Some(k.clone());
            break;
        }
    }
    if let Some(k) = unknown {
        if explain {
            return json_response(
                200,
                json!({
                    "valid": false,
                    "error": format!("ParsingException[unknown query [{}]]", k)
                }),
            );
        }
        return json_response(200, json!({"valid": false}));
    }
    let mut shard_error: Option<String> = None;
    if let Some(term) = qm.get("term").and_then(|t| t.as_object()) {
        if let Some((field, val)) = term.iter().next() {
            let ft = query_field_type(idx, field);
            if is_numeric_type(&ft) {
                if let Some(sval) = val.as_str() {
                    if sval.parse::<f64>().is_err() {
                        shard_error = Some(format!(
                            "NumberFormatException[For input string: \"{}\"]",
                            sval
                        ));
                    }
                }
            }
        }
    }
    if let Some(err) = shard_error {
        if explain {
            let mut exp = json!({
                "index": idx_name,
                "valid": false,
                "error": err
            });
            if all_shards {
                exp["shard"] = json!(0);
            }
            return json_response(
                200,
                json!({"_shards": shards, "valid": false, "explanations": [exp]}),
            );
        }
        return json_response(200, json!({"_shards": shards, "valid": false}));
    }
    if explain {
        let explanation = if let Some(m) = qm.get("match").and_then(|v| v.as_object()) {
            if let Some((field, val)) = m.iter().next() {
                let text = val.as_str().map(|s| s.to_string()).unwrap_or_default();
                let toks: Vec<String> = text
                    .split_whitespace()
                    .map(|t| format!("{}:{}", field, t))
                    .collect();
                toks.join(" ")
            } else {
                "MatchAllDocsQuery(*:*)".to_string()
            }
        } else if let Some(t) = qm.get("term").and_then(|v| v.as_object()) {
            t.iter()
                .next()
                .map(|(f, v)| format!("{}:{}", f, v.as_str().unwrap_or("")))
                .unwrap_or_default()
        } else if let Some(f) = qm.get("fuzzy").and_then(|v| v.as_object()) {
            f.iter()
                .next()
                .map(|(field, v)| {
                    let value = v
                        .as_str()
                        .or_else(|| v.get("value").and_then(|x| x.as_str()))
                        .unwrap_or("");
                    format!("{}:{}", field, value)
                })
                .unwrap_or_default()
        } else {
            "MatchAllDocsQuery(*:*)".to_string()
        };
        let mut exp = json!({
            "index": idx_name,
            "valid": true,
            "explanation": explanation
        });
        if all_shards {
            exp["shard"] = json!(0);
        }
        return json_response(
            200,
            json!({"_shards": shards, "valid": true, "explanations": [exp]}),
        );
    }
    json_response(200, json!({"_shards": shards, "valid": true}))
}

pub fn handle_field_caps(store: &mut Store, req: &Req, name: &str) -> Response {
    if req.method != "GET" && req.method != "POST" {
        return crate::http::method_not_allowed(req, &["GET", "POST"]);
    }
    let o = shared::parse_resolve_params(req, shared::ResolveOpts2::default());
    let names = match shared::resolve_spec2(store, name, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let mut fields_param = req.param("fields").unwrap_or("").to_string();
    if fields_param.is_empty() {
        if let Ok(b) = req.json_body() {
            if let Some(f) = b.get("fields") {
                match f {
                    Value::String(s) => fields_param = s.clone(),
                    Value::Array(a) => {
                        fields_param = a
                            .iter()
                            .filter_map(|v| v.as_str())
                            .collect::<Vec<_>>()
                            .join(",");
                    }
                    _ => {}
                }
            }
        }
    }
    if fields_param.trim().is_empty() {
        return err_response(ApiError::bad(
            "specified fields can't be null or empty",
        ));
    }
    let include_unmapped = req.param("include_unmapped") == Some("true");

    let metadata: Vec<(&str, Value)> = vec![
        ("_id", json!({"type": "_id", "searchable": true, "aggregatable": true})),
        ("_index", json!({"type": "_index", "searchable": true, "aggregatable": true})),
        ("_source", json!({"type": "_source", "searchable": false, "aggregatable": false})),
        ("_seq_no", json!({"type": "_seq_no", "searchable": true, "aggregatable": true})),
        ("_routing", json!({"type": "_routing", "searchable": true, "aggregatable": false})),
    ];

    let mut patterns: Vec<String> = vec![];
    for p in fields_param.split(',') {
        let p = p.trim();
        if !p.is_empty() {
            patterns.push(p.to_string());
        }
    }

    let mut all_fields: Vec<String> = vec![];
    for n in &names {
        for (f, _) in mapping_fields_of(&store.indices[n]) {
            if !all_fields.contains(&f) {
                all_fields.push(f);
            }
        }
        for f in store.indices[n].field_types.keys() {
            if !all_fields.contains(f) {
                all_fields.push(f.clone());
            }
        }
    }

    let mut selected: Vec<String> = vec![];
    let mut sel_meta: Vec<&str> = vec![];
    for p in &patterns {
        if p.contains('*') || p.contains('?') {
            let re = wildcard_regex(p);
            for f in &all_fields {
                if re.is_match(f) && !selected.contains(f) {
                    selected.push(f.clone());
                }
            }
            for (m, _) in &metadata {
                if re.is_match(m) && !sel_meta.contains(m) {
                    sel_meta.push(m);
                }
            }
        } else {
            if all_fields.contains(p) && !selected.contains(p) {
                selected.push(p.clone());
            }
            if let Some((m, _)) = metadata.iter().find(|(m, _)| *m == p) {
                if !sel_meta.contains(m) {
                    sel_meta.push(m);
                }
            }
        }
    }

    let mut out = Map::new();
    for m in &sel_meta {
        if let Some((_, v)) = metadata.iter().find(|(k, _)| k == m) {
            let mut entry = Map::new();
            entry.insert((*m).to_string(), v.clone());
            out.insert((*m).to_string(), Value::Object(entry));
        }
    }

    for field in &selected {
        let mut types: Vec<(String, Vec<String>)> = vec![];
        let mut present: Vec<String> = vec![];
        for n in &names {
            let idx = &store.indices[n];
            let ft = match idx.field_types.get(field) {
                Some(t) => t.clone(),
                None => match mapping_fields_of(idx).into_iter().find(|(f, _)| f == field) {
                    Some((_, t)) => t,
                    None => continue,
                },
            };
            present.push(n.clone());
            match types.iter_mut().find(|(t, _)| *t == ft) {
                Some((_, idxs)) => idxs.push(n.clone()),
                None => types.push((ft, vec![n.clone()])),
            }
        }
        if include_unmapped {
            for n in &names {
                if !present.contains(n) {
                    match types.iter_mut().find(|(t, _)| t == "unmapped") {
                        Some((_, idxs)) => idxs.push(n.clone()),
                        None => types.push(("unmapped".to_string(), vec![n.clone()])),
                    }
                }
            }
        }
        if types.is_empty() {
            continue;
        }
        let multi = types.len() > 1;
        let mut entry = Map::new();
        for (ft, idxs) in &types {
            let mut cap = if ft == "unmapped" {
                json!({"type": "unmapped", "searchable": false, "aggregatable": false})
            } else {
                let d = mapping_field_def(&store.indices[&idxs[0]], field)
                    .cloned()
                    .unwrap_or_default();
                let index_opt = d.get("index").and_then(|v| v.as_bool()).unwrap_or(true);
                let text_like = text_like_type(ft)
                    || matches!(ft.as_str(), "object" | "nested" | "annotated_text");
                let searchable = index_opt && !matches!(ft.as_str(), "object" | "nested");
                let has_dv = match d.get("doc_values").and_then(|v| v.as_bool()) {
                    Some(b) => b,
                    None => !text_like_type(ft),
                };
                let aggregatable = has_dv && !text_like;
                json!({"type": ft, "searchable": searchable, "aggregatable": aggregatable})
            };
            if multi {
                if let Some(obj) = cap.as_object_mut() {
                    obj.insert("indices".to_string(), json!(idxs));
                }
            }
            entry.insert(ft.clone(), cap);
        }
        out.insert(field.clone(), Value::Object(entry));
    }

    json_response(
        200,
        json!({
            "indices": names,
            "fields": out,
        }),
    )
}

pub fn handle_rank_eval(store: &mut Store, req: &Req, name: &str) -> Response {
    if req.method != "POST" && req.method != "GET" {
        return crate::http::method_not_allowed(req, &["POST", "GET"]);
    }
    let body = match req.json_body() {
        Ok(b) => Value::Object(b),
        Err(e) => return err_response(e),
    };
    let metric = body.get("metric").and_then(|v| v.as_object());
    let Some(metric) = metric else {
        let reason = "[rank_eval] failed to parse field [metric]";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.root_cause = Some(Box::new(ApiError::new(
            "named_object_not_found_exception",
            reason,
            400,
        )));
        return err_response(e);
    };
    let Some((metric_name, metric_opts)) = metric.iter().next() else {
        let reason = "[rank_eval] failed to parse field [metric]";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.root_cause = Some(Box::new(ApiError::new(
            "named_object_not_found_exception",
            reason,
            400,
        )));
        return err_response(e);
    };
    let opts = metric_opts.as_object().cloned().unwrap_or_default();
    let k = opts.get("k").and_then(to_f64).map(|n| n as usize).unwrap_or(10);
    let threshold = opts
        .get("relevant_rating_threshold")
        .and_then(to_f64)
        .unwrap_or(1.0);
    let known = matches!(
        metric_name.as_str(),
        "precision" | "recall" | "mean_reciprocal_rank"
    );
    if !known {
        let reason = "[rank_eval] failed to parse field [metric]";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.root_cause = Some(Box::new(ApiError::new(
            "named_object_not_found_exception",
            reason,
            400,
        )));
        return err_response(e);
    }
    let o = shared::parse_resolve_params(req, shared::ResolveOpts2::default());
    let names = match shared::resolve_spec2(store, name, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let result: Result<Value, ApiError> = (|| {
    let requests = body
        .get("requests")
        .and_then(|v| v.as_array())
        .cloned()
        .unwrap_or_default();
    let mut details = Map::new();
    let mut score_sum = 0.0f64;
    let mut count = 0f64;
    for rq in &requests {
        let Some(rm) = rq.as_object() else { continue };
        let id = rm
            .get("id")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let request = rm.get("request").and_then(|v| v.as_object()).cloned().unwrap_or_default();
        let query = request
            .get("query")
            .and_then(|v| v.as_object())
            .cloned()
            .unwrap_or_else(|| json!({"match_all": {}}).as_object().unwrap().clone());
        let mut ratings: HashMap<(String, String), f64> = HashMap::new();
        if let Some(list) = rm.get("ratings").and_then(|v| v.as_array()) {
            for r in list {
                let Some(obj) = r.as_object() else { continue };
                let rid = obj.get("_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                let ixn = obj
                    .get("_index")
                    .and_then(|v| v.as_str())
                    .unwrap_or_else(|| names.first().map(|s| s.as_str()).unwrap_or(""))
                    .to_string();
                let rating = obj.get("rating").and_then(to_f64).unwrap_or(0.0);
                ratings.insert((ixn, rid), rating);
            }
        }
        let mut hits: Vec<MultiHit> = Vec::new();
        for n in &names {
            let idx = &store.indices[n];
            let extra = precompute_doc_values(idx);
            for doc_id in &idx.order {
                let Some(doc) = idx.docs.get(doc_id) else { continue };
                if idx.pending.contains_key(doc_id) {
                    continue;
                }
                let (ok, score) = eval_doc(idx, doc_id, doc, &extra, &query)?;
                if ok {
                    let pos = idx
                        .order
                        .iter()
                        .position(|x| x == doc_id)
                        .unwrap_or(0) as i64;
                    hits.push(MultiHit {
                        idx_name: n.clone(),
                        id: doc_id.clone(),
                        doc,
                        score,
                        pos,
                        named: vec![],
                    });
                }
            }
        }
        let specs = parse_sort_specs(request.get("sort"))?;
        if specs.is_empty() {
            hits.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(Ordering::Equal));
        } else {
            let mut keyed: Vec<(Vec<(bool, Value)>, MultiHit)> = Vec::with_capacity(hits.len());
            for h in hits {
                let mut cells = Vec::with_capacity(specs.len());
                for sp in &specs {
                    cells.push(compute_sort_cell(store, &h, sp)?);
                }
                keyed.push((cells, h));
            }
            keyed.sort_by(|a, b| {
                for i in 0..specs.len() {
                    let ca = &a.0[i];
                    let cb = &b.0[i];
                    let mut c = cmp_sort_cells(ca, cb);
                    if specs[i].desc && !ca.0 && !cb.0 {
                        c = c.reverse();
                    }
                    if c != Ordering::Equal {
                        return c;
                    }
                }
                Ordering::Equal
            });
            hits = keyed.into_iter().map(|(_, h)| h).collect();
        }
        hits.truncate(k);
        let total_relevant = ratings.values().filter(|r| **r >= threshold).count();
        let mut relevant_retrieved = 0usize;
        let mut first_relevant: Option<usize> = None;
        let mut unrated = Vec::new();
        let mut hit_list = Vec::new();
        for (i, h) in hits.iter().enumerate() {
            let rating = ratings.get(&(h.idx_name.clone(), h.id.clone())).copied();
            if rating.is_none() {
                unrated.push(json!({"_index": h.idx_name, "_id": h.id}));
            }
            if let Some(r) = rating {
                if r >= threshold {
                    relevant_retrieved += 1;
                    if first_relevant.is_none() {
                        first_relevant = Some(i + 1);
                    }
                }
            }
            hit_list.push(json!({
                "hit": {"_index": h.idx_name, "_id": h.id},
                "rating": match rating { Some(r) => json!(r), None => Value::Null }
            }));
        }
        let (metric_score, metric_details) = match metric_name.as_str() {
            "recall" => {
                let s = if total_relevant == 0 {
                    0.0
                } else {
                    relevant_retrieved as f64 / total_relevant as f64
                };
                (
                    s,
                    json!({"recall": {
                        "relevant_docs_retrieved": relevant_retrieved,
                        "relevant_docs": total_relevant
                    }}),
                )
            }
            "mean_reciprocal_rank" => {
                let pos = first_relevant.unwrap_or(0);
                let s = if pos == 0 { 0.0 } else { 1.0 / pos as f64 };
                (
                    s,
                    json!({"mean_reciprocal_rank": {"first_relevant": pos}}),
                )
            }
            _ => {
                let retrieved = hits.len();
                let s = if retrieved == 0 {
                    0.0
                } else {
                    relevant_retrieved as f64 / retrieved as f64
                };
                (
                    s,
                    json!({"precision": {
                        "relevant_docs_retrieved": relevant_retrieved,
                        "docs_retrieved": retrieved
                    }}),
                )
            }
        };
        score_sum += metric_score;
        count += 1.0;
        details.insert(
            id,
            json!({
                "metric_score": metric_score,
                "unrated_docs": unrated,
                "hits": hit_list,
                "metric_details": metric_details
            }),
        );
    }
    let overall = if count == 0.0 { 0.0 } else { score_sum / count };
    Ok(json!({
        "metric_score": overall,
        "details": details,
        "failures": {}
    }))
    })();
    match result {
        Ok(v) => json_response(200, v),
        Err(e) => err_response(e),
    }
}
