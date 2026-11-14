# Chat MCP and plugin commands — tasks

- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)
- **Progress:** 12/14 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CPC-01](CPC-01.md) | ✅ Done | Plan, task breakdown and indexes | 0.5 h | None |
| [CPC-02](CPC-02.md) | ✅ Done | Validate provider inventory contracts and command/log shapes | 2 h | CPC-01 |
| [CPC-03](CPC-03.md) | ✅ Done | Retain ACP command capabilities across setup, replay and reattach | 3 h | CPC-02 |
| [CPC-04](CPC-04.md) | ✅ Done | Read-only agent/host MCP and plugin inventory backend | 4 h | CPC-02 |
| [CPC-05](CPC-05.md) | ✅ Done | Command picker in the composer, with `/mcp` and plugin commands | 4 h | CPC-03, CPC-04 |
| [CPC-06](CPC-06.md) | ✅ Done | Normalize explicit plugin invocations in saved Claude transcripts | 3 h | CPC-02 |
| [CPC-07](CPC-07.md) | ✅ Done | Keep live invocations visible once across echo and saved reconciliation | 3 h | CPC-03, CPC-06 |
| [CPC-08](CPC-08.md) | ⬜ Not started | Cross-provider, desktop and remote QA | 3 h | CPC-05, CPC-07, CPC-10, CPC-13 |
| [CPC-09](CPC-09.md) | ⬜ Not started | O‍penCode MCP and plugin inventory once a machine-readable interface exists | 2 h | CPC-04 |
| [CPC-10](CPC-10.md) | ✅ Done | `/plugin` inventory panel | 3 h | CPC-04, CPC-05 |
| [CPC-11](CPC-11.md) | ✅ Done | Capture an exact `/mcp` turn out of the transcript | 3 h | CPC-03, CPC-07 |
| [CPC-12](CPC-12.md) | ✅ Done | MCP servers modal: parsed status rows, filter, Markdown fallback | 4 h | CPC-11 |
| [CPC-13](CPC-13.md) | ✅ Done | `/plugin` in the same modal | 2 h | CPC-10, CPC-12 |
| [CPC-14](CPC-14.md) | ✅ Done | `cmd` tag and mono font on slash-command user messages | 1 h | CPC-07 |
