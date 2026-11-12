# WFR-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [workspace-filter-release.md](../../plan/workspace-filter-release.md)

## Scope

- `docs/plan/workspace-filter-release.md`, this task group, and the rows
  in `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan covers what counts as empty space, what the release keeps, the
  navigation step and the highlight.

## Evidence

- Written before any code. `GroupTree.svelte`, `GroupNodeItem.svelte`,
  `appState.selectGroup` and `nodeToHighlight` were read first: nothing
  clears `selectedGroupKey` today except a grouping mode change or a
  vanished group, and the open session's group shares the selected style.
