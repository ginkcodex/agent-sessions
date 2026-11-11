import test from 'node:test';
import assert from 'node:assert/strict';
import { applySessionUpdate, beginTurn, emptyTail, endTurn } from '../src/lib/chatTail.ts';
import { chatAgentAvailable, permissionView, planMarkdown, tailMessages, toolStatus, unifiedDiff } from '../src/lib/chat.ts';
import { fixtureStore, loadServerComponent, loadServerModule, renderSessionHeader, renderWithFixture, renderedButtons, unescapeHTML } from './helpers.mjs';

const SENT = Date.parse('2026-11-11T10:00:00Z');
const chunk = (kind, text, messageId) => ({ sessionUpdate: kind, content: { type: 'text', text }, messageId });
const apply = (tail, ...updates) => updates.reduce(applySessionUpdate, tail);

test('the tail shows as a user message per prompt and one assistant message per turn', () => {
  let tail = apply(
    beginTurn(emptyTail(), { text: 'list files', sentAt: SENT }),
    chunk('agent_thought_chunk', 'Looking.', 'm1'),
    chunk('agent_message_chunk', 'Here:', 'm1'),
    { sessionUpdate: 'tool_call', toolCallId: 't1', title: 'ls', kind: 'execute', status: 'in_progress', rawInput: { command: 'ls' } },
    { sessionUpdate: 'plan', entries: [{ content: 'list', status: 'completed' }, { content: 'sum\nup', status: 'in_progress' }, { content: 'end' }] },
  );
  tail = apply(beginTurn(endTurn(tail), { text: 'thanks', sentAt: SENT }), chunk('agent_message_chunk', 'Welcome', 'm2'));

  const messages = tailMessages(tail);
  assert.deepEqual(messages.map((m) => [m.role, m.time]), [
    ['user', '2026-11-11T10:00:00.000Z'],
    ['assistant', ''],
    ['user', '2026-11-11T10:00:00.000Z'],
    ['assistant', ''],
  ]);
  assert.ok(messages.every((m) => m.id.startsWith('live:')));
  assert.equal(new Set(messages.map((m) => m.id)).size, messages.length, 'IDs are unique');
  assert.deepEqual(messages[0].parts, [{ kind: 'text', text: 'list files' }]);
  assert.deepEqual(messages[1].parts.map((p) => p.kind), ['reasoning', 'text', 'tool', 'text']);
  const tool = messages[1].parts[2].tool;
  assert.deepEqual({ id: tool.id, name: tool.name, input: tool.input, status: tool.status }, { id: 't1', name: 'ls', input: { command: 'ls' }, status: 'pending' });
  assert.equal(messages[1].parts[3].text, '**Plan**\n\n- ✓ list\n- ▸ sum up\n- ○ end');
  assert.deepEqual(messages[3].parts, [{ kind: 'text', text: 'Welcome' }]);
});

test('a tool with diffs adds a patch part, and a failed command says its exit code', () => {
  const tail = apply(
    beginTurn(emptyTail()),
    { sessionUpdate: 'tool_call', toolCallId: 'w', name: 'Write', title: '', status: 'completed', content: [{ type: 'diff', path: '/p/a.txt', oldText: 'one\ntwo\n', newText: 'one\n2\n' }] },
    { sessionUpdate: 'tool_call', toolCallId: 'x', kind: 'execute', title: 'make', status: 'failed', _meta: { terminal_output_delta: { data: 'boom' }, terminal_exit: { exit_code: 2 } } },
  );
  const [reply] = tailMessages(tail);
  assert.equal(reply.role, 'assistant');
  assert.deepEqual(reply.parts.map((p) => p.kind), ['tool', 'patch', 'tool']);
  assert.equal(reply.parts[0].tool.name, 'Write', 'the name stands in for a missing title');
  assert.deepEqual(reply.parts[1].files, ['/p/a.txt']);
  assert.equal(reply.parts[1].text, '--- /p/a.txt\n+++ /p/a.txt\n@@ -1,2 +1,2 @@\n-one\n-two\n+one\n+2');
  assert.equal(reply.parts[2].tool.status, 'error');
  assert.equal(reply.parts[2].tool.output, 'boom\nExit code 2');
});

