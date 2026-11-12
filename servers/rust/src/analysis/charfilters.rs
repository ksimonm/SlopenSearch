//! Char filters with Lucene's offset correction (BaseCharFilter): each filter
//! records (output offset, cumulative diff) pairs, and a token offset in the
//! filtered text maps back through the filters in reverse order.

use std::collections::HashSet;

use serde_json::{Map, Value};

use super::jregex::{self, JRegex};
use super::{bad, p_bool, p_list, p_str, AResult};

#[derive(Debug, Clone, Default)]
pub struct OffsetMap {
    offs: Vec<usize>,
    diffs: Vec<i64>,
}

impl OffsetMap {
    fn add(&mut self, off: usize, cumulative: i64) {
        if let Some(last) = self.offs.last() {
            if *last == off {
                *self.diffs.last_mut().unwrap() = cumulative;
                return;
            }
        }
        self.offs.push(off);
        self.diffs.push(cumulative);
    }

    fn last_cumulative(&self) -> i64 {
        self.diffs.last().copied().unwrap_or(0)
    }

    pub fn correct(&self, off: usize) -> usize {
        // Largest recorded offset <= off.
        let idx = match self.offs.binary_search(&off) {
            Ok(i) => Some(i),
            Err(0) => None,
            Err(i) => Some(i - 1),
        };
        match idx {
            Some(i) => (off as i64 + self.diffs[i]).max(0) as usize,
            None => off,
        }
    }
}

/// Maps an offset of the fully filtered text back to the original input.
pub fn correct(maps: &[OffsetMap], off: usize) -> usize {
    let mut o = off;
    for m in maps.iter().rev() {
        o = m.correct(o);
    }
    o
}

#[derive(Debug, Clone)]
pub enum CharFilter {
    HtmlStrip { escaped: HashSet<String> },
    Mapping { rules: Vec<(Vec<char>, Vec<char>)> },
    PatternReplace { re: JRegex, repl: String },
}

/// Escapes allowed in mapping rules (OpenSearch's parseString).
fn unescape_mapping(s: &str) -> AResult<Vec<char>> {
    let c: Vec<char> = s.chars().collect();
    let mut out = Vec::new();
    let mut i = 0;
    while i < c.len() {
        if c[i] == '\\' {
            i += 1;
            if i >= c.len() {
                return Err(bad(&format!("Invalid escaped char in [{}]", s)));
            }
            match c[i] {
                '\\' => out.push('\\'),
                '"' => out.push('"'),
                '\'' => out.push('\''),
                't' => out.push('\t'),
                'n' => out.push('\n'),
                'r' => out.push('\r'),
                'b' => out.push('\u{08}'),
                'f' => out.push('\u{0C}'),
                'u' => {
                    if i + 4 >= c.len() + 0 && i + 4 > c.len() - 1 + 1 {
                        return Err(bad(&format!("Invalid escaped char in [{}]", s)));
                    }
                    let h: String = c[i + 1..(i + 5).min(c.len())].iter().collect();
                    let v = u32::from_str_radix(&h, 16).map_err(|_| bad(&format!("Invalid escaped char in [{}]", s)))?;
                    out.push(char::from_u32(v).unwrap_or('\u{FFFD}'));
                    i += 4;
                }
                _ => return Err(bad(&format!("Invalid escaped char in [{}]", s))),
            }
            i += 1;
            continue;
        }
        out.push(c[i]);
        i += 1;
    }
    Ok(out)
}

fn parse_mapping_rule(rule: &str) -> AResult<(Vec<char>, Vec<char>)> {
    let invalid = || bad(&format!("Invalid Mapping Rule : [{}]", rule));
    let at = rule.find("=>").ok_or_else(invalid)?;
    let lhs = rule[..at].trim();
    let rhs = rule[at + 2..].trim();
    if lhs.is_empty() {
        return Err(invalid());
    }
    Ok((unescape_mapping(lhs)?, unescape_mapping(rhs)?))
}

