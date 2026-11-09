# macOS terminal Vietnamese input

## Goal

Allow Vietnamese replacement text from OpenKey, EVKey and GoTiengViet in the embedded terminal without losing characters or moving the cursor unexpectedly. Preserve Linux fcitx composition behavior and normal terminal controls.

## Design

Investigate native macOS WebKit keyboard events alongside xterm's input routing and the plain-shell environment. The existing macOS helper assumes replacement events have printable ASCII key codes (48–90); test that boundary rather than broadening it speculatively. Native probes so far forward replacements correctly. The user's latest clarification narrows the failure to a plain shell on a remote host. Local macOS shells and local/remote Claude Code, Codex and OpenCode input work. Investigate SSH environment inheritance, the remote locale and shell line editor before changing frontend input handling. Establish a failing reproduction before changing production code. See [investigation evidence](macos-terminal-input-qa.md).

## Remote locale fix

The user's remote bash reports empty LANG/LC_ALL, POSIX LC_CTYPE and ASCII charmap, with `C.utf8` installed; SSH from iTerm/Terminal works. Add a remote plain-shell bootstrap fallback before shell startup: when LANG, LC_CTYPE and LC_ALL are all empty, discover a UTF-8 name using the remote `locale -a` and export LANG. Never guess a locale name, never override explicit locale settings, and do nothing when discovery fails or no UTF-8 locale exists. Local shell and agent launch paths are unchanged. Preserve native IME handling.

Tests execute the actual bootstrap with controlled locale discovery and a fake login shell, including quoted transport, missing utilities, explicit settings and unsupported hosts. Live remote typing remains a separate verification gate.

## Decisions

- Do not enable the Linux `insertFromComposition` bridge on macOS without event-order evidence: third-party replacement tools and native composition are different paths.
- Preserve Backspace, arrows, shortcuts, native IME preedit, and ordinary Unicode typing.
- Cover replacement routing and complete emitted terminal data, not just string classification.
- Keep native verification separate from unit coverage; synthetic events do not prove every third-party application's settings work.

## Risks

Legacy key codes may conflict with navigation; an overly broad detector could turn named keys into terminal text. Double handling of keydown/keypress/input could duplicate replacement text. macOS event traces may differ across IMEs, output modes and OS versions.

## Release criteria

- Regression reproduces a native input mismatch and verifies the chosen fix.
- Focused input tests, frontend checks/build and project tests run with results recorded.
- Native third-party IME verification and remaining limitations are explicitly recorded; no unperformed manual QA is marked passed.
