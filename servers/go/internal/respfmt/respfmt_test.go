package respfmt

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Request URLs and expected bodies are taken verbatim from tests/reference
// (OpenSearch 3.9 answers): test_01_cluster.jsonl, test_62_tasks_http.jsonl,
// test_33_search_api.jsonl, test_21_documents.jsonl. Input bodies are the
// unfiltered OpenSearch responses recorded for the same resources.

const (
	// GET /_cluster/health/c01-43077bc9-gr
	healthGreen = `{"cluster_name":"opensearch","status":"green","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"discovered_master":true,"discovered_cluster_manager":true,"active_primary_shards":2,"active_shards":2,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0,"delayed_unassigned_shards":0,"number_of_pending_tasks":7,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":76,"active_shards_percent_as_number":100.0}`
	// GET /_cluster/health/c01-43077bc9-gr?level=shards
	healthShards = `{"cluster_name":"opensearch","status":"green","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"discovered_master":true,"discovered_cluster_manager":true,"active_primary_shards":2,"active_shards":2,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0,"delayed_unassigned_shards":0,"number_of_pending_tasks":3,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":14,"active_shards_percent_as_number":100.0,"indices":{"c01-43077bc9-gr":{"status":"green","number_of_shards":2,"number_of_replicas":0,"active_primary_shards":2,"active_shards":2,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0,"shards":{"0":{"status":"green","primary_active":true,"active_shards":1,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0},"1":{"status":"green","primary_active":true,"active_shards":1,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0}}}}}`
	// GET /idx/_doc/1 for {"a":"x","n":1}
	getDoc = `{"_index":"slopensearch-test-efdad27c","_id":"1","_version":1,"_seq_no":0,"_primary_term":1,"found":true,"_source":{"a":"x","n":1}}`
	// POST /sa-corpus-31d2e98d5a/_search {"query":{"match":{"title":"fox"}}}
	search  = `{"took":1,"timed_out":false,"_shards":{"total":1,"successful":1,"skipped":0,"failed":0},"hits":{"total":{"value":2,"relation":"eq"},"max_score":0.3743781,"hits":[{"_index":"sa-corpus-31d2e98d5a","_id":"1","_score":0.3743781,"_source":{"title": "quick brown fox", "body": "the quick brown fox jumps over the lazy dog", "tag": "a", "n": 1, "nums": [3, 10], "price": 1.5, "date": "2024-01-01T00:00:00Z", "loc": {"lat": 0, "lon": 0}, "items": [{"name": "x", "qty": 5}, {"name": "y", "qty": 1}], "obj": {"a": "p", "b": 1}}},{"_index":"sa-corpus-31d2e98d5a","_id":"3","_score":0.3743781,"_source":{"title": "quick red fox", "body": "red foxes are quick and clever animals", "tag": "a", "n": 3, "nums": [5], "price": 3.5, "date": "2024-03-01T00:00:00Z", "loc": {"lat": 0, "lon": 2}, "items": [{"name": "y", "qty": 9}], "obj": {"a": "p", "b": 3}}}]}}`
	bulk    = `{"took":26,"errors":false,"items":[{"index":{"_index":"slopensearch-auto-5e3a905d8f","_id":"1","_version":1,"result":"created","_shards":{"total":2,"successful":1,"failed":0},"_seq_no":0,"_primary_term":1,"status":201}}]}`
	errBody = `{"error":{"root_cause":[{"type":"illegal_argument_exception","reason":"No enum constant org.opensearch.cluster.health.ClusterHealthStatus.PURPLE"}],"type":"illegal_argument_exception","reason":"No enum constant org.opensearch.cluster.health.ClusterHealthStatus.PURPLE"},"status":400}`
)

