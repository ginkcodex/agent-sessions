#!/usr/bin/env bash
# Makes and looks after the Linux repositories' two signing keys
# (docs/plan/linux-repos-keys.md). Run it on an offline machine, with <dir> on
# encrypted storage; it never sends anything anywhere.
#
#   repo-keys.sh new    <repo|rpm> <dir>         a new key, valid for 3 years
#   repo-keys.sh extend <dir> <fingerprint>      3 more years from today (repository key)
#   repo-keys.sh show   <dir>                    fingerprints, expiry, renewal date
#   repo-keys.sh secret <dir> <fingerprint>...   the secret keys, in that order,
#                                                for REPO_SIGNING_KEY or RPM_SIGNING_KEY
#   repo-keys.sh backup <dir> <file>             <dir>, encrypted with a passphrase
#   repo-keys.sh restore <file> <dir>            the other way round
#
# <dir> holds a GnuPG home (gnupg/) and, per key, <name>.pub.asc,
# <name>.secret.asc and <name>.rev.asc (its revocation certificate), where
# <name> is <role>-<key ID>. The keys have no passphrase: the Worker and
# sign-rpms.sh read them as they are, so <dir> and its backups are what need protecting.
#
# Both keys are v4 EdDSA (algorithm 22) Ed25519 keys: RFC 9580's Ed25519
# (algorithm 27) fails on Ubuntu 24.04's gpgv and Fedora 40's rpm (LR-02).
# The script checks what gpg made, in case a newer gpg defaults otherwise.
#
# backup and restore ask gpg for the passphrase; REPO_KEYS_PASSPHRASE_FILE
# (a file holding it) is for tests.
set -euo pipefail

VALIDITY=3y
RENEW_BEFORE_DAYS=183

usage() {
  sed -n '/^#   repo-keys.sh/,/^#$/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

die() {
  echo "repo-keys: $*" >&2
  exit 1
}

need_gpg() {
  command -v gpg >/dev/null || die "gpg is not installed"
}

use_home() { # <dir>
  [ -d "$1/gnupg" ] || die "$1 has no gnupg/ (make a key with: $0 new <role> $1)"
  export GNUPGHOME
  GNUPGHOME=$(cd "$1/gnupg" && pwd)
}

# gpg without prompts. The keys have no passphrase, so nothing is asked.
g() {
  gpg --batch --quiet --no-tty --pinentry-mode loopback --passphrase '' "$@"
}

# Seconds since the epoch to a UTC date, with GNU or BSD date.
day() {
  date -u -d "@$1" +%Y-%m-%d 2>/dev/null || date -u -r "$1" +%Y-%m-%d
}

# The user ID's start, per role. A key's user ID adds the year it was made,
# so the old and new key of a rotation can be told apart in rpm -qa gpg-pubkey.
uid_for() {
  case $1 in
    repo) echo "Agent Sessions repository signing key" ;;
    rpm) echo "Agent Sessions RPM signing key" ;;
    *) die "role must be repo or rpm, not: $1" ;;
  esac
}

# The role a key was made for, from its user ID.
role_of() { # <fingerprint>
  local uid
  uid=$(g --with-colons --list-keys "$1" | awk -F: '$1 == "uid" { print $10; exit }')
  case $uid in
    "$(uid_for repo) "*) echo repo ;;
    "$(uid_for rpm) "*) echo rpm ;;
    *) die "$1 is not one of this script's keys (user ID: $uid)" ;;
  esac
}

full_fingerprint() { # <fingerprint or key ID>
  local fpr
  fpr=$(g --with-colons --list-secret-keys "$1" 2>/dev/null |
    awk -F: '$1 == "sec" { primary = 1; next } $1 == "fpr" && primary { print $10; exit }')
  [ -n "$fpr" ] || die "no secret key $1 in $GNUPGHOME"
  echo "$fpr"
}

# Fails unless the key is v4 EdDSA on Ed25519 and can sign.
check_key() { # <fingerprint>
  local packets
  packets=$(g --export "$1" | gpg --list-packets 2>/dev/null)
  echo "$packets" | grep -q 'public key packet:' || die "$1: no public key packet"
  echo "$packets" | grep -A1 -m1 'public key packet:' | grep -q 'version 4, algo 22,' ||
    die "$1 is not a v4 EdDSA (algorithm 22) key: $(echo "$packets" | grep -A1 -m1 'public key packet:' | tail -1)"
  g --with-colons --list-keys "$1" | awk -F: '$1 == "pub" { exit !($12 ~ /s/ || $12 ~ /S/) }' ||
    die "$1 cannot sign"
}

write_files() { # <dir> <fingerprint>
  local dir=$1 fpr=$2 name
  name="$(role_of "$fpr")-${fpr: -16}"
  # gpg replaces the self-signature when it extends a key, so the files carry
  # the newest only. The Worker signs at each rebuild, after it.
  g --armor --export "$fpr" >"$dir/$name.pub.asc"
  (umask 077 && g --armor --export-secret-keys "$fpr" >"$dir/$name.secret.asc")
  if [ -f "$GNUPGHOME/openpgp-revocs.d/$fpr.rev" ]; then
    cp "$GNUPGHOME/openpgp-revocs.d/$fpr.rev" "$dir/$name.rev.asc"
  fi
  echo "$dir/$name.{pub,secret,rev}.asc"
}

