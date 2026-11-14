//! The search engine: query evaluation and scoring (`eval`), analyzed
//! query terms, highlighting, k-NN queries, sorting, source filtering, inner
//! hits, and the painless/expressions glue for script queries, scores,
//! fields and sorts.

pub(crate) mod analyzed;
pub(crate) mod eval;
pub(crate) mod eval_extra;
pub(crate) mod expressions;
pub(crate) mod fetch;
pub(crate) mod highlight;
pub(crate) mod hl_analyzed;
pub(crate) mod inner_hits;
pub(crate) mod knn_query;
pub(crate) mod painless_doc;
pub(crate) mod request;
pub(crate) mod script_ctx;
pub(crate) mod script_env;
pub(crate) mod sort;
pub(crate) mod source;
pub(crate) mod validate;
