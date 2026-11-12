// Painless engine: runtime values with Java semantics.
//
// The engine is split over the pl_*.rs files (a port of the Go server's
// pl_*.go design): lexing (pl_lex), parsing (pl_parse), static checks
// (pl_check), runtime values (this file), the interpreter (pl_interp), the
// class library (pl_lib), java.time (pl_time), regexes (pl_regex) and the
// script contexts / error rendering / _execute API (pl_api).
//
// Boxed primitives keep their Java width: Int is java.lang.Integer, Long is
// java.lang.Long, Char is java.lang.Character and so on.

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use serde_json::Value;

use crate::pl_interp::PFn;
use crate::pl_regex::{PMatcher, PPattern};
use crate::pl_time::{Dtf, Zdt};

pub type R<T> = Result<T, RtErr>;

/// A runtime exception raised inside a script.
#[derive(Clone)]
pub struct RtErr {
    pub typ: String,
    /// None renders as `"reason": null`.
    pub reason: Option<String>,
    pub offset: usize,
    /// Java frames printed before the script frame in script_stack.
    pub frames: Vec<String>,
    /// Extra keys of the script_exception (Debug.explain).
    pub extra: Vec<(String, Value)>,
    /// java.lang.Error: not catchable by `catch (Exception e)`.
    pub is_error: bool,
    pub thrown: Option<Rc<PException>>,
}

impl RtErr {
    pub fn new(typ: &str, reason: &str, off: usize) -> Self {
        RtErr {
            typ: typ.to_string(),
            reason: Some(reason.to_string()),
            offset: off,
            frames: Vec::new(),
            extra: Vec::new(),
            is_error: false,
            thrown: None,
        }
    }
    pub fn nil(typ: &str, off: usize) -> Self {
        let mut e = RtErr::new(typ, "", off);
        e.reason = None;
        e
    }
    pub fn frames(mut self, frames: &[&str]) -> Self {
        self.frames = frames.iter().map(|s| s.to_string()).collect();
        self
    }
}

pub fn npe(off: usize) -> RtErr {
    RtErr::new(
        "null_pointer_exception",
        "Cannot invoke \"Object.getClass()\" because \"callArgs[0]\" is null",
        off,
    )
}

pub fn iae(off: usize, reason: &str) -> RtErr {
    RtErr::new("illegal_argument_exception", reason, off)
}

