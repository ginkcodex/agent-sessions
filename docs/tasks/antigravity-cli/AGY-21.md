# AGY-21 — `/skills` in an Antigravity chat

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** [AGY-13](AGY-13.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Request

From desktop QA on macOS (user, 2026-11-20). The `agy` TUI answers `/skills`
with the skills it can use: 7 on the user's Mac. In an Antigravity chat the
app has no `/skills`, so it would go to the agent as a prompt. Answer it in
the app, as `/mcp` is ([AGY-13](AGY-13.md)), OpenCode's `/skills` and
Codex's `/skills` are.

## Spike (agy 1.5.12, Linux)

Where else the list could come from, and why not:

- `agy` has no `skills` subcommand (`agy --help`).
- `agy acp`'s `available_commands_update` gives 13 commands with only a name
  and description. Four are skills (`agy-customizations`,
  `antigravity-guide`, `run-workflow`, `swarm`), nine are agy's own
  commands (`goal`, `compact`, …), and nothing tells them apart. Three skills
  the TUI lists (`automation`, `migrate-agent-settings`,
  `migrate-workflows`) are not advertised.
- Reading `SKILL.md` folders cannot match agy: it walks `.agents/` (and
  `.agent/`, `_agents/`, `_agent/`) up to the repository root, reads
  `skills.json` entries with include and exclude patterns, and mounts
  built-in skills by name from server-side experiments. The built-in folder
  holds 17 skills; agy uses 7.

What works: `agy -p /skills` answers in agy itself, without a model turn:

- It ran in 3 s. No session was made (`brain/` count unchanged), and
  `--output-format json` gives `conversation_id: ""`, `num_turns: 0` and
  zero tokens.
- The text form is one `name<TAB>description` line per skill, sorted by
  name. A multi-line description breaks it.
- The JSON form adds
  `command: {name: "skills", data: {skills: [{name, description, path,
  builtin, model_invocable}]}}`. A multi-line description stays one string.
- It lists the same 7 skills as the user's macOS TUI, plus a workspace skill
  put in `<cwd>/.agents/skills/` for the test (removed afterwards).

The probe ran against the user's sign-in in throwaway workspace
`/tmp/agy20-ws`. Reaching the keyring the token moved to needed the app's
D-Bus session address. The ACP capture (`session/new`, no prompt) left one
empty session, `d7ae3c02-…`.

## Scope

- `antigravity.Provider.InventoryList(skills)`: run
  `agy -p /skills --output-format json --mode plan` in the chat's directory.
- Parse only the `command` block with `name: "skills"`. A reply without it,
  with a conversation ID or with a status other than `SUCCESS` is an error,
  never an empty list. Keep each skill's name and description; drop the
  path.
- `--mode plan`: if an agy without a local `/skills` sends it to the model,
  that turn cannot edit files.
- Frontend: `inventoryCommands('antigravity')` adds `/skills`, so a bare
  `/skills` opens the skills modal and is not sent; the picker offers it
  once.
- Plan non-goal: skills are no longer excluded.

## Acceptance criteria

- A bare `/skills` in an Antigravity chat lists agy's skills for the chat's
  directory. Nothing is sent to the agent.
- `/skills <text>` and the other agents' `/skills` are unchanged.

## Tests

- Go: `InventoryList` args; `parseSkills` reads the captured JSON (with a
  multi-line description) and refuses a model turn, a missing command
  block, another command and a nameless skill.
- Frontend: `inventoryListing('/skills', 'antigravity')` is `skills`; the
  picker has one `/skills`.
- Manual: `/skills` in a live Antigravity chat on macOS.

## Evidence

Done:

- `internal/provider/antigravity/inventory.go`: `InventoryList(skills)`
  runs `agy -p /skills --output-format json --mode plan`; `parseSkills`
  reads the command block as scoped. The chat then says "this Antigravity
  version does not list its skills in a form the app reads; update it" for
  anything else, as for the other agents.
- `frontend/src/lib/chat.ts`: `inventoryCommands('antigravity')` adds
  `/skills`. The skills modal reads it from the backend, as for OpenCode.
- `mockChat.ts` lists one agy skill. The plan's non-goal now excludes only
  the plugin listing.

Tests:

- `go test ./internal/provider/antigravity/ ./internal/engine/`: ok.
  `TestInventoryList` checks the args; `TestParseSkills` reads the captured
  JSON (a multi-line description stays one string, the path is dropped),
  reads `skills: null` as none, and refuses plain text, a model's reply, a
  conversation ID, `status: ERROR`, another command and a nameless skill.
  `golangci-lint` on the package: 0 issues.
- A throwaway test (deleted afterwards) ran the real `agy` with
  `InventoryList`'s args in `/tmp/agy20-ws` and parsed the 7 skills.
  The `brain/` count stayed at 10.
- `frontend/test/chat-ui.test.mjs`: new test, `inventoryListing` gives
  `skills` for a bare `/skills` only, and the picker has one `/skills`. The
  AGY-13 test's picker list now ends with `skills`.
- `npm test`: 1631 tests, 1626 pass, 0 fail, 5 skipped. `npm run check`:
  0 errors, 0 warnings.

Not checked:

- `/skills` in the desktop app on macOS, and through a remote (WSL or SSH)
  host's server.
- Plugin skills: no agy plugin is installed here, so a namespaced name was
  not seen.
- An agy older than 1.5.12: whether it answers `/skills` itself. If it
  does not, the app shows the update message after a plan-mode turn.
