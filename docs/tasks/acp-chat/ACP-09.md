# ACP-09 — Chat UI and entry points

- **Status:** ⬜ Not started
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
