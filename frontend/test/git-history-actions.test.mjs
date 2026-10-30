import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fixtureStore, loadServerModule } from './helpers.mjs';

const fixtureName = '__historyActionsFixture';
const { GitHistoryActionsStore } = await loadServerModule(new URL('../src/lib/stores/gitHistoryActions.svelte.ts', import.meta.url), [
  ["import { api } from '../api';", fixtureStore('api', fixtureName)],
  ["'../manage'", JSON.stringify(new URL('../src/lib/manage.ts', import.meta.url).href)],
  ...['files', 'link', 'connectionStore', 'filePrompt', 'gitHistory', 'gitChanges', 'codeDiagnostics'].map(name => [
    `import { ${name} } from './${name === 'connectionStore' ? 'connection' : name}.svelte';`, name === 'gitChanges'
      ? `const gitChanges = new Proxy({}, { get: (_, key) => globalThis.${fixtureName}.gitChanges[key], set: (_, key, value) => { globalThis.${fixtureName}.gitChanges[key] = value; return true; } });`
      : fixtureStore(name, fixtureName),
  ]),
]);
const ref = 'refs/heads/topic';
function fixture() {
  const calls = [];
  const f = {
    files: { dir: '/workspace/sub', contextVersion: '1', tabs: [{ path: 'clean' }],
      invalidateListings(root, host) { calls.push(['invalidate', root, host]); return true; },
      async refresh() { calls.push('files'); }, async reloadReplaced(paths) { calls.push(['reload', paths]); } },
    link: { dataHost: 'host-a' },
    connectionStore: { capabilities: { gitHistoryActions: true, gitHistoryCompare: true, gitSync: true, gitWrite: true, saveFile: true } },
    gitHistory: { contextKey: 'context', selectedRepository: '.', repository: { scope: 'sub' }, revision: 1, headRef: 'refs/heads/main', head: 'head-oid',
      refs: [{ ref, name: 'topic', oid: 'ref-oid' }], blockedReason: null, stale: false,
      invalidateHost(host) { calls.push(['history', host]); ++this.revision; } },
    gitChanges: { busy: false, draftPaths: [], async refresh() { calls.push('status'); }, async loadBranches() { calls.push('branches'); } },
    codeDiagnostics: { async hide() { calls.push('hide'); }, async show(dir) { calls.push(['show', dir]); } },
    filePrompt: { async ask(...args) { calls.push(['confirm', ...args]); return 'checkout'; } },
    api: { async gitHistoryRefAction(req, signal, host) { calls.push(['write', req, signal, host]); return { summary: 'Done' }; } },
  };
  globalThis[fixtureName] = f;
  return { f, calls, store: new GitHistoryActionsStore() };
}

test('ref action pins original workspace/repository, HEAD and OID, acknowledges wider scope and shares lock', async () => {
  const { f, calls, store } = fixture();
  f.api.gitHistoryRefAction = async (req, signal, host) => {
    assert.equal(f.gitChanges.busy, true);
    calls.push(['write', req, signal, host]); return { summary: 'Done' };
  };
  assert.equal(await store.run('checkout', ref), true);
  const [, req, signal, host] = calls.find(c => c[0] === 'write');
  assert.deepEqual(req, { dir: '/workspace/sub', repo: '.', action: 'checkout', ref, expectedOID: 'ref-oid', expectedHead: { symbolicRef: 'refs/heads/main', oid: 'head-oid' }, draftPaths: [], acknowledgeWholeRepository: true });
  assert.equal(host, 'host-a'); assert.ok(signal instanceof AbortSignal);
  assert.match(calls[0][2], /WHOLE repository/);
  assert.equal(f.gitChanges.busy, false);
  assert.ok(calls.find(c => c[0] === 'reload'));
});

test('confirmation never authorizes changed host, workspace, repository, HEAD, ref, capability, draft or busy state', async () => {
  for (const mutate of [
    f => f.link.dataHost = 'host-b', f => f.files.dir = '/other', f => f.gitHistory.contextKey = 'new',
    f => f.gitHistory.selectedRepository = 'nested', f => f.gitHistory.head = 'new-head',
    f => f.gitHistory.headRef = 'refs/heads/new', f => f.gitHistory.refs[0].oid = 'new-ref',
    f => f.connectionStore.capabilities.saveFile = false, f => f.connectionStore.capabilities.gitHistoryActions = false,
    f => f.gitChanges.draftPaths = ['draft'], f => f.gitChanges.busy = true,
  ]) {
    const { f, calls, store } = fixture();
    f.filePrompt.ask = async () => { mutate(f); return 'checkout'; };
    assert.equal(await store.run('checkout', ref), false);
    assert.equal(calls.some(c => c[0] === 'write'), false);
  }
});

