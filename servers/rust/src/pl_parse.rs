// Painless engine: AST and recursive-descent parser.

use std::collections::HashMap;
use std::rc::Rc;

use crate::pl_lex::{lex, CompileErr, Tok, TokKind};
use crate::pl_value::{parse_number, PV};

pub struct Expr {
    /// Unique per program: the key of the checker's type information.
    pub id: usize,
    pub pos: usize,
    pub kind: EK,
}

pub enum EK {
    Lit(PV, &'static str),
    Regex(String, String),
    Var(String),
    /// A type used as a value: static access (Math.max).
    Type(String),
    Field { obj: Box<Expr>, name: String, null_safe: bool },
    /// obj is None for local function calls.
    Call { obj: Option<Box<Expr>>, name: String, args: Vec<Expr>, null_safe: bool },
    Index { obj: Box<Expr>, idx: Box<Expr> },
    New { typ: String, args: Vec<Expr> },
    NewArray { elem: String, dims: Vec<Expr>, init: Vec<Expr>, rank: usize, has_init: bool },
    List(Vec<Expr>),
    Map(Vec<(Expr, Expr)>),
    Unary { op: String, x: Box<Expr> },
    Binary { op: String, l: Box<Expr>, r: Box<Expr> },
    Instanceof { x: Box<Expr>, typ: String },
    Cast { typ: String, x: Box<Expr> },
    Cond { c: Box<Expr>, a: Box<Expr>, b: Box<Expr> },
    Elvis { a: Box<Expr>, b: Box<Expr> },
    /// op is "=" or the binary operator of a compound assignment.
    Assign { target: Box<Expr>, op: String, val: Box<Expr> },
    IncDec { target: Box<Expr>, op: String, prefix: bool },
    Lambda(Rc<Lambda>),
    FuncRef { typ: String, method: String },
}

pub struct Lambda {
    pub ptypes: Vec<String>,
    pub pnames: Vec<String>,
    pub body: LBody,
}

pub enum LBody {
    Expr(Expr),
    Block(Vec<Stmt>),
}

pub struct Stmt {
    pub pos: usize,
    pub kind: SK,
}

pub enum SK {
    Block(Vec<Stmt>),
    Decl { typ: String, names: Vec<String>, inits: Vec<Option<Expr>> },
    Expr(Expr),
    If { cond: Expr, then: Box<Stmt>, els: Option<Box<Stmt>> },
    While { cond: Expr, body: Box<Stmt> },
    Do { body: Box<Stmt>, cond: Expr },
    For { init: Option<Box<Stmt>>, cond: Option<Expr>, update: Vec<Expr>, body: Box<Stmt> },
    ForEach { typ: String, name: String, iter: Expr, body: Box<Stmt> },
    Break,
    Continue,
    Return(Option<Expr>),
    Throw(Expr),
    Try { body: Box<Stmt>, catches: Vec<Catch> },
}

pub struct Catch {
    pub typ: String,
    pub name: String,
    pub body: Box<Stmt>,
}

pub struct FuncDef {
    pub ret: String,
    pub name: String,
    pub ptypes: Vec<String>,
    pub pnames: Vec<String>,
    pub body: Vec<Stmt>,
}

pub struct Program {
    pub funcs: HashMap<String, Rc<FuncDef>>,
    pub body: Vec<Stmt>,
    /// Start offsets of every statement, for runtime error spans.
    pub stmts: Vec<usize>,
}

struct Parser<'a> {
    src: &'a str,
    toks: Vec<Tok>,
    i: usize,
    stmts: Vec<usize>,
    next_id: usize,
}

type PR<T> = Result<T, CompileErr>;

pub fn parse(src: &str) -> PR<Program> {
    let toks = lex(src)?;
    let mut p = Parser { src, toks, i: 0, stmts: Vec::new(), next_id: 0 };
    let mut funcs = HashMap::new();
    while p.is_func_start() {
        let f = p.parse_func()?;
        funcs.insert(f.name.clone(), Rc::new(f));
    }
    let mut body = Vec::new();
    while p.peek().kind != TokKind::Eof {
        body.push(p.parse_statement(true)?);
    }
    Ok(Program { funcs, body, stmts: p.stmts })
}

