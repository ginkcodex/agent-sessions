# Session content uses the editor font

Planned and implemented 2026-11-12. Tasks: [session-mono-font](../tasks/session-mono-font/README.md).
Builds on [font-settings](font-settings.md).

Request: Settings has a font for the file viewer and editor. Add a checkbox
that applies that font to the monospace text in session content too, so users
can have one mono font for files and for code blocks in sessions.

## Goal

In Settings → Appearance → Session content, a checkbox **Use the editor font
for code in session content**, off by default. When on, code blocks, inline
code, tool input and output and the other monospace text in a session use the
font chosen under Appearance → Editor. It applies at once and is kept across
restarts.

## Design

- `PreferencesStore.sessionUsesEditorFont` (boolean), stored as `on` in
  `agent-sessions:session-uses-editor-font`; the key is removed when off.
  Anything other than `on` loads as off.
- `appearance.ts` `sessionMonoFontVar(use, editorFont)` returns the font
  family list (`monoFontFamily`) or `undefined`: off, no editor font chosen
  or an invalid name.
- `TranscriptView` sets `--font-mono` on `.transcript-pane` to that value,
  the same way it sets `--session-font-size`. Session parts already use
  `var(--font-mono)`, so none of them changes. The override is local to the
  pane, so the file viewer, the terminal and the rest of the app keep their
  fonts. Changing it keeps a scrolled-up reader on the same message, as a
  size change does.
- The Session content sample (`TypeSample`) takes the same value, so the
  code line previews the font.

## Decisions

- **Shared font, not a second picker.** The checkbox follows the editor font
  rather than adding a session font list.
- **Off by default,** so nothing changes for current users.
- **With no editor font chosen** (the app default) the checkbox changes
  nothing: both use JetBrains Mono.
- **Scope is the transcript pane,** so the session header and the live chat
  and composer follow too. Print and previews keep the app font.

## Risks

- Another font's metrics change line wrapping in tool output; the reader's
  place is kept by the existing anchor logic.

## Release criteria

`npm test`, `npm run check` and `npm run build` pass. Not checked: the
WebKitGTK and WebView2 desktop apps.
