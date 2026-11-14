# Chat MCP and plugin commands

## Goal

Start with read-only `/mcp` and `/plugin` listings in Live chat, plus invoking
installed plugin commands such as Ponytail and Understand Anything. Preserve
the user's invocation as a normal user message, during the live turn and
when the saved transcript is reopened. A general catalog of terminal built-ins
and plugin management (install, enable, disable, update, uninstall) is not part
of this first release.

## Design

### Read-only inventory

- `/mcp` lists the selected agent session's configured MCP servers and their
  status. Prefer the adapter's existing command so session-specific state is
  authoritative. Do not send a natural-language request asking the model to
  guess the inventory. Listing must not reconnect or change a server.
- `/plugin` lists installed plugins for the selected agent on the selected
  host, distinguishing installed, enabled and loaded when those facts are
  available. Include identity, version and scope where reported. Plugins with
  no commands must still appear; ACP command names are not a plugin inventory.
- Claude's CLI supports `claude plugin list --json`; run its read-only listing
  in the same cwd/environment/host used to launch the session, with bounded
  execution/output and parsed, validated output. Verify supported agents'
  native inventory interfaces before implementing their branches. Never read
  credential files or expose environment variables, headers, tokens or secret
  plugin configuration in a listing.
- Missing inventory support is explicit, not an empty success and not a
  fallback model prompt. Do not imply every provider has Claude-style plugins.
- Keep inventory UI/results separate from model turns unless the adapter
  itself implements the listing as a prompt command. Define result lifetime
  explicitly: transient inspection results must not masquerade as saved agent
  messages or be written into the agent's session store.

### Plugin command discovery and invocation

- Retain ACP `available_commands_update` as per-chat capability state, separate
  from transcript text. Preserve updates received during setup/load replay,
  and return the latest catalog with chat state for reattachment. An empty
  update clears the catalog; closed chats/host switches cannot leak old data.
- Limit the initial command UI to inventory entries and plugin command
  discovery, not an exhaustive terminal command palette. Show descriptions and
  argument hints supplied by the adapter. The precise plugin-membership signal
  must be validated per adapter; do not classify every colon name as a plugin.
- Use the canonical name advertised by the adapter, e.g.
  `/ponytail:ponytail` and `/understand-anything:understand`. Match familiar
  short names in the picker, resolving to a canonical command only when
  unambiguous; ambiguous names require a choice. Preserve arguments verbatim.
- Sending a plugin command is an ordinary, permission-controlled agent turn,
  not a client-side reimplementation of its skill prompt or hooks. Do not
  automatically run a plugin to test discovery.

### Command result modal (added 2026-11-15)

User feedback: the `/mcp` reply reads as an assistant message. Inventory
results belong in a modal like VS Code's "MCP servers", not in the transcript.

- An exact `/mcp` or `/plugin` opens one modal shell (title, filter box, close
  button, Escape, focus trap via `modalFocus`). Neither result appears in the
  transcript.
- `/mcp` is still the adapter's own command, because only the session knows
  server status. Claude's adapter has no extension method for it, and Codex's
  runs inside the prompt handler. The chat sends `/mcp` as a prompt and marks
  that turn as an inspection. Its user prompt and agent text go to the modal,
  not the live tail. It is offered only while the chat is idle with an empty
  queue, so no other turn's text can mix in.
- Both pinned adapters write the same Markdown shape:
  - a `**MCP servers:** N (…)` summary;
  - `**<Status>**` groups of ``- `name`: detail`` lines;
  - a `**Disabled:**` line;
  - free notes and hints.

  The modal parses it into server rows grouped by status, each with a status
  badge and its detail. A reply it cannot parse is shown as rendered Markdown,
  never as an empty list.
- `/mcp` with arguments (`reconnect`, `enable`, `disable`) changes server
  state. It stays an ordinary agent turn in the transcript. The modal has no
  Add, Reconnect or Enable actions.
- `/plugin` moves from the panel above the composer into the same modal, with
  the same data and states (CPC-10).

### Transcript fidelity

- Display a user-submitted command and its arguments once as a user message,
  regardless of whether the adapter echoes it or expands it into skill text.
- In saved Claude transcripts, recognize explicit command invocation metadata
  and normalize it to readable `/command arguments`, rather than hiding it
  with injected context. Keep expanded skill instructions and local-command
  caveats hidden at Chat level; never expose all meta messages as a workaround.
