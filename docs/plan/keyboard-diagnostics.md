# Keyboard diagnostics

## Goal

Find out why the macOS app sometimes stops taking Vietnamese input from
OpenKey. When it happens, typing `aa` gives `a` instead of `â`, in every field
of every window, including a newly opened window. Other apps work at the same
time. `dd` still gives `đ`. Quitting the app clears the fault, and so does
switching to another app and back. The fault comes back now and then, and
nothing found so far explains it (see "Ruled out").

The plan adds an opt-in flight recorder for keyboard events. While it is on,
the app keeps the most recent native (AppKit) and page (DOM) keyboard events
in memory. When the fault shows up, the user saves the log, and the log shows
where OpenKey's replacement events get lost.

## What a log must answer

OpenKey swallows the second `a` and posts synthetic events instead: Backspace,
then `â` as a Unicode keystroke with key code 0. In its browser mode it also
sends a U+202F spacer first (see
[macos-terminal-input-qa.md](macos-terminal-input-qa.md)). When the fault is
present, the trace should show which of these happens:

1. The synthetic events never reach the app. The native monitor sees `a` and
   nothing after it.
2. They reach AppKit, but the page never gets them. The native monitor sees
   Backspace and `â`, but there is no matching `keydown`, `beforeinput` or
   `input` in the page.
3. They reach the page, but it handles them differently. For example, it may
   still be in a composition, or `isComposing` is set, or the `inputType` is
   different.
4. Something in the app's text-input state is different, for example a
   leftover marked text, a different selected input source on the window's
   `NSTextInputContext`, or a different first responder.

Each native record carries the event's source process ID, source state ID and
user-data field, so OpenKey's posts can be told apart from hardware keys.

## Design

### Recorder (Go, `internal/keylog`)

- A ring buffer of the last 20,000 events, shared by all windows of the
  process. It is off at launch and kept in memory only. Nothing reaches disk
  until the user saves.
- Turning recording on clears the buffer. Turning it off keeps the buffer, so
  the user can still save the log afterwards.
- Each event has a wall-clock time, a source (`native`, `web` or `mark`), the
  window, and an opaque JSON payload.
- Saving writes JSON Lines, ordered by time. The first line is a header with the
  app version, OS, recording start, save time, event and drop counts, and a
  native state snapshot taken at save time.

### Native monitor (macOS, `cmd/agent-sessions/keylog_darwin.m`)

- An `NSEvent` local monitor for keyDown, keyUp and flagsChanged. It only
  reads events and always returns the event unchanged. While recording is
  off, it returns at once.
- For each event it records:
  - the event type, key code, `characters` and `charactersIgnoringModifiers`,
    each with its code points (so DEL, Backspace and U+202F are visible);
  - the modifiers and whether the key repeats;
  - the window number and whether that window is the key window;
  - the first responder's class and its `hasMarkedText`;
  - the `NSTextInputContext`'s selected input source, and the TIS current
    input source;
  - from the `CGEvent`: the source PID, source state ID, user data, key code,
    flags and Unicode string.
- While Secure Event Input is on, it records the key code and flags, but no
  characters.
- Markers: the app becoming active or inactive, windows becoming or ceasing to
  be key, `NSTextInputContext` keyboard selection changes, and the system
  input source changing (TIS distributed notification).

### Page capture (frontend, `keyboardDiagnostics.ts`)

- While recording is on, window capture-phase listeners record keydown, keyup,
  keypress, beforeinput, input and composition events, plus window
  focus/blur and visibility changes.
- Each page record has:
  - the event time (`performance.timeOrigin + timeStamp`);
  - the type, `key`, `code`, `keyCode` and `repeat`;
  - `isComposing`, the modifiers, `data` and `inputType`;
  - a short description of the target (tag, type and first class);
  - `document.hasFocus()`.
- It never reads field values. Inside password inputs, `key` and `data` are
  dropped.
- Records are sent to Go in batches every 250 ms through
  `App.RecordKeyboardEvents`. The page learns the recorder's state from
  `App.KeyboardLogEnabled` at startup and from the `keyboard-log:state` event
  afterwards. When recording is off, no listeners are attached.

### Controls (macOS Help menu)

- **Record Keyboard Events** is a checkbox that turns recording on and off.
- **Save Keyboard Log** writes
  `~/Library/Logs/agent-sessions/keyboard-<time>.jsonl` and shows the path. The
  file is created with mode 0600.
- These two items replace the toolkit's default Help → Learn More item. That
  item navigated the current app window to wails.io.

## Decisions

- Use a flight recorder rather than a timed capture. The fault is
  intermittent and cannot be triggered on demand. The user turns recording on
  once per launch, works normally, and saves the log when the fault shows up.
- Monitor inside the app, not system-wide. An event tap in another process
  needs Accessibility or Input Monitoring permission and records every app.
  The in-app monitor sees only what reaches this process, which is the
  boundary in question.
- Do not change input handling in this feature. The fix follows from the
  evidence, in its own task.
- Native recording is macOS only. Page capture is plain DOM code and runs in
  every desktop build, but the menu that turns it on exists only on macOS.

## Privacy

The log contains the keys typed while recording is on, which can include
secrets typed into terminals. Recording is off at every launch and only the
menu turns it on. The buffer is in memory, and the saved file is private to
the user (0600). Secure Event Input and password fields are not recorded. The
user reviews the file before sharing it.

## Risks

- The monitor runs on the main thread for every key. It must stay cheap and
  must never raise. `characters` raises on flagsChanged events, so it is read
  only for key events.
- If the fault comes from the event never reaching AppKit, the in-app log can
  only show that the event is missing, not why. That would point to OpenKey's
  posting path or the window server. The next step would then be a
  system-wide tap, run with the user's permission.
