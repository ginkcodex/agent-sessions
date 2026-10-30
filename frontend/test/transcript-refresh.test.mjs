import test from 'node:test';
import assert from 'node:assert/strict';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { applyDelta, mixesRevisions, TranscriptRefresher, windowOf } from '../src/lib/transcriptRefresh.ts';

const ref = { agent: 'claude', id: 'refresh-fixture' };
const message = (id, text = id) => ({ id, role: 'assistant', time: '', parts: [{ kind: 'text', text }] });

// A mock with n messages, counting the calls a refresh makes.
function setup(n = 120) {
  const api = new MockBackendAPI();
  api.messages[ref.id] = Array.from({ length: n }, (_, i) => message(`m-${i}`));
  const calls = { pages: 0, deltas: [] };
  const pager = {
    getMessages: (...args) => { calls.pages++; return api.getMessages(...args); },
    getMessagesSince: async (req) => {
      const reply = await api.getMessagesSince(req);
      calls.deltas.push(reply);
      return reply;
    },
  };
  let epoch = 1;
  const refresher = new TranscriptRefresher(pager, () => epoch);
  return { api, calls, refresher, list: () => api.messages[ref.id], reconnect: () => epoch++ };
}

async function fresh(api, offset, limit) {
  return windowOf(await api.getMessages(ref, offset, limit));
}

test('a tail append while following fetches only the new messages and keeps the rest', async () => {
  const { api, calls, refresher, list } = setup();
  const win = windowOf(await api.getMessages(ref, -1, 50));
  list().push(message('new-1'), message('new-2'));
  const next = await refresher.refresh(ref, win, -1, Math.max(win.messages.length, 50));
  assert.deepEqual(next, await fresh(api, -1, 50));
  assert.equal(next.offset, 72);
  const delta = calls.deltas.at(-1).delta;
  assert.equal(delta.reused, 48);
  assert.deepEqual(delta.messages.map((m) => m.id), ['new-1', 'new-2']);
  assert.equal(calls.pages, 0);
  // Reused rows are the window's own objects, so their rows do not re-render.
  assert.equal(next.messages[0], win.messages[2]);
  assert.notEqual(next.revision, win.revision);

  // Unchanged: nothing is resent.
  const same = await refresher.refresh(ref, next, -1, 50);
  assert.equal(calls.deltas.at(-1).delta.messages.length, 0);
  assert.deepEqual(same, next);
});

test('a reading-history window refreshes in place', async () => {
  const { api, calls, refresher, list } = setup();
  const win = windowOf(await api.getMessages(ref, 20, 30));
  list().push(message('new'));
  const next = await refresher.refresh(ref, win, win.offset, Math.max(win.messages.length, 50));
  assert.deepEqual(next, await fresh(api, 20, 50));
  // The window's 30 messages are reused; the other 20 are new to the client.
  assert.equal(calls.deltas.at(-1).delta.reused, 30);
  assert.equal(next.messages.length, 50);
  assert.equal(next.hasMore, true);
});

test('an earlier tool-result change resends from the changed message', async () => {
  const { api, calls, refresher, list } = setup();
  const win = windowOf(await api.getMessages(ref, -1, 50));
  // A late tool result completes a message in the middle of the window.
  list()[100] = { ...list()[100], parts: [...list()[100].parts, { kind: 'tool', tool: { name: 'Bash', output: 'done' } }] };
  const next = await refresher.refresh(ref, win, -1, 50);
  assert.deepEqual(next, await fresh(api, -1, 50));
  const delta = calls.deltas.at(-1).delta;
  assert.equal(delta.reused, 30);
  assert.equal(delta.messages[0].parts.length, 2);
  assert.equal(next.messages[29], win.messages[29]);
  assert.notEqual(next.messages[30], win.messages[30]);
});

