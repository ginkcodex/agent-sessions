# Chat MCP and plugin commands — provider contract

Supporting note for [chat-plugin-commands.md](chat-plugin-commands.md),
written by [CPC-02](../tasks/chat-plugin-commands/CPC-02.md). Everything
below was observed on 2026-11-15 on the local Linux host with read-only CLI
help/list calls, ACP `initialize` + `session/new` probes in an empty
`/tmp` workspace (no prompt sent) and existing native logs. No plugin, model
turn or MCP mutation ran.

## Versions

| Tool | Version |
|---|---|
| Claude Code CLI | 2.1.325 |
| `@agentclientprotocol/claude-agent-acp` (pinned) | 0.106.0 |
| Codex CLI | 0.169.0 |
| `@agentclientprotocol/codex-acp` (pinned) | 2.6.0, bundling `@openai/codex` 0.166.0 |
| O‍penCode | 2.3.0 |

## Support matrix

| Agent | `/mcp` | `/plugin` | Plugin commands |
|---|---|---|---|
| Claude | ✅ adapter command | ✅ app runs `claude plugin list --json` | ✅ advertised as `/plugin:command` |
| Codex | ✅ adapter command | ✅ app runs `codex plugin list --json` (slow) | 🔶 advertised as `$plugin:skill` mentions; invocation unverified |
| O‍penCode | ❌ not advertised; CLI has text output only | ❌ CLI has text output only | ❔ custom commands advertised; no plugin signal |

✅ verified interface · 🔶 listed, behavior unverified · ❔ unverified ·
❌ unsupported in the first release, said so explicitly in the UI. O‍penCode
inventory is tracked in [CPC-09](../tasks/chat-plugin-commands/CPC-09.md).

## `/mcp`: use the adapter

Both pinned adapters advertise `mcp` in `available_commands_update` and handle
it themselves, so the chat sends the plain prompt text `/mcp` as an ordinary
turn. The status is session state, which no CLI listing can give.

- **Claude** (`dist/mcp-command.js`): exactly `/mcp` reads
  `query.mcpServerStatus()` with a 5 s timeout and renders a Markdown list
  grouped Failed / Needs authentication / Connecting / Connected, plus the
  disabled servers. Hint: `[reconnect|enable|disable [<server>|all]]`. Those
  arguments change server state, so the picker inserts the bare `/mcp` only.
  The adapter's replay skips `/mcp` (`REPLAY_HIDDEN_COMMANDS`), so the result
  is not replayed as a saved message.
- **Codex** (`dist/index.js`, `case "mcp"`): the adapter runs its own
  `mcpCommand.run(rest, …)` and sends the result as an
  `agent_message_chunk`; no Codex model turn starts. Description: "Show the
  status of the MCP servers, or reconnect them."
- **Not used:** `claude mcp list` health-checks (spawns or connects) every
  approved server and has no JSON. `codex mcp list --json` returns
  `transport.{url, http_headers, env_http_headers, bearer_token_env_var,
  http_headers_helper}`, which can carry secrets. `opencode mcp list`
  reports connection status as text only.

