# AGY-18 — Share-safe export and import of Antigravity sessions

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-10](AGY-10.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). Export Session fails for
Antigravity. The Complete profile gives "unsupported action for this agent:
export: capture native records" (that is [AGY-19](AGY-19.md)).

The Share-safe profile fails too. On Linux, with agy 1.5.12:

```
$ agent-sessions-cli export antigravity 2b9a4c81-… --profile share-safe -o x.bundle
export: bundle: unsafe source agent "antigravity"
```

## Cause

`internal/bundle/safe.go` `safeAgent` allows only `claude-code`, `codex`
and `opencode`. AGY-02 added the agent ID elsewhere but missed this list.
`validateRequest` (`write.go`) and the manifest check (`read.go`) both use
it, so an Antigravity session can be neither written to a bundle nor read
from one.

Nothing else blocks a share-safe bundle. It holds only the redacted
transcript and the handoff, with no native records. The handoff already
names Antigravity as a source (AGY-10).

## Scope

- Add `antigravity` to `safeAgent`.
- Check that a share-safe bundle from Antigravity can be inspected and
  imported:
  - Import opens it as a handoff into a chosen agent.
  - Restore is not offered (there are no native records).
- Check the export dialog: Share-safe works, and Complete fails with the
  AGY-19 error until that task is done. If the dialog can tell up front,
  disable Complete for Antigravity with the reason rather than failing on
  Export.

## Likely files

- `internal/bundle/safe.go`, `internal/bundle/*_test.go`.
- `internal/engine/export.go` / `import.go` tests.

## Acceptance criteria

- `agent-sessions-cli export antigravity <id> --profile share-safe -o …`
  writes a bundle that `inspect` reads.
- The desktop Export Session dialog exports an Antigravity session with the
  Share-safe profile.
- Importing that bundle offers the handoff, and not restore.
- Claude Code, Codex and OpenCode bundles are unchanged.

## Tests

- Go:
  - a share-safe write/read round trip with an Antigravity source and
    sessions;
  - an unknown agent is still refused.
- Manual: export and import in the desktop app.

## Evidence

Done:

- `safeAgent` in `internal/bundle/safe.go` allows `antigravity`. Bundle
  write and read both use it, so this one change fixes export and import.
- The export dialog is unchanged. Complete still fails with the AGY-19
  error until [AGY-19](AGY-19.md) adds native capture, so disabling it in
  the dialog now would be undone there.

Tests:

- New `TestShareSafeAntigravity` (`internal/bundle/bundle_test.go`): a
  share-safe bundle with an Antigravity source and sessions writes and reads
  back. `TestWriterRejectsUnsafeInput` still refuses an unknown agent
  (`evil`).
- `go test ./internal/bundle/`: ok.
- Live, on Linux with agy 1.5.12:
  - `agent-sessions-cli export antigravity 2b9a4c81-… --profile share-safe`
    wrote the bundle;
  - `inspect` read it: share-safe, source antigravity, 2 sessions,
    0 native files, 811 redactions.
- A throwaway test (deleted afterwards) opened that bundle with
  `Service.OpenBundle`. It reported restore not available and handoff
  available. `BuildBundleHandoff` into Claude Code gave a `claude` command,
  and the prompt names Antigravity.
- `golangci-lint run ./internal/bundle/`: 3 errcheck findings, all from
  before this change (`read.go:30`, `read.go:119`, `bundle_test.go:201`).

Not checked:

- Export and import in the desktop app.
