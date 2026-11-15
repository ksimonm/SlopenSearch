package model

import "testing"

func TestDefaultRoutingNumShards(t *testing.T) {
	for n, want := range map[int]int{1: 1024, 2: 1024, 3: 768, 4: 1024, 5: 640, 6: 768, 1024: 2048} {
		if got := DefaultRoutingNumShards(n); got != want {
			t.Errorf("DefaultRoutingNumShards(%d) = %d, want %d", n, got, want)
		}
	}
}

// Shards from OpenSearch 3.9 (preference=_shards:N searches of indexed ids).
func TestShardFor(t *testing.T) {
	idx := func(settings map[string]any) *Index { return &Index{Settings: settings} }
	tests := []struct {
		settings    map[string]any
		id, routing string
		want        int
	}{
		{map[string]any{"number_of_shards": 2}, "1", "", 0},
		{map[string]any{"number_of_shards": 2}, "4", "", 1},
		{map[string]any{"number_of_shards": 2}, "😀", "", 1},
		{map[string]any{"number_of_shards": 2}, "rt-x", "x", 1},
		{map[string]any{"number_of_shards": 3}, "abc", "", 0},
		{map[string]any{"number_of_shards": 3}, "é", "", 1},
		{map[string]any{"number_of_shards": 3}, "a b", "", 2},
		{map[string]any{"number_of_shards": 5}, "4", "", 1},
		{map[string]any{"number_of_shards": 5}, "日本", "", 4},
		{map[string]any{"number_of_shards": 5}, "rt-😀", "😀", 2},
		{map[string]any{"number_of_shards": 4, "number_of_routing_shards": 8}, "10", "", 3},
		{map[string]any{"number_of_shards": 4, "number_of_routing_shards": 8}, "rt-y", "y", 0},
		{map[string]any{"number_of_shards": 6, "routing_partition_size": 2}, "abc", "r0", 1},
		{map[string]any{"number_of_shards": 6, "routing_partition_size": 2}, "1", "r1", 0},
		{map[string]any{"number_of_shards": 1}, "x", "", 0},
	}
	for _, tt := range tests {
		if got := idx(tt.settings).ShardFor(tt.id, tt.routing); got != tt.want {
			t.Errorf("%v ShardFor(%q, %q) = %d, want %d", tt.settings, tt.id, tt.routing, got, tt.want)
		}
	}
}
