import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

// The type scale (docs/plan/font-settings.md): every text size in session
// content and the project docs preview is body × one of these ratios.
const SCALE = {
  '--type-h1': 1.43,
  '--type-h2': 1.25,
  '--type-h3': 1.08,
  '--type-body': 1,
  '--type-code': 0.92,
  '--type-small': 0.83,
  '--type-label': 0.79,
  '--type-tag': 0.75,
  '--type-inline-code': 0.85,
};

const read = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

/** Unitless `--type-*` custom properties declared in style.css's first `:root`. */
function typeTokens() {
  const root = read('style.css').match(/:root\s*\{([^}]*)\}/)[1];
  return Object.fromEntries([...root.matchAll(/(--type-[\w-]+):\s*([^;]+);/g)].map(([, name, value]) => [name, Number(value)]));
}

/** `{ selector, size }` for every font-size in a component's <style> block. */
function fontSizes(path) {
  const style = read(path).match(/<style[^>]*>([\s\S]*?)<\/style>/)[1].replace(/\/\*[\s\S]*?\*\//g, '');
  const rules = [];
  for (const [, selector, body] of style.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    for (const [, size] of body.matchAll(/font-size:\s*([^;]+);/g)) rules.push({ selector: selector.trim(), size: size.trim() });
  }
  return rules;
}

/** The token a size uses: body × token (`calc(var(<body>, <px>) * var(<token>))`) or em × inline code. */
function scaleOf(size, bodyVar) {
  const block = size.match(/^calc\(var\((--[\w-]+), ([\d.]+)px\) \* var\((--type-[\w-]+)\)\)$/);
  if (block && block[1] === bodyVar && block[3] !== '--type-inline-code') return { token: block[3], fallback: Number(block[2]) };
  if (size === 'calc(var(--type-inline-code) * 1em)') return { token: '--type-inline-code', inline: true };
  return null;
}

/** Size in px at a body size, or em for inline code. */
const sizeAt = (scale, body) => scale.inline ? SCALE[scale.token] : SCALE[scale.token] * body;

const SURFACES = [
  { path: 'lib/components/transcript/parts/TextPart.svelte', bodyVar: '--session-font-size', body: 14.5 },
  { path: 'lib/components/common/MarkdownDoc.svelte', bodyVar: '--markdown-font-size', body: 13 },
];

test('style.css declares exactly the planned type scale', () => {
  assert.deepEqual(typeTokens(), SCALE);
});

for (const { path, bodyVar, body } of SURFACES) {
  test(`${path.split('/').pop()} sizes every text as body × a type-scale ratio`, () => {
    const rules = fontSizes(path);
    assert.ok(rules.length > 0);
    for (const { selector, size } of rules) {
      const scale = scaleOf(size, bodyVar);
      assert.ok(scale, `${selector}: font-size ${size} is not body × a --type-* ratio`);
      if (!scale.inline) assert.equal(scale.fallback, body, `${selector}: default body`);
    }
  });
}

test('message text keeps its sizes at the default 14.5px body', () => {
  const sizes = Object.fromEntries(fontSizes(SURFACES[0].path).map(({ selector, size }) => [selector, sizeAt(scaleOf(size, '--session-font-size'), 14.5)]));
  // Today's sizes before the scale, and how far the merge may move them.
  const expected = {
    '.text-part-container': [14.5, 0],
    ':global(.text-part-container h1)': [20.7, 0.1],
    ':global(.text-part-container h2)': [18.2, 0.1],
    ':global(.text-part-container h3)': [15.7, 0.1],
    ':global(.text-part-container table)': [13.3, 0.1],
    ':global(.text-part-container code.hljs)': [13.3, 0.1],
    ':global(.text-part-container .code-lang-tag)': [10.88, 0.1],
    ':global(.text-part-container .copy-code-btn, .text-part-container .mermaid-toggle-btn, .text-part-container .mermaid-expand-btn)': [10.88, 0.1],
  };
  for (const [selector, [px, tolerance]] of Object.entries(expected)) {
    assert.ok(Math.abs(sizes[selector] - px) <= tolerance + 1e-9, `${selector}: ${sizes[selector]}px, was ${px}px`);
  }
  assert.equal(sizes[':global(.text-part-container .inline-code)'], 0.85);
});

test('the docs preview uses the same ratios as message text', () => {
  const ratios = (path, bodyVar) => new Set(fontSizes(path).map(({ size }) => scaleOf(size, bodyVar).token));
  const docs = ratios(SURFACES[1].path, '--markdown-font-size');
  const messages = ratios(SURFACES[0].path, '--session-font-size');
  assert.deepEqual([...docs].sort(), [...messages].sort());
});

test('block text is sized from the body, so nesting does not compound', () => {
  // A table or code block inside a list or quote: em would multiply by the
  // parent's size; body × ratio does not.
  for (const { path } of SURFACES) {
    for (const { selector, size } of fontSizes(path)) {
      if (/\b(table|code\.hljs|h[1-3])\)/.test(selector)) assert.doesNotMatch(size, /em\b/, `${selector}: ${size}`);
    }
  }
});