/// Builds a char filter of the given type; Ok(None) for unknown types.
pub fn build(typ: &str, p: &Map<String, Value>) -> AResult<Option<CharFilter>> {
    Ok(Some(match typ {
        "html_strip" | "htmlStrip" => {
            let escaped = p_list(p, "escaped_tags").unwrap_or_default().into_iter().map(|s| s.to_lowercase()).collect();
            CharFilter::HtmlStrip { escaped }
        }
        "mapping" => {
            let rules = match p_list(p, "mappings") {
                Some(r) => r,
                None => {
                    if p.contains_key("mappings_path") {
                        return Err(bad("IOException while reading mappings_path: file not readable"));
                    }
                    return Err(bad("mapping requires either `mappings` or `mappings_path` to be configured"));
                }
            };
            let mut out = Vec::new();
            for r in rules {
                out.push(parse_mapping_rule(&r)?);
            }
            CharFilter::Mapping { rules: out }
        }
        "pattern_replace" => {
            let pat = match p_str(p, "pattern") {
                Some(s) => s,
                None => return Err(bad("pattern is missing for [pattern_replace] char filter")),
            };
            let flags = jregex::parse_flags(&p_str(p, "flags").unwrap_or_default()).map_err(|e| bad(&e))?;
            let re = super::tokenizers::compile_regex(&pat, flags)?;
            let _ = p_bool(p, "all", true);
            CharFilter::PatternReplace { re, repl: p_str(p, "replacement").unwrap_or_default() }
        }
        _ => return Ok(None),
    }))
}

fn u16len(c: &[char]) -> usize {
    c.iter().map(|x| x.len_utf16()).sum()
}

impl CharFilter {
    pub fn filter(&self, text: &str) -> (String, OffsetMap) {
        match self {
            CharFilter::HtmlStrip { escaped } => html_strip(text, escaped),
            CharFilter::Mapping { rules } => mapping(text, rules),
            CharFilter::PatternReplace { re, repl } => pattern_replace(text, re, repl),
        }
    }
}

fn mapping(text: &str, rules: &[(Vec<char>, Vec<char>)]) -> (String, OffsetMap) {
    let c: Vec<char> = text.chars().collect();
    let mut map = OffsetMap::default();
    let mut out = String::new();
    let mut i = 0usize;
    let mut input_off = 0usize; // UTF-16
    while i < c.len() {
        // Longest matching rule.
        let mut best: Option<&(Vec<char>, Vec<char>)> = None;
        for r in rules {
            let l = r.0.len();
            if i + l <= c.len() && c[i..i + l] == r.0[..] && best.map(|b| l > b.0.len()).unwrap_or(true) {
                best = Some(r);
            }
        }
        match best {
            Some((from, to)) => {
                let match_len = u16len(from) as i64;
                input_off += match_len as usize;
                let out_len = u16len(to) as i64;
                let diff = match_len - out_len;
                if diff != 0 {
                    let prev = map.last_cumulative();
                    if diff > 0 {
                        map.add((input_off as i64 - diff - prev) as usize, prev + diff);
                    } else {
                        let output_start = input_off as i64 - prev;
                        for extra in 0..(-diff) {
                            map.add((output_start + extra) as usize, prev - extra - 1);
                        }
                    }
                }
                out.extend(to.iter());
                i += from.len();
            }
            None => {
                out.push(c[i]);
                input_off += c[i].len_utf16();
                i += 1;
            }
        }
    }
    (out, map)
}

fn pattern_replace(text: &str, re: &JRegex, repl: &str) -> (String, OffsetMap) {
    let c: Vec<char> = text.chars().collect();
    let mut map = OffsetMap::default();
    let mut out: Vec<char> = Vec::new();
    let mut cumulative: i64 = 0;
    let mut last_end = 0usize; // char index
    for caps in re.find_all(&c) {
        let (s, e) = caps[0].unwrap();
        let group_size = u16len(&c[s..e]) as i64;
        out.extend_from_slice(&c[last_end..s]);
        let length_before = u16len(&out) as i64;
        let r: Vec<char> = re.expand(repl, &c, &caps).unwrap_or_default().chars().collect();
        let repl_size = u16len(&r) as i64;
        out.extend(r);
        last_end = e;
        if group_size != repl_size {
            if repl_size < group_size {
                cumulative += group_size - repl_size;
                map.add((length_before + repl_size) as usize, cumulative);
            } else {
                for i in group_size..repl_size {
                    cumulative -= 1;
                    map.add((length_before + i) as usize, cumulative);
                }
            }
        }
    }
    out.extend_from_slice(&c[last_end..]);
    (out.into_iter().collect(), map)
}

