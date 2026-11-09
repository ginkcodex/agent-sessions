# MTI-03 — Default unconfigured remote shells to installed UTF-8

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** None
- **Plan:** [macos-terminal-input.md](../../plan/macos-terminal-input.md)

## Scope

The user's remote bash reports empty LANG/LC_ALL and ASCII POSIX LC_CTYPE, while SSH from iTerm/Terminal works. Add a remote plain-shell-only UTF-8 fallback discovered on the host before interactive startup. Do not override explicit LANG, LC_CTYPE or LC_ALL, or change local/agent launches.

## Likely files

- `internal/launch/panel.go`
- `internal/launch/panel_locale.go`
- `internal/launch/panel_locale_test.go`

## Acceptance criteria and tests

- Prefer installed C UTF-8 locale; otherwise use an installed UTF-8 locale; never invent a name.
- Explicit locale selections, missing locale command, discovery errors and no UTF-8 locales leave the environment unchanged.
- Execute the production remote bootstrap with controlled discovery and verify locale in the launched shell, including quoted SSH transport.
- Run launch tests, race suite, lint, Windows cross-compile/vet and whitespace checks. Record skipped native checks honestly.

## Evidence

Done:

- `internal/launch/panel_locale.go` adds `panelRemoteLocale`. It runs only when LANG, LC_CTYPE and LC_ALL are all unset or empty. It reads `LC_ALL=C locale -a` on the host and picks C.UTF-8/C.utf8 first, then en_US.\*, then the first other UTF-8 locale. It exports LANG only when one was found.
- `PanelShellScript` (WSL and SSH shell tabs) prepends the fallback. Local `PanelShellArgv` and the agent scripts are unchanged.

Verified on macOS (2026-11-09):

- `TestPanelRemoteLocale` covers 13 cases, each run both raw and through `JoinCommand(PosixBootstrapArgv(...))`:
  - The user's reported host list selects `C.utf8`.
  - Hyphenated C, English and other UTF-8 fallbacks are selected as designed.
  - No UTF-8 locale, a failing discovery and a missing `locale` command leave the environment unchanged.
  - Explicit `LANG=vi_VN.UTF-8`, `LANG=C`, `LC_CTYPE=POSIX` and `LC_ALL=C` are left untouched.
  - Empty values and an unrelated `LC_TIME` still get the fallback.
  - All 13 pass.
- `TestPanelRemoteLocaleWithInstalledLocales` runs the bootstrap with this Mac's real `locale -a` and no locale variables. The launched shell reports a UTF-8 charmap: pass.
- `TestPanelLocaleOnlyRemotePlainShell` confirms the fallback is absent from local shells and agent scripts: pass.
- `go test -race ./internal/launch` passed, including the existing `TestPanelShellScriptThroughSSH`. The full `go test -race ./...` (`make test`) passed, but that run started before the real-locale test was added.
- `go tool golangci-lint run --new-from-rev=HEAD ./internal/launch`: 0 issues. gofumpt applied.
- `GOOS=windows go vet ./internal/launch` passed. `git diff --check` is clean.

Verified by the user (2026-11-09):

- With a build containing this fix, Vietnamese typing in a plain shell on the previously failing remote host "works well now". The user did not say which IME they used. The original report and the installed app point to OpenKey.

Not checked:

- EVKey and GoTiengViet (tracked in MTI-02).
- Running against a real Linux `locale -a`. The host list was replayed through a stub, and Docker was unavailable.
- Remote startup files that set locale themselves. The fallback runs before the login shell, so the user's files still win.
- WSL shell tabs share this script, but nothing was run natively on Windows. The cross-vet is compile-only.
