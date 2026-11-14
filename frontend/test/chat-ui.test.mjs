import test from 'node:test';
import assert from 'node:assert/strict';
import { addAnswer, applySessionUpdate, beginTurn, emptyTail, endTurn } from '../src/lib/chatTail.ts';
import {
  addAttachments,
  canSendPrompt,
  chatAgentAvailable,
  commandChoices,
  endChatQuestion,
  isMcpListing,
  parseMcpStatus,
  isPluginListing,
  PLUGIN_COMMAND,
  pluginRows,
  MAX_CHAT_ATTACHMENTS,
  pendingChatMeta,
  permissionView,
  planMarkdown,
  queuedLabel,
  skippedToast,
  switchToChatQuestion,
  switchToTerminalQuestion,
  tailBlocks,
  tailMessages,
  toolStatus,
  unifiedDiff,
} from '../src/lib/chat.ts';
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

test("a question's outcome is a line of its own, and what the agent says after it a new message", () => {
  let tail = apply(beginTurn(emptyTail(), { text: 'add a setting', sentAt: SENT }), chunk('agent_message_chunk', 'One question first.', 'm1'));
  tail = addAnswer(tail, { questionId: 'q1', toolCallId: 't-ask', outcome: 'accept', summary: 'Answered: New module' });
  tail = addAnswer(tail, { questionId: 'q1', outcome: 'cancel', summary: 'The question was cancelled' });
  tail = apply(tail, chunk('agent_message_chunk', 'Going with a new module.', 'm2'));
  const blocks = tailBlocks(tail);
  assert.deepEqual(
    blocks.map((b) => (b.kind === 'answer' ? ['answer', b.answer.summary] : [b.message.role, b.message.parts.map((p) => p.text).join('')])),
    [
      ['user', 'add a setting'],
      ['assistant', 'One question first.'],
      ['answer', 'Answered: New module'],
      ['assistant', 'Going with a new module.'],
    ],
    "a question's second outcome is ignored",
  );
  assert.deepEqual(tailMessages(tail).map((m) => m.role), ['user', 'assistant', 'assistant'], 'messages leave the line out');
  const ids = blocks.filter((b) => b.kind === 'message').map((b) => b.message.id);
  assert.equal(new Set(ids).size, ids.length, 'IDs are unique');
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

test('commandChoices lists the commands a / or $ word could name, by full or short name', () => {
  const commands = [
    { name: 'mcp', description: 'MCP status', input: { hint: '[reconnect|enable|disable]' } },
    { name: 'ponytail:ponytail', description: '(ponytail) Lazy', input: { hint: '[lite|full]' } },
    { name: 'understand-anything:understand-chat', input: { hint: '<question>' } },
    { name: 'understand-anything:understand', input: null },
    { name: 'other:understand' },
    { name: '$sites:sites', description: 'Sites' },
  ];
  const texts = (t) => commandChoices(commands, t).map((c) => c.text);
  assert.deepEqual(texts('/'), ['/mcp', '/ponytail:ponytail ', '/understand-anything:understand-chat ', '/understand-anything:understand', '/other:understand']);
  // A short name two plugins share lists both; the exact full name comes first.
  assert.deepEqual(texts('/understand'), ['/understand-anything:understand-chat ', '/understand-anything:understand', '/other:understand']);
  assert.deepEqual(texts('/understand-anything:understand'), ['/understand-anything:understand', '/understand-anything:understand-chat ']);
  assert.deepEqual(texts('/PONY'), ['/ponytail:ponytail ']);
  assert.deepEqual(texts('$s'), ['$sites:sites']);
  // Not one command word: no list.
  for (const t of ['', 'hi /mcp', '/mcp ', '/ponytail full', ' /mcp', '/nope']) assert.deepEqual(texts(t), [], t);
  // /mcp's mutating arguments are not offered; other hints and descriptions are.
  assert.deepEqual(commandChoices(commands, '/mc'), [{ text: '/mcp', description: 'MCP status', hint: '' }]);
  assert.deepEqual(commandChoices(commands, '/ponytail:'), [{ text: '/ponytail:ponytail ', description: '(ponytail) Lazy', hint: '[lite|full]' }]);
  assert.deepEqual(commandChoices([], '/'), []);
});

test('only a bare /mcp the agent advertises is the captured listing', () => {
  const mcp = [{ name: 'mcp', input: { hint: '[reconnect|enable|disable [<server>|all]]' } }];
  assert.equal(isMcpListing('/mcp', mcp), true);
  assert.equal(isMcpListing(' /mcp\n', mcp), true);
  assert.equal(isMcpListing('/mcp reconnect docs', mcp), false, 'arguments change servers: an ordinary turn');
  assert.equal(isMcpListing('/mcp', []), false, 'an agent without /mcp gets the text as a prompt');
});

test("parseMcpStatus reads Claude's /mcp reply, scopes, escapes and code spans", () => {
  const claude = [
    '**MCP servers:** 5 (1 failed, 1 needs authentication, 2 connected, 1 disabled)',
    '**Failed**\n- `docs`: connect ECONNREFUSED 127.0.0.1:3000; 0 tools',
    '**Needs authentication**\n- `git``hub`',
    '**Connected**\n- `fetch` (user): 1 tool\n- `fetch` (project): a\\_b \\*c\\* \\<x\\> \\| y; 3 tools',
    '**Disabled:** `old`, `` `tick` ``',
    'Run `/mcp reconnect <server>` to reconnect one server, or `/mcp reconnect` to reconnect every server that is not connected and not disabled.',
  ].join('\n\n');
  assert.deepEqual(parseMcpStatus(claude), {
    summary: '5 (1 failed, 1 needs authentication, 2 connected, 1 disabled)',
    groups: [
      { title: 'Failed', servers: [{ name: 'docs', detail: 'connect ECONNREFUSED 127.0.0.1:3000; 0 tools' }] },
      { title: 'Needs authentication', servers: [{ name: 'git``hub', detail: '' }] },
      { title: 'Connected', servers: [{ name: 'fetch', scope: 'user', detail: '1 tool' }, { name: 'fetch', scope: 'project', detail: 'a_b *c* <x> | y; 3 tools' }] },
      { title: 'Disabled', servers: [{ name: 'old', detail: '' }, { name: '`tick`', detail: '' }] },
    ],
    notes: ['Run `/mcp reconnect <server>` to reconnect one server, or `/mcp reconnect` to reconnect every server that is not connected and not disabled.'],
  });
  assert.deepEqual(parseMcpStatus('No MCP servers are configured.'), { summary: 'No MCP servers are configured.', groups: [], notes: [] });
});

test("parseMcpStatus reads Codex's statuses and notes, and refuses other shapes", () => {
  const codex = [
    'Reloaded the MCP configuration.\nSome MCP servers are still starting. Run `/mcp` again later to see their status.',
    '',
    '**MCP servers:** 4 (1 cancelled, 1 not started, 1 unknown, 1 connected)',
    '',
    '**Cancelled**\n- `a`',
    '',
    '**Not started**\n- (unnamed): not signed in',
    '',
    '**Connected**\n- `b`: 2 tools; tool \\~list\\~ failed',
    '',
    '**Unknown**\n- `c`',
    '',
    'Run `/mcp reconnect` to restart the servers that are not connected.',
  ].join('\n');
  const st = parseMcpStatus(codex);
  assert.equal(st.summary, '4 (1 cancelled, 1 not started, 1 unknown, 1 connected)');
  assert.deepEqual(st.groups.map((g) => [g.title, g.servers.map((s) => `${s.name}|${s.detail}`)]), [
    ['Cancelled', ['a|']],
    ['Not started', ['(unnamed)|not signed in']],
    ['Connected', ['b|2 tools; tool ~list~ failed']],
    ['Unknown', ['c|']],
  ]);
  assert.equal(st.notes.length, 2);
  assert.match(st.notes[0], /^Reloaded/);

  for (const text of ['', 'Done. You said "/mcp"', '**Connected**\n- `a`', '**MCP servers:** 1 (1 connected)\n\n**Connected**\n- a', '**MCP servers:** 1\n\n**Disabled:** `a` `b`', '**MCP servers:** 1\n\n**MCP servers:** 2']) {
    assert.equal(parseMcpStatus(text), null, text);
  }
});

test('/plugin is the app\'s listing unless the agent has its own, and a plugin is loaded only by its commands', () => {
  assert.equal(isPluginListing('/plugin', []), true);
  assert.equal(isPluginListing(' /plugin\n', [{ name: 'mcp' }]), true);
  assert.equal(isPluginListing('/plugin list', []), false);
  assert.equal(isPluginListing('/plugins', []), false);
  assert.equal(isPluginListing('/plugin', [{ name: 'plugin' }]), false, 'the agent answers its own');
  assert.deepEqual(commandChoices([PLUGIN_COMMAND], '/plug').map((c) => c.text), ['/plugin']);

  const plugins = [
    { id: 'ponytail@ponytail', name: 'ponytail', enabled: true },
    { id: 'sites@m', name: 'sites', enabled: true },
    { id: 'gopls-lsp@m', name: 'gopls-lsp', enabled: false },
    { id: 'mcp@m', name: 'mcp', enabled: true },
    { id: 'pony@m', name: 'pony', enabled: true },
  ];
  const commands = [{ name: 'ponytail:ponytail' }, { name: '$sites:sites' }, { name: 'mcp:server:prompt' }, { name: 'gopls-lsp' }];
  assert.deepEqual(
    pluginRows(plugins, commands).map((p) => [p.name, p.loaded]),
    [['ponytail', true], ['sites', true], ['gopls-lsp', false], ['mcp', false], ['pony', false]],
  );
  assert.deepEqual(pluginRows([], commands), []);
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
    configOptions: [], phase: 'idle', tail: emptyTail(), permissions: [], questions: [], queue: [], epoch: 0, ...fields,
  };
}

const OPTIONS_URL = new URL('../src/lib/components/chat/ChatOptions.svelte', import.meta.url);

async function renderComposer(c, props = {}) {
  if (!globalThis.__chatOptions) {
    const options = await loadServerComponent(OPTIONS_URL, [["'../../chatOptions'", lib('../../chatOptions.ts', OPTIONS_URL)]]);
    globalThis.__chatOptions = options.default;
  }
  composerModule ??= await loadServerComponent(COMPOSER_URL, [
    ["import ChatOptions from './ChatOptions.svelte';", 'const ChatOptions = globalThis.__chatOptions;'],
    ["import { onMount } from 'svelte';", 'const onMount = () => {};'],
    ["import { api } from '../../api';", 'const api = {};'],
    ["'../../chat'", lib('../../chat.ts', COMPOSER_URL)],
    ["import AttachmentChips from './AttachmentChips.svelte';", 'const AttachmentChips = () => {};'],
    ["import PluginInventory from './PluginInventory.svelte';", 'const PluginInventory = () => {};'],
    ["import McpServersDialog from './McpServersDialog.svelte';", 'const McpServersDialog = () => {};'],
    [
      "import QueuedMessages from './QueuedMessages.svelte';",
      'const QueuedMessages = ($$renderer, p) => $$renderer.push(`<div data-stub-queue="${p.chat.queue.length}" data-empty="${p.composerEmpty}"></div>`);',
    ],
    ["import { chats } from '../../stores/chats.svelte';", fixtureStore('chats')],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
    ["import { filePrompt } from '../../stores/filePrompt.svelte';", 'const filePrompt = {};'],
    ["'../../link'", lib('../../link.ts', COMPOSER_URL)],
    ["'../../manage'", lib('../../manage.ts', COMPOSER_URL)],
    ["'../../portable'", lib('../../portable.ts', COMPOSER_URL)],
  ]);
  return renderWithFixture(composerModule, { chats: {}, toast: {} }, { chat: c, ...props });
}

async function renderLiveTail(c, props = {}) {
  liveTailModule ??= await loadServerComponent(LIVE_TAIL_URL, [
    ["'../../chat'", lib('../../chat.ts', LIVE_TAIL_URL)],
    ["'../../chatQuestions'", lib('../../chatQuestions.ts', LIVE_TAIL_URL)],
    ["'../../portable'", lib('../../portable.ts', LIVE_TAIL_URL)],
    [
      "import MessageBubble from '../transcript/MessageBubble.svelte';",
      'const MessageBubble = ($$renderer, p) => $$renderer.push(`<div data-stub-message="${p.message.id}" data-role="${p.message.role}"></div>`);',
    ],
    [
      "import PermissionCard from './PermissionCard.svelte';",
      'const PermissionCard = ($$renderer, p) => $$renderer.push(`<div data-stub-permission="${p.permission.id}" data-agent="${p.agentName}"></div>`);',
    ],
    [
      "import QuestionCard from './QuestionCard.svelte';",
      'const QuestionCard = ($$renderer, p) => $$renderer.push(`<div data-stub-question="${p.question.id}" data-agent="${p.agentName}"></div>`);',
    ],
    ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ]);
  return unescapeHTML(await renderWithFixture(liveTailModule, {}, { chat: c, sessionRef: c.session, ...props }));
}

const CARD_URL = new URL('../src/lib/components/chat/PermissionCard.svelte', import.meta.url);
let cardModule;

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

// A button's name: its text without the icon's markup.
const composerButtons = (html) => renderedButtons(html).map((b) => `${b.text.replace(/<[^>]*>/g, '').trim()}${b.disabled ? ' (disabled)' : ''}`);

test('the composer sends only when idle, stops a running turn and dismisses once ended', async () => {
  const buttons = async (fields) => {
    const html = await renderComposer(chat(fields));
    return { html, buttons: composerButtons(html) };
  };

  const starting = await buttons({ phase: 'starting', chatId: '' });
  assert.deepEqual(starting.buttons, ['End chat (disabled)', 'Send (disabled)']);
  assert.match(starting.html, /<textarea[^>]*\sdisabled/);
  assert.match(starting.html, /placeholder="Starting Codex…"/);

  const idle = await buttons({ phase: 'idle' });
  assert.deepEqual(idle.buttons, ['End chat', 'Send (disabled)'], 'nothing written yet');
  assert.doesNotMatch(idle.html, /<textarea[^>]*\sdisabled/);
  assert.match(idle.html, /placeholder="Message Codex…"/);
  // The key hint is for screen readers, tied to the text box.
  assert.match(idle.html, /<textarea[^>]*aria-describedby="chat-hint-chat-1"/);
  // Prompts are code and commands: no spellcheck, autofill, autocorrect or capitals.
  for (const attr of ['spellcheck="false"', 'autocomplete="off"', 'autocorrect="off"', 'autocapitalize="off"']) {
    assert.match(idle.html, new RegExp(`<textarea[^>]*\\s${attr}`), attr);
  }
  assert.match(idle.html, /id="chat-hint-chat-1" class="sr-only[^"]*">Enter sends, Shift\+Enter starts a new line/);
  // Icon buttons carry a name and a tooltip.
  assert.match(idle.html, /<button[^>]*title="End the chat and stop Codex"[^>]*>.*?<svg[^]*?End chat<\/span><\/button>/);
  assert.match(idle.html, /<button type="submit" class="round send[^"]*" title="Send \(Enter\)"/);

  const working = await buttons({ phase: 'working' });
  assert.deepEqual(working.buttons, ['End chat', 'Cancel'], 'the send button turns into stop');
  assert.match(working.html, /<button type="button" class="round stop[^"]*" title="Stop Codex's turn"/);

  const exited = await buttons({ phase: 'exited', exit: { chatId: 'c1', seq: 9, reason: 'closed' } });
  assert.deepEqual(exited.buttons, ['Dismiss']);
  const restart = await renderComposer(chat({ phase: 'exited', exit: { chatId: 'c1', seq: 9, reason: 'crashed' } }), { onRestart() {} });
  assert.deepEqual(composerButtons(restart), ['Dismiss', 'Restart']);
  assert.match(exited.html, /<textarea[^>]*\sdisabled/);
});

test('while the agent works a chat that queues takes a message for later, and shows its queue', async () => {
  const working = unescapeHTML(await renderComposer(chat({ phase: 'working', canQueue: true })));
  assert.match(working, /placeholder="Queue a message for Codex…"/);
  assert.doesNotMatch(working, /<textarea[^>]*\sdisabled/);
  assert.match(working, /id="chat-hint-chat-1" class="sr-only[^"]*">Enter queues the message until Codex finishes/);
  assert.deepEqual(composerButtons(working), ['End chat', 'Cancel'], 'nothing to queue yet: only Stop');
  assert.doesNotMatch(working, /data-stub-queue/, 'no queue to show');

  // An older host does not queue: the composer waits for the turn's end.
  const older = unescapeHTML(await renderComposer(chat({ phase: 'working' })));
  assert.match(older, /placeholder="Message Codex…"/);

  const queued = chat({ phase: 'working', canQueue: true, queue: [{ id: 'queued-1', text: 'next' }, { id: 'queued-2', text: 'after' }] });
  assert.match(await renderComposer(queued), /<div data-stub-queue="2" data-empty="true"><\/div>/);
  // A paused queue shows while idle; an ended chat drops it.
  assert.match(await renderComposer({ ...queued, phase: 'idle' }), /data-stub-queue="2"/);
  assert.doesNotMatch(await renderComposer({ ...queued, phase: 'exited', exit: { chatId: 'c1', seq: 9, reason: 'closed' } }), /data-stub-queue/);
});

test('the composer shows the context in use once the agent reports it', async () => {
  const none = await renderComposer(chat());
  assert.doesNotMatch(none, /data-chat-usage/, 'nothing before a usage_update');

  const html = unescapeHTML(await renderComposer(chat({ tail: { ...emptyTail(), usage: { used: 15385, size: 475000 } } })));
  assert.match(html, /data-chat-usage[^>]*title="Context: 15,385 of 475,000 tokens in use"/);
  assert.match(html, />\s*3%<span class="sr-only[^"]*"> of the context in use<\/span>/);
  const full = unescapeHTML(await renderComposer(chat({ tail: { ...emptyTail(), usage: { used: 300, size: 200 } } })));
  assert.match(full, />\s*100%</, 'never more than full');
  const ended = await renderComposer(chat({ phase: 'exited', exit: { chatId: 'c1', seq: 2, reason: 'closed' }, tail: { ...emptyTail(), usage: { used: 1, size: 2 } } }));
  assert.doesNotMatch(ended, /data-chat-usage/);
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
  // The agent's log, folded.
  assert.match(crashed, /<details class="exit-log[^"]*"><summary[^>]*>Show the agent's log<\/summary> <pre class="stderr[^"]*">panic: <oops><\/pre><\/details>/);
  assert.match(await renderLiveTail(chat({ phase: 'exited', exit: { chatId: 'c1', seq: 4, reason: 'exited', stderrTail: 'bye' } })), /Show the agent's log/);

  // A chat the user ended, or a dropped connection, says so in one line.
  for (const [reason, text] of [['closed', 'The chat ended.'], ['disconnected', 'The connection to the host dropped, which ended the chat.']]) {
    const html = await renderLiveTail(chat({ phase: 'exited', exit: { chatId: 'c1', seq: 4, reason, stderrTail: '[session/create] sessionId=x phase=settings' } }));
    assert.match(html, new RegExp(`<span>${text.replace(/\./g, '\\.')}</span>`), reason);
    assert.doesNotMatch(html, /session\/create|exit-log/, reason);
  }
});

test('the live tail shows question cards, waits on them, and shows how a question ended', async () => {
  const q = { id: 'q1', message: 'Pick one', fields: [] };
  const waiting = await renderLiveTail(chat({ phase: 'working', questions: [q] }));
  assert.match(waiting, /data-stub-question="q1" data-agent="Codex"/);
  assert.match(waiting, /Codex waits for your answer\./);

  // A late card, or an async question Codex keeps working through, does not hold the turn.
  for (const other of [{ ...q, late: true }, { ...q, meta: { codex: { delivery: 'async' } } }]) {
    const html = await renderLiveTail(chat({ phase: 'working', questions: [other] }));
    assert.match(html, /data-stub-question="q1"/);
    assert.match(html, /Codex is working…/);
  }

  const tail = addAnswer(beginTurn(emptyTail(), { text: 'go', sentAt: SENT }), { questionId: 'q1', outcome: 'accept', summary: 'Answered: <Extend> · Yes' });
  const done = await renderLiveTail(chat({ phase: 'working', tail }));
  assert.doesNotMatch(done, /data-stub-question/);
  assert.match(done, /Codex is working…/);
  assert.match(done, /data-answer-line="q1"[^>]*>[\s\S]*Answered: <Extend> · Yes/);
  const skipped = await renderLiveTail(chat({ tail: addAnswer(emptyTail(), { questionId: 'q2', outcome: 'decline', summary: 'Skipped the question' }) }));
  assert.match(skipped, /class="answer-line[^"]*"[^>]*data-answer-line="q2"/);
  assert.doesNotMatch(skipped, /answer-line answered/, 'only an answer is marked answered');
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

test('a new chat keeps the session header layout before its agent saves the session', async () => {
  const ref = { agent: 'claude-code', id: 'new-1' };
  const meta = pendingChatMeta(ref, { dir: '/home/me/proj', phase: 'idle' }, 'Claude Code');
  assert.equal(meta.title, 'New Claude Code chat');
  assert.equal(meta.live, true);
  assert.equal(pendingChatMeta(ref, { dir: '/x', phase: 'exited' }, 'Claude Code').live, false);

  const html = unescapeHTML(await renderSessionHeader(
    { link: { dataHost: undefined }, launcher: { canOpen: true }, chats: { forSession: () => chat() } },
    { meta, pending: true, onFind() {} },
  ));
  for (const part of ['header-top-row', 'header-title-row', 'header-meta-row', 'aria-label="Transcript display"', 'aria-label="Find in session"', '● Live in chat']) {
    assert.ok(html.includes(part), `the pending header has ${part}`);
  }
  assert.match(html, /<h2 class="title-text[^"]*"[^>]*>\s*New Claude Code chat/);
  assert.match(html, /class="cwd-text[^"]*">\/home\/me\/proj</);
  assert.match(html, /class="action-btn resume-btn[^"]*" title="Available once the agent saves this session" disabled[^>]*>Resume/);
  assert.doesNotMatch(html, /More ways to resume/);
  assert.match(html, /more-btn[^"]*"[^>]*disabled/);
  assert.doesNotMatch(html, /msgs|Token use|header-time-row/, 'no counts, tokens or times before the session is saved');

  const saved = await renderSessionHeader({ link: { dataHost: undefined }, launcher: { canOpen: true } });
  assert.match(saved, /msgs/);
  assert.doesNotMatch(saved, /more-btn[^"]*"[^>]*disabled/);
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
    ["'../chatOptions'", lib('../chatOptions.ts', OPENER_URL)],
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
    assert.deepEqual(calls.shift(), ['start', { agent: 'codex', dir: '/p', resume: META.ref, questions: true }], 'the window shows questions');

    assert.equal(await opener.startNew('codex', '/p'), true);
    assert.deepEqual(calls, [['start', { agent: 'codex', dir: '/p', questions: true }], ['select', { agent: 'codex', id: 'new' }, 'push']]);
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

test('a session Codex still holds says so and offers to try again', async () => {
  await withOpener({}, async ({ opener, toasts, calls }) => {
    const start = globalThis.__openerFixture.chats.start;
    let refusals = 1;
    globalThis.__openerFixture.chats.start = async (req) => {
      if (refusals-- > 0) {
        // As engine.ErrSessionInUse reads locally and after crossing rpc.
        throw new Error('the session is open in another Codex client: its CLI, app or IDE extension, or one that closed it in the last minute. Close it there, or try again in a minute');
      }
      return start(req);
    };
    assert.equal(await opener.openSession(META), false);
    const shown = toasts.at(-1);
    assert.equal(shown.title, 'Codex is still using this session');
    assert.match(shown.body, /about a minute/);
    assert.equal(shown.actions[0].label, 'Try again');
    assert.equal(opener.focusRequest, 0);

    shown.actions[0].onSelect();
    await new Promise((resolve) => setTimeout(resolve, 0));
    assert.deepEqual(calls, [['start', { agent: 'codex', dir: '/p', resume: META.ref, questions: true }]], 'the same session again');
    assert.equal(opener.focusRequest, 1);
    assert.equal(toasts.length, 1);
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
  assert.match(html, /<pre class="command[^"]*"><code[^>]*>make <test><\/code><\/pre>/);
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

test('restart continues a saved session, starts a new one otherwise, and leaves a running chat alone', async () => {
  await withOpener({}, async ({ opener, calls }) => {
    const dismissed = [];
    globalThis.__openerFixture.chats.dismiss = (key) => dismissed.push(key);
    const ended = chat({ key: 'old', phase: 'exited', exit: { chatId: 'c1', seq: 3, reason: 'crashed' } });

    assert.equal(await opener.restart(chat({ phase: 'idle' }), true), false, 'a running chat');
    assert.deepEqual(calls, []);

    assert.equal(await opener.restart(ended, true), true);
    assert.deepEqual(calls.splice(0), [['start', { agent: 'codex', dir: '/p', resume: { agent: 'codex', id: 's1' }, questions: true }]]);
    assert.deepEqual(dismissed.splice(0), ['old'], 'the ended chat goes once the new one runs');

    assert.equal(await opener.restart(ended, false), true, 'the agent never saved the session');
    assert.deepEqual(calls.splice(0), [['start', { agent: 'codex', dir: '/p', questions: true }], ['select', { agent: 'codex', id: 'new' }, 'push']]);
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

// --- Session options -----------------------------------------------------------

const OPTIONS = [
  { id: 'mode', name: 'Mode', category: 'mode', type: 'select', currentValue: 'agent', options: [
    { value: 'read-only', name: 'Read Only' }, { value: 'agent', name: 'Agent' }, { value: 'agent-full-access', name: 'Full Access' },
  ] },
  { id: 'model', name: 'Model', category: 'model', type: 'select', currentValue: 'gpt-b', options: [
    { group: 'openai', name: 'OpenAI', options: [{ value: 'gpt-a', name: 'GPT A' }, { value: 'gpt-b', name: 'GPT B' }] },
  ] },
];
// A picker is the shared dropdown: it shows the chosen value, and its name is its tooltip.
const dropdowns = (html) =>
  [...html.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)]
    .filter(([, attrs]) => attrs.includes('aria-haspopup="listbox"'))
    .map(([, attrs, body]) => ({
      name: attrs.match(/aria-label="([^"]*)"/)?.[1],
      title: attrs.match(/title="([^"]*)"/)?.[1],
      text: body.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]*>/g, '').trim(),
      disabled: /\sdisabled(?:=|\s|$)/.test(attrs),
    }));

test('the composer shows a picker per option, on the value the agent reports', async () => {
  const onOption = async () => {};
  const html = unescapeHTML(await renderComposer(chat({ configOptions: OPTIONS }), { onOption }));
  assert.match(html, /data-chat-options/);
  assert.doesNotMatch(html, /<select\b/, 'native selects are not the convention');
  assert.deepEqual(dropdowns(html).map((d) => [d.name, d.title, d.text, d.disabled]), [
    ['Mode', 'Mode', 'Agent', false],
    ['Model', 'Model', 'GPT B', false],
  ]);
  assert.doesNotMatch(html, /runs tools without asking/);
  // A picker shows only its value; the option's name is its name and tooltip.
  assert.doesNotMatch(html, /picker-name/);

  // The agent reports another value: the picker follows it.
  const moved = chat({ configOptions: [{ ...OPTIONS[0], currentValue: 'agent-full-access' }, { ...OPTIONS[1], currentValue: 'gpt-a' }] });
  const after = unescapeHTML(await renderComposer(moved, { onOption }));
  assert.deepEqual(dropdowns(after).map((d) => d.text), ['Full Access', 'GPT A']);
  assert.match(after, /Mode: the agent runs tools without asking/);

  // The agent reports no value yet: the picker says so and still offers the choices.
  const unset = unescapeHTML(await renderComposer(chat({ configOptions: [{ ...OPTIONS[0], currentValue: undefined }] }), { onOption }));
  assert.deepEqual(dropdowns(unset).map((d) => d.text), ['Not set']);

  const starting = unescapeHTML(await renderComposer(chat({ phase: 'starting', configOptions: OPTIONS }), { onOption }));
  assert.equal(dropdowns(starting).filter((d) => d.disabled).length, 2, 'no change while the agent starts');
  assert.doesNotMatch(await renderComposer(chat({ phase: 'exited', configOptions: OPTIONS, exit: { chatId: 'c1', seq: 2, reason: 'closed' } }), { onOption }), /data-chat-options/);
  assert.doesNotMatch(await renderComposer(chat({ configOptions: OPTIONS })), /data-chat-options/, 'no pickers without a setter');
  assert.doesNotMatch(await renderComposer(chat(), { onOption }), /data-chat-options/, 'no options');
});

function withStorage() {
  const data = new Map();
  globalThis.localStorage = { getItem: (k) => (data.has(k) ? data.get(k) : null), setItem: (k, v) => data.set(k, String(v)), removeItem: (k) => data.delete(k) };
  return () => delete globalThis.localStorage;
}

test('a chosen option is set and remembered for the next new chat of the agent on this host', async () => {
  const restore = withStorage();
  try {
    const sets = [];
    const optionsAfter = (id, value) => OPTIONS.map((o) => (o.id === id ? { ...o, currentValue: value } : o));
    const chatsFixture = {
      agents: [{ agent: 'codex', available: true }],
      forSession: () => undefined,
      start: async (req) => chat({ session: { agent: req.agent, id: 'new' }, configOptions: OPTIONS }),
      setOption: async (chatId, id, value) => {
        sets.push([chatId, id, value]);
        if (value === 'refused') throw new Error('the agent refused');
        return optionsAfter(id, value);
      },
    };
    await withOpener({ chats: chatsFixture }, async ({ opener, toasts }) => {
      // Nothing remembered: a new chat keeps the agent's options.
      assert.equal(await opener.startNew('codex', '/p'), true);
      assert.deepEqual(sets.splice(0), []);

      const c = chat({ configOptions: OPTIONS });
      await opener.choose(c, 'model', 'gpt-a');
      await opener.choose(c, 'mode', 'read-only');
      assert.deepEqual(sets.splice(0), [['c1', 'model', 'gpt-a'], ['c1', 'mode', 'read-only']]);
      await opener.choose(c, 'model', 'refused');
      assert.equal(toasts.splice(0)[0].title, 'Could not change the option');
      sets.splice(0);

      // The next new chat gets them before its first prompt, in the agent's order.
      assert.equal(await opener.startNew('codex', '/p'), true);
      assert.deepEqual(sets.splice(0), [['c1', 'mode', 'read-only'], ['c1', 'model', 'gpt-a']]);
      // A continued session keeps its own.
      assert.equal(await opener.openSession(META), true);
      assert.deepEqual(sets.splice(0), []);

      // Full access is set when picked, but never remembered: the next chat
      // starts with the agent's own mode.
      await opener.choose(c, 'mode', 'agent-full-access');
      assert.deepEqual(sets.splice(0), [['c1', 'mode', 'agent-full-access']]);
      assert.equal(await opener.startNew('codex', '/p'), true);
      assert.deepEqual(sets.splice(0), [['c1', 'model', 'gpt-a']]);
    });

    // Another host has its own memory.
    await withOpener({ chats: chatsFixture, link: { dataHost: 'dev-box' } }, async ({ opener }) => {
      assert.equal(await opener.startNew('codex', '/p'), true);
      assert.deepEqual(sets.splice(0), []);
    });

    // A remembered choice the agent refuses is skipped, and the chat still starts.
    await withOpener({ chats: { ...chatsFixture, setOption: async () => { throw new Error('no such model'); } } }, async ({ opener, toasts }) => {
      assert.equal(await opener.startNew('codex', '/p'), true);
      assert.equal(toasts[0].title, 'Could not apply your Model choice');
    });
  } finally {
    restore();
  }
});

// --- Attachments -----------------------------------------------------------

const image = (name) => ({ kind: 'image', name, mimeType: 'image/png', size: 70, data: 'iVBORw0KGgo=' });
const textFile = (name) => ({ kind: 'text', name, mimeType: 'text/plain', size: 5, text: 'hello', uri: `file:///p/${name}` });

test('a prompt with files shows them in the live user message, an image from its bytes', () => {
  const tail = beginTurn(emptyTail(), {
    text: '',
    sentAt: SENT,
    attachments: [{ name: 'shot.png', mimeType: 'image/png', size: 70, data: 'iVBORw0KGgo=' }, { name: 'notes.txt', mimeType: 'text/plain', size: 5 }],
  });
  const [user] = tailMessages(tail);
  assert.equal(user.role, 'user');
  assert.deepEqual(user.parts, [
    { kind: 'file', file: { name: 'shot.png', mime: 'image/png', size: 70, data: 'iVBORw0KGgo=' } },
    { kind: 'file', file: { name: 'notes.txt', mime: 'text/plain', size: 5 } },
  ], 'no empty text part');
  const [withText] = tailMessages(beginTurn(emptyTail(), { text: 'look', sentAt: SENT, attachments: [{ name: 'a.txt', mimeType: 'text/plain', size: 1 }] }));
  assert.deepEqual(withText.parts.map((p) => p.kind), ['text', 'file']);
});

test('files are added in order, skipping images the agent does not take and those past the limit', () => {
  const agent = { name: 'Codex', images: false };
  let r = addAttachments([textFile('a.txt')], { attachments: [image('shot.png'), textFile('b.txt')], skipped: [{ name: 'app.bin', reason: 'not a text file' }] }, agent);
  assert.deepEqual(r.attachments.map((a) => a.name), ['a.txt', 'b.txt']);
  assert.deepEqual(r.skipped, [{ name: 'app.bin', reason: 'not a text file' }, { name: 'shot.png', reason: 'Codex does not take images' }]);

  const nine = Array.from({ length: MAX_CHAT_ATTACHMENTS - 1 }, (_, i) => textFile(`f${i}.txt`));
  r = addAttachments(nine, { attachments: [image('x.png'), textFile('y.txt')], skipped: [] }, { name: 'Claude', images: true });
  assert.equal(r.attachments.length, MAX_CHAT_ATTACHMENTS);
  assert.equal(r.attachments.at(-1).name, 'x.png');
  assert.deepEqual(r.skipped, [{ name: 'y.txt', reason: 'a message can carry at most 10 attachments' }]);
});

test('the toast for skipped files names one file and its reason, or counts several', () => {
  assert.equal(skippedToast([]), null);
  assert.deepEqual(skippedToast([{ name: 'app.bin', reason: 'is not a text file' }]), { title: 'Could not attach app.bin', body: 'Is not a text file' });
  assert.deepEqual(skippedToast([{ name: 'a', reason: 'x' }, { name: 'b', reason: 'y' }]), { title: 'Could not attach 2 files', body: 'a: x; b: y' });
});

test('a prompt of only files can be sent; nothing at all cannot', () => {
  assert.equal(canSendPrompt('idle', false, '', 1), true);
  assert.equal(canSendPrompt('idle', false, '  \n', 0), false);
  assert.equal(canSendPrompt('idle', false, 'hi', 0), true);
  assert.equal(canSendPrompt('working', false, 'hi', 1), false);
  assert.equal(canSendPrompt('idle', true, 'hi', 1), false);
  // A chat that queues takes a message while it works, not while it starts.
  assert.equal(canSendPrompt('working', false, 'hi', 0, true), true);
  assert.equal(canSendPrompt('working', false, ' ', 0, true), false);
  assert.equal(canSendPrompt('working', true, 'hi', 0, true), false);
  assert.equal(canSendPrompt('starting', false, 'hi', 0, true), false);
});

test('a queued message shows as its first line, or its files, with a count of its files', () => {
  assert.deepEqual(queuedLabel({ id: 'q', text: '  fix the build\nthen the tests', attachments: [{ kind: 'image', name: 'a.png' }, { kind: 'text', name: 'b.md' }] }), {
    line: 'fix the build',
    full: 'fix the build\nthen the tests',
    files: '2 files',
  });
  assert.deepEqual(queuedLabel({ id: 'q', text: 'hi', attachments: [{ kind: 'text', name: 'b.md' }] }).files, '1 file');
  assert.deepEqual(queuedLabel({ id: 'q', attachments: [{ kind: 'image', name: 'a.png' }, { kind: 'text', name: 'b.md' }] }), { line: 'a.png, b.md', full: 'a.png, b.md', files: '' });
});

test('the attach button shows when the agent says what a prompt may carry, and the card takes dropped files', async () => {
  const caps = { image: true, embeddedContext: true };
  const idle = await renderComposer(chat({ promptCapabilities: caps }));
  assert.deepEqual(composerButtons(idle), ['Attach images or files', 'End chat', 'Send (disabled)']);
  assert.match(idle, /<form[^>]*data-file-drop-target="chat"/);

  const starting = await renderComposer(chat({ phase: 'starting', promptCapabilities: caps }));
  assert.deepEqual(composerButtons(starting), ['Attach images or files (disabled)', 'End chat (disabled)', 'Send (disabled)']);
  assert.doesNotMatch(starting, /data-file-drop-target/);

  // An agent (or an older host) that does not say: text only.
  const unknown = await renderComposer(chat());
  assert.deepEqual(composerButtons(unknown), ['End chat', 'Send (disabled)']);
  assert.doesNotMatch(unknown, /data-file-drop-target/);

  const ended = await renderComposer(chat({ phase: 'exited', promptCapabilities: caps }));
  assert.doesNotMatch(ended, /Attach images or files/);
  assert.doesNotMatch(ended, /data-file-drop-target/);
});

const CHIPS_URL = new URL('../src/lib/components/chat/AttachmentChips.svelte', import.meta.url);
let chipsModule;

test('attached files show as chips: a thumbnail or an icon, the name, and a remove button', async () => {
  chipsModule ??= await loadServerComponent(CHIPS_URL, [["'../../format'", lib('../../format.ts', CHIPS_URL)]]);
  const html = unescapeHTML(await renderWithFixture(chipsModule, {}, { attachments: [image('shot.png'), textFile('notes.txt')], onRemove: () => {} }));
  assert.match(html, /<ul class="chips[^"]*" aria-label="Attached files">/);
  assert.match(html, /<img src="data:image\/png;base64,iVBORw0KGgo="/);
  assert.equal([...html.matchAll(/<img /g)].length, 1, 'a text file has no thumbnail');
  assert.match(html, /title="notes\.txt \(5 B\)"/);
  assert.deepEqual(composerButtons(html), ['Remove shot.png', 'Remove notes.txt']);
});

test('End chat asks while the agent works, and ends an idle chat at once', () => {
  assert.equal(endChatQuestion('idle', 'Codex'), null);
  assert.equal(endChatQuestion('starting', 'Codex'), null);
  assert.deepEqual(endChatQuestion('working', 'Codex'), {
    title: 'End the chat?',
    message: 'Codex is still working. Ending the chat stops its turn.',
    choices: [{ value: 'end', label: 'End chat', danger: true }],
  });
});

test('End chat and Switch to terminal ask when messages are queued, and say they will not be sent', () => {
  assert.deepEqual(endChatQuestion('idle', 'Codex', 1), {
    title: 'End the chat?',
    message: '1 queued message will not be sent.',
    choices: [{ value: 'end', label: 'End chat', danger: true }],
  });
  assert.equal(endChatQuestion('working', 'Codex', 2).message, 'Codex is still working. Ending the chat stops its turn. 2 queued messages will not be sent.');
  assert.equal(switchToTerminalQuestion('idle', 'Codex', 2).message, '2 queued messages will not be sent.');
  assert.equal(switchToTerminalQuestion('working', 'Codex', 1).message, 'Codex is still working. Switching stops its turn. 1 queued message will not be sent.');
  assert.equal(endChatQuestion('exited', 'Codex', 2), null);
  assert.equal(endChatQuestion('idle', 'Codex', 0), null);
});

test('Switch to terminal asks while the agent works, worded for switching', () => {
  assert.equal(switchToTerminalQuestion('idle', 'Codex'), null);
  assert.equal(switchToTerminalQuestion('starting', 'Codex'), null);
  assert.deepEqual(switchToTerminalQuestion('working', 'Codex'), {
    title: 'Switch to terminal?',
    message: 'Codex is still working. Switching stops its turn.',
    choices: [{ value: 'switch', label: 'Switch', danger: true }],
  });
});

test('Switch to chat asks while a tab works or waits for an answer', () => {
  assert.deepEqual(switchToChatQuestion('working', 'Codex'), {
    title: 'Switch to chat?',
    message: 'Codex is still working. Switching stops its turn.',
    choices: [{ value: 'switch', label: 'Switch', danger: true }],
  });
  assert.deepEqual(switchToChatQuestion('needs', 'Codex'), {
    title: 'Switch to chat?',
    message: 'Codex is waiting for your answer. Switching discards it.',
    choices: [{ value: 'switch', label: 'Switch', danger: true }],
  });
  assert.equal(switchToChatQuestion('idle', 'Codex'), null, 'nothing runs');
  assert.equal(switchToChatQuestion('done', 'Codex'), null, 'its turn already finished');
  assert.equal(switchToChatQuestion('exited', 'Codex'), null, 'the agent is gone');
  assert.equal(switchToChatQuestion('', 'Codex'), null, 'nothing known says nothing runs');
});
