#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  build-essential cmake ninja-build pkg-config \
  qt6-base-dev qt6-base-dev-tools \
  libxml2-dev libxslt1-dev zlib1g-dev libssl-dev \
  libsnmp-dev dpkg-dev file ca-certificates

test "$(dpkg --print-architecture)" = arm64
cmake -S . -B build-ubuntu26-arm64 -G Ninja \
  -DUseQt6=ON -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr \
  -DCMAKE_CXX_FLAGS=-Wno-error=deprecated-declarations
cmake --build build-ubuntu26-arm64 --parallel 2

stage="$(mktemp -d)"
DESTDIR="$stage" cmake --install build-ubuntu26-arm64 --prefix /usr
test -x "$stage/usr/bin/fwbuilder"
readelf -h "$stage/usr/bin/fwbuilder" | grep -q AArch64

mkdir -p "$stage/DEBIAN" debian
cat > debian/control <<'EOF'
Source: fwbuilder
Section: net
Priority: optional
Maintainer: Firewall Builder contributors <noreply@github.com>
Standards-Version: 4.6.2

Package: fwbuilder
Architecture: any
Description: Firewall Builder Qt 6 GUI and firewall compilers
EOF
cat > "$stage/DEBIAN/control" <<'EOF'
Package: fwbuilder
Version: 6.0.0~rc1
Section: net
Priority: optional
Architecture: arm64
Maintainer: Firewall Builder contributors <noreply@github.com>
Description: Firewall Builder Qt 6 GUI and firewall compilers
EOF

dpkg-shlibdeps -O -e"$stage/usr/bin/fwbuilder" -e"$stage/usr/bin/fwb_ipt" \
  | sed -n 's/^shlibs:Depends=//p' > /tmp/fwb-deps
test -s /tmp/fwb-deps
qt_version="$(pkg-config --modversion Qt6Core)"
case "$qt_version" in
  6.10.*) ;;
  *) echo "Expected Ubuntu 26.04 Qt 6.10.x, got $qt_version" >&2; exit 1 ;;
esac
echo "Building against Qt $qt_version"
grep -Eq 'libqt6core6(t64)? \\(' /tmp/fwb-deps
grep -q 'libxml2-16' /tmp/fwb-deps
if grep -Eq 'libqt6[^ ,]*t64|libxml2 \(' /tmp/fwb-deps; then
  echo "Unexpected Ubuntu 24.04 dependencies:" >&2
  cat /tmp/fwb-deps >&2
  exit 1
fi
printf 'Depends: %s\n' "$(cat /tmp/fwb-deps)" >> "$stage/DEBIAN/control"
dpkg-deb --build "$stage" /workspace/fwbuilder-6.0.0-ubuntu26.04-arm64.deb
dpkg-deb -f /workspace/fwbuilder-6.0.0-ubuntu26.04-arm64.deb Depends