const ASSIGN_OPS: &[(&str, &str)] = &[
    ("=", "="),
    ("+=", "+"),
    ("-=", "-"),
    ("*=", "*"),
    ("/=", "/"),
    ("%=", "%"),
    ("&=", "&"),
    ("|=", "|"),
    ("^=", "^"),
    ("<<=", "<<"),
    (">>=", ">>"),
    (">>>=", ">>>"),
];

/// Binary precedence levels, lowest first.
const BIN_LEVELS: &[&[&str]] = &[
    &["?:"],
    &["||"],
    &["&&"],
    &["|"],
    &["^"],
    &["&"],
    &["==", "!=", "===", "!=="],
    &["<", "<=", ">", ">=", "instanceof"],
    &["<<", ">>", ">>>"],
    &["=~", "==~"],
    &["+", "-"],
    &["*", "/", "%"],
];

impl<'a> Parser<'a> {
    fn mk(&mut self, pos: usize, kind: EK) -> Expr {
        self.next_id += 1;
        Expr { id: self.next_id, pos, kind }
    }
    fn peek(&self) -> &Tok {
        &self.toks[self.i]
    }
    fn peek_at(&self, k: usize) -> &Tok {
        let j = (self.i + k).min(self.toks.len() - 1);
        &self.toks[j]
    }
    fn next(&mut self) -> Tok {
        let t = self.toks[self.i].clone();
        if self.i < self.toks.len() - 1 {
            self.i += 1;
        }
        t
    }
    fn is_op(&self, s: &str) -> bool {
        let t = self.peek();
        t.kind == TokKind::Op && t.text == s
    }
    fn is_op_at(&self, k: usize, s: &str) -> bool {
        let t = self.peek_at(k);
        t.kind == TokKind::Op && t.text == s
    }
    fn accept_op(&mut self, s: &str) -> bool {
        if self.is_op(s) {
            self.next();
            return true;
        }
        false
    }
    fn is_ident(&self, s: &str) -> bool {
        let t = self.peek();
        t.kind == TokKind::Ident && t.text == s
    }
    fn tok_text(&self, t: &Tok) -> String {
        if t.kind == TokKind::Eof {
            return "<EOF>".into();
        }
        self.src[t.pos..t.end].to_string()
    }
    fn err_unexpected(&self, expecting: &str) -> CompileErr {
        let t = self.peek();
        if t.kind == TokKind::Eof {
            return CompileErr::iae("unexpected end of script.", t.pos).with_nil_cause("no_viable_alt_exception");
        }
        let mut reason = format!("unexpected token ['{}']", self.tok_text(t));
        if !expecting.is_empty() {
            reason.push_str(&format!(" was expecting one of [{}].", expecting));
        } else {
            reason.push('.');
        }
        CompileErr::iae(&reason, t.pos)
    }
    fn expect_op(&mut self, s: &str) -> PR<()> {
        if self.accept_op(s) {
            return Ok(());
        }
        if self.peek().kind == TokKind::Eof {
            return Err(self.err_unexpected(""));
        }
        Err(self.err_unexpected(&format!("'{}'", s)))
    }
    fn skip_dims(&self, mut k: usize) -> usize {
        while self.is_op_at(k, "[") && self.is_op_at(k + 1, "]") {
            k += 2;
        }
        k
    }
    /// TYPE ID '(' at top level.
    fn is_func_start(&self) -> bool {
        if self.peek().kind != TokKind::Type {
            return false;
        }
        let k = self.skip_dims(1);
        self.peek_at(k).kind == TokKind::Ident && self.is_op_at(k + 1, "(")
    }
    fn parse_type_name(&mut self) -> String {
        let mut name = self.next().text;
        while self.is_op("[") && self.is_op_at(1, "]") {
            self.next();
            self.next();
            name.push_str("[]");
        }
        name
    }
    fn parse_func(&mut self) -> PR<FuncDef> {
        let ret = self.parse_type_name();
        let name = self.next().text;
        self.next(); // (
        let mut f = FuncDef { ret, name, ptypes: vec![], pnames: vec![], body: vec![] };
        if !self.is_op(")") {
            loop {
                if self.peek().kind != TokKind::Type {
                    return Err(self.err_unexpected(""));
                }
                let pt = self.parse_type_name();
                if self.peek().kind != TokKind::Ident {
                    return Err(self.err_unexpected(""));
                }
                f.ptypes.push(pt);
                f.pnames.push(self.next().text);
                if !self.accept_op(",") {
                    break;
                }
            }
        }
        self.expect_op(")")?;
        if !self.is_op("{") {
            return Err(self.err_unexpected("'{'"));
        }
        let b = self.parse_block()?;
        if let SK::Block(stmts) = b.kind {
            f.body = stmts;
        }
        Ok(f)
    }
    fn parse_block(&mut self) -> PR<Stmt> {
        let pos = self.next().pos; // {
        let mut stmts = Vec::new();
        while !self.is_op("}") {
            if self.peek().kind == TokKind::Eof {
                return Err(self.err_unexpected(""));
            }
            stmts.push(self.parse_statement(false)?);
        }
        self.next();
        Ok(Stmt { pos, kind: SK::Block(stmts) })
    }
    /// Consumes the terminator of a simple statement: ';', or nothing before
    /// '}' / EOF.
    fn end_statement(&mut self, top: bool) -> PR<()> {
        if self.accept_op(";") {
            return Ok(());
        }
        let t = self.peek();
        if t.kind == TokKind::Eof {
            return Ok(());
        }
        if !top && t.kind == TokKind::Op && t.text == "}" {
            return Ok(());
        }
        if top {
            return Err(self.err_unexpected("{<EOF>, ';'}"));
        }
        Err(self.err_unexpected("{';', '}'}"))
    }
    /// The body of if/while/for: a block or one statement.
    fn parse_trailer(&mut self) -> PR<Stmt> {
        if self.is_op("{") {
            return self.parse_block();
        }
        self.parse_statement(false)
    }
    fn is_decl_start(&self) -> bool {
        if self.peek().kind != TokKind::Type {
            return false;
        }
        let k = self.skip_dims(1);
        self.peek_at(k).kind == TokKind::Ident
    }
    fn paren_expr(&mut self) -> PR<Expr> {
        self.expect_op("(")?;
        let c = self.parse_expr()?;
        self.expect_op(")")?;
        Ok(c)
    }
    fn parse_statement(&mut self, top: bool) -> PR<Stmt> {
        let t = self.peek().clone();
        self.stmts.push(t.pos);
        let pos = t.pos;
        if t.kind == TokKind::Ident {
            match t.text.as_str() {
                "if" => {
                    self.next();
                    let cond = self.paren_expr()?;
                    let then = Box::new(self.parse_trailer()?);
                    let mut els = None;
                    if self.is_ident("else") {
                        self.next();
                        els = Some(Box::new(self.parse_trailer()?));
                    }
                    return Ok(Stmt { pos, kind: SK::If { cond, then, els } });
                }
                "while" => {
                    self.next();
                    let cond = self.paren_expr()?;
                    if self.accept_op(";") {
                        let body = Box::new(Stmt { pos, kind: SK::Block(vec![]) });
                        return Ok(Stmt { pos, kind: SK::While { cond, body } });
                    }
                    let body = Box::new(self.parse_trailer()?);
                    return Ok(Stmt { pos, kind: SK::While { cond, body } });
                }
                "do" => {
                    self.next();
                    if !self.is_op("{") {
                        return Err(self.err_unexpected("'{'"));
                    }
                    let body = Box::new(self.parse_block()?);
                    if !self.is_ident("while") {
                        return Err(self.err_unexpected("'while'"));
                    }
                    self.next();
                    let cond = self.paren_expr()?;
                    self.end_statement(top)?;
                    return Ok(Stmt { pos, kind: SK::Do { body, cond } });
                }
                "for" => return self.parse_for(pos),
                "break" => {
                    self.next();
                    self.end_statement(top)?;
                    return Ok(Stmt { pos, kind: SK::Break });
                }
                "continue" => {
                    self.next();
                    self.end_statement(top)?;
                    return Ok(Stmt { pos, kind: SK::Continue });
                }
                "return" => {
                    self.next();
                    let mut x = None;
                    if !self.is_op(";") && !self.is_op("}") && self.peek().kind != TokKind::Eof {
                        x = Some(self.parse_expr()?);
                    }
                    self.end_statement(top)?;
                    return Ok(Stmt { pos, kind: SK::Return(x) });
                }
                "throw" => {
                    self.next();
                    let x = self.parse_expr()?;
                    self.end_statement(top)?;
                    return Ok(Stmt { pos, kind: SK::Throw(x) });
                }
                "try" => {
                    self.next();
                    if !self.is_op("{") {
                        return Err(self.err_unexpected("'{'"));
                    }
                    let body = Box::new(self.parse_block()?);
                    let mut catches = Vec::new();
                    while self.is_ident("catch") {
                        self.next();
                        self.expect_op("(")?;
                        let typ = self.parse_type_name();
                        let name = self.next().text;
                        self.expect_op(")")?;
                        if !self.is_op("{") {
                            return Err(self.err_unexpected("'{'"));
                        }
                        let body = Box::new(self.parse_block()?);
                        catches.push(Catch { typ, name, body });
                    }
                    return Ok(Stmt { pos, kind: SK::Try { body, catches } });
                }
                _ => {}
            }
        }
        if t.kind == TokKind::Op && t.text == "{" {
            return self.parse_block();
        }
        if t.kind == TokKind::Op && t.text == ";" {
            self.next();
            return Ok(Stmt { pos, kind: SK::Block(vec![]) });
        }
        if self.is_decl_start() {
            let d = self.parse_decl()?;
            self.end_statement(top)?;
            return Ok(d);
        }
        let x = self.parse_expr()?;
        self.end_statement(top)?;
        Ok(Stmt { pos, kind: SK::Expr(x) })
    }
    fn parse_decl(&mut self) -> PR<Stmt> {
        let pos = self.peek().pos;
        let typ = self.parse_type_name();
        let mut names = Vec::new();
        let mut inits = Vec::new();
        loop {
            if self.peek().kind != TokKind::Ident {
                return Err(self.err_unexpected(""));
            }
            names.push(self.next().text);
            if self.accept_op("=") {
                inits.push(Some(self.parse_expr()?));
            } else {
                inits.push(None);
            }
            if !self.accept_op(",") {
                break;
            }
        }
        Ok(Stmt { pos, kind: SK::Decl { typ, names, inits } })
    }
    fn parse_for(&mut self, pos: usize) -> PR<Stmt> {
        self.next();
        self.expect_op("(")?;
        // for (x in expr) / for (def x : expr) / for (Type x : expr)
        if self.peek().kind == TokKind::Ident && self.peek_at(1).kind == TokKind::Ident && self.peek_at(1).text == "in" {
            let name = self.next().text;
            self.next();
            let iter = self.parse_expr()?;
            self.expect_op(")")?;
            let body = Box::new(self.parse_trailer()?);
            return Ok(Stmt { pos, kind: SK::ForEach { typ: "def".into(), name, iter, body } });
        }
        if self.is_decl_start() {
            let save = self.i;
            let typ = self.parse_type_name();
            if self.peek().kind == TokKind::Ident && self.is_op_at(1, ":") {
                let name = self.next().text;
                self.next();
                let iter = self.parse_expr()?;
                self.expect_op(")")?;
                let body = Box::new(self.parse_trailer()?);
                return Ok(Stmt { pos, kind: SK::ForEach { typ, name, iter, body } });
            }
            self.i = save;
        }
        let mut init = None;
        if !self.is_op(";") {
            if self.is_decl_start() {
                init = Some(Box::new(self.parse_decl()?));
            } else {
                let x = self.parse_expr()?;
                init = Some(Box::new(Stmt { pos: x.pos, kind: SK::Expr(x) }));
            }
        }
        self.expect_op(";")?;
        let mut cond = None;
        if !self.is_op(";") {
            cond = Some(self.parse_expr()?);
        }
        self.expect_op(";")?;
        let mut update = Vec::new();
        if !self.is_op(")") {
            loop {
                update.push(self.parse_expr()?);
                if !self.accept_op(",") {
                    break;
                }
            }
        }
        self.expect_op(")")?;
        let body = if self.accept_op(";") {
            Box::new(Stmt { pos, kind: SK::Block(vec![]) })
        } else {
            Box::new(self.parse_trailer()?)
        };
        Ok(Stmt { pos, kind: SK::For { init, cond, update, body } })
    }

