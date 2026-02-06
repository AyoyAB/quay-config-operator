#!/usr/bin/env bash
# Creates a Kind cluster with a local Docker registry for e2e testing.
# Based on https://kind.sigs.k8s.io/docs/user/local-registry/
set -euo pipefail

KIND_CLUSTER_NAME="${1:-quay-config-operator}"
LOCAL_REGISTRY_PORT="${2:-5001}"
REG_NAME="kind-registry"
NODE_IMAGE="kindest/node:v1.31.14"

# --- Local registry ---
if [ "$(docker inspect -f '{{.State.Running}}' "${REG_NAME}" 2>/dev/null || true)" != "true" ]; then
  echo "Starting local registry ${REG_NAME} on port ${LOCAL_REGISTRY_PORT}..."
  docker run -d --restart=always -p "127.0.0.1:${LOCAL_REGISTRY_PORT}:5000" --network bridge --name "${REG_NAME}" registry:2
fi

# --- Kind cluster ---
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
  echo "Kind cluster '${KIND_CLUSTER_NAME}' already exists, skipping creation."
else
  echo "Creating Kind cluster '${KIND_CLUSTER_NAME}' with node image ${NODE_IMAGE}..."
  cat <<EOF | kind create cluster --name "${KIND_CLUSTER_NAME}" --image "${NODE_IMAGE}" --kubeconfig "${KUBECONFIG:-${HOME}/.kube/config}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF
fi

# --- Configure registry on nodes ---
REGISTRY_DIR="/etc/containerd/certs.d/localhost:${LOCAL_REGISTRY_PORT}"
for node in $(kind get nodes --name "${KIND_CLUSTER_NAME}"); do
  docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
  cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${REG_NAME}:5000"]
EOF
done

# --- Connect registry to Kind network ---
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REG_NAME}" 2>/dev/null)" = "null" ] || \
   [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REG_NAME}" 2>/dev/null)" = "" ]; then
  echo "Connecting registry to Kind network..."
  docker network connect "kind" "${REG_NAME}" || true
fi

# --- Document the registry for cluster consumers ---
echo "Creating local-registry-hosting ConfigMap..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${LOCAL_REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

echo "Kind cluster '${KIND_CLUSTER_NAME}' with local registry ready."
