**English** · [Русский](AGGREGATE.ru.md)

# Aggregate of 12 runs

Each cell is the median over runs with the minimum and maximum, and the spread as the share of the median.

## RSS at the last step

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 254.9 MiB | 249.9 MiB to 259.2 MiB | 3.6% |
| Prometheus 3.15.0 | 12 | 1.11 GiB | 1.11 GiB to 1.13 GiB | 1.5% |
| Prometheus 2.55.1 | 12 | 1.09 GiB | 1.07 GiB to 1.13 GiB | 5.4% |

## Working set at the last step

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 175.8 MiB | 169.7 MiB to 191.1 MiB | 12.2% |
| Prometheus 3.15.0 | 12 | 1.06 GiB | 1.04 GiB to 1.06 GiB | 2.1% |
| Prometheus 2.55.1 | 12 | 1.03 GiB | 1.00 GiB to 1.07 GiB | 6.1% |

## Ingest cpu cores, average

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 0.05 | 0.05 to 0.05 | 6.2% |
| Prometheus 3.15.0 | 12 | 0.19 | 0.18 to 0.20 | 7.7% |
| Prometheus 2.55.1 | 12 | 0.23 | 0.23 to 0.24 | 5.6% |

## Data directory

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 280.9 MiB | 279.5 MiB to 281.0 MiB | 0.5% |
| Prometheus 3.15.0 | 12 | 303.3 MiB | 302.3 MiB to 303.9 MiB | 0.5% |
| Prometheus 2.55.1 | 12 | 302.7 MiB | 298.0 MiB to 304.4 MiB | 2.1% |

## Query geomean p50, concurrency 1

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 298 ms | 296 ms to 300 ms | 1.4% |
| Prometheus 3.15.0 | 12 | 348 ms | 345 ms to 351 ms | 1.8% |
| Prometheus 2.55.1 | 12 | 345 ms | 343 ms to 348 ms | 1.5% |

## Query geomean p50, concurrency 4

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 682 ms | 671 ms to 693 ms | 3.2% |
| Prometheus 3.15.0 | 12 | 828 ms | 817 ms to 848 ms | 3.8% |
| Prometheus 2.55.1 | 12 | 839 ms | 829 ms to 858 ms | 3.5% |

## Query geomean p50, concurrency 16

| engine | runs | median | min to max | spread |
|---|---|---|---|---|
| Prom++ 0.8.15 | 12 | 2.53 s | 2.50 s to 2.56 s | 2.5% |
| Prometheus 3.15.0 | 12 | 3.35 s | 3.30 s to 3.39 s | 2.9% |
| Prometheus 2.55.1 | 12 | 3.46 s | 3.40 s to 3.52 s | 3.6% |

