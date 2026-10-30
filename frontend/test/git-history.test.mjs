import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerModule } from './helpers.mjs';

const lib = name => JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href);
const { GitHistoryStore, HISTORY_RETAIN_LIMIT } = await loadServerModule(new URL('../src/lib/stores/gitHistory.svelte.ts', import.meta.url), [
  ["import { api, GIT_HISTORY_UNSUPPORTED } from '../api';", `${fixtureStore('api', '__historyFixture')} const GIT_HISTORY_UNSUPPORTED = 'Git history is not supported. Update the app or remote server.';`],
  ["'../manage'", lib('manage')],
  ...['files', 'link', 'connectionStore'].map(name => [
    `import { ${name} } from './${name === 'connectionStore' ? 'connection' : name}.svelte';`, fixtureStore(name, '__historyFixture'),
  ]),
]);
const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; };
const repo = (path = '.', scope = '') => ({ path, scope, name: path });
const person = { name: 'Author', email: 'a@test', time: 10 };
const commit = oid => ({ oid, subject: oid, author: person, parents: [] });
const page = (ids = ['a', 'b'], cursor = '', tip = 'pinned') => ({ commits: ids.map(commit), cursor, tip, shallow: false, limited: false });
const detail = oid => ({ ...commit(oid), committer: person, message: oid, base: '', changes: [], truncated: false });
const refs = () => ({ refs: [{ ref: 'refs/heads/main', name: 'main', oid: 'tip', remote: false, current: true }, { ref: 'refs/remotes/origin/main', name: 'origin/main', oid: 'tip2', remote: true, current: false }], head: 'tip', unborn: false, detached: false, shallow: false, truncated: false });
function fixture() {
  const calls = [];
  const f = {
    files: { dir: 'C:\\project', contextVersion: '1', invalidateComparisons() {} },
    link: { dataHost: undefined, host: undefined, phase: 'local', blockedReason: null },
    connectionStore: { capabilities: { gitHistory: true }, generation: 1 },
    api: {
      async gitHistoryRepositories(dir) { calls.push(['repositories', dir]); return { repositories: [repo()], truncated: false }; },
      async gitHistoryBranches(target) { calls.push(['refs', target]); return refs(); },
      async gitHistoryList(req) { calls.push(['page', req]); return page(); },
      async gitHistoryCommit(req) { calls.push(['detail', req]); return detail(req.commit); },
    },
  };
  globalThis.__historyFixture = f;
  return { f, calls, store: new GitHistoryStore() };
}

test('discovery defaults to HEAD, preserves full target/scope, and ensureLoaded does not storm', async () => {
  const { f, calls, store } = fixture();
  f.api.gitHistoryRepositories = async () => ({ repositories: [repo('.', 'scoped/path')], truncated: true });
  await store.ensureLoaded();
  assert.equal(store.selectedRepository, '.');
  assert.equal(store.repository.scope, 'scoped/path');
  assert.equal(store.repositoriesTruncated, true);
  assert.equal(store.selectedRef, 'HEAD');
  assert.deepEqual(calls[0], ['refs', { dir: 'C:\\project', repo: '.' }]);
  assert.deepEqual(calls[1][1], { dir: 'C:\\project', repo: '.', ref: 'HEAD', query: '', cursor: '', limit: 50 });
  await store.ensureLoaded();
  assert.equal(calls.length, 2);
  await store.selectCommit('a');
  assert.deepEqual(calls.at(-1), ['detail', { dir: 'C:\\project', repo: '.', tip: 'pinned', commit: 'a' }]);
  assert.equal(store.detail.message, 'a');
  assert.equal(store.loading, false);
});

test('multi-repo requires a picker, refresh retains only valid prior choices; host change forgets them', async () => {
  const { store, f, calls } = fixture();
  f.api.gitHistoryRepositories = async () => ({ repositories: [repo('one'), repo('two')], truncated: false });
  await store.refresh();
  assert.equal(store.requiresRepositorySelection, true);
  assert.equal(calls.length, 0);
  await store.selectRepository('invalid');
  assert.equal(calls.length, 0);
  await store.selectRepository('two');
  await store.selectRef('refs/remotes/origin/main');
  await store.refresh();
  assert.equal(store.selectedRepository, 'two');
  assert.equal(store.selectedRef, 'refs/remotes/origin/main');
  f.api.gitHistoryRepositories = async () => ({ repositories: [repo('one'), repo('three')], truncated: false });
  await store.refresh();
  assert.equal(store.selectedRepository, null);
  assert.equal(store.selectedRef, 'HEAD');
  await store.selectRepository('one');
  f.link.dataHost = f.link.host = 'new'; f.link.phase = 'connected';
  await store.ensureLoaded();
  assert.equal(store.selectedRepository, null, 'another host never inherits a choice');
  assert.deepEqual(store.commits, []);
  f.files.dir = 'C:\\other';
  await store.ensureLoaded();
  assert.equal(store.selectedRepository, null);
  f.files.dir = 'C:\\project'; f.link.dataHost = f.link.host = undefined; f.link.phase = 'local';
  await store.ensureLoaded();
  assert.equal(store.selectedRepository, 'one', 'a still-valid prior choice for this workspace/host is kept');
  assert.equal(store.tip, 'pinned');
});

