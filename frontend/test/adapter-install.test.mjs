import test from 'node:test';
import assert from 'node:assert/strict';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { fixtureStore, loadServerComponent, loadServerModule, renderWithFixture, renderedButtons, unescapeHTML } from './helpers.mjs';

const STORE_URL = new URL('../src/lib/stores/adapterInstall.svelte.ts', import.meta.url);
const ROW_URL = new URL('../src/lib/components/chat/ChatAdapterRow.svelte', import.meta.url);
const lib = (rel, base) => JSON.stringify(new URL(rel, base).href);

const { AdapterInstaller } = await loadServerModule(STORE_URL, [
  ["import { api } from '../api';", 'const api = {};'],
  ["import { epoch } from '../link';", 'const epoch = () => 0;'],
  ["'../manage'", lib('../manage.ts', STORE_URL)],
  ["import { chats } from './chats.svelte';", 'const chats = { loadAgents: async () => [] };'],
]);
const rowModule = await loadServerComponent(ROW_URL, [
  ["'../../managedTools'", lib('../../managedTools.ts', ROW_URL)],
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

test('one click proposes and installs that proposal, with no second question', async () => {
  const { store, calls } = setup((r) => (r.action === 'propose' ? state('proposed', { tool: r.tool, proposal }) : state('installing', { tool: 'claude-agent-acp', jobId: 'job0' })));
  await store.run('claude-agent-acp');
  assert.deepEqual(calls, [
    { action: 'propose', expectedHostId: HOST, tool: 'claude-agent-acp' },
    { action: 'install', expectedHostId: HOST, token: 'once' },
  ]);
  assert.equal(store.state.state, 'installing');
  assert.equal(store.tool, 'claude-agent-acp');
  store.reset();
});

test('Install sends the token once, polls the job and reloads the agents when it ends', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  let polls = 0;
  const { store, calls, installed } = setup((r) => {
    if (r.action === 'propose') return state('proposed', { tool: r.tool, proposal });
    if (r.action === 'install') return state('installing', { tool: 'claude-agent-acp', jobId: 'job1' });
    return ++polls < 2 ? state('installing', { tool: 'claude-agent-acp', jobId: 'job1' }) : state('installed', { tool: 'claude-agent-acp', jobId: 'job1' });
  });
  await store.run('claude-agent-acp');
  await Promise.all([store.install(), store.install()]);
  assert.deepEqual(calls.filter((c) => c.action === 'install'), [{ action: 'install', expectedHostId: HOST, token: 'once' }]);
  assert.equal(store.state.state, 'installing');
  assert.ok(store.busy);
  await store.run('codex-acp');
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
  await failing.store.run('codex-acp');
  t.mock.timers.tick(2000);
  await flush();
  assert.equal(failing.store.state.state, 'failed');
  assert.equal(failing.store.error, 'Installation failed while installing the codex-acp packages with npm. Try again', 'no approval step to ask for');
  assert.deepEqual(failing.installed, ['codex-acp']);
  assert.ok(!failing.store.busy, 'Install can be tried again');

  const cancelled = setup((r) => {
    if (r.action === 'propose') return state('proposed', { tool: r.tool, proposal });
    if (r.action === 'install') return state('installing', { tool: 'claude-agent-acp', jobId: 'job3' });
    if (r.action === 'cancel') return state('canceled', { tool: 'claude-agent-acp', jobId: 'job3', message: 'Installation canceled' });
    return state('installing', { tool: 'claude-agent-acp', jobId: 'job3' });
  });
  await cancelled.store.run('claude-agent-acp');
  await cancelled.store.cancel();
  assert.deepEqual(cancelled.calls.at(-1), { action: 'cancel', expectedHostId: HOST, jobId: 'job3' });
  assert.equal(cancelled.store.error, 'Installation canceled');
  t.mock.timers.tick(10_000);
  await flush();
  assert.equal(cancelled.calls.at(-1).action, 'cancel', 'no polling after the end');
});

test('old servers, hosts without managed installs, other hosts and expired offers fail closed', async () => {
  const old = setup(() => assert.fail('no call'), { managed: false });
  await old.store.run('claude-agent-acp');
  assert.match(old.store.error, /older/);
  assert.equal(old.store.state, undefined);

  const noCaps = setup(() => assert.fail('no call'), { conn: { ...connection, capabilities: { acpChat: true } } });
  await noCaps.store.run('claude-agent-acp');
  assert.match(noCaps.store.error, /no app-managed installs/);
  assert.deepEqual(noCaps.calls, []);

  const refused = setup(() => state('unavailable', { tool: 'claude-agent-acp', message: 'Node.js v16 on this host is too old.' }));
  await refused.store.run('claude-agent-acp');
  assert.equal(refused.store.error, 'Node.js v16 on this host is too old.');
  assert.equal(refused.store.state, undefined);

  const other = setup(() => state('proposed', { tool: 'claude-agent-acp', proposal: { ...proposal, hostId: 'ssh:other' } }));
  await other.store.run('claude-agent-acp');
  assert.match(other.store.error, /another host/);
  assert.equal(other.store.state, undefined);
  assert.equal(other.calls.length, 1, 'nothing installed');

  const wrongTool = setup(() => state('proposed', { tool: 'gopls', proposal: { ...proposal, tool: 'gopls' } }));
  await wrongTool.store.run('claude-agent-acp');
  assert.equal(wrongTool.store.state, undefined);

  const expired = setup(() => state('proposed', { tool: 'claude-agent-acp', proposal: { ...proposal, expires: '2000-01-01T00:00:00Z' } }));
  await expired.store.run('claude-agent-acp');
  assert.deepEqual(expired.calls.map((c) => c.action), ['propose']);
  assert.match(expired.store.error, /expired/);

  const unknown = setup(() => assert.fail('no call'));
  await unknown.store.run('gopls');
  assert.deepEqual(unknown.calls, [], 'only adapters install from chat');
});

test('a host switch drops the flow and its late answers', async () => {
  let answer;
  const { store, bump } = setup(() => new Promise((resolve) => (answer = resolve)));
  const pending = store.run('claude-agent-acp');
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

async function renderRow(agent, { installer = {}, capabilities = { codeDiagnostics: true }, host = 'dev-box' } = {}) {
  return unescapeHTML(await renderWithFixture(rowModule, { connectionStore: { capabilities }, installer }, { agent, host }));
}

const claude = {
  agent: 'claude-code',
  name: 'Claude Code',
  available: false,
  installTool: 'claude-agent-acp',
  pinned: '0.106.0',
  problem: 'claude-agent-acp is not installed',
};
const ready = { ...claude, available: true, managed: true, version: '0.106.0', path: '/data/managed-tools/claude-agent-acp-0.106.0-linux-x64/main.js', problem: undefined };
const buttons = (html) => renderedButtons(html).map((b) => `${b.text}${b.disabled ? ' (disabled)' : ''}`);
const status = (html) => /class="status[^"]*"[^>]*>([^<]*)</.exec(html)?.[1];

test('each agent is one row: ready, outdated with Update, or Install', async () => {
  const offer = await renderRow(claude);
  assert.equal(status(offer), 'Not installed');
  assert.deepEqual(buttons(offer), ['Install']);
  assert.match(offer, /title="Installs claude-agent-acp 0\.106\.0 for this app on dev-box"/);

  const ok = await renderRow(ready);
  assert.equal(status(ok), 'Ready · 0.106.0');
  assert.deepEqual(buttons(ok), []);
  assert.match(ok, /title="\/data\/managed-tools\/claude-agent-acp-0\.106\.0-linux-x64\/main\.js"/);

  const old = await renderRow({ ...ready, version: '0.105.2', outdated: true });
  assert.equal(status(old), '0.105.2 · 0.106.0 available');
  assert.deepEqual(buttons(old), ['Update']);

  const opencode = await renderRow({ agent: 'opencode', name: 'OpenCode', available: true, path: '/usr/bin/opencode' });
  assert.equal(status(opencode), 'Ready');
  assert.deepEqual(buttons(opencode), []);

  // Nothing about npm or copying a command, in any state.
  for (const html of [offer, ok, old, opencode]) assert.doesNotMatch(html, /npm|Copy/);

  const busy = await renderRow(claude, { installer: { busy: true, tool: 'codex-acp' } });
  assert.deepEqual(buttons(busy), ['Install (disabled)'], 'another adapter is installing');
});

test('where the app cannot install, the row says why and offers nothing', async () => {
  for (const [why, html] of [
    ['an older server', await renderRow({ ...claude, installTool: undefined, problem: 'claude-agent-acp was not found on PATH', install: 'npm install -g x' })],
    ['no managed installs on the host', await renderRow({ ...claude, problem: 'claude-agent-acp was not found on PATH' }, { capabilities: {} })],
  ]) {
    assert.deepEqual(buttons(html), [], why);
    assert.equal(status(html), 'claude-agent-acp was not found on PATH', why);
    assert.doesNotMatch(html, /npm install/, why);
  }
  const override = await renderRow({ ...claude, override: true, problem: 'the configured adapter /opt/x does not exist' });
  assert.deepEqual(buttons(override), []);
  assert.equal(status(override), 'the configured adapter /opt/x does not exist');
  assert.match(override, /\[chat\.adapters\]<\/code> in config\.toml/);
  assert.equal(status(await renderRow({ ...ready, override: true })), 'Ready · set in config.toml');
});

test('while it installs the row offers Cancel, and a failure shows its message', async () => {
  const asking = await renderRow(claude, { installer: { tool: 'claude-agent-acp', pending: true, busy: true } });
  assert.equal(status(asking), 'Installing…');
  assert.deepEqual(buttons(asking), []);

  const running = await renderRow(claude, { installer: { tool: 'claude-agent-acp', state: state('installing', { jobId: 'j' }), pending: false, busy: true } });
  assert.equal(status(running), 'Installing…');
  assert.deepEqual(buttons(running), ['Cancel']);
  const updating = await renderRow({ ...ready, outdated: true, version: '0.105.2' }, { installer: { tool: 'claude-agent-acp', state: state('installing', { jobId: 'j' }), busy: true } });
  assert.equal(status(updating), 'Updating…');

  const failed = await renderRow(claude, { installer: { tool: 'claude-agent-acp', state: state('failed'), error: 'Installation failed while installing the claude-agent-acp packages with npm. Try again' } });
  assert.match(failed, /role="alert"[^>]*>Installation failed while installing/);
  assert.deepEqual(buttons(failed), ['Install']);
});

test('the mock installs Claude Code, and fails Codex once before it installs', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const mock = new MockBackendAPI();
  mock.chat.missing.add('claude-code');
  mock.chat.missing.add('codex');
  mock.chat.outdated.add('codex');
  let agents = await mock.chatAgents();
  assert.deepEqual(agents.map((a) => [a.agent, a.available, a.installTool, a.pinned]), [
    ['claude-code', false, 'claude-agent-acp', '0.106.0'],
    ['codex', false, 'codex-acp', '2.6.0'],
    ['opencode', true, undefined, undefined],
  ]);
  const store = new AdapterInstaller(mock, () => 0, async () => {
    agents = await mock.chatAgents();
  });
  const run = async (tool) => {
    await store.run(tool);
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
  assert.equal(claudeNow.version, '0.106.0');
  assert.match(claudeNow.path, /managed-tools\/claude-agent-acp-0\.106\.0-linux-x64\//);

  assert.equal(await run('codex-acp'), 'failed');
  assert.match(store.error, /codex-acp packages/);
  assert.equal(agents.find((a) => a.agent === 'codex').available, false);
  assert.equal(await run('codex-acp'), 'installed');
  assert.equal(agents.find((a) => a.agent === 'codex').available, true);
  const codexNow = agents.find((a) => a.agent === 'codex');
  assert.equal(codexNow.version, '2.6.0');
  assert.equal(codexNow.outdated, false);
  const chat = await mock.chatStart({ agent: 'codex', dir: '/home/user/projects/web-app' });
  assert.equal(chat.agent, 'codex');
  mock.chat.closeAll();
});
