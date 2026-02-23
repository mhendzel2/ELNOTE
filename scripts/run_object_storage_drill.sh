#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="${ROOT_DIR}/server"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 2
fi

if [[ -z "${OBJECT_DRILL_ADMIN_PASSWORD:-}" ]]; then
  echo "OBJECT_DRILL_ADMIN_PASSWORD is required" >&2
  exit 2
fi

API_BASE_URL="${OBJECT_DRILL_API_BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="${OBJECT_DRILL_ADMIN_EMAIL:-labadmin}"
DEVICE_NAME="${OBJECT_DRILL_DEVICE_NAME:-ops-object-drill}"
ARTIFACT_DIR="${OBJECT_DRILL_ARTIFACT_DIR:-../docs/drills/object-storage}"
MAX_DOWNLOAD_BYTES="${OBJECT_DRILL_MAX_DOWNLOAD_BYTES:-1024}"

EXTRA_ARGS=()
if [[ -n "${OBJECT_DRILL_ATTACHMENT_ID:-}" ]]; then
  EXTRA_ARGS+=(--attachment-id "${OBJECT_DRILL_ATTACHMENT_ID}")
fi
if [[ -n "${OBJECT_DRILL_RESTORE_CMD:-}" ]]; then
  EXTRA_ARGS+=(--restore-command "${OBJECT_DRILL_RESTORE_CMD}")
fi

cd "${SERVER_DIR}"
go run ./cmd/objectdrill \
  --source-dsn "${DATABASE_URL}" \
  --api-base-url "${API_BASE_URL}" \
  --admin-email "${ADMIN_EMAIL}" \
  --admin-password "${OBJECT_DRILL_ADMIN_PASSWORD}" \
  --device-name "${DEVICE_NAME}" \
  --artifact-dir "${ARTIFACT_DIR}" \
  --max-download-bytes "${MAX_DOWNLOAD_BYTES}" \
  "${EXTRA_ARGS[@]}"
