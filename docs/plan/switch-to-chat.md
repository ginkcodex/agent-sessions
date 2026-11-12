# Switch to chat

## Goal

The mirror of [end-chat-menu](end-chat-menu.md): move a session from a
terminal tab to a live chat in one click.

- **Switch to chat** on a terminal tab's title context menu ends the
  tab's agent and continues the saved session in a live chat at once.
  The tab goes; the chat takes over.
- The reverse ("Switch to terminal" in the composer's End chat menu) is
  done; together the two move a session either way without the manual
  sequence of close, dismiss and resume.

## Design

### The menu item

`TerminalPanel.svelte`'s tab context menu (`tabItems`), right after
"Show Transcript", gets **Switch to chat** on the same terms that item
has: agent tabs only, the focused agent's (or first agent's) pane of a
split tab.

- **Hidden** when the tab's agent cannot chat on this connection
  (`chatOpen.canChat`), as SessionHeader hides "Open in chat".
- **Disabled** with Show Transcript's reasons when the chat has no saved
  session yet ("The session appears once the agent saves it…") or the tab
  runs on another host ("Switch to … to see this session"), and while a
  chat is already starting (`chatOpen.opening`).
- The item's shape (label, disabled, title) comes from a pure
  `switchToChatItem` in `tabActions.ts`, beside `tabsToClose`; the panel
  only supplies `onSelect`.

### The question

Killing the terminal's CLI stops its running turn, so switching asks
while the agent works — with wording for switching, from a pure
`switchToChatQuestion(attention, agentName)` in `chat.ts` beside its two
siblings. A terminal tab knows its agent's state only through the
attention tracker (`attention.ts`: recent output, signals, and the
session file's recorded turn):

- `working`: "Switch to chat?", "<Agent> is still working. Switching
  stops its turn.", danger "Switch".
- `needs` (an approval prompt or a notification waits): "Switch to
  chat?", "<Agent> is waiting for your answer. Switching discards it.",
  danger "Switch".
- `idle`, `done`, `exited`: no question, switch at once — there is no
  turn to stop.

The panel asks through `filePrompt.ask` as the composer does, then runs
the switch.

### The switch

Coordinated by `ChatOpener.switchToChat(tab)` — `ChatOpener` owns the
one-place-at-a-time rule between chats and terminals (the plan in
[end-chat-menu](end-chat-menu.md)):

1. `terminals.close(tab.id, true)` — only the pane that runs the session
   goes; split siblings stay. Its CLI is ended (SIGHUP, SIGKILL after a
   grace). `confirmed` because the question was ours: no second confirm
   bar. A pane whose CLI already exited just goes.
2. `openSession(meta)` with a minimal meta built from the tab (ref =
   `tab.session`, `cwd` = `tab.cwd || tab.dir`, title = `tab.label`, as
   `pendingChatMeta` builds one). The mirror guard `refuseChat` passes —
   the pane is gone — so `chats.start` continues the saved session
   (`resume`) in a new chat and `focus()` takes the user to its composer.

Failures fall on `run()`'s existing handling: the session-in-use toast
(Codex keeps its session for about a minute after its CLI closes it)
offers **Try again**, which re-runs `openSession`; any other failure says
"Could not start the chat". In both the session is saved in the catalog
and the pane is already gone.

## Decisions

1. **Ask while the agent works or waits** (attention `working` or
   `needs`), worded for switching; an idle or finished tab switches at
   once. Confirmed by the user, matching End chat and Switch to
   terminal. The attention state is a heuristic (1.5 s output window,
   plus the session file's turn), accepted.
2. **The item lives in the tab-title context menu only**, where the user
   asked for it.
3. **Only the session's pane closes**; a split tab's other panes are the
   user's other work and stay.
4. **The switch is `ChatOpener.switchToChat`**, mirroring
   `switchToTerminal`, so both directions of the exclusivity rule live in
   one store.
5. **No meta is fetched**: the tab already carries ref, directory and
   title; the catalog refreshes counts and titles when the chat starts.

## Risks

- The kill is fire-and-forget (`api.terminalClose`), and Codex can hold
  a session lock about a minute after its CLI closes it, so the chat may
  refuse to resume right after a switch: the existing "Codex is still
  using this session" toast with Try again covers it, as for a manual
  resume.
- A long silent tool run can look `idle` to the attention tracker and
  switch without asking; accepted with decision 1.
- Tabs on another host cannot switch (the chat runs on the shown host),
  same as Show Transcript; the item says so.

## Release criteria

- An agent tab with a saved session on the shown host shows "Switch to
  chat" after "Show Transcript"; plain shells do not; the disabled
  reasons match Show Transcript's.
- A working tab asks "Switch to chat?" first and cancelling changes
  nothing; a tab waiting on an answer asks with the discarding wording;
  idle and finished tabs switch at once.
- The switch ends only the session's pane (no confirm bar) and opens the
  session in a live chat with its composer focused; a session still runs
  in one place only.
- Tests: the question's wording per attention state, the item's rules,
  the switch sequence (close the pane, then start the chat resumed) and
  the failure paths; `npm test`, `npm run check`, `npm run build`.
