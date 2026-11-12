// Painless engine: static analysis.
//
// Painless is statically typed with an escape hatch (def). The checker infers
// the static type of every expression so that compile errors match the real
// compiler (illegal casts, unknown whitelist methods, constant folding of
// division by zero, ...) and records the types the interpreter needs for
// conversions (declared variables, ternary promotion, method parameters).

use std::collections::HashMap;
use std::sync::OnceLock;

use crate::pl_interp::arith;
use crate::pl_lex::{is_type_name, CompileErr};
use crate::pl_parse::{Expr, LBody, Program, Stmt, EK, SK};
use crate::pl_value::{
    box_of, canon, cast_num, is_number, is_numeric_type, is_primitive, jstr, promote, unbox, i64_of, PV,
};

#[derive(Clone, Debug)]
pub struct Sig {
    pub params: Vec<String>,
    pub ret: String,
}

pub type Sigs = HashMap<String, Sig>;

/// Reads "name:ret(p1,p2)" signatures; methods are looked up by name and
/// arity, like the Painless whitelist.
pub fn parse_sigs(specs: &[&str]) -> Sigs {
    let mut out = HashMap::new();
    for s in specs {
        let (name, rest) = s.split_once(':').unwrap();
        let (ret, args) = rest.split_once('(').unwrap();
        let args = args.trim_end_matches(')');
        let params: Vec<String> = if args.is_empty() { vec![] } else { args.split(',').map(|a| a.to_string()).collect() };
        out.insert(format!("{}/{}", name, params.len()), Sig { params, ret: ret.to_string() });
    }
    out
}

