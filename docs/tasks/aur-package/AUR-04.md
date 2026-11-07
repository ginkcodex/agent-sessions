# AUR-04 — AUR workflow, called after stable releases

- **Status:** 🟡 Partial (not run on GitHub yet; see AUR-07)
- **Estimate:** 1.5 h
- **Depends on:** AUR-03
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

`.github/workflows/aur.yml`, reusable (`workflow_call`) and manual
(`workflow_dispatch` with `version`, `pkgrel`, `publish`): `make aur-package`,
upload `PKGBUILD` and `.SRCINFO` as an artifact, then clone the AUR repository
with the `AUR_SSH_PRIVATE_KEY` secret and pinned host keys, and run
`publish-aur.sh --push`.

`release.yml` gets an `aur` job after `release`, for published versions
without a pre-release suffix.

## Likely files

`.github/workflows/aur.yml`, `.github/workflows/release.yml`.

## Acceptance criteria and tests

- actionlint passes for both workflows.
- A stable release publishes to the AUR; a pre-release and a manual Release
  run do not.
- Without the secret, a publishing run fails with a message saying so.
- The pinned host keys match the fingerprints aur.archlinux.org publishes.

## Evidence

- actionlint (latest, via `go run`): `aur.yml` and `release.yml` clean.
- The pinned host keys are the ones `ssh-keyscan aur.archlinux.org` returned
  on 2026-11-08. Their fingerprints match the local `known_hosts` entries
  from cloning `aur/` and the ones https://aur.archlinux.org/ lists (fetched
  the same day): ED25519
  `RFzBCUIt…Z11+4`, ECDSA `uTa/0Pnd…GOCI`, RSA `5s5cIyRe…d3Q8`.
- The steps it runs (`make aur-package`, `publish-aur.sh --push`) passed
  locally (AUR-02, AUR-03).
- Not run on GitHub: the workflow, the skipping of pre-releases and manual
  Release runs, and the missing-secret error are checked only by reading
  them. The first run is AUR-07.
