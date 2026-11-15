package httpapi

// The sections of GET _nodes/stats, in the order and shape OpenSearch 3.9
// writes them. Process, OS and memory figures are this process's own (from
// the Go runtime and /proc); the JVM-only parts (memory pools, collectors,
// buffer pools, classes) are mapped onto the Go runtime or reported empty, so
// that monitoring tools that read them (opensearch-benchmark's telemetry,
// ...) find every key they expect.

import (
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
)

// nodeStatsSections are the metrics of _nodes/stats in response order; the
// section of "breaker" is "breakers" and of "concurrency_limiter"
// "concurrency_limiters". native_memory is always written.
var nodeStatsSections = []string{
	"indices", "os", "process", "jvm", "thread_pool", "fs", "transport", "http", "breaker",
	"script", "discovery", "ingest", "adaptive_selection", "script_cache", "indexing_pressure",
	"shard_indexing_pressure", "search_backpressure", "cluster_manager_throttling",
	"weighted_routing", "file_cache", "task_cancellation", "search_pipeline",
	"resource_usage_stats", "segment_replication_backpressure", "repositories",
	"admission_control", "caches", "remote_store", "concurrency_limiter",
}

func init() {
	for _, m := range nodeStatsSections {
		nodeStatsMetrics[m] = true
	}
}

var processStart = time.Now()

// peaks remembers the high-water marks reported as peak_* values.
var peaks struct {
	sync.Mutex
	threads                  int
	young, survivor, old, nh uint64
}

func maxU64(p *uint64, v uint64) uint64 {
	if v > *p {
		*p = v
	}
	return *p
}

// procFields reads "Key: value" lines of a /proc file (kB values in bytes).
func procFields(path string) map[string]uint64 {
	out := map[string]uint64{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			n *= 1024
		}
		out[strings.TrimSpace(k)] = n
	}
	return out
}

func osMemory() (total, free, swapTotal, swapFree uint64) {
	m := procFields("/proc/meminfo")
	total, free = m["MemTotal"], m["MemAvailable"]
	if free == 0 {
		free = m["MemFree"]
	}
	if total == 0 {
		total, free = 1<<34, 1<<33
	}
	return total, free, m["SwapTotal"], m["SwapFree"]
}

func pct(part, whole uint64) int {
	if whole == 0 {
		return 0
	}
	return int(math.Round(float64(part) * 100 / float64(whole)))
}

func nodeOSStats() ojson.Object {
	total, free, swapTotal, swapFree := osMemory()
	load := []float64{0, 0, 0}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		for i, f := range strings.Fields(string(b)) {
			if i < 3 {
				load[i], _ = strconv.ParseFloat(f, 64)
			}
		}
	}
	return ojson.Object{
		{"timestamp", nowMillis()},
		{"cpu", ojson.Object{{"percent", 0},
			{"load_average", ojson.Object{{"1m", jsonv.Float(load[0])}, {"5m", jsonv.Float(load[1])},
				{"15m", jsonv.Float(load[2])}}}}},
		{"mem", ojson.Object{{"total_in_bytes", total}, {"free_in_bytes", free}, {"used_in_bytes", total - free},
			{"free_percent", pct(free, total)}, {"used_percent", pct(total-free, total)}}},
		{"swap", ojson.Object{{"total_in_bytes", swapTotal}, {"free_in_bytes", swapFree},
			{"used_in_bytes", swapTotal - swapFree}}},
	}
}

// processCPUMillis is the user + system CPU time of this process.
func processCPUMillis() int64 {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	if i := strings.LastIndexByte(s, ')'); i >= 0 {
		s = s[i+1:]
	}
	f := strings.Fields(s)
	if len(f) < 13 {
		return 0
	}
	ut, _ := strconv.ParseInt(f[11], 10, 64)
	st, _ := strconv.ParseInt(f[12], 10, 64)
	return (ut + st) * 10 // clock ticks of 10ms
}

func maxOpenFiles() int64 {
	b, err := os.ReadFile("/proc/self/limits")
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "Max open files") {
				if f := strings.Fields(line[len("Max open files"):]); len(f) > 0 {
					if n, err := strconv.ParseInt(f[0], 10, 64); err == nil {
						return n
					}
				}
			}
		}
	}
	return 65535
}

