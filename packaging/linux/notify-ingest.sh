#!/usr/bin/env bash
# Asks the website to record a release it just published, for the apt and
# dnf repositories (docs/plan/linux-repos.md):
#
#   LINUX_REPOS_TOKEN=... notify-ingest.sh https://agent-sessions.ginklab.cv v1.6.0
#
# Only the tag is sent; the website reads the release from GitHub itself.
# Without LINUX_REPOS_TOKEN this warns and succeeds: the website's hourly
# cron adds the release anyway. Network errors, 404 and 5xx answers are
# retried three times; other 4xx answers are not, since asking again cannot
# change them.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: LINUX_REPOS_TOKEN=... $0 <site URL> <tag>" >&2
  exit 2
fi
site=${1%/}
tag=$2
if [[ ! $site =~ ^https?://[A-Za-z0-9.:-]+$ ]]; then
  echo "::error::not a site URL: $site" >&2
  exit 2
fi
if [[ ! $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "::error::not a release tag: $tag" >&2
  exit 2
fi
if [ -z "${LINUX_REPOS_TOKEN:-}" ]; then
  echo "::warning::LINUX_REPOS_TOKEN is not set: $tag was not sent to $site. The website's hourly cron adds it within the hour."
  exit 0
fi

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
# The token goes to curl in a file, not on its command line.
(umask 077 && printf 'Authorization: Bearer %s\n' "$LINUX_REPOS_TOKEN" >"$dir/auth")

attempts=4
delay=${NOTIFY_INGEST_DELAY:-15}
for attempt in $(seq 1 "$attempts"); do
  rm -f "$dir/body"
  status=$(curl -sS -o "$dir/body" -w '%{http_code}' --max-time 60 \
    -X POST "$site/api/ingest" -H @"$dir/auth" -H 'Content-Type: application/json' \
    --data "{\"tag\":\"$tag\"}") || status=000
  echo "attempt $attempt: $status $(head -c 2000 "$dir/body" 2>/dev/null || true)"
  case $status in
    200) exit 0 ;;
    # GitHub's API may not show a release made seconds ago: retried.
    404) ;;
    4??)
      echo "::error::$site answered $status for $tag; not retried."
      exit 1
      ;;
  esac
  # Recorded, but the rebuild failed: asking again would be a no-op that
  # hides it. The cron retries the rebuild.
  if grep -qs '"changed": *true' "$dir/body"; then
    echo "::error::$site recorded $tag but did not rebuild the repositories; the hourly cron retries."
    exit 1
  fi
  if [ "$attempt" -lt "$attempts" ]; then sleep $((attempt * delay)); fi
done
echo "::error::$site did not record $tag after $attempts attempts (last: $status). The hourly cron tries again."
exit 1
