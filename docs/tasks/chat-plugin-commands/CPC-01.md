# CPC-01 — Plan and task breakdown

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope and likely files

Write the plan, this task group and the feature rows in `docs/plan/README.md`,
`TASKS.md` and `docs/tasks/README.md` before implementation.

## Acceptance criteria and tests

- Initial scope is read-only MCP/plugin inventory, plugin command invocation
  and transcript fidelity, not full terminal parity or plugin management.
- Every implementation/QA task is at most 4 h; uncertain provider contracts
  have an explicit investigation task before implementation.
- All index links resolve and no existing work is changed.

## Evidence

- Plan and eight tasks written before product code changes.
- Inspected pinned adapter versions (Claude ACP 0.106.0, Codex ACP 2.6.0).
- A direct Claude ACP initialize/session-new probe on 2026-11-14 advertised
  `ponytail:ponytail` and `understand-anything:understand`, with descriptions
  and argument hints. `/mcp` was present; `/plugin` was absent. No model prompt
  or plugin command was sent.
- `claude plugin list --help` and `claude plugin list --json` ran successfully;
  inventory included Ponytail 5.3.3, Understand Anything 2.9.8 and gopls-lsp.
- CLI disk inventory is not proof of loaded runtime state. Real plugin
  execution, desktop and remote QA have not run. No product tests were run
  for this documentation-only task.
- A Python path check validated all 23 links in the new plan/task documents
  and feature index rows. `git diff --check` passed.
