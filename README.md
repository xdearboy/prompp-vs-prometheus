# Prom++ versus Prometheus

[![ci](https://github.com/xdearboy/prompp-vs-prometheus/actions/workflows/ci.yml/badge.svg)](https://github.com/xdearboy/prompp-vs-prometheus/actions/workflows/ci.yml)

A reproducible benchmark of Deckhouse Prom++ 0.8.15 against Prometheus 3.15.0 and
2.55.1. One engine at a time, single replica, pinned to one node with the same
limits (2 cpu, 6 GiB), the same Go runtime settings and the same synthetic data.

## Result

500 000 active series, 27.4 million samples per engine, 38 PromQL queries.
Full report with every table: [runs/series-20261006/20261006T210616Z/REPORT.md](runs/series-20261006/20261006T210616Z/REPORT.md).

![compared with Prometheus 3.15.0](runs/series-20261006/20261006T210616Z/charts/summary.svg)

Median of 12 full runs, one every four hours for two days, 500k series at the end of
the ramp. The spread between the fastest and the slowest run stays under 8 percent
for everything except the working set of Prom++ (12 percent), see
[runs/series-20261006/AGGREGATE.md](runs/series-20261006/AGGREGATE.md).

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

Memory is where the engines differ, queries are 15 to 25 percent faster and disk is
a wash. 33 of 38 queries return byte identical results on all three engines. The five
that differ are rate and `_over_time` range queries: Prom++ 0.8.15 evaluates range
selectors left-open like Prometheus 3.x, 2.55.1 includes the sample on the window
start. The charts are from the first run of the series, every run is in
[runs/series-20261006](runs/series-20261006).

![latency under concurrency](runs/series-20261006/20261006T210616Z/charts/latency-scaling.svg)

![resident memory](runs/series-20261006/20261006T210616Z/charts/rss-by-series.svg)

![resident memory during ingest](runs/series-20261006/20261006T210616Z/charts/rss-timeline.svg)

Read [METHODOLOGY.md](METHODOLOGY.md) before quoting these numbers: one node, short
runs and synthetic data are real limits.

## Engines

| name | image | upstream base | notes |
|---|---|---|---|
| `prompp-0815` | `mirror.gcr.io/prompp/prompp:0.8.15` | Prometheus 2.55.1 | C++ head and WAL |
| `prom-3150` | `quay.io/prometheus/prometheus:v3.15.0` | current upstream | |
| `prom-2551` | `quay.io/prometheus/prometheus:v2.55.1` | base of Prom++ 0.8.x | |

All three get the same four flags and the same Go runtime pinning through the
environment: `GOMEMLIMIT=5529MiB` (90% of the 6Gi limit) and `GOMAXPROCS=2`
(the cpu limit). Left to their defaults the engines would not be comparable:
Prom++ 0.8.15 and Prometheus 2.55.1 ship `auto-gomemlimit` and `auto-gomaxprocs`
as opt-in feature flags, while Prometheus 3.x enables both. The report shows the
values each engine actually reported through `/api/v1/status/runtimeinfo`, so
the pinning is verified rather than assumed. `internal/engines` holds the
registry and a test keeps the manifests in sync with it.

## Layout

```
tools/          submodule, github.com/xdearboy/prom-loadgen: loadgen, querybench,
                collector, series generator, remote write client, artifact schema
cmd/meta        run metadata
cmd/report      markdown and svg report of one run
cmd/aggregate   median and spread over many runs
internal/       engine registry, report and chart generator
deploy/         kustomize manifests for the three engines, runner.yaml for the in-cluster runner
scripts/        harness, run, series, runner, teardown, preflight, node overlay
runs/           published results
  series-20261006/    twelve repeated runs and their aggregate, the first run (20261006T210616Z)
                      keeps the raw resource samples, the others only the summaries
```

Clone with `git clone --recurse-submodules`, or run `git submodule update --init`.

## Requirements

- `kubectl` with kustomize v5 and a cluster context you can write to
- Go 1.24 or newer
- a node you are willing to pin the benchmark to
- enough free node disk for a `30Gi` emptyDir data volume

No container build tooling is needed. The tools are cross compiled to
`linux/amd64` and copied into a harness pod with `kubectl cp`.

## Running

```sh
export BENCH_NODE=<node>

RUN_ID=$(date -u +%Y%m%dT%H%M%SZ) scripts/node-overlay.sh
scripts/harness.sh
RUN_ID="$RUN_ID" scripts/run.sh
scripts/teardown.sh --yes
```

`node-overlay.sh` renders the kustomize overlay that pins every engine to
`$BENCH_NODE`. The overlay lives in `overlays/local/`, which is gitignored, so the
node name of a real cluster never lands in the repository. The committed manifests
keep a `bench-node-placeholder` value and `scripts/preflight.sh` fails if a
placeholder ever reaches a rendered manifest.

### Repeated runs

One run says little about variance. `scripts/series.sh` starts a run every
`PERIOD_HOURS` (default 4) for `RUNS` times (default 12, two days) and writes the
median, minimum and maximum over all finished runs to `results/AGGREGATE.md`.
A run takes about 2.5 hours, so the period must stay above that.

`scripts/runner.sh` starts the whole series inside the cluster, in a small pod that
holds the scripts and talks to the API server through its own service account, so
the machine that launched it can be switched off:

```sh
export BENCH_NODE=<the node>
scripts/harness.sh
scripts/runner.sh
kubectl -n prompp-bench exec bench-runner -- tail /work/series.log
kubectl cp prompp-bench/bench-runner:/work/results ./results
```

### Knobs

| variable | default | meaning |
|---|---|---|
| `BENCH_NODE` | required | node the engines and the harness are pinned to |
| `BENCH_NAMESPACE` | `prompp-bench` | benchmark namespace |
| `ENGINES` | all three | space separated engine names |
| `RAMP` | `50000:300,200000:480,500000:600` | `series:seconds` steps |
| `SEED` | `20261005` | series generation seed |
| `INTERVAL` | `15s` | sample interval |
| `CONCURRENCIES` | `1 4 16` | parallel requests of the same query |
| `SUITES` | `heavy` | query suites, `heavy` is a superset of `core` |
| `PER_QUERY` | `10s` | time budget per query and concurrency level |
| `MIN_RUNS` | `5` | measured requests per query even past the budget |
| `WARMUP` | `1` | unmeasured requests per query before measuring |
| `EPOCH_MS` | `1767225600000` | pinned first sample timestamp |

## Outputs

```
results/<run-id>/
  run.json                        node, engine and harness metadata
  <engine>/ingest.json            per step throughput, latency and head stats
  <engine>/samples.ingest.jsonl   resource samples during ingest
  <engine>/samples.query-<suite>-c<n>.jsonl  resource samples per query run
  <engine>/query-<suite>-c<n>.json
  <engine>/dump-<suite>.json      sha256 of every query result
  <engine>/disk.jsonl             data dir, WAL and block sizes per step
  REPORT.md                       generated
  charts/*.svg                    generated
```

`run.sh` stops with an error if any artifact the report needs is missing or
empty after it is copied out of the harness pod, and it replaces the node name
with `bench-node` in every artifact so a run directory can be published as is.

## Reading the report

The generator compares memory and cpu per active series, ingest throughput,
query latency per query and concurrency level, and result equality. Each query
is measured on its own with repeated requests, the tables show p50, p99 and the
number of measured requests, and the summaries use geometric means so a few
multi second range queries do not dominate. Result
equality is the guard against benchmarking engines that quietly dropped data:
the same pinned dataset is ingested into every engine and every query result is
hashed, so any digest difference shows up as a mismatch.

See METHODOLOGY.md for the fairness rules and the known threats to validity.