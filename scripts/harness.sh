#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

POD="${BENCH_POD:-bench-harness}"
NAMESPACE="${BENCH_NAMESPACE:-prompp-bench}"
IMAGE="${BENCH_HARNESS_IMAGE:-quay.io/prometheus/prometheus:v3.15.0}"
BINARIES=(loadgen collector querybench meta)

: "${BENCH_NODE:?BENCH_NODE must be set}"

step() { printf '\n== %s\n' "$1"; }

step "build linux/amd64 binaries"
mkdir -p bin
version="$(git rev-parse --short HEAD 2>/dev/null || printf 'dev')-$(git -C tools rev-parse --short HEAD 2>/dev/null || printf 'dev')"
printf '%s\n' "${version}" > bin/version
[[ -f tools/go.mod ]] || { printf 'tools submodule is empty, run git submodule update --init\n' >&2; exit 1; }
for name in "${BINARIES[@]}"; do
  if [[ "${name}" == meta ]]; then dir=.; else dir=tools; fi
  (cd "${dir}" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "${root}/bin/${name}" "./cmd/${name}")
  printf '%s ok\n' "${name}"
done
printf 'version %s\n' "${version}"

step "apply namespace, config and rbac"
kubectl apply -f deploy/base/namespace.yaml
kubectl apply -f deploy/base/configmap.yaml
kubectl apply -f deploy/base/rbac.yaml

step "apply harness pod on ${BENCH_NODE}"
cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: prompp-bench-harness
spec:
  serviceAccountName: collector
  nodeName: ${BENCH_NODE}
  restartPolicy: Never
  tolerations:
    - key: node.kubernetes.io/unschedulable
      operator: Exists
      effect: NoSchedule
  containers:
    - name: harness
      image: ${IMAGE}
      command: ["/bin/sleep", "86400"]
      securityContext:
        runAsUser: 0
      resources:
        requests:
          cpu: 200m
          memory: 512Mi
        limits:
          cpu: 2000m
          memory: 4Gi
      volumeMounts:
        - name: tools
          mountPath: /tools
        - name: results
          mountPath: /results
  volumes:
    - name: tools
      emptyDir: {}
    - name: results
      emptyDir: {}
YAML

kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=180s

step "copy tools into pod"
for name in "${BINARIES[@]}"; do
  kubectl cp "bin/${name}" "${NAMESPACE}/${POD}:/tools/${name}" -c harness
  printf '%s copied\n' "${name}"
done
kubectl cp bin/version "${NAMESPACE}/${POD}:/tools/version" -c harness

step "verify tools"
kubectl exec -n "${NAMESPACE}" "${POD}" -c harness -- /tools/loadgen -h 2>&1 | head -3
kubectl exec -n "${NAMESPACE}" "${POD}" -c harness -- /tools/collector -h 2>&1 | head -3
kubectl exec -n "${NAMESPACE}" "${POD}" -c harness -- /tools/querybench -h 2>&1 | head -3
kubectl exec -n "${NAMESPACE}" "${POD}" -c harness -- sh -c 'command -v killall' \
  || { printf 'harness image has no killall, run.sh cannot stop collectors\n' >&2; exit 1; }

printf '\nharness ready: %s/%s\n' "${NAMESPACE}" "${POD}"