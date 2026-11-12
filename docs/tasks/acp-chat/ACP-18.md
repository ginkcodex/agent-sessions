# ACP-18 — Session header for a new chat before it is saved

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

A new chat (New Claude Code chat, New Codex chat…) selects its session
before the agent saves it, so the transcript view has no session metadata
yet. It showed a one-line "New <agent> chat <dir>" bar instead of the
session header, and the layout jumped when the metadata arrived. The view
now shows the usual `SessionHeader` for that session:

- `pendingChatMeta` (`chat.ts`) builds a placeholder `SessionMeta`: the
  agent, the chat's directory, the title "New <agent> chat", Live while the
  chat runs, no counts, tokens or times.
- `SessionHeader` takes `pending`: the same rows (navigation, agent badge,
  Live, "● Live in chat", New ▾, Find, display mode, title, directory with its
  copy button), with Resume and the ⋯ menu disabled ("Available once the agent
  saves this session"), and without the message count, token use and times.
- Once the agent saves the session, the real metadata replaces the
  placeholder in the same component.

## Likely files

- `frontend/src/lib/chat.ts`
- `frontend/src/lib/components/transcript/SessionHeader.svelte`
- `frontend/src/lib/components/transcript/TranscriptView.svelte`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria and tests

- The pending header has the top, title and meta rows, the display mode
  switch and Find, the title "New <agent> chat" and the directory.
- Resume and ⋯ are disabled; no Resume ▾, message count, tokens or times.
- A saved session's header is unchanged.
- `npm test`, `npm run check`, `npm run build`.

## Evidence

- Done: as in the scope; the old `chat-header` bar and its styles are gone.
- Tests:
  - `chat-ui.test.mjs`: "a new chat keeps the session header layout before
    its agent saves the session" checks `pendingChatMeta` and the rendered
    pending header against a saved one; 25/25 pass.
  - `npm test`: 1340 tests, 1339 pass, 0 fail, 1 skipped (as before).
  - `npm run check`: 0 errors, 0 warnings. `npm run build`: OK.
- Not checked: the header in the app window with a real agent (no display
  here).
