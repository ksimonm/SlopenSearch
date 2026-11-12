// Painless engine: the java.time subset. Zones are UTC or fixed offsets.

use std::rc::Rc;

use crate::pl_value::{i64_of, iae, jstr, npe, RtErr, PV, R};

pub const DAY_NAMES: [&str; 7] = ["MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"];
pub const MONTH_NAMES: [&str; 12] = [
    "JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER",
    "DECEMBER",
];

const NANOS_PER_SEC: i128 = 1_000_000_000;

/// ZonedDateTime: an instant plus a fixed offset (and the zone id it was
/// given with, if any).
#[derive(Clone)]
pub struct Zdt {
    pub sec: i64,
    pub nanos: u32,
    pub offset: i32,
    pub zone: Option<String>,
}

pub struct Dtf {
    pub pattern: String,
    pub iso: bool,
}

impl Dtf {
    pub fn java_string(&self) -> String {
        if self.iso {
            return "ParseCaseSensitive(false)(Value(Year,4,10,EXCEEDS_PAD)'-'Value(MonthOfYear,2)'-'Value(DayOfMonth,2))'T'(Value(HourOfDay,2)':'Value(MinuteOfHour,2)[':'Value(SecondOfMinute,2)[Fraction(NanoOfSecond,0,9,DecimalPoint)]])".into();
        }
        self.pattern.clone()
    }
}

pub fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let mp = (m + 9) % 12;
    let doy = (153 * mp + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146097 + doe - 719468
}

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

fn is_leap(y: i64) -> bool {
    y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)
}

fn month_len(y: i64, m: i64) -> i64 {
    match m {
        2 => {
            if is_leap(y) {
                29
            } else {
                28
            }
        }
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    }
}

/// Local wall-clock fields of a ZonedDateTime.
pub struct Fields {
    pub year: i64,
    pub month: i64,
    pub day: i64,
    pub hour: i64,
    pub minute: i64,
    pub second: i64,
    pub nanos: u32,
    pub epoch_day: i64,
}

impl Zdt {
    pub fn utc(sec: i64, nanos: u32) -> Self {
        Zdt { sec, nanos, offset: 0, zone: None }
    }
    pub fn from_millis(ms: i64) -> Self {
        Zdt::utc(ms.div_euclid(1000), (ms.rem_euclid(1000) * 1_000_000) as u32)
    }
    pub fn instant_nanos(&self) -> i128 {
        self.sec as i128 * NANOS_PER_SEC + self.nanos as i128
    }
    pub fn fields(&self) -> Fields {
        let local = self.sec + self.offset as i64;
        let days = local.div_euclid(86400);
        let sod = local.rem_euclid(86400);
        let (y, m, d) = civil_from_days(days);
        Fields { year: y, month: m, day: d, hour: sod / 3600, minute: (sod % 3600) / 60, second: sod % 60, nanos: self.nanos, epoch_day: days }
    }
    /// Builds a value with the same offset from local fields.
    fn with_local(&self, y: i64, m: i64, d: i64, sod: i64, nanos: u32) -> Zdt {
        let local = days_from_civil(y, m, d) * 86400 + sod;
        Zdt { sec: local - self.offset as i64, nanos, offset: self.offset, zone: self.zone.clone() }
    }
    pub fn to_string_java(&self) -> String {
        let f = self.fields();
        let mut s = format!("{}-{:02}-{:02}T{:02}:{:02}", fmt_year(f.year), f.month, f.day, f.hour, f.minute);
        if f.second != 0 || f.nanos != 0 {
            s.push_str(&format!(":{:02}{}", f.second, fmt_nanos(f.nanos)));
        }
        s.push_str(&fmt_offset(self.offset));
        if let Some(z) = &self.zone {
            if z != "Z" && z != "UTC" && !z.starts_with('+') && !z.starts_with('-') {
                s.push_str(&format!("[{}]", z));
            }
        }
        s
    }
}

fn fmt_year(y: i64) -> String {
    if y > 9999 {
        format!("+{}", y)
    } else {
        format!("{:04}", y)
    }
}

fn fmt_offset(off: i32) -> String {
    if off == 0 {
        return "Z".into();
    }
    let sign = if off < 0 { '-' } else { '+' };
    let a = off.abs();
    format!("{}{:02}:{:02}", sign, a / 3600, (a % 3600) / 60)
}

fn fmt_nanos(n: u32) -> String {
    if n == 0 {
        String::new()
    } else if n % 1_000_000 == 0 {
        format!(".{:03}", n / 1_000_000)
    } else if n % 1000 == 0 {
        format!(".{:06}", n / 1000)
    } else {
        format!(".{:09}", n)
    }
}