pub fn static_methods() -> &'static HashMap<&'static str, Sigs> {
    static M: OnceLock<HashMap<&'static str, Sigs>> = OnceLock::new();
    M.get_or_init(|| {
        let mut m = HashMap::new();
        m.insert("Math", parse_sigs(&[
            "abs:double(double)", "acos:double(double)", "asin:double(double)", "atan:double(double)",
            "cbrt:double(double)", "ceil:double(double)", "cos:double(double)", "cosh:double(double)",
            "exp:double(double)", "expm1:double(double)", "floor:double(double)", "log:double(double)",
            "log10:double(double)", "log1p:double(double)", "rint:double(double)", "signum:double(double)",
            "sin:double(double)", "sinh:double(double)", "sqrt:double(double)", "tan:double(double)",
            "tanh:double(double)", "toDegrees:double(double)", "toRadians:double(double)", "ulp:double(double)",
            "atan2:double(double,double)", "hypot:double(double,double)", "IEEEremainder:double(double,double)",
            "max:double(double,double)", "min:double(double,double)", "pow:double(double,double)",
            "round:long(double)", "random:double()",
        ]));
        m.insert("Integer", parse_sigs(&[
            "bitCount:int(int)", "compare:int(int,int)", "max:int(int,int)", "min:int(int,int)", "sum:int(int,int)",
            "parseInt:int(String)", "parseInt:int(String,int)", "signum:int(int)", "reverse:int(int)",
            "highestOneBit:int(int)", "lowestOneBit:int(int)", "numberOfLeadingZeros:int(int)",
            "numberOfTrailingZeros:int(int)", "toBinaryString:String(int)", "toHexString:String(int)",
            "toOctalString:String(int)", "toString:String(int)", "toString:String(int,int)",
            "valueOf:Integer(int)", "hashCode:int(int)", "rotateLeft:int(int,int)", "rotateRight:int(int,int)",
            "toUnsignedLong:long(int)", "compareUnsigned:int(int,int)", "parseUnsignedInt:int(String)",
        ]));
        m.insert("Long", parse_sigs(&[
            "bitCount:int(long)", "compare:int(long,long)", "max:long(long,long)", "min:long(long,long)",
            "sum:long(long,long)", "parseLong:long(String)", "parseLong:long(String,int)", "signum:int(long)",
            "numberOfLeadingZeros:int(long)", "numberOfTrailingZeros:int(long)",
            "toBinaryString:String(long)", "toHexString:String(long)", "toOctalString:String(long)",
            "toString:String(long)", "toString:String(long,int)", "valueOf:Long(long)", "hashCode:int(long)",
        ]));
        m.insert("Double", parse_sigs(&[
            "compare:int(double,double)", "isFinite:boolean(double)", "isInfinite:boolean(double)",
            "isNaN:boolean(double)", "max:double(double,double)", "min:double(double,double)",
            "sum:double(double,double)", "parseDouble:double(String)", "toString:String(double)",
            "valueOf:Double(double)", "hashCode:int(double)", "doubleToLongBits:long(double)",
            "longBitsToDouble:double(long)", "toHexString:String(double)",
        ]));
        m.insert("Float", parse_sigs(&[
            "compare:int(float,float)", "isFinite:boolean(float)", "isInfinite:boolean(float)", "isNaN:boolean(float)",
            "max:float(float,float)", "min:float(float,float)", "sum:float(float,float)",
            "parseFloat:float(String)", "toString:String(float)", "valueOf:Float(float)", "hashCode:int(float)",
            "floatToIntBits:int(float)", "intBitsToFloat:float(int)",
        ]));
        m.insert("Short", parse_sigs(&["compare:int(short,short)", "parseShort:short(String)", "toString:String(short)",
            "valueOf:Short(short)", "hashCode:int(short)"]));
        m.insert("Byte", parse_sigs(&["compare:int(byte,byte)", "parseByte:byte(String)", "toString:String(byte)",
            "valueOf:Byte(byte)", "hashCode:int(byte)"]));
        m.insert("Boolean", parse_sigs(&["compare:int(boolean,boolean)", "logicalAnd:boolean(boolean,boolean)",
            "logicalOr:boolean(boolean,boolean)", "logicalXor:boolean(boolean,boolean)",
            "parseBoolean:boolean(String)", "toString:String(boolean)", "valueOf:Boolean(boolean)",
            "hashCode:int(boolean)"]));
        m.insert("Character", parse_sigs(&["isDigit:boolean(int)", "isLetter:boolean(int)", "isLetterOrDigit:boolean(int)",
            "isLowerCase:boolean(int)", "isUpperCase:boolean(int)", "isWhitespace:boolean(int)",
            "isAlphabetic:boolean(int)", "isSpaceChar:boolean(int)", "toLowerCase:char(char)",
            "toUpperCase:char(char)", "getNumericValue:int(int)", "valueOf:Character(char)",
            "compare:int(char,char)", "toString:String(char)", "digit:int(int,int)", "hashCode:int(char)",
            "toChars:char[](int)"]));
        m.insert("String", parse_sigs(&["valueOf:String(def)", "join:String(CharSequence,Iterable)",
            "format:String(String,def[])", "copyValueOf:String(char[])"]));
        m.insert("Objects", parse_sigs(&["equals:boolean(def,def)", "hashCode:int(def)", "isNull:boolean(def)",
            "nonNull:boolean(def)", "requireNonNull:def(def)", "requireNonNull:def(def,String)",
            "toString:String(def)", "toString:String(def,String)", "compare:int(def,def,Comparator)",
            "deepEquals:boolean(def,def)", "hash:int(def[])", "requireNonNullElse:def(def,def)"]));
        m.insert("Arrays", parse_sigs(&["asList:List(def[])", "toString:String(def)", "equals:boolean(def,def)",
            "sort:void(def)", "stream:Stream(def)", "fill:void(def,def)", "hashCode:int(def)",
            "deepToString:String(def)", "copyOf:def(def,int)"]));
        m.insert("Collections", parse_sigs(&["sort:void(List)", "sort:void(List,Comparator)", "reverse:void(List)",
            "max:def(Collection)", "min:def(Collection)", "max:def(Collection,Comparator)",
            "min:def(Collection,Comparator)", "shuffle:void(List)", "emptyList:List()", "emptyMap:Map()",
            "emptySet:Set()", "singletonList:List(def)", "singleton:Set(def)", "singletonMap:Map(def,def)",
            "unmodifiableList:List(List)", "unmodifiableMap:Map(Map)", "unmodifiableSet:Set(Set)",
            "frequency:int(Collection,def)", "swap:void(List,int,int)", "nCopies:List(int,def)",
            "reverseOrder:Comparator()"]));
        m.insert("Collectors", parse_sigs(&["toList:def()", "toSet:def()", "joining:def()", "joining:def(CharSequence)",
            "joining:def(CharSequence,CharSequence,CharSequence)", "counting:def()", "toMap:def(def,def)",
            "groupingBy:def(def)", "groupingBy:def(def,def)", "summingInt:def(def)", "summingLong:def(def)",
            "summingDouble:def(def)", "averagingInt:def(def)", "averagingDouble:def(def)", "mapping:def(def,def)"]));
        m.insert("Comparator", parse_sigs(&["naturalOrder:Comparator()", "reverseOrder:Comparator()",
            "comparing:Comparator(def)", "comparing:Comparator(def,Comparator)", "comparingInt:Comparator(def)",
            "comparingLong:Comparator(def)", "comparingDouble:Comparator(def)",
            "nullsFirst:Comparator(Comparator)", "nullsLast:Comparator(Comparator)"]));
        m.insert("Debug", parse_sigs(&["explain:void(def)"]));
        m.insert("Instant", parse_sigs(&["ofEpochMilli:Instant(long)", "ofEpochSecond:Instant(long)", "parse:Instant(CharSequence)"]));
        m.insert("ZonedDateTime", parse_sigs(&["parse:ZonedDateTime(CharSequence)", "ofInstant:ZonedDateTime(Instant,ZoneId)",
            "of:ZonedDateTime(int,int,int,int,int,int,int,ZoneId)"]));
        m.insert("LocalDate", parse_sigs(&["of:LocalDate(int,int,int)", "parse:LocalDate(CharSequence)", "ofEpochDay:LocalDate(long)"]));
        m.insert("LocalDateTime", parse_sigs(&["of:LocalDateTime(int,int,int,int,int)", "of:LocalDateTime(int,int,int,int,int,int)",
            "parse:LocalDateTime(CharSequence)"]));
        m.insert("ZoneId", parse_sigs(&["of:ZoneId(String)"]));
        m.insert("Duration", parse_sigs(&["ofDays:Duration(long)", "ofHours:Duration(long)", "ofMinutes:Duration(long)",
            "ofSeconds:Duration(long)", "ofMillis:Duration(long)", "ofNanos:Duration(long)",
            "between:Duration(def,def)"]));
        m.insert("Period", parse_sigs(&["ofDays:Period(int)", "ofMonths:Period(int)", "ofYears:Period(int)",
            "between:Period(LocalDate,LocalDate)"]));
        m.insert("DayOfWeek", parse_sigs(&["of:DayOfWeek(int)", "valueOf:DayOfWeek(String)"]));
        m.insert("DateTimeFormatter", parse_sigs(&["ofPattern:DateTimeFormatter(String)"]));
        m.insert("Pattern", parse_sigs(&["compile:Pattern(String)", "compile:Pattern(String,int)", "quote:String(String)"]));
        m.insert("Matcher", parse_sigs(&["quoteReplacement:String(String)"]));
        m.insert("Base64", parse_sigs(&["getEncoder:def()", "getDecoder:def()"]));
        m.insert("IntStream", parse_sigs(&["range:IntStream(int,int)", "rangeClosed:IntStream(int,int)"]));
        m.insert("LongStream", parse_sigs(&["range:LongStream(long,long)", "rangeClosed:LongStream(long,long)"]));
        m.insert("Stream", parse_sigs(&["of:Stream(def[])", "empty:Stream()"]));
        m.insert("List", parse_sigs(&["of:List(def[])", "copyOf:List(Collection)"]));
        m.insert("Map", parse_sigs(&["of:Map()", "of:Map(def,def)", "of:Map(def,def,def,def)", "entry:def(def,def)"]));
        m.insert("Set", parse_sigs(&["of:Set(def[])"]));
        m.insert("StrictMath", parse_sigs(&["abs:double(double)", "sqrt:double(double)", "max:double(double,double)",
            "min:double(double,double)"]));
        m
    })
}