func nodeProcessStats() ojson.Object {
	fds := 0
	if ents, err := os.ReadDir("/proc/self/fd"); err == nil {
		fds = len(ents)
	}
	return ojson.Object{
		{"timestamp", nowMillis()},
		{"open_file_descriptors", fds},
		{"max_file_descriptors", maxOpenFiles()},
		{"cpu", ojson.Object{{"percent", 0}, {"total_in_millis", processCPUMillis()}}},
		{"mem", ojson.Object{{"total_virtual_in_bytes", procFields("/proc/self/status")["VmSize"]}}},
	}
}

// heapMax is the memory limit of the Go runtime, or the machine's memory.
func heapMax() uint64 {
	if lim := debug.SetMemoryLimit(-1); lim > 0 && lim < math.MaxInt64 {
		return uint64(lim)
	}
	total, _, _, _ := osMemory()
	return total
}

func memPool(used, max, peak, peakMax uint64, gcUsed, gcMax uint64) ojson.Object {
	usage := -1
	if gcMax > 0 {
		usage = pct(gcUsed, gcMax)
	}
	return ojson.Object{
		{"used_in_bytes", used}, {"max_in_bytes", max},
		{"peak_used_in_bytes", peak}, {"peak_max_in_bytes", peakMax},
		{"last_gc_stats", ojson.Object{{"used_in_bytes", gcUsed}, {"max_in_bytes", gcMax}, {"usage_percent", usage}}},
	}
}

// nodeJVMStats maps the Go runtime onto the JVM section: the heap that
// survived the last collection is the "old" pool, what was allocated since
// the "young" one; collections are young collections.
func nodeJVMStats() ojson.Object {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	max := heapMax()
	live := ms.NextGC / 2 // the heap goal is twice the live heap (GOGC=100)
	if live > ms.HeapAlloc {
		live = ms.HeapAlloc
	}
	young := ms.HeapAlloc - live
	nonHeap := ms.Sys - ms.HeapSys
	threads := int(procFields("/proc/self/status")["Threads"])
	if threads == 0 {
		threads = runtime.NumGoroutine()
	}
	peaks.Lock()
	if threads > peaks.threads {
		peaks.threads = threads
	}
	peakThreads := peaks.threads
	peakYoung := maxU64(&peaks.young, young)
	peakOld := maxU64(&peaks.old, live)
	peaks.Unlock()
	return ojson.Object{
		{"timestamp", nowMillis()},
		{"uptime_in_millis", time.Since(processStart).Milliseconds()},
		{"mem", ojson.Object{
			{"heap_used_in_bytes", ms.HeapAlloc},
			{"heap_used_percent", pct(ms.HeapAlloc, max)},
			{"heap_committed_in_bytes", ms.HeapSys},
			{"heap_max_in_bytes", max},
			{"non_heap_used_in_bytes", nonHeap},
			{"non_heap_committed_in_bytes", nonHeap},
			{"pools", ojson.Object{
				{"young", memPool(young, 0, peakYoung, 0, 0, 0)},
				{"old", memPool(live, max, peakOld, max, live, max)},
				{"survivor", memPool(0, 0, 0, 0, 0, 0)},
			}},
		}},
		{"threads", ojson.Object{{"count", threads}, {"peak_count", peakThreads}}},
		{"gc", ojson.Object{{"collectors", ojson.Object{
			{"young", ojson.Object{{"collection_count", int64(ms.NumGC)},
				{"collection_time_in_millis", int64(ms.PauseTotalNs / 1e6)}}},
			{"old", ojson.Object{{"collection_count", 0}, {"collection_time_in_millis", 0}}},
		}}}},
		{"buffer_pools", ojson.Object{
			{"mapped", bufferPool()},
			{"direct", bufferPool()},
			{"mapped - 'non-volatile memory'", bufferPool()},
		}},
		{"classes", ojson.Object{{"current_loaded_count", 0}, {"total_loaded_count", 0}, {"total_unloaded_count", 0}}},
	}
}

func bufferPool() ojson.Object {
	return ojson.Object{{"count", 0}, {"used_in_bytes", 0}, {"total_capacity_in_bytes", 0}}
}

