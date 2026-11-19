# AGY-07 — Frontend agent integration (types, icon, badges, settings, mocks)

- **Status:** ✅ Done
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

## Evidence

Done:

- `AgentID` gains `'antigravity'`, and `ALL_AGENTS` gains an Antigravity
  entry, which gives the label in chat, terminal, alert and discovery text.
- Accent colors `--agent-antigravity`: `#6d28d9` in the light theme and
  `#a78bfa` in the dark theme.
- `AgentIcon`: an arch glyph with the title `Antigravity`.
- Color rules for the sidebar chip, the session row (and its name), the
  header badge, and `Badge`'s `antigravity` variant.
- `GroupTree`'s empty state lists `~/.gemini/antigravity-cli/`.
- `HostEnvDialog` lists `ANTIGRAVITY_CLI_DIR`.
- Mocks:
  - a sample Antigravity session in `fixtures.ts`;
  - the agent in the mock agent settings, the new-session agents, `agy` for
    the new-session and resume commands (`agy --conversation <id>`), the
    panel label and the group filter;
  - an `agy` chat adapter in `mockChat.ts`.
- Six frontend tests asserted three agents; they now expect the fourth.
- Settings › Coding agents, chat adapters, and both New Session menus list
  agents from the backend (`GetAgentSettings`, `ChatAgents`,
  `NewSessionAgents`). They needed no change.

Results:

- `npm test` in `frontend/`: 1624 tests, 1619 pass, 0 fail, 5 skipped.
- `npm run check`: 0 errors, 0 warnings.
- Browser dev mode (`vite` with the mock API) in headless Chrome, light and
  dark themes. The session list shows the Antigravity row with its arch icon
  and violet badge, and the sidebar footer shows its chip with a count of 1.
  Both themes are legible.

Not checked:

- The header badge of an open Antigravity session. The headless screenshot
  cannot click a row, but the rule matches the other agents'.
- The desktop app itself. That is AGY-08.

Found: handoff into Antigravity launches a missing `antigravity` command.
This is [AGY-10](AGY-10.md).
