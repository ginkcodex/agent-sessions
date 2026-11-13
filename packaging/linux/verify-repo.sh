#!/usr/bin/env bash
# Checks the apt and dnf repositories at BASE_URL from clean containers, with
# the setup commands users are given (docs/plan/linux-repos.md, LR-16).
#
#   verify-repo.sh [--between CMD] [--images "IMAGE..."] BASE_URL VERSION...
#
# VERSION... is every version the repositories should list. Each image runs
# these steps; the images run in parallel:
#
#   1. The documented setup commands, then apt update or dnf makecache. dnf
#      also installs the oldest version here.
#   2. CMD, once for all the images, if given. For example, it adds a release
#      to a Worker on this machine, so that step 3 runs on an older package
#      list, or adds a new RPM key, so that step 5 imports it.
#   3. apt installs the oldest version, without another apt update, from
#      step 1's lists.
#   4. The repository must list exactly VERSION...
#   5. Upgrade straight to the newest version, then check what is installed.
#   6. The documented removal commands; the package manager must still work
#      and no longer offer the app.
#
# Packages are for the machine's own architecture. dnf checks the package
# signatures (gpgcheck=1) and repomd.xml's (repo_gpgcheck=1), and imports the
# keys from the key file, as a user answering yes would. The step logs the
# installed package's signature and the keys rpm has imported.
#
# BASE_URL must be reachable from the containers. For a Worker on this
# machine, use http://host.docker.internal:<port>.
#
# Needs docker. Each image's log goes to LOG_DIR (default: a new temp dir).
# The containers are removed at the end, unless KEEP=1.
#
# SC2016: the step scripts are single-quoted on purpose; $BASE and the rest
# expand in the container. SC2329: the phases run through in_parallel's
# "$fn", and cleanup through the trap.
# shellcheck disable=SC2016,SC2329
set -euo pipefail

IMAGES="ubuntu:24.04 debian:13 ubuntu:rolling fedora:40 fedora:latest"
between=
while [ $# -gt 0 ]; do
  case $1 in
    --between) between=$2; shift 2 ;;
    --images) IMAGES=$2; shift 2 ;;
    --) shift; break ;;
    -*) echo "verify-repo: unknown option $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
