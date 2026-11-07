# AUR-06 — First push of the current release to the AUR

- **Status:** ⬜ Not started
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
