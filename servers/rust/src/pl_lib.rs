// Painless engine: the class library (whitelisted methods and fields).

use std::cell::{Cell, RefCell};
use std::rc::Rc;

use crate::pl_interp::{exception_type, native, Interp, PFn};
use crate::pl_regex::{compile_pattern, java_replacement, PMatcher};
use crate::pl_time::{call_static_time, static_time_enum, time_method};
use crate::pl_value::*;

fn str_arg(v: &PV) -> String {
    match v {
        PV::Str(s) => s.to_string(),
        other => jstr(other),
    }
}

pub fn dyn_method_err(recv: &PV, name: &str, n: usize, off: usize) -> RtErr {
    iae(off, &format!("dynamic method [{}, {}/{}] not found", class_name(recv), name, n))
}

fn oob(i: i64, n: usize, off: usize, frames: &[&str]) -> RtErr {
    RtErr::new("index_out_of_bounds_exception", &format!("Index {} out of bounds for length {}", i, n), off).frames(frames)
}

const ARRAYLIST_GET_FRAMES: &[&str] = &[
    "java.base/jdk.internal.util.Preconditions.outOfBounds(Preconditions.java:100)",
    "java.base/jdk.internal.util.Preconditions.outOfBoundsCheckIndex(Preconditions.java:106)",
    "java.base/jdk.internal.util.Preconditions.checkIndex(Preconditions.java:302)",
    "java.base/java.util.Objects.checkIndex(Objects.java:365)",
    "java.base/java.util.ArrayList.get(ArrayList.java:428)",
];

// ---- random numbers (xorshift, seeded from the clock) ----

thread_local! {
    static RNG: Cell<u64> = Cell::new({
        let t = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos() as u64)
            .unwrap_or(0x9E3779B97F4A7C15);
        t | 1
    });
}

fn next_u64() -> u64 {
    RNG.with(|r| {
        let mut x = r.get();
        x ^= x << 13;
        x ^= x >> 7;
        x ^= x << 17;
        r.set(x);
        x
    })
}

fn random_f64() -> f64 {
    (next_u64() >> 11) as f64 / (1u64 << 53) as f64
}

// ---- fields / indexing ----

pub fn get_field(inp: &Interp<'_>, obj: &PV, name: &str, off: usize) -> R<PV> {
    match obj {
        PV::Map(m) => return Ok(m.borrow().get(&PV::str(name)).unwrap_or(PV::Null)),
        PV::DocMap(d) => return (d.get)(name, off),
        PV::Array(a) if name == "length" => return Ok(PV::Int(a.a.borrow().len() as i32)),
        PV::List(l) => {
            match name {
                "length" => return Ok(PV::Int(l.borrow().len() as i32)),
                "empty" => return Ok(PV::Bool(l.borrow().is_empty())),
                _ => {}
            }
            if let Ok(i) = name.parse::<i32>() {
                return index_get(obj, &PV::Int(i), off);
            }
        }
        PV::DocValues(d) => match name {
            "value" => return doc_value(d, off),
            "length" => return Ok(PV::Int(d.vals.len() as i32)),
            "empty" => return Ok(PV::Bool(d.vals.is_empty())),
            "lat" | "lon" if d.kind == "GeoPoints" => {
                if let PV::GeoPoint(lat, lon) = doc_value(d, off)? {
                    return Ok(PV::Double(if name == "lat" { lat } else { lon }));
                }
            }
            _ => {}
        },
        PV::GeoPoint(lat, lon) => match name {
            "lat" => return Ok(PV::Double(*lat)),
            "lon" => return Ok(PV::Double(*lon)),
            _ => {}
        },
        PV::Entry(_, k, v) => match name {
            "key" => return Ok((**k).clone()),
            "value" => return Ok((**v).clone()),
            _ => {}
        },
        _ => {}
    }
    // getter shortcut: x.foo -> x.getFoo() / x.isFoo()
    if !name.is_empty() {
        let cap = name[..1].to_uppercase() + &name[1..];
        for m in [format!("get{}", cap), format!("is{}", cap)] {
            match call_method(inp, obj, &m, vec![], off) {
                Ok(v) => return Ok(v),
                Err(e) => {
                    if !e.reason.as_deref().unwrap_or("").starts_with("dynamic method [") {
                        return Err(e);
                    }
                }
            }
        }
    }
    Err(iae(off, &format!("dynamic getter [{}, {}] not found", class_name(obj), name)))
}

pub fn set_field(obj: &PV, name: &str, v: PV, off: usize) -> R<()> {
    match obj {
        PV::Map(m) => {
            m.borrow_mut().put(PV::str(name), v);
            return Ok(());
        }
        PV::List(_) => {
            if let Ok(i) = name.parse::<i32>() {
                return index_set(obj, &PV::Int(i), v, off);
            }
        }
        _ => {}
    }
    Err(iae(off, &format!("dynamic setter [{}, {}] not found", class_name(obj), name)))
}

fn list_index(idx: &PV, n: usize, off: usize) -> R<i64> {
    if !is_number(idx) {
        return Err(cce(off, &format!("Cannot cast {} to java.lang.Number", class_name(idx))));
    }
    let mut i = i64_of(idx);
    if i < 0 {
        i += n as i64;
    }
    Ok(i)
}

fn array_oob(idx: &PV, n: usize, off: usize) -> RtErr {
    RtErr::new(
        "array_index_out_of_bounds_exception",
        &format!("Index {} out of bounds for length {}", i64_of(idx), n),
        off,
    )
}

pub fn index_get(obj: &PV, idx: &PV, off: usize) -> R<PV> {
    match obj {
        PV::Null => Err(npe(off)),
        PV::List(l) => {
            let l = l.borrow();
            let i = list_index(idx, l.len(), off)?;
            if i < 0 || i as usize >= l.len() {
                return Err(oob(i64_of(idx), l.len(), off, ARRAYLIST_GET_FRAMES));
            }
            Ok(l[i as usize].clone())
        }
        PV::Array(a) => {
            let v = a.a.borrow();
            let i = list_index(idx, v.len(), off)?;
            if i < 0 || i as usize >= v.len() {
                return Err(array_oob(idx, v.len(), off));
            }
            Ok(v[i as usize].clone())
        }
        PV::Map(m) => Ok(m.borrow().get(idx).unwrap_or(PV::Null)),
        PV::DocMap(d) => (d.get)(&str_arg(idx), off + 1),
        PV::DocValues(d) => {
            let i = list_index(idx, d.vals.len(), off)?;
            doc_get(d, i, off)
        }
        _ => Err(iae(
            off,
            &format!("Attempting to address a non-array-like type [{}] as an array.", class_name(obj)),
        )),
    }
}

pub fn index_set(obj: &PV, idx: &PV, v: PV, off: usize) -> R<()> {
    match obj {
        PV::Null => Err(npe(off)),
        PV::List(l) => {
            let mut l = l.borrow_mut();
            let i = list_index(idx, l.len(), off)?;
            if i < 0 || i as usize >= l.len() {
                return Err(oob(i64_of(idx), l.len(), off, &[]));
            }
            l[i as usize] = v;
            Ok(())
        }
        PV::Array(a) => {
            let mut arr = a.a.borrow_mut();
            let i = list_index(idx, arr.len(), off)?;
            if i < 0 || i as usize >= arr.len() {
                return Err(array_oob(idx, arr.len(), off));
            }
            arr[i as usize] = v;
            Ok(())
        }
        PV::Map(m) => {
            m.borrow_mut().put(idx.clone(), v);
            Ok(())
        }
        _ => Err(iae(
            off,
            &format!("Attempting to address a non-array-like type [{}] as an array.", class_name(obj)),
        )),
    }
}

pub fn iter_items(v: &PV, off: usize) -> R<Vec<PV>> {
    match v {
        PV::Null => Err(npe(off)),
        PV::Iter(it) => {
            let it = it.borrow();
            Ok(it.0[it.1.min(it.0.len())..].to_vec())
        }
        PV::Map(_) => Err(iae(off, &format!("Cannot iterate over [{}]", class_name(v)))),
        _ => to_items(v).ok_or_else(|| iae(off, &format!("Cannot iterate over [{}]", class_name(v)))),
    }
}

/// The elements of any collection-like value.
pub fn to_items(v: &PV) -> Option<Vec<PV>> {
    match v {
        PV::List(l) => Some(l.borrow().clone()),
        PV::EntrySet(l) => Some(l.borrow().clone()),
        PV::DocValues(d) => Some(d.vals.clone()),
        PV::Set(s) => Some(s.borrow().keys()),
        PV::Array(a) => Some(a.a.borrow().clone()),
        PV::Stream(s) => Some(s.items.clone()),
        _ => None,
    }
}

// ---- doc values ----

fn doc_missing(d: &PDocValues, off: usize) -> RtErr {
    let f1 = format!("org.opensearch.index.fielddata.ScriptDocValues${}.get(ScriptDocValues.java:145)", d.kind);
    let f2 = format!("org.opensearch.index.fielddata.ScriptDocValues${}.getValue(ScriptDocValues.java:139)", d.kind);
    let mut e = RtErr::new(
        "illegal_state_exception",
        "A document doesn't have a value for a field! Use doc[<field>].size()==0 to check if a document is missing a field!",
        off,
    );
    e.frames = vec![f1, f2];
    e
}

pub fn doc_value(d: &PDocValues, off: usize) -> R<PV> {
    d.vals.first().cloned().ok_or_else(|| doc_missing(d, off))
}

fn doc_get(d: &PDocValues, i: i64, off: usize) -> R<PV> {
    if d.vals.is_empty() {
        return Err(doc_missing(d, off));
    }
    if i < 0 || i as usize >= d.vals.len() {
        return Err(oob(i, d.vals.len(), off, &[]));
    }
    Ok(d.vals[i as usize].clone())
}

// ---- construction ----

pub fn new_object(typ: &str, args: Vec<PV>, off: usize) -> R<PV> {
    match typ {
        "ArrayList" | "LinkedList" | "ArrayDeque" => {
            let items = if args.len() == 1 { to_items(&args[0]).unwrap_or_default() } else { vec![] };
            Ok(PV::list(items))
        }
        "HashMap" | "TreeMap" | "LinkedHashMap" => {
            let kind = match typ {
                "TreeMap" => MapKind::Tree,
                "LinkedHashMap" => MapKind::Linked,
                _ => MapKind::Hash,
            };
            let mut m = PMap::new(kind);
            if let Some(PV::Map(src)) = args.first() {
                for (k, v) in src.borrow().ordered() {
                    m.put(k, v);
                }
            }
            Ok(PV::map(m))
        }
        "HashSet" | "TreeSet" | "LinkedHashSet" => {
            let kind = match typ {
                "TreeSet" => MapKind::Tree,
                "LinkedHashSet" => MapKind::Linked,
                _ => MapKind::Hash,
            };
            let items = if args.len() == 1 { to_items(&args[0]).unwrap_or_default() } else { vec![] };
            Ok(new_set_from(kind, items))
        }
        "StringBuilder" => {
            let s: Vec<char> = match args.first() {
                Some(PV::Str(s)) => s.chars().collect(),
                _ => vec![],
            };
            Ok(PV::SB(Rc::new(RefCell::new(s))))
        }
        "Exception" | "RuntimeException" | "IllegalArgumentException" | "IllegalStateException"
        | "ArithmeticException" | "NullPointerException" | "UnsupportedOperationException"
        | "IndexOutOfBoundsException" | "NumberFormatException" | "ClassCastException" => {
            let msg = match args.first() {
                Some(a) if !a.is_null() => Some(jstr(a)),
                _ => None,
            };
            Ok(PV::Exception(Rc::new(PException { typ: exception_type(typ), msg, class: typ.to_string() })))
        }
        "Object" => Ok(PV::map(PMap::new(MapKind::Hash))),
        "Random" => Ok(PV::StaticRef(Rc::from("Random"))),
        _ => Err(iae(off, &format!("constructor [[{}], <init>/{}] not found", canon(typ), args.len()))),
    }
}

