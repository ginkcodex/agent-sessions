# Pane gaps

## Goal

Separate the main window's panes the way VS Code's current layout does: each
pane (sidebar, session list, session content, terminal panel, collapsed
navigation rail) is a rounded card on a darker gutter, and the splitters sit
in the gaps between the cards with a small grip, instead of a hairline
between flat, edge-to-edge panes.

## Current behavior

- `.pane` in `App.svelte` is a flat box. Panes touch each other and the
  window edges.
- `Splitter.svelte` is 6px wide with -3px margins on both sides, so it takes
  no space, and it draws a 1px `--border-color` line that turns into a 2px
  accent line on hover and while dragging.
- The same splitter divides panes inside the file viewer's history layout,
  the problems panel and the terminal panel's split panes.

## Design

- Two theme variables in `style.css`: `--bg-gutter`, darker than both pane
  backgrounds in each theme, and `--pane-gap` (6px).
- `.workspace` gets `--pane-gap` padding and the gutter background. `.shell`
  loses its own background so the gutter shows between the panes.
- `.pane` and the terminal panel get a 1px `--border-color` border, a
  `--radius-md` radius and `overflow: hidden`. The nav rail's right border
  becomes a right margin of one gap.
- `Splitter` gets a `gap` prop. A gap splitter is as wide (or tall) as the
  gap, with no negative margins, and shows a faint grip of three dots at
  rest. Hover, drag and keyboard focus show the full-length accent line.
- Only the three splitters in `App.svelte` use `gap`. Nested splitters keep
  the hairline, so the inside of a pane does not look like more cards.
- The left-dock grid's splitter columns change from `0` to `var(--pane-gap)`.

## Decisions

- Gap splitters take real space. The narrowest transcript now also needs the
  gaps: `LIMITS.paneGaps` (24px, four gaps) is added to the window-width
  guard in `PreferencesStore.ensureTranscriptVisible`.
- Equalizing the navigation and the transcript measures the panes' own
  widths instead of the workspace width, so the gaps belong to neither side.
- Equalizing the terminal panel's height keeps using half the workspace
  height. The gaps take 18px from the panes above, which is not worth a
  separate rule.
- No setting to turn the gaps off. The look is the default for every theme.

## Risks

- Popups positioned inside a pane are clipped by its rounded corners. The
  panes already had `overflow: hidden`. The terminal panel gains it, and its
  only absolutely positioned children (the terminal view and its find bar)
  stay inside it.
- `--pane-gap` and `LIMITS.paneGaps` must change together. A comment on
  each points to the other.
- Native title bars on Windows and macOS sit outside the web view and are
  not affected, but the gutter meets them at the top edge.

## Release criteria

- Light and dark mode show cards on a gutter with grips, with the terminal
  panel at the bottom and on the left and with the navigation collapsed.
- Dragging, double-click and keyboard resizing still work on all three gap
  splitters. Nested splitters are unchanged.
- The width guard keeps the transcript's minimum beside the gaps (unit test).
- The frontend check and the full frontend suite pass.
