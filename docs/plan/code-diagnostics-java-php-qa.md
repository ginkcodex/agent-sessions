# Java/PHP code intelligence — QA record

Date: 2026-11-09. Implementation is not yet release-verified.

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
  passed after all Java adapter lint corrections.

Earlier regression attempts caught old Ruby-only provider-count fixtures and
Settings language-list expectations; those fixtures were updated and reruns
passed. A transient Java signature mismatch while edits were still running was
also corrected before final adapter tests.

## Real-server execution is blocked pending user approval

The session permission gate denied the attempted Intelephense smoke command
because it executes downloaded external server code. No workaround was used.
Neither downloaded server has been run. Approval is needed to execute:

- Intelephense 1.18.5 in an isolated `/tmp/as-php-smoke.*` folder, using native
  Node.js and a temporary PHP source project.
- Eclipse JDT LS 1.64.0 snapshot (2026-11-06) in an isolated
  `/tmp/as-jdtls-qa.*` folder, using native Java 25 and a temporary Java project.

`internal/codediagnostics/java_php_real_test.go` provides opt-in checks using
`CD_PHP_SMOKE_PATH` / `CD_JAVA_SMOKE_PATH`. These exercise actual approved session
startup, an unsaved syntax error and source definition resolution. They download
nothing and install nothing. The test uses a temporary HOME and Java cache.

Before release, confirm real JDT initialization with the existing 10-second
startup limit, any required safe LSP request handling, Java opened-file errors
with autobuild disabled, and Intelephense settings/definition behavior. Synthetic
fixtures do not establish those compatibility points.

## Manual gates not run

- Desktop settings, missing-tool hints, approval, F12/Ctrl-click, Problems panel,
  draft updates, close/reopen and failure/recovery.
- SSH/WSL rebuilt-server deployment and host-local runtime/tool lookup.
- macOS native runtime/distribution layouts; Windows through WSL/SSH.
- Maven/Gradle multi-module dependency import and real Composer autoload projects.
- Java virtual/archive definition targets are deliberately unsupported; ensure
  unsupported answers never create external-file read grants.
