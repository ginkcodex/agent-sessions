# Chat questions

## Goal

In a terminal, Claude Code, Codex and OpenCode can stop and ask the user
questions: one or more tabs, each a single choice (often with a recommended
option), a multi-select, or a yes/no, plus "Type your own answer", and a
Submit step where answers can still be changed. A live chat should show the
same questions with their options, let the user move between them and change
answers before submitting, and send the answers back so the turn continues.

Today the chat cannot: the client does not advertise ACP form elicitation, so
each adapter falls back without asking (see
[chat-questions-adapters](chat-questions-adapters.md)):

- Claude Code disallows its AskUserQuestion tool;
- Codex answers its `request_user_input` with no answers;
- OpenCode cancels the question and tells the model it could not be shown.

## Design

### Protocol

All three adapters ask through the same ACP request, `elicitation/create`
(agent → client) in `mode: "form"`, once the client's `initialize` advertises
`clientCapabilities.elicitation.form = {}`:

```json
{ "sessionId": "…", "toolCallId": "…", "mode": "form", "message": "…",
  "requestedSchema": { "type": "object", "properties": { … }, "required": [ … ] } }
```

The reply is `{ "action": "accept", "content": { key: value } }`,
`{ "action": "decline" }` or `{ "action": "cancel" }`. Values are a string, a
number, a boolean or a string array. The agent withdraws a question it no
longer needs (Codex's auto-resolution timeout, a cancelled tool call) with
`$/cancel_request { requestId }`.

Only form mode is advertised. URL mode (sending the user to a web page, used
for MCP OAuth) is out of scope.

### Which forms the chat shows

A property is one of:

| Schema | Shown as |
|---|---|
| `string` with `oneOf` (titled) or `enum` | single choice; each option's `description` under its title |
| `array` with `items.anyOf` or `items.enum` | multi-select, with `minItems` and `maxItems` |
| `string` | a text box, with `minLength`, `maxLength` |
| `boolean` | Yes / No |
| `number`, `integer` | a number box, with `minimum`, `maximum` |

A text property that is another property's "own answer" is shown as a last
**Type your own answer** option of that question, as in the terminal. The
adapters mark it differently, so the pairing knows each way:

- Claude Code: `question_<n>` and `question_<n>_custom`;
- OpenCode: `<key>` and `<key>_custom`;
- Codex: a property whose `_meta.codex.role` is `user_note` names its question
  in `_meta.codex.questionId`; Codex also adds an "Other" option to the
  choice, which the pairing hides, since picking it means "the note is my
  answer".

A schema with anything else (another type, a `format` the chat cannot check,
a nested object) is refused with a JSON-RPC error, which each adapter turns
into its own fallback. The chat never shows half a form.

### The question card

A card in the chat, where permission cards are, styled after them:

- **Header:** "<Agent> asks" and the message. Claude Code's message is the
  question itself for one question, "Please answer the following
  questions." for several; Codex's is generic, so each question's own title
  is what the user reads.
- **One question:** its options, then Submit and Skip.
- **Each question** reads as a short header and the question. Claude Code
  sends the header as `title` and the question as `description`; Codex sends
  them the other way round (its properties carry `_meta.codex`); a property
  with only a title uses it for both.
- **Several questions:** one tab per question, named by its header (such as
  "Extras to include"), and a last
  **Review** tab listing each question with the answer chosen, or
  "Not answered", and a Change link back to its tab. Submit is on the Review
  tab, and on the last question.
- A tab shows a check once its question is answered. Choosing an option of a
  single choice moves to the next tab, as the terminal does; the user can go
  back to any tab and change it until Submit.
- **Keyboard:** 1–9 picks an option of the current question, ← and → move
  between tabs. Enter in a text box does not submit.
- **Required:** Submit is disabled, with a line saying which question still
  needs an answer, until every required question is answered.
- **Nothing is chosen for the user.** A schema `default` is marked "Default"
  but not preselected; "(Recommended)" stays part of the label, as sent.
- **Skip** answers `decline`. Claude Code tells the model the user skipped;
  Codex gets no answers; OpenCode cancels the question. Cancelling the turn,
  closing the chat or the agent exiting answers `cancel`.
- **Withdrawn:** when the agent withdraws the question, a line in the live
  tail says it went on without an answer, and the card stays as a late
  question (CQ-08): the same answers, kept as drafted, and Submit becomes
  "Send as message", which sends them as the next prompt once the turn has
  ended. Required questions no longer block it; at least one answer does.
  A hidden (secret) answer is never put in a message. Dismiss drops the card.
  The end of a turn alone does not remove a question: only its resolution
  does, since the agent may still be waiting for it.

After Submit the card is replaced by a one-line summary in the live tail
("Answered: Extend existing module · Unit tests, Documentation · Yes,
overwrite") until the turn's saved log arrives. An answer that was sent
cannot be changed; the user corrects it in a new message.

### Validation

The Go side checks the answer against the schema before it replies:

- only the schema's properties are sent;
- a choice is one of its options, a multi-select's items are options, unique,
  within `minItems`/`maxItems`;
- text within `minLength`/`maxLength` (and `pattern` when given, with Go's
  RE2), numbers within `minimum`/`maximum`, integers whole;
- every `required` property is present.

An invalid answer is refused to the frontend; nothing is sent to the agent.

### Where it runs

- `internal/acp`: elicitation types; `ClientOptions.OnElicitation` turns on the
  form capability; `elicitation/create` handling; a per-request context so
  `$/cancel_request` withdraws a question (the reply is then error -32800,
  request cancelled); schema support check and answer validation.
- `internal/engine`: pending questions per chat, as permissions are:
  `question` and `question_resolved` updates, `ChatState.questions` for a
  reloaded window, `ChatManager.Answer(chatID, questionID, action, content)`,
  and every exit path answers `cancel`.
- `internal/app` and `internal/rpc`: a `ChatAnswer` binding, a `chatAnswer`
  method, and a `ChatQuestions` capability. The start request says whether the
  app shows questions; the server advertises elicitation to the agent only
  then, so an older app on a newer server keeps today's fallback instead of a
  question no one sees.
- Frontend: `chatQuestions.ts` turns a schema into questions (pairing, the
  Codex "Other" option), keeps a draft, validates it and builds the content;
  `QuestionCard.svelte`; the store keeps pending questions; the mock agent asks
  forms shaped like each adapter's.

## Decisions

1. **Use ACP form elicitation**, not adapter-specific `_meta` or permission
   options: all three adapters use it, and it is the documented path.
2. **Advertise it only when the app can show it** (the start flag), and only
   form mode.
3. **Refuse what cannot be shown in full**, so each adapter's own fallback
   runs instead of a partial answer.
4. **No preselection, and Enter does not submit.** A question can be consent
   ("Yes, overwrite"); as with permissions, the user chooses.
5. **Answers can change until Submit, not after.** The agent has the answer
   once it is sent.
6. **Saved transcripts are unchanged.** How AskUserQuestion and similar tool
   calls look in a saved session is a separate change.

## Risks

- **Elicitation is unstable in ACP v1** and the adapters' conventions for the
  own-answer field differ and may change. The pairing is per convention, and
  an unpaired text field still works as a plain text box.
- **A question waits for the user.** The turn blocks until it is answered,
  skipped or withdrawn; the chat shows the card where the user looks, and the
  turn can still be cancelled.
- **Codex goes on without the answer.** Its blocking questions carry
  `autoResolutionMs`, and the adapter withdraws them after that long, at
  once when the value is missing. Codex 0.167 mostly asks through
  `request_user_input_async`: the tool returns at once, Codex keeps working
  and streaming, and the adapter withdraws the question when the turn ends.
  Either way the card used to vanish while text still streamed. The late
  question (CQ-08) keeps the answers reachable: they go to the agent as a
  message, as Codex itself suggests ("Send it as a new message if it still
  matters").

## Release criteria

- Go tests: the capability is advertised only with `OnElicitation`; a form
  request reaches the callback and its answer goes back; `$/cancel_request`
  withdraws it; an unsupported schema is refused; invalid answers are refused;
  turn cancel, close and exit answer `cancel`; the RPC method and capability;
  an old start request does not advertise elicitation.
- Frontend tests: each adapter's shape becomes the right questions (pairing,
  the hidden Codex "Other"); tabs, change before Submit, required, Skip,
  withdrawal; content built for each adapter's shape; no preselection.
- `make lint`, `go test -race` for the changed packages, `npm test`,
  `npm run check`, `npm run build`.
- A real question from each agent in a chat (CQ-07), with every check that did
  not run marked so.

Tasks: [chat-questions](../tasks/chat-questions/README.md).
