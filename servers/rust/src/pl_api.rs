// Painless engine: script contexts, compilation, error rendering, the doc
// lookup and the _scripts/painless/_execute API.
//
// Public entry points for other parts of the server:
//   compile_script(src, context, name) -> Result<Script, ScriptErr>
//   Script::exec(&Bindings, name)      -> Result<PV, ScriptErr>
//   ScriptErr::object / error_body     -> the script_exception JSON
//   doc_lookup(Rc<Index>, id, values)  -> the `doc` variable
//   from_json / to_json / jstr (pl_value) for value conversion.

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::OnceLock;

use axum::response::Response;
use serde_json::{json, Map, Value};

use crate::errors::{err_response, json_response, ApiError};
use crate::eval::{mapping_field_def, resolve_alias_field, QueryCtx};
use crate::pl_check::{check, parse_sigs, CheckCtx, Info};
use crate::pl_interp::{declare, new_env, Interp, NativeFn};
use crate::pl_lex::CompileErr;
use crate::pl_parse::{parse, Expr, Program, Stmt, SK};
use crate::pl_time::Zdt;
use crate::pl_value::*;
use crate::store::Index;
use crate::{Req, Store};

fn contexts() -> &'static HashMap<&'static str, CheckCtx> {
    static C: OnceLock<HashMap<&'static str, CheckCtx>> = OnceLock::new();
    C.get_or_init(|| {
        let score_funcs = parse_sigs(&[
            "saturation:double(double,double)",
            "sigmoid:double(double,double,double)",
            "decayNumericGauss:double(double,double,double,double,double)",
            "decayNumericExp:double(double,double,double,double,double)",
            "decayNumericLinear:double(double,double,double,double,double)",
            "decayGeoGauss:double(String,String,String,double,def)",
            "decayGeoExp:double(String,String,String,double,def)",
            "decayGeoLinear:double(String,String,String,double,def)",
            "decayDateGauss:double(String,String,String,double,def)",
            "decayDateExp:double(String,String,String,double,def)",
            "decayDateLinear:double(String,String,String,double,def)",
            "randomScore:double(int)",
            "randomScore:double(int,String)",
            "l2Squared:double(def,def)",
            "l1Norm:double(def,def)",
            "lInfNorm:double(def,def)",
            "innerProduct:double(def,def)",
            "cosineSimilarity:double(def,def)",
            "hamming:double(def,def)",
        ]);
        let ctx = |vars: &[(&'static str, &'static str)], ret: &'static str| CheckCtx {
            vars: vars.to_vec(),
            funcs: HashMap::new(),
            ret,
        };
        let p = ("params", "Map");
        let doc = ("doc", "Map");
        let score = ("_score", "double");
        let mut m = HashMap::new();
        m.insert("painless_test", ctx(&[p], "Object"));
        m.insert("filter", ctx(&[p, doc], "boolean"));
        m.insert("score", CheckCtx { vars: vec![p, doc, score], funcs: score_funcs, ret: "double" });
        m.insert("field", ctx(&[p, doc], "Object"));
        m.insert("number_sort", ctx(&[p, doc, score], "double"));
        m.insert("string_sort", ctx(&[p, doc, score], "String"));
        m.insert("aggs", ctx(&[p, doc, ("_value", "def"), score], "Object"));
        m.insert("aggs_init", ctx(&[p, ("state", "Map")], "void"));
        m.insert("aggs_map", ctx(&[p, ("state", "Map"), doc, score], "void"));
        m.insert("aggs_combine", ctx(&[p, ("state", "Map")], "Object"));
        m.insert("aggs_reduce", ctx(&[p, ("states", "List")], "Object"));
        m.insert("bucket_aggregation", ctx(&[p], "Object"));
        m.insert("aggregation_selector", ctx(&[p], "boolean"));
        m.insert("terms_set", ctx(&[p, doc], "Object"));
        m.insert("update", ctx(&[p, ("ctx", "Map")], "void"));
        m.insert("ingest", ctx(&[p, ("ctx", "Map")], "void"));
        m.insert("processor_conditional", ctx(&[p, ("ctx", "Map")], "boolean"));
        m.insert("template", ctx(&[p], "String"));
        m
    })
}

/// A compiled script.
pub struct Script {
    pub src: String,
    prog: Program,
    info: Info,
    ctx: &'static CheckCtx,
}

