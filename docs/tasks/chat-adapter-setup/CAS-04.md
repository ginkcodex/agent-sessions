# CAS-04 — Settings rows and one-click Install

- **Status:** ✅ Done
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

## Evidence

What was done:

- New `ChatAdapterRow.svelte`: one row per agent with its name, a short
  status (Not installed · Installing… / Updating… · `<version> · <pinned>
  available` · Ready · `<version>` · Ready · set in config.toml · the reason
  when the app cannot install), and Install, Update or Cancel. The path is in
  the status tooltip; the button's tooltip names the package, version and
  host. A config.toml adapter that is missing gets a one-line pointer to
  `[chat.adapters]`.
- `ChatAdapters.svelte`: one hint ("Chats run each agent through its ACP
  adapter on <host>.") and the rows. `AdapterMissing.svelte` (review card,
  npm command, Copy) is removed.
- `adapterInstall.svelte.ts`: `run(tool)` proposes and, when the proposal is
  for the same tool, installs it with its token at once; `dismiss()` is gone.
  The installer's shared "request fresh approval to retry" ending becomes
  ". Try again", since the button is the approval.
- Mock: managed agents report `version`, `pinned` and `outdated`; an
  `outdated` set lets the mock show Update. The start error points to
  Settings › Coding agents.

Verified:

- `node --test test/adapter-install.test.mjs`: 9 pass. Covers one click
  sending `propose` then `install` with the proposal's token (and one install
  only); nothing installed for an old server, a host without managed
  installs, another host or an expired offer; every row state; no `npm` or
  Copy rendered; not installable; config.toml override; Installing…,
  Updating… and Cancel; a failure's message, reworded.
- `npm run check`: 0 errors, 0 warnings. `npm test`: 1316 tests, 1315 pass,
  1 skipped, 0 fail. `npm run build`: ok.
- Screenshots of the section in dark and light (SSR render of the real
  components with the app's theme CSS, headless Chrome): fresh host;
  installing with an Update offered; all ready with one failure. Rows align,
  the disabled Update while another install runs reads as disabled, and the
  failure line wraps under its row.

Not checked: the section inside the running desktop app, and a real install
or update (CAS-05).
