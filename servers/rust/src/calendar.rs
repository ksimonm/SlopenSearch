//! The proleptic Gregorian calendar and time zones with `java.time` semantics.
//!
//! Zone rules come from the IANA tz database bundled with `jiff`; the ID syntax,
//! the IDs Java does not know and the exception messages follow `ZoneId.of` and
//! `ZoneOffset.of`. The day-number arithmetic stays hand-written: Java dates span
//! years far beyond the ±9999 that `jiff`'s civil types cover.

use jiff::civil::DateTime;
use jiff::tz::{AmbiguousOffset, Offset, TimeZone};
use jiff::Timestamp;

/// Days since 1970-01-01 of a proleptic Gregorian date.
pub fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146097 + doe - 719468
}

/// The (year, month, day) of a day number since 1970-01-01.
pub fn civil_from_days(z: i64) -> (i64, i64, i64) {
    let z = z + 719468;
    let era = if z >= 0 { z } else { z - 146096 } / 146097;
    let doe = z - era * 146097;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    (if m <= 2 { y + 1 } else { y }, m, d)
}

/// A failed `ZoneId.of`: the OpenSearch error type of the Java exception
/// (`zone_rules_exception` or `date_time_exception`) and its message.
#[derive(Debug, Clone)]
pub struct ZoneError {
    pub kind: &'static str,
    pub message: String,
}

impl ZoneError {
    /// The Java exception class, which OpenSearch puts in front of some messages.
    pub fn java_class(&self) -> &'static str {
        match self.kind {
            "zone_rules_exception" => "java.time.zone.ZoneRulesException",
            _ => "java.time.DateTimeException",
        }
    }
    fn unknown(id: &str) -> Self {
        ZoneError { kind: "zone_rules_exception", message: format!("Unknown time-zone ID: {}", id) }
    }
    fn invalid(message: String) -> Self {
        ZoneError { kind: "date_time_exception", message }
    }
}

/// A `java.time.ZoneId`: a fixed offset or a region with tz database rules.
#[derive(Debug, Clone)]
pub struct Zone {
    tz: TimeZone,
    fixed: Option<i32>,
}

/// IDs the tz database has but Java's tzdb.dat leaves out.
const NOT_IN_JAVA: &[&str] = &["EST", "MST", "HST", "EST5EDT", "CST6CDT", "MST7MDT", "PST8PDT", "ROC"];

/// `ZoneOffset.of`: `Z`, `±h`, `±hh`, `±hh:mm`, `±hhmm`, `±hh:mm:ss`, `±hhmmss`;
/// returns the total seconds.
pub fn parse_offset_id(id: &str) -> Result<i32, ZoneError> {
    if id == "Z" {
        return Ok(0);
    }
    let bad = || ZoneError::invalid(format!("Invalid ID for ZoneOffset, invalid format: {}", id));
    let b = id.as_bytes();
    let neg = match b.first() {
        Some(b'+') => false,
        Some(b'-') => true,
        _ => return Err(bad()),
    };
    // `+h` is read as `+0h`
    let padded = if b.len() == 2 { format!("{}0{}", &id[..1], &id[1..]) } else { id.to_string() };
    let b = padded.as_bytes();
    let two = |at: usize, colon: bool| -> Result<i32, ZoneError> {
        if colon && b.get(at - 1) != Some(&b':') {
            return Err(ZoneError::invalid(format!(
                "Invalid ID for ZoneOffset, colon not found when expected: {}",
                padded
            )));
        }
        match (b.get(at), b.get(at + 1)) {
            (Some(x), Some(y)) if x.is_ascii_digit() && y.is_ascii_digit() => Ok(((x - b'0') * 10 + (y - b'0')) as i32),
            _ => Err(ZoneError::invalid(format!(
                "Invalid ID for ZoneOffset, non numeric characters found: {}",
                padded
            ))),
        }
    };
    let (h, m, s) = match b.len() {
        3 => (two(1, false)?, 0, 0),
        5 => (two(1, false)?, two(3, false)?, 0),
        6 => (two(1, false)?, two(4, true)?, 0),
        7 => (two(1, false)?, two(3, false)?, two(5, false)?),
        9 => (two(1, false)?, two(4, true)?, two(7, true)?),
        _ => return Err(bad()),
    };
    if h > 18 {
        let v = if neg { -h } else { h };
        return Err(ZoneError::invalid(format!(
            "Zone offset hours not in valid range: value {} is not in the range -18 to 18",
            v
        )));
    }
    if m > 59 {
        let v = if neg { -m } else { m };
        return Err(ZoneError::invalid(format!(
            "Zone offset minutes not in valid range: value {} is not in the range -59 to 59",
            v
        )));
    }
    if s > 59 {
        let v = if neg { -s } else { s };
        return Err(ZoneError::invalid(format!(
            "Zone offset seconds not in valid range: value {} is not in the range -59 to 59",
            v
        )));
    }
    if h == 18 && (m != 0 || s != 0) {
        return Err(ZoneError::invalid("Zone offset not in valid range: -18:00 to +18:00".to_string()));
    }
    let total = h * 3600 + m * 60 + s;
    Ok(if neg { -total } else { total })
}

