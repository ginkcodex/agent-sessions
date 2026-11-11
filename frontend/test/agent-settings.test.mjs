import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule, loadServerComponent, fixtureStore, renderWithFixture } from './helpers.mjs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { desktopAPI, useDesktopApp } from './desktop-app.mjs';

const url = new URL('../src/lib/stores/agentSettings.svelte.ts', import.meta.url);
const { AgentSettingsStore } = await loadServerModule(url, [
  ["import { api } from '../api';", fixtureStore('api', '__agentSettingsFixture')],
  ["import { link } from './link.svelte';", fixtureStore('link', '__agentSettingsFixture')],
  ["import { epoch } from '../link';", 'const epoch = () => globalThis.__agentSettingsFixture.epoch;'],
  ["'../portable'", JSON.stringify(new URL('../src/lib/portable.ts', import.meta.url).href)],
]);
const supported = [{ id: 'claude-code', name: 'Claude Code' }, { id: 'codex', name: 'Codex' }, { id: 'opencode', name: 'OpenCode' }];
const defaults = () => ({ supported, enabled: supported.map(a => a.id), active: supported.map(a => a.id), restartRequired: false });
function fixture() {
  const calls = [];
  return globalThis.__agentSettingsFixture = {
    epoch: 0, link: { stale: false, locked: false }, calls,
    api: {
      getAgentSettings: async () => defaults(),
      setAgentSettings: async enabled => { calls.push(enabled); return { ...defaults(), enabled, restartRequired: true }; },
    },
  };
}

test('agent selection defaults to all, prevents disabling the last and saves canonical order', async () => {
  const f = fixture();
  const store = new AgentSettingsStore();
  await store.refresh();
  assert.deepEqual(store.selected, supported.map(a => a.id));
  store.toggle('claude-code', false);
  store.toggle('codex', false);
  store.toggle('opencode', false);
  store.toggle('unknown', true);
  assert.deepEqual(store.selected, ['opencode']);
  store.toggle('claude-code', true);
  assert.deepEqual(store.selected, ['claude-code', 'opencode']);
  assert.equal(store.dirty, true);
  assert.deepEqual(store.activeAgents.map(a => a.id), supported.map(a => a.id), 'unsaved drafts do not change filter buttons');
  await store.save();
  assert.deepEqual(f.calls, [['claude-code', 'opencode']]);
  assert.equal(store.settings.restartRequired, true);
  assert.deepEqual(store.activeAgents.map(a => a.id), supported.map(a => a.id), 'saved filters stay synchronized with backend until restart');
  assert.equal(store.dirty, false);
  assert.deepEqual(store.activeAgents.map(a => a.id), supported.map(a => a.id), 'restart-pending selections do not hide currently active agents');
});

test('stale settings fetch/save never change another host and errors preserve drafts', async () => {
  const f = fixture();
  const store = new AgentSettingsStore();
  f.api.getAgentSettings = async () => { f.epoch++; return defaults(); };
  await store.refresh();
  assert.equal(store.settings, null);
  f.api.getAgentSettings = async () => defaults();
  await store.refresh();
  store.toggle('codex', false);
  f.api.setAgentSettings = async () => { throw new Error('Save failed'); };
  await store.save();
  assert.equal(store.error, 'Save failed');
  assert.deepEqual(store.selected, ['claude-code', 'opencode']);
  f.api.setAgentSettings = async enabled => { f.epoch++; return { ...defaults(), enabled, restartRequired: true }; };
  await store.save();
  assert.equal(store.settings.restartRequired, false);
  assert.equal(store.saving, false);
  await store.save();
  assert.deepEqual(f.calls, []);
});

test('older host settings errors are visible and cannot be saved', async () => {
  const f = fixture();
  f.api.getAgentSettings = async () => { throw new Error('Coding agent settings are not supported'); };
  const store = new AgentSettingsStore();
  await store.refresh();
  assert.match(store.error, /not supported/);
  await store.save();
  assert.deepEqual(f.calls, []);
});

test('mock settings validate nonempty supported IDs and retain the active subset until restart', async () => {
  const api = new MockBackendAPI();
  assert.deepEqual((await api.getAgentSettings()).enabled, supported.map(a => a.id));
  await assert.rejects(api.setAgentSettings([]), /at least one/);
  await assert.rejects(api.setAgentSettings(['kiro']), /supported/);
  const saved = await api.setAgentSettings(['opencode', 'codex', 'codex']);
  assert.deepEqual(saved.enabled, ['codex', 'opencode']);
  assert.deepEqual(saved.active, supported.map(a => a.id));
  assert.equal(saved.restartRequired, true);
});