pub fn cce(off: usize, reason: &str) -> RtErr {
    RtErr::new("class_cast_exception", reason, off)
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum MapKind {
    Hash,
    Linked,
    Tree,
}

/// java.util.Map: HashMap (hash iteration order), LinkedHashMap (insertion
/// order) or TreeMap (sorted keys).
#[derive(Clone)]
pub struct PMap {
    pub kind: MapKind,
    pub entries: Vec<(PV, PV)>,
    index: HashMap<String, usize>,
    cap: usize,
}

impl PMap {
    pub fn new(kind: MapKind) -> Self {
        PMap { kind, entries: Vec::new(), index: HashMap::new(), cap: 16 }
    }
    pub fn get(&self, k: &PV) -> Option<PV> {
        self.index.get(&key_id(k)).map(|&i| self.entries[i].1.clone())
    }
    pub fn contains(&self, k: &PV) -> bool {
        self.index.contains_key(&key_id(k))
    }
    /// Puts a value and returns the previous one (Null when absent).
    pub fn put(&mut self, k: PV, v: PV) -> PV {
        let id = key_id(&k);
        if let Some(&i) = self.index.get(&id) {
            return std::mem::replace(&mut self.entries[i].1, v);
        }
        self.index.insert(id, self.entries.len());
        self.entries.push((k, v));
        while self.entries.len() as f64 > self.cap as f64 * 0.75 {
            self.cap *= 2;
        }
        PV::Null
    }
    pub fn remove(&mut self, k: &PV) -> Option<PV> {
        let i = *self.index.get(&key_id(k))?;
        let (_, old) = self.entries.remove(i);
        self.reindex();
        Some(old)
    }
    fn reindex(&mut self) {
        self.index.clear();
        for (i, (k, _)) in self.entries.iter().enumerate() {
            self.index.insert(key_id(k), i);
        }
    }
    pub fn clear(&mut self) {
        *self = PMap::new(self.kind);
    }
    pub fn len(&self) -> usize {
        self.entries.len()
    }
    /// Entries in Java iteration order.
    pub fn ordered(&self) -> Vec<(PV, PV)> {
        let mut out = self.entries.clone();
        match self.kind {
            MapKind::Hash => {
                let mask = (self.cap - 1) as i32;
                out.sort_by_key(|(k, _)| spread(hash_code(k)) & mask);
            }
            MapKind::Tree => {
                out.sort_by(|a, b| compare(&a.0, &b.0).unwrap_or(0).cmp(&0));
            }
            MapKind::Linked => {}
        }
        out
    }
    pub fn keys(&self) -> Vec<PV> {
        self.ordered().into_iter().map(|(k, _)| k).collect()
    }
    pub fn values(&self) -> Vec<PV> {
        self.ordered().into_iter().map(|(_, v)| v).collect()
    }
}

fn spread(h: i32) -> i32 {
    h ^ ((h as u32) >> 16) as i32
}

pub struct PArray {
    pub elem: String,
    pub a: RefCell<Vec<PV>>,
}

pub struct PException {
    pub typ: String,
    pub msg: Option<String>,
    pub class: String,
}

pub struct PDocValues {
    /// Longs, Doubles, Strings, Booleans, Dates or GeoPoints.
    pub kind: String,
    pub vals: Vec<PV>,
}

pub type DocGetter = dyn Fn(&str, usize) -> R<PV>;

/// The doc variable (LeafDocLookup).
pub struct PDocMap {
    pub get: Box<DocGetter>,
    pub has: Box<dyn Fn(&str) -> bool>,
}

pub struct PStream {
    /// obj, int, long or double.
    pub kind: &'static str,
    pub items: Vec<PV>,
}

pub struct POptional {
    pub name: &'static str,
    pub v: Option<PV>,
}

pub struct PCollector {
    pub kind: String,
    pub sep: String,
    pub prefix: String,
    pub suffix: String,
    pub f1: PV,
    pub f2: PV,
}

#[derive(Clone)]
pub enum PV {
    Null,
    Bool(bool),
    Byte(i8),
    Short(i16),
    Char(u16),
    Int(i32),
    Long(i64),
    Float(f32),
    Double(f64),
    Str(Rc<str>),
    List(Rc<RefCell<Vec<PV>>>),
    Map(Rc<RefCell<PMap>>),
    Set(Rc<RefCell<PMap>>),
    Array(Rc<PArray>),
    SB(Rc<RefCell<Vec<char>>>),
    /// Map.Entry: the map and the entry's key.
    Entry(Rc<RefCell<PMap>>, Rc<PV>, Rc<PV>),
    EntrySet(Rc<RefCell<Vec<PV>>>),
    Iter(Rc<RefCell<(Vec<PV>, usize)>>),
    Pattern(Rc<PPattern>),
    Matcher(Rc<RefCell<PMatcher>>),
    Stream(Rc<PStream>),
    Optional(Rc<POptional>),
    Collector(Rc<PCollector>),
    Fn(Rc<PFn>),
    Exception(Rc<PException>),
    StaticRef(Rc<str>),
    DocValues(Rc<PDocValues>),
    DocMap(Rc<PDocMap>),
    GeoPoint(f64, f64),
    Zdt(Rc<Zdt>),
    /// LocalDate as epoch day.
    LocalDate(i64),
    /// Instant: epoch millis and nanos within the milli.
    Instant(i64, i64),
    /// Duration in nanoseconds.
    Duration(i128),
    /// 1 = MONDAY
    DayOfWeek(u8),
    ChronoUnit(Rc<str>),
    ZoneId(Rc<str>),
    Dtf(Rc<Dtf>),
    StrEnum(Rc<str>),
}

impl PV {
    pub fn str(s: &str) -> PV {
        PV::Str(Rc::from(s))
    }
    pub fn list(v: Vec<PV>) -> PV {
        PV::List(Rc::new(RefCell::new(v)))
    }
    pub fn map(m: PMap) -> PV {
        PV::Map(Rc::new(RefCell::new(m)))
    }
    pub fn set(m: PMap) -> PV {
        PV::Set(Rc::new(RefCell::new(m)))
    }
    pub fn array(elem: &str, a: Vec<PV>) -> PV {
        PV::Array(Rc::new(PArray { elem: elem.to_string(), a: RefCell::new(a) }))
    }
    pub fn stream(kind: &'static str, items: Vec<PV>) -> PV {
        PV::Stream(Rc::new(PStream { kind, items }))
    }
    pub fn optional(name: &'static str, v: Option<PV>) -> PV {
        PV::Optional(Rc::new(POptional { name, v }))
    }
    pub fn iter(items: Vec<PV>) -> PV {
        PV::Iter(Rc::new(RefCell::new((items, 0))))
    }
    pub fn is_null(&self) -> bool {
        matches!(self, PV::Null)
    }
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            PV::Bool(b) => Some(*b),
            _ => None,
        }
    }
}

pub fn set_add(s: &Rc<RefCell<PMap>>, v: PV) -> bool {
    let mut m = s.borrow_mut();
    if m.contains(&v) {
        return false;
    }
    m.put(v, PV::Bool(true));
    true
}

pub fn new_set_from(kind: MapKind, items: Vec<PV>) -> PV {
    let mut m = PMap::new(kind);
    for it in items {
        if !m.contains(&it) {
            m.put(it, PV::Bool(true));
        }
    }
    PV::set(m)
}

/// Identity for map keys following Java equals: Integer 1 and Long 1 are
/// different keys.
pub fn key_id(k: &PV) -> String {
    match k {
        PV::Null => "n".into(),
        PV::Str(s) => format!("s{}", s),
        PV::Int(i) => format!("i{}", i),
        PV::Long(i) => format!("l{}", i),
        PV::Short(i) => format!("h{}", i),
        PV::Byte(i) => format!("b{}", i),
        PV::Char(c) => format!("c{}", c),
        PV::Double(f) => format!("d{:x}", f.to_bits()),
        PV::Float(f) => format!("f{:x}", f.to_bits()),
        PV::Bool(b) => if *b { "zt".into() } else { "zf".into() },
        _ => format!("o{}", jstr(k)),
    }
}

pub fn utf16(s: &str) -> Vec<u16> {
    s.encode_utf16().collect()
}

