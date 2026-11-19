// Painless engine: a compiled tier for per-document scripts (score scripts over doc values).
//
// `compile` turns a checked script into a tree of nodes with local variables in slots,
// `params` members as constants, static and context calls resolved, and `doc['f']`
// accesses read from columns the caller provides (one value list per document, the same
// values the `doc` lookup gives). Every operation runs through the interpreter's own
// functions (binary_op, unary, explicit_cast, store, call_static, get_field, ...), so a
// result is the interpreter's result. Scripts outside the subset do not compile; a run
// that fails returns None, and the caller runs the interpreter on that document, which
// produces the exact error.

use std::collections::HashMap;
use std::rc::Rc;

use crate::painless::api::Script;
use crate::painless::check::{static_field, Info};
use crate::painless::interp::{explicit_cast, store, unary, zero, Interp, NativeFn};
use crate::painless::lib::{call_static, get_field, index_get};
use crate::painless::parse::{Expr, Program, Stmt, EK, SK};
use crate::painless::value::*;

/// One doc value of a column.
#[derive(Clone, Copy)]
pub enum CV {
    L(i64),
    D(f64),
    G(f64, f64),
    B(bool),
}

impl CV {
    fn pv(self) -> PV {
        match self {
            CV::L(v) => PV::Long(v),
            CV::D(v) => PV::Double(v),
            CV::G(lat, lon) => PV::GeoPoint(lat, lon),
            CV::B(b) => PV::Bool(b),
        }
    }
}

/// The doc values of the current document, by column.
pub trait Cols {
    fn vals(&self, col: usize) -> &[CV];
}

/// Columns a script reads: `field(name)` gives the column and the doc values kind
/// (Longs, Doubles, GeoPoints, Booleans) of a field, or None when the field cannot be
/// served from a column.
pub type FieldCols<'a> = dyn FnMut(&str) -> Option<(usize, &'static str)> + 'a;

enum N {
    Const(PV),
    Slot(usize),
    Score,
    /// The doc values object of a column (allocated per use).
    DocVals(usize, &'static str),
    /// `.value` / `getValue()` / `get(0)`-style first value.
    DocFirst(usize),
    /// `.lat` / `.lon` of a GeoPoints column.
    DocGeo(usize, bool),
    /// `size()` / `length` (Int) or `isEmpty()` / `empty` (Bool).
    DocLen(usize, bool),
    /// `get(i)` (i64_of of the index).
    DocGet(usize, Box<N>),
    /// `[i]` (list index rules).
    DocIndex(usize, Box<N>),
    Field(Box<N>, String, usize),
    Index(Box<N>, Box<N>, usize),
    Unary(String, Box<N>, usize),
    Binary(String, Box<N>, Box<N>, String, String, usize),
    And(Box<N>, Box<N>),
    Or(Box<N>, Box<N>),
    Cast(String, Box<N>, usize),
    Cond(Box<N>, Box<N>, Box<N>, String),
    Elvis(Box<N>, Box<N>),
    Static { class: String, name: String, args: Vec<N>, casts: Vec<String>, pos: usize, arg_pos: Option<usize> },
    Native { f: NativeFn, args: Vec<N>, casts: Vec<String>, pos: usize },
    Assign(usize, String, Box<N>, usize),
}

enum S {
    Block(Vec<S>),
    Decl(Vec<(usize, String, Option<(N, usize)>)>),
    /// The expression and whether it is an assignment (no statement value).
    Expr(N, bool),
    If(N, Box<S>, Option<Box<S>>),
    Return(N),
}

/// A compiled script.
pub struct Fast {
    body: Vec<S>,
    slots: usize,
    ret: &'static str,
}

struct Compiler<'a, 'b> {
    prog: &'a Program,
    info: &'a Info,
    funcs: &'a HashMap<String, NativeFn>,
    params: &'a PV,
    cols: &'a mut FieldCols<'b>,
    scopes: Vec<HashMap<String, (usize, String)>>,
    slots: usize,
}

/// Compiles a score script; `params` are the script's params (constant for a request).
pub fn compile(script: &Script, funcs: &HashMap<String, NativeFn>, params: &PV, cols: &mut FieldCols<'_>) -> Option<Fast> {
    let (prog, info, ret) = script.parts();
    if !prog.funcs.is_empty() || ret != "double" {
        return None;
    }
    let mut c = Compiler { prog, info, funcs, params, cols, scopes: vec![HashMap::new()], slots: 0 };
    let body = c.stmts(&prog.body)?;
    Some(Fast { body, slots: c.slots, ret })
}

fn is_list_type(t: &str) -> bool {
    matches!(t, "def" | "Object" | "List" | "ArrayList" | "Collection" | "Iterable")
}

impl Compiler<'_, '_> {
    fn stmts(&mut self, ss: &[Stmt]) -> Option<Vec<S>> {
        ss.iter().map(|s| self.stmt(s)).collect()
    }

