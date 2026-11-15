//! Top-k for field sorts (see docs/engine-design.md).
//!
//! A search sorted by fields whose response only reads one page needs the
//! first `from + size` hits in sort order, not every hit sorted. The sort
//! cell of a document (`compute_sort_cell`: the value the comparator reads,
//! with missing values, `mode` and conversions applied) depends only on the
//! document and the sort spec, so it is computed once per index content and
//! spec into a *sort column* (by doc number) and reused by later searches.
//! Selection compares the cells exactly as the general sort does (integer
//! cells as `i128`, the rest with `cmp_sort_cells`, descending order reversed
//! for present values, ties by index order, which is what the stable sort of
//! the hits in index order gives), and `search_after` filters with the
//! comparison of `sort_after_gt`. The selected hits then go through the
//! general sort and rendering, so the response is the same.

use std::cmp::Ordering;
use std::sync::{Arc, Mutex};

use serde_json::Value;

use crate::model::{Index, Store};
use crate::search::sort::{cmp_sort_cells, compare_sort_values, compute_sort_cell, sort_after_value, MultiHit, SortSpec};

/// One document's sort cell for a spec, with its integer key.
pub(crate) struct Cell {
    pub cell: (bool, Value),
    pub int: Option<i128>,
}

fn int_key(v: &Value) -> Option<i128> {
    match v {
        Value::Number(n) => n.as_i64().map(i128::from).or_else(|| n.as_u64().map(i128::from)),
        _ => None,
    }
}

type Column = Arc<Vec<Option<Cell>>>;

static COLUMNS: Mutex<Vec<(String, Column)>> = Mutex::new(Vec::new());

/// What a field sort's cells depend on besides the document.
fn spec_key(sp: &SortSpec) -> String {
    format!(
        "{}\u{1}{}\u{1}{:?}\u{1}{:?}\u{1}{:?}\u{1}{:?}\u{1}{:?}",
        sp.field, sp.desc, sp.mode, sp.missing, sp.unmapped_type, sp.numeric_type, sp.nested_filter
    )
}

/// The sort column of field spec `sp` over the documents of `idx` (None:
/// index-backed execution is off, or some cell is an error, which the
/// general sort reports).
fn column(store: &Store, idx_name: &str, idx: &Index, sp: &SortSpec) -> Option<(u64, Column)> {
    crate::search::index::with_doc_ids(idx, |version, ids| {
        let key = format!(
            "{}\u{1}{}\u{1}{}\u{1}{}\u{1}{}",
            idx.uuid,
            version,
            idx.mappings.generation(),
            idx.settings.generation(),
            spec_key(sp)
        );
        if let Some(c) = COLUMNS.lock().unwrap_or_else(|e| e.into_inner()).iter().find(|e| e.0 == key) {
            return Some((version, c.1.clone()));
        }
        use rayon::prelude::*;
        let cells: Result<Vec<Option<Cell>>, ()> = ids
            .par_iter()
            .with_min_len(2048)
            .map(|id| {
                let Some(id) = id else { return Ok(None) };
                let Some(doc) = idx.docs.get(id) else { return Ok(None) };
                let h = MultiHit { idx_name, id, doc, score: 0.0, pos: 0, named: Vec::new() };
                match compute_sort_cell(store, &h, sp) {
                    Ok(cell) => {
                        let int = int_key(&cell.1);
                        Ok(Some(Cell { cell, int }))
                    }
                    Err(_) => Err(()),
                }
            })
            .collect();
        let col: Column = Arc::new(cells.ok()?);
        let mut cache = COLUMNS.lock().unwrap_or_else(|e| e.into_inner());
        cache.retain(|e| !e.0.starts_with(&format!("{}\u{1}", idx.uuid)) || e.0.split('\u{1}').nth(1) == Some(&version.to_string()));
        if cache.len() >= 16 {
            cache.remove(0);
        }
        cache.push((key, col.clone()));
        Some((version, col))
    })?
}

/// Whether the sort specs are ones this module selects for: fields (not
/// nested) and `_doc`.
pub(crate) fn eligible(specs: &[SortSpec]) -> bool {
    !specs.is_empty()
        && specs.iter().all(|sp| match sp.kind {
            0 => sp.nested.is_none() && !sp.field.is_empty() && !sp.field.starts_with('_'),
            4 => true,
            _ => false,
        })
}

/// A sort key part: a field cell, or the index-order position (`_doc`).
enum Part<'a> {
    Cell(&'a Cell),
    Pos(usize),
}

/// The candidates of one index to keep: index-order positions of the first
/// `want` hits in sort order after `after`, and the number of hits.
pub(crate) struct Selected {
    pub positions: Vec<usize>,
    pub count: usize,
}

