import test from 'node:test';
import assert from 'node:assert/strict';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { fixtureStore, loadServerComponent, loadServerModule, renderWithFixture, renderedButtons, unescapeHTML } from './helpers.mjs';

const STORE_URL = new URL('../src/lib/stores/adapterInstall.svelte.ts', import.meta.url);
const MISSING_URL = new URL('../src/lib/components/chat/AdapterMissing.svelte', import.meta.url);
const lib = (rel, base) => JSON.stringify(new URL(rel, base).href);

const { AdapterInstaller } = await loadServerModule(STORE_URL, [
  ["import { api } from '../api';", 'const api = {};'],
  ["import { epoch } from '../link';", 'const epoch = () => 0;'],
  ["'../manage'", lib('../manage.ts', STORE_URL)],
  ["import { chats } from './chats.svelte';", 'const chats = { loadAgents: async () => [] };'],
]);
const missingModule = await loadServerComponent(MISSING_URL, [
  ["'../../portable'", lib('../../portable.ts', MISSING_URL)],
  ["'../../managedTools'", lib('../../managedTools.ts', MISSING_URL)],
  ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
  ["import { connectionStore } from '../../stores/connection.svelte';", fixtureStore('connectionStore')],
  ["import { adapterInstall as installer } from '../../stores/adapterInstall.svelte';", fixtureStore('installer')],
]);

const HOST = 'ssh:actual-host';
const connection = { hostId: HOST, phase: 'connected', host: 'dev-box', generation: 1, capabilities: { codeDiagnostics: true, acpChat: true } };
const proposal = {
  token: 'once',
  hostId: HOST,
  connectionId: 'consent-owner',
  tool: 'claude-agent-acp',
  version: '0.106.0',
  source: 'npm registry (@agentclientprotocol/claude-agent-acp@0.106.0)',
  integrity: 'package-lock.json sha256:abc; optional packages (platform binaries) left out',
  note: 'Node.js v20.11.0 on this host is older than 22. Also downloads Node.js v24.22.1 to run it.',
  destination: '/data/managed-tools/claude-agent-acp-0.106.0-linux-x64',
  expires: '2099-01-01T00:00:00Z',
};
const state = (s, extra = {}) => ({ hostId: HOST, state: s, ...extra });
const flush = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve();
};

// The real store on a scripted backend; reply answers each managedTool call.
function setup(reply, { conn = connection, managed = true } = {}) {
  const calls = [];
  const installed = [];
  let currentEpoch = 0;
  const backend = {
    async connectionState() {
      return conn;
    },
    ...(managed ? { async managedTool(r) { calls.push(r); return reply(r); } } : {}),
  };
  const store = new AdapterInstaller(backend, () => currentEpoch, (tool) => installed.push(tool));
  return { store, calls, installed, bump: () => currentEpoch++ };
}

test('nothing installs before the proposal is approved; Not now sends no token', async () => {
  const { store, calls, installed } = setup((r) => (r.action === 'propose' ? state('proposed', { tool: r.tool, proposal }) : state('idle')));
  await store.propose('claude-agent-acp');
  assert.deepEqual(calls, [{ action: 'propose', expectedHostId: HOST, tool: 'claude-agent-acp' }]);
  assert.equal(store.state.state, 'proposed');
  assert.equal(store.tool, 'claude-agent-acp');
  await store.dismiss();
  assert.deepEqual(calls.map((c) => c.action), ['propose', 'dismiss']);
  assert.ok(!calls.some((c) => c.token), 'no token left the app');
  assert.equal(store.state, undefined);
  assert.equal(store.tool, undefined);
  await store.install();
  assert.equal(calls.length, 2, 'nothing to install after Not now');
  assert.deepEqual(installed, []);
});

test('Install sends the token once, polls the job and reloads the agents when it ends', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  let polls = 0;
  const { store, calls, installed } = setup((r) => {
    if (r.action === 'propose') return state('proposed', { tool: r.tool, proposal });
    if (r.action === 'install') return state('installing', { tool: 'claude-agent-acp', jobId: 'job1' });
    return ++polls < 2 ? state('installing', { tool: 'claude-agent-acp', jobId: 'job1' }) : state('installed', { tool: 'claude-agent-acp', jobId: 'job1' });
  });
  await store.propose('claude-agent-acp');
  await Promise.all([store.install(), store.install()]);
  assert.deepEqual(calls.filter((c) => c.action === 'install'), [{ action: 'install', expectedHostId: HOST, token: 'once' }]);
  assert.equal(store.state.state, 'installing');
  assert.ok(store.busy);
  await store.propose('codex-acp');
  assert.equal(calls.filter((c) => c.action === 'propose').length, 1, 'one install at a time');

  t.mock.timers.tick(2000);
  await flush();
  assert.deepEqual(calls.at(-1), { action: 'status', expectedHostId: HOST, jobId: 'job1' });
  assert.equal(store.state.state, 'installing');
  t.mock.timers.tick(2000);
  await flush();
  assert.equal(store.state.state, 'installed');
  assert.deepEqual(installed, ['claude-agent-acp']);
  assert.equal(store.error, '');
  assert.equal(calls.filter((c) => c.action === 'install').length, 1, 'never replayed');
});

