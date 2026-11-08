import test from 'node:test';
import assert from 'node:assert/strict';
import { NavHistory, navigationButton, navigationShortcut } from '../src/lib/navHistory.ts';
import { loadServerModule } from './helpers.mjs';

const navAt = (id, groupKey = null) => ({ ref: id ? { agent: 'claude', id } : null, groupKey, groupMode: 'dir-agent' });

const navId = (entry) => entry?.ref?.id ?? null;

test('NavHistory goes back and forward and drops the places ahead on a new move', () => {
  const h = new NavHistory();
  assert.equal(h.canGoBack, false);
  h.replace(navAt('a')); // the session selected on load
  h.push(navAt('b'));
  h.push(navAt('b')); // the same place again is not a new entry
  h.push(navAt('c'));
  assert.equal(navId(h.back()), 'b');
  assert.equal(navId(h.back()), 'a');
  assert.equal(h.back(), null);
  assert.equal(navId(h.forward()), 'b');
  h.push(navAt('d'));
  assert.equal(h.canGoForward, false);
  assert.equal(navId(h.back()), 'b');
  assert.equal(navId(h.back()), 'a');

  const groups = new NavHistory();
  groups.push(navAt('a'));
  groups.push(navAt('a', 'g1')); // picking a group is a place of its own
  groups.replace(navAt('x', 'g1')); // ...and its first session updates it
  assert.deepEqual(groups.back(), navAt('a'));
  assert.deepEqual(groups.forward(), navAt('x', 'g1'));
});

test('NavHistory records a run of arrow-key steps as one place', () => {
  const h = new NavHistory();
  h.push(navAt('a'));
  h.step(navAt('b'));
  h.step(navAt('c'));
  h.step(navAt('d'));
  assert.equal(navId(h.back()), 'a');
  assert.equal(navId(h.forward()), 'd');
  h.push(navAt('e'));
  h.step(navAt('f'));
  assert.equal(navId(h.back()), 'e');
});

test('NavHistory caps its length', () => {
  const h = new NavHistory(3);
  for (const id of ['a', 'b', 'c', 'd']) h.push(navAt(id));
  assert.equal(navId(h.back()), 'c');
  assert.equal(navId(h.back()), 'b');
  assert.equal(h.back(), null);
});

test('NavHistory forgets deleted sessions and their subagents', () => {
  const h = new NavHistory();
  for (const id of ['a', 'b', 'b/agent-1', 'a', 'c']) h.push(navAt(id));
  h.back(); // at the second "a"
  h.remove([{ agent: 'claude', id: 'b' }]);
  // b and its subagent are gone and the two a's merge into one place.
  assert.equal(h.canGoBack, false);
  assert.equal(navId(h.forward()), 'c');
  h.remove([{ agent: 'codex', id: 'a' }]); // another agent's session
  assert.equal(navId(h.back()), 'a');

  // Deleting the session shown: the neighbour selected next takes its place
  // between the remaining ones.
  const shown = new NavHistory();
  for (const id of ['a', 'b', 'c']) shown.push(navAt(id));
  shown.back(); // at b
  shown.remove([{ agent: 'claude', id: 'b' }]);
  assert.equal(shown.canGoBack, true);
  shown.replace(navAt('n'));
  assert.equal(navId(shown.back()), 'a');
  assert.equal(navId(shown.forward()), 'n');
  assert.equal(navId(shown.forward()), 'c');

  // Deleting every session leaves nothing to go to.
  const all = new NavHistory();
  all.push(navAt('a'));
  all.push(navAt('b'));
  all.remove([{ agent: 'claude', id: 'a' }, { agent: 'claude', id: 'b' }]);
  assert.equal(all.canGoBack || all.canGoForward, false);
  assert.equal(all.back(), null);
  all.replace(navAt(null));
  assert.equal(all.canGoBack || all.canGoForward, false);

  // Deleting the shown first place, then going forward without a new one.
  const first = new NavHistory();
  first.push(navAt('a'));
  first.push(navAt('b'));
  first.back();
  first.remove([{ agent: 'claude', id: 'a' }]);
  assert.equal(first.canGoBack, false);
  assert.equal(navId(first.forward()), 'b');
});

test('the drop guard keeps drags from other apps from navigating the app', async () => {
  const { setupDropGuard } = await import('../src/lib/navigation.ts');
  const target = new EventTarget();
  setupDropGuard(target);
  const drag = (type) => {
    const e = new Event(type, { cancelable: true });
    e.dataTransfer = { dropEffect: 'none' };
    target.dispatchEvent(e);
    return e;
  };
  const over = drag('dragover');
  assert.deepEqual([over.defaultPrevented, over.dataTransfer.dropEffect], [true, 'copy']);
  assert.equal(drag('drop').defaultPrevented, true);

  // Dragging selected text inside the app keeps its default.
  target.dispatchEvent(new Event('dragstart'));
  assert.equal(drag('dragover').defaultPrevented, false);
  assert.equal(drag('drop').defaultPrevented, false);
  target.dispatchEvent(new Event('dragend'));
  assert.equal(drag('drop').defaultPrevented, true);
});