/// The `ZoneOffset` ID of an offset in seconds: `Z`, `+05:30`, `-00:00:30`.
pub fn offset_id(secs: i32) -> String {
    if secs == 0 {
        return "Z".to_string();
    }
    let a = secs.unsigned_abs();
    let sign = if secs < 0 { '-' } else { '+' };
    let (h, m, s) = (a / 3600, a / 60 % 60, a % 60);
    if s == 0 {
        format!("{}{:02}:{:02}", sign, h, m)
    } else {
        format!("{}{:02}:{:02}:{:02}", sign, h, m, s)
    }
}

impl Zone {
    /// `ZoneId.of(id)`.
    pub fn of(id: &str) -> Result<Zone, ZoneError> {
        let fixed = |secs: i32| Zone {
            tz: TimeZone::fixed(Offset::from_seconds(secs).unwrap_or(Offset::UTC)),
            fixed: Some(secs),
        };
        if id == "Z" || id.starts_with('+') || id.starts_with('-') {
            let secs = parse_offset_id(id)?;
            return Ok(fixed(secs));
        }
        for prefix in ["UTC", "GMT", "UT"] {
            if id == prefix {
                return Ok(fixed(0));
            }
            if let Some(rest) = id.strip_prefix(prefix) {
                if rest.starts_with('+') || rest.starts_with('-') {
                    let secs = parse_offset_id(rest)
                        .map_err(|_| ZoneError::invalid(format!("Invalid ID for offset-based ZoneId: {}", id)))?;
                    return Ok(fixed(secs));
                }
            }
        }
        let well_formed = id.len() >= 2
            && id.as_bytes()[0].is_ascii_alphabetic()
            && id.bytes().all(|c| c.is_ascii_alphanumeric() || matches!(c, b'~' | b'/' | b'.' | b'_' | b'+' | b'-'));
        if !well_formed {
            return Err(ZoneError::invalid(format!("Invalid ID for region-based ZoneId, invalid format: {}", id)));
        }
        if NOT_IN_JAVA.contains(&id) {
            return Err(ZoneError::unknown(id));
        }
        match TimeZone::get(id) {
            // the tz database lookup ignores case, Java's does not
            Ok(tz) if tz.iana_name() == Some(id) => {
                let fixed = match tz.to_fixed_offset() {
                    Ok(o) => Some(o.seconds()),
                    Err(_) => None,
                };
                Ok(Zone { tz, fixed })
            }
            _ => Err(ZoneError::unknown(id)),
        }
    }

    /// `Zone::of` through a per-thread cache (hot loops resolve the same IDs).
    pub fn cached(id: &str) -> Result<Zone, ZoneError> {
        thread_local! {
            static CACHE: std::cell::RefCell<std::collections::HashMap<String, Result<Zone, ZoneError>>> =
                Default::default();
        }
        CACHE.with(|c| {
            if let Some(z) = c.borrow().get(id) {
                return z.clone();
            }
            let z = Zone::of(id);
            let mut c = c.borrow_mut();
            if c.len() > 256 {
                c.clear();
            }
            c.insert(id.to_string(), z.clone());
            z
        })
    }

    /// UTC.
    pub fn utc() -> Zone {
        Zone { tz: TimeZone::UTC, fixed: Some(0) }
    }

    /// The offset in seconds when the zone has one fixed offset.
    pub fn fixed_offset(&self) -> Option<i32> {
        self.fixed
    }

