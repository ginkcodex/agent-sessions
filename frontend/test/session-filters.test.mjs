// The session list's subagent toggle: the filter it sends and how the mock
// backend applies it to the list, the tree and the agent counts.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const sort = { field: 'updated', desc: true };

test('hideSubagents leaves subagent sessions out of the list, the tree and the counts', async () => {
  const api = new MockBackendAPI();
  const [parent] = api.sessions;
  const child = { ...parent, ref: { agent: parent.ref.agent, id: 'mock-subagent' }, title: 'Mock subagent', parentId: parent.ref.id, archived: false };
  api.sessions = [...api.sessions, child];
  const ids = sessions => sessions.map(s => s.ref.id);
  const treeIds = nodes => nodes.flatMap(n => [...(n.sessions ?? []).map(r => r.id), ...treeIds(n.children ?? [])]);

  assert.ok(ids(await api.listSessions('', {}, sort)).includes('mock-subagent'));
  const shown = await api.agentCounts({});
  assert.ok(treeIds(await api.listGroups('dir-agent', {})).includes('mock-subagent'));

  const hidden = await api.listSessions('', { hideSubagents: true }, sort);
  assert.ok(!ids(hidden).includes('mock-subagent'));
  assert.ok(ids(hidden).includes(parent.ref.id), 'the parent stays');
  assert.ok(!treeIds(await api.listGroups('dir-agent', { hideSubagents: true })).includes('mock-subagent'));
  assert.equal((await api.agentCounts({ hideSubagents: true }))[parent.ref.agent], shown[parent.ref.agent] - 1);
});

test('the toolbar collapses every open subagent family instead of filtering subagents out', () => {
  const list = readFileSync(new URL('../src/lib/components/sessionlist/SessionList.svelte', import.meta.url), 'utf8');
  assert.match(list, /aria-label="Collapse all subagent sessions"[\s\S]*?disabled=\{!anyExpanded\}[\s\S]*?onclick=\{collapseAll\}/);
  assert.doesNotMatch(list, /setFilter\(\{ hideSubagents: !/, 'subagents stay listed, under their parents');
  // A hideSubagents filter set through the API still narrows and is cleared.
  assert.match(list, /appState\.filter\.hideSubagents \|\| appState\.filter\.archived/);
  assert.match(list, /appState\.setFilter\(\{ query: '', liveOnly: false, archived: false, hideSubagents: false \}\)/);
});
