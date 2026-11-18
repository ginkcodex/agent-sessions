# Code completion — tasks

- **Plan:** [code-completion.md](../../plan/code-completion.md)
- **Progress:** 7/11 done (CC-01 and CC-09 partial)

Suggestions while typing in the Go to File editor: first from the file and
the language, then from the project's language server.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CC-01](CC-01.md) | 🟡 Partial (native macOS Cmd+I check) | Completion without a language server: words and language completions, setting, keys | 4 h | — |
| [CC-02](CC-02.md) | ✅ Done | LSP completion requests: capabilities, routing, normalized and bounded answers | 4 h | — |
| [CC-03](CC-03.md) | ✅ Done | Item details through `completionItem/resolve`, kept on the host | 2 h | CC-02 |
| [CC-04](CC-04.md) | ✅ Done | Completion over the wire: bindings, RPC, capability flag, cancel | 3 h | CC-02, CC-03 |
| [CC-05](CC-05.md) | ✅ Done | Send pending typing at once before a completion request | 3 h | — |
| [CC-06](CC-06.md) | ✅ Done | Language-server completion in the editor: triggers, merging, edits, details | 4 h | CC-01, CC-04, CC-05 |
| [CC-07](CC-07.md) | ✅ Done | Snippet completions with tab stops | 3 h | CC-06 |
| [CC-08](CC-08.md) | ✅ Done | ruby-lsp completion and Pyright auto-imports; approval carries over | 2 h | CC-06 |
| [CC-09](CC-09.md) | 🟡 Partial (local real servers done; SSH latency, native GUI and IME, rust-analyzer, PHP, Java, Windows and Linux not checked) | Real-server and SSH latency QA | 3 h | CC-07, CC-08 |
| [CC-10](CC-10.md) | ⬜ Not started | Import edits that arrive on resolve (vtsls) | 4 h | CC-03, CC-06 |
| [CC-11](CC-11.md) | ⬜ Not started (needs a decision) | Pyright auto-imports from project files not opened | 3 h | CC-08 |
