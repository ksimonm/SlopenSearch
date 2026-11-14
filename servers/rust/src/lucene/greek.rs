//! Ports of Lucene's el/GreekLowerCaseFilter and el/GreekStemmer (Ntais'
//! stemmer, as used by GreekStemFilter).
//!
//! The stemmer expects input already folded by GreekLowerCaseFilter (lower
//! case, no tonos/dialytika, final sigma as σ). Like the Java code, some rules
//! grow `len` again after shrinking it and rely on the characters still being
//! in the buffer, so the whole term slice is kept until the final truncate.

use super::case_width::java_to_lower;
use super::stem_util::apply;

// ---------------------------------------------------------------------------
// GreekLowerCaseFilter

fn lower_case(codepoint: u32) -> u32 {
    match codepoint {
        // There are two lowercase forms of sigma (U+03C2 final, U+03C3
        // otherwise); standardize both to U+03C3
        0x03C2 => 0x03C3,
        // Some greek characters contain diacritics; remove the tonos/dialytika.
        0x0386 | 0x03AC => 0x03B1,                            // alpha
        0x0388 | 0x03AD => 0x03B5,                            // epsilon
        0x0389 | 0x03AE => 0x03B7,                            // eta
        0x038A | 0x03AA | 0x03AF | 0x03CA | 0x0390 => 0x03B9, // iota
        0x038E | 0x03AB | 0x03CD | 0x03CB | 0x03B0 => 0x03C5, // upsilon
        0x038C | 0x03CC => 0x03BF,                            // omicron
        0x038F | 0x03CE => 0x03C9,                            // omega
        // The previous implementation did this conversion (reserved code point).
        0x03A2 => 0x03C2,
        _ => java_to_lower(codepoint),
    }
}

/// GreekLowerCaseFilter (`lowercase` filter with `language: greek`, and the
/// greek analyzer): Java lowercase per code point plus Greek accent and final
/// sigma folding. Applies to every token (no keyword check in the filter).
pub fn greek_lowercase(term: &mut Vec<u16>) {
    let s = term.as_mut_slice();
    let limit = s.len();
    let mut i = 0;
    while i < limit {
        // Character.codePointAt(chArray, i, chLen)
        let mut cp = s[i] as u32;
        if (0xD800..0xDC00).contains(&cp) && i + 1 < limit && (0xDC00..0xE000).contains(&(s[i + 1] as u32)) {
            cp = 0x10000 + ((cp - 0xD800) << 10) + (s[i + 1] as u32 - 0xDC00);
        }
        // Character.toChars(lowerCase(cp), chArray, i) (Java's simple
        // lowercase never changes the UTF-16 length)
        let lc = lower_case(cp);
        if lc < 0x10000 {
            s[i] = lc as u16;
            i += 1;
        } else {
            let v = lc - 0x10000;
            s[i] = 0xD800 + (v >> 10) as u16;
            s[i + 1] = 0xDC00 + (v & 0x3FF) as u16;
            i += 2;
        }
    }
}

// ---------------------------------------------------------------------------
// GreekStemmer

/// GreekStemmer.endsWith (no allocation: compares UTF-16 units from the end).
fn ends_with(s: &[u16], len: usize, suffix: &str) -> bool {
    // all Greek letters are single BMP units; compare backwards like Java
    let mut word = s[..len].iter().rev();
    for c in suffix.chars().rev() {
        match word.next() {
            Some(&u) if u as u32 == c as u32 => {}
            _ => return false,
        }
    }
    true
}

fn ends_with_any(s: &[u16], len: usize, suffixes: &[&str]) -> bool {
    suffixes.iter().any(|suf| ends_with(s, len, suf))
}

/// CharArraySet.contains(s, 0, len) on a case-sensitive set.
fn contains(set: &[&str], s: &[u16], len: usize) -> bool {
    // (Greek letters are 2 UTF-8 bytes each)
    set.iter().any(|w| w.len() == 2 * len && ends_with(s, len, w))
}

fn ends_with_vowel(s: &[u16], len: usize) -> bool {
    if len == 0 {
        return false;
    }
    matches!(char::from_u32(s[len - 1] as u32), Some('α' | 'ε' | 'η' | 'ι' | 'ο' | 'υ' | 'ω'))
}

fn ends_with_vowel_no_y(s: &[u16], len: usize) -> bool {
    if len == 0 {
        return false;
    }
    matches!(char::from_u32(s[len - 1] as u32), Some('α' | 'ε' | 'η' | 'ι' | 'ο' | 'ω'))
}

