#!/usr/bin/env bash
set -Eeuo pipefail

# Firewall Builder 6 - native Apple Silicon / Homebrew Qt 5 or Qt 6
# --build (default): configure + incremental build
# --install: build + install + ad-hoc sign development bundle
# --dmg: build + install + standalone deployment + sign + DMG
QT_VERSION=5
MODE=--build
for arg in "$@"; do
  case "$arg" in
    --qt5) QT_VERSION=5 ;;
    --qt6) QT_VERSION=6 ;;
    --configure-only|--build|--install|--dmg) MODE="$arg" ;;
    *) echo "Usage: $0 [--qt5|--qt6] [--configure-only|--build|--install|--dmg]" >&2; exit 2 ;;
  esac
done
case "$MODE" in
  --configure-only|--build|--install|--dmg) ;;
  *) echo "Usage: $0 [--configure-only|--build|--install|--dmg]" >&2; exit 2 ;;
esac
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$QT_VERSION" == 6 ]]; then BUILD="$ROOT/build-arm64-qt6"; else BUILD="$ROOT/build-arm64"; fi
PREFIX="$BUILD/install"
RELEASE="$BUILD/release"
fail() { echo "ERROR: $*" >&2; exit 1; }
info() { printf '\n==> %s\n' "$*"; }

[[ "$(uname -s)" == "Darwin" ]] || fail "macOS required"
[[ "$(uname -m)" == "arm64" ]] || fail "Native ARM64 shell required (not Rosetta)"
[[ -f "$ROOT/CMakeLists.txt" ]] || fail "Put this script in the repository root"
xcode-select -p >/dev/null 2>&1 || fail "Install Xcode Command Line Tools: xcode-select --install"
command -v brew >/dev/null || fail "Homebrew missing"
BREW_PREFIX="$(brew --prefix)"
[[ "$BREW_PREFIX" == "/opt/homebrew" ]] || fail "Expected ARM64 Homebrew at /opt/homebrew"

info "Checking build dependencies"
PACKAGES=(cmake ninja pkgconf libxml2 libxslt net-snmp openssl@3 brotli)
if [[ "$QT_VERSION" == 5 ]]; then PACKAGES+=(qt@5); else PACKAGES+=(qtbase qttools); fi
for pkg in "${PACKAGES[@]}"; do
  if ! brew list --versions "$pkg" >/dev/null 2>&1; then brew install "$pkg"; fi
done
if [[ "$QT_VERSION" == 5 ]]; then
  QT_PREFIX="$(brew --prefix qt@5)"
  QT_DEPLOY="$QT_PREFIX/bin/macdeployqt"
else
  QT_PREFIX="$(brew --prefix qtbase)"
  QT_DEPLOY=""
  for candidate in \
    "$QT_PREFIX/bin/macdeployqt" \
    "$(brew --prefix qttools)/bin/macdeployqt" \
    "$(brew --prefix qttools)/libexec/macdeployqt" \
    "$BREW_PREFIX/bin/macdeployqt"; do
    if [[ -x "$candidate" ]]; then QT_DEPLOY="$candidate"; break; fi
  done
fi
XML_PREFIX="$(brew --prefix libxml2)"
XSLT_PREFIX="$(brew --prefix libxslt)"
SNMP_PREFIX="$(brew --prefix net-snmp)"
SSL_PREFIX="$(brew --prefix openssl@3)"
export PATH="$QT_PREFIX/bin:$BREW_PREFIX/bin:$PATH"
export PKG_CONFIG_PATH="$XML_PREFIX/lib/pkgconfig:$XSLT_PREFIX/lib/pkgconfig:$SNMP_PREFIX/lib/pkgconfig:$SSL_PREFIX/lib/pkgconfig:${PKG_CONFIG_PATH:-}"
export CMAKE_PREFIX_PATH="$QT_PREFIX;$XML_PREFIX;$XSLT_PREFIX;$SNMP_PREFIX;$SSL_PREFIX"
[[ -f "$QT_PREFIX/lib/cmake/Qt${QT_VERSION}/Qt${QT_VERSION}Config.cmake" ]] || fail "Qt${QT_VERSION}Config.cmake missing"
if [[ "$MODE" == "--dmg" ]]; then
  [[ -n "$QT_DEPLOY" && -x "$QT_DEPLOY" ]] || fail "macdeployqt missing in Qt installation (qtbase/qttools); check: brew list qtbase | grep macdeployqt"
fi