**Result display:** an exact `/mcp` is still sent as a prompt, but the app
captures that turn's reply and shows it in the command result modal instead
of the transcript (see the plan's "Command result modal"). Both adapters
format the reply the same way: a `**MCP servers:** N (…)` summary,
`**<Status>**` groups of ``- `name`: detail`` lines, a `**Disabled:**` line,
and notes. Claude's groups are Failed, Needs authentication, Connecting and
Connected. `/mcp` with arguments stays an ordinary agent turn. We write
nothing about it to any session store.

## `/plugin`: app-side native listing

Neither adapter advertises `plugin`. The app runs a fixed argv in the
session's cwd, login environment and host, with no shell and no user input
in the argv.

### Claude: `claude plugin list --json`

- Took under 1 s. Output is a JSON array, one object per installed plugin.
- Observed keys: `id`, `version`, `scope`, `enabled`, `projectEnabled`,
  `installPath`, `installedAt`, `lastUpdated`, `hasUserConfig` (the last one
  only appears on some entries).
- `id` is `<name>@<marketplace>`, e.g. `ponytail@ponytail` 5.3.3,
  `understand-anything@understand-anything` 2.9.8,
  `gopls-lsp@claude-plugins-official` 1.0.0. gopls-lsp has no commands but
  is still listed.
- **Safe fields to keep:** `id` (split into name and marketplace), `version`,
  `scope`, `enabled`, `projectEnabled`. Drop the rest. `installPath` points
  into the user's home and the UI does not need it. `hasUserConfig` hints
  that plugin config exists; never read that config.

### Codex: `codex plugin list --json`

- Took 68 s here, because it queries remote marketplaces. A 20 s timeout
  killed it. It needs a ≥ 90 s timeout, a visible loading state, and
  cancellation when the chat closes.
- Output is an object `{installed: [...], available: [...]}`. `available`
  is only filled with `--available`, which we don't pass.
- Observed keys on installed entries: `pluginId`, `name`, `marketplaceName`,
  `version`, `installed`, `enabled`, `source`, `installPolicy`,
  `authPolicy`. Five were installed, e.g. `sites@openai-curated-remote`
  1.1.6 and `visualize@…` 1.0.89. `visualize` advertised no skills.
- **Safe fields to keep:** `pluginId`, `name`, `marketplaceName`, `version`,
  `enabled`. Drop `source` (it holds a connector id), `installPolicy` and
  `authPolicy`.

### O‍penCode

`opencode plugin list` and `opencode mcp list` print plain text ("No plugins
found", "No MCP servers configured"), with no JSON flag. `opencode api GET
/mcp` returned the web UI's HTML. The first release reports "not supported
for O‍penCode" instead of parsing text.

### Disk state vs session state

Both CLI listings report installed state on disk. The ACP catalog shows what
the running session loaded. Claude advertises `reload-plugins` ("Activate
pending plugin changes in the current session"), which confirms the two can
differ. The UI labels listing rows "installed" and "enabled". It shows a
plugin as loaded only when the session catalog has a command whose plugin
prefix matches.

## Command catalog and plugin membership

`available_commands_update` entries are `{name, description, input:
{hint}|null}`. Neither adapter sent `_meta` on any command.

- **Claude:** plugin commands are named `<plugin>:<command>`, e.g.
  `ponytail:ponytail` (hint `[lite|full|ultra|off]`) or
  `understand-anything:understand`. The description starts with
  `(<plugin>) `. Other names without a colon also appear: user skills
  (`plugin-authoring`), built-ins (`reload-plugins`, `reload-skills`, `mcp`).
  MCP prompts are `mcp:<server>:<prompt>`, which also has a colon.
- **Codex:** built-ins are `plan, mcp, skills, status, review,
  review-branch, review-commit, compact, goal, rename, logout`. Skills come
  with a `$` prefix. Plugin skills look like `$sites:sites` or
  `$work-pets:create-pet`. Standalone skills look like `$imagegen`.
- **Membership rule:** a command belongs to plugin P only when its name,
  after stripping a leading `$`, starts with `P:`, and P is the `name` of an
  installed plugin from the inventory. `mcp:` is never a plugin. The
  `(<plugin>)` description prefix is only a hint, not proof. Without an
  inventory, commands with a colon are shown as "commands" and are not
  labeled as plugins.
- **Canonical names and aliases:** insert the advertised name exactly:
  `/ponytail:ponytail args` for Claude and `$sites:sites args` for Codex.
  The short name `ponytail` resolves to `ponytail:ponytail` only when exactly
  one advertised command has that suffix after the colon, or that plugin
  name. Otherwise the picker asks the user to choose. Arguments pass
  through verbatim.

## Saved invocation records (Claude)

CLI sessions (`"entrypoint":"cli"`) record an explicit plugin invocation as:

1. A user record, not meta, carrying `origin`, whose string content is
   markers only. Two variants were found:
   - `<command-message>ponytail:ponytail</command-message>\n<command-name>/ponytail:ponytail</command-name>`
     (3 seen, plus 1 for `understand-anything:understand`);
   - the same plus `\n<command-args>ARGS</command-args>` (2 seen for
     ponytail, 1 for `understand-anything:understand-chat`).

   `command-message` comes before `command-name`. The name is canonical and
   has a leading `/`.
2. A user record with `isMeta: true, turnCompanion: true` and array text
   content that is the expanded skill (`Base directory for this skill: …`).
   It must stay hidden.
3. `attachment` records (`environment`, `model`, `deferred_tools_delta`, and
   in other files `command_permissions`, `hook_success`), then the assistant
   turn.

Local built-ins (`/model`, `/plugin`, `/help`) put `<command-name>` first and
carry no `origin`. A `<local-command-stdout>` record follows them. Both stay
hidden, consistent with the adapter's `REPLAY_HIDDEN_COMMANDS` and its
skipping of local-command output.

The normalized text is `` `${name} ${args}` `` trimmed, or the name alone,
which matches what the adapter's `commandInvocationFromMarkerOnlyText`
replays. A record counts as an invocation only if its whole content is
markers and whitespace. Prompts that mention `<command-name>` inside other
text stay ordinary prompts.

**Unverified:** none of the 72 ACP-written logs (`"entrypoint":"sdk-ts"`)
contain command markers, because no plugin command has been sent through ACP
yet. CPC-06 implements the CLI shape above. CPC-08 must confirm the ACP shape
with an approved run in a disposable workspace. If ACP stores the raw
`/ponytail:ponytail args` text instead, it is already an ordinary user
prompt and only the CPC-07 reconciliation applies.

Codex and O‍penCode saved invocation shapes were not examined. Their prompts
are already stored as plain user text.

## Not checked

- Remote (WSL/SSH) hosts: same argv expected, not run.
- macOS/Windows CLI output.
- Older CLI versions without `--json`. CPC-04 treats an unknown flag or a
  JSON parse error as "unsupported version", never as an empty listing.
