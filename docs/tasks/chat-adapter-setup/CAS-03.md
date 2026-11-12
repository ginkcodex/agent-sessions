# CAS-03 — Older copies

- **Status:** ✅ Done
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

## Evidence

What was done:

- `managedtools`: `Installer.Adapter(tool)` returns the pinned copy, else
  the newest valid copy of another plain x.y.z version for this platform;
  `Pinned(tool)`; `OlderThan(a, b)`. An older copy is checked like any
  install (private folders, receipt version and policy, the main script's
  SHA-256, `.bin` link) with only the adapter's own `package.json` version,
  since its lockfile is not in this app. A damaged pinned copy is reported;
  damaged older ones are skipped.
- `engine`: `ManagedAdapters` is `Adapter`, `Runtime` and `Pinned`;
  `ChatAgent` reports `version`, `pinned` and `outdated`; an older copy
  launches until it is updated. `types.ts` has the three fields.

Verified:

- `TestAdapterFindsPinnedThenOlderCopies`: nothing installed; the newest
  older copy by number (0.10.0 over 0.9.0); other platforms, non-plain
  versions and other adapters' folders ignored; a changed main script, a
  wrong receipt and a group-writable folder skipped; the pinned copy first;
  a damaged pinned copy reported. `TestOtherVersionChecksTheAdapterPackage`
  and `TestOlderThan`.
- `TestChatAgentsManagedAdapter`: version and pinned for the pinned copy;
  an older copy is available and outdated; pinned is reported when nothing
  is installed and not where the app cannot install.
  `TestChatLaunchManagedAdapter`: an older copy launches.
- `go test -race` for `internal/managedtools`, `internal/engine`,
  `internal/rpc` and `internal/app`: ok. gofumpt clean; golangci-lint shows
  no finding in the changed files (it caught an unchecked prefix in the
  first draft, fixed). `npm run check`: 0 errors.

Not checked: an older copy left by a real earlier install (CAS-05).
