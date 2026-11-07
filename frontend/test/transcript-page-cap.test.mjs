import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { withDesktopApp } from './desktop-app.mjs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { capPage, pageCap, PAGE_BYTES, PAGE_SIZE } from '../src/lib/pageCap.ts';
import { refreshLimit, TranscriptRefresher, windowOf } from '../src/lib/transcriptRefresh.ts';

const ref = { agent: 'codex', id: 'cap-fixture' };
// Message i weighs sizes[i] in the test's sizeOf.
const pageFrom = (sizes, offset = 0, totalCount = sizes.length) => ({
  messages: sizes.map((size, i) => ({ id: `m${offset + i}`, size })),
  offset, limit: sizes.length, totalCount, hasMore: offset + sizes.length < totalCount, revision: 'r1',
});
const sizeOf = (m) => m.size;
const ids = (page) => page.messages.map((m) => m.id).join(',');

test('capPage keeps the run around its anchor, as the engine does', () => {
  const all = [1, 1, 1, 10, 1, 1, 1, 1];
  const cases = [
    ['no cap', { maxBytes: 0, anchor: -1 }, 'm0,m1,m2,m3,m4,m5,m6,m7', 0, false, undefined],
    ['budget fits all', { maxBytes: 17, anchor: -1 }, 'm0,m1,m2,m3,m4,m5,m6,m7', 0, false, undefined],
    ['newest within budget', { maxBytes: 4, anchor: -1 }, 'm4,m5,m6,m7', 4, false, true],
    ['newest stops at a large message', { maxBytes: 13, anchor: -1 }, 'm4,m5,m6,m7', 4, false, true],
    ['jump grows both ways', { maxBytes: 3, anchor: 5 }, 'm4,m5,m6', 4, true, true],
    ['one side blocked, the other grows', { maxBytes: 4, anchor: 4 }, 'm4,m5,m6,m7', 4, false, true],
    ['anchor alone over budget', { maxBytes: 1, anchor: 3 }, 'm3', 3, true, true],
    ['anchor after the page is clamped', { maxBytes: 1, anchor: 99 }, 'm7', 7, false, true],
  ];
  for (const [name, cap, want, offset, hasMore, capped] of cases) {
    const got = capPage(pageFrom(all), cap, sizeOf);
    assert.deepEqual([ids(got), got.offset, got.hasMore, got.capped], [want, offset, hasMore, capped], name);
    if (capped) assert.equal(got.limit, got.messages.length, name);
  }
  // A later page keeps its start; an anchor before it is clamped to it.
  const later = capPage(pageFrom([1, 1, 1, 1], 4, 8), { maxBytes: 2, anchor: 0 }, sizeOf);
  assert.deepEqual([ids(later), later.offset, later.hasMore], ['m4,m5', 4, true]);
  // An earlier page keeps its end.
  const earlier = capPage(pageFrom([1, 1, 1], 0, 8), { maxBytes: 2, anchor: 2 }, sizeOf);
  assert.deepEqual([ids(earlier), earlier.offset, earlier.hasMore], ['m1,m2', 1, true]);
  const empty = pageFrom([], 0, 0);
  assert.equal(capPage(empty, { maxBytes: 1, anchor: -1 }, sizeOf), empty);
});

test('the viewer cap is PAGE_BYTES around the given anchor', () => {
  assert.deepEqual(pageCap(-1), { maxBytes: PAGE_BYTES, anchor: -1 });
  assert.deepEqual(pageCap(12), { maxBytes: 256 * 1024, anchor: 12 });
  assert.equal(PAGE_SIZE, 50);
});

// A mock transcript of n messages, each about `bytes` of JSON.
function mockWith(n, bytes) {
  const api = new MockBackendAPI();
  api.messages[ref.id] = Array.from({ length: n }, (_, i) => ({
    id: `m-${i}`, role: 'assistant', time: '', parts: [{ kind: 'text', text: 'x'.repeat(bytes) }],
  }));
  return api;
}

test('the mock caps pages by JSON size and rejects invalid caps', async () => {
  const api = mockWith(80, 100 * 1024);
  const latest = await api.getMessages(ref, -1, PAGE_SIZE, pageCap(-1));
  assert.deepEqual([latest.messages.length, latest.offset, latest.capped, latest.hasMore], [2, 78, true, false]);
  const whole = await api.getMessages(ref, -1, PAGE_SIZE);
  assert.deepEqual([whole.messages.length, whole.offset, whole.capped], [50, 30, undefined]);
  await assert.rejects(api.getMessages(ref, -1, 50, { maxBytes: -1, anchor: -1 }));
  await assert.rejects(api.getMessages(ref, -1, 50, { maxBytes: 1, anchor: -2 }));
});

