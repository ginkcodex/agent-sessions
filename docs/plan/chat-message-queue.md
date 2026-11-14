# Chat message queue

## Goal

While a chat's agent works, the user cannot send anything: the composer
takes text, but Send turns into Stop, and the engine refuses a prompt
(`ErrChatBusy`). The user should be able to:

- **Queue** a message while the agent works: Enter adds it to a queue shown
  above the composer. When the turn ends, the next queued message is sent as
  the next turn.
- **Send now**: put a queued message into the running turn, so the agent
  reads it at its next step, without stopping the turn. This is ACP
  steering, which every chat adapter supports today.

## What the adapters support

Checked in the managed adapters' code (2026-11-14):

| Adapter | `_session/steering` | A second `session/prompt` during a turn |
|---|---|---|
| claude-agent-acp 0.106.0 | Yes; `_meta.steering.supported` at initialize | Queued by the adapter (`promptQueueing`) |
| codex-acp 2.6.0 | Yes; Codex's `turn/steer` | Waits for the running turn |
| opencode 2.2.7 (`opencode acp`) | Yes; `steering: {supported: true}` | Not checked |

The steering request is the same for all three:

```json
{"method": "_session/steering",
 "params": {"sessionId": "…", "prompt": [ContentBlock…],
            "_meta": {"steering": {"idleBehavior": "promptRequired"}}}}
```

The reply is `{"outcome": "injected"}` when the message joined the running
turn. With `idleBehavior: "promptRequired"`, an idle session answers
`{"outcome": "promptRequired", "reason": "noRunningTurn"}` and starts
nothing, so the client keeps the message and sends it as a normal
`session/prompt`. Without the opt-in the adapters start a turn on their own
(`startedNewTurn`), which the engine would not track. An adapter that does
not advertise `_meta.steering.supported` (older versions, or one the user
installed) gets no steering: its queue only sends at turn end.

## Design

### Engine (`internal/engine/chat.go`)

The queue lives in the engine, beside the chat's permissions and
questions, so a reloaded window gets it back from `ChatList`.

- `Prompt(chatID, text, atts) (ChatPromptResult, error)`. An idle chat
  starts the turn as today. A working chat appends the message to the queue
  and returns its ID in `ChatPromptResult.Queued`. The attachments are
  checked against the agent's prompt capabilities when queued, so a refusal
  comes back at once. `ErrChatBusy` is no longer returned.
- A turn that ends with `end_turn` and no error starts the next queued
  message as a new turn. A turn that ends any other way (Stop, an error, a
  login, `max_tokens`) leaves the queue alone: it is **paused**. A new
  message sent to an idle chat starts at once; a paused queue goes on
  after that turn ends normally.
- `SendNow(ctx, chatID, queuedID)` takes the message out of the queue:
  - an idle chat starts a turn with it;
  - a working chat whose agent steers sends `_session/steering` with
    `idleBehavior: "promptRequired"`. `injected` emits a `steered` update.
    `promptRequired` (the turn ended meanwhile) puts the message back first
    in the queue, where the turn's end sends it (or leaves it paused). An
    error puts it back first too and is returned;
  - a working chat whose agent does not steer returns `ErrSteerUnsupported`.
- `Unqueue(chatID, queuedID) (ChatDraft, error)` removes a queued message
  and returns it with its attachments, for Remove and Edit.
- The turn's end waits for steering calls in flight, as it waits for
  permission answers, so a `steered` update never follows its turn's
  `turn_end`.
- Closing the chat drops the queue.

New data:

- `ChatQueuedPrompt{ID, Text, Attachments []ChatAttachmentInfo}`: a queued
  message as events and `ChatState` show it. `ChatAttachmentInfo` is an
  attachment's kind, name, MIME type and size, without its bytes, so events
  stay small.
- `ChatState.Queue`, `ChatState.CanQueue` (always true from this engine;
  false from an older host) and `ChatState.CanSteer`.
- Update kinds: `queue` (the whole queue after each change) and `steered`
  (the message that joined the turn, in `Prompt`). `turn_start` carries
  `Prompt` when the turn started from the queue.

