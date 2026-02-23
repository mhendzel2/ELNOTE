#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_PATH="${1:-${ROOT_DIR}/docs/release-gates/pilot-uat-go-live.json}"

cd "${ROOT_DIR}/server"
go run ./cmd/releasegate --artifact "${ARTIFACT_PATH}"
