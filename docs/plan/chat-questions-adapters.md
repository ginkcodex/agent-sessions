# Chat questions — how each adapter asks

Read from the adapters' published code, not run: `claude-agent-acp` 0.106.0
and `codex-acp` 2.6.0 (the pinned npm packages), and OpenCode 2.2.6
(`packages/cli/src/acp/elicitation.ts` and `capabilities.ts` at the `v2.2.6`
tag). The ACP schema is `@agentclientprotocol/sdk` 2.6.0, protocol v1.

## Protocol (ACP v1, unstable)

- Capability: `initialize.clientCapabilities.elicitation.form = {}` (and
  `url = {}` for URL mode, not used here).
- Request, agent → client: `elicitation/create`. Params: `sessionId`,
  optional `toolCallId`, `mode` (`form` or `url`), `message`, and for form
  mode `requestedSchema` (`type: "object"`, `properties`, optional `required`,
  `title`, `description`).
- Property schemas: `string` (`title`, `description`, `minLength`,
  `maxLength`, `pattern`, `format` email/uri/date/date-time, `default`,
  `enum`, `oneOf: [{const, title, description?, _meta?}]`), `number` and
  `integer` (`minimum`, `maximum`, `default`), `boolean` (`default`), `array`
  (`items: {type: "string", enum}` or `items: {anyOf: [EnumOption]}`,
  `minItems`, `maxItems`, `default`).
- Response: `{action: "accept", content?: {key: string | number | boolean |
  string[]}}`, `{action: "decline"}`, `{action: "cancel"}`.
- Withdrawal: `$/cancel_request {requestId}` from the agent.
- `elicitation/complete {elicitationId}` (notification) is for URL mode only.

## Claude Code (`claude-agent-acp` 0.106.0)

- Without `elicitation.form` the adapter passes
  `disallowedTools: ["AskUserQuestion"]`: the model cannot ask.
- With it, AskUserQuestion's questions become one form
  (`askUserQuestionsToCreateRequest`):
  - `question_<n>`: `{type: "string", title: header, description: question,
    oneOf: [{const: label, title: label, description}]}` for a single choice,
    or `{type: "array", items: {anyOf: [...]}}` for `multiSelect`;
  - `question_<n>_custom`: `{type: "string", title: "Other", description:
    "Type your own answer…"}`;
  - nothing required; `message` is the question for one question, else
    "Please answer the following questions.", and the field descriptions carry
    the questions; `toolCallId` is the tool use.
  - An option's `preview` travels in `_meta["_claude/askUserQuestionOption"]`.
- Answer (`applyAskElicitationResponse`): single choice → the label; with a
  custom text as well, the text is a note; custom only → the text.
  Multi-select → labels plus the custom text, comma-joined.
  `decline` → empty answers ("the user skipped"); `cancel` or an error →
  the tool use is aborted (`cancel`) or denied (error).
- The same path carries MCP servers' form elicitations and the
  refusal-fallback dialog (a single choice `choice`).

## Codex (`codex-acp` 2.6.0)

- Without `elicitation.form`, `request_user_input` returns `{answers: {}}`.
- With it (`buildUserInputRequest`): `message` is "Codex needs your input to
  continue.", `toolCallId` the item, every question `required`:
  - `<id>`: `{type: "string", title: question, description: header,
    oneOf: [options…, {const: "Other", title: "Other", description: "Provide a
    different answer in the note field."}]}` when the question has options and
    `isOther`; a plain `string` when it has none;
  - a note field when `isOther`: `{type: "string", title: "Additional answer
    or note", _meta: {codex: {questionId, role: "user_note", isSecret}}}`;
  - `_meta.codex.isSecret` on secret questions.
- `_meta.codex.autoResolutionMs` on the request: Codex withdraws the request
  after that long (the adapter aborts it with `$/cancel_request`). Only
  `null` waits for good; a missing value is read as 0 ms
  (`Math.max(0, autoResolutionMs ?? 0)`), so the question is withdrawn as
  soon as it is shown.
- Async questions (Codex's `request_user_input_async`, an agent message with
  `delivery: "async"` and questions): the same schema, nothing required,
  message "Codex keeps working while you answer. An answer sent after it
  finishes is not delivered.", `_meta.codex.delivery: "async"`, no
  `toolCallId`. Codex goes on working and streaming; the adapter steers an
  answer into the running turn, and withdraws every open async question when
  the turn completes. The Codex 0.167 prompt prefers this tool for questions
  ("The tool returns immediately without ending the turn or waiting for a
  reply; any reply arrives asynchronously as a new user message").
- Answer: the value, and `note:<text>` for the note; "Other" is dropped when
  the answer is read.

## OpenCode 2.2.6

- Without `elicitation.form`, the question is cancelled with "The question
  couldn't be shown to the user in this client. Continue without an answer…".
- With it, only forms of kind `question` (and `websearch.provider`) are sent,
  never one that looks like a credential, and only when every field can be
  represented:
  - `<key>`: `string` with `oneOf: [{const: value, title: label,
    description}]`, or `array` with `items.anyOf`, `minItems` (≥ 1 when
    required), `maxItems`; `default` may be set;
  - `<key>_custom` when the field allows a custom answer: `{type: "string",
    title: "<title> (other)", description: "Type your own answer" | "Add your
    own answer"}`;
  - number, integer, boolean and plain string fields as is; `required` lists
    the required keys.
- Answer: a custom text replaces a single choice's value, or is added to a
  multi-select's values. Anything but `accept` cancels the form.

## Live Codex correction (CQ-09)

The installed codex-acp 2.6.0 source uses `USER_INPUT_OTHER_OPTION =
"None of the above"`, not the `Other` value described above and used by
the original fixtures. Its note prefix is `user_note: `, not `note:`.
The live screenshot confirms the visible “None of the above” option next to
the custom input. The frontend now recognizes both sentinel values only
when pairing a Codex note, hides that sentinel and sends its original value
with the note. This preserves legacy fixture coverage without assuming
that real 2.6.0 sends the old shape.