pub fn fmt_local_date(days: i64) -> String {
    let (y, m, d) = civil_from_days(days);
    format!("{}-{:02}-{:02}", fmt_year(y), m, d)
}

pub fn fmt_instant(ms: i64, nanos: i64) -> String {
    let z = Zdt::utc(ms.div_euclid(1000), (ms.rem_euclid(1000) * 1_000_000 + nanos) as u32);
    let f = z.fields();
    format!(
        "{}-{:02}-{:02}T{:02}:{:02}:{:02}{}Z",
        fmt_year(f.year),
        f.month,
        f.day,
        f.hour,
        f.minute,
        f.second,
        fmt_nanos(f.nanos)
    )
}

pub fn fmt_duration(d: i128) -> String {
    if d == 0 {
        return "PT0S".into();
    }
    let neg = d < 0;
    let a = d.abs();
    let sign = if neg { "-" } else { "" };
    let h = a / (3600 * NANOS_PER_SEC);
    let m = (a % (3600 * NANOS_PER_SEC)) / (60 * NANOS_PER_SEC);
    let rest = a % (60 * NANOS_PER_SEC);
    let mut s = String::from("PT");
    if h != 0 {
        s.push_str(&format!("{}{}H", sign, h));
    }
    if m != 0 {
        s.push_str(&format!("{}{}M", sign, m));
    }
    if rest != 0 {
        let whole = rest / NANOS_PER_SEC;
        let frac = (rest % NANOS_PER_SEC) as u32;
        s.push_str(&format!("{}{}{}S", sign, whole, fmt_nanos(frac).trim_end_matches('0')));
    }
    s
}

fn parse_err(s: &str, off: usize) -> RtErr {
    RtErr::new("date_time_parse_exception", &format!("Text '{}' could not be parsed at index 0", s), off)
}

fn num(s: &str) -> Option<i64> {
    if s.is_empty() || !s.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    s.parse().ok()
}

/// Parses an ISO date "yyyy-MM-dd" to its epoch day.
fn parse_date(s: &str) -> Option<i64> {
    let p: Vec<&str> = s.split('-').collect();
    if p.len() != 3 || p[0].len() < 4 || p[1].len() != 2 || p[2].len() != 2 {
        return None;
    }
    let (y, m, d) = (num(p[0])?, num(p[1])?, num(p[2])?);
    if !(1..=12).contains(&m) || d < 1 || d > month_len(y, m) {
        return None;
    }
    Some(days_from_civil(y, m, d))
}

/// Parses "HH:mm[:ss[.fff]]" to seconds of day and nanos.
fn parse_time(s: &str) -> Option<(i64, u32)> {
    let (main, frac) = match s.split_once('.') {
        Some((a, b)) => (a, Some(b)),
        None => (s, None),
    };
    let p: Vec<&str> = main.split(':').collect();
    if p.len() < 2 || p.len() > 3 || p.iter().any(|x| x.len() != 2) {
        return None;
    }
    let h = num(p[0])?;
    let m = num(p[1])?;
    let sec = if p.len() == 3 { num(p[2])? } else { 0 };
    if h > 23 || m > 59 || sec > 59 {
        return None;
    }
    let mut nanos = 0u32;
    if let Some(f) = frac {
        if f.is_empty() || f.len() > 9 || p.len() != 3 {
            return None;
        }
        nanos = (num(f)? * 10i64.pow(9 - f.len() as u32)) as u32;
    }
    Some((h * 3600 + m * 60 + sec, nanos))
}

fn parse_offset(s: &str) -> Option<i32> {
    if s == "Z" {
        return Some(0);
    }
    let sign = match s.as_bytes().first()? {
        b'+' => 1,
        b'-' => -1,
        _ => return None,
    };
    let body = &s[1..];
    let (h, m) = match body.split_once(':') {
        Some((h, m)) => (num(h)?, num(m)?),
        None if body.len() == 4 => (num(&body[..2])?, num(&body[2..])?),
        None if body.len() == 2 => (num(body)?, 0),
        _ => return None,
    };
    Some(sign * (h * 3600 + m * 60) as i32)
}

