// Painless engine: tree-walking interpreter.

use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;

use crate::pl_check::{static_field, Info};
use crate::pl_lex::is_type_name;
use crate::pl_lib::{get_field, index_get, index_set, iter_items, new_object, set_field, static_enum};
use crate::pl_parse::{Expr, FuncDef, LBody, Lambda, Program, Stmt, EK, SK};
use crate::pl_regex::compile_pattern;
use crate::pl_value::*;

pub struct Var {
    pub v: PV,
    pub typ: String,
}

pub struct Scope {
    vars: RefCell<HashMap<String, Rc<RefCell<Var>>>>,
    parent: Option<Env>,
}

pub type Env = Rc<Scope>;

pub fn new_env() -> Env {
    Rc::new(Scope { vars: RefCell::new(HashMap::new()), parent: None })
}

fn child(e: &Env) -> Env {
    Rc::new(Scope { vars: RefCell::new(HashMap::new()), parent: Some(e.clone()) })
}

pub fn declare(e: &Env, name: &str, v: PV, typ: &str) {
    e.vars
        .borrow_mut()
        .insert(name.to_string(), Rc::new(RefCell::new(Var { v, typ: typ.to_string() })));
}

fn lookup(e: &Env, name: &str) -> Option<Rc<RefCell<Var>>> {
    let mut s = Some(e);
    while let Some(sc) = s {
        if let Some(v) = sc.vars.borrow().get(name) {
            return Some(v.clone());
        }
        s = sc.parent.as_ref();
    }
    None
}

