//! The geo and date decay functions of score scripts (OpenSearch's
//! ScoreScriptUtils DecayGeo* / DecayDate*): `decayGeoGauss(origin, scale,
//! offset, decay, doc['location'].value)` and friends. The numeric ones are
//! bound next to the other score functions (search::painless_doc).

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use serde_json::Value;

use crate::painless::interp::NativeFn;
use crate::painless::value::*;

/// What a decay function derives from its constant arguments once (the Java
/// classes cache it per script).
struct DecayParams {
    lat: f64,
    lon: f64,
    origin: i64,
    offset: f64,
    scaling: f64,
    err: Option<(&'static str, String)>,
}

thread_local! {
    static CACHE: RefCell<HashMap<String, Rc<DecayParams>>> = RefCell::new(HashMap::new());
}

fn scaling_of(curve: &str, scale: f64, decay: f64) -> f64 {
    match curve {
        "Linear" => scale / (1.0 - decay),
        "Exp" => decay.ln() / scale,
        _ => 0.5 * scale.powf(2.0) / decay.ln(),
    }
}

fn score_of(curve: &str, scaling: f64, distance: f64) -> f64 {
    match curve {
        "Linear" => ((scaling - distance) / scaling).max(0.0),
        "Exp" => (scaling * distance).exp(),
        _ => (0.5 * distance.powf(2.0) / scaling).exp(),
    }
}

fn params_for(geo: bool, curve: &str, a: &[PV]) -> Rc<DecayParams> {
    let (origin_s, scale_s, offset_s) = (jstr(&a[0]), jstr(&a[1]), jstr(&a[2]));
    let decay = f64_of(&a[3]);
    let key = format!("{}{}\0{}\0{}\0{}\0{}", geo, curve, origin_s, scale_s, offset_s, decay);
    if let Some(p) = CACHE.with(|c| c.borrow().get(&key).cloned()) {
        return p;
    }
    let mut p = DecayParams { lat: 0.0, lon: 0.0, origin: 0, offset: 0.0, scaling: 0.0, err: None };
    if geo {
        match parse_geo_point(&origin_s) {
            Ok((lat, lon)) => {
                p.lat = lat;
                p.lon = lon;
            }
            Err(e) => p.err = Some(("illegal_argument_exception", e)),
        }
        let scale = crate::mapping::geo::parse_distance_meters(&Value::String(scale_s.clone()));
        let offset = crate::mapping::geo::parse_distance_meters(&Value::String(offset_s.clone()));
        match (scale, offset) {
            (Some(s), Some(o)) => {
                p.offset = o;
                p.scaling = scaling_of(curve, s, decay);
            }
            (s, _) => {
                let bad = if s.is_none() { &scale_s } else { &offset_s };
                p.err = Some(("number_format_exception", format!("For input string: \"{}\"", bad)));
            }
        }
    } else {
        match parse_date_origin(&origin_s) {
            Some(ms) => p.origin = ms,
            None => {
                p.err = Some((
                    "illegal_argument_exception",
                    format!("failed to parse date field [{}] with format [strict_date_optional_time||epoch_millis]", origin_s),
                ))
            }
        }
        let name = format!("DecayDate{}", curve);
        let scale = time_value_millis(&scale_s, &format!("{}.scale", name));
        let offset = time_value_millis(&offset_s, &format!("{}.offset", name));
        match (scale, offset) {
            (Ok(s), Ok(o)) => {
                p.offset = o as f64;
                p.scaling = scaling_of(curve, s as f64, decay);
            }
            (Err(e), _) | (_, Err(e)) => p.err = Some(("illegal_argument_exception", e)),
        }
    }
    let p = Rc::new(p);
    if key.len() < 512 {
        CACHE.with(|c| {
            let mut c = c.borrow_mut();
            if c.len() > 1024 {
                c.clear();
            }
            c.insert(key, p.clone());
        });
    }
    p
}

/// Adds decayGeo{Linear,Exp,Gauss} and decayDate{Linear,Exp,Gauss}.
pub fn add_decay_bindings(m: &mut HashMap<String, NativeFn>) {
    for curve in ["Linear", "Exp", "Gauss"] {
        m.insert(
            format!("decayGeo{}", curve),
            Rc::new(move |_, a, off| {
                let p = params_for(true, curve, &a);
                if let Some((typ, reason)) = &p.err {
                    return Err(RtErr::new(typ, reason, off));
                }
                let (lat, lon) = match &a[4] {
                    PV::GeoPoint(lat, lon) => (*lat, *lon),
                    PV::Null => return Err(npe(off)),
                    other => {
                        return Err(cce(
                            off,
                            &format!("Cannot cast {} to org.opensearch.common.geo.GeoPoint", class_name(other)),
                        ))
                    }
                };
                // doc values hold the point quantized to Lucene's 32-bit encoding
                let d = arc_distance_meters(p.lat, p.lon, quantize_lat(lat), quantize_lon(lon));
                Ok(PV::Double(score_of(curve, p.scaling, (d - p.offset).max(0.0))))
            }),
        );
        m.insert(
            format!("decayDate{}", curve),
            Rc::new(move |_, a, off| {
                let p = params_for(false, curve, &a);
                if let Some((typ, reason)) = &p.err {
                    return Err(RtErr::new(typ, reason, off));
                }
                let doc = match &a[4] {
                    PV::Zdt(z) => z.instant_nanos().div_euclid(1_000_000) as i64,
                    PV::Null => return Err(npe(off)),
                    other => {
                        return Err(cce(
                            off,
                            &format!(
                                "Cannot cast {} to org.opensearch.script.JodaCompatibleZonedDateTime",
                                class_name(other)
                            ),
                        ))
                    }
                };
                let diff = (doc - p.origin).abs();
                let distance = (diff as f64 - p.offset).max(0.0) as i64;
                Ok(PV::Double(score_of(curve, p.scaling, distance as f64)))
            }),
        );
    }
}

/// GeoDistance.ARC: Lucene's SloppyMath.haversinMeters.
fn arc_distance_meters(lat1: f64, lon1: f64, lat2: f64, lon2: f64) -> f64 {
    const TO_METERS: f64 = 6_371_008.7714;
    let (x1, x2) = (lat1.to_radians(), lat2.to_radians());
    let h1 = 1.0 - (x1 - x2).cos();
    let h2 = 1.0 - (lon1 - lon2).to_radians().cos();
    let h = h1 + x1.cos() * x2.cos() * h2;
    let h = f64::from_bits(h.to_bits() & 0xFFFF_FFFF_FFFF_FFF8);
    TO_METERS * 2.0 * (h * 0.5).sqrt().min(1.0).asin()
}

const LAT_DECODE: f64 = 180.0 / 4294967296.0;
const LON_DECODE: f64 = 360.0 / 4294967296.0;

fn quantize_lat(lat: f64) -> f64 {
    let lat = if lat == 90.0 { f64::from_bits(90f64.to_bits() - 1) } else { lat };
    ((lat / LAT_DECODE).floor() as i32) as f64 * LAT_DECODE
}

fn quantize_lon(lon: f64) -> f64 {
    let lon = if lon == 180.0 { f64::from_bits(180f64.to_bits() - 1) } else { lon };
    ((lon / LON_DECODE).floor() as i32) as f64 * LON_DECODE
}

/// GeoUtils.parseGeoPoint of a string: "lat, lon", "POINT (lon lat)" or a geohash.
fn parse_geo_point(s: &str) -> Result<(f64, f64), String> {
    let t = s.trim();
    if t.to_uppercase().starts_with("POINT") {
        let inner = t[5..].trim();
        if let Some(body) = inner.strip_prefix('(').and_then(|x| x.strip_suffix(')')) {
            let f: Vec<&str> = body.split_whitespace().collect();
            if f.len() == 2 {
                if let (Ok(lon), Ok(lat)) = (f[0].parse::<f64>(), f[1].parse::<f64>()) {
                    return Ok((lat, lon));
                }
            }
        }
        return Err(format!("Unable to parse shape [{}]", s));
    }
    if t.contains(',') {
        let parts: Vec<&str> = t.split(',').collect();
        if parts.len() > 3 {
            return Err(format!("failed to parse [{}], expected 2 or 3 coordinates but found: [{}]", s, parts.len()));
        }
        let lat = parts[0].trim().parse::<f64>().map_err(|_| "latitude must be a number".to_string())?;
        let lon = parts[1].trim().parse::<f64>().map_err(|_| "longitude must be a number".to_string())?;
        if parts.len() == 3 {
            return Err(format!(
                "Exception parsing coordinates: found Z value [{}] but [ignore_z_value] parameter is [false]",
                parts[2].trim()
            ));
        }
        return Ok((lat, lon));
    }
    // GeoPoint#resetFromGeoHash: the bottom-left corner of the cell
    crate::geohash::decode(t).ok_or_else(|| format!("unsupported symbol in geohash [{}]", t))
}

/// The origin of a date decay function: "strict_date_optional_time||epoch_millis"
/// with date math, in UTC.
fn parse_date_origin(s: &str) -> Option<i64> {
    let (anchor, expr) = if let Some(rest) = s.strip_prefix("now") {
        (None, rest)
    } else if let Some(i) = s.find("||") {
        (Some(&s[..i]), &s[i + 2..])
    } else {
        (Some(s), "")
    };
    let mut ms = match anchor {
        None => std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).ok()?.as_millis() as i64,
        Some(a) => {
            if let Ok(n) = a.trim().parse::<i64>() {
                n
            } else {
                crate::dates::parse_date_string(a)? as i64
            }
        }
    };
    let b = expr.as_bytes();
    let mut i = 0;
    while i < b.len() {
        match b[i] {
            b'/' => {
                let unit = *b.get(i + 1)?;
                ms = round_down(ms, unit)?;
                i += 2;
            }
            c @ (b'+' | b'-') => {
                i += 1;
                let start = i;
                while i < b.len() && b[i].is_ascii_digit() {
                    i += 1;
                }
                let n: i64 = expr[start..i].parse().ok()?;
                let unit = *b.get(i)?;
                i += 1;
                ms = add_unit(ms, unit, if c == b'-' { -n } else { n })?;
            }
            _ => return None,
        }
    }
    Some(ms)
}