show() { # <dir>
  use_home "$1"
  g --with-colons --list-secret-keys | awk -F: '
    $1 == "sec" { created = $6; expires = $7; primary = 1; next }
    $1 == "fpr" && primary { fpr = $10; primary = 0; next }
    $1 == "uid" && fpr != "" { print fpr "\t" created "\t" expires "\t" $10; fpr = "" }' |
    while IFS=$'\t' read -r fpr created expires uid; do
      echo "$fpr  $uid"
      echo "    created $(day "$created")"
      # An RPM key is replaced by a new one, never extended (cmd_extend).
      case $uid in
        "$(uid_for rpm) "*) renew="make its successor" ;;
        *) renew="renew" ;;
      esac
      if [ -n "$expires" ]; then
        if [ "$expires" -le "$(date -u +%s)" ]; then
          echo "    EXPIRED $(day "$expires"): $renew, then update the secrets and public files"
        else
          echo "    expires $(day "$expires"); $renew by $(day $((expires - RENEW_BEFORE_DAYS * 86400)))"
        fi
      else
        echo "    never expires"
      fi
    done
}

cmd_new() { # <role> <dir>
  [ $# -eq 2 ] || usage
  local role=$1 dir=$2 uid fpr
  uid=$(uid_for "$role")
  need_gpg
  mkdir -p "$dir"
  if [ ! -d "$dir/gnupg" ]; then
    (umask 077 && mkdir "$dir/gnupg")
  fi
  use_home "$dir"
  # --yes: a rotation's second key may have the same user ID as the first.
  fpr=$(g --yes --status-fd 1 --quick-gen-key "$uid ($(date -u +%Y))" ed25519 sign "$VALIDITY" |
    awk '$1 == "[GNUPG:]" && $2 == "KEY_CREATED" { print $4 }')
  [ -n "$fpr" ] || die "gpg made no key"
  check_key "$fpr"
  write_files "$dir" "$fpr"
  show "$dir" | grep -A2 "^$fpr"
}

cmd_extend() { # <dir> <fingerprint>
  [ $# -eq 2 ] || usage
  local dir=$1 fpr
  need_gpg
  use_home "$dir"
  fpr=$(full_fingerprint "$2")
  # rpm 4.19 keeps the copy of a key it imported first: importing the
  # extended key changes nothing, and packages signed after the old expiry
  # date fail on those machines (LR-08). A new RPM key has a new key ID,
  # which dnf imports from the .repo file's gpgkey.
  [ "$(role_of "$fpr")" = repo ] ||
    die "$fpr is an RPM key: make a new one instead (docs/plan/linux-repos-keys.md, Rotating the RPM key)"
  g --quick-set-expire "$fpr" "$VALIDITY"
  check_key "$fpr"
  write_files "$dir" "$fpr"
  show "$dir" | grep -A2 "^$fpr"
}

cmd_secret() { # <dir> <fingerprint>...
  [ $# -ge 2 ] || usage
  local dir=$1 fpr seen=" "
  shift
  need_gpg
  use_home "$dir"
  for f in "$@"; do
    fpr=$(full_fingerprint "$f")
    case $seen in *" $fpr "*) die "$fpr is listed twice" ;; esac
    seen="$seen$fpr "
    check_key "$fpr"
    # One armored block per key, in the order given: the Worker signs
    # repomd.xml with the first.
    g --armor --export-secret-keys "$fpr"
  done
}

# gpg's options for the backup passphrase: a prompt, or the test's file.
passphrase=()
if [ -n "${REPO_KEYS_PASSPHRASE_FILE:-}" ]; then
  passphrase=(--batch --pinentry-mode loopback --passphrase-file "$REPO_KEYS_PASSPHRASE_FILE")
fi

cmd_backup() { # <dir> <file>
  [ $# -eq 2 ] || usage
  local dir=$1 file=$2 tmp
  need_gpg
  use_home "$dir"
  [ ! -e "$file" ] || die "$file exists"
  gpgconf --kill gpg-agent 2>/dev/null || true
  tmp=$(mktemp -d)
  tar -C "$dir" --exclude 'S.*' --exclude '*.lock' -cf - . |
    GNUPGHOME="$tmp" gpg --quiet --symmetric --cipher-algo AES256 --armor \
      ${passphrase[@]+"${passphrase[@]}"} -o "$file"
  rm -rf "$tmp"
  echo "$file"
}

cmd_restore() { # <file> <dir>
  [ $# -eq 2 ] || usage
  local file=$1 dir=$2 tmp
  need_gpg
  [ ! -e "$dir" ] || die "$dir exists"
  tmp=$(mktemp -d)
  GNUPGHOME="$tmp" gpg --quiet --decrypt ${passphrase[@]+"${passphrase[@]}"} "$file" >"$tmp/backup.tar"
  (umask 077 && mkdir -p "$dir")
  tar -C "$dir" -xf "$tmp/backup.tar"
  rm -rf "$tmp"
  show "$dir"
}

[ $# -ge 1 ] || usage
command=$1
shift
case $command in
  new) cmd_new "$@" ;;
  extend) cmd_extend "$@" ;;
  show) [ $# -eq 1 ] || usage; need_gpg; show "$1" ;;
  secret) cmd_secret "$@" ;;
  backup) cmd_backup "$@" ;;
  restore) cmd_restore "$@" ;;
  *) usage ;;
esac
