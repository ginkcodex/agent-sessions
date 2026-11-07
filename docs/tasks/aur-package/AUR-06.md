# AUR-06 — First push of the current release to the AUR

- **Status:** 🟡 Partial (on the AUR; paru on an Arch desktop and app start not checked)
- **Estimate:** 0.5 h
- **Depends on:** AUR-03
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

The maintainer, from the AUR account that owns `agent-sessions-bin`:

```sh
make aur-package
git -C aur config user.name '...'; git -C aur config user.email '...'
bash packaging/arch/publish-aur.sh --push build/aur aur
```

## Acceptance criteria and tests

- https://aur.archlinux.org/packages/agent-sessions-bin shows the version.
- `paru -S agent-sessions-bin` (or `yay`) installs it on an Arch machine,
  and the app starts.

## Evidence

- Pushed by the maintainer on 2026-11-08: the AUR RPC
  (`/rpc/v5/info?arg[]=agent-sessions-bin`) reports 1.3.2-1, maintainer
  `ginkcode`; the AUR repository has one commit, `a827b7c Update to 1.3.2-1`.
- A fresh `git clone https://aur.archlinux.org/agent-sessions-bin.git`
  passed `verify-aur-package.sh` (build, install, 157 libraries resolved,
  server reports 1.3.2, clean removal). Its `.SRCINFO` equals the one makepkg
  writes, and its `PKGBUILD` and `LICENSE` equal the checked `build/aur/`.
- Not checked: `paru`/`yay` on a real Arch desktop, and starting the app.
