#!/usr/bin/env bash
# Statically inspects the .msi and _setup.exe `make package-windows` writes.
# Nothing is installed or run: this proves what the files contain, not that
# Windows accepts them (CI's windows-test-build job builds natively, and
# installing still needs a manual check on Windows).
#
# MSI: summary (x64, per-user), product properties and UpgradeCode from
# packaging/windows/agent-sessions.wxs, numeric ProductVersion, upgrade rules,
# the compatibility custom actions and their order, the WebView2 launch
# condition, shortcuts, and the extracted payload: the exe with remote\ and
# conpty\ next to it, as internal/remote/locate.go expects. Setup: a 64-bit
# GUI exe with an asInvoker manifest that embeds this exact MSI. Needs
# msitools (msiinfo, msiextract), x86_64-w64-mingw32-objdump, strings and
# python3.
#
#   verify-packages.sh [--binary EXE] [--remote-sums SUMS] [--conpty DIR] MSI SETUP VERSION
#
#   --binary EXE       the packaged agent-sessions.exe must equal EXE
#   --remote-sums SUMS the packaged servers must match SUMS
#   --conpty DIR       the packaged ConPTY files must equal DIR's
set -euo pipefail

usage() {
    printf 'Usage: %s [--binary EXE] [--remote-sums SUMS] [--conpty DIR] MSI SETUP VERSION\n' "$0" >&2
    exit 2
}
binary='' remote_sums='' conpty=''
while [ "$#" -gt 0 ]; do
    case $1 in
        --binary) [ "$#" -ge 2 ] || usage; binary=$2; shift 2 ;;
        --remote-sums) [ "$#" -ge 2 ] || usage; remote_sums=$2; shift 2 ;;
        --conpty) [ "$#" -ge 2 ] || usage; conpty=$2; shift 2 ;;
        -*) usage ;;
        *) break ;;
    esac
done
[ "$#" -eq 3 ] || usage
msi=$1 setup=$2 version=$3
numeric=${version%%-*}
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
wxs="$root/packaging/windows/agent-sessions.wxs"

fail() {
    printf 'verify-windows: %s\n' "$*" >&2
    exit 1
}