// nodeThreadPoolStats lists OpenSearch's core thread pools (name order).
func nodeThreadPoolStats() ojson.Object {
	procs := runtime.GOMAXPROCS(0)
	sizes := map[string]int{"generic": 4, "management": 1, "refresh": 1, "search": procs*3/2 + 1,
		"index_searcher": procs, "write": procs, "system_read": 1, "system_write": 1}
	waits := map[string]bool{"index_searcher": true, "search": true, "search_throttled": true, "stream_search": true}
	names := []string{
		"analyze", "fetch_shard_started", "fetch_shard_store", "flush", "force_merge", "generic", "get",
		"index_searcher", "listener", "management", "merge", "refresh", "remote_download", "remote_purge",
		"remote_recovery", "remote_refresh_retry", "remote_state_checksum", "remote_state_read", "search",
		"search_throttled", "snapshot", "snapshot_deletion", "stream_search", "system_read", "system_write",
		"translog_recovery", "translog_sync", "translog_transfer", "warmer", "write",
	}
	out := make(ojson.Object, 0, len(names))
	for _, n := range names {
		t := sizes[n]
		p := ojson.Object{{"threads", t}, {"queue", 0}, {"active", 0}, {"rejected", 0}, {"largest", t}, {"completed", 0}}
		if waits[n] {
			p = append(p, ojson.KV{"total_wait_time_in_nanos", 0})
		}
		out = append(out, ojson.KV{n, p})
	}
	return out
}

func nodeFSStats() ojson.Object {
	dir := os.Getenv("SLOP_DATA_DIR")
	if dir == "" {
		dir = "data"
	}
	total, free, avail := diskUsage(dir)
	return ojson.Object{
		{"timestamp", nowMillis()},
		{"total", ojson.Object{{"total_in_bytes", total}, {"free_in_bytes", free}, {"available_in_bytes", avail},
			{"cache_reserved_in_bytes", 0}}},
		{"data", []any{ojson.Object{{"path", dir}, {"mount", "/"}, {"type", "ext4"},
			{"total_in_bytes", total}, {"free_in_bytes", free}, {"available_in_bytes", avail},
			{"cache_reserved_in_bytes", 0}}}},
		{"io_stats", ojson.Object{}},
	}
}

func byteSize(n int64) string { return formatCatBytes(n, "") }

func nodeBreakerStats() ojson.Object {
	max := int64(heapMax())
	b := func(limit int64, overhead float64, used int64) ojson.Object {
		return ojson.Object{{"limit_size_in_bytes", limit}, {"limit_size", byteSize(limit)},
			{"estimated_size_in_bytes", used}, {"estimated_size", byteSize(used)},
			{"overhead", jsonv.Float(overhead)}, {"tripped", 0}}
	}
	return ojson.Object{
		{"request", b(max*6/10, 1.0, 0)},
		{"fielddata", b(max*4/10, 1.03, 0)},
		{"in_flight_requests", b(max, 2.0, 0)},
		{"parent", b(max*95/100, 1.0, 0)},
	}
}

func scriptStats() ojson.Object {
	return ojson.Object{{"compilations", 0}, {"cache_evictions", 0}, {"compilation_limit_triggered", 0}}
}

var scriptContexts = []string{
	"aggregation_selector", "aggs", "aggs_combine", "aggs_init", "aggs_map", "aggs_reduce", "analysis",
	"bucket_aggregation", "field", "filter", "ingest", "interval", "moving-function", "number_sort",
	"painless_test", "processor_conditional", "score", "script_heuristic", "similarity",
	"similarity_weight", "string_sort", "template", "terms_set", "trigger", "update",
}

func scriptCacheStats() ojson.Object {
	ctxs := make([]any, 0, len(scriptContexts))
	for _, c := range scriptContexts {
		ctxs = append(ctxs, append(ojson.Object{{"context", c}}, scriptStats()...))
	}
	return ojson.Object{{"sum", scriptStats()}, {"contexts", ctxs}}
}

func pipelineTotals() ojson.Object {
	return ojson.Object{{"count", 0}, {"time_in_millis", 0}, {"current", 0}, {"failed", 0}}
}

func backpressureTask() ojson.Object {
	tracker := func(unit string) ojson.Object {
		return ojson.Object{{"cancellation_count", 0}, {"current_max_" + unit, 0}, {"current_avg_" + unit, 0}}
	}
	heap := append(tracker("bytes"), ojson.KV{"rolling_avg_bytes", 0})
	return ojson.Object{
		{"resource_tracker_stats", ojson.Object{{"heap_usage_tracker", heap},
			{"elapsed_time_tracker", tracker("millis")}, {"cpu_usage_tracker", tracker("millis")}}},
		{"completion_count", 0},
		{"cancellation_stats", ojson.Object{{"cancellation_count", 0}, {"cancellation_limit_reached_count", 0}}},
	}
}

