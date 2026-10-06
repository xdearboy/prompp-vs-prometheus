#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

fail=0

step() { printf '\n== %s\n' "$1"; }

step "gofmt"
unformatted="$(gofmt -l . || true)"
if [[ -n "${unformatted}" ]]; then
  printf 'unformatted files:\n%s\n' "${unformatted}" >&2
  fail=1
else
  printf 'ok\n'
fi

step "go vet"
if go vet ./... ; then printf 'ok\n'; else fail=1; fi

step "go test"
if go test ./... ; then printf 'ok\n'; else fail=1; fi

step "kustomize render"
if rendered="$(kubectl kustomize deploy 2>&1)"; then
  printf 'deploy rendered\n'
else
  printf '%s\n' "${rendered}" >&2
  fail=1
fi

if [[ -f overlays/local/kustomization.yaml ]]; then
  if rendered="$(kubectl kustomize overlays/local 2>&1)"; then
    printf 'overlays/local rendered\n'
    if grep -q 'bench-node-placeholder' <<<"${rendered}"; then
      printf 'node placeholder still present in overlays/local\n' >&2
      fail=1
    fi
    if grep -Eq '[A-Za-z0-9.-]+\.internal|\.lan\.' <<<"${rendered}"; then
      printf 'internal hostname detected in overlays/local\n' >&2
      fail=1
    fi
  else
    printf '%s\n' "${rendered}" >&2
    fail=1
  fi
else
  printf 'overlays/local missing, run scripts/node-overlay.sh with BENCH_NODE set\n' >&2
  fail=1
fi

step "namespace confinement"
if rendered="$(kubectl kustomize overlays/local 2>&1)"; then
  stray="$(grep -E '^  namespace: ' <<<"${rendered}" | sort -u | grep -v 'namespace: prompp-bench' || true)"
  if [[ -n "${stray}" ]]; then
    printf 'objects rendered outside the benchmark namespace:\n%s\n' "${stray}" >&2
    fail=1
  else
    printf 'ok\n'
  fi
else
  printf '%s\n' "${rendered}" >&2
  fail=1
fi

step "server dry-run"
if kubectl get namespace prompp-bench >/dev/null 2>&1; then
  dry_target=(-k overlays/local)
else
  printf 'namespace prompp-bench absent, validating only the namespace object\n'
  dry_target=(-f deploy/base/namespace.yaml)
fi
if kubectl apply "${dry_target[@]}" --dry-run=server >/dev/null 2>&1; then
  printf 'ok\n'
else
  kubectl apply "${dry_target[@]}" --dry-run=server || true
  fail=1
fi

step "resource limits agree with run metadata"
bad_limits=0
for engine_dir in deploy/engines/*/; do
  manifest="${engine_dir}statefulset.yaml"
  want_cpu="$(awk '/resources:/{r=1} r && /cpu:/{print $2; exit}' "${manifest}")"
  want_mem="$(awk '/resources:/{r=1} r && /memory:/{print $2; exit}' "${manifest}")"
  if [[ "${want_cpu}" != "2000m" || "${want_mem}" != "6Gi" ]]; then
    printf '%s declares cpu=%s memory=%s but run metadata records 2000m and 6Gi\n' \
      "${manifest}" "${want_cpu}" "${want_mem}" >&2
    bad_limits=1
  fi
done
if [[ "${bad_limits}" -ne 0 ]]; then
  fail=1
else
  printf 'ok\n'
fi

step "secret scan"
if git rev-parse --git-dir >/dev/null 2>&1; then
  if git grep -nIE '(internal\.[a-z-]+\.(net|com|org|io)|10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+)' -- \
      ':!go.sum' ':!*.md' ':!runs' >/dev/null 2>&1; then
    printf 'possible internal identifiers in tracked files\n' >&2
    git grep -nIE '(internal\.[a-z-]+\.(net|com|org|io)|10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+)' -- ':!go.sum' ':!*.md' ':!runs' >&2 || true
    fail=1
  else
    printf 'ok\n'
  fi
  node_name="$(awk '/nodeName:/ {print $2; exit}' overlays/local/node-patch.yaml 2>/dev/null || true)"
  if [[ -n "${node_name}" ]] && git grep -nIF "${node_name}" >/dev/null 2>&1; then
    printf 'the real bench node name is committed:\n' >&2
    git grep -nIF "${node_name}" >&2 || true
    fail=1
  fi
else
  printf 'skipped, not a git repo\n'
fi

printf '\n'
if [[ "${fail}" -ne 0 ]]; then
  printf 'preflight failed\n' >&2
  exit 1
fi
printf 'preflight passed\n'
