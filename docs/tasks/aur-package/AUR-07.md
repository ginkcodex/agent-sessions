# AUR-07 — CI secret and the first automated update

- **Status:** ⬜ Not started
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
