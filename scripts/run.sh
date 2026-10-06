#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

NAMESPACE="${BENCH_NAMESPACE:-prompp-bench}"
POD="${BENCH_POD:-bench-harness}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
RESULTS_DIR="${RESULTS_DIR:-results}/${RUN_ID}"
ENGINES="${ENGINES:-prompp-0815 prom-3150 prom-2551}"
RAMP="${RAMP:-50000:300,200000:480,500000:600}"
SEED="${SEED:-20261005}"
INTERVAL="${INTERVAL:-15s}"
BATCH_SERIES="${BATCH_SERIES:-5000}"
WORKERS="${WORKERS:-4}"
CONCURRENCIES="${CONCURRENCIES:-1 4 16}"
SUITES="${SUITES:-heavy}"
PER_QUERY="${PER_QUERY:-10s}"
MIN_RUNS="${MIN_RUNS:-5}"
WARMUP="${WARMUP:-1}"
EPOCH_MS="${EPOCH_MS:-1767225600000}"
COLLECT_INTERVAL="${COLLECT_INTERVAL:-2s}"
DISK_INTERVAL="${DISK_INTERVAL:-15s}"
READY_TIMEOUT="${READY_TIMEOUT:-600s}"

: "${BENCH_NODE:?BENCH_NODE must be set}"

step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

ramp_total_ms() {
  awk -F, '{
    total = 0;
    for (i = 1; i <= NF; i++) {
      split($i, pair, ":");
      total += pair[2];
    }
    printf "%d", total * 1000;
  }' <<<"${RAMP}"
}

TOTAL_MS="$(ramp_total_ms)"
EVAL_MS=$((EPOCH_MS + TOTAL_MS))

mkdir -p "${RESULTS_DIR}"

meta_engines="${ENGINES// /,}"

step "run ${RUN_ID}"
printf 'node        %s\n' "${BENCH_NODE}"
printf 'engines     %s\n' "${ENGINES}"
printf 'ramp        %s\n' "${RAMP}"
printf 'interval    %s\n' "${INTERVAL}"
printf 'epoch ms    %s\n' "${EPOCH_MS}"
printf 'eval ms     %s\n' "${EVAL_MS}"
printf 'results     %s\n' "${RESULTS_DIR}"

exec_pod() { kubectl exec -n "${NAMESPACE}" "${POD}" -c harness -- "$@"; }

reset_engine() {
  local engine="$1"
  kubectl -n "${NAMESPACE}" exec "${POD}" -c harness -- killall -9 loadgen collector querybench 2>/dev/null || true
  kubectl -n "${NAMESPACE}" exec "${POD}" -c harness -- rm -rf "/results/${engine}" 2>/dev/null || true
  kubectl -n "${NAMESPACE}" delete statefulset "${engine}" --ignore-not-found --wait=true --timeout=300s
  kubectl -n "${NAMESPACE}" delete pod "${engine}-0" --ignore-not-found --wait=true --timeout=300s
  kubectl -n "${NAMESPACE}" wait --for=delete "pod/${engine}-0" --timeout=300s 2>/dev/null || true
  kubectl -n "${NAMESPACE}" delete pvc "data-${engine}-0" --ignore-not-found --wait=true --timeout=300s
}

disk_snapshot() {
  local engine="$1"
  local total_kb wal_kb blocks_kb raw
  raw="$(kubectl -n "${NAMESPACE}" exec "${engine}-0" -c diskprobe -- sh -c '
    total_kb=$(du -sk /data 2>/dev/null | cut -f1)
    wal_kb=$(du -sk /data/wal 2>/dev/null | cut -f1)
    blocks_kb=$(du -sk /data/01* 2>/dev/null | cut -f1 | awk "{s+=\$1} END {printf \"%d\", s}")
    printf "%s %s %s" "${total_kb:-0}" "${wal_kb:-0}" "${blocks_kb:-0}"
  ' 2>/dev/null || printf '0 0 0')"
  read -r total_kb wal_kb blocks_kb <<<"${raw}"
  printf ',"data_dir_bytes":%s,"wal_bytes":%s,"blocks_bytes":%s' \
    "$(( ${total_kb:-0} * 1024 ))" "$(( ${wal_kb:-0} * 1024 ))" "$(( ${blocks_kb:-0} * 1024 ))"
}

disk_watch() {
  local engine="$1"
  while true; do
    printf '{"ts":"%s","run_id":"%s","engine":"%s"%s}\n' \
      "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${RUN_ID}" "${engine}" \
      "$(disk_snapshot "${engine}")" >> "${RESULTS_DIR}/${engine}/disk.jsonl"
    sleep "${DISK_INTERVAL}"
  done
}

start_collector() {
  local engine="$1" phase="$2"
  shift 2
  local cmd
  cmd="$(printf '%q ' /tools/collector -engine "${engine}" -target "http://${engine}:9090" -namespace "${NAMESPACE}" \
    -node "${BENCH_NODE}" -run-id "${RUN_ID}" -phase "${phase}" -interval "${COLLECT_INTERVAL}" \
    -out "/results/${engine}/samples.${phase}.jsonl" "$@")"
  exec_pod sh -c "nohup ${cmd} >/dev/null 2>&1 &"
}

stop_collector() {
  exec_pod killall -TERM collector 2>/dev/null || true
  sleep 3
}

