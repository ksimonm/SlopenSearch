package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// storedTask is a task started with wait_for_completion=false. It runs in
// the background; once completed its result stays retrievable.
type storedTask struct {
	id          int64
	action      string
	description string
	start       time.Time
	runNanos    int64
	cancellable bool
	headers     map[string]any
	status      orderedJSON // nil for tasks without a status (forcemerge)
	response    any
	errBody     any

	// live state, guarded by taskReg
	done           chan struct{} // closed on completion
	wake           chan struct{} // rethrottle/cancel nudges for the worker
	completed      bool
	bulk           bool // BulkByScrollTask with a live status
	cancelled      bool
	cancelAt       time.Time
	canceled       string // "canceled" reason of a by-scroll result
	ctr            bbsCounters
	rps            float64 // +Inf when unthrottled
	delayEnd       time.Time
	delayRPS       float64
	throttledNanos int64
	failures       []any
}

var taskReg = struct {
	sync.Mutex
	next  int64
	tasks map[int64]*storedTask
}{next: 1000, tasks: map[int64]*storedTask{}}

const rpsReason = "[requests_per_second] must be a float greater than 0. Use -1 to disable throttling."

// parseRPS validates a requests_per_second value; "" means unthrottled.
func parseRPS(v string) (float64, *apiError) {
	if v == "" {
		return -1, nil
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: rpsReason,
			CausedBy: &apiError{Type: "number_format_exception", Reason: fmt.Sprintf("For input string: \"%s\"", v)}}
	}
	if f == -1 {
		return -1, nil
	}
	if f <= 0 {
		return 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: rpsReason}
	}
	return f, nil
}

func malformedTaskID(id string) *apiError {
	return &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "malformed task id " + id}
}

// parseTaskID splits "node:number" the way TaskId does.
func parseTaskID(id string) (string, int64, *apiError) {
	i := strings.Index(id, ":")
	if i < 0 {
		return "", 0, malformedTaskID(id)
	}
	node, num := id[:i], id[i+1:]
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		e := malformedTaskID(id)
		e.CausedBy = &apiError{Type: "number_format_exception", Reason: fmt.Sprintf("For input string: \"%s\"", num)}
		return "", 0, e
	}
	return node, n, nil
}

func taskNodeFailure(reason string) orderedJSON {
	return orderedJSON{
		{"node_failures", []any{orderedJSON{
			{"type", "failed_node_exception"}, {"reason", "Failed node [" + nodeID + "]"}, {"node_id", nodeID},
			{"caused_by", orderedJSON{{"type", "resource_not_found_exception"}, {"reason", reason}}},
		}}},
		{"nodes", map[string]any{}},
	}
}

func asyncAction(segs []string) (string, bool) {
	if len(segs) == 0 {
		return "", false
	}
	switch segs[len(segs)-1] {
	case "_reindex":
		return "indices:data/write/reindex", len(segs) == 1
	case "_update_by_query":
		return "indices:data/write/update/byquery", len(segs) == 2
	case "_delete_by_query":
		return "indices:data/write/delete/byquery", len(segs) == 2
	case "_forcemerge":
		return "indices:admin/forcemerge", len(segs) <= 2
	}
	return "", false
}

