#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-logbook-smoke}"
RELEASE_NAME="${RELEASE_NAME:-logbook}"
NAMESPACE="${NAMESPACE:-logbook-smoke}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-logbook}"
IMAGE_TAG="${IMAGE_TAG:-smoke}"
TIMEOUT="${TIMEOUT:-120s}"
CREATE_CLUSTER="${CREATE_CLUSTER:-false}"

cleanup() {
  if [[ "${CREATE_CLUSTER}" == "true" ]]; then
    kind delete cluster --name "${CLUSTER_NAME}"
  else
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

require_command docker
require_command helm
require_command kind
require_command kubectl

if [[ "${CREATE_CLUSTER}" == "true" ]]; then
  kind create cluster --name "${CLUSTER_NAME}" --wait "${TIMEOUT}"
fi

docker build \
  --build-arg VERSION=smoke \
  --build-arg COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "${IMAGE_REPOSITORY}:${IMAGE_TAG}" .

kind load docker-image "${IMAGE_REPOSITORY}:${IMAGE_TAG}" --name "${CLUSTER_NAME}"

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install "${RELEASE_NAME}" ./helmchart \
  --namespace "${NAMESPACE}" \
  --set "image.repository=${IMAGE_REPOSITORY}" \
  --set "image.tag=${IMAGE_TAG}" \
  --set "image.pullPolicy=Never" \
  --set "logbook.namespace=${NAMESPACE}" \
  --wait \
  --timeout "${TIMEOUT}"

kubectl rollout status "deployment/${RELEASE_NAME}" --namespace "${NAMESPACE}" --timeout "${TIMEOUT}"
kubectl create configmap logbook-smoke-subject --namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

for attempt in $(seq 1 30); do
  event_name="logbook-smoke-${attempt}"
  event_time="$(date -u +%Y-%m-%dT%H:%M:%S.000000Z)"

  cat <<EOF | kubectl apply -f -
apiVersion: events.k8s.io/v1
kind: Event
metadata:
  name: ${event_name}
  namespace: ${NAMESPACE}
eventTime: "${event_time}"
reportingController: logbook-smoke
reportingInstance: smoke-test
action: Testing
reason: LogbookSmokeTest
type: Normal
note: logbook smoke test event ${attempt}
regarding:
  apiVersion: v1
  kind: ConfigMap
  name: logbook-smoke-subject
  namespace: ${NAMESPACE}
EOF

  sleep 2
  if kubectl logs "deployment/${RELEASE_NAME}" --namespace "${NAMESPACE}" --since=5m | grep -q "LogbookSmokeTest"; then
    kubectl logs "deployment/${RELEASE_NAME}" --namespace "${NAMESPACE}" --tail=20
    exit 0
  fi
done

kubectl describe "deployment/${RELEASE_NAME}" --namespace "${NAMESPACE}" >&2 || true
kubectl get pods --namespace "${NAMESPACE}" -o wide >&2 || true
kubectl logs "deployment/${RELEASE_NAME}" --namespace "${NAMESPACE}" --tail=100 >&2 || true
echo "logbook did not observe the smoke test event" >&2
exit 1
