import test from 'node:test';
import assert from 'node:assert/strict';
import { TerminalResize } from '../src/lib/terminalResize.ts';

const wait = () => new Promise((resolve) => setTimeout(resolve));
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};

function setup(t) {
  const calls = [];
  const pending = [];
  const resizer = new TerminalResize((cols, rows) => {
    calls.push([cols, rows]);
    const call = deferred();
    pending.push(call);
    return call.promise;
  }, 0, 0);
  t.after(() => resizer.dispose());
  return { resizer, calls, pending };
}

test('terminal resizes serialize RPCs and coalesce to the latest geometry', async (t) => {
  const { resizer, calls, pending } = setup(t);
  resizer.request(80, 24);
  await wait();
  resizer.request(132, 40);
  resizer.request(100, 30);
  await wait();
  assert.deepEqual(calls, [[80, 24]], 'no second RPC until the first completes');
  pending.shift().resolve();
  await wait();
  await wait();
  assert.deepEqual(calls, [[80, 24], [100, 30]]);
  pending.shift().resolve();
  await wait();
  resizer.request(100, 30);
  await wait();
  assert.equal(calls.length, 2, 'successful unchanged sizes are not sent again');
});

test('failed resizes retry without another layout event', async (t) => {
  const { resizer, calls, pending } = setup(t);
  resizer.request(80, 24);
  await wait();
  pending.shift().reject(new Error('resize failed'));
  await wait();
  await wait();
  assert.deepEqual(calls, [[80, 24], [80, 24]]);
  pending.shift().resolve();
  await wait();
});

test('returning to a previous width while an RPC is pending still restores it', async (t) => {
  const { resizer, calls, pending } = setup(t);
  resizer.request(80, 24);
  await wait();
  pending.shift().resolve();
  await wait();
  resizer.request(100, 30);
  await wait();
  resizer.request(80, 24);
  pending.shift().resolve();
  await wait();
  await wait();
  assert.deepEqual(calls, [[80, 24], [100, 30], [80, 24]]);
  pending.shift().resolve();
});

test('disposal prevents pending timers, in-flight completions and retries from sending', async (t) => {
  const { resizer, calls, pending } = setup(t);
  resizer.request(80, 24);
  await wait();
  resizer.request(100, 30);
  resizer.dispose();
  pending.shift().reject(new Error('terminal closed'));
  resizer.request(132, 40);
  await wait();
  await wait();
  assert.deepEqual(calls, [[80, 24]]);
});