pub fn parse_zdt(s: &str, off: usize) -> R<Zdt> {
    let mut text = s;
    let mut zone = None;
    if let Some(i) = s.find('[') {
        if s.ends_with(']') {
            zone = Some(s[i + 1..s.len() - 1].to_string());
            text = &s[..i];
        }
    }
    let (date, rest) = text.split_once('T').ok_or_else(|| parse_err(s, off))?;
    let day = parse_date(date).ok_or_else(|| parse_err(s, off))?;
    let zpos = rest.find(['Z', '+', '-']).ok_or_else(|| parse_err(s, off))?;
    let (sod, nanos) = parse_time(&rest[..zpos]).ok_or_else(|| parse_err(s, off))?;
    let offset = parse_offset(&rest[zpos..]).ok_or_else(|| parse_err(s, off))?;
    let local = day * 86400 + sod;
    Ok(Zdt { sec: local - offset as i64, nanos, offset, zone })
}

fn zone_offset(z: &str) -> Option<i32> {
    match z {
        "Z" | "UTC" | "GMT" | "UT" => Some(0),
        _ => parse_offset(z.trim_start_matches("UTC").trim_start_matches("GMT")),
    }
}

fn zdt_in_zone(sec: i64, nanos: u32, z: &str) -> Zdt {
    match zone_offset(z) {
        Some(o) => Zdt { sec, nanos, offset: o, zone: if o == 0 { None } else { Some(z.to_string()) } },
        None => Zdt { sec, nanos, offset: 0, zone: Some(z.to_string()) },
    }
}

fn duration_unit(name: &str) -> Option<i128> {
    Some(match name {
        "NANOS" => 1,
        "MICROS" => 1000,
        "MILLIS" => 1_000_000,
        "SECONDS" => NANOS_PER_SEC,
        "MINUTES" => 60 * NANOS_PER_SEC,
        "HOURS" => 3600 * NANOS_PER_SEC,
        "HALF_DAYS" => 12 * 3600 * NANOS_PER_SEC,
        "DAYS" => 86400 * NANOS_PER_SEC,
        "WEEKS" => 7 * 86400 * NANOS_PER_SEC,
        "MONTHS" | "YEARS" | "DECADES" | "CENTURIES" | "MILLENNIA" => 0,
        _ => return None,
    })
}

pub fn is_chrono_unit(name: &str) -> bool {
    duration_unit(name).is_some()
}

fn chrono_of(v: &PV) -> String {
    match v {
        PV::ChronoUnit(u) => u.to_string(),
        other => jstr(other).to_uppercase(),
    }
}

fn add_months(y: i64, m: i64, d: i64, n: i64) -> (i64, i64, i64) {
    let total = y * 12 + (m - 1) + n;
    let ny = total.div_euclid(12);
    let nm = total.rem_euclid(12) + 1;
    (ny, nm, d.min(month_len(ny, nm)))
}

fn add_unit(z: &Zdt, unit: &str, n: i64) -> Zdt {
    let f = z.fields();
    let sod = f.hour * 3600 + f.minute * 60 + f.second;
    match unit {
        "YEARS" | "MONTHS" | "DECADES" | "CENTURIES" | "MILLENNIA" => {
            let months = match unit {
                "YEARS" => 12,
                "DECADES" => 120,
                "CENTURIES" => 1200,
                "MILLENNIA" => 12000,
                _ => 1,
            } * n;
            let (y, m, d) = add_months(f.year, f.month, f.day, months);
            z.with_local(y, m, d, sod, f.nanos)
        }
        "WEEKS" | "DAYS" => {
            let days = if unit == "WEEKS" { 7 * n } else { n };
            let (y, m, d) = civil_from_days(f.epoch_day + days);
            z.with_local(y, m, d, sod, f.nanos)
        }
        _ => {
            let total = z.instant_nanos() + duration_unit(unit).unwrap_or(0) * n as i128;
            Zdt {
                sec: total.div_euclid(NANOS_PER_SEC) as i64,
                nanos: total.rem_euclid(NANOS_PER_SEC) as u32,
                offset: z.offset,
                zone: z.zone.clone(),
            }
        }
    }
}

fn add_nanos(z: &Zdt, d: i128) -> Zdt {
    let total = z.instant_nanos() + d;
    Zdt {
        sec: total.div_euclid(NANOS_PER_SEC) as i64,
        nanos: total.rem_euclid(NANOS_PER_SEC) as u32,
        offset: z.offset,
        zone: z.zone.clone(),
    }
}

/// The instant of a temporal value as a UTC ZonedDateTime-like value.
fn temporal(v: &PV) -> Option<Zdt> {
    match v {
        PV::Zdt(z) => Some((**z).clone()),
        PV::LocalDate(d) => Some(Zdt::utc(d * 86400, 0)),
        PV::Instant(ms, nanos) => {
            let z = Zdt::from_millis(*ms);
            Some(Zdt::utc(z.sec, z.nanos + *nanos as u32))
        }
        _ => None,
    }
}

