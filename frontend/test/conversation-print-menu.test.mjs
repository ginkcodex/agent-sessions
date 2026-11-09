import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';
import { loadConversationPrintParts } from '../src/lib/conversationPrint.ts';
import { isStaleReply } from '../src/lib/link.ts';
import { errorText } from '../src/lib/manage.ts';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);

async function loadHeader() {
  const url = new URL('../src/lib/components/transcript/SessionHeader.svelte', import.meta.url);
  const filename = fileURLToPath(url);
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  code = code.replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'));
  for (const [statement, names, from] of [...code.matchAll(/^import ([\s\S]*?) from '([^']+)';$/gm)].map(m => [m[0], m[1], m[2]])) {
    if (from.startsWith('file:')) continue;
    if (/^[A-Z]\w*$/.test(names)) code = code.replace(statement, `const ${names} = () => {};`);
    else if (from.includes('/stores/')) {
      const name = names.replace(/[{}\s]/g, '');
      code = code.replace(statement, `const ${name} = new Proxy({}, { get: (_, key) => globalThis.__printHeader.${name}[key] });`);
    } else code = code.replace(`'${from}'`, JSON.stringify(new URL(`${from}.ts`, url).href));
  }
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}
const Header = await loadHeader();

function open({ busy = false, empty = false, locked = false, callback = true } = {}) {
  globalThis.__printHeader = {
    appState: {}, handoff: {}, exporter: {}, toast: {}, agentSettings: { activeAgents: [] },
    link: { locked }, launcher: { canOpen: false }, preferences: { timeZone: 'utc' },
    newSession: { available: false, choices: [] }, terminals: { tabFor: () => undefined },
  };
  const target = document.createElement('div');
  document.body.appendChild(target);
  let clicks = 0;
  const meta = {
    ref: { agent: 'codex', id: 'session' }, title: 'Chat', cwd: '/tmp/project',
    counts: { user: empty ? 0 : 1, assistant: empty ? 0 : 1 }, tokens: {},
    createdAt: '', updatedAt: '',
  };
  const component = mount(Header, { target, props: {
    meta, mode: 'all', onModeChange() {}, onResume() {}, printPreparing: busy,
    onPrint: callback ? () => { clicks++; } : undefined,
  } });
  flushSync();
  const trigger = target.querySelector('[aria-label="More actions"]');
  trigger.click();
  flushSync();
  const item = [...target.querySelectorAll('[role="menuitem"]')].find(el => /conversation/.test(el.textContent));
  return { target, trigger, item, clicks: () => clicks, close: async () => { await unmount(component); target.remove(); } };
}

test('three-dot menu prints the entire Chat conversation and closes before invoking the callback', async () => {
  const header = open();
  try {
    assert.equal(header.item.textContent.trim(), 'Print conversation to PDF…');
    assert.match(header.item.title, /entire conversation at Chat level/);
    assert.equal(header.item.disabled, false);
    header.item.click();
    flushSync();
    assert.equal(header.clicks(), 1);
    assert.equal(header.target.querySelector('[role="menu"]'), null);
  } finally { await header.close(); }
});

test('menu refuses busy, empty and locked conversations and exposes preparation state', async () => {
  for (const options of [{ busy: true }, { empty: true }, { locked: true }]) {
    const header = open(options);
    try {
      assert.equal(header.item.disabled, true);
      header.item.click();
      flushSync();
      assert.equal(header.clicks(), 0);
      if (options.busy) {
        assert.equal(header.trigger.getAttribute('aria-busy'), 'true');
        assert.match(header.trigger.title, /Preparing conversation/);
        assert.equal(header.item.textContent.trim(), 'Preparing conversation…');
      }
    } finally { await header.close(); }
  }
});

test('header does not offer conversation printing without a handler', async () => {
  const header = open({ callback: false });
  try { assert.equal(header.item, undefined); }
  finally { await header.close(); }
});

