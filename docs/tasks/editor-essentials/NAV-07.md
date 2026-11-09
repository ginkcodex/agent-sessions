# NAV-07 — Hide unavailable navigation menu items

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** NAV-04, NAV-05
- **Plan:** [editor-essentials.md](../../plan/editor-essentials.md)

## Scope

Hide the disabled definition/reference entries and blocker explanation from the shared source/editor word context menu. Keep actionable settings and Copy; avoid a leading divider in a copy-only menu. Do not globally hide disabled context-menu items elsewhere.

## Likely files

- `frontend/src/lib/components/common/go-to-file/FileViewer.svelte`
- `frontend/test/source-navigation.test.mjs`

## Acceptance criteria

- Unavailable navigation yields no gray navigation actions or blocker paragraph.
- Available definition/reference actions still dispatch with existing guards.
- Settings remains where supported; Copy works even when it is the only entry.
- Keyboard availability feedback, permission checks and other context menus are unchanged.

## Tests

- Execute the actual shared menu handler for blocked/ready states and settings-ineligible contexts; verify Copy, separators and ownership guards.
- Full frontend tests, type checks, production build and whitespace validation. Record native/manual gaps.

## Evidence

The shared word menu now includes definition/references only when navigation is available and no longer appends the disabled blocker paragraph. Settings and Copy remain actionable; Copy has no leading divider when alone. No changes to global context-menu rendering, navigation guards or keyboard feedback. NAV-04's original presentation note now points to this follow-up.

Verified on 2026-11-09:
- `node --loader ./test/resolver.mjs --test test/source-navigation.test.mjs test/code-navigation.test.mjs`: 45/45 passed. Tests execute the actual source/editor menu handler for approval/missing-server/disconnected/starting blockers, available definition/reference dispatch, settings-ineligible contexts, copy-only separators and existing ownership invalidation.
- Full `npm test`: 1,175 passed, 0 failed, 1 existing opt-in Chrome layout test skipped.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed; existing large-chunk advisory remains.
- `git diff --check`: passed.

Not checked: native desktop visual interaction. Go tests/lint were not rerun for this frontend-only change.
