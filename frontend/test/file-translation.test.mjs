import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';
import { captureSelection, holdSelection, selectionMenuItems } from '../src/lib/selectionTranslation.ts';
import { errorText } from '../src/lib/manage.ts';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
dom.window.Range.prototype.getBoundingClientRect = () => ({ left: 0, top: 0, right: 100, bottom: 20, width: 100, height: 20 });
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const viewer = readFileSync(new URL('../src/lib/components/common/go-to-file/FileViewer.svelte', import.meta.url), 'utf8');

// Compile the viewer's actual translation orchestration, independent of the
// unrelated diagnostics/navigation observers. Menu dispatch closes first, just
// like ContextMenu. Source/render/copy/print wiring is asserted below.
const flow = viewer.slice(viewer.indexOf('  let previewMenu ='), viewer.indexOf('  let status ='));
const status = viewer.slice(viewer.indexOf('      {#if showMarkdown && fileTranslation}'), viewer.indexOf('      {#if editing && edit}'));
let code = compile(`<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  type SelectedText = any;
  type Translation = any;
  type ContextMenuItem = any;
  const { api, toast, copyToClipboard, copyFailed, manage, captureSelection,
    holdSelection, selectionMenuItems, errorText, epoch } = globalThis.__fileTranslate;
  let files = $state({ selected: 'notes.md', dir: '/project' });
  let activeTab = $state({ path: 'notes.md' });
  let link = $state({ dataHost: '' });
  let translateSettings = $state({ configured: globalThis.__fileTranslate.configured, language: 'Vietnamese' });
  let text = $state(globalThis.__fileTranslate.source);
  let showMarkdown = $state(true);
  let contentShown = $state(true);
  let textContent = $state(true);
  export function change(kind, value) {
    if (kind === 'source') text = value;
    if (kind === 'tab') activeTab = { path: value };
    if (kind === 'dir') files.dir = value;
    if (kind === 'host') link.dataHost = value;
    if (kind === 'language') translateSettings.language = value;
    if (kind === 'preview') showMarkdown = value;
    if (kind === 'shown') contentShown = value;
    if (kind === 'textContent') textContent = value;
  }
  ${flow}
  onDestroy(() => { translationRequest++; });
</script>
<div class="toolbar">${status}</div>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="markdown-preview" oncontextmenu={openPreviewMenu}>{previewText}</div>
{#if previewMenu}
  <div role="menu">
    {#each previewMenuItems as item}
      <button role="menuitem" disabled={item.disabled} title={item.title} onclick={() => { previewMenu = null; item.onSelect(); }}>{item.label}</button>
    {/each}
  </div>
{/if}
{#if previewTranslation}<div class="selection-result">{previewTranslation.text}</div>{/if}
`, { generate: 'client', filename: 'FileTranslationFlow.svelte', css: 'external' }).js.code;
code = code.replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
  .replace("'svelte/internal/client'", client('internal/client/index.js'))
  .replace("'svelte'", client('index-client.js'));
const Flow = (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const settle = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); flushSync(); };

function open({ source = '# Heading\n\nFirst paragraph.\n\nLast paragraph.', configured = true, translate = async () => '# Bản dịch' } = {}) {
  const calls = [], settings = [], copies = [];
  let currentEpoch = 1;
  globalThis.__fileTranslate = {
    source, configured, api: { translate: text => { calls.push(text); return translate(text); } },
    toast: { show() {} }, copyToClipboard: async text => { copies.push(text); return true; }, copyFailed() {},
    manage: { openSettings: section => settings.push(section) },
    captureSelection, holdSelection, selectionMenuItems, errorText, epoch: () => currentEpoch,
  };
  const target = document.createElement('div');
  document.body.appendChild(target);
  const component = mount(Flow, { target });
  flushSync();
  const preview = target.querySelector('.markdown-preview');
  const items = () => [...target.querySelectorAll('[role="menuitem"]')];
  const menu = () => {
    const event = new window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 10, clientY: 20 });
    preview.dispatchEvent(event); flushSync(); return event;
  };
  const choose = label => { const item = items().find(i => i.textContent === label); assert.ok(item, label); item.click(); flushSync(); return item; };
  return { component, target, preview, calls, settings, copies, items, menu, choose,
    bumpEpoch: () => currentEpoch++, close: async () => { window.getSelection().removeAllRanges(); await unmount(component); target.remove(); } };
}

test('no-selection menu translates complete source, not viewport text, and toggles without changing source', async () => {
  const pending = deferred();
  const source = '# Heading\n\n' + 'Long content **with Markdown**.\n'.repeat(300) + '\nLast line.';
  const f = open({ source, translate: () => pending.promise });
  try {
    assert.equal(f.menu().defaultPrevented, true);
    f.choose('Translate entire file');
    assert.deepEqual(f.calls, [source]);
    assert.equal(f.target.querySelector('[role="menu"]'), null);
    assert.match(f.target.querySelector('[role="status"]').textContent, /Translating/);
    f.menu();
    assert.equal(f.items()[0].textContent, 'Translating…');
    assert.equal(f.items()[0].disabled, true);
    f.items()[0].click(); assert.equal(f.calls.length, 1);
    pending.resolve('# Translated\n\nAll content'); await settle();
    assert.equal(f.preview.textContent, '# Translated\n\nAll content');
    f.menu(); f.choose('Show original');
    assert.equal(f.preview.textContent, source);
    f.menu(); f.choose('Show translation');
    assert.equal(f.preview.textContent, '# Translated\n\nAll content');
    f.menu(); f.choose('Retranslate entire file');
    assert.deepEqual(f.calls, [source, source], 'retranslation uses original, never translated text');
    await settle();
  } finally { await f.close(); }
});

