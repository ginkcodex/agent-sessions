# ACP-06 — Engine chat manager

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-05
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

`internal/engine/chat.go`, `ChatManager`:

- `Start` (new, or continue an existing session: `session/resume` when
  advertised, otherwise `session/load` with the replay discarded), `Prompt`,
  `Cancel`, `Respond`, `SetOption`, `Close`, `CloseAll` and `Agents`.
- Keep each chat's `configOptions` current from `config_option_update` and
  `current_mode_update`, and include them in the chat state.
- A prompt that fails with an auth error (`-32000` "Authentication
  required") is reported as a typed error carrying the agent's
  `authMethods`.
- Chat IDs, and an owner registry keyed by `SessionRef`. A second chat for a
  session is refused with a typed error. `Owned(ref)` is used by manage.
- Emit `chat:update` (with a `seq` per chat) and `chat:exit`. Every
  permission request gets an ID, and all pending requests are answered
  `cancelled` on cancel, close or exit.
- Link a new chat to its `SessionRef` once ACP returns the `sessionId`.
- When a turn ends, ask for a targeted rescan of the agent so the saved
  records arrive quickly.

## Likely files

- `internal/engine/chat.go`, `internal/engine/chat_test.go`

## Acceptance criteria

- No goroutine or process outlives `Close` or `CloseAll`.
- Events for one chat are emitted in `seq` order.
- Ownership is released on every exit path.

## Tests

Race tests with the fake agent: concurrent prompts refused while a turn runs,
close during a permission request, agent crash, CloseAll with several chats,
and ownership refusal and release.

## Evidence

- **Added:** `internal/engine/chat.go`:
  - `ChatManager` with `Start`, `Prompt`, `Cancel`, `Respond`, `SetOption`,
    `Close`, `CloseAll`, `Agents`, `Owned` and `Chats`. Each chat runs its own
    adapter process.
  - `Start` refuses a session that already has a chat
    (`SessionOwnedError`, which wraps `ErrSessionOwned` and names the chat).
    It reserves the session before starting the agent. A continued session
    uses `session/resume` when advertised. Otherwise it uses
    `session/load`, and the updates that arrive before the reply are dropped.
    A new chat is linked to the `sessionId` that `session/new` returns.
  - `Prompt` returns once the turn has started; a second prompt meanwhile is
    `ErrChatBusy`. The turn ends with a `turn_end` event that carries the stop
    reason or the error. An auth error becomes `AuthRequiredError`, and its
    `authMethods` go in the event. After every turn, `Refresh` asks for a
    rescan of the agent.
  - `chat:update` kinds: `started` (the state), `session_update`,
    `permission`, `permission_resolved`, `turn_start`, `turn_end` and
    `config_options`. Then `chat:exit` with `reason` (`closed`, `exited` or
    `crashed`), the error and the stderr tail. A chat's events, `chat:exit`
    included, carry `seq` from 1. They are emitted under one lock per chat,
    so they leave in `seq` order. A turn's end is emitted before the next
    turn can start.
  - Permission requests get IDs (`perm-N`). `Respond` takes one of the
    request's options, or an empty option for cancelled. An unknown option
    or a request that is no longer pending is an error. Cancel, close and the
    agent's exit answer pending requests `cancelled`.
  - `config_option_update` replaces the options. `current_mode_update` sets
    the option whose category (or ID) is `mode`. `SetOption` stores what the
    agent returns.
  - Close ends a start in progress too. The chat's exit watcher waits for
    the turn and the permission waits before it releases the session and
    emits `chat:exit`. `Close` and `CloseAll` return only after that.
- **Engine:**
  - `NewEngine` creates the manager with `ChatLaunch`, `ChatAgents` and a
    targeted rescan. `refreshAgent` reuses the watcher's refresh, now
    `refreshProvider`, and does nothing without the cache.
  - `SetEmitter` passes the emitter on, and `Close` closes every chat first.
  - `Engine.Chats()` returns the manager.
- **Fake agent:** session IDs now include the agent's PID
  (`fake-<pid>-<n>`), so chats in one test get distinct sessions.
- **Tests (`internal/engine/chat_test.go`, with the fake agent):**
  - a new session streams in order and starts a rescan; state, ownership,
    and closing release the session;
  - a concurrent prompt is refused while a turn runs; the turn is cancelled,
    then the next one runs;
  - a permission answered: an option that was not offered, an unknown ID,
    and a second answer are refused;
  - cancel while a permission request is pending;
  - close while a permission request is pending: the request is resolved
    `cancelled`, the turn ends unsuccessfully, `chat:exit` is `closed`, and
    `Close` does not block;
  - an agent crash: the turn error and the `crashed` exit carry the stderr,
    and the session is released and can be opened again;
  - an auth error carries the agent's `authMethods`;
  - ownership: a second chat and a mismatched agent are refused; the
    session is released on close;
  - a failed start (protocol mismatch or launch error) releases its
    reservation;
  - `session/load` replay dropped when the agent has no resume;
  - options: set, stored, emitted, an invalid value refused; `withMode`;
  - `CloseAll` with three chats (one mid-turn, one waiting on a permission,
    one idle): every agent process has exited, every chat sent one `closed`
    exit, no chat or owner is left, and the goroutine count returns to its
    baseline;
  - `CloseAll` during a start;
  - `Engine.Close` closes chats through the engine's emitter;
  - `Agents` with and without a lookup.
  - Every test checks `seq` order where events are involved.
- **Results:**
  - `go test -race -count=10 -cpu 1,4` of the chat tests passed.
  - `go test -race ./internal/engine ./internal/acp/...` passed.
  - The chat tests cover 92.9 % of `chat.go` statements.
  - `golangci-lint` reported nothing in the changed files for linux, windows
    and darwin.
  - `go vet` passed for windows and darwin.
- **Not checked:**
  - No real adapter was run. ACP-14 runs them through the app.
  - The rescan was checked only as a call to `Refresh`. That it brings the
    saved records into the catalog is left to ACP-11 and ACP-14.
  - A permission request that arrives after the agent has exited is
    answered `cancelled` by the code, but no test makes one arrive.
