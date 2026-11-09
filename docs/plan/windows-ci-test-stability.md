# Windows CI test stability

## Goal

Remove two timing assumptions behind `windows-test-build` failures in run 42639054985 (v1.3.8), without relaxing production resource limits or changing watcher behavior.

## Design and decisions

- Prepare all nested Git repositories before starting the scope under test. Give the call-cap test its own generous deadline so it tests the 16-call cap, not Windows process startup speed. Test an already expired deadline separately.
- In the fake filesystem watcher, signal readiness when the watcher first requests its event channel. This happens after both startup scans and initial publication. Wait for that barrier before modifying Ruby files; adding the root watch alone is too early.
- Keep the production five-second Git deadline, initial scan suppression, and size/mtime change detection unchanged.

## Risks

- Cross-vet and macOS tests do not prove native Windows behavior. Record that limit explicitly; the next Windows CI run remains the native confirmation.
- Readiness must use synchronization, not sleeps or unsynchronized reads of watcher fields.

## Release criteria

- Git call-cap and expired-deadline regressions pass.
- Watcher readiness is exercised with delayed startup and Ruby saved-file transitions still reach ruby-lsp.
- Repeated race-enabled targeted tests, the diagnostics package suite, and Windows cross-vet pass locally. Record any checks not run.

## Tasks

See [tasks](../tasks/windows-ci-test-stability/README.md).
