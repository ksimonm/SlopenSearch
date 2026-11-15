//! Java's Double.toString and Float.toString (JDK 19+: the shortest digits
//! that round-trip, `1.0E7` / `1.0E-4` outside [1e-3, 1e7)), which is how
//! Jackson writes the doubles and floats OpenSearch holds.

use serde_json::{Number, Value};

/// A JSON number whose text is `f` as a Java double; NaN and infinities
/// (not JSON numbers) as their Java strings.
pub fn double_value(f: f64) -> Value {
    if f.is_finite() {
        Value::Number(Number::from_string_unchecked(double(f)))
    } else {
        Value::String(double(f))
    }
}

/// A JSON number whose text is `f` as a Java float.
pub fn float_value(f: f32) -> Value {
    if f.is_finite() {
        Value::Number(Number::from_string_unchecked(float(f)))
    } else {
        Value::String(float(f))
    }
}

pub fn double(f: f64) -> String {
    if f.is_nan() {
        return "NaN".into();
    }
    if f.is_infinite() {
        return if f > 0.0 { "Infinity".into() } else { "-Infinity".into() };
    }
    if f == 0.0 {
        return if f.is_sign_negative() { "-0.0".into() } else { "0.0".into() };
    }
    let mut e = even_tie(format!("{:e}", f), |p| format!("{:.*e}", p, f), |t| t.parse::<f64>().ok() == Some(f));
    let (m, _) = e.split_once('e').unwrap();
    if !m.contains('.') {
        let two = format!("{:.1e}", f);
        if !two.contains(".0e") {
            e = two;
        }
    }
    fmt_shortest(&e, f.abs())
}

pub fn float(f: f32) -> String {
    if f.is_nan() {
        return "NaN".into();
    }
    if f.is_infinite() {
        return if f > 0.0 { "Infinity".into() } else { "-Infinity".into() };
    }
    if f == 0.0 {
        return if f.is_sign_negative() { "-0.0".into() } else { "0.0".into() };
    }
    let mut e = even_tie(format!("{:e}", f), |p| format!("{:.*e}", p, f), |t| t.parse::<f32>().ok() == Some(f));
    let (m, _) = e.split_once('e').unwrap();
    if !m.contains('.') {
        let two = format!("{:.1e}", f);
        if !two.contains(".0e") {
            e = two;
        }
    }
    fmt_shortest(&e, f.abs() as f64)
}

