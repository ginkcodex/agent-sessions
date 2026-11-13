#!/usr/bin/env bash
# Writes DIST_DIR/repo-meta.json, the index entries of this release's Linux
# packages, for the website Worker that builds the apt and dnf repositories
# (docs/plan/linux-repos.md). The Worker cannot run apt-ftparchive, so the
# release does it here, one package at a time, as the file will be listed:
# under pool/<version>/<file name>.
#
#   {"version": "1.5.5",
#    "deb": [{"arch", "file", "size", "sha256", "stanza"}, ...],
#    "rpm": []}
#
# stanza is the package's Packages paragraph, without the blank line after
# it. Entries are in file name order. Needs apt-ftparchive (apt-utils), jq
# and sha256sum.
#
#   repo-meta.sh VERSION DIST_DIR
set -euo pipefail
export LC_ALL=C

if [ "$#" -ne 2 ]; then
    printf 'Usage: %s VERSION DIST_DIR\n' "$0" >&2
    exit 2
fi
version=$1 dist=$2

fail() {
    printf 'repo-meta: %s\n' "$*" >&2
    exit 1
}

# field NAME: the value of NAME in $stanza.
field() { printf '%s\n' "$stanza" | sed -n "s/^$1: //p"; }

case $version in
    '' | */* | .* | *[[:space:]]*) fail "not a release version: '$version'" ;;
esac
[ -d "$dist" ] || fail "not a directory: $dist"
for tool in apt-ftparchive jq sha256sum; do
    command -v "$tool" >/dev/null || fail "$tool is not installed"
done

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
pool="pool/$version"
entries=$work/deb.jsonl
: > "$entries"

shopt -s nullglob
debs=("$dist"/*.deb)
shopt -u nullglob
[ "${#debs[@]}" -gt 0 ] || fail "no .deb in $dist"

# In the C locale, glob order is file name order.
for deb in "${debs[@]}"; do
    file=${deb##*/}
    # apt-ftparchive names a file by its path from where it runs, so it gets
    # a folder of its own laid out as the repository will be.
    one=$work/one
    rm -rf -- "$one"
    mkdir -p -- "$one/$pool"
    cp -- "$deb" "$one/$pool/$file"
    stanza=$(cd "$one" && apt-ftparchive packages "$pool")
    # One paragraph, for this file. $(...) drops the trailing blank line.
    [ -n "$stanza" ] || fail "$file: apt-ftparchive wrote nothing"
    case $stanza in
        *$'\n\n'*) fail "$file: apt-ftparchive wrote more than one paragraph" ;;
    esac

    size=$(wc -c < "$deb" | tr -d ' ')
    sha256=$(sha256sum -- "$deb" | cut -d' ' -f1)
    arch=$(field Architecture)
    [ "$(field Filename)" = "$pool/$file" ] || fail "$file: Filename is '$(field Filename)'"
    [ "$(field Size)" = "$size" ] || fail "$file: Size is '$(field Size)', the file has $size bytes"
    [ "$(field SHA256)" = "$sha256" ] || fail "$file: SHA256 differs from the file's"
    [[ -n $(field Package) && -n $(field Version) && -n $arch ]] ||
        fail "$file: the paragraph lacks Package, Version or Architecture"

    jq -cn --arg arch "$arch" --arg file "$file" --argjson size "$size" \
        --arg sha256 "$sha256" --arg stanza "$stanza" \
        '{arch: $arch, file: $file, size: $size, sha256: $sha256, stanza: $stanza}' >> "$entries"
done

out=$dist/repo-meta.json
jq -s --arg version "$version" '{version: $version, deb: ., rpm: []}' "$entries" > "$work/repo-meta.json"
mv -- "$work/repo-meta.json" "$out"
printf 'repo-meta: %s: %s .deb\n' "$out" "${#debs[@]}"