func rejectionCounts() ojson.Object {
	return ojson.Object{{"transport", ojson.Object{{"rejection_count", ojson.Object{}}}}}
}

// nodeStatsSection is one section of the node stats, or nil for none.
func (s *Server) nodeStatsSection(name, indexMetric, level string) any {
	switch name {
	case "indices":
		return s.nodeIndicesStats(indexMetric, level)
	case "os":
		return nodeOSStats()
	case "process":
		return nodeProcessStats()
	case "jvm":
		return nodeJVMStats()
	case "thread_pool":
		return nodeThreadPoolStats()
	case "fs":
		return nodeFSStats()
	case "transport":
		return ojson.Object{{"server_open", 0}, {"total_outbound_connections", 0}, {"rx_count", 0},
			{"rx_size_in_bytes", 0}, {"tx_count", 0}, {"tx_size_in_bytes", 0}}
	case "http":
		return ojson.Object{{"current_open", 1}, {"total_opened", 1}}
	case "breaker":
		return nodeBreakerStats()
	case "script":
		return scriptStats()
	case "discovery":
		return ojson.Object{
			{"cluster_state_queue", ojson.Object{{"total", 0}, {"pending", 0}, {"committed", 0}}},
			{"published_cluster_states", ojson.Object{{"full_states", 0}, {"incompatible_diffs", 0}, {"compatible_diffs", 0}}},
			{"cluster_state_stats", ojson.Object{{"overall", ojson.Object{{"update_count", 0},
				{"total_time_in_millis", 0}, {"failed_count", 0}}}}},
		}
	case "ingest":
		return ojson.Object{{"total", pipelineTotals()}, {"pipelines", ojson.Object{}}}
	case "adaptive_selection":
		return ojson.Object{}
	case "script_cache":
		return scriptCacheStats()
	case "indexing_pressure":
		cur := ojson.Object{{"combined_coordinating_and_primary_in_bytes", 0}, {"coordinating_in_bytes", 0},
			{"primary_in_bytes", 0}, {"replica_in_bytes", 0}, {"all_in_bytes", 0}}
		total := append(append(ojson.Object{}, cur...), ojson.KV{"coordinating_rejections", 0},
			ojson.KV{"primary_rejections", 0}, ojson.KV{"replica_rejections", 0})
		return ojson.Object{{"memory", ojson.Object{{"current", cur}, {"total", total},
			{"limit_in_bytes", int64(heapMax() / 10)}}}}
	case "shard_indexing_pressure":
		return ojson.Object{{"stats", ojson.Object{}},
			{"total_rejections_breakup_shadow_mode", ojson.Object{{"node_limits", 0},
				{"no_successful_request_limits", 0}, {"throughput_degradation_limits", 0}}},
			{"enabled", false}, {"enforced", false}}
	case "search_backpressure":
		return ojson.Object{{"search_task", backpressureTask()}, {"search_shard_task", backpressureTask()},
			{"mode", "monitor_only"}}
	case "cluster_manager_throttling":
		return ojson.Object{{"stats", ojson.Object{{"total_throttled_tasks", 0}, {"throttled_tasks_per_task_type", ojson.Object{}}}}}
	case "weighted_routing":
		return ojson.Object{{"stats", ojson.Object{{"fail_open_count", 0}}}}
	case "task_cancellation":
		t := ojson.Object{{"current_count_post_cancel", 0}, {"total_count_post_cancel", 0}}
		return ojson.Object{{"search_task", t}, {"search_shard_task", t}}
	case "search_pipeline":
		return ojson.Object{{"total_request", pipelineTotals()}, {"total_response", pipelineTotals()},
			{"pipelines", ojson.Object{}},
			{"system_generated_processors", ojson.Object{{"request_processors", []any{}}, {"response_processors", []any{}}}},
			{"system_generated_factories", ojson.Object{{"request_processor_factories", []any{}},
				{"response_processor_factories", []any{}}}}}
	case "resource_usage_stats":
		total, free, _, _ := osMemory()
		return ojson.Object{{model.NodeID, ojson.Object{{"timestamp", nowMillis()},
			{"cpu_utilization_percent", "0.0"},
			{"memory_utilization_percent", strconv.FormatFloat(float64(total-free)*100/float64(total), 'f', 1, 64)},
			{"native_memory_utilization_percent", "0.0"},
			{"io_usage_stats", ojson.Object{{"max_io_utilization_percent", "0.0"}}}}}}
	case "segment_replication_backpressure":
		return ojson.Object{{"total_rejected_requests", 0}}
	case "repositories":
		return []any{}
	case "admission_control":
		return ojson.Object{{"global_io_usage", rejectionCounts()}, {"global_cpu_usage", rejectionCounts()},
			{"global_native_memory_usage", rejectionCounts()}}
	case "caches":
		return ojson.Object{{"request_cache", ojson.Object{{"size_in_bytes", 0}, {"evictions", 0}, {"hit_count", 0},
			{"miss_count", 0}, {"item_count", 0}, {"store_name", "opensearch_onheap"}}}}
	case "remote_store":
		return ojson.Object{{"last_successful_fetch_of_pinned_timestamps", -1}}
	case "concurrency_limiter":
		return ojson.Object{}
	}
	return nil
}

