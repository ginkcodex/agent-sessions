# WFR-02 — Release the group filter

- **Status:** ⬜ Not started
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
- `frontend/test/tree.test.mjs`, `frontend/test/workspace-stars.test.mjs`

## Acceptance criteria and tests

- `isEmptyTreeClick`: true for the container and gaps, false for rows,
  row buttons, links, inputs and menus.
- A row is `selected` only when it is the filter, `current` when it only
  holds the open session.
- `npm test`, `npm run check`.
