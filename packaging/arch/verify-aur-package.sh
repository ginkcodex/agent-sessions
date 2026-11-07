#!/usr/bin/env bash
# Checks a rendered agent-sessions-bin package (render-pkgbuild.sh) in a
# throwaway Arch Linux container, the way an AUR helper builds it, and writes
# its .SRCINFO into PKGDIR. In the container:
#   - makepkg writes .SRCINFO, downloads the published x86_64 .deb, checks its
#     checksum and builds the package;
#   - makepkg checks the aarch64 .deb's checksum (the image is x86_64 only, so
#     that package is not built);
#   - namcap reviews the PKGBUILD and the package (printed, not enforced);
#   - pacman installs it with its declared dependencies, every shared library
#     the binary links resolves, the files land where the app looks for them,
#     the host's remote server reports the package version, and removing the
#     package leaves nothing behind.
# The app itself is not started: there is no display. Needs docker and the
# network.
#
#   verify-aur-package.sh PKGDIR [IMAGE]
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    printf 'Usage: %s PKGDIR [IMAGE]\n' "$0" >&2
    exit 2
fi
image=${2:-archlinux:base-devel}
fail() {
    printf 'verify-aur-package: %s\n' "$*" >&2
    exit 1
}
[ -f "$1/PKGBUILD" ] && [ -f "$1/LICENSE" ] || fail "$1 needs PKGBUILD and LICENSE (see render-pkgbuild.sh)"
command -v docker >/dev/null 2>&1 || fail "docker not found"
dir=$(cd -- "$1" && pwd)

# Bounded: pacman and the release download need the network, but a stuck
# mirror must not hang the job.
timeout 1200 docker run --rm -i -v "$dir:/pkg" -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
    "$image" bash -euo pipefail -s <<'IN_CONTAINER'
fail() { printf 'verify-aur-package: %s\n' "$*" >&2; exit 1; }
as_builder() { runuser -u builder -- "$@"; }

pacman -Syu --noconfirm --needed namcap desktop-file-utils >/tmp/pacman.log 2>&1 ||
    { cat /tmp/pacman.log; fail "pacman -Syu failed"; }
useradd -m builder
work=/home/builder/pkg
install -d -o builder -g builder "$work"
install -o builder -g builder -m 0644 /pkg/PKGBUILD /pkg/LICENSE "$work/"
cd "$work"

as_builder makepkg --printsrcinfo > .SRCINFO
pkgver=$(sed -n 's/^\tpkgver = //p' .SRCINFO)
pkgrel=$(sed -n 's/^\tpkgrel = //p' .SRCINFO)
[ -n "$pkgver" ] && [ -n "$pkgrel" ] || fail ".SRCINFO has no pkgver/pkgrel"
printf 'verify-aur-package: agent-sessions-bin %s-%s on %s\n' "$pkgver" "$pkgrel" "$(. /etc/os-release && echo "$PRETTY_NAME")"

# Installed as dependencies first, so makepkg itself runs without root.
mapfile -t deps < <(sed -n 's/^\tdepends = //p' .SRCINFO)
pacman -S --noconfirm --needed --asdeps "${deps[@]}" >/tmp/pacman.log 2>&1 ||
    { cat /tmp/pacman.log; fail "installing dependencies failed: ${deps[*]}"; }
as_builder makepkg --noconfirm || fail "makepkg failed"
pkg=$(ls agent-sessions-bin-"$pkgver-$pkgrel"-x86_64.pkg.tar.*)

sed 's/^CARCH=.*/CARCH="aarch64"/' /etc/makepkg.conf > /tmp/makepkg-aarch64.conf
as_builder makepkg --config /tmp/makepkg-aarch64.conf --verifysource --noconfirm ||
    fail "aarch64 source does not match its checksum"

printf 'verify-aur-package: namcap (advisory)\n'
namcap PKGBUILD "$pkg" || true

pacman -U --noconfirm "$pkg" >/tmp/pacman.log 2>&1 || { cat /tmp/pacman.log; fail "pacman -U failed"; }
pacman -Qq agent-sessions-bin >/dev/null || fail "package not installed"
pacman -T agent-sessions >/dev/null || fail "installed package does not provide agent-sessions"
test -x /usr/bin/agent-sessions || fail "/usr/bin/agent-sessions missing or not executable"
libs=$(ldd /usr/bin/agent-sessions)
if grep -q 'not found' <<<"$libs"; then
    printf '%s\n' "$libs" >&2
    fail "unresolved shared libraries after installing the declared dependencies"
fi
for lib in 'libgtk-4\.so' 'libwebkitgtk-6\.0\.so'; do
    grep -Eq "$lib[^ ]* => /" <<<"$libs" || fail "/usr/bin/agent-sessions does not resolve ${lib//\\/}"
done
desktop=/usr/share/applications/io.github.ginkcode.AgentSessions.desktop
test -f "$desktop" || fail "desktop entry missing"
desktop-file-validate "$desktop" || fail "desktop entry does not validate"
test -f /usr/share/pixmaps/agent-sessions.png || fail "icon missing"
test -f /usr/share/licenses/agent-sessions-bin/LICENSE || fail "license missing"
servers=(/usr/lib/agent-sessions/remote/*.gz)
[ "${#servers[@]}" -eq 4 ] || fail "expected 4 remote servers, found ${#servers[@]}"
for f in "${servers[@]}"; do gzip -t "$f" || fail "corrupt $f"; done
gzip -dc /usr/lib/agent-sessions/remote/agent-sessions-cli-linux-amd64.gz > /tmp/agent-sessions-cli
chmod 0755 /tmp/agent-sessions-cli
got=$(/tmp/agent-sessions-cli version)
[ "$got" = "$pkgver" ] || fail "the linux-amd64 server reports version '$got', expected '$pkgver'"

pacman -Rns --noconfirm agent-sessions-bin >/tmp/pacman.log 2>&1 || { cat /tmp/pacman.log; fail "pacman -Rns failed"; }
for path in /usr/bin/agent-sessions "$desktop" /usr/lib/agent-sessions /usr/share/licenses/agent-sessions-bin; do
    test ! -e "$path" || fail "$path left behind after removal"
done

install -o "$HOST_UID" -g "$HOST_GID" -m 0644 .SRCINFO /pkg/.SRCINFO
printf 'verify-aur-package: built, installed, %d shared libraries resolved, %d servers, server reports %s, removed cleanly\n' \
    "$(grep -c '=> /' <<<"$libs")" "${#servers[@]}" "$got"
IN_CONTAINER
