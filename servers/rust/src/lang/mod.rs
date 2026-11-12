//! Language analysis: OpenSearch's language analyzers with their exact
//! Lucene 10.5.1 chains, the language token filters (stemmers, normalizers,
//! stop sets, elision, CJK bigrams) and the standard tokenizer they build on.
#![allow(dead_code)]

pub mod arabic_family;
pub mod brazilian;
mod brazilian_data;
pub mod case_width;
pub mod chains;
mod char_data;
pub mod english_misc;
pub mod germanic_stem;
pub mod greek;
pub mod indic;
mod java_char_data;
pub mod kstem;
mod kstem_data;
pub mod other_light_stem;
pub mod porter;
pub mod romance_stem;
pub mod rslp;
mod rslp_data;
pub mod slavic_baltic;
pub mod snowball;
pub mod snowball_gen;
pub mod stem_util;
pub mod std_tok;
mod std_tok_data;
mod stopwords_data;
pub mod thai;
mod thai_data;

/// One token of an analysis chain. Offsets are UTF-16 code units.
#[derive(Clone, Debug)]
pub struct Tok {
    pub term: String,
    pub start: usize,
    pub end: usize,
    pub typ: String,
    pub pos_inc: usize,
    pub pos_len: usize,
    pub keyword: bool,
}
