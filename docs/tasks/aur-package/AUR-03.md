# AUR-03 — Publish script for an AUR checkout

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** AUR-02
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

`packaging/arch/publish-aur.sh [--push] PKGDIR AUR_CHECKOUT` copies
`PKGBUILD`, `.SRCINFO` and `LICENSE` into a clone of the AUR repository and
commits them on `master`, the only branch the AUR accepts. It pushes only
with `--push`. The local clone lives in `aur/`, which git ignores.

## Likely files

`packaging/arch/publish-aur.sh`, `.gitignore`.

## Acceptance criteria and tests

- Refuses a `.SRCINFO` that does not match the PKGBUILD's version, a checkout
  whose origin is not the `agent-sessions-bin` AUR repository, uncommitted
  changes, a missing commit author, and a local `master` with commits the AUR
  lacks.
- Works on an empty AUR repository (first push) and on one with history
  (update), and is a no-op when the AUR already has the files.
- A commit made without `--push` is pushed by a later run with `--push`.
