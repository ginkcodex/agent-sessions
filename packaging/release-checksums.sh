#!/usr/bin/env bash
# Checks the merged release artifacts in DIR and writes DIR/SHA256SUMS over
# all of them. DIR must hold exactly the release set for VERSION: a .deb and
# an .rpm per Linux architecture, the macOS .dmg, the Windows .msi and
# _setup.exe, the gzipped servers with the SHA256SUMS they were built with,
# which must still match after the artifact round trip, and repo-meta.json
# (packaging/linux/repo-meta.sh), which must describe the packages as they
# are. The servers' list is replaced by the aggregate one, as the release has
# always published.
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
    "repo-meta.json"
)
# shellcheck disable=SC2013 # the platforms are words on one line
for pair in $(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile"); do
    patterns+=("agent-sessions-cli-${pair%/*}-${pair#*/}.gz")
done
for want in "${patterns[@]}"; do
    [[ -f $dir/$want && -s $dir/$want ]] || fail "missing or empty release asset: $want"
done
for path in "$dir"/* "$dir"/.[!.]*; do
    [ -e "$path" ] || continue
    name=${path##*/} known=0
    for want in "${patterns[@]}"; do
        [ "$name" = "$want" ] && known=1
    done
    [ "$known" = 1 ] || fail "unexpected file in the release set: $name"
done

# The Linux repositories are built from repo-meta.json alone, so it must list
# exactly these packages, with their sizes and hashes, in the paragraphs apt
# and dnf read too.
meta=$dir/repo-meta.json
jq -e --arg v "$version" '.version == $v and (.deb | type) == "array" and (.rpm | type) == "array"' \
    "$meta" >/dev/null 2>&1 || fail "repo-meta.json is not for $version, or is malformed"
listed=$(jq -r '.deb[].file' "$meta" | LC_ALL=C sort)
want=$(printf '%s\n' "agent-sessions_${pkg_version}_amd64.deb" "agent-sessions_${pkg_version}_arm64.deb" | LC_ALL=C sort)
[ "$listed" = "$want" ] || fail "repo-meta.json lists other .deb files than the release's: ${listed//$'\n'/ }"
listed=$(jq -r '.rpm[].file' "$meta" | LC_ALL=C sort)
want=$(printf '%s\n' "agent-sessions-${pkg_version}-1.x86_64.rpm" "agent-sessions-${pkg_version}-1.aarch64.rpm" | LC_ALL=C sort)
[ "$listed" = "$want" ] || fail "repo-meta.json lists other .rpm files than the release's: ${listed//$'\n'/ }"
while IFS=$'\t' read -r file size sha256; do
    [ -f "$dir/$file" ] || fail "repo-meta.json lists $file, which is not in the release"
    actual_size=$(wc -c < "$dir/$file" | tr -d ' ')
    [ "$size" = "$actual_size" ] || fail "repo-meta.json: $file has $actual_size bytes, not $size"
    [ "$sha256" = "$(sha256sum -- "$dir/$file" | cut -d' ' -f1)" ] || fail "repo-meta.json: $file has another SHA256"
done < <(jq -r '(.deb + .rpm)[] | [.file, (.size | tostring), .sha256] | @tsv' "$meta")
bad=$(jq -r --arg v "$version" '.deb[] | . as $e | ($e.stanza | split("\n")) as $l
    | select(any($l[]; . == "Filename: pool/\($v)/\($e.file)") and any($l[]; . == "Size: \($e.size)")
        and any($l[]; . == "SHA256: \($e.sha256)") | not) | $e.file' "$meta")
[ -z "$bad" ] || fail "repo-meta.json: the paragraph of $bad disagrees with its file"
bad=$(jq -r --arg v "$version" '.rpm[] | . as $e | ($e.primary | split("\n")) as $l
    | select(any($l[]; . == "  <location href=\"pool/\($v)/\($e.file)\"/>")
        and any($l[]; . == "  <checksum type=\"sha256\" pkgid=\"YES\">\($e.sha256)</checksum>")
        and any($l[]; startswith("  <size package=\"\($e.size)\" "))
        and ($e.filelists | startswith("<package pkgid=\"\($e.sha256)\" "))
        and ($e.other | startswith("<package pkgid=\"\($e.sha256)\" ")) | not) | $e.file' "$meta")
[ -z "$bad" ] || fail "repo-meta.json: the metadata of $bad disagrees with its file"

(cd "$dir" && sha256sum -- * > SHA256SUMS && sha256sum --check --strict --quiet SHA256SUMS)
printf 'release-checksums: %s assets for %s\n' "${#patterns[@]}" "$version"
cat "$dir/SHA256SUMS"