test('discovery, refs and pages report loading independently', async () => {
  const { store, f } = fixture();
  const refsReply = deferred(), pageReply = deferred();
  f.api.gitHistoryBranches = () => refsReply.promise; f.api.gitHistoryList = () => pageReply.promise;
  const pending = store.refresh();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(store.repositoriesLoading, false); assert.equal(store.refsLoading, true);
  refsReply.resolve(refs());
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(store.refsLoading, false); assert.equal(store.pagesLoading, true); assert.equal(store.loading, true);
  pageReply.resolve(page()); await pending;
  assert.equal(store.loading, false);
});

test('discovery generations reject old replies and old finally cannot end new loading', async () => {
  const { store, f } = fixture();
  const old = deferred(), fresh = deferred(); let count = 0;
  f.api.gitHistoryRepositories = () => (++count === 1 ? old : fresh).promise;
  const a = store.refresh();
  const b = store.refresh();
  old.resolve({ repositories: [repo('old')], truncated: false }); await a;
  assert.deepEqual(store.repositories, []);
  assert.equal(store.repositoriesLoading, true);
  fresh.resolve({ repositories: [repo('new')], truncated: false }); await b;
  assert.equal(store.selectedRepository, 'new');
  assert.equal(store.repositoriesLoading, false);
});

test('repository ref requests and their errors/loading are independent of obsolete requests', async () => {
  const { store, f } = fixture();
  f.api.gitHistoryRepositories = async () => ({ repositories: [repo('one'), repo('two')], truncated: false });
  await store.refresh();
  const old = deferred(), fresh = deferred();
  f.api.gitHistoryBranches = target => (target.repo === 'one' ? old : fresh).promise;
  const a = store.selectRepository('one'), b = store.selectRepository('two');
  old.reject(new Error('obsolete refs')); await a;
  assert.equal(store.refsLoading, true);
  assert.equal(store.error, null);
  fresh.resolve(refs()); await b;
  assert.equal(store.selectedRepository, 'two');
  assert.equal(store.refsLoading, false);
  assert.equal(store.tip, 'pinned');
});

test('ref and query changes invalidate pages immediately, including UI-debounced query seam', async () => {
  const { store, f, calls } = fixture();
  await store.refresh(); await store.selectCommit('a');
  const old = deferred(); f.api.gitHistoryList = () => old.promise;
  const a = store.selectRef('refs/heads/main');
  assert.equal(store.detail, null); assert.equal(store.selectedCommit, null);
  await store.setQuery('literal.*', { defer: true });
  assert.equal(store.pagesLoading, false);
  assert.deepEqual(store.commits, []);
  old.resolve(page(['obsolete'])); await a;
  assert.deepEqual(store.commits, []);
  f.api.gitHistoryList = async req => { calls.push(['query', req]); return page(['match']); };
  await store.ensureLoaded();
  assert.equal(calls.at(-1)[1].query, 'literal.*');
  assert.equal(calls.at(-1)[1].ref, 'refs/heads/main');
  assert.deepEqual(store.commits.map(c => c.oid), ['match']);
});

test('out-of-order query pages cannot clear loading or replace newer rows', async () => {
  const { store, f } = fixture(); await store.refresh();
  const old = deferred(), fresh = deferred();
  f.api.gitHistoryList = req => (req.query === 'old' ? old : fresh).promise;
  const a = store.setQuery('old'), b = store.setQuery('new');
  old.reject(new Error('old page error')); await a;
  assert.equal(store.pagesLoading, true); assert.equal(store.pageError, null);
  fresh.resolve(page(['new'])); await b;
  assert.equal(store.pagesLoading, false);
  assert.deepEqual(store.commits.map(c => c.oid), ['new']);
});

