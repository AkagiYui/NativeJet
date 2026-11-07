#!/bin/bash
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
export GOMODCACHE="$TASK_ROOT/.cache/gomod"
export GOCACHE="$TASK_ROOT/.cache/gobuild"
export GOPATH="$TASK_ROOT/.cache/gopath"
export CGO_ENABLED=1
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
if [ -x "$TASK_ROOT/.cache/toolchain/go/bin/go" ]; then
  export PATH="$TASK_ROOT/.cache/toolchain/go/bin:$PATH"
  export GOTOOLCHAIN=local
fi
export GOFLAGS="${GOFLAGS:-} -modfile=$(python3 scripts/mygo-compat.py)"
TASK_APP="$TASK_ROOT/build/NativeJet.app"
mkdir -p "$TASK_APP/Contents/MacOS" "$TASK_APP/Contents/Library/LaunchServices" "$TASK_APP/Contents/Library/LaunchDaemons" "$TASK_APP/Contents/Resources"
TASK_FLAGS="-s -w -X nativejet/internal/buildinfo.Version=0.2.7"
if [ -n "${NATIVEJET_TEAM_ID:-}" ]; then
  TASK_FLAGS="$TASK_FLAGS -X nativejet/internal/buildinfo.TeamID=$NATIVEJET_TEAM_ID"
fi
go build -trimpath -ldflags "$TASK_FLAGS" -o "$TASK_APP/Contents/MacOS/NativeJet" ./cmd/nativejet
go build -trimpath -ldflags "$TASK_FLAGS" -o "$TASK_APP/Contents/Library/LaunchServices/nativejet-helper" ./cmd/nativejet-helper
go build -trimpath -o "$TASK_ROOT/build/nativejetctl" ./cmd/nativejetctl
go run ./cmd/notices
cp packaging/macos/Info.plist "$TASK_APP/Contents/Info.plist"
cp packaging/macos/NativeJet.icns "$TASK_APP/Contents/Resources/"
cp packaging/macos/dev.nativejet.helper.plist "$TASK_APP/Contents/Library/LaunchDaemons/"
cp -R licenses "$TASK_APP/Contents/Resources/"
if [ -n "${NATIVEJET_SIGN_IDENTITY:-}" ]; then
  if [ -z "${NATIVEJET_TEAM_ID:-}" ]; then
    echo "NATIVEJET_TEAM_ID is required for a signed service build" >&2
    exit 1
  fi
  codesign --force --options runtime --timestamp --identifier dev.nativejet.helper --sign "$NATIVEJET_SIGN_IDENTITY" "$TASK_APP/Contents/Library/LaunchServices/nativejet-helper"
  codesign --force --options runtime --timestamp --sign "$NATIVEJET_SIGN_IDENTITY" "$TASK_APP"
else
  codesign --force --identifier dev.nativejet.helper --sign - "$TASK_APP/Contents/Library/LaunchServices/nativejet-helper"
  codesign --force --sign - "$TASK_APP"
fi
codesign --verify --deep --strict "$TASK_APP"
echo "$TASK_APP"
