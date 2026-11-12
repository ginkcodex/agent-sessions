# SMF-01 — Editor font for session content

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** —
- **Plan:** [session-mono-font.md](../../plan/session-mono-font.md)

## Scope

- `preferences.svelte.ts`: `sessionUsesEditorFont`, `setSessionUsesEditorFont`,
  key `agent-sessions:session-uses-editor-font`.
- `appearance.ts`: `sessionMonoFontVar`.
- `TranscriptView.svelte`: `style:--font-mono` on the pane; the font joins the
  size in the keep-my-place effects.
- `AppearanceSettings.svelte`: the checkbox and hint under Session content;
  `TypeSample.svelte`: optional `monoFont`.

## Acceptance criteria

- Off by default; nothing is stored until it is turned on.
- On with an editor font chosen: session code uses that font; the file viewer
  is unchanged. On with no editor font: no change.
- Kept after a restart.

## Evidence

- `test/appearance.test.mjs`: `sessionMonoFontVar` (off, no font, invalid
  name, valid); the preference persists, restores, resets, and garbage is off.
- `test/appearance-settings.test.mjs`: the checkbox is unchecked by default
  and checked when on; the sample gets `--font-mono` only when on.
- `test/transcript-font-size.test.mjs`: updated for the font in the anchor
  effect.
- `npm test`: 1395 tests, 1394 pass, 0 fail, 1 skipped. `npm run check`:
  0 errors, 0 warnings. `npm run build`: passes.
- Not checked: the running app (visual result in a session), and the
  WebKitGTK and WebView2 desktop apps.