test('details generations survive out-of-order success/failure/finally and page append', async () => {
  const { store, f } = fixture();
  f.api.gitHistoryList = async () => page(['a', 'b'], 'next');
  await store.refresh();
  const old = deferred(), fresh = deferred();
  f.api.gitHistoryCommit = req => (req.commit === 'a' ? old : fresh).promise;
  const a = store.selectCommit('a'), b = store.selectCommit('b');
  old.reject(new Error('wrong detail')); await a;
  assert.equal(store.detailLoading, true); assert.equal(store.detailError, null);
  f.api.gitHistoryList = async () => page(['c']);
  await store.loadMore();
  fresh.resolve(detail('b')); await b;
  assert.equal(store.detail.oid, 'b'); assert.equal(store.selectedCommit, 'b');
  assert.equal(store.detailLoading, false);
  const pending = deferred(); f.api.gitHistoryCommit = () => pending.promise;
  const c = store.selectCommit('c');
  await store.setQuery('different');
  pending.resolve(detail('c')); await c;
  assert.equal(store.detail, null); assert.equal(store.selectedCommit, null);
});

test('pagination uses opaque pinned continuation, deduplicates and refuses changed snapshots', async () => {
  const { store, f, calls } = fixture();
  f.api.gitHistoryList = async req => { calls.push(['page', req]); return req.cursor ? page(['b', 'c'], 'next2') : page(['a', 'b'], 'next'); };
  await store.refresh();
  await store.selectCommit('a');
  await store.loadMore();
  assert.equal(calls.at(-1)[1].cursor, 'next');
  assert.equal(calls.at(-1)[1].limit, 50);
  assert.deepEqual(store.commits.map(c => c.oid), ['a', 'b', 'c']);
  assert.equal(store.detail.oid, 'a');
  f.api.gitHistoryList = async () => page(['other'], '', 'moved-tip');
  await store.loadMore();
  assert.match(store.pageError, /snapshot changed/);
  assert.equal(store.tip, 'pinned');
  assert.deepEqual(store.commits.map(c => c.oid), ['a', 'b', 'c']);
  f.api.gitHistoryList = async () => page(['fresh'], '', 'moved-tip');
  await store.refresh();
  assert.equal(store.tip, 'moved-tip'); assert.equal(store.detail, null);
  assert.equal(store.selectedCommit, null);
});

test('loadMore serializes requests; refresh retires pending append and selection', async () => {
  const { store, f } = fixture();
  f.api.gitHistoryList = async () => page(['a'], 'next'); await store.refresh();
  const old = deferred(); let n = 0;
  f.api.gitHistoryList = () => { n++; return old.promise; };
  const a = store.loadMore(); await store.loadMore(); assert.equal(n, 1);
  f.api.gitHistoryList = async () => page(['fresh']);
  await store.refresh();
  old.resolve(page(['stale'])); await a;
  assert.deepEqual(store.commits.map(c => c.oid), ['fresh']);
  assert.equal(store.pagesLoading, false);
});

test('retained rows cap at 2000, report incomplete traversal and UTF-8 query bound honestly', async () => {
  const { store, f } = fixture(); let n = 0;
  f.api.gitHistoryList = async req => {
    n++; const offset = req.cursor ? Number(req.cursor) : 0;
    return { ...page(Array.from({ length: 50 }, (_, i) => String(i + offset)), String(offset + 50)), limited: true, shallow: true };
  };
  await store.refresh();
  for (let i = 1; i < HISTORY_RETAIN_LIMIT / 50; i++) await store.loadMore();
  assert.equal(store.commits.length, HISTORY_RETAIN_LIMIT);
  assert.equal(store.retainedLimit, true); assert.equal(store.canLoadMore, false);
  assert.equal(store.limited, true); assert.equal(store.shallow, true);
  await store.loadMore(); assert.equal(n, HISTORY_RETAIN_LIMIT / 50);
  await store.setQuery('é'.repeat(129));
  assert.match(store.pageError, /256 UTF-8 bytes/); assert.equal(store.loading, false);
  assert.equal(n, HISTORY_RETAIN_LIMIT / 50);
  await store.setQuery('é'.repeat(128));
  assert.equal(store.pageError, null); assert.equal(n, HISTORY_RETAIN_LIMIT / 50 + 1);
  // Even an overlarge backend reply cannot break the frontend retention bound.
  f.api.gitHistoryList = async () => page(Array.from({ length: 2100 }, (_, i) => String(i)));
  await store.refresh(); assert.equal(store.commits.length, 2000); assert.equal(store.retainedLimit, true);
});