pub fn from_utf16(u: &[u16]) -> String {
    String::from_utf16_lossy(u)
}

pub fn string_hash(s: &str) -> i32 {
    let mut h: i32 = 0;
    for u in s.encode_utf16() {
        h = h.wrapping_mul(31).wrapping_add(u as i32);
    }
    h
}

pub fn hash_code(v: &PV) -> i32 {
    match v {
        PV::Null => 0,
        PV::Str(s) => string_hash(s),
        PV::Int(i) => *i,
        PV::Short(i) => *i as i32,
        PV::Byte(i) => *i as i32,
        PV::Char(c) => *c as i32,
        PV::Long(l) => (*l ^ ((*l as u64) >> 32) as i64) as i32,
        PV::Double(f) => {
            if *f == 0.0 && !f.is_sign_negative() {
                return 0;
            }
            let b = if f.is_nan() { 0x7ff8000000000000u64 } else { f.to_bits() };
            (b ^ (b >> 32)) as i32
        }
        PV::Float(f) => {
            if f.is_nan() {
                0x7fc00000
            } else {
                f.to_bits() as i32
            }
        }
        PV::Bool(b) => if *b { 1231 } else { 1237 },
        PV::List(l) => {
            let mut h: i32 = 1;
            for e in l.borrow().iter() {
                h = h.wrapping_mul(31).wrapping_add(hash_code(e));
            }
            h
        }
        PV::Map(m) => {
            let mut h: i32 = 0;
            for (k, v) in m.borrow().entries.iter() {
                h = h.wrapping_add(hash_code(k) ^ hash_code(v));
            }
            h
        }
        PV::Set(m) => {
            let mut h: i32 = 0;
            for (k, _) in m.borrow().entries.iter() {
                h = h.wrapping_add(hash_code(k));
            }
            h
        }
        PV::Array(a) => (Rc::as_ptr(a) as usize as u32 >> 4) as i32,
        _ => string_hash(&jstr(v)),
    }
}

pub fn is_number(v: &PV) -> bool {
    matches!(
        v,
        PV::Int(_) | PV::Long(_) | PV::Float(_) | PV::Double(_) | PV::Byte(_) | PV::Short(_) | PV::Char(_)
    )
}

pub fn prim_name(v: &PV) -> &'static str {
    match v {
        PV::Int(_) => "int",
        PV::Long(_) => "long",
        PV::Float(_) => "float",
        PV::Double(_) => "double",
        PV::Byte(_) => "byte",
        PV::Short(_) => "short",
        PV::Char(_) => "char",
        PV::Bool(_) => "boolean",
        _ => "",
    }
}

pub fn promote(a: &PV, b: &PV) -> &'static str {
    let (ta, tb) = (prim_name(a), prim_name(b));
    if ta == "double" || tb == "double" {
        "double"
    } else if ta == "float" || tb == "float" {
        "float"
    } else if ta == "long" || tb == "long" {
        "long"
    } else {
        "int"
    }
}

pub fn i64_of(v: &PV) -> i64 {
    match v {
        PV::Int(i) => *i as i64,
        PV::Long(i) => *i,
        PV::Short(i) => *i as i64,
        PV::Byte(i) => *i as i64,
        PV::Char(c) => *c as i64,
        PV::Float(f) => *f as i64,
        PV::Double(f) => *f as i64,
        _ => 0,
    }
}

pub fn f64_of(v: &PV) -> f64 {
    match v {
        PV::Int(i) => *i as f64,
        PV::Long(i) => *i as f64,
        PV::Short(i) => *i as f64,
        PV::Byte(i) => *i as f64,
        PV::Char(c) => *c as f64,
        PV::Float(f) => *f as f64,
        PV::Double(f) => *f,
        _ => 0.0,
    }
}

/// Converts a numeric value to a primitive type with Java narrowing and
/// widening semantics (Rust's saturating float casts match Java's).
pub fn cast_num(v: &PV, to: &str) -> PV {
    let is_float = matches!(v, PV::Float(_) | PV::Double(_));
    match to {
        "int" => PV::Int(if is_float { f64_of(v) as i32 } else { i64_of(v) as i32 }),
        "long" => PV::Long(if is_float { f64_of(v) as i64 } else { i64_of(v) }),
        "short" => PV::Short(if is_float { f64_of(v) as i32 as i16 } else { i64_of(v) as i16 }),
        "byte" => PV::Byte(if is_float { f64_of(v) as i32 as i8 } else { i64_of(v) as i8 }),
        "char" => PV::Char(if is_float { f64_of(v) as i32 as u16 } else { i64_of(v) as u16 }),
        "float" => match v {
            PV::Double(d) => PV::Float(*d as f32),
            PV::Long(l) => PV::Float(*l as f32),
            _ => PV::Float(f64_of(v) as f32),
        },
        "double" => PV::Double(f64_of(v)),
        _ => v.clone(),
    }
}

/// Painless == on def values: numbers compare by value after promotion,
/// everything else via equals().
pub fn equals(a: &PV, b: &PV) -> bool {
    if a.is_null() || b.is_null() {
        return a.is_null() && b.is_null();
    }
    if is_number(a) && is_number(b) {
        return match promote(a, b) {
            "double" | "float" => f64_of(a) == f64_of(b),
            _ => i64_of(a) == i64_of(b),
        };
    }
    java_equals(a, b)
}