    fn scoped(&mut self, s: &Stmt) -> Option<S> {
        self.scopes.push(HashMap::new());
        let r = self.stmt(s);
        self.scopes.pop();
        r
    }

    fn stmt(&mut self, s: &Stmt) -> Option<S> {
        Some(match &s.kind {
            SK::Block(b) => {
                self.scopes.push(HashMap::new());
                let r = self.stmts(b);
                self.scopes.pop();
                S::Block(r?)
            }
            SK::Decl { typ, names, inits } => {
                let mut out = Vec::new();
                for (i, name) in names.iter().enumerate() {
                    let init = match &inits[i] {
                        Some(x) => Some((self.expr(x)?, x.pos)),
                        None => None,
                    };
                    let slot = self.slots;
                    self.slots += 1;
                    self.scopes.last_mut()?.insert(name.clone(), (slot, typ.clone()));
                    out.push((slot, typ.clone(), init));
                }
                S::Decl(out)
            }
            SK::Expr(x) => S::Expr(self.expr(x)?, matches!(x.kind, EK::Assign { .. })),
            SK::If { cond, then, els } => {
                let c = self.expr(cond)?;
                let t = self.scoped(then)?;
                let e = match els {
                    Some(e) => Some(Box::new(self.scoped(e)?)),
                    None => None,
                };
                S::If(c, Box::new(t), e)
            }
            SK::Return(Some(x)) => S::Return(self.expr(x)?),
            _ => return None,
        })
    }

    fn local(&self, name: &str) -> Option<(usize, String)> {
        self.scopes.iter().rev().find_map(|s| s.get(name).cloned())
    }

    fn is_ctx_var(&self, x: &Expr, name: &str) -> bool {
        matches!(&x.kind, EK::Var(n) if n == name && self.local(name).is_none())
    }

    fn const_str(&self, x: &Expr) -> Option<String> {
        match self.info.consts.get(&x.id) {
            Some(PV::Str(s)) => return Some(s.to_string()),
            Some(_) => return None,
            None => {}
        }
        match &x.kind {
            EK::Lit(PV::Str(s), _) => Some(s.to_string()),
            _ => None,
        }
    }

    /// The column of a doc values expression: `doc['f']`, `doc.f`, or one of them cast to
    /// a list type (an identity for doc values).
    fn doc_col(&mut self, x: &Expr) -> Option<(usize, &'static str)> {
        match &x.kind {
            EK::Index { obj, idx } if self.is_ctx_var(obj, "doc") => {
                let f = self.const_str(idx)?;
                (self.cols)(&f)
            }
            EK::Field { obj, name, .. } if self.is_ctx_var(obj, "doc") => (self.cols)(name),
            EK::Cast { typ, x: inner } if is_list_type(typ) => self.doc_col(inner),
            _ => None,
        }
    }

    fn casts(&self, x: &Expr) -> Vec<String> {
        self.info.sigs.get(&x.id).map(|s| s.params.clone()).unwrap_or_default()
    }

    fn args(&mut self, args: &[Expr]) -> Option<Vec<N>> {
        args.iter().map(|a| self.expr(a)).collect()
    }

