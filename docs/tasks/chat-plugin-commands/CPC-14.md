# CPC-14 — `cmd` tag on slash-command user messages

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CPC-07
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

User messages such as `/ponytail:ponytail full` read like prose. Mark them:
a `cmd` tag beside "You" and the message in the mono font. One rule in
`MessageBubble`, so saved transcripts and the live tail both get it.

Rule: the trimmed text starts with `/`, a name of letters, digits, `_`, `.`
or `-`, optional `:segment` parts, then whitespace or the end. Paths
(`/home/x`) do not match. Codex `$skill` mentions are not slash commands and
stay as they are.

## Likely files

- `frontend/src/lib/chat.ts` (the rule)
- `frontend/src/lib/components/transcript/MessageBubble.svelte`
- `frontend/test/chat-ui.test.mjs`, a transcript render test

## Acceptance criteria and tests

- `/ponytail:ponytail full`, `/understand-anything:understand` and `/review`
  match. `/home/haith/x`, `hello /review`, `/` and an assistant message do
  not.
- The bubble shows `cmd` and uses `--font-mono`; other user messages are
  unchanged.
- A command message is not offered for translation.
- Run frontend tests, check and build; look at it in the browser.

## Evidence

- `isSlashCommand` in `frontend/src/lib/transcript.ts`; `MessageBubble`
  adds the `cmd` tag, the `slash-command` class (mono body) and leaves such
  messages out of Translate.
- `test/transcript.test.mjs`: the rule matches `/ponytail:ponytail full`,
  `/understand-anything:understand`, `/review`, `/mcp:docs:search q`, and
  rejects `/home/haith/x`, `hello /review`, `/`, `/ ponytail`, `//x`,
  `ponytail:ponytail`, `$sites:sites`.
- `test/manual-bash.test.mjs` (real `MessageBubble`, SSR): a command gets the
  tag and class; a path message, an assistant `/review` and a manual Bash
  record do not. `test/type-scale.test.mjs` lists `.command-tag` at the tag
  token.
- `npm test` (1492 pass), `npm run check` (0 errors), `npm run build` pass.
- Browser (vite + mock, live chat): `/review the scan engine` shows `cmd`
  beside "You" in JetBrains Mono, in light and dark; a message mentioning
  `/home/haith` is untagged.
- Not checked: a saved Claude transcript with a real plugin invocation in the
  desktop app (same component, so expected to match).
