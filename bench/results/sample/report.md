# SlopenSearch benchmark — 2026-11-15

- size **tiny**: 10,000 text documents (0.01 GB NDJSON), 5,000 vectors × 128 dims
- every target alone in Docker with `--cpus 4 --memory 8g` (no swap); reference OpenSearch heap 4g
- clients: bulk/update 8 (batch 1000), search 4, mixed 8; 3.0 s per search workload
- host `p7760`: 11th Gen Intel(R) Core(TM) i7-11850H @ 2.50GHz, 16 CPUs, 91 GB RAM, kernel 7.0.0-43-generic; load average at start 4.2, at end 20.9
- git `aa824cc`; targets: ref (3.9.0), go (3.9.0), rust (3.9.0), rust-mem (3.9.0)

## Indexing

| | ref | go | rust | rust-mem |
|---|---:|---:|---:|---:|
| startup, empty (s) | 23.14 | 0.38 | 0.34 | 0.21 |
| text bulk (docs/s) | 4,181 | 12,554 | 10,405 | 14,830 |
| text bulk p50 / p99 per batch (ms) | 1286.53 / 2153.06 | 659.26 / 662.05 | 570.72 / 764.87 | 400.13 / 544.0 |
| text docs indexed | 10,000 | 10,000 | 10,000 | 10,000 |
| refresh after bulk (s) | 0.72 | 0.00 | 0.04 | 0.03 |
| vector bulk (docs/s) | 4,348 | 3,248 | 3,197 | 5,096 |
| vector refresh (s) | 0.33 | 0.00 | 0.04 | 0.03 |
| CPU during text bulk (avg / peak cores) | 3.99 / 4.57 | 1.68 / 2.00 | 0.96 / 0.97 | 1.02 / 1.02 |
| memory peak during text bulk (MiB) | 4,595 | 112 | 205 | 149 |
| memory idle after indexing (MiB) | 4,606 | 174 | 453 | 336 |
| memory peak, whole run (MiB) | 4,608 | 368 | 519 | 934 |
| on-disk size after indexing (MiB) | 15 | 16 | 25 | - |
| OOM kills | 0 | 0 | 0 | 0 |

### Bulk batch size / client sweep (docs/s, separate index)

| batch × clients | ref | go | rust | rust-mem |
|---|---:|---:|---:|---:|
| 100 × 4 | 9,064 | 5,054 | 7,997 | 14,102 |
| 1000 × 1 | 9,588 | 5,101 | 11,126 | 15,799 |
| 1000 × 4 | 17,201 | 5,672 | 5,745 | 16,321 |
| 1000 × 8 | 15,365 | 5,683 | 11,532 | 16,430 |
| 5000 × 4 | 30,108 | 5,655 | 10,525 | 14,982 |

## Search throughput (ops/s) and latency p50 / p95 / p99 (ms)

4 concurrent clients, closed loop. `!n` = n failed requests; *too slow* = a single probe request took longer than 5.0 s, so the timed run was skipped (the probe time is shown). Correctness is in its own section.

