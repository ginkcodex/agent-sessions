# Chat option dropdowns

## Goal

The chat composer's session-option pickers (mode, model, effort…) use
native `<select>` controls, so their popups are drawn by the platform:
WebKitGTK paints them with the GTK theme, ignoring the app's colors and
fonts, and they look different on Linux, macOS and Windows. Make every
picker the shared `Dropdown`, like the font picker in Settings →
Appearance.

## Design

### Options from the picker

`chatOptions.ts` gains a pure `pickerOptions(picker)` beside `pickers`,
mapping a picker's choices to the shared `Dropdown`'s option shape:

- `label` is the choice's name, bare — `Bypass Permissions`, not
  `Bypass Permissions (no permission requests)` (kept plain on review).
  The unguarded warning under the pickers says what such a mode means.
- `title` is the choice's description, shown on hover in the menu (the
  `<option title>` the browsers never showed).
- `group` keeps the agent's group headings (Anthropic, OpenAI…).

### Opening upward at the composer

`Dropdown` gains `placement?: 'bottom' | 'top'`, default `'bottom'` so
its eight other users are untouched. The composer's toolbar sits on the
window's bottom edge, so its menus open above the trigger
(`.menu.up`: `bottom: calc(100% + 4px)`).

### The pickers

`ChatOptions` renders one `Dropdown` per select picker instead of a
`<select>`: the trigger shows only the chosen value's name (the
placeholder `Not set` while the agent reports none), its tooltip is
`name: description`, and the menu is `align="right"`, `placement="top"`.
It is disabled while the agent starts or another change is in flight,
exactly like the select was.

The picker stays controlled by the value the agent reports, so a
refused change re-shows it without the old fix-ups that put the value
back on the `<select>` element; only the boolean checkbox keeps its
manual reset. The unguarded warning, the toggles and the compact
toolbar look (a picker shows its value, its name is the tooltip) are
unchanged.

## Decisions

1. **The shared `Dropdown`**, not restyled native selects: one look on
   every platform and popups that follow the app theme — the same
   choice as the font picker in Settings.
2. **`placement` on the shared component** rather than a composer-only
   popup: the other dropdowns keep opening down, and any control near a
   window edge can ask for the other side later.
3. **The toolbar presentation stays compact**: value only, name as
   tooltip, `Not set` kept, and option names bare — the review dropped
   the ` (no permission requests)` mark the select carried, since the
   unguarded warning already says it.
4. **Booleans stay checkboxes** — they are not selects.

## Risks

- `Dropdown` is shared by eight controls; the change is an additive
  prop whose default keeps today's behavior.
- Long model names truncate at `14rem`, as the select did.
- Found along the way: `WorkspaceStart`'s agent select is the last
  native `<select>` (in the session list, not the chat box). It is
  tracked as CHD-04, not part of this change.

## Release criteria

- Each select picker looks like the font picker in Settings: themed
  trigger with chevron, themed menu with check marks.
- The pickers' menus open upward and stay inside the window.
- The menu keeps the agent's group headings, each choice's bare name
  and its description on hover; the trigger's tooltip is
  `name: description`; `Not set` shows while the agent reports no
  value.
- Choosing a value sends it and the picker follows the agent's answer;
  a refused change shows the agent's value again; toggles and the busy
  state behave as before.
- The other dropdowns (font picker, branch and diff-basis pickers,
  settings) open downward as before.
- `npm test`, `npm run check` and `npm run build` pass in `frontend/`;
  QA in the desktop app records what was run.
