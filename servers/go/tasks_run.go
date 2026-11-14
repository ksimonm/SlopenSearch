package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Background execution of tasks started with wait_for_completion=false.
//
// A throttled by-scroll task (requests_per_second > 0) runs batch by batch
// the way AbstractAsyncBulkByScrollAction does: the matching documents are
// collected once (the scroll snapshot), every batch of scroll_size documents
// is processed by running the synchronous operation restricted to the
// batch's ids, and before the next batch the worker waits
// batchSize / requests_per_second seconds minus the time the batch took.
// Rethrottling and cancelling wake the waiting worker.

type bbsCounters struct {
	total, updated, created, deleted, batches, conflicts, noops int64
}

type bbsRef struct{ index, id string }

// rpsJSON renders a requests_per_second float (-1 for unlimited).
func rpsJSON(f float64) any {
	if math.IsInf(f, 1) || f < 0 {
		return -1.0
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 32))
}

func rpsInternal(f float64) float64 {
	if f < 0 {
		return math.Inf(1)
	}
	return f
}

// liveStatus renders the BulkByScrollTask status of a running or finished
// throttled task; the caller holds taskReg.
func (t *storedTask) liveStatus(now time.Time, asResponse bool) orderedJSON {
	c := t.ctr
	o := orderedJSON{}
	if asResponse {
		o = append(o, orderedKV{"took", now.Sub(t.start).Milliseconds()}, orderedKV{"timed_out", false})
	}
	o = append(o, orderedKV{"total", c.total}, orderedKV{"updated", c.updated},
		orderedKV{"created", c.created}, orderedKV{"deleted", c.deleted},
		orderedKV{"batches", c.batches}, orderedKV{"version_conflicts", c.conflicts},
		orderedKV{"noops", c.noops}, orderedKV{"retries", orderedJSON{{"bulk", 0}, {"search", 0}}})
	if asResponse {
		o = append(o, orderedKV{"throttled", "0s"})
	}
	o = append(o, orderedKV{"throttled_millis", t.throttledNanos / 1e6}, orderedKV{"requests_per_second", rpsJSON(t.rps)})
	if t.canceled != "" {
		o = append(o, orderedKV{"canceled", t.canceled})
	}
	until := int64(0)
	if !t.delayEnd.IsZero() && t.delayEnd.After(now) {
		until = t.delayEnd.Sub(now).Milliseconds()
	}
	if asResponse {
		o = append(o, orderedKV{"throttled_until", "0s"})
	}
	o = append(o, orderedKV{"throttled_until_millis", until})
	if asResponse {
		f := t.failures
		if f == nil {
			f = []any{}
		}
		o = append(o, orderedKV{"failures", f})
	}
	return o
}

// taskEntry renders a running task as the task list, cancel and rethrottle
// APIs show it; the caller holds taskReg.
func (t *storedTask) taskEntry(now time.Time, detailed bool) orderedJSON {
	e := orderedJSON{{"node", nodeID}, {"id", t.id}, {"type", "transport"}, {"action", t.action}}
	if detailed {
		if t.bulk {
			e = append(e, orderedKV{"status", t.liveStatus(now, false)})
		}
		e = append(e, orderedKV{"description", t.description})
	}
	e = append(e, orderedKV{"start_time_in_millis", t.start.UnixMilli()},
		orderedKV{"running_time_in_nanos", now.Sub(t.start).Nanoseconds()},
		orderedKV{"cancellable", t.cancellable}, orderedKV{"cancelled", t.cancelled},
		orderedKV{"headers", t.headers})
	if t.cancelled {
		e = append(e, orderedKV{"cancellation_time_millis", t.cancelAt.UnixMilli()})
	}
	if detailed {
		e = append(e, orderedKV{"resource_stats", resourceStatsJSON()})
	}
	return e
}

func resourceStatsJSON() orderedJSON {
	z := func() orderedJSON { return orderedJSON{{"cpu_time_in_nanos", 0}, {"memory_in_bytes", 0}} }
	return orderedJSON{{"average", z()}, {"total", z()}, {"min", z()}, {"max", z()},
		{"thread_info", orderedJSON{{"thread_executions", 0}, {"active_threads", 0}}}}
}

