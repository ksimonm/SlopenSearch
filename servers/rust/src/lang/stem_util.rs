//! Port of Lucene's org.apache.lucene.analysis.util.StemmerUtil, on UTF-16
//! buffers (`s[..len]` is the term, like Java's `char[] s, int len`).

/// UTF-16 units of a string literal (all stemmer literals are in the BMP).
pub fn u(s: &str) -> Vec<u16> {
    s.encode_utf16().collect()
}

pub fn starts_with(s: &[u16], len: usize, prefix: &str) -> bool {
    let p: Vec<u16> = prefix.encode_utf16().collect();
    if p.len() > len {
        return false;
    }
    s[..p.len()] == p[..]
}

pub fn ends_with(s: &[u16], len: usize, suffix: &str) -> bool {
    let p: Vec<u16> = suffix.encode_utf16().collect();
    ends_with_units(s, len, &p)
}

pub fn ends_with_units(s: &[u16], len: usize, suffix: &[u16]) -> bool {
    if suffix.len() > len {
        return false;
    }
    s[len - suffix.len()..len] == suffix[..]
}

/// Deletes the char at `pos`; returns the new length.
pub fn delete(s: &mut [u16], pos: usize, len: usize) -> usize {
    if pos + 1 < len {
        s.copy_within(pos + 1..len, pos);
    }
    len - 1
}

/// Deletes `n` chars at `pos`; returns the new length.
pub fn delete_n(s: &mut [u16], pos: usize, len: usize, n: usize) -> usize {
    if pos + n < len {
        s.copy_within(pos + n..len, pos);
    }
    len - n
}

/// Runs a Lucene-style `int stem(char[] s, int len)` port over a term buffer.
pub fn apply(term: &mut Vec<u16>, f: impl FnOnce(&mut [u16], usize) -> usize) {
    let len = term.len();
    let n = f(term.as_mut_slice(), len);
    term.truncate(n);
}
