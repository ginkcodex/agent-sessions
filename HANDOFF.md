# Handoff: Codex questions closing in the live chat

- **Reported:** "It doesn't work well with Codex, some options appears but then
  it's closed immediately. While some text still streaming from the agent"
- **Fix:** commit `87af9ea` (CQ-08). Task:
  [docs/tasks/chat-questions/CQ-08.md](docs/tasks/chat-questions/CQ-08.md).
  Plan: [docs/plan/chat-questions.md](docs/plan/chat-questions.md), sections
  "The question card → Withdrawn" and "Risks".
- **Status:** tested with unit tests and the mock app only. Real Codex has not
  run yet; that is CQ-07.

## What was going on

When Codex withdraws a question, the app gets the question's resolution with
outcome `withdrawn`, and the card used to disappear. Codex does this in two
ways. Both come from reading the code of codex-acp 2.6.0 and Codex 0.167;
neither was seen in a real session.

1. **Async questions.** Codex 0.167 prefers the `request_user_input_async`
   tool. Its prompt says: "The tool returns immediately without ending the
   turn or waiting for a reply". Codex keeps working and streaming.
   - codex-acp asks these when it sees `item/completed` for an agent message
     with `delivery: "async"`. The message is "Codex keeps working while you
     answer. An answer sent after it finishes is not delivered." and the
     request carries `_meta.codex.delivery: "async"`.
   - When the turn completes, codex-acp withdraws every open async question
     (`$/cancel_request`).
   - Code: `AsyncQuestionElicitations` in
     `~/.local/share/agent-sessions/managed-tools/codex-acp-2.6.0-linux-amd64/node_modules/@agentclientprotocol/codex-acp/dist/index.js`,
     around line 47826.
2. **Blocking questions with a time limit** (`request_user_input`).
   - The request carries `_meta.codex.autoResolutionMs`.
   - codex-acp waits for good only when the value is `null`. Otherwise it
     withdraws after `Math.max(0, autoResolutionMs ?? 0)`, so a missing value
     withdraws at once.
   - Code: `requestUserInputElicitation` in the same file, around line 40513.

There was also a bug on our side: the chat store's `turn_end` cleared every
question, including ones the backend was still waiting on.

My guess is that the report matches case 2 with a missing value, since the
card closed at once mid-stream. Case 1 fits too if the turn ended soon after
the question. Only a real run can tell.

## What changed (frontend only)

- **Store** (`frontend/src/lib/stores/chats.svelte.ts`):
  - A question resolved as `withdrawn` stays in `chat.questions` with
    `late: true`, after the line "The agent went on without an answer".
  - `turn_end` no longer clears questions; only their resolution does.
  - New `sendLate` and `dismissLate` methods.
- **Card** (`frontend/src/lib/components/chat/QuestionCard.svelte`):
  - A late card says "Codex asked" and "Codex went on without your answer.
    You can still send it as a message.", and keeps what the user picked or
    typed.
  - "Send as message" sends the answers as the next prompt. It is disabled
    while Codex is working and enabled once the turn ends.
  - Required questions don't block it. Hidden (secret) answers are never put
    in a message. Dismiss replaces Skip.
- **Helpers** (`frontend/src/lib/chatQuestions.ts`): `lateBlocker`,
  `lateMessage` and `asksWhileWorking`.
- **LiveTail:** late and async questions don't make the status say "waits for
  your answer".
- **Mock:** in a Codex chat, a prompt with "async" asks an async question
  while the reply streams; "withdraw" withdraws a blocking one after 8 s.

The Go backend did not change. `internal/engine/chat.go` `onQuestion` already
emits `question_resolved` with outcome `withdrawn` when the adapter sends
`$/cancel_request`.

## How to verify with Codex

Use a new chat with Codex (codex-acp 2.6.0), with questions enabled (the
default in the desktop app).

1. **Async question.** Ask something open-ended so Codex asks a question
   while it keeps working, for example "Before you change anything, ask me
   which approach I prefer, then keep investigating."
   - [ ] The card shows Codex's message ("Codex keeps working while you
     answer…") and the options.
   - [ ] While Codex streams, the status says "Codex is working…", not
     "waits for your answer".
   - [ ] If you answer before the turn ends, the answer goes through and a
     line "Answered: …" appears.
   - [ ] If you don't answer, then when the turn ends the card stays, marked
     "Codex asked", with your picks kept.
   - [ ] "Send as message" is disabled while Codex works, enabled after the
     turn ends, and sends "My answer to your question: - … — …" as your
     next message. Codex replies to it.
2. **Blocking question.** Try Plan mode, or ask Codex to "use
   request_user_input to ask me…".
   - [ ] If the card closes at once, it now turns late instead of
     disappearing. Note whether the card showed "Codex goes on without an
     answer after about N seconds". That hint appears only when
     `autoResolutionMs` is a positive number, so no hint plus an instant
     withdrawal points to a missing value (case 2).
   - [ ] If it waits, answering normally still works (Submit, then the
     "Answered: …" line).
3. **Edge cases.**
   - [ ] Dismiss removes a late card.
   - [ ] Cancelling a turn with an open question removes it, with no line
     and no late card.
   - [ ] Closing the chat clears all cards.
   - [ ] A secret question's answer never appears in a sent message.

Record results in CQ-07 or CQ-08's Evidence. Mark only what actually ran.

## If it still misbehaves

The real event order would settle it: `question`, `question_resolved`
(`withdrawn`/`cancel`), `turn_end`, and the request's `_meta.codex`. Sources:

- codex-acp's own log, and Codex's session log under `~/.codex/sessions/` for
  that chat. I was not allowed to read these without your approval.
- In the app, the chat events reach the store's `apply()` in
  `chats.svelte.ts`. A temporary `console.debug` there shows the sequence in
  the devtools console.

Things to look for:

- **A different outcome, or none.** If a question resolves `cancel` instead
  of `withdrawn`, it is dropped with no late card. If the card goes away with
  no resolution at all, that is a different bug.
- **A different shape.** For example, async questions arriving without
  `delivery: "async"`, or a newer codex-acp that names things differently.
  `docs/plan/chat-questions-adapters.md` (Codex section) records the shapes
  this fix expects.

## Known limits

- A late answer can't reach Codex during the running turn. ACP has no way to
  reach codex-acp's steering, so it waits for the turn to end and goes as a
  new prompt.
- Late cards are kept only in the window's memory. Reloading the window loses
  them, and the backend has nothing to restore.
- In the mock, a prompt starting with "My answer(s) to your question(s):"
  never asks a question, so a sent late answer doesn't trigger another one.

## Loose ends from this session

- The Vite mock-mode server (port 5391) and a headless Chrome (port 9231,
  profile `/tmp/question-shot/profile`) are still running. Stop them when
  done.
- `AGENTS.md` in the repo root is untracked and not part of this work.
- Other QA tasks still need approval to run: ACP-14 (partial), AAI-05, CA-06,
  CEG-05, CAS-05 and CQ-07.
