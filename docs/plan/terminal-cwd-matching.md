# Terminal cwd matching

## Goal

Keep the manually selected shell when it currently belongs to the selected session's workspace, even when it was launched elsewhere and reached that workspace with `cd`.

## Design and decisions

- For shell panes, workspace matching uses `tab.cwd || tab.dir`: reported current directory first, launch directory only until cwd is known.
- Keep the original launch directory intact. Reuse existing cwd detection; add no polling or backend changes.
- Agent panes keep exact-session/parent-session identity and launch-directory workspace matching.
- Preserve existing ranking, same-host/running guards, path normalization, equal-rank active preservation, hidden-panel behavior and no-focus-stealing behavior.
- A shell that reports moving away no longer matches its launch workspace.

## Risks

Shells without cwd reporting retain launch-directory behavior. Reported cwd can remain at the last prompt while a command runs; this change does not improve detection. Existing tests deliberately asserting launch-only shell matching must change.

## Release criteria

Regression tests cover an active shell reached by `cd` alongside another shell launched in the workspace, moving away, launch fallback, agent stability, normalization and split matching. Terminal tests, frontend checks and the full test suite pass. Live desktop verification is reported separately, not inferred from automated tests.
