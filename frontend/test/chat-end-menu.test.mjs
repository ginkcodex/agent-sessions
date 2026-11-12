// The composer's End chat menu in a DOM: the two items, End the chat as
// always, and Switch to terminal, which stops the chat and continues the
// session in a terminal tab. Plus the switch sequence in the opener.
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

async function loadComposer() {
  const url = new URL('../src/lib/components/chat/ChatComposer.svelte', import.meta.url);
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
  },
  toast: { show: (t) => calls.push(['toast', t.title]) },
  filePrompt: {
    ask: async (title, message, choices) => {
      calls.push(['ask', title, choices.map((c) => c.label)]);
      return ask();
    },
  },
};
const Composer = await loadComposer();

// Lets every pending promise chain run before an assertion.
const settle = () => new Promise((resolve) => setImmediate(resolve));

const chat = (fields = {}) => ({
  key: 'chat-1', chatId: 'c1', agent: 'codex', dir: '/p', session: { agent: 'codex', id: 's1' },
  configOptions: [], phase: 'idle', tail: { items: [], usage: null }, permissions: [], questions: [], epoch: 0,
  promptCapabilities: { image: true }, ...fields,
});

async function openComposer(fields, props = {}) {
  const target = document.createElement('div');
  document.body.appendChild(target);
  const state = proxy({ chat: chat(fields), onSwitchToTerminal: props.onSwitchToTerminal, onRestart: props.onRestart });
  const component = mount(Composer, { target, props: state });
  flushSync();
  const text = (el) => el.textContent.trim().replace(/\s+/g, ' ');
  const trigger = () => target.querySelector('[aria-haspopup="menu"]');
  const items = () => [...target.querySelectorAll('[role="menu"] [role="menuitem"]')].map(text);
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
  return { target, trigger, items, clickItem, openMenu, escape, outside, close };
}

test('the power button opens a menu with the two items, and only for a saved session both', async () => {
  const both = await openComposer({ phase: 'idle' }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  assert.deepEqual(both.items(), [], 'closed at first');
  both.openMenu();
  assert.deepEqual(both.items(), ['End the chat', 'Switch to terminal']);
  both.close();

  const unsaved = await openComposer({ phase: 'idle', session: undefined }, { onSwitchToTerminal: async () => calls.push(['switch']) });
  unsaved.openMenu();
  assert.deepEqual(unsaved.items(), ['End the chat'], 'nothing to resume yet');
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