/// A static constant: its value and static type.
pub fn static_field(class: &str, name: &str) -> Option<(PV, &'static str)> {
    Some(match (class, name) {
        ("Math", "PI") => (PV::Double(std::f64::consts::PI), "double"),
        ("Math", "E") => (PV::Double(std::f64::consts::E), "double"),
        ("Integer", "MAX_VALUE") => (PV::Int(i32::MAX), "int"),
        ("Integer", "MIN_VALUE") => (PV::Int(i32::MIN), "int"),
        ("Integer", "SIZE") => (PV::Int(32), "int"),
        ("Integer", "BYTES") => (PV::Int(4), "int"),
        ("Long", "MAX_VALUE") => (PV::Long(i64::MAX), "long"),
        ("Long", "MIN_VALUE") => (PV::Long(i64::MIN), "long"),
        ("Long", "SIZE") => (PV::Int(64), "int"),
        ("Long", "BYTES") => (PV::Int(8), "int"),
        ("Short", "MAX_VALUE") => (PV::Short(i16::MAX), "short"),
        ("Short", "MIN_VALUE") => (PV::Short(i16::MIN), "short"),
        ("Byte", "MAX_VALUE") => (PV::Byte(i8::MAX), "byte"),
        ("Byte", "MIN_VALUE") => (PV::Byte(i8::MIN), "byte"),
        ("Character", "MAX_VALUE") => (PV::Char(0xffff), "char"),
        ("Character", "MIN_VALUE") => (PV::Char(0), "char"),
        ("Boolean", "TRUE") => (PV::Bool(true), "Boolean"),
        ("Boolean", "FALSE") => (PV::Bool(false), "Boolean"),
        ("Double", "NaN") => (PV::Double(f64::NAN), "double"),
        ("Double", "POSITIVE_INFINITY") => (PV::Double(f64::INFINITY), "double"),
        ("Double", "NEGATIVE_INFINITY") => (PV::Double(f64::NEG_INFINITY), "double"),
        ("Double", "MAX_VALUE") => (PV::Double(f64::MAX), "double"),
        ("Double", "MIN_VALUE") => (PV::Double(4.9e-324), "double"),
        ("Double", "MIN_NORMAL") => (PV::Double(f64::MIN_POSITIVE), "double"),
        ("Float", "NaN") => (PV::Float(f32::NAN), "float"),
        ("Float", "POSITIVE_INFINITY") => (PV::Float(f32::INFINITY), "float"),
        ("Float", "NEGATIVE_INFINITY") => (PV::Float(f32::NEG_INFINITY), "float"),
        ("Float", "MAX_VALUE") => (PV::Float(f32::MAX), "float"),
        ("Float", "MIN_VALUE") => (PV::Float(1.4e-45), "float"),
        _ => return None,
    })
}

/// The java.lang.String whitelist including Painless augmentations.
pub fn string_methods() -> &'static Sigs {
    static M: OnceLock<Sigs> = OnceLock::new();
    M.get_or_init(|| {
        parse_sigs(&[
            "charAt:char(int)", "chars:IntStream()", "codePointAt:int(int)", "codePointBefore:int(int)",
            "codePointCount:int(int,int)", "compareTo:int(String)", "compareToIgnoreCase:int(String)",
            "concat:String(String)", "contains:boolean(CharSequence)", "contentEquals:boolean(CharSequence)",
            "endsWith:boolean(String)", "equals:boolean(def)", "equalsIgnoreCase:boolean(String)",
            "hashCode:int()", "indexOf:int(def)", "indexOf:int(def,int)", "isEmpty:boolean()",
            "lastIndexOf:int(def)", "lastIndexOf:int(def,int)", "length:int()", "regionMatches:boolean(int,String,int,int)",
            "regionMatches:boolean(boolean,int,String,int,int)", "replace:String(CharSequence,CharSequence)",
            "startsWith:boolean(String)", "startsWith:boolean(String,int)", "subSequence:CharSequence(int,int)",
            "substring:String(int)", "substring:String(int,int)", "toCharArray:char[]()", "toLowerCase:String()",
            "toUpperCase:String()", "trim:String()", "toString:String()", "getClass:def()",
            "decodeBase64:String()", "encodeBase64:String()", "replaceAll:String(Pattern,Function)",
            "replaceFirst:String(Pattern,Function)", "splitOnToken:String[](String)", "splitOnToken:String[](String,int)",
            "codePoints:IntStream()", "strip:String()", "isBlank:boolean()",
        ])
    })
}

// ---- cast legality ----

