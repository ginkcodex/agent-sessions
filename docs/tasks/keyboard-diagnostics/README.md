# Keyboard diagnostics — tasks

- **Plan:** [keyboard-diagnostics.md](../../plan/keyboard-diagnostics.md)
- **Progress:** 3/6 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [KD-01](KD-01.md) | ✅ Done | Recorder: ring buffer, JSONL log, App methods, state event | 3 h | — |
| [KD-02](KD-02.md) | ✅ Done | macOS native monitor, markers and Help menu controls | 4 h | KD-01 |
| [KD-03](KD-03.md) | ✅ Done | Page-side capture batched to the recorder | 3 h | KD-01 |
| [KD-04](KD-04.md) | 🟡 Partial — fault log analysed (WebKit gives the page OpenKey's Backspaces after the replacement); no healthy app log or terminal log | Desktop check with OpenKey and analysis of a fault log | 2 h | KD-02, KD-03 |
| [KD-05](KD-05.md) | 🟡 Partial — guard built, unit-tested and verified in a WKWebView replay; desktop app not yet confirmed | Keep OpenKey's Backspaces before its replacement text | 4 h | KD-04 |
| [KD-06](KD-06.md) | 🟡 Partial — page attributes and WebKit defaults built and tested; desktop app not yet checked | Spellcheck, autocorrect and text substitutions off in every field | 3 h | KD-04 |
