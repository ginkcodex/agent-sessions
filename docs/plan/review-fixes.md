# Review fixes (architecture and feature review, 2026-11-14)

- **Tasks:** [review-fixes](../tasks/review-fixes/README.md)

## Goal

Fix what the 2026-11-14 review of `develop` at `8cdaeee` found: security
gaps first, then reliability bugs, then CI gaps and stale task statuses.

The review used six read-only reviewers in parallel, one per area: the
session data pipeline, the app, RPC and remote layer, ACP chat, code
diagnostics and files, the frontend, and build and release. The most serious
findings were then checked against the code. Each task says whether its
finding was checked or comes from a reviewer only. A reviewer-only finding
is confirmed or dropped when its task starts.

Baseline at review time:

- `go vet ./...`: clean.
- `go test ./...`: 48 packages pass.
- Frontend: `npm run check` reports 0 errors; `npm test` passes 1468 of
  1469 tests, with 1 skipped.
- `npm audit --omit=dev`: 0 issues.
- Reviewers ran `go test -race` on their packages; it passed everywhere.
- Not available locally: `golangci-lint` and `govulncheck`.

## Design

### Release and supply chain (RF-01 to RF-03, RF-08)

- **RF-01:** the `release` environment holds `MINISIGN_KEY`,
  `MINISIGN_PASSWORD` and `RPM_SIGNING_KEY`. Before RF-01 it had no
  protection rules and no branch policy. Any `workflow_dispatch` run therefore
  got the keys, and it ran the workflow file and `cmd/release-sign` from the
  ref that was chosen; two runs on `develop` did so on 2026-11-13. The
  minisign key is the self-updater's root of trust
  (`internal/update/keys.go`).
- **RF-02:** the apt `.deb` cache is restored by a predictable prefix and its
  packages are installed as root without a signature check, in the jobs that
  hold the keys.
- **RF-03:** actions are pinned to moving tags.
- **RF-08:** the website Worker and the AUR job trust the unsigned
  `SHA256SUMS`, so the minisign check that protects the desktop updater does
  not protect Linux users.

### Untrusted content reaching a program (RF-04 to RF-07, RF-10)

- **RF-04:** `git blame` runs without `--no-textconv`
  (`internal/files/blame.go:87`), so a repository can make it run a program.
  The submodule diff has the same gap. `git status` filters are already
  [IS-06](../tasks/code-diagnostics-ignore-scope/IS-06.md).
- **RF-05:** OSC 52 clipboard writes from terminal output are accepted
  unfiltered (`frontend/src/lib/terminalClipboard.ts`), and xterm's `paste()`
  passes ESC through. A later paste can end bracketed paste early.
- **RF-06:** a dropped file name on Windows is wrapped in double quotes
  (`internal/app/panel.go`, `typedPath`), and PowerShell expands `$(…)`
  inside double quotes.
- **RF-07:** every public `App` method can be called from the webview,
  including ones that read any path and the ones that switch on deletes.
  SPEC §9 forbids reading credential files.
- **RF-10:** `openURL` in `api.ts` ignores Go's refusal of a scheme and
  falls back to openers that don't check it. Translate base URLs, the update
  URL override and the repository ingest site accept `http://`.

### Privacy (RF-09)

Share-safe bundles leave the manifest's `CWD`, `RepoRoot`, `GitBranch` and
`Title` unredacted (`internal/engine/export.go`). `redact.JSONBytes` never
sees a JSON key next to its value, and the home-path rules miss Windows
profiles.

### Reliability (RF-11 to RF-23, RF-26, RF-27)