fn widening(from: &str) -> &'static [&'static str] {
    match from {
        "byte" => &["short", "int", "long", "float", "double"],
        "short" => &["int", "long", "float", "double"],
        "char" => &["int", "long", "float", "double"],
        "int" => &["long", "float", "double"],
        "long" => &["float", "double"],
        "float" => &["double"],
        _ => &[],
    }
}

fn supertypes(t: &str) -> &'static [&'static str] {
    match t {
        "ArrayList" => &["List", "Collection", "Iterable"],
        "LinkedList" => &["List", "Collection", "Iterable", "Deque", "Queue"],
        "List" => &["Collection", "Iterable"],
        "HashMap" => &["Map"],
        "LinkedHashMap" => &["Map", "HashMap"],
        "TreeMap" => &["Map"],
        "HashSet" => &["Set", "Collection", "Iterable"],
        "LinkedHashSet" => &["Set", "Collection", "Iterable", "HashSet"],
        "TreeSet" => &["Set", "Collection", "Iterable"],
        "Set" => &["Collection", "Iterable"],
        "Collection" => &["Iterable"],
        "String" => &["CharSequence", "Comparable"],
        "StringBuilder" => &["CharSequence"],
        "Integer" | "Long" | "Double" | "Float" | "Short" | "Byte" => &["Number"],
        "IllegalArgumentException" | "IllegalStateException" | "ArithmeticException" | "NullPointerException" => {
            &["RuntimeException", "Exception"]
        }
        "RuntimeException" => &["Exception"],
        _ => &[],
    }
}

pub fn can_implicit(from: &str, to: &str) -> bool {
    if from == to || to == "def" || from == "def" {
        return true;
    }
    if from == "null" {
        return !is_primitive(to);
    }
    if is_primitive(from) {
        return widening(from).contains(&to);
    }
    if is_primitive(to) {
        return false;
    }
    if to == "Object" {
        return true;
    }
    supertypes(from).contains(&to)
}

fn can_explicit(from: &str, to: &str, const_val: Option<&PV>) -> bool {
    if can_implicit(from, to) {
        return true;
    }
    if is_numeric_type(from) && is_numeric_type(to) {
        return true;
    }
    if from == "String" && to == "char" {
        return matches!(const_val, Some(PV::Str(s)) if s.chars().count() == 1);
    }
    let (fp, tp) = (is_primitive(from), is_primitive(to));
    match (fp, tp) {
        (true, false) => to == "Object" || to == "Number" || to == box_of(from),
        (false, true) => from == "Object" || from == "Number" || box_of(to) == from,
        (false, false) => {
            if from == "String" || to == "String" {
                return from == "Object" || to == "Object" || from == "CharSequence" || to == "CharSequence";
            }
            true
        }
        _ => false,
    }
}

pub fn cast_err(from: &str, to: &str, off: usize) -> CompileErr {
    CompileErr::new(
        "class_cast_exception",
        &format!("Cannot cast from [{}] to [{}].", canon(from), canon(to)),
        off,
    )
}

// ---- checker ----

pub struct CheckCtx {
    pub vars: Vec<(&'static str, &'static str)>,
    /// Context functions (score functions, ...).
    pub funcs: Sigs,
    /// Required return type of the script.
    pub ret: &'static str,
}

#[derive(Default)]
pub struct Info {
    pub types: HashMap<usize, String>,
    /// The resolved whitelist method of a call.
    pub sigs: HashMap<usize, Sig>,
    pub consts: HashMap<usize, PV>,
}

impl Info {
    pub fn typ(&self, e: &Expr) -> &str {
        self.types.get(&e.id).map(|s| s.as_str()).unwrap_or("")
    }
}

struct Checker<'a> {
    prog: &'a Program,
    ctx: &'a CheckCtx,
    info: Info,
    scopes: Vec<HashMap<String, String>>,
    in_func_ret: Option<String>,
    in_lambda: usize,
}

type CR<T> = Result<T, CompileErr>;

pub fn check(prog: &Program, ctx: &CheckCtx) -> CR<Info> {
    let mut root = HashMap::new();
    for (k, v) in &ctx.vars {
        root.insert(k.to_string(), v.to_string());
    }
    let mut c = Checker { prog, ctx, info: Info::default(), scopes: vec![root], in_func_ret: None, in_lambda: 0 };
    for f in prog.funcs.values() {
        c.in_func_ret = Some(f.ret.clone());
        c.push();
        for (i, n) in f.pnames.iter().enumerate() {
            c.declare(n, &f.ptypes[i]);
        }
        c.stmts(&f.body)?;
        c.pop();
    }
    c.in_func_ret = None;
    let n = prog.body.len();
    for (i, s) in prog.body.iter().enumerate() {
        c.stmt(s)?;
        if i + 1 == n {
            if let SK::Expr(x) = &s.kind {
                if !matches!(x.kind, EK::Assign { .. }) {
                    let t = c.info.typ(x).to_string();
                    c.check_return(&t, x)?;
                }
            }
        }
    }
    Ok(c.info)
}

fn binary_promote(a: &str, b: &str) -> &'static str {
    if a == "def" || b == "def" {
        return "def";
    }
    let rank = |t: &str| match unbox(t) {
        "byte" | "short" | "char" | "int" => Some(1),
        "long" => Some(2),
        "float" => Some(3),
        "double" => Some(4),
        _ => None,
    };
    match (rank(a), rank(b)) {
        (Some(x), Some(y)) => ["", "int", "long", "float", "double"][x.max(y)],
        _ => "",
    }
}

