# ACP-01 — Plan, task group and non-goal reversal

- **Status:** ✅ Done
- **Estimate:** 1.5 h
- **Depends on:** —
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Write the plan and this task group, before any code. Update the SPEC non-goal
"Starting/driving agents from within the app" and the M8 note "no ACP client"
so both point to the plan. Add the group to the indexes.

## Likely files

- `docs/plan/acp-chat.md`, `docs/plan/README.md`
- `docs/tasks/acp-chat/`, `docs/tasks/README.md`, `TASKS.md`
- `SPEC.md`, `docs/plan/M8.md`

## Acceptance criteria

- The plan has a goal, design, decisions, risks and release criteria.
- Every task is at most 4 h and has scope, likely files, acceptance criteria
  and tests.
- The SPEC and M8 no longer contradict the plan.
- All three indexes list the group.

## Tests

Docs only. Check the links with `git grep`.

## Evidence

- Wrote the plan, the task group (ACP-01 to ACP-14) and the index rows.
- Updated SPEC §1 non-goals and the M8-11 note.
- Checked the links to `acp-chat.md` and `acp-chat/README.md` with `git grep`.
  No code changed, so no tests ran.
