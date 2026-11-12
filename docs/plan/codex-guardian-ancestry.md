# Codex Guardian ancestry

## Goal

Display Guardian review threads as subagents of their recorded main session,
so existing filters, counts and child-first deletion include them correctly.

## Design and decisions

Read top-level parent_thread_id for a structured Codex subagent source.
Remember Guardian's explicit parent separately in scanner checkpoints. When
thread_spawn_edges exists, keep its ordinary authority (including removed
edges), but use Guardian's recorded parent if that child has no edge. Only
attach the fallback to an existing, different session. Never infer ancestry
from a title, working directory, timestamp or session_id. Bump checkpoint
version so unchanged, previously indexed Guardian files are reparsed.

Existing session trees and filters consume ParentID. Directory/group totals count main sessions; agent totals retain their existing
matching-record semantics. Hiding subagents excludes Guardian from those
matching-record totals too. Deletion uses
the existing validated descendant plan and child-first CLI execution.

## Risks

Stale caches, missing/self parent IDs, conflicting DB edges and changes to
ordinary subagent edge removal. Synthetic regressions cover these cases.
No live session is deleted during verification.

## Release criteria

Provider cold/warm/old-checkpoint regressions, engine tree/count/filter checks,
and a provider-to-deletion test with recorded CLI calls. Run affected Go suites
and existing frontend session-tree checks. Installed desktop QA is separate.
