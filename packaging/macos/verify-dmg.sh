#!/usr/bin/env bash
# Checks the .dmg `make package-macos` writes, without launching the app:
# image checksum, layout (the app and an /Applications link), a universal
# arm64 + x86_64 executable built for MACOS_MIN, the Info.plist values the
# Makefile fills in from wails.json, the ad-hoc signature over the whole
# bundle, and the remote servers inside it. macOS only.
#
#   verify-dmg.sh DMG VERSION [MACOS_MIN]
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    printf 'Usage: %s DMG VERSION [MACOS_MIN]\n' "$0" >&2
    exit 2
fi
dmg=$1 version=$2 macos_min=${3:-12.0}
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)

fail() {
    printf 'verify-dmg: %s\n' "$*" >&2
    exit 1
}

for tool in hdiutil plutil lipo otool codesign jq; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool not found"
done
[ -f "$dmg" ] || fail "not found: $dmg"

hdiutil verify -quiet "$dmg" || fail "hdiutil verify failed: $dmg"

mnt=$(mktemp -d "${TMPDIR:-/tmp}/verify-dmg.XXXXXX")
detach() {
    local i
    for i in 1 2 3; do
        hdiutil detach -quiet "$mnt" 2>/dev/null && break
        sleep 2
    done
    mount | grep -F " on $mnt " >/dev/null && hdiutil detach -quiet -force "$mnt" || true
    rmdir "$mnt" 2>/dev/null || true
}
hdiutil attach -quiet -readonly -nobrowse -noautoopen -mountpoint "$mnt" "$dmg"
trap detach EXIT

product=$(jq -r '.info.productName' "$root/wails.json")
app="$mnt/$product.app"
[ -d "$app" ] || fail "missing $product.app at the image root"
[ -L "$mnt/Applications" ] && [ "$(readlink "$mnt/Applications")" = /Applications ] ||
    fail "missing Applications -> /Applications link at the image root"

plist="$app/Contents/Info.plist"
plutil -lint -s "$plist" || fail "invalid Info.plist"
expect_key() { # KEY VALUE
    local got
    got=$(plutil -extract "$1" raw -o - "$plist" 2>/dev/null) || fail "Info.plist has no $1"
    [ "$got" = "$2" ] || fail "Info.plist $1 is '$got', expected '$2'"
}
expect_key CFBundlePackageType APPL
expect_key CFBundleExecutable agent-sessions
# Keep in step with packaging/macos/Info.plist.in: changing it resets the
# permissions users granted.
expect_key CFBundleIdentifier com.wails.agent-sessions
expect_key CFBundleName "$product"
expect_key CFBundleShortVersionString "$version"
expect_key CFBundleVersion "$version"
expect_key LSMinimumSystemVersion "$macos_min"
expect_key CFBundleIconFile iconfile
usage=$(plutil -extract NSAppleEventsUsageDescription raw -o - "$plist" 2>/dev/null) || usage=''
[ -n "$usage" ] || fail "Info.plist lacks NSAppleEventsUsageDescription: Open in terminal would fail with -1743"
[ -s "$app/Contents/Resources/iconfile.icns" ] || fail "missing Contents/Resources/iconfile.icns"

exe="$app/Contents/MacOS/agent-sessions"
[ -f "$exe" ] && [ -x "$exe" ] || fail "missing executable $exe"
archs=$(lipo -archs "$exe" | tr ' ' '\n' | grep . | LC_ALL=C sort | paste -sd ' ' -)
[ "$archs" = "arm64 x86_64" ] || fail "executable architectures are '$archs', expected 'arm64 x86_64'"
for arch in arm64 x86_64; do
    minos=$(otool -arch "$arch" -l "$exe" | awk '/LC_BUILD_VERSION/ {f = 1} f && $1 == "minos" {print $2; exit}')
    [ "$minos" = "$macos_min" ] || fail "$arch slice targets macOS '$minos', expected $macos_min"
done

# --strict also rejects files added to the bundle after signing.
codesign --verify --deep --strict --verbose=2 "$app" || fail "codesign verification failed"
details=$(codesign -d --verbose=2 "$app" 2>&1)
grep -qx 'Signature=adhoc' <<<"$details" || fail "expected an ad-hoc signature: $details"
grep -qx 'Identifier=com.wails.agent-sessions' <<<"$details" || fail "signature identifier differs: $details"

# The servers' own SHA256SUMS ships too; --run-host checks the version stamp.
bash "$root/packaging/verify-remote-servers.sh" --require-sums --run-host "$version" \
    "$app/Contents/Resources/remote"

printf 'verify-dmg: %s OK (%s, %s, macOS %s+, ad-hoc signed)\n' "$dmg" "$version" "$archs" "$macos_min"
