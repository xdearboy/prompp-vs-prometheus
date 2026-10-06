#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

RUNS="${RUNS:-12}"
PERIOD_HOURS="${PERIOD_HOURS:-4}"
: "${BENCH_NODE:?BENCH_NODE must be set}"

period=$((PERIOD_HOURS * 3600))
next=$(date +%s)

for i in $(seq "${RUNS}"); do
  now=$(date +%s)
  if ((next > now)); then
    sleep $((next - now))
  fi
  next=$((next + period))

  export RUN_ID
  RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
  printf '\n== run %d of %d, %s\n' "${i}" "${RUNS}" "${RUN_ID}"
  scripts/run.sh > "bench-${RUN_ID}.log" 2>&1 || printf 'run %s failed, see bench-%s.log\n' "${RUN_ID}" "${RUN_ID}"
done

${AGGREGATE_CMD:-go run ./cmd/aggregate} -root results