# long commands run detached in the pod and are polled, a dropped apiserver
# stream must not kill a 20 minute ingest
pod_run() {
  local name="$1"
  shift
  local cmd rc=""
  cmd="$(printf '%q ' "$@")"
  exec_pod sh -c "rm -f /results/.rc.${name}; nohup sh -c '${cmd} > /results/.log.${name} 2>&1; echo \$? > /results/.rc.${name}' >/dev/null 2>&1 &"
  until rc="$(exec_pod cat "/results/.rc.${name}" 2>/dev/null)" && [[ -n "${rc}" ]]; do sleep 10; done
  exec_pod cat "/results/.log.${name}" || true
  return "${rc}"
}

check_artifacts() {
  local engine="$1" dir="${RESULTS_DIR}/$1" missing=0 name suite concurrency
  local want=(ingest.json samples.ingest.jsonl disk.jsonl)
  for suite in ${SUITES}; do
    want+=("dump-${suite}.json")
    for concurrency in ${CONCURRENCIES}; do
      want+=("query-${suite}-c${concurrency}.json" "samples.query-${suite}-c${concurrency}.jsonl")
    done
  done
  for name in "${want[@]}"; do
    if [[ ! -s "${dir}/${name}" ]]; then
      printf 'missing or empty artifact %s/%s\n' "${dir}" "${name}" >&2
      missing=1
    fi
  done
  return "${missing}"
}

run_engine() {
  local engine="$1"
  local out="${RESULTS_DIR}/${engine}"
  local target="http://${engine}:9090"

  mkdir -p "${out}"

  step "${engine}: reset"
  reset_engine "${engine}"

  step "${engine}: apply"
  kubectl apply -k "overlays/local/engines/${engine}"

  step "${engine}: wait ready"
  kubectl -n "${NAMESPACE}" wait --for=create "pod/${engine}-0" --timeout="${READY_TIMEOUT}"
  kubectl -n "${NAMESPACE}" wait --for=condition=Ready "pod/${engine}-0" --timeout="${READY_TIMEOUT}"

  step "${engine}: collector for ingest"
  start_collector "${engine}" ingest -duration "$(( TOTAL_MS / 1000 + 300 ))s"

  step "${engine}: disk watcher"
  disk_watch "${engine}" &
  local disk_pid=$!

  step "${engine}: ingest"
  pod_run ingest /tools/loadgen \
    -target "${target}" \
    -engine "${engine}" \
    -run-id "${RUN_ID}" \
    -seed "${SEED}" \
    -interval "${INTERVAL}" \
    -batch-series "${BATCH_SERIES}" \
    -workers "${WORKERS}" \
    -ramp "${RAMP}" \
    -epoch-ms "${EPOCH_MS}" \
    -out "/results/${engine}/ingest.json"

  step "${engine}: settle"
  sleep 30
  stop_collector

  step "${engine}: queries"
  local suite concurrency phase
  for suite in ${SUITES}; do
    for concurrency in ${CONCURRENCIES}; do
      phase="query-${suite}-c${concurrency}"
      start_collector "${engine}" "${phase}" -duration 3h
      pod_run "${phase}" /tools/querybench \
        -target "${target}" \
        -engine "${engine}" \
        -run-id "${RUN_ID}" \
        -suite "${suite}" \
        -concurrency "${concurrency}" \
        -per-query "${PER_QUERY}" \
        -min-runs "${MIN_RUNS}" \
        -warmup "${WARMUP}" \
        -mode bench \
        -phase "${phase}" \
        -at-ms "${EVAL_MS}" \
        -out "/results/${engine}/query-${suite}-c${concurrency}.json"
      stop_collector
    done
  done

  step "${engine}: correctness dump"
  for suite in ${SUITES}; do
    pod_run "dump-${suite}" /tools/querybench \
      -target "${target}" \
      -engine "${engine}" \
      -run-id "${RUN_ID}" \
      -suite "${suite}" \
      -mode dump \
      -at-ms "${EVAL_MS}" \
      -out "/results/${engine}/dump-${suite}.json"
  done

  kill "${disk_pid}" 2>/dev/null || true
  wait "${disk_pid}" 2>/dev/null || true

  step "${engine}: collect engine artifacts"
  for attempt in 1 2 3; do
    kubectl cp "${NAMESPACE}/${POD}:/results/${engine}/." "${out}" -c harness && break
    [[ "${attempt}" -lt 3 ]] || exit 1
    sleep 5
  done
  ls -l "${out}"
  check_artifacts "${engine}"

  step "${engine}: tear down"
  reset_engine "${engine}"
}

exec_pod /tools/meta \
  -node "${BENCH_NODE}" \
  -run-id "${RUN_ID}" \
  -engines "${meta_engines}" \
  -out "/results/run.json"

for engine in ${ENGINES}; do
  run_engine "${engine}"
done

step "collect run metadata"
kubectl cp "${NAMESPACE}/${POD}:/results/run.json" "${RESULTS_DIR}/run.json" -c harness

step "redact node name"
node_pattern="$(printf '%s' "${BENCH_NODE}" | sed 's/[.[\*^$/]/\\&/g')"
grep -rlF -- "${BENCH_NODE}" "${RESULTS_DIR}" | while read -r file; do
  sed -i.bak "s/${node_pattern}/bench-node/g" "${file}" && rm -f "${file}.bak"
  printf 'redacted %s\n' "${file}"
done || true

step "report"
go run ./cmd/report -root results -run "${RUN_ID}" -quiet

step "done"
printf 'run %s artifacts in %s\n' "${RUN_ID}" "${RESULTS_DIR}"
