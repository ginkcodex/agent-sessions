# CMQ-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `docs/plan/chat-message-queue.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan records what each adapter supports, the queue and Send now
  design, and the decisions: Enter queues, Send now steers, the engine owns
  the queue, and only a normal turn end sends the next message.

## Evidence

- Wrote the plan, this task group (CMQ-01 to CMQ-08) and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.
- Adapter support was checked in the installed bundles: claude-agent-acp
  0.106.0 (`dist/v1/acp-agent.js`, steering and the initialize `_meta`),
  codex-acp 2.6.0 (`performSteeringRequest`) and the opencode binary's
  strings.
- Links checked by opening each path; nothing else to run.
