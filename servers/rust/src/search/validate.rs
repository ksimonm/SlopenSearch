//! Shard-level query validation (what OpenSearch rejects before running a query).

use super::fetch::*;
use crate::search::eval::{
    as_query_list, single_entry,
};
use crate::dates::parse_date;
use crate::errors::ApiError;
use crate::jsonv::to_f64;
use crate::model::Index;
use serde_json::Value;

pub fn validate_query(idx: &Index, query: &Value) -> Result<(), ApiError> {
    let m = match query.as_object() {
        Some(m) => m,
        None => return Ok(()),
    };
    for (clause, raw) in m {
        let known = matches!(
            clause.as_str(),
            "match_all" | "match" | "term" | "terms" | "range" | "bool" | "match_phrase"
                | "multi_match" | "wildcard" | "fuzzy" | "exists" | "prefix"
        );
        if !known {
            return Err(ApiError::parsing(&format!("unknown query [{}]", clause)));
        }
        if clause == "range" {
            if let Some(rm) = raw.as_object() {
                if let Some((field, cond_raw)) = single_entry(rm) {
                    let ft = field_type(idx, field);
                    if let Some(cond) = cond_raw.as_object() {
                        for bound in cond.values() {
                            if ft == "date" {
                                if parse_date(bound).is_none() {
                                    return Err(ApiError::parsing(&format!(
                                        "failed to parse date [{}]",
                                        field
                                    )));
                                }
                            } else if is_numeric_type(&ft) && to_f64(bound).is_none() {
                                return Err(ApiError::parsing(&format!(
                                    "failed to parse number [{}]",
                                    field
                                )));
                            }
                        }
                    }
                }
            }
        }
        if clause == "bool" {
            if let Some(bm) = raw.as_object() {
                for key in ["must", "should", "filter", "must_not"] {
                    for q in as_query_list(bm.get(key)) {
                        validate_query(idx, &q)?;
                    }
                }
            }
        }
    }
    Ok(())
}