test('drafts block checkout/merge before confirmation, not ref-only operations; capability and ref restrictions explain refusal', () => {
  const { f, store } = fixture();
  f.gitChanges.draftPaths = ['draft'];
  assert.match(store.blocker('checkout', ref), /drafts/); assert.match(store.blocker('merge', ref), /drafts/);
  assert.equal(store.blocker('push', ref), null); assert.equal(store.blocker('delete', ref), null);
  f.gitHistory.refs[0].remote = true;
  assert.match(store.blocker('push', ref), /Remote-tracking/);
  assert.match(store.blocker('delete', 'HEAD'), /exact branch/);
  f.connectionStore.capabilities.gitHistoryCompare = false;
  assert.match(store.blocker('compare', ref), /comparisons/);
});

test('failed mutation still invalidates history and listings and reloads via draft-safe file path', async () => {
  const { f, calls, store } = fixture();
  f.api.gitHistoryRefAction = async () => { f.gitChanges.draftPaths = ['typed-during-operation']; throw new Error('merge conflict'); };
  assert.equal(await store.run('checkout', ref), false);
  assert.match(store.error, /merge conflict/);
  assert.deepEqual(calls.slice(1), [['history', 'host-a'], ['invalidate', null, 'host-a'], ['reload', ['clean']], 'files', 'status', 'branches', 'hide', ['show', '/workspace/sub']]);
  assert.deepEqual(f.gitChanges.draftPaths, ['typed-during-operation']);
  assert.equal(store.busy, false);
});

test('late failure from a previous context invalidates original host without reloading new-context tabs or errors', async () => {
  const { f, calls, store } = fixture();
  let fail;
  f.api.gitHistoryRefAction = () => new Promise((_, reject) => fail = reject);
  const pending = store.run('checkout', ref);
  await Promise.resolve();
  f.link.dataHost = 'host-b'; f.gitHistory.contextKey = 'other';
  fail(new Error('old failure'));
  assert.equal(await pending, false);
  assert.equal(store.error, null);
  assert.deepEqual(calls.slice(1), [['history', 'host-a'], ['invalidate', null, 'host-a'], 'files']);
});

test('shared mutation lock excludes another ref write and cancellation still cleans up', async () => {
  const { f, calls, store } = fixture();
  f.api.gitHistoryRefAction = (_req, signal) => new Promise((_, reject) => {
    signal.addEventListener('abort', () => reject(new Error('canceled')));
  });
  const pending = store.run('checkout', ref);
  await Promise.resolve();
  assert.equal(f.gitChanges.busy, true);
  assert.equal(await store.run('delete', ref), false);
  store.cancel();
  assert.equal(await pending, false);
  assert.equal(f.gitChanges.busy, false);
  assert.ok(calls.some(c => c[0] === 'history'));
});

test('context change during cleanup never refreshes the replacement workspace status or diagnostics', async () => {
  const { f, calls, store } = fixture();
  f.files.reloadReplaced = async () => { f.gitHistory.contextKey = 'replacement'; };
  assert.equal(await store.run('checkout', ref), true);
  assert.equal(calls.includes('status'), false);
  assert.equal(calls.includes('hide'), false);
  assert.equal(calls.includes('files'), false);
});

test('menu preserves exact action ordering, keyboard access, tags and explicit mutation hint', async () => {
  const source = await readFile(new URL('../src/lib/components/common/go-to-file/HistoryBranches.svelte', import.meta.url), 'utf8');
  const labels = ["['checkout', 'Checkout']", "['compare', 'Show Diff with Working Tree']", "['merge',", "['push', 'Push to origin']", "['delete', 'Delete']"];
  let previous = -1;
  for (const label of labels) { const index = source.indexOf(label); assert.ok(index > previous); previous = index; }
  assert.match(source, /event.key === 'ContextMenu'/); assert.match(source, /event.shiftKey && event.key === 'F10'/);
  assert.match(source, /Actions for selected history ref/); assert.match(source, /Explicit context-menu actions may change/);
  assert.match(source, /<span class="label">Tags/);
});