// handleTaskRoutes serves async starts, rethrottle and single-task routes.
// It returns false when the request is not one of them.
func (s *Server) handleTaskRoutes(w http.ResponseWriter, r *http.Request) bool {
	segs := []string{}
	for _, p := range strings.Split(strings.Trim(r.URL.Path, "/"), "/") {
		if p != "" {
			segs = append(segs, p)
		}
	}
	q := r.URL.Query()
	if len(segs) == 3 && segs[2] == "_rethrottle" && r.Method == http.MethodPost &&
		(segs[0] == "_reindex" || segs[0] == "_update_by_query" || segs[0] == "_delete_by_query") {
		v, ok := q["requests_per_second"]
		if !ok {
			writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "requests_per_second is a required parameter"})
			return true
		}
		if _, e := parseRPS(v[len(v)-1]); e != nil {
			writeError(w, e)
			return true
		}
		rps, _ := parseRPS(v[len(v)-1])
		node, n, e := parseTaskID(segs[1])
		if e != nil {
			writeError(w, e)
			return true
		}
		if t := liveTask(n); t != nil && node == nodeID && t.bulk {
			writeJSON(w, 200, rethrottleTask(t, segs[1], rps))
			return true
		}
		writeJSON(w, 200, taskNodeFailure("task ["+segs[1]+"] is missing"))
		return true
	}
	if len(segs) >= 2 && segs[0] == "_tasks" && segs[1] != "_cancel" {
		id := segs[1]
		if len(segs) == 3 && segs[2] == "_cancel" && r.Method == http.MethodPost {
			node, n, e := parseTaskID(id)
			if e != nil {
				writeError(w, e)
				return true
			}
			if t := liveTask(n); t != nil && node == nodeID && t.cancellable {
				writeJSON(w, 200, cancelTask(t, id))
				return true
			}
			writeJSON(w, 200, taskNodeFailure("task ["+id+"] is not found"))
			return true
		}
		if len(segs) != 2 || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			return false
		}
		node, n, e := parseTaskID(id)
		if e != nil {
			writeError(w, e)
			return true
		}
		notStored := "task [" + id + "] isn't running and hasn't stored its results"
		if node != nodeID {
			writeError(w, &apiError{Type: "resource_not_found_exception", Status: 404,
				Reason:    "task [" + id + "] belongs to the node [" + node + "] which isn't part of the cluster and there is no record of the task",
				CausedBy:  &apiError{Type: "resource_not_found_exception", Reason: notStored},
				RootCause: &apiError{Type: "resource_not_found_exception", Reason: notStored}})
			return true
		}
		taskReg.Lock()
		t := taskReg.tasks[n]
		taskReg.Unlock()
		if t == nil {
			writeError(w, &apiError{Type: "resource_not_found_exception", Status: 404, Reason: notStored})
			return true
		}
		if wv, has := q["wait_for_completion"]; has && (wv[len(wv)-1] == "true" || wv[len(wv)-1] == "") {
			timeout, ok := parseTimeout(q.Get("timeout"))
			if !ok {
				timeout = 30 * time.Second
			}
			if !waitTask(t, timeout) {
				writeError(w, taskTimeoutError(t))
				return true
			}
		}
		taskReg.Lock()
		out := t.render()
		taskReg.Unlock()
		writeJSON(w, 200, out)
		return true
	}
	action, ok := asyncAction(segs)
	if !ok || r.Method != http.MethodPost {
		return false
	}
	wfc, has := q["wait_for_completion"]
	if !has {
		return false
	}
	wv := wfc[len(wfc)-1]
	if wv != "true" && wv != "false" {
		writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "Failed to parse value [" + wv + "] as only [true] or [false] are allowed."})
		return true
	}
	if wv == "true" {
		return false
	}
	return s.startAsync(w, r, segs, action)
}

func (s *Server) startAsync(w http.ResponseWriter, r *http.Request, segs []string, action string) bool {
	q := r.URL.Query()
	isForcemerge := action == "indices:admin/forcemerge"
	rps := -1.0
	if !isForcemerge {
		var e *apiError
		if rps, e = parseRPS(q.Get("requests_per_second")); e != nil {
			writeError(w, e)
			return true
		}
		if v := q.Get("slices"); v != "" && v != "auto" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				e := &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("[slices] must be a positive integer or the string \"auto\", but was [%s]", v)}
				if err != nil {
					e.CausedBy = &apiError{Type: "number_format_exception", Reason: fmt.Sprintf("For input string: \"%s\"", v)}
				}
				writeError(w, e)
				return true
			}
		}
		if q.Get("scroll_size") == "0" {
			writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: [size] cannot be [0] in a scroll context;"})
			return true
		}
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = unmarshalJSON(raw, &body)
	batchSize := int64(1000)
	if v := q.Get("scroll_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			batchSize = n
		}
	}
	var desc string
	switch action {
	case "indices:data/write/reindex":
		srcName, dstName := "", ""
		if src, ok := body["source"].(map[string]any); ok {
			srcName = joinNames(src["index"])
			if n, ok := toFloat(src["size"]); ok && n > 0 {
				batchSize = int64(n)
			}
		}
		if dst, ok := body["dest"].(map[string]any); ok {
			dstName = joinNames(dst["index"])
		}
		desc = "reindex from [" + srcName + "] to [" + dstName + "]"
	case "indices:data/write/update/byquery":
		desc = "update-by-query [" + segs[0] + "]"
	case "indices:data/write/delete/byquery":
		desc = "delete-by-query [" + segs[0] + "]"
	default:
		name := "_all"
		if len(segs) == 2 {
			name = segs[0]
		}
		desc = "Force-merge indices [" + name + "], maxSegments[-1], onlyExpungeDeletes[false], flush[true], primaryOnly[false]"
	}
	t := &storedTask{action: action, description: desc, start: time.Now(),
		cancellable: !isForcemerge, headers: map[string]any{}, bulk: !isForcemerge,
		done: make(chan struct{}), wake: make(chan struct{}, 1), rps: rpsInternal(rps)}
	if oid := r.Header.Get("X-Opaque-Id"); oid != "" {
		t.headers["X-Opaque-Id"] = oid
	}
	taskReg.Lock()
	taskReg.next++
	t.id = taskReg.next
	taskReg.tasks[t.id] = t
	taskReg.Unlock()
	ctx := context.WithoutCancel(r.Context())
	r = r.WithContext(ctx)
	if !isForcemerge && rps > 0 {
		if refs, ok := s.bbsMatches(action, segs, q, body); ok {
			go s.runThrottled(t, r, segs, q, body, refs, int(batchSize))
			writeJSON(w, 200, orderedJSON{{"task", nodeID + ":" + strconv.FormatInt(t.id, 10)}})
			return true
		}
	}
	// run the whole operation in one piece, with wait_for_completion removed
	nq := url.Values{}
	for k, v := range q {
		if !formatParams[k] {
			nq[k] = v
		}
	}
	r2 := subRequest(r, r.URL.Path, nq, raw)
	r2.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if r2.Header.Get("Content-Type") == "" {
		r2.Header.Del("Content-Type")
	}
	go func() {
		rec := &captureWriter{h: http.Header{}}
		s.route(rec, r2)
		var out map[string]any
		_ = json.Unmarshal(rec.buf.Bytes(), &out)
		status := rec.status
		if status == 0 {
			status = 200
		}
		taskReg.Lock()
		defer taskReg.Unlock()
		if !isForcemerge {
			t.status = taskStatus(out, batchSize, rps, false, status >= 400)
		}
		if status >= 400 {
			t.errBody = out["error"]
			if t.errBody == nil {
				t.errBody = out
			}
		} else if isForcemerge {
			t.response = out
		} else {
			t.response = taskStatus(out, batchSize, rps, true, false)
		}
		t.finishLocked(time.Now())
	}()
	writeJSON(w, 200, orderedJSON{{"task", nodeID + ":" + strconv.FormatInt(t.id, 10)}})
	return true
}

