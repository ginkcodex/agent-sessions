# Chat attachments

## Goal

A chat sends text only. Let the user add images and files to a message as
extra context:

- a **+** button in the composer's toolbar opens the system's file picker
  (several files at once);
- files dragged onto the composer and images pasted into it are attached too;
- attached files show as chips in the composer (a thumbnail for an image),
  each with a remove button, until the message is sent;
- the sent message shows its attachments in the live tail, until the saved
  transcript replaces it.

This amends decision 3 of [acp-chat](acp-chat.md) ("prompts are text only;
images wait until an agent advertises `promptCapabilities.image`").

## Facts the design relies on

- ACP v1 `session/prompt` takes `prompt: ContentBlock[]`. Besides text, an
  agent may take:
  - `image {data (base64), mimeType, uri?}` when it advertises
    `promptCapabilities.image`;
  - `resource {resource: {uri, mimeType?, text | blob}}` (embedded context)
    when it advertises `promptCapabilities.embeddedContext`;
  - `resource_link {uri, name}` always.
- `initialize` from claude-agent-acp 0.106.0, codex-acp 2.6.0 and
  `opencode acp` 2.2.6 advertises `image` and `embeddedContext` (spike logs,
  [acp-chat-spike](acp-chat-spike.md)).
- The Claude API refuses an image over 5 MB (base64) or 8000 px on a side,
  and scales down anything over about 1568 px anyway.
- The desktop shell already gets dropped file paths (`files:dropped`, only on
  elements marked `data-file-drop-target`) and has native open dialogs; Wails
  offers `PromptForMultipleSelection`.
- Saved transcripts already show image and file parts (`PartFile`), so what
  the agent saves needs no new rendering.

## Design

### What a file becomes

A new package, `internal/chatattach`, turns a file into an `Attachment`
(`name`, `mimeType`, `size`, and either `data` for an image or `text` for a
text file, plus `uri`):

- **Images** (PNG, JPEG, GIF, WebP, by their bytes, not their name): sent as
  an `image` block. One over 3.75 MiB (5 MB in base64) or over 2000 px on its
  long edge is scaled to fit 2000 px and encoded again, as PNG when that fits
  and as JPEG (quality 85) otherwise; a GIF keeps its first frame. The
  standard library decodes PNG, JPEG and GIF; it cannot decode WebP, so a
  WebP over 3.75 MiB is refused.
- **Text files** (valid UTF-8 without NUL bytes, up to 512 KiB): sent as an
  embedded `resource {uri: file:///…, mimeType, text}`. An agent without
  `embeddedContext` gets the file as a text block instead, its name as a
  heading and its text fenced.
- **Anything else** (PDFs, archives, binaries) is refused: "only images and
  text files can be attached".
- At most 10 attachments per message and 24 MiB in total. Files are read up
  to 20 MiB; larger ones are refused before they are read in full.

The prompt's blocks are the message text (when there is any), then one block
per attachment, in the order they were added.

### Where it runs

- **Reading** happens in the desktop app, next to the files: the picker, a
  drop and a paste all give the app local paths or bytes, and it reads and
  prepares them there. For a WSL or SSH host the prepared attachments travel
  in the prompt; nothing is copied to the host's disk.
- **Checking** happens where the agent runs (engine, local or in `serve`):
  count and size limits again, the image's bytes against its type, and the
  agent's `promptCapabilities`. An agent without `image` gets a clear error,
  not a silent drop.
- `ChatState` gains `promptCapabilities`, so the composer can hide the image
  picker for an agent that takes none. A server too old for attachments sends
  no `promptCapabilities`, and the + button stays hidden for its chats.
- RPC: `chatPrompt` gains `attachments`, behind a new
  `Capabilities.ChatAttachments`. The client refuses to send attachments to a
  host without it ("reconnect to update it"), rather than have an old server
  drop them silently.

### App bindings

- `ChatPrompt(chatID, text, attachments)`: the existing binding with the
  attachments added.
- `ChatPickAttachments(chatID)`: opens a multi-select file dialog (in the
  chat's directory for a local chat) and returns the prepared attachments and
  the files it skipped, with why.
- `ChatAttachFiles(paths)`: the same for dropped paths.
- `ChatAttachImage(name, data)`: a pasted image (base64), prepared like a
  picked one.
- `app.Desktop` gains `OpenFiles` (multi-select).

### Frontend

- The composer keeps the attachments of the message being written. The +
  button sits at the left of the toolbar, before End chat. Chips sit above the
  text box; Send is enabled with text or with an attachment.
- The composer is a drop target (`data-file-drop-target="chat"`); the terminal
  panel ignores drops on it. Pasting an image attaches it; pasting text still
  pastes text.
- A failed send gives back both the text and the attachments.
- The prompt's live-tail item keeps the attachments' names (and an image's
  data, which the window already holds), shown as file parts until the saved
  message replaces it.
- The mock backend takes attachments and echoes how many it got.

## Decisions

1. **Embed, do not link.** Text goes in as embedded context rather than a
   `resource_link`, so it works for files outside the project and on remote
   hosts, where the agent cannot read the user's local path.
2. **No new dependencies.** Scaling uses the standard library (area
   averaging), so WebP over the size limit is refused rather than decoded.
3. **Fail loudly.** A file that cannot be attached shows a toast naming it
   and why; an agent that cannot take an image refuses the send with an
   error. Nothing is dropped from a prompt without telling the user.
4. **No new trust.** The user picks every file. The app reads only what the
   picker, a drop or a paste gave it; the agent gets content, not access.

## Risks

- **Adapters may treat blocks differently** (for example, a Codex or
  OpenCode version that turns embedded text into a file path). QA with each
  agent records what it does; the fallback text block covers agents without
  `embeddedContext`.
- **Large prompts.** 24 MiB fits the RPC frame (64 MiB) but costs tokens;
  the limits keep one message from filling the context.
- **The saved user message may differ** from the text sent (agents may save
  attachments as their own parts or inline text). The live tail then drops
  the prompt when the turn's last message is saved, as it does today.
- **WebKitGTK and WebView2 paste events** may not expose clipboard images the
  same way; paste is a convenience, the picker is the main path.

## Release criteria

- `chatattach` tests: images by their bytes, scaling (size and edge), PNG to
  JPEG fallback, the first frame of a GIF, WebP over the limit refused, text
  detection, refused binaries, limits, and blocks with and without
  `embeddedContext` and `image`.
- Engine and RPC tests: attachments reach the agent as blocks, are refused
  for an agent without `image`, and are refused by the client for a host
  without `ChatAttachments`.
- App tests: the bindings, skipped files with reasons, and the binding
  contract.
- Frontend tests: the + button per phase and capability, chips and removal,
  Send with only attachments, the failed-send restore, and the live tail's
  file parts.
- `go test -race ./...`, `npm test`, `npm run check`, `npm run build`;
  `make lint` reports nothing in changed files.
- QA with real agents (each sees an image and a text file) runs only with
  the user's approval to run the adapters, and is recorded as such.