test('unifiedDiff, toolStatus and chatAgentAvailable', () => {
  assert.equal(unifiedDiff({ path: 'n.txt', oldText: null, newText: 'hi\n' }), '--- /dev/null\n+++ n.txt\n@@ -0,0 +1,1 @@\n+hi');
  assert.equal(unifiedDiff({ path: 'e.txt', oldText: 'x', newText: '' }), '--- e.txt\n+++ e.txt\n@@ -1,1 +0,0 @@\n-x');
  assert.deepEqual(['completed', 'failed', 'pending', 'in_progress', 'odd'].map(toolStatus), ['completed', 'error', 'pending', 'pending', 'unknown']);
  assert.equal(planMarkdown([]), '**Plan**\n');
  const agents = [{ agent: 'codex', available: true }, { agent: 'claude-code', available: false }];
  assert.equal(chatAgentAvailable(agents, 'codex'), true);
  assert.equal(chatAgentAvailable(agents, 'claude-code'), false, 'a missing adapter');
  assert.equal(chatAgentAvailable(agents, 'opencode'), false, 'not listed');
});

test('a permission request shows its command or diff, and its options by kind in the agent order', () => {
  const edit = permissionView({
    id: 'p1',
    toolCall: {
      toolCallId: 't1', title: 'Write notes', kind: 'edit',
      content: [{ type: 'diff', path: '/p/NOTES.md', oldText: null, newText: 'hi\n' }, { type: 'content', content: { type: 'text', text: 'x' } }],
      locations: [{ path: '/p/NOTES.md' }, { path: '/p/other.md' }, { path: '/p/other.md' }],
    },
    options: [
      { optionId: 'a1', name: 'Allow', kind: 'allow_once' },
      { optionId: 'a2', name: 'Always allow edits', kind: 'allow_always' },
      { optionId: 'r1', name: 'Reject', kind: 'reject_once' },
      { optionId: 'r2', name: '', kind: 'reject_always' },
      { optionId: 'q', name: 'Ask later', kind: 'later' },
    ],
  });
  assert.equal(edit.title, 'Write notes');
  assert.equal(edit.kind, 'edit');
  assert.equal(edit.command, '');
  assert.deepEqual(edit.diffs, [{ path: '/p/NOTES.md', text: '--- /dev/null\n+++ /p/NOTES.md\n@@ -0,0 +1,1 @@\n+hi' }]);
  assert.deepEqual(edit.paths, ['/p/other.md'], 'a file with a diff is not listed again');
  assert.deepEqual(edit.choices.map((c) => [c.optionId, c.name, c.kindLabel, c.tone, c.lasting]), [
    ['a1', 'Allow', 'Allow once', 'allow', false],
    ['a2', 'Always allow edits', 'Always allow', 'allow', true],
    ['r1', 'Reject', 'Reject once', 'reject', false],
    ['r2', 'Always reject', 'Always reject', 'reject', false],
    ['q', 'Ask later', '', 'other', false],
  ]);

  const argv = permissionView({ id: 'p2', toolCall: { toolCallId: 't2', kind: 'execute', rawInput: { command: ['bash', '-lc', "echo 'hi' > a.txt"] } } });
  assert.equal(argv.command, `bash -lc 'echo '\\''hi'\\'' > a.txt'`);
  assert.equal(argv.title, 'A tool call');
  assert.deepEqual(argv.choices, []);
  assert.equal(permissionView({ id: 'p3', toolCall: { toolCallId: 't3', rawInput: { command: 'make test' } } }).command, 'make test');
});

// --- Components ------------------------------------------------------------

const COMPOSER_URL = new URL('../src/lib/components/chat/ChatComposer.svelte', import.meta.url);
const LIVE_TAIL_URL = new URL('../src/lib/components/chat/LiveTail.svelte', import.meta.url);
const lib = (rel, base) => JSON.stringify(new URL(rel, base).href);

let composerModule;
let liveTailModule;

function chat(fields = {}) {
  return {
    key: 'chat-1', chatId: 'c1', agent: 'codex', dir: '/p', session: { agent: 'codex', id: 's1' },
    configOptions: [], phase: 'idle', tail: emptyTail(), permissions: [], epoch: 0, ...fields,
  };
}

