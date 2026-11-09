import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { render } from 'svelte/server';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';
import { loadServerComponent } from './helpers.mjs';
import { bashPreview, manualBashMarkdown, manualBashMessage, parseManualBash } from '../src/lib/manualBash.ts';
import { visibleTranscriptParts } from '../src/lib/transcript.ts';

const moduleUrl = JSON.stringify(new URL('../src/lib/manualBash.ts', import.meta.url).href);
const url = new URL('../src/lib/components/transcript/parts/ManualBashPart.svelte', import.meta.url);
const clipboard = "import { copyToClipboard } from '../../../portable';";
const replacements = [
  ["'../../../manualBash'", moduleUrl],
  [clipboard, 'const copyToClipboard = async text => globalThis.__bashCopy(text);'],
];
const { default: Card } = await loadServerComponent(url, replacements);
const command = '<bash-input> CD_PATH="/tmp/bin:$PATH" go test -v ./...\n</bash-input>';
const output = '<bash-stdout></bash-stdout><bash-stderr>=== RUN TestSmoke\nFAIL\n</bash-stderr>';
const message = text => ({ id: 'manual-1', role: 'user', time: '', parts: [{ kind: 'text', text }] });
const ref = { agent: 'claude-code', id: 'session' };

test('recognizes screenshot-shaped commands/output and preserves literal payload whitespace', () => {
  assert.deepEqual(parseManualBash(command), { kind: 'input', command: ' CD_PATH="/tmp/bin:$PATH" go test -v ./...\n' });
  assert.deepEqual(parseManualBash(output), { kind: 'output', stdout: '', stderr: '=== RUN TestSmoke\nFAIL\n' });
  assert.deepEqual(parseManualBash('<bash-stdout>  aligned\n\n</bash-stdout>\n<bash-stderr>warning</bash-stderr>'), { kind: 'output', stdout: '  aligned\n\n', stderr: 'warning' });
  assert.deepEqual(parseManualBash('<bash-stdout></bash-stdout><bash-stderr></bash-stderr>'), { kind: 'output', stdout: '', stderr: '' });
});

test('rejects quoted/malformed/mixed wrappers and non-Claude or non-user messages', () => {
  for (const text of [
    'Explain <bash-input>ls</bash-input>', '`<bash-input>ls</bash-input>`',
    '```\n<bash-input>ls</bash-input>\n```', '<bash-input>ls',
    '<bash-input>ls</bash-input> trailing', '<bash-input>a</bash-input><bash-input>b</bash-input>',
    '<bash-stdout>ok</bash-stdout>', '<bash-stderr>err</bash-stderr>',
    '<bash-stdout>ok</bash-stdout>prose<bash-stderr>err</bash-stderr>',
    '<bash-stdout>ok</bash-stdout><bash-stderr>err</bash-stderr> trailing',
  ]) assert.equal(parseManualBash(text), null, text);
  for (const agent of ['codex', 'opencode']) assert.equal(manualBashMessage(message(command), { ...ref, agent }), null);
  for (const role of ['assistant', 'system']) assert.equal(manualBashMessage({ ...message(command), role }, ref), null);
  assert.equal(manualBashMessage({ ...message(command), parts: [...message(command).parts, { kind: 'file', file: { name: 'x' } }] }, ref), null);
  assert.equal(manualBashMessage({ ...message(command), parts: [...message(command).parts, { kind: 'text', text: 'prose' }] }, ref), null);
  assert.equal(manualBashMessage({ ...message(command), parts: null }, ref), null);
});

test('manual records stay visible in Chat/Activity/All without mutating IDs, content or requiring a paired message', () => {
  for (const text of [command, output]) {
    const original = message(text);
    const before = structuredClone(original);
    for (const mode of ['chat', 'activity', 'all']) assert.deepEqual(visibleTranscriptParts(original, mode), original.parts);
    assert.ok(manualBashMessage(original, ref));
    assert.deepEqual(original, before);
  }
});

