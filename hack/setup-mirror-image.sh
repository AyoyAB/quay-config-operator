#!/usr/bin/env bash
# Tags and pushes the local operator image to the Kind local registry
# so Quay can use it as a mirror source.
set -euo pipefail

LOCAL_REGISTRY_PORT="${1:-5001}"
IMG="${IMG:-ghcr.io/ayoyab/quay-config-operator:latest}"
MIRROR_IMAGE="localhost:${LOCAL_REGISTRY_PORT}/ayoy/quay-config-operator"
TAG="latest"

echo "==> Tagging ${IMG} as ${MIRROR_IMAGE}:${TAG}..."
docker tag "${IMG}" "${MIRROR_IMAGE}:${TAG}"

echo "==> Pushing to local registry..."
docker push "${MIRROR_IMAGE}:${TAG}"

echo "==> Mirror source image ready at kind-registry:5000/ayoy/quay-config-operator:${TAG}"
