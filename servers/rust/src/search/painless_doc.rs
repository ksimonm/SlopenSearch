//! The painless `doc` variable (LeafDocLookup) over an index's doc values,
//! and the score functions of score scripts (decayNumericGauss, cosineSimilarity, ...).

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;
use serde_json::{Map, Value};
use crate::search::eval::QueryCtx;
use crate::mapping::{mapping_field_def, resolve_alias_field};
use crate::painless::interp::NativeFn;
use crate::painless::time::Zdt;
use crate::painless::value::*;
use crate::model::Index;

const TEXT_FIELDDATA_REASON: &str = "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [%s] in order to load field data by uninverting the inverted index. Note that this can use significant memory.";

fn json_f64(v: &Value) -> Option<f64> {
    match v {
        Value::Number(n) => n.to_string().parse().ok(),
        Value::String(s) => s.parse().ok(),
        _ => None,
    }
}

/// What a doc lookup needs of a mapped field.
struct FieldInfo {
    def: Map<String, Value>,
    ft: String,
    /// The target of an alias (when it is another field).
    alias_of: Option<String>,
    /// The doc values key (`QueryCtx::values_in`); None: not readable here (nested).
    values_key: Option<String>,
}

thread_local! {
    /// Field infos by skeleton index (kept for the process, see `script_ctx::skeleton`) and field.
    static FIELD_INFOS: RefCell<HashMap<(usize, String), Option<Rc<FieldInfo>>>> = RefCell::new(HashMap::new());
}

fn field_info(idx: &Index, field: &str) -> Option<Rc<FieldInfo>> {
    let key = (idx as *const Index as usize, field.to_string());
    if let Some(i) = FIELD_INFOS.with(|c| c.borrow().get(&key).cloned()) {
        return i;
    }
    let info = mapping_field_def(idx, field).map(|d| {
        let ft = d.get("type").and_then(|v| v.as_str()).unwrap_or("object").to_string();
        let target = resolve_alias_field(idx, field);
        let qc = QueryCtx::new(idx, "", &Value::Null);
        let values_key = qc.values_key(field);
        Rc::new(FieldInfo { def: d.clone(), alias_of: (ft == "alias" && target != field).then_some(target), ft, values_key })
    });
    FIELD_INFOS.with(|c| {
        let mut c = c.borrow_mut();
        if c.len() > 4096 {
            c.clear();
        }
        c.insert(key, info.clone());
    });
    info
}