pub fn static_enum(class: &str, name: &str, off: usize) -> R<PV> {
    static_time_enum(class, name)
        .ok_or_else(|| iae(off, &format!("static field [{}, {}] not found", canon(class), name)))
}

// ---- comparison helpers ----

pub fn compare_with(inp: &Interp<'_>, cmp: &PV, a: &PV, b: &PV, off: usize) -> R<i32> {
    if cmp.is_null() {
        return match compare(a, b) {
            Some(c) => Ok(c),
            None => {
                if a.is_null() || b.is_null() {
                    return Err(npe(off));
                }
                Err(cce(off, &format!("class {} cannot be cast to class java.lang.Comparable", class_name(a))))
            }
        };
    }
    let r = inp.call_fn(cmp, vec![a.clone(), b.clone()], off)?;
    Ok(i64_of(&r) as i32)
}

/// Stable merge sort with a fallible comparator.
pub fn sort_items(inp: &Interp<'_>, items: &mut Vec<PV>, cmp: &PV, off: usize) -> R<()> {
    if items.len() < 2 {
        return Ok(());
    }
    let mid = items.len() / 2;
    let mut right = items.split_off(mid);
    sort_items(inp, items, cmp, off)?;
    sort_items(inp, &mut right, cmp, off)?;
    let left = std::mem::take(items);
    let (mut i, mut j) = (0, 0);
    while i < left.len() && j < right.len() {
        if compare_with(inp, cmp, &right[j], &left[i], off)? < 0 {
            items.push(right[j].clone());
            j += 1;
        } else {
            items.push(left[i].clone());
            i += 1;
        }
    }
    items.extend_from_slice(&left[i..]);
    items.extend_from_slice(&right[j..]);
    Ok(())
}

fn comparator_natural() -> PV {
    native(|inp, a, off| Ok(PV::Int(compare_with(inp, &PV::Null, &a[0], &a[1], off)?)))
}

fn comparator_reverse(base: PV) -> PV {
    native(move |inp, a, off| Ok(PV::Int(compare_with(inp, &base, &a[1], &a[0], off)?)))
}

fn comparator_comparing(key: PV, base: PV) -> PV {
    native(move |inp, a, off| {
        let ka = inp.call_fn(&key, vec![a[0].clone()], off)?;
        let kb = inp.call_fn(&key, vec![a[1].clone()], off)?;
        Ok(PV::Int(compare_with(inp, &base, &ka, &kb, off)?))
    })
}

// ---- static methods ----

fn to_radix(v: i128, radix: u32) -> String {
    if v == 0 {
        return "0".into();
    }
    let neg = v < 0;
    let mut u = v.unsigned_abs();
    let mut digits = Vec::new();
    while u > 0 {
        digits.push(std::char::from_digit((u % radix as u128) as u32, radix).unwrap());
        u /= radix as u128;
    }
    if neg {
        digits.push('-');
    }
    digits.iter().rev().collect()
}

fn parse_int_str(s: &str, radix: u32, bits: u32, off: usize) -> R<i64> {
    let fail = || {
        let mut msg = format!("For input string: \"{}\"", s);
        if radix != 10 {
            msg.push_str(&format!(" under radix {}", radix));
        }
        RtErr::new("number_format_exception", &msg, off)
    };
    if !(2..=36).contains(&radix) {
        return Err(fail());
    }
    let v = i64::from_str_radix(s, radix).map_err(|_| fail())?;
    let (lo, hi) = match bits {
        8 => (i8::MIN as i64, i8::MAX as i64),
        16 => (i16::MIN as i64, i16::MAX as i64),
        32 => (i32::MIN as i64, i32::MAX as i64),
        _ => (i64::MIN, i64::MAX),
    };
    if v < lo || v > hi {
        return Err(fail());
    }
    Ok(v)
}

fn java_round(f: f64) -> i64 {
    if f.is_nan() {
        return 0;
    }
    (f + 0.5).floor() as i64
}

fn round_half_even(x: f64) -> f64 {
    let r = x.round();
    if (x - x.trunc()).abs() == 0.5 {
        2.0 * (x / 2.0).round()
    } else {
        r
    }
}

fn ulp(x: f64) -> f64 {
    let a = x.abs();
    if a.is_nan() {
        return a;
    }
    if a.is_infinite() {
        return f64::INFINITY;
    }
    if a == f64::MAX {
        return a - f64::from_bits(a.to_bits() - 1);
    }
    f64::from_bits(a.to_bits() + 1) - a
}

fn java_double_bits(f: f64) -> i64 {
    if f.is_nan() {
        0x7ff8000000000000
    } else {
        f.to_bits() as i64
    }
}

fn sign_i(c: std::cmp::Ordering) -> PV {
    PV::Int(match c {
        std::cmp::Ordering::Less => -1,
        std::cmp::Ordering::Greater => 1,
        _ => 0,
    })
}

fn char_of(v: &PV) -> Option<char> {
    char::from_u32(i64_of(v) as u32)
}

fn char_pv(c: char) -> PV {
    let mut buf = [0u16; 2];
    let u = c.encode_utf16(&mut buf);
    PV::Char(u[0])
}

