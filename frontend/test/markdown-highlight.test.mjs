import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { highlightFile, splitHighlightedLines } from '../src/lib/codeHighlight.ts';

test('Markdown filename underscores and code spans cannot leak emphasis into prose', async () => {
  const source = '| Windows | `agent-sessions_<version>_windows_amd64_setup.exe` |\n\nOn Linux, `uname -m` reports `x86_64` for amd64.\n\n1. Download **`_setup.exe`** installer. Use it for both new installations\n   and updates.\n2. Close Agent Sessions.\n\nPlain text afterwards.';
  const html = await highlightFile(source, 'markdown');
  assert.doesNotMatch(html, /hljs-emphasis/);
  assert.match(html, /hljs-code/);
  assert.match(html, /hljs-strong/);
  assert.equal(html.replace(/<[^>]*>/g, '').replace(/&lt;/g, '<').replace(/&gt;/g, '>'), source);
});

test('Markdown keeps intentional emphasis and protects escaped and intraword underscores', async () => {
  const html = await highlightFile('*italic* and **bold** and snake_case_name and \\_literal\\_', 'markdown');
  assert.match(html, /class="hljs-emphasis"/);
  assert.match(html, /class="hljs-strong"/);
  assert.match(html, /snake_case_name and \\_literal\\_/);
});

test('README Windows installation prose does not inherit italics', async () => {
  const source = readFileSync(new URL('../../README.md', import.meta.url), 'utf8');
  const lines = splitHighlightedLines(await highlightFile(source, 'markdown'));
  const original = source.split('\n');
  for (const text of ['2. Close Agent Sessions', 'The app installs for the current user', 'The installer is **not code-signed**']) {
    const index = original.findIndex(line => line.startsWith(text));
    assert.ok(index >= 0);
    assert.doesNotMatch(lines[index], /hljs-emphasis/);
  }
});
