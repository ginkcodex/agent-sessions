#!/usr/bin/env bash
# Checks the merged release artifacts in DIR and writes DIR/SHA256SUMS over
# all of them. DIR must hold exactly the release set for VERSION: a .deb and
# an .rpm per Linux architecture, the macOS .dmg, the Windows .msi and
# _setup.exe, and the gzipped servers with the SHA256SUMS they were built
# with, which must still match after the artifact round trip. That list is
# replaced by the aggregate one, as the release has always published.
#
#   release-checksums.sh DIR VERSION
set -euo pipefail

if [ "$#" -ne 2 ]; then
    printf 'Usage: %s DIR VERSION\n' "$0" >&2
    exit 2
fi
dir=$1 version=$2
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

fail() {
    printf 'release-checksums: %s\n' "$*" >&2
    exit 1
}

[ -d "$dir" ] || fail "not a directory: $dir"
[ -f "$dir/SHA256SUMS" ] || fail "missing the servers' SHA256SUMS in $dir"
# The servers' list must match before it goes.
(cd "$dir" && sha256sum --check --strict --quiet SHA256SUMS) || fail "remote servers changed in transit"
rm -f -- "$dir/SHA256SUMS"
extra=$(find "$dir" -mindepth 1 \( -type d -o -type l \) -print -quit)
[ -z "$extra" ] || fail "unexpected directory or link in $dir: $extra"

# Every expected asset must be there, and nothing else. Linux names come
# from nfpm, which writes 1.2.0-beta.1 as 1.2.0~beta.1.
pkg_version=${version%%-*}
[ "$pkg_version" = "$version" ] || pkg_version+="~${version#*-}"
patterns=(
    "agent-sessions_${pkg_version}_amd64.deb"
    "agent-sessions_${pkg_version}_arm64.deb"
    "agent-sessions-${pkg_version}-1.x86_64.rpm"
    "agent-sessions-${pkg_version}-1.aarch64.rpm"
    "agent-sessions_${version}_macos_universal.dmg"
    "agent-sessions_${version}_windows_amd64.msi"
    "agent-sessions_${version}_windows_amd64_setup.exe"
)
for pair in $(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile"); do
    patterns+=("agent-sessions-cli-${pair%/*}-${pair#*/}.gz")
done
for want in "${patterns[@]}"; do
    [ -f "$dir/$want" ] && [ -s "$dir/$want" ] || fail "missing or empty release asset: $want"
done
for path in "$dir"/* "$dir"/.[!.]*; do
    [ -e "$path" ] || continue
    name=${path##*/} known=0
    for want in "${patterns[@]}"; do
        [ "$name" = "$want" ] && known=1
    done
    [ "$known" = 1 ] || fail "unexpected file in the release set: $name"
done

(cd "$dir" && sha256sum -- * > SHA256SUMS && sha256sum --check --strict --quiet SHA256SUMS)
printf 'release-checksums: %s assets for %s\n' "${#patterns[@]}" "$version"
cat "$dir/SHA256SUMS"