test('a rewrite or an unknown revision resets to a full page', async () => {
  const { api, calls, refresher, list } = setup();
  const win = windowOf(await api.getMessages(ref, -1, 50));
  // Truncated and rewritten: nothing before the window's end survives.
  api.messages[ref.id] = Array.from({ length: 60 }, (_, i) => message(`r-${i}`));
  let next = await refresher.refresh(ref, win, -1, 50);
  assert.equal(calls.deltas.at(-1).delta.reused, 0);
  assert.deepEqual(next, await fresh(api, -1, 50));

  // A base the host no longer knows, e.g. after a restart, is a reset too.
  list().push(message('after'));
  next = await refresher.refresh(ref, { ...next, revision: 'gone.1.1' }, -1, 50);
  assert.equal(calls.deltas.at(-1).delta.reused, 0);
  assert.deepEqual(next, await fresh(api, -1, 50));
  assert.equal(calls.pages, 0);
});

test('an unsupported host switches its connection to full pages until it reconnects', async () => {
  const { api, calls, refresher, list, reconnect } = setup();
  api.transcriptDelta = false;
  let win = windowOf(await api.getMessages(ref, -1, 50));
  list().push(message('a'));
  win = await refresher.refresh(ref, win, -1, 50);
  assert.deepEqual(win, await fresh(api, -1, 50));
  assert.equal(calls.deltas.length, 1);
  assert.equal(calls.pages, 1);
  // No further delta requests on this connection.
  list().push(message('b'));
  win = await refresher.refresh(ref, win, -1, 50);
  assert.deepEqual(win, await fresh(api, -1, 50));
  assert.equal(calls.deltas.length, 1);
  assert.equal(calls.pages, 2);

  // A reconnect may reach an upgraded host: deltas are tried again.
  reconnect();
  api.transcriptDelta = true;
  list().push(message('c'));
  win = await refresher.refresh(ref, win, -1, 50);
  assert.deepEqual(win, await fresh(api, -1, 50));
  assert.equal(calls.deltas.length, 2);
  assert.equal(calls.deltas.at(-1).delta.reused, 49);
  assert.equal(calls.pages, 2);
});

test('pages without revisions, from older hosts, never ask for deltas', async () => {
  const { calls, refresher, list } = setup();
  const win = { messages: list().slice(70), offset: 70, hasMore: false };
  list().push(message('new'));
  const next = await refresher.refresh(ref, win, -1, 50);
  assert.equal(calls.deltas.length, 0);
  assert.equal(calls.pages, 1);
  assert.equal(next.messages.at(-1).id, 'new');
});

test('superseded reloads each describe their own window', async () => {
  const { api, list } = setup();
  // Replies are held, then answered newest first.
  const held = [];
  const pager = {
    getMessages: (...args) => api.getMessages(...args),
    getMessagesSince: (req) => {
      const reply = api.getMessagesSince(req);
      return new Promise((resolve) => held.push(() => resolve(reply)));
    },
  };
  const refresher = new TranscriptRefresher(pager, () => 1);
  const win = windowOf(await api.getMessages(ref, -1, 50));
  list().push(message('first'));
  const older = refresher.refresh(ref, win, -1, 50);
  const olderPage = await fresh(api, -1, 50);
  // A newer reload starts from a window that grew into history meanwhile.
  const earlier = await api.getMessages(ref, win.offset - 20, 20);
  const grown = { ...win, offset: earlier.offset, messages: [...earlier.messages, ...win.messages] };
  list().push(message('second'));
  const newer = refresher.refresh(ref, grown, -1, grown.messages.length);
  const newerPage = await fresh(api, -1, 70);
  held[1]();
  assert.deepEqual(await newer, newerPage);
  held[0]();
  // The late older reply is still a consistent page of its own window and
  // revision, so whichever one the view keeps never mixes the two.
  assert.deepEqual(await older, olderPage);
  assert.notEqual(olderPage.revision, newerPage.revision);
});

