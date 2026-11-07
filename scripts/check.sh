#!/bin/bash
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
export GOMODCACHE="$TASK_ROOT/.cache/gomod"
export GOCACHE="$TASK_ROOT/.cache/gobuild"
export GOPATH="$TASK_ROOT/.cache/gopath"
export CGO_ENABLED=1
if [ -x "$TASK_ROOT/.cache/toolchain/go/bin/go" ]; then
  export PATH="$TASK_ROOT/.cache/toolchain/go/bin:$PATH"
  export GOTOOLCHAIN=local
fi
export GOFLAGS="${GOFLAGS:-} -modfile=$(python3 scripts/mygo-compat.py)"
go test -race ./...
go vet ./...
