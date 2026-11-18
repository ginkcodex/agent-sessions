# CSB-09 — Ctrl+B / Cmd+B toggles the sidebar

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-01
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

Cmd+B on macOS and Ctrl+B on Windows and Linux hide or show the sidebar, as
the toggle buttons do. The key is left alone in text fields, the terminal and
the editor (tmux's prefix, readline's back-char, bold), while a modal is open
and while the terminal is maximized. The toggles' tooltips name the key.

Likely files: `frontend/src/lib/terminal.ts`, `frontend/src/App.svelte`,
`frontend/test/terminal.test.mjs`.

## Acceptance criteria and tests

- Outside text fields, the key collapses and expands the sidebar, with focus
  on the other toggle as after a click.
- In a text field or the terminal the key does nothing to the sidebar.
- Unit test for the key match on both platforms and non-Latin layouts.
- Run the frontend tests, `npm run check`, `npm run build`, and drive the
  browser mock.

## Evidence

Done: `isSidebarToggleShortcut` and `sidebarToggleShortcutLabel` in
`lib/terminal.ts`; `App.svelte`'s keydown handler calls
`setNavigationCollapsed`, so the left-terminal and focus handling of the
buttons apply.

- `npm test`: 1542 pass, 1 skipped, 0 fail (new test: "sidebar toggle").
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passes.
- Browser mock (Playwright, Chrome, 1400×800): Ctrl+B collapsed (focus on
  "Show sidebar"), Ctrl+B again expanded; in the search input and in the
  terminal Ctrl+B left the sidebar as it was. Tooltips read
  "Hide sidebar (Ctrl+B)" / "Show sidebar (Ctrl+B)". No page errors.

Not checked: macOS (Cmd+B) and the desktop app (CSB-02).
