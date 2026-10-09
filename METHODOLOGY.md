**English** · [Русский](METHODOLOGY.ru.md)

# Methodology

## What is compared

Three engines, one at a time, on one node:

- Deckhouse Prom++ 0.8.15, a Prometheus fork with a C++ head and WAL and a PromQL
  engine that uses 3.x range selector semantics
- Prometheus 3.15.0
- Prometheus 2.55.1

Each runs as a single-replica StatefulSet with a 30 GiB `emptyDir` data volume in
the benchmark namespace. No replication, no `ServiceMonitor`, no connection to the
cluster monitoring. The engine scrapes nothing, it only receives remote write.

## Fairness rules

1. **One engine at a time.** They never compete for cpu or memory bandwidth.
2. **Same input.** Same seed, label layout, series counts and sample timestamps.
3. **Same configuration.** Same config file, same flags, same `2000m` / `6Gi` limits.
4. **Same Go runtime.** Every engine gets `GOMEMLIMIT=5529MiB` and `GOMAXPROCS=2`
   through the environment. Their defaults differ: Prom++ 0.8.15 and Prometheus
   2.55.1 keep `auto-gomemlimit` and `auto-gomaxprocs` behind opt-in feature flags,
   Prometheus 3.x enables both. Without pinning, the 2.x based engines would run with
   no memory limit and, if built with Go older than 1.25, with one Go processor per
   core of the node under a two core quota. What each engine reports through
   `/api/v1/status/runtimeinfo` is stored with every resource sample and shown in
   the report.
5. **Fresh state.** The StatefulSet and its volume are deleted between engines, so no
   engine inherits another one's WAL or blocks.
6. **Same queries.** The same PromQL suite at the same concurrency levels, evaluated
   at the same timestamp on data of the same size and shape.
7. **Pinned node.** Every object has `nodeName`, so the kernel, page cache and cgroup
   hierarchy are the same for all engines.

## Phases

| phase | what happens |
|---|---|
| ingest | ramp the active series through 50k, 200k and 500k, hold each step |
| settle | 30 seconds of pause to read head stats and disk usage |
| query | run every query of the suite at each concurrency level |
| dump | hash every query result for the equality check |

## What is measured

- **Ingest throughput.** Samples per second actually achieved in each step.
- **Request latency.** p50 and p99 per remote write request.
- **Head stats.** Series, chunks and label pairs from the engine's own API.
- **Memory.** `process_resident_memory_bytes` from the engine and the cgroup working
  set from cAdvisor through the kubelet proxy.
- **Cpu.** Rate of the cAdvisor container cpu counter, `process_cpu_seconds_total`
  as a fallback, sampled every two seconds.
- **Disk.** Data directory, WAL and block sizes from a sidecar that mounts the same
  volume, every 15 seconds. A step gets the last sample taken inside it.
- **Query latency.** Each query on its own: one unmeasured warmup request, then `c`
  workers repeat the query until the time budget is spent and at least the minimum
  number of requests finished. The report shows p50, p99 and the number of requests per
  query, and geometric means over all queries, so a few range queries that take
  seconds do not dominate. A separate resource collector runs for every suite and
  concurrency level, so cpu and memory belong to that run only.
- **Result equality.** sha256 of the canonicalised result of each query, per engine.

Per series figures divide the last sample of a step by the active series of that step,
which is the number that scales when a fleet grows.

## Repeated runs

A single run cannot show variance. The published results are 12 full runs started
every four hours for two days, from a pod inside the cluster. The aggregate takes the
median of every figure and shows the minimum, the maximum and the spread as a share of
the median. The fastest and the slowest run differ by less than 8 percent everywhere
except the working set of Prom++ (12 percent), and the gaps between the engines are
several times larger than that.

## Threats to validity

- **Shared node.** The node also runs control plane components. Running one engine at
  a time limits that interference but does not remove it. It hits every engine about
  equally and adds variance. The repeated runs at different hours of the day exist to
  show how large it is.
- **WAL is not like for like.** Prom++ has its own WAL format, the upstream engines
  use their default snappy compression. Disk and WAL figures compare what a user gets
  by default, not equal encodings.
- **`GOMEMLIMIT` covers the Go heap only.** Prom++ keeps most of its head in C++
  memory managed by jemalloc, which the Go limit does not govern. The limit is pinned
  so that the Go behaviour is equal, not to cap total memory.
- **Single node, single replica.** Replication cost is not measured.
- **Short steps.** A ramp step lasts minutes, not hours, so compaction, mmap reuse and
  long-tail allocator behaviour are only partly exercised.
- **Few requests for slow queries.** A range query that takes seconds gets only the
  minimum number of requests. Its p99 is close to the maximum, compare the median.
- **Range selector semantics.** Prometheus 3.0 made range selectors left-open. On five
  `rate` and `_over_time` queries Prom++ 0.8.15 and Prometheus 3.15.0 agree with each
  other and differ from 2.55.1, because 2.55.1 counts the sample that sits exactly on
  the window start. The report states this next to the equality table. In
  `promql/engine.go` Prom++ drops samples with `T <= mint`, 2.55.1 with `T < mint`.
- **Synthetic data.** The generator gives realistic label cardinality and a mix of
  gauges and counters, but it is not a production scrape. Query plans behave
  differently on real label sets.

## Correctness

Throughput figures mean nothing if an engine dropped samples. The dataset is pinned to
a fixed epoch, every engine receives the same bytes, and every query result is
canonicalised and hashed. A different digest is reported as a mismatch and never
averaged away.

The harness also refuses to produce a report from incomplete data: after each engine
the artifacts are copied out of the harness pod, and `run.sh` stops if a file the report
needs is missing or empty.

## Reproducibility

The harness writes the node description, the exact images, the flag list and the
declared environment to `results/<run-id>/run.json`. Timestamps, seed and ramp are
explicit inputs, so two runs with the same inputs ingest the same samples into the
same query windows.