test('a page from another revision marks the window as mixed until refreshed', async () => {
  const { api, calls, refresher, list } = setup();
  let win = windowOf(await api.getMessages(ref, -1, 50));
  // A message in the window changed, then history is loaded from the new revision.
  list()[100] = message('m-100', 'edited');
  list().push(message('new'));
  const page = await api.getMessages(ref, win.offset - 20, 20);
  assert.ok(mixesRevisions(win, page));
  // The view splices the page in but keeps the window's older revision...
  win = { ...win, offset: page.offset, messages: [...page.messages, ...win.messages] };
  assert.equal(win.messages[50].parts[0].text, 'm-100');
  // ...so the refresh resends everything changed since, wherever it is.
  const next = await refresher.refresh(ref, win, win.offset, win.messages.length);
  assert.deepEqual(next, await fresh(api, win.offset, win.messages.length));
  assert.equal(calls.deltas.at(-1).delta.reused, 50);
  assert.equal(next.messages[50].parts[0].text, 'edited');
  assert.ok(!mixesRevisions(next, await api.getMessages(ref, 0, 10)));
  // Without revisions, nothing is mixed.
  assert.ok(!mixesRevisions({ ...next, revision: undefined }, page));
  assert.ok(!mixesRevisions(next, { ...page, revision: undefined }));
});

test('a delta that does not fit the window is rejected for a full page', async () => {
  const win = { messages: [message('a'), message('b')], offset: 10, hasMore: false, revision: 'r1' };
  const delta = (offset, reused) => ({ messages: [message('c')], offset, limit: 50, totalCount: 13, hasMore: false, revision: 'r2', reused });
  assert.equal(applyDelta(win, delta(9, 1)), null);
  assert.equal(applyDelta(win, delta(11, 2)), null);
  assert.equal(applyDelta(win, delta(10, -1)), null);
  assert.deepEqual(applyDelta(win, delta(11, 1)).messages.map((m) => m.id), ['b', 'c']);
  assert.deepEqual(applyDelta(win, delta(5, 0)).messages.map((m) => m.id), ['c']);

  // The refresher then falls back to getMessages.
  let pages = 0;
  const pager = {
    getMessages: async () => { pages++; return { messages: [], offset: 0, limit: 50, totalCount: 0, hasMore: false }; },
    getMessagesSince: async () => ({ supported: true, host: '', generation: 0, delta: delta(9, 1) }),
  };
  await new TranscriptRefresher(pager, () => 0).refresh(ref, win, -1, 50);
  assert.equal(pages, 1);
});

test('windows refreshed only by deltas always equal fresh pages', async () => {
  const { api, calls, refresher, list } = setup(80);
  let seed = 7;
  const random = (n) => { seed = (seed * 1103515245 + 12345) % 2147483648; return seed % n; };
  let id = 0;
  let win = windowOf(await api.getMessages(ref, -1, 50));
  for (let round = 0; round < 300; round++) {
    const msgs = list();
    switch (random(5)) {
      case 0: msgs.push(message(`n-${id++}`)); break;
      case 1: if (msgs.length) msgs.splice(msgs.length - 1 - random(Math.min(3, msgs.length)), 1); break;
      case 2: if (msgs.length) msgs[random(msgs.length)] = message(`e-${id++}`); break;
      case 3: if (random(10) === 0) api.revisions[ref.id].history = []; break;
    }
    const offset = random(3) === 0 ? random(msgs.length + 2) : random(2) === 0 ? -1 : win.offset;
    const limit = offset === win.offset || offset === -1 ? Math.max(win.messages.length, 50) : 1 + random(60);
    win = await refresher.refresh(ref, win, offset, limit);
    assert.deepEqual(win, await fresh(api, offset, limit), `round ${round}`);
    assert.ok(win.messages.length <= limit);
  }
  // Both paths ran often: partial reuse and full resends.
  const reused = calls.deltas.filter((r) => r.delta.reused > 0 && r.delta.messages.length > 0).length;
  const resets = calls.deltas.filter((r) => r.delta.reused === 0).length;
  assert.equal(calls.pages, 0);
  assert.ok(reused >= 60 && resets >= 30, `reused ${reused} resets ${resets}`);
});
