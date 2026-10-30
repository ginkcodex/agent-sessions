import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';

const changedFiles = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/HistoryChangedFiles.svelte', import.meta.url), [
  ["import FileIcon from '../FileIcon.svelte';", 'const FileIcon = () => {};'],
]);
globalThis.__historyChangedFiles = changedFiles.default;
const component = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/HistoryDetails.svelte', import.meta.url), [
  ["import { gitHistory } from '../../../stores/gitHistory.svelte';", fixtureStore('gitHistory')],
  ["import { copyToClipboard } from '../../../portable';", 'const copyToClipboard = async () => true;'],
  ["import { toast } from '../../../stores/toast.svelte';", 'const toast = {};'],
  ["import HistoryChangedFiles from './HistoryChangedFiles.svelte';", 'const HistoryChangedFiles = globalThis.__historyChangedFiles;'],
  ["'../../../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
]);
const person = { name: 'Author <script>', email: 'author@example.test', time: 1700000000 };
const detail = { oid: 'a'.repeat(40), subject: 'Commit', message: '<script>alert(1)</script>\n\nBody', author: person,
  committer: person, parents: ['b'.repeat(40), 'c'.repeat(40)], base: 'b'.repeat(40), truncated: true,
  changes: [{ path: 'new/name.txt', oldPath: 'old/name.txt', status: 'R100', oldMode: '100644', newMode: '100644' },
    { path: 'deleted.txt', status: 'D', oldMode: '100644', newMode: '000000' }] };
function render(state = {}, props = {}) {
  return renderWithFixture(component, { gitHistory: state }, { onOpen() {}, ...props });
}

test('commit details show metadata, escaped full message, first parent and renamed/deleted paths', async () => {
  const html = await render({ detail }, { selectedPath: 'new/name.txt' });
  assert.match(html, /Changed files/);
  assert.match(html, /Some changed files are not listed/);
  assert.match(html, /from old\/name.txt/);
  assert.match(html, /name.txt/);
  assert.match(html, /aria-label="Renamed"/);
  assert.match(html, /aria-label="Deleted"/);
  assert.match(html, /aria-selected="true"/);
  assert.match(html, /First parent \(merge; other parents are not compared\)/);
  assert.match(html, /Copy hash/);
  assert.match(html, /role="tree"/);
  assert.match(html, /aria-expanded="true"/);
  assert.match(html, /aria-level="2"/);
  assert.ok(html.includes(detail.parents[1]));
  assert.match(html, /Author &lt;script/);
  assert.match(html, /&lt;script>alert\(1\)&lt;\/script>/);
  assert.doesNotMatch(html, /<script>alert/);
});

test('changed files read as icon, name, origin, then a labelled status badge', async () => {
  const changes = [
    { path: 'src/a.ts', status: 'A' }, { path: 'src/b.ts', status: 'M' }, { path: 'src/copy.ts', oldPath: 'src/b.ts', status: 'C100' },
    { path: 'run.sh', status: 'T' },
  ];
  const html = (await render({ detail: { ...detail, truncated: false, changes } }, { selectedPath: 'src/b.ts' })).replace(/<!--.*?-->/g, '');
  assert.match(html, /Changed files <span class="count[^"]*">4<\/span>/);
  assert.match(html, /aria-expanded="true" data-tree-row="" data-dir="src"[^>]*>[\s\S]*?class="dir-name[^"]*" title="src">src<\/span><span class="count[^"]*">3<\/span>/);
  for (const [status, label] of [['A', 'Added'], ['M', 'Modified'], ['C', 'Copied'], ['T', 'Type changed']])
    assert.match(html, new RegExp(`<span class="status status-${status}[^"]*" aria-label="${label}">${status}</span></button>`));
  assert.match(html, /class="file-name status-C[^"]*">copy\.ts<\/span><span class="old-path[^"]*">from src\/b\.ts<\/span><\/span>/);
  assert.match(html, /data-file="run\.sh"[^>]*aria-level="1"|aria-level="1"[^>]*data-file="run\.sh"/);
  assert.match(html, /data-dir="src" data-file="src\/b\.ts"/);
  assert.doesNotMatch(html, /Conflict/);
});

test('root and scoped empty commit details explain the comparison', async () => {
  const html = await render({ detail: { ...detail, base: '', parents: [], changes: [], truncated: false } });
  assert.match(html, /No changed files in this workspace scope/);
  assert.match(html, /None \(root commit\)/);
  assert.match(html, /Empty tree \(root commit\)/);
  assert.doesNotMatch(html, /Some changed files are not listed/);
});

test('commit details distinguish loading, errors with retry and no selection', async () => {
  assert.match(await render({ detailLoading: true }), /Loading commit details/);
  assert.match(await render({ detailError: 'Snapshot was removed', selectedCommit: detail.oid }), /Retry details/);
  assert.match(await render({}), /Select a commit/);
});

const comparison = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/HistoryComparison.svelte', import.meta.url), [
  ["import { gitHistory } from '../../../stores/gitHistory.svelte';", fixtureStore('gitHistory')],
  ["import HistoryChangedFiles from './HistoryChangedFiles.svelte';", 'const HistoryChangedFiles = globalThis.__historyChangedFiles;'],
]);
test('working tree comparison labels the ref and saved disk without fake commit metadata', async () => {
  const state = { comparisonLabel: 'main → working tree', comparison: { ref: 'refs/heads/main', oid: 'a', commit: 'a', token: 't', truncated: false,
    changes: [{ path: 'src/a.ts', status: 'M', oldMode: '100644', newMode: '100644' }] } };
  const html = await renderWithFixture(comparison, { gitHistory: state }, { onOpen() {}, selectedPath: 'src/a.ts' });
  assert.match(html, /main → working tree/);
  assert.match(html, /Unsaved editor buffers are not compared/);
  assert.match(html, /Refresh comparison/);
  assert.match(html, /aria-selected="true"/);
  assert.doesNotMatch(html, /Commit details|Author|Parents/);
  assert.match(await renderWithFixture(comparison, { gitHistory: { comparisonLabel: 'x', comparisonError: 'gone' } }, { onOpen() {} }), /role="alert"[^>]*>gone/);
});