info "Configuring with Qt $QT_VERSION"
cmake -S "$ROOT" -B "$BUILD" -G Ninja \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DCMAKE_OSX_DEPLOYMENT_TARGET=$([[ "$QT_VERSION" == 6 ]] && echo 12.0 || echo 10.9) \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_PREFIX_PATH="$CMAKE_PREFIX_PATH" \
  -DUseQt6=$([[ "$QT_VERSION" == 6 ]] && echo ON || echo OFF) \
  -DCMAKE_CXX_STANDARD=$([[ "$QT_VERSION" == 6 ]] && echo 17 || echo 11) \
  -DQt${QT_VERSION}_DIR="$QT_PREFIX/lib/cmake/Qt${QT_VERSION}" \
  -DOPENSSL_ROOT_DIR="$SSL_PREFIX" \
  -DCMAKE_EXPORT_COMPILE_COMMANDS=ON
[[ "$MODE" == "--configure-only" ]] && exit 0

info "Incremental build"
cmake --build "$BUILD" --parallel "$(sysctl -n hw.ncpu)"
[[ "$MODE" == "--build" ]] && exit 0

info "Installing"
cmake --install "$BUILD"
APP="$PREFIX/fwbuilder.app"
[[ -d "$APP" ]] || fail "Missing $APP"

# Keep development install behavior, including the known Brotli fix.
FRAMEWORKS="$APP/Contents/Frameworks"
mkdir -p "$FRAMEWORKS"
BROTLI_COMMON="$(brew --prefix brotli)/lib/libbrotlicommon.1.dylib"
if [[ -f "$FRAMEWORKS/libbrotlidec.1.dylib" ]]; then
  cp -f "$BROTLI_COMMON" "$FRAMEWORKS/libbrotlicommon.1.dylib"
  chmod u+w "$FRAMEWORKS/libbrotlidec.1.dylib" "$FRAMEWORKS/libbrotlicommon.1.dylib"
  install_name_tool -change '@rpath/libbrotlicommon.1.dylib' \
    '@loader_path/libbrotlicommon.1.dylib' "$FRAMEWORKS/libbrotlidec.1.dylib" 2>/dev/null || true
fi

if [[ "$MODE" == "--install" ]]; then
  info "Signing development bundle"
  codesign --force --deep --sign - "$APP"
  codesign --verify --deep --strict --verbose=2 "$APP"
  info "Installed: $APP"
  exit 0
fi

info "Preparing isolated release copy"
mkdir -p "$RELEASE"
RELEASE_APP="$RELEASE/fwbuilder.app"
rm -rf "$RELEASE_APP"
ditto "$APP" "$RELEASE_APP"

info "Deploying Qt frameworks/plugins (already-deployed warning is harmless)"
"$QT_DEPLOY" "$RELEASE_APP" -no-strip -verbose=1 || \
  echo "WARNING: macdeployqt returned nonzero; checking complete dependency graph next"
[[ -f "$RELEASE_APP/Contents/PlugIns/platforms/libqcocoa.dylib" ]] || \
  fail "Qt Cocoa platform plugin missing after macdeployqt"

info "Resolving and embedding all non-system Mach-O dependencies"
# Python is part of macOS Command Line Tools. This embedded resolver works on the
# release copy only. It handles transitive dylibs and Qt frameworks, and rewrites
# all references to loader-relative paths (no dependence on Homebrew at runtime).
/usr/bin/python3 - "$RELEASE_APP" "$BREW_PREFIX" "$QT_PREFIX" <<'PY'
import os, sys, shutil, subprocess
from pathlib import Path

app = Path(sys.argv[1]).resolve()
brew = Path(sys.argv[2])
qt = Path(sys.argv[3])
contents = app / 'Contents'
frameworks = contents / 'Frameworks'
frameworks.mkdir(parents=True, exist_ok=True)

def cmd(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE)

def macho(p):
    try:
        return 'Mach-O' in cmd('/usr/bin/file', '-b', str(p))
    except (subprocess.CalledProcessError, OSError):
        return False

def files():
    return [p for p in contents.rglob('*') if p.is_file() and not p.is_symlink() and macho(p)]

def deps(p):
    lines = cmd('/usr/bin/otool', '-L', str(p)).splitlines()[1:]
    return [line.strip().split(' (', 1)[0] for line in lines if line.strip()]

def install_id(p):
    try:
        x = cmd('/usr/bin/otool', '-D', str(p)).splitlines()
        return x[1].strip() if len(x) > 1 else None
    except subprocess.CalledProcessError:
        return None

def inside(p):
    try: return p.resolve().is_relative_to(contents)
    except AttributeError:
        try: p.resolve().relative_to(contents); return True
        except ValueError: return False

