use axum::response::Response;
use serde_json::{json, Map, Value};

use crate::errors::{err_response, json_response, method_not_allowed, ApiError};
use crate::shared::{resolve_spec2, validate_index_name, ResolveOpts2};
use crate::store::{ensure_properties, merge_mapping, settings_value_int, Store};
use crate::Req;

struct ReindexFailure {
    index: String,
    id: String,
    typ: String,
    rex: String,
    code: u16,
}

impl ReindexFailure {
    fn json(&self, dest_uuid: &str) -> Value {
        json!({
            "index": self.index,
            "id": self.id,
            "cause": {
                "type": self.typ,
                "reason": self.rex,
                "index": self.index,
                "shard": "0",
                "index_uuid": dest_uuid,
            },
            "status": self.code,
        })
    }
}

pub fn handle_reindex(store: &mut Store, req: &Req) -> Response {
    if req.method != "POST" {
        return method_not_allowed(req, &["POST"]);
    }
    let body = match req.json_body() {
        Ok(b) => b,
        Err(e) => return err_response(e),
    };
    for k in body.keys() {
        match k.as_str() {
            "source" | "dest" | "script" | "max_docs" | "conflicts" | "wait_for_completion"
            | "refresh" | "timeout" | "scroll" | "size" | "slices" => {}
            _ => {
                return err_response(ApiError::new(
                    "x_content_parse_exception",
                    &format!("[1:0] [reindex] unknown field [{}]", k),
                    400,
                ))
            }
        }
    }
    let src_raw = body.get("source").and_then(|s| s.as_object());
    let dest_raw = body.get("dest").and_then(|d| d.as_object());
    let dest_raw = match dest_raw {
        Some(d) => d,
        None => {
            return err_response(ApiError::validation(
                "Validation Failed: 1: index must be specified;",
            ))
        }
    };
    let dest_name = dest_raw
        .get("index")
        .and_then(|i| i.as_str())
        .unwrap_or("")
        .to_string();
    let dest_pipeline = dest_raw
        .get("pipeline")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string());
    if dest_name.is_empty() {
        return err_response(ApiError::validation(
            "Validation Failed: 1: index must be specified;",
        ));
    }
    let src_raw = match src_raw {
        Some(s) if s.get("index").is_some() => s,
        _ => {
            return err_response(ApiError::validation(
                "Validation Failed: 1: use _all if you really want to copy from all existing indexes;",
            ))
        }
    };
    if let Some(remote) = src_raw.get("remote").and_then(|r| r.as_object()) {
        let host = remote
            .get("host")
            .and_then(|h| h.as_str())
            .unwrap_or("")
            .trim_start_matches("http://")
            .trim_start_matches("https://")
            .to_string();
        return err_response(ApiError::bad(&format!(
            "[{}] not allowlisted in reindex.remote.allowlist",
            host
        )));
    }
    let mut src_specs: Vec<String> = vec![];
    match src_raw.get("index") {
        Some(Value::String(s)) => src_specs.push(s.clone()),
        Some(Value::Array(a)) => {
            for v in a {
                if let Some(s) = v.as_str() {
                    src_specs.push(s.to_string());
                }
            }
        }
        _ => {}
    }
    let src_spec = src_specs.join(",");
    if src_spec.is_empty() {
        return err_response(ApiError::validation(
            "Validation Failed: 1: use _all if you really want to copy from all existing indexes;",
        ));
    }
    if src_spec == dest_name {
        return err_response(ApiError::validation(&format!(
            "Validation Failed: 1: reindex cannot write into an index its reading from [{}];",
            src_spec
        )));
    }
    let mut max_docs: i64 = -1;
    if let Some(v) = body.get("max_docs") {
        if !v.is_null() {
            match settings_value_int(v) {
                None => {
                    return err_response(
                        ApiError::new(
                            "x_content_parse_exception",
                            "[1:0] [reindex] failed to parse field [max_docs]",
                            400,
                        )
                        .caused_by(ApiError::bad("[max_docs] should be >= [slices]")),
                    )
                }
                Some(n) => {
                    if n < 0 {
                        return err_response(
                            ApiError::new(
                                "x_content_parse_exception",
                                "[1:0] [reindex] failed to parse field [max_docs]",
                                400,
                            )
                            .caused_by(ApiError::bad(&format!(
                                "[max_docs] parameter cannot be negative, found [{}]",
                                n
                            ))),
                        );
                    }
                    if n < 1 {
                        return err_response(
                            ApiError::new(
                                "x_content_parse_exception",
                                "[1:0] [reindex] failed to parse field [max_docs]",
                                400,
                            )
                            .caused_by(ApiError::bad("[max_docs] should be >= [slices]")),
                        );
                    }
                    max_docs = n;
                }
            }
        }
    }
    let conflicts_proceed = body.get("conflicts").and_then(|c| c.as_str()) == Some("proceed");
    let mut size: i64 = 1000;
    if let Some(v) = src_raw.get("size") {
        if let Some(n) = settings_value_int(v) {
            if n > 0 {
                size = n;
            }
        }
    }
    let op_type = dest_raw
        .get("op_type")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let version_type = dest_raw
        .get("version_type")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let script_raw = body.get("script").cloned();
    let query = src_raw.get("query").cloned();
    let mut source_include: Option<Vec<String>> = None;
    if let Some(Value::Array(si)) = src_raw.get("_source") {
        let mut out = vec![];
        for f in si {
            if let Some(fs) = f.as_str() {
                out.push(fs.to_string());
            }
        }
        source_include = Some(out);
    }

    let mut o = ResolveOpts2::default();
    o.allow_no_indices = Some(false);
    let src_indices = match resolve_spec2(store, &src_spec, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let mut items: Vec<(String, String, Value)> = vec![];
    for sidx in &src_indices {
        let idx = &store.indices[sidx];
        let extra = crate::search::precompute_doc_values(idx);
        for id in &idx.order {
            let doc = match idx.docs.get(id) {
                Some(d) => d,
                None => continue,
            };
            if let Some(q) = &query {
                if let Some(qm) = q.as_object() {
                    match crate::search::eval_doc(idx, id, doc, &extra, qm) {
                        Ok((ok, _)) => {
                            if !ok {
                                continue;
                            }
                        }
                        Err(e) => return err_response(e),
                    }
                }
            }
            items.push((sidx.clone(), id.clone(), doc.clone()));
        }
    }
    if max_docs >= 0 && items.len() as i64 > max_docs {
        items.truncate(max_docs as usize);
    }
    let total = items.len();
    let mut batches = 0;
    if total > 0 {
        batches = (total as i64 + size - 1) / size;
    }

    let (mut created, mut updated, mut deleted, mut noops, mut version_conflicts) =
        (0i64, 0i64, 0i64, 0i64, 0i64);
    let mut failures: Vec<Value> = vec![];
    let mut dest_created = store.indices.contains_key(&dest_name);

    let base_dest = dest_name.clone();
    for (it_idx, it_id, it_doc) in items {
        let mut doc = match it_doc.as_object() {
            Some(m) => m.clone(),
            None => Map::new(),
        };
        let mut id = it_id.clone();
        let mut dest_name = base_dest.clone();
        if let Some(raw) = &script_raw {
            let routing = store.indices.get(&it_idx).and_then(|i| i.routing.get(&it_id)).cloned();
            let res = match crate::script_ctx::run_update_script(raw, &doc, &it_idx, &it_id, -1, "index", routing.as_deref()) {
                Ok(r) => r,
                Err(e) => return err_response(crate::script_ctx::bulk_script_failure(&e)),
            };
            match res.raw_op.as_str() {
                "noop" => {
                    noops += 1;
                    continue;
                }
                "delete" => {
                    deleted += 1;
                    continue;
                }
                "index" | "create" => {}
                other => {
                    return err_response(ApiError::bad(&format!(
                        "Operation type [{}] not allowed, only [noop, index, delete] are allowed",
                        other
                    )))
                }
            }
            doc = res.source;
            id = match &res.id {
                Value::Null => store.next_id(),
                v => crate::script_ctx::value_text(v),
            };
            if let Value::String(n) = &res.index {
                if *n != it_idx {
                    dest_name = n.clone();
                }
            }
        }
        dest_created = store.indices.contains_key(&dest_name);
        let dest_name_err = validate_index_name(&dest_name).err();
        let mut dest_uuid = store.indices.get(&dest_name).map(|i| i.uuid.clone()).unwrap_or_else(|| "_na_".to_string());
        if let Some(pname) = &dest_pipeline {
            let mut pm = Map::new();
            pm.insert("_index".to_string(), json!(dest_name));
            pm.insert("_id".to_string(), json!(id));
            match crate::ingest::run_ingest_pipeline(store, pname, doc.clone(), pm) {
                Ok(out) => {
                    if out.dropped {
                        noops += 1;
                        continue;
                    }
                    doc = out.source;
                    if let Some(nid) = out.meta.get("_id").and_then(|v| v.as_str()) {
                        if !nid.is_empty() {
                            id = nid.to_string();
                        }
                    }
                }
                Err(e) => {
                    failures.push(
                        ReindexFailure {
                            index: dest_name.clone(),
                            id: it_id.clone(),
                            typ: e.typ.clone(),
                            rex: e.reason.clone(),
                            code: e.status,
                        }
                        .json("_na_"),
                    );
                    continue;
                }
            }
        }
        if !dest_created {
            if let Some(e) = &dest_name_err {
                failures.push(
                    ReindexFailure {
                        index: dest_name.clone(),
                        id: it_id.clone(),
                        typ: e.typ.clone(),
                        rex: e.reason.clone(),
                        code: e.status,
                    }
                    .json("_na_"),
                );
                continue;
            }
            match crate::admin::create_index_internal(store, &dest_name, &Map::new(), false) {
                Ok(()) => {
                    dest_created = true;
                    dest_uuid = store.indices[&dest_name].uuid.clone();
                }
                Err(e) => {
                    failures.push(
                        ReindexFailure {
                            index: dest_name.clone(),
                            id: it_id.clone(),
                            typ: e.typ.clone(),
                            rex: e.reason.clone(),
                            code: e.status,
                        }
                        .json("_na_"),
                    );
                    continue;
                }
            }
        }
        let existing = store.indices[&dest_name].docs.contains_key(&id);
        if existing && (op_type == "create" || version_type == "external" || version_type == "external_gte")
        {
            version_conflicts += 1;
            if !conflicts_proceed {
                let reason = if op_type == "create" {
                    format!(
                        "[{}]: version conflict, document already exists (current version [1])",
                        id
                    )
                } else {
                    format!(
                        "[{}]: version conflict, current version [1] is higher or equal to the one provided [1]",
                        id
                    )
                };
                failures.push(
                    ReindexFailure {
                        index: dest_name.clone(),
                        id: it_id.clone(),
                        typ: "version_conflict_engine_exception".to_string(),
                        rex: reason,
                        code: 409,
                    }
                    .json(&dest_uuid),
                );
            }
            continue;
        }
        if let Some(include) = &source_include {
            let mut filtered = Map::new();
            for f in include {
                if let Some(v) = doc.get(f) {
                    filtered.insert(f.clone(), v.clone());
                }
            }
            doc = filtered;
        }
        if existing {
            updated += 1;
        } else {
            created += 1;
        }
        let vres = {
            let idx = store.indices.get_mut(&dest_name).unwrap();
            {
                let props = ensure_properties(&mut idx.mappings);
                merge_mapping(props, &doc);
            }
            idx.ensure_field_types();
            let mut doc_mut = doc.clone();
            let res = crate::fields::compute_doc_values(idx, &id, &mut doc_mut);
            if res.is_ok() {
                doc = doc_mut;
            }
            res
        };
        if let Err(e) = vres {
            if existing {
                updated -= 1;
            } else {
                created -= 1;
            }
            failures.push(
                ReindexFailure {
                    index: dest_name.clone(),
                    id: id.clone(),
                    typ: e.typ.clone(),
                    rex: e.reason.clone(),
                    code: e.status,
                }
                .json(&dest_uuid),
            );
            continue;
        }
        let idx = store.indices.get_mut(&dest_name).unwrap();
        if !existing {
            idx.order.push(id.clone());
        }
        idx.docs.insert(id.clone(), Value::Object(doc));
        idx.index_total += 1;
    }
    if dest_created {
        store.bump_state();
    }
    let status = if version_conflicts > 0 && !conflicts_proceed {
        409
    } else if !failures.is_empty() {
        400
    } else {
        200
    };
    json_response(
        status,
        json!({
            "took": 1,
            "timed_out": false,
            "total": total,
            "updated": updated,
            "created": created,
            "deleted": deleted,
            "batches": batches,
            "version_conflicts": version_conflicts,
            "noops": noops,
            "retries": {"bulk": 0, "search": 0},
            "throttled_millis": 0,
            "requests_per_second": -1.0,
            "throttled_until_millis": 0,
            "failures": failures,
        }),
    )
}
