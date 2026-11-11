# Chat adapter setup

## Goal

Settings › Coding agents › Chat should say, per agent, whether chat is ready
and offer one button when it is not. Today each missing adapter shows a
sentence on why chat needs it, why it was not found, an **Install…** button
that opens a review card, and an `npm install -g …` command to copy. The user
asked for:

1. a shorter section;
2. the app's own pinned adapter copy, for stability, with no mention of a
   global npm install;
3. **Install** that installs on the first click, with no second question;
4. **Update** when the app's copy is older than the version this app pins.

## Design

### Which adapter runs

`findAdapter` (`internal/engine/chat_launch.go`) changes its order. On a host
where the app can install the agent's adapter (the managed-tools installer
runs there and the agent has a catalog entry: Claude Code and Codex on Linux
and macOS):

1. the `[chat.adapters]` override in config.toml, as today;
2. the app's copy: the pinned version, else the newest valid older copy;
3. nothing else. A copy on PATH or in the usual npm locations is no longer
   used, so every chat runs a version this app was tested with.

Where the app cannot install (OpenCode, whose `opencode acp` is built in, and
the Windows desktop's own chats), the PATH lookup stays as it is.

### Older copies

Each version installs in its own folder,
`managed-tools/<id>-<version>-<os>-<arch>/`, so an app release that bumps the
pin leaves the old copy beside the missing new one. Managed tools gain a
read-only lookup for an adapter that returns the pinned copy when it is valid,
else the newest valid older copy, with its version. An older copy is valid
when its receipt names that folder's version and the adapter policy, the main
script's SHA-256 matches the receipt, and its package's `package.json` names
that version. Its locked dependencies cannot be rechecked (the old lockfile is
not in this app), but they were checked against their hashes when it was
installed, and the folder stays private to the user.

Chats keep running the older copy until the user updates, so a release that
bumps the pin never breaks chat.

`ChatAgent` gains:

- `version`: the version of the app's copy that runs;
- `pinned`: the version this app installs;
- `outdated`: the copy that runs is older than `pinned`.

Old copies stay on disk after an update. Removing them is out of scope: each
is tens of megabytes and the pin changes only with app releases.

### UI

The Chat section becomes one short hint and one row per agent:

| State | Row |
|---|---|
| ready | **Claude Code** · Ready · 0.106.0 (the path in a tooltip) |
| set in config.toml | **Claude Code** · Ready · set in config.toml |
| outdated | **Claude Code** · 0.105.0 · **Update** |
| missing, installable | **Claude Code** · Not installed · **Install** |
| installing | **Claude Code** · Installing… · **Cancel** |
| missing, not installable | **Claude Code** · Not found, and a short reason |
| failed | the row as before, the installer's one-line message under it |

**Install** and **Update** install on the first click: the frontend asks the
host for a proposal and, when it comes back for the same host and tool,
installs it with its token at once. The single-use token, host checks and
pinned lockfile stay as they are; only the review card goes. A tooltip on the
button says what it does ("Installs <package> <version> for this app on
<host>"). Code checks keep their review card.

The chat start error no longer names an npm command: it points to
Settings › Coding agents.

## Decisions

1. **The app's copy wins over the user's.** This reverses
   [acp-adapter-install](acp-adapter-install.md) decision 3 ("the user's own
   adapter wins"). A user who wants another adapter sets it in
   `[chat.adapters]`. Someone who has relied on a global install clicks
   Install once.
2. **One click is the consent.** The button names what it installs. The
   review card was a second question about the same thing.
3. **Update means "to the version this app pins".** The app never asks npm
   for the latest release: the pin is what keeps chats stable, and new
   versions come with app releases.
4. **No npm command in the UI.** Hosts where the app cannot install keep the
   PATH lookup but show only a short reason.
5. **Old servers.** A host on an older agent-sessions sends no `version`,
   `pinned` or `outdated`; the row then shows Ready, or Install when it sends
   `installTool`, as now.

## Risks

- **Users with a working global adapter** see Not installed after updating
  the app. The row's Install fixes it in one click; the release notes should
  say so.
- **An older copy's dependencies are not rechecked.** Covered by the private
  folder checks; the main script's hash and the package version are.
- **One click downloads tens of megabytes** (and Node.js when the host has
  none new enough). Progress and Cancel stay.
- **Windows desktop chats** still need an adapter on PATH, since the
  installer refuses Windows. Chats from WSL or SSH hosts get the button.

## Release criteria

- Go tests: the app's copy runs before one on PATH; no PATH fallback where
  the app installs; the PATH lookup where it cannot; an older valid copy is
  found and reported outdated; a damaged older copy is not used; the start
  error names Settings, not npm.
- Frontend tests: each row state; Install and Update send `propose` then
  `install` with the proposal's token, and nothing when the proposal is for
  another host or tool; no copy command is rendered.
- `make lint` reports nothing new; `go test -race` for the changed packages,
  `npm test`, `npm run check` and `npm run build` pass.
- A real install and an update from an older copy, followed by a chat, on a
  Linux host (CAS-05), with every check that did not run marked so.

Tasks: [chat-adapter-setup](../tasks/chat-adapter-setup/README.md).