fn months_between(a: &Zdt, b: &Zdt) -> i64 {
    let (fa, fb) = (a.fields(), b.fields());
    let mut months = (fb.year - fa.year) * 12 + (fb.month - fa.month);
    let rest = |f: &Fields| (f.day, f.hour, f.minute, f.second, f.nanos);
    // a partial month does not count
    if months > 0 && rest(&fb) < rest(&fa) {
        months -= 1;
    } else if months < 0 && rest(&fb) > rest(&fa) {
        months += 1;
    }
    months
}

fn title(name: &str, n: usize) -> String {
    let mut s = name[..1].to_string();
    s.push_str(&name[1..n.min(name.len())].to_lowercase());
    s
}

/// Renders local fields with a java.time DateTimeFormatter pattern.
pub fn format_pattern(p: &str, z: &Zdt) -> String {
    let f = z.fields();
    let b = p.as_bytes();
    let mut out = String::new();
    let mut i = 0;
    while i < b.len() {
        let c = b[i];
        if c == b'\'' {
            match p[i + 1..].find('\'') {
                None => {
                    out.push_str(&p[i + 1..]);
                    break;
                }
                Some(0) => out.push('\''),
                Some(j) => out.push_str(&p[i + 1..i + 1 + j]),
            }
            i += p[i + 1..].find('\'').unwrap() + 2;
            continue;
        }
        if !c.is_ascii_alphabetic() {
            let ch = p[i..].chars().next().unwrap();
            out.push(ch);
            i += ch.len_utf8();
            continue;
        }
        let mut j = i;
        while j < b.len() && b[j] == c {
            j += 1;
        }
        let n = j - i;
        let pad = |v: i64| format!("{:0width$}", v, width = n);
        match c {
            b'y' | b'u' => {
                if n == 2 {
                    out.push_str(&format!("{:02}", f.year.rem_euclid(100)));
                } else {
                    out.push_str(&pad(f.year));
                }
            }
            b'M' | b'L' => {
                let name = MONTH_NAMES[(f.month - 1) as usize];
                if n >= 4 {
                    out.push_str(&title(name, name.len()));
                } else if n == 3 {
                    out.push_str(&title(name, 3));
                } else {
                    out.push_str(&pad(f.month));
                }
            }
            b'd' => out.push_str(&pad(f.day)),
            b'D' => {
                let doy = f.epoch_day - days_from_civil(f.year, 1, 1) + 1;
                out.push_str(&pad(doy));
            }
            b'H' => out.push_str(&pad(f.hour)),
            b'h' => {
                let h = if f.hour % 12 == 0 { 12 } else { f.hour % 12 };
                out.push_str(&pad(h));
            }
            b'm' => out.push_str(&pad(f.minute)),
            b's' => out.push_str(&pad(f.second)),
            b'S' => {
                let frac = format!("{:09}", f.nanos);
                out.push_str(&frac[..n.min(9)]);
            }
            b'a' => out.push_str(if f.hour < 12 { "AM" } else { "PM" }),
            b'E' => {
                let name = DAY_NAMES[day_of_week(f.epoch_day) as usize - 1];
                out.push_str(&title(name, if n >= 4 { name.len() } else { 3 }));
            }
            b'X' | b'x' | b'Z' => {
                if z.offset == 0 && c == b'X' {
                    out.push('Z');
                } else {
                    let a = z.offset.abs();
                    let sign = if z.offset < 0 { '-' } else { '+' };
                    out.push_str(&format!("{}{:02}{:02}", sign, a / 3600, (a % 3600) / 60));
                }
            }
            b'z' | b'V' => out.push_str(z.zone.as_deref().unwrap_or("Z")),
            _ => out.push_str(&p[i..j]),
        }
        i = j;
    }
    out
}

/// 1 = MONDAY
fn day_of_week(epoch_day: i64) -> i64 {
    (epoch_day + 3).rem_euclid(7) + 1
}

fn zdt(z: Zdt) -> PV {
    PV::Zdt(Rc::new(z))
}

fn arg_str(args: &[PV], i: usize) -> String {
    jstr(&args[i])
}

