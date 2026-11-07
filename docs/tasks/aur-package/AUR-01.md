# AUR-01 — PKGBUILD template and render script

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** —
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

`packaging/arch/PKGBUILD.in`: `agent-sessions-bin` for x86_64 and aarch64,
unpacking the release's `.deb` and installing `LICENSE`. It declares the
libraries the binary links directly, `provides`/`conflicts` `agent-sessions`,
and sets `!strip`/`!debug`.

`packaging/arch/render-pkgbuild.sh [--pkgrel N] VERSION SHA256SUMS OUTDIR`
fills it from the release's `SHA256SUMS`, copies `LICENSE`, and removes a
stale `.SRCINFO`.

## Likely files

`packaging/arch/PKGBUILD.in`, `packaging/arch/render-pkgbuild.sh`.

## Acceptance criteria and tests

- The checksums in the PKGBUILD are the release's for both `.deb` files.
- Pre-release versions, a pkgrel below 1, and versions missing from
  `SHA256SUMS` are refused with a message.
- No placeholder is left unfilled; shellcheck passes.
