#!/usr/bin/env bash
# Writes the AUR package agent-sessions-bin for one published stable release
# into OUTDIR: PKGBUILD, filled from PKGBUILD.in, and the LICENSE it installs.
# The .deb checksums come from the release's SHA256SUMS. .SRCINFO needs
# makepkg, so verify-aur-package.sh writes it.
#
#   render-pkgbuild.sh [--pkgrel N] VERSION SHA256SUMS OUTDIR
#
# VERSION is x.y.z without the v. Pre-releases (x.y.z-beta.1) are refused:
# pacman versions cannot hold a '-', and the AUR follows the latest release.
set -euo pipefail

usage() {
    printf 'Usage: %s [--pkgrel N] VERSION SHA256SUMS OUTDIR\n' "$0" >&2
    exit 2
}
fail() {
    printf 'render-pkgbuild: %s\n' "$*" >&2
    exit 1
}

pkgrel=1
while [ "$#" -gt 0 ]; do
    case $1 in
        --pkgrel) [ "$#" -ge 2 ] || usage; pkgrel=$2; shift 2 ;;
        -*) usage ;;
        *) break ;;
    esac
done
[ "$#" -eq 3 ] || usage
version=$1 sums=$2 out=$3

[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
    fail "'$version' is not a stable x.y.z version (pre-releases are not published to the AUR)"
[[ $pkgrel =~ ^[1-9][0-9]*$ ]] || fail "--pkgrel must be a positive integer, got '$pkgrel'"
[ -f "$sums" ] || fail "not found: $sums"

here=$(cd -- "$(dirname -- "$0")" && pwd)
root=$(cd -- "$here/../.." && pwd)

# One "<sha256>  <name>" line per asset; sha256sum marks binary mode with '*'.
sum_of() {
    local name=$1 found
    found=$(awk -v n="$name" '{ f = $2; sub(/^\*/, "", f) } f == n { print $1 }' "$sums")
    [ -n "$found" ] || fail "$name is not listed in $sums"
    [ "$(wc -l <<<"$found")" -eq 1 ] || fail "$name is listed more than once in $sums"
    [[ $found =~ ^[0-9a-f]{64}$ ]] || fail "bad checksum for $name in $sums: $found"
    printf '%s' "$found"
}
sha_x86_64=$(sum_of "agent-sessions_${version}_amd64.deb")
sha_aarch64=$(sum_of "agent-sessions_${version}_arm64.deb")
sha_license=$(sha256sum "$root/LICENSE" | cut -d' ' -f1)

mkdir -p -- "$out"
sed -e "s/@PKGVER@/$version/g" \
    -e "s/@PKGREL@/$pkgrel/g" \
    -e "s/@SHA256_LICENSE@/$sha_license/g" \
    -e "s/@SHA256_X86_64@/$sha_x86_64/g" \
    -e "s/@SHA256_AARCH64@/$sha_aarch64/g" \
    "$here/PKGBUILD.in" > "$out/PKGBUILD"
grep -q '@[A-Z0-9_]*@' "$out/PKGBUILD" && fail "unfilled placeholder in $out/PKGBUILD"
cp -- "$root/LICENSE" "$out/LICENSE"
# A .SRCINFO from another version must not be published with this PKGBUILD.
rm -f -- "$out/.SRCINFO"

printf 'render-pkgbuild: %s/PKGBUILD for agent-sessions-bin %s-%s\n' "$out" "$version" "$pkgrel"