fn op_name(op: &str) -> &'static str {
    match op {
        "<" => "less than",
        "<=" => "less than or equals",
        ">" => "greater than",
        ">=" => "greater than or equals",
        "|" => "boolean or",
        "&" => "boolean and",
        "+" => "add",
        "-" => "subtraction",
        "*" => "multiplication",
        "/" => "division",
        "%" => "remainder",
        "<<" => "left shift",
        ">>" => "right shift",
        ">>>" => "unsigned shift",
        "^" => "xor",
        _ => "",
    }
}

fn apply_err(op: &str, tl: &str, tr: &str, off: usize) -> CompileErr {
    CompileErr::new(
        "class_cast_exception",
        &format!(
            "cannot apply the {} operator [{}] to the types [{}] and [{}]",
            op_name(op),
            op,
            canon(tl),
            canon(tr)
        ),
        off,
    )
}

pub fn is_enum_class(name: &str) -> bool {
    matches!(name, "ChronoUnit" | "DayOfWeek" | "Month" | "DateTimeFormatter" | "ZoneOffset" | "Instant")
}

fn is_bool_type(t: &str) -> bool {
    t == "boolean" || t == "def" || t == "Boolean"
}

impl<'a> Checker<'a> {
    fn check_return(&self, t: &str, x: &Expr) -> CR<()> {
        let want = self.ctx.ret;
        if want.is_empty() || want == "def" || want == "Object" || want == "void" || t.is_empty() || t == "def" || t == "void" {
            return Ok(());
        }
        if !can_implicit(t, want) {
            if t == "null" && is_primitive(want) {
                return Err(cast_err(t, want, x.pos));
            }
            if is_primitive(t) && !is_primitive(want) && (want == "Number" || want == box_of(t)) {
                return Ok(());
            }
            return Err(cast_err(t, want, x.pos));
        }
        Ok(())
    }

    /// An int constant narrows implicitly into byte, short or char when the
    /// value is in range.
    fn const_fits(&self, x: &Expr, to: &str) -> bool {
        let v = match self.info.consts.get(&x.id) {
            Some(PV::Int(v)) => *v,
            _ => return false,
        };
        match to {
            "byte" => (-128..=127).contains(&v),
            "short" => (-32768..=32767).contains(&v),
            "char" => (0..=65535).contains(&v),
            _ => false,
        }
    }

    fn push(&mut self) {
        self.scopes.push(HashMap::new());
    }
    fn pop(&mut self) {
        self.scopes.pop();
    }
    fn declare(&mut self, name: &str, typ: &str) {
        self.scopes.last_mut().unwrap().insert(name.to_string(), typ.to_string());
    }
    fn lookup(&self, name: &str) -> Option<String> {
        self.scopes.iter().rev().find_map(|s| s.get(name).cloned())
    }

    fn stmts(&mut self, list: &[Stmt]) -> CR<()> {
        for s in list {
            self.stmt(s)?;
        }
        Ok(())
    }

    fn check_type(&self, t: &str, off: usize) -> CR<()> {
        if !is_type_name(t.trim_end_matches("[]")) {
            return Err(CompileErr::iae(&format!("Not a type [{}].", t), off));
        }
        Ok(())
    }

    fn scoped(&mut self, s: &Stmt) -> CR<()> {
        self.push();
        let r = self.stmt(s);
        self.pop();
        r
    }

    fn stmt(&mut self, s: &Stmt) -> CR<()> {
        match &s.kind {
            SK::Block(list) => {
                self.push();
                let r = self.stmts(list);
                self.pop();
                r
            }
            SK::Decl { typ, names, inits } => {
                for (i, name) in names.iter().enumerate() {
                    if let Some(init) = &inits[i] {
                        let t = self.expr(init)?;
                        if !can_implicit(&t, typ) && !self.const_fits(init, typ) {
                            return Err(cast_err(&t, typ, init.pos));
                        }
                    }
                    self.declare(name, typ);
                }
                Ok(())
            }
            SK::Expr(x) => self.expr(x).map(|_| ()),
            SK::If { cond, then, els } => {
                self.cond(cond)?;
                self.scoped(then)?;
                if let Some(e) = els {
                    self.scoped(e)?;
                }
                Ok(())
            }
            SK::While { cond, body } => {
                self.cond(cond)?;
                self.scoped(body)
            }
            SK::Do { body, cond } => {
                self.scoped(body)?;
                self.cond(cond)
            }
            SK::For { init, cond, update, body } => {
                self.push();
                let r = (|| {
                    if let Some(i) = init {
                        self.stmt(i)?;
                    }
                    if let Some(c) = cond {
                        self.cond(c)?;
                    }
                    for u in update {
                        self.expr(u)?;
                    }
                    self.stmt(body)
                })();
                self.pop();
                r
            }
            SK::ForEach { typ, name, iter, body } => {
                self.expr(iter)?;
                self.push();
                self.declare(name, typ);
                let r = self.stmt(body);
                self.pop();
                r
            }
            SK::Return(x) => {
                let x = match x {
                    Some(x) => x,
                    None => return Ok(()),
                };
                let t = self.expr(x)?;
                if let Some(ret) = &self.in_func_ret {
                    if self.in_lambda == 0 {
                        if !can_implicit(&t, ret) {
                            return Err(cast_err(&t, ret, x.pos));
                        }
                        return Ok(());
                    }
                }
                if self.in_lambda > 0 {
                    return Ok(());
                }
                self.check_return(&t, x)
            }
            SK::Throw(x) => self.expr(x).map(|_| ()),
            SK::Try { body, catches } => {
                self.stmt(body)?;
                for ct in catches {
                    self.push();
                    self.declare(&ct.name, &ct.typ);
                    let r = self.stmt(&ct.body);
                    self.pop();
                    r?;
                }
                Ok(())
            }
            SK::Break | SK::Continue => Ok(()),
        }
    }

