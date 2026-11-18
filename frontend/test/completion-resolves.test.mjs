import test from 'node:test';
import assert from 'node:assert/strict';
import { CompletionResolves } from '../src/lib/completionResolves.ts';

// A source whose resolves wait until released or cancelled.
function source() {
  let ids = 0;
  const asked = [], cancels = [], held = new Map();
  const src = {
    newId: () => `r${++ids}`,
    ask: (requestId, index, version) => {
      asked.push([requestId, index, version]);
      return new Promise((resolve, reject) => held.set(requestId, {
        release: () => resolve({ documentId: 'd', resultId: 'x', index, detail: `${index}@${version}` }),
        fail: () => reject(new Error('failed')),
      }));
    },
    cancel: requestId => { cancels.push(requestId); held.get(requestId)?.fail(); },
  };
  return { src, asked, cancels, held, last: () => held.get(asked.at(-1)[0]) };
}

const tick = () => new Promise(r => setTimeout(r, 0));
const state = p => Promise.race([p.then(d => d.detail, () => 'rejected'), tick().then(() => 'waiting')]);

test('a caller that aborts stops waiting at once; the others keep the shared resolve', async () => {
  const s = source(), r = new CompletionResolves(s.src);
  const panel = new AbortController(), accept = new AbortController();
  const shown = r.at(0, 3, panel.signal), edits = r.at(0, 3, accept.signal);
  assert.equal(s.asked.length, 1, 'asked once');
  panel.abort();
  assert.equal(await state(shown), 'rejected', 'the aborted caller');
  assert.equal(await state(edits), 'waiting', 'the other caller');
  assert.deepEqual(s.cancels, [], 'still waited for');
  s.last().release();
  assert.equal((await edits).detail, '0@3');
  assert.equal(r.settled(0, 3).detail, '0@3');
});

test('once no caller waits the resolve is cancelled, and asked again next time', async () => {
  const s = source(), r = new CompletionResolves(s.src);
  const one = new AbortController(), two = new AbortController();
  const a = r.at(1, 0, one.signal), b = r.at(1, 0, two.signal);
  one.abort();
  assert.deepEqual(s.cancels, []);
  two.abort();
  assert.deepEqual(s.cancels, ['r1']);
  assert.equal(await state(a), 'rejected');
  assert.equal(await state(b), 'rejected');
  assert.deepEqual(r.keys(), { resolving: [], settled: [] });
  const again = r.at(1, 0);
  assert.equal(s.asked.length, 2);
  s.last().release();
  assert.equal((await again).detail, '1@0');
  // An aborted signal never asks.
  const gone = new AbortController();
  gone.abort();
  assert.equal(await state(r.at(2, 0, gone.signal)), 'rejected');
  assert.equal(s.asked.length, 2);
});

test('a caller without a signal keeps the resolve; a failure is asked again', async () => {
  const s = source(), r = new CompletionResolves(s.src);
  const plain = r.at(0, 0), signalled = new AbortController();
  r.at(0, 0, signalled.signal).catch(() => {});
  signalled.abort();
  assert.deepEqual(s.cancels, [], 'the caller without a signal still waits');
  s.last().fail();
  await assert.rejects(plain);
  assert.deepEqual(r.keys(), { resolving: [], settled: [] });
  const again = r.at(0, 0);
  assert.equal(s.asked.length, 2);
  s.last().release();
  assert.equal((await again).detail, '0@0');
});

test('only version 0 and the latest version asked are kept', async () => {
  const s = source(), r = new CompletionResolves(s.src);
  const settle = async p => { s.last().release(); return p; };
  await settle(r.at(0, 1));
  await settle(r.at(1, 0));
  const old = r.at(2, 1); // still answering when version 2 is asked
  assert.deepEqual(r.keys(), { resolving: ['0@1', '1@0', '2@1'], settled: ['0@1', '1@0'] });
  await settle(r.at(0, 2));
  assert.deepEqual(r.keys(), { resolving: ['0@2', '1@0'], settled: ['0@2', '1@0'] });
  // The old version's resolve answers its caller but is not stored.
  s.held.get(s.asked.find(a => a[1] === 2)[0]).release();
  assert.equal((await old).detail, '2@1');
  assert.deepEqual(r.keys().settled, ['0@2', '1@0']);
  assert.equal(r.settled(0, 1), undefined);
  // Version 0's is read again without asking.
  const before = s.asked.length;
  assert.equal((await r.at(1, 0)).detail, '1@0');
  assert.equal(s.asked.length, before);
  // Version 10 drops version 2 but keeps version 0, also beside `@10`.
  await settle(r.at(1, 10));
  assert.deepEqual(r.keys().settled, ['1@0', '1@10']);
});
