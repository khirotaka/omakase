#!/usr/bin/env bash
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

# Download Go modules; also triggers auto-download of the Go 1.26 toolchain
# declared in go.mod so that subsequent go tool invocations use it.
go mod download

# Install golangci-lint built with Go 1.26 to match the project's Go version.
# The pre-installed binary is built with Go 1.25 and fails to lint Go 1.26 code.
GOTOOLCHAIN=go1.26.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
GOTOOLCHAIN=go1.26.3 go install golang.org/x/tools/gopls@latest
cp "$(go env GOPATH)/bin/golangci-lint" /usr/local/bin/golangci-lint
