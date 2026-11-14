# CPC-06 — Show explicit plugin commands in saved transcripts

- **Status:** ✅ Done
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

## Evidence

- Added `internal/provider/claude/command.go`, with `commandPrompt`. A text
  counts as a typed command only when it is entirely markers, in the order
  `<command-message>`, `<command-name>/…`, then optional `<command-args>`.
  The helper returns `/name args` trimmed, the same text the ACP adapter
  replays. Name-first built-ins (`/model`, `/help`), local output and the
  adapter's hidden set (`/compact`, `/mcp`, …) stay hidden. Prose that
  mentions markers is an ordinary prompt.
- In `load.go`, both string and block text are normalized. A record already
  marked `isMeta` stays meta. The expanded-skill companion is `isMeta`, so it
  stays hidden.
- `scan.go` counts invocations as prompts: they set the first and last
  prompt, and a turn starts as `working`. `checkpointVersion` is now 8, so
  cached metadata is rescanned.
- Tests:
  - `TestCommandPrompt`: 13 cases.
  - `TestLoadPluginCommand` plus the `load_plugin_command` golden. The
    fixture covers a string invocation, a block invocation with multiline
    arguments, a duplicate meta marker record, `/model` with its
    local-command output, and a prose mention.
  - The `plugin_command` fixture was added to the scan, every-boundary
    resume and incremental-parity tests.
  - A new turn-state case covers a typed plugin command.
- Ran `go test ./internal/provider/claude/` and `go test ./internal/...`: both
  pass. `go tool golangci-lint run ./internal/provider/claude/...` reports the
  same 4 issues as the base commit, all in untouched test files.
- Real logs, read-only: the built CLI's `show --json` on 3 local sessions
  shows `/ponytail:ponytail`, `/ponytail:ponytail full`, `lite`,
  `/understand-anything:understand` and `/understand-anything:understand-chat
  <args>` as plain user messages, with no markers or skill text.
- Limitation: the ACP-written log shape is unverified (see the contract). If
  ACP stores raw `/name args`, it already shows as a prompt. Frontend tests
  were not run: no frontend code changed, and meta filtering is unchanged.
  The live-to-saved reconciliation belongs to CPC-07.