fn list_items_of(v: &PV) -> Option<Vec<PV>> {
    match v {
        PV::List(l) => Some(l.borrow().clone()),
        PV::DocValues(d) => Some(d.vals.clone()),
        _ => None,
    }
}

/// Object.equals: boxed numbers are only equal to the same type.
pub fn java_equals(a: &PV, b: &PV) -> bool {
    if a.is_null() || b.is_null() {
        return a.is_null() && b.is_null();
    }
    match (a, b) {
        (PV::List(_), _) => {
            let (x, y) = match (list_items_of(a), list_items_of(b)) {
                (Some(x), Some(y)) => (x, y),
                _ => return false,
            };
            x.len() == y.len() && x.iter().zip(y.iter()).all(|(p, q)| java_equals(p, q))
        }
        (PV::Map(x), PV::Map(y)) => {
            if Rc::ptr_eq(x, y) {
                return true;
            }
            let (x, y) = (x.borrow(), y.borrow());
            if x.len() != y.len() {
                return false;
            }
            x.entries.iter().all(|(k, v)| match y.get(k) {
                Some(w) => java_equals(v, &w),
                None => false,
            })
        }
        (PV::Set(x), PV::Set(y)) => {
            if Rc::ptr_eq(x, y) {
                return true;
            }
            let (x, y) = (x.borrow(), y.borrow());
            x.len() == y.len() && x.entries.iter().all(|(k, _)| y.contains(k))
        }
        (PV::Double(x), PV::Double(y)) => x.to_bits() == y.to_bits() || (x == y && *x != 0.0),
        (PV::Float(x), PV::Float(y)) => x == y,
        (PV::Int(x), PV::Int(y)) => x == y,
        (PV::Long(x), PV::Long(y)) => x == y,
        (PV::Short(x), PV::Short(y)) => x == y,
        (PV::Byte(x), PV::Byte(y)) => x == y,
        (PV::Char(x), PV::Char(y)) => x == y,
        (PV::Str(x), PV::Str(y)) => x == y,
        (PV::Bool(x), PV::Bool(y)) => x == y,
        (PV::Zdt(x), PV::Zdt(y)) => x.instant_nanos() == y.instant_nanos(),
        (PV::LocalDate(x), PV::LocalDate(y)) => x == y,
        (PV::Instant(a1, a2), PV::Instant(b1, b2)) => a1 == b1 && a2 == b2,
        (PV::Duration(x), PV::Duration(y)) => x == y,
        (PV::DayOfWeek(x), PV::DayOfWeek(y)) => x == y,
        (PV::ChronoUnit(x), PV::ChronoUnit(y)) => x == y,
        (PV::ZoneId(x), PV::ZoneId(y)) => x == y,
        (PV::StrEnum(x), PV::StrEnum(y)) => x == y,
        (PV::GeoPoint(a1, a2), PV::GeoPoint(b1, b2)) => a1 == b1 && a2 == b2,
        (PV::Entry(_, k1, v1), PV::Entry(_, k2, v2)) => java_equals(k1, k2) && java_equals(v1, v2),
        _ => identical(a, b),
    }
}

/// === (reference equality). Boxed integral values from -128..127 are cached
/// by the JVM and therefore identical.
pub fn identical(a: &PV, b: &PV) -> bool {
    if a.is_null() || b.is_null() {
        return a.is_null() && b.is_null();
    }
    match (a, b) {
        (PV::Int(_), PV::Int(_))
        | (PV::Long(_), PV::Long(_))
        | (PV::Short(_), PV::Short(_))
        | (PV::Byte(_), PV::Byte(_))
        | (PV::Char(_), PV::Char(_)) => {
            let v = i64_of(a);
            v == i64_of(b) && (-128..=127).contains(&v)
        }
        (PV::Float(_), _) | (PV::Double(_), _) => false,
        (PV::Bool(x), PV::Bool(y)) => x == y,
        (PV::Str(x), PV::Str(y)) => x == y,
        (PV::List(x), PV::List(y)) => Rc::ptr_eq(x, y),
        (PV::Map(x), PV::Map(y)) => Rc::ptr_eq(x, y),
        (PV::Set(x), PV::Set(y)) => Rc::ptr_eq(x, y),
        (PV::Array(x), PV::Array(y)) => Rc::ptr_eq(x, y),
        (PV::SB(x), PV::SB(y)) => Rc::ptr_eq(x, y),
        (PV::Fn(x), PV::Fn(y)) => Rc::ptr_eq(x, y),
        (PV::Exception(x), PV::Exception(y)) => Rc::ptr_eq(x, y),
        (PV::DocValues(x), PV::DocValues(y)) => Rc::ptr_eq(x, y),
        (PV::Zdt(x), PV::Zdt(y)) => Rc::ptr_eq(x, y),
        (PV::DayOfWeek(x), PV::DayOfWeek(y)) => x == y,
        (PV::ChronoUnit(x), PV::ChronoUnit(y)) => x == y,
        (PV::StrEnum(x), PV::StrEnum(y)) => x == y,
        _ => false,
    }
}

