import test from 'node:test';
import assert from 'node:assert/strict';
import { needsCodexRepair } from '../src/lib/codexRepair.ts';
import { loadServerModule, loadServerComponent, fixtureStore, renderWithFixture } from './helpers.mjs';
import { desktopAPI, useDesktopApp } from './desktop-app.mjs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
const id = '11111111-1111-4111-8111-111111111111';
const session = { ref: { agent: 'codex', id }, sourcePath: `/home/test/.codex/sessions/2026/11/06/rollout-restored-${id}.jsonl` };
const repaired = { path: `/home/test/.codex/sessions/2026/11/06/rollout-2026-08-21T13-52-35-${id}.jsonl`, backupPath: '/home/test/.codex/rollout-repair-backup-test' };
test('only exact legacy Codex filenames are repairable on Unix and Windows', () => {
  assert.equal(needsCodexRepair(session), true);
  assert.equal(needsCodexRepair({ ...session, sourcePath: `C:\\Users\\test\\.codex\\rollout-restored-${id}.jsonl` }), true);
  for (const meta of [{ ...session, sourcePath: repaired.path }, { ...session, sourcePath: undefined }, { ...session, ref: { ...session.ref, agent: 'opencode' } }, { ...session, ref: { ...session.ref, id: 'invalid' } }]) assert.equal(needsCodexRepair(meta), false);
});
const { CodexRepair } = await loadServerModule(new URL('../src/lib/stores/codexRepair.svelte.ts', import.meta.url), [
  ["'../codexRepair'", JSON.stringify(new URL('../src/lib/codexRepair.ts', import.meta.url).href)],
  ["import { api } from '../api';", fixtureStore('api', '__repairFixture')],
  ["import { epoch } from '../link';", 'const epoch = () => globalThis.__repairFixture.epoch;'],
  ["import { appState } from './appState.svelte';", fixtureStore('appState', '__repairFixture')],
  ["import { link } from './link.svelte';", fixtureStore('link', '__repairFixture')],
  ["import { toast } from './toast.svelte';", fixtureStore('toast', '__repairFixture')],
]);
function fixture() {
  const calls = [];
  return globalThis.__repairFixture = { epoch: 0, calls,
    api: { repairCodexSession: async ref => { calls.push(ref); return repaired; } },
    appState: { refresh: async () => { calls.push('refresh'); } },
    link: { stale: false, locked: false }, toast: { show: value => calls.push(value.title) } };
}
test('repair sends only the ref, reports its backup and refreshes without an optimistic path edit', async () => {
  const f = fixture(), store = new CodexRepair(); store.setSession(session); await store.run();
  assert.deepEqual(f.calls, [session.ref, 'Session repaired', 'refresh']);
  assert.equal(store.result, repaired); assert.equal(store.busy, false); assert.equal(store.error, '');
  assert.match(session.sourcePath, /rollout-restored-/); await store.run(); assert.equal(f.calls.length, 3);
});
test('errors permit retry; a refresh failure keeps completed-repair status', async () => {
  const f = fixture(), store = new CodexRepair();
  f.api.repairCodexSession = async () => { throw new Error('Close this session in Codex and retry.'); };
  store.setSession(session); await store.run(); assert.match(store.error, /Close this session/); assert.equal(store.result, null);
  f.api.repairCodexSession = async () => repaired;
  f.appState.refresh = async () => { throw new Error('reload failed'); };
  await store.run(); assert.equal(store.result, repaired); assert.match(store.error, /Session repaired.*Refresh/); assert.equal(store.busy, false);
});
test('duplicate clicks, stale links, old host epochs and normal sessions cannot write', async () => {
  const f = fixture(), store = new CodexRepair(); let finish;
  f.api.repairCodexSession = ref => { f.calls.push(ref); return new Promise(resolve => { finish = resolve; }); };
  store.setSession(session); const first = store.run(); await store.run(); assert.deepEqual(f.calls, [session.ref]); finish(repaired); await first;
  store.setSession({ ...session, sourcePath: repaired.path }); await store.run();
  store.setSession(session); f.link.stale = true; await store.run(); f.link.stale = false; f.link.locked = true; await store.run();
  f.link.locked = false; f.epoch++; await store.run(); assert.equal(f.calls.filter(v => typeof v === 'object').length, 1);
});
test('late replies cannot refresh or report success on another host or session', async () => {
  for (const change of ['host', 'session']) {
    const f = fixture(), store = new CodexRepair(); let finish;
    f.api.repairCodexSession = () => new Promise(resolve => { finish = resolve; }); store.setSession(session); const pending = store.run();
    if (change === 'host') f.epoch++; store.setSession({ ...session, sourcePath: change === 'session' ? '/other-source' : session.sourcePath });
    finish(repaired); await pending; assert.deepEqual(f.calls, []); assert.equal(store.result, null); assert.equal(store.busy, false);
  }
});
test('desktop binding forwards the repair, normalizes errors and refuses old/mock support', async () => {
  const calls = [], App = { RepairCodexSession: async ref => { calls.push(ref); return repaired; } }; useDesktopApp(App);
  const api = desktopAPI(); assert.deepEqual(await api.repairCodexSession(session.ref), repaired); assert.deepEqual(calls, [session.ref]);
  App.RepairCodexSession = async () => { throw 'repair failed'; }; await assert.rejects(api.repairCodexSession(session.ref), /repair failed/);
  delete App.RepairCodexSession; await assert.rejects(api.repairCodexSession(session.ref), /Update the app or connected host/);
  await assert.rejects(new MockBackendAPI().repairCodexSession(session.ref), /unavailable in the browser mock/);
});
const noticeURL = new URL('../src/lib/components/transcript/CodexRepairNotice.svelte', import.meta.url);
const notice = await loadServerComponent(noticeURL, [
  ["'../../codexRepair'", JSON.stringify(new URL('../../codexRepair.ts', noticeURL).href)],
  ["import { CodexRepair } from '../../stores/codexRepair.svelte';", `const CodexRepair = class {
    get busy() { return globalThis.__componentFixture.repair?.busy ?? false; }
    get error() { return globalThis.__componentFixture.repair?.error ?? ''; }
    get result() { return globalThis.__componentFixture.repair?.result ?? null; }
    setSession() {} run() {}
  };`],
  ["import { link } from '../../stores/link.svelte';", fixtureStore('link')],
  ["import { connectionStore } from '../../stores/connection.svelte';", fixtureStore('connectionStore')],
]);
const renderNotice = (extra = {}, props = {}) => renderWithFixture(notice, { link: { stale: false, locked: false }, connectionStore: { capabilities: { repairCodexSession: true } }, ...extra }, { meta: session, ...props });
test('notice explains import compatibility and backups, progress, errors and unsupported hosts', async () => {
  let html = await renderNotice(); assert.match(html, /This imported session needs a repair/); assert.match(html, /creates a backup/); assert.match(html, /Repair session/);
  html = await renderNotice({ repair: { busy: true } }); assert.match(html, /disabled/); assert.match(html, /Repairing/);
  html = await renderNotice({ repair: { error: 'Close Codex and retry' } }); assert.match(html, /role="alert">Close Codex and retry/);
  html = await renderNotice({ repair: { result: repaired } }); assert.match(html, /Repair complete/); assert.match(html, /Repaired/);
  html = await renderNotice({ connectionStore: { capabilities: {} } }); assert.match(html, /Update the app or connected host/); assert.match(html, /disabled/);
  html = await renderNotice({}, { disabledReason: 'Stop this terminal before repairing.' }); assert.match(html, /Stop this terminal/); assert.match(html, /disabled/);
  html = await renderNotice({}, { meta: { ...session, sourcePath: repaired.path } }); assert.doesNotMatch(html, /data-codex-repair/);
});
