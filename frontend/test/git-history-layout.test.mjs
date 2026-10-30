import test from 'node:test';
import assert from 'node:assert/strict';
import { loadServerComponent, renderWithFixture } from './helpers.mjs';

const base = new URL('../src/lib/components/common/go-to-file/', import.meta.url);
const layout = await loadServerComponent(new URL('HistoryLayout.svelte', base), [
  ["import Splitter from '../Splitter.svelte';", 'const Splitter = () => {};'],
]);

test('history layout renders the agreed three-pane browser and closable bottom diff', async () => {
  globalThis.localStorage = { getItem: key => ({ 'git-history:branches': '99', 'git-history:details': '1', 'git-history:diff': 'bad' })[key] ?? null, setItem() {} };
  const closed = await renderWithFixture(layout, {}, {});
  assert.match(closed, /aria-label="History branches"/);
  assert.match(closed, /aria-label="Commit history"/);
  assert.match(closed, /aria-label="Changed files and commit details"/);
  assert.doesNotMatch(closed, /Historical file diff/);
  // Stored sizes are bounded; malformed sizes fall back to agreed proportions.
  assert.match(closed, /width: 30%/);
  assert.match(closed, /width: 22%/);
  const open = await renderWithFixture(layout, {}, { diffOpen: true });
  assert.match(open, /Historical file diff/);
  assert.match(open, /Close historical diff/);
  assert.match(open, /flex-basis: 50%/);
  delete globalThis.localStorage;
});