def target_for(ref, loader):
    # Bundle-relative references are preferred over any installed brew copy.
    if ref.startswith('@loader_path/'):
        return (loader.parent / ref[len('@loader_path/'):]).resolve()
    if ref.startswith('@executable_path/'):
        return (contents / 'MacOS' / ref[len('@executable_path/'):]).resolve()
    if ref.startswith('@rpath/'):
        tail = ref[len('@rpath/'):]
        candidates = [frameworks / tail, frameworks / Path(tail).name]
        if '.framework/' in tail:
            candidates += [frameworks / tail[tail.index('.framework/') + len('.framework/'):]]
        for p in candidates:
            if p.exists(): return p.resolve()
        return None
    return Path(ref).resolve() if ref.startswith('/') else None

def find_source(ref):
    # Resolve external @rpath references against Homebrew Qt and its dependencies.
    tail = ref.split('/', 1)[1] if ref.startswith('@rpath/') else ref
    if '.framework/' in tail:
        name = tail.split('.framework/', 1)[0].split('/')[-1]
        suffix = tail.split('.framework/', 1)[1]
        for base in (qt / 'lib', brew / 'lib'):
            p = base / (name + '.framework') / suffix
            if p.exists(): return p
    name = Path(tail).name
    for base in (qt / 'lib', brew / 'lib', brew / 'opt'):
        p = base / name
        if p.is_file(): return p
    for p in (brew / 'opt').glob('*/lib/' + name):
        if p.is_file(): return p
    return None

def embed(src):
    src = src.resolve()
    parts = src.parts
    fwparts = [i for i, part in enumerate(parts) if part.endswith('.framework')]
    if fwparts:
        i = fwparts[-1]
        name = parts[i]
        dst_fw = frameworks / name
        if not dst_fw.exists():
            shutil.copytree(Path(*parts[:i+1]), dst_fw, symlinks=True)
            print('  framework:', name)
        rel = Path(*parts[i+1:])
        return (dst_fw / rel).resolve()
    dst = frameworks / src.name
    if not dst.exists():
        shutil.copy2(src, dst)
        print('  dylib:', src.name)
    return dst.resolve()

# Scan repeatedly because newly copied libraries have their own dependencies.
for iteration in range(40):
    changed = False
    for p in files():
        own_id = install_id(p)
        for dep in deps(p):
            if dep == own_id: continue
            if dep.startswith(('/usr/lib/', '/System/Library/')): continue
            resolved = target_for(dep, p)
            if resolved is None or not resolved.exists():
                resolved = find_source(dep)
            if resolved is None or not resolved.exists():
                raise SystemExit(f'Unresolved dependency: {p}: {dep}')
            if not inside(resolved):
                resolved = embed(resolved)
                changed = True
            if not resolved.exists():
                raise SystemExit(f'Missing bundled dependency: {p}: {dep}')
            new = '@loader_path/' + os.path.relpath(resolved, p.parent)
            if dep != new:
                os.chmod(p, p.stat().st_mode | 0o200)
                subprocess.check_call(['/usr/bin/install_name_tool', '-change', dep, new, str(p)])
                changed = True
        # A plugin/dylib's own ID is not a dependency, but normalize external IDs.
        if own_id and own_id.startswith(str(brew)):
            os.chmod(p, p.stat().st_mode | 0o200)
            subprocess.check_call(['/usr/bin/install_name_tool', '-id',
                                   '@rpath/' + p.name, str(p)])
            changed = True
    if not changed: break
else:
    raise SystemExit('Dependency resolution did not converge')

# Validate *all* Mach-O dependencies, including @rpath and @loader_path.
errors = []
for p in files():
    own_id = install_id(p)
    for dep in deps(p):
        if dep == own_id or dep.startswith(('/usr/lib/', '/System/Library/')):
            continue
        resolved = target_for(dep, p)
        if not resolved or not resolved.exists() or not inside(resolved):
            errors.append(f'{p}: {dep}')
if errors:
    raise SystemExit('Unresolved or external dependencies:\n' + '\n'.join(errors))
print('PASS: all', len(files()), 'Mach-O files have bundle-local or system dependencies')
PY

info "Signing release app after all Mach-O modifications"
codesign --force --deep --sign - "$RELEASE_APP"
codesign --verify --deep --strict --verbose=2 "$RELEASE_APP"

info "Creating compressed DMG"
DMGROOT="$RELEASE/dmg"
rm -rf "$DMGROOT"
mkdir -p "$DMGROOT"
ditto "$RELEASE_APP" "$DMGROOT/fwbuilder.app"
ln -s /Applications "$DMGROOT/Applications"
DMG="$RELEASE/FirewallBuilder-6.0-qt${QT_VERSION}-arm64.dmg"
hdiutil create -volname 'Firewall Builder 6.0' -srcfolder "$DMGROOT" \
  -format UDZO -ov "$DMG"
info "DMG ready: $DMG"
echo 'NOTE: Ad-hoc signing is not Developer ID signing/notarization.'