test('a failed or cancelled install says why and still reloads the agents', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const failing = setup((r) => {
    if (r.action === 'propose') return state('proposed', { tool: r.tool, proposal: { ...proposal, tool: 'codex-acp' } });
    if (r.action === 'install') return state('installing', { tool: 'codex-acp', jobId: 'job2' });
    return state('failed', { tool: 'codex-acp', jobId: 'job2', message: 'Installation failed while installing the codex-acp packages with npm; request fresh approval to retry' });
  });
  await failing.store.propose('codex-acp');
  await failing.store.install();
  t.mock.timers.tick(2000);
  await flush();
  assert.equal(failing.store.state.state, 'failed');
  assert.match(failing.store.error, /while installing the codex-acp packages/);
  assert.deepEqual(failing.installed, ['codex-acp']);
  assert.ok(!failing.store.busy, 'Install can be tried again');

  const cancelled = setup((r) => {
    if (r.action === 'propose') return state('proposed', { tool: r.tool, proposal });
    if (r.action === 'install') return state('installing', { tool: 'claude-agent-acp', jobId: 'job3' });
    if (r.action === 'cancel') return state('canceled', { tool: 'claude-agent-acp', jobId: 'job3', message: 'Installation canceled' });
    return state('installing', { tool: 'claude-agent-acp', jobId: 'job3' });
  });
  await cancelled.store.propose('claude-agent-acp');
  await cancelled.store.install();
  await cancelled.store.cancel();
  assert.deepEqual(cancelled.calls.at(-1), { action: 'cancel', expectedHostId: HOST, jobId: 'job3' });
  assert.equal(cancelled.store.error, 'Installation canceled');
  t.mock.timers.tick(10_000);
  await flush();
  assert.equal(cancelled.calls.at(-1).action, 'cancel', 'no polling after the end');
});

test('old servers, hosts without managed installs, other hosts and expired offers fail closed', async () => {
  const old = setup(() => assert.fail('no call'), { managed: false });
  await old.store.propose('claude-agent-acp');
  assert.match(old.store.error, /older/);
  assert.equal(old.store.state, undefined);

  const noCaps = setup(() => assert.fail('no call'), { conn: { ...connection, capabilities: { acpChat: true } } });
  await noCaps.store.propose('claude-agent-acp');
  assert.match(noCaps.store.error, /no app-managed installs/);
  assert.deepEqual(noCaps.calls, []);

  const refused = setup(() => state('unavailable', { tool: 'claude-agent-acp', message: 'Node.js v16 on this host is too old.' }));
  await refused.store.propose('claude-agent-acp');
  assert.equal(refused.store.error, 'Node.js v16 on this host is too old.');
  assert.equal(refused.store.state, undefined);

  const other = setup(() => state('proposed', { tool: 'claude-agent-acp', proposal: { ...proposal, hostId: 'ssh:other' } }));
  await other.store.propose('claude-agent-acp');
  assert.match(other.store.error, /another host/);
  assert.equal(other.store.state, undefined);
  await other.store.install();
  assert.equal(other.calls.length, 1, 'nothing to install');

  const wrongTool = setup(() => state('proposed', { tool: 'gopls', proposal: { ...proposal, tool: 'gopls' } }));
  await wrongTool.store.propose('claude-agent-acp');
  assert.equal(wrongTool.store.state, undefined);

  const expired = setup(() => state('proposed', { tool: 'claude-agent-acp', proposal: { ...proposal, expires: '2000-01-01T00:00:00Z' } }));
  await expired.store.propose('claude-agent-acp');
  await expired.store.install();
  assert.deepEqual(expired.calls.map((c) => c.action), ['propose']);
  assert.match(expired.store.error, /expired/);

  const unknown = setup(() => assert.fail('no call'));
  await unknown.store.propose('gopls');
  assert.deepEqual(unknown.calls, [], 'only adapters install from chat');
});

test('a host switch drops the flow and its late answers', async () => {
  let answer;
  const { store, bump } = setup(() => new Promise((resolve) => (answer = resolve)));
  const pending = store.propose('claude-agent-acp');
  await flush();
  assert.ok(store.pending);
  bump();
  store.reset();
  answer(state('proposed', { tool: 'claude-agent-acp', proposal }));
  await pending;
  assert.equal(store.state, undefined);
  assert.equal(store.tool, undefined);
  assert.equal(store.pending, false);
});

