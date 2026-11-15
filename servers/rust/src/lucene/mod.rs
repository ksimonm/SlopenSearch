//! Language analysis: OpenSearch's language analyzers with their exact
//! Lucene 10.5.1 chains, the language token filters (stemmers, normalizers,
//! stop sets, elision, CJK bigrams) and the standard tokenizer they build on.
#![allow(dead_code)]

pub(crate) mod arabic_family;
pub(crate) mod brazilian;
mod brazilian_data;
pub(crate) mod case_width;
pub(crate) mod chains;
mod char_data;
pub(crate) mod english_misc;
pub(crate) mod germanic_stem;
pub(crate) mod greek;
pub(crate) mod indic;
mod java_char_data;
pub(crate) mod kstem;
mod kstem_data;
pub(crate) mod other_light_stem;
pub(crate) mod porter;
pub(crate) mod romance_stem;
pub(crate) mod rslp;
mod rslp_data;
pub(crate) mod slavic_baltic;
pub(crate) mod stem_util;
pub(crate) mod std_tok;
mod std_tok_data;
mod stopwords_data;
pub(crate) mod thai;
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