    // ---- expressions ----

    fn parse_expr(&mut self) -> PR<Expr> {
        let pos = self.peek().pos;
        let lhs = self.parse_binary(0)?;
        let t = self.peek().clone();
        if t.kind == TokKind::Op {
            if let Some((_, op)) = ASSIGN_OPS.iter().find(|(o, _)| *o == t.text) {
                self.next();
                let rhs = self.parse_expr()?;
                return Ok(self.mk(pos, EK::Assign { target: Box::new(lhs), op: op.to_string(), val: Box::new(rhs) }));
            }
            if t.text == "?" {
                self.next();
                let a = self.parse_expr()?;
                self.expect_op(":")?;
                let b = self.parse_expr()?;
                return Ok(self.mk(pos, EK::Cond { c: Box::new(lhs), a: Box::new(a), b: Box::new(b) }));
            }
        }
        Ok(lhs)
    }

    fn parse_binary(&mut self, level: usize) -> PR<Expr> {
        if level >= BIN_LEVELS.len() {
            return self.parse_unary();
        }
        let pos = self.peek().pos;
        if BIN_LEVELS[level][0] == "?:" {
            let lhs = self.parse_binary(level + 1)?;
            if self.is_op("?:") {
                self.next();
                let rhs = self.parse_binary(level)?; // right assoc
                return Ok(self.mk(pos, EK::Elvis { a: Box::new(lhs), b: Box::new(rhs) }));
            }
            return Ok(lhs);
        }
        let mut lhs = self.parse_binary(level + 1)?;
        loop {
            let t = self.peek().clone();
            let op = if t.kind == TokKind::Op {
                BIN_LEVELS[level].iter().find(|o| **o == t.text).copied()
            } else if t.kind == TokKind::Ident && t.text == "instanceof" {
                BIN_LEVELS[level].iter().find(|o| **o == "instanceof").copied()
            } else {
                None
            };
            let op = match op {
                Some(o) => o,
                None => return Ok(lhs),
            };
            self.next();
            if op == "instanceof" {
                if self.peek().kind != TokKind::Type {
                    if self.peek().kind == TokKind::Ident {
                        let nt = self.peek();
                        return Err(CompileErr::iae(&format!("Not a type [{}].", nt.text), nt.pos));
                    }
                    return Err(self.err_unexpected(""));
                }
                let typ = self.parse_type_name();
                lhs = self.mk(pos, EK::Instanceof { x: Box::new(lhs), typ });
                continue;
            }
            let rhs = self.parse_binary(level + 1)?;
            lhs = self.mk(pos, EK::Binary { op: op.to_string(), l: Box::new(lhs), r: Box::new(rhs) });
        }
    }

