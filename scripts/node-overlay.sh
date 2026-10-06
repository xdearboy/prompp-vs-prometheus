#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
overlay="${root}/overlays/local"

: "${BENCH_NODE:?BENCH_NODE must be set to the target node name}"

if [[ ! "${BENCH_NODE}" =~ ^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$ ]]; then
  printf 'invalid BENCH_NODE: %s\n' "${BENCH_NODE}" >&2
  exit 1
fi

engines=(prompp-0815 prom-3150 prom-2551)

write_node_patch() {
  cat > "$1" <<PATCH
apiVersion: v1
kind: ConfigMap
metadata:
  name: prompp-bench-node
  namespace: prompp-bench
data:
  nodeName: ${BENCH_NODE}
PATCH
}

write_replacements() {
  cat >> "$1" <<'PATCH'
replacements:
  - source:
      kind: ConfigMap
      name: prompp-bench-node
      fieldPath: data.nodeName
    targets:
      - select:
          kind: StatefulSet
          namespace: prompp-bench
        fieldPaths:
          - spec.template.spec.nodeName
        options:
          delimiter: ''
          index: 0
PATCH
}

rm -rf "${overlay}"
mkdir -p "${overlay}"

cat > "${overlay}/kustomization.yaml" <<'PATCH'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../deploy
patches:
  - path: node-patch.yaml
    target:
      kind: ConfigMap
      name: prompp-bench-node
PATCH
write_node_patch "${overlay}/node-patch.yaml"
write_replacements "${overlay}/kustomization.yaml"

for engine in "${engines[@]}"; do
  dir="${overlay}/engines/${engine}"
  mkdir -p "${dir}"
  cat > "${dir}/kustomization.yaml" <<PATCH
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../../../deploy/base
  - ../../../../deploy/engines/${engine}
patches:
  - path: node-patch.yaml
    target:
      kind: ConfigMap
      name: prompp-bench-node
PATCH
  write_node_patch "${dir}/node-patch.yaml"
  write_replacements "${dir}/kustomization.yaml"
done

printf 'wrote %s for node %s\n' "${overlay}" "${BENCH_NODE}"
printf 'engine overlays: %s\n' "${engines[*]}"