// ------------------------------------------------------------- html_strip

const BLOCK_TAGS: &[&str] = &[
    "address", "article", "aside", "blockquote", "body", "br", "button", "caption", "center", "col",
    "colgroup", "dd", "dir", "div", "dl", "dt", "embed", "fieldset", "figcaption", "figure", "footer",
    "form", "frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr",
    "html", "iframe", "isindex", "li", "map", "menu", "nav", "noframes", "noscript", "object", "ol",
    "optgroup", "option", "output", "p", "pre", "section", "select", "table", "tbody", "td",
    "textarea", "tfoot", "th", "thead", "title", "tr", "ul", "video",
];

const LATIN1_ENTITIES: &[&str] = &[
    "nbsp", "iexcl", "cent", "pound", "curren", "yen", "brvbar", "sect", "uml", "copy", "ordf", "laquo",
    "not", "shy", "reg", "macr", "deg", "plusmn", "sup2", "sup3", "acute", "micro", "para", "middot",
    "cedil", "sup1", "ordm", "raquo", "frac14", "frac12", "frac34", "iquest", "Agrave", "Aacute",
    "Acirc", "Atilde", "Auml", "Aring", "AElig", "Ccedil", "Egrave", "Eacute", "Ecirc", "Euml",
    "Igrave", "Iacute", "Icirc", "Iuml", "ETH", "Ntilde", "Ograve", "Oacute", "Ocirc", "Otilde",
    "Ouml", "times", "Oslash", "Ugrave", "Uacute", "Ucirc", "Uuml", "Yacute", "THORN", "szlig",
    "agrave", "aacute", "acirc", "atilde", "auml", "aring", "aelig", "ccedil", "egrave", "eacute",
    "ecirc", "euml", "igrave", "iacute", "icirc", "iuml", "eth", "ntilde", "ograve", "oacute", "ocirc",
    "otilde", "ouml", "divide", "oslash", "ugrave", "uacute", "ucirc", "uuml", "yacute", "thorn", "yuml",
];

const OTHER_ENTITIES: &[(&str, u32)] = &[
    ("quot", 0x22), ("amp", 0x26), ("apos", 0x27), ("lt", 0x3C), ("gt", 0x3E), ("OElig", 0x152),
    ("oelig", 0x153), ("Scaron", 0x160), ("scaron", 0x161), ("Yuml", 0x178), ("fnof", 0x192),
    ("circ", 0x2C6), ("tilde", 0x2DC), ("ensp", 0x2002), ("emsp", 0x2003), ("thinsp", 0x2009),
    ("zwnj", 0x200C), ("zwj", 0x200D), ("lrm", 0x200E), ("rlm", 0x200F), ("ndash", 0x2013),
    ("mdash", 0x2014), ("lsquo", 0x2018), ("rsquo", 0x2019), ("sbquo", 0x201A), ("ldquo", 0x201C),
    ("rdquo", 0x201D), ("bdquo", 0x201E), ("dagger", 0x2020), ("Dagger", 0x2021), ("bull", 0x2022),
    ("hellip", 0x2026), ("permil", 0x2030), ("prime", 0x2032), ("Prime", 0x2033), ("lsaquo", 0x2039),
    ("rsaquo", 0x203A), ("oline", 0x203E), ("frasl", 0x2044), ("euro", 0x20AC), ("image", 0x2111),
    ("weierp", 0x2118), ("real", 0x211C), ("trade", 0x2122), ("alefsym", 0x2135), ("larr", 0x2190),
    ("uarr", 0x2191), ("rarr", 0x2192), ("darr", 0x2193), ("harr", 0x2194), ("crarr", 0x21B5),
    ("lArr", 0x21D0), ("uArr", 0x21D1), ("rArr", 0x21D2), ("dArr", 0x21D3), ("hArr", 0x21D4),
    ("forall", 0x2200), ("part", 0x2202), ("exist", 0x2203), ("empty", 0x2205), ("nabla", 0x2207),
    ("isin", 0x2208), ("notin", 0x2209), ("ni", 0x220B), ("prod", 0x220F), ("sum", 0x2211),
    ("minus", 0x2212), ("lowast", 0x2217), ("radic", 0x221A), ("prop", 0x221D), ("infin", 0x221E),
    ("ang", 0x2220), ("and", 0x2227), ("or", 0x2228), ("cap", 0x2229), ("cup", 0x222A), ("int", 0x222B),
    ("there4", 0x2234), ("sim", 0x223C), ("cong", 0x2245), ("asymp", 0x2248), ("ne", 0x2260),
    ("equiv", 0x2261), ("le", 0x2264), ("ge", 0x2265), ("sub", 0x2282), ("sup", 0x2283),
    ("nsub", 0x2284), ("sube", 0x2286), ("supe", 0x2287), ("oplus", 0x2295), ("otimes", 0x2297),
    ("perp", 0x22A5), ("sdot", 0x22C5), ("lceil", 0x2308), ("rceil", 0x2309), ("lfloor", 0x230A),
    ("rfloor", 0x230B), ("lang", 0x2329), ("rang", 0x232A), ("loz", 0x25CA), ("spades", 0x2660),
    ("clubs", 0x2663), ("hearts", 0x2665), ("diams", 0x2666),
];