async function renderMissing(agent, { installer = {}, capabilities = { codeDiagnostics: true }, host = 'dev-box' } = {}) {
  return unescapeHTML(await renderWithFixture(missingModule, { toast: {}, connectionStore: { capabilities }, installer }, { agent, host }));
}

const claude = {
  agent: 'claude-code',
  name: 'Claude Code',
  available: false,
  install: 'npm install -g @agentclientprotocol/claude-agent-acp',
  installTool: 'claude-agent-acp',
  problem: 'claude-agent-acp was not found on PATH',
};
const buttons = (html) => renderedButtons(html).map((b) => `${b.text}${b.disabled ? ' (disabled)' : ''}`);

test('the adapter offers Install where the host can install it, and always keeps the command', async () => {
  const offer = await renderMissing(claude);
  assert.deepEqual(buttons(offer), ['Install…', 'Copy']);
  assert.match(offer, /Installs a pinned copy for this app only, after you review it/);
  assert.match(offer, /Or install it yourself: run this on dev-box:/);

  for (const [why, html] of [
    ['an older server', await renderMissing({ ...claude, installTool: undefined })],
    ['no managed installs on the host', await renderMissing(claude, { capabilities: {} })],
    ['an override in config.toml', await renderMissing({ ...claude, override: true })],
  ]) {
    assert.deepEqual(buttons(html), ['Copy'], why);
    assert.match(html, /To install it, run this on dev-box:/, why);
  }

  const busy = await renderMissing(claude, { installer: { busy: true, tool: 'codex-acp' } });
  assert.deepEqual(buttons(busy), ['Install… (disabled)', 'Copy'], 'another adapter is installing');
});

test('the review card names the host, package, source, integrity, Node.js and destination', async () => {
  const html = await renderMissing(claude, { installer: { tool: 'claude-agent-acp', state: state('proposed', { proposal }), pending: false } });
  for (const text of [
    'Install claude-agent-acp 0.106.0 on dev-box?',
    proposal.source,
    proposal.integrity,
    proposal.note,
    'install scripts disabled',
    "the host's Node.js 22 or later",
    'network access',
    'your own Claude Code CLI',
    `App-private destination: ${proposal.destination}`,
    `Target host identity: ${HOST}`,
  ]) {
    assert.ok(html.includes(text), text);
  }
  assert.deepEqual(buttons(html), ['Install', 'Not now', 'Copy']);
});

test('progress offers Cancel, and a failure shows its message beside the command', async () => {
  const running = await renderMissing(claude, { installer: { tool: 'claude-agent-acp', state: state('installing', { jobId: 'j' }), pending: false, busy: true } });
  assert.match(running, /Downloading claude-agent-acp into this app's folder on dev-box/);
  assert.deepEqual(buttons(running), ['Cancel download', 'Copy']);

  const failed = await renderMissing(claude, { installer: { tool: 'claude-agent-acp', state: state('failed'), error: 'Installation failed while installing the claude-agent-acp packages with npm; request fresh approval to retry' } });
  assert.match(failed, /role="alert"[^>]*>Installation failed while installing/);
  assert.deepEqual(buttons(failed), ['Install…', 'Copy']);
});

test('the mock installs Claude Code, and fails Codex once before it installs', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const mock = new MockBackendAPI();
  mock.chat.missing.add('claude-code');
  mock.chat.missing.add('codex');
  let agents = await mock.chatAgents();
  assert.deepEqual(agents.map((a) => [a.agent, a.available, a.installTool]), [
    ['claude-code', false, 'claude-agent-acp'],
    ['codex', false, 'codex-acp'],
    ['opencode', true, undefined],
  ]);
  const store = new AdapterInstaller(mock, () => 0, async () => {
    agents = await mock.chatAgents();
  });
  const run = async (tool) => {
    await store.propose(tool);
    assert.equal(store.state?.state, 'proposed', tool);
    await store.install();
    for (let i = 0; i < 3 && store.state?.state === 'installing'; i++) {
      t.mock.timers.tick(2000);
      await flush();
    }
    return store.state?.state;
  };

  assert.equal(await run('claude-agent-acp'), 'installed');
  const claudeNow = agents.find((a) => a.agent === 'claude-code');
  assert.equal(claudeNow.available, true);
  assert.equal(claudeNow.managed, true);
  assert.match(claudeNow.path, /managed-tools\/claude-agent-acp-0\.106\.0-linux-x64\//);

  assert.equal(await run('codex-acp'), 'failed');
  assert.match(store.error, /codex-acp packages/);
  assert.equal(agents.find((a) => a.agent === 'codex').available, false);
  assert.equal(await run('codex-acp'), 'installed');
  assert.equal(agents.find((a) => a.agent === 'codex').available, true);
  const chat = await mock.chatStart({ agent: 'codex', dir: '/home/user/projects/web-app' });
  assert.equal(chat.agent, 'codex');
  mock.chat.closeAll();
});
