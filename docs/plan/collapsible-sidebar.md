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
  every transcript state (empty, loading, error, session, chat). Where the
  app has a terminal panel, the rail's bottom holds the same terminal button
  as the sidebar footer (one Svelte snippet for both). Below it, the host
  selector's status dot and tooltip (`HostSelector dotOnly`) tell Local from
  a remote host and show the connection phase.
- The sidebar, the session list and both splitters are hidden with CSS
  (`display: none`), not unmounted: filters, tree expansion and list state stay
  as they were. Hidden panes leave the tab order and the accessibility tree.
- Saved widths are kept, so expanding restores the previous layout.
- Focus moves to the other toggle after a click, since the clicked one
  disappears.
- Terminal panel placed on the left: it belongs to the left side, so
  collapsing hides it with the sidebar and the list, and expanding shows it
  again (CSB-06). While collapsed, the rail's terminal button (or Ctrl+`)
  opens it at the bottom, under the session content; expanding then moves it
  to the left. If the user opens it from the rail and closes it again,
  expanding leaves it closed.
- A maximized terminal hides the rail like the other panes.

### Session switcher in the rail

While collapsed, a Sessions button under the expand button opens a floating
panel beside the rail, so the user can change session without expanding.

- It lists the workspaces (the directory groups, whatever the sidebar's
  Dir/Agent/Flat mode), with the sidebar's filters: search, Live, Archived,
  agent, path, starred-only and the age filter. Workspaces with no matching
  session are left out. Subagent sessions are left out, as the list tucks
  them under their parents.
- Each workspace is a toggle row; its sessions load when it opens, in the
  list's sort order. The workspace holding the open session starts open and
  the open session is marked.
- Picking a session opens it as a click in the session list does (the
  terminal follows its directory, back/forward records it) and closes the
  panel. The sidebar's group selection is left alone.
- The panel is a non-modal dialog: Escape or a click outside closes it and
  focus returns to the button. It loads fresh each time it opens; a reply
  that arrives after it closed or after a host switch is dropped.
- The button is disabled while the window shows stale data, like the
  sidebar's other actions.

## Decisions

- One click target per state, no keyboard shortcut: Ctrl+B and similar keys
  belong to shells and editors inside the app. A shortcut can be added later.
- The rail shows the expand button, the terminal button and the connection
  dot (an indicator, not the host menu, which would not fit the rail). Other sidebar
  actions (search, settings, refresh) keep their keyboard shortcuts (`/`,
  Cmd/Ctrl+`,`) while collapsed.
- No animation: the panes have pixel widths driven by the splitters.
- The switcher is a dialog with buttons, not an ARIA menu: Tab moves through
  rows. Arrow-key movement can come later.

## Risks

- A scrolled list or tree may return at the top after being hidden; the
  selection is kept.
- The "hidden by collapse" mark lives only for this run: after a restart in
  the collapsed state, expanding does not reopen the left terminal.
- At the sidebar's minimum width the header has one more button; the title
  already truncates there.

## Release criteria

- Collapse and expand work from the buttons, persist across restarts and keep
  pane widths.
- Hidden panes cannot be focused; the toggles have labels and
  `aria-expanded`.
- Terminal at the bottom, on the left and maximized all lay out correctly in
  both states.
- The switcher lists filtered workspaces and sessions, opens the picked
  session and closes on Escape, outside click and selection.
- Frontend tests, `npm run check` and the build pass.

## Tasks

See [Collapsible sidebar tasks](../tasks/collapsible-sidebar/README.md).
