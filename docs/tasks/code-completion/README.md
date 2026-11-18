# Code completion — tasks

- **Plan:** [code-completion.md](../../plan/code-completion.md)
- **Progress:** 13/18 done (CC-01 and CC-09 partial)

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
| [CC-10](CC-10.md) | ✅ Done | Import edits from resolve, on the host and the wire (vtsls) | 4 h | CC-03, CC-04 |
| [CC-11](CC-11.md) | ⬜ Not started (needs a decision) | Pyright auto-imports from project files not opened | 3 h | CC-08 |
| [CC-12](CC-12.md) | ✅ Done | Accepting an item adds the import it resolves to | 3 h | CC-06, CC-10 |
| [CC-13](CC-13.md) | ✅ Done | Review fixes on the host: conflicting extra edits, resolve guard, kept raw bytes | 3 h | CC-02, CC-03, CC-10 |
| [CC-14](CC-14.md) | ✅ Done | Review fixes in the editor, store and texts | 3 h | CC-06, CC-12 |
| [CC-15](CC-15.md) | ✅ Done | Review fixes in the completion tests and the Chrome harness | 2 h | CC-12 |
| [CC-16](CC-16.md) | ✅ Done | Second review: server order past 99 items, snippet choices and transforms | 2 h | CC-06, CC-07 |
| [CC-17](CC-17.md) | ⬜ Not started | Second review: late answers, awaited items whose word is gone, language labels | 3 h | CC-06, CC-12, CC-14 |
| [CC-18](CC-18.md) | ⬜ Not started | Second review: one shortcut label, mock positions, test cleanups | 3 h | CC-01, CC-15 |
