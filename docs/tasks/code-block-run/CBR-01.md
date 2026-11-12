# CBR-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [code-block-run.md](../../plan/code-block-run.md)

## Scope

- `docs/plan/code-block-run.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan says which blocks get Run, how the command is cleaned, when it is
  typed into the shell, and why the chat itself cannot run it.

## Evidence

- Written before any code. Read first: the fenced-block renderer and Copy
  handler (`markdown.ts`, `copycode.ts`, `messageFileClick.ts`), `TextPart`,
  `MessageBubble`, `TranscriptView`, `LiveTail`, the terminals store
  (`newShell`, `write`, sinks' `paste`, `setCwd`), the panel shells' OSC 7
  prompt hook and payload mechanism (`internal/launch/panel_shell.go`), and
  xterm's `paste` (no control-character filtering).
- Checked the installed `claude-agent-acp` 0.106.0: no `!` or
  `<bash-input>` handling; text goes to the model as is.
