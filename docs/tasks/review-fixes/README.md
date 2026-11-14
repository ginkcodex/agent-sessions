# Review fixes — tasks

- **Plan:** [review-fixes.md](../../plan/review-fixes.md)
- **Progress:** 2/25 done

From the architecture and feature review of 2026-11-14. RF-01 to RF-10 are
security and privacy; RF-11 to RF-23 are reliability; RF-24 and RF-25 are CI
and docs.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [RF-01](RF-01.md) | ✅ Done | Limit the `release` environment to `main` and `v*` tags; tag ruleset | 1 h | None |
| [RF-02](RF-02.md) | ⬜ Not started | The release and artifacts jobs install apt packages without the shared cache, or check them | 2 h | None |
| [RF-03](RF-03.md) | ⬜ Not started | Every `uses:` pinned to a full SHA with its tag in a comment; Dependabot for actions | 1.5 h | None |
| [RF-04](RF-04.md) | ✅ Done | Pass `--no-textconv` to blame and the submodule diff; filters stay with IS-06 | 2 h | None |
| [RF-05](RF-05.md) | ⬜ Not started | Strip control characters from pasted text; bound OSC 52 writes | 2 h | None |
| [RF-06](RF-06.md) | ⬜ Not started | Quote dropped file names for PowerShell and cmd so `$`, backtick and `%` stay literal | 1.5 h | None |
| [RF-07](RF-07.md) | ⬜ Not started | Denylist for webview-supplied paths; opt-in switches only from their own dialog flow | 3 h | None |
| [RF-08](RF-08.md) | ⬜ Not started | The website ingest and the AUR job verify `SHA256SUMS.minisig` before trusting `SHA256SUMS` | 3 h | None |
| [RF-09](RF-09.md) | ⬜ Not started | Redact manifest fields; key/value pairs in JSON; Windows home paths | 3 h | None |
| [RF-10](RF-10.md) | ⬜ Not started | `openURL` stops falling back after a refusal; `http://` refused for keyed endpoints | 1.5 h | None |
| [RF-11](RF-11.md) | ⬜ Not started | Decode the reply ID as an integer, not `any` | 1 h | None |
| [RF-12](RF-12.md) | ⬜ Not started | Anchor missing roots at their provider's base directory, non-recursively | 2.5 h | None |
| [RF-13](RF-13.md) | ⬜ Not started | Per-provider fallback, `ErrEventOverflow` rescan, retry re-installing watches | 3 h | RF-12 |
| [RF-14](RF-14.md) | ⬜ Not started | One running scan plus a dirty flag per provider; `Close` waits for refreshes | 3.5 h | None |
| [RF-15](RF-15.md) | ⬜ Not started | An unreadable source keeps its previous state; it neither blocks the provider nor drops sessions | 3 h | None |
| [RF-16](RF-16.md) | ⬜ Not started | The client kills the agent when the connection ends with an error it did not cause | 2 h | None |
| [RF-17](RF-17.md) | ⬜ Not started | A start deadline and a cancel path while the frontend has no chat ID yet | 3 h | RF-16 |
| [RF-18](RF-18.md) | ⬜ Not started | Decide whether `turn_end` waits for questions; make the engine, store and tests agree | 3 h | None |
| [RF-19](RF-19.md) | ⬜ Not started | Lstat (or open non-blocking) before reading in Read, Blame, diff and external | 1.5 h | None |
| [RF-20](RF-20.md) | ⬜ Not started | Wire `SessionManager.Retry` into a production path with backoff | 3 h | None |
| [RF-21](RF-21.md) | ⬜ Not started | Run remote probe, deploy and serve scripts through `/bin/sh`, as panels do | 3 h | None |
| [RF-22](RF-22.md) | ⬜ Not started | Drop or version-gate `ForkAfterAuthentication=no` | 2 h | None |
| [RF-23](RF-23.md) | ⬜ Not started | Unescape on read, TOML-valid quoting on write, check `Scanner.Err` | 2 h | None |
| [RF-24](RF-24.md) | ⬜ Not started | Run the existing `.golangci.yml` and govulncheck in CI; fix `nilerr` hits | 3 h | None |
| [RF-25](RF-25.md) | ⬜ Not started | AUR-07, LR-19, M5-04, M5-06, M8-07 and the code-diagnostics count | 1 h | None |
