# CAS-04 — Settings rows and one-click Install

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** CAS-03
- **Plan:** [chat-adapter-setup.md](../../plan/chat-adapter-setup.md)

## Scope

- `ChatAdapters.svelte`: one short hint and one row per agent (ready,
  config.toml, outdated, missing, installing, not installable, failed), as in
  the plan's table.
- Replace `AdapterMissing.svelte` with a row component; no copy command.
- `adapterInstall.svelte.ts`: Install and Update propose, then install the
  proposal at once when it is for the same host and tool; Cancel while it
  runs.
- The mock reports versions and an outdated agent.

## Acceptance and tests

- SSR tests for each row state; no npm command rendered.
- Store tests: one click sends `propose` then `install` with the token;
  nothing is installed for another host's or tool's proposal; an expired or
  failed proposal shows its message.
- `npm run check`, `npm test`, `npm run build`; a screenshot of the
  section in dark and light.
