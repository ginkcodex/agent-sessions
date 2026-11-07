# Go to File code diagnostics — tasks

- **Plan:** [code-diagnostics.md](../../plan/code-diagnostics.md), contract [code-diagnostics-contract.md](../../plan/code-diagnostics-contract.md)
- **Progress:** 19/28 done

The stage tables below are the canonical status for each task;
[status notes](#status-notes) explain the partial and blocked ones. CD-16 is
automated coverage, not native CD-17 QA. CD-10 used an explicit dependency
exception against committed RPC contracts and tests, not remote activation.
Local controls are wired; remote SSH/WSL diagnostics use a host-side identity
with unit and RPC tests only. Manual keyboard/focus QA is not claimed. The
authoritative wire contract is [contract v1](../../plan/code-diagnostics-contract.md);
see [the implementation plan](../../plan/code-diagnostics.md) for shared
behavior, architecture, trust rules and release criteria.

## Remaining work and totals

The [remaining-work backlog](remaining-work.md) splits CD-22…CD-26
acceptance gaps into ≤4 h packages, followed by tool/native gates and automation last.
It distinguishes independent offline coding from approval/tool/environment blockers;
no installs, activation or dependency exception are authorized.

Parent totals: **28 tasks = 19 complete + 8 partial + 1 blocked + 0 not started**.
The partial parents are CD-09, CD-17 and CD-22…CD-27; the blocked parent is
CD-28. Packages do not change the denominator.

## How to use these briefs

- Finalize contracts in CD-01 before implementation; API names elsewhere remain
  provisional until that task is complete. Keep the plan and dependent briefs
  synchronized when a contract changes.
- Respect listed dependencies. Frontend presentation tasks can run in parallel
  after CD-10 using fixtures/mocks based on CD-01. Dependency ranges such as
  `CD-05…CD-15` include both endpoints and every task between them.
- The frontend presentation tasks share `FileViewer.svelte` and range helpers;
  coordinate ownership of those edits even when the task graph permits parallel work.
- Estimates include focused tests and are capped at four developer hours per
  task. Split and re-estimate work before starting if exploration shows it will
  exceed that limit; do not silently omit acceptance requirements.
- Likely file lists identify integration points, not instructions to create
  duplicate infrastructure. Recheck the current code before making changes.
- Follow the repository Definition of Done in TASKS.md. Mark implementation,
  automated testing, and manual QA accurately; planned QA is not a passing result.
- Code diagnostics are separate from existing session-provider diagnostics.
  Checking must not modify source files or persist drafts/diagnostic payloads.

## Cross-cutting implementation notes

- Existing `FileLineSelection` is one-based and line-only. Diagnostic range
  navigation needs an explicit UTF-16 column-aware path; do not silently change
  line-selection behavior used by blame and normal source selection.
- `FileEditState.version` invalidates cached editor state; it is not the monotonic
  document version required for live diagnostic freshness.
- Preserve lazy editor loading. Adding diagnostic display to the read-only
  viewer must not statically import CodeMirror into the main application bundle.
- Current Node/SSR tests do not prove client focus, scrolling, or native WebView
  layout. The Stage 1/2 QA tasks must record those checks separately.

## Estimate summary

| Stage | Tasks | Developer effort |
|---|---|---|
| 1 — Explicit project checks | 17 | 62 h |
| 2 — Live opened-file diagnostics | 10 | 40 h |
| 3 — Optional automatic project checks | 1 | 4 h |
| **Total** | **28** | **106 h** |

These are planning estimates, not elapsed-time commitments. Tool-version spikes
and platform QA may uncover follow-up tasks that must be estimated separately.

## Stage 1 — Explicit project checks

The first release uses approved, host-local tools and an explicit Check Project
action. It does **not** automatically execute tools when the dialog opens.

| Task | Status | Scope | Release gate | Est |
|---|---|---|---|---|
| [CD-01](CD-01.md) | ✅ Done | Authoritative contract v1, synthetic fixtures, validation/round-trip tests | Full Go suite, targeted race tests, and package lint pass | 3 h |
| [CD-02](CD-02.md) | ✅ Done | Backend project trust, grants, revocation and contained paths | [Trust policy and approval copy](../../plan/code-diagnostics-trust.md); focused race tests | 4 h |
| [CD-03](CD-03.md) | ✅ Done | Bounded argv-only checker supervisor, trust hooks and process-tree cleanup | [Execution semantics](../../plan/code-diagnostics-supervisor.md); fake/helper race tests and package lint pass | 4 h |
| [CD-04](CD-04.md) | ✅ Done | Installed Go vet/Svelte-check format spike, sanitized fixtures and tests | [Version/invocation evidence and explicit platform limits](../../plan/code-diagnostics-checkers.md); full race suite and scoped lint pass; full lint has existing unrelated failures | 3 h |
| [CD-05](CD-05.md) | ✅ Done | Contained Go vet adapter, fixture parser and fake-supervisor tests | [Policy, evidence and limits](../../plan/code-diagnostics-go-vet.md); full race suite/scoped lint pass; existing unrelated full-lint failures remain | 3 h |
| [CD-06](CD-06.md) | ✅ Done | Contained Svelte-check adapter and fixture/fake-supervisor tests | [Policy, evidence and limits](../../plan/code-diagnostics-svelte.md); targeted race/scoped lint and full Go tests pass | 4 h |
| [CD-07](CD-07.md) | ✅ Done | Headless result manager, bounded snapshots/events and lifecycle ownership | [Policy, evidence and limits](../../plan/code-diagnostics-manager.md); deterministic fake-adapter/supervisor races, scoped lint and full Go tests pass | 4 h |
| [CD-08](CD-08.md) | ✅ Done | Engine/App Stage 1 bindings, ownership and offline routing | [Identity, registry, lifecycle and validation evidence](../../plan/code-diagnostics-engine.md); full Go/race tests and new-code scoped lint pass | 3 h |
| [CD-09](CD-09.md) | 🟡 Partial | RPC transport, recovery evidence, and host-side SSH/WSL identity with remote managed installs ([editor model](../../plan/code-diagnostics-zed-model.md), 2026-10-23) | Unit/RPC tests only; no real SSH or WSL session run; not counted | 4 h |
| [CD-10](CD-10.md) | ✅ Done | Standalone frontend API/store and synthetic mock lifecycle | [Atomic multipart assembly, bounded recovery, identity/epoch races and frontend validation](../../plan/code-diagnostics-frontend.md); CD-09 activation exception explicit | 4 h |
| [CD-11](CD-11.md) | ✅ Done | Dialog approval/status and explicit Check/Cancel/provider Retry controls | Local capability/identity discovery; SSR and store lifecycle tests; manual focus/keyboard QA not performed; CD-09 remotes still blocked | 3 h |
| [CD-12](CD-12.md) | ✅ Done | Problems panel, severity filters, keyboard navigation and draft-safe UTF-16 range reveal | Stage 1: explicit project checks; remote CD-09 blocked | 4 h |
| [CD-13](CD-13.md) | ✅ Done | Shared exact-deduplication file/tree badges and ancestor totals | Stage 1; 403 frontend tests, check/build passed; native layout QA deferred | 3 h |
| [CD-14](CD-14.md) | ✅ Done | Verified source gutter/range indicators and plain-text file/stale messages | Stage 1; 409 frontend tests, check/build passed; native paint/layout QA deferred | 4 h |
| [CD-15](CD-15.md) | ✅ Done | Lazy CodeMirror lint, verified saved ranges, draft/cache freshness guards | 412 frontend tests; check/build passed; native tooltip/gutter QA deferred | 4 h |
| [CD-16](CD-16.md) | ✅ Done | Deterministic checker→manager→RPC and real-store/view/editor integration regressions; [evidence](CD-16.md#automated-evidence-2026-10-22) | Automated Stage 1 gate only; CD-09 remote identity remains blocked | 4 h |
| [CD-17](CD-17.md) | 🟡 Partial | Partial/blocked: non-GUI inventory/build/runtime preflight and [blocked QA matrix](../../plan/code-diagnostics-qa.md) | Stage 1 HOLD; native checks unperformed; macOS link preflight failed; CD-09 remotes blocked | 4 h |

## Stage 2 — Live opened-file diagnostics

Services start asynchronously for approved projects. Opened-file coverage is
not a promise that the entire workspace has been checked.

| Task | Status | Scope | Release gate | Est |
|---|---|---|---|---|
| [CD-18](CD-18.md) | ✅ Done | Bounded LSP Content-Length stdio transport: framing, correlation, cancel/timeout, queues, close, fake-server tests; [evidence](CD-18.md#evidence-2026-10-22) | Transport only: no process start, adapters or trust bypass; Stage1 HOLD/CD-17 and CD-09 do not block it | 4 h |
| [CD-19](CD-19.md) | ✅ Done | Safe LSP session lifecycle: trust/known-dir gate, initialize/initialized, UTF-16/sync negotiation, deny-by-default server requests, idle grace/limits, shutdown/kill, capped restarts; [evidence](CD-19.md#evidence-2026-10-22) | Fake server/process/clock only: no adapters, document sync or live capability; file watchers rejected (`unwatched`); Stage1 HOLD/CD-17 and CD-09 exceptions do not block local lifecycle | 4 h |
| [CD-20](CD-20.md) | ✅ Done | Bounded versioned live-document operations across Backend/App/RPC | Automated transport/contract evidence only; no production live activation | 4 h |
| [CD-21](CD-21.md) | ✅ Done | Frontend displayed-document lifecycle bridge | Automated fake API/store evidence; inert without unadvertised live capability | 4 h |
| [CD-22](CD-22.md) | 🟡 Partial | gopls v0.24.0 spike/decision ([doc](../../plan/code-diagnostics-gopls.md)), contained Go workspace detection, availability, publishDiagnostics normalization and version guards; fixtures + fake-server tests | Partial: **local desktop route active (experimental, darwin/arm64, 2026-10-23)**: installed gopls v0.24.0, approval-gated fixed argv, toolchain bound into approval, real-gopls draft smoke (error then empty clear, never saved); remote fail-closed, TS/Svelte unsupported, no native/manual draft QA | 4 h |
| [CD-23](CD-23.md) | 🟡 Partial | TypeScript adapter seam, package discovery, ownership, availability, shared normalizer; [doc](../../plan/code-diagnostics-typescript.md) | Partial/blocked: no approved LSP server installed (observed only vtsls 0.3.0 in Zed's private cache, unversioned pushes); synthetic fake-server tests; not wired, `liveCodeDiagnostics` unadvertised | 4 h |
| [CD-24](CD-24.md) | 🟡 Partial | Svelte LS adapter seam, package detection, Svelte/TS ownership; [doc](../../plan/code-diagnostics-svelte.md) | Partial/blocked: none verified (no svelte-language-server found on PATH, project or Zed cache); synthetic tests; not wired, `liveCodeDiagnostics` unadvertised | 4 h |
| [CD-25](CD-25.md) | 🟡 Partial | Reopen/refresh/pre-check saved revision reconciliation; draft/undo preservation; fake-session idle/publication guards; explicit unwatched stale fallback; approved local project watcher + `go-syntax` saved-file provider ([evidence](../../plan/code-diagnostics-project-watch.md)) | Partial: TS/Svelte/gopls unopened coverage still `unwatched`; no unopened type checks, remote watching or manual QA | 4 h |
| [CD-26](CD-26.md) | 🟡 Partial | Deterministic framed fake-server lifecycle and real frontend store integration | Partial: live retention/ownership/status gaps, exhaustive cleanup and manual UI smoke remain; not production integration | 4 h |
| [CD-27](CD-27.md) | 🟡 Partial | Partial/blocked: focused fake preflight and [live QA matrix](../../plan/code-diagnostics-live-qa.md); no real-server/native/remote runs or measurements | Stage 2 HOLD; production wiring/consumers, identity/bounds/status, approved tools and native/remote prerequisites remain; not counted | 4 h |

## Stage 3 — Optional automatic project checks

| Task | Status | Scope | Release gate | Est |
|---|---|---|---|---|
| [CD-28](CD-28.md) | ⛔ Blocked | Blocked: release prerequisite review only; all acceptance unchecked | [Stage 1 HOLD](../../plan/code-diagnostics-qa.md#release-decision-and-remaining-work) and [Stage 2 HOLD](../../plan/code-diagnostics-live-qa.md#release-decision); no dependency exception; automation absent/off-default | 4 h |

Additional languages and checker adapters require separately scoped tasks after
feedback from the first two stages; they are not included in CD-28.

## Status notes

Progress: **19/28 complete**. CD-01…CD-08, CD-10…CD-16 and CD-18…CD-21 are ✅ done; CD-09 is 🟡 partial (remote SSH/WSL diagnostics implemented 2026-10-23 with a host-side identity per the [editor model](../../plan/code-diagnostics-zed-model.md); no real SSH/WSL QA, not counted); CD-17 is 🟡 partial/blocked (non-GUI QA preparation only; Stage 1 HOLD); CD-22 is 🟡 partial (gopls adapter with a local experimental desktop route and a real installed-gopls smoke; remote stays fail-closed and there is no native/manual QA, so not counted); CD-23 is 🟡 partial/blocked (TypeScript adapter seam, contained discovery and synthetic tests; no host-approved LSP server verified, not counted); CD-24 is 🟡 partial/blocked (Svelte adapter seam, ownership policy and synthetic tests; no Svelte language server verified, not counted); CD-25 is 🟡 partial (revision reconciliation and fake-session guards; external rename/delete reconciliation incomplete, not counted); CD-26 is 🟡 partial (deterministic fake lifecycle integration; live retention/ownership/status gaps and manual UI smoke remain, not counted); CD-27 is 🟡 partial/blocked ([live QA matrix](../../plan/code-diagnostics-live-qa.md), focused fake preflight only; every live cell blocked/untested, Stage 2 HOLD, not counted); CD-28 is ⛔ blocked: Step 1 release prerequisites are unmet (both gates HOLD), no dependency exception granted; no automation implemented/enabled, off by default. CD-16 proves deterministic automated seams only, not native CD-17 QA or production remote activation.
CD-10 explicitly proceeded locally against committed RPC contracts/tests; this dependency exception does not complete CD-09 or activate production remotes.
Only the experimental local desktop route advertises diagnostics capabilities (Stage 1 plus live Go via an installed supported gopls); remote hosts remain fail-closed and both release HOLDs stand. Detailed estimates, dependencies,
contracts, and acceptance criteria are in
[docs/plan/code-diagnostics.md](../../plan/code-diagnostics.md). Individual task
briefs and acceptance checklists are in [docs/tasks](../README.md).
These follow-up tasks are not included in the historical milestone totals above.
Actual CD totals: **28 = 19 complete + 8 partial + 1 blocked + 0 not started**
(partial: CD-09, CD-17, CD-22…CD-27; blocked: CD-28).
See the [≤4h remaining-work backlog](remaining-work.md) for
safe independent coding versus approval/tool/environment blockers; no installs,
activation or automation are authorized by that split.