pub fn call_static_time(class: &str, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let i = |k: usize| i64_of(&args[k]);
    Some(match (class, name) {
        ("Instant", "ofEpochMilli") => Ok(PV::Instant(i(0), 0)),
        ("Instant", "ofEpochSecond") => Ok(PV::Instant(i(0) * 1000, 0)),
        ("Instant", "parse") => parse_zdt(&arg_str(args, 0), off).map(|z| {
            let ms = z.sec * 1000 + (z.nanos / 1_000_000) as i64;
            PV::Instant(ms, (z.nanos % 1_000_000) as i64)
        }),
        ("ZonedDateTime", "parse") => parse_zdt(&arg_str(args, 0), off).map(zdt),
        ("ZonedDateTime", "ofInstant") => match (&args[0], &args[1]) {
            (PV::Instant(ms, nanos), zid) => {
                let base = Zdt::from_millis(*ms);
                let z = match zid {
                    PV::ZoneId(z) => z.to_string(),
                    _ => "Z".into(),
                };
                Ok(zdt(zdt_in_zone(base.sec, base.nanos + *nanos as u32, &z)))
            }
            _ => Err(npe(off)),
        },
        ("ZonedDateTime", "of") => {
            let z = Zdt::utc(0, 0).with_local(i(0), i(1), i(2), i(3) * 3600 + i(4) * 60 + i(5), i(6) as u32);
            let zone = match &args[7] {
                PV::ZoneId(z) => z.to_string(),
                _ => "Z".into(),
            };
            match zone_offset(&zone) {
                Some(o) if o != 0 => Ok(zdt(Zdt { sec: z.sec - o as i64, nanos: z.nanos, offset: o, zone: Some(zone) })),
                _ => Ok(zdt(z)),
            }
        }
        ("LocalDate", "of") => Ok(PV::LocalDate(days_from_civil(i(0), i(1), i(2)))),
        ("LocalDate", "parse") => {
            let s = arg_str(args, 0);
            match parse_date(&s) {
                Some(d) => Ok(PV::LocalDate(d)),
                None => Err(parse_err(&s, off)),
            }
        }
        ("LocalDate", "ofEpochDay") => Ok(PV::LocalDate(i(0))),
        ("ZoneId", "of") => Ok(PV::ZoneId(Rc::from(arg_str(args, 0).as_str()))),
        ("Duration", "between") => match (temporal(&args[0]), temporal(&args[1])) {
            (Some(a), Some(b)) => Ok(PV::Duration(b.instant_nanos() - a.instant_nanos())),
            _ => Err(iae(off, "Unsupported temporal")),
        },
        ("Duration", _) => {
            let unit: i128 = match name {
                "ofDays" => 86400 * NANOS_PER_SEC,
                "ofHours" => 3600 * NANOS_PER_SEC,
                "ofMinutes" => 60 * NANOS_PER_SEC,
                "ofSeconds" => NANOS_PER_SEC,
                "ofMillis" => 1_000_000,
                "ofNanos" => 1,
                _ => return None,
            };
            Ok(PV::Duration(i(0) as i128 * unit))
        }
        ("DayOfWeek", "of") => Ok(PV::DayOfWeek(i(0).clamp(1, 7) as u8)),
        ("DayOfWeek", "valueOf") => {
            let s = arg_str(args, 0);
            match DAY_NAMES.iter().position(|d| *d == s) {
                Some(k) => Ok(PV::DayOfWeek(k as u8 + 1)),
                None => return None,
            }
        }
        ("DateTimeFormatter", "ofPattern") => Ok(PV::Dtf(Rc::new(Dtf { pattern: arg_str(args, 0), iso: false }))),
        _ => return None,
    })
}

fn plus_minus_unit(name: &str) -> Option<(String, bool)> {
    let (rest, neg) = if let Some(r) = name.strip_prefix("plus") {
        (r, false)
    } else if let Some(r) = name.strip_prefix("minus") {
        (r, true)
    } else {
        return None;
    };
    let unit = rest.to_uppercase();
    if is_chrono_unit(&unit) {
        Some((unit, neg))
    } else {
        None
    }
}

fn cmp_i(a: i128, b: i128) -> PV {
    PV::Int(match a.cmp(&b) {
        std::cmp::Ordering::Less => -1,
        std::cmp::Ordering::Greater => 1,
        _ => 0,
    })
}

