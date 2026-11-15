package model

import (
	"math/bits"

	"slopensearch/internal/murmur3"
	"slopensearch/internal/settings"
)

// Shard routing as OpenSearch's OperationRouting computes it: Murmur3 of the
// routing value (the _id without one), plus the partition offset of the _id
// on a partitioned index, floorMod the number of routing shards, divided by
// the routing factor. Shards are not stored: they follow from the _id and
// the stored routing value.

// RoutingNumShards is index.number_of_routing_shards, by default the largest
// number_of_shards * 2^x <= 1024 (at least one split).
func (idx *Index) RoutingNumShards() int {
	n := idx.NumShards()
	if v, ok := settings.Int(idx.Settings, "number_of_routing_shards"); ok && v >= n && v > 0 {
		return v
	}
	return DefaultRoutingNumShards(n)
}

// DefaultRoutingNumShards is MetadataCreateIndexService.calculateNumRoutingShards.
func DefaultRoutingNumShards(numShards int) int {
	if numShards < 1 {
		return 1
	}
	log2NumShards := 32 - bits.LeadingZeros32(uint32(numShards-1))
	splits := max(10-log2NumShards, 1)
	return numShards << splits
}

// RoutingPartitionSize is index.routing_partition_size (1: not partitioned).
func (idx *Index) RoutingPartitionSize() int {
	if v, ok := settings.Int(idx.Settings, "routing_partition_size"); ok && v > 1 {
		return v
	}
	return 1
}

func floorMod(a, n int64) int64 {
	return ((a % n) + n) % n
}

// shardForRouting is calculateScaledShardId.
func (idx *Index) shardForRouting(routing string, partitionOffset int) int {
	n := idx.NumShards()
	if n <= 1 {
		return 0
	}
	rns := idx.RoutingNumShards()
	factor := rns / n
	h := int64(murmur3.HashRouting(routing)) + int64(partitionOffset)
	// Java adds in int arithmetic
	h = int64(int32(h))
	return int(floorMod(h, int64(rns))) / factor
}

// ShardFor is the shard of a document with the given _id and routing value
// (generateShardId).
func (idx *Index) ShardFor(id, routing string) int {
	effective := routing
	if effective == "" {
		effective = id
	}
	offset := 0
	if p := idx.RoutingPartitionSize(); p > 1 {
		offset = int(floorMod(int64(murmur3.HashRouting(id)), int64(p)))
	}
	return idx.shardForRouting(effective, offset)
}

// DocShard is the shard that holds document id.
func (idx *Index) DocShard(id string) int {
	return idx.ShardFor(id, idx.Routing[id])
}

// RoutingShards lists the shards a search with this routing value reaches:
// one per partition offset.
func (idx *Index) RoutingShards(routing string) []int {
	var out []int
	seen := map[int]bool{}
	for off := 0; off < idx.RoutingPartitionSize(); off++ {
		if s := idx.shardForRouting(routing, off); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
