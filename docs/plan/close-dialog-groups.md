# Close dialog groups

## Goal

The closing dialogs ("Close window?", and the host-switch prompts that
end open chats) list what they end as flat text lines, where terminals
and chats read alike. Group the list into **Terminals** and **Chats**
sections and make the dialog itself more beautiful.

- Each group is a subtle panel: a header row with an icon, the group's
  name and its count, then one row per item — icon, name, and a small
  detail line (host for terminals, directory for chats). Working chats
  carry a small "working" badge.
- Under a heading the labels simplify: "Codex in api (working)" becomes
  a Chats row named `Codex`, detail `in api`, badge `working`.
- The same treatment covers every closing dialog: "Close window?", the
  host-switch prompts ("End open chats?" and the combined terminal tabs
  question) and the unsaved-changes question ("Close window?",
  "Switch host?", "Close files?") share the list-building code and share
  the look.

## Design

### Structured items

`closeConfirm.ts` stops returning flat strings and returns a warning
object instead:

```ts
export interface CloseWarning { message: string; groups: FilePromptGroup[] }
```

- `closeWindowWarning(tabs, chats)` and `endingChatsWarning(chats,
  action)` return `CloseWarning | null` (null unchanged: nothing open).
- `message` keeps today's prose ("3 terminals are still running,
  including 2 coding agents. Closing the window ends them:") without the
  line list.
- `groups` holds one `FilePromptGroup` per kind that has items:
  `Terminals` (running tabs) and `Chats` (open chats). The shapes live in
  `filePrompt.svelte.ts` beside `FilePromptChoice`:

```ts
export interface FilePromptItem {
  name: string;
  detail?: string;            // "on Ubuntu (WSL)", "in api"
  icon: 'terminal' | 'agent' | 'chat' | 'file';
  agent?: string;             // an agent id renders its icon instead
  badge?: string;             // "working"
}
export interface FilePromptGroup {
  title: string;              // "Terminals", "Chats", "Unsaved files"
  icon: 'terminal' | 'chat' | 'file';  // the heading's icon
  items: FilePromptItem[];
  more?: number;              // items past the cap, as "and N more"
}
```

- Terminal rows: name is `tab.label`, detail is `on <host>` when the tab
  runs elsewhere, `icon: 'agent' | 'shell' → 'agent' | 'terminal'` from
  `tab.kind`.
- Chat rows: name is the agent's label, detail `in <folder>` from the
  directory, `icon: 'chat'` with `agent` set for the agent's own icon,
  `badge: 'working'` while `phase === 'working'`.
- The `NAMED = 6` cap becomes per group; the overflow is `more` and the
  card ends with a muted "and N more" row. The prose counts are
  unchanged and stay exact.

### The prompt

`filePrompt.ask(title, message, choices, groups?)` takes an optional
fourth argument, defaulting to `[]` — every other caller is untouched.
`FilePrompt.svelte` renders the groups under the message as the section
cards, and the dialog chrome gets a polish pass (spacing, typography,
a quieter Cancel beside the danger action, scrollable body) while every
shared confirmation keeps working exactly as before.

The dialog is 480px wide, and wider when its buttons need it: a label
never wraps (`white-space: nowrap`, the panel's `min-width: min-content`),
so the host-switch question's "End and close terminal tabs" stays on one
line. Below a 640px window (only the browser build: the desktop window is
at least 960px) the buttons wrap onto rows instead of overflowing.

`confirmClosingWindow` passes `warning.groups` through; the host-switch
prompts in `connection.svelte.ts` compose their messages from
`CloseWarning` — the combined terminal-tabs question keeps its tabs
paragraph in the message and puts the chats warning's cards under it.

### The unsaved-files question

`fileActions.resolve` asks before closing or switching away with unsaved
edits ("Close window?", "Switch host?", "Close files?") and lists the
dirty files as plain `dir/path` lines. A pure `unsavedDrafts(dirty)` in
`closeConfirm.ts` beside the other builders returns the same
`CloseWarning` shape: the message ("Unsaved changes in:") and one
**Unsaved files** card whose rows name the file's path in its workspace
(`name` = the tab's path, `detail` = `in <workspace dir>`, `icon:
'file'`). Every file is named — no cap and no "and N more" — since the
old dialog listed them all and the decision (discard or save) is taken
per file. `resolve` passes the card to `ask` like the other callers.

## Decisions

1. **Sectioned cards with icons**, chosen by the user over plain
   headings or status dots: a panel per group, header row with icon,
   name and count, item rows with icon, name and detail line, and a
   "working" badge on working chats.
2. **All closing dialogs match**, chosen by the user: the host-switch
   prompts use the same list code and get the same cards.
3. **Labels simplify under their heading** ("Codex", "in api", badge
   "working" instead of "Chat with Codex in api (working)"); the prose
   sentence keeps its counts, so nothing is lost.
4. **The groups ride along on `ask`** as an optional argument rather
   than a new dialog: `FilePrompt` stays the one prompt, and the other
   sixteen callers keep their plain message.
5. **The cap is per group** (6 named rows each), so a busy window names
   up to 12 items before it sums up — each card reads complete on its
   own.
6. **Rows show agent icons** where an agent is known (chats always,
   agent terminals when the tab carries its session); a shell row shows
   the terminal icon, a dirty file row the file icon.
7. **The unsaved-files card names every file**, uncapped: the old dialog
   listed them all and discarding edits is a per-file decision. The
   six-item cap stays where it was, on the Terminals and Chats cards.

## Risks

- `FilePrompt` is shared by every destructive confirmation; the markup
  change is additive (groups render only when present) but the chrome
  pass touches all of them — kept modest so nothing else regresses.

## Release criteria

- "Close window?" shows Terminals and Chats cards with counts, one row
  per item with icon, name and detail line, "working" badges, and "and
  N more" past six per group; the prose sentence keeps today's counts.
- Shell and coding-agent terminals are told apart by their icons, and
  each chat row shows its agent's icon.
- The host-switch prompts ("End open chats?", and the combined terminal
  tabs question) show the same Chats card.
- The unsaved-changes question ("Close window?", "Switch host?",
  "Close files?") lists its files in an Unsaved files card, every file
  named with its workspace directory.
- Every other prompt using `FilePrompt` renders as before.
- Tests cover the structured warnings (message, rows, badges, cap) and
  the store's groups pass-through; `npm test`, `npm run check`,
  `npm run build` pass; QA in the desktop app records what was run.