pub fn call_static(
    inp: &Interp<'_>,
    class: &str,
    name: &str,
    args: Vec<PV>,
    off: usize,
    first_arg_pos: Option<usize>,
) -> R<PV> {
    let n = args.len();
    let a = |i: usize| args[i].clone();
    let f = |i: usize| f64_of(&args[i]);
    let s = |i: usize| str_arg(&args[i]);
    let key = format!("{}/{}", name, n);
    match class {
        "Math" | "StrictMath" => {
            let r = match name {
                "abs" => f(0).abs(),
                "acos" => f(0).acos(),
                "asin" => f(0).asin(),
                "atan" => f(0).atan(),
                "cbrt" => f(0).cbrt(),
                "ceil" => f(0).ceil(),
                "cos" => f(0).cos(),
                "cosh" => f(0).cosh(),
                "exp" => f(0).exp(),
                "expm1" => f(0).exp_m1(),
                "floor" => f(0).floor(),
                "log" => f(0).ln(),
                "log10" => f(0).log10(),
                "log1p" => f(0).ln_1p(),
                "rint" => round_half_even(f(0)),
                "signum" => {
                    let x = f(0);
                    if x == 0.0 || x.is_nan() {
                        x
                    } else {
                        1f64.copysign(x)
                    }
                }
                "sin" => f(0).sin(),
                "sinh" => f(0).sinh(),
                "sqrt" => f(0).sqrt(),
                "tan" => f(0).tan(),
                "tanh" => f(0).tanh(),
                "toDegrees" => f(0) * 180.0 / std::f64::consts::PI,
                "toRadians" => f(0) / 180.0 * std::f64::consts::PI,
                "ulp" => ulp(f(0)),
                "atan2" => f(0).atan2(f(1)),
                "hypot" => f(0).hypot(f(1)),
                "IEEEremainder" => f(0) - f(1) * round_half_even(f(0) / f(1)),
                "max" => {
                    let (x, y) = (f(0), f(1));
                    if x.is_nan() || y.is_nan() {
                        f64::NAN
                    } else if x == y {
                        if x.is_sign_negative() { y } else { x }
                    } else {
                        x.max(y)
                    }
                }
                "min" => {
                    let (x, y) = (f(0), f(1));
                    if x.is_nan() || y.is_nan() {
                        f64::NAN
                    } else if x == y {
                        if x.is_sign_negative() { x } else { y }
                    } else {
                        x.min(y)
                    }
                }
                "pow" => f(0).powf(f(1)),
                "round" => return Ok(PV::Long(java_round(f(0)))),
                "random" => random_f64(),
                _ => return Err(iae(off, &format!("static method [{}, {}] not found", canon(class), key))),
            };
            return Ok(PV::Double(r));
        }
        "Integer" | "Long" | "Short" | "Byte" => {
            let bits = match class {
                "Integer" => 32,
                "Long" => 64,
                "Short" => 16,
                _ => 8,
            };
            let prim = unbox(class);
            match name {
                "parseInt" | "parseLong" | "parseShort" | "parseByte" | "parseUnsignedInt" => {
                    let radix = if n == 2 { i64_of(&a(1)) as u32 } else { 10 };
                    let v = parse_int_str(&s(0), radix, bits, off)?;
                    return Ok(cast_num(&PV::Long(v), prim));
                }
                "valueOf" => {
                    if let PV::Str(st) = &args[0] {
                        let radix = if n == 2 { i64_of(&a(1)) as u32 } else { 10 };
                        let v = parse_int_str(st, radix, bits, off)?;
                        return Ok(cast_num(&PV::Long(v), prim));
                    }
                    return Ok(cast_num(&a(0), prim));
                }
                "toString" => {
                    let radix = if n == 2 { i64_of(&a(1)) as u32 } else { 10 };
                    let radix = if (2..=36).contains(&radix) { radix } else { 10 };
                    return Ok(PV::str(&to_radix(i64_of(&a(0)) as i128, radix)));
                }
                "toBinaryString" | "toHexString" | "toOctalString" => {
                    let radix = match name {
                        "toBinaryString" => 2,
                        "toHexString" => 16,
                        _ => 8,
                    };
                    let v = i64_of(&a(0));
                    let u: u128 = if bits == 32 { v as i32 as u32 as u128 } else { v as u64 as u128 };
                    return Ok(PV::str(&to_radix(u as i128, radix)));
                }
                "bitCount" => {
                    let v = i64_of(&a(0));
                    let c = if class == "Integer" { (v as i32 as u32).count_ones() } else { (v as u64).count_ones() };
                    return Ok(PV::Int(c as i32));
                }
                "compare" => return Ok(sign_i(i64_of(&a(0)).cmp(&i64_of(&a(1))))),
                "max" => return Ok(if i64_of(&a(0)) >= i64_of(&a(1)) { a(0) } else { a(1) }),
                "min" => return Ok(if i64_of(&a(0)) <= i64_of(&a(1)) { a(0) } else { a(1) }),
                "sum" => return crate::pl_interp::arith("+", &a(0), &a(1)),
                "signum" => return Ok(sign_i(i64_of(&a(0)).cmp(&0))),
                "hashCode" => return Ok(PV::Int(hash_code(&a(0)))),
                "toUnsignedLong" => return Ok(PV::Long(i64_of(&a(0)) as i32 as u32 as i64)),
                "numberOfLeadingZeros" | "numberOfTrailingZeros" | "highestOneBit" | "lowestOneBit" | "reverse"
                | "rotateLeft" | "rotateRight" => {
                    let v = i64_of(&a(0));
                    if bits == 32 {
                        let x = v as i32;
                        let r = match name {
                            "numberOfLeadingZeros" => x.leading_zeros() as i32,
                            "numberOfTrailingZeros" => x.trailing_zeros() as i32,
                            "highestOneBit" => {
                                if x == 0 { 0 } else { ((1u32 << 31) >> x.leading_zeros()) as i32 }
                            }
                            "lowestOneBit" => x & x.wrapping_neg(),
                            "reverse" => x.reverse_bits(),
                            "rotateLeft" => x.rotate_left((i64_of(&a(1)) & 31) as u32),
                            _ => x.rotate_right((i64_of(&a(1)) & 31) as u32),
                        };
                        return Ok(PV::Int(r));
                    }
                    let r = match name {
                        "numberOfLeadingZeros" => return Ok(PV::Int(v.leading_zeros() as i32)),
                        "numberOfTrailingZeros" => return Ok(PV::Int(v.trailing_zeros() as i32)),
                        "highestOneBit" => {
                            if v == 0 { 0 } else { ((1u64 << 63) >> v.leading_zeros()) as i64 }
                        }
                        "lowestOneBit" => v & v.wrapping_neg(),
                        _ => v.reverse_bits(),
                    };
                    return Ok(PV::Long(r));
                }
                "compareUnsigned" => {
                    return Ok(sign_i((i64_of(&a(0)) as i32 as u32).cmp(&(i64_of(&a(1)) as i32 as u32))))
                }
                _ => {}
            }
        }
        "Double" | "Float" => {
            let prim = unbox(class);
            match name {
                "parseDouble" | "parseFloat" | "valueOf" => {
                    if matches!(args[0], PV::Str(_)) || name != "valueOf" {
                        let st = s(0);
                        let t = st.trim();
                        let t = t.strip_suffix(['d', 'D', 'f', 'F']).unwrap_or(t);
                        // Java spells the specials NaN and Infinity only
                        let body = t.trim_start_matches(['+', '-']);
                        let special = body.eq_ignore_ascii_case("nan") || body.to_ascii_lowercase().starts_with("inf");
                        let java_special = body == "NaN" || body == "Infinity";
                        return match t.parse::<f64>() {
                            Ok(v) if !special || java_special => Ok(cast_num(&PV::Double(v), prim)),
                            _ => Err(RtErr::new("number_format_exception", &format!("For input string: \"{}\"", st), off)),
                        };
                    }
                    return Ok(cast_num(&a(0), prim));
                }
                "toString" => return Ok(PV::str(&jstr(&cast_num(&a(0), prim)))),
                "isNaN" => return Ok(PV::Bool(f(0).is_nan())),
                "isInfinite" => return Ok(PV::Bool(f(0).is_infinite())),
                "isFinite" => return Ok(PV::Bool(f(0).is_finite())),
                "compare" => {
                    let (x, y) = (f(0), f(1));
                    if x < y {
                        return Ok(PV::Int(-1));
                    }
                    if x > y {
                        return Ok(PV::Int(1));
                    }
                    if x == y && x.is_sign_negative() == y.is_sign_negative() {
                        return Ok(PV::Int(0));
                    }
                    return Ok(sign_i(java_double_bits(x).cmp(&java_double_bits(y))));
                }
                "max" | "min" => {
                    return call_static(inp, "Math", name, args, off, first_arg_pos).map(|v| cast_num(&v, prim));
                }
                "sum" => return Ok(cast_num(&PV::Double(f(0) + f(1)), prim)),
                "hashCode" => return Ok(PV::Int(hash_code(&a(0)))),
                "doubleToLongBits" => return Ok(PV::Long(java_double_bits(f(0)))),
                "longBitsToDouble" => return Ok(PV::Double(f64::from_bits(i64_of(&a(0)) as u64))),
                "floatToIntBits" => return Ok(PV::Int((f(0) as f32).to_bits() as i32)),
                "intBitsToFloat" => return Ok(PV::Float(f32::from_bits(i64_of(&a(0)) as u32))),
                _ => {}
            }
        }
        "Boolean" => match name {
            "parseBoolean" => return Ok(PV::Bool(s(0).eq_ignore_ascii_case("true"))),
            "valueOf" => {
                if let PV::Str(st) = &args[0] {
                    return Ok(PV::Bool(st.eq_ignore_ascii_case("true")));
                }
                return Ok(a(0));
            }
            "toString" => return Ok(PV::str(&jstr(&a(0)))),
            "compare" => return Ok(PV::Int(compare(&a(0), &a(1)).unwrap_or(0))),
            "logicalAnd" => return Ok(PV::Bool(a(0).as_bool() == Some(true) && a(1).as_bool() == Some(true))),
            "logicalOr" => return Ok(PV::Bool(a(0).as_bool() == Some(true) || a(1).as_bool() == Some(true))),
            "logicalXor" => return Ok(PV::Bool(a(0).as_bool() != a(1).as_bool())),
            "hashCode" => return Ok(PV::Int(hash_code(&a(0)))),
            _ => {}
        },
        "Character" => {
            let c = char_of(&a(0));
            let test = |p: fn(char) -> bool| Ok(PV::Bool(c.map(p).unwrap_or(false)));
            match name {
                "isDigit" => return test(|c| c.is_numeric() && c.to_digit(10).is_some() || c.is_ascii_digit()),
                "isLetter" => return test(|c| c.is_alphabetic()),
                "isLetterOrDigit" => return test(|c| c.is_alphanumeric()),
                "isLowerCase" => return test(|c| c.is_lowercase()),
                "isUpperCase" => return test(|c| c.is_uppercase()),
                "isWhitespace" => return test(|c| c.is_whitespace() && c != '\u{a0}' && c != '\u{2007}' && c != '\u{202f}'),
                "isSpaceChar" => return test(|c| matches!(c, ' ' | '\u{a0}' | '\u{1680}' | '\u{2000}'..='\u{200a}' | '\u{202f}' | '\u{205f}' | '\u{3000}')),
                "isAlphabetic" => return test(|c| c.is_alphabetic()),
                "toLowerCase" => {
                    return Ok(c.map(|c| char_pv(c.to_lowercase().next().unwrap_or(c))).unwrap_or(a(0)));
                }
                "toUpperCase" => {
                    return Ok(c.map(|c| char_pv(c.to_uppercase().next().unwrap_or(c))).unwrap_or(a(0)));
                }
                "getNumericValue" | "digit" => {
                    let radix = if name == "digit" { i64_of(&a(1)) as u32 } else { 36 };
                    return Ok(PV::Int(c.and_then(|c| c.to_digit(radix.clamp(2, 36))).map(|d| d as i32).unwrap_or(-1)));
                }
                "valueOf" => return Ok(cast_num(&a(0), "char")),
                "toString" => return Ok(PV::str(&jstr(&cast_num(&a(0), "char")))),
                "compare" => return Ok(PV::Int((i64_of(&a(0)) - i64_of(&a(1))) as i32)),
                "hashCode" => return Ok(PV::Int(i64_of(&a(0)) as i32)),
                "toChars" => {
                    let s: String = c.map(|c| c.to_string()).unwrap_or_default();
                    return Ok(PV::array("char", s.encode_utf16().map(PV::Char).collect()));
                }
                _ => {}
            }
        }
        "String" => match name {
            "valueOf" | "copyValueOf" => {
                if let PV::Array(arr) = &args[0] {
                    if arr.elem == "char" {
                        let u: Vec<u16> = arr.a.borrow().iter().map(|c| i64_of(c) as u16).collect();
                        return Ok(PV::str(&from_utf16(&u)));
                    }
                }
                return Ok(PV::str(&jstr(&a(0))));
            }
            "join" => {
                let items = match to_items(&a(1)) {
                    Some(i) => i,
                    None => return Err(npe(off)),
                };
                let parts: Vec<String> = items.iter().map(jstr).collect();
                return Ok(PV::str(&parts.join(&s(0))));
            }
            _ => {}
        },
        "Objects" => match name {
            "equals" | "deepEquals" => return Ok(PV::Bool(java_equals(&a(0), &a(1)))),
            "hashCode" => return Ok(PV::Int(hash_code(&a(0)))),
            "isNull" => return Ok(PV::Bool(a(0).is_null())),
            "nonNull" => return Ok(PV::Bool(!a(0).is_null())),
            "requireNonNull" => {
                if a(0).is_null() {
                    let mut e = RtErr::nil("null_pointer_exception", off)
                        .frames(&["java.base/java.util.Objects.requireNonNull(Objects.java:220)"]);
                    if n == 2 {
                        e.reason = Some(s(1));
                        e.frames = vec!["java.base/java.util.Objects.requireNonNull(Objects.java:246)".into()];
                    }
                    return Err(e);
                }
                return Ok(a(0));
            }
            "requireNonNullElse" => return Ok(if a(0).is_null() { a(1) } else { a(0) }),
            "toString" => {
                if a(0).is_null() && n == 2 {
                    return Ok(a(1));
                }
                return Ok(PV::str(&jstr(&a(0))));
            }
            "compare" => {
                if identical(&a(0), &a(1)) {
                    return Ok(PV::Int(0));
                }
                return Ok(PV::Int(compare_with(inp, &a(2), &a(0), &a(1), off)?));
            }
            _ => {}
        },
        "Collections" => match name {
            "sort" => {
                let l = match &args[0] {
                    PV::List(l) => l.clone(),
                    _ => return Err(npe(off)),
                };
                let cmp = if n == 2 { a(1) } else { PV::Null };
                let mut items = l.borrow().clone();
                sort_items(inp, &mut items, &cmp, off)?;
                *l.borrow_mut() = items;
                return Ok(PV::Null);
            }
            "reverse" => {
                if let PV::List(l) = &args[0] {
                    l.borrow_mut().reverse();
                }
                return Ok(PV::Null);
            }
            "shuffle" => {
                if let PV::List(l) = &args[0] {
                    let mut v = l.borrow_mut();
                    for i in (1..v.len()).rev() {
                        let j = (next_u64() % (i as u64 + 1)) as usize;
                        v.swap(i, j);
                    }
                }
                return Ok(PV::Null);
            }
            "max" | "min" => {
                let items = match to_items(&a(0)) {
                    Some(i) if !i.is_empty() => i,
                    _ => return Err(RtErr::nil("no_such_element_exception", off)),
                };
                let cmp = if n == 2 { a(1) } else { PV::Null };
                let mut best = items[0].clone();
                for it in &items[1..] {
                    let c = compare_with(inp, &cmp, it, &best, off)?;
                    if (name == "max" && c > 0) || (name == "min" && c < 0) {
                        best = it.clone();
                    }
                }
                return Ok(best);
            }
            "emptyList" => return Ok(PV::list(vec![])),
            "emptyMap" => return Ok(PV::map(PMap::new(MapKind::Hash))),
            "emptySet" => return Ok(PV::set(PMap::new(MapKind::Hash))),
            "singletonList" => return Ok(PV::list(vec![a(0)])),
            "singleton" => return Ok(new_set_from(MapKind::Hash, vec![a(0)])),
            "singletonMap" => {
                let mut m = PMap::new(MapKind::Hash);
                m.put(a(0), a(1));
                return Ok(PV::map(m));
            }
            "unmodifiableList" | "unmodifiableMap" | "unmodifiableSet" => return Ok(a(0)),
            "frequency" => {
                let items = to_items(&a(0)).unwrap_or_default();
                return Ok(PV::Int(items.iter().filter(|it| java_equals(it, &args[1])).count() as i32));
            }
            "swap" => {
                if let PV::List(l) = &args[0] {
                    let (i, j) = (i64_of(&a(1)) as usize, i64_of(&a(2)) as usize);
                    let mut v = l.borrow_mut();
                    if i < v.len() && j < v.len() {
                        v.swap(i, j);
                    }
                }
                return Ok(PV::Null);
            }
            "nCopies" => {
                let k = i64_of(&a(0));
                if k < 0 {
                    return Err(iae(off, &format!("List length = {}", k)));
                }
                crate::pl_interp::check_alloc(k as u64, off)?;
                return Ok(PV::list(vec![a(1); k as usize]));
            }
            "reverseOrder" => return Ok(comparator_reverse(PV::Null)),
            _ => {}
        },
        "Collectors" => {
            let mut c = PCollector {
                kind: name.to_string(),
                sep: String::new(),
                prefix: String::new(),
                suffix: String::new(),
                f1: PV::Null,
                f2: PV::Null,
            };
            if name == "joining" {
                if n >= 1 {
                    c.sep = s(0);
                }
                if n == 3 {
                    c.prefix = s(1);
                    c.suffix = s(2);
                }
            } else {
                if n >= 1 {
                    c.f1 = a(0);
                }
                if n >= 2 {
                    c.f2 = a(1);
                }
            }
            return Ok(PV::Collector(Rc::new(c)));
        }
        "Comparator" => match name {
            "naturalOrder" => return Ok(comparator_natural()),
            "reverseOrder" => return Ok(comparator_reverse(PV::Null)),
            "comparing" | "comparingInt" | "comparingLong" | "comparingDouble" => {
                let base = if n == 2 { a(1) } else { PV::Null };
                return Ok(comparator_comparing(a(0), base));
            }
            "nullsFirst" | "nullsLast" => {
                let base = a(0);
                let first = name == "nullsFirst";
                return Ok(native(move |inp, xs, o| {
                    let (x, y) = (&xs[0], &xs[1]);
                    Ok(PV::Int(match (x.is_null(), y.is_null()) {
                        (true, true) => 0,
                        (true, false) => if first { -1 } else { 1 },
                        (false, true) => if first { 1 } else { -1 },
                        _ => compare_with(inp, &base, x, y, o)?,
                    }))
                }));
            }
            _ => {}
        },
        "Arrays" => match name {
            "asList" => return Ok(PV::list(to_items(&a(0)).unwrap_or_default())),
            "toString" => {
                return Ok(PV::str(&match to_items(&a(0)) {
                    Some(items) => jstr(&PV::list(items)),
                    None => "null".into(),
                }))
            }
            "sort" => {
                if let PV::Array(arr) = &args[0] {
                    let mut items = arr.a.borrow().clone();
                    sort_items(inp, &mut items, &PV::Null, off)?;
                    *arr.a.borrow_mut() = items;
                }
                return Ok(PV::Null);
            }
            "stream" => {
                let items = to_items(&a(0)).unwrap_or_default();
                let kind = match &args[0] {
                    PV::Array(arr) => match arr.elem.as_str() {
                        "int" => "int",
                        "long" => "long",
                        "double" => "double",
                        _ => "obj",
                    },
                    _ => "obj",
                };
                return Ok(PV::stream(kind, items));
            }
            "equals" => {
                let x = to_items(&a(0)).unwrap_or_default();
                let y = to_items(&a(1)).unwrap_or_default();
                return Ok(PV::Bool(java_equals(&PV::list(x), &PV::list(y))));
            }
            "fill" => {
                if let PV::Array(arr) = &args[0] {
                    for x in arr.a.borrow_mut().iter_mut() {
                        *x = args[1].clone();
                    }
                }
                return Ok(PV::Null);
            }
            _ => {}
        },
        "Debug" if name == "explain" => {
            let (pc, jc) = runtime_class(&a(0));
            let mut e = RtErr::nil("painless_explain_error", first_arg_pos.unwrap_or(off));
            e.extra = vec![
                ("painless_class".into(), serde_json::Value::String(pc)),
                ("to_string".into(), serde_json::Value::String(jstr(&a(0)))),
                ("java_class".into(), serde_json::Value::String(jc)),
            ];
            return Err(e);
        }
        "Pattern" => match name {
            "compile" => return compile_pattern(&s(0), "", off),
            "quote" => return Ok(PV::str(&format!("\\Q{}\\E", s(0)))),
            _ => {}
        },
        "IntStream" | "LongStream" => {
            let (lo, mut hi) = (i64_of(&a(0)), i64_of(&a(1)));
            if name == "rangeClosed" {
                hi += 1;
            }
            let long = class == "LongStream";
            crate::pl_interp::check_alloc(hi.saturating_sub(lo).max(0) as u64, off)?;
            let items: Vec<PV> = (lo..hi.max(lo)).map(|i| if long { PV::Long(i) } else { PV::Int(i as i32) }).collect();
            return Ok(PV::stream(if long { "long" } else { "int" }, items));
        }
        "Stream" => {
            if name == "empty" {
                return Ok(PV::stream("obj", vec![]));
            }
            return Ok(PV::stream("obj", to_items(&a(0)).unwrap_or_default()));
        }
        "List" => return Ok(PV::list(to_items(&a(0)).unwrap_or_default())),
        "Set" => return Ok(new_set_from(MapKind::Hash, to_items(&a(0)).unwrap_or_default())),
        "Map" => {
            let mut m = PMap::new(MapKind::Hash);
            let mut i = 0;
            while i + 1 < n {
                m.put(args[i].clone(), args[i + 1].clone());
                i += 2;
            }
            return Ok(PV::map(m));
        }
        "Base64" => return Ok(PV::StaticRef(Rc::from(format!("Base64.{}", name).as_str()))),
        _ => {}
    }
    if let Some(r) = call_static_time(class, name, &args, off) {
        return r;
    }
    Err(iae(off, &format!("static method [{}, {}] not found", canon(class), key)))
}