    fn cond(&mut self, x: &Expr) -> CR<()> {
        let t = self.expr(x)?;
        if !is_bool_type(&t) {
            return Err(cast_err(&t, "boolean", x.pos));
        }
        Ok(())
    }

    fn set(&mut self, x: &Expr, t: &str) -> CR<String> {
        self.info.types.insert(x.id, t.to_string());
        Ok(t.to_string())
    }

    fn expr(&mut self, x: &Expr) -> CR<String> {
        match &x.kind {
            EK::Lit(v, t) => {
                self.info.consts.insert(x.id, v.clone());
                self.set(x, t)
            }
            EK::Regex(..) => self.set(x, "Pattern"),
            EK::Var(name) => match self.lookup(name) {
                Some(t) => self.set(x, &t),
                None => Err(CompileErr::iae(&format!("cannot resolve symbol [{}]", name), x.pos)),
            },
            EK::Type(name) => self.set(x, &format!("static:{}", name)),
            EK::Field { obj, name, .. } => {
                if let EK::Type(class) = &obj.kind {
                    if let Some((v, t)) = static_field(class, name) {
                        self.info.consts.insert(x.id, v);
                        self.info.types.insert(obj.id, format!("static:{}", class));
                        return self.set(x, t);
                    }
                    if is_enum_class(class) {
                        self.info.types.insert(obj.id, format!("static:{}", class));
                        return self.set(x, class);
                    }
                    return Err(CompileErr::iae(
                        &format!("static field [{}, {}] not found", canon(class), name),
                        x.pos,
                    ));
                }
                let ot = self.expr(obj)?;
                if ot.ends_with("[]") && name == "length" {
                    return self.set(x, "int");
                }
                if ot == "String" {
                    return Err(CompileErr::iae(&format!("field [java.lang.String, {}] not found", name), x.pos));
                }
                self.set(x, "def")
            }
            EK::Call { .. } => self.call(x),
            EK::Index { obj, idx } => {
                let ot = self.expr(obj)?;
                self.expr(idx)?;
                if ot == "String" || (is_primitive(&ot) && ot != "def") {
                    return Err(CompileErr::iae(&format!("Illegal array access on type [{}].", canon(&ot)), x.pos));
                }
                if let Some(e) = ot.strip_suffix("[]") {
                    let e = e.to_string();
                    return self.set(x, &e);
                }
                self.set(x, "def")
            }
            EK::New { typ, args } => {
                self.check_type(typ, x.pos)?;
                for a in args {
                    self.expr(a)?;
                }
                self.set(x, typ)
            }
            EK::NewArray { elem, dims, init, rank, .. } => {
                for d in dims {
                    let t = self.expr(d)?;
                    if !can_implicit(&t, "int") {
                        return Err(cast_err(&t, "int", d.pos));
                    }
                }
                for e in init {
                    let t = self.expr(e)?;
                    if !can_implicit(&t, elem) {
                        return Err(cast_err(&t, elem, e.pos));
                    }
                }
                let t = format!("{}{}", elem, "[]".repeat(*rank));
                self.set(x, &t)
            }
            EK::List(elems) => {
                for e in elems {
                    self.expr(e)?;
                }
                self.set(x, "ArrayList")
            }
            EK::Map(pairs) => {
                for (k, v) in pairs {
                    self.expr(k)?;
                    self.expr(v)?;
                }
                self.set(x, "HashMap")
            }
            EK::Unary { op, x: inner } => {
                let t = self.expr(inner)?;
                match op.as_str() {
                    "!" => {
                        if !is_bool_type(&t) {
                            return Err(cast_err(&t, "boolean", inner.pos));
                        }
                        self.fold_unary(x, op, inner);
                        self.set(x, "boolean")
                    }
                    "~" => {
                        let p = binary_promote(&t, "int");
                        if p.is_empty() || p == "float" || p == "double" {
                            return Err(CompileErr::new(
                                "class_cast_exception",
                                &format!("cannot apply the bitwise not operator [~] to the type [{}]", canon(&t)),
                                x.pos,
                            ));
                        }
                        self.fold_unary(x, op, inner);
                        self.set(x, p)
                    }
                    _ => {
                        let p = binary_promote(&t, "int");
                        if p.is_empty() {
                            let n = if op == "-" { "negation" } else { "positive" };
                            return Err(CompileErr::new(
                                "class_cast_exception",
                                &format!("cannot apply the {} operator [{}] to the type [{}]", n, op, canon(&t)),
                                x.pos,
                            ));
                        }
                        self.fold_unary(x, op, inner);
                        self.set(x, p)
                    }
                }
            }
            EK::Binary { .. } => self.binary(x),
            EK::Instanceof { x: inner, typ } => {
                self.expr(inner)?;
                self.check_type(typ, x.pos)?;
                self.set(x, "boolean")
            }
            EK::Cast { typ, x: inner } => {
                let t = self.expr(inner)?;
                self.check_type(typ, x.pos)?;
                let cv = self.info.consts.get(&inner.id).cloned();
                if !can_explicit(&t, typ, cv.as_ref()) {
                    return Err(cast_err(&t, typ, inner.pos));
                }
                if let Some(cv) = cv {
                    if is_primitive(typ) {
                        if let (PV::Str(s), "char") = (&cv, typ.as_str()) {
                            let c = s.encode_utf16().next().unwrap_or(0);
                            self.info.consts.insert(x.id, PV::Char(c));
                        } else if is_number(&cv) && is_numeric_type(typ) {
                            self.info.consts.insert(x.id, cast_num(&cv, typ));
                        }
                    }
                }
                self.set(x, typ)
            }
            EK::Cond { c, a, b } => {
                self.cond(c)?;
                let ta = self.expr(a)?;
                let tb = self.expr(b)?;
                let rt = self.promote_cond(&ta, &tb, a, b)?;
                self.set(x, &rt)
            }
            EK::Elvis { a, b } => {
                let ta = self.expr(a)?;
                if is_primitive(&ta) {
                    let mut e = CompileErr::iae("Extraneous elvis operator. LHS is a primitive.", x.pos);
                    e.no_pos = true;
                    return Err(e);
                }
                let tb = self.expr(b)?;
                if ta != "def" && tb != "def" && ta == tb {
                    return self.set(x, &ta);
                }
                self.set(x, "def")
            }
            EK::Assign { target, op, val } => {
                let tt = self.target(target)?;
                let vt = self.expr(val)?;
                if op == "=" {
                    if !can_implicit(&vt, &tt) && !self.const_fits(val, &tt) {
                        return Err(cast_err(&vt, &tt, val.pos));
                    }
                    return self.set(x, &tt);
                }
                if tt == "String" && op == "+" {
                    return self.set(x, &tt);
                }
                if tt != "def"
                    && vt != "def"
                    && binary_promote(&tt, &vt).is_empty()
                    && !(tt == "boolean" && matches!(op.as_str(), "&" | "|" | "^"))
                {
                    return Err(cast_err(&vt, &tt, val.pos));
                }
                self.set(x, &tt)
            }
            EK::IncDec { target, .. } => {
                let tt = self.target(target)?;
                if tt != "def" && !is_numeric_type(unbox(&tt)) {
                    return Err(cast_err(&tt, "int", x.pos));
                }
                self.set(x, &tt)
            }
            EK::Lambda(lam) => {
                self.push();
                self.in_lambda += 1;
                for (i, n) in lam.pnames.iter().enumerate() {
                    self.declare(n, &lam.ptypes[i]);
                }
                let r = match &lam.body {
                    LBody::Block(stmts) => self.stmts(stmts),
                    LBody::Expr(e) => self.expr(e).map(|_| ()),
                };
                self.in_lambda -= 1;
                self.pop();
                r?;
                self.set(x, "def")
            }
            EK::FuncRef { .. } => self.set(x, "def"),
        }
    }

