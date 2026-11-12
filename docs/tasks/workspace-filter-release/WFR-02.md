# WFR-02 — Release the group filter

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** WFR-01
- **Plan:** [workspace-filter-release.md](../../plan/workspace-filter-release.md)

## Scope

- `tree.ts`: pure `isEmptyTreeClick(target)`.
- `GroupTree.svelte`: click and Escape on the tree release the selected
  group with `appState.selectGroup(null, 'push')`; pass `filterKey`; the
  reveal effect scrolls to selected or current rows.
- `GroupNodeItem.svelte`: `filterKey` prop; `selected` vs `current`
  classes and the quieter style.

## Likely files

- `frontend/src/lib/tree.ts`
- `frontend/src/lib/components/sidebar/GroupTree.svelte`
- `frontend/src/lib/components/sidebar/GroupNodeItem.svelte`
- `frontend/test/workspace-filter-release.test.mjs`

## Acceptance criteria and tests

- `isEmptyTreeClick`: true for the container and gaps, false for rows,
  row buttons, links, inputs and menus.
- A row is `selected` only when it is the filter, `current` when it only
  holds the open session.
- `npm test`, `npm run check`.

## Evidence

- `frontend/test/workspace-filter-release.test.mjs` (new, 5 tests, all
  pass):
  - `isEmptyTreeClick` is true for the tree, its list, an item and a
    message; false for a row, its label and star button, an open menu and
    its item, a link and an input; false for null and a text node.
  - `GroupTree` and `GroupNodeItem` compiled and mounted in jsdom with a
    stubbed `appState`:
    - A click on the tree calls `selectGroup(null, 'push')` once, and opens
      no other session. The filtering row goes from `selected` to
      `current`. A second click does nothing.
    - The star button and the row itself do not release the filter; the
      row still selects its group. A click on the item around a row, outside
      it, releases the filter.
    - Escape on a row or the tree releases the filter, and does nothing
      once there is no filter.
    - With another group filtering, the open session's group is `current`.
      With no filter and no open session, rows are plain and a click does
      nothing.
- `npm test`: 1409 tests, 1408 pass, 0 fail, 1 skipped (already skipped).
- `npm run check`: 0 errors, 0 warnings.
- Not checked: the desktop app (WFR-03), real layout and focus, the look of
  the quieter highlight, Back after a release.
