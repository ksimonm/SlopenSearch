//! Search contexts kept between requests: scrolls and points in time.

use serde_json::Value;
use std::collections::HashMap;

use super::Index;

#[derive(Clone)]
pub struct PitState {
    pub indices: HashMap<String, Index>,
    pub keep_ms: i64,
    pub creation_time: i64,
}

#[derive(Clone)]
pub struct ScrollState {
    pub body: Value,
    pub names: Vec<String>,
    pub url_name: String,
    pub pos: usize,
    pub page: usize,
    pub slice: Option<(usize, usize)>,
    /// The searched indices as they were when the scroll started: later pages
    /// read this point-in-time view, not the live indices.
    pub snapshot: HashMap<String, Index>,
    /// Query parameters of the starting request that apply to every page.
    pub params: Vec<(String, String)>,
    /// The body of the starting request (error locations).
    pub raw_body: Vec<u8>,
    /// The hits of the following pages, searched together (see `http::scroll_pages`).
    pub window: Option<crate::http::scroll_pages::Window>,
}
