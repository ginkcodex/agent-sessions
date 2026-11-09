# macOS terminal input — investigation evidence

## 2026-11-09

Latest user clarification: **only a plain shell on a remote host fails**. Local macOS shells work. Claude Code, Codex and OpenCode input works on both local and remote hosts. The earlier local reproduction attempts below did not exercise the failing target.

### Remote-specific investigation

- `internal/app/panel.go`: local shell launch uses `sys.env()` (login-shell environment); SSH shell launch uses `remote.SSHCommandLine` and its returned environment.
- `internal/remote/ssh.go`: `sshEnvironment` starts from `os.Environ()`, not `term.LoginEnv()`. A Finder-launched app can therefore lack the local terminal's locale variables even when the local shell recovers them. Whether those variables reach the remote shell also depends on SSH client/server configuration.
- This is a concrete environment-path difference, **not proof of the root cause**. Remote locale, shell configuration and cursor movement have not been inspected; no remote connection was made.
- Next diagnostic must run in the failing **remote shell**: `locale`, `locale charmap`, `locale -a`, `$SHELL` and `$TERM`. Choose a UTF-8 locale actually available on that host before trying a clean shell. Do not assume macOS `/bin/zsh` or `en_US.UTF-8` exists remotely.

### Failing remote shell diagnostics (from the user)

Output from the failing Agent Sessions shell tab on the Linux host:

- `SHELL=/bin/bash`, `TERM=xterm-256color`.
- `LANG` and `LANGUAGE` are empty. `LC_CTYPE` through `LC_IDENTIFICATION` are `"POSIX"`, and `LC_ALL` is empty.
- `locale charmap` reports `ANSI_X3.4-1968` (ASCII).
- `locale -a` lists `C`, `C.utf8`, `POSIX` and many `en_*.utf8` locales, including `en_US.utf8`.

SSH from iTerm/Terminal to the same host works. Those clients run with a LANG that ssh forwards (macOS's default `SendEnv LANG LC_*`). The desktop app's SSH environment comes from `os.Environ()`, which may have no LANG, so the remote bash starts in POSIX/ASCII. Readline then treats each UTF-8 byte of the IME's replacement text as a separate character, and its redraw garbles the text and moves the cursor. Agents keep working because their TUIs handle UTF-8 input themselves, without depending on readline's locale.

**Fix (MTI-03):** the remote shell-tab bootstrap sets `LANG` to an installed UTF-8 locale (`C.utf8` on this host) only when LANG, LC_CTYPE and LC_ALL are all unset. Evidence and test results are in [MTI-03](../tasks/macos-terminal-input/MTI-03.md). With a fixed build, the user confirmed on 2026-11-09 that Vietnamese typing in the remote shell works.

User workaround that needs no new build: put `export LANG=C.UTF-8` in the remote `~/.bashrc` or `~/.profile`.

## Existing behavior

- `64beba7` added macOS replacement-string forwarding; `342cd09` added the separate Linux WebKit/fcitx bridge. The Linux bridge does not execute on macOS.
- Installed desktop version: 1.3.4. Installed OpenKey: 2.0.5, Unicode table 0, input type 1, batch replacement, browser recommendation fix enabled.
- Native WKWebView probe using installed xterm 6 and the current replacement helper forwarded `ộng`, `Cộng hòa`, decomposed `à` and single `ộ` intact.
- Native virtual-key replay of VNI `co6ng5 hoa2` through installed OpenKey produced spacer U+202F, DELs and full replacement strings. In a standalone WKWebView/xterm/zsh PTY with `en_US.UTF-8`, the final line was `cộng hòa ` on the expected row.
- The same probe with LANG/LC_CTYPE/LC_ALL absent corrupted text, but did **not** establish the reported first-line jump. `C.UTF-8` and `en_US.UTF-8` both worked on this host. Missing locale remains a hypothesis, not a confirmed root cause.

## Actual desktop app check

Built `go build -tags desktop` to a fresh temporary directory and launched with a temporary HOME/ZDOTDIR. No production code was changed. The original installed app was not terminated or modified.

- Opened plain shells in the real Wails app, ran locale and printed preceding lines.
- Replayed the OpenKey Unicode/spacer/DEL sequence with native keyboard events into only the isolated app PID. The shell displayed `cộng hòa` correctly, with cursor on the prompt line.
- A nested zsh loading the user's startup files with the temporary HOME also displayed the replay correctly. This is not equivalent to the user's full normal shell environment.
- Inspected screenshots of the real app after typing. No first-line jump reproduced.
- App build succeeded with linker warnings about duplicate `-lobjc` and macOS deployment-version mismatch; those warnings are not evidence for the input issue.

## Tests and remaining gaps

- Focused terminal/input tests: 58/58 passed (`node --loader ./test/resolver.mjs --test test/terminal-text-input.test.mjs test/terminal.test.mjs`, from frontend).
- The frontend investigation made no production change. The later remote-locale fix is Go-only; see MTI-03 for its test runs.
- User's exact failing shell environment, actual failure with EVKey/GoTiengViet, and first-line cursor motion are not reproduced.
- Requested `locale`, shell/TERM and a clean UTF-8 `/bin/zsh -f` comparison in the failing terminal before selecting a fix. Do not broaden keyboard interception on this evidence alone.
