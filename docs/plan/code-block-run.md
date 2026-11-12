# Run a code block's command

## Goal

When an agent suggests a shell command in a fenced code block, as in "run it
yourself with: ```bash …```", the user can run it with one click. A **Run**
button sits left of **Copy** on the block's header. It opens a shell tab in
the internal terminal panel, in the session's directory, and runs the
command there. The output stays in that tab; the user copies what they want
back into the chat.

The chat cannot run commands itself: a `!` line typed in the composer
reaches the agent as text (the ACP adapter has no shell mode), and
`<bash-input>` is a record of a command the Claude Code terminal app has
already run, not a request. The terminal panel already runs shells properly,
with the user's startup files, so the button hands the command to it.

## Design

### Which blocks

A fenced block in an assistant message's text whose language is `bash`,
`sh`, `shell` or `zsh` and whose command is not empty. It shows in the
transcript and in a live chat's tail, only where the terminal panel is
available and the session has a directory. Docs, file previews, hover
cards, reasoning, user and system messages keep Copy only.

### The command

`runnableCommand(lang, code)` (pure, in `lib/runCommand.ts`):

- Removes control characters other than tab and newline (C0, DEL and C1).
  xterm's paste does not, and an escape sequence in the text could end the
  bracketed paste early and run what follows.
- Removes one leading `!` (Claude Code's shell-mode prefix, which agents
  quote), with the spaces after it.
- Removes the `$ ` prompts when every non-empty line starts with one.
- Trims surrounding blank lines and trailing spaces. Empty means no button.

The renderer puts it in the button's `data-command`, HTML-escaped like
Copy's `data-code`.

### Running it

`terminals.runCommand(command, dir)` opens a shell tab as the "New shell"
action does (`newShell(dir)`: the window's selected host, the directory or
else home), then types the command at the shell's first prompt: it pastes it
through the tab's sink (bracketed paste when the shell asks for it, so a
multi-line block arrives whole) and sends Enter. The first prompt is the
shell's first directory report (OSC 7, `setCwd`), which the panel's shells
emit from their prompt hook, after the startup files and hooks such as
mise's have run. A shell that never reports one (an unsupported shell) gets
the command after 5 s. The shell stays open after the command, with its
output, its history entry and any `cd` it made.

### Wiring

`renderMarkdown` takes a `runnable` option; `TextPart` passes it when it has
an `onRunCommand` handler, and `handleMessageClick` calls the handler for a
`.run-code-btn`. `TranscriptView` (session directory from the selected
session's meta, unless `cwdMissing`) and `LiveTail` (the chat's directory)
pass the handler to `MessageBubble`, which hands it to assistant text parts
only.

## Decisions

1. **A new tab per click.** An existing tab may be busy or running an agent.
2. **Run, not just type.** The click is explicit and the whole command is
   visible in the block. The shell stays open, so nothing is lost.
3. **Typed into the user's own shell**, not run by a hidden `sh -c`: aliases,
   version-manager hooks for the project directory and history work as when
   the user types it, and a `cd` in the command stays.
4. **POSIX shell languages only.** On this computer under Windows the tab
   runs PowerShell; a bash block may fail there, and PowerShell blocks are
   not offered yet.
5. **No result capture.** The user copies the output from the terminal, as
   with any shell tab.

## Risks

- A one-click run of agent-written text. The user sees the full command in
  the block before clicking; control characters are removed; the shell tab
  stays open to show what happened.
- A shell slow to print its first prompt without a directory report gets
  the command as type-ahead after 5 s, which shells read when ready.

## Release criteria

- A bash block in an assistant message, in a past transcript and in a live
  chat, shows Run left of Copy; other blocks and other places do not.
- Run opens a shell tab in the session's directory, the command runs at the
  first prompt with mise's project tools, and the tab stays open.
- A block quoting `! cmd` runs `cmd`; a multi-line block runs as a whole.
- `npm test` and `npm run check` pass.
