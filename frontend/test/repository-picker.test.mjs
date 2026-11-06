// The Repository pickers in Changes and History use the app's dropdown, as
// Settings does, not the platform's select.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { repositoryOptions } from '../src/lib/gitHistory.ts';
import { loadServerComponent, renderWithFixture } from './helpers.mjs';

const base = new URL('../src/lib/components/common/', import.meta.url);
const read = (path) => readFileSync(new URL(path, base), 'utf8');

test('the project\'s own repository goes by its name, the others by their path', () => {
  const own = { path: '.', name: 'agent-sessions', scope: '' };
  const inner = [{ path: 'web/app', name: 'app', scope: '' }];
  assert.deepEqual(repositoryOptions([own, ...inner]), [
    { value: '.', label: 'agent-sessions', title: 'agent-sessions' },
    { value: 'web/app', label: 'web/app', title: 'web/app' },
  ]);
  // Changes keeps its own repository apart, as the empty choice.
  assert.deepEqual(repositoryOptions(inner, own).map((o) => [o.value, o.label]), [['', 'agent-sessions'], ['web/app', 'web/app']]);
  assert.deepEqual(repositoryOptions([], null), []);
});

test('Changes and History pick the repository with the shared dropdown', () => {
  for (const file of ['go-to-file/ChangesPane.svelte', 'go-to-file/HistoryToolbar.svelte']) {
    const source = read(file);
    assert.match(source, /import Dropdown from '\.\.\/Dropdown\.svelte';/, file);
    assert.match(source, /<Dropdown ariaLabel="(Changes|History) repository" options=\{repositoryOptions\(/, file);
    assert.match(source, /placeholder="Choose a repository…"/, file);
    assert.doesNotMatch(source, /<select aria-label="(Changes|History) repository"/, file);
  }
});

test('the dropdown can be disabled, shows a placeholder and cuts long labels short', async () => {
  const dropdown = await loadServerComponent(new URL('Dropdown.svelte', base), []);
  const options = [{ value: 'a', label: 'a/very/long/path', title: 'a/very/long/path' }];
  let html = await renderWithFixture(dropdown, {}, { options, value: '', onChange() {}, ariaLabel: 'Repo', placeholder: 'Choose…', disabled: true });
  assert.match(html, /<button[^>]*disabled[^>]*>/);
  assert.match(html, /class="trigger-label[^"]* placeholder[^"]*">Choose…<\/span>/);
  html = await renderWithFixture(dropdown, {}, { options, value: 'a', onChange() {}, ariaLabel: 'Repo' });
  assert.match(html, /<button[^>]*title="a\/very\/long\/path"/, 'the whole label on hover');
  assert.doesNotMatch(html, /<button[^>]*disabled/);
  const source = read('Dropdown.svelte');
  assert.match(source, /\.trigger-label,\s*\.option-label \{[^}]*text-overflow: ellipsis;/);
  assert.match(source, /\.menu \{[^}]*max-height: min\(320px, 60vh\);[^}]*overflow-y: auto;/);
});