for (const stage of ['discovery', 'refs', 'page', 'detail']) {
  test(`host/context switch retires pending ${stage} response`, async () => {
    const { store, f } = fixture(); const old = deferred(); let pending;
    if (stage !== 'discovery') await store.refresh();
    if (stage === 'discovery') { f.api.gitHistoryRepositories = () => old.promise; pending = store.refresh(); }
    if (stage === 'refs') {
      const started = deferred();
      f.api.gitHistoryBranches = () => { started.resolve(); return old.promise; };
      pending = store.refresh();
      await started.promise; // Barrier: the refs request is actually in flight.
    }
    if (stage === 'page') { f.api.gitHistoryList = () => old.promise; pending = store.setQuery('old'); }
    if (stage === 'detail') { f.api.gitHistoryCommit = () => old.promise; pending = store.selectCommit('a'); }
    f.files.contextVersion = '2'; f.link.dataHost = f.link.host = 'remote'; f.link.phase = 'connected';
    // Even without starting a replacement request, current identity rejects replies.
    old.resolve(stage === 'discovery' ? { repositories: [repo('old')], truncated: false } : stage === 'refs' ? refs() : stage === 'page' ? page(['old']) : detail('old'));
    await pending;
    assert.notEqual(store.detail?.oid, 'old'); assert.notEqual(store.commits[0]?.oid, 'old');
    assert.notEqual(store.selectedRepository, 'old');
    f.api.gitHistoryRepositories = async () => ({ repositories: [repo()], truncated: false });
    f.api.gitHistoryBranches = async () => refs(); f.api.gitHistoryList = async () => page(['new']);
    await store.ensureLoaded();
    assert.deepEqual(store.commits.map(c => c.oid), ['new']);
    assert.equal(store.loading, false); assert.equal(store.detailLoading, false);
  });
}

test('offline blocks all calls; same-host reconnect revalidates prior repo and rejects pre-drop reads', async () => {
  const { store, f, calls } = fixture();
  f.link.dataHost = f.link.host = 'ssh'; f.link.phase = 'connected';
  await store.refresh();
  const old = deferred(); f.api.gitHistoryCommit = () => old.promise;
  const pending = store.selectCommit('a');
  f.link.phase = 'disconnected'; f.link.blockedReason = 'Not connected to ssh.';
  await store.ensureLoaded();
  assert.equal(store.blockedReason, 'Not connected to ssh.');
  assert.equal(store.detailLoading, false); assert.equal(store.stale, true);
  const count = calls.length;
  await store.refresh(); await store.selectRef('HEAD'); await store.loadMore();
  assert.equal(calls.length, count);
  f.link.phase = 'connected'; f.link.blockedReason = null; f.connectionStore.generation++;
  await store.ensureLoaded();
  old.resolve(detail('a')); await pending;
  assert.equal(store.selectedRepository, '.'); assert.equal(store.detail, null);
  assert.equal(store.selectedCommit, null); assert.equal(store.detailLoading, false);
  assert.equal(store.stale, false);
});

test('capability is independent of Changes, local old bindings error and remote missing capability makes no calls', async () => {
  const { store, f, calls } = fixture();
  f.link.phase = 'connected'; f.link.dataHost = f.link.host = 'old';
  f.connectionStore.capabilities = { gitChanges: true };
  await store.ensureLoaded();
  assert.match(store.blockedReason, /not supported/); assert.equal(calls.length, 0);
  f.connectionStore.capabilities.gitHistory = true;
  await store.ensureLoaded(); assert.equal(store.tip, 'pinned');
  f.connectionStore.capabilities.gitHistory = false;
  await store.ensureLoaded(); assert.deepEqual(store.commits, []);
  // Local hosts advertise history; an old desktop binding still fails in the API.
  f.link.phase = 'local'; f.link.dataHost = f.link.host = undefined; f.connectionStore.capabilities = { gitHistory: true };
  f.api.gitHistoryRepositories = async () => { throw new Error('Old desktop: update required'); };
  await store.ensureLoaded();
  assert.match(store.error, /update required/); assert.equal(store.loading, false);
});

