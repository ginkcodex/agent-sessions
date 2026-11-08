import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('session list scroller keeps its own compositing layer for the Linux scrollbar', () => {
  const source = readFileSync(new URL('../src/lib/components/sessionlist/VirtualList.svelte', import.meta.url), 'utf8');
  const container = source.match(/\.virtual-list-container \{([^}]*)\}/);
  assert.ok(container, 'precondition: the scroll container has a style rule');
  assert.match(container[1], /overflow-y: auto/);
  // WebKitGTK loses the scrollbar near the top and flashes it on row hover without this layer.
  assert.match(container[1], /transform: translateZ\(0\)/);
});