// ---- instance methods ----

pub fn call_method(inp: &Interp<'_>, recv: &PV, name: &str, args: Vec<PV>, off: usize) -> R<PV> {
    let n = args.len();
    match recv {
        PV::Str(s) => return string_method(inp, s, name, args, off),
        PV::List(l) => {
            if let Some(r) = list_method(inp, l, name, &args, off) {
                return r;
            }
        }
        PV::DocValues(d) => {
            match (name, n) {
                ("getValue", 0) => return doc_value(d, off),
                ("get", 1) => return doc_get(d, i64_of(&args[0]), off),
                ("size", _) | ("getLength", _) => return Ok(PV::Int(d.vals.len() as i32)),
                ("isEmpty", _) => return Ok(PV::Bool(d.vals.is_empty())),
                ("contains", 1) => return Ok(PV::Bool(d.vals.iter().any(|v| equals(v, &args[0])))),
                ("stream", 0) => return Ok(PV::stream("obj", d.vals.clone())),
                ("iterator", 0) => return Ok(PV::iter(d.vals.clone())),
                ("toString", 0) => return Ok(PV::str(&jstr(recv))),
                ("getLat", 0) | ("getLon", 0) if d.kind == "GeoPoints" => {
                    if let PV::GeoPoint(lat, lon) = doc_value(d, off)? {
                        return Ok(PV::Double(if name == "getLat" { lat } else { lon }));
                    }
                }
                ("indexOf", 1) => {
                    return Ok(PV::Int(d.vals.iter().position(|v| equals(v, &args[0])).map(|i| i as i32).unwrap_or(-1)))
                }
                _ => {}
            }
            if let Some(r) = collection_augment(inp, &d.vals, name, &args, off) {
                return r;
            }
        }
        PV::Map(m) => {
            if let Some(r) = map_method(inp, m, name, &args, off) {
                return r;
            }
        }
        PV::DocMap(d) => match name {
            "containsKey" if n == 1 => return Ok(PV::Bool((d.has)(&str_arg(&args[0])))),
            "get" if n == 1 => return (d.get)(&str_arg(&args[0]), off),
            _ => {}
        },
        PV::Set(s) => {
            if let Some(r) = set_method(inp, s, name, &args, off) {
                return r;
            }
        }
        PV::EntrySet(l) => {
            if let Some(r) = list_method(inp, l, name, &args, off) {
                return r;
            }
        }
        PV::Array(a) => {
            if name == "clone" {
                return Ok(PV::array(&a.elem, a.a.borrow().clone()));
            }
        }
        PV::SB(sb) => {
            if let Some(r) = sb_method(sb, recv, name, &args, off) {
                return r;
            }
        }
        PV::Pattern(p) => match name {
            "matcher" if n == 1 => {
                return Ok(PV::Matcher(Rc::new(RefCell::new(PMatcher::new(p.clone(), &str_arg(&args[0]))))))
            }
            "pattern" | "toString" => return Ok(PV::str(&p.src)),
            "split" if n == 1 => {
                let input = str_arg(&args[0]);
                let mut parts: Vec<&str> = p.re.split(&input).collect();
                while parts.len() > 1 && parts.last() == Some(&"") {
                    parts.pop();
                }
                return Ok(PV::array("String", parts.into_iter().map(PV::str).collect()));
            }
            "asPredicate" => {
                let p = p.clone();
                return Ok(native(move |_, xs, _| Ok(PV::Bool(p.re.is_match(&jstr(&xs[0]))))));
            }
            _ => {}
        },
        PV::Matcher(m) => {
            if let Some(r) = matcher_method(m, name, &args, off) {
                return r;
            }
        }
        PV::Stream(s) => {
            if let Some(r) = stream_method(inp, s, name, &args, off) {
                return r;
            }
        }
        PV::Optional(o) => match name {
            "get" | "getAsInt" | "getAsDouble" | "getAsLong" | "orElseThrow" => {
                return o.v.clone().ok_or_else(|| RtErr::new("no_such_element_exception", "No value present", off))
            }
            "isPresent" => return Ok(PV::Bool(o.v.is_some())),
            "isEmpty" => return Ok(PV::Bool(o.v.is_none())),
            "orElse" if n == 1 => return Ok(o.v.clone().unwrap_or_else(|| args[0].clone())),
            "orElseGet" if n == 1 => {
                return match &o.v {
                    Some(v) => Ok(v.clone()),
                    None => inp.call_fn(&args[0], vec![], off),
                }
            }
            "ifPresent" if n == 1 => {
                if let Some(v) = &o.v {
                    inp.call_fn(&args[0], vec![v.clone()], off)?;
                }
                return Ok(PV::Null);
            }
            "map" if n == 1 => {
                return match &o.v {
                    None => Ok(recv.clone()),
                    Some(v) => {
                        let r = inp.call_fn(&args[0], vec![v.clone()], off)?;
                        Ok(PV::optional("Optional", if r.is_null() { None } else { Some(r) }))
                    }
                }
            }
            _ => {}
        },
        PV::Entry(m, k, v) => match name {
            "getKey" => return Ok((**k).clone()),
            "getValue" => return Ok((**v).clone()),
            "setValue" if n == 1 => {
                m.borrow_mut().put((**k).clone(), args[0].clone());
                return Ok((**v).clone());
            }
            _ => {}
        },
        PV::Iter(it) => match name {
            "hasNext" => {
                let it = it.borrow();
                return Ok(PV::Bool(it.1 < it.0.len()));
            }
            "next" => {
                let mut it = it.borrow_mut();
                if it.1 >= it.0.len() {
                    return Err(RtErr::nil("no_such_element_exception", off));
                }
                it.1 += 1;
                return Ok(it.0[it.1 - 1].clone());
            }
            _ => {}
        },
        PV::Exception(e) => match name {
            "getMessage" | "getLocalizedMessage" => {
                return Ok(e.msg.as_ref().map(|m| PV::str(m)).unwrap_or(PV::Null))
            }
            "toString" => return Ok(PV::str(&jstr(recv))),
            _ => {}
        },
        PV::Fn(f) => return fn_method(inp, f, recv, name, args, off),
        PV::StaticRef(t) => match &**t {
            "Base64.getEncoder" if name == "encodeToString" && n == 1 => {
                if let PV::Array(arr) = &args[0] {
                    let b: Vec<u8> = arr.a.borrow().iter().map(|x| i64_of(x) as u8).collect();
                    return Ok(PV::str(&base64::encode(b)));
                }
            }
            "Random" => match name {
                "nextInt" => {
                    if n == 1 {
                        let bound = i64_of(&args[0]).max(1) as u64;
                        return Ok(PV::Int((next_u64() % bound) as i32));
                    }
                    return Ok(PV::Int(next_u64() as i32));
                }
                "nextDouble" => return Ok(PV::Double(random_f64())),
                _ => {}
            },
            _ => {}
        },
        PV::GeoPoint(lat, lon) => match name {
            "getLat" => return Ok(PV::Double(*lat)),
            "getLon" => return Ok(PV::Double(*lon)),
            _ => {}
        },
        _ => {}
    }
    if let Some(r) = time_method(recv, name, &args, off) {
        return r;
    }
    match (name, n) {
        ("toString", 0) => return Ok(PV::str(&jstr(recv))),
        ("equals", 1) => return Ok(PV::Bool(java_equals(recv, &args[0]))),
        ("hashCode", 0) => return Ok(PV::Int(hash_code(recv))),
        ("compareTo", 1) => {
            if let Some(c) = compare(recv, &args[0]) {
                return Ok(PV::Int(c.signum()));
            }
        }
        _ => {}
    }
    if is_number(recv) {
        let to = match name {
            "intValue" => Some("int"),
            "longValue" => Some("long"),
            "doubleValue" => Some("double"),
            "floatValue" => Some("float"),
            "shortValue" => Some("short"),
            "byteValue" => Some("byte"),
            _ => None,
        };
        if let Some(t) = to {
            return Ok(cast_num(recv, t));
        }
        match name {
            "isNaN" => return Ok(PV::Bool(f64_of(recv).is_nan())),
            "isInfinite" => return Ok(PV::Bool(f64_of(recv).is_infinite())),
            _ => {}
        }
    }
    if let (PV::Bool(b), "booleanValue") = (recv, name) {
        return Ok(PV::Bool(*b));
    }
    if let (PV::Char(_), "charValue") = (recv, name) {
        return Ok(recv.clone());
    }
    Err(dyn_method_err(recv, name, n, off))
}

