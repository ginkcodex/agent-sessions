# AGY-13 — `/mcp` in an Antigravity chat

- **Status:** ⬜ Not started
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
