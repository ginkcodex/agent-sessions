import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';
import { loadServerComponent } from './helpers.mjs';
import { firstVisibleMessage, restoreMessage } from '../src/lib/transcriptAnchor.ts';

// ---------------------------------------------------------------------------
// transcriptAnchor: a fake scroll container whose rows move with scrollTop.
// ---------------------------------------------------------------------------

/** Rows of the given heights, stacked from y = 0 in content coordinates. */
function container(heights, { scrollTop = 0, viewTop = 100 } = {}) {
  const rows = [];
  const el = {
    scrollTop,
    getBoundingClientRect: () => ({ top: viewTop }),
    querySelectorAll: () => rows,
    querySelector: (selector) => rows.find((row) => selector === `[data-message-index="${row.dataset.messageIndex}"]`) ?? null,
    // Resizes the rows, as a new text size would, without touching scrollTop.
    resize(next) { next.forEach((h, i) => { rows[i].height = h; }); },
  };
  heights.forEach((height, i) => {
    const row = {
      height,
      dataset: { messageIndex: String(10 + i) },
      getBoundingClientRect() {
        const y = rows.slice(0, i).reduce((sum, r) => sum + r.height, 0);
        const top = viewTop + y - el.scrollTop;
        return { top, bottom: top + row.height, height: row.height };
      },
    };
    rows.push(row);
  });
  return el;
}

test('the first visible message is the first whose bottom is below the top of the view', () => {
  assert.equal(firstVisibleMessage(container([])), null);
  assert.deepEqual(firstVisibleMessage(container([100, 200, 300])), { index: 10, offset: 0, height: 100 });
  // Scrolled 150px: message 10 is gone, 11 started 50px above.
  assert.deepEqual(firstVisibleMessage(container([100, 200, 300], { scrollTop: 150 })), { index: 11, offset: -50, height: 200 });
  // Exactly at a boundary, the next message is the first.
  assert.equal(firstVisibleMessage(container([100, 200, 300], { scrollTop: 100 })).index, 11);
});

test('a message that started in view keeps its top in place when the text grows or shrinks', () => {
  for (const scale of [1.5, 0.75]) {
    // Scrolled to the top of message 11.
    const el = container([100, 200, 300], { scrollTop: 100 });
    const anchor = firstVisibleMessage(el);
    assert.deepEqual(anchor, { index: 11, offset: 0, height: 200 });
    el.resize([100 * scale, 200 * scale, 300 * scale]);
    assert.equal(restoreMessage(el, anchor), true);
    assert.equal(el.scrollTop, 100 * scale);
    assert.equal(el.querySelector('[data-message-index="11"]').getBoundingClientRect().top, 100, 'at the top of the view again');
  }
});

test('a message that started above the view keeps the same share of it above', () => {
  // 60px of the 200px message 11 are above the view: 30%.
  const el = container([100, 200, 300], { scrollTop: 160 });
  const anchor = firstVisibleMessage(el);
  assert.deepEqual(anchor, { index: 11, offset: -60, height: 200 });
  el.resize([150, 300, 450]);
  assert.equal(restoreMessage(el, anchor), true);
  // 30% of the new 300px message, under the new 150px message 10.
  assert.equal(el.scrollTop, 150 + 90);
});

test('restoring a message that is no longer shown changes nothing', () => {
  const el = container([100, 200], { scrollTop: 40 });
  assert.equal(restoreMessage(el, { index: 99, offset: 0, height: 10 }), false);
  assert.equal(el.scrollTop, 40);
});

// ---------------------------------------------------------------------------
// TranscriptView (SSR): the session size is a property on the scroll area.
// ---------------------------------------------------------------------------

const VIEW_URL = new URL('../src/lib/components/transcript/TranscriptView.svelte', import.meta.url);

// Anything: a value that can be read, called and constructed to any depth,
// so the view's many stores and helpers render without their real modules.
const anything = new Proxy(function () {}, {
  get: (_, key) => (key === Symbol.toPrimitive ? () => '' : key === Symbol.iterator ? [][Symbol.iterator] : key === 'then' ? undefined : anything),
  apply: () => anything,
  construct: () => anything,
});

globalThis.__transcriptFont = {
  anything,
  appState: new Proxy({}, { get: (_, key) => (key === 'selectedSessionRef' ? { id: 's1', agent: 'claude' } : key === 'selectedSessionMeta' ? null : anything) }),
  preferences: { transcriptMode: 'normal', sessionFontSize: null },
};

async function loadView() {
  const code = compile(readFileSync(VIEW_URL, 'utf8'), { generate: 'server', filename: fileURLToPath(VIEW_URL) }).js.code;
  const replacements = [];
  for (const [statement, names, from] of code.matchAll(/^import ([\s\S]*?) from '([^']+)';$/gm).map((m) => [m[0], m[1], m[2]])) {
    if (from === 'svelte/internal/server') continue;
    if (from === 'svelte') replacements.push([`'svelte'`, JSON.stringify(import.meta.resolve('svelte'))]);
    else if (from === '../../appearance') replacements.push([`'${from}'`, JSON.stringify(new URL('../src/lib/appearance.ts', import.meta.url).href)]);
    else if (from.endsWith('.svelte') && !from.endsWith('.svelte.ts') && /^[A-Z]\w*$/.test(names)) replacements.push([statement, `const ${names} = () => {};`]);
    else {
      const bound = names.replace(/^\{|\}$/g, '').split(',').map((n) => n.trim()).filter(Boolean)
        .map((n) => (n in globalThis.__transcriptFont ? `${n}: ${n}` : `${n}: ${n} = globalThis.__transcriptFont.anything`));
      replacements.push([statement, `const { ${bound.join(', ')} } = globalThis.__transcriptFont;`]);
    }
  }
  return loadServerComponent(VIEW_URL, replacements);
}

const view = await loadView();

async function scrollArea(sessionFontSize) {
  const { render } = await import('svelte/server');
  globalThis.__transcriptFont.preferences.sessionFontSize = sessionFontSize;
  const html = render(view.default, { props: {} }).body;
  return html.match(/<div class="messages-scroll-area[^"]*"[^>]*>/)?.[0];
}

test('the scroll area sets --session-font-size only when a size is chosen', async () => {
  const plain = await scrollArea(null);
  assert.ok(plain, 'the scroll area renders');
  assert.doesNotMatch(plain, /style=/, 'the default sets nothing: every part uses its 14.5px fallback');
  assert.match(await scrollArea(18), /style="--session-font-size: 18px;"/);
  assert.match(await scrollArea(11.5), /style="--session-font-size: 11.5px;"/);
});

test('a size change keeps a scrolled-up reader on the first visible message', () => {
  const source = readFileSync(VIEW_URL, 'utf8');
  // Taken before the new size renders, put back after; not while following.
  assert.match(source, /\$effect\.pre\(\(\) => \{\s*void sessionFontSize;\s*untrack\(\(\) => \{\s*fontAnchor = containerEl && !following && !anchoredToBottom \? firstVisibleMessage\(containerEl\) : null;/);
  assert.match(source, /restoreMessage\(containerEl, anchor\);\s*lastScrollTop = containerEl\.scrollTop;/);
  // Following: the ResizeObserver keeps the end in view as the text grows.
  assert.match(source, /if \(anchoredToBottom \|\| following\) el\.scrollTop = el\.scrollHeight;/);
});