| workload | ref ops/s | ref p50/p95/p99 | go ops/s | go p50/p95/p99 | rust ops/s | rust p50/p95/p99 | rust-mem ops/s | rust-mem p50/p95/p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| match | 1,132.1 | 2.21 / 4.69 / 37.69 | 1.4 | 2769.28 / 3199.94 / 3199.94 | 1.4 | 2325.02 / 3272.45 / 3272.45 | 1.0 | 3708.42 / 4243.7 / 4243.7 |
| match_frequent | 1,523.5 | 1.77 / 3.19 / 29.36 | 1.1 | 3018.15 / 3090.43 / 3090.43 | 1.4 | 2743.93 / 3110.08 / 3110.08 | 0.7 | 5792.6 / 5832.04 / 5832.04 |
| match_and | 1,723.2 | 1.63 / 2.89 / 27.51 | 1.0 | 3957.78 / 4088.45 / 4088.45 | 1.6 | 2370.4 / 2717.59 / 2717.59 | 0.6 | 4555.82 / 6431.31 / 6431.31 |
| multi_match | 581.0 | 6.16 / 12.6 / 16.29 | 6.5 | 549.86 / 951.58 / 1009.77 | 1.3 | 2890.91 / 3088.75 / 3088.75 | 0.3 | 6268.55 / 12009.86 / 12009.86 |
| bool | 686.5 | 5.28 / 11.43 / 15.77 | 1.5 | 2491.28 / 2836.43 / 2836.43 | 1.5 | 2346.35 / 3025.44 / 3025.44 | 0.5 | 6229.39 / 8608.38 / 8608.38 |
| filter_term_range | 1,244.6 | 1.82 / 8.31 / 26.64 | 104.4 | 35.83 / 66.23 / 86.34 | 27.5 | 141.75 / 171.34 / 200.89 | 13.1 | 290.51 / 361.42 / 365.35 |
| geo_distance | 843.7 | 3.28 / 11.24 / 21.83 | 72.6 | 51.06 / 93.56 / 128.82 | 23.2 | 171.91 / 182.54 / 184.38 | 8.4 | 469.26 / 525.21 / 526.83 |
| sort_price | 1,225.0 | 2.19 / 7.8 / 30.34 | 51.6 | 66.58 / 168.81 / 202.5 | 15.0 | 249.55 / 357.22 / 385.95 | 7.7 | 514.13 / 598.86 / 612.15 |
| sort_date | 2,295.8 | 1.57 / 2.54 / 3.24 | 1.1 | 3771.9 / 3815.36 / 3815.36 | 1.3 | 2986.4 / 3306.51 / 3306.51 | 0.4 | 7770.36 / 10922.43 / 10922.43 |
| agg_terms | 4,299.2 | 0.68 / 1.1 / 3.26 | 1.1 | 4643.75 / 4645.64 / 4645.64 | 1.6 | 1749.68 / 2584.47 / 2584.47 | 0.7 | 5316.44 / 7566.91 / 7566.91 |
| agg_date_histogram | 2,344.9 | 1.25 / 2.47 / 24.06 | 1,119.0 | 2.62 / 6.96 / 32.08 | 1,721.9 | 1.56 / 2.33 / 3.65 | 79.0 | 8.61 / 275.98 / 509.19 |
| agg_metrics | 6,234.4 | 0.56 / 0.96 / 1.23 | 16.3 | 216.34 / 613.58 / 853.63 | 3.5 | 1052.75 / 1366.48 / 1366.48 | 1.3 | 2677.47 / 3613.73 / 3613.73 |
| highlight | 422.5 | 8.09 / 15.73 / 24.09 | 1.0 | 4059.28 / 4104.51 / 4104.51 | 1.0 | 4142.17 / 4198.34 / 4198.34 | 0.4 | 7701.3 / 10599.24 / 10599.24 |
| deep_from | 543.8 | 6.38 / 16.56 / 25.11 | 1.0 | 2975.56 / 3024.96 / 3024.96 | 1.0 | 3971.89 / 4461.84 / 4461.84 | 0.5 | 5405.06 / 7318.1 / 7318.1 |
| search_after | 210.8 | 16.12 / 30.74 / 38.25 | 79.0 | 42.4 / 82.39 / 97.19 | 17.4 | 235.67 / 268.92 / 279.29 | 8.9 | 387.38 / 559.39 / 566.86 |
| knn | 501.3 | 7.24 / 13.77 / 19.09 | 21.9 | 149.73 / 303.54 / 402.79 | 0.4 | 7312.61 / 9567.0 / 9567.0 | 0.3 | 8619.13 / 14817.58 / 14817.58 |
| knn_filter | 614.2 | 6.03 / 11.14 / 14.35 | 25.7 | 140.62 / 216.88 / 261.01 | 0.3 | 5433.83 / 15157.58 / 15157.58 | 0.3 | 6195.36 / 13644.07 / 13644.07 |

### CPU (avg cores) and memory peak (MiB) during search

