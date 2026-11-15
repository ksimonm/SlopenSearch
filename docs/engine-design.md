# Index-based engine for the Rust server

Status: phases 1 and 2 implemented in `servers/rust`: inverted index, numeric columns
and parallel searches (phase 1); indexing at write time, faster analysis, results
computed from the index for `match`/`range`, top-k, aggregation and sort preparation,
metric columns and a geo prefilter (phase 2, [below](#phase-2)). This note describes
the design, what is index-backed today, and the plan for the rest. Benchmark numbers
are at the end of each phase.

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
      ├─ fields["g|f"]    geo_point latitudes: [(lat, doc)] sorted (phase 2)
      ├─ fields["a|f"]    metric column: doc -> the single number a metric
      │                   aggregation reads from the _source (phase 2)
      ├─ order_map        doc number per position of Index::order (phase 2)
      └─ missing          ids that have a _source but no doc values
```

- `TrackedMap` wraps the two hash maps: reads through `Deref`, every mutation through
  `insert`/`remove`/`clear`, which note the changed id (a clone or `clear` marks all).
  No write path had to learn about the index: every path that changes documents (index,
  bulk, update, delete, by-query, reindex, resize, recovery, close/open, refresh
  visibility swaps) already goes through these maps.
- Before a search the index catches up (`search::index::ensure`): changed ids are
  re-indexed into every built field; a field is built on first use (phase 2: at write
  time); when the mapping or settings change (`Tracked` generation counters) the fields
  whose analysis signature changed are dropped and rebuilt, so analysis always matches
  the current analyzers. The numbering is compacted
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
  unchanged, so data written by `main` loads). Phase 1 built each field lazily on first
  use; phase 2 builds every field at write time and at startup (see below).

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
| `geo_distance` (geo_point, phase 2) | latitude band of the circle from the latitude column |

`match` scoring reads tf/dl/df from the postings (`index::match_stats`) for any document
the index holds (fuzziness still analyzes the document). Everything else scans:
fuzzy/wildcard/regexp, text `prefix`, `query_string`, geo other than `geo_distance`,
nested/join, scripts, span, intervals, k-NN, `match_phrase_prefix`, alias fields,
runtime/derived fields. Phase 2 answers a top-level `match` and `range` without the
evaluator (see [direct results](#candidates-and-results-from-the-index)).

## Concurrency

`Arc<Mutex<Store>>` became `Arc<RwLock<Store>>`. A `GET|POST [/<index>]/_search` runs
under the shared lock when it changes nothing (`search::read_only_search`): no scroll,
PIT or request-level derived fields, and no index with unrefreshed writes (a search would
swap in the refreshed view, a scheduled refresh could fire). The request cache is behind
its own mutex, the inverted index behind its own `RwLock`, so concurrent searches can
build or catch up the index. Everything else, all writes included, takes the lock
exclusively as before. Per-index locks are the next step (writes to one index would then
not block searches of another).

## Phase 2

### Indexing at write time

- Every request that took the store exclusively ends with `index::after_write`: each
  index whose documents changed catches up (`sync_locked`, incremental per changed id,
  as before) and builds every *eager* field that is missing: each plain (not aliased,
  not nested) `text`, `keyword`, numeric, `date` and `geo_point` field of the mapping,
  plus a metric column per numeric/date field. Other kinds are still built on first
  use. So a bulk pays for its own analysis, and the first query after a bulk load does
  not.
- Fields are no longer dropped when the mapping *generation* changes: the dynamic
  mapping code takes `&mut mappings` on every document write, so the generation changes
  with every bulk and eager indexing would rebuild everything each time. Each field
  keeps a *signature* of what its terms depend on (text: the settings generation, the
  alias resolution and the field definition; metric: the field definition and the
  `dynamic` setting; others: the alias resolution) and is rebuilt only when it changes.
- Startup: after recovery a background thread indexes every index under the shared store
  lock (rayon over indices, fields and document chunks; chunks merged pairwise in
  parallel). The server answers meanwhile; writes and `_count` (exclusive) wait for it,
  a search on an index still being built waits for that index.
- Documents whose `_source` changed are re-indexed too (the metric column reads the
  source).

### Faster analysis

- `QueryCtx::field_analyzer` caches resolved analyzers per thread, keyed by settings and
  mapping generation, field, side and query override: resolving parsed the analysis
  settings and built the components on every call, i.e. per document in searches.
- Standard tokenizer: the UAX#29 word-break class of ASCII characters comes from a table
  filled by the general classification; the plain lowercase filter lowercases ASCII
  terms in place; token types are `Cow<'static, str>` instead of a `String` per token.
  225 -> 105 ns per token on the bench corpus (single thread, standard analyzer).

### Candidates and results from the index

- Candidates are order positions, not ids: a cached doc number per position of
  `Index::order` (`order_map`, per index content version) and a bitset of candidate doc
  numbers; no id is hashed per document. The hit's index-order position needs no `seen`
  set when no id occurs twice in the order.
- *Direct results*: for a top-level `match` the planner proves the evaluator would score
  every indexed document from the index statistics, so the search loop takes
  `(matched, score)` per candidate from the postings, computed by the same
  `score_match` function the evaluator calls (same f64 arithmetic). For a `range` on a
  numeric/date field it calls the evaluator's own comparison
  (`QueryCtx::range_on_values`) on the candidate's doc values, with the bounds parsed
  once and numeric bounds pre-parsed into the parts `cmp_ord` compares (`NumParts`); a
  keyword/text `term` is posting membership with the constant score `boost`; a `bool`
  whose clauses all have direct forms replays `eval_bool`'s arithmetic (must+filter
  sum, minus the filter scores, should count and `minimum_should_match`, must_not,
  `adjust_pure_negative`, boost) in clause order. Large candidate sets are decided on
  several threads. With `SLOP_VERIFY_INDEX=true` the loop still evaluates every document and
  logs any difference between the direct and the evaluated result.
- *Top-k*: a top-level `match` whose response only reads the page (body keys `query`,
  `from`, `size`, `_source`, `track_total_hits`, `timeout`, no routing/preference/named
  queries) keeps the best `from + size` hits by (score desc, index order), which is the
  order the stable score sort produces, and counts the others for `hits.total`.
- `range` queries prepare their document-independent part (mapping lookup, bound
  parsing, date math) once per request (a per-thread memo keyed by a request epoch, so
  `now` is still evaluated per request).

### Aggregations and sorts

- `doc_field_values` (the value reader of every aggregation) looked up the alias, the
  field definition and the `dynamic` setting for every document; a per-thread cache of
  prepared field readers (per mapping generation, field and prefix) leaves only the
  per-document part.
- *Simple* aggregation trees (`terms`, `histogram`, `date_histogram`, `range` and the
  numeric metrics on mapped, non-nested fields, no scripts, one index) run on the stored
  sources directly instead of deep copies with an injected `_id`; the ids they may need
  come from a side table keyed by source address. Term and range buckets hold
  references.
- Float metrics (`avg/min/max/sum/stats` on float, half_float, scaled_float) take the
  doc-value double directly instead of formatting it as Java double text and parsing it
  back (the text is chosen to round-trip, so the value is the same).
- *Metric columns* (`fields["a|f"]`): per doc number, the single number the metric
  aggregation code itself reads from the document's `_source` (`aggs::metric_column_value`,
  None for no value or several); maintained at write time like every field. The metric
  aggregations over the root documents of a search read it (the search passes the doc
  numbers of the hits with the content version), every other document takes the general
  path. The column is a cache of the same function on the same source, so values and
  their order (Kahan sums) are unchanged.
