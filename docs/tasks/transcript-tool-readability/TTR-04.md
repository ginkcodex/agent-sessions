# TTR-04 — Keep long live commands clear of status badges

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** TTR-01
- **Plan:** [transcript-tool-readability.md](../../plan/transcript-tool-readability.md)

## Scope
Live ACP tool names can be entire commands, not short names such as Bash. Bound both the name and summary inside a shrinkable header column, reserving a separate column for the status. Retain the complete title on hover and the call ID.

## Likely files
`frontend/src/lib/components/transcript/parts/ToolPart.svelte`, `frontend/test/tool-card.test.mjs`.

## Acceptance criteria and tests
Long commands with or without input summaries cannot paint under running/completed/error badges. Header text truncates with an ellipsis and status remains on one line. Cover long names and layout CSS; run frontend check and tests. Record browser/native layout verification separately from DOM assertions.

## Evidence
- The tool name previously had `flex-shrink: 0` and no clipping. ACP full-command names could therefore paint outside the shrinking group under the status badge, even though the optional input summary already had ellipsis.
- Header now uses `minmax(0, 1fr) auto` grid columns. Name and summary both clip with ellipsis inside the bounded text group; status stays on one line. Hover shows the complete name plus call ID.
- Added layout-rule regression assertions and real-component DOM cases for long names with pending/completed/error status. Existing summary and expansion tests still pass.
- Full frontend suite: 1,398 passed, 1 skipped, 0 failures. `npm run check`: 0 errors/warnings. `npm run build`: passed (large-chunk warning).
- Headless Chrome geometry check against the real server-rendered component and compiled CSS: 72 cases, 0 overlaps. Widths 240/320/600/920 px, session font sizes 14.5/22 px, pending/completed/error status, long name/long summary/both. Checked group/name right edges against badge left edge and badge containment in header.
- Native macOS/WebKit live-chat visual replay not run; remains part of TTR-02.

