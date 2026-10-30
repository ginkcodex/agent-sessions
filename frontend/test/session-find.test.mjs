import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { highlightTextNodes } from '../src/lib/search.ts';
import { loadServerModule } from './helpers.mjs';

const FIND_URL = new URL('../src/lib/sessionFind.svelte.ts', import.meta.url);
const lib = (rel) => JSON.stringify(new URL(rel, FIND_URL).href);
const { SessionFind, findSeed } = await loadServerModule(FIND_URL, [
  ["'./search'", lib('./search.ts')],
  ["'./fileFind'", lib('./fileFind.ts')],
  ["'./contentSearch'", lib('./contentSearch.ts')],
  ["'./manage'", lib('./manage.ts')],
]);

const ref = { agent: 'claude', id: 's-1' };
const hits = (...indexes) => indexes.map((messageIndex) => ({ messageIndex, parts: [0] }));

function deferred() {
  let resolve;
  const promise = new Promise((r) => (resolve = r));
  return { promise, resolve };
}

function fixture(overrides = {}) {
  const host = {
    requests: [],
    jumps: [],
    clears: 0,
    top: 0,
    replies: [],
    search(req) {
      host.requests.push(req);
      const reply = host.replies.shift() ?? { supported: true, result: { hits: [] } };
      return reply.promise ?? Promise.resolve(reply);
    },
    ref: () => ref,
    includeHidden: () => false,
    topIndex: () => host.top,
    jump: (jump) => host.jumps.push(jump),
    clear: () => host.clears++,
    isStale: () => false,
    ...overrides,
  };
  return { host, find: new SessionFind(host) };
}

test('a new search starts at the first match from the top of the screen', async () => {
  const { host, find } = fixture();
  find.open = true;
  find.query = 'retry';
  host.top = 12;
  host.replies.push({ supported: true, result: { hits: hits(3, 10, 12, 40), revision: 'e.1.1' } });
  await find.runNow();
  assert.deepEqual(host.requests, [{ ref, query: 'retry', includeHidden: false }]);
  assert.equal(find.current, 2);
  assert.equal(find.count, '3 of 4');
  assert.deepEqual(
    { ...host.jumps.at(-1), id: undefined },
    { id: undefined, ref, messageIndex: 12, kind: 'text', query: 'retry', parts: [0], sticky: true },
  );

  // Past the last match, the search starts at the last match above.
  host.top = 50;
  host.replies.push({ supported: true, result: { hits: hits(3, 10), truncated: true } });
  await find.runNow();
  assert.equal(find.current, 1);
  assert.equal(find.count, '2 of 2+');
});

test('stepping wraps at both ends and Enter steps', async () => {
  const { host, find } = fixture();
  find.query = 'retry';
  host.replies.push({ supported: true, result: { hits: hits(1, 5, 9) } });
  await find.runNow();
  assert.equal(find.current, 0);
  find.step(-1);
  assert.equal(find.current, 2);
  find.step(1);
  assert.equal(find.current, 0);
  const key = (shiftKey) => ({ key: 'Enter', shiftKey, preventDefault() {}, stopPropagation() {} });
  find.handleKey(key(false));
  find.handleKey(key(false));
  assert.equal(find.current, 2);
  find.handleKey(key(true));
  assert.equal(find.current, 1);
  assert.deepEqual(host.jumps.map((j) => j.messageIndex), [1, 9, 1, 5, 9, 5]);
  assert.equal(new Set(host.jumps.map((j) => j.id)).size, host.jumps.length);
});

test('a reply to a superseded search is dropped', async () => {
  const { host, find } = fixture();
  const slow = deferred();
  find.query = 'old';
  host.replies.push({ promise: slow.promise });
  const first = find.runNow();
  find.query = 'new';
  host.replies.push({ supported: true, result: { hits: hits(7) } });
  await find.runNow();
  slow.resolve({ supported: true, result: { hits: hits(1, 2, 3) } });
  await first;
  assert.deepEqual(find.hits, hits(7));
  assert.equal(find.loading, false);
  assert.deepEqual(host.jumps.map((j) => j.query), ['new']);
});

test('a refresh recounts without moving the view and keeps the current message', async () => {
  const { host, find } = fixture();
  find.open = true;
  find.query = 'retry';
  host.replies.push({ supported: true, result: { hits: hits(2, 8, 20) } });
  await find.runNow();
  find.step(1);
  assert.equal(find.hits[find.current].messageIndex, 8);
  const jumps = host.jumps.length;

  // A new earlier match shifts the position; the message stays current.
  host.replies.push({ supported: true, result: { hits: hits(1, 2, 8, 20, 30) } });
  find.refresh();
  await new Promise((r) => setImmediate(r));
  assert.equal(find.hits[find.current].messageIndex, 8);
  assert.equal(find.count, '3 of 5');
  // Its message no longer matches: the next one becomes current.
  host.replies.push({ supported: true, result: { hits: hits(1, 2, 20) } });
  find.refresh();
  await new Promise((r) => setImmediate(r));
  assert.equal(find.hits[find.current].messageIndex, 20);
  assert.equal(host.jumps.length, jumps);

  // A closed bar does not search.
  const requests = host.requests.length;
  find.close();
  find.refresh();
  assert.equal(host.requests.length, requests);
});