var nodeStatsSectionKey = map[string]string{"breaker": "breakers", "concurrency_limiter": "concurrency_limiters"}

func (s *Server) buildNodeStats(r *http.Request, metrics []string, indexMetric, level string) ojson.Object {
	stats := ojson.Object{
		{"timestamp", nowMillis()},
		{"name", model.NodeName},
		{"transport_address", nodeTransport},
		{"host", nodeHost},
		{"ip", nodeHost},
		{"roles", nodeRoles()},
		{"attributes", nodeAttributes()},
	}
	want := map[string]bool{}
	for _, m := range metrics {
		want[m] = true
	}
	if want["breakers"] {
		want["breaker"] = true
	}
	for _, name := range nodeStatsSections {
		if !want[name] {
			continue
		}
		if v := s.nodeStatsSection(name, indexMetric, level); v != nil {
			key := name
			if k, ok := nodeStatsSectionKey[name]; ok {
				key = k
			}
			stats = append(stats, ojson.KV{key, v})
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return append(stats, ojson.KV{"native_memory", ojson.Object{{"total_estimated_bytes", ms.Sys}}})
}

// nodeIndicesStats is the indices section: the index stats of the node's
// open indices added up, with search request phases and status counters.
func (s *Server) nodeIndicesStats(indexMetric, level string) ojson.Object {
	var requested map[string]bool
	if indexMetric != "" && !strings.Contains(indexMetric, "_all") {
		requested = map[string]bool{}
		for _, m := range strings.Split(indexMetric, ",") {
			if m == "merge" {
				m = "merges"
			}
			requested[m] = true
		}
	}
	s.mu.RLock()
	indices, _ := s.resolveIndices("*", resolveOpts{wildcardClosed: true})
	total := map[string]any{}
	perIndex := map[string]any{}
	for _, idx := range indices {
		if idx.Closed {
			continue
		}
		m := s.indexMetrics(idx, requested)
		mergeMetrics(total, m)
		perIndex[idx.Name] = m
	}
	s.mu.RUnlock()
	for _, name := range statsMetricNames {
		if _, ok := total[name]; !ok && (requested == nil || requested[name]) {
			total[name] = emptyMetric(name)
		}
	}
	if search, ok := total["search"].(map[string]any); ok {
		phase := func() ojson.Object { return ojson.Object{{"time_in_millis", 0}, {"current", 0}, {"total", 0}} }
		search["request"] = ojson.Object{{"took", phase()}, {"dfs_pre_query", phase()}, {"query", phase()},
			{"fetch", phase()}, {"dfs_query", phase()}, {"expand", phase()}, {"can_match", phase()}}
	}
	out := ojson.Object{}
	for _, name := range statsMetricNames {
		if v, ok := total[name]; ok {
			out = append(out, ojson.KV{name, v})
		}
	}
	status := func() ojson.Object { return ojson.Object{{"success", 0}, {"user_error", 0}, {"system_failure", 0}} }
	out = append(out, ojson.KV{"status_counter", ojson.Object{{"doc_status", status()}, {"search_response_status", status()}}})
	if level == "indices" {
		out = append(out, ojson.KV{"indices", perIndex})
	}
	return out
}
