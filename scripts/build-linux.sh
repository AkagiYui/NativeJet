#!/bin/bash
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$TASK_ROOT"
TASK_ARCH="${1:-amd64}"
case "$TASK_ARCH" in
  amd64|arm64) ;;
  *) echo "Usage: bash scripts/build-linux.sh [amd64|arm64]" >&2; exit 2 ;;
esac
TASK_DPKG_DEB="${DPKG_DEB:-dpkg-deb}"
if ! command -v "$TASK_DPKG_DEB" >/dev/null; then
  echo "Linux packaging needs dpkg-deb (from dpkg)." >&2
  exit 1
fi
if [ -x "$TASK_ROOT/.cache/toolchain/go/bin/go" ]; then
  export PATH="$TASK_ROOT/.cache/toolchain/go/bin:$PATH"
  export GOTOOLCHAIN=local
  export GOMODCACHE="$TASK_ROOT/.cache/gomod"
  export GOCACHE="$TASK_ROOT/.cache/gobuild"
fi

mkdir -p build
TASK_STAGE="$(mktemp -d "build/.linux-$TASK_ARCH.XXXXXX")"
trap 'rm -rf -- "$TASK_STAGE"' EXIT
CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo build -platform "linux/$TASK_ARCH" -o "$TASK_ROOT/$TASK_STAGE/mygo" packaging/linux
TASK_PACKAGE="$TASK_STAGE/package"
"$TASK_DPKG_DEB" --raw-extract "$TASK_STAGE"/mygo/linux-"$TASK_ARCH"/nativejet_*.deb "$TASK_PACKAGE"
TASK_APP="$TASK_PACKAGE/opt/nativejet"
GOOS=linux GOARCH="$TASK_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$TASK_APP/nativejet-helper" ./cmd/nativejet-helper
GOOS=linux GOARCH="$TASK_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$TASK_APP/nativejetctl" ./cmd/nativejetctl
ln -s /opt/nativejet/nativejetctl "$TASK_PACKAGE/usr/bin/nativejetctl"
go run ./cmd/notices -goos linux -goarch "$TASK_ARCH" -output "$TASK_APP/licenses/dependencies"
cp licenses/THIRD_PARTY_NOTICES.md "$TASK_APP/licenses/"
mkdir -p "$TASK_PACKAGE/usr/lib/systemd/system" "$TASK_PACKAGE/usr/share/polkit-1/actions"
install -m 644 packaging/linux/dev.nativejet.helper.service "$TASK_PACKAGE/usr/lib/systemd/system/dev.nativejet.helper.service"
install -m 644 packaging/linux/dev.nativejet.helper.policy "$TASK_PACKAGE/usr/share/polkit-1/actions/dev.nativejet.helper.policy"
for script in postinst prerm postrm; do
  install -m 755 "packaging/linux/$script" "$TASK_PACKAGE/DEBIAN/$script"
done
(
  cd "$TASK_PACKAGE"
  TASK_SIZE="$(du -sk . | awk '{print $1}')"
  awk -v size="$TASK_SIZE" '/^Installed-Size:/ { $0 = "Installed-Size: " size } { print }' DEBIAN/control > DEBIAN/control.new
  mv DEBIAN/control.new DEBIAN/control
  find . -path ./DEBIAN -prune -o -type f -print0 |
    LC_ALL=C sort -z | xargs -0 openssl dgst -md5 -r |
    sed 's@ \*\./@  @' > DEBIAN/md5sums.new
  mv DEBIAN/md5sums.new DEBIAN/md5sums
)
"$TASK_DPKG_DEB" --root-owner-group --build "$TASK_PACKAGE" "$TASK_STAGE/NativeJet-linux-$TASK_ARCH.deb"
mv -f "$TASK_STAGE/NativeJet-linux-$TASK_ARCH.deb" "build/NativeJet-linux-$TASK_ARCH.deb"
echo "build/NativeJet-linux-$TASK_ARCH.deb"
