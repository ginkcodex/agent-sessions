import test from 'node:test';
import assert from 'node:assert/strict';
import { conversationPrintParts, loadConversationPrintParts } from '../src/lib/conversationPrint.ts';
import { printMarkdown, printBlocks } from '../src/lib/print.ts';
import { StaleReplyError } from '../src/lib/link.ts';

const ref = { agent: 'claude-code', id: 'session' };
const msg = (text, role = 'user', extra = {}) => ({ id: '', time: '', role, parts: [{ kind: 'text', text }], ...extra });
const page = (messages, offset, totalCount, extra = {}) => ({ messages, offset, totalCount, limit: messages.length, hasMore: offset + messages.length < totalCount, revision: 'r1', ...extra });

test('conversation prints only Chat content in order, with roles and one attachment namespace', () => {
  const first = { name: 'first.png', mime: 'image/png', ref: 'first' };
  const second = { name: 'second.png', mime: 'image/png', ref: 'second' };
  const parts = conversationPrintParts(ref, [
    msg('system secret', 'system'), msg('meta secret', 'user', { isMeta: true }),
    msg('hello', 'user', { parts: [{ kind: 'text', text: 'hello' }, { kind: 'file', file: first }] }),
    msg('', 'assistant', { parts: [{ kind: 'reasoning', text: 'hidden thinking' }, { kind: 'tool', tool: { output: 'hidden output' } }] }),
    msg('## Answer\n\n```js\nconst a = 1;\n```', 'assistant', { parts: [
      { kind: 'text', text: '## Answer\n\n```js\nconst a = 1;\n```' },
      { kind: 'file', file: second }, { kind: 'notice', text: 'Assistant error' },
      { kind: 'compaction', text: 'hidden compaction' },
    ] }),
    msg('', 'assistant', { parts: null }),
  ]);
  const printed = printMarkdown(parts);
  assert.deepEqual(printed.attachments, [first, second]);
  assert.equal(printed.markdown, '## User\n\nhello\n\n![first.png](attachment:1)\n\n---\n\n## Assistant\n\n## Answer\n\n```js\nconst a = 1;\n```\n\n![second.png](attachment:2)\n\n> Assistant error');
  assert.deepEqual(printBlocks(printed.markdown).filter(b => b.kind === 'image').map(b => b.index), [1, 2]);
});

test('conversation preserves manual Bash and applies supplied translations at absolute indices', () => {
  const seen = [];
  const printed = printMarkdown(conversationPrintParts(ref, [
    msg('<bash-input>printf `hi`</bash-input>'), msg('original', 'assistant'),
    msg('hidden', 'user', { isMeta: true }),
  ], 50, (message, index) => { seen.push(index); return 'translated'; }));
  assert.match(printed.markdown, /```bash\nprintf `hi`\n```/);
  assert.match(printed.markdown, /## Assistant\n\ntranslated/);
  assert.doesNotMatch(printed.markdown, /bash-input|original|hidden/);
  assert.deepEqual(seen, [50, 51]);
});

test('loader fetches beyond 50 messages from zero using actual capped page lengths', async () => {
  const all = Array.from({ length: 123 }, (_, index) => msg(`Turn ${index}`, index % 2 ? 'assistant' : 'user'));
  const calls = [];
  const parts = await loadConversationPrintParts(ref, async (gotRef, offset, limit, cap) => {
    calls.push(offset);
    assert.deepEqual(gotRef, ref);
    assert.equal(limit, 50);
    assert.equal(cap.anchor, offset);
    assert.ok(cap.maxBytes > 0);
    return page(all.slice(offset, offset + 17), offset, all.length, { capped: true });
  }, () => true);
  assert.deepEqual(calls, [0, 17, 34, 51, 68, 85, 102, 119]);
  const markdown = printMarkdown(parts).markdown;
  assert.equal((markdown.match(/## (User|Assistant)/g) ?? []).length, 123);
  assert.equal((markdown.match(/\n\n---\n\n/g) ?? []).length, 122);
  for (let index = 1; index < 123; index++) {
    assert.ok(markdown.indexOf(`Turn ${index - 1}\n`) < markdown.indexOf(`Turn ${index}${index === 122 ? '' : '\n'}`));
  }
});

test('loader numbers attachments globally across pages and skips hidden-only pages', async () => {
  const all = [msg('a', 'user', { parts: [{ kind: 'file', file: { mime: 'image/png', ref: 'a' } }] }),
    msg('hidden', 'system'), msg('b', 'assistant', { parts: [{ kind: 'file', file: { mime: 'image/png', ref: 'b' } }] })];
  const parts = await loadConversationPrintParts(ref, async (_, offset) => page([all[offset]], offset, 3), () => true);
  const printed = printMarkdown(parts);
  assert.deepEqual(printed.attachments.map(f => f.ref), ['a', 'b']);
  assert.match(printed.markdown, /attachment:1/);
  assert.match(printed.markdown, /attachment:2/);
  assert.equal((printed.markdown.match(/---/g) ?? []).length, 1);
});

test('loader handles empty/hidden conversations and legacy hosts without revisions', async () => {
  assert.deepEqual(await loadConversationPrintParts(ref, async () => page([], 0, 0), () => true), []);
  assert.deepEqual(await loadConversationPrintParts(ref, async () => page([msg('hidden', 'system')], 0, 1), () => true), []);
  const parts = await loadConversationPrintParts(ref, async (_, offset) => page([msg(`part${offset}`)], offset, 2, { revision: undefined }), () => true);
  assert.match(printMarkdown(parts).markdown, /part0[\s\S]*part1/);
});

test('loader refuses changing revisions/counts, missing pages and backend errors', async () => {
  for (const bad of [
    page([msg('new')], 1, 2, { revision: 'r2' }),
    page([msg('new')], 1, 3),
    page([], 1, 2),
    page([msg('gap')], 2, 3),
    page([], 0, 2, { hasMore: false }),
  ]) {
    let calls = 0;
    await assert.rejects(loadConversationPrintParts(ref, async () => ++calls === 1 ? page([msg('first')], 0, 2) : bad, () => true), /changed|complete conversation/);
    assert.equal(calls, 2);
  }
  await assert.rejects(loadConversationPrintParts(ref, async () => { throw new Error('offline'); }, () => true), /offline/);
  await assert.rejects(loadConversationPrintParts(ref, async () => page([msg('first')], 0, 2, { hasMore: false }), () => true), /complete conversation/);
});

test('operation-wide guard rejects selection/host changes before, during and between fetches', async () => {
  await assert.rejects(loadConversationPrintParts(ref, async () => { assert.fail('must not fetch'); }, () => false), StaleReplyError);
  let valid = true;
  await assert.rejects(loadConversationPrintParts(ref, async () => { valid = false; return page([msg('old')], 0, 1); }, () => valid), StaleReplyError);
  let checks = 0;
  let calls = 0;
  await assert.rejects(loadConversationPrintParts(ref, async () => { calls++; return page([msg('old')], 0, 2); }, () => ++checks < 3), StaleReplyError);
  assert.equal(calls, 1);
});
