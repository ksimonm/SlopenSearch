//! Geo field values: points (all accepted notations, geohashes), distances
//! with units, and geo shapes (GeoJSON and WKT).

use crate::jsonv::to_f64;
use serde_json::Value;

pub(crate) fn geo_point_of(v: &Value) -> Option<(f64, f64)> {
    match v {
        Value::Object(m) => {
            if let (Some(lat), Some(lon)) = (m.get("lat").and_then(to_f64), m.get("lon").and_then(to_f64)) {
                return Some((lat, lon));
            }
            let typ = m.get("type").and_then(|t| t.as_str()).unwrap_or("").to_ascii_lowercase();
            if typ == "point" {
                if let Some(coords) = m.get("coordinates").and_then(|c| c.as_array()) {
                    if coords.len() >= 2 {
                        return Some((to_f64(&coords[1])?, to_f64(&coords[0])?));
                    }
                }
            }
            None
        }
        Value::Array(a) => {
            if a.len() == 2 {
                let lon = to_f64(&a[0])?;
                let lat = to_f64(&a[1])?;
                return Some((lat, lon));
            }
            None
        }
        Value::String(s) => {
            let t = s.trim();
            if t.to_ascii_uppercase().starts_with("POINT") {
                let inner = t
                    .split_once('(')
                    .and_then(|(_, r)| r.rsplit_once(')'))
                    .map(|(l, _)| l)
                    .unwrap_or(t);
                let parts: Vec<&str> = inner.split_whitespace().collect();
                if parts.len() >= 2 {
                    let lon: f64 = parts[0].parse().ok()?;
                    let lat: f64 = parts[1].parse().ok()?;
                    return Some((lat, lon));
                }
                return None;
            }
            let parts: Vec<&str> = t.split(',').collect();
            if parts.len() == 2 {
                if let (Ok(lat), Ok(lon)) =
                    (parts[0].trim().parse::<f64>(), parts[1].trim().parse::<f64>())
                {
                    return Some((lat, lon));
                }
            }
            crate::geohash::decode(t)
        }
        _ => None,
    }
}

pub(crate) fn valid_latlon(lat: f64, lon: f64) -> bool {
    (-90.0..=90.0).contains(&lat) && (-180.0..=180.0).contains(&lon)
}

pub(crate) fn unit_to_meters(unit: &str) -> f64 {
    match unit.to_lowercase().as_str() {
        "mi" | "miles" => 1609.344,
        "yd" | "yards" => 0.9144,
        "ft" | "feet" => 0.3048,
        "in" | "inch" => 0.0254,
        "km" | "kilometers" => 1000.0,
        "cm" => 0.01,
        "mm" => 0.001,
        "nmi" | "nauticalmiles" => 1852.0,
        _ => 1.0,
    }
}

pub(crate) fn parse_distance_meters(v: &Value) -> Option<f64> {
    match v {
        Value::Number(n) => n.as_f64(),
        Value::String(t) => {
            let s = t.trim().to_lowercase();
            let bytes = s.as_bytes();
            let mut i = 0;
            while i < bytes.len() {
                let c = bytes[i] as char;
                if c.is_ascii_digit() || c == '.' || c == '-' || c == '+' || c == 'e' {
                    i += 1;
                } else {
                    break;
                }
            }
            let num: f64 = s[..i].trim().parse().ok()?;
            let unit = s[i..].trim();
            Some(num * unit_to_meters(unit))
        }
        _ => None,
    }
}

#[derive(Default, Clone)]
pub(crate) struct GeoShape {
    pub(crate) typ: String,
    pub(crate) lat: f64,
    pub(crate) lon: f64,
    pub(crate) radius_m: f64,
    pub(crate) top: f64,
    pub(crate) left: f64,
    pub(crate) bot: f64,
    pub(crate) right: f64,
    pub(crate) points: Vec<(f64, f64)>,
}

