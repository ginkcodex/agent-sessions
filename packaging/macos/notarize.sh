#!/usr/bin/env bash
# Notarizes a signed .dmg and staples the ticket to it. PROFILE is a
# notarytool keychain profile, saved once with `xcrun notarytool
# store-credentials PROFILE ...` (API key or Apple ID + app-specific
# password), so no secret appears on the command line or in logs. CI keeps
# the profile in a temporary keychain, named by MACOS_NOTARY_KEYCHAIN.
# On rejection it prints Apple's log, which names the offending files.
# macOS only; needs jq.
#
#   notarize.sh DMG PROFILE
set -euo pipefail

if [ "$#" -ne 2 ]; then
    printf 'Usage: %s DMG PROFILE\n' "$0" >&2
    exit 2
fi
dmg=$1 profile=$2

fail() {
    printf 'notarize: %s\n' "$*" >&2
    exit 1
}

for tool in xcrun jq; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool not found"
done
[ -f "$dmg" ] || fail "not found: $dmg"
auth=(--keychain-profile "$profile")
if [ -n "${MACOS_NOTARY_KEYCHAIN:-}" ]; then
    auth+=(--keychain "$MACOS_NOTARY_KEYCHAIN")
fi

printf 'notarize: submitting %s (this usually takes a few minutes)\n' "$dmg"
# notarytool can exit 0 for a rejected submission, so trust only the status.
out=$(xcrun notarytool submit "$dmg" "${auth[@]}" --wait --output-format json) ||
    fail "submission failed: $out"
id=$(jq -r '.id // empty' <<<"$out")
status=$(jq -r '.status // empty' <<<"$out")
printf 'notarize: submission %s: %s\n' "${id:-?}" "${status:-?}"
if [ "$status" != Accepted ]; then
    [ -n "$id" ] && xcrun notarytool log "$id" "${auth[@]}" >&2 || true
    fail "not accepted: $out"
fi

xcrun stapler staple "$dmg"
xcrun stapler validate "$dmg"