    fn expr(&mut self, x: &Expr) -> Option<N> {
        if matches!(x.kind, EK::Lit(..) | EK::Field { .. } | EK::Binary { .. } | EK::Unary { .. } | EK::Cast { .. }) {
            if let Some(cv) = self.info.consts.get(&x.id) {
                return Some(N::Const(cv.clone()));
            }
        }
        Some(match &x.kind {
            EK::Lit(v, _) => N::Const(v.clone()),
            EK::Var(name) => match self.local(name) {
                Some((slot, _)) => N::Slot(slot),
                None if name == "_score" => N::Score,
                None => return None,
            },
            EK::Field { obj, name, null_safe } => {
                if let EK::Type(class) = &obj.kind {
                    return static_field(class, name).map(|(v, _)| N::Const(v));
                }
                if self.is_ctx_var(obj, "params") {
                    return Some(N::Const(get_member(self.params, name)));
                }
                if let Some((col, kind)) = self.doc_col(x) {
                    return Some(N::DocVals(col, kind));
                }
                if let Some((col, kind)) = self.doc_col(obj) {
                    return Some(match name.as_str() {
                        "value" => N::DocFirst(col),
                        "length" => N::DocLen(col, false),
                        "empty" => N::DocLen(col, true),
                        "lat" | "lon" if kind == "GeoPoints" => N::DocGeo(col, name == "lat"),
                        _ => return None,
                    });
                }
                if *null_safe {
                    return None;
                }
                N::Field(Box::new(self.expr(obj)?), name.clone(), x.pos)
            }
            EK::Index { obj, idx } => {
                if self.is_ctx_var(obj, "params") {
                    let k = self.expr(idx)?;
                    return match k {
                        N::Const(k) => Some(N::Const(index_get(self.params, &k, x.pos).ok()?)),
                        _ => None,
                    };
                }
                if let Some((col, kind)) = self.doc_col(x) {
                    return Some(N::DocVals(col, kind));
                }
                if let Some((col, _)) = self.doc_col(obj) {
                    return Some(N::DocIndex(col, Box::new(self.expr(idx)?)));
                }
                N::Index(Box::new(self.expr(obj)?), Box::new(self.expr(idx)?), x.pos)
            }
            EK::Call { obj, name, args, .. } => {
                let casts = self.casts(x);
                match obj {
                    None => {
                        if self.prog.funcs.contains_key(name) || name == "randomScore" {
                            return None;
                        }
                        let f = self.funcs.get(name)?.clone();
                        N::Native { f, args: self.args(args)?, casts, pos: x.pos }
                    }
                    Some(o) => {
                        if let EK::Type(class) = &o.kind {
                            return Some(N::Static {
                                class: class.clone(),
                                name: name.clone(),
                                args: self.args(args)?,
                                casts,
                                pos: x.pos,
                                arg_pos: args.first().map(|e| e.pos),
                            });
                        }
                        let (col, kind) = self.doc_col(o)?;
                        match (name.as_str(), args.len()) {
                            ("getValue", 0) => N::DocFirst(col),
                            ("get", 1) => {
                                let mut a = self.args(args)?;
                                let i = a.pop()?;
                                // the whitelist parameter conversion of the argument
                                let i = match casts.first() {
                                    Some(p) if is_primitive(p) => N::Cast(p.clone(), Box::new(i), x.pos),
                                    _ => i,
                                };
                                N::DocGet(col, Box::new(i))
                            }
                            ("size", 0) | ("getLength", 0) => N::DocLen(col, false),
                            ("isEmpty", 0) => N::DocLen(col, true),
                            ("getLat", 0) | ("getLon", 0) if kind == "GeoPoints" => N::DocGeo(col, name == "getLat"),
                            _ => return None,
                        }
                    }
                }
            }
            EK::Unary { op, x: inner } => N::Unary(op.clone(), Box::new(self.expr(inner)?), x.pos),
            EK::Binary { op, l, r } => {
                let (ln, rn) = (Box::new(self.expr(l)?), Box::new(self.expr(r)?));
                match op.as_str() {
                    "&&" => N::And(ln, rn),
                    "||" => N::Or(ln, rn),
                    "=~" | "==~" => return None,
                    _ => N::Binary(op.clone(), ln, rn, self.info.typ(l).to_string(), self.info.typ(r).to_string(), x.pos),
                }
            }
            EK::Cast { typ, x: inner } => {
                if let Some((col, kind)) = self.doc_col(x) {
                    return Some(N::DocVals(col, kind));
                }
                N::Cast(typ.clone(), Box::new(self.expr(inner)?), inner.pos)
            }
            EK::Cond { c, a, b } => N::Cond(
                Box::new(self.expr(c)?),
                Box::new(self.expr(a)?),
                Box::new(self.expr(b)?),
                self.info.typ(x).to_string(),
            ),
            EK::Elvis { a, b } => N::Elvis(Box::new(self.expr(a)?), Box::new(self.expr(b)?)),
            EK::Assign { target, op, val } if op == "=" => {
                let EK::Var(name) = &target.kind else { return None };
                let (slot, typ) = self.local(name)?;
                N::Assign(slot, typ, Box::new(self.expr(val)?), val.pos)
            }
            _ => return None,
        })
    }
}

/// `params.name`: a member of the params map (null when absent).
fn get_member(params: &PV, name: &str) -> PV {
    match params {
        PV::Map(m) => m.borrow().get(&PV::str(name)).unwrap_or(PV::Null),
        _ => PV::Null,
    }
}

type F<T> = Result<T, ()>;

fn ok<T>(r: R<T>) -> F<T> {
    r.map_err(|_| ())
}

enum Ctl {
    Normal,
    Return,
}

struct Run<'a, 'i> {
    inp: &'a Interp<'i>,
    cols: &'a dyn Cols,
    score: f64,
    slots: &'a mut Vec<PV>,
}

impl Fast {
    /// The script's result for the current document (`cols`), as the interpreter's
    /// return conversion gives it; None: run the interpreter instead.
    pub fn run(&self, script: &Script, funcs: &HashMap<String, NativeFn>, cols: &dyn Cols, score: f64, slots: &mut Vec<PV>) -> Option<f64> {
        let (prog, info, _) = script.parts();
        let inp = Interp::new(prog, info, funcs);
        slots.clear();
        slots.resize(self.slots, PV::Null);
        let mut r = Run { inp: &inp, cols, score, slots };
        let (_, v) = r.block(&self.body, true).ok()?;
        match self.ret {
            "double" if is_number(&v) => Some(f64_of(&v)),
            _ => None,
        }
    }
}

impl Run<'_, '_> {
    fn block(&mut self, ss: &[S], last: bool) -> F<(Ctl, PV)> {
        let mut last_val = PV::Null;
        let n = ss.len();
        for (i, s) in ss.iter().enumerate() {
            let is_last = last && i + 1 == n;
            let (ctl, v) = self.stmt(s, is_last)?;
            if let Ctl::Return = ctl {
                return Ok((ctl, v));
            }
            if is_last {
                last_val = v;
            }
        }
        Ok((Ctl::Normal, last_val))
    }

