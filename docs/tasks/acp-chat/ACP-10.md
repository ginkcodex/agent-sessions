# ACP-10 — Permission cards and failure paths

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `PermissionCard.svelte`: the tool call title and kind, a diff or command
  preview when present, and one button per option, labelled with its kind.
  There is no default option and Enter does not approve. `allow_always`
  options say that the choice lasts beyond this chat (Codex's
  `accept_execpolicy_amendment` writes an exec-policy rule).
- After an answer, the chat follows the turn's `stopReason`: a reject ends
  the turn `cancelled` for Codex and OpenCode, and Claude goes on.
- Fail-closed paths: closing the chat, cancelling, the agent exiting, a host
  switch and quitting all clear pending cards. The backend answers them
  `cancelled`.
- `AdapterMissing.svelte`: the install command to copy, and the host it
  applies to.
- An exit or error banner with the stderr tail, and Restart.
- A reloaded page loses the permission cards: `ChatList` returns no pending
  requests, so the store (ACP-08) finds the chat but not its cards. Add the
  pending requests to `ChatState` (engine and binding) and seed the queue
  from them in `chats.init()`, or cancel them on reload.
- An auth error shows the agent's `authMethods` description (for example
  "Run `opencode auth login`") with an action that opens a terminal tab in
  the chat's directory.

## Likely files

- `frontend/src/lib/components/chat/`
- `frontend/src/lib/stores/chats.svelte.ts`

## Acceptance criteria

- No path approves a permission without a click on an allow option.
- A missing adapter never starts a process.

## Tests

Component tests for card rendering, keyboard behavior and each clearing path.
Go tests (in ACP-06) cover the backend side.

## Evidence

Done:

- **Reload.** `engine.ChatState` gains `permissions`: the requests still
  waiting, oldest first (`pendingPermission.n` keeps the order). `ChatList`
  returns them, and `chats.init()` seeds the cards from them, so a reloaded
  window can answer them.
- **`lib/chat.ts` `permissionView`** (pure):
  - the title (or tool name) and kind;
  - the command from `rawInput.command`, as a string (Claude, OpenCode) or
    argv (Codex), with shell quoting;
  - a unified diff per `diff` content;
  - the other paths from `locations`;
  - the options in the agent's order, each with its kind in words
    (`Allow once`, `Always allow`, `Reject once`, `Always reject`), a tone,
    and `lasting` for `allow_always`.
- **`components/chat/PermissionCard.svelte`**, shown in the live tail for
  each waiting request (it replaces ACP-09's muted lines):
  - every button is `type="button"`;
  - none is a default and none takes focus;
  - Enter on an allow option is held back (Space and a click still choose);
  - one answer at a time, and a failed answer shows a toast;
  - `allow_always` options say they can last beyond this chat;
  - a request without options says to cancel the turn.

  While a request waits, the status line reads "<agent> waits for your
  answer."
- **Clearing.** `turn_end` now also clears the cards. Exit, a host switch
  (`reset`) and `permission_resolved` already did. The backend answers
  `cancelled` on close, cancel, agent exit and quit (ACP-06 and ACP-07).
- **`components/chat/AdapterMissing.svelte`**: the agent, the host
  ("this computer" or the host's label), the problem, and the install
  command with Copy. It points at `[chat.adapters]` when an override is the
  problem.
- **`components/chat/ChatAdapters.svelte`** in Settings › Coding agents: each
  agent is either ready (with its path) or shown with `AdapterMissing`. The
  `New ▾` menu has "Set up chat…" when some adapter is missing; it opens that
  section.
- **Restart.** An ended chat's composer has Restart next to Dismiss.
  `chatOpen.restart(chat, saved)`:
  - continues the session when the agent saved it, and refuses when a
    terminal tab runs it;
  - otherwise starts a new chat in the same directory and selects it;
  - dismisses the ended chat only once the new one runs.
- **Auth.** A login error lists the agent's `authMethods`, each with its name
  and description. Where the terminal panel exists, "Open a terminal in
  <dir>" opens a shell there.
- **Mock agent.**
  - The permission request carries a diff and `rawInput`.
  - `chatList` returns the pending request.
  - The OpenCode install command matches the backend's.
  - `?mockChatMissing=opencode` reports adapters as missing in the browser
    mock.

Acceptance:

- **No approval without a click on an allow option.** Nothing chooses an
  option except a click (or Space) on a card button. Enter is held back on
  allow options; the composer's Enter cannot send while a turn runs. Every
  exit path clears the cards, and the backend answers `cancelled`.
- **A missing adapter never starts a process.** `ChatLaunch` fails before
  anything is spawned (`TestChatAgents`, `TestChatFailedStartReleasesSession`
  with a failing launch), and the UI offers no chat entry for that agent.

Tests run:

- `go test -race ./internal/engine ./internal/app`: passed.
  `TestChatPermissionAnswered` now checks that `Chats()` lists the waiting
  request, with its tool call and options, and drops it once answered.
- `node --test test/chat-ui.test.mjs`: 13/13 passed. New coverage:
  - `permissionView`: diffs, paths, kinds, tones, lasting, unknown kinds,
    argv quoting;
  - SSR of the card: buttons, no autofocus, the kind labels, the command,
    the diff lines, the lasting note, no options;
  - `AdapterMissing`: the host, the problem, the install command and Copy,
    the override hint;
  - the live tail's cards, the waiting line, and the auth methods (the
    terminal action only when there is a panel);
  - the composer's Restart;
  - `chatOpen.restart`: continue, new, a running chat, a failed restart,
    and the terminal refusal.
- `node --test test/chat-permission-card.test.mjs` (new, jsdom): 2/2 passed.
  - Nothing is focused after mount.
  - Enter is prevented on both allow options and not on reject.
  - A click answers once, and the buttons are disabled until the reply.
  - A failed answer shows a toast.
- `node --test test/chats-store.test.mjs`: 17/17 passed. New coverage:
  - a reload seeds the waiting request, which can then be rejected (the
    agent sees the reject);
  - close, turn end, close-all (quit) and a host switch each clear the
    cards.
- `npm test`: 1274 passed, 1 skipped, 0 failed.
- `svelte-check`: 0 errors, 0 warnings.
- `npm run build`: passed.
- A manual browser run in mock mode, with headless Chrome over CDP and
  screenshots inspected:
  - the card showed the kind, title, diff, three options with their kinds,
    and the lasting note, with no focus inside it;
  - a real Enter key press on a focused Allow left the request waiting;
  - Reject cleared the card and ended the turn;
  - "login" showed the auth method and "Open a terminal in <dir>", which
    opened a Shell tab;
  - "crash" showed the stderr tail, and Dismiss and Restart; Restart
    continued the session in a new chat;
  - with `?mockChatMissing=opencode`, `New ▾` showed "Set up chat…", which
    opened Settings › Coding agents with OpenCode's install command;
  - no console errors.

Not checked:

- Real agents and the Wails backend (ACP-14), including how each agent
  reacts to a reject after the card (the plan's `stopReason` note).
- Copy in the desktop app's clipboard. Only the browser path ran.
- A reload in the desktop app. The store test covers the seeding against
  the mock.