test('earlier pages sit right before the window, by the offset the page reports', async () => {
  const api = mockWith(80, 100 * 1024);
  const latest = await api.getMessages(ref, -1, PAGE_SIZE, pageCap(-1));
  // loadEarlierMessages: [start, end) asked, the cap keeps the end.
  const end = latest.offset;
  const start = Math.max(0, end - PAGE_SIZE);
  const page = await api.getMessages(ref, start, end - start, pageCap(end - 1));
  assert.ok(page.capped && page.offset > start);
  assert.equal(page.offset + page.messages.length, end);
  assert.equal(page.messages.at(-1).id, `m-${end - 1}`);
  // loadMoreMessages: [end, end+limit), the cap keeps the start.
  const more = await api.getMessages(ref, 10, PAGE_SIZE, pageCap(10));
  assert.deepEqual([more.offset, more.messages[0].id, more.hasMore], [10, 'm-10', true]);
  // A search jump keeps its match.
  const jump = await api.getMessages(ref, 20, PAGE_SIZE, pageCap(40));
  assert.ok(jump.messages.some((m) => m.id === 'm-40') && jump.messages.length < PAGE_SIZE);
});

test('a capped window refreshes with its own length, not a full page', async () => {
  const api = mockWith(80, 100 * 1024);
  const calls = [];
  const pager = {
    getMessages: (...args) => { calls.push(['page', ...args.slice(1, 3)]); return api.getMessages(...args); },
    getMessagesSince: async (req) => { calls.push(['since', req.offset, req.limit]); return api.getMessagesSince(req); },
  };
  const refresher = new TranscriptRefresher(pager, () => 1);
  let win = windowOf(await api.getMessages(ref, -1, PAGE_SIZE, pageCap(-1)));
  assert.equal(win.capped, true);
  assert.equal(refreshLimit(win, PAGE_SIZE), 2);
  api.messages[ref.id].push({ id: 'new', role: 'assistant', time: '', parts: [{ kind: 'text', text: 'small' }] });
  win = await refresher.refresh(ref, win, -1, refreshLimit(win, PAGE_SIZE));
  assert.deepEqual(win.messages.map((m) => m.id), ['m-79', 'new']);
  assert.equal(win.capped, true, 'still capped after a delta');
  assert.deepEqual(calls, [['since', -1, 2]]);
  // Without deltas (an older host), the full-page fallback keeps it too.
  const old = new TranscriptRefresher({ getMessages: pager.getMessages, getMessagesSince: async () => ({ supported: false, host: '', generation: 0 }) }, () => 1);
  win = await old.refresh(ref, win, -1, refreshLimit(win, PAGE_SIZE));
  assert.deepEqual([win.messages.length, win.capped], [2, true]);
  // An uncapped window still asks for at least a page.
  assert.equal(refreshLimit({ messages: [1, 2], offset: 0, hasMore: false }, PAGE_SIZE), 50);
  assert.equal(refreshLimit({ messages: Array(70), offset: 0, hasMore: false }, PAGE_SIZE), 70);
});

test('the desktop API uses GetMessagesCapped for a cap, and GetMessages otherwise or on older Apps', async () => {
  const seen = [];
  const page = { messages: [], offset: 0, limit: 50, totalCount: 0, hasMore: false };
  const binding = {
    GetMessages: async (...args) => { seen.push(['GetMessages', ...args.slice(1)]); return page; },
    GetMessagesCapped: async (...args) => { seen.push(['GetMessagesCapped', ...args.slice(1)]); return page; },
  };
  await withDesktopApp(binding, async (api) => {
    await api.getMessages(ref, -1, 50, pageCap(-1));
    await api.getMessages(ref, 3, 50);
    delete binding.GetMessagesCapped;
    await api.getMessages(ref, -1, 50, pageCap(-1));
  });
  assert.deepEqual(seen, [
    ['GetMessagesCapped', -1, 50, { maxBytes: PAGE_BYTES, anchor: -1 }],
    ['GetMessages', 3, 50],
    ['GetMessages', -1, 50],
  ]);
});

test('TranscriptView asks for capped pages with each request\'s anchor', () => {
  const source = readFileSync(new URL('../src/lib/components/transcript/TranscriptView.svelte', import.meta.url), 'utf8');
  const calls = source.match(/api\.getMessages\([^\n]*\)/g) || [];
  assert.deepEqual(calls, [
    'api.getMessages(ref, -1, PAGE_SIZE, pageCap(-1))',
    'api.getMessages(jump.ref, offset, PAGE_SIZE, pageCap(Math.max(jump.messageIndex, 0)))',
    'api.getMessages(ref, start, end - start, pageCap(end - 1))',
    'api.getMessages(ref, end, limit, pageCap(end))',
    'api.getMessages(ref, -1, PAGE_SIZE, pageCap(-1))',
  ]);
  assert.match(source, /pageOffset = page\.offset;/);
  assert.doesNotMatch(source, /pageOffset = start;/);
  assert.match(source, /refresher\.refresh\(ref, win, latest \? -1 : win\.offset, refreshLimit\(win, PAGE_SIZE\)\)/);
  assert.match(source, /windowCapped = Boolean\(win\.capped\);/);
});
