# Code completion — tasks

- **Plan:** [code-completion.md](../../plan/code-completion.md)
- **Progress:** 6/9 done (CC-01 partial)

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
| [CC-08](CC-08.md) | ⬜ Not started | ruby-lsp completion and Pyright auto-imports; approval carries over | 2 h | CC-06 |
| [CC-09](CC-09.md) | ⬜ Not started | Real-server and SSH latency QA | 3 h | CC-07, CC-08 |