pub fn time_method(recv: &PV, name: &str, args: &[PV], off: usize) -> Option<R<PV>> {
    let n = args.len();
    match recv {
        PV::Zdt(z) => {
            let f = z.fields();
            let r = match (name, n) {
                ("getYear", 0) => Ok(PV::Int(f.year as i32)),
                ("getMonthValue", 0) => Ok(PV::Int(f.month as i32)),
                ("getMonth", 0) => Ok(PV::StrEnum(Rc::from(MONTH_NAMES[(f.month - 1) as usize]))),
                ("getDayOfMonth", 0) => Ok(PV::Int(f.day as i32)),
                ("getDayOfYear", 0) => Ok(PV::Int((f.epoch_day - days_from_civil(f.year, 1, 1) + 1) as i32)),
                ("getDayOfWeek", 0) => Ok(PV::DayOfWeek(day_of_week(f.epoch_day) as u8)),
                ("getHour", 0) => Ok(PV::Int(f.hour as i32)),
                ("getMinute", 0) => Ok(PV::Int(f.minute as i32)),
                ("getSecond", 0) => Ok(PV::Int(f.second as i32)),
                ("getNano", 0) => Ok(PV::Int(f.nanos as i32)),
                ("toEpochSecond", 0) => Ok(PV::Long(z.sec)),
                ("toInstant", 0) => {
                    Ok(PV::Instant(z.sec * 1000 + (z.nanos / 1_000_000) as i64, (z.nanos % 1_000_000) as i64))
                }
                ("toLocalDate", 0) => Ok(PV::LocalDate(f.epoch_day)),
                ("getZone", 0) => Ok(PV::ZoneId(Rc::from(z.zone.clone().unwrap_or_else(|| "Z".into()).as_str()))),
                ("isBefore", 1) | ("isAfter", 1) | ("isEqual", 1) => match temporal(&args[0]) {
                    None => Err(npe(off)),
                    Some(o) => {
                        let (a, b) = (z.instant_nanos(), o.instant_nanos());
                        Ok(PV::Bool(match name {
                            "isBefore" => a < b,
                            "isAfter" => a > b,
                            _ => a == b,
                        }))
                    }
                },
                ("format", 1) => match &args[0] {
                    PV::Dtf(d) => Ok(PV::str(&if d.iso { z.to_string_java() } else { format_pattern(&d.pattern, z) })),
                    _ => Err(npe(off)),
                },
                ("truncatedTo", 1) => {
                    let u = duration_unit(&chrono_of(&args[0])).unwrap_or(0);
                    if u > 0 {
                        let local = (z.sec + z.offset as i64) as i128 * NANOS_PER_SEC + z.nanos as i128;
                        let t = local - local.rem_euclid(u) - z.offset as i128 * NANOS_PER_SEC;
                        Ok(zdt(Zdt {
                            sec: t.div_euclid(NANOS_PER_SEC) as i64,
                            nanos: t.rem_euclid(NANOS_PER_SEC) as u32,
                            offset: z.offset,
                            zone: z.zone.clone(),
                        }))
                    } else {
                        Ok(recv.clone())
                    }
                }
                ("withZoneSameInstant", 1) => match &args[0] {
                    PV::ZoneId(id) => Ok(zdt(zdt_in_zone(z.sec, z.nanos, id))),
                    _ => return None,
                },
                ("plus", 2) | ("minus", 2) => {
                    let k = if name == "minus" { -i64_of(&args[0]) } else { i64_of(&args[0]) };
                    Ok(zdt(add_unit(z, &chrono_of(&args[1]), k)))
                }
                ("plus", 1) | ("minus", 1) => match &args[0] {
                    PV::Duration(d) => Ok(zdt(add_nanos(z, if name == "minus" { -*d } else { *d }))),
                    _ => return None,
                },
                ("compareTo", 1) => match temporal(&args[0]) {
                    Some(o) => Ok(cmp_i(z.instant_nanos(), o.instant_nanos())),
                    None => Err(npe(off)),
                },
                ("withDayOfMonth", 1) => {
                    Ok(zdt(z.with_local(f.year, f.month, i64_of(&args[0]), f.hour * 3600 + f.minute * 60 + f.second, f.nanos)))
                }
                ("withHour", 1) => Ok(zdt(z.with_local(
                    f.year,
                    f.month,
                    f.day,
                    i64_of(&args[0]) * 3600 + f.minute * 60 + f.second,
                    f.nanos,
                ))),
                _ => {
                    let (unit, neg) = match plus_minus_unit(name) {
                        Some(u) if n == 1 => u,
                        _ => return None,
                    };
                    let k = if neg { -i64_of(&args[0]) } else { i64_of(&args[0]) };
                    Ok(zdt(add_unit(z, &unit, k)))
                }
            };
            Some(r)
        }
        PV::LocalDate(d) => {
            let z = Zdt::utc(d * 86400, 0);
            let f = z.fields();
            let r = match (name, n) {
                ("getYear", 0) => Ok(PV::Int(f.year as i32)),
                ("getMonthValue", 0) => Ok(PV::Int(f.month as i32)),
                ("getMonth", 0) => Ok(PV::StrEnum(Rc::from(MONTH_NAMES[(f.month - 1) as usize]))),
                ("getDayOfMonth", 0) => Ok(PV::Int(f.day as i32)),
                ("getDayOfYear", 0) => Ok(PV::Int((d - days_from_civil(f.year, 1, 1) + 1) as i32)),
                ("getDayOfWeek", 0) => Ok(PV::DayOfWeek(day_of_week(*d) as u8)),
                ("lengthOfMonth", 0) => Ok(PV::Int(month_len(f.year, f.month) as i32)),
                ("lengthOfYear", 0) => Ok(PV::Int(if is_leap(f.year) { 366 } else { 365 })),
                ("isLeapYear", 0) => Ok(PV::Bool(is_leap(f.year))),
                ("toEpochDay", 0) => Ok(PV::Long(*d)),
                ("isBefore", 1) | ("isAfter", 1) | ("isEqual", 1) => match temporal(&args[0]) {
                    None => Err(npe(off)),
                    Some(o) => {
                        let (a, b) = (z.instant_nanos(), o.instant_nanos());
                        Ok(PV::Bool(match name {
                            "isBefore" => a < b,
                            "isAfter" => a > b,
                            _ => a == b,
                        }))
                    }
                },
                ("format", 1) => match &args[0] {
                    PV::Dtf(dt) => Ok(PV::str(&if dt.iso { fmt_local_date(*d) } else { format_pattern(&dt.pattern, &z) })),
                    _ => return None,
                },
                ("atStartOfDay", 0) | ("atStartOfDay", 1) => Ok(zdt(z.clone())),
                ("plus", 2) | ("minus", 2) => {
                    let k = if name == "minus" { -i64_of(&args[0]) } else { i64_of(&args[0]) };
                    Ok(PV::LocalDate(add_unit(&z, &chrono_of(&args[1]), k).fields().epoch_day))
                }
                ("compareTo", 1) => match temporal(&args[0]) {
                    Some(o) => Ok(cmp_i(z.instant_nanos(), o.instant_nanos())),
                    None => Err(npe(off)),
                },
                _ => {
                    let (unit, neg) = match plus_minus_unit(name) {
                        Some(u) if n == 1 && matches!(u.0.as_str(), "DAYS" | "WEEKS" | "MONTHS" | "YEARS") => u,
                        _ => return None,
                    };
                    let k = if neg { -i64_of(&args[0]) } else { i64_of(&args[0]) };
                    Ok(PV::LocalDate(add_unit(&z, &unit, k).fields().epoch_day))
                }
            };
            Some(r)
        }
        PV::Instant(ms, nanos) => Some(match (name, n) {
            ("toEpochMilli", 0) => Ok(PV::Long(*ms)),
            ("getEpochSecond", 0) => Ok(PV::Long(ms.div_euclid(1000))),
            ("getNano", 0) => Ok(PV::Int((ms.rem_euclid(1000) * 1_000_000 + nanos) as i32)),
            ("plusMillis", 1) => Ok(PV::Instant(ms + i64_of(&args[0]), *nanos)),
            ("plusSeconds", 1) => Ok(PV::Instant(ms + 1000 * i64_of(&args[0]), *nanos)),
            ("minusMillis", 1) => Ok(PV::Instant(ms - i64_of(&args[0]), *nanos)),
            ("minusSeconds", 1) => Ok(PV::Instant(ms - 1000 * i64_of(&args[0]), *nanos)),
            ("isBefore", 1) | ("isAfter", 1) => match &args[0] {
                PV::Instant(o, on) => Ok(PV::Bool(if name == "isBefore" { (ms, nanos) < (o, on) } else { (ms, nanos) > (o, on) })),
                _ => Err(npe(off)),
            },
            ("atZone", 1) => {
                let base = Zdt::from_millis(*ms);
                let zone = match &args[0] {
                    PV::ZoneId(z) => z.to_string(),
                    _ => "Z".into(),
                };
                Ok(zdt(zdt_in_zone(base.sec, base.nanos + *nanos as u32, &zone)))
            }
            _ => return None,
        }),
        PV::Duration(d) => Some(match (name, n) {
            ("toMinutes", 0) => Ok(PV::Long((d / (60 * NANOS_PER_SEC)) as i64)),
            ("toHours", 0) => Ok(PV::Long((d / (3600 * NANOS_PER_SEC)) as i64)),
            ("toDays", 0) => Ok(PV::Long((d / (86400 * NANOS_PER_SEC)) as i64)),
            ("toMillis", 0) => Ok(PV::Long((d / 1_000_000) as i64)),
            ("toNanos", 0) => Ok(PV::Long(*d as i64)),
            ("getSeconds", 0) | ("toSeconds", 0) => Ok(PV::Long(d.div_euclid(NANOS_PER_SEC) as i64)),
            ("isNegative", 0) => Ok(PV::Bool(*d < 0)),
            ("isZero", 0) => Ok(PV::Bool(*d == 0)),
            ("plus", 1) | ("minus", 1) => match &args[0] {
                PV::Duration(o) => Ok(PV::Duration(if name == "plus" { d + o } else { d - o })),
                _ => return None,
            },
            _ => return None,
        }),
        PV::DayOfWeek(d) => Some(match name {
            "getValue" => Ok(PV::Int(*d as i32)),
            "name" | "toString" => Ok(PV::str(DAY_NAMES[*d as usize - 1])),
            "plus" if n == 1 => Ok(PV::DayOfWeek(((*d as i64 - 1 + i64_of(&args[0]) % 7 + 7) % 7 + 1) as u8)),
            _ => return None,
        }),
        PV::StrEnum(s) => Some(match name {
            "name" | "toString" => Ok(PV::str(s)),
            "getValue" => match MONTH_NAMES.iter().position(|m| **m == **s) {
                Some(i) => Ok(PV::Int(i as i32 + 1)),
                None => return None,
            },
            _ => return None,
        }),
        PV::ChronoUnit(u) => {
            if name == "between" && n == 2 {
                let (a, b) = match (temporal(&args[0]), temporal(&args[1])) {
                    (Some(a), Some(b)) => (a, b),
                    _ => return Some(Err(npe(off))),
                };
                let r = match &**u {
                    "MONTHS" => months_between(&a, &b),
                    "YEARS" => months_between(&a, &b) / 12,
                    "DECADES" => months_between(&a, &b) / 120,
                    "CENTURIES" => months_between(&a, &b) / 1200,
                    "MILLENNIA" => months_between(&a, &b) / 12000,
                    other => {
                        let unit = duration_unit(other).unwrap_or(1).max(1);
                        ((b.instant_nanos() - a.instant_nanos()) / unit) as i64
                    }
                };
                return Some(Ok(PV::Long(r)));
            }
            match name {
                "name" => Some(Ok(PV::str(u))),
                "toString" => Some(Ok(PV::str(&jstr(recv)))),
                _ => None,
            }
        }
        PV::Dtf(dt) => {
            if name == "format" && n == 1 {
                let z = match temporal(&args[0]) {
                    Some(z) => z,
                    None => return Some(Err(npe(off))),
                };
                if dt.iso {
                    if let PV::LocalDate(d) = &args[0] {
                        return Some(Ok(PV::str(&fmt_local_date(*d))));
                    }
                    return Some(Ok(PV::str(&z.to_string_java())));
                }
                return Some(Ok(PV::str(&format_pattern(&dt.pattern, &z))));
            }
            if name == "withZone" {
                return Some(Ok(recv.clone()));
            }
            None
        }
        _ => None,
    }
}

pub fn static_time_enum(class: &str, name: &str) -> Option<PV> {
    match class {
        "ChronoUnit" if is_chrono_unit(name) => Some(PV::ChronoUnit(Rc::from(name))),
        "DayOfWeek" => DAY_NAMES.iter().position(|d| *d == name).map(|i| PV::DayOfWeek(i as u8 + 1)),
        "Month" => MONTH_NAMES.iter().find(|m| **m == name).map(|m| PV::StrEnum(Rc::from(*m))),
        "ZoneOffset" if name == "UTC" => Some(PV::ZoneId(Rc::from("Z"))),
        "Instant" if name == "EPOCH" => Some(PV::Instant(0, 0)),
        "DateTimeFormatter" => match name {
            "ISO_LOCAL_DATE" => Some(PV::Dtf(Rc::new(Dtf { pattern: "yyyy-MM-dd".into(), iso: false }))),
            "ISO_INSTANT" | "ISO_OFFSET_DATE_TIME" | "ISO_ZONED_DATE_TIME" | "ISO_DATE_TIME" => {
                Some(PV::Dtf(Rc::new(Dtf { pattern: String::new(), iso: true })))
            }
            _ => None,
        },
        _ => None,
    }
}