fn fn_method(inp: &Interp<'_>, f: &Rc<PFn>, recv: &PV, name: &str, args: Vec<PV>, off: usize) -> R<PV> {
    match name {
        "reversed" => return Ok(comparator_reverse(recv.clone())),
        "thenComparing" if args.len() == 1 => {
            let mut next = args[0].clone();
            if let PV::Fn(nf) = &next {
                if let PFn::Lambda { lam, .. } = &**nf {
                    if lam.pnames.len() == 1 {
                        next = comparator_comparing(next.clone(), PV::Null);
                    }
                }
            }
            let first = recv.clone();
            return Ok(native(move |inp, xs, o| {
                let c = compare_with(inp, &first, &xs[0], &xs[1], o)?;
                if c != 0 {
                    return Ok(PV::Int(c));
                }
                Ok(PV::Int(compare_with(inp, &next, &xs[0], &xs[1], o)?))
            }));
        }
        "negate" => {
            let g = recv.clone();
            return Ok(native(move |inp, xs, o| {
                let v = inp.call_fn(&g, xs, o)?;
                Ok(PV::Bool(v.as_bool() != Some(true)))
            }));
        }
        "andThen" | "compose" if args.len() == 1 => {
            let (first, second) = if name == "compose" {
                (args[0].clone(), recv.clone())
            } else {
                (recv.clone(), args[0].clone())
            };
            return Ok(native(move |inp, xs, o| {
                let v = inp.call_fn(&first, xs, o)?;
                inp.call_fn(&second, vec![v], o)
            }));
        }
        _ => {}
    }
    let _ = f;
    inp.call_fn(recv, args, off)
}

fn matcher_method(m: &Rc<RefCell<PMatcher>>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    Some(match name {
        "find" => m.borrow_mut().find(off).map(PV::Bool),
        "matches" => m.borrow_mut().matches(off).map(PV::Bool),
        "lookingAt" => Ok(PV::Bool(m.borrow_mut().looking_at())),
        "group" => {
            let mm = m.borrow();
            if n == 0 {
                return Some(mm.group(0, off));
            }
            if let PV::Str(s) = &args[0] {
                return Some(match mm.group_index(s) {
                    Some(i) => mm.group(i, off),
                    None => Err(iae(off, &format!("No group with name <{}>", s))),
                });
            }
            mm.group(i64_of(&args[0]).max(0) as usize, off)
        }
        "namedGroup" => {
            let mm = m.borrow();
            match mm.group_index(&jstr(&args[0])) {
                Some(i) => mm.group(i, off),
                None => Ok(PV::Null),
            }
        }
        "groupCount" => Ok(PV::Int(m.borrow().p.re.captures_len() as i32 - 1)),
        "start" | "end" => {
            let mm = m.borrow();
            let loc = match &mm.loc {
                None => return Some(Err(RtErr::new("illegal_state_exception", "No match available", off))),
                Some(l) => l,
            };
            let g = if n == 1 { i64_of(&args[0]).max(0) as usize } else { 0 };
            match loc.get(g).copied().flatten() {
                Some((s, e)) => {
                    let b = if name == "end" { e } else { s };
                    Ok(PV::Int(mm.input[..b].encode_utf16().count() as i32))
                }
                None => Ok(PV::Int(-1)),
            }
        }
        "replaceAll" | "replaceFirst" if n == 1 => {
            let mm = m.borrow();
            let repl = java_replacement(&jstr(&args[0]));
            let out = if name == "replaceAll" {
                mm.p.re.replace_all(&mm.input, repl.as_str()).into_owned()
            } else {
                mm.p.re.replace(&mm.input, repl.as_str()).into_owned()
            };
            Ok(PV::str(&out))
        }
        "reset" => {
            let mut mm = m.borrow_mut();
            mm.pos = 0;
            mm.loc = None;
            drop(mm);
            Ok(PV::Matcher(m.clone()))
        }
        "hitEnd" => {
            let mm = m.borrow();
            Ok(PV::Bool(mm.pos >= mm.input.len()))
        }
        _ => return None,
    })
}

