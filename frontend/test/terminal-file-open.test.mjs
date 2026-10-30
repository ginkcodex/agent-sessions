import test from 'node:test';
import assert from 'node:assert/strict';
import { createTerminalFileOpener, terminalFileHostReady } from '../src/lib/terminalFileOpen.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { createAPI } from '../src/lib/api.ts';

const local = { phase: 'local', stale: false, locked: false };
function setup() {
  const calls = [], errors = [];
  let state = local, version = 0;
  const files = { dir: null, listError: null, async show(dir) { this.dir = dir; calls.push(['show', dir]); }, openFile(path, opts) { calls.push(['open', path, opts]); } };
  const deps = {
    link: () => state, epoch: () => version,
    async resolve(dir, path) { calls.push(['resolve', dir, path]); return { dir: '/repo', path: 'docs/formats.md' }; },
    files, report: msg => errors.push(msg), isStale: () => false,
  };
  return { calls, errors, deps, files, setState: s => state = s, bump: () => version++ };
}

test('terminal file opener resolves then opens a kept Go to File tab', async () => {
  const s = setup();
  await createTerminalFileOpener(s.deps).open('', 'docs/formats.md', '/repo');
  assert.deepEqual(s.calls, [['resolve', '/repo', 'docs/formats.md'], ['show', '/repo'], ['open', 'docs/formats.md', { keep: true }]]);
  assert.deepEqual(s.errors, []);
});

test('terminal file opener blocks mismatched and disconnected hosts before any request', async () => {
  const s = setup();
  await createTerminalFileOpener(s.deps).open('server', 'Makefile', '/repo');
  assert.equal(s.calls.length, 0);
  assert.match(s.errors[0], /terminal’s host first/);
  assert.equal(terminalFileHostReady('server', { ...local, phase: 'disconnected', host: 'server', dataHost: 'server' }), false);
  assert.equal(terminalFileHostReady('server', { ...local, phase: 'connected', host: 'server', dataHost: 'server' }), true);
  assert.equal(terminalFileHostReady('', { ...local, locked: true }), false);
});

test('late terminal resolution is dropped after a host epoch change or view disposal', async () => {
  for (const action of ['epoch', 'dispose']) {
    const s = setup();
    let finish;
    s.deps.resolve = () => new Promise(resolve => { finish = resolve; });
    const opener = createTerminalFileOpener(s.deps);
    const pending = opener.open('', 'Makefile', '/repo');
    if (action === 'epoch') s.bump(); else opener.dispose();
    finish({ dir: '/repo', path: 'Makefile' });
    await pending;
    assert.deepEqual(s.calls, []);
  }
});

test('terminal opener does not open a file when listing fails or a newer click wins', async () => {
  const s = setup();
  s.files.listError = 'Cannot list project';
  await createTerminalFileOpener(s.deps).open('', 'Makefile', '/repo');
  assert.equal(s.calls.some(c => c[0] === 'open'), false);
  assert.deepEqual(s.errors, ['Cannot list project']);
  const t = setup();
  const pending = [];
  t.deps.resolve = () => new Promise(resolve => pending.push(resolve));
  const opener = createTerminalFileOpener(t.deps);
  const first = opener.open('', 'old', '/repo');
  const second = opener.open('', 'new', '/repo');
  pending[1]({ dir: '/repo', path: 'new' });
  await second;
  pending[0]({ dir: '/repo', path: 'old' });
  await first;
  assert.deepEqual(t.calls.filter(c => c[0] === 'open'), [['open', 'new', { keep: true }]]);
});

test('message ownership changes suppress late resolution and location callbacks', async () => {
  const s = setup();
  let valid = true, finish, opened = 0;
  s.deps.resolve = () => new Promise(resolve => { finish = resolve; });
  const opener = createTerminalFileOpener(s.deps);
  const pending = opener.open('', 'file.ts', '/session', { valid: () => valid, opened: () => opened++ });
  valid = false;
  finish({ dir: '/session', path: 'file.ts' });
  await pending;
  assert.deepEqual(s.calls, []);
  assert.equal(opened, 0);
  await opener.open('', 'file.ts', '/session', { valid: () => false });
  assert.deepEqual(s.calls, []);
});

test('location callback runs only after successful kept-file opening', async () => {
  const s = setup();
  await createTerminalFileOpener(s.deps).open('', 'file.ts', '/session', {
    valid: () => true,
    opened: target => s.calls.push(['reveal', target]),
  });
  assert.deepEqual(s.calls.slice(-2), [['open', 'docs/formats.md', { keep: true }], ['reveal', { dir: '/repo', path: 'docs/formats.md' }]]);
});

test('terminal resolver passes cwd and path through native bindings', async () => {
  const previous = globalThis.window;
  const calls = [];
  globalThis.window = { go: { app: { App: { async ResolveTerminalFile(...args) {
    calls.push(args);
    return { dir: '/repo', path: 'Makefile' };
  } } } } };
  try {
    assert.deepEqual(await createAPI().resolveTerminalFile('/repo/sub', '../Makefile'), { dir: '/repo', path: 'Makefile' });
    assert.deepEqual(calls, [['/repo/sub', '../Makefile']]);
  } finally {
    if (previous === undefined) delete globalThis.window;
    else globalThis.window = previous;
  }
  const mock = new MockBackendAPI();
  assert.deepEqual(await mock.resolveTerminalFile('/repo', 'Makefile'), { dir: '/repo', path: 'Makefile' });
  await assert.rejects(mock.resolveTerminalFile('/repo', 'missing'), /File not found/);
});