    /// Whether "( TYPE [[]]* )" starts at the current token.
    fn is_cast_ahead(&self) -> bool {
        if !self.is_op("(") || self.peek_at(1).kind != TokKind::Type {
            return false;
        }
        let k = self.skip_dims(2);
        if !self.is_op_at(k, ")") {
            return false;
        }
        // "(Type) ..." followed by '.' is a parenthesised static access, not a cast
        let nt = self.peek_at(k + 1);
        if nt.kind == TokKind::Op && matches!(nt.text.as_str(), "." | ")" | ";" | "," | "?") {
            return false;
        }
        nt.kind != TokKind::Eof
    }

    fn parse_unary(&mut self) -> PR<Expr> {
        let t = self.peek().clone();
        if t.kind == TokKind::Op {
            match t.text.as_str() {
                "++" | "--" => {
                    self.next();
                    let x = self.parse_unary()?;
                    return Ok(self.mk(t.pos, EK::IncDec { target: Box::new(x), op: t.text.clone(), prefix: true }));
                }
                "!" | "~" | "-" | "+" => {
                    self.next();
                    // fold negative numeric literals so that -2147483648 is legal
                    if t.text == "-" && matches!(self.peek().kind, TokKind::Int | TokKind::Float) {
                        let after_dot = self.is_op_at(1, ".") || self.is_op_at(1, "[");
                        if !after_dot {
                            let nt = self.next();
                            return self.num_lit(&nt, true, t.pos);
                        }
                    }
                    let x = self.parse_unary()?;
                    return Ok(self.mk(t.pos, EK::Unary { op: t.text.clone(), x: Box::new(x) }));
                }
                "(" => {
                    if self.is_cast_ahead() {
                        self.next();
                        let typ = self.parse_type_name();
                        self.next(); // )
                        let x = self.parse_unary()?;
                        return Ok(self.mk(t.pos, EK::Cast { typ, x: Box::new(x) }));
                    }
                }
                _ => {}
            }
        }
        self.parse_postfix()
    }

