#!/usr/bin/env bash
# Deploys the quay-mock server and creates the quay-credentials secret
# for e2e testing.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

LOCAL_REGISTRY_PORT="${LOCAL_REGISTRY_PORT:-5001}"
MOCK_IMAGE="localhost:${LOCAL_REGISTRY_PORT}/quay-mock:latest"
QUAY_NAMESPACE="quay-system"
# Hardcoded token is acceptable: only used locally in Kind during e2e tests.
QUAY_TOKEN="e2e-test-token"

# ---------- Step 1: Build and push mock image ----------
echo "==> Building quay-mock image..."
docker build -t "${MOCK_IMAGE}" "${PROJECT_DIR}/e2e/quay-mock"

echo "==> Pushing quay-mock to local registry..."
docker push "${MOCK_IMAGE}"

# ---------- Step 2: Deploy mock ----------
echo "==> Deploying quay-mock..."
kubectl apply -f "${PROJECT_DIR}/e2e/quay/namespace.yaml"
kubectl apply -f "${PROJECT_DIR}/e2e/quay/quay-mock.yaml"

echo "==> Waiting for quay-mock to be ready..."
kubectl -n "${QUAY_NAMESPACE}" rollout status deployment/quay-mock --timeout=60s

# ---------- Step 3: Create credentials secret ----------
echo "==> Creating quay-credentials secret..."
kubectl -n "${QUAY_NAMESPACE}" create secret generic quay-credentials \
  --from-literal=host="http://quay-mock.${QUAY_NAMESPACE}.svc.cluster.local" \
  --from-literal=token="${QUAY_TOKEN}" \
  --from-literal=validateCerts="false" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> Quay mock setup complete!"