/// A script_exception: a compile or runtime error with the script position.
#[derive(Clone)]
pub struct ScriptErr {
    pub compile: bool,
    pub link_err: bool,
    pub lang: String,
    pub script: String,
    pub stack: Vec<String>,
    /// offset, start, end
    pub pos: Option<(usize, usize, usize)>,
    pub cause_type: String,
    pub reason: Option<String>,
    pub inner: Option<CompileErr>,
    pub extra: Vec<(String, Value)>,
}

fn opt_str(s: &Option<String>) -> Value {
    match s {
        Some(s) => Value::String(s.clone()),
        None => Value::Null,
    }
}

impl ScriptErr {
    pub fn kind(&self) -> &'static str {
        if self.link_err {
            "link error"
        } else if self.compile {
            "compile error"
        } else {
            "runtime error"
        }
    }

    /// The script_exception object; root causes omit caused_by.
    pub fn object(&self, with_cause: bool) -> Value {
        let mut o = Map::new();
        o.insert("type".into(), json!("script_exception"));
        o.insert("reason".into(), json!(self.kind()));
        for (k, v) in &self.extra {
            o.insert(k.clone(), v.clone());
        }
        o.insert("script_stack".into(), json!(self.stack));
        o.insert("script".into(), json!(self.script));
        o.insert("lang".into(), json!(if self.lang.is_empty() { "painless" } else { &self.lang }));
        if let Some((off, start, end)) = self.pos {
            o.insert("position".into(), json!({"offset": off, "start": start, "end": end}));
        }
        if with_cause {
            o.insert("caused_by".into(), self.cause_object());
        }
        Value::Object(o)
    }

    pub fn cause_object(&self) -> Value {
        let mut c = Map::new();
        c.insert("type".into(), json!(self.cause_type));
        c.insert("reason".into(), opt_str(&self.reason));
        if let Some(inner) = &self.inner {
            c.insert("caused_by".into(), json!({"type": inner.typ, "reason": opt_str(&inner.reason)}));
        }
        Value::Object(c)
    }

    /// The full {"error": ..., "status": 400} response body.
    pub fn error_body(&self) -> Value {
        let mut err = Map::new();
        err.insert("root_cause".into(), json!([self.object(false)]));
        if let Value::Object(o) = self.object(true) {
            for (k, v) in o {
                err.insert(k, v);
            }
        }
        json!({"error": Value::Object(err), "status": 400})
    }
}

fn floor_boundary(s: &str, mut i: usize) -> usize {
    i = i.min(s.len());
    while !s.is_char_boundary(i) {
        i -= 1;
    }
    i
}

fn ceil_boundary(s: &str, mut i: usize) -> usize {
    i = i.min(s.len());
    while !s.is_char_boundary(i) {
        i += 1;
    }
    i
}

pub fn compile_failure(src: &str, name: &str, ce: &CompileErr) -> ScriptErr {
    let mut e = ScriptErr {
        compile: true,
        link_err: false,
        lang: String::new(),
        script: name.to_string(),
        stack: vec![],
        pos: None,
        cause_type: ce.typ.clone(),
        reason: ce.reason.clone(),
        inner: ce.cause.as_deref().cloned(),
        extra: vec![],
    };
    if ce.no_pos {
        return e;
    }
    let off = ce.offset.min(src.len());
    let start = floor_boundary(src, off.saturating_sub(25));
    let end = ceil_boundary(src, off + 25);
    let mut snippet = src[start..end].to_string();
    let mut pad = off - start;
    if start > 0 {
        snippet = format!("... {}", snippet);
        pad += 4;
    }
    if end < src.len() {
        snippet.push_str(" ...");
    }
    e.stack = vec![snippet, format!("{}^---- HERE", " ".repeat(pad))];
    e.pos = Some((off, start, end));
    e
}

fn runtime_failure(s: &Script, name: &str, re: &RtErr) -> ScriptErr {
    let src = &s.src;
    let off = re.offset.min(src.len());
    let (mut start, mut end) = (0, src.len());
    for &st in &s.prog.stmts {
        if st <= off && st >= start {
            start = st;
        }
    }
    for &st in &s.prog.stmts {
        if st > off && st < end {
            end = st;
        }
    }
    let mut stack = re.frames.clone();
    stack.push(src[start..end].to_string());
    stack.push(format!("{}^---- HERE", " ".repeat(off - start)));
    ScriptErr {
        compile: false,
        link_err: false,
        lang: String::new(),
        script: name.to_string(),
        stack,
        pos: Some((off, start, end)),
        cause_type: re.typ.clone(),
        reason: re.reason.clone(),
        inner: None,
        extra: re.extra.clone(),
    }
}

