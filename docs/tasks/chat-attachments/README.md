# Chat attachments — tasks

- **Plan:** [chat-attachments.md](../../plan/chat-attachments.md)
- **Progress:** 4/6 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CA-01](CA-01.md) | ✅ Done | Plan and task docs, index rows, amend acp-chat decision 3 | 1 h | None |
| [CA-02](CA-02.md) | ✅ Done | `internal/chatattach`: read files, detect images and text, scale large images, limits, prompt blocks | 4 h | CA-01 |
| [CA-03](CA-03.md) | ✅ Done | Engine prompt with attachments, `promptCapabilities` on the chat, RPC `attachments` behind `ChatAttachments` | 3 h | CA-02 |
| [CA-04](CA-04.md) | ✅ Done | App bindings: prompt with attachments, multi-select picker, dropped files, pasted images | 3 h | CA-03 |
| [CA-05](CA-05.md) | ⬜ Not started | Composer: + button, chips, drop and paste, send with attachments, live tail file parts, mock | 4 h | CA-04 |
| [CA-06](CA-06.md) | ⬜ Not started | QA with Claude, Codex and OpenCode: an image and a text file each | 2 h | CA-05 |
