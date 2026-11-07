# AUR-05 — README install instructions

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** AUR-01
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

An Arch Linux row in the README's download table and an **Arch Linux**
section under Linux with the `paru`/`yay` command.

## Likely files

`README.md`.

## Acceptance criteria and tests

- The link points at https://aur.archlinux.org/packages/agent-sessions-bin.

## Evidence

- README: an Arch Linux row in the download table and an **Arch Linux**
  section with `paru -S agent-sessions-bin` / `yay -S agent-sessions-bin`.
- The links point at https://aur.archlinux.org/packages/agent-sessions-bin.
  That page exists only after the first push (AUR-06).
