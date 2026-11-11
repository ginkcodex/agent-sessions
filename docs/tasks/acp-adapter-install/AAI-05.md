# AAI-05 — QA with real installs

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** AAI-04
- **Plan:** [acp-adapter-install.md](../../plan/acp-adapter-install.md)

## Scope

Install each adapter through the app and chat with it:

- on Linux and on macOS;
- on a WSL distribution and on an SSH host;
- with a host Node.js 22 or later, and with an older one (pinned Node.js
  download).

Check that each adapter starts without its optional packages. If one does
not, change its entry to keep them and record the download size. On the
Windows desktop app, check that a local chat shows only the copy command.
Record results in `docs/plan/acp-adapter-install-qa.md`.

## Likely files

- `docs/plan/acp-adapter-install-qa.md`

## Acceptance criteria

Only checks that actually ran are marked passed. Failures become new tasks.

## Tests

The manual QA above, plus `make lint test`, `npm test`, `npm run check` and
`npm run build` on the release commit.
