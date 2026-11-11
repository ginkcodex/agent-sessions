# ACP-02 — Spike with the real adapters

- **Status:** ⬜ Not started
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
