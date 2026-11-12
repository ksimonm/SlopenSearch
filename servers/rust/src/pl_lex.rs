// Painless engine: lexer.

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum TokKind {
    Eof,
    Ident,
    Type,
    Int,
    Float,
    Str,
    Regex,
    Op,
}

#[derive(Clone, Debug)]
pub struct Tok {
    pub kind: TokKind,
    /// Operator text, identifier, raw number text or decoded string.
    pub text: String,
    pub pos: usize,
    pub end: usize,
    /// Regex flags.
    pub flags: String,
}

/// A compile error of the Painless compiler.
#[derive(Clone, Debug)]
pub struct CompileErr {
    pub typ: String,
    /// None renders as `"reason": null`.
    pub reason: Option<String>,
    pub offset: usize,
    /// The error has no script position (no script_stack / position).
    pub no_pos: bool,
    pub cause: Option<Box<CompileErr>>,
}

impl CompileErr {
    pub fn new(typ: &str, reason: &str, offset: usize) -> Self {
        CompileErr { typ: typ.into(), reason: Some(reason.into()), offset, no_pos: false, cause: None }
    }
    pub fn iae(reason: &str, offset: usize) -> Self {
        CompileErr::new("illegal_argument_exception", reason, offset)
    }
    pub fn with_nil_cause(mut self, typ: &str) -> Self {
        self.cause = Some(Box::new(CompileErr {
            typ: typ.into(),
            reason: None,
            offset: 0,
            no_pos: false,
            cause: None,
        }));
        self
    }
}

/// Identifiers the lexer turns into type tokens, like the Painless lexer does
/// for every class in the whitelist.
pub fn is_type_name(w: &str) -> bool {
    matches!(
        w,
        "def" | "void" | "boolean" | "byte" | "short" | "char" | "int" | "long" | "float" | "double"
            | "Object" | "String" | "Integer" | "Long" | "Double" | "Float" | "Short" | "Byte"
            | "Character" | "Boolean" | "Number" | "Math" | "StrictMath" | "CharSequence"
            | "StringBuilder" | "List" | "ArrayList" | "LinkedList" | "Map" | "HashMap" | "TreeMap"
            | "LinkedHashMap" | "Set" | "HashSet" | "TreeSet" | "LinkedHashSet" | "Collection"
            | "Collections" | "Collectors" | "Comparator" | "Iterator" | "Iterable" | "Arrays"
            | "Objects" | "Pattern" | "Matcher" | "Optional" | "OptionalInt" | "OptionalLong"
            | "OptionalDouble" | "Stream" | "IntStream" | "LongStream" | "DoubleStream" | "Deque"
            | "ArrayDeque" | "Queue" | "Exception" | "RuntimeException" | "IllegalArgumentException"
            | "IllegalStateException" | "ArithmeticException" | "NullPointerException"
            | "UnsupportedOperationException" | "IndexOutOfBoundsException"
            | "NumberFormatException" | "ClassCastException" | "Instant" | "ZonedDateTime"
            | "LocalDate" | "LocalDateTime" | "LocalTime" | "ZoneId" | "ZoneOffset" | "Duration"
            | "Period" | "ChronoUnit" | "DayOfWeek" | "Month" | "DateTimeFormatter" | "Debug"
            | "Base64" | "Function" | "BiFunction" | "Supplier" | "Consumer" | "Predicate"
            | "BiConsumer" | "Random"
    )
}

const OPS: &[&str] = &[
    ">>>=", "===", "!==", "==~", ">>>", "<<=", ">>=", "==", "!=", "<=", ">=", "&&", "||", "++",
    "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<", ">>", "=~", "?.", "?:", "->",
    "::", "+", "-", "*", "/", "%", "&", "|", "^", "!", "~", "<", ">", "=", "?", ":", ";", ",",
    ".", "(", ")", "[", "]", "{", "}",
];

fn is_alpha(c: u8) -> bool {
    c.is_ascii_alphabetic()
}

fn is_digit(c: u8) -> bool {
    c.is_ascii_digit()
}

fn lex_err(src: &str, start: usize, end: Option<usize>) -> CompileErr {
    let text = match end {
        Some(e) => &src[start..e],
        None => &src[start..],
    };
    CompileErr::iae(&format!("unexpected character [{}].", text), start)
        .with_nil_cause("lexer_no_viable_alt_exception")
}