/// Comparable.compareTo for naturally ordered values.
pub fn compare(a: &PV, b: &PV) -> Option<i32> {
    fn sign<T: PartialOrd>(x: T, y: T) -> i32 {
        if x < y {
            -1
        } else if x > y {
            1
        } else {
            0
        }
    }
    if is_number(a) && is_number(b) {
        return Some(match promote(a, b) {
            "double" | "float" => sign(f64_of(a), f64_of(b)),
            _ => sign(i64_of(a), i64_of(b)),
        });
    }
    match (a, b) {
        (PV::Str(x), PV::Str(y)) => Some(string_compare(x, y)),
        (PV::Bool(x), PV::Bool(y)) => Some(sign(*x, *y)),
        (PV::Zdt(x), PV::Zdt(y)) => Some(sign(x.instant_nanos(), y.instant_nanos())),
        (PV::LocalDate(x), PV::LocalDate(y)) => Some(sign(*x, *y)),
        (PV::Instant(a1, a2), PV::Instant(b1, b2)) => Some(sign((*a1, *a2), (*b1, *b2))),
        (PV::Duration(x), PV::Duration(y)) => Some(sign(*x, *y)),
        (PV::DayOfWeek(x), PV::DayOfWeek(y)) => Some(sign(*x, *y)),
        _ => None,
    }
}

/// String.compareTo over UTF-16 code units.
pub fn string_compare(a: &str, b: &str) -> i32 {
    let ua = utf16(a);
    let ub = utf16(b);
    for (x, y) in ua.iter().zip(ub.iter()) {
        if x != y {
            return *x as i32 - *y as i32;
        }
    }
    ua.len() as i32 - ub.len() as i32
}

// ---- number literals ----

/// Parses a numeric literal the way the Painless compiler does.
pub fn parse_number(text: &str, is_float: bool, neg: bool) -> Result<(PV, &'static str), String> {
    if is_float {
        let last = text.as_bytes()[text.len() - 1];
        let mut body = text;
        let mut typ = "double";
        if last == b'f' || last == b'F' {
            typ = "float";
            body = &text[..text.len() - 1];
        } else if last == b'd' || last == b'D' {
            body = &text[..text.len() - 1];
        }
        let body = if neg { format!("-{}", body) } else { body.to_string() };
        if typ == "float" {
            return match body.parse::<f32>() {
                Ok(f) => Ok((PV::Float(f), "float")),
                Err(_) => Err(format!("Invalid float constant [{}].", text)),
            };
        }
        return match body.parse::<f64>() {
            Ok(f) => Ok((PV::Double(f), "double")),
            Err(_) => Err(format!("Invalid double constant [{}].", text)),
        };
    }
    let mut t = text;
    let mut long = false;
    if t.ends_with('l') || t.ends_with('L') {
        long = true;
        t = &t[..t.len() - 1];
    }
    let mut base = 10;
    if t.starts_with("0x") || t.starts_with("0X") {
        base = 16;
        t = &t[2..];
    } else if t.len() > 1 && t.starts_with('0') {
        base = 8;
        t = &t[1..];
    }
    let signed = if neg { format!("-{}", t) } else { t.to_string() };
    if long {
        if let Ok(n) = i64::from_str_radix(&signed, base) {
            return Ok((PV::Long(n), "long"));
        }
        if base != 10 {
            if let Ok(u) = u64::from_str_radix(t, base) {
                let n = u as i64;
                return Ok((PV::Long(if neg { n.wrapping_neg() } else { n }), "long"));
            }
        }
        return Err(format!("Invalid long constant [{}].", text));
    }
    if let Ok(n) = i32::from_str_radix(&signed, base) {
        return Ok((PV::Int(n), "int"));
    }
    if base != 10 {
        if let Ok(u) = u32::from_str_radix(t, base) {
            let n = u as i32;
            return Ok((PV::Int(if neg { n.wrapping_neg() } else { n }), "int"));
        }
    }
    Err(format!("Invalid int constant [{}].", text))
}

// ---- string conversion ----

pub fn fmt_double(f: f64) -> String {
    if f.is_nan() {
        return "NaN".into();
    }
    if f.is_infinite() {
        return if f > 0.0 { "Infinity".into() } else { "-Infinity".into() };
    }
    if f == 0.0 {
        return if f.is_sign_negative() { "-0.0".into() } else { "0.0".into() };
    }
    let mut e = format!("{:e}", f);
    let (m, _) = e.split_once('e').unwrap();
    if !m.contains('.') {
        let two = format!("{:.1e}", f);
        if !two.contains(".0e") {
            e = two;
        }
    }
    fmt_shortest(&e, f.abs())
}

pub fn fmt_float(f: f32) -> String {
    if f.is_nan() {
        return "NaN".into();
    }
    if f.is_infinite() {
        return if f > 0.0 { "Infinity".into() } else { "-Infinity".into() };
    }
    if f == 0.0 {
        return if f.is_sign_negative() { "-0.0".into() } else { "0.0".into() };
    }
    let mut e = format!("{:e}", f);
    let (m, _) = e.split_once('e').unwrap();
    if !m.contains('.') {
        let two = format!("{:.1e}", f);
        if !two.contains(".0e") {
            e = two;
        }
    }
    fmt_shortest(&e, f.abs() as f64)
}

