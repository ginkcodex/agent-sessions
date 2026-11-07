#!/usr/bin/env bash
# Commits a verified agent-sessions-bin package (PKGBUILD, .SRCINFO and
# LICENSE from verify-aur-package.sh) to a clone of its AUR repository, on
# master, the only branch the AUR accepts. Pushes only with --push.
#
#   publish-aur.sh [--push] PKGDIR AUR_CHECKOUT
#
# AUR_CHECKOUT is a clean clone of
# ssh://aur@aur.archlinux.org/agent-sessions-bin.git, such as aur/ in this
# repository. It is moved to the AUR's master first; a local master with
# commits the AUR lacks stops the script. When the files already match, it
# commits and pushes nothing.
set -euo pipefail

usage() {
    printf 'Usage: %s [--push] PKGDIR AUR_CHECKOUT\n' "$0" >&2
    exit 2
}
fail() {
    printf 'publish-aur: %s\n' "$*" >&2
    exit 1
}

push=0
while [ "$#" -gt 0 ]; do
    case $1 in
        --push) push=1; shift ;;
        -*) usage ;;
        *) break ;;
    esac
done
[ "$#" -eq 2 ] || usage
pkg=$1 aur=$2
files=(PKGBUILD .SRCINFO LICENSE)
for f in "${files[@]}"; do
    [ -f "$pkg/$f" ] || fail "$pkg/$f missing (run render-pkgbuild.sh, then verify-aur-package.sh)"
done

# The .SRCINFO must describe this PKGBUILD, not an earlier render.
field() { sed -n "s/^$1=//p" "$pkg/PKGBUILD"; }
info() { sed -n "s/^\t$1 = //p" "$pkg/.SRCINFO"; }
pkgver=$(field pkgver) pkgrel=$(field pkgrel)
[ "$(info pkgver)" = "$pkgver" ] && [ "$(info pkgrel)" = "$pkgrel" ] ||
    fail ".SRCINFO is $(info pkgver)-$(info pkgrel), PKGBUILD is $pkgver-$pkgrel: run verify-aur-package.sh again"
grep -qx 'pkgbase = agent-sessions-bin' "$pkg/.SRCINFO" || fail ".SRCINFO is not for agent-sessions-bin"

git -C "$aur" rev-parse --git-dir >/dev/null 2>&1 || fail "$aur is not a git checkout"
origin=$(git -C "$aur" remote get-url origin)
case $origin in
    *aur.archlinux.org/agent-sessions-bin.git | *aur.archlinux.org:agent-sessions-bin.git) ;;
    *) fail "$aur's origin is $origin, not the agent-sessions-bin AUR repository" ;;
esac
[ -z "$(git -C "$aur" status --porcelain)" ] || fail "$aur has uncommitted changes"
# Checked before anything changes: the AUR shows this as the commit author.
if ! git -C "$aur" config user.name >/dev/null || ! git -C "$aur" config user.email >/dev/null; then
    fail "set the commit author first: git -C $aur config user.name '...'; git -C $aur config user.email '...'"
fi

git -C "$aur" fetch -q origin
if git -C "$aur" rev-parse -q --verify refs/remotes/origin/master >/dev/null; then
    if git -C "$aur" rev-parse -q --verify refs/heads/master >/dev/null &&
       ! git -C "$aur" merge-base --is-ancestor master origin/master; then
        fail "$aur's master has commits the AUR does not; push or drop them first"
    fi
    git -C "$aur" checkout -q -B master origin/master
elif git -C "$aur" rev-parse -q --verify HEAD >/dev/null; then
    # A new AUR package committed here but not pushed yet.
    [ "$(git -C "$aur" symbolic-ref -q --short HEAD)" = master ] ||
        fail "the AUR has no master yet, and $aur has commits on another branch; start from a fresh clone"
else
    # A new AUR package: its repository has no commits yet.
    git -C "$aur" symbolic-ref HEAD refs/heads/master
fi

cp -- "${files[@]/#/$pkg/}" "$aur/"
git -C "$aur" add -- "${files[@]}"
if git -C "$aur" diff --cached --quiet; then
    printf 'publish-aur: %s already has agent-sessions-bin %s-%s\n' "$aur" "$pkgver" "$pkgrel"
else
    git -C "$aur" commit -q -m "Update to $pkgver-$pkgrel"
    git -C "$aur" show --stat --format='%h %s' HEAD
fi

# Commits on master the AUR does not have yet.
if git -C "$aur" rev-parse -q --verify refs/remotes/origin/master >/dev/null; then
    ahead=$(git -C "$aur" rev-list --count origin/master..master)
else
    ahead=$(git -C "$aur" rev-list --count master)
fi
if [ "$ahead" -eq 0 ]; then
    printf 'publish-aur: the AUR already has agent-sessions-bin %s-%s\n' "$pkgver" "$pkgrel"
elif [ "$push" -eq 1 ]; then
    git -C "$aur" push -q origin master
    printf 'publish-aur: pushed agent-sessions-bin %s-%s to the AUR\n' "$pkgver" "$pkgrel"
else
    printf 'publish-aur: agent-sessions-bin %s-%s is committed in %s, not pushed; run again with --push\n' \
        "$pkgver" "$pkgrel" "$aur"
fi
