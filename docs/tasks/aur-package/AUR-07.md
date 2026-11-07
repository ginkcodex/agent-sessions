# AUR-07 — CI secret and the first automated update

- **Status:** 🟡 Partial (secret set; workflow not run, not on main yet)
- **Estimate:** 1 h
- **Depends on:** AUR-04, AUR-06
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

1. Make a key for CI alone:
   `ssh-keygen -t ed25519 -N '' -C 'agent-sessions CI → AUR' -f aur-ci`.
2. Add `aur-ci.pub` to the AUR account (My Account → SSH Public Key; the
   field takes one key per line).
3. Store the private key `aur-ci` as the repository secret
   `AUR_SSH_PRIVATE_KEY`, then delete both files.
4. Run the AUR workflow by hand with publish off, then on, for the current
   release. The second run should report that the AUR already has it.
5. Release the next stable version and check that it reaches the AUR.

## Acceptance criteria and tests

- The next stable release updates the AUR package without manual steps.
- A pre-release leaves it alone.

## Evidence

- Steps 1–3 done by the maintainer on 2026-11-08: the CI key is on the AUR
  account, and `gh secret list` shows `AUR_SSH_PRIVATE_KEY` (set
  2026-11-07T19:29:57Z).
- Not done: `aur.yml` is not on `main` yet (`develop` is not pushed), so the
  workflow cannot run (steps 4–5).
