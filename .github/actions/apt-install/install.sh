#!/usr/bin/env bash
# Usage: install.sh CACHE_DIR PACKAGE...
#
# Installs PACKAGE... from CACHE_DIR (restored from the Actions cache) when
# it holds them: archives/ has the .debs and lists/ the package lists they
# were downloaded with. Otherwise it downloads both into CACHE_DIR,
# preferring Canonical's archive over Azure's mirror, and sets the step
# output save=true so the action caches them.
set -euo pipefail

cache=$1
shift
[ "$#" -gt 0 ] || { echo "install.sh: no packages given" >&2; exit 2; }

installed() {
	local status
	status=$(dpkg-query -W -f='${db:Status-Status}\n' "$@" 2>/dev/null) || return 1
	! grep -qvx installed <<<"$status"
}

if installed "$@"; then
	echo "Already installed: $*"
	exit 0
fi

# GitHub's x64 runners fetch from a mirror list that puts
# azure.archive.ubuntu.com first. apt moves down the list only when a mirror
# fails, not when it is slow, and Azure's mirror can trickle at tens of KB/s.
# Canonical's archive goes first; Azure's stays as the fallback. arm64
# runners use ports.ubuntu.com, which the list does not cover.
mirrors=/etc/apt/apt-mirrors.txt
if [ -f "$mirrors" ] && grep -q 'azure\.archive\.ubuntu\.com' "$mirrors"; then
	printf '%s\tpriority:%s\n' \
		https://archive.ubuntu.com/ubuntu/ 1 \
		http://azure.archive.ubuntu.com/ubuntu/ 2 \
		https://security.ubuntu.com/ubuntu/ 3 | sudo tee "$mirrors" >/dev/null
fi

# apt reads lists and .debs from CACHE_DIR, so the cached .debs match the
# lists they came from rather than the image's. Its binary caches are built
# in memory so the system's stay valid for later apt runs.
apt_dirs=(
	-o Dir::State::Lists="$cache/lists/"
	-o Dir::Cache::Archives="$cache/archives/"
	-o Dir::Cache::pkgcache= -o Dir::Cache::srcpkgcache=
)

if [ -d "$cache/lists" ] && compgen -G "$cache/archives/*.deb" >/dev/null; then
	# Handing apt the .deb files would make it fetch the same versions from
	# the archive instead. --no-download makes it use the cached ones, or
	# fail when the image or Ubuntu has moved on; the download below then
	# replaces the cache.
	if sudo apt-get install -y --no-install-recommends --no-download "${apt_dirs[@]}" "$@" &&
		installed "$@"; then
		exit 0
	fi
	echo "::warning::Cached .debs did not install $*; downloading them instead."
fi

sudo rm -rf "$cache"
mkdir -p "$cache/lists" "$cache/archives"
sudo apt-get update "${apt_dirs[@]}"
sudo apt-get install -y --no-install-recommends -o APT::Keep-Downloaded-Packages=true \
	"${apt_dirs[@]}" "$@"
sudo rm -rf "$cache/lists/partial" "$cache/lists/lock" "$cache/archives/partial" "$cache/archives/lock"
sudo chown -R "$(id -u):$(id -g)" "$cache"

if compgen -G "$cache/archives/*.deb" >/dev/null; then
	echo "save=true" >>"$GITHUB_OUTPUT"
fi
