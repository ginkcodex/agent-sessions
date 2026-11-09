import test from 'node:test';
import assert from 'node:assert/strict';
import { SourceHistory, wordRange, caretPoint, sourcePosition, sourceRange, linkModifier, contextPress, menuKey, navigationKey, historyKey, locationText, locationList, rankRubyDefinitions, rubyFileName, hoverHtml } from '../src/lib/sourceNavigation.ts';
import { languageOfPath } from '../src/lib/codeLanguages.ts';
import { loadServerComponent } from './helpers.mjs';
import { emptyNavigationAnswer } from '../src/lib/codeNavigation.ts';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

const key = (extra = {}) => ({ key: '', code: '', metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, target: null, ...extra });
const place = (path, line = 0) => ({ path, range: { start: { line, character: 2 }, end: { line, character: 2 } } });

test('identifier ranges use UTF-16 columns, accept word ends and reject numeric literals', () => {
  for (const column of [0, 2, 5]) assert.deepEqual(wordRange('hello + 42', column), { start: 0, end: 5 });
  assert.equal(wordRange('hello + 42', 7), undefined);
  assert.equal(wordRange('hello + 42', 9), undefined);
  assert.deepEqual(wordRange('😀 café_$2', 6), { start: 3, end: 10 });
  assert.equal(wordRange('', 0), undefined);
});

test('a context press is a right-click anywhere, Ctrl+click only on macOS; modifiers alone keep it', () => {
  for (const mac of [true, false]) assert.equal(contextPress({ button: 2, ctrlKey: false }, mac), true);
  assert.equal(contextPress({ button: 0, ctrlKey: true }, true), true);
  assert.equal(contextPress({ button: 0, ctrlKey: true }, false), false, 'Ctrl+click is the link click on Windows and Linux');
  assert.equal(contextPress({ button: 0, ctrlKey: false }, true), false);
  assert.equal(contextPress({ button: 1, ctrlKey: true }, true), false);
  for (const key of ['ContextMenu', 'F10', 'a']) assert.equal(menuKey({ key }), true);
  for (const key of ['Shift', 'Control', 'Alt', 'Meta']) assert.equal(menuKey({ key }), false, `${key} repeats while held on Windows`);
});

test('navigation modifiers and history shortcuts preserve text-field bindings', () => {
  assert.equal(linkModifier(key({ metaKey: true }), true), true);
  assert.equal(linkModifier(key({ ctrlKey: true }), false), true);
  assert.equal(linkModifier(key({ ctrlKey: true, altKey: true }), false), false);
  assert.equal(navigationKey(key({ key: 'F12' })), 'definition');
  assert.equal(navigationKey(key({ key: 'F12', shiftKey: true })), 'references');
  assert.equal(navigationKey(key({ key: 'F12', metaKey: true })), undefined);
  assert.equal(historyKey(key({ key: '[', metaKey: true }), true), 'back');
  assert.equal(historyKey(key({ key: '[', metaKey: true, target: { tagName: 'TEXTAREA' } }), true), null);
  assert.equal(historyKey(key({ code: 'Minus', ctrlKey: true, shiftKey: true, target: { isContentEditable: true } }), true), 'forward');
  assert.equal(historyKey(key({ key: 'ArrowLeft', altKey: true }), false), 'back');
});

test('history bounds, deduplicates, drops the forward branch and forgets removed places', () => {
  const h = new SourceHistory(2), a = place('a'), b = place('b'), c = place('c'), d = place('d');
  h.visit(a); h.visit(a); h.visit(b); h.visit(c);
  assert.deepEqual(h.goBack(d), c);
  assert.deepEqual(h.goBack(c), b);
  assert.equal(h.goBack(b), undefined);
  assert.deepEqual(h.goForward(b), c);
  h.visit(c);
  assert.equal(h.canGoForward, false);
  h.forget(p => p.path !== 'c');
  assert.deepEqual(h.goBack(d), b);
  h.clear();
  assert.equal(h.canGoBack, false);
  assert.equal(h.canGoForward, false);
});

test('hover escapes plaintext and renders markdown through the safe transcript renderer', () => {
  assert.equal(hoverHtml({ kind: 'plaintext', value: '<script>"&' }), '<pre class="hover-plain">&lt;script&gt;&quot;&amp;</pre>');
  const html = hoverHtml({ kind: 'markdown', value: '**symbol**\n\n```ts\nconst n = 1;\n```\n<script>alert(1)</script>' });
  assert.match(html, /<strong>symbol<\/strong>/);
  assert.match(html, /code-block/);
  assert.doesNotMatch(html, /<script>/);
  assert.equal(locationText({ ...place('/home/a/lib.go', 3), external: true }, () => '~/lib.go'), '~/lib.go:4:3');
});