async function renderComposer(c, props = {}) {
  composerModule ??= await loadServerComponent(COMPOSER_URL, [
    ["import { chats } from '../../stores/chats.svelte';", fixtureStore('chats')],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
    ["'../../link'", lib('../../link.ts', COMPOSER_URL)],
    ["'../../manage'", lib('../../manage.ts', COMPOSER_URL)],
    ["'../../portable'", lib('../../portable.ts', COMPOSER_URL)],
  ]);
  return renderWithFixture(composerModule, { chats: {}, toast: {} }, { chat: c, ...props });
}

async function renderLiveTail(c, props = {}) {
  liveTailModule ??= await loadServerComponent(LIVE_TAIL_URL, [
    ["'../../chat'", lib('../../chat.ts', LIVE_TAIL_URL)],
    ["'../../portable'", lib('../../portable.ts', LIVE_TAIL_URL)],
    [
      "import MessageBubble from '../transcript/MessageBubble.svelte';",
      'const MessageBubble = ($$renderer, p) => $$renderer.push(`<div data-stub-message="${p.message.id}" data-role="${p.message.role}"></div>`);',
    ],
    [
      "import PermissionCard from './PermissionCard.svelte';",
      'const PermissionCard = ($$renderer, p) => $$renderer.push(`<div data-stub-permission="${p.permission.id}" data-agent="${p.agentName}"></div>`);',
    ],
    ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ]);
  return unescapeHTML(await renderWithFixture(liveTailModule, {}, { chat: c, sessionRef: c.session, ...props }));
}

const CARD_URL = new URL('../src/lib/components/chat/PermissionCard.svelte', import.meta.url);
const MISSING_URL = new URL('../src/lib/components/chat/AdapterMissing.svelte', import.meta.url);
let cardModule;
let missingModule;

async function renderCard(permission) {
  cardModule ??= await loadServerComponent(CARD_URL, [
    ["import { chats } from '../../stores/chats.svelte';", fixtureStore('chats')],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
    ["'../../chat'", lib('../../chat.ts', CARD_URL)],
    ["'../../link'", lib('../../link.ts', CARD_URL)],
    ["'../../manage'", lib('../../manage.ts', CARD_URL)],
    ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ]);
  return unescapeHTML(await renderWithFixture(cardModule, { chats: {}, toast: {} }, { chat: chat(), permission, agentName: 'Codex' }));
}

async function renderMissing(agent, host) {
  missingModule ??= await loadServerComponent(MISSING_URL, [
    ["'../../portable'", lib('../../portable.ts', MISSING_URL)],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
  ]);
  return unescapeHTML(await renderWithFixture(missingModule, { toast: {} }, { agent, host }));
}

test('the composer sends only when idle, cancels while working and dismisses once ended', async () => {
  const buttons = async (fields) => {
    const html = await renderComposer(chat(fields));
    return { html, buttons: renderedButtons(html).map((b) => `${b.text}${b.disabled ? ' (disabled)' : ''}`) };
  };

  const starting = await buttons({ phase: 'starting', chatId: '' });
  assert.deepEqual(starting.buttons, ['End chat (disabled)', 'Send (disabled)']);
  assert.match(starting.html, /<textarea[^>]*\sdisabled/);
  assert.match(starting.html, /placeholder="Starting Codex…"/);

  const idle = await buttons({ phase: 'idle' });
  assert.deepEqual(idle.buttons, ['End chat', 'Send (disabled)'], 'nothing written yet');
  assert.doesNotMatch(idle.html, /<textarea[^>]*\sdisabled/);
  assert.match(idle.html, /Enter sends, Shift\+Enter starts a new line/);

  const working = await buttons({ phase: 'working' });
  assert.deepEqual(working.buttons, ['End chat', 'Cancel', 'Send (disabled)']);

  const exited = await buttons({ phase: 'exited', exit: { chatId: 'c1', seq: 9, reason: 'closed' } });
  assert.deepEqual(exited.buttons, ['Dismiss']);
  const restart = await renderComposer(chat({ phase: 'exited', exit: { chatId: 'c1', seq: 9, reason: 'crashed' } }), { onRestart() {} });
  assert.deepEqual(renderedButtons(restart).map((b) => b.text), ['Dismiss', 'Restart']);
  assert.match(exited.html, /<textarea[^>]*\sdisabled/);
});

