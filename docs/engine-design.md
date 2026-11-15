# Index-based engine for the Rust server

Status: phase 1 implemented in `servers/rust` (inverted index, numeric columns, parallel
searches). This note describes the design, what is index-backed today, and the plan for
the rest. Benchmark numbers are at the end.

## Where the time went

Measured on the `small` bench corpus (100 k documents, 73 MB NDJSON, native release
build, one query at a time):

- **Analysis per document per query.** `match` re-tokenized the field of *every*
  document twice: once to compute the corpus statistics (docs with the field, total
  length, document frequency of each query token) and once to score it. Building the
  analyzer from the index settings was repeated per call. ~80-100 µs per document:
  8 s for a `match`, 29 s for a frequent term.
- **Quadratic hit bookkeeping.** Each matching document looked up its own position in
  the index order with a linear search (`order.iter().position`): with 60 k hits that is
  ~2·10⁹ string comparisons; and each hit deep-copied its `_source` (`doc.clone()`), the
  hit list was copied twice more (aggregations, collapse).
- **Per-search fixed costs.** Finding documents without doc values walked every id
  (17 ms at 100 k docs) before the first document was evaluated.
- **One global mutex.** Every request, searches included, ran under `Mutex<Store>`.

## Principle: candidates from the index, decisions by the evaluator

Responses must stay byte-identical, and the evaluator (`search/eval*.rs`) encodes
thousands of compatibility details. So the index does not replace it; it narrows its
input. For a query the index produces a **candidate set**, a superset of the documents
the query can match; the search loop runs the existing `QueryCtx::eval` on the
candidates only, in index order, exactly as before. A query shape the index does not
understand produces no plan and the loop scans every document, as before. Incorrect
candidate generation can therefore only lose hits, never change a score or an error,
and `SLOP_VERIFY_INDEX=true` checks exactly that: it also scans and logs any document
that matches without being a candidate, and recomputes the BM25 statistics by scan and
compares them (the whole suite runs clean in this mode). `SLOP_NO_INDEX=true` turns the
index off.

Errors: the evaluator raises some errors only when it meets a document with the field
(e.g. an unparsable value for a numeric field). The planner only accepts query shapes
whose doc-dependent errors it pre-checks (it runs the same validation and falls back on
any error), so the first error a scan would hit is still raised.

## Data model

```
Index
 ├─ docs:        TrackedMap<Value>                id -> _source (as before)
 ├─ doc_values:  TrackedMap<Map<field, values>>   id -> indexed field values (as before)
 └─ inverted:    RwLock<InvertedIndex>            derived, never persisted
      ├─ ids / by_id      doc number <-> id; a changed document gets a new number,
      │                   the old one is dead (append-only, Lucene-style)
      ├─ fields["t|f"]    text: term -> [(doc, tf)] sorted by doc; per-doc token count;
      │                   live doc count and total length for BM25
      ├─ fields["k|f"]    exact (keyword) values -> [(doc, 1)]
      ├─ fields["n|f"]    numeric/date column: [(value, doc)] sorted by value
      └─ missing          ids that have a _source but no doc values
```

- `TrackedMap` wraps the two hash maps: reads through `Deref`, every mutation through
  `insert`/`remove`/`clear`, which note the changed id (a clone or `clear` marks all).
  No write path had to learn about the index: every path that changes documents (index,
  bulk, update, delete, by-query, reindex, resize, recovery, close/open, refresh
  visibility swaps) already goes through these maps.
- Before a search the index catches up (`search::index::ensure`): changed ids are
  re-indexed into every built field; a field is built on first use; when the mapping or
  settings change (`Tracked` generation counters) the fields are dropped and rebuilt
  lazily, so analysis always matches the current analyzers. The numbering is compacted
  when dead numbers outnumber live ones.