- Distinguish explicit user invocation from model-triggered Skill tool calls;
  the latter must not become invented user messages. Preserve stable message
  IDs, timestamps and ordering, and avoid duplicate invocation/expansion rows.
- Verify the live-to-saved reconciliation path so a command does not disappear
  or duplicate when the agent saves, on reload, or during incremental loading.
  If the native log never records the invocation, document the limitation;
  do not fabricate a saved user message or edit the native session store.

## Investigation evidence (2026-11-14)

- The current Claude ACP probe advertised `ponytail:ponytail` and
  `understand-anything:understand` after session setup, without sending a
  prompt. Both plugins are therefore discoverable through the pinned adapter.
- Read-only `claude plugin list --json` returned Ponytail, Understand Anything
  and gopls-lsp, including identity/version/scope/enabled fields. `/plugin`
  itself was not advertised as an ACP command.
- A read-only transcript investigation located existing CLI invocation records
  for both plugins in this workspace's native logs: a non-meta user record
  containing command markers, followed by an `isMeta` expanded-skill companion.
  `internal/provider/claude/load.go` currently treats `<command-` prefixes as
  meta, and `scan.go` excludes them from prompt counting/title/turn state.
- `frontend/src/lib/chatTail.ts` reconciles prompts by exact text; a short
  submitted command and its saved namespaced form need identity-aware matching
  without accidentally merging different commands/arguments or alias collisions.
- These observations do not verify a real ACP plugin turn. CPC-02/CPC-08 must
  validate that path rather than assuming the CLI log shape is universal.
- The verified per-agent interfaces, safe fields, membership rule and record
  shapes are in the [provider contract](chat-plugin-commands-contract.md)
  (CPC-02). `/mcp` is handled by both adapters. `/plugin` is an app-side
  listing for Claude and Codex. O‍penCode is unsupported in the first release.

## Decisions

1. Start with inventory and plugin commands, not all terminal slash commands.
2. Inventory is deterministic and read-only; execution belongs to the agent.
3. Installed plugins and callable commands are distinct data sources.
4. The host and workspace determine the inventory, not the desktop's home.
5. No automatic execution of `/understand`, `/ponytail`, or other plugins for
   QA: those can modify a workspace or spend tokens. Use fixtures by default
   and obtain approval for a real command run in a disposable workspace.

6. Inventory results (`/mcp`, `/plugin`) show in a modal, not in the
   transcript. `/mcp` stays the adapter's turn, captured for the modal.

## Risks

- `/plugin` is terminal-bound in the inspected Claude session and absent from
  the ACP catalog; native CLI inventory needs its own app-side path.
- Provider versions have different plugin/extension concepts and inventories.
  The capability investigation must establish the actual support matrix.
- Logs may persist command markers, expanded skill prompts, or both. A broad
  metadata filter change could leak internal context or create duplicates.
- Short command aliases can collide. Never silently choose a plugin.
- The modal depends on the adapters' `/mcp` Markdown shape. A change falls
  back to showing the Markdown; parser tests pin today's shape from both
  adapters.
- Codex's `/mcp` reloads the MCP configuration and may try a server sign-in.
  That is the adapter's behavior for any `/mcp`; the modal does not add to it.
- A captured `/mcp` turn is hidden from the live tail. Claude's saved log
  keeps local commands hidden (CPC-06), and Codex answers `/mcp` without a
  model turn. CPC-08 must confirm that nothing about it reappears after a
  reload.
- A CLI inventory reports disk state; a running session can still have an older
  loaded plugin set. Label those states rather than promising hot reload.

## Release criteria

- `/mcp` and `/plugin` show accurate read-only results for each supported
  agent/host combination; unsupported cases say so explicitly.
- Claude lists Ponytail, Understand Anything and commandless plugins when
  installed; discovery shows the advertised canonical plugin commands.
- Plugin commands preserve arguments and the ordinary approval/cancellation
  lifecycle; command selection alone executes nothing.
- Explicit command invocations appear once at Chat level live and after reopen
  where the native log records them, without XML or expanded skill prompts.
- Tests cover setup/replay/reattach, empty updates, host isolation, inventory
  errors/secret exclusion, aliases/collisions, marker normalization, metadata
  filtering, incremental parsing and live-to-saved reconciliation.
- Focused Go tests and frontend tests/check/build pass; broader checks and
  desktop/local/WSL/SSH QA are recorded as run, failed, or not checked.
