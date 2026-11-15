"""Client-side tolerance patches for opensearch-benchmark (OSB), loaded into every Python
process of bench/.venv-osb through slop_osb_compat.pth when SLOP_OSB_COMPAT=1.

OSB aborts the whole run (not just one task) when a telemetry response lacks a field it
expects. The servers are not changed for the benchmark; instead these patches make OSB
treat missing statistics as zero, and every patch is a compatibility gap listed in
bench/README.md. They do not change what OSB sends or measures, and they are no-ops
against the reference OpenSearch (all fields present).

Installed by `make bench-osb-setup` (bench/osb_setup.sh).
"""

import os

PATCHES = []


def _apply():
    from osbenchmark import telemetry

    # 1. _nodes/stats: jvm.gc.collectors.{young,old}.collection_time_in_millis and
    #    jvm.mem.pools.*.peak_used_in_bytes are read unconditionally at benchmark start
    orig_collector_stats = telemetry.JvmStatsSummary.collector_stats

    def collector_stats(self, node_name, collectors, names):
        r = orig_collector_stats(self, node_name, collectors, names)
        return {"collection_time_in_millis": 0, "collection_count": 0, **(r or {})}

    telemetry.JvmStatsSummary.collector_stats = collector_stats
    orig_jvm_stats = telemetry.JvmStatsSummary.jvm_stats

    def jvm_stats(self):
        try:
            return orig_jvm_stats(self)
        except (KeyError, TypeError) as e:
            self.logger.warning("slop_osb_compat: incomplete _nodes/stats jvm section (%r), using zeros", e)
            out = {}
            stats = self.client.nodes.stats(metric="_all")
            for node in stats.get("nodes", {}).values():
                out[node.get("name", "?")] = {"young_gc_time": 0, "young_gc_count": 0, "old_gc_time": 0,
                                              "old_gc_count": 0, "pools": {}}
            return out

    telemetry.JvmStatsSummary.jvm_stats = jvm_stats
    PATCHES.append("JvmStatsSummary: missing jvm.gc collection_time_in_millis / mem.pools tolerated")

    # 2. the summary devices read _stats / _nodes/stats sections and fail on a missing key
    for cls_name in ("SegmentStats", "NodeStatsSummary", "IndexStats", "ClusterEnvironmentInfo",
                     "ExternalEnvironmentInfo", "MlBucketProcessingTime", "IngestPipelineStats"):
        cls = getattr(telemetry, cls_name, None)
        if cls is None:
            continue
        for meth in ("on_benchmark_start", "on_benchmark_stop"):
            f = cls.__dict__.get(meth)
            if f is None:
                continue

            def wrap(f=f, cls_name=cls_name, meth=meth):
                def g(self, *a, **k):
                    try:
                        return f(self, *a, **k)
                    except (KeyError, TypeError, AttributeError, IndexError) as e:
                        self.logger.warning("slop_osb_compat: %s.%s failed on the response (%r); skipped",
                                            cls_name, meth, e)
                        return None
                return g

            setattr(cls, meth, wrap())
    PATCHES.append("summary telemetry devices: KeyError on missing stats fields logged and skipped")


if os.environ.get("SLOP_OSB_COMPAT") == "1":
    try:
        _apply()
    except ImportError:
        pass