if [ $# -lt 2 ]; then
  sed -n '/^#   verify-repo.sh/p' "$0" | sed 's/^#   /usage: /' >&2
  exit 2
fi
BASE=${1%/}
shift
# Not mapfile: macOS has bash 3.2. Versions have no spaces.
read -ra VERSIONS <<<"$(printf '%s\n' "$@" | sort -uV | tr '\n' ' ')"
OLDEST=${VERSIONS[0]}
NEWEST=${VERSIONS[${#VERSIONS[@]} - 1]}
WANT=$(printf '%s\n' "${VERSIONS[@]}")

command -v docker >/dev/null || { echo "verify-repo: docker is not installed" >&2; exit 1; }
LOG_DIR=${LOG_DIR:-$(mktemp -d)}
mkdir -p "$LOG_DIR"

# The commands users are given, with $BASE for the site. README and the
# website's download page (test/linux-repos.spec.ts) show the same lines.
SETUP_APT='
curl -fsSL "$BASE/apt/agent-sessions.gpg" | sudo tee /usr/share/keyrings/agent-sessions.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/agent-sessions.gpg] $BASE/apt stable main" | sudo tee /etc/apt/sources.list.d/agent-sessions.list
sudo apt update
'
SETUP_DNF='
curl -fsSL "$BASE/rpm/agent-sessions.repo" | sudo tee /etc/yum.repos.d/agent-sessions.repo >/dev/null
sudo dnf -y makecache
'
REMOVE_APT='
sudo apt remove -y agent-sessions
sudo rm /etc/apt/sources.list.d/agent-sessions.list /usr/share/keyrings/agent-sessions.gpg
sudo apt update
'
REMOVE_DNF='
sudo dnf -y remove agent-sessions
sudo rm /etc/yum.repos.d/agent-sessions.repo
'

# What a fresh image lacks for the steps above: curl and sudo.
PREP_APT='
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends curl ca-certificates sudo >/dev/null
'
PREP_DNF='
command -v sudo >/dev/null || dnf -y -q install sudo
'
INSTALL_DNF='
grep -qx "gpgcheck=1" /etc/yum.repos.d/agent-sessions.repo
grep -qx "repo_gpgcheck=1" /etc/yum.repos.d/agent-sessions.repo
sudo dnf -y install "agent-sessions-$OLDEST"
test "$(rpm -q --qf "%{VERSION}" agent-sessions)" = "$OLDEST"
rpm -q --qf "signature: %{DSAHEADER:pgpsig}\n" agent-sessions
'

PHASE2_APT='
export DEBIAN_FRONTEND=noninteractive
# No apt update: the lists are the ones from before the pause.
sudo -E apt-get install -y agent-sessions="$OLDEST"
test "$(dpkg-query -W -f="\${Version}" agent-sessions)" = "$OLDEST"
sudo apt update
listed=$(apt-cache madison agent-sessions | awk -F"|" "{gsub(/ /, \"\", \$2); print \$2}" | sort -uV)
echo "listed: $(echo $listed)"
test "$listed" = "$WANT"
sudo -E apt-get install -y --only-upgrade agent-sessions
installed=$(dpkg-query -W -f="\${Version}" agent-sessions)
echo "installed: $installed"
test "$installed" = "$NEWEST"
test -x /usr/bin/agent-sessions
'"$REMOVE_APT"'
if dpkg-query -W -f="\${Status}" agent-sessions 2>/dev/null | grep -q " installed\$"; then exit 1; fi
test ! -e /usr/bin/agent-sessions
test -z "$(apt-cache madison agent-sessions)"
'
PHASE2_DNF='
listed=$(dnf -q --refresh list --showduplicates agent-sessions |
  awk "\$1 ~ /^agent-sessions\\./ { v = \$2; sub(/^[0-9]+:/, \"\", v); sub(/-[^-]*\$/, \"\", v); print v }" | sort -uV)
echo "listed: $(echo $listed)"
test "$listed" = "$WANT"
sudo dnf -y --refresh upgrade agent-sessions
installed=$(rpm -q --qf "%{VERSION}" agent-sessions)
echo "installed: $installed"
test "$installed" = "$NEWEST"
test -x /usr/bin/agent-sessions
sig=$(rpm -q --qf "%{DSAHEADER:pgpsig}" agent-sessions)
echo "signature: $sig"
case $sig in *"Key ID "*) ;; *) exit 1 ;; esac
rpm -q gpg-pubkey --qf "imported: %{SUMMARY}\n"
'"$REMOVE_DNF"'
if rpm -q agent-sessions; then exit 1; fi
test ! -e /usr/bin/agent-sessions
if dnf -q --refresh list --available agent-sessions; then exit 1; fi
'

slug() { tr -c 'a-zA-Z0-9.\n' '-' <<<"$1"; }
names=()
cleanup() {
  [ "${KEEP:-}" = 1 ] || [ ${#names[@]} -eq 0 ] || docker rm -f "${names[@]}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# run <image> <log> <script>: the script in that image's container, traced.
run() {
  docker exec -e BASE="$BASE" -e OLDEST="$OLDEST" -e NEWEST="$NEWEST" -e WANT="$WANT" \
    "$(container "$1")" bash -c "set -euxo pipefail; $3" >>"$2" 2>&1
}
container() { echo "verify-repo-$$-$(slug "$1")"; }
kind() { # apt or dnf, from the image's package manager
  if docker exec "$(container "$1")" sh -c 'command -v apt-get' >/dev/null 2>&1; then echo apt; else echo dnf; fi
}

phase1() { # <image>
  local image=$1 log k
  log="$LOG_DIR/$(slug "$image").log"
  docker run -d --name "$(container "$image")" --add-host=host.docker.internal:host-gateway \
    "$image" sleep infinity >/dev/null
  docker exec "$(container "$image")" sh -c '. /etc/os-release; echo "=== $PRETTY_NAME ($(uname -m))"' >"$log"
  k=$(kind "$image")
  if [ "$k" = apt ]; then
    run "$image" "$log" "$PREP_APT" && run "$image" "$log" "$SETUP_APT"
  else
    run "$image" "$log" "$PREP_DNF" && run "$image" "$log" "$SETUP_DNF" &&
      run "$image" "$log" "$INSTALL_DNF"
  fi
}

phase2() { # <image>
  local image=$1 log
  log="$LOG_DIR/$(slug "$image").log"
  if [ "$(kind "$image")" = apt ]; then
    run "$image" "$log" "$PHASE2_APT"
  else
    run "$image" "$log" "$PHASE2_DNF"
  fi
}

# in_parallel <function> <image>...: writes <slug>.<function> with the exit
# status of each.
in_parallel() {
  local fn=$1 image pids=()
  shift
  for image in "$@"; do
    ( set +e; "$fn" "$image"; echo $? >"$LOG_DIR/$(slug "$image").$fn" ) &
    pids+=($!)
  done
  wait "${pids[@]}"
}

read -ra images <<<"$IMAGES"
for image in "${images[@]}"; do names+=("$(container "$image")"); done
echo "verify-repo: $BASE, versions ${VERSIONS[*]}, logs in $LOG_DIR"

in_parallel phase1 "${images[@]}"
if [ -n "$between" ]; then
  echo "verify-repo: running $between"
  bash -c "$between"
fi
ready=()
for image in "${images[@]}"; do
  [ "$(cat "$LOG_DIR/$(slug "$image").phase1")" = 0 ] && ready+=("$image")
done
[ ${#ready[@]} -eq 0 ] || in_parallel phase2 "${ready[@]}"

failed=0
for image in "${images[@]}"; do
  s=$(slug "$image")
  log="$LOG_DIR/$s.log"
  if [ "$(cat "$LOG_DIR/$s.phase1")" != 0 ]; then
    result="FAIL in setup"
  elif [ "$(cat "$LOG_DIR/$s.phase2")" != 0 ]; then
    result="FAIL"
  else
    result="ok"
  fi
  [ "$result" = ok ] || failed=1
  printf '%-16s %-40s %s\n' "$image" "$(head -1 "$log" | sed 's/^=== //')" "$result"
done
if [ $failed = 1 ]; then
  echo "verify-repo: failed; see the logs in $LOG_DIR" >&2
fi
exit $failed
