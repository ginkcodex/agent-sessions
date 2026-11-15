# CCP-01 — Polish the chat command picker

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** —
- **Plan:** [chat-command-picker.md](../../plan/chat-command-picker.md)

## Scope

Increase popover/row padding and spacing; soften corners, emphasize selection and command names, style hints as badges, and improve description readability. Preserve full text, narrow-pane wrapping, ARIA and command behavior. Use only existing theme tokens and component-scoped CSS.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- Existing coverage in `frontend/test/chat-end-menu.test.mjs` and `frontend/test/chat-ui.test.mjs`
- Plan/task indexes

## Acceptance criteria

- Panel and rows have more breathing room; selection is distinct from hover.
- Hints read as badges; long names/hints/descriptions wrap rather than forcing horizontal overflow.
- Existing listbox/option structure and selection, dismissal, insertion and sending behavior remain unchanged.
- Frontend checks, tests and production build pass; any unperformed checks are recorded.

## Tests

Run `npm run check`, `npm test`, and `npm run build` in `frontend/`. This is a CSS-only change; existing command interaction tests cover the unchanged behavior.

## Evidence

- Doubled panel padding to 8px and row vertical padding to 12px, added row separation and an 8px composer/popover gap.
- Added rounded panel/rows, accent-tinted selection with an inset outline, semibold command names, theme-aware argument badges and roomier, higher-contrast descriptions.
- Retained the DOM, handlers, scroll limit and complete text; added wrapping and scroll padding for narrow panes and keyboard selection.
- `npm --prefix frontend run check`: passed, 642 files, zero errors or warnings.
- `npm --prefix frontend test`: passed, 1,492 passed, zero failed, one skipped (1,493 total), including command keyboard/click interactions.
- `npm --prefix frontend run build`: passed; Vite warned about chunks exceeding 500 kB.
- `git diff --check`: passed.
- Not checked: native app appearance, light/dark visual QA, Windows runtime, or measured narrow-pane overflow. No Go changes, so `make test` and Go lint were not run; `golangci-lint` was not available. No new tests needed for CSS-only changes.

