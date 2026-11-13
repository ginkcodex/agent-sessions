#!/usr/bin/env bash
# Tests repo-meta.sh and the repo-meta.json checks of release-checksums.sh on
# a published release's two .deb files, in DEB_DIR with that release's
# SHA256SUMS. Runs on Linux with apt-ftparchive, jq and GNU coreutils;
# `make test-repo-meta` runs it in an Ubuntu container.
#
#   test-repo-meta.sh DEB_DIR VERSION
set -euo pipefail
export LC_ALL=C

if [ "$#" -ne 2 ]; then
    printf 'Usage: %s DEB_DIR VERSION\n' "$0" >&2
    exit 2
fi
debs=$1 version=$2
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(cd -- "$here/../.." && pwd)

failures=0
pass() { printf 'ok   %s\n' "$*"; }
fail() {
    printf 'FAIL %s\n' "$*" >&2
    failures=$((failures + 1))
}

# check WHAT CMD...: passes when CMD succeeds. refuses WHAT CMD...: when it fails.
check() {
    local what=$1
    shift
    if "$@"; then pass "$what"; else fail "$what"; fi
}
refuses() {
    local what=$1
    shift
    if "$@" 2>/dev/null; then fail "accepts $what"; else pass "refuses $what"; fi
}

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

files=("agent-sessions_${version}_amd64.deb" "agent-sessions_${version}_arm64.deb")
(cd "$debs" && grep -F -e "${files[0]}" -e "${files[1]}" SHA256SUMS | sha256sum --check --strict --quiet) ||
    { printf 'the .deb files do not match their SHA256SUMS\n' >&2; exit 1; }

# 1. The stanzas are apt-ftparchive's for the two files, apart from Filename.
dist=$work/dist
mkdir -p "$dist"
cp -- "${files[@]/#/$debs/}" "$dist/"
bash "$here/repo-meta.sh" "$version" "$dist" >/dev/null
meta=$dist/repo-meta.json

flat=$work/flat
mkdir -p "$flat"
cp -- "${files[@]/#/$debs/}" "$flat/"
(cd "$flat" && apt-ftparchive packages .) > "$work/Packages"
# One paragraph per file, keyed by its file name, without Filename.
paragraph() {
    awk -v want="./$1" 'BEGIN { RS = ""; FS = "\n" }
        { keep = 0; out = ""
          for (i = 1; i <= NF; i++) {
              if ($i == "Filename: " want) keep = 1
              else if ($i !~ /^Filename: /) out = out $i "\n"
          }
          if (keep) printf "%s", out }' "$work/Packages"
}
for file in "${files[@]}"; do
    want=$(paragraph "$file")
    got=$(jq -r --arg f "$file" '.deb[] | select(.file == $f) | .stanza' "$meta" | grep -v '^Filename: ')
    filename=$(jq -r --arg f "$file" '.deb[] | select(.file == $f) | .stanza' "$meta" | sed -n 's/^Filename: //p')
    if [ -n "$want" ] && [ "$got" = "$want" ]; then
        pass "$file: the stanza is apt-ftparchive's"
    else
        fail "$file: the stanza differs from apt-ftparchive's"
        diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2 || true
    fi
    if [ "$filename" = "pool/$version/$file" ]; then
        pass "$file: Filename is pool/$version/$file"
    else
        fail "$file: Filename is '$filename'"
    fi
done

