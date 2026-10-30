import test from 'node:test';
import assert from 'node:assert/strict';
import { createAPI, GIT_HISTORY_UNSUPPORTED } from '../src/lib/api.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

async function withApp(App, fn) {
  const previous = globalThis.window;
  globalThis.window = { go: { app: { App } } };
  try { await fn(); } finally {
    if (previous === undefined) delete globalThis.window;
    else globalThis.window = previous;
  }
}

test('history forwards full target and pinned snapshot without host-path rewriting', async () => {
  const calls = [];
  const target = { dir: 'C:\\work\\parent', repo: 'child' };
  const list = { ...target, ref: 'refs/remotes/origin/main', query: 'literal.*', cursor: 'page', limit: 50 };
  const commit = { ...target, tip: 'a'.repeat(40), commit: 'b'.repeat(40) };
  const diff = { ...commit, path: 'deleted/file.txt' };
  const App = Object.fromEntries(['GitHistoryRepositories', 'GitHistoryBranches', 'GitHistoryList', 'GitHistoryCommit', 'ReadGitHistoryDiff'].map(name => [name, async value => { calls.push([name, value]); return { marker: name }; }]));
  await withApp(App, async () => {
    const api = createAPI();
    for (const [method, value, binding] of [['gitHistoryRepositories', target.dir, 'GitHistoryRepositories'], ['gitHistoryBranches', target, 'GitHistoryBranches'], ['gitHistoryList', list, 'GitHistoryList'], ['gitHistoryCommit', commit, 'GitHistoryCommit'], ['readGitHistoryDiff', diff, 'ReadGitHistoryDiff']]) {
      assert.deepEqual(await api[method](value), { marker: binding });
      assert.deepEqual(calls.at(-1), [binding, value]);
    }
  });
});

test('old bindings report unsupported history independently from changes', async () => {
  await withApp({}, async () => {
    const api = createAPI();
    for (const method of ['gitHistoryRepositories', 'gitHistoryBranches', 'gitHistoryList', 'gitHistoryCommit', 'readGitHistoryDiff', 'gitHistoryRefAction', 'gitHistoryCompare', 'readGitHistoryCompareDiff']) {
      await assert.rejects(api[method]({}), e => e.message === GIT_HISTORY_UNSUPPORTED);
    }
  });
});

test('ref actions and comparison preserve captured host and cancellation waits for completion', async () => {
  const calls = [];
  let finish;
  const pending = new Promise(resolve => { finish = resolve; });
  const req = { dir: '/workspace', repo: 'child', ref: 'refs/tags/v1', expectedOID: 'a'.repeat(40), action: 'push' };
  await withApp({
    GitHistoryRefAction: (...args) => { calls.push(args); return pending; },
    CancelGitSync: async id => { calls.push(['cancel', id]); },
    GitHistoryCompare: async (...args) => { calls.push(args); return {}; },
    ReadGitHistoryCompareDiff: async (...args) => { calls.push(args); return {}; },
  }, async () => {
    const api = createAPI(), controller = new AbortController();
    let settled = false;
    const action = api.gitHistoryRefAction(req, controller.signal, 'ssh:original').then(() => { settled = true; });
    const id = calls[0][1];
    assert.deepEqual(calls[0], [req, id, 'ssh:original']);
    controller.abort();
    await Promise.resolve();
    assert.deepEqual(calls[1], ['cancel', id]);
    assert.equal(settled, false);
    finish({ action: 'push', ref: req.ref, summary: 'done' });
    await action;
    await api.gitHistoryCompare(req, 'ssh:original');
    assert.deepEqual(calls.at(-1), [req, 'ssh:original']);
    await api.readGitHistoryCompareDiff({ ...req, token: 'snapshot', path: 'file' }, 'ssh:original');
    assert.equal(calls.at(-1)[1], 'ssh:original');
    const before = calls.length;
    await assert.rejects(api.gitHistoryRefAction(req, controller.signal, 'ssh:original'), /canceled/);
    assert.equal(calls.length, before);
  });
});

test('mock history provides immutable review content and local/remote refs', async () => {
  const api = new MockBackendAPI();
  const target = { dir: '/project', repo: '.' };
  assert.equal((await api.gitHistoryRepositories(target.dir)).repositories.length, 1);
  assert.equal((await api.gitHistoryBranches(target)).refs.length, 2);
  const page = await api.gitHistoryList({ ...target, ref: 'HEAD', query: '', cursor: '', limit: 50 });
  const req = { ...target, tip: page.tip, commit: page.commits[0].oid };
  const detail = await api.gitHistoryCommit(req);
  const diff = await api.readGitHistoryDiff({ ...req, path: detail.changes[0].path });
  assert.equal(diff.content.edit.eligible, false);
  assert.equal(diff.content.revision, undefined);
  assert.equal(diff.commit, req.commit);
});
