import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { captureSelection, holdSelection, requestSelectionTranslation, selectionMenuItems, sentenceContext, MAX_SELECTION_CONTEXT } from '../src/lib/selectionTranslation.ts';
import { createAPI } from '../src/lib/api.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { loadServerComponent } from './helpers.mjs';
import { render } from 'svelte/server';

test('selection popover renders an accessible loading card without printing controls', async () => {
  const lib = name => JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href);
  const { default: Popover } = await loadServerComponent(new URL('../src/lib/components/transcript/SelectionTranslation.svelte', import.meta.url), [
    ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
    ["import { api } from '../../api';", 'const api = {};'],
    ["import { toast } from '../../stores/toast.svelte';", 'const toast = {};'],
    ...['portable', 'manage', 'guidance', 'selectionTranslation'].map(name => [`'../../${name}'`, lib(name)]),
  ]);
  const html = render(Popover, { props: { selection: { text: 'bank', context: 'River bank.', rect: { left: 0, right: 20, top: 0, bottom: 10 } }, language: 'Vietnamese', onClose() {} } }).body;
  assert.match(html, /role="dialog" aria-label="Selection translation"/);
  assert.match(html, /role="status"[^>]*>Translating/);
  assert.match(html, /Close translation/);
  assert.doesNotMatch(html, /Print|Save as PDF/);
});

// A small DOM: nodes know their parents, and ranges compare boundary points in
// tree order, so intersection, collapse and text follow the DOM Range rules.
function text(value) { return { nodeType: 3, textContent: value, childNodes: [], contains(node) { return node === this; } }; }
function element(tagName, ...childNodes) {
  const el = {
    nodeType: 1, tagName, childNodes, style: { display: 'block', visibility: 'visible' },
    contains(node) { return node === this || childNodes.some(c => c.contains(node)); },
    matches() { return this.hidden || tagName === 'BUTTON'; },
  };
  for (const child of childNodes) child.parentNode = child.parentElement = el;
  return el;
}
const pathOf = node => node.parentNode ? [...pathOf(node.parentNode), node.parentNode.childNodes.indexOf(node)] : [];
function compare([a, ao], [b, bo]) {
  const x = [...pathOf(a), ao], y = [...pathOf(b), bo];
  for (let i = 0; i < Math.min(x.length, y.length); i++) if (x[i] !== y[i]) return x[i] - y[i];
  return x.length - y.length;
}
const textsIn = node => node.nodeType === 3 ? [node] : node.childNodes.flatMap(textsIn);
const topOf = node => node.parentNode ? topOf(node.parentNode) : node;
function range(startContainer, startOffset, endContainer, endOffset) {
  return {
    startContainer, startOffset, endContainer, endOffset,
    get collapsed() { return compare([this.startContainer, this.startOffset], [this.endContainer, this.endOffset]) === 0; },
    get commonAncestorContainer() {
      let node = this.startContainer;
      while (!node.contains(this.endContainer)) node = node.parentNode;
      return node;
    },
    setStart(node, offset) { this.startContainer = node; this.startOffset = offset; },
    setEnd(node, offset) { this.endContainer = node; this.endOffset = offset; },
    cloneRange() { return range(this.startContainer, this.startOffset, this.endContainer, this.endOffset); },
    intersectsNode(node) {
      const at = node.parentNode.childNodes.indexOf(node);
      return compare([node.parentNode, at], [this.endContainer, this.endOffset]) < 0
        && compare([node.parentNode, at + 1], [this.startContainer, this.startOffset]) > 0;
    },
    toString() {
      return textsIn(topOf(this.startContainer)).map(n => {
        if (n !== this.startContainer && n !== this.endContainer && !this.intersectsNode(n)) return '';
        return n.textContent.slice(n === this.startContainer ? this.startOffset : 0, n === this.endContainer ? this.endOffset : undefined);
      }).join('');
    },
    getBoundingClientRect: () => ({ left: 10, right: 40, top: 20, bottom: 35 }),
  };
}
function selection(...bounds) {
  const r = range(...bounds);
  return { isCollapsed: r.collapsed, rangeCount: 1, getRangeAt: () => r };
}
function withDom(run) {
  const saved = { getComputedStyle: globalThis.getComputedStyle, document: globalThis.document, NodeFilter: globalThis.NodeFilter };
  try {
    globalThis.getComputedStyle = el => el.style;
    globalThis.NodeFilter = { SHOW_TEXT: 4 };
    globalThis.document = { createTreeWalker(root) { const nodes = textsIn(root).filter(n => n !== root); return { nextNode: () => nodes.shift() ?? null }; } };
    run();
  } finally { Object.assign(globalThis, saved); }
}