# 2. The rest of each entry.
shape=$(jq -r --arg v "$version" '
    [.version == $v, (.deb | length) == 2, .rpm == [],
     (.deb | map(.file)) == (.deb | map(.file) | sort),
     (.deb | map(.arch) | sort) == ["amd64", "arm64"],
     all(.deb[]; (keys | sort) == ["arch", "file", "sha256", "size", "stanza"])] | all' "$meta")
check "version, arch, order and fields" [ "$shape" = true ]
for file in "${files[@]}"; do
    size=$(wc -c < "$dist/$file" | tr -d ' ')
    sha256=$(sha256sum -- "$dist/$file" | cut -d' ' -f1)
    entry=$(jq -c --arg f "$file" '.deb[] | select(.file == $f) | [.size, .sha256]' "$meta")
    check "$file: size and sha256" [ "$entry" = "[$size,\"$sha256\"]" ]
done
# Running it again writes the same file.
cp -- "$meta" "$work/first.json"
bash "$here/repo-meta.sh" "$version" "$dist" >/dev/null
check "the same packages give the same file" cmp -s "$meta" "$work/first.json"

# 3. repo-meta.sh refuses what it cannot describe.
mkdir -p "$work/empty"
refuses "a folder with no .deb" bash "$here/repo-meta.sh" "$version" "$work/empty"
refuses "a version with a slash" bash "$here/repo-meta.sh" "../$version" "$dist"

# 4. release-checksums.sh: a whole release set, with the real .deb files and
# stand-ins for the rest.
release_set() {
    local set=$1
    rm -rf -- "$set"
    mkdir -p "$set"
    cp -- "${files[@]/#/$dist/}" "$dist/repo-meta.json" "$set/"
    for name in "agent-sessions-${version}-1.x86_64.rpm" "agent-sessions-${version}-1.aarch64.rpm" \
        "agent-sessions_${version}_macos_universal.dmg" "agent-sessions_${version}_windows_amd64.msi" \
        "agent-sessions_${version}_windows_amd64_setup.exe"; do
        printf 'stand-in for %s\n' "$name" > "$set/$name"
    done
    local pair
    # shellcheck disable=SC2013 # the platforms are words on one line
    for pair in $(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile"); do
        printf 'server %s\n' "$pair" | gzip -n > "$set/agent-sessions-cli-${pair%/*}-${pair#*/}.gz"
    done
    (cd "$set" && sha256sum -- *.gz > SHA256SUMS)
}
checksums() { bash "$root/packaging/release-checksums.sh" "$1" "$version" > "$work/out" 2>&1; }
expect_refused() {
    local what=$1 want=$2
    if checksums "$set"; then
        fail "release-checksums accepts $what"
    elif grep -q -- "$want" "$work/out"; then
        pass "release-checksums refuses $what"
    else
        fail "release-checksums refuses $what, but says: $(cat "$work/out")"
    fi
}
set=$work/set

release_set "$set"
if checksums "$set" && grep -q ' repo-meta.json$' "$set/SHA256SUMS"; then
    pass "release-checksums accepts the set, and SHA256SUMS covers repo-meta.json"
else
    fail "release-checksums refuses a good set: $(cat "$work/out")"
fi

release_set "$set"
rm -- "$set/repo-meta.json"
expect_refused "a set without repo-meta.json" "missing or empty release asset: repo-meta.json"

release_set "$set"
jq '.deb[0].size += 1' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a wrong size" "bytes, not"

release_set "$set"
jq '.deb[1].sha256 = ("0" * 64)' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a wrong sha256" "has another SHA256"

release_set "$set"
jq '.deb[0].stanza |= sub("SHA256: [0-9a-f]+"; "SHA256: " + ("1" * 64))' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a stanza with another hash" "disagrees with its file"

release_set "$set"
jq --arg v "$version" '.deb[0].stanza |= sub("Filename: pool/[^\n]*"; "Filename: pool/\($v)/other.deb")' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a stanza with another Filename" "disagrees with its file"

release_set "$set"
jq '.deb |= .[:1]' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a meta that leaves out a .deb" "lists other .deb files"

release_set "$set"
jq '.version = "0.0.1"' "$dist/repo-meta.json" > "$set/repo-meta.json"
expect_refused "a meta for another version" "is not for $version"

release_set "$set"
printf 'not json' > "$set/repo-meta.json"
expect_refused "a meta that is not JSON" "is malformed"

release_set "$set"
printf 'changed' >> "$set/${files[1]}"
expect_refused "a package changed after repo-meta.sh" "bytes, not"

if [ "$failures" -gt 0 ]; then
    printf '%d check(s) failed\n' "$failures" >&2
    exit 1
fi
printf 'all repo-meta checks passed\n'