test('the live tail shows the streamed messages, then the chat state under them', async () => {
  const tail = apply(beginTurn(emptyTail(), { text: 'hi', sentAt: SENT }), chunk('agent_message_chunk', 'Hello', 'm1'));
  const working = await renderLiveTail(chat({
    phase: 'working',
    tail,
    permissions: [{ id: 'p1', toolCall: { toolCallId: 't1', title: 'Run make' }, options: [] }],
  }));
  assert.deepEqual([...working.matchAll(/data-stub-message="([^"]+)" data-role="(\w+)"/g)].map((m) => m[2]), ['user', 'assistant']);
  assert.match(working, /data-stub-permission="p1" data-agent="Codex"/, 'a card per waiting request');
  assert.match(working, /Codex waits for your answer\./);
  assert.match(await renderLiveTail(chat({ phase: 'working', tail })), /Codex is working…/);

  assert.match(await renderLiveTail(chat({ phase: 'starting', session: undefined })), /Starting Codex…/);
  assert.match(await renderLiveTail(chat({ phase: 'starting' })), /Opening the session in Codex…/, 'a resume');

  const idle = await renderLiveTail(chat({ phase: 'idle', stopReason: 'end_turn' }));
  assert.doesNotMatch(idle, /chat-notice|is working/, 'a normal end says nothing');
  assert.match(await renderLiveTail(chat({ phase: 'idle', stopReason: 'cancelled' })), /The turn was cancelled\./);
  assert.match(await renderLiveTail(chat({ phase: 'idle', error: 'Authentication required' })), /role="alert"[^>]*>[\s\S]*Authentication required/);
  const login = { phase: 'idle', error: 'Authentication required', authMethods: [{ id: 'oc', name: 'Log in', description: 'Run `opencode auth login`' }] };
  const auth = await renderLiveTail(chat(login), { onOpenTerminal() {} });
  assert.match(auth, /<strong>Log in<\/strong>: Run `opencode auth login`/);
  assert.match(auth, />Open a terminal in \/p<\/button>/);
  assert.doesNotMatch(await renderLiveTail(chat(login)), /Open a terminal/, 'only where a terminal can open');

  const crashed = await renderLiveTail(chat({
    phase: 'exited',
    exit: { chatId: 'c1', seq: 4, reason: 'crashed', error: 'exit status 1', stderrTail: 'panic: <oops>' },
  }));
  assert.match(crashed, /The agent stopped unexpectedly\. exit status 1/);
  assert.match(crashed, /<pre class="stderr[^"]*">panic: <oops><\/pre>/);
});

test('the session header says when a chat drives the session', async () => {
  const host = { link: { dataHost: undefined }, launcher: { canOpen: true } };
  const live = await renderSessionHeader({ ...host, chats: { forSession: () => chat() } });
  assert.match(live, /● Live in chat/);
  assert.doesNotMatch(live, /● Running/);

  const both = await renderSessionHeader({ chats: { forSession: () => undefined }, terminals: { tabFor: () => ({ id: 't1' }) }, ...host });
  assert.match(both, /● Running/);
  assert.doesNotMatch(both, /Live in chat/);
});

// --- Terminal and chat exclusivity -------------------------------------------

const OPENER_URL = new URL('../src/lib/stores/chatOpen.svelte.ts', import.meta.url);
let openerModule;

async function withOpener(fixture, fn) {
  openerModule ??= await loadServerModule(OPENER_URL, [
    ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
    ["'../link'", lib('../link.ts', OPENER_URL)],
    ["'../manage'", lib('../manage.ts', OPENER_URL)],
    ["'../chat'", lib('../chat.ts', OPENER_URL)],
    ...['appState', 'chats', 'connectionStore', 'link', 'terminals', 'toast'].map((name) => {
      const file = name === 'connectionStore' ? 'connection' : name;
      return [`import { ${name} } from './${file}.svelte';`, fixtureStore(name, '__openerFixture')];
    }),
  ]);
  const toasts = [];
  const calls = [];
  globalThis.__openerFixture = {
    appState: { selectSession: async (ref, record) => calls.push(['select', ref, record]) },
    chats: {
      agents: [{ agent: 'codex', available: true }],
      forSession: () => undefined,
      start: async (req) => {
        calls.push(['start', req]);
        return chat({ session: req.resume ?? { agent: req.agent, id: 'new' } });
      },
    },
    connectionStore: { canChat: true },
    link: { dataHost: '' },
    terminals: { tabFor: () => undefined, show: (id) => calls.push(['show', id]) },
    toast: { show: (t) => toasts.push(t) },
    ...fixture,
  };
  try {
    await fn({ opener: new openerModule.ChatOpener(), toasts, calls });
  } finally {
    delete globalThis.__openerFixture;
  }
}

const META = { ref: { agent: 'codex', id: 's1' }, cwd: '/p' };

test('a session a terminal tab runs does not open in a chat', async () => {
  await withOpener({ terminals: { tabFor: () => ({ id: 'tab-3' }), show: () => {} } }, async ({ opener, toasts, calls }) => {
    assert.equal(await opener.openSession(META), false);
    assert.deepEqual(calls, [], 'no chat started');
    assert.equal(toasts[0].title, 'This session runs in a terminal');
    let shown;
    globalThis.__openerFixture.terminals.show = (id) => (shown = id);
    toasts[0].actions[0].onSelect();
    assert.equal(shown, 'tab-3');
  });
});

test('a session a chat drives does not resume in a terminal, and a second open shows the chat', async () => {
  const driving = chat();
  await withOpener({ chats: { agents: [], forSession: (ref) => (ref.id === 's1' ? driving : undefined) } }, async ({ opener, toasts, calls }) => {
    assert.equal(opener.refuseTerminal({ agent: 'codex', id: 's2' }), false, 'another session');
    assert.equal(opener.refuseTerminal(META.ref), true);
    assert.equal(toasts[0].title, 'This session is open in a chat');
    toasts[0].actions[0].onSelect();
    assert.deepEqual(calls, [['select', META.ref, 'push']]);
    assert.equal(opener.focusRequest, 1);

    assert.equal(await opener.openSession(META), true, 'shows the chat that drives it');
    assert.equal(opener.focusRequest, 2);
    assert.equal(calls.length, 1, 'no second chat');
  });
});

test('opening starts a chat, a new one selects its session, and a failed start says why', async () => {
  await withOpener({}, async ({ opener, toasts, calls }) => {
    assert.equal(opener.canChat('codex'), true);
    assert.equal(opener.canChat('claude-code'), false, 'no adapter');

    assert.equal(await opener.openSession(META), true);
    assert.deepEqual(calls.shift(), ['start', { agent: 'codex', dir: '/p', resume: META.ref }]);

    assert.equal(await opener.startNew('codex', '/p'), true);
    assert.deepEqual(calls, [['start', { agent: 'codex', dir: '/p' }], ['select', { agent: 'codex', id: 'new' }, 'push']]);
    assert.equal(opener.focusRequest, 2);
    assert.equal(opener.opening, false);

    globalThis.__openerFixture.chats.start = async () => {
      throw new Error('codex-acp was not found');
    };
    assert.equal(await opener.startNew('codex', '/p'), false);
    assert.equal(toasts.at(-1).title, 'Could not start the chat');
    assert.match(toasts.at(-1).body, /codex-acp was not found/);
  });
  await withOpener({ connectionStore: { canChat: false } }, async ({ opener }) => {
    assert.equal(opener.canChat('codex'), false, 'not on this host');
  });
});

// --- Permission cards and missing adapters ----------------------------------

test('a permission card offers every option and chooses none', async () => {
  const html = await renderCard({
    id: 'p1',
    toolCall: { toolCallId: 't1', title: 'Run the tests', kind: 'execute', rawInput: { command: 'make <test>' } },
    options: [
      { optionId: 'a1', name: 'Allow', kind: 'allow_once' },
      { optionId: 'a2', name: 'Always allow', kind: 'allow_always' },
      { optionId: 'r1', name: 'Reject', kind: 'reject_once' },
    ],
  });
  assert.match(html, /Codex asks for permission/);
  assert.match(html, /Run the tests/);
  assert.match(html, /<span class="kind[^"]*">execute<\/span>/);
  assert.match(html, /<pre class="command[^"]*"><code>make <test><\/code><\/pre>/);
  const buttons = [...html.matchAll(/<button[^>]*>/g)].map((m) => m[0]);
  assert.equal(buttons.length, 3);
  for (const b of buttons) {
    assert.match(b, /type="button"/, 'no option submits a form');
    assert.doesNotMatch(b, /autofocus/, 'no option takes focus');
  }
  assert.deepEqual(renderedButtons(html).map((b) => b.text.replace(/<[^>]+>/g, '').replace(/\s+/g, ' ')), ['Allow Allow once', 'Always allow', 'Reject Reject once'], 'the kind shows unless the name says it');
  assert.match(html, /An “always” choice can last beyond this chat/);
  assert.match(buttons[1], /title="Can last beyond this chat/);

  const diff = await renderCard({
    id: 'p2',
    toolCall: { toolCallId: 't2', title: 'Edit a.txt', kind: 'edit', content: [{ type: 'diff', path: 'a.txt', oldText: 'one\n', newText: 'two\n' }] },
    options: [{ optionId: 'a1', name: 'Allow', kind: 'allow_once' }],
  });
  assert.match(diff, /aria-label="Changes to a.txt"/);
  assert.match(diff, /<span class="del[^"]*">-one<\/span>/);
  assert.match(diff, /<span class="add[^"]*">\+two<\/span>/);
  assert.doesNotMatch(diff, /“always” choice/);

  assert.match(await renderCard({ id: 'p3', toolCall: { toolCallId: 't3' }, options: [] }), /The request has no options\. Cancel the turn to answer it\./);
});

test('a missing adapter says where it is missing and how to install it', async () => {
  const html = await renderMissing(
    { agent: 'codex', name: 'Codex', available: false, install: 'npm install -g @agentclientprotocol/codex-acp', problem: 'codex-acp was not found on PATH' },
    'Ubuntu (WSL)',
  );
  assert.match(html, /<strong>Codex<\/strong> chats need its ACP adapter on Ubuntu \(WSL\)\./);
  assert.match(html, /codex-acp was not found on PATH/);
  assert.match(html, /To install it, run this on Ubuntu \(WSL\):/);
  assert.match(html, /<code[^>]*>npm install -g @agentclientprotocol\/codex-acp<\/code>/);
  assert.deepEqual(renderedButtons(html).map((b) => b.text), ['Copy']);
  assert.doesNotMatch(html, /config\.toml/);

  const override = await renderMissing({ agent: 'codex', name: 'Codex', available: false, override: true, install: 'npm i x', problem: '/opt/x is not executable' }, 'this computer');
  assert.match(override, /\[chat\.adapters\]<\/code> in config\.toml/);
});

test('restart continues a saved session, starts a new one otherwise, and leaves a running chat alone', async () => {
  await withOpener({}, async ({ opener, calls }) => {
    const dismissed = [];
    globalThis.__openerFixture.chats.dismiss = (key) => dismissed.push(key);
    const ended = chat({ key: 'old', phase: 'exited', exit: { chatId: 'c1', seq: 3, reason: 'crashed' } });

    assert.equal(await opener.restart(chat({ phase: 'idle' }), true), false, 'a running chat');
    assert.deepEqual(calls, []);

    assert.equal(await opener.restart(ended, true), true);
    assert.deepEqual(calls.splice(0), [['start', { agent: 'codex', dir: '/p', resume: { agent: 'codex', id: 's1' } }]]);
    assert.deepEqual(dismissed.splice(0), ['old'], 'the ended chat goes once the new one runs');

    assert.equal(await opener.restart(ended, false), true, 'the agent never saved the session');
    assert.deepEqual(calls.splice(0), [['start', { agent: 'codex', dir: '/p' }], ['select', { agent: 'codex', id: 'new' }, 'push']]);
    assert.deepEqual(dismissed.splice(0), ['old']);

    globalThis.__openerFixture.chats.start = async () => {
      throw new Error('boom');
    };
    assert.equal(await opener.restart(ended, true), false);
    assert.deepEqual(dismissed, [], 'a failed restart keeps the ended chat and its exit notice');
  });
  await withOpener({ terminals: { tabFor: () => ({ id: 'tab-1' }), show() {} } }, async ({ opener, toasts, calls }) => {
    const ended = chat({ phase: 'exited', exit: { chatId: 'c1', seq: 3, reason: 'closed' } });
    assert.equal(await opener.restart(ended, true), false);
    assert.equal(toasts[0].title, 'This session runs in a terminal');
    assert.deepEqual(calls, []);
  });
});