    /// Types an assignment target.
    fn target(&mut self, x: &Expr) -> CR<String> {
        match &x.kind {
            EK::Var(name) => match self.lookup(name) {
                Some(t) => {
                    self.info.types.insert(x.id, t.clone());
                    Ok(t)
                }
                None => Err(CompileErr::iae(&format!("cannot resolve symbol [{}]", name), x.pos)),
            },
            EK::Field { .. } | EK::Index { .. } => self.expr(x),
            _ => Err(CompileErr::iae(
                "invalid assignment: cannot assign a value to a non-variable expression",
                x.pos,
            )),
        }
    }

    fn promote_cond(&self, ta: &str, tb: &str, a: &Expr, b: &Expr) -> CR<String> {
        if ta == "def" || tb == "def" {
            return Ok("def".into());
        }
        if ta == tb {
            return Ok(ta.into());
        }
        let p = binary_promote(ta, tb);
        if !p.is_empty() && is_primitive(ta) && is_primitive(tb) {
            return Ok(p.into());
        }
        if ta == "null" && !is_primitive(tb) {
            return Ok(tb.into());
        }
        if tb == "null" && !is_primitive(ta) {
            return Ok(ta.into());
        }
        // mixed kinds promote to Object, which primitives cannot implicitly reach
        if is_primitive(ta) {
            return Err(cast_err(ta, "Object", a.pos));
        }
        if is_primitive(tb) {
            return Err(cast_err(tb, "Object", b.pos));
        }
        Ok("Object".into())
    }

    fn fold_unary(&mut self, x: &Expr, op: &str, inner: &Expr) {
        let cv = match self.info.consts.get(&inner.id) {
            Some(v) if !v.is_null() => v.clone(),
            _ => return,
        };
        let folded = match op {
            "!" => cv.as_bool().map(|b| PV::Bool(!b)),
            "-" if is_number(&cv) => {
                let zero = cast_num(&PV::Int(0), promote(&cv, &PV::Int(0)));
                arith("-", &zero, &cv).ok()
            }
            "~" if is_number(&cv) => Some(if promote(&cv, &PV::Int(0)) == "long" {
                PV::Long(!i64_of(&cv))
            } else {
                PV::Int(!(i64_of(&cv) as i32))
            }),
            _ => None,
        };
        if let Some(v) = folded {
            self.info.consts.insert(x.id, v);
        }
    }

