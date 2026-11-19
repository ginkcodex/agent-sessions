# AGY-13 — `/mcp` in an Antigravity chat

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** [AGY-06](AGY-06.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). Sending `/mcp` in an
Antigravity chat does not list the MCP servers. The chat shows
"Antigravity is working…". The agent then treats `/mcp` as a prompt. It
reads agy's docs and `mcp_config.json` files and asks for permission to
read `~/.gemini/antigravity/mcp_config.json`. In the `agy` TUI, `/mcp`
("Manage MCP servers") is a local command.

## What the code does now

- `frontend/src/lib/chat.ts`:
  - `isMcpListing` captures a bare `/mcp` for the MCP modal only when the
    agent advertises `mcp`. It then reads Claude's or Codex's Markdown
    reply.
  - `inventoryCommands` adds the app's own listings only for OpenCode
    (`/mcps`, `/skills`) and Codex (`/skills`). For Antigravity there is
    none, so `/mcp` is sent to `agy acp` as a prompt.
- `internal/provider/antigravity` does not implement
  `provider.InventoryLister`. The plan listed the slash-command inventory as
  a v1 non-goal.
- The "CMD" chip on the sent prompt comes from `isSlashCommand`, which looks
  only at the text. It does not show that agy advertised the command.

## Facts so far

- `agy mcp list` (agy 1.5.12, Linux) prints `No MCP servers configured.`
  and exits 0. It has no JSON flag. `agy mcp` also has `add`, `remove`,
  `enable` and `disable`, which the app must never run.
- The AGY-01 spike saw `available_commands_update` with 13 commands (for
  example `goal`, `compact`, `schedule`). It did not record whether `mcp` is
  one of them.

## Scope

1. **Spike (in this task):**
   - Record the command names `agy acp` advertises.
   - Record what `agy acp` does with a bare `/mcp` prompt.
   - Record what `agy mcp list` prints with one or more servers configured:
     enabled, disabled and failing ones. This needs a machine with servers
     configured (none on the Linux box).
2. **Fix:**
   - Answer a bare `/mcp` in the app, as `/mcps` is for OpenCode. Add an
     `InventoryLister` on `antigravity.Provider` that runs `agy mcp list`
     (read-only). Show the servers in the existing MCP modal.
   - Add `antigravity` to `inventoryCommands` with the CLI's spelling
     `/mcp`. Then `isMcpListing` and `pickerCommands` do not send it to the
     agent.
   - If `agy mcp list` has no stable format, parse only what was seen. Fail
     with the CLI's text in the error rather than guess.
   - `/mcp` with arguments stays an ordinary prompt, as for the other
     agents.

## Likely files

- `internal/provider/antigravity/inventory.go` (new), with tests and a
  fixture of `agy mcp list` output.
- `frontend/src/lib/chat.ts` (`inventoryCommands`, `inventoryListing`).
- `frontend/test/` chat command tests, `frontend/src/lib/mock/mockChat.ts`.
- `docs/plan/antigravity-cli.md` (drop the non-goal).

## Acceptance criteria

- A bare `/mcp` in an Antigravity chat opens the MCP modal with the servers
  `agy mcp list` reports, or says none are configured. Nothing is sent to
  the agent.
- The composer's picker offers `/mcp` for Antigravity once, not twice.
- `/mcp` for Claude Code, Codex and OpenCode works as before.

## Tests

- Go: parse fixtures of `agy mcp list` output (none, several, unknown
  format).
- Frontend: `inventoryListing('/mcp', 'antigravity')` is `'mcp'`.
  `pickerCommands` has no duplicate. The other agents are unchanged.
- Manual: `/mcp` in a live Antigravity chat on macOS.

## Evidence

Spike, agy 1.5.12 on Linux:

- `agy acp` advertises 13 commands in `available_commands_update`, and `mcp`
  is not one of them:
  - goal, compact, schedule, browser, plan, grill-me, teamwork;
  - learn, boost, agy-customizations, antigravity-guide, run-workflow,
    swarm.
- A bare `/mcp` prompt over ACP goes to the model. It answers that the
  command "was sent directly as a prompt" (throwaway session `dacefd7c-…`,
  `/tmp/agy-probe-ws`).
- `agy mcp list`, run with `HOME` set to a throwaway folder (the real
  config was not touched), after `agy mcp add` and `agy mcp disable`:

  ```
  NAME       TYPE   STATUS    COMMAND/URL
  api        http   disabled  https://example.com/mcp
  fs         stdio  enabled   npx -y @modelcontextprotocol/server-filesystem /work
  gone       stdio  enabled   /nonexistent/bin
  my server  stdio  enabled   echo hi
  ```

  - With none configured it prints `No MCP servers configured.`
  - The status is only `enabled` or `disabled`. `list` does not connect, so
    a server whose command does not exist (`gone`) is still `enabled`; there
    is no failing state to show.
  - Names may hold spaces. Headers and env values are not printed. A URL or
    command line may still carry a token.
  - The config it reads is `~/.gemini/config/mcp_config.json`.

Done:

- `internal/provider/antigravity/inventory.go`:
  - `InventoryList` runs `agy mcp list` for MCP, and lists no skills.
  - `parseMCPList` cuts each row at the header's column starts, so names
    with spaces work. It keeps the name and status, and drops the
    command/URL. Any other text, or a status other than enabled or
    disabled, is an error. The chat then says "this Antigravity version does
    not list its MCP servers in a form the app reads; update it", the same
    message the other agents get.
- `frontend/src/lib/chat.ts`:
  - `inventoryCommands('antigravity')` is `/mcp`, so a bare `/mcp` opens the
    MCP modal and is not sent.
  - `pickerCommands` now adds the app's listings for every agent, not only
    Codex and OpenCode. An advertised command of the same name is replaced,
    so `/mcp` is offered once.
  - The modal titles the `enabled` group "Enabled".
- `mockChat.ts` lists two agy servers. The plan's non-goal now excludes only
  plugins and skills.

Tests:

- `go test ./internal/provider/antigravity/ ./internal/engine/`: ok.
  `TestParseMCPList` covers the captured output (none, and several with
  enabled, disabled and a name with a space), and refuses other text.
  `golangci-lint`: 0 issues.
- A throwaway engine test (deleted afterwards) ran the real `agy mcp list`
  through `runListing` and `parseMCPList`. It gave the 4 servers above with
  the throwaway `HOME`, and `[]` with an empty one.
- `frontend/test/chat-ui.test.mjs`: 40 pass, 0 fail. New test:
  - `inventoryListing('/mcp', 'antigravity')` is `mcp`;
  - `/mcp list` and the other agents' `/mcp` are not;
  - the picker has one `/mcp`;
  - an enabled and a disabled server give "2 (1 enabled, 1 disabled)".
- `npm run check`: 0 errors, 0 warnings.

Not checked:

- `/mcp` in the desktop app on macOS.
- Workspace-level MCP config: whether `agy mcp list` lists it is not known.
