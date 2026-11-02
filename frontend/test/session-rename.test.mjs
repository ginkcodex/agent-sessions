import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule, fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';
import { createAPI } from '../src/lib/api.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const { SessionRename } = await loadServerModule(new URL('../src/lib/stores/sessionRename.svelte.ts', import.meta.url), [
  ["import { api } from '../api';", fixtureStore('api', '__renameFixture')],
  ["import { epoch } from '../link';", 'const epoch = () => globalThis.__renameFixture.epoch;'],
  ["import { appState } from './appState.svelte';", fixtureStore('appState', '__renameFixture')],
  ["import { link } from './link.svelte';", fixtureStore('link', '__renameFixture')],
]);
const session = { ref: { agent: 'codex', id: 'test-session' }, title: 'Original' };
function fixture() {
  const calls = [];
  return globalThis.__renameFixture = { epoch: 0, calls,
    api: { renameSession: async (ref, title) => { calls.push([ref, title]); } },
    appState: { refresh: async () => { calls.push('refresh'); } },
    link: { stale: false, locked: false },
  };
}

test('native rename saves a trimmed name then refreshes; no optimistic title alias', async () => {
  const f = fixture(); const store = new SessionRename();
  store.open(session); store.title = '  New name ✨  ';
  await store.save();
  assert.deepEqual(f.calls, [[session.ref, 'New name ✨'], 'refresh']);
  assert.equal(store.target, null);
  assert.equal(store.busy, false);
  assert.equal(session.title, 'Original');
});

test('rename errors preserve the draft and keep the dialog open', async () => {
  const f = fixture(); const store = new SessionRename();
  f.api.renameSession = async () => { throw new Error('Native rename not supported'); };
  store.open(session); store.title = 'Draft'; await store.save();
  assert.equal(store.error, 'Native rename not supported');
  assert.equal(store.title, 'Draft'); assert.ok(store.target); assert.equal(store.busy, false);
  assert.deepEqual(f.calls, []);
});

test('blank, multiline, stale and cross-host rename attempts do not write', async () => {
  const f = fixture(); const store = new SessionRename();
  store.open(session);
  for (const title of ['', '   ', 'hello\nworld', 'hello\u0000world']) { store.title = title; await store.save(); }
  store.title = 'New'; f.link.stale = true; await store.save();
  f.link.stale = false; f.epoch++; await store.save();
  assert.deepEqual(f.calls, []);
});

test('duplicate submits make only one native mutation', async () => {
  const f = fixture(); const store = new SessionRename(); let finish;
  f.api.renameSession = (ref, title) => { f.calls.push([ref, title]); return new Promise(resolve => { finish = resolve; }); };
  store.open(session); store.title = 'New'; const pending = store.save(); await store.save();
  assert.deepEqual(f.calls, [[session.ref, 'New']]);
  finish(); await pending;
  assert.equal(store.busy, false);
});

test('host reconciliation dismisses stale dialogs before another submission', () => {
  const f = fixture(); const store = new SessionRename();
  store.open(session); f.epoch++; store.reconcileHost(); assert.equal(store.target, null);
  store.open(session); f.link.locked = true; store.reconcileHost(); assert.equal(store.target, null);
});

test('late mutation replies cannot close or refresh a different host dialog', async () => {
  const f = fixture(); const store = new SessionRename(); let finish;
  f.api.renameSession = () => new Promise(resolve => { finish = resolve; });
  store.open(session); store.title = 'New'; const save = store.save();
  f.epoch++; store.dismiss(); store.open({ ...session, title: 'Other host' });
  finish(); await save;
  assert.equal(store.target.title, 'Other host'); assert.equal(store.busy, false);
  assert.deepEqual(f.calls, []);
});

test('native API forwards rename and normalizes failures; old bindings fail explicitly', async () => {
  const previous = globalThis.window; const calls = [];
  const App = { RenameSession: async (ref, title) => { calls.push([ref, title]); } };
  globalThis.window = { go: { app: { App } } };
  try {
    const api = createAPI(); await api.renameSession(session.ref, 'New');
    assert.deepEqual(calls, [[session.ref, 'New']]);
    App.RenameSession = async () => { throw 'native failure'; };
    await assert.rejects(api.renameSession(session.ref, 'New'), /native failure/);
    delete App.RenameSession;
    await assert.rejects(api.renameSession(session.ref, 'New'), /does not support native session renaming/);
  } finally { if (previous === undefined) delete globalThis.window; else globalThis.window = previous; }
  await assert.rejects(new MockBackendAPI().renameSession(session.ref, 'New'), /unavailable in the browser mock/);
});

const dialog = await loadServerComponent(new URL('../src/lib/components/common/RenameSessionDialog.svelte', import.meta.url), [
  ["import { tick } from 'svelte';", 'const tick = async () => {};'],
  ["import { modalFocus } from '../../modalFocus';", 'const modalFocus = () => {};'],
  ["import { link } from '../../stores/link.svelte';", fixtureStore('link')],
  ["import { sessionRename } from '../../stores/sessionRename.svelte';", fixtureStore('sessionRename')],
  ["import DialogCloseButton from './DialogCloseButton.svelte';", 'const DialogCloseButton = () => {};'],
]);
test('rename dialog explains native persistence and shows mutation errors', async () => {
  const html = await renderWithFixture(dialog, {
    link: { stale: false, locked: false },
    sessionRename: { target: session, title: 'New', busy: false, valid: true, error: 'Session is active' },
  });
  assert.match(html, /not just in Agent Sessions/);
  assert.match(html, /role="dialog"/);
  assert.match(html, /Session name/);
  assert.match(html, /role="alert">Session is active/);
});

test('session menu renders outside the virtual list so transformed rows cannot clip it', async () => {
  const { readFileSync } = await import('node:fs');
  const source = name => readFileSync(new URL(`../src/lib/components/sessionlist/${name}`, import.meta.url), 'utf8');
  const row = source('SessionRow.svelte'), list = source('SessionList.svelte'), virtual = source('VirtualList.svelte');
  assert.match(virtual, /transform: translateY/, 'precondition: virtual items are transformed containing blocks');
  assert.doesNotMatch(row, /<ContextMenu/, 'rows must not render a fixed-position menu inside a virtual item');
  assert.match(row, /oncontextmenu=\{openMenu\}/);
  assert.match(list, /onMenu=\{openSessionMenu\}/);
  const closeList = list.indexOf('</VirtualList>');
  assert.ok(closeList > 0 && list.indexOf('<ContextMenu') > closeList, 'menu must be outside VirtualList');
  assert.match(list, /Rename session…/);
  // ContextMenu clears sessionMenu before running the action. An {@const} is a
  // lazy derived of sessionMenu, so the action would throw on null and do nothing.
  assert.doesNotMatch(list, /\{@const[^}]*sessionMenu/);
  assert.match(list, /items=\{sessionMenuItems\(sessionMenu\.session\)\}/);
  assert.match(list, /onSelect: \(\) => sessionRename\.open\(session\)/);
});
