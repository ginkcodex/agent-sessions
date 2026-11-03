import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { createAPI } from '../src/lib/api.ts';
import { APP_METHODS, createAppBinding } from '../src/lib/appBinding.ts';
import { APP_SERVICE, DesktopBridgeError, FILES_DROPPED_EVENT, isDesktop } from '../src/lib/desktopRuntime.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { showStartupError } from '../src/lib/startupError.ts';
import { flushRuntime, installDesktopRuntime } from './desktop-app.mjs';

const APP = 'github.com/ginkcode/agent-sessions/internal/app.App';

// A stand-in for @wailsio/runtime that records calls and dispatches events.
function fakeRuntime(reply = async () => undefined) {
  const calls = [];
  const listeners = new Map();
  const opened = [];
  const titles = [];
  return {
    calls, listeners, opened, titles,
    emit(name, data) { for (const cb of listeners.get(name) ?? []) cb({ name, data, sender: 'main' }); },
    Call: { ByName: (name, ...args) => { calls.push([name, ...args]); return reply(name, ...args); } },
    Events: { On(name, cb) {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name).add(cb);
      return () => listeners.get(name).delete(cb);
    } },
    Browser: { OpenURL: async (url) => { opened.push(url); } },
    Window: { SetTitle: async (title) => { titles.push(title); } },
  };
}

test('only the marker the shell injects selects the desktop backend', () => {
  const restore = installDesktopRuntime(fakeRuntime());
  try {
    assert.equal(isDesktop(), true);
    assert.ok(!(createAPI() instanceof MockBackendAPI));
    // The runtime's own global is created by importing it, so it proves nothing.
    globalThis.window = { _wails: {}, __AGENT_SESSIONS_DESKTOP__: 'true' };
    assert.equal(isDesktop(), false);
    assert.ok(createAPI() instanceof MockBackendAPI);
  } finally { restore(); }
  assert.ok(createAPI() instanceof MockBackendAPI, 'no window: the browser demo keeps its mock');
});

test('desktop calls go through Call.ByName with the full Go method name', async () => {
  assert.equal(APP_SERVICE, APP);
  const rt = fakeRuntime(async (name) => (name.endsWith('.AppVersion') ? '1.2.3' : { root: '/repo', files: ['a.go'] }));
  const restore = installDesktopRuntime(rt);
  try {
    const api = createAPI();
    assert.equal(await api.appVersion(), '1.2.3');
    assert.deepEqual(await api.listFiles('/repo'), { root: '/repo', files: ['a.go'] });
    await api.terminalResize('t1', 80, 24);
    assert.deepEqual(rt.calls, [
      [`${APP}.AppVersion`],
      [`${APP}.ListFiles`, '/repo'],
      [`${APP}.TerminalResize`, 't1', 80, 24],
    ]);
  } finally { restore(); }
});

test('Go errors keep their message and string rejections still normalize', async () => {
  let failure;
  const restore = installDesktopRuntime(fakeRuntime(async () => { throw failure; }));
  try {
    const api = createAPI();
    failure = Object.assign(new Error('not a session directory'), { name: 'RuntimeError' });
    await assert.rejects(api.gitStatus('/x', ''), (e) => e instanceof Error && e.message === 'not a session directory');
    failure = 'permission denied';
    await assert.rejects(api.deleteFile('/x', 'a', ''), (e) => e instanceof Error && e.message === 'permission denied');
  } finally { restore(); }
});

test('the facade is an explicit object with every App method, so presence checks still work', async () => {
  const calls = [];
  const binding = createAppBinding(async (method, ...args) => { calls.push([method, ...args]); });
  assert.deepEqual(Object.keys(binding), [...APP_METHODS]);
  assert.equal(new Set(APP_METHODS).size, APP_METHODS.length);
  for (const name of ['OpenURL', 'ToggleFullscreen', 'CompleteFileClose', 'ListWSLDistros']) {
    assert.equal(typeof binding[name], 'function', name);
  }
  assert.equal('NotAMethod' in binding, false);
  await binding.CompleteFileClose(7, true);
  assert.deepEqual(calls, [['CompleteFileClose', 7, true]]);
  for (const hidden of ['OnStartup', 'OnShutdown', 'OnBeforeClose', 'SetTransport', 'Close']) {
    assert.ok(!APP_METHODS.includes(hidden), `${hidden} must not be callable from JS`);
  }
});

test('a broken desktop bridge rejects calls visibly instead of using the mock', async () => {
  const restore = installDesktopRuntime(async () => { throw new Error('module not found'); });
  try {
    const api = createAPI();
    assert.ok(!(api instanceof MockBackendAPI));
    await assert.rejects(api.listWorkspaces(), (e) => e instanceof DesktopBridgeError && /module not found/.test(e.message));
    // Events and drops quietly do nothing; startup shows the error once.
    const off = api.onEvent('catalog:changed', () => assert.fail('no events without a bridge'));
    await flushRuntime();
    off();
  } finally { restore(); }
  const incomplete = installDesktopRuntime({ incomplete: true, Call: {} });
  try {
    await assert.rejects(createAPI().appVersion(), (e) => e instanceof DesktopBridgeError);
  } finally { incomplete(); }
});