test('typing debounces, an empty or unsupported search clears, and Escape closes', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const { host, find } = fixture();
  find.show();
  assert.equal(find.open, true);
  assert.equal(find.focusRequest, 1);
  find.setQuery('ret');
  find.setQuery('retry');
  assert.equal(find.loading, true);
  assert.equal(host.requests.length, 0);
  t.mock.timers.tick(150);
  await Promise.resolve();
  assert.deepEqual(host.requests.map((r) => r.query), ['retry']);

  const clears = host.clears;
  find.setQuery('agent:codex  ');
  assert.equal(find.loading, false);
  assert.equal(find.count, '');
  assert.equal(host.clears, clears + 1);

  // A quote still being typed is searched closed.
  find.setQuery('"retry pol');
  assert.equal(find.search, '"retry pol"');

  host.replies.push({ supported: false });
  await find.runNow();
  assert.equal(find.unsupported, true);
  assert.equal(find.count, '');

  let stopped = false;
  find.handleKey({ key: 'Escape', preventDefault() {}, stopPropagation() { stopped = true; } });
  assert.equal(find.open, false);
  assert.equal(stopped, true);
  assert.equal(find.unsupported, false);
  find.reset();
  assert.equal(find.query, '');
});

test('errors show, except a reply from another connection', async () => {
  const { host, find } = fixture();
  find.query = 'retry';
  host.replies.push({ promise: Promise.reject(new Error('search: too many terms (max 20)')) });
  await find.runNow();
  assert.equal(find.error, 'search: too many terms (max 20)');
  assert.equal(find.count, '');

  host.isStale = () => true;
  find.error = null;
  host.replies.push({ promise: Promise.reject(new Error('stale')) });
  await find.runNow();
  assert.equal(find.error, null);
});

test('a selection seeds the search, as a phrase when it has spaces', () => {
  assert.equal(findSeed('  retry  '), 'retry');
  assert.equal(findSeed('retry policy'), '"retry policy"');
  assert.equal(findSeed('say "hi" now'), 'say "hi" now');
  assert.equal(findSeed('two\nlines'), undefined);
  assert.equal(findSeed(''), undefined);
  const { host, find } = fixture();
  find.show('retry policy');
  assert.equal(find.query, '"retry policy"');
  assert.equal(host.requests.length, 0);
  find.close();
});

test('the mock searches a whole session in order, like the host', async () => {
  const mock = new MockBackendAPI();
  const [meta] = await mock.listSessions('', {}, { field: 'updatedAt', dir: 'desc' });
  const page = await mock.getMessages(meta.ref, 0, 10_000);
  const word = page.messages.flatMap((m) => m.parts ?? []).find((p) => p.kind === 'text' && /\w{5,}/.test(p.text ?? ''))
    .text.match(/\w{5,}/)[0];
  const reply = await mock.searchSession({ ref: meta.ref, query: word.toUpperCase(), includeHidden: true });
  assert.equal(reply.supported, true);
  assert.ok(reply.result.hits.length > 0);
  const indexes = reply.result.hits.map((h) => h.messageIndex);
  assert.deepEqual(indexes, [...indexes].sort((a, b) => a - b));
  for (const hit of reply.result.hits) {
    assert.ok(hit.parts.length > 0);
  }
  const limited = await mock.searchSession({ ref: meta.ref, query: word, includeHidden: true, limit: 1 });
  assert.equal(limited.result.hits.length, 1);
  assert.equal(limited.result.truncated, reply.result.hits.length > 1);
  mock.sessionSearch = false;
  assert.deepEqual(await mock.searchSession({ ref: meta.ref, query: word }), { supported: false });
});

test('marks find a phrase across a line break', (t) => {
  const originals = { document: globalThis.document, NodeFilter: globalThis.NodeFilter };
  t.after(() => Object.assign(globalThis, originals));
  const inserted = [];
  const text = {
    nodeValue: 'the retry\n  policy holds',
    parentElement: { closest: () => null },
    replaceWith: (fragment) => inserted.push(...fragment.childNodes),
  };
  globalThis.NodeFilter = { SHOW_TEXT: 4, FILTER_ACCEPT: 1, FILTER_REJECT: 2 };
  globalThis.document = {
    createTreeWalker: (_root, _show, filter) => {
      let done = false;
      return { nextNode: () => (done || filter.acceptNode(text) !== 1 ? null : ((done = true), text)) };
    },
    createDocumentFragment: () => ({ childNodes: [], append(...nodes) { this.childNodes.push(...nodes); } }),
    createElement: () => ({ className: '', textContent: '' }),
  };
  highlightTextNodes({}, '"retry policy"');
  const marks = inserted.filter((n) => typeof n === 'object');
  assert.deepEqual(marks.map((m) => m.textContent), ['retry\n  policy']);
});

test('find steps inside the loaded rows do not reload them', () => {
  const source = readFileSync(new URL('../src/lib/components/transcript/TranscriptView.svelte', import.meta.url), 'utf8');
  const jump = source.match(/async function loadSearchJump[\s\S]*?\n {2}\}\n/)[0];
  const fast = jump.match(/if \(soft && inWindow && !jumpLoading\) \{([\s\S]*?)\n {4}\}/)[1];
  assert.doesNotMatch(fast, /setWindow|getMessages|isLoading = true|loadGeneration/);
  assert.match(fast, /markJump\(jump, true\)/);
  // Elsewhere, the old rows stay until the target's page arrives.
  assert.match(jump, /if \(!soft\) setWindow\(\{ messages: \[\], offset, hasMore: false \}\);/);
  assert.match(jump, /if \(!soft\) isLoading = true;/);
  // Only a search result's marks fade.
  assert.match(source, /if \(!jump\.sticky\) highlightTimer = setTimeout\(clearJumpHighlight, 5000\);/);
});
