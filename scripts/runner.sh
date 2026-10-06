#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

NAMESPACE="${BENCH_NAMESPACE:-prompp-bench}"
RUNNER="${BENCH_RUNNER:-bench-runner}"
IMAGE="${BENCH_RUNNER_IMAGE:-mirror.gcr.io/alpine/k8s:1.32.9}"

: "${BENCH_NODE:?BENCH_NODE must be set}"

mkdir -p bin/runner
for name in report aggregate; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "bin/runner/${name}" "./cmd/${name}"
done

kubectl apply -f deploy/runner.yaml
kubectl delete pod "${RUNNER}" -n "${NAMESPACE}" --ignore-not-found --wait=true
cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: ${RUNNER}
  namespace: ${NAMESPACE}
spec:
  serviceAccountName: runner
  restartPolicy: Never
  containers:
    - name: runner
      image: ${IMAGE}
      command: ["/bin/sleep", "infinity"]
      resources:
        requests: {cpu: 50m, memory: 128Mi}
        limits: {cpu: 500m, memory: 512Mi}
YAML
kubectl wait --for=condition=Ready "pod/${RUNNER}" -n "${NAMESPACE}" --timeout=300s

kubectl exec -n "${NAMESPACE}" "${RUNNER}" -- mkdir -p /work
tar -cf - scripts deploy overlays bin/runner | kubectl exec -i -n "${NAMESPACE}" "${RUNNER}" -- tar -xf - -C /work

kubectl exec -n "${NAMESPACE}" "${RUNNER}" -- sh -c "cd /work && \
  BENCH_NODE='${BENCH_NODE}' RUNS='${RUNS:-12}' PERIOD_HOURS='${PERIOD_HOURS:-4}' \
  REPORT_CMD=/work/bin/runner/report AGGREGATE_CMD=/work/bin/runner/aggregate \
  nohup bash scripts/series.sh > series.log 2>&1 &"
printf 'series started in %s/%s, results in /work/results\n' "${NAMESPACE}" "${RUNNER}"
