import test from 'node:test';
import assert from 'node:assert/strict';
import { createAPI } from '../src/lib/api.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

async function withBinding(binding, fn) {
  const previous = globalThis.window;
  globalThis.window = { go: { app: { App: binding } } };
  try { await fn(createAPI()); }
  finally { if (previous === undefined) delete globalThis.window; else globalThis.window = previous; }
}

const req = { ref: { agent: 'claude', id: 's' }, baseRevision: 'e.1.2', haveOffset: 0, haveCount: 50, offset: -1, limit: 50 };

test('delta API passes the request and preserves the reply; offline errors remain errors', async () => {
  const reply = {
    supported: true, host: 'box', generation: 3,
    delta: { messages: [], offset: 10, limit: 50, totalCount: 60, hasMore: false, revision: 'e.1.3', reused: 50 },
  };
  let seen;
  const binding = { GetMessagesSince: async r => { seen = r; return reply; } };
  await withBinding(binding, async api => {
    assert.deepEqual(await api.getMessagesSince(req), reply);
    assert.deepEqual(seen, req);
    binding.GetMessagesSince = async () => ({ supported: false, host: 'old', generation: 4 });
    assert.deepEqual(await api.getMessagesSince(req), { supported: false, host: 'old', generation: 4 });
    binding.GetMessagesSince = async () => { throw 'disconnected'; };
    await assert.rejects(api.getMessagesSince(req), /disconnected/);
  });
});

test('old App bindings and a mock without deltas report deltas unsupported', async () => {
  const unsupported = { supported: false, host: '', generation: 0 };
  await withBinding({}, async api => assert.deepEqual(await api.getMessagesSince(req), unsupported));
  const mock = new MockBackendAPI();
  mock.transcriptDelta = false;
  assert.deepEqual(await mock.getMessagesSince(req), unsupported);
});

test('the browser mock answers deltas and validates them like getMessages', async () => {
  const mock = new MockBackendAPI();
  const reply = await mock.getMessagesSince(req);
  // An unknown base revision resends the whole page.
  assert.equal(reply.supported, true);
  assert.equal(reply.delta.reused, 0);
  assert.deepEqual({ ...reply.delta, reused: undefined }, { ...(await mock.getMessages(req.ref, -1, 50)), reused: undefined });
  for (const bad of [{ offset: -2 }, { limit: 0 }, { haveOffset: -1 }, { haveCount: -1 }]) {
    await assert.rejects(mock.getMessagesSince({ ...req, ...bad }), /Invalid page/);
  }
  await assert.rejects(mock.getMessages(req.ref, -2, 50), /Invalid page/);
});