func TestProcess(t *testing.T) {
	cases := []struct {
		name   string
		target string
		status int
		body   string
		want   string
	}{
		// filter_path
		{"exclusions with wildcards", "/_cluster/health/c01-43077bc9-gr?filter_path=-cluster_name%2C-number_of_%2A%2C-discovered%2A%2C-active%2A%2C-%2A_shards%2C-task_max_waiting_in_queue_millis",
			200, healthGreen, `{"status":"green","timed_out":false}`},
		{"suffix wildcard", "/_cluster/health/c01-43077bc9-gr?filter_path=%2Astatus",
			200, healthGreen, `{"status":"green"}`},
		{"multiple keep source order", "/_cluster/health/c01-43077bc9-gr?filter_path=status%2Cnumber_of_nodes%2Cactive_primary_shards",
			200, healthGreen, `{"status":"green","number_of_nodes":1,"active_primary_shards":2}`},
		{"no match is empty object", "/_cluster/health?filter_path=nothing_here",
			200, healthGreen, `{}`},
		{"double wildcard", "/_cluster/health/c01-43077bc9-gr?filter_path=%2A%2A.status&level=shards",
			200, healthShards, `{"indices":{"c01-43077bc9-gr":{"status":"green","shards":{"0":{"status":"green"},"1":{"status":"green"}}}}}`},
		{"include then exclude nested", "/_cluster/health/c01-43077bc9-gr?level=shards&filter_path=indices%2C-indices.%2A.shards",
			200, healthShards, `{"indices":{"c01-43077bc9-gr":{"status":"green","number_of_shards":2,"number_of_replicas":0,"active_primary_shards":2,"active_shards":2,"relocating_shards":0,"initializing_shards":0,"unassigned_shards":0}}}`},
		{"not applied to errors", "/_cluster/health?wait_for_status=purple&filter_path=status",
			400, errBody, errBody},
		{"source subfield", "/slopensearch-test-088c7e4d/_doc/1?filter_path=_source.a",
			200, getDoc, `{"_source":{"a":"x"}}`},
		{"exclude list", "/slopensearch-test-1d1686d4/_doc/1?filter_path=-_source%2C-_seq_no%2C-_primary_term",
			200, getDoc, `{"_index":"slopensearch-test-efdad27c","_id":"1","_version":1,"found":true}`},
		{"empty filter_path", "/slopensearch-test-efdad27c/_doc/1?filter_path=",
			200, getDoc, getDoc},
		{"hits ids through arrays", "/sa-corpus-31d2e98d5a/_search?filter_path=hits.hits._id",
			200, search, `{"hits":{"hits":[{"_id":"1"},{"_id":"3"}]}}`},
		{"double wildcard prefix", "/sa-corpus-31d2e98d5a/_search?filter_path=%2A%2A._id",
			200, search, `{"hits":{"hits":[{"_id":"1"},{"_id":"3"}]}}`},
		{"object value kept whole", "/sa-corpus-31d2e98d5a/_search?filter_path=hits.total",
			200, search, `{"hits":{"total":{"value":2,"relation":"eq"}}}`},
		{"deep no match", "/sa-corpus-31d2e98d5a/_search?filter_path=nothing.matches",
			200, search, `{}`},
		{"wildcard inside source", "/sa-corpus-31d2e98d5a/_search?filter_path=hits.hits._source.ti%2A",
			200, search, `{"hits":{"hits":[{"_source":{"title":"quick brown fox"}},{"_source":{"title":"quick red fox"}}]}}`},
		{"two includes keep response order", "/sa-corpus-31d2e98d5a/_search?filter_path=hits.hits._id%2Chits.max_score",
			200, search, `{"hits":{"max_score":0.3743781,"hits":[{"_id":"1"},{"_id":"3"}]}}`},
		{"took first", "/sa-corpus-31d2e98d5a/_search?filter_path=hits.total%2Ctook",
			200, search, `{"took":1,"hits":{"total":{"value":2,"relation":"eq"}}}`},
		{"exclude source took shards", "/sa-corpus-31d2e98d5a/_search?filter_path=-hits.hits._source%2C-took%2C-_shards",
			200, search, `{"timed_out":false,"hits":{"total":{"value":2,"relation":"eq"},"max_score":0.3743781,"hits":[{"_index":"sa-corpus-31d2e98d5a","_id":"1","_score":0.3743781},{"_index":"sa-corpus-31d2e98d5a","_id":"3","_score":0.3743781}]}}`},
		{"bulk item wildcard", "/slopensearch-test-b0913f36/_bulk?filter_path=errors%2Citems.%2A.status",
			200, bulk, `{"errors":false,"items":[{"index":{"status":201}}]}`},
		{"top-level array", "/_cat/indices/c01-43077bc9-gr?format=json&h=index%2Cdocs.count&filter_path=index",
			200, `[{"index":"c01-43077bc9-gr","docs.count":"3"}]`, `[{"index":"c01-43077bc9-gr"}]`},
		// pretty
		{"not pretty by default", "/_cluster/health?filter_path=number_of_nodes",
			200, healthGreen, `{"number_of_nodes":1}`},
		{"bare pretty", "/_cluster/health?pretty=&filter_path=number_of_nodes",
			200, healthGreen, "{\n  \"number_of_nodes\" : 1\n}\n"},
		{"pretty with filter reformats source", "/slopensearch-test-3184cc83/_doc/1?pretty=true&filter_path=_source.a",
			200, getDoc, "{\n  \"_source\" : {\n    \"a\" : \"x\"\n  }\n}\n"},
		{"pretty keeps source bytes", "/slopensearch-test-e1c16c76/_doc/1?pretty=true",
			200, `{"_index":"slopensearch-test-e1c16c76","_id":"1","_version":1,"_seq_no":0,"_primary_term":1,"found":true,"_source":{"a":"x","n":1}}`,
			"{\n  \"_index\" : \"slopensearch-test-e1c16c76\",\n  \"_id\" : \"1\",\n  \"_version\" : 1,\n  \"_seq_no\" : 0,\n  \"_primary_term\" : 1,\n  \"found\" : true,\n  \"_source\":{\"a\":\"x\",\"n\":1}\n}\n"},
		{"pretty empty object", "/_remote/info?pretty=true", 200, `{}`, "{ }\n"},
		{"pretty array", "/_cat/indices/c01-43077bc9-gr?format=json&h=index&pretty=true",
			200, `[{"index":"c01-43077bc9-gr"}]`, "[\n  {\n    \"index\" : \"c01-43077bc9-gr\"\n  }\n]\n"},
		// yaml
		{"yaml quotes strings and reserved keys", "/slopensearch-test-542b9723/_doc/1?format=yaml&filter_path=_id%2Cfound%2C_source",
			200, getDoc, "---\n_id: \"1\"\nfound: true\n_source:\n  a: \"x\"\n  \"n\": 1\n"},
		{"yaml nested", "/?format=yaml&filter_path=version.number",
			200, `{"name":"645feb3a01bc","cluster_name":"opensearch","version":{"distribution":"opensearch","number":"3.9.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`,
			"---\nversion:\n  number: \"3.9.0\"\n"},
		{"yaml array of objects", "/_cat/indices/c01-43077bc9-gr?format=yaml&h=index%2Cpri",
			200, `[{"index":"c01-43077bc9-gr","pri":"2"}]`, "---\n- index: \"c01-43077bc9-gr\"\n  pri: \"2\"\n"},
		{"yaml health", "/_cluster/health?format=yaml",
			200, `{"cluster_name":"opensearch","status":"yellow","timed_out":false,"number_of_nodes":1,"number_of_data_nodes":1,"discovered_master":true,"discovered_cluster_manager":true,"active_primary_shards":12,"active_shards":12,"relocating_shards":0,"initializing_shards":1,"unassigned_shards":2,"delayed_unassigned_shards":0,"number_of_pending_tasks":4,"number_of_in_flight_fetch":0,"task_max_waiting_in_queue_millis":106,"active_shards_percent_as_number":80.0}`,
			"---\ncluster_name: \"opensearch\"\nstatus: \"yellow\"\ntimed_out: false\nnumber_of_nodes: 1\nnumber_of_data_nodes: 1\ndiscovered_master: true\ndiscovered_cluster_manager: true\nactive_primary_shards: 12\nactive_shards: 12\nrelocating_shards: 0\ninitializing_shards: 1\nunassigned_shards: 2\ndelayed_unassigned_shards: 0\nnumber_of_pending_tasks: 4\nnumber_of_in_flight_fetch: 0\ntask_max_waiting_in_queue_millis: 106\nactive_shards_percent_as_number: 80.0\n"},
		// human
		{"human index stats sizes", "/slopensearch-test-18bd9acb/_stats/store,docs?filter_path=_all.primaries.docs%2C_all.primaries.store.size%2A&human=true",
			200, `{"_all":{"primaries":{"docs":{"count":1,"deleted":0},"store":{"size_in_bytes":4834}}}}`,
			`{"_all":{"primaries":{"docs":{"count":1,"deleted":0},"store":{"size":"4.7kb","size_in_bytes":4834}}}}`},
		{"no human no size", "/slopensearch-test-18bd9acb/_stats/store,docs?filter_path=_all.primaries.docs%2C_all.primaries.store.size%2A",
			200, `{"_all":{"primaries":{"docs":{"count":1,"deleted":0},"store":{"size_in_bytes":4834}}}}`,
			`{"_all":{"primaries":{"docs":{"count":1,"deleted":0},"store":{"size_in_bytes":4834}}}}`},
		{"human index settings", "/slopensearch-test-abb032d8/_settings?filter_path=%2A.settings.index.creation_date%2A%2C%2A.settings.index.version&human=true",
			200, `{"slopensearch-test-abb032d8":{"settings":{"index":{"creation_date":"1794490127429","version":{"created":"137307827"}}}}}`,
			`{"slopensearch-test-abb032d8":{"settings":{"index":{"creation_date_string":"2026-11-12T13:28:47.429Z","creation_date":"1794490127429","version":{"created_string":"3.9.0","created":"137307827"}}}}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, c.target, nil)
			h := http.Header{}
			got := string(Process(h, false, r, c.status, []byte(c.body), "3.9.0"))
			if got != c.want {
				t.Fatalf("Process(%s)\n got: %q\nwant: %q", c.target, got, c.want)
			}
			wantYAML := r.URL.Query().Get("format") == "yaml"
			if ct := h.Get("Content-Type"); (ct == "application/yaml") != wantYAML {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}
}

func TestHumanBytes(t *testing.T) {
	// test_human_on_index_stats (4834 -> 4.7kb), test_human_flag_on_nodes (1073741824 -> 1gb)
	for n, want := range map[int64]string{4834: "4.7kb", 1073741824: "1gb"} {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestYAMLPlainMaps(t *testing.T) {
	// test_cat_format_yaml
	got := string(YAML([]any{map[string]any{"index": "c01-43077bc9-gr", "pri": "2"}}))
	if want := "---\n- index: \"c01-43077bc9-gr\"\n  pri: \"2\"\n"; got != want {
		t.Fatalf("YAML\n got: %q\nwant: %q", got, want)
	}
}
