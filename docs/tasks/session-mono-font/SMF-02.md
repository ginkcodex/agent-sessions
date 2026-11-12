# SMF-02 — Tool parameters and output take the mono font

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** SMF-01
- **Plan:** [session-mono-font.md](../../plan/session-mono-font.md)

Found by the user after SMF-01: with the setting on, tool parameters and tool
output did not change font.

## Cause

Browsers give `<code>` (and `<pre>`, `<kbd>`, `<samp>`) their own
`font-family: monospace`, so a `<code>` inside a styled `<pre>` does not
inherit the pre's font. In these places only the `<pre>` had a rule, so the
`<code>` inside used the system's generic monospace (DejaVu Sans Mono on this
Linux machine). That was true before SMF-01 as well: these never used JetBrains
Mono, so `--font-mono` had nothing to change. SMF-01's scan test checked only
`font-family` rules that exist, not elements that have none.

## Scope

- `ToolPart.svelte`: `pre code { font: inherit; }` (tool parameters and
  tool output).
- `PatchPart.svelte`: `.diff-code code { font: inherit; }` (patch diffs).
- `PermissionCard.svelte`: `pre code { font: inherit; }` (the command) and
  `.paths code { font-family: var(--font-mono); }` (file paths).
- The same pattern `ManualBashPart` already used (`code { font: inherit; }`).

## Acceptance criteria

- With the setting on, tool parameters and tool output render in the editor
  font; with it off, in the app's mono font (JetBrains Mono).
- A test fails if any `<code>`, `<pre>`, `<kbd>` or `<samp>` in session
  content has no font rule of its own.

## Evidence

- `test/appearance-settings.test.mjs`: new test. For every `<code>`, `<pre>`,
  `<kbd>` and `<samp>` in the markup of `transcript/`, `transcript/parts/`
  and `chat/` components, a rule in that component must set `font` or
  `font-family` on it. With the fixes removed it fails, first on PatchPart's
  diff `<code>`; with only ToolPart's fix removed it fails on ToolPart's
  `<code class="hljs language-json">`.
- `test/chat-ui.test.mjs`: one pattern now allows the scoping class Svelte
  adds to the permission card's `<code>`.
- `npm test`: 1401 tests, 1400 pass, 0 fail, 1 skipped. `npm run check`:
  0 errors, 0 warnings. `npm run build`: passes. `git diff --check`: clean.
- Browser check: Vite dev server with the mock backend, headless Chrome over
  the DevTools protocol. Editor font set to Liberation Mono, transcript mode
  All, a session with a tool call, input and output sections opened. The
  rendered font is from `CSS.getPlatformFontsForNode`.

  | | Tool parameters `<code>` | Tool output `<code>` |
  |---|---|---|
  | Before the fix, setting on | `monospace` → DejaVu Sans Mono | `monospace` → DejaVu Sans Mono |
  | After, setting on | Liberation Mono | Liberation Mono |
  | After, setting off | JetBrains Mono | JetBrains Mono |

- Not checked in a browser: patch diffs and the permission card (the static
  test covers them, with the same fix). Not checked: the WebKitGTK and
  WebView2 desktop apps.
- Visible change at the default: tool parameters, tool output, patch diffs
  and permission card commands now use JetBrains Mono instead of the
  system's generic monospace.
