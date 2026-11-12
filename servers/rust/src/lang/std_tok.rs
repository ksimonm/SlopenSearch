//! Lucene 10.5.1 StandardTokenizer (UAX#29 word breaks): a port of the
//! JFlex-generated scanner `StandardTokenizerImpl`, driven by its original
//! tables (`std_tok_data.rs`). Offsets are UTF-16 code units, like Lucene.

use super::std_tok_data::*;
use super::Tok;

pub const TOKEN_TYPES: [&str; 8] = [
    "<ALPHANUM>",
    "<NUM>",
    "<SOUTHEAST_ASIAN>",
    "<IDEOGRAPHIC>",
    "<HIRAGANA>",
    "<KATAKANA>",
    "<HANGUL>",
    "<EMOJI>",
];

fn cmap(input: u32) -> usize {
    let offset = (input & 255) as usize;
    if offset as u32 == input {
        ZZ_CMAP_BLOCKS[offset] as usize
    } else {
        ZZ_CMAP_BLOCKS[ZZ_CMAP_TOP[(input >> 8) as usize] as usize | offset] as usize
    }
}

fn code_point_at(buf: &[u16], i: usize) -> (u32, usize) {
    let c = buf[i];
    if (0xD800..0xDC00).contains(&c) && i + 1 < buf.len() {
        let d = buf[i + 1];
        if (0xDC00..0xE000).contains(&d) {
            return (0x10000 + (((c as u32) - 0xD800) << 10) + ((d as u32) - 0xDC00), 2);
        }
    }
    (c as u32, 1)
}

/// Scans `buf` and returns (type index, start, length) of each token, in
/// UTF-16 units. `max_len` caps a token like the scanner's buffer size does.
pub fn scan(buf: &[u16], max_len: usize) -> Vec<(usize, usize, usize)> {
    let mut out = Vec::new();
    let end_read = buf.len();
    let mut marked_pos = 0usize;
    loop {
        let start_read = marked_pos;
        let mut marked_l = marked_pos;
        let mut current = start_read;
        let mut state = ZZ_LEXSTATE[0] as usize;
        let mut action: i32 = -1;
        if ZZ_ATTRIBUTE[state] & 1 == 1 {
            action = state as i32;
        }
        let limit = end_read.min(start_read + max_len.max(1));
        let mut at_eof = false;
        loop {
            if current >= limit {
                at_eof = true;
                break;
            }
            let (cp, n) = code_point_at(buf, current);
            if current + n > limit && current > start_read {
                at_eof = true;
                break;
            }
            current += n;
            let next = ZZ_TRANS[ZZ_ROWMAP[state] as usize + cmap(cp)];
            if next == -1 {
                break;
            }
            state = next as usize;
            let attrs = ZZ_ATTRIBUTE[state];
            if attrs & 1 == 1 {
                action = state as i32;
                marked_l = current;
                if attrs & 8 == 8 {
                    break;
                }
            }
        }
        marked_pos = marked_l;
        if at_eof && start_read == current {
            return out;
        }
        if action < 0 {
            // cannot happen with Lucene's grammar (it has a catch-all rule);
            // skip one code unit to make progress
            marked_pos = start_read + 1;
            continue;
        }
        let typ = match ZZ_ACTION[action as usize] {
            2 => 1,
            3 => 0,
            4 => 7,
            5 => 2,
            6 => 6,
            7 => 3,
            8 => 5,
            9 => 4,
            _ => usize::MAX,
        };
        if marked_pos == start_read {
            // empty match: force progress
            marked_pos = start_read + 1;
            continue;
        }
        if typ != usize::MAX {
            out.push((typ, start_read, marked_pos - start_read));
        }
    }
}

/// StandardTokenizer: tokens with term, UTF-16 offsets, type and position increments.
pub fn tokenize(text: &str, max_len: usize) -> Vec<Tok> {
    let buf: Vec<u16> = text.encode_utf16().collect();
    tokenize_units(&buf, max_len)
}

pub fn tokenize_units(buf: &[u16], max_len: usize) -> Vec<Tok> {
    scan(buf, max_len)
        .into_iter()
        .map(|(typ, start, len)| Tok {
            term: String::from_utf16_lossy(&buf[start..start + len]),
            start,
            end: start + len,
            typ: TOKEN_TYPES[typ].to_string(),
            pos_inc: 1,
            pos_len: 1,
            keyword: false,
        })
        .collect()
}
