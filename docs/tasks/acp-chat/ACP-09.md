# ACP-09 — Chat UI and entry points

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-08
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `components/chat/ChatComposer.svelte`: a multi-line input, Send, and Cancel
  while a turn runs. It is disabled while the chat is starting or has exited.
- `components/chat/LiveTail.svelte`: items rendered through `MessageBubble`
  and `transcript/parts`, below the saved messages in `TranscriptView`, with
  the tail-follow scroll behavior.
- Entry points:
  - "Open in Chat" in the `Resume ▾` menu;
  - a Chat choice in New Session;
  - a "Live in chat" state in the session header.
- Terminal and chat exclusivity. Opening chat for a session linked to a
  running terminal tab, or the reverse, is refused with an action to focus the
  other one. Use the tab ↔ session links.

## Likely files

- `frontend/src/lib/components/chat/`
- `frontend/src/lib/components/TranscriptView.svelte`,
  `SessionHeader.svelte`
- `frontend/src/lib/stores/newSession.svelte.ts`,
  `stores/sessionLinks.svelte.ts`

## Acceptance criteria

- Streamed text appears as it arrives, with the mock agent in `npm run dev`.
- Entry points are hidden for agents whose adapter is missing (see ACP-10) and
  on hosts without the capability.

## Tests

SSR or component tests for the composer states, tail rendering and the
exclusivity refusal. `npm run check` and `npm run build`.

## Evidence

Done:

- `lib/chat.ts` (pure): `tailMessages` turns the live tail into transcript
  messages. Each prompt becomes a user message and each turn's items one
  assistant message, with IDs starting `live:`. Thoughts become reasoning,
  plans a Markdown list, and tools tool parts, plus a patch part for diffs and
  "Exit code N" for a failed command. It also has `toolStatus`,
  `planMarkdown`, `unifiedDiff` and `chatAgentAvailable`.
- `components/chat/LiveTail.svelte`: the tail through `MessageBubble`, below
  the saved messages. Under it:
  - "Starting <agent>…" or "Opening the session in <agent>…";
  - "<agent> is working…";
  - one muted line per waiting permission request (ACP-10 replaces these
    with cards);
  - the turn's error, or a note when a turn stopped short (cancelled, token
    limit, request limit, refusal);
  - how the chat ended, with the agent's stderr tail.
- `components/chat/ChatComposer.svelte`:
  - a multi-line input: Enter sends; Shift+Enter, or an IME composition,
    does not;
  - Send works only when idle with text;
  - text that could not be sent comes back, with a toast;
  - Cancel while working; End chat; Dismiss once ended;
  - disabled while starting and once ended;
  - takes focus when a chat is opened, even if it mounts later.
- `stores/chatOpen.svelte.ts`: opens chats from the UI and enforces
  exclusivity.
  - Open in chat on a session a running terminal tab drives shows "This
    session runs in a terminal" with "Show terminal".
  - Resume on a session a chat drives shows "This session is open in a chat"
    with "Show chat".
  - A second open shows the existing chat.
- `TranscriptView.svelte`:
  - the tail and composer; the scroll pills moved inside the messages frame,
    so they stay above the composer;
  - an `$effect` reconciles the tail with saved messages;
  - a new chat's session shows before its first save, with a minimal header;
  - the Resume refusal.
- `SessionHeader.svelte`: "● Live in chat"; "Open in chat" in `Resume ▾`; a
  Chat group in `New ▾`.
- `WorkspaceStart.svelte`: a "New chat" button.
- `connection.svelte.ts`: `canChat`, which is local only until ACP-13.
  `HostCapabilities.acpChat`.
- `chats.viewFor(ref)` keeps an ended chat's tail and exit notice on screen
  until it is dismissed.
- Fixes found in the browser run:
  - `chatOpen.refresh()` read reactive state inside `SessionHeader`'s
    effect, so every agent load started the next one and the page hung. It
    now reads untracked.
  - A resume passed the reactive `meta.ref` proxy, which the mock's
    `structuredClone` could not copy. It now passes a plain copy, and the
    mock copies it too.

Tests run:

- `node --test test/chat-ui.test.mjs`: 9/9 passed. Covers:
  - `tailMessages` grouping, tools, plans, patches and exit codes;
  - `unifiedDiff`, `toolStatus` and `chatAgentAvailable`;
  - SSR of the composer in each phase (starting, idle, working, exited);
  - SSR of the live tail (messages, permissions, starting and resuming,
    stop notes, error, crash with stderr);
  - the header badge;
  - the opener: refusing a session a terminal tab runs, refusing Resume for
    a chat's session, showing the existing chat, start and new chat, a failed
    start, no adapter, and no capability.
- Fixtures were added to `helpers.mjs` (SessionHeader),
  `conversation-print-menu.test.mjs` and `workspace-stars.test.mjs` for the
  new store imports.
- `npm test`: 1266 passed, 1 skipped, 0 failed.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed.
- A manual browser run: Vite in mock mode, driven by headless Chrome over
  CDP, with screenshots inspected.
  - Open in chat on a Claude Code session: the composer took focus; a
    prompt streamed (plan, then "Claude Code is working…"); the saved turn
    replaced the tail; "● Live in chat" showed; Resume showed the refusal
    toast; End chat showed "The chat ended." and Dismiss.
  - New ▾ → New Codex chat: the minimal "New Codex chat" header; a sent
    prompt streamed; the saved session then loaded with its full header and
    "● Live in chat".
  - No console errors or exceptions.

Not checked:

- Real agents and the Wails backend (ACP-14).
- The "Show terminal" refusal in a browser: the mock has no running tab for
  the session. The unit test covers it.
- There is no automated test that `chatOpen.refresh()` stays untracked
  inside an effect. SSR runs no effects; the browser run covers it.
- Remote hosts: entry points stay hidden off this computer until ACP-13.
