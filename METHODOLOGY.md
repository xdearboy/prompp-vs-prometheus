# Methodology

## What is being compared

Three TSDB engines, one at a time, on one node:

- Deckhouse Prom++ 0.8.15, a fork of Prometheus 2.55.1 with a C++ head and WAL
- upstream Prometheus 3.15.0
- upstream Prometheus 2.55.1

Each engine runs as a single-replica StatefulSet with a size limited `30Gi`
`emptyDir` data volume in the benchmark namespace. There is no replication, no
`ServiceMonitor`, no `PodMonitor`, and no connection to the cluster monitoring
stack. The engine scrapes nothing, it only receives remote write.

## Fairness rules

1. **One engine at a time.** Engines never run concurrently, so they cannot compete
   for cpu or memory bandwidth.
2. **Identical inputs.** The same seed, the same label layout, the same series
   counts and the same pinned sample timestamps for every engine.
3. **Identical configuration.** Every engine gets the same config file, the same
   four command line flags and the same `2000m`/`6Gi` limits.
4. **Identical Go runtime.** Every engine gets `GOMEMLIMIT=5529MiB` and
   `GOMAXPROCS=2` through the environment. The defaults differ: Prom++ 0.8.15 and
   Prometheus 2.55.1 keep `auto-gomemlimit` and `auto-gomaxprocs` behind opt-in
   feature flags, Prometheus 3.x turns both on. Without pinning, the 2.x based
   engines would run without a Go memory limit and, when built with a Go older
   than 1.25, with one Go processor per node core under a two core quota. The values each engine reports through
   `/api/v1/status/runtimeinfo` are recorded in every resource sample and shown in
   the report.
5. **Fresh state.** The StatefulSet and its volume are deleted between engines, so
   no engine inherits another engine's WAL or blocks.
6. **Identical queries.** The same PromQL suite at the same concurrency levels,
   evaluated at the same pinned timestamp against a data set of the same size and
   shape.
7. **Node pinned.** Every object carries `nodeName`, so the kernel, the page cache
   and the cgroup hierarchy are the same for all engines.

## Phases

| phase | what happens |
|---|---|
| ingest | ramp active series through `50k`, `200k`, `500k` and hold each step |
| settle | a 30 second pause so head stats and disk usage can be read |
| query | measure every query of the suite at each concurrency level |
| dump | hash every query result for the equality check |

## Measurements

- **Ingest throughput.** Samples per second actually achieved per ramp step.
- **Request latency.** p50 and p99 per remote write request, and per sample.
- **Head stats.** Series, chunks and label pairs read from the engine's own API.
- **Memory.** `process_resident_memory_bytes` from the engine's self metrics, plus
  the cgroup working set from cAdvisor through the kubelet proxy.
- **Cpu.** Rate of the cAdvisor container cpu counter, falling back to
  `process_cpu_seconds_total`, sampled every two seconds.
- **Disk.** Data directory, WAL and block sizes from a sidecar that mounts the
  same volume, every 15 seconds, attributed to the step by the last sample in it.
- **Query latency.** Each query is measured on its own. One unmeasured warmup
  request, then `c` workers repeat the same query until the time budget is spent
  and at least the minimum number of requests finished. The report gives p50,
  p99 and the number of measured requests per query, and geometric means across
  queries so that a few multi second range queries do not dominate the summary.
  A separate resource collector runs for every suite and concurrency level, so
  cpu and memory are attributed to that run only.
- **Result equality.** sha256 of the canonicalised query result per engine.

Per-series figures use the last sample of the step divided by the active series of
that step, which is the number that scales when a fleet grows.

## Threats to validity

- **Shared node.** The node runs control plane components. Sequential execution
  limits but does not eliminate interference from them. Background load inflates
  every engine roughly equally, but it adds variance.
- **WAL is not like for like.** Prom++ has its own WAL format. Upstream engines run
  with their default snappy WAL compression. Disk and WAL figures compare what a
  user gets by default, not an equal encoding.
- **GOMEMLIMIT covers the Go heap only.** Prom++ keeps most of its head in C++
  memory managed by jemalloc, which the Go limit does not govern. The limit is
  pinned for equal Go behaviour, not to cap total memory.
- **Single node, single replica.** No replication overhead is measured. A clustered
  or replicated deployment has a different cost profile.
- **Short runs.** Each ramp step is minutes rather than hours, so compaction,
  mmap reuse and long-tail allocator behaviour are only partially exercised.
- **Small samples for slow queries.** Range queries that take seconds get only the
  minimum number of measured requests. Their p99 is close to the maximum, compare
  the median.
- **Range selector semantics.** Prometheus 3.0 made range selectors left-open. Some
  subquery results legitimately differ between 2.x based engines and 3.x, and the
  report says so next to the equality table.
- **Synthetic data.** The generator produces realistic label cardinality and a
  gauge/counter mix, but it is not a production scrape. Query plans behave
  differently against real label sets.

## Correctness

Throughput numbers are meaningless if an engine dropped samples. The dataset is
pinned to a fixed epoch, every engine receives the same bytes, and every query
result is canonicalised and hashed. Any digest difference between engines is
reported as a mismatch rather than being averaged away.

The harness also refuses to produce a report from incomplete data. After each
engine the artifacts are copied out of the harness pod and `run.sh` stops if any
file the report needs is missing or empty.

## Reproducibility

The harness records the node description, the exact images, the compact flag list
and the declared environment into `results/<run-id>/run.json`. Timestamps, seed and
ramp are all explicit inputs, so two runs with the same inputs ingest the same
samples into the same query windows.
