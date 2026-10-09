**English** · [Русский](README.ru.md)

<img src="assets/banner.svg" alt="Prom++ vs Prometheus" width="100%">

[![ci](https://github.com/xdearboy/prompp-vs-prometheus/actions/workflows/ci.yml/badge.svg)](https://github.com/xdearboy/prompp-vs-prometheus/actions/workflows/ci.yml)

A benchmark of Deckhouse Prom++ 0.8.15 against Prometheus 3.15.0 and 2.55.1. The
engines run one at a time, single replica, on the same node with the same limits
(2 cpu, 6 GiB), the same Go runtime settings and the same synthetic data.

## Results

500 000 active series, 27.4 million samples per engine, 38 PromQL queries. The
numbers are the median of 12 full runs, one every four hours for two days. Between
the fastest and the slowest run the spread stays under 8 percent, except for the
working set of Prom++ (12 percent). Details: [AGGREGATE.md](runs/series-20261006/AGGREGATE.md).

![compared with Prometheus 3.15.0](runs/series-20261006/20261006T210616Z/charts/summary.svg)

| at 500k series | Prom++ 0.8.15 | Prometheus 3.15.0 | Prometheus 2.55.1 |
|---|---|---|---|
| resident memory | **255 MiB** | 1.11 GiB | 1.09 GiB |
| cgroup working set | **176 MiB** | 1.06 GiB | 1.03 GiB |
| cpu while ingesting | **0.05 cores** | 0.19 | 0.23 |
| data directory | 281 MiB | 303 MiB | 303 MiB |

Query latency, geometric mean of the median over all queries:

| concurrency | Prom++ 0.8.15 | Prometheus 3.15.0 | Prometheus 2.55.1 |
|---|---|---|---|
| 1 | **298 ms** | 348 ms | 345 ms |
| 4 | **682 ms** | 828 ms | 839 ms |
| 16 | **2.53 s** | 3.35 s | 3.46 s |

Memory is the big difference. Queries are 15 to 25 percent faster, disk is about
the same. 33 of 38 queries return byte identical results on all three engines. The
other five are `rate` and `_over_time` range queries: Prom++ 0.8.15 treats range
selectors as left-open like Prometheus 3.x, while 2.55.1 counts the sample that sits
exactly on the window start.

![latency under concurrency](runs/series-20261006/20261006T210616Z/charts/latency-scaling.svg)

![resident memory during ingest](runs/series-20261006/20261006T210616Z/charts/rss-timeline.svg)

The charts come from the first run of the series, the full report of that run is
[REPORT.md](runs/series-20261006/20261006T210616Z/REPORT.md). Read
[METHODOLOGY.md](METHODOLOGY.md) before quoting the numbers: one node, short runs
and synthetic data are real limits.

## Engines

| name | image | notes |
|---|---|---|
| `prompp-0815` | `mirror.gcr.io/prompp/prompp:0.8.15` | C++ head and WAL, range selectors like 3.x |
| `prom-3150` | `quay.io/prometheus/prometheus:v3.15.0` | current upstream |
| `prom-2551` | `quay.io/prometheus/prometheus:v2.55.1` | closed range selectors |

The head is the part of the TSDB that lives in memory: fresh samples stay there until they
are flushed to disk as a block. It is what drives memory use while writing.

All three get the same flags and the same runtime settings through the environment:
`GOMEMLIMIT=5529MiB` (90% of the 6 GiB limit) and `GOMAXPROCS=2`. Without them the
engines would not be comparable: Prom++ 0.8.15 and Prometheus 2.55.1 keep
`auto-gomemlimit` and `auto-gomaxprocs` behind opt-in feature flags, Prometheus 3.x
turns both on. The report shows the values each engine reports through
`/api/v1/status/runtimeinfo`, so the pinning is checked, not assumed. The engine list
lives in `internal/engines`, a test keeps the manifests in sync with it.

## Layout

```text
tools/          submodule github.com/xdearboy/prom-loadgen: loadgen, querybench,
                collector, series generator, remote write client, result schema
cmd/meta        run metadata
cmd/report      markdown and svg report of one run
cmd/aggregate   median and spread over many runs
internal/       engine registry, report and chart generator
deploy/         kustomize manifests, runner.yaml for the in-cluster runner
scripts/        harness, run, series, runner, teardown, preflight, node overlay
runs/           published results
  series-20261006/   twelve runs and their aggregate; the first run keeps the raw
                     resource samples, the others only the summaries
```

Clone with `git clone --recurse-submodules` or run `git submodule update --init`.

## Requirements

- `kubectl` with kustomize v5 and a cluster context you can write to
- Go 1.24 or newer
- a node you can pin the benchmark to
- free disk on that node for a 30 GiB emptyDir

No image build is needed: the tools are cross compiled for linux/amd64 and copied
into a harness pod with `kubectl cp`.

## Running

```sh
export BENCH_NODE=<node>

RUN_ID=$(date -u +%Y%m%dT%H%M%SZ) scripts/node-overlay.sh
scripts/harness.sh
RUN_ID="$RUN_ID" scripts/run.sh
scripts/teardown.sh --yes
```

`node-overlay.sh` renders an overlay that pins every engine to `$BENCH_NODE`. It lives
in `overlays/local/`, which is gitignored, so the name of a real node never reaches
the repository. The committed manifests keep a `bench-node-placeholder`, and
`scripts/preflight.sh` fails if it survives into a rendered manifest.

### Repeated runs

One run says nothing about variance. `scripts/series.sh` starts a run every
`PERIOD_HOURS` (4 by default) `RUNS` times (12 by default) and writes the median,
minimum and maximum of all finished runs to `results/AGGREGATE.md`. A run takes about
2.5 hours, so keep the period above that.

`scripts/runner.sh` starts the whole series from a small pod inside the cluster, so
the machine that launched it can go offline:

```sh
export BENCH_NODE=<node>
scripts/harness.sh
scripts/runner.sh
kubectl -n prompp-bench exec bench-runner -- tail /work/series.log
kubectl cp prompp-bench/bench-runner:/work/results ./results
```

### Variables

| variable | default | meaning |
|---|---|---|
| `BENCH_NODE` | required | node for the engines and the harness |
| `BENCH_NAMESPACE` | `prompp-bench` | namespace |
| `ENGINES` | all three | space separated engine names |
| `RAMP` | `50000:300,200000:480,500000:600` | steps as `series:seconds` |
| `SEED` | `20261005` | series generation seed |
| `INTERVAL` | `15s` | sample interval |
| `CONCURRENCIES` | `1 4 16` | parallel requests of the same query |
| `SUITES` | `heavy` | query suites, `heavy` includes `core` |
| `PER_QUERY` | `10s` | time budget per query and concurrency |
| `MIN_RUNS` | `5` | measured requests per query even past the budget |
| `WARMUP` | `1` | unmeasured requests before measuring |
| `EPOCH_MS` | `1767225600000` | timestamp of the first sample |

## Output of a run

```text
results/<run-id>/
  run.json                                   node, engines, harness version
  <engine>/ingest.json                       throughput, latency, head stats per step
  <engine>/samples.ingest.jsonl              resource samples during ingest
  <engine>/samples.query-<suite>-c<n>.jsonl  resource samples per query run
  <engine>/query-<suite>-c<n>.json
  <engine>/dump-<suite>.json                 sha256 of every query result
  <engine>/disk.jsonl                        data dir, WAL and blocks over time
  REPORT.md, charts/*.svg                    generated
```

`run.sh` stops if an artifact the report needs is missing or empty after the copy out
of the pod, and it replaces the node name with `bench-node` everywhere, so a run
directory can be published as it is.

## What the report compares

Memory and cpu per active series, ingest throughput, latency of every query at every
concurrency, and result equality. Each query is measured on its own: one warmup
request, then the workers repeat it until the time budget is spent. The tables give
p50, p99 and the number of measured requests; summaries use geometric means, so a few
range queries that take seconds do not outweigh the rest. Equality guards against
comparing engines that silently dropped data: every engine gets the same pinned
dataset and every query result is hashed.

Fairness rules and threats to validity are in [METHODOLOGY.md](METHODOLOGY.md).
