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

test('rows always show the agent mark, even when the list is filtered to one agent', async () => {
  assert.match(await renderRow(undefined), /class="agent-icon[^"]*"[^>]*title="Claude Code"/);
  // The name stays beside the mark, so it still shows while a checkbox covers the mark.
  assert.match(await renderRow(undefined), /class="agent-name[^"]*">Claude Code</);
  const managing = await renderRow(undefined, false, 'Session title', { manage: { settings: { enabled: true }, selectedRefKeys: new Set(['other']) } });
  assert.match(managing, /class="lead[^"]*checking[\s\S]*class="agent-name[^"]*">Claude Code</);
  // It keeps the old badge's tint: the agent's color on a faint wash of it.
  assert.match(readFileSync(url, 'utf8'), /\.agent-name\s*\{[^}]*color: var\(--row-agent\);[^}]*background-color: color-mix\(in srgb, var\(--row-agent\) 15%, transparent\);/);
  const list = readFileSync(new URL('../src/lib/components/sessionlist/SessionList.svelte', import.meta.url), 'utf8');
  assert.doesNotMatch(list, /showAgent/, 'an agent filter does not hide the mark');
});

test('rows are tree items: nested by level, with a subagent toggle only on parents', async () => {
  const top = await renderRow(undefined);
  assert.match(top, /role="treeitem"[^>]*aria-level="1"/);
  assert.doesNotMatch(top, /aria-expanded|subagent-toggle|class="session-row[^"]*nested/);

  const parent = await renderRow(undefined, false, 'Session title', { childCount: 3 });
  assert.match(parent, /role="treeitem"[^>]*aria-expanded="false"/);
  assert.match(parent, /class="subagent-toggle[^"]*"[^>]*aria-expanded="false"[^>]*>[\s\S]*?3 subagents/);
  const open = await renderRow(undefined, false, 'Session title', { childCount: 1, expanded: true });
  assert.match(open, /class="subagent-toggle[^"]*open[^"]*"[^>]*aria-expanded="true"[^>]*>[\s\S]*?1 subagent</);

  // Deep rows keep their real level but stop indenting after three.
  const deep = await renderRow('parent', false, 'Session title', { depth: 5 });
  assert.match(deep, /class="session-row[^"]*nested[^"]*"[^>]*style="--indent: 3;"[^>]*aria-level="6"/);
  const source = readFileSync(url, 'utf8');
  assert.match(source, /onclick=\{handleToggle\}\s*onkeydown=\{handleToggleKeydown\}/);
  assert.match(source, /function handleToggle\(e: MouseEvent\) \{\s*e\.stopPropagation\(\);/, 'toggling does not open the session');
});

test('a context parent is dimmed and says why it is listed', async () => {
  const html = await renderRow(undefined, false, 'Session title', { context: true, childCount: 1, expanded: true });
  assert.match(html, /class="session-row[^"]*context[^"]*"[^>]*title="Does not match the filters; shown because a subagent under it does"/);
  assert.doesNotMatch(await renderRow(undefined), /class="session-row[^"]*context/);
});

test('a subagent under a picked parent shows checked and locked', async () => {
  const html = await renderRow('parent', false, 'Session title', { manage: { settings: { enabled: true } }, covered: true });
  assert.match(html, /class="session-row[^"]*bulk-selected/);
  assert.match(html, /class="lead[^"]*checking/);
  assert.match(html, /class="bulk-check[^"]*"[^>]*checked[^>]*disabled[^>]*title="Deleted along with its parent"/);
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
