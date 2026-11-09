# Terminal file path detection

## Goal

Ctrl/Cmd-click on terminal tool output such as
`Update(docs/tasks/editor-essentials/README.md)` opens the file, not a path
prefixed with `Update(`. Only the path itself is highlighted.

## Design

Keep extraction in `frontend/src/lib/terminalFileLinks.ts`. Recognize an
identifier-style tool-call wrapper around a path before applying existing path
validation. Retain exact UTF-16 offsets so the xterm provider maps the inner path
to physical cells, including wrapped rows. Support quoted arguments with spaces
and existing line/column suffixes. Strip only a complete outer wrapper; preserve
parentheses belonging to filenames. Git status handling remains unchanged.

## Decisions

- Use structural wrapper detection, not an agent-specific tool-name allowlist.
- Preserve URL rejection and the existing bounded extraction limits.
- Do not change cwd tracking, backend resolution or modifier-key behavior.
- File existence checks remain click-time checks, never hover-time I/O.

## Risks

A bare filename shaped exactly like a tool invocation is ambiguous; structural
wrappers are interpreted as presentation syntax. Ordinary parentheses within
paths (such as `src/file(copy).ts`) must survive. URLs containing parentheses
must not become partial file links.

## Release criteria

- The screenshot's `Update(...)` path and extensionless `Update(TASKS.md)`
  targets have exact inner-path offsets.
- Quoted paths, location suffixes, balanced filename parentheses and multiple
  calls on one row have regression coverage.
- Provider tests verify wrapped path ranges and Ctrl/Cmd activation targets.
- Focused/full frontend tests, type checks and production build run; record
  actual results and any native desktop verification gap in the task evidence.
