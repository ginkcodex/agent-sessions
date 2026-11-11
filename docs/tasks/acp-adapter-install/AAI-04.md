# AAI-04 — Install button and review card in the chat setup

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** AAI-03
- **Plan:** [acp-adapter-install.md](../../plan/acp-adapter-install.md)

## Scope

- Move the managed-tools review card and progress out of `CodeChecks.svelte`
  into a shared component, used by code checks and chat.
- `AdapterMissing` shows **Install** when the agent has `installTool` and
  the host supports managed tools: propose, review card, install, progress
  with Cancel, then reload the chat agents. A failure keeps the copy command.
- The mock API proposes and installs the adapters (Install succeeds for
  Claude, fails once for Codex) so the flow can be seen in `npm run dev`.
- An old server (no `installTool`) shows only the copy command.

## Likely files

- `frontend/src/lib/components/chat/AdapterMissing.svelte`,
  `ChatAdapters.svelte`
- `frontend/src/lib/components/common/go-to-file/CodeChecks.svelte` and a new
  shared component
- `frontend/src/lib/stores/codeDiagnostics.svelte.ts` (or a small managed-tools
  store split from it), `frontend/src/lib/managedTools.ts`
- `frontend/src/lib/mock/mockApi.ts`, tests under `frontend/test/`

## Acceptance criteria

- Nothing is downloaded before the user approves the card.
- The card names the host, package, version, source, integrity, Node.js and
  destination.
- After a successful install the agent shows as ready and New ▾ offers its
  chat without reloading the window.
- Code checks' install flow behaves as before.

## Tests

SSR and store tests for the chat install flow (propose, approve, cancel,
failure, old server) and the existing code-checks tests. `npm test`,
`npm run check`, `npm run build`.
