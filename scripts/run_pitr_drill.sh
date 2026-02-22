#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 2
fi

ARTIFACT_DIR="${ARTIFACT_DIR:-$REPO_ROOT/docs/drills/pitr}"
RESTORE_DB_PREFIX="${RESTORE_DB_PREFIX:-elnote_restore_drill}"
TARGET_OFFSET="${TARGET_OFFSET:-2m}"
KEEP_RESTORE_DB="${KEEP_RESTORE_DB:-false}"

cd "$REPO_ROOT/server"

ARGS=(
  "./cmd/pitrdrill"
  "--source-dsn=$DATABASE_URL"
  "--artifact-dir=$ARTIFACT_DIR"
  "--restore-db-prefix=$RESTORE_DB_PREFIX"
  "--target-offset=$TARGET_OFFSET"
)

if [[ "$KEEP_RESTORE_DB" == "true" ]]; then
  ARGS+=("--keep-restore-db")
fi

go run "${ARGS[@]}" "$@"
