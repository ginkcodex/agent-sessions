# ACP-14 — QA with real agents on every host

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-11, ACP-13
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Run the release criteria by hand with Claude Code, Codex and OpenCode:

- on Linux;
- on a WSL distribution;
- on an SSH host;
- in a Windows test build made as CLAUDE.md describes.

Record each check, its result, the adapter versions and anything not run in
`docs/plan/acp-chat-qa.md`.

## Likely files

- `docs/plan/acp-chat-qa.md`

## Acceptance criteria

Only checks that actually ran are marked passed. Failures become new tasks.

## Tests

The manual QA above, plus `make lint test`, `npm test`, `npm run check` and
`npm run build` on the release commit.