    /// The offset in seconds at an instant (epoch seconds).
    pub fn offset_at(&self, epoch_sec: i64) -> i32 {
        if let Some(f) = self.fixed {
            return f;
        }
        let sec = epoch_sec.clamp(Timestamp::MIN.as_second(), Timestamp::MAX.as_second());
        let ts = Timestamp::from_second(sec).unwrap_or(Timestamp::UNIX_EPOCH);
        self.tz.to_offset(ts).seconds()
    }

    /// The offset in milliseconds at an instant (epoch milliseconds).
    pub fn offset_ms(&self, epoch_ms: i64) -> i64 {
        self.offset_at(epoch_ms.div_euclid(1000)) as i64 * 1000
    }

    /// The instant (epoch seconds) of a local date-time given as seconds since
    /// 1970-01-01T00:00 local time, resolved like `ZonedDateTime.ofLocal`: a time
    /// in a gap moves forward by the gap's length, an overlap takes the earlier
    /// offset.
    pub fn local_to_epoch(&self, local_sec: i64) -> i64 {
        if let Some(f) = self.fixed {
            return local_sec - f as i64;
        }
        let days = local_sec.div_euclid(86_400);
        let sod = local_sec.rem_euclid(86_400);
        let (y, m, d) = civil_from_days(days);
        let dt = if (-9998..=9998).contains(&y) {
            DateTime::new(y as i16, m as i8, d as i8, (sod / 3600) as i8, (sod / 60 % 60) as i8, (sod % 60) as i8, 0).ok()
        } else {
            None
        };
        match dt.and_then(|dt| self.tz.to_ambiguous_timestamp(dt).compatible().ok()) {
            Some(ts) => ts.as_second(),
            None => local_sec - self.offset_at(local_sec) as i64,
        }
    }

    fn local_datetime(local_ms: i64) -> Option<DateTime> {
        let days = local_ms.div_euclid(86_400_000);
        let msod = local_ms.rem_euclid(86_400_000);
        let (y, m, d) = civil_from_days(days);
        if !(-9998..=9998).contains(&y) {
            return None;
        }
        let (h, mi, s, ms) = (msod / 3_600_000, msod / 60_000 % 60, msod / 1000 % 60, msod % 1000);
        DateTime::new(y as i16, m as i8, d as i8, h as i8, mi as i8, s as i8, (ms * 1_000_000) as i32).ok()
    }

    /// `ZoneRules.getValidOffsets` of a local date-time (ms since 1970-01-01T00:00
    /// local): one offset, none in a gap, the earlier and the later one in an overlap.
    pub fn valid_offsets(&self, local_ms: i64) -> Vec<i32> {
        if let Some(f) = self.fixed {
            return vec![f];
        }
        let Some(dt) = Self::local_datetime(local_ms) else {
            return vec![self.offset_at(local_ms.div_euclid(1000))];
        };
        match self.tz.to_ambiguous_timestamp(dt).offset() {
            AmbiguousOffset::Unambiguous { offset } => vec![offset.seconds()],
            AmbiguousOffset::Gap { .. } => vec![],
            AmbiguousOffset::Fold { before, after } => vec![before.seconds(), after.seconds()],
        }
    }

    /// The latest offset transition at or before an instant, in epoch ms.
    pub fn transition_at_or_before(&self, epoch_ms: i64) -> Option<i64> {
        if self.fixed.is_some() {
            return None;
        }
        let ts = Timestamp::from_millisecond(epoch_ms.saturating_add(1)).ok()?;
        self.tz.preceding(ts).next().map(|t| t.timestamp().as_millisecond())
    }

    /// `ZoneRules.getTransition(local)` for a local time in a gap: the instant of
    /// the transition that skipped it, in epoch ms.
    pub fn gap_transition(&self, local_ms: i64) -> i64 {
        // local - offset_before lies at or after the transition, local - offset_after before it
        let before = match Self::local_datetime(local_ms).map(|dt| self.tz.to_ambiguous_timestamp(dt).offset()) {
            Some(AmbiguousOffset::Gap { before, .. }) => before.seconds() as i64 * 1000,
            _ => self.offset_ms(local_ms),
        };
        let probe = local_ms - before;
        self.transition_at_or_before(probe).unwrap_or(probe)
    }

    /// `local_to_epoch` in milliseconds.
    pub fn local_to_epoch_ms(&self, local_ms: i64) -> i64 {
        self.local_to_epoch(local_ms.div_euclid(1000)) * 1000 + local_ms.rem_euclid(1000)
    }
}