fn add_unit(ms: i64, unit: u8, n: i64) -> Option<i64> {
    let fixed = match unit {
        b'w' => 7 * 86_400_000,
        b'd' => 86_400_000,
        b'h' | b'H' => 3_600_000,
        b'm' => 60_000,
        b's' => 1000,
        b'M' | b'y' => {
            let days = ms.div_euclid(86_400_000);
            let tod = ms.rem_euclid(86_400_000);
            let (y, m, d) = crate::calendar::civil_from_days(days);
            let months = y * 12 + (m - 1) + if unit == b'y' { 12 * n } else { n };
            let (ny, nm) = (months.div_euclid(12), months.rem_euclid(12) + 1);
            let (next_y, next_m) = if nm == 12 { (ny + 1, 1) } else { (ny, nm + 1) };
            let dim = crate::calendar::days_from_civil(next_y, next_m, 1) - crate::calendar::days_from_civil(ny, nm, 1);
            let nd = d.min(dim);
            return Some(crate::calendar::days_from_civil(ny, nm, nd) * 86_400_000 + tod);
        }
        _ => return None,
    };
    Some(ms + n * fixed)
}

fn round_down(ms: i64, unit: u8) -> Option<i64> {
    let floor = |step: i64| ms.div_euclid(step) * step;
    Some(match unit {
        b'd' => floor(86_400_000),
        b'h' | b'H' => floor(3_600_000),
        b'm' => floor(60_000),
        b's' => floor(1000),
        b'w' => {
            // weeks start on Monday; 1970-01-01 was a Thursday
            let days = ms.div_euclid(86_400_000);
            (days - (days + 3).rem_euclid(7)) * 86_400_000
        }
        b'M' | b'y' => {
            let (y, m, _) = crate::calendar::civil_from_days(ms.div_euclid(86_400_000));
            crate::calendar::days_from_civil(y, if unit == b'y' { 1 } else { m }, 1) * 86_400_000
        }
        _ => return None,
    })
}

