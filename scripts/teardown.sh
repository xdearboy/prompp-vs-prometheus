#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${BENCH_NAMESPACE:-prompp-bench}"
POD="${BENCH_POD:-bench-harness}"
ENGINES="${ENGINES:-prompp-0815 prom-3150 prom-2551}"

if [[ "${1:-}" == "--yes" || "${BENCH_TEARDOWN:-}" == "1" ]]; then
  confirm() { return 0; }
else
  confirm() { read -r -p "$1 [y/N] " reply; [[ "${reply}" == "y" || "${reply}" == "Y" ]]; }
fi

printf 'namespace %s, engines %s, harness %s\n' "${NAMESPACE}" "${ENGINES}" "${POD}"

if ! confirm "delete benchmark statefulsets, pvcs, services and the harness pod?"; then
  printf 'aborted\n'
  exit 0
fi

for engine in ${ENGINES}; do
  kubectl -n "${NAMESPACE}" delete statefulset "${engine}" --ignore-not-found --wait=true --timeout=300s
  kubectl -n "${NAMESPACE}" delete pvc "data-${engine}-0" --ignore-not-found --wait=true --timeout=300s
  kubectl -n "${NAMESPACE}" delete service "${engine}" --ignore-not-found
done

kubectl -n "${NAMESPACE}" delete pod "${POD}" --ignore-not-found --wait=true --timeout=120s

kubectl -n "${NAMESPACE}" delete serviceaccount collector --ignore-not-found
kubectl -n "${NAMESPACE}" delete rolebinding collector --ignore-not-found
kubectl -n "${NAMESPACE}" delete role collector --ignore-not-found
kubectl -n "${NAMESPACE}" delete configmap prompp-bench-config --ignore-not-found
kubectl -n "${NAMESPACE}" delete configmap prompp-bench-node --ignore-not-found
kubectl -n "${NAMESPACE}" delete clusterrolebinding prompp-bench-metrics --ignore-not-found
kubectl -n "${NAMESPACE}" delete clusterrole prompp-bench-metrics --ignore-not-found 2>/dev/null \
  || kubectl delete clusterrole prompp-bench-metrics --ignore-not-found

if confirm "delete the namespace ${NAMESPACE}?"; then
  kubectl delete namespace "${NAMESPACE}" --ignore-not-found --wait=true --timeout=300s
fi

printf 'teardown complete\n'