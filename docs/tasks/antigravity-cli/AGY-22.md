# AGY-22 — URLs agy prints in the terminal do not open

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-05](AGY-05.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-20). In the terminal panel,
agy's TUI shows URLs such as `https://example.com/mcp` with a dashed
underline, but clicking one does not open the browser.

## Cause

The app's terminal, not agy. agy writes its links as OSC 8 hyperlinks
(`ESC ]8;;<url> ESC \`; the sequence is in the agy binary). The terminal
handled only plain-text URLs, through `WebLinksAddon`, which calls
`api.openURL`. OSC 8 links go to xterm's `linkHandler` option, which was not
set, so xterm's default ran: `confirm()` and then `window.open()`, neither of
which does anything in the desktop WebView.

## Scope

- `frontend/src/lib/components/terminal/TerminalView.svelte`: one `openLink`
  callback (`preventDefault`, then `api.openURL`) shared by `WebLinksAddon`
  and xterm's `linkHandler: { activate: openLink }`.
- OSC 8 links stay http(s) only: `allowNonHttpProtocols` is not set, so a
  program in the terminal cannot open `file:` or other schemes this way.
- Any program that writes OSC 8 links (not only agy) benefits.

## Evidence

- `frontend/test/terminal.test.mjs`: new test 'terminal opens both plain URLs
  and OSC 8 hyperlinks through the app'. It failed before the fix and passes
  after it (62/62 in the file).
- `cd frontend && npm test`: 1632 tests, 1627 pass, 0 fail, 5 skipped.
- `npm run check`: 0 errors, 0 warnings.
- Not checked: a real click in the desktop app on macOS, Linux or Windows.
  The test reads the source; it does not drive xterm.