// Exercise the actual view orchestration in a small Svelte host, without the
// unrelated terminal/search/scroll observers of the full transcript viewer.
async function loadPrintFlow() {
  const url = new URL('../src/lib/components/transcript/TranscriptView.svelte', import.meta.url);
  const source = readFileSync(url, 'utf8');
  assert.match(source, /onPrint=\{\(\) => void printConversation\(\)\}/);
  assert.match(source, /\{printPreparing\}/);
  const flow = source.slice(source.indexOf('  let printPreparing ='), source.indexOf('  let containerEl:'));
  const harness = `<script>
    let appState = $state(globalThis.__printFlow.appState);
    let link = $state({ locked: false, phase: 'local', dataHost: undefined });
    const { api, epoch, printMessage, toast, translations, translationKey,
      translateSettings, loadConversationPrintParts, errorText, isStaleReply } = globalThis.__printFlow;
    const refsMatch = (a, b) => a?.id === b.id && a?.agent === b.agent;
    export function select(id) { appState.selectedSessionRef = { agent: 'codex', id }; }
    export function lock() { link.phase = 'disconnected'; link.locked = true; }
    ${flow}
  </script>
  <button aria-busy={printPreparing} onclick={() => void printConversation()}>Print</button>`;
  let code = compile(harness, { generate: 'client', filename: 'PrintFlow.svelte', css: 'external' }).js.code;
  code = code.replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'));
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}
const PrintFlow = await loadPrintFlow();
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const settle = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); flushSync(); };
const message = text => ({ role: 'user', parts: [{ kind: 'text', text }] });
const fullPage = messages => ({ messages, offset: 0, totalCount: messages.length, hasMore: false, revision: 'one' });

function flow(getMessages) {
  const jobs = [], notices = [];
  let currentEpoch = 1;
  globalThis.__printFlow = {
    appState: { selectedSessionRef: { agent: 'codex', id: 'original' }, selectedSessionMeta: { ref: { agent: 'codex', id: 'original' }, title: 'Original title' } },
    api: { getMessages }, epoch: () => currentEpoch,
    printMessage: job => jobs.push(job), toast: { show: notice => notices.push(notice) },
    translations: { get: () => undefined }, translationKey: () => '', translateSettings: { language: 'en' },
    loadConversationPrintParts, errorText, isStaleReply,
  };
  const target = document.createElement('div');
  document.body.appendChild(target);
  const component = mount(PrintFlow, { target });
  flushSync();
  const button = target.querySelector('button');
  return { component, button, jobs, notices, bumpEpoch: () => currentEpoch++, close: async () => { await unmount(component); target.remove(); } };
}

test('view preparation prevents duplicate requests, captures title/ref and opens the shared editor', async () => {
  const pending = deferred();
  let calls = 0;
  const f = flow(async ref => { calls++; assert.equal(ref.id, 'original'); return pending.promise; });
  try {
    f.button.click(); f.button.click(); flushSync();
    assert.equal(calls, 1);
    assert.equal(f.button.getAttribute('aria-busy'), 'true');
    pending.resolve(fullPage([message('All conversation')]));
    await settle();
    assert.equal(f.button.getAttribute('aria-busy'), 'false');
    assert.equal(f.jobs.length, 1);
    assert.equal(f.jobs[0].title, 'Original title');
    assert.equal(f.jobs[0].sessionRef.id, 'original');
    assert.deepEqual(f.jobs[0].parts, [{ kind: 'text', text: '## User' }, { kind: 'text', text: 'All conversation' }]);
    assert.deepEqual(f.notices, []);
  } finally { await f.close(); }
});

test('view preparation reports empty content/fetch errors without opening a partial editor', async () => {
  for (const [fetch, expected] of [
    [async () => fullPage([]), /Nothing to print/],
    [async () => { throw new Error('offline'); }, /Could not print conversation/],
  ]) {
    const f = flow(fetch);
    try {
      f.button.click(); await settle();
      assert.deepEqual(f.jobs, []);
      assert.match(f.notices[0].title, expected);
      assert.equal(f.button.getAttribute('aria-busy'), 'false');
    } finally { await f.close(); }
  }
});

test('view preparation silently cancels on session/host changes and teardown', async () => {
  for (const change of ['select', 'lock', 'epoch', 'unmount']) {
    const pending = deferred();
    const f = flow(async () => pending.promise);
    f.button.click(); flushSync();
    if (change === 'select') f.component.select('other');
    if (change === 'lock') f.component.lock();
    if (change === 'epoch') f.bumpEpoch();
    if (change === 'unmount') await f.close();
    flushSync();
    pending.resolve(fullPage([message('Old session')]));
    await settle();
    assert.deepEqual(f.jobs, [], change);
    assert.deepEqual(f.notices, [], change);
    if (change !== 'unmount') await f.close();
  }
});