fn set_chars(s: &mut [u16], at: usize, chars: &str) {
    for (i, ch) in chars.chars().enumerate() {
        s[at + i] = ch as u16;
    }
}

fn stem(s: &mut [u16], len: usize) -> usize {
    if len < 4 {
        return len; // too short
    }
    let orig_len = len;
    // "short rules": if it hits one of these, it skips the "long list"
    let mut len = rule0(s, len);
    len = rule1(s, len);
    len = rule2(s, len);
    len = rule3(s, len);
    len = rule4(s, len);
    len = rule5(s, len);
    len = rule6(s, len);
    len = rule7(s, len);
    len = rule8(s, len);
    len = rule9(s, len);
    len = rule10(s, len);
    len = rule11(s, len);
    len = rule12(s, len);
    len = rule13(s, len);
    len = rule14(s, len);
    len = rule15(s, len);
    len = rule16(s, len);
    len = rule17(s, len);
    len = rule18(s, len);
    len = rule19(s, len);
    len = rule20(s, len);
    // "long list"
    if len == orig_len {
        len = rule21(s, len);
    }
    rule22(s, len)
}

fn rule0(s: &[u16], len: usize) -> usize {
    if len > 9 && ends_with_any(s, len, &["καθεστωτοσ", "καθεστωτων"]) {
        return len - 4;
    }
    if len > 8 && ends_with_any(s, len, &["γεγονοτοσ", "γεγονοτων"]) {
        return len - 4;
    }
    if len > 8 && ends_with(s, len, "καθεστωτα") {
        return len - 3;
    }
    if len > 7 && ends_with_any(s, len, &["τατογιου", "τατογιων"]) {
        return len - 4;
    }
    if len > 7 && ends_with(s, len, "γεγονοτα") {
        return len - 3;
    }
    if len > 7 && ends_with(s, len, "καθεστωσ") {
        return len - 2;
    }
    // (Java parenthesizes only the first endsWith here: `len > 6 && a || b || ...`)
    if len > 6 && ends_with(s, len, "σκαγιου")
        || ends_with_any(
            s,
            len,
            &["σκαγιων", "ολογιου", "ολογιων", "κρεατοσ", "κρεατων", "περατοσ", "περατων", "τερατοσ", "τερατων"],
        )
    {
        return len - 4;
    }
    if len > 6 && ends_with(s, len, "τατογια") {
        return len - 3;
    }
    if len > 6 && ends_with(s, len, "γεγονοσ") {
        return len - 2;
    }
    if len > 5 && ends_with_any(s, len, &["φαγιου", "φαγιων", "σογιου", "σογιων"]) {
        return len - 4;
    }
    if len > 5 && ends_with_any(s, len, &["σκαγια", "ολογια", "κρεατα", "περατα", "τερατα"]) {
        return len - 3;
    }
    if len > 4 && ends_with_any(s, len, &["φαγια", "σογια", "φωτοσ", "φωτων"]) {
        return len - 3;
    }
    if len > 4 && ends_with_any(s, len, &["κρεασ", "περασ", "τερασ"]) {
        return len - 2;
    }
    if len > 3 && ends_with(s, len, "φωτα") {
        return len - 2;
    }
    if len > 2 && ends_with(s, len, "φωσ") {
        return len - 1;
    }
    len
}

fn rule1(s: &[u16], mut len: usize) -> usize {
    if len > 4 && ends_with_any(s, len, &["αδεσ", "αδων"]) {
        len -= 4;
        if !ends_with_any(s, len, &["οκ", "μαμ", "μαν", "μπαμπ", "πατερ", "γιαγι", "νταντ", "κυρ", "θει", "πεθερ"]) {
            len += 2; // add back -αδ
        }
    }
    len
}

fn rule2(s: &[u16], mut len: usize) -> usize {
    if len > 4 && ends_with_any(s, len, &["εδεσ", "εδων"]) {
        len -= 4;
        if ends_with_any(s, len, &["οπ", "ιπ", "εμπ", "υπ", "γηπ", "δαπ", "κρασπ", "μιλ"]) {
            len += 2; // add back -εδ
        }
    }
    len
}

fn rule3(s: &[u16], mut len: usize) -> usize {
    if len > 5 && ends_with_any(s, len, &["ουδεσ", "ουδων"]) {
        len -= 5;
        if ends_with_any(
            s,
            len,
            &[
                "αρκ", "καλιακ", "πεταλ", "λιχ", "πλεξ", "σκ", "σ", "φλ", "φρ", "βελ", "λουλ", "χν", "σπ", "τραγ", "φε",
            ],
        ) {
            len += 3; // add back -ουδ
        }
    }
    len
}