    fn parse_postfix(&mut self) -> PR<Expr> {
        let mut x = self.parse_primary()?;
        loop {
            let t = self.peek().clone();
            if t.kind != TokKind::Op {
                return Ok(x);
            }
            match t.text.as_str() {
                "." | "?." => {
                    self.next();
                    let nt = self.peek().clone();
                    if nt.kind != TokKind::Ident && nt.kind != TokKind::Type {
                        return Err(self.err_unexpected(""));
                    }
                    self.next();
                    let null_safe = t.text == "?.";
                    if self.is_op("(") {
                        let args = self.parse_args()?;
                        x = self.mk(t.pos, EK::Call { obj: Some(Box::new(x)), name: nt.text, args, null_safe });
                    } else {
                        x = self.mk(t.pos, EK::Field { obj: Box::new(x), name: nt.text, null_safe });
                    }
                }
                "[" => {
                    self.next();
                    let idx = self.parse_expr()?;
                    self.expect_op("]")?;
                    x = self.mk(t.pos, EK::Index { obj: Box::new(x), idx: Box::new(idx) });
                }
                "++" | "--" => {
                    self.next();
                    x = self.mk(t.pos, EK::IncDec { target: Box::new(x), op: t.text.clone(), prefix: false });
                }
                _ => return Ok(x),
            }
        }
    }