    fn stmt(&mut self, s: &S, last: bool) -> F<(Ctl, PV)> {
        match s {
            S::Block(b) => self.block(b, last),
            S::Decl(ds) => {
                for (slot, typ, init) in ds {
                    let v = match init {
                        Some((x, pos)) => {
                            let v = self.eval(x)?;
                            ok(store(v, typ, *pos))?
                        }
                        None => zero(typ),
                    };
                    self.slots[*slot] = v;
                }
                Ok((Ctl::Normal, PV::Null))
            }
            S::Expr(x, assign) => {
                let v = self.eval(x)?;
                if last && !assign {
                    return Ok((Ctl::Normal, v));
                }
                Ok((Ctl::Normal, PV::Null))
            }
            S::If(c, t, e) => {
                if self.eval_bool(c)? {
                    return self.stmt(t, last);
                }
                if let Some(e) = e {
                    return self.stmt(e, last);
                }
                Ok((Ctl::Normal, PV::Null))
            }
            S::Return(x) => Ok((Ctl::Return, self.eval(x)?)),
        }
    }

    fn eval_bool(&mut self, x: &N) -> F<bool> {
        match self.eval(x)? {
            PV::Bool(b) => Ok(b),
            _ => Err(()),
        }
    }

    fn first(&self, col: usize) -> F<CV> {
        self.cols.vals(col).first().copied().ok_or(())
    }

