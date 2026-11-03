#!/usr/bin/env bash
# Checks the .deb and .rpm `make package-linux` writes to DIST, without
# installing them: name, version, architecture and the runtime dependencies
# declared in packaging/nfpm.yaml; the exact file list and modes; and the
# payload itself (the desktop binary for ARCH, desktop entry, icon, remote
# servers). Both packages must carry byte-identical payloads. Needs
# dpkg-deb, rpm and bsdtar (apt install rpm libarchive-tools).
#
#   verify-packages.sh [--binary FILE] [--remote-sums SUMS] DIST VERSION ARCH
#
#   --binary FILE      the packaged /usr/bin/agent-sessions must equal FILE
#   --remote-sums SUMS the packaged servers must match SUMS
set -euo pipefail

usage() {
    printf 'Usage: %s [--binary FILE] [--remote-sums SUMS] DIST VERSION ARCH\n' "$0" >&2
    exit 2
}
binary='' remote_sums=''
while [ "$#" -gt 0 ]; do
    case $1 in
        --binary) [ "$#" -ge 2 ] || usage; binary=$2; shift 2 ;;
        --remote-sums) [ "$#" -ge 2 ] || usage; remote_sums=$2; shift 2 ;;
        -*) usage ;;
        *) break ;;
    esac
done
[ "$#" -eq 3 ] || usage
dist=$1 version=$2 arch=$3
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)

fail() {
    printf 'verify-packages: %s\n' "$*" >&2
    exit 1
}

for tool in dpkg-deb rpm bsdtar; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool not found: apt install rpm libarchive-tools"
done

one() { # PATTERN: the single file in DIST matching it
    local matches=("$dist"/$1)
    [ "${#matches[@]}" = 1 ] && [ -f "${matches[0]}" ] ||
        fail "expected exactly one $1 in $dist, found: ${matches[*]}"
    printf '%s\n' "${matches[0]}"
}
deb=$(one '*.deb')
rpm=$(one '*.rpm')

case $arch in
    amd64) rpm_arch=x86_64 elf_machine=3e00 ;;
    arm64) rpm_arch=aarch64 elf_machine=b700 ;;
    *) fail "unsupported ARCH $arch" ;;
esac
# nfpm writes a semver pre-release (1.2.0-beta.1) as 1.2.0~beta.1 in both
# formats, so it sorts before the final release.
pkg_version=${version%%-*}
[ "$pkg_version" = "$version" ] || pkg_version+="~${version#*-}"

