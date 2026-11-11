import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

test('only Linux forces a session-list compositing layer for scrollbar stability', () => {
  const url = new URL('../src/lib/components/sessionlist/VirtualList.svelte', import.meta.url);
  const source = readFileSync(url, 'utf8');
  const css = compile(source, { generate: 'client', filename: url.pathname }).css.code;
  const container = css.match(/\.virtual-list-container[^{}]*\{([^}]*)\}/);
  assert.ok(container);
  assert.match(container[1], /overflow-y: auto/);
  assert.doesNotMatch(container[1], /transform:/, 'macOS and Windows do not force a surface');
  assert.match(css, /\[data-platform="linux"\] \.virtual-list-container[^{}]*\{[^}]*transform: translateZ\(0\)/);
});
