#!/usr/bin/env bash
# The Fedora half of `make test-repo-meta`. Runs as root in a Fedora container
# with dnf, createrepo_c, jq, rpm-sign and gnupg2:
#
#  1. dnf reads the repository test-repo-meta.sh assembled from repo-meta.json
#     in OUT_DIR/rpm-repo: both packages, their files, and other.xml.
#  2. Fedora's createrepo_c writes the same .rpm entries as Ubuntu's did.
#  3. repo-meta.sh leaves signed packages as they are: the same bytes, a good
#     signature, and the entry describes the signed file.
#
#   test-repo-meta-fedora.sh PKG_DIR VERSION OUT_DIR
set -euo pipefail
export LC_ALL=C

if [ "$#" -ne 3 ]; then
    printf 'Usage: %s PKG_DIR VERSION OUT_DIR\n' "$0" >&2
    exit 2
fi
pkgs=$1 version=$2 out=$3
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

failures=0
pass() { printf 'ok   %s\n' "$*"; }
fail() {
    printf 'FAIL %s\n' "$*" >&2
    failures=$((failures + 1))
}
quiet() { "$@" >/dev/null 2>&1; }
check() {
    local what=$1
    shift
    if "$@"; then pass "$what"; else fail "$what"; fi
}

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
rpms=("agent-sessions-${version}-1.aarch64.rpm" "agent-sessions-${version}-1.x86_64.rpm")

# 1. dnf reads the assembled repository. Only it: no network, no cache.
query() {
    dnf repoquery --quiet --repofrompath="t,file://$out/rpm-repo" \
        --repo=t --setopt=cachedir="$work/cache" "$@"
}
listed=$(query --queryformat '%{name} %{version} %{arch} %{location}\n' agent-sessions | sort)
base=file://$out/rpm-repo/pool/$version
want=$(printf 'agent-sessions %s aarch64 %s/%s\nagent-sessions %s x86_64 %s/%s\n' \
    "$version" "$base" "${rpms[0]}" "$version" "$base" "${rpms[1]}")
if [ "$listed" = "$want" ]; then
    pass "dnf repoquery lists both packages under pool/$version"
else
    fail "dnf repoquery lists: ${listed:-nothing}"
fi
check "filelists.xml: dnf knows the files" grep -qx '/usr/bin/agent-sessions' <<< "$(query --files --arch=x86_64 agent-sessions)"
# The packages have no changelog entries; this shows dnf reads other.xml.
check "other.xml: dnf reads it" \
    [ "$(query --changelogs --arch=aarch64 agent-sessions)" = "Changelogs for agent-sessions-$version-1.aarch64" ]

# 2. Fedora's createrepo_c writes the same entries as the release runner's
# (Ubuntu's, an older version), so the Worker's files read the same whichever
# made a release's part.
plain=$work/plain
mkdir -p "$plain"
cp -- "${rpms[@]/#/$pkgs/}" "$plain/"
bash "$here/repo-meta.sh" "$version" "$plain" >/dev/null 2>"$work/err" || { cat "$work/err" >&2; exit 1; }
check "Fedora's createrepo_c writes the entries Ubuntu's did" \
    [ "$(jq -c .rpm "$plain/repo-meta.json")" = "$(jq -c .rpm "$out/repo-meta.json")" ]

# 3. Signed packages: a throwaway key signs copies, then repo-meta.sh reads them.
export GNUPGHOME=$work/gnupg
mkdir -m 700 "$GNUPGHOME"
gpg --batch --quiet --passphrase '' --quick-gen-key 'repo-meta test <test@example.invalid>' rsa2048 sign never
gpg --batch --quiet --armor --export 'repo-meta test' > "$work/key.asc"
rpmdb=$work/rpmdb
mkdir -p "$rpmdb"
rpm --dbpath "$rpmdb" --import "$work/key.asc"

signed=$work/signed
mkdir -p "$signed"
cp -- "${rpms[@]/#/$pkgs/}" "$signed/"
rpm --define "_gpg_name repo-meta test" --addsign "$signed"/*.rpm >/dev/null 2>&1
before=$(cd "$signed" && sha256sum -- *.rpm)
bash "$here/repo-meta.sh" "$version" "$signed" >/dev/null 2>"$work/err" || { cat "$work/err" >&2; exit 1; }
check "repo-meta.sh leaves the signed packages as they are" [ "$(cd "$signed" && sha256sum -- *.rpm)" = "$before" ]
for file in "${rpms[@]}"; do
    check "$file: the signature is good" quiet rpm --dbpath "$rpmdb" --checksig "$signed/$file"
    sha256=$(sha256sum -- "$signed/$file" | cut -d' ' -f1)
    original=$(sha256sum -- "$pkgs/$file" | cut -d' ' -f1)
    entry=$(jq -r --arg f "$file" '.rpm[] | select(.file == $f) | .sha256' "$signed/repo-meta.json")
    signed_entry=no
    [[ $entry = "$sha256" && $sha256 != "$original" ]] && signed_entry=yes
    check "$file: the entry describes the signed file" [ "$signed_entry" = yes ]
done

if [ "$failures" -gt 0 ]; then
    printf '%d check(s) failed\n' "$failures" >&2
    exit 1
fi
printf 'all Fedora repo-meta checks passed\n'
