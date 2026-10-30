# Code checks: choosing languages — task briefs

Plan: [code-diagnostics-languages.md](../plan/code-diagnostics-languages.md) ·
[Root tracker](../../TASKS.md#follow-up--code-check-languages)

Planned 2026-10-30. Each task is at most four developer hours and ends with
its tests passing: `go test ./internal/...`, `GOOS=windows` and `GOOS=linux`
`go vet ./internal/... ./cmd/...`, and from `frontend/` `npm test` and
`npx svelte-check --threshold error`. Native Windows/Linux runs and real remote
hosts are outside these tasks unless a brief says otherwise.

| ID | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CL-01](#cl-01--language-table) | ✅ Done | Language table; hard-coded ID lists use it | 3 | — |
| [CL-02](#cl-02--one-bridge-for-plain-pushes) | ✅ Done | One bridge for plain diagnostic pushes; table dispatch | 3 | CL-01 |
| [CL-03](#cl-03--live-discovery-from-the-table) | ✅ Done | Workspace adapters and live discovery loop over the table | 4 | CL-01 |
| [CL-04](#cl-04--discovery-skips-languages-that-are-off) | ✅ Done | Discovery skips languages that are off; `disabled` reports | 4 | CL-03 |
| [CL-05](#cl-05--disabled-languages-over-the-wire) | ✅ Done | `disabledLanguages` through Engine, App and RPC; capability | 3 | CL-04 |
| [CL-06](#cl-06--frontend-preference-and-display) | ✅ Done | Preference, attach, rediscovery, "Off in Settings" | 4 | CL-05 |
| [CL-07](#cl-07--settings-section) | ✅ Done | Settings section "Code checks" | 3 | CL-06 |
| [CL-08](#cl-08--contract-docs-and-gate) | ✅ Done | Contract and docs; automated gate | 2 | CL-07 |
| **Total** | **8/8** | | **26** | |

## CL-01 — Language table

**Goal.** One table names each language, its providers and its managed tools.

- `internal/codediagnostics/languages.go`: `Language{ID, Name, Providers,
  Tools}` for `go`, `typescript`, `svelte`, `python`, `rust`, and lookups
  `LanguageOf(providerID)` and `Languages()`.
- Replace the hand-kept provider lists with the table:
  `engine.discoveryFailed` (live providers only) and the tool list in
  `engine/managed_tools_contract.go`.
- `managedtools` cannot import `codediagnostics`; it keeps its lists, and a
  test in `codediagnostics` checks that the table's tools are exactly
  `managedtools`' proposable tools.

**Done when.** A test checks that every provider ID registered by discovery
belongs to exactly one language and every managed tool to at least one. No
behavior changes; existing tests pass unchanged.

**Outcome (2026-10-30).** `languages.go` splits each language's providers into
`Live` and `Saved`; `LiveProviderIDs()` and `ManagedToolNames()` replace the
lists in `engine.discoveryFailed` and the managed-tools status.
`managedtools.ProposableTools()` lets `TestLanguagesCoverEveryProviderAndTool`
compare the tool sets.

## CL-02 — One bridge for plain pushes

**Goal.** Adding a language does not mean copying a bridge type.

- TypeScript, Ruff, Pyright and rust-analyzer bridges only call
  `handleOwnedPush` with a provider ID, a partial notice and an ownership
  function. Replace them with one `ownedPushBridge` built from a table entry.
- gopls and Svelte keep their own bridges (their normalizers differ).
- `LiveDocuments`' `switch n.ProviderID` in `live_owner.go` becomes a lookup.
- Keep the exported constructors that tests use as thin wrappers, or move the
  tests to the new constructor; no behavior changes.

**Done when.** Existing bridge tests pass, plus a test that an unknown
provider's notification is ignored as before.

**Outcome (2026-10-30).** `push_bridge.go` holds `PushBridge`, its spec table
and `liveBridge`, which `live_owner.go` now calls. `TSBridge`, `RuffBridge`,
`PyrightBridge` and `RustBridge` are aliases with thin constructors, so the
existing tests run unchanged. `TestLiveBridgeCoversEveryLiveProvider` checks
every live provider has a bridge and that others are ignored.

## CL-03 — Live discovery from the table

**Goal.** Live languages are wired in one place.

- Give the live part of a table entry: language-ID function, document
  extensions, workspace config markers and a discover function returning
  `(ServerConfig, ProviderState, error)` that wraps today's `discoverXWorkspace`.
- `workspace_adapters.go`: the provider list and both `switch provider.id`
  blocks use the entry.
- `live_discovery.go`: `DiscoverLive` and `DiscoverLocalLive` build their
  registrations by looping over the entries instead of listing each language.
  Keep the Ruff/Pyright pairing rule and the gopls/Rust specifics explicit.

**Done when.** Discovery returns the same registrations, in the same order, as
before for the existing fixtures (single root, nested workspaces, missing
tools). Existing tests pass unchanged.

**Outcome (2026-10-30).** `workspaceProviders` in `workspace_adapters.go` gives
TypeScript, Ruff and Pyright their language function, extensions, config test
and discover function; the nested scan and the single-root `register` both use
it. `DiscoverLocalLive` collects each provider's registration and returns them
in `LiveProviderIDs()` order; gopls, Svelte and Rust stay explicit, and so do
the shared Python roots. `DiscoverLive`, a test-only seam for gopls,
TypeScript and Svelte, is unchanged. `TestDiscoverLocalLiveFollowsLanguageOrder`
covers the single-root and nested paths.

## CL-04 — Discovery skips languages that are off

**Goal.** A language that is off adds nothing to approval and starts nothing.

- `discoverAll(root, disabled)`: saved checkers (`discoverCheckersIn`), both
  live discoveries and `go-syntax` skip the providers of disabled languages.
  No tool lookup, preparation, proposal entry or managed-tool offer is made for
  them.
- When the project scan saw files of a disabled language, add a registration
  with `availability: "disabled"`, the notice "Off in Settings" and no adapter
  or live entry. Otherwise add nothing.
- The diagnostics owner keeps the set per attachment, beside `o.dirs`, so
  rediscovery after approved preparation (`startPreparation`) uses it too.
  The set is part of the attach call; until CL-05 it is always empty.

**Done when.** Tests show, for a project with Go and Python files: Python off
gives no Ruff/Pyright tools or proposal entries and two `disabled` providers;
a project without Python gives none; the fingerprint differs from the
all-languages one and the project's grant carries over to it; turning the
language off stops its sessions; rediscovery keeps the set.

**Outcome (2026-10-30).** `cd.DisabledLanguages` (unknown IDs dropped) feeds
`LocalLiveEnvironment.Disabled`, so `DiscoverLocalLive` and the nested scan
skip those providers before any tool lookup; `discoverCheckersIn` skips go-vet,
svelte-check, cargo-check (with its setup) and go-syntax the same way.
`discoverAll` also drops them from injected discoveries and appends
`cd.DisabledRegistrations`: live providers as `openedDocuments` entries, saved
ones plain, all with "Off in Settings" and nothing to approve. The owner keeps
the set per attachment in `o.disabled`; `rediscovery` uses it for setup.
`Engine.attachCodeDiagnostics` takes the set; the public method passes none
until CL-05. Tests: `TestDisabledLanguagesLeaveDiscovery` (single root,
nested, no Python) and `TestCodeDiagnosticsLanguageOff` (proposal without Ruff,
new fingerprint, grant carried over, disabled rows, no launch, rediscovery).
Stopping the old sessions is the existing lease end on a changed fingerprint
(`trust_test.go`), not a new mechanism.

## CL-05 — Disabled languages over the wire

**Goal.** The desktop's choice reaches local and remote discovery.

- `engine.AttachCodeDiagnostics(ctx, dir, version, disabled []string)` and
  the `CodeDiagnosticsBackend` interface, offline and unsupported backends.
- RPC: `codeDiagnosticsParams.DisabledLanguages`; at most 16 names of at most
  32 bytes, else `invalidArgument`; unknown names ignored. The client sends it.
- Capability `codeDiagnosticsLanguages` in `rpc/types.go`, advertised only by
  servers that apply the field.
- `App.AttachCodeDiagnostics(dir, version, host, disabled)` and the Wails
  binding.

**Done when.** RPC tests cover: field absent means all languages; bounds;
unknown names; the capability. An App test shows local and remote routes pass
the set.

**Outcome (2026-10-30).** The set travels as an optional interface,
`engine.CodeDiagnosticsLanguagesBackend.AttachCodeDiagnosticsWithout`, rather
than a new parameter on `CodeDiagnosticsBackend`, matching how
`CodeNavigationBackend` and the other optional features are added; the plain
attach means "all on", so test doubles and old servers need no change. The RPC
server advertises `codeDiagnosticsLanguages` only when its backend implements
it, rejects more than 16 names, an empty name or one over 32 bytes with
`invalidArgument`, and passes unknown names through for
`cd.NewDisabledLanguages` to ignore. The client drops the set for a server
without the capability. `App.AttachCodeDiagnostics` takes a fourth argument and
the local route advertises the capability itself; the frontend sends `[]` until
CL-06. Tests: `TestDiagnosticsRPCDisabledLanguages`,
`TestCodeDiagnosticsAttachPassesDisabledLanguages`, and the Wails argument
check in `code-diagnostics-api.test.mjs`.

## CL-06 — Frontend preference and display

**Goal.** The choice is stored, sent and visible.

- `preferences.svelte.ts`: `disabledLanguages`, saved in `localStorage`,
  default none; a list of the five languages and labels in `codeLanguages.ts`.
- `api.ts` / `codeDiagnostics.ts`: attach passes the list.
- `codeDiagnostics.svelte.ts`: changing the preference rediscovers the shown
  project.
- `codeCheckLabels.ts`: a `disabled` provider reads "Off in Settings".
  `codeNavigation.ts`: Go to Definition in such a file says the language is off
  in Settings.
- When the host lacks `codeDiagnosticsLanguages` and a language is off, the
  panel says the host checks every language until its server is updated.
- The panel's setup rows and text name only languages that are on.
- Mock API support.

**Done when.** Unit and server-render tests cover persistence and default,
the attach argument, rediscovery on change, the labels, the navigation reason
and the old-host notice.

**Outcome (2026-10-30).** `codeLanguages.ts` holds the five languages with
their providers and extensions; `codeCheckLabels.ts` now reads language names
from it. `preferences.disabledLanguages` is saved as a JSON list under
`agent-sessions:disabled-code-languages`; unknown or malformed values leave
every language on. The store takes the setting like the auto-install one,
sends it on every attach, and `setLanguageEnabled` saves and rediscovers; a
change it could not apply yet (a request pending) shows "Language settings
changed" with Rediscover. Only when the host advertises
`codeDiagnosticsLanguages` does the store treat an off language's files as not
live and give navigation the "off in Settings" reason; an older host keeps its
old behavior and the panel says it still checks those languages. Disabled rows
read "Off in Settings" and point to Settings; the approval reason and status
line say so when every language of the project is off. Panel text, live server
lists and manual install commands name only languages that are on, and Rust
guidance ignores disabled Rust rows. The mock advertises the capability and
lists a disabled row per off language. Tests: `code-languages.test.mjs` and the
Wails argument check. `partial` still counts disabled providers, so an
approved project with a language off reads "partial".

## CL-07 — Settings section

**Goal.** Users can choose languages in Settings.

- `ManageSettingsDialog.svelte`: "Code check logs" becomes "Code checks":
  language switches, then "Download missing language servers automatically"
  (moved from the panel, which links to Settings), then the logs.
- Beside the switches: turning a language on starts its checks in projects
  already approved; turning it off stops them.

**Done when.** Server-render tests cover the switches, their saved state, the
moved auto-install switch and the panel link. Visual check by the user.

**Outcome (2026-10-30).** Settings › Code checks has three parts: Languages
(one switch per language, with its extensions), Language servers (the
automatic download switch, moved from the panel) and Logs. The switches act
through the code checks store (`setLanguageEnabled`, `setAutoInstall`), so an
open project is rediscovered or a download starts at once; their state comes
from the saved preferences. The panel's Tools setup says whether missing
servers are downloaded automatically or offered for review, and links to
Settings › Code checks. Settings now stacks above the file dialog (z-index
1250, below toasts) so the link can open it there; the file dialog's own
shortcuts other than Escape still work behind it. Tests: `settings.test.mjs`
and `code-languages.test.mjs`; the visual check is the user's.

## CL-08 — Contract, docs and gate

**Goal.** The change is documented and checked as a whole.

- `code-diagnostics-contract.md`: the `disabledLanguages` field, its bounds,
  the capability and the `disabled` provider notice.
- Update `code-diagnostics-zed-model.md` where it says every language is
  discovered, the plan's status, this file and the trackers.
- Full gate: Go tests, cross-platform vet, frontend tests and svelte-check.

**Done when.** The gate passes and the docs match the code. Native
Windows/Linux and real remote host runs are recorded as not done.

**Outcome (2026-10-30).** The contract has an R11 section (field, bounds,
capability, `disabled` rows) plus the method table, capability list and bounds
table; the editor-model doc says discovery follows Settings and where the
auto-install switch moved; the plan has a status block with the differences
from it. Gate: `go test ./internal/... ./cmd/...`, `GOOS=windows` and
`GOOS=linux` `go vet`, `npm test` (944 pass, 1 existing Chrome skip),
`svelte-check` with no errors and `npm run build` all pass. Not done: native
Windows and Linux runs, a real remote (SSH/WSL) host, and the visual check of
Settings and the panel.