test('a list of places in one outside folder names the folder once', () => {
  const ext = (path, line = 0) => ({ ...place(path, line), external: true });
  const gem = '/Users/a/.local/share/mise/installs/ruby/4.0.9/lib/ruby/gems/4.0.0/gems/activejob-8.1.4.1';
  const home = p => p.replace('/Users/a', '~');
  // ActiveJob reopened across the gem: the gem folder is the heading.
  assert.deepEqual(locationList([ext(`${gem}/lib/active_job/arguments.rb`, 5), ext(`${gem}/lib/active_job/queue_adapters/async_adapter.rb`, 7), ext(`${gem}/lib/active_job.rb`)], home), {
    heading: home(gem) + '/lib',
    labels: ['active_job/arguments.rb:6:3', 'active_job/queue_adapters/async_adapter.rb:8:3', 'active_job.rb:1:3'],
  });
  // A shared name prefix is not a folder.
  assert.deepEqual(locationList([ext('/g/activejob-8/a.rb'), ext('/g/activerecord-8/b.rb')]), { heading: '/g', labels: ['activejob-8/a.rb:1:3', 'activerecord-8/b.rb:1:3'] });
  assert.deepEqual(locationList([ext('C:\\Ruby\\lib\\a.rb'), ext('C:\\Ruby\\lib\\b.rb')]), { heading: 'C:\\Ruby\\lib', labels: ['a.rb:1:3', 'b.rb:1:3'] });
  // Only a root in common, one place, or a place in the project: full paths, no heading.
  for (const list of [[ext('/a/x.rb'), ext('/b/y.rb')], [ext('C:\\a\\x.rb'), ext('D:\\a\\y.rb')], [ext('C:\\a.rb'), ext('C:\\b.rb')], [ext(`${gem}/a.rb`)], [ext(`${gem}/a.rb`), place('app/a.rb')]]) {
    assert.deepEqual(locationList(list, home), { labels: list.map(l => locationText(l, home)) }, JSON.stringify(list));
  }
});

test('Python documentation hover renders entities, escaped identifiers and admonitions', () => {
  const value = [
    '```python', 'class ItemCreate(*, name: str):', '    pass', '```', '---',
    '!!! abstract "Usage Documentation"',
    '&nbsp;&nbsp;&nbsp;&nbsp;[Models](https://docs.example/models)', '',
    'A base class for creating models.', '', 'Attributes:',
    String.raw`&nbsp;&nbsp;&nbsp;&nbsp;\_\_class\_vars\_\_: The class variables.`,
  ].join('\n');
  const html = hoverHtml({ kind: 'markdown', value });
  assert.match(html, /language-python/);
  assert.match(html, /<blockquote><p><strong>Usage Documentation<\/strong>/);
  assert.match(html, /href="https:\/\/docs.example\/models"/);
  assert.match(html, /<\/blockquote>\n<p>A base class/);
  assert.match(html, / {4}__class_vars__: The class variables/);
  assert.doesNotMatch(html, /&amp;nbsp;|\\_|!!! abstract/);
});

test('hover admonitions respect dedents, code literals, malformed headers and plaintext', () => {
  const value = 'Intro\n!!! note "Custom <title>"\n    First\n\n    Second\nAfter';
  const html = hoverHtml({ kind: 'markdown', value });
  assert.match(html, /<p>Intro<\/p>\n<blockquote>/);
  assert.match(html, /Custom &lt;title&gt;/);
  assert.match(html, /<p>First<\/p>\n<p>Second<\/p><\/blockquote>\n<p>After<\/p>/);
  for (const content of ['!!! broken "title', '!!! note "No body"\nAfter']) {
    assert.doesNotMatch(hoverHtml({ kind: 'markdown', value: content }), /<blockquote>/);
  }
  const literal = '```text\n!!! note "Literal"\n    &nbsp; \\_name\n```';
  const code = hoverHtml({ kind: 'markdown', value: literal });
  assert.doesNotMatch(code, /<blockquote>/);
  assert.match(code, /data-code="!!! note &quot;Literal&quot;\n    &amp;nbsp; \\_name"/);
  assert.match(hoverHtml({ kind: 'plaintext', value }), /!!! note &quot;Custom &lt;title&gt;&quot;/);
  assert.equal(hoverHtml({ kind: 'plaintext', value: '&nbsp; \\_name' }), '<pre class="hover-plain">&amp;nbsp; \\_name</pre>');
});

test('caret lookup supports Chromium and WebKit and missing points', () => {
  const node = {};
  assert.deepEqual(caretPoint({ caretPositionFromPoint: () => ({ offsetNode: node, offset: 3 }) }, 1, 2), { node, offset: 3 });
  assert.deepEqual(caretPoint({ caretRangeFromPoint: () => ({ startContainer: node, startOffset: 2 }) }, 1, 2), { node, offset: 2 });
  assert.equal(caretPoint({ caretPositionFromPoint: () => null }, 1, 2), undefined);
  assert.equal(caretPoint({}, 1, 2), undefined);
});

