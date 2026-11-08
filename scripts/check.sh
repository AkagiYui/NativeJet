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
GOFLAGS="${GOFLAGS:-} -modfile=$(python3 scripts/mygo-compat.py)"
export GOFLAGS
if [ "$(go env GOARCH)" = amd64 ]; then
  # The signature tests need signed binaries, which Go only supplies on arm64.
  TASK_TEST_EXEC="$(cat <<'EOF'
sh -ec '
  codesign --force --sign - "$0"
  exec "$0" "$@"
'
EOF
)"
  go test -race -exec "$TASK_TEST_EXEC" ./...
else
  go test -race ./...
fi
go vet ./...