- Text fields hold exactly the tokens `QueryCtx::doc_tokens` produces (same analyzer
  resolution, resolved once per field), so term frequencies, field lengths and document
  frequencies equal what the scan computed. Scoring keeps the current formula
  (`idf = ln(1 + (N - df + 0.5) / (df + 0.5))`, `tf / (tf + 1.2 (0.25 + 0.75 dl/avgdl))`,
  exact token counts, not Lucene's one-byte norms) and the same f64 arithmetic in the
  same order; only the source of `tf`, `dl`, `N`, `Σdl` and `df` changes.
- Refreshed-view visibility (`model/visibility.rs`) keeps working unchanged: a search of
  an index with unrefreshed overwrites swaps the refreshed versions into the maps, the
  swap marks those ids dirty, the index re-indexes them (proportional to the number of
  unrefreshed documents), and the swap back marks them again.
- Persistence: the index is rebuilt from the documents (the journal/snapshot format is
  unchanged, so data written by `main` loads). Building is lazy per field; a 100 k-doc
  text field takes a few seconds on first use.

## Query coverage (phase 1)

| query | candidates |
|---|---|
| `term` | keyword: exact value; text: one token |
| `terms` | keyword values |
| `match` (text) | OR / `minimum_should_match` ≥ 1: union of the token postings; AND or msm = all: intersection |
| `match_phrase` | intersection of the token postings (positions checked by the evaluator) |
| `multi_match` best/most_fields | union over the fields of the `match` plans |
| `prefix` (keyword) | terms of the dictionary with the prefix |
| `exists` (plain keyword) | docs with a value |
| `ids` | the ids |
| `range` numeric/date | sorted column, bounds parsed like the evaluator and widened (1 %, 2 days) |
| `bool` | ∩ of must/filter plans, ∩ with ∪ of should when should is required; must_not ignored |
| `constant_score`, `boosting`, `dis_max` | their inner plans |

`match` scoring reads tf/dl/df from the postings (`index::match_stats`) for any document
the index holds (fuzziness still analyzes the document). Everything else scans:
fuzzy/wildcard/regexp, text `prefix`, `query_string`, geo, nested/join, scripts, span,
intervals, k-NN, `match_phrase_prefix`, alias fields, runtime/derived fields.

## Concurrency

`Arc<Mutex<Store>>` became `Arc<RwLock<Store>>`. A `GET|POST [/<index>]/_search` runs
under the shared lock when it changes nothing (`search::read_only_search`): no scroll,
PIT or request-level derived fields, and no index with unrefreshed writes (a search would
swap in the refreshed view, a scheduled refresh could fire). The request cache is behind
its own mutex, the inverted index behind its own `RwLock`, so concurrent searches can
build or catch up the index. Everything else, all writes included, takes the lock
exclusively as before. Per-index locks are the next step (writes to one index would then
not block searches of another).

## Next steps

1. Eager indexing: index text/keyword fields at write time (after each bulk) instead of
   on first query, so the first query after a restart or bulk load is fast; and make
   analysis itself faster (the standard analyzer costs ~30-60 µs per 700-byte document).
2. Per-search fixed costs: iterate candidates in index order without hashing every id
   (keep the index-order position per doc number); memoize query analysis and date-bound
   parsing per search instead of per document.
3. Aggregations and sorts from the columns (terms on keyword postings, metrics and
   histograms on the numeric columns) instead of per-hit `_source`/doc-values walks.
4. Top-k scoring: evaluate `match` entirely from postings (WAND-style) and only fetch the
   page, instead of evaluating every candidate.
5. Compact storage: `_source` as raw bytes, doc values per field as columns (the
   inverted index already has them), f32 vectors; then persist the index segments.
6. Per-index `RwLock`s; geo (bounding-box prefilter on the lat/lon columns), k-NN on
   f32 arrays.

## Bench note (2026-11-15)

`small` corpus of `bench/datagen.py` (100 k docs, 73 MB), the query generators of
`bench/workloads.py` (seed 7), native release binaries on the same host (16 threads,
load average 1-4 from other agents), persistence on, one client unless noted. *before* =
`main` at `3f75a11`; *after* = this branch. Latency is the median wall time of one
request; *warm* = the fields the query uses were already built (each field's postings
are built once, on first use: a few seconds for the 100 k-doc `body` field, ~0.1-0.3 s
for keyword/numeric fields). The first-query responses of all 15 workloads are
byte-identical between before and after.

| workload | before | after, first use | after, warm |
|---|---:|---:|---:|
| match (mid-frequency term) | 8,015 ms | 3,557 ms (builds `body`) | 56 ms |
| match_frequent (stop-word-like) | 28,637 ms | 1,323 ms | 1,161 ms |
| match_and | 7,558 ms | 19 ms | 22 ms |
| multi_match (title^2, body) | 9,629 ms | 354 ms (builds `title`) | 51 ms |
| bool (match + term + range + must_not) | 8,707 ms | 327 ms | 21 ms |
| filter_term_range (term + date range) | 484 ms | 428 ms | 21 ms |
| geo_distance (scan) | 561 ms | 951 ms | 811 ms |
| sort_price | 1,060 ms | 53 ms | 53 ms |
| sort_date | 9,682 ms | 27 ms | 78 ms |
| agg_terms | 8,245 ms | 90 ms | 67 ms |
| agg_date_histogram | 1,489 ms | 161 ms | 119 ms |
| agg_metrics (range over most docs) | 20,342 ms | 1,755 ms | 1,348 ms |
| highlight | 8,262 ms | 25 ms | 21 ms |
| deep_from (from 5000, frequent term) | 18,594 ms | 415 ms | 377 ms |
| search_after page | 1,988 ms | 60 ms | 45 ms |

| | before | after |
|---|---:|---:|
| bulk indexing, 1 client × 1000 docs (docs/s) | 9,867 | 8,848 (noise; indexing path unchanged) |
| RSS after indexing / after the queries (MiB) | 1,303 / 1,980 | 1,323 / 1,864 |
| warm searches, 1 vs 4 clients (req/s) | (one core) | 32 vs 148 |
| restart until `_count` answers | — | 6.4 s |
| first `match` after restart | ~8 s | 3.2 s (builds `body`), then 15 ms |

What is still slow and why: `match_frequent`, `deep_from` and `agg_metrics` match
most of the corpus, and every candidate still goes through the per-document evaluator
(~15-20 µs: query re-analysis, value clones) and the hit list/aggregation code;
`geo_distance` scans. These are next steps 2-4 above. The bench was not run in Docker
(`make bench`); the absolute numbers are from one busy machine.