/// Renders shortest digits ("d.ddde±x") the way Double.toString does.
fn fmt_shortest(e: &str, abs: f64) -> String {
    let (neg, e) = match e.strip_prefix('-') {
        Some(r) => (true, r),
        None => (false, e),
    };
    let (mant, exp_s) = e.split_once('e').unwrap();
    let exp: i32 = exp_s.parse().unwrap_or(0);
    let digits: String = mant.replace('.', "");
    let out = if (1e-3..1e7).contains(&abs) {
        if exp >= 0 {
            let p = (exp + 1) as usize;
            if digits.len() <= p {
                format!("{}{}.0", digits, "0".repeat(p - digits.len()))
            } else {
                format!("{}.{}", &digits[..p], &digits[p..])
            }
        } else {
            format!("0.{}{}", "0".repeat((-exp - 1) as usize), digits)
        }
    } else {
        let rest = if digits.len() > 1 { &digits[1..] } else { "0" };
        format!("{}.{}E{}", &digits[..1], rest, exp)
    };
    if neg {
        format!("-{}", out)
    } else {
        out
    }
}

fn join_items(items: &[PV]) -> String {
    let parts: Vec<String> = items.iter().map(jstr).collect();
    format!("[{}]", parts.join(", "))
}

/// Java toString() of a value.
pub fn jstr(v: &PV) -> String {
    match v {
        PV::Null => "null".into(),
        PV::Str(s) => s.to_string(),
        PV::Bool(b) => b.to_string(),
        PV::Int(i) => i.to_string(),
        PV::Long(i) => i.to_string(),
        PV::Short(i) => i.to_string(),
        PV::Byte(i) => i.to_string(),
        PV::Char(c) => from_utf16(&[*c]),
        PV::Double(f) => fmt_double(*f),
        PV::Float(f) => fmt_float(*f),
        PV::List(l) => join_items(&l.borrow()),
        PV::EntrySet(l) => join_items(&l.borrow()),
        PV::DocValues(d) => join_items(&d.vals),
        PV::Set(s) => join_items(&s.borrow().keys()),
        PV::Map(m) => {
            let parts: Vec<String> = m
                .borrow()
                .ordered()
                .iter()
                .map(|(k, v)| format!("{}={}", jstr(k), jstr(v)))
                .collect();
            format!("{{{}}}", parts.join(", "))
        }
        PV::Entry(_, k, v) => format!("{}={}", jstr(k), jstr(v)),
        PV::SB(s) => s.borrow().iter().collect(),
        PV::Array(a) => format!("[{}@{:x}", array_code(&a.elem), hash_code(v) as u32),
        PV::Pattern(p) => p.src.clone(),
        PV::Optional(o) => match &o.v {
            None => format!("{}.empty", o.name),
            Some(x) => format!("{}[{}]", o.name, jstr(x)),
        },
        PV::GeoPoint(lat, lon) => format!("{}, {}", fmt_double(*lat), fmt_double(*lon)),
        PV::Exception(e) => match &e.msg {
            None => format!("java.lang.{}", e.class),
            Some(m) => format!("java.lang.{}: {}", e.class, m),
        },
        PV::StaticRef(s) => s.to_string(),
        PV::Zdt(z) => z.to_string_java(),
        PV::LocalDate(d) => crate::pl_time::fmt_local_date(*d),
        PV::Instant(ms, nanos) => crate::pl_time::fmt_instant(*ms, *nanos),
        PV::Duration(d) => crate::pl_time::fmt_duration(*d),
        PV::DayOfWeek(d) => crate::pl_time::DAY_NAMES[(*d as usize).clamp(1, 7) - 1].to_string(),
        PV::ChronoUnit(u) => {
            let mut s = u[..1].to_string();
            s.push_str(&u[1..].to_lowercase());
            s
        }
        PV::ZoneId(z) => z.to_string(),
        PV::Dtf(f) => f.java_string(),
        PV::StrEnum(s) => s.to_string(),
        PV::Iter(_) => "java.util.ArrayList$Itr".into(),
        PV::Matcher(m) => format!("java.util.regex.Matcher[pattern={}]", m.borrow().p.src),
        PV::Stream(_) => "java.util.stream.ReferencePipeline".into(),
        PV::Collector(_) => "java.util.stream.Collectors$CollectorImpl".into(),
        PV::Fn(_) => "java.util.function.Function".into(),
        PV::DocMap(_) => "org.opensearch.search.lookup.LeafDocLookup".into(),
    }
}

pub fn array_code(elem: &str) -> String {
    match elem {
        "int" => "I".into(),
        "long" => "J".into(),
        "double" => "D".into(),
        "float" => "F".into(),
        "boolean" => "Z".into(),
        "byte" => "B".into(),
        "short" => "S".into(),
        "char" => "C".into(),
        _ => format!("L{};", canon(elem)),
    }
}

// ---- type names ----

