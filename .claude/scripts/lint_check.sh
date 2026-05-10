#!/usr/bin/env bash
# Stop hook: golangci-lint でエラーがあれば exit 2 で中断する

set -uo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$PROJECT_ROOT" || exit 1

OUTPUT=$(CGO_ENABLED=0 golangci-lint run ./... 2>&1)
EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ]; then
  echo "$OUTPUT" >&2
  exit 2
fi
