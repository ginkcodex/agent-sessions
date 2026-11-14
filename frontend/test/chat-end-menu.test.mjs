// The composer's End chat menu in a DOM: the two items, End the chat as
// always, and Switch to terminal, which stops the chat and continues the
// session in a terminal tab. The queued messages above the composer, with
// Send now, Edit and Remove. Plus the opener's switch sequences, either
// direction: to a terminal, and from a terminal pane to a chat. And the
// command list a / word opens, which inserts and never sends by itself.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });

const svelte = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const { proxy } = await import(new URL('../node_modules/svelte/src/internal/client/index.js', import.meta.url).href);
const lib = (rel, base) => JSON.stringify(new URL(rel, base).href);

async function loadComponent(name) {
  const url = new URL(`../src/lib/components/chat/${name}.svelte`, import.meta.url);
  const filename = fileURLToPath(url);
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  code = code
    .replace("import { onMount } from 'svelte';", 'const onMount = () => {};')
    .replace("'svelte/internal/disclose-version'", svelte('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", svelte('internal/client/index.js'))
    .replace("from 'svelte';", `from ${svelte('index-client.js')};`)
    .replace("import { api } from '../../api';", 'const api = {};')
    .replace("'../../chat'", lib('../../chat.ts', url))
    .replace("import AttachmentChips from './AttachmentChips.svelte';", 'const AttachmentChips = () => {};')
    .replace("import ChatOptions from './ChatOptions.svelte';", 'const ChatOptions = () => {};')
    .replace("import QueuedMessages from './QueuedMessages.svelte';", 'const QueuedMessages = globalThis.__endMenu.QueuedMessages;')
    .replace("import Icon from '../common/Icon.svelte';", 'const Icon = () => {};')
    .replace("import { chats } from '../../stores/chats.svelte';", 'const chats = globalThis.__endMenu.chats;')
    .replace("import { toast } from '../../stores/toast.svelte';", 'const toast = globalThis.__endMenu.toast;')
    .replace("import { filePrompt } from '../../stores/filePrompt.svelte';", 'const filePrompt = globalThis.__endMenu.filePrompt;')
    .replace("'../../link'", lib('../../link.ts', url))
    .replace("'../../manage'", lib('../../manage.ts', url))
    .replace("'../../portable'", lib('../../portable.ts', url));
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}

const calls = [];
let ask = async () => 'cancel';
globalThis.__endMenu = {
  chats: {
    close: async (chatId) => calls.push(['close', chatId]),
    dismiss: (key) => calls.push(['dismiss', key]),
    prompt: async (chatId, text) => calls.push(['prompt', chatId, text]),
    sendNow: async (chatId, id) => calls.push(['sendNow', chatId, id]),
    unqueue: async (chatId, id) => {
      calls.push(['unqueue', chatId, id]);
      return { text: `the text of ${id}`, attachments: [{ kind: 'text', name: 'plan.md', mimeType: 'text/plain', size: 4, text: 'plan' }] };
    },
  },
  toast: { show: (t) => calls.push(['toast', t.title]) },
  filePrompt: {
    ask: async (title, message, choices) => {
      calls.push(['ask', title, choices.map((c) => c.label)]);
      return ask();
    },
  },
};
globalThis.__endMenu.QueuedMessages = await loadComponent('QueuedMessages');
const Composer = await loadComponent('ChatComposer');

// Lets every pending promise chain run before an assertion.
const settle = () => new Promise((resolve) => setImmediate(resolve));

const chat = (fields = {}) => ({
  key: 'chat-1', chatId: 'c1', agent: 'codex', dir: '/p', session: { agent: 'codex', id: 's1' },
  configOptions: [], phase: 'idle', tail: { items: [], usage: null }, permissions: [], questions: [], queue: [], epoch: 0,
  promptCapabilities: { image: true }, ...fields,
});

async function openComposer(fields, props = {}) {
  const target = document.createElement('div');
  document.body.appendChild(target);
  const state = proxy({ chat: chat(fields), onSwitchToTerminal: props.onSwitchToTerminal, sessionSaved: props.sessionSaved ?? true, onRestart: props.onRestart });
  const component = mount(Composer, { target, props: state });
  flushSync();
  const text = (el) => el.textContent.trim().replace(/\s+/g, ' ');
  const trigger = () => target.querySelector('[aria-haspopup="menu"]');
  const items = () => [...target.querySelectorAll('[role="menu"] [role="menuitem"]')].map(text);
  const disabledItems = () => [...target.querySelectorAll('[role="menu"] [role="menuitem"]:disabled')].map(text);
  const clickItem = (name) => {
    const b = [...target.querySelectorAll('[role="menuitem"]')].find((el) => text(el) === name);
    assert.ok(b, `a menu item ${name}`);
    b.click();
    flushSync();
  };
  const openMenu = () => {
    trigger().click();
    flushSync();
  };
  const escape = () => {
    window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    flushSync();
  };
  const outside = () => {
    document.body.dispatchEvent(new window.Event('pointerdown', { bubbles: true }));
    flushSync();
  };
  const close = () => {
    unmount(component);
    target.remove();
    calls.length = 0;
    ask = async () => 'cancel';
  };
  return { target, trigger, items, disabledItems, clickItem, openMenu, escape, outside, close };
}

test('the power button opens a menu with the two items, Switch to terminal enabled only for a saved session', async () => {
  const both = await openComposer({ phase: 'idle' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  assert.deepEqual(both.items(), [], 'closed at first');
  both.openMenu();
  assert.deepEqual(both.items(), ['End the chat', 'Switch to terminal']);
  assert.deepEqual(both.disabledItems(), []);
  both.close();

  const noId = await openComposer({ phase: 'idle', session: undefined }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  noId.openMenu();
  assert.deepEqual(noId.items(), ['End the chat', 'Switch to terminal']);
  assert.deepEqual(noId.disabledItems(), ['Switch to terminal'], 'no session to resume');
  noId.close();

  // A new chat: the agent named a session ID, but saves nothing before the
  // first turn, so the index does not know it yet.
  const unsaved = await openComposer({ phase: 'idle' }, { onSwitchToTerminal: async () => calls.push(['switch']), sessionSaved: false });
  unsaved.openMenu();
  assert.deepEqual(unsaved.disabledItems(), ['Switch to terminal'], 'nothing to resume yet');
  assert.match(unsaved.target.querySelector('[role="menuitem"]:disabled').title, /once the agent saves this session/);
  unsaved.clickItem('Switch to terminal');
  await settle();
  assert.deepEqual(calls, [], 'a disabled item does nothing');
  unsaved.close();

  const alone = await openComposer({ phase: 'idle' });
  alone.openMenu();
  assert.deepEqual(alone.items(), ['End the chat'], 'no switch consumer');
  alone.close();

  const starting = await openComposer({ phase: 'starting', chatId: '' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  assert.equal(starting.trigger().disabled, true, 'no menu before the chat starts');
  starting.close();
});

test('Escape and an outside click close the menu without acting', async () => {
  const c = await openComposer({ phase: 'idle' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  c.openMenu();
  c.escape();
  assert.deepEqual(c.items(), [], 'Escape closes');
  assert.equal(document.activeElement === c.trigger(), true, 'the focus returns to the trigger');
  assert.deepEqual(calls, []);

  c.openMenu();
  c.outside();
  assert.deepEqual(c.items(), [], 'an outside click closes');
  assert.deepEqual(calls, []);
  c.close();
});

test('End the chat ends it as always: at once when idle, asked while working', async () => {
  const idle = await openComposer({ phase: 'idle' });
  idle.openMenu();
  idle.clickItem('End the chat');
  await settle();
  assert.deepEqual(calls, [['close', 'c1']]);
  idle.close();

  const working = await openComposer({ phase: 'working' });
  ask = async () => 'cancel';
  working.openMenu();
  working.clickItem('End the chat');
  await settle();
  assert.deepEqual(calls, [['ask', 'End the chat?', ['End chat']]], 'cancelling keeps the chat');
  working.close();

  const done = await openComposer({ phase: 'working' });
  ask = async () => 'end';
  done.openMenu();
  done.clickItem('End the chat');
  await settle();
  await settle();
  assert.deepEqual(calls, [['ask', 'End the chat?', ['End chat']], ['close', 'c1']]);
  done.close();
});

test('Switch to terminal asks while working and switches at once when idle', async () => {
  const idle = await openComposer({ phase: 'idle' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  idle.openMenu();
  idle.clickItem('Switch to terminal');
  await settle();
  assert.deepEqual(calls, [['switch']], 'no question when idle');
  assert.deepEqual(idle.items(), [], 'the menu closed first');
  idle.close();

  const working = await openComposer({ phase: 'working' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  ask = async () => 'cancel';
  working.openMenu();
  working.clickItem('Switch to terminal');
  await settle();
  assert.deepEqual(calls, [['ask', 'Switch to terminal?', ['Switch']]], 'cancelling changes nothing');
  working.close();

  const done = await openComposer({ phase: 'working' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  ask = async () => 'switch';
  done.openMenu();
  done.clickItem('Switch to terminal');
  await settle();
  await settle();
  assert.deepEqual(calls, [['ask', 'Switch to terminal?', ['Switch']], ['switch']]);
  done.close();
});

test('End the chat asks when messages are queued, even while idle', async () => {
  const idle = await openComposer({ phase: 'idle', queue: [{ id: 'queued-1', text: 'next' }] });
  idle.openMenu();
  idle.clickItem('End the chat');
  await settle();
  assert.deepEqual(calls, [['ask', 'End the chat?', ['End chat']]]);
  idle.close();
});

const queueButtons = (target) =>
  [...target.querySelectorAll('[data-queued]')].map((li) => [
    li.querySelector('.line').textContent,
    ...[...li.querySelectorAll('button')].map((b) => `${b.textContent.trim()}${b.disabled ? ' (disabled)' : ''}`),
  ]);

test('the queue shows Send now only where it can work: idle, or an agent that steers', async () => {
  const queue = [{ id: 'queued-1', text: 'first line\nsecond', attachments: [{ kind: 'image', name: 'a.png' }] }, { id: 'queued-2', text: 'after' }];
  const steers = await openComposer({ phase: 'working', canQueue: true, canSteer: true, queue });
  assert.deepEqual(queueButtons(steers.target), [
    ['first line', 'Send now', 'Edit', 'Remove'],
    ['after', 'Send now', 'Edit', 'Remove'],
  ]);
  assert.equal(steers.target.querySelector('[data-queued="queued-1"] .files').textContent, '1 file');
  assert.equal(steers.target.querySelector('[data-chat-queue-paused]'), null, 'a working queue is not paused');
  assert.match(steers.target.querySelector('[data-queued] button').title, /into its running turn/);
  steers.target.querySelector('[data-queued="queued-2"] button').click();
  await settle();
  assert.deepEqual(calls, [['sendNow', 'c1', 'queued-2']]);
  steers.close();

  const waits = await openComposer({ phase: 'working', canQueue: true, queue });
  assert.deepEqual(queueButtons(waits.target)[0], ['first line', 'Edit', 'Remove'], 'the agent takes it at the turn end');
  waits.close();

  const paused = await openComposer({ phase: 'idle', canQueue: true, queue });
  assert.equal(paused.target.querySelector('[data-chat-queue-paused]').textContent, 'Paused. Send now or send a new message to go on.');
  assert.deepEqual(queueButtons(paused.target)[0], ['first line', 'Send now', 'Edit', 'Remove']);
  paused.close();
});

test('Edit moves a queued message into the empty composer; Remove drops it; a typed message queues', async () => {
  const c = await openComposer({ phase: 'working', canQueue: true, queue: [{ id: 'queued-1', text: 'next' }, { id: 'queued-2', text: 'after' }] });
  const box = c.target.querySelector('textarea');
  const type = (value) => {
    box.value = value;
    box.dispatchEvent(new window.Event('input', { bubbles: true }));
    flushSync();
  };
  const button = (id, name) => [...c.target.querySelectorAll(`[data-queued="${id}"] button`)].find((b) => b.textContent.trim() === name);

  type('half written');
  assert.equal(button('queued-1', 'Edit').disabled, true, 'Edit needs an empty composer');
  assert.deepEqual(
    [...c.target.querySelectorAll('.toolbar button')].map((b) => b.textContent.trim()),
    ['Attach images or files', 'End chat', 'Queue', 'Cancel'],
    'a message to queue shows the queue button beside Stop',
  );
  type('');
  button('queued-1', 'Edit').click();
  await settle();
  flushSync();
  assert.deepEqual(calls, [['unqueue', 'c1', 'queued-1']]);
  assert.equal(box.value, 'the text of queued-1');
  assert.equal(document.activeElement, box);

  calls.length = 0;
  button('queued-2', 'Remove').click();
  await settle();
  flushSync();
  assert.deepEqual(calls, [['unqueue', 'c1', 'queued-2']]);
  assert.equal(box.value, 'the text of queued-1', 'Remove leaves the composer alone');
  c.close();
});

// --- The switch sequence in the opener --------------------------------------

const OPENER_URL = new URL('../src/lib/stores/chatOpen.svelte.ts', import.meta.url);
let openerModule;

async function withOpener(fixture, fn) {
  openerModule ??= await import(new URL('./helpers.mjs', import.meta.url).href).then(({ loadServerModule, fixtureStore }) =>
    loadServerModule(OPENER_URL, [
      ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
      ["'../link'", lib('../link.ts', OPENER_URL)],
      ["'../manage'", lib('../manage.ts', OPENER_URL)],
      ["'../chat'", lib('../chat.ts', OPENER_URL)],
      ["'../chatOptions'", lib('../chatOptions.ts', OPENER_URL)],
      ...['appState', 'chats', 'connectionStore', 'link', 'terminals', 'toast'].map((name) => {
        const file = name === 'connectionStore' ? 'connection' : name;
        return [`import { ${name} } from './${file}.svelte';`, fixtureStore(name, '__openerFixture')];
      }),
    ]),
  );
  const toasts = [];
  globalThis.__openerFixture = {
    appState: {},
    chats: {
      close: async (chatId) => calls.push(['close', chatId]),
      dismiss: (key) => calls.push(['dismiss', key]),
    },
    connectionStore: { canChat: true },
    link: { dataHost: '' },
    terminals: { runResume: async (ref, dir, name) => calls.push(['resume', ref, dir, name]) },
    toast: { show: (t) => toasts.push(t) },
    ...fixture,
  };
  try {
    await fn({ opener: new openerModule.ChatOpener(), toasts });
  } finally {
    delete globalThis.__openerFixture;
    calls.length = 0;
    toasts.length = 0;
  }
}

const META = { ref: { agent: 'codex', id: 's1' }, cwd: '/p', title: 'GitHub Actions failures', firstPrompt: 'fix CI' };

test('switching to a terminal stops the chat and resumes the session in a panel tab', async () => {
  await withOpener({}, async ({ opener }) => {
    assert.equal(await opener.switchToTerminal(chat(), META), true);
    assert.deepEqual(calls, [
      ['close', 'c1'],
      ['dismiss', 'chat-1'],
      ['resume', META.ref, '/p', 'GitHub Actions failures'],
    ]);
  });
  await withOpener({}, async ({ opener }) => {
    assert.equal(await opener.switchToTerminal(chat({ session: undefined }), META), false, 'nothing to resume');
    assert.deepEqual(calls, []);
  });
  await withOpener({}, async ({ opener }) => {
    assert.equal(await opener.switchToTerminal(chat(), null), false, 'a new chat its agent has not saved yet');
    assert.deepEqual(calls, [], 'the chat stays');
  });
});

test('a failed close keeps the chat; a failed resume says so and leaves the session saved', async () => {
  await withOpener({ chats: { close: async () => { throw new Error('the chat did not stop'); }, dismiss: (key) => calls.push(['dismiss', key]) } }, async ({ opener, toasts }) => {
    assert.equal(await opener.switchToTerminal(chat(), META), false);
    assert.deepEqual(calls, [], 'the chat is untouched');
    assert.equal(toasts[0].title, 'Could not end the chat');
    assert.match(toasts[0].body, /the chat did not stop/);
  });
  await withOpener({ terminals: { runResume: async () => { throw new Error('no pty'); } } }, async ({ opener, toasts }) => {
    assert.equal(await opener.switchToTerminal(chat(), META), false);
    assert.deepEqual(calls, [['close', 'c1'], ['dismiss', 'chat-1']], 'the chat still went');
    assert.equal(toasts[0].title, 'Could not open the terminal');
    assert.match(toasts[0].body, /no pty/);
    assert.match(toasts[0].body, /The session is saved/);
  });
});

// --- The switch to a chat in the opener -------------------------------------

const TAB = {
  id: 'tab-7', label: 'Fix CI', host: '', kind: 'agent', dir: '/p', cwd: '/wip',
  session: { agent: 'codex', id: 's1' }, status: 'running', exitCode: null,
};
const paneTab = (fields = {}) => ({ ...TAB, ...fields });

function chatSwitchFixture(extra = {}) {
  return {
    chats: {
      forSession: () => undefined,
      start: async (req) => calls.push(['start', req]),
    },
    terminals: {
      close: (id, confirmed) => calls.push(['pane', id, confirmed]),
      tabFor: () => undefined,
    },
    ...extra,
  };
}

test('switching to a chat ends the pane and continues the session in a chat', async () => {
  await withOpener(chatSwitchFixture(), async ({ opener }) => {
    assert.equal(await opener.switchToChat(paneTab()), true);
    assert.deepEqual(calls, [
      ['pane', 'tab-7', true],
      ['start', { agent: 'codex', dir: '/wip', resume: { agent: 'codex', id: 's1' }, questions: true }],
    ], "only the session's pane goes, then the chat continues the session");
  });
  await withOpener(chatSwitchFixture(), async ({ opener }) => {
    assert.equal(await opener.switchToChat(paneTab({ session: undefined })), false, 'nothing to continue');
    assert.deepEqual(calls, [], 'the pane stays');
  });
});

test('a failed chat start leaves the pane gone and the session saved', async () => {
  const failing = chatSwitchFixture({ chats: { forSession: () => undefined, start: async () => { throw new Error('codex-acp was not found'); } } });
  await withOpener(failing, async ({ opener, toasts }) => {
    assert.equal(await opener.switchToChat(paneTab()), false);
    assert.deepEqual(calls, [['pane', 'tab-7', true]], 'the pane still went');
    assert.equal(toasts[0].title, 'Could not start the chat');
    assert.match(toasts[0].body, /codex-acp was not found/);
  });
});

test('a session Codex still holds offers Try again, which starts the chat', async () => {
  let refusals = 1;
  const held = chatSwitchFixture({
    chats: {
      forSession: () => undefined,
      start: async (req) => {
        if (refusals-- > 0) throw new Error('the session is open in another Codex client: its CLI, app or IDE extension, or one that closed it in the last minute. Close it there, or try again in a minute');
        calls.push(['start', req]);
      },
    },
  });
  await withOpener(held, async ({ opener, toasts }) => {
    assert.equal(await opener.switchToChat(paneTab()), false);
    const shown = toasts.at(-1);
    assert.equal(shown.title, 'Codex is still using this session');
    assert.equal(shown.actions[0].label, 'Try again');
    shown.actions[0].onSelect();
    await settle();
    assert.deepEqual(calls, [
      ['pane', 'tab-7', true],
      ['start', { agent: 'codex', dir: '/wip', resume: { agent: 'codex', id: 's1' }, questions: true }],
    ], 'the retry continues the session in the chat');
  });
});

test('a / word lists the agent\'s commands; choosing one inserts it and only sending runs it', async () => {
  const commands = [
    { name: 'mcp', description: 'MCP status', input: { hint: '[reconnect]' } },
    { name: 'ponytail:ponytail', description: '(ponytail) Lazy', input: { hint: '[lite|full]' } },
    { name: 'understand-anything:understand', input: null },
    { name: 'other:understand', input: null },
  ];
  const c = await openComposer({ phase: 'idle', agent: 'claude', commands });
  const box = c.target.querySelector('textarea');
  const type = (value) => {
    box.value = value;
    box.dispatchEvent(new window.Event('input', { bubbles: true }));
    flushSync();
  };
  const key = (k, init = {}) => {
    const e = new window.KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...init });
    box.dispatchEvent(e);
    flushSync();
    return e;
  };
  const options = () => [...c.target.querySelectorAll('[role="option"]')].map((o) => o.querySelector('.command-name').textContent);
  const selected = () => c.target.querySelector('[role="option"][aria-selected="true"] .command-name')?.textContent;

  type('hello');
  assert.deepEqual(options(), [], 'no list for a message');

  // An ambiguous short name lists both plugins; nothing is picked by itself.
  type('/understand');
  assert.deepEqual(options(), ['/understand-anything:understand', '/other:understand']);
  assert.equal(selected(), '/understand-anything:understand');
  assert.equal(box.getAttribute('aria-activedescendant'), c.target.querySelector('[aria-selected="true"]').id);
  key('ArrowDown');
  assert.equal(selected(), '/other:understand');
  key('ArrowDown');
  assert.equal(selected(), '/understand-anything:understand', 'wraps around');
  key('ArrowUp');
  key('Enter');
  await settle();
  assert.equal(box.value, '/other:understand', 'Enter inserts the highlighted command');
  assert.deepEqual(options(), [], 'and closes the list');
  assert.deepEqual(calls, [], 'nothing is sent');
  key('Enter');
  await settle();
  assert.deepEqual(calls, [['prompt', 'c1', '/other:understand']], 'the next Enter sends it');
  calls.length = 0;

  // A short name is completed to the advertised name, with room for arguments.
  type('/pony');
  key('Tab');
  assert.equal(box.value, '/ponytail:ponytail ');
  assert.deepEqual(options(), []);

  // A command typed in full is sent by Enter; /mcp is offered bare.
  type('/mcp');
  assert.deepEqual(options(), ['/mcp']);
  assert.equal(c.target.querySelector('.command-hint'), null, 'no mutating arguments offered');
  key('Enter');
  await settle();
  assert.deepEqual(calls, [['prompt', 'c1', '/mcp']]);
  calls.length = 0;

  // Escape closes the list and sends nothing; while an IME composes, keys are its own.
  type('/u');
  const esc = key('Escape');
  assert.equal(esc.defaultPrevented, true);
  assert.deepEqual(options(), []);
  type('/un');
  key('Enter', { isComposing: true });
  assert.equal(box.value, '/un');
  assert.equal(options().length, 2);

  // A click inserts too.
  c.target.querySelectorAll('[role="option"]')[1].dispatchEvent(new window.MouseEvent('click', { bubbles: true }));
  flushSync();
  await settle();
  assert.equal(box.value, '/other:understand');
  assert.deepEqual(calls, []);
  c.close();
});