| workload | ref | go | rust | rust-mem |
|---|---:|---:|---:|---:|
| match | 3.92 / 4,667 | 3.83 / 396 | 0.96 / 1,307 | 0.97 / 837 |
| match_frequent | 3.85 / 4,674 | 3.27 / 397 | 0.97 / 1,307 | 1.00 / 836 |
| match_and | 3.99 / 4,684 | 3.78 / 396 | 0.97 / 1,307 | 0.97 / 836 |
| multi_match | 3.25 / 4,686 | 3.65 / 397 | 0.97 / 1,306 | 0.97 / 835 |
| bool | 3.12 / 4,689 | 3.82 / 395 | 0.96 / 1,307 | 0.97 / 835 |
| filter_term_range | 3.28 / 4,693 | 3.70 / 399 | 1.01 / 1,307 | 0.95 / 836 |
| geo_distance | 2.88 / 4,693 | 3.51 / 400 | 0.94 / 1,308 | 0.89 / 837 |
| sort_price | 3.86 / 4,696 | 2.98 / 357 | 0.95 / 1,307 | 0.94 / 835 |
| sort_date | 3.47 / 4,697 | 3.51 / 401 | 0.97 / 1,306 | 0.97 / 835 |
| agg_terms | 4.00 / 4,701 | 2.95 / 400 | 0.95 / 1,307 | 0.97 / 835 |
| agg_date_histogram | 3.74 / 4,702 | 0.85 / 283 | 1.10 / 1,308 | 0.84 / 836 |
| agg_metrics | 2.85 / 4,702 | 2.00 / 349 | 0.95 / 1,403 | 0.98 / 934 |
| highlight | 2.98 / 4,704 | 3.20 / 402 | 1.00 / 1,407 | 0.97 / 934 |
| deep_from | 3.18 / 4,705 | 3.04 / 405 | 0.97 / 1,407 | 0.98 / 907 |
| search_after | 2.35 / 4,705 | 3.55 / 382 | 1.00 / 1,407 | 0.98 / 907 |
| knn | 2.49 / 4,707 | 3.59 / 420 | 0.98 / 1,407 | 0.98 / 906 |
| knn_filter | 2.40 / 4,708 | 3.38 / 421 | 0.98 / 1,407 | 0.99 / 906 |

## Mixed load, updates and deletes

mixed: 8 clients, 80 % searches / 20 % bulk writes of 10 docs. Updates/deletes: 8 clients, single-document requests; bulk_update in batches of 1000 (docs/s); delete_by_query sequential, one user each.

| workload | ref ops/s | ref p50/p99 | ref CPU/MiB | go ops/s | go p50/p99 | go CPU/MiB | rust ops/s | rust p50/p99 | rust CPU/MiB | rust-mem ops/s | rust-mem p50/p99 | rust-mem CPU/MiB |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| mixed_rw (read) | 439.8 | 9.07 / 35.44 | 3.60 / 4,712 | 1.2 | 3701.23 / 10781.72 | 2.87 / 428 | 1.6 | 4353.2 / 6305.05 | 1.00 / 1,408 | 0.7 | 8576.78 / 13598.48 | 0.98 / 907 |
| mixed_rw (write) | 112.6 | 24.59 / 146.91 | 3.60 / 4,712 | 0.1 | 10207.46 / 10207.46 | 2.87 / 428 | 0.2 | 4735.59 / 4735.59 | 1.00 / 1,408 | - | - | - |
| update_single | 828.5 | 8.78 / 30.88 | 2.68 / 4,713 | 320.4 | 21.58 / 61.62 | 0.91 / 386 | 324.6 | 24.0 / 35.7 | 0.23 / 1,409 | 1,651.2 | 3.74 / 11.23 | 0.50 / 908 |
| delete_single | 809.8 | 8.88 / 43.03 | 2.52 / 4,714 | 376.8 | 16.23 / 66.8 | 0.97 / 386 | 335.9 | 23.86 / 28.34 | 0.16 / 1,409 | 2,857.9 | 2.06 / 7.67 | 0.16 / 908 |
| bulk_update | 6,942 docs/s | 557.54 / 715.3 | 4.02 / 4,716 | 7,462 docs/s | 655.51 / 658.24 | 1.30 / 385 | 8,834 docs/s | 348.96 / 559.52 | 0.69 / 1,408 | 5,469 docs/s | 548.31 / 896.55 | 0.80 / 929 |
| delete_by_query | 74.2 | 3.39 / 54.86 | - / - | 73.2 | 12.44 / 16.29 | - / - | 33.4 | 28.91 / 36.72 | 0.09 / 1,407 | 26.7 | 31.82 / 56.27 | 0.26 / 927 |

