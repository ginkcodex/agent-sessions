# MTI-02 — Verify native Vietnamese typing

- **Status:** 🟡 Partial — user confirmed remote-shell typing with the MTI-03 build; EVKey, GoTiengViet, Linux fcitx and Windows remain unverified
- **Estimate:** 1 h
- **Depends on:** MTI-01
- **Plan:** [macos-terminal-input.md](../../plan/macos-terminal-input.md)

## Scope

Native manual verification with third-party Vietnamese input applications and checks against Linux/Windows input regressions.

## Likely files

- This task's evidence and group/index progress records.

## Acceptance criteria and tests

- With OpenKey, EVKey and GoTiengViet where available, type `Cộng hòa`, `Tiếng Việt`, accents, punctuation, repeated spaces, Backspace and Enter below several terminal output lines. Check both emitted text and cursor position.
- Verify ordinary English, arrows/Home, shortcuts, native macOS composition and split-pane focus.
- Recheck Linux fcitx commits and Windows ordinary typing on native platforms when available.
- Record app/settings and actual tested platforms. Unavailable platforms/tools remain explicitly unverified.

## Evidence

- [Native investigation record](../../plan/macos-terminal-input-qa.md) records OpenKey 2.0.5 input events in native WebKit/xterm and native replacement replay in the actual isolated desktop app.
- Correct text/cursor observed in a clean UTF-8 zsh shell and a partially configured nested shell. This does not reproduce or resolve the user's failing new shell.
- 2026-11-09: after the MTI-03 fix, the user reported that Vietnamese typing in a plain shell on the failing remote host "works well now". The user did not say which IME, nor whether they checked the full acceptance list (Backspace, Enter, shortcuts, split panes).
- EVKey, GoTiengViet, Linux fcitx and Windows native runtime checks not performed; no complete manual QA pass claimed.