pub fn canon_name(t: &str) -> Option<&'static str> {
    Some(match t {
        "String" => "java.lang.String",
        "Object" => "java.lang.Object",
        "Integer" => "java.lang.Integer",
        "Long" => "java.lang.Long",
        "Double" => "java.lang.Double",
        "Float" => "java.lang.Float",
        "Short" => "java.lang.Short",
        "Byte" => "java.lang.Byte",
        "Character" => "java.lang.Character",
        "Boolean" => "java.lang.Boolean",
        "Number" => "java.lang.Number",
        "Math" => "java.lang.Math",
        "StrictMath" => "java.lang.StrictMath",
        "CharSequence" => "java.lang.CharSequence",
        "StringBuilder" => "java.lang.StringBuilder",
        "Iterable" => "java.lang.Iterable",
        "List" => "java.util.List",
        "ArrayList" => "java.util.ArrayList",
        "LinkedList" => "java.util.LinkedList",
        "Map" => "java.util.Map",
        "HashMap" => "java.util.HashMap",
        "TreeMap" => "java.util.TreeMap",
        "LinkedHashMap" => "java.util.LinkedHashMap",
        "Set" => "java.util.Set",
        "HashSet" => "java.util.HashSet",
        "TreeSet" => "java.util.TreeSet",
        "LinkedHashSet" => "java.util.LinkedHashSet",
        "Collection" => "java.util.Collection",
        "Collections" => "java.util.Collections",
        "Collectors" => "java.util.stream.Collectors",
        "Comparator" => "java.util.Comparator",
        "Iterator" => "java.util.Iterator",
        "Arrays" => "java.util.Arrays",
        "Objects" => "java.util.Objects",
        "Optional" => "java.util.Optional",
        "OptionalInt" => "java.util.OptionalInt",
        "OptionalLong" => "java.util.OptionalLong",
        "OptionalDouble" => "java.util.OptionalDouble",
        "Stream" => "java.util.stream.Stream",
        "IntStream" => "java.util.stream.IntStream",
        "LongStream" => "java.util.stream.LongStream",
        "DoubleStream" => "java.util.stream.DoubleStream",
        "Pattern" => "java.util.regex.Pattern",
        "Matcher" => "java.util.regex.Matcher",
        "Instant" => "java.time.Instant",
        "ZonedDateTime" => "java.time.ZonedDateTime",
        "LocalDate" => "java.time.LocalDate",
        "LocalDateTime" => "java.time.LocalDateTime",
        "LocalTime" => "java.time.LocalTime",
        "ZoneId" => "java.time.ZoneId",
        "ZoneOffset" => "java.time.ZoneOffset",
        "Duration" => "java.time.Duration",
        "Period" => "java.time.Period",
        "DayOfWeek" => "java.time.DayOfWeek",
        "Month" => "java.time.Month",
        "ChronoUnit" => "java.time.temporal.ChronoUnit",
        "DateTimeFormatter" => "java.time.format.DateTimeFormatter",
        "Base64" => "java.util.Base64",
        "Exception" => "java.lang.Exception",
        "RuntimeException" => "java.lang.RuntimeException",
        "IllegalArgumentException" => "java.lang.IllegalArgumentException",
        "IllegalStateException" => "java.lang.IllegalStateException",
        "ArithmeticException" => "java.lang.ArithmeticException",
        "NullPointerException" => "java.lang.NullPointerException",
        "UnsupportedOperationException" => "java.lang.UnsupportedOperationException",
        "Debug" => "org.opensearch.painless.api.Debug",
        _ => return None,
    })
}

/// The canonical (error message) name of a static type.
pub fn canon(t: &str) -> String {
    if let Some(base) = t.strip_suffix("[]") {
        return format!("{}[]", canon(base));
    }
    canon_name(t).map(|s| s.to_string()).unwrap_or_else(|| t.to_string())
}

pub fn box_of(p: &str) -> &'static str {
    match p {
        "int" => "Integer",
        "long" => "Long",
        "double" => "Double",
        "float" => "Float",
        "short" => "Short",
        "byte" => "Byte",
        "char" => "Character",
        "boolean" => "Boolean",
        _ => "",
    }
}

pub fn unbox(t: &str) -> &str {
    match t {
        "Integer" => "int",
        "Long" => "long",
        "Double" => "double",
        "Float" => "float",
        "Short" => "short",
        "Byte" => "byte",
        "Character" => "char",
        "Boolean" => "boolean",
        _ => t,
    }
}

pub fn is_primitive(t: &str) -> bool {
    matches!(t, "boolean" | "byte" | "short" | "char" | "int" | "long" | "float" | "double")
}

pub fn is_numeric_type(t: &str) -> bool {
    matches!(t, "byte" | "short" | "char" | "int" | "long" | "float" | "double")
}