test('navigation shortcuts and mouse side buttons', () => {
  const key = (key, mods = {}) => ({ key, altKey: false, ctrlKey: false, metaKey: false, shiftKey: false, ...mods });
  assert.equal(navigationShortcut(key('BrowserBack'), false), 'back');
  assert.equal(navigationShortcut(key('BrowserForward'), true), 'forward');
  assert.equal(navigationShortcut(key('ArrowLeft', { altKey: true }), false), 'back');
  assert.equal(navigationShortcut(key('ArrowRight', { altKey: true }), false), 'forward');
  assert.equal(navigationShortcut(key('ArrowLeft', { altKey: true, shiftKey: true }), false), null);
  assert.equal(navigationShortcut(key('ArrowLeft', { altKey: true }), true), null);
  assert.equal(navigationShortcut(key('ArrowLeft'), false), null);
  assert.equal(navigationShortcut(key('[', { metaKey: true }), true), 'back');
  assert.equal(navigationShortcut(key(']', { metaKey: true }), true), 'forward');
  assert.equal(navigationShortcut(key('[', { ctrlKey: true }), false), null);
  assert.equal(navigationButton(3), 'back');
  assert.equal(navigationButton(4), 'forward');
  assert.equal(navigationButton(0), null);
  assert.equal(navigationButton(2), null);
});

const APP_STATE_URL = new URL('../src/lib/stores/appState.svelte.ts', import.meta.url);
const { AppState } = await loadServerModule(APP_STATE_URL, [
  ...['tree', 'catalog', 'manage', 'link', 'navHistory'].map(name => [
    `'../${name}'`, JSON.stringify(new URL(`../${name}.ts`, APP_STATE_URL).href),
  ]),
  ["import { api, subscribeCatalogChanged } from '../api';",
    `const api = new Proxy({}, { get: (_, key) => globalThis.__navigationAPI[key] });
     const subscribeCatalogChanged = () => () => {};`],
]);

async function reconnectFixture(run) {
  const rows = ['a', 'b', 'c'].map(id => ({ ref: navAt(id).ref, title: id }));
  const calls = [];
  const api = {
    listGroups: async () => { calls.push('groups'); return [{ key: 'group', children: [] }]; },
    agentCounts: async () => ({}),
    listSessions: async group => { calls.push(['sessions', group]); return rows; },
    getSessionMeta: async ref => {
      calls.push(['meta', ref.id]);
      const row = rows.find(row => row.ref.id === ref.id);
      if (!row) throw new Error('session not found');
      return { ...row };
    },
  };
  globalThis.__navigationAPI = api;
  const store = new AppState();
  try {
    await store.reset('box');
    await store.selectGroup('group');
    await store.selectSession(rows[1].ref, 'push');
    await store.selectSession(rows[2].ref, 'push');
    await store.goBack(); // b, with history on both sides
    calls.length = 0;
    await run({ store, rows, calls, api });
  } finally { delete globalThis.__navigationAPI; }
}

test('same-host reconnect keeps group, selected session and back/forward history and reloads transcript', async () => {
  await reconnectFixture(async ({ store, rows, calls }) => {
    const version = store.transcriptVersion;
    rows[1].title = 'reloaded header';
    await store.reset('box', true);
    assert.equal(store.selectedGroupKey, 'group');
    assert.equal(store.selectedSessionRef.id, 'b');
    assert.equal(store.selectedSessionMeta.title, 'reloaded header');
    assert.equal(store.transcriptVersion, version + 1, 'mounted transcript reload signal');
    assert.deepEqual(calls.slice(0, 2), ['groups', ['sessions', 'group']]);
    assert.equal(store.canGoBack, true);
    assert.equal(store.canGoForward, true);
    await store.goBack();
    assert.equal(store.selectedSessionRef.id, 'a');
    await store.goForward();
    assert.equal(store.selectedSessionRef.id, 'b');
    await store.goForward();
    assert.equal(store.selectedSessionRef.id, 'c');
  });
});

test('same-host reconnect falls back to the first session if the selected session was deleted', async () => {
  await reconnectFixture(async ({ store, rows }) => {
    rows.splice(1, 1);
    await store.reset('box', true);
    assert.equal(store.selectedGroupKey, null);
    assert.equal(store.selectedSessionRef.id, 'a');
    assert.equal(store.canGoBack, false);
    assert.equal(store.canGoForward, false);
  });
});

for (const host of ['other', undefined]) {
  test(`host reset to ${host ?? 'Local'} clears navigation`, async () => {
    await reconnectFixture(async ({ store }) => {
      await store.reset(host);
      assert.equal(store.targetHost, host);
      assert.equal(store.selectedGroupKey, null);
      assert.equal(store.selectedSessionRef.id, 'a');
      assert.equal(store.canGoBack || store.canGoForward, false);
    });
  });
}

test('late pre-reconnect list, tree and metadata replies cannot overwrite the reload', async () => {
  await reconnectFixture(async ({ store, api, rows }) => {
    const pending = {};
    const original = { ...api };
    for (const method of ['listGroups', 'listSessions', 'getSessionMeta']) {
      api[method] = () => new Promise(resolve => { pending[method] = resolve; });
    }
    const oldTree = store.loadGroups();
    const oldList = store.loadSessions();
    const oldMeta = store.selectSession(rows[1].ref);
    Object.assign(api, original);
    rows[1].title = 'fresh';
    await store.reset('box', true);
    pending.listGroups([{ key: 'stale', children: [] }]);
    pending.listSessions([{ ref: navAt('stale').ref }]);
    pending.getSessionMeta({ ref: rows[1].ref, title: 'stale' });
    await Promise.all([oldTree, oldList, oldMeta]);
    assert.equal(store.groups[0].key, 'group');
    assert.deepEqual(store.sessions, rows);
    assert.equal(store.selectedSessionMeta.title, 'fresh');
    assert.equal(store.error, null);
  });
});
