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

const T = 'lib/components/transcript/';
const session = (path) => ({ path: T + path, bodyVar: '--session-font-size', body: 14.5 });

const SURFACES = [
  session('parts/TextPart.svelte'),
  { path: 'lib/components/common/MarkdownDoc.svelte', bodyVar: '--markdown-font-size', body: 13 },
  session('MessageBubble.svelte'),
  ...['ToolPart', 'PatchPart', 'ReasoningPart', 'CompactionPart', 'NoticePart', 'ImagePart', 'SubagentLink'].map((name) => session(`parts/${name}.svelte`)),
];

// Sizes that stay fixed on purpose: not text, so not part of the scale.
const EXCEPTIONS = {
  [T + 'MessageBubble.svelte']: {
    '.reset-level': 'icon glyph in a fixed 20px button',
  },
};

// Which token takes over each of today's sizes (the plan's type scale table).
const TOKEN_FOR_PX = {
  13.7: '--type-code', 12.8: '--type-code',
  12.48: '--type-small', 12: '--type-small',
  11.52: '--type-label', 11.2: '--type-label',
  10.88: '--type-tag', 10.4: '--type-tag',
};

// Session part sizes before the type scale (FS-02), per selector.
const SESSION_PX = {
  'MessageBubble.svelte': {
    '.message-meta-header': 11.52,
    '.model-tag': 10.88,
    '.sidechain-tag': 10.4,
    '.translation-tag': 10.4,
    // CPC-14: new, on the same tag token as its neighbours.
    '.command-tag': 10.4,
    '.file-attachment-pill': 12,
  },
  'parts/ToolPart.svelte': {
    '.tool-name': 12.8,
    // TTR-01: useful target summary replaces the ID; tool text uses the code token.
    '.tool-summary': 12.8,
    '.expand-output-btn': 12,
    '.tool-status-badge': 10.88,
    '.section-summary': 11.52,
    '.code-wrapper pre': 12.8,
    '.output-label': 11.52,
    '.output-content': 12.8,
    '.truncated-banner': 11.52,
    '.load-blob-btn': 11.2,
    '.mini-copy-btn': 10.88,
    '.blob-error-msg': 11.52,
  },
  'parts/PatchPart.svelte': {
    '.patch-title': 12.48,
    '.patch-count': 11.2,
    '.toggle-diff-btn': 11.2,
    '.patch-file-item': 12,
    '.diff-code': 11.52,
  },
  'parts/ReasoningPart.svelte': {
    '.reasoning-summary': 12,
    '.reasoning-body': 13.7,
  },
  'parts/CompactionPart.svelte': {
    '.compaction-pill': 11.52,
    '.pill-chevron': 10.4,
    '.compaction-details': 12.48,
    '.summary-header': 10.88,
  },
  'parts/NoticePart.svelte': {
    '.notice-pill': 11.52,
    '.notice-icon': 12,
  },
  'parts/ImagePart.svelte': {
    '.image-loading': 12,
    '.image-failed': 12,
  },
  'parts/SubagentLink.svelte': {
    '.subagent-link-btn': 12,
  },
};

test('style.css declares exactly the planned type scale', () => {
  assert.deepEqual(typeTokens(), SCALE);
});

for (const { path, bodyVar, body } of SURFACES) {
  test(`${path.split('/').pop()} sizes every text as body × a type-scale ratio`, () => {
    const rules = fontSizes(path);
    assert.ok(rules.length > 0);
    const exceptions = EXCEPTIONS[path] ?? {};
    for (const { selector, size } of rules) {
      if (selector in exceptions) continue;
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
    ':global(.text-part-container .copy-code-btn, .text-part-container .run-code-btn, .text-part-container .mermaid-toggle-btn, .text-part-container .mermaid-expand-btn)': [10.88, 0.1],
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

test('fixed sizes are only the listed non-text exceptions', () => {
  for (const [path, exceptions] of Object.entries(EXCEPTIONS)) {
    const selectors = new Set(fontSizes(path).map(({ selector }) => selector));
    for (const selector of Object.keys(exceptions)) assert.ok(selectors.has(selector), `${path}: stale exception ${selector}`);
  }
});

// The largest move is the tool name, 12.8 → 13.34px; code is its nearest token.
const MAX_DRIFT = 0.55;

test('session parts take the token planned for their old size, within 0.55px', () => {
  for (const [file, expected] of Object.entries(SESSION_PX)) {
    const rules = Object.fromEntries(fontSizes(T + file).map(({ selector, size }) => [selector, scaleOf(size, '--session-font-size')]));
    for (const [selector, px] of Object.entries(expected)) {
      const scale = rules[selector];
      assert.ok(scale, `${file} ${selector}: not on the type scale`);
      assert.equal(scale.token, TOKEN_FOR_PX[px], `${file} ${selector}: was ${px}px`);
      const now = sizeAt(scale, 14.5);
      assert.ok(Math.abs(now - px) <= MAX_DRIFT + 1e-9, `${file} ${selector}: ${now}px, was ${px}px`);
    }
    // Every scaled rule in the file is accounted for.
    const exceptions = EXCEPTIONS[T + file] ?? {};
    assert.deepEqual(Object.keys(rules).filter((s) => !(s in exceptions)).sort(), Object.keys(expected).sort(), file);
  }
});
