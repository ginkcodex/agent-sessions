# CAS-03 — Older copies

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CAS-02
- **Plan:** [chat-adapter-setup.md](../../plan/chat-adapter-setup.md)

## Scope

- `internal/managedtools`: a read-only adapter lookup that returns the
  pinned copy, else the newest valid older copy, and its version. An older
  copy needs its receipt (that version, the adapter policy, the main
  script's SHA-256) and its package's `package.json` version to match, in a
  private folder.
- `ManagedAdapters` and `findAdapter` use it; `ChatAgent` gains
  `version`, `pinned` and `outdated` (Go and `types.ts`).
- Chats run the older copy until the user updates.

## Acceptance and tests

- Managed-tools tests: pinned copy preferred; an older one found with its
  version; a changed main script, a wrong receipt version or an unsafe folder
  is skipped; nothing is written.
- Engine tests: an older copy runs and is reported outdated with both
  versions.
