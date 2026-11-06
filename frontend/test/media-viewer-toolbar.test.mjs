// The image and diagram viewer's toolbar matches the app's other buttons.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { icons } from '../src/lib/icons.ts';

const source = readFileSync(new URL('../src/lib/components/common/MediaViewer.svelte', import.meta.url), 'utf8');
const rule = (selector) => {
  const match = source.match(new RegExp(`\\n  ${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} \\{([^}]*)\\}`));
  assert.ok(match, `${selector} has a rule`);
  return match[1];
};

test('zoom out, the level and zoom in share a segmented track, with icons', () => {
  assert.match(source, /<div class="zoom-group" role="group" aria-label="Zoom">[\s\S]*?aria-label="Zoom out"[\s\S]*?<Icon name="minus"[\s\S]*?class="zoom-level"[\s\S]*?aria-label="Zoom in"[\s\S]*?<Icon name="plus"[\s\S]*?<\/div>/);
  assert.match(rule('.zoom-group'), /background-color: var\(--segment-track\);/);
  assert.doesNotMatch(source, />−<\/button>|>\+<\/button>/, 'no bare text glyphs');
});

test('Fit and Export are borderless buttons with icons; Fit shows when it is on', () => {
  assert.match(source, /class:active=\{fitted\} aria-pressed=\{fitted\}[^>]*>\s*<Icon name="scan"[^>]*\/>Fit/);
  assert.match(source, /<Icon name="download"[^>]*\/>\{exporting \? 'Exporting…' : 'Export'\}<Icon name="chevron-down"/);
  assert.match(source, /class:open=\{exportMenu !== null\}/);
  const button = rule('.viewer-btn');
  assert.match(button, /border-radius: var\(--radius-sm\);/);
  assert.match(rule('.viewer-btn,\n  .zoom-btn'), /border: none;[\s\S]*background-color: transparent;/);
  assert.match(source, /\.viewer-btn\.active,\s*\.viewer-btn\.active:hover \{[^}]*color: var\(--accent-color\);[^}]*background-color: var\(--active-bg\);/);
  assert.doesNotMatch(source, /border: 1px solid var\(--border-color\);\s*background-color: var\(--bg-primary\);/, 'no outlined boxes');
  for (const name of ['minus', 'plus', 'scan', 'download', 'chevron-down']) assert.ok(icons[name], `${name} icon`);
});
