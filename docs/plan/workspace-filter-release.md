# Workspace filter release

## Goal

A click on a workspace (or agent) row in the sidebar's group tree filters
the session list to that group, and nothing undoes it short of switching
the grouping mode. A click on the tree's empty space now releases the
filter: the session list shows every session again.

## Design

### The click

`GroupTree.svelte`'s `.tree-container` takes a click handler. A click that
lands outside any row, button, link, input or menu (pure
`isEmptyTreeClick(target)` in `tree.ts`) while a group is selected calls
`appState.selectGroup(null, 'push')`. That is the same state a fresh
grouping mode starts with, recorded as a navigation step, so Back brings
the filter back. Escape while focus is in the tree does the same, for
keyboard use.

The open session stays open: releasing the filter changes the list, not
the transcript. A row click still in flight (its sessions loading) loses
to the release, as it already loses to a later row click.

### The highlight

`nodeToHighlight` highlights the selected group or, with none, the group
holding the open session, both in the same full selected style. After a
release the same row would keep looking selected, as if the click did
nothing. The two cases now look different:

- **selected** (today's style): the row whose group filters the list, or
  the row just clicked.
- **current** (quieter: the hover background, no accent): the row that
  only holds the open session.

`GroupNodeItem` takes a `filterKey` next to `selectedKey` for this. The
reveal effect scrolls to either kind.

## Decisions

1. **Empty space is everything in the tree that is not a control**: the
   area below the last row, the gaps around rows and the tree's messages.
   Rows, the disclosure, ⋯ and star buttons, and an open context menu keep
   their own clicks.
2. **Release keeps the open session.** Closing the transcript would be a
   surprising side effect of clearing a filter.
3. **The release is a navigation step** (`push`), like selecting a group,
   so Back restores the filter.
4. **A quieter highlight for the open session's group** rather than none:
   the tree still shows where the open session lives, and the filter row
   is unmistakable. This also applies when a session is opened from
   outside the filtering group (search, links), which was the full style
   before.

## Risks

- A click meant for a row that lands in a gap releases the filter. The
  row's whole width is clickable and the gaps are small; Back undoes it.
- The quieter style must stay visible in both themes; it reuses the hover
  background, which every theme defines.

## Release criteria

- With a workspace selected, a click below the last row or in a gap
  releases the filter: the list shows all sessions, the open session stays
  open, and the row turns from selected to the quieter current style.
- Clicks on rows, their buttons and an open context menu behave as before.
- Escape in the tree releases the filter; Back restores it.
- `npm test` and `npm run check` pass.
