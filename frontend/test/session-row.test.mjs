import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';

const url = new URL('../src/lib/components/sessionlist/SessionRow.svelte', import.meta.url);
const component = await loadServerComponent(url, [
  ["'../../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
  ["import CountTooltip from './CountTooltip.svelte';", 'const CountTooltip = () => {};'],
  ["import AttentionDot from '../common/AttentionDot.svelte';", 'const AttentionDot = () => {};'],
  ["import { sessionRename } from '../../stores/sessionRename.svelte';", 'const sessionRename = { busy: false };'],
  ...['manage', 'preferences', 'terminals', 'link'].map(name => [
    `import { ${name} } from '../../stores/${name}.svelte';`, fixtureStore(name),
  ]),
]);

async function renderRow(parentId, isSelected = false, title = 'Session title', { manage = {}, ...props } = {}) {
  return renderWithFixture(component, {
    manage: { settings: { enabled: false }, isRefSelected: () => false, selectedRefKeys: new Set(), ...manage },
    preferences: { timeZone: 'utc' },
    terminals: { tabFor: () => undefined },
    link: { dataHost: null },
  }, {
    session: { ref: { agent: 'claude-code', id: 'session' }, parentId, title, firstPrompt: 'Prompt fallback', counts: { user: 1, assistant: 2 }, updatedAt: '2026-11-03T00:00:00Z' },
    isSelected, onSelect() {}, ...props,
  });
}

test('only subagent session titles are marked as subagents, even when selected', async () => {
  for (const selected of [false, true]) {
    const child = await renderRow('parent', selected);
    assert.match(child, /class="title-text[^\"]*subagent/);
    assert.match(child, /title="Session title"/);
    const root = await renderRow(undefined, selected);
    assert.doesNotMatch(root, /class="title-text[^\"]*subagent/);
  }
  const fallback = await renderRow('parent', false, '');
  assert.match(fallback, /class="title-text[^\"]*subagent[^>]*title="Prompt fallback"/);
  // An arrow marks the subagent, not italics.
  assert.match(fallback, /aria-label="Subagent"/);
  assert.doesNotMatch(await renderRow(undefined), /aria-label="Subagent"/);
  const source = readFileSync(url, 'utf8');
  assert.doesNotMatch(source, /font-style:\s*italic/);
});

test('rows show the agent mark unless the list is filtered to one agent', async () => {
  assert.match(await renderRow(undefined), /class="agent-icon[^"]*"[^>]*title="Claude Code"/);
  const filtered = await renderRow(undefined, false, 'Session title', { showAgent: false });
  assert.doesNotMatch(filtered, /agent-icon/);
  assert.doesNotMatch(filtered, /class="lead/);
});

test('manage checkboxes stay quiet until a row is hovered or a session is picked', async () => {
  assert.doesNotMatch(await renderRow(undefined), /bulk-check/);
  const idle = await renderRow(undefined, false, 'Session title', { manage: { settings: { enabled: true } } });
  assert.match(idle, /class="lead[^"]*checkable/);
  assert.doesNotMatch(idle, /class="lead[^"]*checking/);
  assert.match(idle, /class="bulk-check[^"]*"[^>]*aria-label="Select session Session title"/);
  const picking = await renderRow(undefined, false, 'Session title', { manage: { settings: { enabled: true }, selectedRefKeys: new Set(['other']) } });
  assert.match(picking, /class="lead[^"]*checking/);
  const source = readFileSync(url, 'utf8');
  assert.match(source, /\.lead \.bulk-check\s*\{[^}]*opacity: 0;/);
  assert.match(source, /\.session-row:hover \.lead\.checkable \.bulk-check,[^{]*\.lead\.checking \.bulk-check\s*\{\s*opacity: 1;/);
});
