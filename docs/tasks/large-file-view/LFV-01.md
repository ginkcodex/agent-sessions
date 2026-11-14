# LFV-01 — Plan and task breakdown

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope and likely files

Write the plan, this task group and the feature rows in `docs/plan/README.md`,
`TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria and tests

- The plan names the cause, the design and what a large file's view loses.
- Every task is at most 4 h.
- All index links resolve.

## Evidence

- Cause found by reading `SourceView.svelte` and `FileViewer.svelte`: every
  line is rendered, highlighting is already off above 500,000 characters.
- Reported file measured: 2,561,942 bytes, 107,506 lines.
- Documentation only; no product tests for this task.
