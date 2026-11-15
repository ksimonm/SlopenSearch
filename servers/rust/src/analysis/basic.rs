//! The simple built-in text analysis used where no analysis chain applies:
//! lowercasing, CJK bigrams, a light English stemmer, ASCII folding and edit
//! distances for fuzzy matching.

pub(crate) fn is_cjk(c: char) -> bool {
    let c = c as u32;
    (0x3040..=0x30FF).contains(&c)
        || (0x31F0..=0x31FF).contains(&c)
        || (0x3400..=0x4DBF).contains(&c)
        || (0x4E00..=0x9FFF).contains(&c)
        || (0xF900..=0xFAFF).contains(&c)
        || (0xFF66..=0xFF9D).contains(&c)
}

/// Unicode simple lowercase mapping, matching Go's strings.ToLower: no
/// multi-char special cases (İ stays single 'i') and no final-sigma rule.
pub fn simple_lower(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c.to_lowercase().next() {
            Some(l) => out.push(l),
            None => out.push(c),
        }
    }
    out
}

pub fn analyze(text: &str) -> Vec<String> {
    let chars: Vec<char> = text.chars().collect();
    let mut out = Vec::new();
    let mut buf = String::new();
    let flush = |buf: &mut String, out: &mut Vec<String>| {
        if !buf.is_empty() {
            out.push(simple_lower(buf));
            buf.clear();
        }
    };
    let mut i = 0usize;
    while i < chars.len() {
        let c = chars[i];
        if is_han(c) {
            flush(&mut buf, &mut out);
            out.push(simple_lower(&c.to_string()));
            i += 1;
        } else if is_cjk(c) {
            if !buf.is_empty() && !buf.chars().all(is_kana) {
                flush(&mut buf, &mut out);
            }
            buf.push(c);
            i += 1;
        } else if c.is_alphanumeric() {
            buf.push(c);
            i += 1;
        } else if (c == '.' || c == '\'')
            && i > 0
            && i + 1 < chars.len()
            && chars[i - 1].is_alphanumeric()
            && chars[i + 1].is_alphanumeric()
        {
            buf.push(c);
            i += 1;
        } else {
            flush(&mut buf, &mut out);
            i += 1;
        }
    }
    flush(&mut buf, &mut out);
    out
}

pub fn is_han(c: char) -> bool {
    let c = c as u32;
    (0x3400..=0x4DBF).contains(&c)
        || (0x4E00..=0x9FFF).contains(&c)
        || (0xF900..=0xFAFF).contains(&c)
}

fn is_kana(c: char) -> bool {
    let c = c as u32;
    (0x3040..=0x30FF).contains(&c)
        || (0x31F0..=0x31FF).contains(&c)
        || (0xFF66..=0xFF9D).contains(&c)
}

pub fn english_stem(s: &str) -> String {
    let chars: Vec<char> = s.chars().collect();
    let len = chars.len();
    if len > 5 && s.ends_with("ing") {
        let mut base: Vec<char> = chars[..len - 3].to_vec();
        let n = base.len();
        if n >= 2 && base[n - 1] == base[n - 2] && !matches!(base[n - 1], 'a' | 'e' | 'i' | 'o' | 'u') {
            base.pop();
        }
        return base.into_iter().collect();
    }
    if len > 4 && s.ends_with("ies") {
        let mut base: Vec<char> = chars[..len - 3].to_vec();
        base.push('y');
        return base.into_iter().collect();
    }
    if len > 4 && s.ends_with("es") {
        return chars[..len - 2].iter().collect();
    }
    if len > 3 && s.ends_with('s') && !s.ends_with("ss") {
        return chars[..len - 1].iter().collect();
    }
    s.to_string()
}

pub fn asciifold(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            'ß' => out.push_str("ss"),
            'æ' => out.push_str("ae"),
            'œ' => out.push_str("oe"),
            'ø' => out.push('o'),
            'đ' | 'ð' => out.push('d'),
            'ł' => out.push('l'),
            'þ' => out.push_str("th"),
            'á' | 'à' | 'â' | 'ä' | 'ã' | 'å' | 'ā' | 'ă' | 'ą' => out.push('a'),
            'é' | 'è' | 'ê' | 'ë' | 'ē' | 'ė' | 'ę' => out.push('e'),
            'í' | 'ì' | 'î' | 'ï' | 'ī' | 'į' => out.push('i'),
            'ó' | 'ò' | 'ô' | 'ö' | 'õ' | 'ō' | 'ő' => out.push('o'),
            'ú' | 'ù' | 'û' | 'ü' | 'ū' | 'ů' | 'ű' | 'ų' => out.push('u'),
            'ç' | 'ć' | 'č' => out.push('c'),
            'ñ' | 'ń' | 'ň' => out.push('n'),
            'š' => out.push('s'),
            'ž' | 'ź' | 'ż' => out.push('z'),
            'ý' | 'ÿ' => out.push('y'),
            'ř' => out.push('r'),
            'ť' => out.push('t'),
            'ď' => out.push('d'),
            'ğ' => out.push('g'),
            _ => out.push(c),
        }
    }
    out
}

pub fn analyze_with(analyzer: &str, text: &str) -> Vec<String> {
    match analyzer {
        "keyword" => vec![text.to_string()],
        "whitespace" => text.split_whitespace().map(|s| s.to_string()).collect(),
        "lowercase" => vec![simple_lower(text)],
        _ => analyze(text),
    }
}

pub fn levenshtein(a: &str, b: &str) -> usize {
    let a: Vec<char> = a.chars().collect();
    let b: Vec<char> = b.chars().collect();
    let mut prev: Vec<usize> = (0..=b.len()).collect();
    let mut curr = vec![0usize; b.len() + 1];
    for i in 1..=a.len() {
        curr[0] = i;
        for j in 1..=b.len() {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            curr[j] = (prev[j] + 1).min(curr[j - 1] + 1).min(prev[j - 1] + cost);
        }
        std::mem::swap(&mut prev, &mut curr);
    }
    prev[b.len()]
}

pub fn auto_fuzziness(term: &str) -> usize {
    match term.chars().count() {
        0..=2 => 0,
        3..=5 => 1,
        _ => 2,
    }
}
