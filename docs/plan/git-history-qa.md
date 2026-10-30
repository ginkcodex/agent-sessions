# Git history QA — 2026-10-26

Branch: `feature/git-history`. Implementation GH-01…GH-17 is complete.
**GH-18 remains blocked/pending**: native UI interaction has not been verified.

## Actual results

| Gate / route | Result | Evidence / limitation |
|---|---|---|
| macOS backend, native Go | Pass | `go test -race ./...`, exit 0 |
| Focused history race/repetition | Pass | `go test -race -count=3 ./internal/files ./internal/engine ./internal/app ./internal/rpc -run History`, exit 0 |
| Frontend automated | Pass | 814 tests; check 0 errors; production build succeeds |
| Windows compile/static | Pass | `GOOS=windows go vet ./internal/files ./internal/engine ./internal/app ./internal/rpc ./cmd/...`; not Windows execution |
| Docker SSH integration | Pass after environment adjustment | `TMPDIR=/tmp AGENT_SESSIONS_SSH_DOCKER=1 go test -race -count=1 -run SSHD ./internal/remote/... ./internal/app/...`; initial normal TMPDIR failed due to OpenSSH ControlPath >=104 bytes on macOS |
| Repository-wide lint | Fails | Existing errcheck/gocritic/gofumpt/staticcheck/unparam/unused findings outside history; all reported `git_history` findings fixed and focused tests rerun |
| macOS desktop compile | Pass after link adjustment | Standalone `desktop,production` build needs `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` on this SDK; initial direct build failed on `_OBJC_CLASS_$_UTType`. Version 0.9.32; no installed app/build output overwritten |
| macOS native desktop UI | Pending / blocked | QA executable launched, but no computer-use tools are available in this conversation. No screenshot, click, focus or layout assertion is claimed |
| Windows / WSL native UI | Untested | No Windows/WSL environment available |
| SSH history UI / real remote host | Untested | Docker SSH lifecycle tests are not a history WebView walkthrough or real-host QA |

Warnings: existing tsconfig `ignoreDeprecations` warning and production chunk-size
warning; native build emits Wails/AppKit deprecation and duplicate library warnings.

## Native user feedback and usability fixes — 2026-10-26

The user confirmed History works and supplied a macOS screenshot, while reporting
small text, an unscrollable bottom diff, and difficulty reaching sparse changes.
This is user smoke-test evidence, not completion of the acceptance matrix.

Follow-up changes enlarge History text to a rem-based scale, allow the shared
viewer to shrink within the bottom flex pane, and add Previous/Next change-group
navigation in source diffs. Navigation includes removed-only rows, wraps at either
end, and does not automatically jump on open. The viewer header stays fixed while
the code area owns scrolling.

Validation: 815 frontend tests pass, Svelte check has zero errors (existing
`ignoreDeprecations` warning), and production build succeeds (existing chunk-size
warning). New tests cover displayed-row grouping, deletion-only/EOF changes,
stepping/wraparound, historical navigation controls, and special-content states.
Native scrolling, font legibility, and button-driven reveal after these fixes
still need interaction verification; automated SSR does not establish those.

## Isolated fixture and launch details

Temporary paths (not repository content):

- `/private/tmp/gh18/repo`: 84 commits, merge/root/rename/deletion/binary/large
  changes, local and fetched-remote refs, dirty README.
- `/private/tmp/gh18/repo/sub`: scoped workspace.
- `/private/tmp/gh18/multi`: non-repository parent with `a`, `b`, and plain child.
- `/private/tmp/gh18-home`: isolated HOME and Claude session fixtures for all
  three workspaces. No real sessions, preferences or Git configuration edited.
- `/private/tmp/gh18-bin/agent-sessions`: version 0.9.32 QA binary. Built after
  GH-17 security fixes, before the subsequent lint-only cleanup. Logs beside it.

Two launch attempts produced QA processes with cwd `/private/tmp/gh18-bin`
(observed PIDs 38235 and 38284). Stopping these was denied by the execution
permission layer; cleanup was not retried through another mechanism. A later
check found no QA process still running. No desktop window was inspected, and
process presence was never proof of a usable UI. The temporary fixtures were
kept for the remaining native checklist.

## Remaining native acceptance checklist

- Open each fixture workspace's Go to File → History.
- Multi-repo picker must require a choice; scoped repo must label its scope.
- Browse HEAD/local/remote refs without changing checkout; compare HEAD/index/
  worktree before and after (automated no-write snapshots already pass).
- Search by subject/author/OID; exercise debounce, paging and refresh.
- Select root/merge/rename/deletion/binary/large entries; inspect details and
  first-parent/root comparison labels; copy hash.
- Keep two revisions beside a dirty working draft; switch all four modes;
  close/reopen bottom diff and verify working draft/selection is unchanged.
- Drag and keyboard-resize splitters, resize through 850/580 px, scroll each
  pane, check branch fallback selector, tree keys, focus and dialog shortcuts.
- Reopen/focus the dialog; disconnect/reconnect while pages/details/diffs load;
  ensure no stale remote/local mixing or request storms.
- Repeat applicable cases on native Windows, WSL and an authorized SSH host.

Use `/run-skill-generator` to capture a verified macOS launch/driver recipe once
interactive desktop tooling is available; this session established compilation
and process launch only, not a verified cold-start UI workflow.
