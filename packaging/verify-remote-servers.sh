#!/usr/bin/env bash
# Checks a directory of gzipped headless servers as `make remote-servers`
# writes them, or as a package ships them: exactly one archive per
# REMOTE_PLATFORMS entry (read from the Makefile), each a sound gzip holding
# a 64-bit executable for its own OS and architecture. Release CI runs it on
# every bundle it builds or packages.
#
#   verify-remote-servers.sh [--require-sums] [--reference SUMS] [--run-host VERSION] DIR
#
#   --require-sums   DIR/SHA256SUMS must exist. When present it must list
#                    exactly the archives and match them.
#   --reference SUMS every archive must match its line in SUMS, e.g. the
#                    build's own list when DIR is a package payload.
#   --run-host V     run the archive for this machine's OS/arch with
#                    `version` and expect V: the remote handshake compares it.
set -euo pipefail

usage() {
    printf 'Usage: %s [--require-sums] [--reference SUMS] [--run-host VERSION] DIR\n' "$0" >&2
    exit 2
}

require_sums=0 reference='' run_version=''
while [ "$#" -gt 0 ]; do
    case $1 in
        --require-sums) require_sums=1; shift ;;
        --reference) [ "$#" -ge 2 ] || usage; reference=$2; shift 2 ;;
        --run-host) [ "$#" -ge 2 ] || usage; run_version=$2; shift 2 ;;
        -*) usage ;;
        *) break ;;
    esac
done
[ "$#" -eq 1 ] || usage
dir=$1

fail() {
    printf 'verify-remote-servers: %s\n' "$*" >&2
    exit 1
}

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -- "$1" | cut -d' ' -f1
    else
        shasum -a 256 -- "$1" | cut -d' ' -f1
    fi
}

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
platforms=$(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile")
[ -n "$platforms" ] || fail "REMOTE_PLATFORMS not found in $root/Makefile"
[ -d "$dir" ] || fail "not a directory: $dir"

expected=()
for pair in $platforms; do
    expected+=("agent-sessions-cli-${pair%/*}-${pair#*/}.gz")
done

# Nothing but the expected archives (and their SHA256SUMS) may be there.
for path in "$dir"/* "$dir"/.[!.]*; do
    [ -e "$path" ] || [ -L "$path" ] || continue
    name=${path##*/}
    [ "$name" = SHA256SUMS ] && continue
    found=0
    for want in "${expected[@]}"; do
        [ "$name" = "$want" ] && found=1
    done
    [ "$found" = 1 ] || fail "unexpected file in $dir: $name"
done

for pair in $platforms; do
    os=${pair%/*} arch=${pair#*/}
    name="agent-sessions-cli-$os-$arch.gz"
    file="$dir/$name"
    [ -f "$file" ] && [ ! -L "$file" ] || fail "missing archive: $file"
    gzip -t -- "$file" || fail "corrupt gzip: $file"
    # First 20 bytes of the executable as hex; head closes the pipe early.
    header=$( (gzip -dc -- "$file" 2>/dev/null || true) | head -c 20 | od -An -v -tx1 | tr -d ' \n')
    case $os/$arch in
        linux/amd64) want_magic=7f454c4602 want_machine=3e00 ;;
        linux/arm64) want_magic=7f454c4602 want_machine=b700 ;;
        darwin/amd64) want_magic=cffaedfe want_machine=07000001 ;;
        darwin/arm64) want_magic=cffaedfe want_machine=0c000001 ;;
        *) fail "no header check for $os/$arch: extend this script" ;;
    esac
    if [ "$os" = linux ]; then
        machine=${header:36:4} # ELF e_machine, little endian
    else
        machine=${header:8:8} # Mach-O cputype, little endian
    fi
    [ "${header:0:${#want_magic}}" = "$want_magic" ] && [ "$machine" = "$want_machine" ] ||
        fail "$name does not hold a 64-bit $os/$arch executable (header $header)"
done

check_sums() { # LIST: every archive must have a matching line in LIST
    local list=$1 name want got
    for name in "${expected[@]}"; do
        want=$(awk -v n="$name" '{f = $2; sub(/^\*/, "", f)} f == n {print $1}' "$list")
        [ "$(printf '%s\n' "$want" | grep -c .)" = 1 ] || fail "$list needs exactly one line for $name"
        got=$(sha256 "$dir/$name")
        [ "$want" = "$got" ] || fail "$name: SHA-256 $got, $list says $want"
    done
}

if [ -f "$dir/SHA256SUMS" ]; then
    listed=$(awk '{f = $2; sub(/^\*/, "", f); print f}' "$dir/SHA256SUMS" | LC_ALL=C sort)
    wanted=$(printf '%s\n' "${expected[@]}" | LC_ALL=C sort)
    [ "$listed" = "$wanted" ] || fail "$dir/SHA256SUMS lists [$(echo $listed)], expected [$(echo $wanted)]"
    check_sums "$dir/SHA256SUMS"
elif [ "$require_sums" = 1 ]; then
    fail "missing $dir/SHA256SUMS"
fi
if [ -n "$reference" ]; then
    [ -f "$reference" ] || fail "reference SHA256SUMS not found: $reference"
    check_sums "$reference"
fi

if [ -n "$run_version" ]; then
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case $(uname -m) in
        x86_64 | amd64) arch=amd64 ;;
        aarch64 | arm64) arch=arm64 ;;
        *) arch=$(uname -m) ;;
    esac
    file="$dir/agent-sessions-cli-$os-$arch.gz"
    [ -f "$file" ] || fail "--run-host: no server for this host ($os/$arch)"
    tmp=$(mktemp -d "${TMPDIR:-/tmp}/verify-remote.XXXXXX")
    trap 'rm -rf -- "$tmp"' EXIT
    gzip -dc -- "$file" > "$tmp/agent-sessions-cli"
    chmod 0755 "$tmp/agent-sessions-cli"
    got=$("$tmp/agent-sessions-cli" version)
    [ "$got" = "$run_version" ] || fail "${file##*/} reports version '$got', expected '$run_version'"
    printf 'verify-remote-servers: %s runs and reports %s\n' "${file##*/}" "$got"
fi

printf 'verify-remote-servers: %s OK (%s)\n' "$dir" "$platforms"
