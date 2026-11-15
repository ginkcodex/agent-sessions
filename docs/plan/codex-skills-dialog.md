# Codex skills dialog

## Goal

Open a searchable dialog for Codex `/skills` without sending a prompt.

## Design and decisions

Use the existing inventory dialog and the live ACP command catalog. Codex ACP publishes skills as `$name` commands with descriptions, sourced from its skills/list request. Filter only those commands; keep skill invocation unchanged. OpenCode continues to read its CLI inventory. Explain that Codex results are skills available in this chat.

## Risks

The catalog reflects the latest adapter update, rather than a forced rescan. Empty catalogs show an empty state. Existing uncommitted inventory work must be preserved.

## Release criteria

Codex `/skills` appears in the command picker, opens/filter/closes the inventory without a prompt or CLI request, and excludes builtins. Frontend checks and relevant interaction tests pass.
