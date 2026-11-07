#!/bin/bash
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
: "${NATIVEJET_NOTARY_PROFILE:?Set an existing notarytool keychain profile}"
TASK_APP="$TASK_ROOT/build/NativeJet.app"
codesign --verify --deep --strict "$TASK_APP"
ditto -c -k --keepParent "$TASK_APP" "$TASK_ROOT/build/NativeJet.zip"
xcrun notarytool submit "$TASK_ROOT/build/NativeJet.zip" --keychain-profile "$NATIVEJET_NOTARY_PROFILE" --wait
xcrun stapler staple "$TASK_APP"
xcrun stapler validate "$TASK_APP"