pub fn lex(src: &str) -> Result<Vec<Tok>, CompileErr> {
    let b = src.as_bytes();
    let n = b.len();
    let mut toks: Vec<Tok> = Vec::new();
    let mut i = 0;
    let tok = |kind: TokKind, text: String, pos: usize, end: usize| Tok { kind, text, pos, end, flags: String::new() };
    while i < n {
        let c = b[i];
        if c == b' ' || c == b'\t' || c == b'\n' || c == b'\r' {
            i += 1;
            continue;
        }
        if c == b'/' && i + 1 < n && b[i + 1] == b'/' {
            while i < n && b[i] != b'\n' {
                i += 1;
            }
            continue;
        }
        if c == b'/' && i + 1 < n && b[i + 1] == b'*' {
            match src[i + 2..].find("*/") {
                Some(j) => i = i + 2 + j + 2,
                None => i = n,
            }
            continue;
        }
        let start = i;
        let slash_is_regex = match toks.last() {
            None => true,
            Some(t) => match t.kind {
                TokKind::Ident | TokKind::Type | TokKind::Int | TokKind::Float | TokKind::Str | TokKind::Regex => false,
                TokKind::Op => !matches!(t.text.as_str(), ")" | "]" | "}" | "++" | "--"),
                TokKind::Eof => true,
            },
        };
        if c == b'\'' || c == b'"' {
            let q = c;
            let mut out: Vec<u8> = Vec::new();
            let mut j = i + 1;
            let mut closed = false;
            while j < n {
                let ch = b[j];
                if ch == b'\\' {
                    if j + 1 < n && (b[j + 1] == b'\\' || b[j + 1] == q) {
                        out.push(b[j + 1]);
                        j += 2;
                        continue;
                    }
                    let mut end = (j + 2).min(n);
                    while !src.is_char_boundary(end) {
                        end += 1;
                    }
                    let qs = q as char;
                    return Err(CompileErr::iae(
                        &format!(
                            "unexpected character [{}]. The only valid escape sequences in strings starting with [{}] are [\\\\] and [\\{}].",
                            &src[start..end],
                            qs,
                            qs
                        ),
                        start,
                    )
                    .with_nil_cause("lexer_no_viable_alt_exception"));
                }
                if ch == q {
                    closed = true;
                    j += 1;
                    break;
                }
                out.push(ch);
                j += 1;
            }
            if !closed {
                return Err(lex_err(src, start, None));
            }
            toks.push(tok(TokKind::Str, String::from_utf8_lossy(&out).into_owned(), start, j));
            i = j;
        } else if is_digit(c) || (c == b'.' && i + 1 < n && is_digit(b[i + 1])) {
            let mut j = i;
            let mut is_float = false;
            if c == b'0' && j + 1 < n && (b[j + 1] == b'x' || b[j + 1] == b'X') {
                j += 2;
                while j < n && b[j].is_ascii_hexdigit() {
                    j += 1;
                }
                if j < n && (b[j] == b'l' || b[j] == b'L') {
                    j += 1;
                }
                toks.push(tok(TokKind::Int, src[i..j].to_string(), start, j));
                i = j;
                continue;
            }
            while j < n && is_digit(b[j]) {
                j += 1;
            }
            if j < n && b[j] == b'.' && j + 1 < n && is_digit(b[j + 1]) {
                is_float = true;
                j += 1;
                while j < n && is_digit(b[j]) {
                    j += 1;
                }
            } else if j < n && b[j] == b'.' && !(j + 1 < n && (is_alpha(b[j + 1]) || b[j + 1] == b'_')) {
                // "1." is a double literal
                is_float = true;
                j += 1;
            }
            if j < n && (b[j] == b'e' || b[j] == b'E') {
                let mut k = j + 1;
                if k < n && (b[k] == b'+' || b[k] == b'-') {
                    k += 1;
                }
                if k < n && is_digit(b[k]) {
                    is_float = true;
                    j = k;
                    while j < n && is_digit(b[j]) {
                        j += 1;
                    }
                }
            }
            if j < n {
                match b[j] {
                    b'f' | b'F' | b'd' | b'D' => {
                        is_float = true;
                        j += 1;
                    }
                    b'l' | b'L' if !is_float => j += 1,
                    _ => {}
                }
            }
            let kind = if is_float { TokKind::Float } else { TokKind::Int };
            toks.push(tok(kind, src[i..j].to_string(), start, j));
            i = j;
        } else if is_alpha(c) || c == b'_' || c == b'$' {
            let mut j = i;
            while j < n && (is_alpha(b[j]) || b[j] == b'_' || b[j] == b'$' || is_digit(b[j])) {
                j += 1;
            }
            let word = &src[i..j];
            let kind = if is_type_name(word) { TokKind::Type } else { TokKind::Ident };
            toks.push(tok(kind, word.to_string(), start, j));
            i = j;
        } else if c == b'/' && slash_is_regex {
            let mut j = i + 1;
            let mut out: Vec<u8> = Vec::new();
            while j < n && b[j] != b'/' && b[j] != b'\n' {
                if b[j] == b'\\' && j + 1 < n {
                    out.push(b[j]);
                    out.push(b[j + 1]);
                    j += 2;
                    continue;
                }
                out.push(b[j]);
                j += 1;
            }
            if j >= n || b[j] != b'/' {
                return Err(lex_err(src, start, None));
            }
            j += 1;
            let fs = j;
            while j < n && b"cilmsUux".contains(&b[j]) {
                j += 1;
            }
            toks.push(Tok {
                kind: TokKind::Regex,
                text: String::from_utf8_lossy(&out).into_owned(),
                pos: start,
                end: j,
                flags: src[fs..j].to_string(),
            });
            i = j;
        } else {
            let rest = &src[i..];
            match OPS.iter().find(|op| rest.starts_with(**op)) {
                Some(op) => {
                    toks.push(tok(TokKind::Op, op.to_string(), start, i + op.len()));
                    i += op.len();
                }
                None => {
                    let ch = rest.chars().next().unwrap();
                    return Err(lex_err(src, start, Some(start + ch.len_utf8())));
                }
            }
        }
    }
    toks.push(Tok { kind: TokKind::Eof, text: String::new(), pos: n, end: n, flags: String::new() });
    Ok(toks)
}
