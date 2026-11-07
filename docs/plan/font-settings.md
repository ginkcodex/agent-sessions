# Font settings: editor, project docs and session content

Planned 2026-11-07. Tasks: [font-settings](../tasks/font-settings/README.md).

Request: let the user change the font size of the code editor, the project
docs preview and the session content in Settings, and choose the editor's
font family as the terminal's font can be chosen today. In the transcript and
the docs preview, the different kinds of text (headings, code, tables,
labels) keep fixed ratios to the body text, and all of them scale with the
body text size.

## Goal

A new **Appearance** section in Settings with:

- **Editor:** font family (installed monospace fonts, as for the terminal)
  and font size.
- **Project docs preview:** body text size.
- **Session content:** body text size.

In the docs preview and session content every text size is a fixed ratio of
the body size, from one type scale shared by both, so changing the body size
scales headings, code, tables and labels together. Every change applies at
once, without reopening a file or a session, and is kept across restarts.
Unchanged settings keep today's look, apart from the small merge of sizes
described under Type scale.

## Today

- **Editor.** Go to File shows code in two views that are kept identical so
  switching between them does not move the text: the read-only viewer
  (`SourceView.svelte`, also used for Git diffs and history) and the
  CodeMirror editor (`fileEditor.ts` theme). Both use `var(--font-mono)` at
  `0.78rem` (14.04 px on the 18 px app base) with line height 1.5.
  Presentation mode overrides the size for the session (12–48 px, starts at
  20 px, not saved).
- **Project docs preview.** The Markdown preview in Go to File renders
  `MarkdownDoc.svelte` with `--markdown-font-size: 14.5px` (presentation mode
  overrides it). `MarkdownDoc` already sizes most text in `em` of that body
  size: headings 1.428 / 1.255 / 1.083em, inline code 0.825em, tables and
  code blocks 0.923em. The code block language tag and Copy button are fixed
  at 10.88 px. Import and handoff dialogs also use `MarkdownDoc`, at its 13 px
  default.
- **Session content.** Everything is fixed `px`. Message text
  (`TextPart`) is 14.5 px, with the same heading and inline-code ratios as
  the docs preview but tables and code blocks at 13.3 px (0.917, not the
  docs' 0.923). The rest of the content (message headers, tool, patch,
  reasoning, compaction and notice parts) uses ten different sizes from
  10.4 to 13.7 px, left over from `rem` values of the old 16 px base. About
  60 declarations across `MessageBubble`, `TextPart`, `ToolPart`,
  `PatchPart`, `ReasoningPart`, `CompactionPart`, `NoticePart`, `ImagePart`
  and `SubagentLink`. Changing a base size alone would change nothing.
- **Terminal font.** `TerminalFontSetting.svelte` lists installed monospace
  families from `App.TerminalFonts()` (local desktop only, names only),
  stores the name in `localStorage` (`agent-sessions:terminal-font`) through
  `preferences`, and builds a safe CSS family list with
  `terminalFontFamily()` (quoted name, then system monospace fallbacks). A
  missing font is shown as "(unavailable)".

## Design

### Type scale (ratios first)

Before any setting is added, each text size in the docs preview and session
content becomes **body size × one of eight ratios**. The fourteen sizes in use
today (the seven below the body are left over from old `rem` values, four of
them less than 1 px apart) are merged into this short scale. No visible size
moves by more than 0.55 px at the 14.5 px default body (the largest move is
the tool name, +0.54 px), except inline code, which is made slightly larger
on purpose. The tokens live once in `style.css`
as unitless custom properties and are used by `TextPart`, `MarkdownDoc` and
the session parts:

| Token | Ratio | At 14.5 px | Takes over today's sizes |
|---|---|---|---|
| `--type-h1` | 1.43 | 20.7 px | 20.7 px heading 1 |
| `--type-h2` | 1.25 | 18.1 px | 18.2 px heading 2 |
| `--type-h3` | 1.08 | 15.7 px | 15.7 px heading 3 |
| `--type-body` | 1 | 14.5 px | 14.5 px message and doc text, heading 4 |
| `--type-code` | 0.92 | 13.3 px | 13.3 px code blocks and tables (docs: 0.923em); 13.7 px reasoning text (−0.4); 12.8 px tool name (+0.54) |
| `--type-small` | 0.83 | 12 px | 12 px tool input and output, file attachment pill, patch file list, reasoning summary, subagent link, image messages; 12.48 px patch title and compaction details (−0.5) |
| `--type-label` | 0.79 | 11.5 px | 11.52 px message header, compaction and notice pills, diff code, section summaries, output label, banners, errors; 11.2 px call id, counts, Show diff and Load blob buttons (+0.3) |
| `--type-tag` | 0.75 | 10.9 px | 10.88 px model and status tags, code language tag, Copy buttons, summary header; 10.4 px sidechain and translation tags, chevrons (+0.5) |
| `--type-inline-code` | 0.85 | 0.85em | 0.825em inline code, relative to the text around it (+0.36 px in body text) |