test('setup and UTF-8 size limit are explained without making requests', async () => {
  const f = open({ configured: false });
  try {
    f.menu(); assert.equal(f.items()[0].disabled, true);
    f.choose('Set up translation…');
    assert.deepEqual(f.settings, ['translation']); assert.deepEqual(f.calls, []);
  } finally { await f.close(); }
  for (const [source, disabled] of [['é'.repeat(100_000), false], ['é'.repeat(100_000) + 'x', true]]) {
    const f = open({ source });
    try {
      f.menu(); assert.equal(f.items()[0].disabled, disabled);
      if (disabled) assert.match(f.items()[0].title, /200,000-byte/);
      assert.deepEqual(f.calls, []);
    } finally { await f.close(); }
  }
});

test('empty, hidden, non-text and non-preview states keep native no-selection menu', async () => {
  for (const [kind, value] of [['source', ' \n\t'], ['shown', false], ['textContent', false], ['preview', false]]) {
    const f = open();
    try {
      f.component.change(kind, value); flushSync();
      assert.equal(f.menu().defaultPrevented, false, kind);
      assert.deepEqual(f.items(), []); assert.deepEqual(f.calls, []);
    } finally { await f.close(); }
  }
});

test('selected text takes precedence and invalid cross-root selections keep native menu', async () => {
  const f = open({ source: 'First sentence. Second sentence.' });
  try {
    const range = document.createRange();
    range.setStart(f.preview.firstChild, 6); range.setEnd(f.preview.firstChild, 14);
    window.getSelection().addRange(range);
    assert.equal(f.menu().defaultPrevented, true);
    assert.deepEqual(f.items().map(i => i.textContent), ['Copy', 'Translate to Vietnamese']);
    f.choose('Translate to Vietnamese');
    assert.equal(f.target.querySelector('.selection-result').textContent, 'sentence');
    assert.deepEqual(f.calls, []);
    window.getSelection().removeAllRanges();
    const outside = document.createElement('p'); outside.textContent = 'Outside'; f.target.appendChild(outside);
    const invalid = document.createRange(); invalid.setStart(f.preview.firstChild, 0); invalid.setEnd(outside.firstChild, 3);
    window.getSelection().addRange(invalid);
    assert.equal(f.menu().defaultPrevented, false);
    assert.deepEqual(f.items(), []);
  } finally { await f.close(); }
});

test('translation failures retain original source and support Retry', async () => {
  let fail = true;
  const f = open({ translate: async () => { if (fail) throw new Error('Provider offline'); return 'Recovered'; } });
  try {
    const original = f.preview.textContent;
    f.menu(); f.choose('Translate entire file'); await settle();
    assert.equal(f.preview.textContent, original);
    assert.match(f.target.querySelector('[role="status"]').textContent, /Translation failed: Provider offline/);
    fail = false;
    [...f.target.querySelectorAll('button')].find(b => b.textContent === 'Retry').click(); await settle();
    assert.equal(f.preview.textContent, 'Recovered'); assert.equal(f.calls.length, 2);
  } finally { await f.close(); }
});

test('file, project, host, source, language, mode and teardown invalidate late responses', async () => {
  for (const [kind, value] of [['tab', 'other.md'], ['dir', '/other'], ['host', 'ssh:other'], ['source', 'New source'], ['language', 'French'], ['preview', false], ['shown', false], ['epoch', null], ['unmount', null]]) {
    const pending = deferred(), f = open({ translate: () => pending.promise });
    f.menu(); f.choose('Translate entire file');
    if (kind === 'epoch') f.bumpEpoch();
    else if (kind === 'unmount') await f.close();
    else { f.component.change(kind, value); flushSync(); }
    pending.resolve('STALE TRANSLATION'); await settle();
    assert.doesNotMatch(f.preview.textContent, /STALE TRANSLATION/, kind);
    if (kind !== 'unmount') {
      assert.equal(f.target.querySelector('[role="status"]'), null, kind);
      await f.close();
    }
  }
});

test('viewer renders and copies/prints displayed Markdown but never mutates stored source', () => {
  assert.match(viewer, /<MarkdownDoc source=\{previewText\}/);
  assert.match(viewer, /copyToClipboard\(previewText\)/);
  assert.match(viewer, /printFile\(files.selectedPath, previewText\)/);
  assert.match(viewer, /onDestroy\(\(\) => \{\s*translationRequest\+\+;/);
  assert.doesNotMatch(flow, /files\.content\s*=|files\.content\.text\s*=|files\.updateDraft|api\.saveFile/);
  assert.match(viewer, /let contentShown = \$derived\([\s\S]*?!editing && !files.loadingFile && !files.fileError/);
});
