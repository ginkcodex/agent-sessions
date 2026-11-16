# MTI-04 — Apple's Vietnamese Telex and VNI in the terminal

- **Status:** 🟡 Partial (fixed and verified in a WKWebView harness; not yet checked in the desktop app, and Telex, CJK preedit and OpenKey were not run)
- **Estimate:** 3 h
- **Depends on:** [KD-02](../keyboard-diagnostics/KD-02.md), [KD-03](../keyboard-diagnostics/KD-03.md) (the recorder used to capture the trace)
- **Plan:** [macos-terminal-input.md](../../plan/macos-terminal-input.md)

## Scope

The user reported on 2026-11-16 that macOS's own Vietnamese input sources
(Telex and VNI) do not type Vietnamese in the built-in terminal. This is not
the OpenKey path, which sends synthetic replacement keys and is handled by
`terminalReplacementText`. Apple's input methods go through the text input
system instead: marked text (composition events), or a replacement of
earlier text. On macOS the terminal leaves composition to xterm's own
handling; the WebKitGTK commit bridge (`installTerminalCompositionText`) runs
on Linux only.

The work:

1. Capture a trace with Record Keyboard Events while typing with Telex in a
   terminal tab, and also in a page field where Telex works, for comparison.
2. From the trace, find the path Apple's input method uses (composition
   events, `insertReplacementText`, or a replacement range over text that
   xterm has already cleared from its helper textarea) and where xterm loses
   the result.
3. Fix that path and add regression tests. Leave the OpenKey replacement
   path, Linux fcitx and native IME preedit unchanged.

Likely files: `frontend/src/lib/terminalTextInput.ts`,
`frontend/src/lib/components/terminal/TerminalView.svelte`,
`frontend/test/terminal-text-input.test.mjs`.

## Acceptance criteria and tests

- With Telex and with VNI, typing `Việt Nam` in a local plain shell gives
  exactly that text, with nothing duplicated or left behind.
- OpenKey replacement, Japanese or Chinese IME preedit, and plain typing
  still work.
- The fix is backed by unit tests that replay the recorded event order.
  Native typing is checked in the desktop app.

## Evidence

- **Cause.** Instead of a recorder trace, a small Swift harness (a WKWebView
  that posts key events to its own process through the selected VNI input
  method) gave the event order directly:
  - In a visible textarea, VNI sends `insertText "c"`, then
    `insertReplacementText` "co", "con", "cong", "công", each before a
    keyCode 229 keydown. Stock xterm 6 drops the replacements; its keydown-229
    diff sees no change because WebKit already applied them.
  - With real xterm in the harness and the helper textarea sized to one cell
    at the cursor, as xterm does after rendering, typing
    `cong65 hoa2 xa4 nam ` gave `c h x n `: the reported symptom.
  - With the textarea zero-sized, VNI uses marked text and stock xterm gives
    `cộng hoà xã nam `. Bisecting xterm.css's rule on a plain textarea:
    the input method uses marked text only when the box has no size, padding
    or border. Opacity, position, overflow, white-space and box-sizing make
    no difference, and neither do user-select, refocusing or xterm's
    listeners.
- **Fix.** `TerminalView.svelte` marks the host `mac` on macOS, and a rule
  keeps `.xterm-helper-textarea` at `width: 0; height: 0` (`!important`,
  over xterm's inline cell size). xterm.css already zeroes padding and
  border.
- **Rejected.** A bridge that turned `insertReplacementText` into DELs plus
  the changed suffix. In the marked text path, the input method also inserts
  a space after xterm has sent it on keydown, so the bridge doubled every
  space.
- **Harness results with the fix** (the compiled CSS rule, real xterm 6, echo
  on, VNI):
  - `cong65 hoa2 xa4 hoi65 chu3 nghia4 viet65 nam ` gives
    `cộng hoà xã hội chủ nghĩa việt nam `, with and without xterm's cell
    sizing.
  - With arrows, `viet65 nam` ← ← `a ` sends `việt nam`, two left arrows,
    then `a ` (repeated 3×).
  - With Backspace in a word, `cog⌫ng6 a ` gives `cong6 a `, the same as
    VNI in a plain textarea, where the input method starts a new segment
    after Backspace.
- **Tests.**
  - `test/terminal-text-input.test.mjs` adds a test that pins the macOS rule,
    the host class, and xterm.css's zero padding and border. Node cannot run
    the input method, so the event-order regression lives in the harness, not
    in a unit test.
  - `npm test`: 1523 tests, 1522 pass, 1 skipped, 0 fail.
  - `npm run check`: 0 errors. `npm run build` passes, and the built CSS
    contains the rule.
- **Not checked.**
  - Typing in the desktop app.
  - Telex: only VNI is enabled on this Mac.
  - Japanese or Chinese IME candidate window placement.
  - OpenKey after the change (its path is keydown-based and does not read
    the textarea's size).
  - Remote shells.