/// Parses and checks a script for a context (painless_test when unknown).
pub fn compile(src: &str, ctx_name: &str) -> Result<Script, CompileErr> {
    let ctxs = contexts();
    let ctx = ctxs.get(ctx_name).unwrap_or_else(|| &ctxs["painless_test"]);
    let prog = parse(src)?;
    let info = check(&prog, ctx)?;
    Ok(Script { src: src.to_string(), prog, info, ctx })
}

/// Compiles and converts errors to script exceptions. `name` is what the
/// error reports as "script" (the source when empty).
pub fn compile_script(src: &str, ctx_name: &str, name: &str) -> Result<Script, ScriptErr> {
    compile(src, ctx_name).map_err(|e| compile_failure(src, if name.is_empty() { src } else { name }, &e))
}

/// The variables and context functions a script runs with.
#[derive(Default)]
pub struct Bindings {
    pub vars: HashMap<String, PV>,
    pub funcs: HashMap<String, NativeFn>,
}

impl Script {
    /// Runs the script and applies the context's return conversion.
    pub fn exec(&self, b: &Bindings, name: &str) -> Result<PV, ScriptErr> {
        let inp = Interp::new(&self.prog, &self.info, &b.funcs);
        let env = new_env();
        for (k, t) in &self.ctx.vars {
            declare(&env, k, b.vars.get(*k).cloned().unwrap_or(PV::Null), t);
        }
        let r = inp.run(&env).and_then(|v| self.convert_return(v));
        r.map_err(|e| runtime_failure(self, if name.is_empty() { &self.src } else { name }, &e))
    }

    /// The expression whose value the script returns, for error offsets of
    /// failed return conversions.
    fn ret_node(&self) -> Option<&Expr> {
        fn walk<'x>(stmts: &'x [Stmt], found: &mut Option<&'x Expr>) {
            for st in stmts {
                match &st.kind {
                    SK::Return(Some(x)) => *found = Some(x),
                    SK::If { then, .. } => match &then.kind {
                        SK::Block(b) => walk(b, found),
                        _ => walk(std::slice::from_ref(&**then), found),
                    },
                    SK::Block(b) => walk(b, found),
                    _ => {}
                }
            }
            if let Some(Stmt { kind: SK::Expr(x), .. }) = stmts.last() {
                *found = Some(x);
            }
        }
        let mut found = None;
        walk(&self.prog.body, &mut found);
        found
    }

    fn convert_return(&self, v: PV) -> R<PV> {
        let off = self.ret_node().map(|n| n.pos).unwrap_or(0);
        match self.ctx.ret {
            "boolean" => match v {
                PV::Bool(_) => Ok(v),
                PV::Null => Err(npe(off)),
                other => Err(cce(off, &format!("cannot cast def [{}] to boolean", def_type_name(&other)))),
            },
            "double" => {
                if is_number(&v) {
                    return Ok(PV::Double(f64_of(&v)));
                }
                if v.is_null() {
                    return Err(npe(off));
                }
                Err(cce(off, &format!("cannot implicitly cast def [{}] to double", def_type_name(&v))))
            }
            "String" => match v {
                PV::Null | PV::Str(_) => Ok(v),
                other => Err(cce(
                    off,
                    &format!("cannot implicitly cast def [{}] to java.lang.String", def_type_name(&other)),
                )),
            },
            _ => Ok(v),
        }
    }
}

// ---- doc lookup ----

const TEXT_FIELDDATA_REASON: &str = "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [%s] in order to load field data by uninverting the inverted index. Note that this can use significant memory.";

fn json_f64(v: &Value) -> Option<f64> {
    match v {
        Value::Number(n) => n.to_string().parse().ok(),
        Value::String(s) => s.parse().ok(),
        _ => None,
    }
}