- Watching may change what is watched. Opening a menu does not deactivate the
  app, so saving from the Help menu should keep the fault. The buffer already
  holds the events from before the save either way.

## Ruled out so far

From the 2026-11-16 live session:

- Secure Event Input; OpenKey's event tap being disabled; OpenKey's per-app
  mode (the app is set to Vietnamese).
- The page's own key handlers (they only act on Escape) and the native Cmd+`
  monitor (it passes every other key).
- The Press-and-Hold popup; terminal output load; Apple's Vietnamese input
  method (only ABC is enabled).

## Finding from the first fault log

The log saved on 2026-11-16 at 23:23 (app 1.6.9, chat composer, ABC selected)
matches none of the four cases above. OpenKey's events reach AppKit in the
right order, and the page gets all of them, but the page gets them in a
different order:

- Natively, each word is OpenKey's spacer, N Backspaces, then the
  replacement, for example `ông` as one key code 0 event.
- In the page, the Backspace keyups arrive first, the replacement's keydown
  and `insertText` come next, and some or all Backspace keydowns come after
  it. Their `deleteContentBackward` then removes the text just inserted.
  `cong` plus `6` becomes `cong ông` minus four characters, which is `cong`.
  In the same way, `aa` becomes `a` + `â` minus one, which is `a`.
- 18 of the 19 replacements in the log had Backspace keydowns after the
  replacement text.

Keyups are not routed through the input context, while keydowns are, so the
likely cause is WebKit's input-context step finishing out of order for this
process. That would also explain why switching apps or quitting clears it.
This is inferred, not proven. A plain WKWebView in a harness keeps the same
burst in order. See [KD-04](../tasks/keyboard-diagnostics/KD-04.md); the fix is
[KD-05](../tasks/keyboard-diagnostics/KD-05.md).

## Fix: key order guard (KD-05)

The user confirmed that, while the fault is present, other apps type
Vietnamese normally, and that switching to another app and back clears it.
The page cannot reach WebKit's queue, but it can see the symptom: a Backspace
keyup that arrives before its keydown. On macOS a capture listener on the
window (`keyOrderGuard.ts`) pairs Backspace keydowns with their keyups:

- A keyup with no open keydown is counted as early.
- A later Backspace keydown that matches an early keyup arrived before any
  text, so it passes as usual.
- If typed text arrives while early keyups are counted, the guard cancels
  the text's keydown and applies the missing deletes first, then the text.
  In a text field it uses `execCommand('delete')` and
  `execCommand('insertText')`, so undo and `input` events still work. In the
  terminal it sends DELs, then the text. The late Backspace keydowns for
  those deletes are cancelled when they arrive.
- OpenKey's U+202F spacer comes before its Backspaces, so it never starts a
  repair.
- Counts expire after 100 ms and reset when the window loses focus. A stray
  keyup, for example from a key released after switching windows, cannot
  delete text later.

The guard calls `stopPropagation`, not `stopImmediatePropagation`, so the
keyboard recorder still sees the cancelled events. It also fires a
`keyorderguard` event that the recorder logs, so a fault log shows each
repair. Key events in the normal order never change the counts, so ordinary
typing and held Backspace are untouched. A native reset of the input context
(what switching apps does) was considered, but there is no known API for it
and no way to test it without the fault. It stays an option if the guard is
not enough.

## Text checking off everywhere (KD-06)

The user asked to turn off autocorrect and spellcheck in every text input,
on a report that they can upset the Backspace order. The KD-04 fault was in
the chat composer, which already has `spellcheck="false"`,
`autocorrect="off"` and `autocapitalize="off"` (ACP-17), so those three did
not cause that log. They are still off in only some fields. Some macOS
features have no HTML attribute: text replacement, smart quotes and dashes,
smart insert and delete, link detection, and grammar checking. Inline
predictions are off only with `writingsuggestions="false"`. Two layers:

- **Page, all platforms** (`textInputDefaults.ts`). `<html>` gets
  `spellcheck="false"` and `writingsuggestions="false"`, which descendants
  inherit. A capture `focusin` listener sets `spellcheck="false"`,
  `autocorrect="off"`, `autocapitalize="off"` and `writingsuggestions="false"`
  on each text field (textarea, text-like input, contenteditable) as it gets
  focus, before any typing. Fields that set these themselves keep working,
  because the values are the same.
- **WebKit, macOS** (`textcheck_darwin.go`). Before any web view exists, the
  app registers `NO` for WebKit's text-checking user defaults:
  `WebContinuousSpellCheckingEnabled`, `WebGrammarCheckingEnabled`,
  `WebAutomaticSpellingCorrectionEnabled`, `WebAutomaticTextReplacementEnabled`,
  `WebAutomaticQuoteSubstitutionEnabled`, `WebAutomaticDashSubstitutionEnabled`,
  `WebAutomaticLinkDetectionEnabled`, `WebSmartInsertDeleteEnabled` and
  `WebSmartListsEnabled`. WebKit's `TextCheckerMac.mm` uses these keys before
  falling back to the system settings. They go in the registration domain,
  so nothing is written to the user's preferences. If the user turns a
  feature back on from the context menu, WebKit saves that choice and it
  wins. The page attributes still keep spelling and autocorrect off in each
  field.

## Release criteria

- With recording off, nothing is recorded and no page listeners are attached.
- With recording on, typing in a page field and in the terminal produces
  native and page records in time order, in a log saved from the menu.
- Unit tests cover the recorder, the App methods and the page record builder.
  Go tests, frontend tests, `npm run check` and `npm run build` pass, and so
  do the non-macOS desktop builds.
- The user runs a local build with recording on until the fault returns, and
  the saved log is analysed (KD-04).
