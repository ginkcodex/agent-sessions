import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule } from './helpers.mjs';
import { ROUTER_CALL } from '../src/lib/desktopRuntime.ts';
import { flushRuntime, installDesktopRuntime } from './desktop-app.mjs';

const STORE_URL = new URL('../src/lib/stores/theme.svelte.ts', import.meta.url);
const { ThemeStore } = await loadServerModule(STORE_URL, [
  ["'../theme'", JSON.stringify(new URL('../src/lib/theme.ts', import.meta.url).href)],
  ["'../desktopRuntime'", JSON.stringify(new URL('../src/lib/desktopRuntime.ts', import.meta.url).href)],
]);

function fixture(t, { saved = null, dark = false, desktop = true, reply = async () => {} } = {}) {
  const calls = [];
  const restoreRuntime = installDesktopRuntime({
    Call: { ByName: async (...args) => { calls.push(args); return reply(...args); } },
    Events: { On() { return () => {}; } },
  });
  if (!desktop) delete window.__AGENT_SESSIONS_DESKTOP__;
  const listeners = new Set();
  const media = {
    matches: dark,
    addEventListener(_, fn) { listeners.add(fn); },
    removeEventListener(_, fn) { listeners.delete(fn); },
  };
  window.matchMedia = () => media;
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document');
  globalThis.localStorage = { getItem: () => saved, setItem: (_, value) => { saved = value; } };
  let domTheme;
  globalThis.document = { documentElement: { setAttribute: (_, value) => { domTheme = value; } } };
  const store = new ThemeStore();
  t.after(() => {
    store.destroy();
    restoreRuntime();
    for (const [name, descriptor] of [['localStorage', originalStorage], ['document', originalDocument]]) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else delete globalThis[name];
    }
  });
  return {
    store, calls,
    get domTheme() { return domTheme; },
    get saved() { return saved; },
    get listenerCount() { return listeners.size; },
    systemDark(value) {
      media.matches = value;
      for (const listener of listeners) listener({ matches: value });
    },
  };
}

for (const saved of ['light', 'dark', 'system', 'invalid', null]) {
  test(`initialization sends saved mode ${saved} rather than its resolved color`, async (t) => {
    const f = fixture(t, { saved, dark: true });
    f.store.init();
    await flushRuntime();
    const expected = ['light', 'dark', 'system'].includes(saved) ? saved : 'system';
    assert.deepEqual(f.calls, [[ROUTER_CALL, 'SetWindowTheme', [expected]]]);
    assert.equal(f.domTheme, expected === 'light' ? 'light' : 'dark');
  });
}

test('explicit modes ignore system changes; System clears the native override without a media-event loop', async (t) => {
  const f = fixture(t, { saved: 'light', dark: true });
  f.store.init();
  await flushRuntime();
  f.systemDark(false);
  assert.equal(f.domTheme, 'light');
  f.store.toggle(); // light -> dark
  await flushRuntime();
  assert.equal(f.domTheme, 'dark');
  assert.equal(f.saved, 'dark');
  // Simulate the native dark appearance changing the webview media query.
  f.systemDark(true);
  await flushRuntime();
  assert.equal(f.calls.length, 2);
  f.store.toggle(); // dark -> system
  await flushRuntime();
  // Clearing the override restores the actual light system preference.
  f.systemDark(false);
  await flushRuntime();
  assert.equal(f.domTheme, 'light');
  f.systemDark(true);
  await flushRuntime();
  assert.equal(f.domTheme, 'dark');
  assert.deepEqual(f.calls.map((c) => c[2]), [['light'], ['dark'], ['system']]);
  f.store.destroy();
  assert.equal(f.listenerCount, 0);
});

test('rapid mode changes wait for the preceding native call', async (t) => {
  let finish;
  const f = fixture(t, { reply: () => new Promise((resolve) => { finish = resolve; }) });
  f.store.init();
  await flushRuntime();
  f.store.setMode('light');
  f.store.setMode('dark');
  await flushRuntime();
  assert.deepEqual(f.calls.map((c) => c[2]), [['system']]);
  finish();
  await flushRuntime();
  assert.deepEqual(f.calls.map((c) => c[2]), [['system'], ['light']]);
  finish();
  await flushRuntime();
  assert.deepEqual(f.calls.map((c) => c[2]), [['system'], ['light'], ['dark']]);
  finish();
  await flushRuntime();
  assert.equal(f.domTheme, 'dark');
});

test('native failure is reported, leaves the web theme usable and allows later updates', async (t) => {
  const failure = new Error('native unavailable');
  let fail = true;
  const warnings = [];
  t.mock.method(console, 'warn', (...args) => warnings.push(args));
  const f = fixture(t, { saved: 'dark', reply: async () => { if (fail) throw failure; } });
  f.store.init();
  await flushRuntime();
  assert.equal(f.domTheme, 'dark');
  assert.equal(warnings.length, 1);
  assert.equal(warnings[0][1], failure);
  fail = false;
  f.store.setMode('light');
  await flushRuntime();
  assert.equal(f.domTheme, 'light');
  assert.deepEqual(f.calls.map((c) => c[2]), [['dark'], ['light']]);
  assert.equal(warnings.length, 1);
});

test('browser themes update the DOM without native calls', async (t) => {
  const f = fixture(t, { desktop: false });
  f.store.init();
  f.store.setMode('dark');
  await flushRuntime();
  assert.equal(f.domTheme, 'dark');
  assert.deepEqual(f.calls, []);
});
