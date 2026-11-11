// A chat's permission card, driven in a DOM: no option takes focus, Enter
// does not approve, and a click answers once.
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

const client = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync, tick } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);

async function loadCard() {
  const url = new URL('../src/lib/components/chat/PermissionCard.svelte', import.meta.url);
  const filename = fileURLToPath(url);
  const lib = (rel) => JSON.stringify(new URL(rel, url).href);
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  code = code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'))
    .replace("import { chats } from '../../stores/chats.svelte';", 'const chats = globalThis.__card.chats;')
    .replace("import { toast } from '../../stores/toast.svelte';", 'const toast = globalThis.__card.toast;')
    .replace("'../../chat'", lib('../../chat.ts'))
    .replace("'../../link'", lib('../../link.ts'))
    .replace("'../../manage'", lib('../../manage.ts'))
    .replace("import Icon from '../common/Icon.svelte';", 'const Icon = () => {};');
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}

const answers = [];
const toasts = [];
let respond = async (...args) => answers.push(args);
globalThis.__card = { chats: { respond: (...args) => respond(...args) }, toast: { show: (t) => toasts.push(t) } };
const Card = await loadCard();

const PERMISSION = {
  id: 'perm-1',
  toolCall: { toolCallId: 't1', title: 'Run make', kind: 'execute', rawInput: { command: 'make' } },
  options: [
    { optionId: 'allow', name: 'Allow', kind: 'allow_once' },
    { optionId: 'always', name: 'Always allow', kind: 'allow_always' },
    { optionId: 'reject', name: 'Reject', kind: 'reject_once' },
  ],
};

function open() {
  const target = document.createElement('div');
  document.body.appendChild(target);
  const component = mount(Card, { target, props: { chat: { chatId: 'c1', key: 'k1' }, permission: PERMISSION, agentName: 'Codex' } });
  flushSync();
  const button = (name) => [...target.querySelectorAll('button')].find((b) => b.textContent.trim().startsWith(name));
  const close = () => {
    unmount(component);
    target.remove();
  };
  return { target, button, close };
}

const enter = (el) => {
  const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
  el.dispatchEvent(event);
  return event.defaultPrevented;
};

test('no option takes focus, and Enter on an allow option is held back', () => {
  answers.length = 0;
  const { button, close } = open();
  try {
    assert.equal(document.activeElement, document.body, 'the card takes no focus');
    assert.equal(enter(button('Allow')), true, 'Enter does not press Allow');
    assert.equal(enter(button('Always allow')), true);
    assert.equal(enter(button('Reject')), false, 'Enter may reject');
    assert.deepEqual(answers, []);
  } finally {
    close();
  }
});

test('a click answers with that option once, and a failed answer says why', async () => {
  answers.length = 0;
  const { button, close } = open();
  try {
    let release;
    respond = (...args) => {
      answers.push(args);
      return new Promise((resolve) => (release = resolve));
    };
    button('Allow').click();
    flushSync();
    assert.deepEqual(answers, [['c1', 'perm-1', 'allow']]);
    assert.ok([...document.querySelectorAll('[data-permission-card] button')].every((b) => b.disabled), 'one answer at a time');
    button('Reject').click();
    assert.equal(answers.length, 1);
    release();
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
    flushSync();
    assert.ok(!button('Reject').disabled);

    respond = async () => {
      throw new Error('the permission request is no longer pending: perm-1');
    };
    button('Reject').click();
    await new Promise((resolve) => setTimeout(resolve, 0));
    assert.equal(toasts.at(-1).title, 'Could not answer the request');
    assert.match(toasts.at(-1).body, /no longer pending/);
  } finally {
    close();
    respond = async (...args) => answers.push(args);
  }
});