| Task | Finding | Checked |
|---|---|---|
| RF-11 | RPC replies are matched by `%v` of a float64; from ID 1,000,000 that prints `1e+06` and no call gets its answer (`internal/rpc/client.go:267`) | ✅ |
| RF-12 | A missing watch root falls back to `$HOME`. New directories there are then watched recursively, and each change queues a full rescan (`internal/watch/watch.go:390`) | ✅ |
| RF-13 | One fsnotify error moves every provider to polling for the rest of the process | reviewer |
| RF-14 | Watcher refreshes queue without coalescing; `Close` races background readers of `d.db` | reviewer |
| RF-15 | One unreadable Codex rollout blocks all Codex indexing; an unreadable Claude project drops its sessions until the next scan | reviewer |
| RF-16 | When the ACP connection fails (frame over 64 MiB, read error) the agent is not killed, so the chat never ends (`internal/acp/conn.go:264`) | ✅ |
| RF-17 | `ChatStart` has no timeout and can't be cancelled; a hung start blocks every other chat open in the window | ✅ (context) |
| RF-18 | A question still open when the prompt returns holds back `turn_end`; the store and its test assume the opposite | reviewer |
| RF-19 | Reading a FIFO opens it before the regular-file check and hangs (`internal/files/files.go:615`) | ✅ |
| RF-20 | A crashed language server can't restart before the app does; `SessionManager.Retry` has no production caller | reviewer |
| RF-21 | Remote probe, deploy and serve run POSIX scripts under `$SHELL -lc`, which fails with fish, csh and tcsh | ✅ |
| RF-22 | Port forwards use `ForkAfterAuthentication=no`, which needs OpenSSH 8.7 | reviewer |
| RF-23 | `configfile` values are not unescaped on read, so backslashes grow on every save; a line over 64 KiB truncates the rewrite | reviewer |
| RF-26 | Agent panel tabs on a csh or tcsh host run `$SHELL -ilc`, which those shells reject (`internal/launch/panel_shell.go`) | ✅ (found in RF-21) |
| RF-27 | The fish cases of the panel startup test run without a terminal, so fish never prompts and they fail wherever fish is installed | ✅ (found in RF-26) |

### CI and docs (RF-24, RF-25)

- **RF-24:** `golangci-lint` is required by the Definition of Done but not
  run in CI (156 issues locally, 6 of them `nilerr`). Neither `govulncheck`
  nor `npm audit` runs in CI.
- **RF-25:** some task statuses are out of date: AUR-07 and LR-19 say their
  workflows never ran, but both succeeded in v1.5.8. The code-diagnostics
  progress line also miscounts.

## Decisions

- **RF-01, chosen by the user on 2026-11-14:**
  - The `release` environment allows only the `main` branch and `v*` tags.
    Manual dry-runs run only on `main`. A dispatch on any other branch builds
    everything, then the artifacts job fails because the environment refuses
    the branch. The workflow jobs are unchanged.
  - No required reviewer: one maintainer, and `main` already needs a pull
    request.
  - The tag ruleset "Protect release tags" stops anyone but an admin from
    moving or deleting a `v*` tag.
    - Restricting tag **creation** is not possible here. A personal
      repository can't list the GitHub Actions app as a bypass actor
      (`422`: "Actor GitHub Actions integration must be part of the ruleset
      source or owner organization").
    - The release job creates the tag with `gh release create --target`, so
      a creation rule would block every automatic release.
    - Only collaborators with write access can create tags, which today is
      the owner only.
- Pre-release tags from `develop` still get the keys. This is the
  documented pre-release path, and pushing a tag needs write access.

## Risks

- RF-01: if the next release fails at "Branch … is not allowed to deploy",
  the policy is wrong. Check the environment's branch policies before
  re-running.
- RF-02 and RF-03 change CI only; a bad pin or cache change fails a release
  run, not users.
- RF-07 can break a legitimate drop or attachment of a file that matches
  the denylist; the error must say why.

## Release criteria

- Every RF task is ✅, or ➖ with a reason.
- After RF-01 the next release publishes, and a `develop` dispatch is
  refused at the artifacts job.

## Proposals, not planned

These came out of the review as architecture observations. They need a
decision before they become plans of their own:

- Split `internal/engine` (173 `Engine` methods) and `internal/app`
  (174 public methods) along session pipeline versus workspace and IDE
  features.
- Generate the RPC client, server dispatch and TS types from the `Backend`
  interface. Today one backend method is written in about six places, and the
  contract test checks names only.
- One shared JSONL-source scanner for Claude and Codex; one row per source
  for scan state instead of one JSON blob per provider.
- Chat state snapshots or a resync after `chatList`, so the frontend store
  stops rebuilding backend state.
- Subpackages for `internal/codediagnostics` (trust, LSP, adapters), and one
  table for the node-hosted language servers.
- Re-ask for approval when a code-diagnostics provider, or a project-local
  executable, appears after the first grant.