test('events pass e.data to the callback and each subscription unsubscribes alone', async () => {
  const rt = fakeRuntime();
  const restore = installDesktopRuntime(rt);
  try {
    const api = createAPI();
    const a = [], b = [];
    const offA = api.onEvent('term:data', (data) => a.push(data));
    const offB = api.onEvent('term:data', (data) => b.push(data));
    await flushRuntime();
    rt.emit('term:data', { id: 't1', data: 'x' });
    offA();
    rt.emit('term:data', { id: 't1', data: 'y' });
    assert.deepEqual(a, [{ id: 't1', data: 'x' }]);
    assert.deepEqual(b, [{ id: 't1', data: 'x' }, { id: 't1', data: 'y' }]);
    offB();
    // Unsubscribing before the runtime loads never subscribes.
    const early = api.onEvent('notify:open', () => assert.fail('cancelled before load'));
    early();
    await flushRuntime();
    rt.emit('notify:open', { id: 't1' });
    assert.equal(rt.listeners.get('notify:open')?.size ?? 0, 0);
  } finally { restore(); }
});

test('dropped files map the shell event to (x, y, paths)', async () => {
  const rt = fakeRuntime();
  const restore = installDesktopRuntime(rt);
  try {
    const api = createAPI();
    const drops = [];
    const off = api.onFileDrop((x, y, paths) => drops.push([x, y, paths]));
    await flushRuntime();
    rt.emit(FILES_DROPPED_EVENT, { filenames: ['/a b.txt', '/c'], x: 12, y: 34, elementDetails: {} });
    rt.emit(FILES_DROPPED_EVENT, { filenames: [], x: 1, y: 1 });
    rt.emit(FILES_DROPPED_EVENT, null);
    off();
    rt.emit(FILES_DROPPED_EVENT, { filenames: ['/late'], x: 0, y: 0 });
    assert.deepEqual(drops, [[12, 34, ['/a b.txt', '/c']]]);
  } finally { restore(); }
});

test('openURL prefers the Go binding, then the runtime browser', async () => {
  let fail = false;
  const rt = fakeRuntime(async () => { if (fail) throw new Error('no handler'); });
  const restore = installDesktopRuntime(rt);
  try {
    const api = createAPI();
    await api.openURL('https://example.com/a');
    assert.deepEqual(rt.calls, [[`${APP}.OpenURL`, 'https://example.com/a']]);
    assert.deepEqual(rt.opened, []);
    fail = true;
    await api.openURL('https://example.com/b');
    assert.deepEqual(rt.opened, ['https://example.com/b']);
  } finally { restore(); }
});

test('a startup failure is shown on the page, once', () => {
  const { window } = new JSDOM('<!doctype html><body><div id="app"></div></body>');
  const doc = window.document;
  showStartupError(new DesktopBridgeError(new Error('boom')), doc);
  showStartupError('again', doc);
  const boxes = doc.querySelectorAll('#startup-error');
  assert.equal(boxes.length, 1);
  assert.equal(boxes[0].getAttribute('role'), 'alert');
  assert.match(boxes[0].textContent, /could not start/);
  assert.match(boxes[0].textContent, /again/);
});

test('the terminal is a v3 drop target and the runtime is imported in one place', () => {
  const view = readFileSync(new URL('../src/lib/components/terminal/TerminalView.svelte', import.meta.url), 'utf8');
  assert.match(view, /data-file-drop-target=\{tab\.status === 'running'/);
  assert.match(view, /:global\(\.file-drop-target-active\)/);
  assert.doesNotMatch(view, /--wails-drop-target/);
  const main = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8');
  assert.match(main, /if \(isDesktop\(\)\) \{\n\s+loadDesktopRuntime\(\)\.catch\(\(err: unknown\) => showStartupError\(err\)\)/);

  const src = fileURLToPath(new URL('../src', import.meta.url));
  const importers = [];
  const legacy = [];
  const walk = (dir) => {
    for (const name of readdirSync(dir)) {
      const path = join(dir, name);
      if (statSync(path).isDirectory()) { walk(path); continue; }
      if (!/\.(ts|svelte)$/.test(name)) continue;
      const text = readFileSync(path, 'utf8');
      if (text.includes('@wailsio/runtime')) importers.push(path.slice(src.length + 1));
      if (/window as any\)\.(go|runtime)\b|window\.(go|runtime)\b/.test(text)) legacy.push(path);
    }
  };
  walk(src);
  assert.deepEqual(importers, ['lib/desktopRuntime.ts']);
  assert.deepEqual(legacy, [], 'no Wails v2 globals remain');
  const runtime = readFileSync(new URL('../src/lib/desktopRuntime.ts', import.meta.url), 'utf8');
  assert.doesNotMatch(runtime, /^import (?!type)[^\n]*@wailsio\/runtime/m, 'only a type import is static');
});