fn string_method(inp: &Interp<'_>, s: &str, name: &str, args: Vec<PV>, off: usize) -> R<PV> {
    let n = args.len();
    let sa = |i: usize| str_arg(&args[i]);
    let idx = |i: usize| i64_of(&args[i]);
    let u = utf16(s);
    let len = u.len() as i64;
    let sioob = |reason: String| RtErr::new("string_index_out_of_bounds_exception", &reason, off);
    match (name, n) {
        ("length", 0) => Ok(PV::Int(u.len() as i32)),
        ("isEmpty", 0) => Ok(PV::Bool(u.is_empty())),
        ("isBlank", 0) => Ok(PV::Bool(s.trim().is_empty())),
        ("charAt", 1) => {
            let i = idx(0);
            if i < 0 || i >= len {
                return Err(sioob(format!("Index {} out of bounds for length {}", i, len)));
            }
            Ok(PV::Char(u[i as usize]))
        }
        ("codePointAt", 1) => {
            let i = idx(0);
            if i < 0 || i >= len {
                return Err(sioob(format!("Index {} out of bounds for length {}", i, len)));
            }
            let c = from_utf16(&u[i as usize..]).chars().next().map(|c| c as i32).unwrap_or(0);
            Ok(PV::Int(c))
        }
        ("substring", 1) | ("substring", 2) | ("subSequence", 2) => {
            let b = idx(0);
            let e = if n == 2 { idx(1) } else { len };
            if b < 0 || e > len || b > e {
                if n == 1 {
                    return Err(sioob(format!("begin {}, end {}, length {}", b, e, len)));
                }
                return Err(sioob(format!("Range [{}, {}) out of bounds for length {}", b, e, len)).frames(&[
                    "java.base/jdk.internal.util.Preconditions$1.apply(Preconditions.java:55)",
                    "java.base/jdk.internal.util.Preconditions$1.apply(Preconditions.java:52)",
                    "java.base/jdk.internal.util.Preconditions$4.apply(Preconditions.java:213)",
                    "java.base/jdk.internal.util.Preconditions$4.apply(Preconditions.java:210)",
                    "java.base/jdk.internal.util.Preconditions.outOfBounds(Preconditions.java:98)",
                    "java.base/jdk.internal.util.Preconditions.outOfBoundsCheckFromToIndex(Preconditions.java:112)",
                    "java.base/jdk.internal.util.Preconditions.checkFromToIndex(Preconditions.java:349)",
                    "java.base/java.lang.String.checkBoundsBeginEnd(String.java:4937)",
                    "java.base/java.lang.String.substring(String.java:2899)",
                ]));
            }
            Ok(PV::str(&from_utf16(&u[b as usize..e as usize])))
        }
        ("indexOf", 1) | ("indexOf", 2) | ("lastIndexOf", 1) | ("lastIndexOf", 2) => {
            let needle: Vec<u16> = match &args[0] {
                PV::Str(x) => utf16(x),
                PV::Char(c) => vec![*c],
                other if is_number(other) => {
                    char::from_u32(i64_of(other) as u32).map(|c| utf16(&c.to_string())).unwrap_or_default()
                }
                other => utf16(&jstr(other)),
            };
            let last = name.starts_with("last");
            let nl = needle.len() as i64;
            if !last {
                let from = if n == 2 { idx(1).max(0) } else { 0 };
                let mut i = from;
                while i + nl <= len {
                    if u[i as usize..(i + nl) as usize] == needle[..] {
                        return Ok(PV::Int(i as i32));
                    }
                    i += 1;
                }
                return Ok(PV::Int(-1));
            }
            let from = if n == 2 { idx(1) } else { len };
            let mut i = (len - nl).min(from);
            while i >= 0 {
                if u[i as usize..(i + nl) as usize] == needle[..] {
                    return Ok(PV::Int(i as i32));
                }
                i -= 1;
            }
            Ok(PV::Int(-1))
        }
        ("contains", 1) => Ok(PV::Bool(s.contains(sa(0).as_str()))),
        ("contentEquals", 1) => Ok(PV::Bool(s == sa(0))),
        ("startsWith", 1) => Ok(PV::Bool(s.starts_with(sa(0).as_str()))),
        ("startsWith", 2) => {
            let o = idx(1);
            if o < 0 || o > len {
                return Ok(PV::Bool(false));
            }
            Ok(PV::Bool(from_utf16(&u[o as usize..]).starts_with(sa(0).as_str())))
        }
        ("endsWith", 1) => Ok(PV::Bool(s.ends_with(sa(0).as_str()))),
        ("compareTo", 1) => {
            if args[0].is_null() {
                return Err(npe(off));
            }
            Ok(PV::Int(string_compare(s, &sa(0))))
        }
        ("compareToIgnoreCase", 1) => {
            let f = |x: &str| x.to_uppercase().to_lowercase();
            Ok(PV::Int(string_compare(&f(s), &f(&sa(0)))))
        }
        ("equals", 1) => Ok(PV::Bool(matches!(&args[0], PV::Str(x) if &**x == s))),
        ("equalsIgnoreCase", 1) => Ok(PV::Bool(match &args[0] {
            PV::Str(x) => x.to_lowercase() == s.to_lowercase() || x.to_uppercase() == s.to_uppercase(),
            _ => false,
        })),
        ("concat", 1) => Ok(PV::str(&format!("{}{}", s, sa(0)))),
        ("replace", 2) => {
            let (from, to) = (jstr(&args[0]), jstr(&args[1]));
            if from.is_empty() {
                let mut out = String::new();
                for c in s.chars() {
                    out.push_str(&to);
                    out.push(c);
                }
                out.push_str(&to);
                return Ok(PV::str(&out));
            }
            Ok(PV::str(&s.replace(from.as_str(), &to)))
        }
        ("trim", 0) => Ok(PV::str(s.trim_matches(|c: char| c <= ' '))),
        ("strip", 0) => Ok(PV::str(s.trim())),
        ("toUpperCase", 0) => Ok(PV::str(&s.to_uppercase())),
        ("toLowerCase", 0) => Ok(PV::str(&s.to_lowercase())),
        ("toString", 0) => Ok(PV::str(s)),
        ("hashCode", 0) => Ok(PV::Int(string_hash(s))),
        ("chars", 0) => Ok(PV::stream("int", u.iter().map(|c| PV::Int(*c as i32)).collect())),
        ("codePoints", 0) => Ok(PV::stream("int", s.chars().map(|c| PV::Int(c as i32)).collect())),
        ("toCharArray", 0) => Ok(PV::array("char", u.iter().map(|c| PV::Char(*c)).collect())),
        ("encodeBase64", 0) => Ok(PV::str(&base64::encode(s.as_bytes()))),
        ("decodeBase64", 0) => match base64::decode(s) {
            Ok(b) => Ok(PV::str(&String::from_utf8_lossy(&b))),
            Err(_) => Err(iae(off, "Illegal base64 character")),
        },
        ("splitOnToken", 1) | ("splitOnToken", 2) => {
            let tok = sa(0);
            let limit = if n == 2 { idx(1) } else { -1 };
            let parts: Vec<PV> = if limit > 0 {
                s.splitn(limit as usize, tok.as_str()).map(PV::str).collect()
            } else {
                s.split(tok.as_str()).map(PV::str).collect()
            };
            Ok(PV::array("String", parts))
        }
        ("replaceAll", 2) | ("replaceFirst", 2) => {
            let p = match &args[0] {
                PV::Pattern(p) => p.clone(),
                other => {
                    return Err(cce(off, &format!("Cannot cast {} to java.util.regex.Pattern", class_name(other))))
                }
            };
            let mut out = String::new();
            let mut last = 0;
            let all: Vec<Vec<Option<(usize, usize)>>> = p
                .re
                .captures_iter(s)
                .map(|c| (0..c.len()).map(|i| c.get(i).map(|m| (m.start(), m.end()))).collect())
                .collect();
            for (i, loc) in all.into_iter().enumerate() {
                if name == "replaceFirst" && i > 0 {
                    break;
                }
                let (ms, me) = loc[0].unwrap();
                let m = PMatcher { p: p.clone(), input: s.to_string(), loc: Some(loc), pos: me };
                let r = inp.call_fn(&args[1], vec![PV::Matcher(Rc::new(RefCell::new(m)))], off)?;
                out.push_str(&s[last..ms]);
                out.push_str(&jstr(&r));
                last = me;
            }
            out.push_str(&s[last..]);
            Ok(PV::str(&out))
        }
        ("getClass", 0) => Err(dyn_method_err(&PV::str(s), name, n, off)),
        ("regionMatches", 4) | ("regionMatches", 5) => {
            let (ignore, a) = if n == 5 { (args[0].as_bool() == Some(true), &args[1..]) } else { (false, &args[..]) };
            let (to, other, oo, ln) = (i64_of(&a[0]), utf16(&jstr(&a[1])), i64_of(&a[2]), i64_of(&a[3]));
            if to < 0 || oo < 0 || to + ln > len || oo + ln > other.len() as i64 || ln < 0 {
                return Ok(PV::Bool(false));
            }
            let x = from_utf16(&u[to as usize..(to + ln) as usize]);
            let y = from_utf16(&other[oo as usize..(oo + ln) as usize]);
            Ok(PV::Bool(if ignore { x.to_lowercase() == y.to_lowercase() } else { x == y }))
        }
        ("codePointCount", 2) => {
            let (b, e) = (idx(0).clamp(0, len) as usize, idx(1).clamp(0, len) as usize);
            Ok(PV::Int(from_utf16(&u[b.min(e)..e]).chars().count() as i32))
        }
        _ => Err(dyn_method_err(&PV::str(s), name, n, off)),
    }
}

fn sb_method(sb: &Rc<RefCell<Vec<char>>>, recv: &PV, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let i0 = || i64_of(&args[0]);
    Some(match name {
        "append" if args.len() == 1 => {
            let s = jstr(&args[0]);
            sb.borrow_mut().extend(s.chars());
            Ok(recv.clone())
        }
        "toString" => Ok(PV::str(&sb.borrow().iter().collect::<String>())),
        "reverse" => {
            sb.borrow_mut().reverse();
            Ok(recv.clone())
        }
        "length" => Ok(PV::Int(sb.borrow().iter().map(|c| c.len_utf16()).sum::<usize>() as i32)),
        "charAt" if args.len() == 1 => {
            let v = sb.borrow();
            let i = i0();
            if i < 0 || i as usize >= v.len() {
                return Some(Err(RtErr::new(
                    "string_index_out_of_bounds_exception",
                    &format!("index {},length {}", i, v.len()),
                    off,
                )));
            }
            Ok(char_pv(v[i as usize]))
        }
        "insert" if args.len() == 2 => {
            let mut v = sb.borrow_mut();
            let i = (i0().max(0) as usize).min(v.len());
            let ins: Vec<char> = jstr(&args[1]).chars().collect();
            v.splice(i..i, ins);
            drop(v);
            Ok(recv.clone())
        }
        "deleteCharAt" if args.len() == 1 => {
            let mut v = sb.borrow_mut();
            let i = i0();
            if i < 0 || i as usize >= v.len() {
                return Some(Err(RtErr::new(
                    "string_index_out_of_bounds_exception",
                    &format!("index {},length {}", i, v.len()),
                    off,
                )));
            }
            v.remove(i as usize);
            drop(v);
            Ok(recv.clone())
        }
        "setLength" if args.len() == 1 => {
            sb.borrow_mut().resize(i0().max(0) as usize, '\0');
            Ok(PV::Null)
        }
        "indexOf" if args.len() == 1 => {
            let s: String = sb.borrow().iter().collect();
            let needle = jstr(&args[0]);
            Ok(PV::Int(match s.find(needle.as_str()) {
                Some(b) => s[..b].encode_utf16().count() as i32,
                None => -1,
            }))
        }
        "isEmpty" => Ok(PV::Bool(sb.borrow().is_empty())),
        _ => return None,
    })
}

fn truthy(v: &PV) -> bool {
    v.as_bool() == Some(true)
}

fn collection_augment(inp: &Interp<'_>, items: &[PV], name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    let call = |it: &PV| inp.call_fn(&args[0], vec![it.clone()], off);
    let r = (|| -> R<PV> {
        match (name, n) {
            ("sum", 0) | ("sum", 1) => {
                let mut s = 0.0;
                for it in items {
                    let v = if n == 1 { call(it)? } else { it.clone() };
                    if !is_number(&v) {
                        return Err(cce(off, &format!("Cannot cast {} to java.lang.Number", class_name(&v))));
                    }
                    s += f64_of(&v);
                }
                Ok(PV::Double(s))
            }
            ("collect", 1) => {
                let mut out = Vec::with_capacity(items.len());
                for it in items {
                    out.push(call(it)?);
                }
                Ok(PV::list(out))
            }
            ("findAll", 1) => {
                let mut out = Vec::new();
                for it in items {
                    if truthy(&call(it)?) {
                        out.push(it.clone());
                    }
                }
                Ok(PV::list(out))
            }
            ("find", 1) => {
                for it in items {
                    if truthy(&call(it)?) {
                        return Ok(it.clone());
                    }
                }
                Ok(PV::Null)
            }
            ("findResult", 1) => {
                for it in items {
                    let r = call(it)?;
                    if !r.is_null() {
                        return Ok(r);
                    }
                }
                Ok(PV::Null)
            }
            ("any", 1) | ("every", 1) => {
                for it in items {
                    let b = truthy(&call(it)?);
                    if name == "any" && b {
                        return Ok(PV::Bool(true));
                    }
                    if name == "every" && !b {
                        return Ok(PV::Bool(false));
                    }
                }
                Ok(PV::Bool(name == "every"))
            }
            ("join", 1) => {
                let parts: Vec<String> = items.iter().map(jstr).collect();
                Ok(PV::str(&parts.join(&jstr(&args[0]))))
            }
            ("each", 1) | ("forEach", 1) => {
                for it in items {
                    call(it)?;
                }
                Ok(PV::Null)
            }
            ("eachWithIndex", 1) => {
                for (i, it) in items.iter().enumerate() {
                    inp.call_fn(&args[0], vec![it.clone(), PV::Int(i as i32)], off)?;
                }
                Ok(PV::Null)
            }
            ("groupBy", 1) => {
                let mut m = PMap::new(MapKind::Hash);
                for it in items {
                    let k = call(it)?;
                    match m.get(&k) {
                        Some(PV::List(l)) => l.borrow_mut().push(it.clone()),
                        _ => {
                            m.put(k, PV::list(vec![it.clone()]));
                        }
                    }
                }
                Ok(PV::map(m))
            }
            ("stream", 0) => Ok(PV::stream("obj", items.to_vec())),
            ("iterator", 0) => Ok(PV::iter(items.to_vec())),
            ("asList", 0) => Ok(PV::list(items.to_vec())),
            _ => Err(RtErr::new("_unknown", "", off)),
        }
    })();
    match r {
        Err(e) if e.typ == "_unknown" => None,
        other => Some(other),
    }
}