/// Of two shortest decimals equally close to the value (the exact value has
/// one digit more, a 5), Java takes the one with the even last digit; Rust's
/// formatter (`e`, "d.ddde±x") may take the other. `exact(p)` formats the
/// value with `p` fraction digits, `round_trips` checks a candidate.
fn even_tie(e: String, exact: impl Fn(usize) -> String, round_trips: impl Fn(&str) -> bool) -> String {
    let split = |t: &str| -> Option<(bool, String, i32)> {
        let (m, x) = t.split_once('e')?;
        Some((m.starts_with('-'), m.chars().filter(char::is_ascii_digit).collect(), x.parse().ok()?))
    };
    let Some((neg, digits, exp)) = split(&e) else { return e };
    let l = digits.len();
    if !(2..=18).contains(&l) || (digits.as_bytes()[l - 1] - b'0') % 2 == 0 {
        return e;
    }
    let Some((_, long, lexp)) = split(&exact(l + 24)) else { return e };
    if long.as_bytes()[l] != b'5' || long[l + 1..].bytes().any(|b| b != b'0') {
        return e;
    }
    let (Ok(lower), Ok(mine)) = (long[..l].parse::<u64>(), digits.parse::<u64>()) else { return e };
    if exp != lexp || (mine != lower && mine != lower + 1) {
        return e;
    }
    let mut other = if mine == lower { lower + 1 } else { lower }.to_string();
    let mut oexp = lexp;
    if other.len() > l {
        oexp += 1;
    }
    while other.len() > 1 && other.ends_with('0') {
        other.pop();
    }
    let rest = if other.len() > 1 { &other[1..] } else { "0" };
    let cand = format!("{}{}.{}e{}", if neg { "-" } else { "" }, &other[..1], rest, oexp);
    if round_trips(&cand) {
        cand
    } else {
        e
    }
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

/// Writes every number of a JSON text that has the layout of
/// Double.toString (`1.5e+20`, `1.0e-4`: one digit, a fraction, an exponent
/// without leading zeros) as Jackson writes a Java double (`1.5E20`):
/// serde_json lowers the marker and adds a `+` whenever a value is re-parsed
/// or built with `json!`. The `_source` of documents keeps the text it was
/// sent with.
pub fn java_exponents(body: &[u8]) -> Vec<u8> {
    let n = body.len();
    let mut out = Vec::with_capacity(n);
    let mut i = 0;
    // the last token was the key `_source` and its colon
    let mut source_value = false;
    while i < n {
        match body[i] {
            b'"' => {
                let start = i;
                i = skip_string(body, i);
                let mut j = i;
                while j < n && body[j].is_ascii_whitespace() {
                    j += 1;
                }
                source_value = j < n && body[j] == b':' && &body[start..i] == b"\"_source\"";
                if source_value {
                    i = j + 1;
                }
                out.extend_from_slice(&body[start..i]);
            }
            b'{' | b'[' if source_value => {
                let start = i;
                i = skip_container(body, i);
                out.extend_from_slice(&body[start..i]);
                source_value = false;
            }
            b'-' | b'0'..=b'9' => {
                let start = i;
                while i < n && matches!(body[i], b'-' | b'+' | b'.' | b'e' | b'E' | b'0'..=b'9') {
                    i += 1;
                }
                let tok = &body[start..i];
                match java_exponent_at(tok).filter(|_| !source_value) {
                    Some((e, rest)) => {
                        out.extend_from_slice(&tok[..e]);
                        out.push(b'E');
                        out.extend_from_slice(&tok[rest..]);
                    }
                    None => out.extend_from_slice(tok),
                }
                source_value = false;
            }
            c => {
                if !c.is_ascii_whitespace() {
                    source_value = false;
                }
                out.push(c);
                i += 1;
            }
        }
    }
    out
}

/// The offset after the string that starts at `i`.
fn skip_string(body: &[u8], mut i: usize) -> usize {
    i += 1;
    while i < body.len() && body[i] != b'"' {
        if body[i] == b'\\' {
            i += 1;
        }
        i += 1;
    }
    (i + 1).min(body.len())
}

/// The offset after the object or array that starts at `i`.
fn skip_container(body: &[u8], mut i: usize) -> usize {
    let mut depth = 0usize;
    while i < body.len() {
        match body[i] {
            b'"' => {
                i = skip_string(body, i);
                continue;
            }
            b'{' | b'[' => depth += 1,
            b'}' | b']' => {
                depth = depth.saturating_sub(1);
                if depth == 0 {
                    return i + 1;
                }
            }
            _ => {}
        }
        i += 1;
    }
    body.len()
}

/// For a number token in Double.toString layout, the offset of its
/// exponent marker and of the exponent without a `+`.
fn java_exponent_at(tok: &[u8]) -> Option<(usize, usize)> {
    let t = tok.strip_prefix(b"-").unwrap_or(tok);
    let off = tok.len() - t.len();
    if t.len() < 5 || !matches!(t[0], b'1'..=b'9') || t[1] != b'.' {
        return None;
    }
    let e = t.iter().position(|&c| c == b'e' || c == b'E')?;
    if e < 3 || !t[2..e].iter().all(u8::is_ascii_digit) {
        return None;
    }
    let rest = if t.get(e + 1) == Some(&b'+') { e + 2 } else { e + 1 };
    let exp = t[rest..].strip_prefix(b"-").unwrap_or(&t[rest..]);
    if exp.is_empty() || exp[0] == b'0' || !exp.iter().all(u8::is_ascii_digit) {
        return None;
    }
    Some((off + e, off + rest))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn java_layout() {
        assert_eq!(double(1.0e7), "1.0E7");
        assert_eq!(double(1.5e20), "1.5E20");
        assert_eq!(double(1.0e-4), "1.0E-4");
        assert_eq!(double(0.001), "0.001");
        assert_eq!(double(123456789.125), "1.23456789125E8");
        assert_eq!(double(9999999.0), "9999999.0");
        assert_eq!(double(-0.0), "-0.0");
        assert_eq!(double(5e-324), "4.9E-324");
        assert_eq!(float(1.0e-4), "1.0E-4");
        assert_eq!(float(3.4e38), "3.4E38");
        assert_eq!(double(3.4e38f32 as f64), "3.3999999521443642E38");
        // ties between two shortest decimals go to the even digit
        assert_eq!(double(141.90005493164062f32 as f64), "141.90005493164062");
        assert_eq!(double(154.10000610351562f32 as f64), "154.10000610351562");
    }

    #[test]
    fn exponents() {
        let b = br#"{"a":1.0e-4,"b":[1.5e20,1e7,1.5e+20,-2.5e3,1.0e+07],"_source" : {"x":1.5e+20,"y":"}"},"c":"1.0e5","_source":1.5e2,"d":2.0e1}"#;
        assert_eq!(
            String::from_utf8(java_exponents(b)).unwrap(),
            r#"{"a":1.0E-4,"b":[1.5E20,1e7,1.5E20,-2.5E3,1.0e+07],"_source" : {"x":1.5e+20,"y":"}"},"c":"1.0e5","_source":1.5e2,"d":2.0E1}"#
        );
    }
}
