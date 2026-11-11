import test from 'node:test';
import assert from 'node:assert/strict';
import { applySessionUpdate, beginTurn, emptyTail, endTurn } from '../src/lib/chatTail.ts';
import { chatAgentAvailable, planMarkdown, tailMessages, toolStatus, unifiedDiff } from '../src/lib/chat.ts';
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

async function renderComposer(c) {
  composerModule ??= await loadServerComponent(COMPOSER_URL, [
    ["import { chats } from '../../stores/chats.svelte';", fixtureStore('chats')],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
    ["'../../link'", lib('../../link.ts', COMPOSER_URL)],
    ["'../../manage'", lib('../../manage.ts', COMPOSER_URL)],
    ["'../../portable'", lib('../../portable.ts', COMPOSER_URL)],
  ]);
  return renderWithFixture(composerModule, { chats: {}, toast: {} }, { chat: c });
}

async function renderLiveTail(c) {
  liveTailModule ??= await loadServerComponent(LIVE_TAIL_URL, [
    ["'../../chat'", lib('../../chat.ts', LIVE_TAIL_URL)],
    ["'../../portable'", lib('../../portable.ts', LIVE_TAIL_URL)],
    [
      "import MessageBubble from '../transcript/MessageBubble.svelte';",
      'const MessageBubble = ($$renderer, p) => $$renderer.push(`<div data-stub-message="${p.message.id}" data-role="${p.message.role}"></div>`);',
    ],
    ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ]);
  return unescapeHTML(await renderWithFixture(liveTailModule, {}, { chat: c, sessionRef: c.session }));
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
  assert.match(working, /Codex asks for permission: Run make/);
  assert.match(working, /Codex is working…/);

  assert.match(await renderLiveTail(chat({ phase: 'starting', session: undefined })), /Starting Codex…/);
  assert.match(await renderLiveTail(chat({ phase: 'starting' })), /Opening the session in Codex…/, 'a resume');

  const idle = await renderLiveTail(chat({ phase: 'idle', stopReason: 'end_turn' }));
  assert.doesNotMatch(idle, /chat-notice|is working/, 'a normal end says nothing');
  assert.match(await renderLiveTail(chat({ phase: 'idle', stopReason: 'cancelled' })), /The turn was cancelled\./);
  assert.match(await renderLiveTail(chat({ phase: 'idle', error: 'Authentication required' })), /role="alert"[^>]*>[\s\S]*Authentication required/);

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