    fn binary(&mut self, x: &Expr) -> CR<String> {
        let (op, l, r) = match &x.kind {
            EK::Binary { op, l, r } => (op.as_str(), l, r),
            _ => unreachable!(),
        };
        let tl = self.expr(l)?;
        let tr = self.expr(r)?;
        let cl = self.info.consts.get(&l.id).cloned();
        let cr = self.info.consts.get(&r.id).cloned();
        match op {
            "&&" | "||" => {
                for (t, side) in [(&tl, l), (&tr, r)] {
                    if !is_bool_type(t) {
                        return Err(cast_err(t, "boolean", side.pos));
                    }
                }
                return self.set(x, "boolean");
            }
            "==" | "!=" | "===" | "!==" => {
                if tl == "null" && tr == "null" {
                    return Err(CompileErr::iae("extraneous comparison of [null] constants", x.pos));
                }
                return self.set(x, "boolean");
            }
            "<" | "<=" | ">" | ">=" => {
                if tl != "def" && tr != "def" && binary_promote(&tl, &tr).is_empty() {
                    return Err(apply_err(op, &tl, &tr, x.pos));
                }
                return self.set(x, "boolean");
            }
            "=~" | "==~" => return self.set(x, "boolean"),
            "+" => {
                if tl == "String" || tr == "String" {
                    if let (Some(a), Some(b)) = (&cl, &cr) {
                        self.info.consts.insert(x.id, PV::str(&format!("{}{}", jstr(a), jstr(b))));
                    }
                    return self.set(x, "String");
                }
            }
            "&" | "|" | "^" => {
                if (tl == "boolean" || tl == "Boolean") && (tr == "boolean" || tr == "Boolean") {
                    if op == "^" {
                        return self.set(x, "boolean");
                    }
                    return Err(apply_err(op, &tl, &tr, x.pos));
                }
            }
            _ => {}
        }
        if tl == "def" || tr == "def" {
            return self.set(x, "def");
        }
        let rt = match op {
            "<<" | ">>" | ">>>" => {
                if binary_promote(&tr, "int").is_empty() {
                    ""
                } else {
                    binary_promote(&tl, "int")
                }
            }
            _ => binary_promote(&tl, &tr),
        };
        if rt.is_empty() {
            // a String mixed into numeric arithmetic fails casting to the other side
            if tl == "String" && tr != "String" {
                return Err(cast_err(&tl, unbox(&tr), x.pos));
            }
            if tr == "String" && tl != "String" {
                return Err(cast_err(&tr, unbox(&tl), x.pos));
            }
            return Err(apply_err(op, &tl, &tr, x.pos));
        }
        if (rt == "float" || rt == "double") && matches!(op, "&" | "|" | "^" | "<<" | ">>" | ">>>") {
            return Err(apply_err(op, &tl, &tr, x.pos));
        }
        if let (Some(a), Some(b)) = (&cl, &cr) {
            if is_number(a) && is_number(b) {
                match arith(op, a, b) {
                    Ok(v) => {
                        self.info.consts.insert(x.id, v);
                    }
                    Err(e) => {
                        return Err(CompileErr::new(&e.typ, e.reason.as_deref().unwrap_or(""), x.pos));
                    }
                }
            }
        }
        self.set(x, rt)
    }

    fn call(&mut self, x: &Expr) -> CR<String> {
        let (obj, name, args, null_safe) = match &x.kind {
            EK::Call { obj, name, args, null_safe } => (obj, name, args, *null_safe),
            _ => unreachable!(),
        };
        let mut arg_types = Vec::with_capacity(args.len());
        for a in args {
            arg_types.push(self.expr(a)?);
        }
        let key = format!("{}/{}", name, args.len());
        let check_args = |params: &[String]| -> CR<()> {
            for (i, p) in params.iter().enumerate() {
                let at = &arg_types[i];
                if !can_implicit(at, p) && !(is_primitive(at) && (p == "Object" || p == "Number")) {
                    return Err(cast_err(at, p, args[i].pos));
                }
            }
            Ok(())
        };
        let obj = match obj {
            None => {
                if let Some(f) = self.prog.funcs.get(name) {
                    if f.pnames.len() != args.len() {
                        return Err(CompileErr::iae(
                            &format!("Unknown call [{}] with [{}] arguments.", name, args.len()),
                            x.pos,
                        ));
                    }
                    check_args(&f.ptypes)?;
                    let sig = Sig { params: f.ptypes.clone(), ret: f.ret.clone() };
                    self.info.sigs.insert(x.id, sig);
                    let ret = f.ret.clone();
                    return self.set(x, &ret);
                }
                if let Some(sig) = self.ctx.funcs.get(&key) {
                    check_args(&sig.params)?;
                    let sig = sig.clone();
                    let ret = sig.ret.clone();
                    self.info.sigs.insert(x.id, sig);
                    return self.set(x, &ret);
                }
                return Err(CompileErr::iae(
                    &format!("Unknown call [{}] with [{}] arguments.", name, args.len()),
                    x.pos,
                ));
            }
            Some(o) => o,
        };
        if let EK::Type(class) = &obj.kind {
            self.info.types.insert(obj.id, format!("static:{}", class));
            let sig = match static_methods().get(class.as_str()).and_then(|m| m.get(&key)) {
                Some(s) => s.clone(),
                None => {
                    return Err(CompileErr::iae(
                        &format!("static method [{}, {}] not found", canon(class), key),
                        x.pos,
                    ))
                }
            };
            check_args(&sig.params)?;
            let ret = sig.ret.clone();
            self.info.sigs.insert(x.id, sig);
            return self.set(x, &ret);
        }
        if let EK::Var(v) = &obj.kind {
            if self.lookup(v).is_none() {
                return Err(CompileErr::iae(&format!("cannot resolve symbol [{}]", v), x.pos));
            }
        }
        let ot = self.expr(obj)?;
        if ot == "String" {
            let sig = match string_methods().get(&key) {
                Some(s) => s.clone(),
                None => {
                    return Err(CompileErr::iae(
                        &format!("member method [java.lang.String, {}] not found", key),
                        x.pos,
                    ))
                }
            };
            check_args(&sig.params)?;
            if null_safe && is_primitive(&sig.ret) {
                let mut e = CompileErr::iae("Result of null safe operator must be nullable", x.pos);
                e.no_pos = true;
                return Err(e);
            }
            let ret = sig.ret.clone();
            self.info.sigs.insert(x.id, sig);
            return self.set(x, &ret);
        }
        self.set(x, "def")
    }
}