const EXC4: &[&str] = &["θ", "δ", "ελ", "γαλ", "ν", "π", "ιδ", "παρ"];

fn rule4(s: &[u16], mut len: usize) -> usize {
    if len > 3 && ends_with_any(s, len, &["εωσ", "εων"]) {
        len -= 3;
        if contains(EXC4, s, len) {
            len += 1; // add back -ε
        }
    }
    len
}

fn rule5(s: &[u16], mut len: usize) -> usize {
    if len > 2 && ends_with(s, len, "ια") {
        len -= 2;
        if ends_with_vowel(s, len) {
            len += 1; // add back -ι
        }
    } else if len > 3 && ends_with_any(s, len, &["ιου", "ιων"]) {
        len -= 3;
        if ends_with_vowel(s, len) {
            len += 1; // add back -ι
        }
    }
    len
}

const EXC6: &[&str] = &[
    "αλ", "αδ", "ενδ", "αμαν", "αμμοχαλ", "ηθ", "ανηθ", "αντιδ", "φυσ", "βρωμ", "γερ", "εξωδ", "καλπ", "καλλιν",
    "καταδ", "μουλ", "μπαν", "μπαγιατ", "μπολ", "μποσ", "νιτ", "ξικ", "συνομηλ", "πετσ", "πιτσ", "πικαντ", "πλιατσ",
    "ποστελν", "πρωτοδ", "σερτ", "συναδ", "τσαμ", "υποδ", "φιλον", "φυλοδ", "χασ",
];

