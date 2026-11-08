#!/bin/bash
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
TASK_ARCH="${1:-amd64}"
case "$TASK_ARCH" in
  amd64|arm64) ;;
  *) echo "Usage: bash scripts/build-windows.sh [amd64|arm64]" >&2; exit 2 ;;
esac
if [ -x "$TASK_ROOT/.cache/toolchain/go/bin/go" ]; then
  export PATH="$TASK_ROOT/.cache/toolchain/go/bin:$PATH"
  export GOTOOLCHAIN=local
  export GOMODCACHE="$TASK_ROOT/.cache/gomod"
  export GOCACHE="$TASK_ROOT/.cache/gobuild"
fi
TASK_OUT="build/windows-$TASK_ARCH"
CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo build -platform "windows/$TASK_ARCH" -skip-installer packaging/windows
mkdir -p "$TASK_OUT/licenses"
GOOS=windows GOARCH="$TASK_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$TASK_OUT/nativejet-helper.exe" ./cmd/nativejet-helper
GOOS=windows GOARCH="$TASK_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$TASK_OUT/nativejetctl.exe" ./cmd/nativejetctl
go run ./cmd/notices -goos windows -goarch "$TASK_ARCH" -output "$TASK_OUT/licenses/dependencies"
cp licenses/THIRD_PARTY_NOTICES.md "$TASK_OUT/licenses/"
echo "$TASK_OUT"
