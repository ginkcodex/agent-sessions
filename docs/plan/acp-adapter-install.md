# One-click install of ACP chat adapters

## Goal

A chat with Claude Code or Codex needs the agent's ACP adapter
(`claude-agent-acp` or `codex-acp`). Today a missing adapter only shows an
`npm install -g …` command to copy and run by hand. This feature adds an
**Install** button: after the user reviews what will be downloaded and where it
goes, the app installs a pinned copy of the adapter for its own use, on the
host the chat runs on (this computer, a WSL distribution or an SSH host).

Zed works this way: it downloads the adapter into its own data folder and runs
it with Node.js. This app already does the same for language servers
([tool-installation](tool-installation.md),
[code-diagnostics-zed-model](code-diagnostics-zed-model.md)), and this feature
reuses that installer instead of adding a second one.

## Why

Chat asks for an extra global npm install that Zed does not ask for.
[acp-chat](acp-chat.md) decision 2 ("no automatic installs") kept the first
release from downloading anything. The managed-tools installer already gives a
consented, pinned and checked way to install npm packages, so the adapters can
use it.

## What exists

`internal/managedtools` installs npm packages after explicit consent:

- a closed catalog: each npm entry embeds its `package.json` and
  `package-lock.json`, and installs with `npm ci --ignore-scripts`, so every
  package is checked against its sha512 and no install script runs;
- Node.js from the host when it is new enough and has npm, else a pinned
  Node.js archive from nodejs.org, checked against its SHA-256;
- a single-use proposal token (2 minutes) that binds the host, connection,
  tool, version, source, integrity and destination, shown on a review card;
- installs in `<app data>/managed-tools/<name>-<version>-<os>-<arch>/`, staged
  and renamed into place, with a receipt;
- the `ManagedTool` engine contract (`status`, `propose`, `install`, `cancel`,
  `dismiss`), the `managedTool` RPC method for SSH and WSL hosts, and the
  `ManagedTool` app binding, which checks the actual host identity at each
  layer;
- macOS and Linux on amd64 and arm64 only. Windows is refused.

## Design

### Catalog entries

Two new npm entries, with short IDs because the contract caps tool names at 32
characters:

| ID | Package | Version | Main |
|---|---|---|---|
| `claude-agent-acp` | `@agentclientprotocol/claude-agent-acp` | 0.106.0 | `node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js` |
| `codex-acp` | `@agentclientprotocol/codex-acp` | 2.6.0 | `node_modules/@agentclientprotocol/codex-acp/dist/index.js` |

These are the versions the ACP-02 spike tested. Each gets an embedded
`package.json` and `package-lock.json` (lockfile v3), generated with
`npm install --package-lock-only --ignore-scripts`, which resolves the tree
without running any package code.

**Optional dependencies are left out.** The Claude adapter depends on the
Claude Agent SDK, which pulls a native Claude binary per platform as an
optional dependency (about 350 MB on linux-x64); the Codex adapter pulls a
native Codex binary the same way (about 430 MB). The app never uses them: chat
already points the adapter at the user's own CLI through
`CLAUDE_CODE_EXECUTABLE` and `CODEX_PATH`. So these entries install with
`npm ci --omit=optional`, as their own npm policy
(`npm-ci-lockfile-ignore-scripts-omit-optional-v1`), and the install needs the
agent's CLI to be present. Whether each adapter starts without its optional
packages has to be checked with the real adapters (AAI-05).

**Node.js 22 or later.** `claude-agent-acp` declares `engines.node >= 22`. The
installer takes a minimum Node.js major per entry: these entries use the host
Node.js only when it is 22 or later, and otherwise the pinned Node.js 24.

OpenCode has no entry: `opencode acp` is built into OpenCode itself, and
installing the agent is out of scope.

### Finding a managed adapter

`ChatAgents` and `ChatLaunch` look for an adapter in this order:

1. the `[chat.adapters]` override in config.toml;
2. PATH and the usual install locations, as today (the user's own copy wins);
3. the managed copy: `Discover("<id>")` gives the main script; the command is
   `<node> <main>`, with the managed or host Node.js that the install used,
   and that Node's directory first on PATH.

`ChatAgent` gains:

- `installTool`: the catalog ID when the host can install the adapter (a
  managed install is possible on this platform and the agent has an entry);
- `managed`: true when the adapter that would run is the managed copy.

The managed-tools `status` action reports the adapter IDs as well, so the UI
learns about an install's result the same way it does for language servers.

### UI

In Settings › Coding agents and in the "Set up chat…" dialog, a missing
adapter with `installTool` shows **Install** next to the copy command:

1. **Install** proposes the tool. The review card shows the package and
   version, the npm registry, the lockfile hash and the policy (scripts off,
   optional packages left out), which Node.js runs it and whether Node.js is
   downloaded, and the destination, on the named host.
2. **Install** on the card installs; **Not now** dismisses. Progress shows
   with a Cancel button.
3. When the install ends, the chat agents are reloaded. A failure shows the
   installer's sanitized message and keeps the copy command.

The card is the existing managed-tools review card, moved into a shared
component so code checks and chat show the same consent.

There is no automatic install for adapters: the code-diagnostics setting that
installs language servers on its own does not apply to them.

## Decisions

1. **Reuse managed tools.** One installer, one consent model and one RPC path
   for language servers and adapters. No second installer.
2. **Consent first.** Nothing is downloaded until the user approves a
   proposal for the named host. This amends [acp-chat](acp-chat.md) decision
   2: the app still never runs `npx` and never installs globally, but it may
   install a pinned copy for its own use after the user approves.
3. **The user's own adapter wins.** A copy on PATH or in config.toml runs
   before the managed one, so installing it globally or setting a path keeps
   working as before.
4. **Pinned versions.** The app updates an adapter only when a new app release
   bumps the catalog entry. A newer pinned version installs beside the old
   one, after a new consent.
5. **No Windows install yet.** The managed-tools installer refuses Windows.
   On the Windows desktop app a local chat keeps the copy command; WSL and SSH
   hosts are Linux or macOS and get the Install button. Extending the
   installer to Windows is a separate plan.

## Risks

- **The adapter may need its optional packages.** If either one fails to
  start without them, its entry keeps optional packages (and the download
  size grows), decided in AAI-05 with the real adapter.
- **Node.js version drift.** A host Node.js older than 22 means the pinned
  Node.js download; the review card says so.
- **Lockfile churn.** Bumping an adapter means regenerating its lockfile and
  updating the counts the catalog tests check.
- **Large first install.** Even without optional packages the tree is tens of
  megabytes; progress and Cancel cover it.

## Release criteria

- Both entries are in the catalog with embedded lockfiles; catalog, npm and
  discovery tests cover them, including the omit-optional policy and the
  Node.js 22 minimum.
- `ChatAgents` reports `installTool` and `managed`, and `ChatLaunch` starts
  the managed copy with the right Node.js; tests with fake installs.
- The managed-tools `status` reports the adapters, locally and over RPC.
- The Install flow (propose, review, install, cancel, failure, reload) works
  in the UI against the mock, with tests; an old server without the entries
  shows only the copy command.
- `make lint` reports nothing new, and `make test`, `npm test`,
  `npm run check` and `npm run build` pass.
- Real installs and a chat with each managed adapter on Linux, macOS, a WSL
  distribution and an SSH host are recorded in `acp-adapter-install-qa.md`,
  with every check that did not run marked as such.

Tasks: [acp-adapter-install](../tasks/acp-adapter-install/README.md).
