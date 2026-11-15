//! Scroll continuation pages (see `search::handle_scroll`).
//!
//! A page is the search of the scroll's point-in-time view with `from` = the
//! scroll position and `size` = the page size. Instead of searching once per
//! page, a continuation searches a *window* of several pages and keeps it in
//! the scroll context; the following pages are cut from it. Apart from the
//! hits, a search response does not depend on `from`/`size` (`hits.max_score`
//! is the maximum over every hit, as in OpenSearch).

use serde_json::Value;
use std::cell::Cell;

thread_local! {
    static SCROLL_PAGE: Cell<bool> = const { Cell::new(false) };
}

/// Runs `f` as a scroll continuation: the result window limit
/// (`index.max_result_window` for `from + size`) does not apply, only the page
/// size is limited (checked when the scroll started).
pub fn as_scroll_page<R>(f: impl FnOnce() -> R) -> R {
    SCROLL_PAGE.with(|s| s.set(true));
    let r = f();
    SCROLL_PAGE.with(|s| s.set(false));
    r
}

pub fn in_scroll_page() -> bool {
    SCROLL_PAGE.with(|s| s.get())
}

/// How many hits a continuation search fetches for pages of `page` hits.
pub fn window_size(page: usize) -> usize {
    let pages = (4096 / page.max(1)).clamp(4, 64);
    page.saturating_mul(pages)
}

/// A cached window of hits: the response of the search with `from` = `start`
/// and `size` = `size`.
#[derive(Clone)]
pub struct Window {
    pub start: usize,
    pub size: usize,
    pub resp: Value,
}

impl Window {
    /// The response for the page at `pos` with `page` hits, when the window
    /// covers it (a short window means the hits end inside it). The hits of
    /// the page are moved out: pages are read once, in order.
    pub fn take_page(&mut self, pos: usize, page: usize) -> Option<Value> {
        if pos < self.start {
            return None;
        }
        let off = pos - self.start;
        let len = self.resp.get("hits")?.get("hits")?.as_array()?.len();
        if off + page > len && len >= self.size {
            return None;
        }
        let mut out = serde_json::Map::new();
        for (k, v) in self.resp.as_object_mut()? {
            if k != "hits" {
                out.insert(k.clone(), v.clone());
                continue;
            }
            let mut hm = serde_json::Map::new();
            for (hk, hv) in v.as_object_mut()? {
                if hk == "hits" {
                    let all = hv.as_array_mut()?;
                    let (lo, hi) = (off.min(len), (off + page).min(len));
                    let part: Vec<Value> = all[lo..hi].iter_mut().map(std::mem::take).collect();
                    hm.insert(hk.clone(), Value::Array(part));
                } else {
                    hm.insert(hk.clone(), hv.clone());
                }
            }
            out.insert(k.clone(), Value::Object(hm));
        }
        Some(Value::Object(out))
    }
}

/// Query parameters of the request that started a scroll that apply to every
/// page (the paging ones are the scroll's own).
pub fn carried_params(query: &[(String, String)]) -> Vec<(String, String)> {
    query
        .iter()
        .filter(|(k, _)| !matches!(k.as_str(), "scroll" | "from" | "size" | "request_cache" | "search_pipeline" | "verbose_pipeline" | "filter_path" | "pretty" | "human" | "error_trace"))
        .cloned()
        .collect()
}
