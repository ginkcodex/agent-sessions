# CPC-04 — Read-only MCP and plugin inventory backend

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** CPC-02
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Implement the inventory contracts verified in CPC-02
([contract](../../plan/chat-plugin-commands-contract.md)). `/mcp` needs no
backend, because both adapters handle it. Expose a host-side, bounded,
read-only `/plugin` listing:

- Claude: `claude plugin list --json`.
- Codex: `codex plugin list --json`, with a timeout of at least 90 s and
  cancellation.

Run it with the session cwd, resolved CLI, login environment and selected
host. O‍penCode returns "unsupported" (see [CPC-09](CPC-09.md)). Return typed safe inventory fields and
explicit unsupported/error states. No mutation commands or credential reads.

## Likely files

- `internal/engine/`, `internal/provider/`, existing launch/loginenv helpers
- `internal/app/`, `internal/rpc/`, remote server handlers
- `frontend/src/lib/api.ts`, types and mock API

## Acceptance criteria and tests

- Plugins without commands and disabled installations remain visible.
- Distinguish native installed/enabled state from known loaded session state.
- Timeouts, cancellation, missing CLI, nonzero exit, malformed/oversized output
  and older CLI support are covered without falling back to model prompting.
- Fixed argv and allowlisted operations; no shell interpolation of user input,
  secret output, credentials/config payloads or installation side effects.
- Tests verify local/remote routing and workspace isolation with fake runners;
  run focused backend suites and RPC compatibility tests.

## Evidence

Done:

- `provider.PluginLister` and `PluginList` (fixed args, timeout, parser);
  Claude (`plugin list --json`, 30 s) and Codex (`plugin list --json`,
  2 min) implement it. O‍penCode has none and answers
  `{supported: false, plugins: []}`.
- `Service.ChatPlugins` runs `<cli> plugin list --json` through
  `manage.CaptureCLI` (no shell, hidden console, 64 MiB stdout cap, one
  redacted stderr line) in the chat's launch directory and environment. The
  CLI is the bridge's `codex` for a DaemonBridge adapter, else the adapter's
  CLI variable, else the CLI new sessions start. A parse failure is an
  "update it" error, never an empty list.
- `ChatManager.Plugins` lists for a live chat and is cancelled when the chat
  closes (`ErrChatClosed`).
- RPC `chatPlugins` behind the `chatPlugins` capability (an older host gets
  `ErrNoChatPlugins`); it goes through the chat ownership check. App, Wails
  binding, `api.ts` and the mock API expose `chatPlugins(chatId)`.
- Installed/enabled come from the CLI (`enabled`, Claude's `scope`); what
  the session loaded is its command catalog from CPC-03. Combining the two
  in the view is CPC-05.

Tests run:

- `go test -race ./internal/engine/ ./internal/rpc/`: pass.
  `TestServiceChatPlugins` covers argv/dir/env, bridge and fallback CLI,
  unsupported agent, nonzero exit, older CLI output, timeout and unknown
  agent. `TestChatManagerPlugins` covers unknown chat, agent/dir routing and
  close during a listing. RPC tests cover the round trip, another connection
  refused, no chat backend, and an older host.
- Parser tests in `internal/provider/claude` and `internal/provider/codex`
  (enabled and disabled, empty, older CLI text, malformed and missing
  fields). The listing carries no command data, so a plugin without commands
  is listed like any other.
- `go vet ./...` and `go test ./...`: pass. golangci-lint reports nothing in
  the changed files.
- `npm run check`: 0 errors. `npm test`: 1476 pass, 0 fail.

Not checked:

- No real `claude` or `codex` listing was run through the app, and no remote
  (WSL/SSH) host was tried.
- Output over the 64 MiB cap is reported as "could not be started" rather
  than "too large".
