# Chat command picker polish

## Goal

Give the command picker highlighted in the user's screenshot more breathing room and a polished appearance without changing command behavior.

## Design

Keep the existing listbox and options in `frontend/src/lib/components/chat/ChatComposer.svelte`. Increase panel and row padding, separate rows, soften the panel corners, and distinguish keyboard selection with an accent-tinted background and inset outline. Give command names stronger emphasis, argument hints a small badge, and descriptions a comfortable line height. Allow long names, hints and descriptions to wrap within narrow panes; retain complete descriptions and the existing scroll limit.

## Decisions

- Component-scoped CSS only; reuse theme colors, shadows and fonts for light/dark compatibility.
- No new dependencies, animations, controls, command parsing or backend changes.
- Preserve the option DOM structure so active-option scrolling, keyboard navigation, click selection and ARIA relationships remain unchanged.
- Scope reaches the composer styles, existing command interaction tests, and plan/task indexes. Callers, fixtures, exports and configuration need no changes.

## Reopening after dismissal (CCP-02)

Escape closes the current suggestions only until the next user edit. Reset dismissal alongside the active option in the composer's input handler, so clearing and retyping `/` (or Codex's `$`) reopens suggestions. Keep selection insertion dismissed until an actual edit; no backend, caller, fixture, export or configuration changes are needed. Add interaction regression coverage for Escape, clear, retype and repeated dismissal. This affects only transient picker state, not message or session data.

## Risks

Roomier rows show fewer commands at once; the existing scrollable list and active-option scrolling remain available. Long command names and hints must not force horizontal overflow. No session data is read, written or exposed by this styling change.

## Release criteria

- Roomier popover and rows, visible accent selection, readable descriptions and badge-style hints.
- Existing command completion, keyboard/click selection and composer tests pass.
- Svelte checks and production frontend build pass.
- Record exactly which automated checks ran; native Windows and visual light/dark checks are not claimed without running them.