test('cards label manual attribution, omit empty streams and never claim an exit status', () => {
  const input = render(Card, { props: { record: parseManualBash(command) } }).body;
  assert.match(input, /Run by you/);
  assert.match(input, /aria-label="Copy command"/);
  assert.doesNotMatch(input, /bash-input/);
  const html = render(Card, { props: { record: parseManualBash(output) } }).body;
  assert.match(html, /stderr/);
  assert.match(html, /aria-label="Copy stderr"/);
  assert.doesNotMatch(html, /stdout|bash-stderr|completed|status-error|exit code/);
  const empty = render(Card, { props: { record: parseManualBash('<bash-stdout></bash-stdout><bash-stderr></bash-stderr>') } }).body;
  assert.match(empty, /No output/);
  assert.doesNotMatch(empty, /<pre/);
});

test('HTML/Markdown-like terminal output is escaped inert text, not markup', () => {
  const html = render(Card, { props: { record: { kind: 'output', stdout: '<script>alert(1)</script> **literal** <img src=x onerror=alert(1)>', stderr: '' } } }).body;
  assert.match(html, /&lt;script>alert\(1\)&lt;\/script>/);
  assert.match(html, /\*\*literal\*\*/);
  assert.doesNotMatch(html, /<script|<img|<strong>literal/);
});

test('long multiline and single-line output renders a small preview and search can reveal it immediately', () => {
  for (const text of ['line\n'.repeat(100) + 'TAIL', 'x'.repeat(12000) + 'TAIL']) {
    const preview = bashPreview(text);
    assert.equal(preview.clipped, true);
    assert.ok(preview.text.length <= 6000);
    const props = { record: { kind: 'output', stdout: text, stderr: '' } };
    const collapsed = render(Card, { props }).body;
    assert.match(collapsed, /Show full text/);
    assert.doesNotMatch(collapsed, /TAIL/);
    const expanded = render(Card, { props: { ...props, defaultExpanded: true } }).body;
    assert.match(expanded, /TAIL/);
  }
  const css = compile(readFileSync(url, 'utf8'), { filename: fileURLToPath(url), generate: 'client' }).css.code;
  assert.match(css, /max-height:\s*350px/);
  assert.match(css, /overflow:\s*auto/);
  assert.match(css, /white-space:\s*pre/);
  assert.match(css, /var\(--font-mono\)/);
  assert.match(css, /var\(--type-code\)/);
});

test('copy/print Markdown is wrapper-free and uses fences longer than payload backticks', () => {
  assert.equal(manualBashMarkdown(parseManualBash(command)), '```bash\n CD_PATH="/tmp/bin:$PATH" go test -v ./...\n\n```');
  const result = manualBashMarkdown({ kind: 'output', stdout: '```html\n<script>x</script>\n```', stderr: 'warn' });
  assert.ok(result.startsWith('**stdout**\n\n````\n'));
  assert.match(result, /\*\*stderr\*\*/);
  assert.doesNotMatch(result, /bash-stdout|bash-stderr/);
});