test('empty/unborn, truncated refs, disappeared ref, errors and explicit retry are not false successes', async () => {
  const { store, f } = fixture();
  f.api.gitHistoryRepositories = async () => ({ repositories: [], truncated: false });
  await store.refresh(); assert.deepEqual(store.commits, []); assert.equal(store.error, null);
  f.api.gitHistoryRepositories = async () => ({ repositories: [repo()], truncated: false });
  f.api.gitHistoryBranches = async () => ({ ...refs(), unborn: true, truncated: true });
  f.api.gitHistoryList = async () => page([], '', '');
  await store.refresh(); assert.equal(store.unborn, true); assert.equal(store.refsTruncated, true);
  assert.equal(store.canLoadMore, false); assert.equal(store.loading, false);
  await store.selectRef('refs/heads/main');
  f.api.gitHistoryBranches = async () => ({ ...refs(), refs: [] });
  await store.refresh(); assert.equal(store.selectedRef, 'HEAD');
  f.api.gitHistoryList = async () => { throw new Error('missing pinned object; refresh'); };
  await store.refresh(); assert.match(store.pageError, /missing pinned object/);
  assert.equal(store.loading, false);
  f.api.gitHistoryList = async () => page(); await store.ensureLoaded();
  assert.equal(store.pageError, null); assert.equal(store.tip, 'pinned');
  f.api.gitHistoryCommit = async () => { throw new Error('detail unavailable'); };
  await store.selectCommit('a'); assert.match(store.detailError, /detail unavailable/);
  assert.equal(store.detailLoading, false);
  f.api.gitHistoryCommit = async () => detail('a'); await store.selectCommit('a');
  assert.equal(store.detailError, null); assert.equal(store.detail.oid, 'a');
  store.invalidate(); assert.equal(store.stale, true); assert.equal(store.selectedCommit, null);
  await store.ensureLoaded(); assert.equal(store.stale, false);
});

test('GH-16 a matching Changes write invalidates lazily, retires old pages and leaves snapshot tabs alone', async () => {
  const { store, f, calls } = fixture();
  const tabs = [{ id: 'history:a', history: { tip: 'pinned', commit: 'a' } }]; f.files.tabs = tabs;
  f.api.gitHistoryList = async req => req.cursor ? page(['c'], '') : page(['a', 'b'], 'next');
  await store.refresh(); await store.selectCommit('a');
  const revision = store.revision, count = calls.length;
  const old = deferred(), started = deferred();
  f.api.gitHistoryList = () => { started.resolve(); return old.promise; };
  const pending = store.loadMore(); await started.promise;
  store.invalidateHost('other');
  assert.equal(store.revision, revision, 'another host never clears this browser');
  assert.equal(store.pagesLoading, true);
  store.invalidateHost(undefined);
  assert.equal(store.revision, revision + 1); assert.equal(store.stale, true);
  assert.deepEqual(store.commits, []); assert.equal(store.selectedCommit, null);
  assert.equal(store.pagesLoading, false); assert.equal(calls.length, count, 'invalidation itself makes no calls');
  old.resolve(page(['old-page'], '', 'pinned')); await pending;
  assert.deepEqual(store.commits, []); assert.equal(store.pageError, null);
  f.api.gitHistoryList = async req => { calls.push(['page', req]); return page(['new', 'a'], '', 'moved'); };
  await store.ensureLoaded();
  assert.equal(store.tip, 'moved'); assert.deepEqual(store.commits.map(c => c.oid), ['new', 'a']);
  assert.equal(calls.at(-1)[1].cursor, '', 'repinned from a fresh first page');
  assert.equal(store.selectedCommit, null, 'no stale selection restoration');
  assert.equal(f.files.tabs, tabs); assert.deepEqual(tabs[0].history, { tip: 'pinned', commit: 'a' });
  assert.equal(store.revision, revision + 1, 'reloading never bumps revision, so the UI effect cannot loop');
});