/// The painless and java class names of a value.
pub fn runtime_class(v: &PV) -> (String, String) {
    let same = |s: &str| (s.to_string(), s.to_string());
    match v {
        PV::Int(_) => same("java.lang.Integer"),
        PV::Long(_) => same("java.lang.Long"),
        PV::Float(_) => same("java.lang.Float"),
        PV::Double(_) => same("java.lang.Double"),
        PV::Short(_) => same("java.lang.Short"),
        PV::Byte(_) => same("java.lang.Byte"),
        PV::Char(_) => same("java.lang.Character"),
        PV::Bool(_) => same("java.lang.Boolean"),
        PV::Str(_) => same("java.lang.String"),
        PV::List(_) => same("java.util.ArrayList"),
        PV::Map(m) => match m.borrow().kind {
            MapKind::Linked => same("java.util.LinkedHashMap"),
            MapKind::Tree => same("java.util.TreeMap"),
            MapKind::Hash => same("java.util.HashMap"),
        },
        PV::Set(_) => same("java.util.HashSet"),
        PV::EntrySet(_) => same("java.util.HashMap$EntrySet"),
        PV::DocValues(d) => (
            format!("org.opensearch.index.fielddata.ScriptDocValues.{}", d.kind),
            format!("org.opensearch.index.fielddata.ScriptDocValues${}", d.kind),
        ),
        PV::DocMap(_) => same("org.opensearch.search.lookup.LeafDocLookup"),
        PV::SB(_) => same("java.lang.StringBuilder"),
        PV::Pattern(_) => same("java.util.regex.Pattern"),
        PV::Matcher(_) => same("java.util.regex.Matcher"),
        PV::Array(a) => same(&format!("{}[]", canon(&a.elem))),
        PV::Zdt(_) => same("java.time.ZonedDateTime"),
        PV::LocalDate(_) => same("java.time.LocalDate"),
        PV::Instant(..) => same("java.time.Instant"),
        PV::Duration(_) => same("java.time.Duration"),
        PV::DayOfWeek(_) => same("java.time.DayOfWeek"),
        PV::ChronoUnit(_) => same("java.time.temporal.ChronoUnit"),
        PV::ZoneId(_) => same("java.time.ZoneId"),
        PV::Dtf(_) => same("java.time.format.DateTimeFormatter"),
        PV::StrEnum(_) => same("java.time.Month"),
        PV::GeoPoint(..) => same("org.opensearch.common.geo.GeoPoint"),
        PV::Entry(..) => ("java.util.Map$Entry".into(), "java.util.HashMap$Node".into()),
        PV::Stream(_) => ("java.util.stream.Stream".into(), "java.util.stream.ReferencePipeline".into()),
        PV::Optional(o) => same(&format!("java.util.{}", o.name)),
        PV::Fn(_) => same("java.util.function.Function"),
        PV::Exception(e) => same(&format!("java.lang.{}", e.class)),
        PV::Iter(_) => same("java.util.Iterator"),
        PV::Collector(_) => same("java.util.stream.Collector"),
        _ => same("java.lang.Object"),
    }
}

pub fn class_name(v: &PV) -> String {
    runtime_class(v).0
}

/// Names a def value in cast errors: boxed primitives by their primitive
/// name ("def [long]").
pub fn def_type_name(v: &PV) -> String {
    let p = prim_name(v);
    if !p.is_empty() {
        return p.to_string();
    }
    class_name(v)
}

// ---- JSON conversion ----

/// Converts JSON to Painless values; objects become maps of the given kind.
pub fn from_json(v: &Value, kind: MapKind) -> PV {
    match v {
        Value::Null => PV::Null,
        Value::Bool(b) => PV::Bool(*b),
        Value::String(s) => PV::str(s),
        Value::Number(n) => {
            let s = n.to_string();
            if !s.contains(['.', 'e', 'E']) {
                if let Ok(i) = s.parse::<i64>() {
                    if i >= i32::MIN as i64 && i <= i32::MAX as i64 {
                        return PV::Int(i as i32);
                    }
                    return PV::Long(i);
                }
            }
            PV::Double(s.parse::<f64>().unwrap_or(0.0))
        }
        Value::Array(a) => PV::list(a.iter().map(|e| from_json(e, kind)).collect()),
        Value::Object(o) => {
            let mut m = PMap::new(kind);
            let mut keys: Vec<&String> = o.keys().collect();
            if kind == MapKind::Hash {
                keys.sort();
            }
            for k in keys {
                m.put(PV::str(k), from_json(&o[k.as_str()], kind));
            }
            PV::map(m)
        }
    }
}

pub fn json_num(text: &str) -> Value {
    serde_json::from_str::<Value>(text).unwrap_or(Value::Null)
}

fn json_double(f: f64) -> Value {
    if f.is_finite() {
        serde_json::Number::from_f64(f).map(Value::Number).unwrap_or(Value::Null)
    } else {
        Value::String(fmt_double(f))
    }
}

/// Converts a Painless value back to JSON for responses and documents.
pub fn to_json(v: &PV) -> Value {
    match v {
        PV::Null => Value::Null,
        PV::Bool(b) => Value::Bool(*b),
        PV::Str(s) => Value::String(s.to_string()),
        PV::Int(i) => Value::from(*i),
        PV::Long(i) => Value::from(*i),
        PV::Short(i) => Value::from(*i),
        PV::Byte(i) => Value::from(*i),
        PV::Char(_) => Value::String(jstr(v)),
        PV::Double(f) => json_double(*f),
        PV::Float(f) => {
            if f.is_finite() {
                json_num(&fmt_float(*f))
            } else {
                Value::String(fmt_float(*f))
            }
        }
        PV::List(l) => Value::Array(l.borrow().iter().map(to_json).collect()),
        PV::EntrySet(l) => Value::Array(l.borrow().iter().map(to_json).collect()),
        PV::DocValues(d) => Value::Array(d.vals.iter().map(to_json).collect()),
        PV::Set(s) => Value::Array(s.borrow().keys().iter().map(to_json).collect()),
        PV::Array(a) => Value::Array(a.a.borrow().iter().map(to_json).collect()),
        PV::Map(m) => {
            let mut out = serde_json::Map::new();
            for (k, v) in m.borrow().ordered() {
                out.insert(jstr(&k), to_json(&v));
            }
            Value::Object(out)
        }
        _ => Value::String(jstr(v)),
    }
}