pub(crate) fn parse_geo_shape(v: &Value) -> Option<GeoShape> {
    if let Some(s) = v.as_str() {
        return parse_wkt_shape(s);
    }
    let m = v.as_object()?;
    let typ = m
        .get("type")
        .and_then(|t| t.as_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    let mut gs = GeoShape {
        typ: typ.clone(),
        ..Default::default()
    };
    match typ.as_str() {
        "point" => {
            match m.get("coordinates") {
                Some(Value::Array(coords)) => {
                    if coords.len() >= 2 {
                        gs.lon = to_f64(&coords[0]).unwrap_or(0.0);
                        gs.lat = to_f64(&coords[1]).unwrap_or(0.0);
                    }
                }
                Some(Value::String(s)) => {
                    if let Some((lat, lon)) = geo_point_of(&Value::String(s.clone())) {
                        gs.lat = lat;
                        gs.lon = lon;
                    }
                }
                _ => {}
            }
            Some(gs)
        }
        "circle" => {
            if let Some(Value::String(r)) = m.get("radius") {
                gs.radius_m = parse_distance_meters(&Value::String(r.clone())).unwrap_or(0.0);
            } else if let Some(r) = m.get("radius").and_then(to_f64) {
                gs.radius_m = r;
            }
            if let Some(coords) = m.get("coordinates").and_then(|c| c.as_array()) {
                if coords.len() >= 2 {
                    gs.lon = to_f64(&coords[0]).unwrap_or(0.0);
                    gs.lat = to_f64(&coords[1]).unwrap_or(0.0);
                }
            }
            Some(gs)
        }
        "envelope" => {
            if let Some(coords) = m.get("coordinates").and_then(|c| c.as_array()) {
                if coords.len() >= 2 {
                    if let Some(tl) = coords[0].as_array() {
                        if tl.len() >= 2 {
                            gs.left = to_f64(&tl[0]).unwrap_or(0.0);
                            gs.top = to_f64(&tl[1]).unwrap_or(0.0);
                        }
                    }
                    if let Some(br) = coords[1].as_array() {
                        if br.len() >= 2 {
                            gs.right = to_f64(&br[0]).unwrap_or(0.0);
                            gs.bot = to_f64(&br[1]).unwrap_or(0.0);
                        }
                    }
                }
            }
            Some(gs)
        }
        "polygon" => {
            if let Some(rings) = m.get("coordinates").and_then(|c| c.as_array()) {
                if !rings.is_empty() {
                    if let Some(ring) = rings[0].as_array() {
                        for pt in ring {
                            if let Some(arr) = pt.as_array() {
                                if arr.len() >= 2 {
                                    let lon = to_f64(&arr[0]).unwrap_or(0.0);
                                    let lat = to_f64(&arr[1]).unwrap_or(0.0);
                                    gs.points.push((lat, lon));
                                }
                            }
                        }
                    }
                }
            }
            Some(gs)
        }
        "linestring" => {
            if let Some(coords) = m.get("coordinates").and_then(|c| c.as_array()) {
                for pt in coords {
                    if let Some(arr) = pt.as_array() {
                        if arr.len() >= 2 {
                            let lon = to_f64(&arr[0]).unwrap_or(0.0);
                            let lat = to_f64(&arr[1]).unwrap_or(0.0);
                            gs.points.push((lat, lon));
                        }
                    }
                }
            }
            Some(gs)
        }
        "multipoint" => {
            if let Some(coords) = m.get("coordinates").and_then(|c| c.as_array()) {
                for pt in coords {
                    if let Some(arr) = pt.as_array() {
                        if arr.len() >= 2 {
                            gs.points.push((to_f64(&arr[1]).unwrap_or(0.0), to_f64(&arr[0]).unwrap_or(0.0)));
                        }
                    }
                }
            }
            Some(gs)
        }
        "multilinestring" | "multipolygon" => {
            fn collect(v: &Value, out: &mut Vec<(f64, f64)>) {
                if let Some(a) = v.as_array() {
                    if a.len() >= 2 && a.iter().all(|x| x.is_number() || x.is_string()) {
                        let lon = to_f64(&a[0]).unwrap_or(0.0);
                        let lat = to_f64(&a[1]).unwrap_or(0.0);
                        out.push((lat, lon));
                    } else {
                        for e in a {
                            collect(e, out);
                        }
                    }
                }
            }
            if let Some(coords) = m.get("coordinates") {
                collect(coords, &mut gs.points);
            }
            Some(gs)
        }
        "geometrycollection" => {
            let mut out = Vec::new();
            if let Some(geoms) = m.get("geometries").and_then(|g| g.as_array()) {
                for g in geoms {
                    if let Some(child) = parse_geo_shape(g) {
                        match child.typ.as_str() {
                            "point" => out.push((child.lat, child.lon)),
                            "envelope" => {
                                out.push((child.top, child.left));
                                out.push((child.bot, child.right));
                            }
                            _ => out.extend(child.points),
                        }
                    }
                }
            }
            gs.typ = "multipoint".to_string();
            gs.points = out;
            Some(gs)
        }
        _ => None,
    }
}

pub(crate) fn parse_wkt_shape(s: &str) -> Option<GeoShape> {
    let t = s.trim();
    let up = t.to_ascii_uppercase();
    let inner = t
        .split_once('(')
        .and_then(|(_, r)| r.rsplit_once(')'))
        .map(|(l, _)| l.trim())
        .unwrap_or(t);
    if up.starts_with("POINT") {
        let parts: Vec<&str> = inner.split_whitespace().collect();
        if parts.len() >= 2 {
            let lon: f64 = parts[0].parse().ok()?;
            let lat: f64 = parts[1].parse().ok()?;
            return Some(GeoShape {
                typ: "point".to_string(),
                lat,
                lon,
                ..Default::default()
            });
        }
        return None;
    }
    if up.starts_with("BBOX") {
        let nums: Vec<f64> = inner
            .split(',')
            .filter_map(|p| p.trim().parse().ok())
            .collect();
        if nums.len() >= 4 {
            return Some(GeoShape {
                typ: "envelope".to_string(),
                left: nums[0],
                right: nums[1],
                top: nums[2],
                bot: nums[3],
                ..Default::default()
            });
        }
        return None;
    }
    if up.starts_with("MULTIPOINT") {
        let body = inner.trim().trim_start_matches('(').trim_end_matches(')');
        let mut points = Vec::new();
        for pt in body.split(',') {
            let nums: Vec<f64> = pt
                .trim()
                .trim_matches(|c| c == '(' || c == ')')
                .split_whitespace()
                .filter_map(|p| p.parse().ok())
                .collect();
            if nums.len() >= 2 {
                points.push((nums[1], nums[0]));
            }
        }
        if points.is_empty() {
            return None;
        }
        return Some(GeoShape {
            typ: "multipoint".to_string(),
            points,
            ..Default::default()
        });
    }
    if up.starts_with("POLYGON") || up.starts_with("LINESTRING") {
        let body = inner.trim().trim_start_matches('(').trim_end_matches(')');
        let ring = body.split(')').next().unwrap_or(body);
        let mut points = Vec::new();
        for pt in ring.split(',') {
            let nums: Vec<f64> = pt
                .trim()
                .trim_matches(|c| c == '(' || c == ')')
                .split_whitespace()
                .filter_map(|p| p.parse().ok())
                .collect();
            if nums.len() >= 2 {
                points.push((nums[1], nums[0]));
            }
        }
        if points.is_empty() {
            return None;
        }
        return Some(GeoShape {
            typ: if up.starts_with("LINESTRING") {
                "linestring".to_string()
            } else {
                "polygon".to_string()
            },
            points,
            ..Default::default()
        });
    }
    None
}
