# AGY-07 — Frontend agent integration (types, icon, badges, settings, mocks)

- **Status:** ⬜ Not started
- **Estimate:** 2.5 h
- **Depends on:** [AGY-05](AGY-05.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Wire `'antigravity'` across all Svelte/TypeScript UI touchpoints so Antigravity
sessions, sidebar chips, icons, theme accents, host env shortcuts, and
development mocks render cleanly alongside Claude Code, Codex, and OpenCode.

## Likely files

- `frontend/src/lib/types.ts` — add `'antigravity'` to `AgentID`.
- `frontend/src/lib/portable.ts` — add `'antigravity'` to `ALL_AGENTS`.
- `frontend/src/style.css` — define `--agent-antigravity` in light and dark
  themes.
- `frontend/src/lib/components/common/AgentIcon.svelte` — add Antigravity SVG
  glyph, title (`"Antigravity"`), and class.
- `frontend/src/lib/components/sidebar/AgentSummary.svelte` — add short chip
  label and `.chip.agent-antigravity` color rule.
- `frontend/src/lib/components/sessionlist/SessionRow.svelte` — add display
  name and `.session-row.agent-antigravity` rule.
- `frontend/src/lib/components/transcript/SessionHeader.svelte` — add
  `.agent-badge.agent-antigravity` styling.
- `frontend/src/lib/components/common/Badge.svelte` — add `'antigravity'`
  to the `variant` union and a `.badge-antigravity` rule.
- `frontend/src/lib/components/sidebar/GroupTree.svelte` — list
  `~/.gemini/antigravity-cli/` and Antigravity CLI in the empty-state text.
- `frontend/src/lib/components/common/HostEnvDialog.svelte` — add
  `ANTIGRAVITY_CLI_DIR` to `commonKeys`.
- `frontend/src/lib/mock/mockApi.ts` — add a mock Antigravity session and
  agent config entry for browser dev/UI tests.

## Acceptance criteria

- `svelte-check` and frontend unit tests (`npm test`) pass with zero type
  errors or unhandled `AgentID` switch branches.
- Antigravity icon, badge, sidebar filter chip, and session-row accent render
  legibly in both light and dark themes.
- Settings → Coding Agents and Chat settings show Antigravity automatically
  (driven by backend `GetAgentSettings` and `ChatAgents`).
- New Session menus (both terminal and chat) include Antigravity when enabled
  and detected.

## Tests

- Run `npm run check` (`svelte-check`) and `npm test` in `frontend/`.
- Verify browser dev mode (`mockApi.ts`) renders Antigravity sessions, chips,
  and badges cleanly.