fn doc_values_for(idx: &Index, id: &str, vals: &Map<String, Value>, field: &str, off: usize) -> R<PV> {
    let dv = |kind: &str, vals: Vec<PV>| Ok(PV::DocValues(Rc::new(PDocValues { kind: kind.into(), vals })));
    match field {
        "_id" => return dv("Strings", vec![PV::str(id)]),
        "_index" => return dv("Strings", vec![PV::str(&idx.name)]),
        "_seq_no" => return dv("Longs", vec![PV::Long(idx.seq_nos.get(id).copied().unwrap_or(0))]),
        _ => {}
    }
    let info = match field_info(idx, field) {
        Some(i) => i,
        None => {
            return Err(iae(off, &format!("No field found for [{}] in mapping", field)).frames(&[
                "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)",
                "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)",
            ]))
        }
    };
    let (def, ft) = (&info.def, info.ft.as_str());
    if ft == "alias" {
        if let Some(target) = &info.alias_of {
            return doc_values_for(idx, id, vals, target, off);
        }
    }
    let raw = match &info.values_key {
        None => Vec::new(),
        Some(key) => match vals.get(key) {
            Some(v) => crate::search::eval::array_values(v),
            None => match (key != field).then(|| vals.get(field)).flatten() {
                Some(v) => crate::search::eval::array_values(v),
                None => Vec::new(),
            },
        },
    };
    match ft {
        "text" | "match_only_text" | "search_as_you_type" => {
            if def.get("fielddata").and_then(|v| v.as_bool()) != Some(true) {
                return Err(iae(off, &TEXT_FIELDDATA_REASON.replace("%s", field)).frames(&[
                    "org.opensearch.index.mapper.TextFieldMapper$TextFieldType.fielddataBuilder(TextFieldMapper.java:1004)",
                    "org.opensearch.index.fielddata.IndexFieldDataService.getForField(IndexFieldDataService.java:134)",
                    "org.opensearch.index.query.QueryShardContext.lambda$lookup$0(QueryShardContext.java:530)",
                    "org.opensearch.search.lookup.SearchLookup.lambda$new$0(SearchLookup.java:118)",
                    "org.opensearch.search.lookup.LeafDocLookup.lambda$get$0(LeafDocLookup.java:93)",
                    "org.opensearch.secure_sm.AccessController.doPrivileged(AccessController.java:76)",
                    "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:93)",
                    "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)",
                ]));
            }
            let mut terms: Vec<String> = Vec::new();
            for v in &raw {
                let s = match v {
                    Value::String(s) => s.clone(),
                    other => other.to_string(),
                };
                for t in QueryCtx::new(idx, id, &Value::Null).analyze_field(field, None, &s) {
                    if !terms.contains(&t) {
                        terms.push(t);
                    }
                }
            }
            terms.sort();
            dv("Strings", terms.iter().map(|t| PV::str(t)).collect())
        }
        "integer" | "long" | "short" | "byte" | "unsigned_long" | "token_count" => {
            let mut out: Vec<i64> = raw.iter().filter_map(json_f64).map(|f| f as i64).collect();
            out.sort();
            dv("Longs", out.into_iter().map(PV::Long).collect())
        }
        "float" | "double" | "half_float" | "scaled_float" => {
            let mut out: Vec<f64> = raw.iter().filter_map(json_f64).collect();
            out.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
            dv("Doubles", out.into_iter().map(PV::Double).collect())
        }
        "boolean" => {
            let mut out: Vec<bool> = raw
                .iter()
                .filter_map(|v| match v {
                    Value::Bool(b) => Some(*b),
                    Value::String(s) => Some(s == "true"),
                    _ => None,
                })
                .collect();
            out.sort();
            dv("Booleans", out.into_iter().map(PV::Bool).collect())
        }
        "date" | "date_nanos" => {
            let mut ms: Vec<i64> = raw.iter().filter_map(json_f64).map(|f| f as i64).collect();
            if ft == "date_nanos" {
                ms = ms.iter().map(|n| n.div_euclid(1_000_000)).collect();
            }
            ms.sort();
            dv("Dates", ms.into_iter().map(|m| PV::Zdt(Rc::new(Zdt::from_millis(m)))).collect())
        }
        "knn_vector" => {
            let vec: Vec<PV> = raw.iter().filter_map(json_f64).map(|f| PV::Float(f as f32)).collect();
            if vec.is_empty() {
                return dv("KnnVector", Vec::new());
            }
            let arr = PV::Array(Rc::new(PArray { elem: "float".into(), a: RefCell::new(vec) }));
            dv("KnnVector", vec![arr])
        }
        "geo_point" => {
            let out = raw
                .iter()
                .filter_map(|v| match v {
                    Value::Object(m) => Some(PV::GeoPoint(
                        m.get("lat").and_then(json_f64).unwrap_or(0.0),
                        m.get("lon").and_then(json_f64).unwrap_or(0.0),
                    )),
                    Value::Array(a) if a.len() == 2 => {
                        Some(PV::GeoPoint(json_f64(&a[1]).unwrap_or(0.0), json_f64(&a[0]).unwrap_or(0.0)))
                    }
                    _ => None,
                })
                .collect();
            dv("GeoPoints", out)
        }
        _ => {
            let mut out: Vec<String> = raw
                .iter()
                .map(|v| match v {
                    Value::String(s) => s.clone(),
                    other => other.to_string(),
                })
                .collect();
            out.sort();
            dv("Strings", out.iter().map(|s| PV::str(s)).collect())
        }
    }
}

