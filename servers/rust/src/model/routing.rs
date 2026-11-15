//! Shard routing as OpenSearch's OperationRouting computes it: Murmur3 of the
//! routing value (the _id without one), plus the partition offset of the _id
//! on a partitioned index, floorMod the number of routing shards, divided by
//! the routing factor. Shards are not stored: they follow from the _id and
//! the stored routing value.

use super::Index;
use crate::murmur3::hash_routing;
use crate::settings::setting_int;

/// MetadataCreateIndexService.calculateNumRoutingShards: the largest
/// number_of_shards * 2^x <= 1024 (at least one split).
pub fn default_routing_num_shards(num_shards: i64) -> i64 {
    if num_shards < 1 {
        return 1;
    }
    let log2 = 64 - ((num_shards - 1) as u64).leading_zeros() as i64;
    let splits = (10 - log2).max(1);
    num_shards << splits
}

impl Index {
    /// index.number_of_routing_shards, by default [`default_routing_num_shards`].
    pub fn routing_num_shards(&self) -> i64 {
        let n = self.num_shards();
        match setting_int(&self.settings, "number_of_routing_shards") {
            Some(v) if v >= n && v > 0 => v,
            _ => default_routing_num_shards(n),
        }
    }

    /// index.routing_partition_size (1: not partitioned).
    pub fn routing_partition_size(&self) -> i64 {
        match setting_int(&self.settings, "routing_partition_size") {
            Some(v) if v > 1 => v,
            _ => 1,
        }
    }

    /// calculateScaledShardId.
    fn shard_for_routing(&self, routing: &str, partition_offset: i64) -> i64 {
        let n = self.num_shards();
        if n <= 1 {
            return 0;
        }
        let rns = self.routing_num_shards();
        let factor = (rns / n).max(1);
        // Java adds in int arithmetic
        let h = (hash_routing(routing) as i64 + partition_offset) as i32 as i64;
        h.rem_euclid(rns) / factor
    }

    /// The shard of a document with the given _id and routing value
    /// (generateShardId); an empty routing means none.
    pub fn shard_for(&self, id: &str, routing: &str) -> i64 {
        let effective = if routing.is_empty() { id } else { routing };
        let p = self.routing_partition_size();
        let offset = if p > 1 { (hash_routing(id) as i64).rem_euclid(p) } else { 0 };
        self.shard_for_routing(effective, offset)
    }

    /// The shard that holds document id.
    pub fn doc_shard(&self, id: &str) -> i64 {
        self.shard_for(id, self.routing.get(id).map(|s| s.as_str()).unwrap_or(""))
    }

    /// The shards a search with this routing value reaches: one per
    /// partition offset.
    pub fn routing_shards(&self, routing: &str) -> Vec<i64> {
        let mut out = Vec::new();
        for off in 0..self.routing_partition_size() {
            let s = self.shard_for_routing(routing, off);
            if !out.contains(&s) {
                out.push(s);
            }
        }
        out
    }

    /// Documents per shard.
    pub fn docs_per_shard(&self) -> Vec<usize> {
        let mut out = vec![0usize; self.num_shards().max(0) as usize];
        for id in self.docs.keys() {
            if let Some(c) = out.get_mut(self.doc_shard(id) as usize) {
                *c += 1;
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn routing_num_shards() {
        assert_eq!(default_routing_num_shards(1), 1024);
        assert_eq!(default_routing_num_shards(2), 1024);
        assert_eq!(default_routing_num_shards(3), 768);
        assert_eq!(default_routing_num_shards(5), 640);
        assert_eq!(default_routing_num_shards(1024), 2048);
    }
}
