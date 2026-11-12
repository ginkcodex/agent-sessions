import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerModule } from './helpers.mjs';

const { FilePromptStore } = await loadServerModule(new URL('../src/lib/stores/filePrompt.svelte.ts', import.meta.url), []);

test('file prompts serialize decisions and cancel overlapping actions', async () => {
  const p = new FilePromptStore();
  const first = p.ask('Discard?', 'draft', [{ value: 'discard', label: 'Discard' }]);
  assert.equal(p.request.title, 'Discard?');
  assert.deepEqual(p.request.groups, [], 'a prompt may name no items');
  assert.equal(await p.ask('Close?', 'other', []), null);
  p.answer('discard');
  assert.equal(await first, 'discard');
  assert.equal(p.request, null);
  const groups = [{ title: 'Chats', icon: 'chat', items: [{ name: 'Codex', detail: 'in api', icon: 'chat', agent: 'codex', badge: 'working' }] }];
  const cancelled = p.ask('Close?', 'draft', [], groups);
  assert.deepEqual(p.request.groups, groups, 'the cards ride along with the question');
  p.answer(null);
  assert.equal(await cancelled, null);
  p.answer('duplicate');
  assert.equal(p.request, null);
});

test('the prompt keeps focus and owns find/save/close shortcuts while it decides', async () => {
  const { readFileSync } = await import('node:fs');
  const prompt = readFileSync(new URL('../src/lib/components/common/go-to-file/FilePrompt.svelte', import.meta.url), 'utf8');
  assert.match(prompt, /document\.addEventListener\('focusin', contain\)/);
  assert.match(prompt, /document\.removeEventListener\('focusin', contain\)/);
  assert.match(prompt, /\['f', 's', 'w'\]\.includes\(e\.key\.toLowerCase\(\)\)\) e\.preventDefault\(\)/);
  const editor = readFileSync(new URL('../src/lib/components/common/go-to-file/FileEditor.svelte', import.meta.url), 'utf8');
  assert.match(editor, /if \(!filePrompt\.request\) editor\.focus\(\)/, 'a lazily loaded editor must not take focus from an open prompt');
});

test('the prompt shows item groups as counted cards, and none without items', async () => {
  const { readFileSync } = await import('node:fs');
  const prompt = readFileSync(new URL('../src/lib/components/common/go-to-file/FilePrompt.svelte', import.meta.url), 'utf8');
  assert.match(prompt, /\{#if filePrompt\.request\.groups\.length\}/, 'a prompt without items renders as before');
  assert.match(prompt, /\{#each filePrompt\.request\.groups as group\}/);
  assert.match(prompt, /group\.items\.length \+ \(group\.more \?\? 0\)/, 'the count sums up the overflow too');
  assert.match(prompt, /<AgentIcon agent=\{item\.agent\}/, 'a row shows its agent\'s own glyph where known');
  assert.match(prompt, /const GROUP_ICONS = \{ terminal: 'square-terminal', chat: 'message-square', file: 'file' \}/,
    'each heading shows its kind');
  assert.match(prompt, /const ROW_ICONS = \{ terminal: 'square-terminal', agent: 'bot', chat: 'message-square', file: 'file' \}/,
    'shell, chat and file rows are told apart by icon');
  assert.match(prompt, /\{#if item\.badge\}<span class="badge">\{item\.badge\}<\/span>\{\/if\}/);
  assert.match(prompt, /and \{group\.more\} more/);
});

test('the prompt widens for its buttons so each label stays on one line', async () => {
  const { readFileSync } = await import('node:fs');
  const prompt = readFileSync(new URL('../src/lib/components/common/go-to-file/FilePrompt.svelte', import.meta.url), 'utf8');
  assert.match(prompt, /\.file-prompt \{ width: min\(480px, 100%\); min-width: min-content;/, '480px unless the button row needs more');
  assert.match(prompt, /\n  button \{ flex: none; white-space: nowrap;/, 'a label never wraps');
  assert.match(prompt, /@media \(max-width: 640px\) \{[^}]*\.file-prompt \{ min-width: 0; \}/, 'a narrow window wraps the buttons instead');
});