Rules:

- **Only these ratios.** Every text size in the docs preview and session
  content is one of them; no component gets its own size or an override.
  A new kind of text picks the nearest token.
- **Block text uses the container's body size, not `em`:**
  `font-size: calc(var(--session-font-size, 14.5px) * var(--type-code))` in
  session content and `calc(var(--markdown-font-size, 13px) * var(--type-code))`
  in `MarkdownDoc`. A table inside a list inside a quote then gets the same
  size as anywhere else, instead of compounding.
- **Inline code stays `em`** (`calc(var(--type-inline-code) * 1em)`), so it
  follows a heading or a table it sits in, as today.
- **Line heights stay unitless** ratios; fixed-px line heights and heights on
  text containers become ratios or `min-height`.
- **Not scaled:** icons and icon glyphs (the 14 px reset glyph, role icons),
  images, padding and the 900 px column width. View controls in the scroll
  area (Load earlier, end-of-transcript marker, empty notice) and the
  selection Copy/Translate menu follow the app size, as the session header
  does.
- **The docs preview uses the same scale:** headings, `--type-code` for
  tables and code blocks, `--type-tag` for the code block language tag and
  Copy button, `--type-inline-code` for inline code.

### Preferences

Four new fields in `PreferencesStore`, each in its own `localStorage` key as
the existing preferences are, validated on load and on set:

| Field | Key | Default | Range |
|---|---|---|---|
| `editorFont` | `agent-sessions:editor-font` | `''` (app default, JetBrains Mono) | `isTerminalFont` rules |
| `editorFontSize` | `agent-sessions:editor-font-size` | `null` (14.04 px, today's size) | 10–28 px |
| `docsFontSize` | `agent-sessions:docs-font-size` | `null` (14.5 px body) | 11–28 px |
| `sessionFontSize` | `agent-sessions:session-font-size` | `null` (14.5 px body) | 11–24 px |

`null` means "default": nothing is overridden and today's CSS applies, so the
default look cannot drift. A stored value outside the range, non-numeric or
unreadable falls back to the default. Values are kept to 0.5 px.

Like the terminal font, these are per computer (this desktop's
`localStorage`), not per SSH/WSL host, and fonts are always listed from the
local desktop.

### Applying them: CSS custom properties

A small pure module, `appearance.ts`, maps preferences to custom properties;
`App.svelte` applies them in one `$effect`:

- On `document.documentElement`: `--editor-font-family` (built with the
  existing `terminalFontFamily(name, 'var(--font-mono)')`, renamed or wrapped
  as a generic `monoFontFamily`), `--editor-font-size` and
  `--docs-font-size`. Unset when the preference is the default.
- The Go to File preview sets `--markdown-font-size` to
  `var(--docs-font-size, 14.5px)` (presentation size while presenting). The
  import and handoff dialogs keep their 13 px.
- `--session-font-size` is set by `TranscriptView` on `.messages-scroll-area`
  only, so other places that render transcript parts (print, previews) keep
  the default size.

Consumers:

- `SourceView` and the CodeMirror theme use
  `var(--editor-font-family, var(--font-mono))` and
  `var(--editor-font-size, 0.78rem)`, so the viewer and the editor stay
  identical. Presentation mode's inline size still wins while it is on. The
  editor is code only, so it has one size and no type scale.
- Docs preview and session content: body × ratio, as in the type scale.

**Text only, not zoom.** The session setting scales text, not padding, icons,
images or the column width. CSS `zoom` would scale everything in one line,
but it changes the coordinates that scroll-to-bottom, find highlights, the
selection menu and file links rely on, and WebKitGTK and WebView2 report them
differently.

### Settings UI

A new `appearance` section ("Appearance") between General and Coding agents:

- **Editor font:** the same picker as the terminal (installed monospace
  fonts, "App default (JetBrains Mono)", "(unavailable)" for a removed font,
  Refresh). The picker is extracted from `TerminalFontSetting` into a shared
  component; the installed-font list is fetched once and shared by both
  pickers.
- **Editor font size**, **Project docs text size**, **Session text size:** a
  stepper (− value +) with a Reset link, like the presentation-mode buttons.
  The value is the body size in px, with "(default)" when unchanged. Steps
  are 1 px; from a fractional default the first step goes to the next whole
  number (14.5 → 15 or 14).
- Each group has a short live sample; the docs and session samples show a
  heading, body text, inline code and a small code block, so the ratios are
  visible while the size changes.

The Terminal section keeps its own font picker.

### Re-measuring after a change

Several views cache pixel positions and must update when a size changes
while they are open:

- CodeMirror: `view.requestMeasure()` after the properties change (or the
  theme moved into a `Compartment` and reconfigured), so line heights, the
  gutter and the cursor are measured again.
- `FileViewer`: the change-navigation line list, the overview ruler, find
  highlights and the saved scroll position already depend on
  `presentationFontSize`; they also depend on the editor and docs sizes.
- `TranscriptView`: if the view follows the bottom, it stays at the bottom;
  otherwise the first visible message stays in place (WebKit has no scroll
  anchoring). Session find highlights are painted again.

## Decisions

- **Ratios first.** The type scale lands as its own step with no setting
  attached. The settings then only change the body size.
- **Simplicity over exact sizes.** Eight ratios replace fourteen sizes; the
  merge moves sizes by up to 0.55 px. Reasoning text drops from 13.7 to
  13.3 px, the code size; it stays distinct by color and indent. Sizes follow
  the ratios only: no per-component overrides or extra steps.
- **Inline code grows to 0.85em** (from 0.825em) in both surfaces.
- **One type scale for both surfaces,** since headings and inline code
  already match and tables and code blocks differ by 0.006.
- **Default is "no override",** not a stored number, so defaults stay
  identical and future default changes apply to users who never touched the
  setting.
- **Editor font applies to the viewer and diffs too.** They share the
  editor's metrics today; splitting them would make the text jump between
  viewing and editing.
- **Not covered:** the Problems, Search and Changes panes, hover cards, line
  blame, the session header and list, and the terminal's size. Each can be a
  later task if wanted.
- **Presentation mode stays separate:** it still starts at 20 px and is not
  saved; leaving it returns to the saved setting. In the docs preview it sets
  the body size, so the ratios hold there too.
- **No new backend API:** the existing `TerminalFonts()` list serves both
  pickers.
- **No keyboard zoom shortcuts** (Ctrl+= / Ctrl+-) in this feature.

## Risks

- **Missed fixed sizes.** A size left in `px`/`rem` will not scale and breaks
  the ratios. A test scans the docs-preview and session-content styles and
  fails on any `font-size` that is not body × a `--type-*` token (or `em` for
  inline code), apart from a listed set of non-text exceptions with reasons
  (icons, view controls).
- **The default look drifts** more than planned while converting. A test
  checks that each converted rule uses the token the plan's table assigns to
  its old px value, and that its default size is within 0.55 px of today's
  (inline code excepted).
- **Fixed heights clip larger text** (badges, tool headers, one-line
  summaries with `px` heights or line heights). The audit covers heights and
  line heights, and the manual check uses the largest size.
- **Shared code-block rules.** `.code-block-container` styles in `style.css`
  are used by the transcript, `MarkdownDoc` and hover cards. Each surface
  keeps its own scoped rule; hover cards are not changed.
- **CodeMirror stale metrics** if a re-measure is missed: wrong cursor or
  click positions. Covered by a test on the editor module and the manual check.
- **Scroll jumps** in a long transcript when the size changes.
- **A removed editor font** falls back to the system monospace list, as for
  the terminal.

## Release criteria

- Every docs-preview and session-content text size is body × one of the
  eight `--type-*` ratios, or inline code's 0.85em (scan test passes); the
  ratios are the same in both surfaces.
- With no setting changed, the editor and viewer render as before, and docs
  preview and session text is within 0.55 px of today's sizes (inline code
  excepted).
- The three sizes and the editor font can be changed in Settings → Appearance,
  apply at once to open views, and survive a restart; at any body size the
  ratio between, e.g., a heading and body text is the same.
- Viewer and editor show the same font and size; switching between them does
  not move the text; cursor and click positions are right after a change.
- At the largest session size no session-content text is clipped, and a
  following transcript stays at the bottom.
- `npm test`, `npm run check` (0 errors) and `npm run build` pass.
- Manual check in the desktop app on Linux (WebKitGTK) and Windows
  (WebView2), with the result recorded in the regression-gate task.
