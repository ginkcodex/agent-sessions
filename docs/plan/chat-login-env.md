# Chat login environment

## Goal

A live chat's agent sees the same tools as the same agent in a terminal
tab. Languages that a version manager such as mise, nvm or pyenv puts on
PATH from the user's shell startup files are found when the agent runs a
command from a chat, on this computer and on WSL and SSH hosts.

## Problem

Terminal tabs run in the user's interactive login shell:

- On this computer the panel starts tabs with `term.LoginEnv`, the
  environment of `$SHELL -l -i -c env`.
- On a WSL or SSH host, `posixPanelAgentScript` runs the agent inside the
  user's interactive login shell in the project.

A chat starts its ACP adapter from the engine (`Service.ChatLaunch` →
`acp.Start`) with the process's own environment plus the adapter's
directory. That process is either the desktop app, which a desktop launcher
starts without any shell startup files, or the headless server, which the
app starts as `$SHELL -lc` (WSL: `${SHELL:-/bin/sh} -lc`). That is a login
shell but not an interactive one. Version managers are usually activated in
`~/.bashrc` or `~/.zshrc`, and Debian and Ubuntu's `~/.bashrc` returns at its
top when the shell is not interactive. On the reporting machine,
`bash -lc` has no mise directory on PATH; `bash -lic` has ten.

## Design

### A shared login environment reader

`term.LoginEnv` moves to a new package, `internal/loginenv`, so the engine
(which runs in both the desktop app and the headless server) can use it
without the terminal package. `loginenv.Env()` is unchanged in what it
returns: the environment of the user's interactive login shell, read once per
process, else the process's own environment; on Windows always the
process's own. The reader is hardened while it moves:

- **The shell** is `$SHELL`, else the account's shell from `/etc/passwd`,
  else `/bin/sh`. WSL's `wsl.exe --exec` sets no `$SHELL`, so a server on
  WSL would otherwise read nothing.
- **The directory** is the home directory, never a project, so a
  directory-aware hook (mise, direnv) in the startup files sees no project
  configuration.
- **The process tree** runs in its own session and is killed as a group
  when it ends or times out, with a bound on waiting for its output, so a
  background job left by the startup files cannot hold the read open.
- Output is bounded (1 MiB).

The panel's local tabs use `loginenv.Env`, as they used `term.LoginEnv`.

### Chats use it

`Service.ChatLaunch` lays the login environment over the process's own:
each login entry that is missing or different goes into `acp.Command.Env`,
which `acp.Start` appends to the process environment (the last value of a
key wins). The adapter's own directories still go first on PATH, now ahead
of the login PATH; directories only the process's PATH has follow at the
end, so an adapter found there (and so not prepended) keeps its
interpreter. Variable names compare as the platform does (without case on
Windows, where the login environment is the process's own anyway). The user's own `CLAUDE_CODE_EXECUTABLE` or
`CODEX_PATH` setting is looked up in the combined environment, so a
value exported from `~/.bashrc` counts too. The Codex daemon bridge
settings are applied last, as today.

Because the engine does this, it covers every host: the desktop app reads
the local user's shell, and the headless server on a WSL or SSH host reads
that host's user's shell.

## Decisions

1. **Overlay, not replace.** Variables the app has but the shell does not
   (set by the launcher, or the app's own `AGENT_SESSIONS_*` switches) are
   kept; where both have a key, the shell's value wins, as a terminal tab
   would see it.
2. **Read once, in the home directory.** This matches terminal tabs on this
   computer. The first chat after start pays for one shell startup (bounded
   by the 10 s limit); later chats pay nothing.
3. **The server keeps its `-lc` start.** Starting it interactive would
   change every server operation and risks job-control noise on a channel
   with no terminal.
4. **No per-chat shell wrapper.** Wrapping the adapter in `$SHELL -lic`
   would let startup-file output corrupt the ACP stream on stdout.
5. **Code checks keep their own PATH reader** (`loginShellPath`). It reads
   PATH only and has its own tests; merging it is not needed for this fix.

## Known limitation

Project-specific versions are not switched automatically. mise and similar
managers switch versions from a prompt hook, which an agent's commands never
run, so a chat gets the global versions from the home directory. The `mise`
executable itself is on PATH, so the agent can still use `mise exec -- …` or
`mise which`, and shims, where the user has them on PATH, resolve per
project. Terminal tabs on this computer have the same behaviour.

## Risks

- A slow or hanging startup file delays the first chat by up to 10 s; the
  timeout and group kill bound it, and the chat then starts with the
  process environment as today.
- An interactive shell without a terminal can print job-control warnings;
  they go to stderr, which is not parsed, and the marker skips stdout noise.

## Release criteria

- On a host whose version manager is activated only for interactive shells,
  a chat's agent finds the managed tools (for example `node --version` in a
  Codex or Claude chat) on this computer and on a WSL host.
- Terminal tabs on this computer behave as before.
- `go test ./internal/loginenv ./internal/engine ./internal/term ./internal/app`
  passes.