objdump=x86_64-w64-mingw32-objdump
for tool in msiinfo msiextract "$objdump" strings python3; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool not found: apt install msitools gcc-mingw-w64-x86-64 binutils python3"
done
[ -f "$msi" ] || fail "not found: $msi"
[ -f "$setup" ] || fail "not found: $setup"
[[ $numeric =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "VERSION needs a numeric x.y.z: $version"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/verify-windows.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT

# --- MSI database -------------------------------------------------------
table() { # NAME: rows of an MSI table, tab-separated, without headers
    msiinfo export "$msi" "$1" | tr -d '\r' | awk 'NR > 3'
}
summary=$(msiinfo suminfo "$msi")
grep -qx 'Template: x64;1033' <<<"$summary" || fail "summary Template is not x64;1033"
property() {
    table Property | awk -F '\t' -v k="$1" '$1 == k {print $2}'
}
expect_property() { # NAME VALUE
    local got
    got=$(property "$1")
    [ "$got" = "$2" ] || fail "MSI property $1 is '$got', expected '$2'"
}
upgrade_code=$(sed -n 's/.*UpgradeCode="\([^"]*\)".*/\1/p' "$wxs")
[ -n "$upgrade_code" ] || fail "no UpgradeCode in $wxs"
expect_property ProductName 'Agent Sessions'
expect_property Manufacturer ginkcode
expect_property ProductVersion "$numeric"
expect_property UpgradeCode "{$upgrade_code}"
expect_property ARPNOMODIFY 1
expect_property ARPNOREPAIR 1
# Per-user: an ALLUSERS property would make it a per-machine install.
[ -z "$(property ALLUSERS)" ] || fail "MSI sets ALLUSERS: it must stay per-user"

# MajorUpgrade with AllowSameVersionUpgrades: a row from this version to
# itself, 769 = VersionMinInclusive | VersionMaxInclusive | MigrateFeatures.
upgrades=$(table Upgrade)
awk -F '\t' -v c="{$upgrade_code}" -v v="$numeric" \
    '$1 == c && $2 == v && $3 == v && $5 == 769 {f = 1} END {exit !f}' <<<"$upgrades" ||
    fail "Upgrade table does not let $numeric replace the same version:"$'\n'"$upgrades"

actions=$(table CustomAction)
expect_action() { # NAME TYPE: types encode immediate/deferred/rollback and impersonation
    awk -F '\t' -v a="$1" -v t="$2" '$1 == a && $2 == t {f = 1} END {exit !f}' <<<"$actions" ||
        fail "custom action $1 is missing or not type $2:"$'\n'"$actions"
}
expect_action PrepareAppCompat 1 # DLL, immediate
expect_action RollbackAppCompat 1281 # DLL, in-script rollback, impersonated
expect_action ApplyAppCompat 1025 # DLL, deferred, impersonated
table Binary | grep '^AppCompatActions' >/dev/null || fail "Binary table lacks AppCompatActions"

sequence=$(table InstallExecuteSequence)
seq_of() {
    awk -F '\t' -v a="$1" '$1 == a {print $3}' <<<"$sequence"
}
previous=0
for action in InstallInitialize RemoveExistingProducts SetAppCompatPath SetAppCompatRemove \
    PrepareAppCompat RollbackAppCompat ApplyAppCompat InstallFinalize; do
    n=$(seq_of "$action")
    [[ $n =~ ^[0-9]+$ ]] || fail "InstallExecuteSequence lacks $action"
    [ "$n" -gt "$previous" ] || fail "InstallExecuteSequence runs $action ($n) too early (after $previous)"
    previous=$n
done

table LaunchCondition | grep 'WEBVIEW2_MACHINE' >/dev/null || fail "WebView2 launch condition missing"
shortcuts=$(table Shortcut | cut -f1 | LC_ALL=C sort | paste -sd ' ' -)
[ "$shortcuts" = 'DesktopShortcut StartMenuShortcut' ] || fail "shortcuts are '$shortcuts'"

# --- MSI payload --------------------------------------------------------
msiextract -C "$tmp/msi" "$msi" >/dev/null
exe=$(find "$tmp/msi" -type f -name agent-sessions.exe)
[ "$(printf '%s\n' "$exe" | grep -c .)" = 1 ] || fail "expected one agent-sessions.exe in the MSI, found: $exe"
install_dir=$(dirname -- "$exe")
[ "${install_dir##*/}" = 'Agent Sessions' ] || fail "agent-sessions.exe installs to ${install_dir#"$tmp/msi/"}"

remote_files=()
for pair in $(sed -n 's/^REMOTE_PLATFORMS ?= *//p' "$root/Makefile"); do
    remote_files+=("remote/agent-sessions-cli-${pair%/*}-${pair#*/}.gz")
done
expected=$(printf '%s\n' agent-sessions.exe conpty/conpty.dll conpty/OpenConsole.exe conpty/LICENSE.txt \
    "${remote_files[@]}" | LC_ALL=C sort)
payload=$(cd "$tmp/msi" && find . -type f | sed "s|^\./${install_dir#"$tmp/msi/"}/||" | LC_ALL=C sort)
[ "$payload" = "$expected" ] ||
    fail "MSI payload differs:"$'\n'"$(diff <(echo "$expected") <(echo "$payload") || true)"

pe_check() { # FILE [SUBSYSTEM]: x86-64 PE, of SUBSYSTEM when given
    local info
    info=$("$objdump" -p "$1") || fail "$1 is not a PE file"
    grep -q 'file format pei-x86-64' <<<"$info" || fail "${1##*/} is not x86-64 PE"
    [ -z "${2:-}" ] || grep -qE "^Subsystem[[:space:]]+[0-9a-f]+[[:space:]]+\\($2\\)" <<<"$info" ||
        fail "${1##*/} subsystem is not $2"
}
pe_check "$exe" 'Windows GUI'
# windows-app stamps VERSION into the version resource (UTF-16 strings).
strings -el "$exe" | grep -xF "$version" >/dev/null || fail "agent-sessions.exe has no $version version resource"
if [ -n "$binary" ]; then
    cmp -s "$exe" "$binary" || fail "packaged agent-sessions.exe differs from $binary"
fi
pe_check "$install_dir/conpty/OpenConsole.exe"
pe_check "$install_dir/conpty/conpty.dll"
"$objdump" -p "$install_dir/conpty/conpty.dll" | grep -x '[[:space:]]*DLL' >/dev/null || fail "conpty.dll is not a DLL"
cmp -s "$install_dir/conpty/LICENSE.txt" "$root/packaging/windows/conpty-LICENSE.txt" || fail "conpty LICENSE.txt differs"
if [ -n "$conpty" ]; then
    for f in conpty.dll OpenConsole.exe LICENSE.txt; do
        cmp -s "$install_dir/conpty/$f" "$conpty/$f" || fail "packaged conpty/$f differs from $conpty/$f"
    done
fi
remote_args=()
[ -z "$remote_sums" ] || remote_args=(--reference "$remote_sums")
bash "$root/packaging/verify-remote-servers.sh" ${remote_args[@]+"${remote_args[@]}"} "$install_dir/remote"

# --- setup.exe ----------------------------------------------------------
pe_check "$setup" 'Windows GUI'
grep -aq 'level="asInvoker"' "$setup" || fail "setup.exe lacks its asInvoker manifest"
# build-setup.sh embeds the MSI verbatim and pins its SHA-256.
python3 -I - "$setup" "$msi" <<'PY' || fail "setup.exe does not embed this MSI"
import hashlib, sys
setup = open(sys.argv[1], "rb").read()
msi = open(sys.argv[2], "rb").read()
digest = hashlib.sha256(msi).hexdigest().encode()
count = setup.count(msi)
if count != 1 or digest not in setup:
    sys.exit(f"MSI copies: {count}, SHA-256 pinned: {digest in setup}")
PY

printf 'verify-windows: %s and %s OK (static inspection only; ProductVersion %s)\n' \
    "${msi##*/}" "${setup##*/}" "$numeric"
