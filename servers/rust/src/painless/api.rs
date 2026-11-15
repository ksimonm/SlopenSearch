// Painless engine: script contexts, compilation, error rendering, the doc
// lookup and the _scripts/painless/_execute API.
//
// Public entry points for other parts of the server:
//   compile_script(src, context, name) -> Result<Script, ScriptErr>
//   Script::exec(&Bindings, name)      -> Result<PV, ScriptErr>
//   ScriptErr::object / error_body     -> the script_exception JSON
//   doc_lookup(Rc<Index>, id, values)  -> the `doc` variable
//   from_json / to_json / jstr (pl_value) for value conversion.

use std::collections::HashMap;
use std::sync::OnceLock;

use serde_json::{json, Map, Value};

use crate::painless::check::{check, parse_sigs, CheckCtx, Info};
use crate::painless::interp::{declare, new_env, Interp, NativeFn};
use crate::painless::lex::CompileErr;
use crate::painless::parse::{parse, Expr, Program, Stmt, SK};
use crate::painless::value::*;

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

// ---- score functions ----

/// Stack size of every thread that may compile or run scripts or otherwise
/// recurse deeply (the Painless parser, checker and interpreter, nested
/// queries): the server's runtime threads (see main.rs) and task threads.
/// Explicit depth limits turn too-deep input into errors long before this
/// is exhausted.
pub const DEEP_STACK_SIZE: usize = 256 << 20;

// ---- _execute ----