- Sorts: the mapping definition, alias and nested parent of a sort field are prepared
  once per field instead of once per hit; integer sort cells are compared through
  pre-parsed `i128` keys (the order `compare_sort_values` gives u64/i64 numbers) instead
  of re-parsing the arbitrary-precision numbers at every comparison.

### Geo

`geo_point` fields keep their latitudes in a sorted column; `geo_distance` plans the
documents whose latitude is within the distance of the center's (a great-circle distance
is never shorter than the latitude difference), widened by 1 % + 0.001°. The evaluator
decides on the candidates as before. `geo_bounding_box` still scans.

### Verify mode

`SLOP_VERIFY_INDEX=true` recomputed the BM25 corpus statistics by a full scan for every
scored document (quadratic: test_61's parallel load test timed out on `main`); the scan
now runs once per index state and query.

### Not done: per-index locks

Searches of different indices still share the store lock with every write. `Store`
owns its indices by value and every handler takes `&mut Store`; per-index `RwLock`s need
`indices: HashMap<String, Arc<RwLock<Index>>>` and the handlers rewritten to lock the
indices they touch (in a fixed order for multi-index requests) while store-level state
(templates, pipelines, aliases, data streams, the id counter) stays under the store
lock. That touches most of `http/`; it is the first item below.

## Next steps

Phase 1's list (eager indexing, per-search fixed costs, aggregations from columns,
top-k, geo prefilter) is done as far as described in [Phase 2](#phase-2). What remains,
by expected impact:

1. Per-index `RwLock`s (see above), so writes to one index do not block searches of
   another.
2. Per-hit costs that remain: building a `MultiHit` (id and index name cloned) and the
   `agg_matched`/`collapse_pool` copies of the hit list; field sort cells
   (`compute_sort_cell` reads and converts the doc values of every hit, ~1 µs, now on
   several threads for large sets): a sort column per field (the cell of single-valued
   documents) and top-k for field sorts.
3. More direct results: `multi_match`, `terms`, `exists`, `match_phrase` (positions in
   the postings), `constant_score`; top-k also for `bool`.
4. Bucket aggregations from columns (`terms` from the keyword postings,
   `histogram`/`date_histogram`/`range` and sub-aggregations from the metric columns of
   the bucket's documents).
5. Restart: recovery parses the JSON snapshot, recomputes every document's doc values
   sequentially and rewrites the snapshot before the index build starts (~5 s for the
   100 k-doc corpus); parallel doc values and a persisted index would cut it.
6. Compact storage: `_source` as raw bytes, f32 vectors; k-NN on f32 arrays; geo
   bounding boxes from the latitude column.

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

## Bench note, phase 2 (2026-11-15)

Same corpus and query generators as above (100 k docs, seed 7; 10 variants per
workload: *first* = the first request of the workload, *median* = median of the other
nine), native release binaries, persistence on, one client, run back to back on the
same host (16 threads, load average 2-5 during these two runs; earlier runs at load
6-13 moved single numbers by ±30 % but gave the same picture). *main* = `main` at
`92ebed2` (phase 1); *phase 2* = this branch at `73e426a`. The 150 response bodies of
both runs (minus `took`) are byte-identical, and so are the answers of this branch
started on a data directory written by `main`. The whole suite passes, also with
`SLOP_VERIFY_INDEX=true` (no mismatch logged; a deliberately wrong direct score is
reported there), and test_95 passes with a KILL restart.

| workload | main first | main median | phase 2 first | phase 2 median |
|---|---:|---:|---:|---:|
| match (mid-frequency term) | 2,128 ms (builds `body`) | 19.7 ms | 16.1 ms | 1.8 ms |
| match_frequent | 403 ms | 414 ms | 22.4 ms | 20.9 ms |
| match_and | 12.2 ms | 16.4 ms | 1.5 ms | 1.8 ms |
| multi_match (title^2, body) | 218 ms (builds `title`) | 33.6 ms | 14.6 ms | 13.7 ms |
| bool (match + term + range + must_not) | 192 ms | 12.0 ms | 3.2 ms | 4.4 ms |
| filter_term_range | 206 ms | 9.3 ms | 1.8 ms | 1.8 ms |
| geo_distance | 573 ms | 572 ms | 30.7 ms | 26.9 ms |
| sort_price | 118 ms | 47.5 ms | 29.7 ms | 23.6 ms |
| sort_date | 35.7 ms | 29.1 ms | 10.4 ms | 7.0 ms |
| agg_terms | 59.8 ms | 57.3 ms | 5.9 ms | 5.3 ms |
| agg_date_histogram | 65.5 ms | 80.2 ms | 12.6 ms | 14.2 ms |
| agg_metrics (range over most docs) | 1,416 ms | 1,230 ms | 102 ms | 96.0 ms |
| highlight | 16.9 ms | 14.9 ms | 2.8 ms | 2.2 ms |
| deep_from (from 5000, frequent term, sort seq) | 488 ms | 705 ms | 38.7 ms | 52.6 ms |
| search_after page | 47.8 ms | 50.5 ms | 18.0 ms | 15.5 ms |

| | main | phase 2 |
|---|---:|---:|
| bulk indexing, 1 client × 1000 docs (docs/s) | 10,871 (8,400-11,300 over 6 runs) | 9,749 (7,100-9,700 over 6 runs) |
| RSS after indexing / after the queries (MiB) | 1,305 / 1,854 | 1,521 / 1,540 |
| restart (SIGTERM) until `_count` answers | 4.7 s | 5.5 s |
| first / second `match` after restart | 1,927 ms / 8.2 ms | 8.1 ms / 1.4 ms |
| RSS after restart (MiB) | 2,098 | 1,848 |

- Indexing is ~10-30 % slower: every bulk analyzes and indexes its documents (the
  `body` text dominates: ~20 ms of a ~75 ms bulk request at load) instead of leaving it
  to the first search after every restart. Moving the indexing to a background thread
  after the response was tried and not measurably better.
- Restart: the index is built in the background after recovery (~1-1.5 s for 16 fields
  over 100 k docs, rayon); `_count` waits for it because it takes the store
  exclusively. Recovery itself (snapshot parse, doc values, snapshot rewrite) is
  unchanged and dominates.
- RSS: the eager fields and columns cost ~200 MiB after indexing; after the queries
  phase 2 is lower (no per-query hit copies, no lazily built leftovers).
- What still walks every hit or scans: field sorts (`sort_price`, `deep_from`,
  `search_after`: a sort cell per matching hit, on several threads for large sets),
  `agg_metrics` (the range result and the metrics are column-backed, the hit list is
  still materialized), `agg_date_histogram` (bucketing reads each hit's source),
  `multi_match` (evaluator per candidate), and every query shape the planner does not
  know (see the coverage table). The bench was not run in Docker (`make bench`).
