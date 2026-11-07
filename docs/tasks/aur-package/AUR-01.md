# AUR-01 — PKGBUILD template and render script

- **Status:** ✅ Done
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

## Evidence

- `render-pkgbuild.sh` for 1.3.1 and 1.3.2 against each release's published
  `SHA256SUMS`: the PKGBUILD's `sha256sums_x86_64`/`sha256sums_aarch64` equal
  the `.deb` lines (1.3.2: `c12aa0fd…40cd`, `e74f8b2d…112c`).
- Refusals, each exit 1 with its message: `1.4.0-beta.1` (not stable),
  `--pkgrel 0`, `9.9.9` (not in `SHA256SUMS`). `--pkgrel 2` writes
  `pkgrel=2`.
- shellcheck (koalaman/shellcheck:stable): clean.
- `depends` first had `glibc glib2 gtk4 webkitgtk-6.0`; namcap in AUR-02
  found `libsoup3` and `libx11` linked directly, so they were added.
