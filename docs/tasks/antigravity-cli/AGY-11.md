# AGY-11 — Live-tail review fixes: failed prompts, file-only prompts, bounded `sent`

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** [AGY-06](AGY-06.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Branch review of AGY-06's ID-less reconciliation in
`frontend/src/lib/chatTail.ts`:

1. **Greedy prompt match.** A failed prompt that agy never saved took its
   identical retry's saved copy, so the retry never matched and its items
   stayed.
2. **File-only prompts.** A prompt with only attachments was not recorded in
   `sent`, so its turn never matched and its items stayed.
3. **Unbounded `sent`.** Every prompt stayed in `sent` for the life of the
   chat, for every agent, and each reconcile scanned all of them against the
   whole saved transcript.
4. **Mock label.** The nested ternary for the panel label in
   `frontend/src/lib/mock/mockApi.ts` becomes a lookup map.

## Acceptance criteria

- A failed, never-saved prompt followed by an identical retry: the retry's
  turn goes once it is saved.
- A file-only prompt's turn goes once it is saved.
- `sent` keeps only the prompts of turns that run or still show items; the
  repeated-prompt case from AGY-06 still waits for its own saved copy.
- Claude Code, Codex and OpenCode reconciliation is unchanged.

## Evidence

Done:

- `sent` keeps a prompt only while its turn runs or still has items. A
  reconcile drops the rest, so it stays small for every agent.
- `sentAfter` keeps the saved ID of the prompt that the latest ID-less turn
  matched. Matching starts after it, so a dropped prompt's saved copy is not
  taken again (the AGY-06 repeated-prompt case).
- Matching skips an ended turn with nothing left to show. A failed prompt's
  only item goes by its text as soon as the retry's copy is saved, so the
  failed turn no longer takes that copy.
- A turn matched by its prompt also drops the turns before it, as a turn
  reached by ID already does. agy saves turns in order.
- A file-only prompt is recorded with text `""` and matches a saved prompt
  with no text. `shortcut:` comment: agy's saved form of an image prompt was
  not seen. `agy acp` here refuses `authenticate` with "No browser is
  available here" (SSH session), so no image prompt could be sent. If the
  saved form has text, the turn goes once a later turn is saved.
- `isCovered` computes the per-item check once per reconcile, not twice.
- `mockApi.ts`: the panel label is a lookup map.

Tests (`frontend/test/chat-tail.test.mjs`):

- A failed prompt, then an identical retry: everything goes, `sent` is
  empty, and `sentAfter` is the retry's saved prompt.
- A file-only prompt is recorded in `sent` and goes once a saved prompt with
  no text and its reply are saved.
- Three reconciled Claude turns leave `sent` empty. An open turn keeps its
  entry after its prompt item goes by its text.
- Against the old `chatTail.ts`, these 3 tests fail; the rest pass.

Results:

- `npm test`: 1627 tests, 1622 pass, 0 fail, 5 skipped, including
  `chat-reconcile-agents` for Claude Code, Codex and OpenCode.
- `npm run check`: 0 errors, 0 warnings.

Not checked:

- A live `agy acp` image prompt (sign-in, see above).
- A failed prompt that still shows something (only a file-only one does):
  it can take the next saved copy of its text. A `shortcut:` comment
  in `idlessSavedTurns` marks this.
