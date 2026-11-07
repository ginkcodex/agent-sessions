// The app's dropdown, driven in a DOM: groups, options that can't be chosen,
// typing to jump, and the trigger's own key and context-menu handlers.
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
globalThis.PointerEvent ??= dom.window.MouseEvent;
dom.window.Element.prototype.scrollIntoView ??= function scrollIntoView() {};

const client = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);

async function loadDropdown() {
  const url = new URL('../src/lib/components/common/Dropdown.svelte', import.meta.url);
  const filename = fileURLToPath(url);
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  code = code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'))
    .replace("import Icon from './Icon.svelte';", 'const Icon = () => {};');
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}

const Dropdown = await loadDropdown();

function open(props) {
  const target = document.createElement('div');
  document.body.appendChild(target);
  const chosen = [];
  const component = mount(Dropdown, { target, props: { ariaLabel: 'Pick', value: '', onChange: (v) => chosen.push(v), ...props } });
  flushSync();
  const trigger = target.querySelector('button.trigger');
  const key = (el, k, init = {}) => {
    const event = new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...init });
    el.dispatchEvent(event);
    flushSync();
    return event;
  };
  return {
    target, trigger, chosen, key,
    list: () => target.querySelector('[role="listbox"]'),
    active: () => {
      const list = target.querySelector('[role="listbox"]');
      return list && target.querySelector(`#${list.getAttribute('aria-activedescendant')}`)?.textContent.replace('✓', '').trim();
    },
    close: () => { unmount(component); target.remove(); },
  };
}

const refs = [
  { value: 'HEAD', label: 'HEAD' },
  { value: 'refs/heads/main', label: 'main', group: 'Local' },
  { value: 'refs/heads/dev', label: 'dev', group: 'Local' },
  { value: 'refs/remotes/origin/main', label: 'origin/main', group: 'Remote · origin' },
  { value: 'refs/tags/blob', label: 'blob (not a commit)', group: 'Tags', disabled: true },
  { value: 'refs/tags/v1', label: 'v1', group: 'Tags' },
];

test('options sharing a group sit under one heading, and options that can\'t be chosen are skipped', () => {
  const d = open({ options: refs, value: 'HEAD' });
  d.key(d.trigger, 'ArrowDown');
  const list = d.list();
  assert.ok(list, 'ArrowDown opens the list');
  assert.deepEqual([...list.querySelectorAll('li.group')].map((li) => li.textContent), ['Local', 'Remote · origin', 'Tags']);
  assert.equal(list.querySelectorAll('[role="option"]').length, refs.length, 'headings are not options');
  const blob = list.querySelector('[aria-disabled="true"]');
  assert.match(blob.textContent, /blob \(not a commit\)/);
  assert.equal(d.active(), 'HEAD', 'opens on the chosen option');
  d.key(list, 'End');
  assert.equal(d.active(), 'v1');
  d.key(list, 'ArrowUp');
  assert.equal(d.active(), 'origin/main', 'steps over the tag that is not a commit');
  blob.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
  flushSync();
  assert.ok(d.list(), 'clicking it neither chooses nor closes');
  assert.deepEqual(d.chosen, []);
  d.key(d.list(), 'Enter');
  assert.deepEqual(d.chosen, ['refs/remotes/origin/main']);
  assert.equal(d.list(), null);
  d.close();
});

test('typing jumps to the option it starts, spaces included', () => {
  const fonts = ['', 'Fira Code', 'FiraCode Nerd Font', 'JetBrains Mono', 'JetBrainsMono Nerd Font', 'Menlo'].map((v) => ({ value: v, label: v || 'App default' }));
  const d = open({ options: fonts });
  d.trigger.click();
  flushSync();
  const list = d.list();
  d.key(list, 'j');
  assert.equal(d.active(), 'JetBrains Mono');
  d.key(list, 'j');
  assert.equal(d.active(), 'JetBrainsMono Nerd Font', 'the same letter again steps to the next');
  // Reopening starts a new word.
  d.key(list, 'Escape');
  d.trigger.click();
  flushSync();
  for (const k of 'firacode ') d.key(d.list(), k);
  assert.equal(d.active(), 'FiraCode Nerd Font');
  assert.ok(d.list(), 'a space in a word being typed does not choose');
  assert.deepEqual(d.chosen, []);
  d.close();
});

test('the trigger runs its own key and context-menu handlers first, and a disabled one stays shut', () => {
  const seen = [];
  const d = open({
    options: refs, value: 'HEAD',
    onkeydown: (e) => { seen.push(e.key); if (e.key === 'ContextMenu') e.preventDefault(); },
    oncontextmenu: (e) => seen.push(e.type),
  });
  d.key(d.trigger, 'ContextMenu');
  d.trigger.dispatchEvent(new dom.window.MouseEvent('contextmenu', { bubbles: true }));
  d.key(d.trigger, 'ArrowDown');
  assert.deepEqual(seen, ['ContextMenu', 'contextmenu', 'ArrowDown']);
  assert.ok(d.list(), 'arrow keys it lets through still open the list');
  d.close();
  const off = open({ options: refs, value: 'HEAD', disabled: true });
  off.trigger.click();
  off.key(off.trigger, 'ArrowDown');
  assert.equal(off.list(), null);
  off.close();
});

test('the narrow Branch, Diff basis and font pickers use the shared dropdown', () => {
  const read = (path) => readFileSync(new URL(`../src/lib/components/common/${path}`, import.meta.url), 'utf8');
  for (const [file, label] of [['go-to-file/HistoryBranches.svelte', '"History branch"'], ['go-to-file/FileViewer.svelte', '"Diff basis"'], ['MonoFontSetting.svelte', '{ariaLabel}']]) {
    const source = read(file);
    assert.ok(source.includes(`<Dropdown ariaLabel=${label}`), file);
    assert.doesNotMatch(source, /<select\b/, file);
  }
  assert.match(read('TerminalFontSetting.svelte'), /ariaLabel="Built-in terminal font"/);
});