/// Builds the `doc` variable (LeafDocLookup) for one document. `values` are
/// the document's indexed values (fields::doc_values_for), or None to use the
/// index's stored doc values of `id`.
pub fn doc_lookup(idx: Rc<Index>, id: &str, values: Option<Map<String, Value>>) -> PV {
    let vals = values.unwrap_or_else(|| idx.doc_values.get(id).cloned().unwrap_or_default());
    lookup_over(idx, id, Some(Rc::new(vals)))
}

thread_local! {
    /// The doc values `doc_lookup_borrowed` lookups read (set by `with_doc_values`).
    static CUR_VALUES: std::cell::Cell<*const Map<String, Value>> = const { std::cell::Cell::new(std::ptr::null()) };
}

/// Runs `f` with `vals` as the doc values of the lookups made by `doc_lookup_borrowed`.
pub fn with_doc_values<T>(vals: &Map<String, Value>, f: impl FnOnce() -> T) -> T {
    struct Restore(*const Map<String, Value>);
    impl Drop for Restore {
        fn drop(&mut self) {
            CUR_VALUES.with(|c| c.set(self.0));
        }
    }
    let _restore = Restore(CUR_VALUES.with(|c| c.replace(vals as *const _)));
    f()
}

/// `doc_lookup` over the doc values of the enclosing `with_doc_values` (no copy of them);
/// outside of it the document has no values.
pub fn doc_lookup_borrowed(idx: Rc<Index>, id: &str) -> PV {
    lookup_over(idx, id, None)
}

fn lookup_over(idx: Rc<Index>, id: &str, owned: Option<Rc<Map<String, Value>>>) -> PV {
    let id = id.to_string();
    let cache: Rc<RefCell<HashMap<String, PV>>> = Rc::new(RefCell::new(HashMap::new()));
    let idx2 = idx.clone();
    let get = move |field: &str, off: usize| -> R<PV> {
        if let Some(v) = cache.borrow().get(field) {
            return Ok(v.clone());
        }
        let v = match &owned {
            Some(vals) => doc_values_for(&idx, &id, vals, field, off)?,
            None => {
                let p = CUR_VALUES.with(|c| c.get());
                // SAFETY: set only while `with_doc_values` borrows the map
                let empty = Map::new();
                let vals = if p.is_null() { &empty } else { unsafe { &*p } };
                doc_values_for(&idx, &id, vals, field, off)?
            }
        };
        cache.borrow_mut().insert(field.to_string(), v.clone());
        Ok(v)
    };
    let has = move |field: &str| -> bool {
        matches!(field, "_id" | "_index" | "_seq_no" | "_version") || mapping_field_def(&idx2, field).is_some()
    };
    PV::DocMap(Rc::new(PDocMap { get: Box::new(get), has: Box::new(has) }))
}

thread_local! {
    /// The score functions (built once per thread) and the id of the document they run for.
    static SCORE_FUNCS: RefCell<Option<Rc<HashMap<String, NativeFn>>>> = const { RefCell::new(None) };
    static SCORE_ID: RefCell<String> = const { RefCell::new(String::new()) };
}

/// The score context's functions for document `id`.
pub fn score_bindings(id: &str) -> Rc<HashMap<String, NativeFn>> {
    SCORE_ID.with(|c| {
        let mut c = c.borrow_mut();
        c.clear();
        c.push_str(id);
    });
    SCORE_FUNCS.with(|c| c.borrow_mut().get_or_insert_with(|| Rc::new(build_score_bindings())).clone())
}