    fn parse_args(&mut self) -> PR<Vec<Expr>> {
        self.next(); // (
        let mut args = Vec::new();
        if self.accept_op(")") {
            return Ok(args);
        }
        loop {
            args.push(self.parse_arg()?);
            if self.accept_op(",") {
                continue;
            }
            self.expect_op(")")?;
            return Ok(args);
        }
    }

    /// A call argument, which may be a lambda or a function reference.
    fn parse_arg(&mut self) -> PR<Expr> {
        let t = self.peek().clone();
        // x -> ...
        if t.kind == TokKind::Ident && self.is_op_at(1, "->") {
            self.next();
            self.next();
            let body = self.parse_lambda_body()?;
            let lam = Lambda { ptypes: vec!["def".into()], pnames: vec![t.text.clone()], body };
            return Ok(self.mk(t.pos, EK::Lambda(Rc::new(lam))));
        }
        // (a, b) -> ... / (int a) -> ... / () -> ...
        if t.kind == TokKind::Op && t.text == "(" {
            let mut k = 1;
            let mut ok = true;
            loop {
                if self.is_op_at(k, ")") {
                    break;
                }
                if self.peek_at(k).kind == TokKind::Type {
                    k += 1;
                }
                if self.peek_at(k).kind != TokKind::Ident {
                    ok = false;
                    break;
                }
                k += 1;
                if self.is_op_at(k, ",") {
                    k += 1;
                    continue;
                }
                if !self.is_op_at(k, ")") {
                    ok = false;
                }
                break;
            }
            if ok && self.is_op_at(k + 1, "->") {
                self.next();
                let mut ptypes = Vec::new();
                let mut pnames = Vec::new();
                while !self.is_op(")") {
                    let mut typ = "def".to_string();
                    if self.peek().kind == TokKind::Type {
                        typ = self.next().text;
                    }
                    ptypes.push(typ);
                    pnames.push(self.next().text);
                    self.accept_op(",");
                }
                self.next(); // )
                self.next(); // ->
                let body = self.parse_lambda_body()?;
                let lam = Lambda { ptypes, pnames, body };
                return Ok(self.mk(t.pos, EK::Lambda(Rc::new(lam))));
            }
        }
        // Type::method / this::fn
        if (t.kind == TokKind::Type || t.kind == TokKind::Ident) && self.is_op_at(1, "::") {
            self.next();
            self.next();
            let m = self.next();
            return Ok(self.mk(t.pos, EK::FuncRef { typ: t.text.clone(), method: m.text }));
        }
        self.parse_expr()
    }

    fn parse_lambda_body(&mut self) -> PR<LBody> {
        if self.is_op("{") {
            let b = self.parse_block()?;
            if let SK::Block(stmts) = b.kind {
                return Ok(LBody::Block(stmts));
            }
            unreachable!()
        }
        Ok(LBody::Expr(self.parse_expr()?))
    }

