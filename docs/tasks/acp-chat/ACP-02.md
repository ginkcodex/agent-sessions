# ACP-02 — Spike with the real adapters

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-01
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Run `claude-agent-acp`, `codex-acp` and `opencode acp` on this computer with a
throwaway Go test client (not committed as product code). For each, record:

- the install command and the version tested;
- the `initialize` reply: protocol version, `loadSession`, resume, prompt
  capabilities and auth methods;
- example `session/update` and `session/request_permission` payloads for a
  turn that edits a file and runs a command;
- where the session is saved, and whether our provider scan finds it with the
  same ID as the ACP `sessionId`;
- whether `session/load` or `session/resume` accepts a session the normal CLI
  created, and the reverse: can the CLI resume an ACP session;
- whether saved records carry the `messageId` or tool call IDs seen in updates;
- what happens on cancel, on a closed stdin and on SIGTERM.

Then update the plan and the later task files with what was found, for example
an in-memory log for an agent whose ACP sessions are not saved.

## Likely files

- `docs/plan/acp-chat-spike.md` (new)
- `docs/plan/acp-chat.md`, later `docs/tasks/acp-chat/ACP-*.md`

## Acceptance criteria

- Each item above is answered per agent, or marked as not checked with the
  reason.
- The plan says which agents can continue an existing session in chat.

## Tests

Manual runs against real agents. Record the commands and outputs, with any
secrets removed.

## Evidence

Results are in [acp-chat-spike.md](../../plan/acp-chat-spike.md).

- **Setup:**
  - The adapters were installed with
    `npm install @agentclientprotocol/claude-agent-acp @agentclientprotocol/codex-acp`
    into `/tmp/acp-spike/adapters`, not on PATH. The versions were
    claude-agent-acp 0.106.0 and codex-acp 2.6.0. `opencode acp` 2.2.6 is
    built in.
  - A throwaway Go client in `/tmp/acp-spike/client` (not committed) logged
    every frame.
- **Runs:** 14 runs with the client: 4 for OpenCode, 5 for Claude and 5 for
  Codex, in a scratch repository under `/tmp/acp-spike/proj`. They covered:
  - a new session that writes a file and runs `ls`;
  - a permission allow and a reject;
  - a cancel after 4–5 s;
  - `session/load` of a session created by the CLI;
  - `session/resume` (Claude and Codex);
  - `session/set_config_option`.
- **Saved sessions:**
  - `agent-sessions-cli show` found each ACP session under the same ID, and
    the message IDs were compared with it.
  - `claude -p --resume`, `codex exec resume` and `opencode run -s` each
    continued an ACP-created session.
- **Process cleanup:** `pgrep` before and after each run showed no leftover
  adapter or agent processes.
- **Not checked:**
  - SIGTERM or SIGKILL of an adapter mid-turn (only a closed stdin was
    tested);
  - OpenCode `session/resume`;
  - `plan` updates (no agent sent one);
  - images, MCP servers, `session/list`;
  - WSL as a remote target, SSH hosts and Windows.

  ACP-04 covers the signal path with the fake agent, and ACP-14 covers the
  hosts.

The plan and tasks ACP-03, 04, 06, 08, 10, 11 and 14 were updated. ACP-15
(session option pickers) is new.
