#!/usr/bin/env bash
# Checks the .dmg `make package-macos` writes, without launching the app:
# image checksum, layout (the app and an /Applications link), a universal
# arm64 + x86_64 executable built for MACOS_MIN, the Info.plist values the
# Makefile fills in from wails.json, the signature over the whole bundle,
# and the remote servers inside it. macOS only.
#
#   verify-dmg.sh [--developer-id TEAM [--notarized]] DMG VERSION [MACOS_MIN]
#
# By default the app must be ad-hoc signed. --developer-id TEAM expects
# `make package-macos MACOS_SIGN_ID=...` instead: the app and the .dmg signed
# by that team's Developer ID, the app with the hardened runtime and the
# Apple Events entitlement, and the darwin servers signed by it with the
# hardened runtime. --notarized also expects
# MACOS_NOTARY_PROFILE's stapled ticket, and Gatekeeper to accept both.
set -euo pipefail

usage() {
    printf 'Usage: %s [--developer-id TEAM [--notarized]] DMG VERSION [MACOS_MIN]\n' "$0" >&2
    exit 2
}
team='' notarized=''
while [ "$#" -gt 0 ]; do
    case $1 in
        --developer-id) [ "$#" -ge 2 ] || usage; team=$2; shift 2 ;;
        --notarized) notarized=1; shift ;;
        -*) usage ;;
        *) break ;;
    esac
done
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ] || { [ -n "$notarized" ] && [ -z "$team" ]; }; then
    usage
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
grep -qx 'Identifier=com.wails.agent-sessions' <<<"$details" || fail "signature identifier differs: $details"
if [ -z "$team" ]; then
    grep -qx 'Signature=adhoc' <<<"$details" || fail "expected an ad-hoc signature: $details"
    signed='ad-hoc signed'
else
    grep -qx "TeamIdentifier=$team" <<<"$details" || fail "expected team $team: $details"
    grep -q '^Authority=Developer ID Application: ' <<<"$details" || fail "expected a Developer ID signature: $details"
    grep -q '^Timestamp=' <<<"$details" || fail "signature has no secure timestamp: $details"
    grep -Eq '^CodeDirectory .*flags=0x[0-9a-f]*\(.*runtime' <<<"$details" || fail "hardened runtime is off: $details"
    entitlements=$(codesign -d --entitlements - --xml "$app" 2>/dev/null) || entitlements=''
    plutil -extract 'com\.apple\.security\.automation\.apple-events' raw -o - - <<<"$entitlements" 2>/dev/null | grep -qx true ||
        fail "missing the Apple Events entitlement: Open in terminal would fail with -1743"
    dmg_details=$(codesign -d --verbose=2 "$dmg" 2>&1) || fail "the .dmg is not signed: $dmg_details"
    grep -qx "TeamIdentifier=$team" <<<"$dmg_details" || fail "expected the .dmg signed by team $team: $dmg_details"
    # The notary service unpacks these and rejects unsigned executables.
    for gz in "$app"/Contents/Resources/remote/agent-sessions-cli-darwin-*.gz; do
        [ -f "$gz" ] || fail "no darwin servers in $app/Contents/Resources/remote"
        bin="$mnt.server"
        gunzip -c "$gz" >"$bin"
        server_details=$(codesign -d --verbose=2 "$bin" 2>&1) || server_details="unsigned: $server_details"
        rm -f "$bin"
        grep -qx "TeamIdentifier=$team" <<<"$server_details" || fail "$(basename "$gz") is not signed by team $team: $server_details"
        grep -q '^Timestamp=' <<<"$server_details" || fail "$(basename "$gz") has no secure timestamp: $server_details"
        grep -Eq '^CodeDirectory .*flags=0x[0-9a-f]*\(.*runtime' <<<"$server_details" ||
            fail "$(basename "$gz") lacks the hardened runtime: $server_details"
    done
    signed="Developer ID $team"
    if [ -n "$notarized" ]; then
        xcrun stapler validate -q "$dmg" || fail "no stapled notarization ticket on $dmg"
    fi
    # Gatekeeper's verdict, unless assessments are off on this machine.
    if [ -n "$notarized" ] && spctl --status 2>/dev/null | grep -q 'assessments disabled'; then
        printf 'verify-dmg: Gatekeeper assessments are disabled here; checked the stapled ticket only\n' >&2
    elif [ -n "$notarized" ]; then
        assess=$(spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg" 2>&1) ||
            fail "Gatekeeper rejects the .dmg: $assess"
        grep -q 'source=Notarized Developer ID' <<<"$assess" || fail "the .dmg is not notarized: $assess"
        assess=$(spctl --assess --type execute --verbose=2 "$app" 2>&1) || fail "Gatekeeper rejects the app: $assess"
        grep -q 'source=Notarized Developer ID' <<<"$assess" || fail "the app is not notarized: $assess"
        signed="$signed, notarized"
    fi
fi

# The servers' own SHA256SUMS ships too; --run-host checks the version stamp.
bash "$root/packaging/verify-remote-servers.sh" --require-sums --run-host "$version" \
    "$app/Contents/Resources/remote"

printf 'verify-dmg: %s OK (%s, %s, macOS %s+, %s)\n' "$dmg" "$version" "$archs" "$macos_min" "$signed"
