#!/usr/bin/env bash
# Signs every .rpm in <dir> in place with the RPM key, then checks each
# signature against that key alone (docs/plan/linux-repos-keys.md).
#
#   RPM_SIGNING_KEY=... sign-rpms.sh <dir>
#
# RPM_SIGNING_KEY holds one armored secret key without a passphrase, as
# `repo-keys.sh secret` writes it. The release workflow runs this in its
# `release` environment, before repo-meta.sh and release-checksums.sh, which
# record each file's size and SHA-256: signing rewrites the header.
#
# Needs gpg, rpmsign and rpmkeys (Ubuntu: gnupg and rpm). The key goes into a
# GnuPG home made for this run, which is removed at the end. Nothing else on
# the machine is used or changed.
set -euo pipefail

die() {
  echo "sign-rpms: $*" >&2
  exit 1
}

[ $# -eq 1 ] || {
  echo "usage: RPM_SIGNING_KEY=<armored secret key> $0 <dir>" >&2
  exit 2
}
dir=$1
[ -n "${RPM_SIGNING_KEY:-}" ] || die "RPM_SIGNING_KEY is not set"
for tool in gpg gpgconf rpmsign rpmkeys; do
  command -v "$tool" >/dev/null || die "$tool is not installed"
done
shopt -s nullglob
rpms=("$dir"/*.rpm)
[ ${#rpms[@]} -gt 0 ] || die "no .rpm in $dir"

work=$(mktemp -d)
cleanup() {
  gpgconf --homedir "$work/gnupg" --kill all 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT
(umask 077 && mkdir "$work/gnupg")
export GNUPGHOME=$work/gnupg

g() {
  gpg --batch --quiet --no-tty --pinentry-mode loopback --passphrase '' "$@"
}

printf '%s\n' "$RPM_SIGNING_KEY" | g --import ||
  die "RPM_SIGNING_KEY could not be imported"
mapfile -t keys < <(g --with-colons --list-secret-keys |
  awk -F: '$1 == "sec" { caps = $12; primary = 1; next }
           $1 == "fpr" && primary { print $10 " " caps; primary = 0 }')
[ ${#keys[@]} -eq 1 ] ||
  die "RPM_SIGNING_KEY must hold exactly one secret key, not ${#keys[@]}"
fpr=${keys[0]%% *}
caps=${keys[0]#* }
# A capital S: the key as a whole can sign now (not expired or revoked).
[[ $caps == *S* ]] || die "$fpr cannot sign (expired, revoked or not a signing key)"

# rpmsign looks for gpg in /usr/bin by default; on Ubuntu it is elsewhere.
rpmsign --define "_gpg_name $fpr" --define "__gpg $(command -v gpg)" \
  --addsign "${rpms[@]}"

# Check against a database holding only this key. rpm -K also exits 0 for an
# unsigned package, so look for the signature line itself. rpm 4 prints
# "Signature, key ID <last 8>: OK"; rpm 6 prints "signature, key fingerprint:
# <fingerprint>: OK".
g --armor --export "$fpr" >"$work/key.asc"
rpmkeys --dbpath "$work/rpmdb" --import "$work/key.asc"
lower=$(tr 'A-F' 'a-f' <<<"$fpr")
for rpm in "${rpms[@]}"; do
  out=$(rpmkeys --dbpath "$work/rpmdb" -Kv "$rpm") || die "$rpm does not verify: $out"
  grep -Eq "[Ss]ignature, key (ID ${lower: -8}|fingerprint: $lower): OK$" <<<"$out" ||
    die "$rpm carries no signature by $fpr: $out"
  echo "$rpm: signed by $fpr"
done
