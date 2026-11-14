# Tool file links

## Goal

In the session content (Activity and All), a tool card for a file tool shows
the file in its header: `Edit /…/docs/plan/chat-message-queue.md`. The path is
plain text today. Clicking it should open that file in the file viewer, the
same way a file link in the agent's text already does.

## Design

- `toolFileTarget(name, input)` in `toolPresentation.ts` returns the file a
  tool call works on, or nothing:
  - `file_path`, `filePath` or `notebook_path` name a file whatever the tool
    is called (Claude Code's Read/Edit/Write/MultiEdit/NotebookEdit,
    OpenCode's read/edit/write, ACP tool input that carries them).
  - `path` names a file only for tools called read, edit, write, multiedit or
    notebookedit (any case). Elsewhere (Grep, Glob, LS) it is usually a
    directory, so it stays text.
  - Claude Code's `Read` with a positive whole `offset` opens at that line.
  - The path is taken as it is, without URI decoding: tool input is not a
    Markdown destination, so a `%` in a file name stays a `%`.
- `ToolPart` shows that path as a button in place of the summary text, when
  the transcript gives it an `onOpenFile` callback. Without the callback, or
  for any other tool, the summary stays plain text, so a card looks the same.
  The button keeps the summary's look (monospace, ellipsis), with an underline
  and pointer on hover and a focus ring, and its tooltip says
  `Open <path>`.
- `MessageBubble` and `LiveTail` pass `onOpenFile` down. `TranscriptView`
  opens the file through the same path as message file links
  (`createTerminalFileOpener`, `messageFileRecovery`, `revealFileLink`). The
  part that opens a parsed target is split out of `openMessageFile`, so both
  callers use it.

## Decisions

- Open in the app's own file viewer, as message links do, not in an external
  editor. The viewer works the same for local, WSL and SSH sessions.
- The backend stays the authority. `ResolveTerminalFile` only allows a file
  under a known workspace or session folder and only regular files. A path
  outside those (for example `~/.claude/...`) shows the existing
  "Could not open message file" toast with the reason.
- Relative paths resolve against the session's cwd, as message links do.
- Codex `apply_patch` names files only inside the patch text and may touch
  several. Left out here; a later task can link each file in the patch.
- No line target for Edit/Write: finding `old_string` in the current file
  would be a guess once the file has changed since.

## Risks

- A tool whose `path` is a directory but is called read/edit/write: the
  backend refuses it with "not a regular file". Acceptable, and the same as a
  message link.
- The header is a flex row with tight sizing (see `tool-card.test.mjs`). A
  button must keep the summary's truncation, or a long path pushes the status
  badge out.

## Release criteria

- Unit tests for `toolFileTarget`: Claude, OpenCode, notebook and string
  input; `path` on file tools only; Read's offset; `%` kept.
- Tool-card tests: the path is a button that calls `onOpenFile` with the path
  and line; no button without the callback or for a non-file tool.
- Frontend tests, `svelte-check` and the production build pass.
- Record whether a click was checked in the running app.