fn doc_values_for(idx: &Index, id: &str, vals: &Map<String, Value>, field: &str, off: usize) -> R<PV> {
    let qc = QueryCtx::with_values(idx, id, &Value::Null, vals.clone());
    let dv = |kind: &str, vals: Vec<PV>| Ok(PV::DocValues(Rc::new(PDocValues { kind: kind.into(), vals })));
    match field {
        "_id" => return dv("Strings", vec![PV::str(id)]),
        "_index" => return dv("Strings", vec![PV::str(&idx.name)]),
        "_seq_no" => return dv("Longs", vec![PV::Long(idx.seq_nos.get(id).copied().unwrap_or(0))]),
        _ => {}
    }
    let def = match mapping_field_def(idx, field) {
        Some(d) => d.clone(),
        None => {
            return Err(iae(off, &format!("No field found for [{}] in mapping", field)).frames(&[
                "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)",
                "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)",
            ]))
        }
    };
    let ft = def.get("type").and_then(|v| v.as_str()).unwrap_or("object").to_string();
    if ft == "alias" {
        let target = resolve_alias_field(idx, field);
        if target != field {
            return doc_values_for(idx, id, vals, &target, off);
        }
    }
    let (raw, _) = qc.values(field);
    match ft.as_str() {
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
                for t in qc.analyze_field(field, None, &s) {
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
    let vals = Rc::new(vals);
    let id = id.to_string();
    let cache: Rc<RefCell<HashMap<String, PV>>> = Rc::new(RefCell::new(HashMap::new()));
    let idx2 = idx.clone();
    let get = move |field: &str, off: usize| -> R<PV> {
        if let Some(v) = cache.borrow().get(field) {
            return Ok(v.clone());
        }
        let v = doc_values_for(&idx, &id, &vals, field, off)?;
        cache.borrow_mut().insert(field.to_string(), v.clone());
        Ok(v)
    };
    let has = move |field: &str| -> bool {
        matches!(field, "_id" | "_index" | "_seq_no" | "_version") || mapping_field_def(&idx2, field).is_some()
    };
    PV::DocMap(Rc::new(PDocMap { get: Box::new(get), has: Box::new(has) }))
}

// ---- score functions ----

pub fn score_bindings(id: String) -> HashMap<String, NativeFn> {
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
    for name in ["l2Squared", "l1Norm", "lInfNorm", "innerProduct", "cosineSimilarity", "hamming"] {
        m.insert(name.into(), Rc::new(move |_, a, off| vector_function(name, &a, off)));
    }
    m.insert(
        "randomScore".into(),
        Rc::new(move |_, a, _| {
            let key = format!("{}\x00{}", i64_of(&a[0]), id);
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
    Ok(PV::Double(crate::knn::raw(space, &query, &doc) as f32 as f64))
}

/// Runs f on a scoped thread with a large stack, so that deeply nested
/// scripts (parser, checker and interpreter all recurse) cannot overflow the
/// small stack of a server worker thread.
pub fn run_with_stack<T: Send>(f: impl FnOnce() -> T + Send) -> T {
    std::thread::scope(|s| {
        std::thread::Builder::new()
            .stack_size(256 << 20)
            .spawn_scoped(s, f)
            .expect("spawn script thread")
            .join()
            .expect("script thread panicked")
    })
}

// ---- _execute ----

fn bad(reason: &str) -> Response {
    err_response(ApiError::bad(reason))
}

pub fn handle_execute(store: &mut Store, req: &Req) -> Response {
    let raw = String::from_utf8_lossy(&req.body).to_string();
    if raw.trim().is_empty() && req.param("source").is_none() {
        return err_response(ApiError::new("parse_exception", "request body or source parameter is required", 400));
    }
    let body = match req.json_body() {
        Ok(m) => m,
        Err(e) => return err_response(e),
    };
    let sm = match body.get("script") {
        None => return bad("Required [script]"),
        Some(Value::Object(m)) => m.clone(),
        Some(_) => {
            let col = raw.find("\"script\"").map(|i| i + "\"script\"".len() + 2).unwrap_or(0);
            let reason = format!("[1:{}] [painless_execute_request] script doesn't support values of type: VALUE_STRING", col);
            let mut e = ApiError::new("x_content_parse_exception", &reason, 400);
            e.root_cause = Some(Box::new(ApiError::new("x_content_parse_exception", &reason, 400)));
            return err_response(e);
        }
    };
    let ctx_name = body.get("context").and_then(|v| v.as_str()).unwrap_or("painless_test").to_string();
    if !matches!(ctx_name.as_str(), "painless_test" | "filter" | "score") {
        let reason = "Failed to build [painless_execute_request] after last required field arrived";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.root_cause = Some(Box::new(ApiError::new("x_content_parse_exception", reason, 400)));
        e.caused_by = Some(Box::new(ApiError::new(
            "unsupported_operation_exception",
            &format!("unsupported script context name [{}]", ctx_name),
            400,
        )));
        return err_response(e);
    }
    if sm.contains_key("id") {
        return err_response(ApiError::validation("Validation Failed: 1: only inline scripts are supported;"));
    }
    let src = sm.get("source").and_then(|v| v.as_str()).unwrap_or("").to_string();
    if src.len() > 65535 {
        return bad(&format!(
            "exceeded max allowed inline script size in bytes [65535] with size [{}] for script [{}]",
            src.len(),
            src
        ));
    }
    match sm.get("lang").and_then(|v| v.as_str()).unwrap_or("") {
        "" | "painless" => {}
        "expression" => return bad(&format!("expression engine does not know how to handle script context [{}]", ctx_name)),
        "mustache" => return bad(&format!("mustache engine does not know how to handle context [{}]", ctx_name)),
        other => return bad(&format!("script_lang not supported [{}]", other)),
    }
    let params = match sm.get("params") {
        Some(p @ Value::Object(_)) => from_json(p, MapKind::Hash),
        _ => PV::map(PMap::new(MapKind::Hash)),
    };
    let mut b = Bindings::default();
    b.vars.insert("params".into(), params);
    if ctx_name != "painless_test" {
        let setup = body.get("context_setup").and_then(|v| v.as_object()).cloned().unwrap_or_default();
        let idx_name = setup.get("index").and_then(|v| v.as_str()).unwrap_or("").to_string();
        let idx = match store.indices.get(&idx_name) {
            Some(i) => i,
            None => {
                let mut f = Map::new();
                f.insert("resource.type".into(), json!("index_expression"));
                f.insert("resource.id".into(), json!(idx_name));
                f.insert("index_uuid".into(), json!("_na_"));
                f.insert("index".into(), json!(idx_name));
                return err_response(ApiError::not_found(&idx_name).with_fields(f));
            }
        };
        let doc = setup.get("document").and_then(|v| v.as_object()).cloned().unwrap_or_default();
        let mut tmp = Index::skeleton(&idx.name);
        tmp.uuid = idx.uuid.clone();
        tmp.settings = idx.settings.clone();
        tmp.mappings = idx.mappings.clone();
        tmp.field_types = idx.field_types.clone();
        let id = "_execute";
        let values: Map<String, Value> = match crate::fields::doc_values_for(&tmp, id, &doc) {
            Ok((vals, _)) => vals.into_iter().map(|(k, v)| (k, Value::Array(v))).collect(),
            Err(e) => return err_response(e),
        };
        tmp.docs.insert(id.into(), Value::Object(doc.clone()));
        tmp.order.push(id.into());
        tmp.seq_nos.insert(id.into(), 0);
        tmp.versions.insert(id.into(), 1);
        tmp.doc_values.insert(id.into(), values.clone());
        if ctx_name == "score" {
            let mut score = 0.0;
            if let Some(Value::Object(q)) = setup.get("query") {
                let docv = Value::Object(doc.clone());
                let qc = QueryCtx::new(&tmp, id, &docv);
                if let Ok((true, sc)) = qc.eval(q) {
                    score = sc as f32 as f64;
                }
            }
            b.vars.insert("_score".into(), PV::Double(score));
            b.funcs = score_bindings(id.to_string());
        }
        let tmp = Rc::new(tmp);
        b.vars.insert("doc".into(), doc_lookup(tmp, id, Some(values)));
    }
    let script = match compile_script(&src, &ctx_name, "") {
        Ok(s) => s,
        Err(e) => return json_response(400, e.error_body()),
    };
    let v = match script.exec(&b, "") {
        Ok(v) => v,
        Err(e) => return json_response(400, e.error_body()),
    };
    let result = match ctx_name.as_str() {
        "painless_test" => Value::String(jstr(&v)),
        "score" => {
            let f = f64_of(&v) as f32 as f64;
            serde_json::Number::from_f64(f).map(Value::Number).unwrap_or(Value::Null)
        }
        _ => to_json(&v),
    };
    json_response(200, json!({ "result": result }))
}