### ACP client (`internal/acp`)

- `InitializeResponse.Meta`, and `Client.CanSteer()` reading
  `_meta.steering.supported`.
- `Client.Steer(ctx, sessionID, prompt) (outcome string, err error)`,
  always with `idleBehavior: "promptRequired"`.
- The fake agent (`acptest`) advertises steering unless
  `ACPTEST_NO_STEER=1`, and answers it: `injected` during a turn, reported
  in the turn as a chunk `steered:<text>`; `promptRequired` when idle.

### Server and app

- RPC capability `chatQueue`. `chatPrompt` returns `ChatPromptResult`; new
  methods `chatSendNow` and `chatUnqueue`. A client talking to an older
  host sees `CanQueue` false in `ChatState`, so the window does not offer
  the queue there.
- App bindings: `ChatPrompt` returns `ChatPromptResult`; new `ChatSendNow`
  and `ChatUnqueue`, local and remote as the other chat bindings.

### Frontend

- `chats` store: `Chat.queue`, `canQueue`, `canSteer`. `prompt()` keeps a
  queued message's local copy (image bytes for the preview) by its ID, and
  uses it when that message's `turn_start` or `steered` comes. A
  `steered` update adds the user's message to the open turn of the live
  tail. New `sendNow()` and `unqueue()`.
- `ChatComposer`: while the agent works and the chat can queue, Enter and
  the send button queue the message; Stop stays beside them. The
  placeholder says "Queue a message for <Agent>…".
- `QueuedMessages.svelte` above the composer card: one row per message
  (its first line, a count of files) with **Send now**, **Edit** and
  **Remove**. Send now shows while the agent works only if it steers, and
  always while the queue is paused. Edit moves the message back into an
  empty composer. A paused queue says so: "Paused. Send now or send a new
  message to go on."
- End chat and Switch to terminal also ask when messages are queued, and
  say they will not be sent.

## Decisions

1. **Enter queues; Send now steers.** Queueing behaves the same for every
   agent; steering is the user's explicit choice, and only offered where
   the adapter supports it.
2. **The engine owns the queue**, so it survives a window reload and a
   message never waits on a window to be sent.
3. **Queue only after a normal end.** Stop means stop: a cancelled or
   failed turn pauses the queue instead of sending the next message.
4. **Always `idleBehavior: "promptRequired"`.** The engine starts every
   turn itself, so its turn tracking, `turn_start` and `turn_end` stay
   right.
5. **The engine's own queue, not the adapters'.** A message handed to the
   adapter can no longer be edited, removed or steered, and OpenCode's
   behaviour is not known.
6. **No edit in place.** Edit moves the message back to the composer;
   sending it queues it again at the end.

## Risks

- **Ordering of a steered message.** The `steered` update is emitted when
  the steering call returns; the agent's first reply chunks may come just
  before it. The saved transcript shows the right order once it reloads.
- **Adapters change the extension.** It is a `_session/` extension, not
  core ACP. Steering is offered only when advertised; an unknown outcome
  is treated as `injected`, and an error puts the message back in the queue
  and is shown.
- **A queued message that no longer fits.** A model switch could make an
  image refused at send time; the turn then fails, with its error, and the
  queue pauses.
- **A queued turn that starts before the window has the reply.** The
  window shows that message from the queue's info, so its image preview is
  missing. It is rare, because the reply comes as soon as the message is
  queued.

## Release criteria

- While the agent works, Enter queues the message; it shows above the
  composer and is sent when the turn ends normally. Stop pauses the queue.
- Send now on Claude Code, Codex and OpenCode puts the message into the
  running turn; the agent answers it in the same turn.
- Edit and Remove work; End chat with queued messages asks.
- A reloaded window shows the queue again. A host with an older server
  keeps today's behaviour.
- Tests: Go tests for the ACP client, engine, RPC and app; SSR and store
  tests for the frontend; `npm test`, `npm run check`, `npm run build`,
  `go test -race ./internal/acp/... ./internal/engine/... ./internal/rpc/...
  ./internal/app/...`.
