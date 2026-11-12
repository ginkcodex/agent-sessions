# End chat menu

## Goal

The composer's End chat button becomes a menu with two ways out of a live
chat:

- **End the chat**, exactly as today: asks while the agent works, ends the
  chat, and leaves it exited with Dismiss and Restart.
- **Switch to terminal**: stops the chat and continues the session in a
  terminal tab at once. No Dismiss and Restart: the ended chat is not
  offered again, the terminal takes over.

One click on Switch to terminal moves a session from a chat to a terminal,
today a manual sequence of End chat, Dismiss and Resume.

## Design

### The menu

`ChatComposer.svelte`'s power button opens a small anchored menu in the
same style as `SessionHeader`'s action menus (`.action-dropdown-menu`,
`.action-menu-item`, outside-pointerdown and Escape close). The composer
sits at the window's bottom, so the menu opens upward. It closes before an
item's action runs. Items:

- **End the chat** (always): the current `endChat()`.
- **Switch to terminal** (only while the chat has a saved session): asks
  while the agent works, then switches.

### Switch to terminal

The confirmation is the end-chat one with switch wording, from a new pure
`switchToTerminalQuestion(phase, agentName)` in `chat.ts`:
"Switch to terminal?", "<Agent> is still working. Switching stops its
turn.", danger "Switch". An idle chat switches at once.

The sequence is coordinated by `ChatOpener` (`chatOpen.switchToTerminal`),
which owns the one-place-at-a-time rule between chats and terminals:

1. `chats.close(chatId)` — stops the agent; the engine emits `chat:exit`
   before this returns (watch sends the exit, then closes `done`, and
   `Close` waits on `done`), so the frontend already knows the chat ended.
2. `chats.dismiss(chat.key)` — the ended chat goes at once. Dismiss and
   Restart do not show. A late exit event for a removed chat is ignored.
3. `terminals.runResume(ref, cwd, title)` — the session resumes in a panel
   terminal tab (`PanelResume`, `codex resume <id>` etc.), which takes
   focus, as "Open in built-in terminal" does.

A failure before step 2 keeps the chat and its ended UI; a failure to
start the terminal says so in a toast and the session is still saved.

### The composer

A new `onSwitchToTerminal` prop, wired from `TranscriptView` like
`onRestart`, keeps the composer free of terminal and app state. The item
shows only when the prop is given and `chat.session` is set.

## Decisions

1. **Switching asks while the agent works**, worded for switching, as End
   chat does: both stop a running turn (asked and confirmed by the user).
2. **The item hides until the chat has a saved session**: a resume command
   names a session ID, and a chat whose agent has not saved one has none.
3. **The terminal is always the panel tab**, whatever `launcher.openMode`
   says: the feature is "resume into a terminal tab", not into an external
   terminal window.
4. **The ended chat is dismissed, not kept**: the terminal tab is the
   continuation; a Dismiss/Restart offer would compete with it.
5. **The menu opens upward** from the composer's bottom toolbar.

## Risks

- If the terminal tab fails to start after the chat went, the user is left
  with a saved session and no live surface; the toast offers the error and
  Resume stays available in the header.
- A Codex session Codex itself still holds (about a minute) may refuse the
  resume; the terminal shows the CLI's own error, as a manual resume does.
- Upward menus near the window's top edge (short windows) are clipped; the
  composer is always at the bottom edge.

## Release criteria

- The power button opens a menu with "End the chat" and, for a saved
  session, "Switch to terminal"; outside click and Escape close it.
- "End the chat" behaves exactly as today, including the working-turn
  question and the Dismiss/Restart ended state.
- "Switch to terminal" on an idle chat ends the chat and opens a focused
  panel terminal tab resuming the session, with no Dismiss/Restart; on a
  working chat it asks first and cancelling changes nothing.
- A chat without a saved session shows "End the chat" alone.
- Tests: SSR for the menu and its items, the question wording, the switch
  sequence (close, dismiss, resume) and the failure paths; `npm test`,
  `npm run check`, `npm run build`.
