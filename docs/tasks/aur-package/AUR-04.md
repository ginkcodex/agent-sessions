# AUR-04 — AUR workflow, called after stable releases

- **Status:** ⬜ Not started
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
