//! Predefined stop word sets (`_english_`, `_russian_`, ...), taken from the
//! Lucene resources the OpenSearch analyzers use.

/// Lucene's EnglishAnalyzer.ENGLISH_STOP_WORDS_SET, which is what
/// `_english_` and the `stop` analyzer use.
pub const ENGLISH: &[&str] = &[
    "a", "an", "and", "are", "as", "at", "be", "but", "by", "for", "if", "in", "into", "is", "it",
    "no", "not", "of", "on", "or", "such", "that", "the", "their", "then", "there", "these",
    "they", "this", "to", "was", "will", "with",
];

/// Snowball-format list: `|` starts a comment, several words per line allowed.
fn snowball(src: &str) -> Vec<String> {
    let mut out = Vec::new();
    for line in src.lines() {
        let line = match line.find('|') {
            Some(i) => &line[..i],
            None => line,
        };
        for w in line.split_whitespace() {
            out.push(w.to_string());
        }
    }
    out
}

/// Plain word list: one word per line, `#` comment lines.
fn wordset(src: &str) -> Vec<String> {
    src.lines()
        .map(|l| l.trim())
        .filter(|l| !l.is_empty() && !l.starts_with('#'))
        .map(|l| l.to_string())
        .collect()
}

/// The words of a named set such as `_french_`, or None for unknown names.
pub fn named(name: &str) -> Option<Vec<String>> {
    let words = match name {
        "_english_" => ENGLISH.iter().map(|s| s.to_string()).collect(),
        "_none_" => Vec::new(),
        "_arabic_" => wordset(include_str!("stopwords/ar.txt")),
        "_armenian_" => wordset(include_str!("stopwords/hy.txt")),
        "_basque_" => wordset(include_str!("stopwords/eu.txt")),
        "_bengali_" => wordset(include_str!("stopwords/bn.txt")),
        "_brazilian_" => wordset(include_str!("stopwords/br.txt")),
        "_bulgarian_" => wordset(include_str!("stopwords/bg.txt")),
        "_catalan_" => wordset(include_str!("stopwords/ca.txt")),
        "_czech_" => wordset(include_str!("stopwords/cz.txt")),
        "_danish_" => snowball(include_str!("stopwords/danish_stop.txt")),
        "_dutch_" => snowball(include_str!("stopwords/dutch_stop.txt")),
        "_estonian_" => wordset(include_str!("stopwords/et.txt")),
        "_finnish_" => snowball(include_str!("stopwords/finnish_stop.txt")),
        "_french_" => snowball(include_str!("stopwords/french_stop.txt")),
        "_galician_" => wordset(include_str!("stopwords/gl.txt")),
        "_german_" => snowball(include_str!("stopwords/german_stop.txt")),
        "_greek_" => wordset(include_str!("stopwords/el.txt")),
        "_hindi_" => wordset(include_str!("stopwords/hi.txt")),
        "_hungarian_" => snowball(include_str!("stopwords/hungarian_stop.txt")),
        "_indonesian_" => wordset(include_str!("stopwords/id.txt")),
        "_irish_" => snowball(include_str!("stopwords/irish_stop.txt")),
        "_italian_" => snowball(include_str!("stopwords/italian_stop.txt")),
        "_latvian_" => wordset(include_str!("stopwords/lv.txt")),
        "_lithuanian_" => wordset(include_str!("stopwords/lt.txt")),
        "_nepali_" => wordset(include_str!("stopwords/ne.txt")),
        "_norwegian_" => snowball(include_str!("stopwords/norwegian_stop.txt")),
        "_persian_" => wordset(include_str!("stopwords/fa.txt")),
        "_portuguese_" => snowball(include_str!("stopwords/portuguese_stop.txt")),
        "_romanian_" => wordset(include_str!("stopwords/ro.txt")),
        "_russian_" => snowball(include_str!("stopwords/russian_stop.txt")),
        "_serbian_" => wordset(include_str!("stopwords/sr.txt")),
        "_sorani_" => wordset(include_str!("stopwords/ckb.txt")),
        "_spanish_" => snowball(include_str!("stopwords/spanish_stop.txt")),
        "_swedish_" => snowball(include_str!("stopwords/swedish_stop.txt")),
        "_tamil_" => wordset(include_str!("stopwords/ta.txt")),
        "_telugu_" => wordset(include_str!("stopwords/te.txt")),
        "_thai_" => wordset(include_str!("stopwords/th.txt")),
        "_turkish_" => wordset(include_str!("stopwords/tr.txt")),
        _ => return None,
    };
    Some(words)
}