func joinNames(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, p := range t {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func numOf(m map[string]any, k string) int64 {
	if f, ok := toFloat(m[k]); ok {
		return int64(f)
	}
	return 0
}

// taskStatus builds a BulkByScrollTask status (or, with asResponse, the
// matching BulkByScrollResponse) from a synchronous by-scroll result.
func taskStatus(out map[string]any, batchSize int64, rps float64, asResponse, failed bool) orderedJSON {
	if out == nil || failed {
		out = map[string]any{}
	}
	total := numOf(out, "total")
	batches := int64(0)
	if total > 0 {
		batches = int64(math.Ceil(float64(total) / float64(batchSize)))
	}
	build := func(m map[string]any, total, batches int64, slice int) orderedJSON {
		o := orderedJSON{}
		if slice >= 0 {
			o = append(o, orderedKV{"slice_id", slice})
		}
		if asResponse && slice < 0 {
			o = append(o, orderedKV{"took", numOf(m, "took")}, orderedKV{"timed_out", false})
		}
		o = append(o, orderedKV{"total", total}, orderedKV{"updated", numOf(m, "updated")},
			orderedKV{"created", numOf(m, "created")}, orderedKV{"deleted", numOf(m, "deleted")},
			orderedKV{"batches", batches}, orderedKV{"version_conflicts", numOf(m, "version_conflicts")},
			orderedKV{"noops", numOf(m, "noops")}, orderedKV{"retries", orderedJSON{{"bulk", 0}, {"search", 0}}})
		if asResponse {
			o = append(o, orderedKV{"throttled", "0s"})
		}
		o = append(o, orderedKV{"throttled_millis", 0}, orderedKV{"requests_per_second", rps})
		if asResponse {
			o = append(o, orderedKV{"throttled_until", "0s"})
		}
		o = append(o, orderedKV{"throttled_until_millis", 0})
		return o
	}
	res := build(out, total, batches, -1)
	if sl, ok := out["slices"].([]any); ok && len(sl) > 0 {
		arr := make([]any, 0, len(sl))
		var sum int64
		for i, s := range sl {
			m, _ := s.(map[string]any)
			st := numOf(m, "total")
			b := int64(0)
			if st > 0 {
				b = int64(math.Ceil(float64(st) / float64(batchSize)))
			}
			sum += b
			arr = append(arr, build(m, st, b, i))
		}
		res[findKey(res, "batches")].v = sum
		res = append(res, orderedKV{"slices", arr})
	}
	if asResponse {
		f := out["failures"]
		if f == nil {
			f = []any{}
		}
		res = append(res, orderedKV{"failures", f})
	}
	return res
}

func findKey(o orderedJSON, k string) int {
	for i, kv := range o {
		if kv.k == k {
			return i
		}
	}
	return -1
}

// render answers GET _tasks/{id}; the caller holds taskReg.
func (t *storedTask) render() orderedJSON {
	if !t.completed {
		return orderedJSON{{"completed", false}, {"task", t.taskEntry(time.Now(), true)}}
	}
	task := orderedJSON{{"node", nodeID}, {"id", t.id}, {"type", "transport"}, {"action", t.action}}
	if t.status != nil {
		task = append(task, orderedKV{"status", t.status})
	}
	task = append(task, orderedKV{"description", t.description},
		orderedKV{"start_time_in_millis", t.start.UnixMilli()},
		orderedKV{"running_time_in_nanos", t.runNanos},
		orderedKV{"cancellable", t.cancellable}, orderedKV{"cancelled", t.cancelled},
		orderedKV{"headers", t.headers})
	if t.cancelled {
		task = append(task, orderedKV{"cancellation_time_millis", t.cancelAt.UnixMilli()})
	}
	out := orderedJSON{{"completed", true}, {"task", task}}
	if t.errBody != nil {
		out = append(out, orderedKV{"error", t.errBody})
	} else {
		out = append(out, orderedKV{"response", t.response})
	}
	return out
}