fn build_score_bindings() -> HashMap<String, NativeFn> {
    fn f(a: &[PV], i: usize) -> f64 {
        f64_of(&a[i])
    }
    fn dist(a: &[PV]) -> f64 {
        ((f(a, 4) - f(a, 0)).abs() - f(a, 2)).max(0.0)
    }
    let mut m: HashMap<String, NativeFn> = HashMap::new();
    m.insert("saturation".into(), Rc::new(|_, a, _| Ok(PV::Double(f(&a, 0) / (f(&a, 1) + f(&a, 0))))));
    m.insert(
        "sigmoid".into(),
        Rc::new(|_, a, _| {
            let (v, k, e) = (f(&a, 0), f(&a, 1), f(&a, 2));
            Ok(PV::Double(v.powf(e) / (k.powf(e) + v.powf(e))))
        }),
    );
    m.insert(
        "decayNumericLinear".into(),
        Rc::new(|_, a, _| {
            let scaling = f(&a, 1) / (1.0 - f(&a, 3));
            Ok(PV::Double(((scaling - dist(&a)) / scaling).max(0.0)))
        }),
    );
    m.insert(
        "decayNumericExp".into(),
        Rc::new(|_, a, _| {
            let scaling = f(&a, 3).ln() / f(&a, 1);
            Ok(PV::Double((scaling * dist(&a)).exp()))
        }),
    );
    m.insert(
        "decayNumericGauss".into(),
        Rc::new(|_, a, _| {
            let scaling = 0.5 * f(&a, 1).powi(2) / f(&a, 3).ln();
            Ok(PV::Double((0.5 * dist(&a).powi(2) / scaling).exp()))
        }),
    );
    crate::painless::decay::add_decay_bindings(&mut m);
    for name in ["l2Squared", "l1Norm", "lInfNorm", "innerProduct", "cosineSimilarity", "hamming"] {
        m.insert(name.into(), Rc::new(move |_, a, off| vector_function(name, &a, off)));
    }
    m.insert(
        "randomScore".into(),
        Rc::new(|_, a, _| {
            let key = SCORE_ID.with(|id| format!("{}\x00{}", i64_of(&a[0]), id.borrow()));
            let mut h: u32 = 0x811c9dc5;
            for b in key.bytes() {
                h ^= b as u32;
                h = h.wrapping_mul(0x01000193);
            }
            Ok(PV::Double((h & 0x00FF_FFFF) as f64 / (1u32 << 24) as f64))
        }),
    );
    m
}

/// The k-NN plugin's painless vector functions (KNNScoringUtil) over a query
/// vector and a knn_vector doc value.
fn vector_function(name: &str, a: &[PV], off: usize) -> R<PV> {
    let query: Vec<f64> = match &a[0] {
        PV::List(l) => l.borrow().iter().map(f64_of).collect(),
        PV::Array(arr) => arr.a.borrow().iter().map(f64_of).collect(),
        PV::Null => return Err(npe(off)),
        other => return Err(cce(off, &format!("Cannot cast {} to java.util.List", class_name(other)))),
    };
    let doc: Vec<f64> = match &a[1] {
        PV::DocValues(d) if d.kind == "KnnVector" => match d.vals.first() {
            Some(PV::Array(arr)) => arr.a.borrow().iter().map(f64_of).collect(),
            _ => {
                return Err(RtErr::new(
                    "illegal_state_exception",
                    "KNN vector does not have a value for this document",
                    off,
                ))
            }
        },
        other => {
            return Err(iae(
                off,
                &format!("Incompatible type {} for KNN vector function", class_name(other)),
            ))
        }
    };
    if query.len() != doc.len() {
        return Err(iae(
            off,
            &format!(
                "The query vector has a different number of dimensions [{}] than the document vectors [{}].",
                query.len(),
                doc.len()
            ),
        ));
    }
    let space = match name {
        "l2Squared" => "l2",
        "l1Norm" => "l1",
        "lInfNorm" => "linf",
        "innerProduct" => "innerproduct",
        "cosineSimilarity" => "cosinesimil",
        _ => "hamming",
    };
    Ok(PV::Double(crate::mapping::knn::raw(space, &query, &doc) as f32 as f64))
}