test('real MessageBubble dispatches only dedicated Claude user records to the Bash card', async () => {
  const bubbleUrl = new URL('../src/lib/components/transcript/MessageBubble.svelte', import.meta.url);
  const source = readFileSync(bubbleUrl, 'utf8');
  globalThis.__bashCard = Card;
  const compiled = compile(source, { filename: fileURLToPath(bubbleUrl), generate: 'server' }).js.code;
  const imports = [...compiled.matchAll(/^import ([\s\S]+?) from '([^']+)';/gm)];
  const swaps = imports.filter(([, , path]) => path !== 'svelte/internal/server').map(([statement, name, path]) => {
    let replacement;
    if (path === '../../manualBash' || path === '../../transcript') {
      replacement = `import ${name} from ${JSON.stringify(new URL(`../src/lib/${path.split('/').pop()}.ts`, import.meta.url).href)};`;
    } else if (name === 'ManualBashPart') replacement = 'const ManualBashPart = globalThis.__bashCard;';
    else if (path.endsWith('.svelte') && !path.includes('/stores/')) replacement = `const ${name.split(',')[0]} = ($$renderer, $$props) => { if ($$props.text) $$renderer.push('ORDINARY_TEXT'); };`;
    else if (path.includes('preferences')) replacement = 'const preferences = { timeZone: "utc" };';
    else if (path.includes('translate.svelte')) replacement = 'const translateSettings = { language: "English", configured: false };';
    else if (path.includes('translations.svelte')) replacement = 'const translations = { get: () => null }; const translationKey = () => "key";';
    else if (path.includes('selectionTranslation')) replacement = 'const captureSelection = () => null, holdSelection = () => {}, selectionMenuItems = () => [];';
    else if (name.startsWith('{')) replacement = `const ${name.replace(/[{}]/g, '').split(',').map(n => n.trim()).filter(Boolean).map(n => `${n} = () => {}`).join(', ')};`;
    else replacement = `const ${name} = {};`;
    return [statement.trim(), replacement];
  });
  const { default: Bubble } = await loadServerComponent(bubbleUrl, swaps);
  try {
    for (const mode of ['chat', 'activity', 'all']) {
      const html = render(Bubble, { props: { message: message(output), sessionRef: ref, mode } }).body;
      assert.match(html, /Bash output/);
      assert.match(html, /stderr/);
      assert.doesNotMatch(html, /ORDINARY_TEXT|bash-stderr/);
    }
    for (const props of [
      { message: message(command), sessionRef: { ...ref, agent: 'codex' } },
      { message: message('Explain ' + command), sessionRef: ref },
      { message: { ...message(command), role: 'assistant' }, sessionRef: ref },
    ]) {
      const html = render(Bubble, { props }).body;
      assert.match(html, /ORDINARY_TEXT/);
      assert.doesNotMatch(html, /Manually run Bash command/);
    }
  } finally { delete globalThis.__bashCard; }
});

// Drive the actual client component for disclosure, full-text copying and errors.
const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
let code = compile(readFileSync(url, 'utf8'), { filename: fileURLToPath(url), generate: 'client', css: 'external' }).js.code
  .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
  .replace("'svelte/internal/client'", client('internal/client/index.js'))
  .replace("import Icon from '../../common/Icon.svelte';", 'const Icon = () => {};');
for (const [from, to] of replacements) code = code.replace(from, to);
const ClientCard = (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;

test('disclosure reveals full output; copy uses full original stream and reports clipboard failure', async () => {
  const text = 'line\n'.repeat(100) + 'TAIL';
  const target = document.createElement('div');
  document.body.append(target);
  const copied = [];
  globalThis.__bashCopy = value => { copied.push(value); return true; };
  const component = mount(ClientCard, { target, props: { record: { kind: 'output', stdout: text, stderr: 'warning' } } });
  flushSync();
  try {
    assert.ok(!target.querySelector('pre').textContent.includes('TAIL'));
    target.querySelector('[aria-label="Copy stdout"]').click();
    await Promise.resolve(); flushSync();
    assert.deepEqual(copied, [text]);
    assert.equal(target.querySelector('[aria-label="Copy stdout"]').textContent, 'Copied');
    target.querySelector('.disclosure').click(); flushSync();
    assert.equal(target.querySelector('pre').textContent, text);
    assert.equal(target.querySelector('.disclosure').getAttribute('aria-expanded'), 'true');
    target.querySelector('.disclosure').click(); flushSync();
    assert.ok(!target.querySelector('pre').textContent.includes('TAIL'));
    globalThis.__bashCopy = () => false;
    target.querySelector('[aria-label="Copy stderr"]').click();
    await Promise.resolve(); flushSync();
    assert.match(target.querySelector('[role="alert"]').textContent, /Could not copy/);
  } finally {
    await unmount(component); target.remove(); delete globalThis.__bashCopy;
  }
});