    fn eval_args(&mut self, args: &[N], casts: &[String]) -> F<Vec<PV>> {
        let mut a = Vec::with_capacity(args.len());
        for x in args {
            a.push(self.eval(x)?);
        }
        for (i, p) in casts.iter().enumerate() {
            if i < a.len() && is_primitive(p) && is_number(&a[i]) {
                a[i] = cast_num(&a[i], p);
            }
        }
        Ok(a)
    }

    fn eval(&mut self, x: &N) -> F<PV> {
        Ok(match x {
            N::Const(v) => v.clone(),
            N::Slot(i) => self.slots[*i].clone(),
            N::Score => PV::Double(self.score),
            N::DocVals(col, kind) => PV::DocValues(Rc::new(PDocValues {
                kind: kind.to_string(),
                vals: self.cols.vals(*col).iter().map(|v| v.pv()).collect(),
            })),
            N::DocFirst(col) => self.first(*col)?.pv(),
            N::DocGeo(col, lat) => match self.first(*col)? {
                CV::G(a, o) => PV::Double(if *lat { a } else { o }),
                _ => return Err(()),
            },
            N::DocLen(col, empty) => {
                let n = self.cols.vals(*col).len();
                if *empty {
                    PV::Bool(n == 0)
                } else {
                    PV::Int(n as i32)
                }
            }
            N::DocGet(col, i) => {
                let i = i64_of(&self.eval(i)?);
                let vals = self.cols.vals(*col);
                if i < 0 || i as usize >= vals.len() {
                    return Err(());
                }
                vals[i as usize].pv()
            }
            N::DocIndex(col, i) => {
                let i = self.eval(i)?;
                if !is_number(&i) {
                    return Err(());
                }
                let vals = self.cols.vals(*col);
                let mut i = i64_of(&i);
                if i < 0 {
                    i += vals.len() as i64;
                }
                if i < 0 || i as usize >= vals.len() {
                    return Err(());
                }
                vals[i as usize].pv()
            }
            N::Field(o, name, pos) => {
                let o = self.eval(o)?;
                if o.is_null() {
                    return Err(());
                }
                ok(get_field(self.inp, &o, name, *pos))?
            }
            N::Index(o, i, pos) => {
                let o = self.eval(o)?;
                let i = self.eval(i)?;
                ok(index_get(&o, &i, *pos))?
            }
            N::Unary(op, v, pos) => {
                let v = self.eval(v)?;
                ok(unary(op, v, *pos))?
            }
            N::Binary(op, l, r, tl, tr, pos) => {
                let l = self.eval(l)?;
                let r = self.eval(r)?;
                ok(self.inp.binary_op(op, l, r, tl, tr, *pos))?
            }
            N::And(l, r) => PV::Bool(self.eval_bool(l)? && self.eval_bool(r)?),
            N::Or(l, r) => PV::Bool(self.eval_bool(l)? || self.eval_bool(r)?),
            N::Cast(typ, v, pos) => {
                let v = self.eval(v)?;
                ok(explicit_cast(v, typ, *pos))?
            }
            N::Cond(c, a, b, t) => {
                let v = if self.eval_bool(c)? { self.eval(a)? } else { self.eval(b)? };
                if is_numeric_type(t) && is_number(&v) {
                    cast_num(&v, t)
                } else {
                    v
                }
            }
            N::Elvis(a, b) => {
                let v = self.eval(a)?;
                if !v.is_null() {
                    v
                } else {
                    self.eval(b)?
                }
            }
            N::Static { class, name, args, casts, pos, arg_pos } => {
                let a = self.eval_args(args, casts)?;
                ok(call_static(self.inp, class, name, a, *pos, *arg_pos))?
            }
            N::Native { f, args, casts, pos } => {
                let a = self.eval_args(args, casts)?;
                ok(f(self.inp, a, *pos))?
            }
            N::Assign(slot, typ, v, pos) => {
                let v = self.eval(v)?;
                let sv = ok(store(v, typ, *pos))?;
                self.slots[*slot] = sv.clone();
                sv
            }
        })
    }
}
