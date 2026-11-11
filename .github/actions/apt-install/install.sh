#!/usr/bin/env bash
# Usage: install.sh DEB_DIR PACKAGE...
#
# Installs PACKAGE... from the .debs in DEB_DIR (restored from the Actions
# cache) when they cover them. Otherwise it downloads them, preferring
# Canonical's archive over Azure's mirror, keeps the downloaded .debs in
# DEB_DIR and sets the step output save=true so the action caches them.
set -euo pipefail

debs=$1
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

if compgen -G "$debs/*.deb" >/dev/null; then
	# The cached .debs install with no package sources configured, so apt
	# cannot reach a mirror: with sources, it fetches any package whose index
	# version matches a cached .deb from the mirror instead, and stalls with
	# it. Dependencies come only from the .debs and what is installed.
	# _apt cannot read the runner's home, so apt reads the .debs as root.
	# A newer image or point release can leave the cached .debs unable to
	# install; apt then refuses before changing anything, and the download
	# below replaces them.
	if sudo apt-get install -y --no-install-recommends \
		-o Dir::Etc::SourceList=/dev/null -o Dir::Etc::SourceParts=/nonexistent \
		-o Dir::Cache::pkgcache= -o Dir::Cache::srcpkgcache= \
		-o APT::Sandbox::User=root "$debs"/*.deb && installed "$@"; then
		exit 0
	fi
	echo "::warning::Cached .debs did not install $*; downloading them instead."
fi

sudo rm -rf "$debs"
mkdir -p "$debs"
sudo apt-get update
# A separate archive directory keeps exactly this install's downloads.
sudo apt-get install -y --no-install-recommends \
	-o Dir::Cache::Archives="$debs/" -o APT::Keep-Downloaded-Packages=true "$@"
sudo rm -rf "$debs/partial" "$debs/lock"
sudo chown -R "$(id -u):$(id -g)" "$debs"

if compgen -G "$debs/*.deb" >/dev/null; then
	echo "save=true" >>"$GITHUB_OUTPUT"
fi