test('sentence context uses actual offsets for repeated phrases and multiple sentences', () => {
  const source = 'The bank was closed. Walk to the bank by the river. Then stop.';
  const start = source.lastIndexOf('bank');
  assert.equal(sentenceContext(source, start, start + 4), 'Walk to the bank by the river.');
  assert.equal(sentenceContext(source, 4, start + 4), 'The bank was closed. Walk to the bank by the river.');
});

test('sentence fallback handles CJK and context stays bounded in UTF-8', () => {
  const saved = Intl.Segmenter;
  try {
    Intl.Segmenter = undefined;
    assert.equal(sentenceContext('最初。次の文！最後。', 4, 6), '次の文！');
    const long = '😀'.repeat(10000);
    const result = sentenceContext(long, 12000, 12002);
    assert.ok(new TextEncoder().encode(result).length <= MAX_SELECTION_CONTEXT);
    assert.ok(!result.includes('�'));
    assert.ok(result.length > 1000);
  } finally { Intl.Segmenter = saved; }
});

test('selection capture uses ordered endpoints across markup, paragraphs and code', () => withDom(() => {
  const a = text('First sentence. The '), b = text('river bank'), c = text(' is nearby.'), d = text('Run pwd.');
  const root = element('DIV', element('P', a, element('EM', b), c), element('PRE', d));
  const selected = selection(b, 6, b, 10);
  const captured = captureSelection(root, selected);
  assert.equal(captured.text, 'bank');
  assert.equal(captured.context, 'The river bank is nearby.');
  assert.deepEqual(captured.rect, { left: 10, right: 40, top: 20, bottom: 35 });
  // A copy, so the held range survives the selection being cleared.
  assert.notEqual(captured.range, selected.getRangeAt(0));
  assert.deepEqual([captured.range.startContainer, captured.range.startOffset, captured.range.endContainer, captured.range.endOffset], [b, 6, b, 10]);
  // Anchor direction is intentionally irrelevant: the Range is ordered.
  selected.anchorNode = c;
  selected.focusNode = b;
  assert.equal(captureSelection(root, selected).context, captured.context);
  assert.match(captureSelection(root, selection(b, 6, d, 7)).context, /Run pwd/);
  const p = root.childNodes[0];
  const em = captureSelection(root, selection(p, 1, p, 2));
  assert.equal(em.text, 'river bank');
  assert.equal(em.context, 'The river bank is nearby.');
}));

// Two messages as the transcript renders them, then the end marker and the
// bottom pill.
function transcript({ last: lastMessage = false } = {}) {
  const first = text('First paragraph.'), last = text('In panel setting, I see this:');
  const body = element('DIV', element('P', first), element('P', last), element('BUTTON', element('IMG')));
  const icon = element('SPAN', text('🤖')), label = element('SPAN', text('Assistant'));
  const marker = element('DIV', element('SPAN', text('── End of transcript ──')));
  marker.style = { ...marker.style, userSelect: 'none' };
  const pill = element('SPAN');
  const card = element('DIV', element('DIV', element('SPAN', text('👤')), element('SPAN', text('User'))), body);
  const next = element('DIV', element('DIV', element('DIV', icon, label), element('DIV', element('P', text('That matches.')))));
  element('DIV', element('DIV', card), ...(lastMessage ? [] : [next]), marker, element('BUTTON', pill));
  return { body, card, last, icon, label, marker, pill, image: body.childNodes[2] };
}

