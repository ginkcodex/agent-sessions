// The Settings check button's state: Checking…, then what the check found
// until the update state says more.
import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule } from './helpers.mjs';

const STORE_URL = new URL('../src/lib/stores/updates.svelte.ts', import.meta.url);
const lib = (rel) => JSON.stringify(new URL(rel, STORE_URL).href);

// The backend's update state, and the event that announces its changes.
globalThis.__updatesBackend = { status: { state: 'idle', supported: true }, listener: null };
const { UpdatesStore } = await loadServerModule(STORE_URL, [
  [
    "import { api, subscribeUpdateState } from '../api';",
    `const api = { updateState: async () => globalThis.__updatesBackend.status, autoUpdate: async () => true };
     const subscribeUpdateState = (fn) => { globalThis.__updatesBackend.listener = fn; return () => {}; };`,
  ],
  ["'../manage'", lib('../manage.ts')],
]);
const backend = globalThis.__updatesBackend;

const latest = { current: '1.5.5', latest: '1.5.5', url: '', available: false };
const newer = { current: '1.5.5', latest: '1.6.0', url: 'https://example.test/v1.6.0', available: true };

test('without a checker the store cannot check', async () => {
  const store = new UpdatesStore();
  assert.equal(store.canCheck, false);
  await store.checkNow();
  assert.equal(store.checkResult, null);
  store.setChecker(async () => latest);
  assert.equal(store.canCheck, true);
  store.setChecker(null);
  assert.equal(store.canCheck, false);
});

test('a check by hand shows Checking…, then what it found', async () => {
  const store = new UpdatesStore();
  let finish;
  let calls = 0;
  store.setChecker(() => (calls++, new Promise((resolve) => (finish = resolve))));
  const running = store.checkNow();
  assert.equal(store.checking, true);
  void store.checkNow(); // a second click while it runs does nothing
  finish(latest);
  await running;
  assert.equal(calls, 1);
  assert.equal(store.checking, false);
  assert.deepEqual(store.checkResult, { info: latest });
});

test('a failed check by hand keeps its reason', async () => {
  const store = new UpdatesStore();
  store.setChecker(async () => { throw new Error('dial tcp: no route to host'); });
  await store.checkNow();
  assert.deepEqual(store.checkResult, { error: 'dial tcp: no route to host' });
  store.setChecker(async () => { throw {}; });
  await store.checkNow();
  assert.deepEqual(store.checkResult, { error: 'The check failed' });
});

test('the update state replaces what the check found', async () => {
  const store = new UpdatesStore();
  await store.init();
  store.setChecker(async () => latest);
  await store.checkNow();
  assert.deepEqual(store.checkResult, { info: latest });
  // The daily check in another window finds a release later.
  backend.listener({ state: 'downloading', version: '1.6.0', supported: true });
  assert.equal(store.checkResult, null);

  // A release found by the check itself shows as the update state at once.
  store.setChecker(async () => {
    backend.listener({ state: 'ready', version: '1.6.0', supported: true });
    return newer;
  });
  await store.checkNow();
  assert.equal(store.status.state, 'ready');
  assert.equal(store.checkResult, null);

  // With no download to show (no updater here), the check says it.
  backend.listener({ state: 'idle', supported: false });
  store.setChecker(async () => newer);
  await store.checkNow();
  assert.deepEqual(store.checkResult, { info: newer });
  store.destroy();
});
