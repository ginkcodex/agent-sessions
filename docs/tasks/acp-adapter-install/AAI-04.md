# AAI-04 — Install button and review card in the chat setup

- **Status:** ✅ Done
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

## Evidence

- `ManagedInstallReview.svelte` and `ManagedInstallProgress.svelte` (in
  `components/common/`) hold the review card and the progress card.
  `CodeChecks.svelte` uses them with the same text as before.
  `test/helpers.mjs` renders the real cards wherever they are imported.
- `managedTools.ts` gained `AdapterToolName`, `ADAPTER_TOOLS` and
  `ManagedInstallName`. `MANAGED_TOOLS` still lists only the language
  servers, so automatic downloads never pick an adapter.
- `stores/adapterInstall.svelte.ts` (`AdapterInstaller`) runs one install
  at a time on the shown host:
  - it reads the host ID and the `codeDiagnostics` capability from
    `connectionState()`;
  - it checks the proposal's host, tool, token and expiry;
  - it drops the token locally before Install, and never sends it again;
  - it polls the job every 2 s, at most 300 times, then reloads
    `chats.loadAgents()` however the install ends.
  `App.svelte` resets it on every host reset, alongside `chats.reset`.
- `AdapterMissing.svelte` shows **Install…** when the agent has
  `installTool`, the host has `codeDiagnostics` and there is no override. It
  then shows the review card (host, package and version, source, integrity
  and Node.js note, npm policy, destination, host identity), or progress
  with **Cancel download**, or the error. The copy command stays, as
  "Or install it yourself: run this on …". `ChatAdapters.svelte` marks a
  managed adapter as "this app's copy".
- The code-diagnostics store shows a job whose tool is not a language server
  as idle, and does not rediscover checkers after it. Without this, opening
  code checks during an adapter install would show its progress there.
- Mock: the adapters carry `installTool`. `managedTool` proposes and
  installs them over two status polls: Claude Code installs, and Codex fails
  its first install, then installs. Language servers still get the mock's
  "no downloads" answer.
- Tests (`test/adapter-install.test.mjs`, 9 tests):
  - nothing installs before approval, and Not now sends no token;
  - Install sends its token once, polls, and reloads the agents;
  - failure and cancel show their message and stop polling;
  - these fail closed: an old server, a host without managed installs, a
    refusal, another host's or tool's proposal, an expired offer, and a
    language-server ID;
  - a host switch drops late answers;
  - SSR: the Install button appears or not (old server, no capability,
    override, another install running); the review card's contents; the
    progress and failure views;
  - the mock flow end to end, including a chat started on Codex's app copy.

  A new test in `test/managed-tools.test.mjs` checks that code checks ignore
  an adapter job. Against the old store the test did not pass: the run hung
  on the store's poll timer and was stopped. `test/chat-ui.test.mjs`'s
  AdapterMissing render now passes an old-server fixture. The existing
  code-checks tests pass unchanged (64 across the 7 code-check files).
- `npm test`: 1301 tests, 1300 pass, 0 fail, and 1 skip. That test was
  already skipped before this change.
- `npm run check`: 0 errors and 0 warnings in 619 files.
- `npm run build`: builds, with only the existing chunk-size warning.
- Not checked: the flow in a browser or in the desktop app. There is no
  display here, so `npm run dev` with `?mockChatMissing=claude-code,codex`
  was not looked at. A real install against an engine is AAI-05.