fn rule6(s: &[u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 3 && ends_with_any(s, len, &["ικα", "ικο"]) {
        len -= 3;
        removed = true;
    } else if len > 4 && ends_with_any(s, len, &["ικου", "ικων"]) {
        len -= 4;
        removed = true;
    }
    if removed && (ends_with_vowel(s, len) || contains(EXC6, s, len)) {
        len += 2; // add back -ικ
    }
    len
}

const EXC7: &[&str] = &["αναπ", "αποθ", "αποκ", "αποστ", "βουβ", "ξεθ", "ουλ", "πεθ", "πικρ", "ποτ", "σιχ", "χ"];

fn rule7(s: &[u16], mut len: usize) -> usize {
    if len == 5 && ends_with(s, len, "αγαμε") {
        return len - 1;
    }
    if len > 7 && ends_with(s, len, "ηθηκαμε") {
        len -= 7;
    } else if len > 6 && ends_with(s, len, "ουσαμε") {
        len -= 6;
    } else if len > 5 && ends_with_any(s, len, &["αγαμε", "ησαμε", "ηκαμε"]) {
        len -= 5;
    }
    if len > 3 && ends_with(s, len, "αμε") {
        len -= 3;
        if contains(EXC7, s, len) {
            len += 2; // add back -αμ
        }
    }
    len
}

const EXC8A: &[&str] = &["τρ", "τσ"];

const EXC8B: &[&str] = &[
    "βετερ", "βουλκ", "βραχμ", "γ", "δραδουμ", "θ", "καλπουζ", "καστελ", "κορμορ", "λαοπλ", "μωαμεθ", "μ",
    "μουσουλμ", "ν", "ουλ", "π", "πελεκ", "πλ", "πολισ", "πορτολ", "σαρακατσ", "σουλτ", "τσαρλατ", "ορφ", "τσιγγ",
    "τσοπ", "φωτοστεφ", "χ", "ψυχοπλ", "αγ", "ορφ", "γαλ", "γερ", "δεκ", "διπλ", "αμερικαν", "ουρ", "πιθ", "πουριτ",
    "σ", "ζωντ", "ικ", "καστ", "κοπ", "λιχ", "λουθηρ", "μαιντ", "μελ", "σιγ", "σπ", "στεγ", "τραγ", "τσαγ", "φ", "ερ",
    "αδαπ", "αθιγγ", "αμηχ", "ανικ", "ανοργ", "απηγ", "απιθ", "ατσιγγ", "βασ", "βασκ", "βαθυγαλ", "βιομηχ",
    "βραχυκ", "διατ", "διαφ", "ενοργ", "θυσ", "καπνοβιομηχ", "καταγαλ", "κλιβ", "κοιλαρφ", "λιβ", "μεγλοβιομηχ",
    "μικροβιομηχ", "νταβ", "ξηροκλιβ", "ολιγοδαμ", "ολογαλ", "πενταρφ", "περηφ", "περιτρ", "πλατ", "πολυδαπ",
    "πολυμηχ", "στεφ", "ταβ", "τετ", "υπερηφ", "υποκοπ", "χαμηλοδαπ", "ψηλοταβ",
];

fn rule8(s: &mut [u16], mut len: usize) -> usize {
    let mut removed = false;
    // (as in Java, only the first endsWith of each group is guarded by `len >`)
    if len > 8 && ends_with(s, len, "ιουντανε") {
        len -= 8;
        removed = true;
    } else if len > 7 && ends_with(s, len, "ιοντανε") || ends_with_any(s, len, &["ουντανε", "ηθηκανε"]) {
        len -= 7;
        removed = true;
    } else if len > 6 && ends_with(s, len, "ιοτανε") || ends_with_any(s, len, &["οντανε", "ουσανε"]) {
        len -= 6;
        removed = true;
    } else if len > 5 && ends_with(s, len, "αγανε") || ends_with_any(s, len, &["ησανε", "οτανε", "ηκανε"]) {
        len -= 5;
        removed = true;
    }
    if removed && contains(EXC8A, s, len) {
        // add -αγαν (we removed > 4 chars so it's safe)
        len += 4;
        set_chars(s, len - 4, "αγαν");
    }
    if len > 3 && ends_with(s, len, "ανε") {
        len -= 3;
        if ends_with_vowel_no_y(s, len) || contains(EXC8B, s, len) {
            len += 2; // add back -αν
        }
    }
    len
}

const EXC9: &[&str] = &[
    "αβαρ", "βεν", "εναρ", "αβρ", "αδ", "αθ", "αν", "απλ", "βαρον", "ντρ", "σκ", "κοπ", "μπορ", "νιφ", "παγ",
    "παρακαλ", "σερπ", "σκελ", "συρφ", "τοκ", "υ", "δ", "εμ", "θαρρ", "θ",
];

fn rule9(s: &[u16], mut len: usize) -> usize {
    if len > 5 && ends_with(s, len, "ησετε") {
        len -= 5;
    }
    if len > 3 && ends_with(s, len, "ετε") {
        len -= 3;
        if contains(EXC9, s, len)
            || ends_with_vowel_no_y(s, len)
            || ends_with_any(
                s,
                len,
                &[
                    "οδ", "αιρ", "φορ", "ταθ", "διαθ", "σχ", "ενδ", "ευρ", "τιθ", "υπερθ", "ραθ", "ενθ", "ροθ", "σθ",
                    "πυρ", "αιν", "συνδ", "συν", "συνθ", "χωρ", "πον", "βρ", "καθ", "ευθ", "εκθ", "νετ", "ρον", "αρκ",
                    "βαρ", "βολ", "ωφελ",
                ],
            )
        {
            len += 2; // add back -ετ
        }
    }
    len
}

fn rule10(s: &mut [u16], mut len: usize) -> usize {
    if len > 5 && ends_with_any(s, len, &["οντασ", "ωντασ"]) {
        len -= 5;
        if len == 3 && ends_with(s, len, "αρχ") {
            len += 3; // add back *ντ
            set_chars(s, len - 3, "ο");
        }
        if ends_with(s, len, "κρε") {
            len += 3; // add back *ντ
            set_chars(s, len - 3, "ω");
        }
    }
    len
}

fn rule11(s: &mut [u16], mut len: usize) -> usize {
    if len > 6 && ends_with(s, len, "ομαστε") {
        len -= 6;
        if len == 2 && ends_with(s, len, "ον") {
            len += 5; // add back -ομαστ
        }
    } else if len > 7 && ends_with(s, len, "ιομαστε") {
        len -= 7;
        if len == 2 && ends_with(s, len, "ον") {
            len += 5;
            set_chars(s, len - 5, "ομαστ");
        }
    }
    len
}

const EXC12A: &[&str] = &["π", "απ", "συμπ", "ασυμπ", "ακαταπ", "αμεταμφ"];
const EXC12B: &[&str] = &["αλ", "αρ", "εκτελ", "ζ", "μ", "ξ", "παρακαλ", "αρ", "προ", "νισ"];

fn rule12(s: &[u16], mut len: usize) -> usize {
    if len > 5 && ends_with(s, len, "ιεστε") {
        len -= 5;
        if contains(EXC12A, s, len) {
            len += 4; // add back -ιεστ
        }
    }
    if len > 4 && ends_with(s, len, "εστε") {
        len -= 4;
        if contains(EXC12B, s, len) {
            len += 3; // add back -εστ
        }
    }
    len
}

const EXC13: &[&str] = &["διαθ", "θ", "παρακαταθ", "προσθ", "συνθ"];

fn rule13(s: &[u16], mut len: usize) -> usize {
    if len > 6 && ends_with(s, len, "ηθηκεσ") {
        len -= 6;
    } else if len > 5 && ends_with_any(s, len, &["ηθηκα", "ηθηκε"]) {
        len -= 5;
    }
    let mut removed = false;
    if len > 4 && ends_with(s, len, "ηκεσ") {
        len -= 4;
        removed = true;
    } else if len > 3 && ends_with_any(s, len, &["ηκα", "ηκε"]) {
        len -= 3;
        removed = true;
    }
    if removed && (contains(EXC13, s, len) || ends_with_any(s, len, &["σκωλ", "σκουλ", "ναρθ", "σφ", "οθ", "πιθ"])) {
        len += 2; // add back the -ηκ
    }
    len
}

const EXC14: &[&str] = &[
    "φαρμακ", "χαδ", "αγκ", "αναρρ", "βρομ", "εκλιπ", "λαμπιδ", "λεχ", "μ", "πατ", "ρ", "λ", "μεδ", "μεσαζ",
    "υποτειν", "αμ", "αιθ", "ανηκ", "δεσποζ", "ενδιαφερ", "δε", "δευτερευ", "καθαρευ", "πλε", "τσα",
];

fn rule14(s: &[u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 5 && ends_with(s, len, "ουσεσ") {
        len -= 5;
        removed = true;
    } else if len > 4 && ends_with_any(s, len, &["ουσα", "ουσε"]) {
        len -= 4;
        removed = true;
    }
    if removed
        && (contains(EXC14, s, len)
            || ends_with_vowel(s, len)
            || ends_with_any(
                s,
                len,
                &[
                    "ποδαρ", "βλεπ", "πανταχ", "φρυδ", "μαντιλ", "μαλλ", "κυματ", "λαχ", "ληγ", "φαγ", "ομ", "πρωτ",
                ],
            ))
    {
        len += 3; // add back -ουσ
    }
    len
}

const EXC15A: &[&str] = &[
    "αβαστ", "πολυφ", "αδηφ", "παμφ", "ρ", "ασπ", "αφ", "αμαλ", "αμαλλι", "ανυστ", "απερ", "ασπαρ", "αχαρ", "δερβεν",
    "δροσοπ", "ξεφ", "νεοπ", "νομοτ", "ολοπ", "ομοτ", "προστ", "προσωποπ", "συμπ", "συντ", "τ", "υποτ", "χαρ", "αειπ",
    "αιμοστ", "ανυπ", "αποτ", "αρτιπ", "διατ", "εν", "επιτ", "κροκαλοπ", "σιδηροπ", "λ", "ναυ", "ουλαμ", "ουρ", "π",
    "τρ", "μ",
];
const EXC15B: &[&str] = &["ψοφ", "ναυλοχ"];

fn rule15(s: &[u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 4 && ends_with(s, len, "αγεσ") {
        len -= 4;
        removed = true;
    } else if len > 3 && ends_with_any(s, len, &["αγα", "αγε"]) {
        len -= 3;
        removed = true;
    }
    if removed {
        let cond1 = contains(EXC15A, s, len)
            || ends_with_any(s, len, &["οφ", "πελ", "χορτ", "λλ", "σφ", "ρπ", "φρ", "πρ", "λοχ", "σμην"]);
        let cond2 = contains(EXC15B, s, len) || ends_with(s, len, "κολλ");
        if cond1 && !cond2 {
            len += 2; // add back -αγ
        }
    }
    len
}

const EXC16: &[&str] = &["ν", "χερσον", "δωδεκαν", "ερημον", "μεγαλον", "επταν"];

fn rule16(s: &[u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 4 && ends_with(s, len, "ησου") {
        len -= 4;
        removed = true;
    } else if len > 3 && ends_with_any(s, len, &["ησε", "ησα"]) {
        len -= 3;
        removed = true;
    }
    if removed && contains(EXC16, s, len) {
        len += 2; // add back -ησ
    }
    len
}

const EXC17: &[&str] = &["ασβ", "σβ", "αχρ", "χρ", "απλ", "αειμν", "δυσχρ", "ευχρ", "κοινοχρ", "παλιμψ"];

fn rule17(s: &[u16], mut len: usize) -> usize {
    if len > 4 && ends_with(s, len, "ηστε") {
        len -= 4;
        if contains(EXC17, s, len) {
            len += 3; // add back the -ηστ
        }
    }
    len
}

const EXC18: &[&str] = &["ν", "ρ", "σπι", "στραβομουτσ", "κακομουτσ", "εξων"];

fn rule18(s: &mut [u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 6 && ends_with_any(s, len, &["ησουνε", "ηθουνε"]) {
        len -= 6;
        removed = true;
    } else if len > 4 && ends_with(s, len, "ουνε") {
        len -= 4;
        removed = true;
    }
    if removed && contains(EXC18, s, len) {
        len += 3;
        set_chars(s, len - 3, "ουν");
    }
    len
}

const EXC19: &[&str] = &["παρασουσ", "φ", "χ", "ωριοπλ", "αζ", "αλλοσουσ", "ασουσ"];

fn rule19(s: &mut [u16], mut len: usize) -> usize {
    let mut removed = false;
    if len > 6 && ends_with_any(s, len, &["ησουμε", "ηθουμε"]) {
        len -= 6;
        removed = true;
    } else if len > 4 && ends_with(s, len, "ουμε") {
        len -= 4;
        removed = true;
    }
    if removed && contains(EXC19, s, len) {
        len += 3;
        set_chars(s, len - 3, "ουμ");
    }
    len
}

fn rule20(s: &[u16], len: usize) -> usize {
    if len > 5 && ends_with_any(s, len, &["ματων", "ματοσ"]) {
        len - 3
    } else if len > 4 && ends_with(s, len, "ματα") {
        len - 2
    } else {
        len
    }
}

fn rule21(s: &[u16], len: usize) -> usize {
    if len > 9 && ends_with(s, len, "ιοντουσαν") {
        return len - 9;
    }
    if len > 8 && ends_with_any(s, len, &["ιομασταν", "ιοσασταν", "ιουμαστε", "οντουσαν"]) {
        return len - 8;
    }
    if len > 7
        && ends_with_any(
            s,
            len,
            &[
                "ιεμαστε", "ιεσαστε", "ιομουνα", "ιοσαστε", "ιοσουνα", "ιουνται", "ιουνταν", "ηθηκατε", "ομασταν",
                "οσασταν", "ουμαστε",
            ],
        )
    {
        return len - 7;
    }
    if len > 6
        && ends_with_any(
            s,
            len,
            &[
                "ιομουν", "ιονταν", "ιοσουν", "ηθειτε", "ηθηκαν", "ομουνα", "οσαστε", "οσουνα", "ουνται", "ουνταν",
                "ουσατε",
            ],
        )
    {
        return len - 6;
    }
    if len > 5
        && ends_with_any(
            s,
            len,
            &[
                "αγατε", "ιεμαι", "ιεται", "ιεσαι", "ιοταν", "ιουμα", "ηθεισ", "ηθουν", "ηκατε", "ησατε", "ησουν",
                "ομουν", "ονται", "ονταν", "οσουν", "ουμαι", "ουσαν",
            ],
        )
    {
        return len - 5;
    }
    if len > 4
        && ends_with_any(
            s,
            len,
            &[
                "αγαν", "αμαι", "ασαι", "αται", "ειτε", "εσαι", "εται", "ηδεσ", "ηδων", "ηθει", "ηκαν", "ησαν", "ησει",
                "ησεσ", "ομαι", "οταν",
            ],
        )
    {
        return len - 4;
    }
    if len > 3 && ends_with_any(s, len, &["αει", "εισ", "ηθω", "ησω", "ουν", "ουσ"]) {
        return len - 3;
    }
    if len > 2 && ends_with_any(s, len, &["αν", "ασ", "αω", "ει", "εσ", "ησ", "οι", "οσ", "ου", "υσ", "ων"]) {
        return len - 2;
    }
    if len > 1 && ends_with_vowel(s, len) {
        return len - 1;
    }
    len
}

fn rule22(s: &[u16], len: usize) -> usize {
    if ends_with_any(s, len, &["εστερ", "εστατ"]) {
        return len - 5;
    }
    if ends_with_any(s, len, &["οτερ", "οτατ", "υτερ", "υτατ", "ωτερ", "ωτατ"]) {
        return len - 4;
    }
    len
}

/// GreekStemFilter: Ntais' Greek stemmer (terms shorter than 4 are kept).
pub fn greek_stem(term: &mut Vec<u16>) {
    apply(term, stem);
}