const GREEK: &[&str] = &[
    "Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta", "Theta", "Iota", "Kappa", "Lambda",
    "Mu", "Nu", "Xi", "Omicron", "Pi", "Rho", "", "Sigma", "Tau", "Upsilon", "Phi", "Chi", "Psi", "Omega",
];

fn entity(name: &str) -> Option<String> {
    if let Some(num) = name.strip_prefix('#') {
        let v = if let Some(h) = num.strip_prefix('x').or_else(|| num.strip_prefix('X')) {
            u32::from_str_radix(h, 16).ok()?
        } else {
            num.parse::<u32>().ok()?
        };
        return char::from_u32(v).map(|c| c.to_string());
    }
    if name == "nbsp" {
        return Some(" ".to_string());
    }
    if let Some(i) = LATIN1_ENTITIES.iter().position(|e| *e == name) {
        return char::from_u32(0xA0 + i as u32).map(|c| c.to_string());
    }
    if let Some((_, v)) = OTHER_ENTITIES.iter().find(|(e, _)| *e == name) {
        return char::from_u32(*v).map(|c| c.to_string());
    }
    if let Some(i) = GREEK.iter().position(|e| !e.is_empty() && *e == name) {
        return char::from_u32(0x391 + i as u32).map(|c| c.to_string());
    }
    let lower_greek: Option<usize> = GREEK.iter().position(|e| !e.is_empty() && e.to_lowercase() == name);
    if let Some(i) = lower_greek {
        return char::from_u32(0x3B1 + i as u32).map(|c| c.to_string());
    }
    match name {
        "sigmaf" => Some("\u{3C2}".to_string()),
        "thetasym" => Some("\u{3D1}".to_string()),
        "upsih" => Some("\u{3D2}".to_string()),
        "piv" => Some("\u{3D6}".to_string()),
        _ => None,
    }
}

/// Parses a tag at `i` ('<' ...). Returns (end index exclusive, lowercase
/// name, is_end_tag).
fn parse_tag(c: &[char], i: usize) -> Option<(usize, String, bool)> {
    let mut j = i + 1;
    let end_tag = j < c.len() && c[j] == '/';
    if end_tag {
        j += 1;
    }
    let ns = j;
    if j >= c.len() || !c[j].is_ascii_alphabetic() {
        return None;
    }
    while j < c.len() && (c[j].is_ascii_alphanumeric() || c[j] == ':' || c[j] == '_' || c[j] == '-') {
        j += 1;
    }
    let name: String = c[ns..j].iter().collect::<String>().to_lowercase();
    // Attributes, honoring quoted values.
    while j < c.len() {
        match c[j] {
            '>' => return Some((j + 1, name, end_tag)),
            '"' | '\'' => {
                let q = c[j];
                j += 1;
                while j < c.len() && c[j] != q {
                    j += 1;
                }
                if j >= c.len() {
                    return None;
                }
                j += 1;
            }
            '<' => return None,
            _ => j += 1,
        }
    }
    None
}

