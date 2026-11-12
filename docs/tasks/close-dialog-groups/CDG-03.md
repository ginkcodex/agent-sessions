# CDG-03 — Sectioned cards and dialog polish

- **Status:** ✅ Done
- **Estimate:** 2.5 h
- **Depends on:** CDG-01
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

- `frontend/src/lib/components/common/go-to-file/FilePrompt.svelte`:
  render `request.groups` under the message as section cards — header
  row with icon, name and count; item rows with icon (the agent's own
  via `AgentIcon` where known), name, detail line and a "working"
  badge; a muted "and N more" row past the cap.
- A modest polish pass on the dialog chrome (spacing, typography, a
  quieter Cancel beside the danger action, scrollable body) that keeps
  every shared confirmation working as before.
- Tests: the rendering rules (source-level and, where the harness
  allows, SSR output); keep `file-prompt.test.mjs`'s focus and shortcut
  checks passing.

## Likely files

- `frontend/src/lib/components/common/go-to-file/FilePrompt.svelte`
- `frontend/src/lib/components/common/AgentIcon.svelte` (reuse)
- `frontend/test/file-prompt.test.mjs`

## Acceptance criteria and tests

- Groups render as the plan's cards: counts in the headers, one row per
  item, "working" badges only on working chats, "and N more" past six
  rows in a group.
- A prompt without groups renders exactly as before (the markup change
  is additive).
- The prompt still contains focus and owns find/save/close shortcuts
  while it decides (the existing checks pass).

## Evidence

- `frontend/test/file-prompt.test.mjs`: "the prompt shows item groups as
  counted cards, and none without items" — cards render only with
  groups, the header count sums up the overflow, agent rows show
  `AgentIcon`, shell and chat rows are told apart by icon, badges and
  the "and N more" row render. The focus and shortcut checks pass
  unchanged.
- The polish pass (scrollable body around message and cards, section
  cards on `--bg-secondary`, accent-tinted "working" badge, quieter
  Cancel beside the danger action) is additive: prompts without groups
  render the same dialog as before with only the chrome refinements.
- `npm test` 1380 tests, 0 failures (1 pre-existing skip);
  `npm run check` clean; `npm run build` succeeds.
