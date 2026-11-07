# AUR-03 — Publish script for an AUR checkout

- **Status:** ✅ Done
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

## Evidence

- Against copies of the real `aur/` clone (its AUR repository is still
  empty): a run without `--push` commits `PKGBUILD`, `.SRCINFO` and `LICENSE`
  on `master` ("Update to 1.3.1-1"). A second run reports the commit is not
  pushed yet and changes nothing. The origin check passed for the real AUR
  URL.
- Refusals, each exit 1 with its message: a `.SRCINFO` for 1.3.1-1 beside a
  1.3.1-2 PKGBUILD, a `/tmp/…/fake.git` origin, an untracked file in the
  checkout, and no commit author (checked before any file changes).
- With the origin check patched to accept a local bare repository: the first
  `--push` created `master`; a fresh clone pushed 1.3.1-2 on top; a clone
  left behind at 1.3.1-1 fast-forwarded and reported the AUR already had
  1.3.1-2; a local commit not on the remote was refused. A 1.3.2-1 commit
  made without `--push` on an empty remote was pushed by the next run with
  `--push`.
- shellcheck: clean.
- Not checked: a push to aur.archlinux.org itself (AUR-06).
