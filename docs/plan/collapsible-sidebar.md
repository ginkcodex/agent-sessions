# Collapsible sidebar

## Goal

Let the user collapse the whole left side of the window (the directory
sidebar, the session list and their splitters) so the session content gets the
full width, and bring it back with one click. The choice survives restarts.

## Design

- `PreferencesStore.navigationCollapsed`, saved under its own key
  `agent-sessions:navigation-collapsed` (`on` or absent), so the pane-widths
  JSON stays unchanged. `setNavigationCollapsed(on)` follows the other
  boolean preferences: storage failures keep the choice for this run.
- A collapse button (Lucide `panel-left-close`) at the end of the sidebar
  header's actions.
- While collapsed, a narrow rail (`nav.nav-rail`) at the left edge of the
  window holds the expand button (`panel-left-open`), so it is reachable in
  every transcript state (empty, loading, error, session, chat).
- The sidebar, the session list and both splitters are hidden with CSS
  (`display: none`), not unmounted: filters, tree expansion and list state stay
  as they were. Hidden panes leave the tab order and the accessibility tree.
- Saved widths are kept, so expanding restores the previous layout.
- Focus moves to the other toggle after a click, since the clicked one
  disappears.
- Terminal panel placed on the left: its column sits under the sidebar and the
  list, which are gone. While collapsed it is laid out at the bottom, under
  the session content, so an open terminal never disappears. Expanding returns
  it to the left.
- A maximized terminal hides the rail like the other panes.

## Decisions

- One click target per state, no keyboard shortcut: Ctrl+B and similar keys
  belong to shells and editors inside the app. A shortcut can be added later.
- The rail shows only the expand button. Actions that live in the sidebar
  (search, settings, terminal, refresh) keep their keyboard shortcuts (`/`,
  Cmd/Ctrl+`,`, Ctrl+`` ` ``) while collapsed.
- No animation: the panes have pixel widths driven by the splitters.

## Risks

- A scrolled list or tree may return at the top after being hidden; the
  selection is kept.
- At the sidebar's minimum width the header has one more button; the title
  already truncates there.

## Release criteria

- Collapse and expand work from the buttons, persist across restarts and keep
  pane widths.
- Hidden panes cannot be focused; the toggles have labels and
  `aria-expanded`.
- Terminal at the bottom, on the left and maximized all lay out correctly in
  both states.
- Frontend tests, `npm run check` and the build pass.

## Tasks

See [Collapsible sidebar tasks](../tasks/collapsible-sidebar/README.md).
