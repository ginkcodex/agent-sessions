# Java/PHP code intelligence — QA record

Date: 2026-11-09. Automated regression and Linux real-server smoke checks
passed. Manual desktop, remote and cross-platform release gates remain unrun.

## Automated results

- PHP adapter synthetic race tests: passed, 5 top-level tests / 19 subtests.
- Java adapter synthetic race tests: passed, 7 top-level tests, no skips.
- Java/PHP integration tests: passed, nested independent builds, grouped Java
  modules, Composer children, markerless fallback, disabled-language lookup
  suppression, push ownership and app-observed saved-file routing.
- `make test` (`go test -race ./...`): passed across all packages. Real-server
  smoke tests remain opt-in and were skipped without their environment flags.
- Frontend tests: 1152 tests, 1151 passed, 1 skipped, 0 failed.
- `npm --prefix frontend run check`: passed.
- `npm --prefix frontend run build`: passed.
- `make lint`: failed on repository-wide pre-existing errcheck/errorlint/style
  findings in unrelated files, plus findings in the new Java files. New Java
  error handling/formatting was corrected and changed-code lint was rerun.
  `go tool golangci-lint run --new-from-rev=HEAD~2`: passed, 0 issues across
  this feature's commits and working changes (run before JP-03 commit).
- Final `go test -race ./internal/codediagnostics ./internal/engine -count=1`:
  passed again after the real-server harness correction (87.767 s / 224.091 s).
- Final `go tool golangci-lint run --new-from-rev=HEAD~3`: passed, 0 issues
  across all three implementation commits plus JP-04 working changes.

Earlier regression attempts caught old Ruby-only provider-count fixtures and
Settings language-list expectations; those fixtures were updated and reruns
passed. A transient Java signature mismatch while edits were still running was
also corrected before final adapter tests.

## Real-server smoke results

Following explicit user approval, both actual servers passed through the normal
fingerprinted approval/session/document/navigation route on Linux:

- Intelephense 1.18.5, native Node.js 24: initialized in 0.445 s (ordinary run)
  and 1.007 s (race run).
- Eclipse JDT LS 1.64.0 snapshot (2026-11-06), native Java 25: initialized in
  2.117 s (ordinary run) and 7.829 s (race run), within the unchanged 10-second
  initialization timeout. Read-only shared configuration and separate cache
  worked; no additional server-request support or production changes were needed.

The first user-run attempt failed before launch because the smoke test reused a
synthetic manager helper that expected nonexistent `server` and `cfg` fixture
files. Corrected the helper to accept the actual discovered tool identities;
no approval validation was bypassed or weakened.

The strengthened smoke checks passed with `go test -race -v
./internal/codediagnostics -run '^TestJavaPHPRealSmoke$' -count=1 -timeout 4m`
with both `CD_PHP_SMOKE_PATH` and `CD_JAVA_SMOKE_PATH` set. For each server:

1. Open an unsaved syntax error and receive owned document diagnostics.
2. Resolve `greet()` to the exact in-project definition range.
3. Correct the draft, observe a published empty diagnostic set, and resolve again.
4. Reintroduce the error, receive diagnostics again, and resolve again.
5. Confirm draft changes did not modify the saved source file.

Java opened-document diagnostics worked with autobuild disabled. JDT emitted
an outside-project/unverifiable-diagnostic notice; the bridge kept those findings
out of the owned document results. Larger projects, slower hosts and cold build
imports may still exceed the startup budget; these small projects do not establish
performance for all builds.

Tests download/install nothing, use temporary projects/HOME/Java cache, and close
sessions before their temporary directories are removed. Feature-owned downloaded
archives, extracted servers, metadata/docs and scratch logs were removed after
verification; installed host runtimes were left untouched.

## Manual gates not run

- Desktop settings, missing-tool hints, approval, F12/Ctrl-click, Problems panel,
  draft updates, close/reopen and failure/recovery.
- SSH/WSL rebuilt-server deployment and host-local runtime/tool lookup.
- macOS native runtime/distribution layouts; Windows through WSL/SSH.
- Maven/Gradle multi-module dependency import and real Composer autoload projects.
- Java virtual/archive definition targets are deliberately unsupported; ensure
  unsupported answers never create external-file read grants.
