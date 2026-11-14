# CPC-06 — Show explicit plugin commands in saved transcripts

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CPC-02
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Normalize explicit user command metadata in Claude native logs into readable
user messages with command name and arguments. Keep injected skill expansion,
local caveats and unrelated metadata hidden. Preserve stable IDs, timestamps
and ordering; handle marker-only and marker-plus-expansion shapes based on
CPC-02 evidence. Do not turn model-triggered Skill calls into user messages.

## Likely files

- `internal/provider/claude/load.go`, `scan.go`, `incremental.go`
- Focused unit/golden fixtures and frontend transcript tests as necessary

## Acceptance criteria and tests

- `/ponytail` and `/understand`-style invocation records show once at Chat
  level, with arguments and no XML or expanded instructions.
- Ordinary angle-bracket text is not misclassified; malformed marker records
  fail safely. System reminders, caveats and skill context stay hidden.
- Verify string/block-array content, namespaced names, multiline arguments,
  duplicate companion records and incremental/full-load parity.
- Verify scan title/count behavior deliberately; don't invent a saved
  invocation when no native record exists or modify native logs.
- Run focused provider tests and relevant transcript/filter tests; record
  any genuine native-log limitation rather than marking it passed.