test('line selections ending at the next element boundary use the selection menu', () => withDom(() => {
  const { body, card, last, icon, image } = transcript();
  const end = transcript({ last: true });
  const ends = { 'the next message header': [body, icon, 0], 'the card around the body': [body, card, 2], 'an attachment after the text': [body, image, 0], 'past the unselectable end marker': [end.body, end.pill, 0] };
  for (const [where, [body, node, offset]] of Object.entries(ends)) {
    const last = body.childNodes[1].childNodes[0];
    const selected = selection(last, 0, node, offset);
    const captured = captureSelection(body, selected);
    assert.equal(captured?.text, 'In panel setting, I see this:', where);
    assert.equal(captured.context, 'In panel setting, I see this:', where);
    assert.deepEqual([captured.range.endContainer, captured.range.endOffset], [last, last.textContent.length], where);
    assert.deepEqual([selected.getRangeAt(0).endContainer, selected.getRangeAt(0).endOffset], [node, offset], `${where}: the live selection is unchanged`);
  }
  // Starting at the body's own boundary narrows the same way.
  assert.equal(captureSelection(body, selection(body, 0, last, 2)).text, 'First paragraph.In');
}));

test('invalid, cross-message, hidden and control selections stay native', () => withDom(() => {
  const { body, card, last, label, icon } = transcript();
  assert.equal(captureSelection(body, null), null);
  assert.equal(captureSelection(body, { isCollapsed: true }), null);
  assert.equal(captureSelection(body, selection(last, 0, label.childNodes[0], 4)), null, 'into the next header');
  assert.equal(captureSelection(body, selection(card, 0, last, 4)), null, 'from this header');
  assert.equal(captureSelection(body, selection(last.parentNode, 1, icon, 0)), null, 'only an empty boundary');
  // Text the user cannot see between the body and the end does not count.
  const between = element('DIV', text('hidden notice'));
  between.style = { display: 'none', visibility: 'visible' };
  const a = text('hello'), b = text('world'), w = text('  ');
  const button = element('BUTTON', text('Copy'));
  const hidden = element('DIV', text('hidden')); hidden.hidden = true; hidden.style = { display: 'none' };
  const root = element('DIV', a, button, b, hidden, w);
  element('DIV', element('DIV', root), between, element('SPAN', text('next')));
  assert.equal(captureSelection(root, selection(b, 0, between.parentNode.childNodes[2], 0)).text, 'world');
  assert.equal(captureSelection(root, selection(a, 0, b, 2)), null, 'across a control');
  assert.equal(captureSelection(root, selection(b, 0, w, 1)), null, 'across hidden text');
  assert.equal(captureSelection(root, selection(w, 0, w, 2)), null, 'whitespace only');
}));

test('mock selection translation keeps context separate and leaves message translations alone', async () => {
  const mock = new MockBackendAPI();
  mock.translateDelayMs = 0;
  await assert.rejects(mock.translateSelection('bank', 'Walk along the river bank.'), /not set up/);
  mock.translateConfig = { baseURL: 'https://fixture.invalid/v1', apiKey: 'fixture', model: 'fixture', language: 'Vietnamese' };
  assert.equal(await mock.translateSelection('bank', 'Walk along the river bank.'), '[Vietnamese] bank');
  assert.deepEqual(mock.translatedSelections, [{ text: 'bank', context: 'Walk along the river bank.' }]);
  assert.deepEqual(mock.translated, []);
});

test('selection menu has Copy and Translate, without Print or message actions', () => {
  const calls = [];
  const configured = selectionMenuItems(true, 'Vietnamese', () => calls.push('copy'), () => calls.push('translate'), () => calls.push('setup'));
  assert.deepEqual(configured.map(i => i.label), ['Copy', 'Translate to Vietnamese']);
  configured.forEach(i => i.onSelect());
  assert.deepEqual(calls, ['copy', 'translate']);
  const setup = selectionMenuItems(false, 'Vietnamese', () => {}, () => {}, () => calls.push('setup'));
  assert.deepEqual(setup.map(i => [i.label, !!i.disabled]), [['Copy', false], ['Translate', true], ['Set up translation…', false]]);
  assert.ok(!setup.concat(configured).some(i => /Print|Save|original|Retranslate/.test(i.label)));
});