/// A native function: called with the interpreter, the arguments and the
/// script offset of the call.
pub type NativeFn = Rc<dyn Fn(&Interp<'_>, Vec<PV>, usize) -> R<PV>>;

/// A callable: a lambda (with its captured scope) or a native function.
pub enum PFn {
    Lambda { lam: Rc<Lambda>, env: Env },
    Native(NativeFn),
}

pub fn native(f: impl Fn(&Interp<'_>, Vec<PV>, usize) -> R<PV> + 'static) -> PV {
    PV::Fn(Rc::new(PFn::Native(Rc::new(f))))
}

#[derive(PartialEq, Eq, Clone, Copy)]
enum Ctl {
    Normal,
    Break,
    Continue,
    Return,
}

const MAX_LOOP: u64 = 1_000_000;

/// Nesting limit of function and lambda calls; deeper recursion fails like
/// the JVM's StackOverflowError.
const MAX_CALL_DEPTH: usize = 1000;

pub struct Interp<'a> {
    pub prog: &'a Program,
    pub info: &'a Info,
    pub funcs: &'a HashMap<String, NativeFn>,
    loops: Cell<u64>,
    depth: Cell<usize>,
}

type SR = R<(Ctl, PV)>;

fn normal() -> SR {
    Ok((Ctl::Normal, PV::Null))
}

impl<'a> Interp<'a> {
    pub fn new(prog: &'a Program, info: &'a Info, funcs: &'a HashMap<String, NativeFn>) -> Self {
        Interp { prog, info, funcs, loops: Cell::new(0), depth: Cell::new(0) }
    }

    fn loop_tick(&self, off: usize) -> R<()> {
        let n = self.loops.get() + 1;
        self.loops.set(n);
        if n > MAX_LOOP {
            let mut e = RtErr::new(
                "painless_error",
                "The maximum number of statements that can be executed in a loop has been reached.",
                off,
            );
            e.is_error = true;
            return Err(e);
        }
        Ok(())
    }

    /// Executes the program body, returning the script's value.
    pub fn run(&self, env: &Env) -> R<PV> {
        let (_, v) = self.block(&self.prog.body, env, true)?;
        Ok(v)
    }

    fn block(&self, stmts: &[Stmt], env: &Env, last: bool) -> SR {
        let mut last_val = PV::Null;
        let n = stmts.len();
        for (i, s) in stmts.iter().enumerate() {
            let is_last = last && i + 1 == n;
            let (ctl, v) = self.stmt(s, env, is_last)?;
            if ctl != Ctl::Normal {
                return Ok((ctl, v));
            }
            if is_last {
                last_val = v;
            }
        }
        Ok((Ctl::Normal, last_val))
    }

    fn stmt(&self, s: &Stmt, env: &Env, last: bool) -> SR {
        match &s.kind {
            SK::Block(stmts) => self.block(stmts, &child(env), last),
            SK::Decl { typ, names, inits } => {
                for (i, name) in names.iter().enumerate() {
                    let v = match &inits[i] {
                        Some(init) => {
                            let x = self.eval(init, env)?;
                            self.store(x, typ, init.pos)?
                        }
                        None => zero(typ),
                    };
                    declare(env, name, v, typ);
                }
                normal()
            }
            SK::Expr(x) => {
                let v = self.eval(x, env)?;
                if last && !matches!(x.kind, EK::Assign { .. }) {
                    return Ok((Ctl::Normal, v));
                }
                normal()
            }
            SK::If { cond, then, els } => {
                if self.eval_bool(cond, env)? {
                    return self.stmt(then, &child(env), last);
                }
                if let Some(e) = els {
                    return self.stmt(e, &child(env), last);
                }
                normal()
            }
            SK::While { cond, body } => loop {
                if !self.eval_bool(cond, env)? {
                    return normal();
                }
                self.loop_tick(s.pos)?;
                let (ctl, v) = self.stmt(body, &child(env), false)?;
                match ctl {
                    Ctl::Break => return normal(),
                    Ctl::Return => return Ok((ctl, v)),
                    _ => {}
                }
            },
            SK::Do { body, cond } => loop {
                self.loop_tick(s.pos)?;
                let (ctl, v) = self.stmt(body, &child(env), false)?;
                match ctl {
                    Ctl::Break => return normal(),
                    Ctl::Return => return Ok((ctl, v)),
                    _ => {}
                }
                if !self.eval_bool(cond, env)? {
                    return normal();
                }
            },
            SK::For { init, cond, update, body } => {
                let fenv = child(env);
                if let Some(i) = init {
                    self.stmt(i, &fenv, false)?;
                }
                loop {
                    if let Some(c) = cond {
                        if !self.eval_bool(c, &fenv)? {
                            return normal();
                        }
                    }
                    self.loop_tick(s.pos)?;
                    let (ctl, v) = self.stmt(body, &child(&fenv), false)?;
                    match ctl {
                        Ctl::Break => return normal(),
                        Ctl::Return => return Ok((ctl, v)),
                        _ => {}
                    }
                    for u in update {
                        self.eval(u, &fenv)?;
                    }
                }
            }
            SK::ForEach { typ, name, iter, body } => {
                let it = self.eval(iter, env)?;
                let items = iter_items(&it, iter.pos)?;
                for item in items {
                    self.loop_tick(s.pos)?;
                    let benv = child(env);
                    let v = self.store(item, typ, s.pos)?;
                    declare(&benv, name, v, typ);
                    let (ctl, rv) = self.stmt(body, &benv, false)?;
                    match ctl {
                        Ctl::Break => break,
                        Ctl::Return => return Ok((ctl, rv)),
                        _ => {}
                    }
                }
                normal()
            }
            SK::Break => Ok((Ctl::Break, PV::Null)),
            SK::Continue => Ok((Ctl::Continue, PV::Null)),
            SK::Return(x) => match x {
                None => Ok((Ctl::Return, PV::Null)),
                Some(x) => Ok((Ctl::Return, self.eval(x, env)?)),
            },
            SK::Throw(x) => {
                let v = self.eval(x, env)?;
                match v {
                    PV::Exception(ex) => {
                        let mut e = RtErr::new(&ex.typ, "", x.pos);
                        e.reason = ex.msg.clone();
                        e.thrown = Some(ex.clone());
                        Err(e)
                    }
                    PV::Null => Err(RtErr::nil("null_pointer_exception", x.pos)),
                    other => Err(cce(x.pos, &format!("Cannot cast {} to java.lang.Throwable", class_name(&other)))),
                }
            }
            SK::Try { body, catches } => {
                let err = match self.stmt(body, env, last) {
                    Ok(r) => return Ok(r),
                    Err(e) => e,
                };
                if err.is_error {
                    return Err(err);
                }
                for ct in catches {
                    if catch_matches(&ct.typ, &err.typ) {
                        let cenv = child(env);
                        let ex = match &err.thrown {
                            Some(ex) => ex.clone(),
                            None => Rc::new(PException {
                                typ: err.typ.clone(),
                                msg: err.reason.clone(),
                                class: exception_class(&err.typ),
                            }),
                        };
                        declare(&cenv, &ct.name, PV::Exception(ex), &ct.typ);
                        return self.stmt(&ct.body, &cenv, last);
                    }
                }
                Err(err)
            }
        }
    }

    /// Converts a value for storage into a variable of the given static type.
    pub fn store(&self, v: PV, typ: &str, off: usize) -> R<PV> {
        store(v, typ, off)
    }

    fn eval_bool(&self, x: &Expr, env: &Env) -> R<bool> {
        match self.eval(x, env)? {
            PV::Bool(b) => Ok(b),
            PV::Null => Err(npe(x.pos)),
            v => Err(cce(x.pos, &format!("Cannot cast {} to java.lang.Boolean", class_name(&v)))),
        }
    }

    pub fn eval(&self, x: &Expr, env: &Env) -> R<PV> {
        if matches!(x.kind, EK::Lit(..) | EK::Field { .. } | EK::Binary { .. } | EK::Unary { .. } | EK::Cast { .. }) {
            if let Some(cv) = self.info.consts.get(&x.id) {
                return Ok(cv.clone());
            }
        }
        match &x.kind {
            EK::Lit(v, _) => Ok(v.clone()),
            EK::Regex(p, f) => compile_pattern(p, f, x.pos),
            EK::Var(name) => match lookup(env, name) {
                Some(v) => Ok(v.borrow().v.clone()),
                None => Err(iae(x.pos, &format!("cannot resolve symbol [{}]", name))),
            },
            EK::Type(name) => Ok(PV::StaticRef(Rc::from(name.as_str()))),
            EK::Field { obj, name, null_safe } => {
                if let EK::Type(class) = &obj.kind {
                    if let Some((v, _)) = static_field(class, name) {
                        return Ok(v);
                    }
                    return static_enum(class, name, x.pos);
                }
                let o = self.eval(obj, env)?;
                if o.is_null() {
                    if *null_safe {
                        return Ok(PV::Null);
                    }
                    return Err(npe(x.pos));
                }
                get_field(self, &o, name, x.pos)
            }
            EK::Call { .. } => self.eval_call(x, env),
            EK::Index { obj, idx } => {
                let o = self.eval(obj, env)?;
                let i = self.eval(idx, env)?;
                index_get(&o, &i, x.pos)
            }
            EK::New { typ, args } => {
                let a = self.eval_args(args, env)?;
                new_object(typ, a, x.pos)
            }
            EK::NewArray { elem, dims, init, rank, has_init } => {
                if *has_init {
                    let mut out = Vec::new();
                    for e in init {
                        let v = self.eval(e, env)?;
                        out.push(self.store(v, elem, e.pos)?);
                    }
                    return Ok(PV::array(elem, out));
                }
                let mut ds = Vec::new();
                let mut total: u64 = 1;
                for d in dims {
                    let n = i64_of(&self.eval(d, env)?);
                    if n < 0 {
                        return Err(RtErr::new("negative_array_size_exception", &n.to_string(), x.pos));
                    }
                    total = total.saturating_mul(n as u64);
                    check_alloc(total, x.pos)?;
                    ds.push(n as usize);
                }
                Ok(make_array(elem, *rank, &ds))
            }
            EK::List(elems) => {
                let mut out = Vec::with_capacity(elems.len());
                for e in elems {
                    out.push(self.eval(e, env)?);
                }
                Ok(PV::list(out))
            }
            EK::Map(pairs) => {
                let mut m = PMap::new(MapKind::Hash);
                for (k, v) in pairs {
                    let kv = self.eval(k, env)?;
                    let vv = self.eval(v, env)?;
                    m.put(kv, vv);
                }
                Ok(PV::map(m))
            }
            EK::Unary { op, x: inner } => {
                let v = self.eval(inner, env)?;
                unary(op, v, x.pos)
            }
            EK::Binary { op, l, r } => self.eval_binary(op, l, r, x.pos, env),
            EK::Instanceof { x: inner, typ } => {
                let v = self.eval(inner, env)?;
                Ok(PV::Bool(instance_of(&v, typ)))
            }
            EK::Cast { typ, x: inner } => {
                let v = self.eval(inner, env)?;
                explicit_cast(v, typ, inner.pos)
            }
            EK::Cond { c, a, b } => {
                let v = if self.eval_bool(c, env)? { self.eval(a, env)? } else { self.eval(b, env)? };
                let t = self.info.typ(x);
                if is_numeric_type(t) && is_number(&v) {
                    return Ok(cast_num(&v, t));
                }
                Ok(v)
            }
            EK::Elvis { a, b } => {
                let v = self.eval(a, env)?;
                if !v.is_null() {
                    return Ok(v);
                }
                self.eval(b, env)
            }
            EK::Assign { target, op, val } => self.eval_assign(target, op, val, x.pos, env),
            EK::IncDec { target, op, prefix } => self.eval_incdec(target, op, *prefix, x.pos, env),
            EK::Lambda(lam) => Ok(PV::Fn(Rc::new(PFn::Lambda { lam: lam.clone(), env: env.clone() }))),
            EK::FuncRef { typ, method } => self.func_ref(typ, method, x.pos, env),
        }
    }

    fn eval_args(&self, args: &[Expr], env: &Env) -> R<Vec<PV>> {
        let mut out = Vec::with_capacity(args.len());
        for a in args {
            out.push(self.eval(a, env)?);
        }
        Ok(out)
    }

    fn eval_call(&self, x: &Expr, env: &Env) -> R<PV> {
        let (obj, name, args, null_safe) = match &x.kind {
            EK::Call { obj, name, args, null_safe } => (obj, name, args, *null_safe),
            _ => unreachable!(),
        };
        let mut a = self.eval_args(args, env)?;
        if let Some(sig) = self.info.sigs.get(&x.id) {
            for (i, p) in sig.params.iter().enumerate() {
                if i < a.len() && is_primitive(p) && is_number(&a[i]) {
                    a[i] = cast_num(&a[i], p);
                }
            }
        }
        let obj = match obj {
            None => {
                if let Some(f) = self.prog.funcs.get(name) {
                    return self.call_user_func(f, a, x.pos);
                }
                if let Some(f) = self.funcs.get(name) {
                    return f(self, a, x.pos);
                }
                return Err(iae(x.pos, &format!("Unknown call [{}] with [{}] arguments.", name, a.len())));
            }
            Some(o) => o,
        };
        if let EK::Type(class) = &obj.kind {
            let arg_pos = args.first().map(|e| e.pos);
            return crate::pl_lib::call_static(self, class, name, a, x.pos, arg_pos);
        }
        let recv = self.eval(obj, env)?;
        if recv.is_null() {
            if null_safe {
                return Ok(PV::Null);
            }
            return Err(npe(x.pos));
        }
        crate::pl_lib::call_method(self, &recv, name, a, x.pos)
    }

    /// Runs f one call level deeper, failing past the call depth limit.
    fn nested<T>(&self, off: usize, f: impl FnOnce() -> R<T>) -> R<T> {
        let d = self.depth.get();
        if d >= MAX_CALL_DEPTH {
            let mut e = RtErr::nil("stack_overflow_error", off);
            e.is_error = true;
            return Err(e);
        }
        self.depth.set(d + 1);
        let r = f();
        self.depth.set(d);
        r
    }

    pub fn call_user_func(&self, f: &FuncDef, args: Vec<PV>, off: usize) -> R<PV> {
        self.nested(off, || self.call_user_func_inner(f, args, off))
    }

    fn call_user_func_inner(&self, f: &FuncDef, args: Vec<PV>, off: usize) -> R<PV> {
        let env = new_env();
        for (i, (name, v)) in f.pnames.iter().zip(args).enumerate() {
            let v = self.store(v, &f.ptypes[i], off)?;
            declare(&env, name, v, &f.ptypes[i]);
        }
        let (_, v) = self.block(&f.body, &env, false)?;
        self.store(v, &f.ret, off)
    }

    /// Invokes a lambda, function reference or native function.
    pub fn call_fn(&self, fv: &PV, args: Vec<PV>, off: usize) -> R<PV> {
        let f = match fv {
            PV::Fn(f) => f.clone(),
            PV::Null => return Err(npe(off)),
            other => {
                return Err(cce(off, &format!("Cannot cast {} to a functional interface", class_name(other))))
            }
        };
        match &*f {
            PFn::Native(n) => self.nested(off, || n(self, args, off)),
            PFn::Lambda { lam, env } => self.nested(off, || {
                if args.len() != lam.pnames.len() {
                    return Err(iae(off, "Incorrect number of parameters for [lambda] in [java.util.function.Function]"));
                }
                let e = child(env);
                for (i, (name, v)) in lam.pnames.iter().zip(args).enumerate() {
                    let v = self.store(v, &lam.ptypes[i], off)?;
                    declare(&e, name, v, &lam.ptypes[i]);
                }
                match &lam.body {
                    LBody::Block(stmts) => Ok(self.block(stmts, &e, true)?.1),
                    LBody::Expr(x) => self.eval(x, &e),
                }
            }),
        }
    }

    fn func_ref(&self, typ: &str, method: &str, off: usize, env: &Env) -> R<PV> {
        if typ == "this" {
            let f = match self.prog.funcs.get(method) {
                Some(f) => f.clone(),
                None => return Err(iae(off, &format!("Unknown reference [this::{}]", method))),
            };
            return Ok(native(move |inp, args, o| inp.call_user_func(&f, args, o)));
        }
        let method = method.to_string();
        if !is_type_name(typ) {
            // variable::method binds the receiver
            let recv = match lookup(env, typ) {
                Some(v) => v.borrow().v.clone(),
                None => return Err(iae(off, &format!("cannot resolve symbol [{}]", typ))),
            };
            return Ok(native(move |inp, args, o| crate::pl_lib::call_method(inp, &recv, &method, args, o)));
        }
        let typ = typ.to_string();
        if method == "new" {
            return Ok(native(move |_, args, o| new_object(&typ, args, o)));
        }
        Ok(native(move |inp, mut args, o| {
            let key = format!("{}/{}", method, args.len());
            if crate::pl_check::static_methods().get(typ.as_str()).map(|m| m.contains_key(&key)).unwrap_or(false) {
                return crate::pl_lib::call_static(inp, &typ, &method, args, o, None);
            }
            if args.is_empty() {
                return Err(iae(o, &format!("Unknown reference [{}::{}]", typ, method)));
            }
            let recv = args.remove(0);
            crate::pl_lib::call_method(inp, &recv, &method, args, o)
        }))
    }

    fn eval_binary(&self, op: &str, l: &Expr, r: &Expr, off: usize, env: &Env) -> R<PV> {
        match op {
            "&&" => {
                if !self.eval_bool(l, env)? {
                    return Ok(PV::Bool(false));
                }
                return Ok(PV::Bool(self.eval_bool(r, env)?));
            }
            "||" => {
                if self.eval_bool(l, env)? {
                    return Ok(PV::Bool(true));
                }
                return Ok(PV::Bool(self.eval_bool(r, env)?));
            }
            _ => {}
        }
        let lv = self.eval(l, env)?;
        let rv = self.eval(r, env)?;
        self.binary_op(op, lv, rv, self.info.typ(l), self.info.typ(r), off)
    }

    pub fn binary_op(&self, op: &str, l: PV, r: PV, tl: &str, tr: &str, off: usize) -> R<PV> {
        match op {
            "==" => return Ok(PV::Bool(equals(&l, &r))),
            "!=" => return Ok(PV::Bool(!equals(&l, &r))),
            "===" | "!==" => {
                let same = if is_primitive(tl) && is_primitive(tr) { equals(&l, &r) } else { identical(&l, &r) };
                return Ok(PV::Bool(if op == "===" { same } else { !same }));
            }
            "=~" | "==~" => {
                let p = match &r {
                    PV::Pattern(p) => p.clone(),
                    other => {
                        return Err(cce(off, &format!("Cannot cast {} to java.util.regex.Pattern", class_name(other))))
                    }
                };
                let s = match &l {
                    PV::Str(s) => s.to_string(),
                    PV::Null => return Err(npe(off)),
                    other => jstr(other),
                };
                let mut m = crate::pl_regex::PMatcher::new(p, &s);
                let ok = if op == "=~" { m.find(off)? } else { m.matches(off)? };
                return Ok(PV::Bool(ok));
            }
            "+" => {
                if matches!(l, PV::Str(_)) || matches!(r, PV::Str(_)) || tl == "String" || tr == "String" {
                    return Ok(PV::str(&format!("{}{}", jstr(&l), jstr(&r))));
                }
            }
            "<" | "<=" | ">" | ">=" => {
                if !is_number(&l) || !is_number(&r) {
                    if l.is_null() || r.is_null() {
                        return Err(npe(off));
                    }
                    return Err(cce(
                        off,
                        &format!("Cannot apply [{}] operation to types [{}] and [{}].", op, class_name(&l), class_name(&r)),
                    ));
                }
                let p = promote(&l, &r);
                if (p == "double" || p == "float") && (f64_of(&l).is_nan() || f64_of(&r).is_nan()) {
                    return Ok(PV::Bool(false));
                }
                let c = compare(&l, &r).unwrap_or(0);
                return Ok(PV::Bool(match op {
                    "<" => c < 0,
                    "<=" => c <= 0,
                    ">" => c > 0,
                    _ => c >= 0,
                }));
            }
            "&" | "|" | "^" => {
                if let (PV::Bool(a), PV::Bool(b)) = (&l, &r) {
                    return Ok(PV::Bool(match op {
                        "&" => *a && *b,
                        "|" => *a || *b,
                        _ => a != b,
                    }));
                }
            }
            _ => {}
        }
        if l.is_null() || r.is_null() {
            return Err(npe(off));
        }
        if !is_number(&l) || !is_number(&r) {
            let (bad, good_t) = if is_number(&l) { (&r, tl) } else { (&l, tr) };
            if !good_t.is_empty() && good_t != "def" && is_primitive(good_t) {
                let mut e = cce(
                    off,
                    &format!("Cannot cast {} to java.lang.{}", class_name(bad), box_of(promote_static(good_t))),
                );
                e.frames = vec!["java.base/java.lang.Class.cast(Class.java:3492)".into()];
                return Err(e);
            }
            return Err(cce(
                off,
                &format!("Cannot apply [{}] operation to types [{}] and [{}].", op, class_name(&l), class_name(&r)),
            ));
        }
        arith(op, &l, &r).map_err(|mut e| {
            e.offset = off;
            e
        })
    }

    fn eval_assign(&self, target: &Expr, op: &str, val: &Expr, off: usize, env: &Env) -> R<PV> {
        let tt = self.info.typ(target).to_string();
        if op == "=" {
            let v = self.eval(val, env)?;
            return self.assign_to(target, v, env, val.pos);
        }
        let lv = self.lvalue(target, env)?;
        let old = self.lv_get(&lv)?;
        let rv = self.eval(val, env)?;
        let res = self.binary_op(op, old.clone(), rv, &tt, self.info.typ(val), off)?;
        let res = cast_back(res, &old, &tt);
        self.lv_set(&lv, res.clone())?;
        Ok(res)
    }

    fn assign_to(&self, target: &Expr, v: PV, env: &Env, off: usize) -> R<PV> {
        match &target.kind {
            EK::Var(name) => {
                let slot = match lookup(env, name) {
                    Some(s) => s,
                    None => return Err(iae(target.pos, &format!("cannot resolve symbol [{}]", name))),
                };
                let typ = slot.borrow().typ.clone();
                let sv = self.store(v, &typ, off)?;
                slot.borrow_mut().v = sv.clone();
                Ok(sv)
            }
            EK::Field { obj, name, .. } => {
                let o = self.eval(obj, env)?;
                if o.is_null() {
                    return Err(npe(target.pos));
                }
                set_field(&o, name, v.clone(), target.pos)?;
                Ok(v)
            }
            EK::Index { obj, idx } => {
                let o = self.eval(obj, env)?;
                let i = self.eval(idx, env)?;
                let mut v = v;
                if let PV::Array(a) = &o {
                    v = self.store(v, &a.elem, off)?;
                }
                index_set(&o, &i, v.clone(), target.pos)?;
                Ok(v)
            }
            _ => Err(iae(target.pos, "invalid assignment target")),
        }
    }

    fn lvalue(&self, target: &Expr, env: &Env) -> R<LV> {
        match &target.kind {
            EK::Var(name) => match lookup(env, name) {
                Some(s) => Ok(LV::Var(s, target.pos)),
                None => Err(iae(target.pos, &format!("cannot resolve symbol [{}]", name))),
            },
            EK::Field { obj, name, .. } => {
                let o = self.eval(obj, env)?;
                if o.is_null() {
                    return Err(npe(target.pos));
                }
                Ok(LV::Field(o, name.clone(), target.pos))
            }
            EK::Index { obj, idx } => {
                let o = self.eval(obj, env)?;
                let i = self.eval(idx, env)?;
                Ok(LV::Index(o, i, target.pos))
            }
            _ => Err(iae(target.pos, "invalid assignment target")),
        }
    }

    fn lv_get(&self, lv: &LV) -> R<PV> {
        match lv {
            LV::Var(s, _) => Ok(s.borrow().v.clone()),
            LV::Field(o, name, pos) => get_field(self, o, name, *pos),
            LV::Index(o, i, pos) => index_get(o, i, *pos),
        }
    }

    fn lv_set(&self, lv: &LV, v: PV) -> R<()> {
        match lv {
            LV::Var(s, pos) => {
                let typ = s.borrow().typ.clone();
                let sv = self.store(v, &typ, *pos)?;
                s.borrow_mut().v = sv;
                Ok(())
            }
            LV::Field(o, name, pos) => set_field(o, name, v, *pos),
            LV::Index(o, i, pos) => {
                let mut v = v;
                if let PV::Array(a) = o {
                    if is_number(&v) && is_numeric_type(&a.elem) {
                        v = cast_num(&v, &a.elem);
                    }
                }
                index_set(o, i, v, *pos)
            }
        }
    }

    fn eval_incdec(&self, target: &Expr, op: &str, prefix: bool, off: usize, env: &Env) -> R<PV> {
        let lv = self.lvalue(target, env)?;
        let old = self.lv_get(&lv)?;
        if old.is_null() {
            return Err(npe(off));
        }
        let bop = if op == "--" { "-" } else { "+" };
        let tt = self.info.typ(target);
        let res = self.binary_op(bop, old.clone(), PV::Int(1), tt, "int", off)?;
        let res = cast_back(res, &old, tt);
        self.lv_set(&lv, res.clone())?;
        Ok(if prefix { res } else { old })
    }
}

enum LV {
    Var(Rc<RefCell<Var>>, usize),
    Field(PV, String, usize),
    Index(PV, PV, usize),
}

pub fn store(v: PV, typ: &str, off: usize) -> R<PV> {
    match typ {
        "" | "def" | "Object" => Ok(v),
        "int" | "long" | "float" | "double" | "short" | "byte" | "char" => {
            if v.is_null() {
                return Err(npe(off));
            }
            if is_number(&v) {
                return Ok(cast_num(&v, typ));
            }
            Err(cce(off, &format!("Cannot cast {} to java.lang.{}", class_name(&v), box_of(typ))))
        }
        "boolean" => match v {
            PV::Bool(_) => Ok(v),
            PV::Null => Err(npe(off)),
            other => Err(cce(off, &format!("Cannot cast {} to java.lang.Boolean", class_name(&other)))),
        },
        "String" => match v {
            PV::Null | PV::Str(_) => Ok(v),
            other => Err(cce(off, &format!("Cannot cast {} to java.lang.String", class_name(&other)))),
        },
        _ => Ok(v),
    }
}

pub fn zero(typ: &str) -> PV {
    match typ {
        "int" => PV::Int(0),
        "long" => PV::Long(0),
        "float" => PV::Float(0.0),
        "double" => PV::Double(0.0),
        "short" => PV::Short(0),
        "byte" => PV::Byte(0),
        "char" => PV::Char(0),
        "boolean" => PV::Bool(false),
        _ => PV::Null,
    }
}

fn catch_matches(catch_type: &str, err_type: &str) -> bool {
    match catch_type {
        "Exception" | "RuntimeException" => true,
        _ => exception_type(catch_type) == err_type,
    }
}

/// IllegalArgumentException -> illegal_argument_exception
pub fn exception_type(class: &str) -> String {
    let mut out = String::new();
    for (i, c) in class.chars().enumerate() {
        if c.is_ascii_uppercase() {
            if i > 0 {
                out.push('_');
            }
            out.push(c.to_ascii_lowercase());
        } else {
            out.push(c);
        }
    }
    out
}

/// illegal_argument_exception -> IllegalArgumentException
fn exception_class(typ: &str) -> String {
    typ.split('_')
        .map(|w| {
            let mut c = w.chars();
            match c.next() {
                Some(f) => f.to_ascii_uppercase().to_string() + c.as_str(),
                None => String::new(),
            }
        })
        .collect()
}

fn promote_static(t: &str) -> &str {
    match t {
        "byte" | "short" | "char" => "int",
        _ => t,
    }
}

/// Casts a compound-assignment result to the target's type: the static type,
/// or for def targets the runtime type of the old value.
fn cast_back(res: PV, old: &PV, tt: &str) -> PV {
    if is_numeric_type(tt) && is_number(&res) {
        return cast_num(&res, tt);
    }
    if (tt == "def" || tt.is_empty() || tt == "Object") && is_number(old) && is_number(&res) {
        return cast_num(&res, prim_name(old));
    }
    res
}

pub fn unary(op: &str, v: PV, off: usize) -> R<PV> {
    match op {
        "!" => match v {
            PV::Bool(b) => Ok(PV::Bool(!b)),
            other => Err(cce(off, &format!("Cannot cast {} to java.lang.Boolean", class_name(&other)))),
        },
        _ => {
            if v.is_null() {
                return Err(npe(off));
            }
            if !is_number(&v) {
                return Err(cce(off, &format!("Cannot apply [{}] operation to type [{}].", op, class_name(&v))));
            }
            let v = cast_num(&v, promote(&v, &PV::Int(0)));
            match op {
                "-" => Ok(match v {
                    PV::Int(i) => PV::Int(i.wrapping_neg()),
                    PV::Long(i) => PV::Long(i.wrapping_neg()),
                    PV::Float(f) => PV::Float(-f),
                    PV::Double(f) => PV::Double(-f),
                    o => o,
                }),
                "~" => match v {
                    PV::Int(i) => Ok(PV::Int(!i)),
                    PV::Long(i) => Ok(PV::Long(!i)),
                    o => Err(cce(off, &format!("Cannot apply [~] operation to type [{}].", class_name(&o)))),
                },
                _ => Ok(v),
            }
        }
    }
}

fn java_fmod(x: f64, y: f64) -> f64 {
    x % y
}

/// Applies a numeric binary operator with Java promotion rules.
pub fn arith(op: &str, a: &PV, b: &PV) -> R<PV> {
    let div0 = || RtErr::new("arithmetic_exception", "/ by zero", 0);
    match op {
        "<<" | ">>" | ">>>" => {
            let n = i64_of(b);
            if promote(a, &PV::Int(0)) == "long" {
                let x = i64_of(a);
                let s = (n & 63) as u32;
                return Ok(PV::Long(match op {
                    "<<" => x.wrapping_shl(s),
                    ">>" => x >> s,
                    _ => ((x as u64) >> s) as i64,
                }));
            }
            let x = i64_of(a) as i32;
            let s = (n & 31) as u32;
            return Ok(PV::Int(match op {
                "<<" => x.wrapping_shl(s),
                ">>" => x >> s,
                _ => ((x as u32) >> s) as i32,
            }));
        }
        _ => {}
    }
    let t = promote(a, b);
    match t {
        "float" => {
            let (x, y) = (f64_of(a) as f32, f64_of(b) as f32);
            Ok(PV::Float(match op {
                "+" => x + y,
                "-" => x - y,
                "*" => x * y,
                "/" => x / y,
                "%" => x % y,
                _ => {
                    return Err(cce(
                        0,
                        &format!("Cannot apply [{}] operation to types [java.lang.Float] and [java.lang.Float].", op),
                    ))
                }
            }))
        }
        "double" => {
            let (x, y) = (f64_of(a), f64_of(b));
            Ok(PV::Double(match op {
                "+" => x + y,
                "-" => x - y,
                "*" => x * y,
                "/" => x / y,
                "%" => java_fmod(x, y),
                _ => {
                    return Err(cce(
                        0,
                        &format!("Cannot apply [{}] operation to types [java.lang.Double] and [java.lang.Double].", op),
                    ))
                }
            }))
        }
        "long" => {
            let (x, y) = (i64_of(a), i64_of(b));
            Ok(PV::Long(match op {
                "+" => x.wrapping_add(y),
                "-" => x.wrapping_sub(y),
                "*" => x.wrapping_mul(y),
                "/" => {
                    if y == 0 {
                        return Err(div0());
                    }
                    x.wrapping_div(y)
                }
                "%" => {
                    if y == 0 {
                        return Err(div0());
                    }
                    x.wrapping_rem(y)
                }
                "&" => x & y,
                "|" => x | y,
                "^" => x ^ y,
                _ => return Err(cce(0, &format!("Cannot apply [{}] operation.", op))),
            }))
        }
        _ => {
            let (x, y) = (i64_of(a) as i32, i64_of(b) as i32);
            Ok(PV::Int(match op {
                "+" => x.wrapping_add(y),
                "-" => x.wrapping_sub(y),
                "*" => x.wrapping_mul(y),
                "/" => {
                    if y == 0 {
                        return Err(div0());
                    }
                    x.wrapping_div(y)
                }
                "%" => {
                    if y == 0 {
                        return Err(div0());
                    }
                    x.wrapping_rem(y)
                }
                "&" => x & y,
                "|" => x | y,
                "^" => x ^ y,
                _ => return Err(cce(0, &format!("Cannot apply [{}] operation.", op))),
            }))
        }
    }
}

pub fn explicit_cast(v: PV, typ: &str, off: usize) -> R<PV> {
    match typ {
        "int" | "long" | "float" | "double" | "short" | "byte" | "char" => {
            if typ == "char" {
                if let PV::Str(s) = &v {
                    let u: Vec<u16> = s.encode_utf16().collect();
                    if u.len() == 1 {
                        return Ok(PV::Char(u[0]));
                    }
                }
            }
            if v.is_null() {
                return Err(npe(off));
            }
            if !is_number(&v) {
                return Err(cce(off, &format!("Cannot cast {} to {}", class_name(&v), typ)));
            }
            return Ok(cast_num(&v, typ));
        }
        "boolean" | "Boolean" => {
            if matches!(v, PV::Bool(_) | PV::Null) {
                return Ok(v);
            }
        }
        "def" | "Object" => return Ok(v),
        "String" => {
            if matches!(v, PV::Str(_) | PV::Null) {
                return Ok(v);
            }
        }
        "Integer" | "Long" | "Double" | "Float" | "Short" | "Byte" | "Character" => {
            if v.is_null() {
                return Ok(v);
            }
            if is_number(&v) {
                return Ok(cast_num(&v, unbox(typ)));
            }
        }
        _ => {
            if v.is_null() || instance_of(&v, typ) {
                return Ok(v);
            }
        }
    }
    Err(cce(off, &format!("Cannot cast {} to {}", class_name(&v), canon(typ))))
}

pub fn instance_of(v: &PV, typ: &str) -> bool {
    if typ == "def" {
        return true;
    }
    if v.is_null() {
        return false;
    }
    match typ {
        "Object" => true,
        "Integer" => matches!(v, PV::Int(_)),
        "Long" => matches!(v, PV::Long(_)),
        "Double" => matches!(v, PV::Double(_)),
        "Float" => matches!(v, PV::Float(_)),
        "Short" => matches!(v, PV::Short(_)),
        "Byte" => matches!(v, PV::Byte(_)),
        "Character" => matches!(v, PV::Char(_)),
        "Boolean" | "boolean" => matches!(v, PV::Bool(_)),
        "Number" => is_number(v) && !matches!(v, PV::Char(_)),
        "String" | "CharSequence" => matches!(v, PV::Str(_)),
        "int" | "long" | "double" | "float" | "short" | "byte" | "char" => prim_name(v) == typ,
        "List" | "ArrayList" | "Collection" | "Iterable" => match v {
            PV::List(_) | PV::DocValues(_) => true,
            PV::Set(_) | PV::EntrySet(_) => typ == "Collection" || typ == "Iterable",
            _ => false,
        },
        "Map" | "HashMap" => matches!(v, PV::Map(m) if typ == "Map" || m.borrow().kind != MapKind::Tree),
        "TreeMap" => matches!(v, PV::Map(m) if m.borrow().kind == MapKind::Tree),
        "LinkedHashMap" => matches!(v, PV::Map(m) if m.borrow().kind == MapKind::Linked),
        "Set" | "HashSet" => matches!(v, PV::Set(_)),
        "StringBuilder" => matches!(v, PV::SB(_)),
        "Pattern" => matches!(v, PV::Pattern(_)),
        "Matcher" => matches!(v, PV::Matcher(_)),
        "ZonedDateTime" => matches!(v, PV::Zdt(_)),
        "LocalDate" => matches!(v, PV::LocalDate(_)),
        "Exception" | "RuntimeException" => matches!(v, PV::Exception(_)),
        _ => {
            if let Some(elem) = typ.strip_suffix("[]") {
                return matches!(v, PV::Array(a) if a.elem == elem);
            }
            if let PV::Exception(e) = v {
                return exception_type(typ) == e.typ;
            }
            false
        }
    }
}

/// Allocation limit of arrays and generated collections: bigger requests
/// fail like a JVM out of heap instead of exhausting the server's memory.
pub const MAX_ALLOC: u64 = 50_000_000;

pub fn check_alloc(n: u64, off: usize) -> R<()> {
    if n > MAX_ALLOC {
        let mut e = RtErr::new("out_of_memory_error", "Java heap space", off);
        e.is_error = true;
        return Err(e);
    }
    Ok(())
}

pub fn make_array(elem: &str, rank: usize, dims: &[usize]) -> PV {
    let sub = format!("{}{}", elem, "[]".repeat(rank.saturating_sub(1)));
    if dims.is_empty() {
        return PV::array(&sub, vec![]);
    }
    let mut a = Vec::with_capacity(dims[0]);
    for _ in 0..dims[0] {
        if dims.len() > 1 {
            a.push(make_array(elem, rank - 1, &dims[1..]));
        } else if rank == 1 {
            a.push(zero(elem));
        } else {
            a.push(PV::Null);
        }
    }
    PV::array(&sub, a)
}