test('GH-16 refresh, reopen and focus are coalesced without request storms', async () => {
  const { store, f, calls } = fixture();
  await store.ensureLoaded(); await store.refresh(); await store.ensureLoaded();
  assert.equal(store.revision, 0, 'internal and explicit reloads do not bump revision');
  assert.equal(calls.filter(c => c[0] === 'refs').length, 2);
  store.dialogOpened(); assert.equal(store.revision, 1); assert.equal(store.stale, true);
  store.focusRefresh(); assert.equal(store.revision, 1, 'focus while already stale does nothing');
  const reply = deferred(); f.api.gitHistoryRepositories = () => reply.promise;
  const pending = store.ensureLoaded();
  store.focusRefresh(); store.focusRefresh(); await store.ensureLoaded();
  // Opening straight into History: the visible effect may start loading before the open hook.
  store.dialogOpened();
  assert.equal(store.revision, 1, 'focus and reopen during a load coalesce');
  reply.resolve({ repositories: [repo()], truncated: false }); await pending;
  assert.equal(store.stale, false); assert.equal(store.tip, 'pinned');
  store.focusRefresh(); assert.equal(store.revision, 2);
  f.link.phase = 'disconnected'; f.link.blockedReason = 'Not connected.';
  const count = calls.length;
  store.focusRefresh(); store.dialogOpened(); await store.ensureLoaded(); await store.ensureLoaded();
  assert.equal(calls.length, count, 'blocked hosts get no calls');
  store.focusRefresh(); assert.equal(store.revision, 3, 'focus while blocked and stale does not churn');
});

test('GH-16 disconnect during pagination and details drops old replies; reconnect rediscovers once', async () => {
  const { store, f, calls } = fixture();
  f.link.dataHost = f.link.host = 'ssh'; f.link.phase = 'connected';
  f.api.gitHistoryList = async () => page(['a', 'b'], 'next');
  await store.refresh(); await store.selectCommit('b');
  const oldPage = deferred(), oldDetail = deferred();
  f.api.gitHistoryList = () => oldPage.promise; f.api.gitHistoryCommit = () => oldDetail.promise;
  const more = store.loadMore(), details = store.selectCommit('a');
  f.link.phase = 'disconnected'; f.link.blockedReason = 'Not connected to ssh.';
  await store.ensureLoaded();
  assert.deepEqual(store.commits, []); assert.equal(store.pagesLoading, false); assert.equal(store.detailLoading, false);
  f.link.phase = 'connected'; f.link.blockedReason = null; f.connectionStore.generation++;
  let discovery = 0; f.api.gitHistoryRepositories = async () => { discovery++; return { repositories: [repo()], truncated: false }; };
  f.api.gitHistoryList = async () => page(['fresh'], '', 'new-tip'); f.api.gitHistoryCommit = async req => detail(req.commit);
  await Promise.all([store.ensureLoaded(), store.ensureLoaded()]);
  assert.equal(discovery, 1, 'concurrent ensureLoaded callers share one discovery');
  oldPage.resolve(page(['stale'], '', 'pinned')); oldDetail.resolve(detail('a'));
  await more; await details;
  assert.deepEqual(store.commits.map(c => c.oid), ['fresh']); assert.equal(store.tip, 'new-tip');
  assert.equal(store.selectedCommit, null); assert.equal(store.detail, null);
  assert.equal(store.selectedRepository, '.'); assert.equal(store.pageError, null);
  assert.equal(calls.filter(c => c[0] === 'refs').length, 2);
});

test('working tree comparisons pin context and ref, refresh retires old replies, close/context switch cannot revive', async () => {
  const { store, f } = fixture();
  await store.ensureLoaded();
  const pending = [], requests = []; let retired = 0;
  f.files.invalidateComparisons = () => retired++;
  f.api.gitHistoryCompare = (request, host) => { requests.push([request, host]); const d = deferred(); pending.push(d); return d.promise; };
  const reply = (token) => ({ ref: 'HEAD', oid: 'tip', commit: 'tip', token, changes: [], truncated: false });
  const first = store.openComparison('HEAD');
  assert.deepEqual(requests[0], [{ dir: 'C:\\project', repo: '.', ref: 'HEAD', expectedOID: 'tip' }, undefined]);
  assert.equal(store.comparisonLabel, 'HEAD → working tree');
  const second = store.refreshComparison();
  pending[0].resolve(reply('old')); await first;
  assert.equal(store.comparison, null); assert.equal(store.comparisonLoading, true);
  pending[1].resolve(reply('fresh')); await second;
  assert.equal(store.comparison.token, 'fresh');
  const third = store.refreshComparison(); store.closeComparison();
  pending[2].resolve(reply('closed')); await third;
  assert.equal(store.comparison, null); assert.equal(store.comparisonRequest, null);
  const fourth = store.openComparison('HEAD');
  f.link.dataHost = f.link.host = 'other'; f.link.phase = 'connected';
  pending[3].resolve(reply('foreign')); await fourth;
  assert.equal(store.comparison, null);
  await store.ensureLoaded(); assert.equal(store.comparisonRequest, null);
  assert.ok(retired >= 4);
});