test('native agent settings API forwards the selection and reports unsupported older bindings', async () => {
  const calls = [];
  const App = {
    RestartApp: async () => { calls.push('restart'); },
    GetAgentSettings: async () => defaults(),
    SetAgentSettings: async enabled => { calls.push(enabled); return { ...defaults(), enabled, restartRequired: true }; },
  };
  useDesktopApp(App);
  const api = desktopAPI();
  assert.deepEqual(await api.getAgentSettings(), defaults());
  assert.deepEqual((await api.setAgentSettings(['codex'])).enabled, ['codex']);
  assert.deepEqual(calls, [['codex']]);
  await api.restartApp();
  assert.deepEqual(calls, [['codex'], 'restart']);
  delete App.RestartApp;
  await assert.rejects(api.restartApp(), /restart the app manually/);
  App.SetAgentSettings = async () => { throw 'host unavailable'; };
  await assert.rejects(api.setAgentSettings(['codex']), error => error instanceof Error && error.message === 'host unavailable');
  delete App.GetAgentSettings;
  await assert.rejects(api.getAgentSettings(), /does not support coding agent settings/);
});

test('agent summary follows active agents until restart, including future provider labels', async () => {
  const summaryURL = new URL('../src/lib/components/sidebar/AgentSummary.svelte', import.meta.url);
  const summary = await loadServerComponent(summaryURL, [
    ["import { appState } from '../../stores/appState.svelte';", fixtureStore('appState')],
    ["import { agentSettings } from '../../stores/agentSettings.svelte';", fixtureStore('agentSettings')],
  ]);
  const html = await renderWithFixture(summary, {
    appState: { agentCounts: { codex: 8, kiro: 2 }, filter: {} },
    agentSettings: { activeAgents: [{ id: 'codex', label: 'Codex' }, { id: 'kiro', label: 'Kiro' }] },
  });
  assert.match(html, /Codex sessions \(8\)/);
  assert.match(html, /Kiro sessions \(2\)/);
  assert.doesNotMatch(html, /Claude|OpenCode/);
});

const componentURL = new URL('../src/lib/components/common/CodingAgentsSettings.svelte', import.meta.url);
const component = await loadServerComponent(componentURL, [
  ["import { api } from '../../api';", 'const api = {};'],
  ["import { epoch } from '../../link';", 'const epoch = () => 0;'],
  ["import { filePrompt } from '../../stores/filePrompt.svelte';", 'const filePrompt = {};'],
  ["import { fileActions } from '../../stores/fileActions.svelte';", 'const fileActions = {};'],
  ["import ChatAdapters from '../chat/ChatAdapters.svelte';", 'const ChatAdapters = () => {};'],
  ["import { agentSettings } from '../../stores/agentSettings.svelte';", fixtureStore('agentSettings')],
  ["import { link } from '../../stores/link.svelte';", fixtureStore('link')],
  ["'../../hosts'", JSON.stringify(new URL('../src/lib/hosts.ts', import.meta.url).href)],
]);

test('remote agent settings offer Reconnect instead of restarting the local app', async () => {
  const html = await renderWithFixture(component, {
    agentSettings: { settings: { ...defaults(), restartRequired: true }, selected: ['codex'], dirty: false },
    link: { dataHost: 'dev', stale: false, locked: false },
  });
  assert.match(html, />Reconnect<\/button>/);
  assert.doesNotMatch(html, />Restart<\/button>/);
});

test('settings renders supported agent options, last-agent guard and restart guidance', async () => {
  const html = await renderWithFixture(component, {
    agentSettings: { settings: { ...defaults(), enabled: ['codex'], restartRequired: true }, selected: ['codex'], dirty: false },
    link: { dataHost: null, stale: false, locked: false },
  });
  for (const agent of supported) assert.match(html, new RegExp(agent.name));
  assert.match(html, /At least one agent must stay enabled/);
  assert.match(html, /Restart the app/);
  assert.match(html, />Restart<\/button>/);
  assert.match(html, /type="checkbox" checked="" disabled=""/);
  assert.match(html, /Session files are never deleted/);
});