fn list_method(inp: &Interp<'_>, l: &Rc<RefCell<Vec<PV>>>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    let len = l.borrow().len();
    let i0 = || i64_of(&args[0]);
    let r = match (name, n) {
        ("add", 1) => {
            l.borrow_mut().push(args[0].clone());
            Ok(PV::Bool(true))
        }
        ("add", 2) => {
            let i = i0();
            if i < 0 || i as usize > len {
                return Some(Err(oob(i, len, off, &[])));
            }
            l.borrow_mut().insert(i as usize, args[1].clone());
            Ok(PV::Null)
        }
        ("addAll", 1) => match to_items(&args[0]) {
            None => Err(npe(off)),
            Some(items) => {
                let changed = !items.is_empty();
                l.borrow_mut().extend(items);
                Ok(PV::Bool(changed))
            }
        },
        ("get", 1) => {
            let i = i0();
            if i < 0 || i as usize >= len {
                return Some(Err(oob(i, len, off, ARRAYLIST_GET_FRAMES)));
            }
            Ok(l.borrow()[i as usize].clone())
        }
        ("set", 2) => {
            let i = i0();
            if i < 0 || i as usize >= len {
                return Some(Err(oob(i, len, off, &[])));
            }
            Ok(std::mem::replace(&mut l.borrow_mut()[i as usize], args[1].clone()))
        }
        ("remove", 1) => {
            if let PV::Int(i) = args[0] {
                if i < 0 || i as usize >= len {
                    return Some(Err(oob(i as i64, len, off, &[])));
                }
                return Some(Ok(l.borrow_mut().remove(i as usize)));
            }
            let pos = l.borrow().iter().position(|v| java_equals(v, &args[0]));
            match pos {
                Some(p) => {
                    l.borrow_mut().remove(p);
                    Ok(PV::Bool(true))
                }
                None => Ok(PV::Bool(false)),
            }
        }
        ("removeIf", 1) => {
            let items = l.borrow().clone();
            let mut out = Vec::new();
            let mut removed = false;
            for v in items {
                match inp.call_fn(&args[0], vec![v.clone()], off) {
                    Err(e) => return Some(Err(e)),
                    Ok(r) if truthy(&r) => removed = true,
                    Ok(_) => out.push(v),
                }
            }
            *l.borrow_mut() = out;
            Ok(PV::Bool(removed))
        }
        ("size", 0) | ("getLength", 0) => Ok(PV::Int(len as i32)),
        ("isEmpty", 0) => Ok(PV::Bool(len == 0)),
        ("contains", 1) => Ok(PV::Bool(l.borrow().iter().any(|v| java_equals(v, &args[0])))),
        ("containsAll", 1) => {
            let items = to_items(&args[0]).unwrap_or_default();
            let v = l.borrow();
            Ok(PV::Bool(items.iter().all(|it| v.iter().any(|x| java_equals(x, it)))))
        }
        ("indexOf", 1) => Ok(PV::Int(
            l.borrow().iter().position(|v| java_equals(v, &args[0])).map(|i| i as i32).unwrap_or(-1),
        )),
        ("lastIndexOf", 1) => Ok(PV::Int(
            l.borrow().iter().rposition(|v| java_equals(v, &args[0])).map(|i| i as i32).unwrap_or(-1),
        )),
        ("subList", 2) => {
            let (b, e) = (i0(), i64_of(&args[1]));
            if b < 0 || e > len as i64 || b > e {
                return Some(Err(RtErr::new(
                    "index_out_of_bounds_exception",
                    &format!("fromIndex: {}, toIndex: {}, size: {}", b, e, len),
                    off,
                )));
            }
            Ok(PV::list(l.borrow()[b as usize..e as usize].to_vec()))
        }
        ("sort", 1) => {
            let mut items = l.borrow().clone();
            if let Err(e) = sort_items(inp, &mut items, &args[0], off) {
                return Some(Err(e));
            }
            *l.borrow_mut() = items;
            Ok(PV::Null)
        }
        ("clear", 0) => {
            l.borrow_mut().clear();
            Ok(PV::Null)
        }
        ("toArray", 0) => Ok(PV::array("Object", l.borrow().clone())),
        ("replaceAll", 1) => {
            let items = l.borrow().clone();
            let mut out = Vec::with_capacity(items.len());
            for v in items {
                match inp.call_fn(&args[0], vec![v], off) {
                    Ok(r) => out.push(r),
                    Err(e) => return Some(Err(e)),
                }
            }
            *l.borrow_mut() = out;
            Ok(PV::Null)
        }
        ("removeAll", 1) | ("retainAll", 1) => {
            let items = to_items(&args[0]).unwrap_or_default();
            let keep = name == "retainAll";
            let old = l.borrow().clone();
            let out: Vec<PV> = old.iter().filter(|v| items.iter().any(|it| java_equals(v, it)) == keep).cloned().collect();
            let changed = out.len() != old.len();
            *l.borrow_mut() = out;
            Ok(PV::Bool(changed))
        }
        ("getFirst", 0) | ("getLast", 0) => {
            let v = l.borrow();
            let x = if name == "getFirst" { v.first() } else { v.last() };
            match x {
                Some(x) => Ok(x.clone()),
                None => Err(RtErr::nil("no_such_element_exception", off)),
            }
        }
        ("equals", 1) => Ok(PV::Bool(java_equals(&PV::List(l.clone()), &args[0]))),
        ("hashCode", 0) => Ok(PV::Int(hash_code(&PV::List(l.clone())))),
        ("toString", 0) => Ok(PV::str(&jstr(&PV::List(l.clone())))),
        _ => {
            let items = l.borrow().clone();
            return collection_augment(inp, &items, name, args, off);
        }
    };
    Some(r)
}

fn set_method(inp: &Interp<'_>, s: &Rc<RefCell<PMap>>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let r = match (name, args.len()) {
        ("add", 1) => Ok(PV::Bool(set_add(s, args[0].clone()))),
        ("addAll", 1) => {
            let mut changed = false;
            for it in to_items(&args[0]).unwrap_or_default() {
                changed |= set_add(s, it);
            }
            Ok(PV::Bool(changed))
        }
        ("contains", 1) => Ok(PV::Bool(s.borrow().contains(&args[0]))),
        ("remove", 1) => Ok(PV::Bool(s.borrow_mut().remove(&args[0]).is_some())),
        ("size", 0) => Ok(PV::Int(s.borrow().len() as i32)),
        ("isEmpty", 0) => Ok(PV::Bool(s.borrow().len() == 0)),
        ("clear", 0) => {
            s.borrow_mut().clear();
            Ok(PV::Null)
        }
        ("removeIf", 1) => {
            let items = s.borrow().keys();
            let mut removed = false;
            for it in items {
                match inp.call_fn(&args[0], vec![it.clone()], off) {
                    Err(e) => return Some(Err(e)),
                    Ok(r) if truthy(&r) => {
                        s.borrow_mut().remove(&it);
                        removed = true;
                    }
                    Ok(_) => {}
                }
            }
            Ok(PV::Bool(removed))
        }
        ("containsAll", 1) => {
            let items = to_items(&args[0]).unwrap_or_default();
            let m = s.borrow();
            Ok(PV::Bool(items.iter().all(|it| m.contains(it))))
        }
        _ => {
            let items = s.borrow().keys();
            return collection_augment(inp, &items, name, args, off);
        }
    };
    Some(r)
}

fn entry(m: &Rc<RefCell<PMap>>, k: &PV, v: &PV) -> PV {
    PV::Entry(m.clone(), Rc::new(k.clone()), Rc::new(v.clone()))
}

fn map_method(inp: &Interp<'_>, m: &Rc<RefCell<PMap>>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    let call2 = |f: &PV, k: &PV, v: &PV| -> R<PV> {
        if let PV::Fn(pf) = f {
            if let PFn::Lambda { lam, .. } = &**pf {
                if lam.pnames.len() == 1 {
                    return inp.call_fn(f, vec![entry(m, k, v)], off);
                }
            }
        }
        inp.call_fn(f, vec![k.clone(), v.clone()], off)
    };
    let ordered = || m.borrow().ordered();
    let r = (|| -> R<PV> {
        match (name, n) {
            ("get", 1) => Ok(m.borrow().get(&args[0]).unwrap_or(PV::Null)),
            ("put", 2) => Ok(m.borrow_mut().put(args[0].clone(), args[1].clone())),
            ("putAll", 1) => {
                if let PV::Map(src) = &args[0] {
                    let entries = src.borrow().ordered();
                    let mut mm = m.borrow_mut();
                    for (k, v) in entries {
                        mm.put(k, v);
                    }
                }
                Ok(PV::Null)
            }
            ("putIfAbsent", 2) => {
                let cur = m.borrow().get(&args[0]);
                match cur {
                    Some(v) if !v.is_null() => Ok(v),
                    _ => {
                        m.borrow_mut().put(args[0].clone(), args[1].clone());
                        Ok(PV::Null)
                    }
                }
            }
            ("containsKey", 1) => Ok(PV::Bool(m.borrow().contains(&args[0]))),
            ("containsValue", 1) => Ok(PV::Bool(m.borrow().entries.iter().any(|(_, v)| java_equals(v, &args[0])))),
            ("remove", 1) => Ok(m.borrow_mut().remove(&args[0]).unwrap_or(PV::Null)),
            ("size", 0) => Ok(PV::Int(m.borrow().len() as i32)),
            ("isEmpty", 0) => Ok(PV::Bool(m.borrow().len() == 0)),
            ("clear", 0) => {
                m.borrow_mut().clear();
                Ok(PV::Null)
            }
            ("keySet", 0) => Ok(new_set_from(MapKind::Linked, m.borrow().keys())),
            ("values", 0) => Ok(PV::list(m.borrow().values())),
            ("entrySet", 0) => {
                let items: Vec<PV> = ordered().iter().map(|(k, v)| entry(m, k, v)).collect();
                Ok(PV::EntrySet(Rc::new(RefCell::new(items))))
            }
            ("getOrDefault", 2) => Ok(m.borrow().get(&args[0]).unwrap_or_else(|| args[1].clone())),
            ("computeIfAbsent", 2) => {
                let cur = m.borrow().get(&args[0]);
                if let Some(v) = cur {
                    if !v.is_null() {
                        return Ok(v);
                    }
                }
                let v = inp.call_fn(&args[1], vec![args[0].clone()], off)?;
                if !v.is_null() {
                    m.borrow_mut().put(args[0].clone(), v.clone());
                }
                Ok(v)
            }
            ("computeIfPresent", 2) | ("compute", 2) => {
                let old = m.borrow().get(&args[0]);
                if name == "computeIfPresent" && old.as_ref().map(|v| v.is_null()).unwrap_or(true) {
                    return Ok(PV::Null);
                }
                let v = inp.call_fn(&args[1], vec![args[0].clone(), old.unwrap_or(PV::Null)], off)?;
                if v.is_null() {
                    m.borrow_mut().remove(&args[0]);
                } else {
                    m.borrow_mut().put(args[0].clone(), v.clone());
                }
                Ok(v)
            }
            ("merge", 3) => {
                let old = m.borrow().get(&args[0]);
                let v = match old {
                    Some(o) if !o.is_null() => inp.call_fn(&args[2], vec![o, args[1].clone()], off)?,
                    _ => args[1].clone(),
                };
                if v.is_null() {
                    m.borrow_mut().remove(&args[0]);
                } else {
                    m.borrow_mut().put(args[0].clone(), v.clone());
                }
                Ok(v)
            }
            ("forEach", 1) | ("each", 1) => {
                for (k, v) in ordered() {
                    call2(&args[0], &k, &v)?;
                }
                Ok(PV::Null)
            }
            ("collect", 1) => {
                let mut out = Vec::new();
                for (k, v) in ordered() {
                    out.push(call2(&args[0], &k, &v)?);
                }
                Ok(PV::list(out))
            }
            ("findAll", 1) => {
                let mut out = PMap::new(m.borrow().kind);
                for (k, v) in ordered() {
                    if truthy(&call2(&args[0], &k, &v)?) {
                        out.put(k, v);
                    }
                }
                Ok(PV::map(out))
            }
            ("find", 1) => {
                for (k, v) in ordered() {
                    if truthy(&call2(&args[0], &k, &v)?) {
                        return Ok(entry(m, &k, &v));
                    }
                }
                Ok(PV::Null)
            }
            ("any", 1) | ("every", 1) => {
                for (k, v) in ordered() {
                    let b = truthy(&call2(&args[0], &k, &v)?);
                    if name == "any" && b {
                        return Ok(PV::Bool(true));
                    }
                    if name == "every" && !b {
                        return Ok(PV::Bool(false));
                    }
                }
                Ok(PV::Bool(name == "every"))
            }
            ("count", 1) => {
                let mut c = 0;
                for (k, v) in ordered() {
                    if truthy(&call2(&args[0], &k, &v)?) {
                        c += 1;
                    }
                }
                Ok(PV::Int(c))
            }
            ("equals", 1) => Ok(PV::Bool(java_equals(&PV::Map(m.clone()), &args[0]))),
            ("hashCode", 0) => Ok(PV::Int(hash_code(&PV::Map(m.clone())))),
            ("toString", 0) => Ok(PV::str(&jstr(&PV::Map(m.clone())))),
            _ => Err(RtErr::new("_unknown", "", off)),
        }
    })();
    match r {
        Err(e) if e.typ == "_unknown" => None,
        other => Some(other),
    }
}

