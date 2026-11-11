# ACP-11 — Reconcile with saved transcripts and continue existing sessions

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-10
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Using what [the spike](../../plan/acp-chat-spike.md) found for each agent:

- Match message and thought items by `messageId`, removing OpenCode's
  `:reasoning:N` suffix. Match tool items by tool call ID for Claude and
  OpenCode. For Codex (live `exec-…` IDs, saved `call_…`), drop a finished
  turn's tool items once its last `messageId` is saved. The live tail never
  duplicates a saved message.
- After a turn ends, ask for a targeted refresh and drop the covered items
  once the saved transcript arrives.
- Continue an existing session in chat through `session/load` or
  `session/resume` where supported. A replay through `session/load` does not
  duplicate what the transcript already shows. The spike found both work for
  all three agents, so no in-memory log is needed.

## Likely files

- `frontend/src/lib/chatTail.ts`, `frontend/src/lib/stores/chats.svelte.ts`
- `internal/engine/chat.go`
- `docs/plan/acp-chat.md` (record the per-agent behavior)

## Acceptance criteria

- After a turn, the transcript shows each message once.
- The behavior of each agent is recorded in the plan.

## Tests

Reducer tests with saved-record fixtures for each agent (from the spike), and
engine tests for the refresh request.

## Evidence

What was already in place from ACP-06 and ACP-08: matching by `messageId`
with the `:reasoning:N` suffix removed, tool matching by ID, dropping an
ended turn once its last message is saved (Codex), prompt matching by text,
the rescan after each turn, and `session/resume` with a `session/load`
fallback that drops the replay. This task checked that against what each
agent saves and closed the gaps:

- **Leftovers of cancelled and failed turns.** Codex and Claude never save a
  cancelled turn's partial reply, and a failed prompt may never be saved.
  Those items stayed in the tail for good, below newer saved messages.
  `reconcile` now drops every item of a turn before one the saved records
  reach by an agent's own ID (`frontend/src/lib/chatTail.ts`).
- **Prompts with an ID the saved record lacks.** Claude's replayed prompts
  carry a UUID, but Claude saves prompts with no ID. A user item whose ID is
  not found now falls back to matching its text.
- **Agents that cannot continue a session.** `ChatManager.Start` refuses a
  resume with `ErrChatCannotContinue` when the agent advertises neither
  `session/resume` nor `session/load`, instead of sending a `session/load`
  the agent did not offer (`internal/engine/chat.go`). The plan's release
  criterion now says the agent refuses it with a clear message: the
  capabilities are only known once the adapter runs.
- **Plan:** "Saved transcript and live tail" in
  [acp-chat.md](../../plan/acp-chat.md) records each agent's behavior in a
  table (prompt, reply, tools, after a cancel) and the continue rules.

Tests:

- `frontend/test/chat-reconcile-agents.test.mjs` (6 tests) replays
  `frontend/test/fixtures/acp-agents.json` turn by turn for each agent: what
  goes while a turn runs, what goes once it ends, cancelled turns, and the
  next saved turn clearing them. The fixtures are synthetic: their IDs,
  update order and saved records follow the spike captures, which were
  compared with `agent-sessions-cli show` for the spike sessions (IDs and
  part kinds only; the spike's sent prompts equal the saved prompt text for
  all three agents). Real conversation logs were not copied into the repo.
- Removing either `reconcile` change makes these tests fail (checked by
  reverting each change on its own: 3 failures and 1 failure).
- `internal/engine/chat_test.go`: `TestChatContinuesSession` (resume when
  advertised, load otherwise, new session, refusal with neither, and the
  refused chat owns nothing). `TestChatLoadDropsReplay` now also checks that
  the session was opened with `session/load`. `TestChatAuthRequired` checks
  that a failed turn still asks for a rescan. The fake agent gained an
  `opened` prompt and `ACPTEST_NO_LOAD`.
- `go test -race -count=1 ./internal/engine ./internal/acp/... ./internal/app`:
  pass. `go vet` on those packages: clean. `gofmt -l` lists only four files
  this task did not touch.
- `npm test`: 1280 pass, 1 skipped, 0 fail. `svelte-check`: 0 errors,
  0 warnings.

Not checked: real agents. Whether the rescan after a turn alone brings the
records, or the watcher brings them a moment later, depends on when each
agent writes; ACP-14 checks it with the real adapters, along with OpenCode's
`session/resume`, which the spike did not try.