test('source coordinates count text rather than syntax spans and exclude non-source lines', () => {
  const nodes = [{ textContent: '😀 ' }, { textContent: 'name' }];
  let range;
  const el = { dataset: { sourceLine: '4' }, closest: () => el };
  const root = {
    contains: node => node === el,
    querySelector: selector => selector === '[data-source-line="4"]' ? el : null,
    ownerDocument: {
      createTreeWalker: () => { let i = 0; return { nextNode: () => nodes[i++] }; },
      createRange: () => (range = { setStart(...p) { this.start = p; }, setEnd(...p) { this.end = p; }, toString: () => '😀 na' }),
    },
  };
  assert.deepEqual(sourcePosition(root, { parentElement: el }, 2), { line: 3, character: 5 });
  assert.equal(sourcePosition(root, { parentElement: null }, 0), undefined);
  assert.equal(sourceRange(root, 0, 0, 1), undefined);
  sourceRange(root, 3, 3, 7);
  assert.deepEqual(range.start, [nodes[0], 3]);
  assert.deepEqual(range.end, [nodes[1], 4]);
  assert.equal(sourceRange(root, 3, 3, 8), undefined);
});

// Execute the actual result handler, not a parallel implementation. DOM paint
// and native focus remain manual checks.
const viewerSource = readFileSync(new URL('../src/lib/components/common/go-to-file/FileViewer.svelte', import.meta.url), 'utf8');
const menuSource = viewerSource.slice(viewerSource.indexOf('  function wordMenu('), viewerSource.indexOf('  let symbols: SourceSymbols'));
const menuJS = ts.transpileModule(menuSource, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
function menuFixture(blocker) {
  const calls = [], tab = {}, context = {
    files: { activeTab: tab, selectedPath: 'a.go' }, activeTab: tab, edit: { draft: 'name' }, text: 'name',
    codeScroll: undefined, historical: false, external: false, symbolMenu: null, symbolBlocker: blocker,
    closeSymbolPopups() { context.symbolMenu = null; }, navigationIdentity: () => context.identity,
    identity: { host: 'local', version: 1 },
    askSymbol: (...args) => calls.push(['ask', ...args]), copyToClipboard: value => calls.push(['copy', value]),
    onCodeSettings: () => calls.push(['settings']), releaseSelection: undefined, tick: () => Promise.resolve(),
  };
  const { wordMenu: menu, selectionMenu } = new Function('ctx', `with(ctx) { ${menuJS}; return { wordMenu, selectionMenu }; }`)(context);
  return { context, calls, menu, selectionMenu };
}
test('source and editor menus hide unavailable navigation and keep actionable settings and copy', () => {
  const h = menuFixture('Approve code checks for this project.');
  h.menu({ line: 0, character: 0 }, 'name', { x: 10, y: 20 });
  const items = h.context.symbolMenu.items;
  assert.deepEqual(items.map(i => i.label), ['Open code navigation settings…', 'Copy “name”']);
  assert.ok(items.every(i => !i.disabled));
  assert.equal(items[1].separator, true);
  items.find(i => /settings/.test(i.label)).onSelect();
  assert.deepEqual(h.calls, [['settings']]);
  items.find(i => i.label === 'Copy “name”').onSelect();
  assert.deepEqual(h.calls[1], ['copy', 'name']);
  for (const state of [{ historical: true }, { external: true }, { activeTab: { gitBasis: 'head' } }, { onCodeSettings: undefined }]) {
    Object.assign(h.context, state);
    h.menu({ line: 0, character: 0 }, 'name', { x: 1, y: 2 });
    assert.deepEqual(h.context.symbolMenu.items.map(i => i.label), ['Copy “name”']);
    assert.equal(h.context.symbolMenu.items[0].separator, false, 'no leading divider when Copy is alone');
    h.context.symbolMenu.items[0].onSelect();
    assert.deepEqual(h.calls.at(-1), ['copy', 'name']);
    Object.assign(h.context, { historical: false, external: false, activeTab: h.context.files.activeTab, onCodeSettings: () => {} });
  }
});
test('all navigation blockers hide gray entries while available symbol actions still work', () => {
  for (const blocker of [
    'Nothing to approve: no supported checker was found on this host.',
    'No language server for this file.',
    'Reconnect to use code navigation.',
    'The language server is still starting.',
  ]) {
    const h = menuFixture(blocker);
    h.menu({ line: 0, character: 0 }, 'DNS', { x: 10, y: 20 });
    assert.deepEqual(h.context.symbolMenu.items.map(i => i.label), ['Open code navigation settings…', 'Copy “DNS”']);
    assert.ok(h.context.symbolMenu.items.every(i => !i.disabled && i.label !== blocker));
  }
  for (const setup of [true, false]) {
    const h = menuFixture();
    if (!setup) h.context.onCodeSettings = undefined;
    h.menu({ line: 0, character: 0 }, 'name', { x: 10, y: 20 });
    const items = h.context.symbolMenu.items;
    assert.deepEqual(items.map(i => i.label), ['Go to Definition (F12)', 'Find References (Shift+F12)',
      ...(setup ? ['Open code navigation settings…'] : []), 'Copy “name”']);
    assert.equal(items.at(-1).separator, true);
    items[0].onSelect(); items[1].onSelect();
    assert.deepEqual(h.calls.map(call => call.slice(0, 2)), [['ask', 'definition'], ['ask', 'references']]);
  }
});
test('shared menus guard action dispatch against text, tab, host and editor ownership changes', () => {
  for (const invalidate of [h => { h.context.edit.draft = 'changed'; }, h => { h.context.files.activeTab = {}; },
    h => { h.context.identity = { host: 'other', version: 2 }; }, h => { h.current = false; }]) {
    const h = menuFixture(); h.current = true;
    h.menu({ line: 0, character: 0 }, 'name', { x: 10, y: 20 }, () => h.current);
    const items = h.context.symbolMenu.items;
    invalidate(h);
    for (const item of items) if (!item.disabled) item.onSelect();
    assert.deepEqual(h.calls, []);
  }
  const h = menuFixture();
  h.menu({ line: 0, character: 0 }, 'name', { x: 10, y: 20 }, () => true);
  h.context.symbolMenu.items[0].onSelect();
  assert.equal(h.calls[0][0], 'ask');
  assert.equal(h.calls[0][1], 'definition');
  assert.equal(h.calls[0][4](), true, 'ownership continues through the synchronization wait');
});
test('a menu on selected text offers Copy only and holds the selection once the menu has focus', async () => {
  const h = menuFixture(), { selectionMenu } = h;
  let held = 0, released = 0;
  selectionMenu('two words', { x: 1, y: 2 }, () => { held++; return () => { released++; }; });
  assert.deepEqual(h.context.symbolMenu.items.map(i => i.label), ['Copy'], 'no symbol actions on a selection');
  assert.equal(held, 0, 'held only once the menu has taken focus');
  await Promise.resolve(); await Promise.resolve();
  assert.equal(held, 1);
  h.context.releaseSelection();
  assert.equal(released, 1);
  h.context.symbolMenu.items[0].onSelect();
  assert.deepEqual(h.calls, [['copy', 'two words']]);
  // A menu replaced before it showed never holds.
  selectionMenu('a', { x: 1, y: 2 }, () => { held++; });
  h.context.symbolMenu = null;
  await Promise.resolve(); await Promise.resolve();
  assert.equal(held, 1);
});

test('setup disclosure is idempotent, reveals settings with existing findings and never approves', async () => {
  const panel = readFileSync(new URL('../src/lib/components/common/go-to-file/ProblemsPanel.svelte', import.meta.url), 'utf8');
  const body = panel.slice(panel.indexOf('  export async function openSettings('), panel.indexOf('  // Dragging the top edge'));
  const js = ts.transpileModule(body.replace('export async', 'async'), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
  for (const expanded of [false, true]) {
    const calls = [];
    const settings = { focus: () => calls.push('focus'), getBoundingClientRect: () => ({ top: 400 }) };
    let scrollTop = 900;
    const ctx = { expanded, settingsOpen: false, tick: async () => calls.push('tick'), details: { clientTop: 0, get scrollTop() { return scrollTop; }, set scrollTop(v) { scrollTop = v; calls.push('reveal'); }, getBoundingClientRect: () => ({ top: 100 }) },
      section: { querySelector(selector) { assert.equal(selector, '.settings'); return settings; } } };
    const open = new Function('ctx', `with(ctx) { ${js}; return openSettings; }`)(ctx);
    await open(); await open();
    assert.equal(ctx.expanded, true); assert.equal(ctx.settingsOpen, true);
    assert.deepEqual(calls, ['tick', 'focus', 'reveal', 'tick', 'focus', 'reveal'], 'each activation aligns the settings card and stays open');
  }
  // The dialog reaches this through the component instance binding, which only exposes exported functions.
  assert.match(panel.slice(0, panel.indexOf('<section')), /export async function openSettings\(/, 'openSettings must be exported or the status and menu routes silently do nothing');
  const dialog = readFileSync(new URL('../src/lib/components/common/GoToFileDialog.svelte', import.meta.url), 'utf8');
  const source = dialog.slice(dialog.indexOf('  async function openCodeSettings('), dialog.indexOf('  let deliveredLink:'));
  const jsDialog = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
  const calls = [], ctx = { files: { open: true }, contentSearch: { mode: 'files' }, presentation: true,
    tick: async () => calls.push('tick'), problems: { openSettings: async () => calls.push('settings') } };
  const open = new Function('ctx', `with(ctx) { ${jsDialog}; return openCodeSettings; }`)(ctx);
  await open();
  assert.equal(ctx.presentation, false); assert.deepEqual(calls, ['tick', 'settings']);
  ctx.contentSearch.mode = 'history'; await open();
  assert.deepEqual(calls, ['tick', 'settings'], 'history cannot disclose working-project settings');
});
const askSource = viewerSource.slice(viewerSource.indexOf('  async function askSymbol('), viewerSource.indexOf('  export function navigateReference('));
const askJS = ts.transpileModule(askSource, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
function answers(navigate, blocker, path = 'main.ts') {
  const tab = { path }, opened = [], notices = [];
  const context = {
    files: { selected: tab.path, selectedPath: tab.path, activeTab: tab }, codeDiagnostics: { navigate }, symbolSequence: 0,
    symbolBlocker: blocker, symbolPicker: null, editing: false, text: '\n\n   symbol', wordRange,
    contentSearch: { showReferences(...args) { context.references = args; } },
    closeSymbolPopups() { context.symbolSequence++; },
    showSymbolNotice: (...args) => notices.push(args),
    openLocation: (...args) => opened.push(args), symbolStart: p => ({ start: p, end: p }),
    locationList, symbolLabel: p => p, emptyNavigationAnswer, languageOfPath, rankRubyDefinitions,
  };
  const ask = new Function('ctx', `with(ctx) { ${askJS}; return askSymbol; }`)(context);
  return { context, opened, notices, ask: op => ask(op, { line: 2, character: 3 }, { x: 10, y: 20 }) };
}
test('viewer opens a single definition, lists multiple results and explains empty or blocked answers', async () => {
  const location = place('target.ts');
  const h = answers(async () => ({ kind: 'result', result: { locations: [location] } }));
  await h.ask('definition');
  assert.equal(h.opened[0][0], location);
  assert.equal(h.opened[0][1].path, 'main.ts');
  await h.ask('references');
  assert.equal(h.context.references[0], 'symbol');
  assert.deepEqual(h.context.references[1], [location]);
  assert.equal(h.opened.length, 1);
  const empty = answers(async () => ({ kind: 'result', result: { locations: [] } }));
  await empty.ask('definition');
  assert.equal(empty.notices[0][0], 'No definition found');
  assert.equal(empty.notices[0][2], 2000);
  // ruby-lsp answers nothing for symbols it has not indexed yet; the notice says so, for longer.
  const ruby = answers(async () => ({ kind: 'result', result: { locations: [] } }), undefined, 'lib/app.rb');
  await ruby.ask('references');
  assert.deepEqual(ruby.notices[0].filter(x => typeof x !== 'object'), ['No references found. ruby-lsp may still be indexing the project; try again in a moment.', 4000]);
  // Many definitions in one gem: the picker names the gem folder once.
  const gem = '/h/gems/activejob-8.1.4.1/lib/active_job';
  const many = answers(async () => ({ kind: 'result', result: { locations: [{ ...place(`${gem}/base.rb`, 16), external: true }, { ...place(`${gem}/core.rb`, 2), external: true }] } }), undefined, 'app/jobs/application_job.rb');
  await many.ask('definition');
  assert.equal(many.context.symbolPicker.heading, gem);
  assert.deepEqual(many.context.symbolPicker.items.map(i => [i.label, i.title]), [['base.rb:17:3', `${gem}/base.rb (read-only, outside the project)`], ['core.rb:3:3', `${gem}/core.rb (read-only, outside the project)`]]);
  many.context.symbolPicker.items[1].onSelect();
  assert.equal(many.opened[0][0].path, `${gem}/core.rb`);
  // A Ruby constant's own file comes first; other words, languages and references keep the server's order.
  const reopened = [`${gem}/arguments.rb`, `${gem}/base.rb`, '/h/gems/activejob-8.1.4.1/lib/active_job.rb'].map(p => ({ ...place(p), external: true }));
  const rubyAsk = async (path, text, op = 'definition') => {
    const h = answers(async () => ({ kind: 'result', result: { locations: reopened } }), undefined, path);
    h.context.text = text;
    await h.ask(op);
    return h;
  };
  const ranked = await rubyAsk('app/jobs/application_job.rb', '\n\nActiveJob::Base'), top = ranked.context.symbolPicker;
  top.items[0].onSelect();
  assert.equal(ranked.opened[0][0], reopened[2], 'each row opens its own place');
  assert.equal(top.heading, '/h/gems/activejob-8.1.4.1/lib');
  assert.deepEqual(top.items.map(i => i.label), ['active_job.rb:1:3', 'active_job/arguments.rb:1:3', 'active_job/base.rb:1:3']);
  // The asked position (column 3) is on Base here.
  assert.deepEqual((await rubyAsk('a.rb', '\n\n::Base')).context.symbolPicker.items.map(i => i.label)[0], 'active_job/base.rb:1:3');
  assert.deepEqual((await rubyAsk('a.rb', '\n\nxx.base')).context.symbolPicker.items.map(i => i.label)[0], 'active_job/arguments.rb:1:3', 'a method is not a file name');
  assert.deepEqual((await rubyAsk('a.ts', '\n\nActiveJob')).context.symbolPicker.items.map(i => i.label)[0], 'active_job/arguments.rb:1:3', 'Ruby files only');
  assert.deepEqual((await rubyAsk('a.rb', '\n\nActiveJob', 'references')).context.references[1], reopened, 'references keep their order');
  const blocked = answers(() => assert.fail('must not request'), 'Install the language server first.');
  await blocked.ask('definition');
  assert.equal(blocked.notices[0][0], 'Install the language server first.');
});
test('viewer drops answers superseded by another request or a popup close', async () => {
  const pending = [];
  const h = answers(() => new Promise(resolve => pending.push(resolve)));
  const first = h.ask('definition'), second = h.ask('definition');
  pending[1]({ kind: 'result', result: { locations: [place('new.ts')] } });
  await second;
  pending[0]({ kind: 'result', result: { locations: [place('old.ts')] } });
  await first;
  assert.deepEqual(h.opened.map(args => args[0].path), ['new.ts']);
});

const card = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/HoverCard.svelte', import.meta.url), [
  ["import { handleCopyCodeClick } from '../../../copycode';", 'const handleCopyCodeClick = () => {};'],
]);
test('hover card renders safe pre-rendered content, scroll limit and truncation notice', () => {
  const html = render(card.default, { props: { html: hoverHtml({ kind: 'plaintext', value: '<img onerror=attack>' }), truncated: true, x: 10, y: 20, maxHeight: 300, onenter() {}, onleave() {} } }).body;
  assert.match(html, /role="tooltip"/);
  assert.match(html, /max-height:\s*300px/);
  assert.match(html, /&lt;img onerror=attack&gt;/);
  assert.match(html, /The description was cut short/);
});


function viewerHistory(context) {
  const source = viewerSource.slice(viewerSource.indexOf('  function currentLocation()'), viewerSource.indexOf('  /**\n   * Definition or references'))
    .replace(/let historyBlocked = \$derived\([^\n]*\n/, '').replace('export function goHistory', 'function goHistory');
  const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
  return new Function('ctx', `with(ctx) { ${js}; return { currentLocation, openLocation, openPlace, goHistory }; }`)(context);
}
// The files store's history, as the viewer uses it; it notes whether an open ran inside withoutHistory.
function historyFiles(tab) {
  const history = new SourceHistory();
  const files = { activeTab: tab, dir: '/p', paused: false, opened: [], kept: [], history,
    visit: p => history.visit(p),
    stepHistory: (direction, current) => (direction === 'back' ? history.goBack(current) : history.goForward(current)),
    withoutHistory(open) { files.paused = true; try { return open(); } finally { files.paused = false; } },
    keepTab: id => files.kept.push(id),
    openExternal: path => files.opened.push({ path, external: true, paused: files.paused }) };
  return files;
}

test('history reads the preview, the live editor/source selection, an old landing, else the file', () => {
  const tab = { path: 'a.ts' };
  const ctx = { files: historyFiles(tab), showPreview: false, editing: true, editor: { caret: () => ({ line: 7, character: 9 }) },
    landed: place('a.ts', 1), codeScroll: undefined, symbolStart: at => ({ start: at, end: at }) };
  const { currentLocation } = viewerHistory(ctx);
  assert.deepEqual(currentLocation().range.start, { line: 7, character: 9 });
  ctx.editing = false;
  ctx.codeScroll = { ownerDocument: { getSelection: () => ({ anchorNode: {}, anchorOffset: 4 }) }, contains: () => true };
  ctx.sourcePosition = () => ({ line: 8, character: 4 });
  assert.deepEqual(currentLocation().range.start, { line: 8, character: 4 });
  ctx.codeScroll = undefined;
  assert.deepEqual(currentLocation(), ctx.landed, 'landing is only a fallback before source selection is available');
  ctx.landed = place('other.ts', 3);
  assert.deepEqual(currentLocation(), { path: 'a.ts' }, 'otherwise the file itself, its tab keeps the scroll');
  ctx.files.activeTab = { path: 'doc.md' };
  ctx.showPreview = true;
  assert.deepEqual(currentLocation(), { path: 'doc.md', preview: true }, 'the preview wins over any selection');
  ctx.files.activeTab = { path: '/lib/x.ts', external: true };
  ctx.showPreview = false;
  assert.deepEqual(currentLocation(), { path: '/lib/x.ts', external: true });
  ctx.files.activeTab = undefined;
  assert.equal(currentLocation(), undefined);
});

test('a definition records where it came from once; steps go back and forward, preview and skipped paths included', () => {
  const tab = { path: 'a.ts' };
  const files = historyFiles(tab);
  const started = [], picked = [], notices = [];
  const ctx = { files, showPreview: false, editing: false, codeScroll: undefined, landed: null, navigation: 'old', previewOn: false,
    frozen: false, blocked: null, historyBlocked: true, symbolStart: at => ({ start: at, end: at }), tabId: t => t.path,
    problemPath: p => !p.startsWith('gone'), filePreview: p => (p.endsWith('.md') ? 'markdown' : null),
    notify: text => notices.push(text), NOTICES: { symbol: { open: 'cannot open' } },
    startNavigation: (path, kind, range, _locate, keep, outside) => { started.push({ path, line: range.start.line, keep, outside, paused: files.paused }); files.activeTab = { path }; },
    pick: path => { picked.push({ path, paused: files.paused }); files.activeTab = { path }; } };
  const viewer = viewerHistory(ctx);

  // Go to Definition: one place recorded (the one asked from), the open does not record again.
  viewer.openLocation(place('b.ts', 4), { path: 'a.ts', range: place('a.ts', 2).range });
  assert.deepEqual(files.kept, ['a.ts'], 'the file navigated from stays open');
  assert.deepEqual(started, [{ path: 'b.ts', line: 4, keep: false, outside: undefined, paused: true }]);
  assert.equal(files.history.canGoBack, true);
  assert.equal(files.history.goBack(undefined).path, 'a.ts');
  assert.equal(files.history.canGoBack, false, 'exactly one place for one jump');

  // Blocked steps keep the stacks.
  files.history.visit({ path: 'doc.md', preview: true });
  files.activeTab = { path: 'b.ts' };
  ctx.landed = place('b.ts', 4);
  viewer.goHistory('back');
  assert.equal(files.history.canGoBack, true, 'blocked back does not consume a place');
  assert.equal(picked.length, 0);

  // Back to a Markdown file left in preview shows the preview, without recording.
  ctx.historyBlocked = false;
  viewer.goHistory('back');
  assert.deepEqual(picked, [{ path: 'doc.md', paused: true }]);
  assert.equal(ctx.previewOn, true);
  assert.equal(ctx.navigation, null);
  assert.equal(ctx.landed, null);
  assert.equal(files.history.canGoBack, false);
  // Forward returns to where the step left, revealing the range.
  ctx.showPreview = true;
  viewer.goHistory('forward');
  assert.deepEqual(started.at(-1), { path: 'b.ts', line: 4, keep: false, outside: undefined, paused: true });
  assert.deepEqual(files.history.goBack(undefined), { path: 'doc.md', preview: true }, 'and back again finds the preview');
  ctx.showPreview = false;

  // A path no longer in the project is dropped and the step goes past it.
  files.history.clear();
  files.history.visit({ path: 'c.ts' });
  files.history.visit({ path: 'gone.ts' });
  files.activeTab = { path: 'b.ts' };
  ctx.landed = null;
  viewer.goHistory('back');
  assert.deepEqual(picked.at(-1), { path: 'c.ts', paused: true });
  assert.equal(ctx.previewOn, true, 'a source file leaves the preview toggle alone');
  assert.equal(files.history.canGoBack, false);
  assert.equal(files.history.goForward(undefined).path, 'b.ts', 'forward has the place the step left, not the dropped one');
  assert.equal(files.history.canGoForward, false);

  // An external place reopens read-only; a Markdown place left as source shows the source.
  viewer.openPlace({ path: '/lib/x.ts', external: true });
  assert.deepEqual(files.opened, [{ path: '/lib/x.ts', external: true, paused: true }]);
  viewer.openPlace({ path: 'doc.md' });
  assert.equal(ctx.previewOn, false);
  viewer.openPlace({ path: 'gone.md' });
  assert.deepEqual(notices, ['cannot open']);
});

test('viewer forwards the draft current guard and one-second sync bound, discarding changed draft answers', async () => {
  let resolve, args;
  const h = answers((...a) => { args = a; return new Promise(r => { resolve = r; }); });
  h.context.DRAFT_SYNC_WAIT = 1000;
  let current = true;
  const ask = new Function('ctx', `with(ctx) { ${askJS}; return askSymbol; }`)(h.context);
  const pending = ask('definition', { line: 2, character: 3 }, { x: 10, y: 20 }, () => current);
  assert.equal(args[3].wait, 1000);
  assert.equal(args[3].current(), true);
  current = false;
  resolve({ kind: 'result', result: { locations: [place('target.ts')] } });
  await pending;
  assert.equal(h.opened.length, 0);
});

test('a long context menu scrolls inside the window and shows its heading', async () => {
  const menu = await loadServerComponent(new URL('../src/lib/components/common/ContextMenu.svelte', import.meta.url), [
    ["import { onMount } from 'svelte';", 'const onMount = () => {};'],
  ]);
  const items = [{ label: 'base.rb:17:9', onSelect() {} }, { label: 'core.rb:3:8', onSelect() {} }];
  const html = render(menu.default, { props: { x: 0, y: 0, items, heading: '~/gems/activejob/lib', ariaLabel: 'Definitions', onClose() {} } }).body;
  assert.match(html, /<div class="context-menu-heading[^"]*" title="~\/gems\/activejob\/lib">~\/gems\/activejob\/lib<\/div>/);
  assert.equal(html.match(/role="menuitem"/g).length, 2);
  assert.doesNotMatch(render(menu.default, { props: { x: 0, y: 0, items, ariaLabel: 'Definitions', onClose() {} } }).body, /context-menu-heading/);
  const source = readFileSync(new URL('../src/lib/components/common/ContextMenu.svelte', import.meta.url), 'utf8');
  assert.match(source, /max-height: calc\(100vh - 8px\);\s*overflow-y: auto;/);
  // Scrolling the menu itself does not dismiss it.
  assert.match(source, /onscrollcapture=\{handleScroll\}/);
  assert.match(source, /function handleScroll\(e: Event\) \{\s*if \(!menu\?\.contains\(e\.target as Node\)\) close\(false\);/);
});

test('Ruby constants name their files the way ActiveSupport underscores them', () => {
  for (const [name, file] of [['ActiveJob', 'active_job'], ['HTTPClient', 'http_client'], ['URI', 'uri'], ['JSONParser2', 'json_parser2'], ['Base64', 'base64'], ['String', 'string'], ['ID3Tag', 'id3_tag']]) {
    assert.equal(rubyFileName(name), file, name);
  }
});

test('a reopened Ruby constant lists its own file first', () => {
  const at = paths => paths.map(path => ({ path }));
  const order = (list, word, before = '') => rankRubyDefinitions(at(list), word, before).map(l => l.path);
  const gem = ['/g/lib/active_job/queue_name.rb', '/g/lib/active_job_extra.rb', '/g/lib/active_job/base.rb', '/g/lib/active_job.rb', '/g/lib/other/base.rb'];
  // The whole path, then the word alone, then the rest in the server's order.
  assert.deepEqual(order(gem, 'ActiveJob'), ['/g/lib/active_job.rb', '/g/lib/active_job/queue_name.rb', '/g/lib/active_job_extra.rb', '/g/lib/active_job/base.rb', '/g/lib/other/base.rb']);
  assert.deepEqual(order(gem, 'Base', 'class X < ActiveJob::'), ['/g/lib/active_job/base.rb', '/g/lib/other/base.rb', '/g/lib/active_job/queue_name.rb', '/g/lib/active_job_extra.rb', '/g/lib/active_job.rb']);
  // The qualifier picks between files of one name; a name ends at a folder.
  assert.deepEqual(order(['/g/other/base.rb', '/g/active_job/base.rb'], 'Base', 'ActiveJob::'), ['/g/active_job/base.rb', '/g/other/base.rb']);
  assert.deepEqual(order(['/g/inactive_job.rb', '/g/x/active_job.rb'], 'ActiveJob'), ['/g/x/active_job.rb', '/g/inactive_job.rb']);
  // A leading :: is ignored; a constant glued to a word is no qualifier.
  assert.deepEqual(order(gem, 'Base', 'x = ::ActiveJob::')[0], '/g/lib/active_job/base.rb');
  assert.deepEqual(order(['/g/other/base.rb', '/g/active_job/base.rb'], 'Base', 'xActiveJob::'), ['/g/other/base.rb', '/g/active_job/base.rb']);
  // Project paths are relative; RBS signatures and Windows paths count too.
  assert.deepEqual(order(['lib/a.rb', 'app/jobs/application_job.rb'], 'ApplicationJob')[0], 'app/jobs/application_job.rb');
  assert.deepEqual(order(['/r/core/symbol.rbs', '/r/core/string.rbs'], 'String')[0], '/r/core/string.rbs');
  assert.deepEqual(order(['C:\\g\\x.rb', 'C:\\g\\active_job\\base.rb'], 'Base', 'ActiveJob::')[0], 'C:\\g\\active_job\\base.rb');
  // Lowercase words are methods or variables: the order is the server's.
  assert.deepEqual(order(['/g/x.rb', '/g/perform.rb'], 'perform'), ['/g/x.rb', '/g/perform.rb']);
  const same = at(['/a.rb']);
  assert.equal(rankRubyDefinitions(same, 'perform', ''), same);
});

test('history places: a preview or a missing range is its own place; rename and delete follow paths', () => {
  const h = new SourceHistory();
  h.visit({ path: 'a.md', preview: true });
  h.visit({ path: 'a.md' });
  h.visit({ path: 'a.md' });
  h.visit(place('a.md', 3));
  assert.deepEqual(h.goBack(undefined), place('a.md', 3));
  assert.deepEqual(h.goBack(undefined), { path: 'a.md' }, 'the same file without a range merges');
  assert.deepEqual(h.goBack(undefined), { path: 'a.md', preview: true }, 'the preview differs from the source');

  const r = new SourceHistory();
  for (const p of ['src/a.ts', 'src-old/b.ts', 'src/c.ts', 'x.ts']) r.visit({ path: p });
  r.visit({ path: '/abs/src/d.ts', external: true });
  r.remap((p) => (p === 'src' || p.startsWith('src/') ? 'lib' + p.slice(3) : p));
  const paths = (history) => { const out = []; let p; while ((p = history.goBack(undefined))) out.push(p.path); return out; };
  assert.deepEqual(paths(r), ['/abs/src/d.ts', 'x.ts', 'lib/c.ts', 'src-old/b.ts', 'lib/a.ts'], 'prefix boundaries hold; external places never move');

  const d = new SourceHistory();
  for (const p of ['a.ts', 'gone.ts', 'a.ts', 'b.ts']) d.visit({ path: p });
  d.forget((p) => p.path !== 'gone.ts');
  assert.deepEqual(paths(d), ['b.ts', 'a.ts'], 'neighbours that became the same place merge');
});
