# CSB-02 — Check the collapsed layout in the desktop app

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** CSB-01
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

Run the desktop app and record what was seen. No new features.

## Acceptance criteria and tests

- Collapse and expand with a session selected, with none, and with a chat.
- Restart the app collapsed; it opens collapsed with the old widths on expand.
- Terminal at the bottom, on the left, and maximized, in both states; the
  rail's terminal button and its attention dot; the rail's connection dot
  while connecting, connected, reconnecting and on Local.
- Tab order skips the hidden panes; the toggles work from the keyboard.
- The rail's session switcher: lists, opens a session, closes on Escape,
  outside click and selection; closes on a host switch.
- Light and dark themes; the sidebar at its minimum width.