fn find_seq(c: &[char], from: usize, pat: &str) -> Option<usize> {
    let p: Vec<char> = pat.chars().collect();
    let mut i = from;
    while i + p.len() <= c.len() {
        if c[i..i + p.len()].iter().zip(&p).all(|(a, b)| a.to_ascii_lowercase() == *b) {
            return Some(i);
        }
        i += 1;
    }
    None
}

fn html_strip(text: &str, escaped: &HashSet<String>) -> (String, OffsetMap) {
    let c: Vec<char> = text.chars().collect();
    let mut map = OffsetMap::default();
    let mut out = String::new();
    let mut out_len = 0usize; // UTF-16
    let mut cum: i64 = 0;
    let mut i = 0usize;
    // Replaces c[i..j] by `rep`, recording the offset correction after it.
    let replace = |out: &mut String, out_len: &mut usize, cum: &mut i64, map: &mut OffsetMap, i: usize, j: usize, rep: &str| {
        let in_len = u16len(&c[i..j]) as i64;
        let rep_len = super::uchar::utf16_len(rep);
        out.push_str(rep);
        *out_len += rep_len;
        *cum += in_len - rep_len as i64;
        map.add(*out_len, *cum);
    };
    while i < c.len() {
        let ch = c[i];
        if ch == '&' {
            let mut j = i + 1;
            while j < c.len() && j - i <= 10 && (c[j].is_ascii_alphanumeric() || c[j] == '#') {
                j += 1;
            }
            if j < c.len() && c[j] == ';' && j > i + 1 {
                let name: String = c[i + 1..j].iter().collect();
                if let Some(dec) = entity(&name) {
                    replace(&mut out, &mut out_len, &mut cum, &mut map, i, j + 1, &dec);
                    i = j + 1;
                    continue;
                }
            }
        } else if ch == '<' {
            if c.get(i + 1) == Some(&'!') {
                if c.get(i + 2) == Some(&'-') && c.get(i + 3) == Some(&'-') {
                    if let Some(e) = find_seq(&c, i + 4, "-->") {
                        replace(&mut out, &mut out_len, &mut cum, &mut map, i, e + 3, "");
                        i = e + 3;
                        continue;
                    }
                } else if let Some(e) = find_seq(&c, i + 2, ">") {
                    replace(&mut out, &mut out_len, &mut cum, &mut map, i, e + 1, "");
                    i = e + 1;
                    continue;
                }
            } else if c.get(i + 1) == Some(&'?') {
                if let Some(e) = find_seq(&c, i + 2, ">") {
                    replace(&mut out, &mut out_len, &mut cum, &mut map, i, e + 1, "");
                    i = e + 1;
                    continue;
                }
            } else if let Some((end, name, is_end)) = parse_tag(&c, i) {
                if escaped.contains(&name) {
                    for x in &c[i..end] {
                        out.push(*x);
                        out_len += x.len_utf16();
                    }
                    i = end;
                    continue;
                }
                if !is_end && (name == "script" || name == "style") {
                    let close = format!("</{}", name);
                    let stop = match find_seq(&c, end, &close) {
                        Some(k) => match parse_tag(&c, k) {
                            Some((e2, _, _)) => e2,
                            None => c.len(),
                        },
                        None => c.len(),
                    };
                    replace(&mut out, &mut out_len, &mut cum, &mut map, i, stop, "\n");
                    i = stop;
                    continue;
                }
                let rep = if BLOCK_TAGS.contains(&name.as_str()) { "\n" } else { "" };
                replace(&mut out, &mut out_len, &mut cum, &mut map, i, end, rep);
                i = end;
                continue;
            }
        }
        out.push(ch);
        out_len += ch.len_utf16();
        i += 1;
    }
    (out, map)
}
