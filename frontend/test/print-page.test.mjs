// The printed page keeps code blocks readable on white paper.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('../src/lib/components/common/PrintPage.svelte', import.meta.url), 'utf8');

test('printed code blocks keep a tinted box, without the label or Copy button', () => {
  // The page's own --code-bg is white; a message tints it, and so does the print page.
  assert.match(source, /\.print-page \{[^}]*--code-bg: var\(--bg-secondary\);/);
  // The tint sets a block off; it needs no outline.
  assert.match(source, /\.print-page :global\(\.code-block-container\) \{\s*break-inside: avoid;\s*\}/);
  assert.match(source, /\.print-page :global\(\.code-block-header\) \{\s*display: none;/);
  // A drawn diagram prints bare.
  assert.match(source, /\.print-page :global\(\.mermaid-block\.drawn\) \{[^}]*border: none;[^}]*background: none;/);
  const sheet = readFileSync(new URL('../src/lib/components/common/PrintSheet.svelte', import.meta.url), 'utf8');
  assert.match(sheet, /print-color-adjust: exact;\s*-webkit-print-color-adjust: exact;/, 'backgrounds print');
});