# Runtime dependencies of one packager, from nfpm.yaml's overrides.
depends_of() {
    awk -v want="$1" '
        /^overrides:/ { o = 1; next }
        o && /^[^ #]/ { o = 0 }
        o && /^  [a-z]+:/ { sec = $1; sub(/:$/, "", sec); dep = 0; next }
        o && sec == want && /^    depends:/ { dep = 1; next }
        o && dep && /^      - / { print $2; next }
        o && dep && /^    [^ ]/ { dep = 0 }
    ' "$root/packaging/nfpm.yaml"
}

# Regular files and modes both packages must ship: nfpm.yaml's contents.
remote_files=()
for pair in $(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile"); do
    remote_files+=("/usr/lib/agent-sessions/remote/agent-sessions-cli-${pair%/*}-${pair#*/}.gz")
done
expected_files=$(
    {
        echo '-rwxr-xr-x /usr/bin/agent-sessions'
        echo '-rw-r--r-- /usr/share/applications/io.github.ginkcode.AgentSessions.desktop'
        echo '-rw-r--r-- /usr/share/pixmaps/agent-sessions.png'
        printf -- '-rw-r--r-- %s\n' "${remote_files[@]}"
    } | LC_ALL=C sort -k2
)

tmp=$(mktemp -d "${TMPDIR:-/tmp}/verify-packages.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT

# --- .deb ---------------------------------------------------------------
field() { dpkg-deb --field "$deb" "$1"; }
[ "$(field Package)" = agent-sessions ] || fail "$deb: Package is '$(field Package)'"
[ "$(field Version)" = "$pkg_version" ] || fail "$deb: Version is '$(field Version)', expected '$pkg_version'"
[ "$(field Architecture)" = "$arch" ] || fail "$deb: Architecture is '$(field Architecture)', expected '$arch'"
deb_depends=$(field Depends | tr ',' '\n' | sed 's/^ *//; s/[ (].*//' | grep . | LC_ALL=C sort)
want_depends=$(depends_of deb | LC_ALL=C sort)
[ -n "$want_depends" ] || fail "no deb depends found in packaging/nfpm.yaml"
[ "$deb_depends" = "$want_depends" ] ||
    fail "$deb: Depends [$(echo $deb_depends)], expected [$(echo $want_depends)]"
deb_files=$(dpkg-deb --contents "$deb" | awk '$1 ~ /^-/ {f = $6; sub(/^\./, "", f); print $1, f}' | LC_ALL=C sort -k2)
[ "$deb_files" = "$expected_files" ] ||
    fail "$deb: files differ from nfpm.yaml:"$'\n'"$(diff <(echo "$expected_files") <(echo "$deb_files") || true)"
mkdir "$tmp/deb"
dpkg-deb --extract "$deb" "$tmp/deb"

# --- .rpm ---------------------------------------------------------------
rpm_query() { rpm -qp --nosignature --queryformat "$1" "$rpm"; }
got=$(rpm_query '%{NAME} %{VERSION} %{RELEASE} %{ARCH} %{OS}')
want="agent-sessions ${pkg_version//-/_} 1 $rpm_arch linux"
[ "$got" = "$want" ] || fail "$rpm: '$got', expected '$want'"
rpm_requires=$(rpm -qp --nosignature --requires "$rpm" | awk '$1 !~ /^rpmlib\(/ {print $1}' | LC_ALL=C sort -u)
want_requires=$(depends_of rpm | LC_ALL=C sort)
[ -n "$want_requires" ] || fail "no rpm depends found in packaging/nfpm.yaml"
[ "$rpm_requires" = "$want_requires" ] ||
    fail "$rpm: Requires [$(echo $rpm_requires)], expected [$(echo $want_requires)]"
rpm_files=$(rpm_query '[%{FILEMODES:perms} %{FILENAMES}\n]' | awk '$1 ~ /^-/' | LC_ALL=C sort -k2)
[ "$rpm_files" = "$expected_files" ] ||
    fail "$rpm: files differ from nfpm.yaml:"$'\n'"$(diff <(echo "$expected_files") <(echo "$rpm_files") || true)"
mkdir "$tmp/rpm"
# bsdtar reads the rpm payload directly; it notes that it strips the leading /.
bsdtar -xf "$rpm" -C "$tmp/rpm"

# --- payload ------------------------------------------------------------
while read -r _ path; do
    cmp -s "$tmp/deb$path" "$tmp/rpm$path" || fail "$path differs between the .deb and the .rpm"
done <<<"$expected_files"

exe="$tmp/deb/usr/bin/agent-sessions"
header=$(head -c 20 "$exe" | od -An -v -tx1 | tr -d ' \n')
[ "${header:0:10}" = 7f454c4602 ] && [ "${header:36:4}" = "$elf_machine" ] ||
    fail "/usr/bin/agent-sessions is not a 64-bit linux/$arch ELF (header $header)"
if [ -n "$binary" ]; then
    cmp -s "$exe" "$binary" || fail "/usr/bin/agent-sessions differs from $binary"
fi
cmp -s "$tmp/deb/usr/share/applications/io.github.ginkcode.AgentSessions.desktop" "$root/packaging/linux/io.github.ginkcode.AgentSessions.desktop" ||
    fail "desktop entry differs from packaging/linux/io.github.ginkcode.AgentSessions.desktop"
cmp -s "$tmp/deb/usr/share/pixmaps/agent-sessions.png" "$root/frontend/assets/Icon-universal.png" ||
    fail "icon differs from frontend/assets/Icon-universal.png"
remote_args=()
[ -z "$remote_sums" ] || remote_args=(--reference "$remote_sums")
bash "$root/packaging/verify-remote-servers.sh" ${remote_args[@]+"${remote_args[@]}"} "$tmp/deb/usr/lib/agent-sessions/remote"

printf 'verify-packages: %s and %s OK (%s, %s)\n' "${deb##*/}" "${rpm##*/}" "$pkg_version" "$arch"