// ---- streams ----

fn opt_name(kind: &str) -> &'static str {
    match kind {
        "int" => "OptionalInt",
        "long" => "OptionalLong",
        "double" => "OptionalDouble",
        _ => "Optional",
    }
}

fn stream_method(inp: &Interp<'_>, s: &Rc<PStream>, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    let call = |it: &PV| inp.call_fn(&args[0], vec![it.clone()], off);
    let map_each = |kind: &'static str| -> R<PV> {
        let mut out = Vec::with_capacity(s.items.len());
        for it in &s.items {
            let mut r = call(it)?;
            if kind != "obj" {
                if !is_number(&r) {
                    return Err(cce(off, &format!("Cannot cast {} to {}", class_name(&r), kind)));
                }
                r = cast_num(&r, kind);
            }
            out.push(r);
        }
        Ok(PV::stream(kind, out))
    };
    let on = opt_name(s.kind);
    let r = (|| -> R<PV> {
        match (name, n) {
            ("map", 1) => map_each(s.kind),
            ("mapToObj", 1) => map_each("obj"),
            ("mapToInt", 1) => map_each("int"),
            ("mapToLong", 1) => map_each("long"),
            ("mapToDouble", 1) => map_each("double"),
            ("boxed", 0) => Ok(PV::stream("obj", s.items.clone())),
            ("asDoubleStream", 0) => Ok(PV::stream("double", s.items.iter().map(|x| PV::Double(f64_of(x))).collect())),
            ("asLongStream", 0) => Ok(PV::stream("long", s.items.iter().map(|x| PV::Long(i64_of(x))).collect())),
            ("filter", 1) => {
                let mut out = Vec::new();
                for it in &s.items {
                    if truthy(&call(it)?) {
                        out.push(it.clone());
                    }
                }
                Ok(PV::stream(s.kind, out))
            }
            ("flatMap", 1) => {
                let mut out = Vec::new();
                for it in &s.items {
                    out.extend(to_items(&call(it)?).unwrap_or_default());
                }
                Ok(PV::stream(s.kind, out))
            }
            ("sorted", 0) | ("sorted", 1) => {
                let mut items = s.items.clone();
                let cmp = if n == 1 { args[0].clone() } else { PV::Null };
                sort_items(inp, &mut items, &cmp, off)?;
                Ok(PV::stream(s.kind, items))
            }
            ("distinct", 0) => {
                let mut m = PMap::new(MapKind::Linked);
                for it in &s.items {
                    if !m.contains(it) {
                        m.put(it.clone(), PV::Bool(true));
                    }
                }
                Ok(PV::stream(s.kind, m.keys()))
            }
            ("limit", 1) => {
                let k = (i64_of(&args[0]).max(0) as usize).min(s.items.len());
                Ok(PV::stream(s.kind, s.items[..k].to_vec()))
            }
            ("skip", 1) => {
                let k = (i64_of(&args[0]).max(0) as usize).min(s.items.len());
                Ok(PV::stream(s.kind, s.items[k..].to_vec()))
            }
            ("peek", 1) | ("forEach", 1) | ("forEachOrdered", 1) => {
                for it in &s.items {
                    call(it)?;
                }
                if name == "peek" {
                    return Ok(PV::Stream(s.clone()));
                }
                Ok(PV::Null)
            }
            ("count", 0) => Ok(PV::Long(s.items.len() as i64)),
            ("sum", 0) => Ok(match s.kind {
                "int" => PV::Int(s.items.iter().fold(0i32, |a, x| a.wrapping_add(i64_of(x) as i32))),
                "long" => PV::Long(s.items.iter().fold(0i64, |a, x| a.wrapping_add(i64_of(x)))),
                _ => PV::Double(s.items.iter().map(f64_of).sum()),
            }),
            ("average", 0) => {
                if s.items.is_empty() {
                    return Ok(PV::optional("OptionalDouble", None));
                }
                let t: f64 = s.items.iter().map(f64_of).sum();
                Ok(PV::optional("OptionalDouble", Some(PV::Double(t / s.items.len() as f64))))
            }
            ("max", 0) | ("min", 0) | ("max", 1) | ("min", 1) => {
                if s.items.is_empty() {
                    return Ok(PV::optional(on, None));
                }
                let cmp = if n == 1 { args[0].clone() } else { PV::Null };
                let mut best = s.items[0].clone();
                for it in &s.items[1..] {
                    let c = compare_with(inp, &cmp, it, &best, off)?;
                    if (name == "max" && c > 0) || (name == "min" && c < 0) {
                        best = it.clone();
                    }
                }
                Ok(PV::optional(on, Some(best)))
            }
            ("reduce", 1) => {
                if s.items.is_empty() {
                    return Ok(PV::optional(on, None));
                }
                let mut acc = s.items[0].clone();
                for it in &s.items[1..] {
                    acc = inp.call_fn(&args[0], vec![acc, it.clone()], off)?;
                }
                Ok(PV::optional(on, Some(acc)))
            }
            ("reduce", 2) => {
                let mut acc = args[0].clone();
                for it in &s.items {
                    acc = inp.call_fn(&args[1], vec![acc, it.clone()], off)?;
                }
                if s.kind != "obj" && is_number(&acc) {
                    acc = cast_num(&acc, s.kind);
                }
                Ok(acc)
            }
            ("anyMatch", 1) | ("allMatch", 1) | ("noneMatch", 1) => {
                for it in &s.items {
                    let b = truthy(&call(it)?);
                    match name {
                        "anyMatch" if b => return Ok(PV::Bool(true)),
                        "allMatch" if !b => return Ok(PV::Bool(false)),
                        "noneMatch" if b => return Ok(PV::Bool(false)),
                        _ => {}
                    }
                }
                Ok(PV::Bool(name != "anyMatch"))
            }
            ("findFirst", 0) | ("findAny", 0) => Ok(PV::optional(on, s.items.first().cloned())),
            ("toArray", 0) => Ok(PV::array(if s.kind == "obj" { "Object" } else { s.kind }, s.items.clone())),
            ("toList", 0) => Ok(PV::list(s.items.clone())),
            ("iterator", 0) => Ok(PV::iter(s.items.clone())),
            ("collect", 1) => match &args[0] {
                PV::Collector(c) => collect(inp, c, s.items.clone(), off),
                other => Err(cce(off, &format!("Cannot cast {} to java.util.stream.Collector", class_name(other)))),
            },
            _ => Err(RtErr::new("_unknown", "", off)),
        }
    })();
    match r {
        Err(e) if e.typ == "_unknown" => None,
        other => Some(other),
    }
}

fn collect(inp: &Interp<'_>, c: &PCollector, items: Vec<PV>, off: usize) -> R<PV> {
    match c.kind.as_str() {
        "toList" => Ok(PV::list(items)),
        "toSet" => Ok(new_set_from(MapKind::Hash, items)),
        "joining" => {
            let mut parts = Vec::with_capacity(items.len());
            for it in &items {
                match it {
                    PV::Str(s) => parts.push(s.to_string()),
                    other => {
                        return Err(cce(
                            off,
                            &format!("class {} cannot be cast to class java.lang.CharSequence", class_name(other)),
                        ))
                    }
                }
            }
            Ok(PV::str(&format!("{}{}{}", c.prefix, parts.join(&c.sep), c.suffix)))
        }
        "counting" => Ok(PV::Long(items.len() as i64)),
        "groupingBy" => {
            let mut m = PMap::new(MapKind::Hash);
            for it in items {
                let k = inp.call_fn(&c.f1, vec![it.clone()], off)?;
                match m.get(&k) {
                    Some(PV::List(l)) => l.borrow_mut().push(it),
                    _ => {
                        m.put(k, PV::list(vec![it]));
                    }
                }
            }
            if let PV::Collector(sub) = &c.f2 {
                let entries = m.entries.clone();
                for (k, v) in entries {
                    let items = to_items(&v).unwrap_or_default();
                    let r = collect(inp, sub, items, off)?;
                    m.put(k, r);
                }
            }
            Ok(PV::map(m))
        }
        "toMap" => {
            let mut m = PMap::new(MapKind::Hash);
            for it in items {
                let k = inp.call_fn(&c.f1, vec![it.clone()], off)?;
                let v = inp.call_fn(&c.f2, vec![it], off)?;
                if m.contains(&k) {
                    return Err(RtErr::new("illegal_state_exception", &format!("Duplicate key {}", jstr(&k)), off));
                }
                m.put(k, v);
            }
            Ok(PV::map(m))
        }
        "summingInt" | "summingLong" | "summingDouble" | "averagingInt" | "averagingDouble" => {
            let mut total = 0.0;
            let mut itotal: i64 = 0;
            for it in &items {
                let v = inp.call_fn(&c.f1, vec![it.clone()], off)?;
                total += f64_of(&v);
                itotal = itotal.wrapping_add(i64_of(&v));
            }
            Ok(match c.kind.as_str() {
                "summingInt" => PV::Int(itotal as i32),
                "summingLong" => PV::Long(itotal),
                "summingDouble" => PV::Double(total),
                _ => PV::Double(if items.is_empty() { 0.0 } else { total / items.len() as f64 }),
            })
        }
        "mapping" => {
            let mut mapped = Vec::with_capacity(items.len());
            for it in items {
                mapped.push(inp.call_fn(&c.f1, vec![it], off)?);
            }
            if let PV::Collector(sub) = &c.f2 {
                return collect(inp, sub, mapped, off);
            }
            Ok(PV::list(mapped))
        }
        other => Err(iae(off, &format!("unsupported collector [{}]", other))),
    }
}
