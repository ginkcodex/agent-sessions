#!/usr/bin/env bash
# Writes DIST_DIR/repo-meta.json, the index entries of this release's Linux
# packages, for the website Worker that builds the apt and dnf repositories
# (docs/plan/linux-repos.md). The Worker cannot run apt-ftparchive or
# createrepo_c, so the release does it here, one package at a time, as the
# file will be listed: under pool/<version>/<file name>.
#
#   {"version": "1.5.5",
#    "deb": [{"arch", "file", "size", "sha256", "stanza"}, ...],
#    "rpm": [{"arch", "file", "size", "sha256", "primary", "filelists", "other"}, ...]}
#
# stanza is the package's Packages paragraph, without the blank line after
# it. primary, filelists and other are its <package> element in each of
# createrepo_c's files, from the line that opens it to the one that closes
# it. Entries are in file name order. Needs jq and sha256sum, apt-ftparchive
# (apt-utils) for .deb files and createrepo_c for .rpm files. The packages
# are only read.
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
need() {
    command -v "$1" >/dev/null || fail "$1 is not installed"
}
need jq
need sha256sum

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
pool="pool/$version"
: > "$work/deb.jsonl"
: > "$work/rpm.jsonl"

# In the C locale, glob order is file name order.
shopt -s nullglob
debs=("$dist"/*.deb)
rpms=("$dist"/*.rpm)
shopt -u nullglob
[ "$((${#debs[@]} + ${#rpms[@]}))" -gt 0 ] || fail "no .deb or .rpm in $dist"
[ "${#debs[@]}" -eq 0 ] || need apt-ftparchive
[ "${#rpms[@]}" -eq 0 ] || need createrepo_c

# place FILE: a scratch folder holding only FILE, at $pool/<its name>, as the
# repository will lay it out. Prints the folder.
place() {
    local one=$work/one
    rm -rf -- "$one"
    mkdir -p -- "$one/$pool"
    cp -- "$1" "$one/$pool/"
    printf '%s\n' "$one"
}

for deb in "${debs[@]}"; do
    file=${deb##*/}
    # apt-ftparchive names a file by its path from where it runs.
    one=$(place "$deb")
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
        '{arch: $arch, file: $file, size: $size, sha256: $sha256, stanza: $stanza}' >> "$work/deb.jsonl"
done

# element NAME: the one <package> element of repodata/NAME.xml.gz in $one.
element() {
    local xml
    xml=$(gzip -dc -- "$one/repodata/$1.xml.gz")
    [[ $(grep -c '^<package[ >]' <<< "$xml") = 1 && $(grep -c '^</package>$' <<< "$xml") = 1 ]] ||
        fail "$file: $1.xml does not hold one <package>"
    sed -n '/^<package[ >]/,/^<\/package>$/p' <<< "$xml"
}

for rpm in "${rpms[@]}"; do
    file=${rpm##*/}
    one=$(place "$rpm")
    # <time file> is the file's mtime, which the download set. It becomes
    # the package's build time, so the same package gives the same entry.
    for pass in 1 2; do
        rm -rf -- "$one/repodata"
        createrepo_c --quiet --no-database --simple-md-filenames \
            --general-compress-type=gz --checksum=sha256 "$one" >&2
        primary=$(element primary)
        [ "$pass" = 2 ] && break
        build=$(sed -n 's|^  <time file="[0-9]*" build="\([0-9]*\)"/>$|\1|p' <<< "$primary")
        [ -n "$build" ] || fail "$file: primary.xml has no build time"
        touch -d "@$build" -- "$one/$pool/$file"
    done
    filelists=$(element filelists)
    other=$(element other)

    size=$(wc -c < "$rpm" | tr -d ' ')
    sha256=$(sha256sum -- "$rpm" | cut -d' ' -f1)
    arch=$(sed -n 's|^  <arch>\([^<]*\)</arch>$|\1|p' <<< "$primary")
    grep -qxF "  <location href=\"$pool/$file\"/>" <<< "$primary" || fail "$file: primary.xml has another location"
    grep -qxF "  <checksum type=\"sha256\" pkgid=\"YES\">$sha256</checksum>" <<< "$primary" ||
        fail "$file: primary.xml has another checksum"
    grep -q "^  <size package=\"$size\" " <<< "$primary" || fail "$file: primary.xml has another size"
    for xml in "$filelists" "$other"; do
        grep -q "^<package pkgid=\"$sha256\" " <<< "$xml" || fail "$file: filelists.xml or other.xml names another package"
    done
    [ -n "$arch" ] || fail "$file: primary.xml has no arch"

    jq -cn --arg arch "$arch" --arg file "$file" --argjson size "$size" --arg sha256 "$sha256" \
        --arg primary "$primary" --arg filelists "$filelists" --arg other "$other" \
        '{arch: $arch, file: $file, size: $size, sha256: $sha256,
          primary: $primary, filelists: $filelists, other: $other}' >> "$work/rpm.jsonl"
done

out=$dist/repo-meta.json
jq -n --arg version "$version" --slurpfile deb "$work/deb.jsonl" --slurpfile rpm "$work/rpm.jsonl" \
    '{version: $version, deb: $deb, rpm: $rpm}' > "$work/repo-meta.json"
mv -- "$work/repo-meta.json" "$out"
printf 'repo-meta: %s: %s .deb, %s .rpm\n' "$out" "${#debs[@]}" "${#rpms[@]}"
