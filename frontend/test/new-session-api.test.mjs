import test from 'node:test';
import assert from 'node:assert/strict';
import { desktopAPI, useDesktopApp } from './desktop-app.mjs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

test('new session API passes the agent and directory to native bindings', async () => {
  const calls = [];
  const tab = { id: 't1', label: 'Codex', host: '', kind: 'agent', dir: '/p', agent: 'codex', startedAt: 1 };
  const App = {
    PanelNewSession: async (...args) => { calls.push(['PanelNewSession', ...args]); return tab; },
    OpenNewSessionInTerminal: async (...args) => { calls.push(['OpenNewSessionInTerminal', ...args]); },
  };
  useDesktopApp(App);
  const backend = desktopAPI();
  assert.deepEqual(await backend.panelNewSession('codex', '/p', { cols: 80, rows: 24 }), tab);
  await backend.openNewSessionInTerminal('opencode', '/p');
  assert.deepEqual(calls, [
    ['PanelNewSession', 'codex', '/p', { cols: 80, rows: 24 }],
    ['OpenNewSessionInTerminal', 'opencode', '/p'],
  ]);
  App.PanelNewSession = async () => { throw 'unknown directory: /gone'; };
  await assert.rejects(backend.panelNewSession('codex', '/gone', { cols: 80, rows: 24 }), /unknown directory/);
});

test('workspace API forwards native arguments and normalizes errors', async () => {
  const calls = [];
  const listing = { path: '/p', parent: '/', directories: ['/p/src'] };
  const App = {
    ListWorkspaces: async () => ['/p'],
    OpenWorkspace: async dir => { calls.push(['open', dir]); return '/p'; },
    RemoveWorkspace: async dir => { calls.push(['remove', dir]); },
    BrowseWorkspace: async path => { calls.push(['browse', path]); return listing; },
  };
  useDesktopApp(App);
  const backend = desktopAPI();
  assert.deepEqual(await backend.listWorkspaces(), ['/p']);
  assert.equal(await backend.openWorkspace('/p/'), '/p');
  await backend.removeWorkspace('/p');
  assert.deepEqual(await backend.browseWorkspace(''), listing);
  assert.deepEqual(calls, [['open', '/p/'], ['remove', '/p'], ['browse', '']]);
  for (const [native, method, args] of [
    ['ListWorkspaces', 'listWorkspaces', []],
    ['OpenWorkspace', 'openWorkspace', ['/p']],
    ['RemoveWorkspace', 'removeWorkspace', ['/p']],
    ['BrowseWorkspace', 'browseWorkspace', ['/p']],
  ]) {
    App[native] = async () => { throw 'workspace unavailable'; };
    await assert.rejects(backend[method](...args), error => error instanceof Error && error.message === 'workspace unavailable');
  }
});

test('mock workspaces are host-scoped, browseable, and authorize empty projects', async () => {
  const mock = new MockBackendAPI();
  assert.equal(await mock.openWorkspace('/home/haith/Workspaces/empty/../empty/'), '/home/haith/Workspaces/empty');
  await mock.openWorkspace('/home/haith/Workspaces/empty');
  assert.deepEqual(await mock.listWorkspaces(), ['/home/haith/Workspaces/empty']);
  const group = (await mock.listGroups('dir-agent')).find(g => g.cwd === '/home/haith/Workspaces/empty');
  assert.equal(group.sessionCount, 0);
  for (const key of [group.key, `${group.key}:codex`, 'agent-dir:codex:/home/haith/Workspaces/empty']) {
    assert.deepEqual(await mock.listSessions(key), []);
  }
  assert.ok(!(await mock.listGroups('agent-dir')).some(g => g.cwd === group.cwd));
  const tab = await mock.panelNewSession('codex', group.cwd, { cols: 80, rows: 24 });
  assert.equal(tab.dir, group.cwd);
  await mock.terminalClose(tab.id);
  mock.launch.terminal = true;
  await mock.openNewSessionInTerminal('opencode', group.cwd);
  assert.match(mock.openedTerminals.at(-1), /opencode$/);
  await assert.rejects(mock.openWorkspace('relative/path'), /absolute/);
  await assert.rejects(mock.browseWorkspace('relative/path'), /absolute/);
  const home = await mock.browseWorkspace('');
  assert.equal(home.path, '/home/haith');
  assert.equal(home.parent, '/home');
  assert.ok(home.directories.includes('/home/haith/Workspaces'));
  assert.ok((await mock.browseWorkspace('/home/haith/Workspaces')).directories.includes(group.cwd));
  assert.equal((await mock.browseWorkspace('/')).parent, '/');
  assert.equal((await mock.browseWorkspace('C:\\')).parent, 'C:/');
  await mock.connect('dev-box');
  assert.deepEqual(await mock.listWorkspaces(), []);
  await assert.rejects(mock.panelNewSession('codex', group.cwd, { cols: 80, rows: 24 }), /unknown directory/);
  await mock.openWorkspace('/srv/remote-project');
  await mock.disconnect();
  assert.deepEqual(await mock.listWorkspaces(), [group.cwd]);
  await mock.removeWorkspace(group.cwd);
  assert.deepEqual(await mock.listWorkspaces(), []);
  await assert.rejects(mock.panelNewSession('codex', group.cwd, { cols: 80, rows: 24 }), /unknown directory/);
});

test('the mock starts new sessions only in known projects, and names the tab', async () => {
  const mock = new MockBackendAPI();
  const [meta] = await mock.listSessions('', {}, { field: 'updatedAt', dir: 'desc' });
  const before = Date.now();
  const tab = await mock.panelNewSession('codex', meta.cwd, { cols: 80, rows: 24 });
  assert.deepEqual([tab.label, tab.kind, tab.agent, tab.dir, tab.host], ['Codex', 'agent', 'codex', meta.cwd, '']);
  assert.ok(tab.startedAt >= before);
  await assert.rejects(mock.panelNewSession('codex', '/nowhere', { cols: 80, rows: 24 }), /unknown directory/);
  await assert.rejects(mock.panelNewSession('gemini', meta.cwd, { cols: 80, rows: 24 }), /unknown provider/);

  mock.launch.terminal = true;
  await mock.openNewSessionInTerminal('claude-code', meta.cwd);
  assert.equal(mock.openedTerminals.at(-1), `cd '${meta.cwd}' && claude`);

  await mock.connect('dev-box');
  assert.equal((await mock.panelNewSession('opencode', meta.cwd, { cols: 80, rows: 24 })).host, 'dev-box');
  await assert.rejects(mock.openNewSessionInTerminal('opencode', meta.cwd), /local and WSL sessions only/);
});
