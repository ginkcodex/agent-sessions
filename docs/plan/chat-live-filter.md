# Live filter for sessions running in chat

## Goal

Sessions shown as Live in chat must appear in the Live filter, including Codex
whose ACP process is not necessarily recognized by the CLI live detector.

## Design and decisions

Overlay process-wide chat ownership on independent Service read snapshots
before filtering/grouping/counting and on GetSessionMeta. Keep provider live
state in the catalog unchanged: a scan cannot erase chat liveness, and closing
a chat immediately falls back to the provider state without leaving a badge
in persisted metadata. An idle open chat is live, matching the header.
Notify catalog subscribers when chat ownership starts or ends so lists refresh
without waiting for filesystem activity. Preserve archive/query/agent filters.

## Risks

Registry access must be thread-safe; do not hold chat locks during callbacks.
Tests must use isolated ownership registries. New unsaved sessions still need
a provider record before appearing in the session list.

## Release criteria

Tests cover Live-only lists, groups, counts and metadata; start/idle/close,
provider-live preservation, scan replacement, and ownership notifications.
Run affected Go suites and existing frontend filter/store tests.

## OpenCode verification (CLF-02)

The same ownership overlay must work for OpenCode's ses_ session IDs and ACP
session/load fallback as well as Codex session/resume. Verify fresh chats
before/after their first catalog record, idle ownership and close/crash
notifications. This scope adds no detection for external CLI sessions.