/// Selects the page for `specs` from the candidates `cand` (index-order
/// position and doc number of each matching document). None: not possible
/// here (the general path decides).
pub(crate) fn select(
    store: &Store,
    idx_name: &str,
    idx: &Index,
    version: u64,
    cand: &[(usize, u32)],
    specs: &[SortSpec],
    after: Option<&[Value]>,
    want: usize,
) -> Option<Selected> {
    let mut cols: Vec<Option<Column>> = Vec::with_capacity(specs.len());
    for sp in specs {
        if sp.kind == 0 {
            let (v, c) = column(store, idx_name, idx, sp)?;
            if v != version {
                return None;
            }
            cols.push(Some(c));
        } else {
            cols.push(None);
        }
    }
    let parts = |pos: usize, doc: u32| -> Option<Vec<Part<'_>>> {
        let mut out = Vec::with_capacity(specs.len());
        for c in &cols {
            match c {
                Some(col) => out.push(Part::Cell(col.get(doc as usize)?.as_ref()?)),
                None => out.push(Part::Pos(pos)),
            }
        }
        Some(out)
    };
    // `general sort` order: per spec, integer cells by value, others with
    // `cmp_sort_cells`; descending reverses the order of present values
    let cmp = |a: &(usize, Vec<Part>), b: &(usize, Vec<Part>)| -> Ordering {
        for (i, sp) in specs.iter().enumerate() {
            let (c, present) = match (&a.1[i], &b.1[i]) {
                (Part::Pos(x), Part::Pos(y)) => (x.cmp(y), true),
                (Part::Cell(x), Part::Cell(y)) => {
                    let c = match (x.int, y.int) {
                        (Some(p), Some(q)) if x.cell.0 == y.cell.0 => p.cmp(&q),
                        _ => cmp_sort_cells(&x.cell, &y.cell),
                    };
                    (c, !x.cell.0 && !y.cell.0)
                }
                _ => return Ordering::Equal,
            };
            let c = if sp.desc && present { c.reverse() } else { c };
            if c != Ordering::Equal {
                return c;
            }
        }
        a.0.cmp(&b.0)
    };
    // search_after: the hit's values compared with the after values as
    // `sort_after_gt` does (integers as i128, the rest compare_sort_values)
    let after: Option<Vec<(Value, Option<i128>)>> = match after {
        Some(a) => {
            if a.len() != specs.len() {
                return None;
            }
            Some(
                a.iter()
                    .zip(specs)
                    .map(|(v, sp)| {
                        let v = sort_after_value(v, sp).unwrap_or(Value::Null);
                        let i = int_key(&v);
                        (v, i)
                    })
                    .collect(),
            )
        }
        None => None,
    };
    let after_ok = |p: &[Part]| -> bool {
        let Some(after) = &after else { return true };
        for (i, sp) in specs.iter().enumerate() {
            let (b, bi) = &after[i];
            let c = match &p[i] {
                Part::Pos(x) => match bi {
                    Some(q) => (*x as i128).cmp(q),
                    None => compare_sort_values(&serde_json::json!(*x as i64), b),
                },
                Part::Cell(x) => match (x.int, bi) {
                    (Some(p), Some(q)) => p.cmp(q),
                    _ => compare_sort_values(&x.cell.1, b),
                },
            };
            let c = if sp.desc { c.reverse() } else { c };
            match c {
                Ordering::Greater => return true,
                Ordering::Less => return false,
                Ordering::Equal => {}
            }
        }
        false
    };
    let mut kept: Vec<(usize, Vec<Part>)> = Vec::new();
    let mut count = 0usize;
    // the worst kept key once `kept` was cut to `want`: later candidates must beat it
    let mut bar: Option<(usize, Vec<Part>)> = None;
    for &(pos, doc) in cand {
        let p = parts(pos, doc)?;
        count += 1;
        if want == 0 || !after_ok(&p) {
            continue;
        }
        let item = (pos, p);
        if let Some(b) = &bar {
            if cmp(&item, b) != Ordering::Less {
                continue;
            }
        }
        kept.push(item);
        if kept.len() >= 2 * want.max(16) {
            kept.select_nth_unstable_by(want - 1, cmp);
            kept.truncate(want);
            let worst = kept.iter().max_by(|a, b| cmp(a, b))?;
            bar = Some((worst.0, worst.1.iter().map(|x| match x { Part::Cell(c) => Part::Cell(c), Part::Pos(q) => Part::Pos(*q) }).collect()));
        }
    }
    kept.sort_by(cmp);
    kept.truncate(want);
    Some(Selected { positions: kept.into_iter().map(|e| e.0).collect(), count })
}