test('dismissed or superseded selection requests ignore late success and failure', async () => {
  const pending = [];
  const seen = [];
  const request = () => new Promise((resolve, reject) => pending.push({ resolve, reject }));
  const first = requestSelectionTranslation(request, t => seen.push(['done', t]), e => seen.push(['error', e]));
  const second = requestSelectionTranslation(request, t => seen.push(['done', t]), e => seen.push(['error', e]));
  await Promise.resolve();
  first();
  pending[0].resolve('old');
  pending[1].reject('current failure');
  await new Promise(r => setImmediate(r));
  assert.deepEqual(seen, [['error', 'current failure']]);
  second();
});

test('the Wails binding receives selection and context separately and errors normalize', async () => {
  const previous = globalThis.window;
  try {
    const binding = {};
    globalThis.window = { go: { app: { App: binding } } };
    const backend = createAPI();
    let args;
    binding.TranslateSelection = async (...a) => { args = a; return 'ngân hàng'; };
    assert.equal(await backend.translateSelection('bank', 'The bank was closed.'), 'ngân hàng');
    assert.deepEqual(args, ['bank', 'The bank was closed.']);
    binding.TranslateSelection = async () => { throw 'translation context is too long'; };
    await assert.rejects(backend.translateSelection('x', 'y'), /context is too long/);
  } finally { globalThis.window = previous; }
  assert.match(readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8'), /'translate',\n  'translateSelection',/);
});

function withSelectionGlobals(highlights, selection, run) {
  const saved = { CSS: globalThis.CSS, Highlight: globalThis.Highlight, window: globalThis.window };
  try {
    globalThis.CSS = highlights ? { highlights } : undefined;
    globalThis.Highlight = highlights ? class { constructor(...ranges) { this.ranges = ranges; } } : undefined;
    globalThis.window = { getSelection: () => selection };
    run();
  } finally { Object.assign(globalThis, saved); }
}
function fakeSelection(collapsed = true) {
  const calls = [];
  return {
    calls, rangeCount: collapsed ? 0 : 1, isCollapsed: collapsed,
    removeAllRanges() { calls.push('clear'); },
    addRange(r) { calls.push(r); },
  };
}

test('a held selection is painted while open and selected again on release', () => {
  const range = { startContainer: { isConnected: true }, endContainer: { isConnected: true } };
  const highlights = new Map();
  const selection = fakeSelection();
  withSelectionGlobals(highlights, selection, () => {
    const release = holdSelection(range);
    assert.deepEqual(highlights.get('held-selection').ranges, [range]);
    assert.deepEqual(selection.calls, []);
    release();
    assert.equal(highlights.has('held-selection'), false);
    assert.deepEqual(selection.calls, ['clear', range]);
  });
});

test('release leaves a newer selection, detached ranges and newer holds alone', () => {
  const live = { startContainer: { isConnected: true }, endContainer: { isConnected: true } };
  const highlights = new Map();
  const newer = fakeSelection(false);
  withSelectionGlobals(highlights, newer, () => {
    const first = holdSelection(live);
    const second = holdSelection(live);
    first();
    assert.ok(highlights.has('held-selection'), 'an older release keeps the newer highlight');
    second();
    assert.deepEqual(newer.calls, [], 'the user selected something else meanwhile');
  });
  const detached = { startContainer: { isConnected: false }, endContainer: { isConnected: true } };
  const empty = fakeSelection();
  withSelectionGlobals(undefined, empty, () => {
    holdSelection(detached)();
    assert.deepEqual(empty.calls, []);
    // Without the highlight API, release still restores the selection.
    holdSelection(live)();
    assert.deepEqual(empty.calls, ['clear', live]);
  });
});

test('the transcript holds the selection across its menu and popover', () => {
  const bubble = readFileSync(new URL('../src/lib/components/transcript/MessageBubble.svelte', import.meta.url), 'utf8');
  assert.match(bubble, /let held = \$derived\(menu\?\.selection\?\.range \?\? snippet\?\.range\);/);
  assert.match(bubble, /return range \? holdSelection\(range\) : undefined;/);
  const css = readFileSync(new URL('../src/style.css', import.meta.url), 'utf8');
  assert.match(css, /::highlight\(held-selection\)\s*\{\s*background-color: Highlight;\s*\}/);
});
