import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule, fixtureStore } from './helpers.mjs';

const url = new URL('../src/lib/stores/workspaces.svelte.ts', import.meta.url);
const { WorkspacesStore } = await loadServerModule(url, [
  ...['api', 'link', 'appState'].map(name => [
    name === 'api' ? "import { api } from '../api';" : `import { ${name} } from './${name}.svelte';`,
    fixtureStore(name, '__workspaceFixture'),
  ]),
  ["import { epoch } from '../link';", 'const epoch = () => globalThis.__workspaceFixture.epoch;'],
  ["'../tree'", JSON.stringify(new URL('../src/lib/tree.ts', import.meta.url).href)],
]);

function fixture() {
  const calls = [];
  return {
    epoch: 0, calls, link: { stale: false, locked: false },
    api: {
      listWorkspaces: async () => ['/new'],
      openWorkspace: async path => { calls.push(['open', path]); return '/new'; },
      removeWorkspace: async path => calls.push(['remove', path]),
    },
    appState: {
      groups: [{ key: 'dir:/new', kind: 'directory', cwd: '/new' }],
      setGroupMode: async mode => calls.push(['mode', mode]),
      selectGroup: async (...args) => calls.push(['group', ...args]),
      selectSession: async ref => calls.push(['session', ref]),
      reload: async () => calls.push(['reload']),
    },
  };
}

test('opening an explicit workspace registers it, selects Dir mode and clears the old transcript', async () => {
  const f = globalThis.__workspaceFixture = fixture();
  try {
    const store = new WorkspacesStore();
    store.show();
    assert.equal(await store.add(' /new '), true);
    assert.deepEqual(f.calls, [['open', '/new'], ['mode', 'dir-agent'], ['group', 'dir:/new', 'push'], ['session', null]]);
    assert.deepEqual(store.directories, ['/new']);
    assert.equal(store.open, false);
  } finally { delete globalThis.__workspaceFixture; }
});

test('failed or stale registration does not navigate into another host', async () => {
  const f = globalThis.__workspaceFixture = fixture();
  try {
    const store = new WorkspacesStore();
    store.show();
    f.api.openWorkspace = async () => { throw new Error('Not a directory'); };
    assert.equal(await store.add('/file'), false);
    assert.equal(store.error, 'Not a directory');
    assert.equal(store.open, true);
    f.api.openWorkspace = async () => { f.epoch++; return '/new'; };
    assert.equal(await store.add('/new'), false);
    assert.deepEqual(f.calls, []);
  } finally { delete globalThis.__workspaceFixture; }
});

test('remove only unregisters and reloads, leaving files and sessions untouched', async () => {
  const f = globalThis.__workspaceFixture = fixture();
  try {
    const store = new WorkspacesStore();
    await store.remove('/new');
    assert.deepEqual(f.calls, [['remove', '/new'], ['reload']]);
  } finally { delete globalThis.__workspaceFixture; }
});
