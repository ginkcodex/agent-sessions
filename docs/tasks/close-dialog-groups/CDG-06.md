# CDG-06 — Button labels on one line

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** CDG-03
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

Found in use: when you switch away from a host that has terminal tabs, the
"Terminal tabs from previous host" question wrapped "Keep terminal tabs"
and "End and close terminal tabs" onto two lines each inside the fixed
480px dialog (macOS WebKit).

- `FilePrompt.svelte`: buttons get `flex: none; white-space: nowrap`. The
  panel keeps `width: min(480px, 100%)` and adds `min-width: min-content`,
  so it widens only when the button row needs more than 480px. Below a
  640px window, the buttons wrap onto rows instead of overflowing (only
  the browser build; the desktop window is at least 960px).

## Likely files

- `frontend/src/lib/components/common/go-to-file/FilePrompt.svelte`
- `frontend/test/file-prompt.test.mjs`

## Acceptance criteria and tests

- Every prompt's button labels sit on one line.
- Prompts whose buttons fit stay 480px wide.

## Evidence

- `frontend/test/file-prompt.test.mjs`: "the prompt widens for its buttons
  so each label stays on one line" checks the CSS rules.
- Rendered the prompt's CSS in headless Chrome (1200px window): the
  terminal-tabs question stays 480px with all buttons 32px tall (one line),
  the chats variant ("End chats, keep terminal tabs" / "End chats and
  terminal tabs") widens to 523px, and a one-button prompt stays 480px.
- `npm test`: 1404 tests, 1403 pass, 0 failures, 1 pre-existing skip.
  `npm run check`: 0 errors, 0 warnings.
- Not checked: the desktop app (macOS WebKit, where the bug showed, and
  WebView2), and the narrow-window wrap.
