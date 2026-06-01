#!/usr/bin/env bash
# Build Sub2API image and push both immutable and latest tags to the private registry.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

REGISTRY="${REGISTRY:-devtest.pointlife365.net:5180}"
REGISTRY_NAMESPACE="${REGISTRY_NAMESPACE:-slzr}"
IMAGE_NAME="${IMAGE_NAME:-sub2api}"
REGISTRY_USER="${REGISTRY_USER:-slzr}"
PASSWORD_FILE="${PASSWORD_FILE:-${SCRIPT_DIR}/registry-password.txt}"
BUILD_DATE="${BUILD_DATE:-$(date +%Y%m%d)}"
VERSION_TAG="${VERSION_TAG:-product-zhanzi-${BUILD_DATE}-$(git -C "${REPO_ROOT}" rev-parse --short=8 HEAD)}"

IMAGE_REPO="${REGISTRY}/${REGISTRY_NAMESPACE}/${IMAGE_NAME}"
VERSION_IMAGE="${IMAGE_REPO}:${VERSION_TAG}"
LATEST_IMAGE="${IMAGE_REPO}:latest"

if [[ ! -f "${PASSWORD_FILE}" ]]; then
  echo "Missing registry password file: ${PASSWORD_FILE}" >&2
  echo "Create it locally with the private registry password. Do not commit it." >&2
  exit 1
fi

echo "Logging in to ${REGISTRY} as ${REGISTRY_USER}"
docker login "${REGISTRY}" --username "${REGISTRY_USER}" --password-stdin < "${PASSWORD_FILE}"

echo "Building ${VERSION_IMAGE}"
docker build \
  -t "${VERSION_IMAGE}" \
  -t "${LATEST_IMAGE}" \
  --build-arg GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  --build-arg GOSUMDB="${GOSUMDB:-sum.golang.google.cn}" \
  -f "${REPO_ROOT}/Dockerfile" \
  "${REPO_ROOT}"

echo "Pushing ${VERSION_IMAGE}"
docker push "${VERSION_IMAGE}"

echo "Pushing ${LATEST_IMAGE}"
docker push "${LATEST_IMAGE}"

cat <<EOF

Done.

Immutable image:
  ${VERSION_IMAGE}

Latest image:
  ${LATEST_IMAGE}

Production .env example:
  SUB2API_IMAGE=${VERSION_IMAGE}

Local/test .env example:
  SUB2API_IMAGE=${LATEST_IMAGE}
EOF