    fn num_lit(&mut self, t: &Tok, neg: bool, pos: usize) -> PR<Expr> {
        match parse_number(&t.text, t.kind == TokKind::Float, neg) {
            Ok((v, typ)) => Ok(self.mk(pos, EK::Lit(v, typ))),
            Err(e) => Err(CompileErr::iae(&e, pos)),
        }
    }

    fn parse_primary(&mut self) -> PR<Expr> {
        let t = self.peek().clone();
        match t.kind {
            TokKind::Eof => Err(self.err_unexpected("")),
            TokKind::Int | TokKind::Float => {
                self.next();
                self.num_lit(&t, false, t.pos)
            }
            TokKind::Str => {
                self.next();
                Ok(self.mk(t.pos, EK::Lit(PV::str(&t.text), "String")))
            }
            TokKind::Regex => {
                self.next();
                Ok(self.mk(t.pos, EK::Regex(t.text.clone(), t.flags.clone())))
            }
            TokKind::Type => {
                self.next();
                Ok(self.mk(t.pos, EK::Type(t.text.clone())))
            }
            TokKind::Ident => match t.text.as_str() {
                "true" | "false" => {
                    self.next();
                    Ok(self.mk(t.pos, EK::Lit(PV::Bool(t.text == "true"), "boolean")))
                }
                "null" => {
                    self.next();
                    Ok(self.mk(t.pos, EK::Lit(PV::Null, "null")))
                }
                "new" => self.parse_new(),
                _ => {
                    self.next();
                    if self.is_op("(") {
                        let args = self.parse_args()?;
                        return Ok(self.mk(t.pos, EK::Call { obj: None, name: t.text.clone(), args, null_safe: false }));
                    }
                    Ok(self.mk(t.pos, EK::Var(t.text.clone())))
                }
            },
            TokKind::Op => match t.text.as_str() {
                "(" => {
                    self.next();
                    let x = self.parse_expr()?;
                    self.expect_op(")")?;
                    Ok(x)
                }
                "[" => self.parse_list_or_map(),
                _ => Err(self.err_unexpected("")),
            },
        }
    }

    fn parse_list_or_map(&mut self) -> PR<Expr> {
        let t = self.next(); // [
        if self.is_op(":") && self.is_op_at(1, "]") {
            self.next();
            self.next();
            return Ok(self.mk(t.pos, EK::Map(vec![])));
        }
        if self.accept_op("]") {
            return Ok(self.mk(t.pos, EK::List(vec![])));
        }
        let first = self.parse_expr()?;
        if self.accept_op(":") {
            let v = self.parse_expr()?;
            let mut pairs = vec![(first, v)];
            while self.accept_op(",") {
                let k = self.parse_expr()?;
                self.expect_op(":")?;
                let v = self.parse_expr()?;
                pairs.push((k, v));
            }
            self.expect_op("]")?;
            return Ok(self.mk(t.pos, EK::Map(pairs)));
        }
        let mut elems = vec![first];
        while self.accept_op(",") {
            elems.push(self.parse_expr()?);
        }
        self.expect_op("]")?;
        Ok(self.mk(t.pos, EK::List(elems)))
    }

    fn parse_new(&mut self) -> PR<Expr> {
        let t = self.next(); // new
        let nt = self.peek().clone();
        if nt.kind == TokKind::Ident {
            return Err(CompileErr::iae(&format!("Not a type [{}].", nt.text), t.pos));
        }
        if nt.kind != TokKind::Type {
            return Err(self.err_unexpected(""));
        }
        self.next();
        let typ = nt.text.clone();
        if self.is_op("[") {
            let mut dims = Vec::new();
            let mut init = Vec::new();
            let mut rank = 0;
            let mut has_init = false;
            while self.is_op("[") {
                self.next();
                if self.accept_op("]") {
                    rank += 1;
                    continue;
                }
                dims.push(self.parse_expr()?);
                self.expect_op("]")?;
                rank += 1;
            }
            if self.is_op("{") {
                self.next();
                has_init = true;
                while !self.is_op("}") {
                    init.push(self.parse_expr()?);
                    if !self.accept_op(",") {
                        break;
                    }
                }
                self.expect_op("}")?;
            }
            return Ok(self.mk(t.pos, EK::NewArray { elem: typ, dims, init, rank, has_init }));
        }
        if !self.is_op("(") {
            return Err(self.err_unexpected("'('"));
        }
        let args = self.parse_args()?;
        Ok(self.mk(t.pos, EK::New { typ, args }))
    }
}