## Restart (persistent targets)

Graceful `docker stop`, then `docker start`; *data available* = until `_count` on the text index answers.

| | ref | go | rust | rust-mem |
|---|---:|---:|---:|---:|
| on-disk size (MiB) | 22 | 64 | 77 | - |
| stop (s) | 0.76 | 0.26 | 60.46 | - |
| start until HTTP ready (s) | 28.10 | 1.02 | 2.61 | - |
| start until data available (s) | 28.17 | 1.04 | 2.63 | - |
| first query after restart (ms) | 71.9 | 1157.7 | 642.8 | - |
| docs after restart / before | 18530 / 18530 | 9510 / 9510 | 9520 / 9520 | - |
| memory after restart (MiB) | 4,598 | 354 | 474 | - |

## Correctness (compared with the first target)

Sequential pass over 10 queries per workload with `track_total_hits: true`. *totals* = queries whose hit count equals ref's; *top ids* = mean overlap of the returned ids with ref's (score ties may legitimately reorder); *aggs* = identical aggregation results (floats to 4 digits). k-NN: recall@10 against exact brute-force neighbours.

| workload | go | rust | rust-mem |
|---|---|---|---|
| match | totals 10/10, top ids 98% | totals 10/10, top ids 98% | totals 10/10, top ids 98% |
| match_frequent | totals 10/10, top ids 92% | totals 10/10, top ids 92% | totals 10/10, top ids 92% |
| match_and | totals 10/10, top ids 98% | totals 10/10, top ids 98% | totals 10/10, top ids 98% |
| multi_match | totals 10/10, top ids 93% | totals 10/10, top ids 94% | totals 10/10, top ids 93% |
| bool | totals 10/10, top ids 100% | totals 10/10, top ids 99% | totals 10/10, top ids 100% |
| filter_term_range | totals 10/10, top ids 75% | totals 10/10, top ids 66% | totals 10/10, top ids 72% |
| geo_distance | totals 10/10, top ids 100% | totals 10/10, top ids 100% | totals 10/10, top ids 100% |
| sort_price | totals 10/10, top ids 100% | totals 10/10, top ids 100% | totals 10/10, top ids 100% |
| sort_date | totals 10/10, top ids 100% | totals 10/10, top ids 100% | totals 10/10, top ids 100% |
| agg_terms | totals 10/10, aggs 10/10 | totals 10/10, aggs 10/10 | totals 10/10, aggs 10/10 |
| agg_date_histogram | totals 10/10, aggs 10/10 | totals 10/10, aggs 0/10, 10/10 within 0.1 % | totals 10/10, aggs 0/10, 10/10 within 0.1 % |
| agg_metrics | totals 10/10, aggs 10/10 | totals 10/10, aggs 10/10 | totals 10/10, aggs 10/10 |
| highlight | totals 10/10, top ids 93% | totals 10/10, top ids 94% | totals 10/10, top ids 93% |
| deep_from | totals 10/10, top ids 100% | totals 10/10, top ids 100% | totals 10/10, top ids 100% |
| search_after | top ids 100% | top ids 100% | top ids 100% |
| knn | top ids 100% | top ids 100% | top ids 100% |
| knn_filter | top ids 100% | top ids 100% | top ids 100% |

| | ref | go | rust | rust-mem |
|---|---:|---:|---:|---:|
| recall@10 knn | 1.000 | 1.000 | 1.000 | 1.000 |
| recall@10 knn_filter | 1.000 | 1.000 | 1.000 | 1.000 |
| count text_after_index | 10000 | 10000 | 10000 | 10000 |
| count vec_after_index | 5000 | 5000 | 5000 | 5000 |
| count removed_by_update_workload | - | - | - | - |
| count rating_4_5_after_updates | 347 | 101 | 101 | 101 |

