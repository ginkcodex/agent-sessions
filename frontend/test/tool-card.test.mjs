import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const svelte = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const url = new URL('../src/lib/components/transcript/parts/ToolPart.svelte', import.meta.url);
const filename = fileURLToPath(url);
let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
code = code
  .replace("'svelte/internal/disclose-version'", svelte('internal/disclose-version.js'))
  .replace("'svelte/internal/client'", svelte('internal/client/index.js'))
  .replace("import Icon from '../../common/Icon.svelte';", 'const Icon = () => {};')
  .replace("import SubagentLink from './SubagentLink.svelte';", 'const SubagentLink = () => {};')
  .replace("import { highlightCode } from '../../../highlight';", 'const highlightCode = text => text;')
  .replace("'../../../toolPresentation'", JSON.stringify(new URL('../src/lib/toolPresentation.ts', import.meta.url).href))
  .replace("import { api } from '../../../api';", 'const api = { getBlob: (...args) => globalThis.__toolBlob(...args) };');
const Card = (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;

const output = JSON.stringify([{ type: 'input_text', text: Array.from({ length: 50 }, (_, i) => `line ${i}`).join('\n') }]);
const props = { tool: { id: 'test-call', name: 'exec', status: 'completed', input: { command: 'git status' }, output }, sessionRef: { agent: 'codex', id: 'test' } };

// jsdom has no layout engine: protect the actual sizing rules here and use
// browser geometry checks separately for overlap at narrow/large-font sizes.
test('header reserves a status column and clips both possible command sources', () => {
  const css = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).css.code;
  const rule = selector => css.match(new RegExp(`${selector}[^{}]*\\{([^}]+)\\}`))[1];
  assert.match(rule('\\.tool-header'), /display:\s*grid/);
  assert.match(rule('\\.tool-header'), /grid-template-columns:\s*minmax\(0,\s*1fr\) auto/);
  assert.match(rule('\\.tool-id-group'), /min-width:\s*0/);
  assert.match(rule('\\.tool-id-group'), /overflow:\s*hidden/);
  // The name has typography and truncation in separate rules.
  const nameRules = [...css.matchAll(/\.tool-name[^{}]*\{([^}]+)\}/g)].map(match => match[1]).join(' ');
  assert.match(nameRules, /min-width:\s*0/);
  assert.match(nameRules, /text-overflow:\s*ellipsis/);
  assert.match(nameRules, /overflow:\s*hidden/);
  assert.match(rule('\\.tool-summary'), /text-overflow:\s*ellipsis/);
  assert.match(rule('\\.tool-status-badge'), /white-space:\s*nowrap/);
});

for (const status of ['pending', 'completed', 'error']) {
  test(`long live command retains its full tooltip beside ${status} status`, async () => {
    const name = `python3 - <<'EOF' ${'import re; assert sub(path, old, new); '.repeat(20)}`;
    const { target, close } = setup({ tool: { ...props.tool, name, status, input: undefined } });
    const label = target.querySelector('.tool-name');
    assert.equal(label.textContent, name);
    assert.equal(label.title, `${name}\ntest-call`);
    assert.equal(target.querySelector('.tool-summary'), null);
    assert.match(target.querySelector('.tool-status-badge').textContent, new RegExp(status === 'pending' ? 'running' : status));
    await close();
  });
}
function setup(extra = {}) {
  const target = document.createElement('div');
  document.body.append(target);
  const card = mount(Card, { target, props: { ...props, ...extra } });
  flushSync();
  return { target, close: async () => { await unmount(card); target.remove(); } };
}

test('output expands and collapses independently from parameters; Copy preserves raw output', async () => {
  let copied;
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async text => { copied = text; } } });
  const { target, close } = setup();
  const preview = target.querySelector('.output-content');
  const button = target.querySelector('.expand-output-btn');
  assert.ok(!preview.textContent.includes('line 49'));
  assert.equal(button.getAttribute('aria-expanded'), 'false');
  flushSync(() => button.click());
  assert.ok(preview.textContent.includes('line 49'));
  assert.equal(button.getAttribute('aria-expanded'), 'true');
  const input = target.querySelector('details');
  assert.equal(input.open, false);
  flushSync(() => { input.open = true; input.dispatchEvent(new window.Event('toggle')); });
  assert.ok(target.querySelector('.code-wrapper').textContent.includes('git status'));
  flushSync(() => button.click());
  assert.ok(!preview.textContent.includes('line 49'));
  assert.equal(input.open, true);
  target.querySelector('.output-header button').click();
  await Promise.resolve();
  assert.equal(copied, output);
  await close();
});

test('search-expanded cards reveal the complete result immediately', async () => {
  const { target, close } = setup({ defaultExpanded: true });
  assert.ok(target.querySelector('.output-content').textContent.includes('line 49'));
  assert.equal(target.querySelector('.expand-output-btn'), null);
  assert.equal(target.querySelector('details').open, true);
  await close();
});

test('blob failure stays retryable and fetched text uses the same readable preview', async () => {
  const { target, close } = setup({ tool: { ...props.tool, outputTruncated: true, outputRef: 'blob-id' } });
  globalThis.__toolBlob = async (ref, id) => { assert.deepEqual(ref, props.sessionRef); assert.equal(id, 'blob-id'); throw new Error('Try again'); };
  target.querySelector('.load-blob-btn').click();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
  assert.ok(target.querySelector('.blob-error-msg').textContent.includes('Try again'));
  globalThis.__toolBlob = async () => ({ data: '[{"type":"text","text":"loaded\\noutput"}]' });
  target.querySelector('.load-blob-btn').click();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
  assert.equal(target.querySelector('.output-content').textContent, 'loaded\noutput');
  assert.equal(target.querySelector('.load-blob-btn'), null);
  assert.equal(target.querySelector('.blob-error-msg'), null);
  await close();
});
