// The printed page keeps code blocks readable on white paper.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fitTables } from '../src/lib/print.ts';

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

test('printed content keeps clear of the right edge macOS cuts off', () => {
  const sheet = readFileSync(new URL('../src/lib/components/common/PrintSheet.svelte', import.meta.url), 'utf8');
  // On screen too, so tables are fitted to the width that prints.
  assert.match(sheet, /^  \.print-sheet \{[^}]*padding-right: 2px;/m);
});

test('the print sheet is laid out at the paper width but unseen on screen', () => {
  const sheet = readFileSync(new URL('../src/lib/components/common/PrintSheet.svelte', import.meta.url), 'utf8');
  assert.match(sheet, /\.print-sheet \{[^}]*visibility: hidden;/);
  assert.match(sheet, /@media print \{\s*\.print-sheet \{[^}]*position: static;[^}]*visibility: visible;/);
  assert.match(sheet, /fitTables\(sheet\);\s*\n[\s\S]*api\.printWindow/, 'tables fit before the dialog opens');
});

test('a table too wide for the page wraps its headers, then its other cells', () => {
  // Fake layout: each wrap step narrows the table by its own amount.
  const box = (widths) => {
    const classes = new Set(['wrap-cells']);
    const table = {};
    const el = {
      clientWidth: 100,
      firstElementChild: table,
      classList: { add: (c) => classes.add(c), remove: (...cs) => cs.forEach((c) => classes.delete(c)) },
      classes,
    };
    Object.defineProperty(table, 'offsetWidth', {
      get: () => widths[classes.has('wrap-cells') ? 2 : classes.has('wrap-head') ? 1 : 0],
    });
    return el;
  };
  const fits = box([100, 100, 100]);
  const headersEnough = box([130, 100, 90]);
  const needsAll = box([130, 120, 100]);
  fitTables({ querySelectorAll: () => [fits, headersEnough, needsAll] });
  assert.deepEqual([...fits.classes], [], 'a table that fits keeps its one-line cells, and stale steps go');
  assert.deepEqual([...headersEnough.classes], ['wrap-head']);
  assert.deepEqual([...needsAll.classes], ['wrap-head', 'wrap-cells']);
});
