import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';

const url = new URL('../src/lib/components/sessionlist/SessionRow.svelte', import.meta.url);
const component = await loadServerComponent(url, [
  ["'../../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
  ["import CountTooltip from './CountTooltip.svelte';", 'const CountTooltip = () => {};'],
  ["import AttentionDot from '../common/AttentionDot.svelte';", 'const AttentionDot = () => {};'],
  ...['manage', 'preferences', 'terminals', 'link'].map(name => [
    `import { ${name} } from '../../stores/${name}.svelte';`, fixtureStore(name),
  ]),
]);

async function renderRow(parentId, isSelected = false, title = 'Session title') {
  return renderWithFixture(component, {
    manage: { settings: { enabled: false }, isRefSelected: () => false },
    preferences: { timeZone: 'utc' },
    terminals: { tabFor: () => undefined },
    link: { dataHost: null },
  }, {
    session: { ref: { agent: 'claude-code', id: 'session' }, parentId, title, firstPrompt: 'Prompt fallback', counts: { user: 1, assistant: 2 }, updatedAt: '2026-11-03T00:00:00Z' },
    isSelected, onSelect() {},
  });
}

test('only subagent session titles are marked for dimmed italic styling, even when selected', async () => {
  for (const selected of [false, true]) {
    const child = await renderRow('parent', selected);
    assert.match(child, /class="title-text[^\"]*subagent/);
    assert.match(child, /title="Session title"/);
    const root = await renderRow(undefined, selected);
    assert.doesNotMatch(root, /class="title-text[^\"]*subagent/);
  }
  const fallback = await renderRow('parent', false, '');
  assert.match(fallback, /class="title-text[^\"]*subagent[^>]*title="Prompt fallback"/);
  const source = readFileSync(url, 'utf8');
  assert.match(source, /\.title-text\.subagent\s*\{[^}]*font-style:\s*italic;[^}]*color:\s*var\(--text-muted\)/);
});