/// TimeValue.parseTimeValue(s, name).getMillis().
fn time_value_millis(s: &str, name: &str) -> Result<i64, String> {
    let norm = s.trim().to_lowercase();
    let unit: Option<(&str, f64)> = if norm.ends_with("nanos") {
        Some(("nanos", 1e-6))
    } else if norm.ends_with("micros") {
        Some(("micros", 1e-3))
    } else if norm.ends_with("ms") {
        Some(("ms", 1.0))
    } else if norm.ends_with('s') {
        Some(("s", 1000.0))
    } else if s.trim().ends_with('m') {
        Some(("m", 60_000.0))
    } else if norm.ends_with('h') {
        Some(("h", 3_600_000.0))
    } else if norm.ends_with('d') {
        Some(("d", 86_400_000.0))
    } else {
        None
    };
    let Some((suffix, factor)) = unit else {
        if !norm.is_empty() && norm.trim_matches('0').is_empty() {
            return Ok(0);
        }
        return Err(format!(
            "failed to parse setting [{}] with value [{}] as a time value: unit is missing or unrecognized",
            name, s
        ));
    };
    let num = norm[..norm.len() - suffix.len()].trim();
    match num.parse::<i64>() {
        Ok(n) if n < -1 => Err(format!(
            "failed to parse setting [{}] with value [{}] as a time value: negative durations are not supported",
            name, s
        )),
        Ok(n) => Ok((n as f64 * factor) as i64),
        Err(_) if num.parse::<f64>().is_ok() => {
            Err(format!("failed to parse [{}], fractional time values are not supported", s))
        }
        Err(_) => Err(format!("failed to parse [{}]", s)),
    }
}
