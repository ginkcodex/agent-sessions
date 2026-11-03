#!/usr/bin/env bash
# Installs a .deb with apt in a throwaway container of a clean distribution
# (default ubuntu:24.04, this machine's architecture) and checks that apt
# resolves its dependencies, every shared library the binary links resolves,
# the files land where the app looks for them, and a purge removes them. The
# app itself is not started: there is no display. Needs docker.
#
#   verify-deb-install.sh DEB [IMAGE]
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    printf 'Usage: %s DEB [IMAGE]\n' "$0" >&2
    exit 2
fi
deb=$1 image=${2:-ubuntu:24.04}
[ -f "$deb" ] || { printf 'verify-deb-install: not found: %s\n' "$deb" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { printf 'verify-deb-install: docker not found\n' >&2; exit 1; }
dir=$(cd -- "$(dirname -- "$deb")" && pwd)
name=$(basename -- "$deb")

# Bounded: apt needs the network, but a stuck mirror must not hang the job.
timeout 900 docker run --rm -i -v "$dir:/pkg:ro" -e DEB="/pkg/$name" "$image" bash -euo pipefail -s <<'IN_CONTAINER'
fail() { printf 'verify-deb-install: %s\n' "$*" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive
. /etc/os-release
printf 'verify-deb-install: %s on %s (%s)\n' "${DEB##*/}" "$PRETTY_NAME" "$(dpkg --print-architecture)"
apt-get update -qq
# apt reads the local file as root; its _apt sandbox warning is harmless.
apt-get install -y -qq --no-install-recommends "$DEB" >/tmp/apt.log 2>&1 || { cat /tmp/apt.log; fail "apt-get install failed"; }
dpkg-query -W -f '${Status}\n' agent-sessions | grep -x 'install ok installed' >/dev/null || fail "package not installed"
test -x /usr/bin/agent-sessions || fail "/usr/bin/agent-sessions missing or not executable"
libs=$(ldd /usr/bin/agent-sessions)
if grep -q 'not found' <<<"$libs"; then
    printf '%s\n' "$libs" >&2
    fail "unresolved shared libraries after installing the declared dependencies"
fi
# The same libraries `make check-gui-linkage` requires, now resolved.
for lib in 'libgtk-4\.so' 'libwebkitgtk-6\.0\.so'; do
    grep -Eq "$lib[^ ]* => /" <<<"$libs" || fail "/usr/bin/agent-sessions does not resolve ${lib//\\/}"
done
test -f /usr/share/applications/agent-sessions.desktop || fail "desktop entry missing"
test -f /usr/share/pixmaps/agent-sessions.png || fail "icon missing"
servers=(/usr/lib/agent-sessions/remote/*.gz)
[ -f "${servers[0]}" ] || fail "no remote servers in /usr/lib/agent-sessions/remote"
for f in "${servers[@]}"; do gzip -t "$f" || fail "corrupt $f"; done
apt-get purge -y -qq agent-sessions >/tmp/apt.log 2>&1 || { cat /tmp/apt.log; fail "apt-get purge failed"; }
for path in /usr/bin/agent-sessions /usr/share/applications/agent-sessions.desktop /usr/lib/agent-sessions; do
    test ! -e "$path" || fail "$path left behind after purge"
done
printf 'verify-deb-install: installed, %d shared libraries resolved, %d servers, purged cleanly\n' \
    "$(grep -c '=> /' <<<"$libs")" "${#servers[@]}"
IN_CONTAINER
