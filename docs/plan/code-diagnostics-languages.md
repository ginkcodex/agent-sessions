# Code checks: choosing languages (2026-10-30)

**Status: implemented 2026-10-30** ([tasks CL-01…CL-08](../tasks/code-languages.md);
wire details in the [contract, R11](code-diagnostics-contract.md#r11-additive-language-settings-2026-10-30)).
Where the code differs from the plan below:

- The engine keeps `AttachCodeDiagnostics(ctx, dir, version)` and adds the
  optional `CodeDiagnosticsLanguagesBackend.AttachCodeDiagnosticsWithout(ctx,
  dir, version, disabled)`, like other additive backend features; the RPC
  server advertises `codeDiagnosticsLanguages` only for backends with it.
- The code checks store, not `api.ts`, reads the preference and sends it with
  each attach.
- Settings › Code checks puts the language switches under "Languages" and the
  auto-install switch under "Language servers", then "Logs".
- `coverage.partial` still counts disabled providers, so an approved project
  with a language off reads "partial".
- Not tested natively on Windows or Linux, nor against a real remote host.

## Goal

Users rarely work in every language the app supports. Today every project gets
every language: discovery, download offers, approval entries, servers and
"not installed" notices for tools the user never uses. Settings gets one switch
per language. A language that is off is left out of code checks and Go to
Definition everywhere, local and remote.

We do not add user-defined language servers. The supported languages stay a
closed list of servers the app knows and pins; adding one is a code change.

## Behavior

- **Languages, not tools.** One switch each:

  | Language | Providers | Managed tools |
  |---|---|---|
  | Go | `gopls`, `go-vet`, `go-syntax` | gopls |
  | TypeScript / JavaScript (with Vue) | `typescript` | vtsls, Vue TypeScript plugin |
  | Svelte | `svelte`, `svelte-check` | svelte-language-server |
  | Python | `ruff`, `pyright` | Ruff, Pyright |
  | Rust | `rust-analyzer`, `cargo-check` | rust-analyzer |

  Node.js is only a runtime and follows the languages that need it.
- **On by default.** Nothing changes until the user turns a language off. The
  setting stores the languages that are off, so a language added in a later
  release starts on, like the others did.
- **Off means left out before approval.** Discovery skips the language: no tool
  lookup, no preparation steps, nothing in the proposal or its fingerprint, no
  download offer, no auto-install, no server. The project scan is metadata only
  and still runs once for the languages that are on.
- **Off is shown, not hidden.** When the project has files of a language that is
  off, its providers are reported with the existing contract value
  `availability: "disabled"` and the notice "Off in Settings". A project without
  such files reports nothing for it. Go to Definition in such a file says the
  language is off in Settings instead of "no language server".
- **Changing the setting.** The shown project is rediscovered at once
  (`codeDiagnostics.rediscover()`); others pick it up when next shown.
  - Turning a language **off** removes it from the proposal; the old
    fingerprint's leases end and its servers stop.
  - Turning a language **on** adds it to the proposal. Since 2026-10-28 a
    project's approval carries over to a changed fingerprint
    ([code-diagnostics-trust.md](code-diagnostics-trust.md)), so in a project the
    user already approved it starts without asking again. The user just chose
    this; Settings says so next to the switches.

## Where the setting lives

- **The desktop app, in `localStorage`**, next to "Download missing language
  servers automatically" (`preferences.svelte.ts`). One setting covers every
  host.
- **Sent as names with each attach**: `attachCodeDiagnostics` gains
  `disabledLanguages: string[]` (at most 16 names of at most 32 bytes). It can
  only narrow discovery, never add a tool or path, so it keeps the rule that
  requests only name things. Unknown names are ignored.
- **Older servers** ignore the field and check every language, as today. A new
  capability, `codeDiagnosticsLanguages`, lets the UI say "This host checks every
  language until its server is updated" instead of silently ignoring the switch.
  `contractVersion` stays 1: the field is additive and `disabled` already exists.

## Changes

### 1. Language registry (structure only, no behavior change)

One table in `internal/codediagnostics/languages.go` names each language, its
provider IDs and its managed tools. Today these are repeated by hand:

- `engine/code_diagnostics.go`: the `discoveryFailed` ID list.
- `engine/managed_tools_contract.go` and `managedtools` (`proposable`, the
  discovery tool list, the npm cases in `discovery.go` and `install.go`).
- `live_owner.go`: the `switch` choosing a bridge. The six `*Bridge` types call
  `handleOwnedPush` with different arguments; one bridge driven by the provider
  entry replaces them.
- `workspace_adapters.go`: the provider list and the two `switch provider.id`
  blocks (config markers, discover call) become fields of the live entry.
- `live_discovery.go`: `DiscoverLive` and `DiscoverLocalLive` loop over the
  table instead of listing each language.

Each language keeps its own `DiscoverX` function; only the wiring moves. The
existing `codediagnostics` and engine tests are the check that nothing changed.
The frontend keeps its own list of the five language names and labels
(`codeLanguages.ts`), named in the contract doc; it never needs provider lists.

### 2. Filtering discovery

- `rpc/code_diagnostics.go`: `codeDiagnosticsParams.DisabledLanguages`,
  validated and passed to `AttachCodeDiagnostics`. The capability is advertised
  in `rpc/types.go`.
- `engine.AttachCodeDiagnostics(ctx, dir, version, disabled)`. The diagnostics
  owner keeps the set per attachment (beside `o.dirs`), because rediscovery
  after approved preparation (`startPreparation` → `discoverAll`) must use the
  same set.
- `discoverAll`, `discoverCheckersIn` and both live discoveries skip disabled
  languages and add their `disabled` registrations when the scan saw their
  files. `discoveryFailed` leaves them out.
- `app.AttachCodeDiagnostics` and the offline/unsupported backends gain the
  parameter; `api.ts` passes `preferences.disabledLanguages`.

### 3. Settings and display

- The Settings section "Code check logs" becomes "Code checks": the language
  switches, the auto-install switch (moved here from the panel, which keeps a
  link to it), then the logs.
- The note beside the switches: turning a language on starts its checks in
  projects already approved; turning it off stops them.
- `codeCheckLabels.ts`: a `disabled` provider reads "Off in Settings".
  `codeNavigation.ts` gives the same reason for Go to Definition.
- The panel's "Tools setup" rows and its setup text list only languages that
  are on.

## Tests

- Go:
  - A disabled language yields no tools, preparation or proposal entry, and a
    `disabled` registration only when the project has its files.
  - A fingerprint change on turning a language on carries the project's grant
    over; turning it off ends the old leases.
  - Rediscovery after preparation keeps the attachment's set.
  - RPC: a missing field means all languages; unknown and oversized names are
    handled; the capability is advertised.
  - Registry: every provider ID and managed tool belongs to exactly one language.
- Frontend:
  - The preference persists and defaults to all on.
  - Attach passes it; changing it rediscovers the shown project.
  - "Off in Settings" in the panel and for Go to Definition.
  - The old-host notice when the capability is missing.
- `GOOS=windows` / `GOOS=linux` vet, as for every code-checks change.

## Not included

- Per-project overrides. A global switch covers "I don't use Rust"; per-project
  choice can come later if asked for.
- Switches for single tools (for example Pyright without Ruff).
- User-defined language servers.