// taskNodeJSON wraps task entries in the node object of task list answers.
func taskNodeJSON(tasks orderedJSON) orderedJSON {
	return orderedJSON{
		{"nodes", orderedJSON{{nodeID, orderedJSON{
			{"name", nodeName}, {"transport_address", nodeTransport}, {"host", nodeHost},
			{"ip", nodeTransport}, {"roles", nodeRoles()}, {"attributes", nodeAttributes()},
			{"tasks", tasks},
		}}}},
	}
}

// runningTasks lists the tasks still running whose action matches.
func runningTasks(actions string, detailed bool) orderedJSON {
	now := time.Now()
	taskReg.Lock()
	defer taskReg.Unlock()
	ids := make([]int64, 0)
	for id, t := range taskReg.tasks {
		if !t.completed && taskActionMatches(map[string]any{"action": t.action}, actions) {
			ids = append(ids, id)
		}
	}
	sortInt64s(ids)
	out := orderedJSON{}
	for _, id := range ids {
		t := taskReg.tasks[id]
		out = append(out, orderedKV{nodeID + ":" + strconv.FormatInt(id, 10), t.taskEntry(now, detailed)})
	}
	return out
}

func sortInt64s(a []int64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// liveTask returns the running task with the given number, or nil.
func liveTask(n int64) *storedTask {
	taskReg.Lock()
	defer taskReg.Unlock()
	if t := taskReg.tasks[n]; t != nil && !t.completed {
		return t
	}
	return nil
}

func (t *storedTask) poke() {
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// cancelTask marks a running task cancelled and answers like
// POST _tasks/{id}/_cancel.
func cancelTask(t *storedTask, tid string) orderedJSON {
	taskReg.Lock()
	if !t.cancelled {
		t.cancelled = true
		t.cancelAt = time.Now()
	}
	entry := t.taskEntry(time.Now(), false)
	taskReg.Unlock()
	t.poke()
	return taskNodeJSON(orderedJSON{{tid, entry}})
}

// rethrottleTask changes a running task's requests_per_second the way
// WorkerBulkByScrollTaskState.rethrottle does: a slower rate applies from
// the next batch on, a faster one reschedules the pending wait.
func rethrottleTask(t *storedTask, tid string, rps float64) orderedJSON {
	newRPS := rpsInternal(rps)
	taskReg.Lock()
	old := t.rps
	t.rps = newRPS
	if !t.delayEnd.IsZero() && !(newRPS < old) {
		remaining := time.Until(t.delayEnd)
		if remaining < 0 {
			remaining = 0
		}
		nd := time.Duration(0)
		if !math.IsInf(newRPS, 1) {
			nd = time.Duration(float64(remaining) * t.delayRPS / newRPS)
		}
		t.delayEnd = time.Now().Add(nd)
		t.delayRPS = newRPS
	}
	entry := t.taskEntry(time.Now(), true)
	taskReg.Unlock()
	t.poke()
	return taskNodeJSON(orderedJSON{{tid, entry}})
}

// waitTask blocks until the task completes or the timeout elapses.
func waitTask(t *storedTask, timeout time.Duration) bool {
	select {
	case <-t.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func taskTimeoutError(t *storedTask) *apiError {
	class := "org.opensearch.tasks.Task"
	if t.bulk {
		class = "org.opensearch.index.reindex.BulkByScrollTask"
	}
	return &apiError{Type: "timeout_exception", Status: 504,
		Reason: fmt.Sprintf("Timed out waiting for completion of [%s@%x]", class, uint32(t.id*2654435761))}
}

// formatParams are dropped from internal sub-requests: their output is
// parsed, never sent to the client.
var formatParams = map[string]bool{"wait_for_completion": true, "pretty": true, "human": true,
	"error_trace": true, "filter_path": true, "format": true}

func subRequest(r *http.Request, path string, q url.Values, body []byte) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	r2.URL.RawPath = ""
	r2.URL.RawQuery = q.Encode()
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.ContentLength = int64(len(body))
	r2.Header.Set("Content-Type", "application/json")
	return r2
}

// bbsMatches collects the documents a by-scroll operation will visit, in
// order. ok is false when the request should rather run in one piece (it
// fails, or uses features the batch runner does not split).
func (s *Server) bbsMatches(action string, segs []string, q url.Values, body map[string]any) ([]bbsRef, bool) {
	if v := q.Get("slices"); v != "" && v != "1" {
		return nil, false
	}
	if v, ok := body["slice"]; ok && v != nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var refs []bbsRef
	maxDocs := int64(-1)
	if v, ok := toFloat(body["max_docs"]); ok {
		maxDocs = int64(v)
	}
	if v := q.Get("max_docs"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, false
		}
		maxDocs = n
	}
	collect := func(idx *Index, query map[string]any, skipPending bool) bool {
		for _, id := range idx.Order {
			if maxDocs >= 0 && int64(len(refs)) >= maxDocs {
				return true
			}
			doc, live := idx.Docs[id]
			if !live {
				continue
			}
			if _, pending := idx.Pending[id]; pending && skipPending {
				continue
			}
			if query != nil {
				qc := newQueryCtxForDoc(idx, id, doc)
				match, _, qerr := qc.eval(query)
				if qerr != nil {
					return false
				}
				if !match {
					continue
				}
			}
			refs = append(refs, bbsRef{idx.Name, id})
		}
		return true
	}
	if action == "indices:data/write/reindex" {
		src, _ := body["source"].(map[string]any)
		dst, _ := body["dest"].(map[string]any)
		if src == nil || dst == nil || src["remote"] != nil {
			return nil, false
		}
		spec := joinNames(src["index"])
		spec = strings.ReplaceAll(spec, ", ", ",")
		if spec == "" {
			return nil, false
		}
		allowNo := false
		indices, e := s.resolveSpec2(spec, resolveOpts2{allowNoIndices: &allowNo})
		if e != nil {
			return nil, false
		}
		var query map[string]any
		if qv, ok := src["query"]; ok {
			if query, ok = qv.(map[string]any); !ok {
				return nil, false
			}
		}
		for _, idx := range indices {
			if idx.Closed || !collect(idx, query, false) {
				return nil, false
			}
		}
		return refs, true
	}
	idx, e := s.resolveReadTarget(segs[0])
	if e != nil || idx.Closed {
		return nil, false
	}
	query, has := body["query"]
	if qp := q.Get("q"); qp != "" {
		query, has = parseQParam(qp), true
	}
	if !has {
		if action == "indices:data/write/delete/byquery" {
			return nil, false
		}
		query = map[string]any{"match_all": map[string]any{}}
	}
	qm, ok := query.(map[string]any)
	if !ok || !collect(idx, qm, true) {
		return nil, false
	}
	return refs, true
}

// runThrottled executes a by-scroll task batch by batch.
func (s *Server) runThrottled(t *storedTask, r *http.Request, segs []string, q url.Values,
	body map[string]any, refs []bbsRef, batchSize int) {
	nq := url.Values{}
	for k, v := range q {
		switch k {
		case "requests_per_second", "slices", "max_docs", "scroll_size", "q":
			continue
		}
		if !formatParams[k] {
			nq[k] = v
		}
	}
	isReindex := t.action == "indices:data/write/reindex"
	path := "/" + strings.Join(segs, "/")
	taskReg.Lock()
	t.ctr.total = int64(len(refs))
	taskReg.Unlock()
	var errBody any
	lastBatchSize := 0
	var lastBatchStart time.Time
	for pos := 0; ; {
		// throttle before the next batch
		if lastBatchSize > 0 {
			taskReg.Lock()
			wait := time.Duration(0)
			if !math.IsInf(t.rps, 1) {
				perfect := time.Duration(float64(lastBatchSize) / t.rps * float64(time.Second))
				wait = time.Until(lastBatchStart.Add(perfect))
				if wait > time.Hour {
					wait = time.Hour
				}
			}
			var planned time.Time
			if wait > 0 && !t.cancelled {
				planned = time.Now().Add(wait)
				t.delayEnd = planned
				t.delayRPS = t.rps
			}
			taskReg.Unlock()
			for {
				taskReg.Lock()
				end, cancelled := t.delayEnd, t.cancelled
				taskReg.Unlock()
				left := time.Until(end)
				if end.IsZero() || cancelled || left <= 0 {
					taskReg.Lock()
					if !end.IsZero() && end.Equal(planned) && !cancelled {
						t.throttledNanos += int64(wait)
					}
					t.delayEnd = time.Time{}
					taskReg.Unlock()
					break
				}
				timer := time.NewTimer(left)
				select {
				case <-timer.C:
				case <-t.wake:
				}
				timer.Stop()
			}
		}
		taskReg.Lock()
		cancelled := t.cancelled
		taskReg.Unlock()
		if cancelled {
			taskReg.Lock()
			t.canceled = "by user request"
			t.rps = math.Inf(1)
			taskReg.Unlock()
			break
		}
		if pos >= len(refs) {
			break
		}
		end := pos + batchSize
		if end > len(refs) {
			end = len(refs)
		}
		batch := refs[pos:end]
		pos = end
		lastBatchStart = time.Now()
		lastBatchSize = len(batch)
		taskReg.Lock()
		t.ctr.batches++
		taskReg.Unlock()
		// group the batch by index, keeping order
		var groups [][]bbsRef
		for _, ref := range batch {
			if n := len(groups); n > 0 && groups[n-1][0].index == ref.index {
				groups[n-1] = append(groups[n-1], ref)
			} else {
				groups = append(groups, []bbsRef{ref})
			}
		}
		stop := false
		for _, g := range groups {
			ids := make([]any, len(g))
			for i, ref := range g {
				ids[i] = ref.id
			}
			sub := map[string]any{}
			for k, v := range body {
				sub[k] = v
			}
			delete(sub, "max_docs")
			idsQuery := map[string]any{"ids": map[string]any{"values": ids}}
			subPath := path
			if isReindex {
				src := map[string]any{}
				for k, v := range body["source"].(map[string]any) {
					src[k] = v
				}
				src["index"] = g[0].index
				src["query"] = idsQuery
				sub["source"] = src
			} else {
				sub["query"] = idsQuery
			}
			raw, _ := json.Marshal(sub)
			rec := &captureWriter{h: http.Header{}}
			s.route(rec, subRequest(r, subPath, nq, raw))
			var out map[string]any
			_ = unmarshalJSON(rec.buf.Bytes(), &out)
			status := rec.status
			if status == 0 {
				status = 200
			}
			if out == nil || (status >= 400 && out["error"] != nil) || (status >= 400 && out["total"] == nil) {
				errBody = out["error"]
				if errBody == nil {
					errBody = out
				}
				stop = true
				break
			}
			taskReg.Lock()
			t.ctr.updated += numOf(out, "updated")
			t.ctr.created += numOf(out, "created")
			t.ctr.deleted += numOf(out, "deleted")
			t.ctr.noops += numOf(out, "noops")
			t.ctr.conflicts += numOf(out, "version_conflicts")
			if f, ok := out["failures"].([]any); ok && len(f) > 0 {
				t.failures = append(t.failures, f...)
				stop = true
			}
			taskReg.Unlock()
			if stop {
				break
			}
		}
		if stop {
			break
		}
	}
	taskReg.Lock()
	now := time.Now()
	t.delayEnd = time.Time{}
	t.status = t.liveStatus(now, false)
	if errBody != nil {
		t.errBody = errBody
	} else {
		t.response = t.liveStatus(now, true)
	}
	t.finishLocked(now)
	taskReg.Unlock()
}

// finishLocked marks the task completed; the caller holds taskReg.
func (t *storedTask) finishLocked(now time.Time) {
	t.runNanos = now.Sub(t.start).Nanoseconds()
	t.completed = true
	close(t.done)
}
