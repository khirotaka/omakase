#!/usr/bin/env bash
# PostToolUse hook: .go ファイルが変更された場合のみ golangci-lint --fix を実行する

set -euo pipefail

INPUT=$(cat)

FILE_PATH=$(echo "$INPUT" | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)

if [[ "$FILE_PATH" != *.go ]]; then
  exit 0
fi

PROJECT_ROOT="$(git -C "$(dirname "$FILE_PATH")" rev-parse --show-toplevel 2>/dev/null || pwd)"

cd "$PROJECT_ROOT"

golangci-lint run --fix ./... 2>/dev/null || true
