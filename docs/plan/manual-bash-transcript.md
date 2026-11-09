# Manual Bash transcript presentation

## Goal

Render Claude Code's user-run `!` Bash commands as readable command/output records, not chat bubbles containing `<bash-input>`, `<bash-stdout>` and `<bash-stderr>` wrappers. Keep generated session titles and previews free of those wrappers and output noise.

## Design

- Recognize complete, dedicated Bash wrapper records from the Claude provider only. Preserve literal tags in ordinary prose, quoted examples and malformed records.
- Present commands in a compact terminal-style card with a Bash / manually run label, monospace whitespace-preserving content and a copy action. Present output with separate stdout/stderr labels, omit empty streams, and bound long output with accessible disclosure/scrolling.
- Keep command and output records independent: JSONL pagination and incremental updates must not require cross-message pairing or synthetic IDs.
- Do not interpret stderr as a failing exit code: no exit status is provided by these wrappers.
- Use the existing transcript typography, theme tokens and safe text rendering. Output must not execute HTML or be interpreted as Markdown.
- Prefer meaningful conversation prompts for generated titles; manual command input can supply a readable fallback, but raw output must not become a title. Explicit user-renamed titles remain authoritative.
- Do not rewrite source session files or change command execution behavior.

## Decisions

Keep the existing shared message contract where practical; choose a small presentation adapter rather than adding a provider-independent execution protocol. Retain original message content for export/search and only transform dedicated records for presentation. No command re-run button.

## Risks

Wrapper-looking text can also be user content, so recognition must be conservative and tested. Large output must not expand the whole chat unboundedly. Existing cached metadata may require an explicit invalidation/version bump if title extraction changes. Native light/dark appearance and remote delivery require manual QA beyond unit tests.

## Release criteria

- Screenshot-shaped command and stdout/stderr records render without protocol wrappers.
- Both streams, empty streams, multiline content, HTML-like output, long output and malformed/quoted wrappers have regression coverage.
- Generated titles/previews do not expose output records; manual-only sessions have useful fallbacks.
- Existing transcript filters, pagination, copying and normal user messages remain supported.
- Targeted backend/frontend tests, frontend type checking/build and available regression suites pass; unrun manual checks are explicitly recorded.
