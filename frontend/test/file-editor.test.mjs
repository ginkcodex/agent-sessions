import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';
import ts from 'typescript';
import { EditorState, Text } from '@codemirror/state';
import { forEachDiagnostic, diagnosticCount } from '@codemirror/lint';
import { EditorView, keymap, lineNumberMarkers } from '@codemirror/view';
import { undo, redo, undoDepth, indentMore, insertNewlineAndIndent } from '@codemirror/commands';
import { language, indentUnit } from '@codemirror/language';
import { searchPanelOpen } from '@codemirror/search';
import { CompletionContext, clearSnippet, completionStatus, hasNextSnippetField, nextSnippetField, prevSnippetField, startCompletion } from '@codemirror/autocomplete';
import { javascriptLanguage, jsxLanguage, tsxLanguage, typescriptLanguage } from '@codemirror/lang-javascript';
import { jsonLanguage } from '@codemirror/lang-json';
import { yamlLanguage } from '@codemirror/lang-yaml';
import { loadServerComponent, loadServerModule } from './helpers.mjs';
import { projectSourceDiagnostics } from '../src/lib/sourceDiagnostics.ts';

const SRC = new URL('../src/', import.meta.url);
const EDITOR_URL = new URL('lib/fileEditor.ts', SRC);
const COMPONENT_URL = new URL('lib/components/common/go-to-file/FileEditor.svelte', SRC);

// ---------------------------------------------------------------------------
// Runtime harness: the real fileEditor.ts with only EditorView swapped for a
// DOM-free stand-in. Every extension, facet, compartment and command is the
// real CodeMirror one, so state, history, search and language behave for real.
// ---------------------------------------------------------------------------

const views = [];

// Node's EventTarget does not match the capture flag on removal, so track
// listeners by (type, capture) explicitly as the DOM does.
class FakeDom {
  listeners = [];
  classes = new Set();
  classList = { add: (c) => this.classes.add(c), remove: (c) => this.classes.delete(c) };
  // The DOM selection, as text offsets; writes are recorded.
  domSelections = [];
  domSelection = { anchorNode: null, anchorOffset: 0, focusNode: null, focusOffset: 0,
    setBaseAndExtent: (anchorNode, anchorOffset, focusNode, focusOffset) => {
      Object.assign(this.domSelection, { anchorNode, anchorOffset, focusNode, focusOffset });
      this.domSelections.push([anchorNode, anchorOffset, focusNode, focusOffset]);
    } };
  ownerDocument = { getSelection: () => this.domSelection };
  addEventListener(type, fn, capture) { this.listeners.push({ type, fn, capture: Boolean(capture) }); }
  removeEventListener(type, fn, capture) {
    this.listeners = this.listeners.filter((l) => !(l.type === type && l.fn === fn && l.capture === Boolean(capture)));
  }
  dispatchEvent(event) {
    for (const l of this.listeners.filter((l) => l.type === event.type && l.capture)) l.fn(event);
  }
}

class FakeEditorView {
  static scrollIntoView = EditorView.scrollIntoView;
  static theme = EditorView.theme;
  static decorations = EditorView.decorations;
  static lineWrapping = EditorView.lineWrapping;
  static editable = EditorView.editable;
  static updateListener = EditorView.updateListener;
  static contentAttributes = EditorView.contentAttributes;
  static domEventHandlers = EditorView.domEventHandlers;
  static domEventObservers = EditorView.domEventObservers;

  constructor({ parent, state }) {
    this.parent = parent;
    this.state = state;
    this.dom = new FakeDom();
    // CodeMirror commands call `dispatch` detached from the view.
    this.dispatch = this.dispatch.bind(this);
    this.scrollDOM = Object.assign(new FakeDom(), { scrollTop: 0, scrollLeft: 0 });
    this.documentPadding = { top: 8, bottom: 8 };
    this.root = { activeElement: null };
    this.destroyed = false;
    this.focused = 0;
    this.measures = [];
    this.dispatches = 0;
    this.lateDispatches = 0;
    views.push(this);
  }

  dispatch(...specs) {
    if (this.destroyed) this.lateDispatches++;
    this.dispatches++;
    const tr = this.state.update(...specs);
    this.state = tr.state;
    for (const effect of tr.effects) {
      const probe = EditorView.scrollIntoView(0);
      if (effect.is(probe.type)) this.scrollTarget = effect.value.range.head;
    }
    for (const listener of this.state.facet(EditorView.updateListener)) {
      listener({ view: this, state: this.state, docChanged: tr.docChanged, transactions: [tr] });
    }
  }

  setState(state) { this.state = state; }
  contentDOM = { contains: (node) => node === 'text' };
  domAtPos(pos) { return { node: 'text', offset: pos }; }
  posAtDOM(node, offset) { return offset; }
  requestMeasure(measure) { this.measures.push(measure); }
  flushMeasures() {
    for (const m of this.measures.splice(0)) m.write(m.read(this), this);
    this.flushScrollTarget();
  }
  flushScrollTarget() {
    if (this.scrollTarget !== undefined) {
      this.scrollDOM.scrollTop = this.lineBlockAt(this.scrollTarget).top + this.documentPadding.top;
      this.scrollTarget = undefined;
    }
  }
  lineBlockAt(from) { return { from, top: (this.state.doc.lineAt(from).number - 1) * 20 }; }
  lineBlockAtHeight(height) {
    this.flushMeasures();
    const number = Math.min(this.state.doc.lines, Math.floor(height / 20) + 1);
    return this.lineBlockAt(this.state.doc.line(number).from);
  }
  coordsAtPos() { return { left: 10, right: 10, top: 0, bottom: 20 }; }
  posAtCoords() { return this.pointerPos ?? this.state.selection.main.head; }
  defaultCharacterWidth = 8;
  plugin() { return null; }
  focus() { this.focused++; }
  destroy() { this.destroyed = true; }
}

globalThis.__fileEditorTestView = FakeEditorView;

async function loadEditorModule() {
  const viewImport = "import { EditorView, GutterMarker, closeHoverTooltips, drawSelection, hasHoverTooltips, highlightActiveLine, hoverTooltip, keymap, lineNumberMarkers, lineNumbers, tooltips, type KeyBinding, type Tooltip } from '@codemirror/view';";
  let source = readFileSync(EDITOR_URL, 'utf8');
  assert.ok(source.includes(viewImport), 'fileEditor.ts no longer has the expected @codemirror/view import');
  source = source.replace(viewImport, [
    "import { EditorView as RealEditorView, GutterMarker, closeHoverTooltips, drawSelection, hasHoverTooltips, highlightActiveLine, hoverTooltip, keymap, lineNumberMarkers, lineNumbers, tooltips, type KeyBinding, type Tooltip } from '@codemirror/view';",
    'void RealEditorView;',
    'const EditorView = globalThis.__fileEditorTestView;',
  ].join('\n'));
  // data: modules cannot resolve bare specifiers; pin every CodeMirror package
  // (including the lazily imported languages) to the same files the test uses.
  source = source.replace(/'(@(?:codemirror|lezer)\/[a-z-]+)'/g, (_, name) => JSON.stringify(import.meta.resolve(name)));
  source = source.replace("'./fullscreen'", JSON.stringify(new URL('fullscreen.ts', EDITOR_URL).href));
  source = source.replace("'./sourceDiagnostics'", JSON.stringify(new URL('sourceDiagnostics.ts', EDITOR_URL).href));
  source = source.replace("'./problemNavigation'",  JSON.stringify(new URL('problemNavigation.ts', EDITOR_URL).href));
  source = source.replace("'./problemOverview'", JSON.stringify(new URL('problemOverview.ts', EDITOR_URL).href));
  source = source.replace("'./files'", JSON.stringify(new URL('files.ts', EDITOR_URL).href));
  source = source.replace("'./searchReplace'", JSON.stringify(new URL('searchReplace.ts', EDITOR_URL).href));
  source = source.replace("'./sourceNavigation'", JSON.stringify(new URL('sourceNavigation.ts', EDITOR_URL).href));
  source = source.replace("'./editorCompletion'", JSON.stringify(new URL('editorCompletion.ts', EDITOR_URL).href));
  source = source.replace("'./editorKeys'", JSON.stringify(new URL('editorKeys.ts', EDITOR_URL).href));
  source = source.replace("import { handleCopyCodeClick } from './copycode';", "const handleCopyCodeClick = () => {};");
  source = source.replace("'./codeHighlight'", JSON.stringify(new URL('codeHighlight.ts', EDITOR_URL).href));
  source = source.replace("'@replit/codemirror-lang-svelte'", JSON.stringify(import.meta.resolve('@replit/codemirror-lang-svelte')));
  const js = ts.transpileModule(source, {
    fileName: fileURLToPath(EDITOR_URL),
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
  }).outputText;
  return import(`data:text/javascript,${encodeURIComponent(js)}`);
}

const { createFileEditor, acceptOrBreakLine, completionKeys, documentWords, codeSuggestions, completionSources, LANGUAGE_SLOTS, completionTrigger, SERVER_SUGGESTION_WAIT, RESOLVE_ACCEPT_WAIT, navigationKeymap, symbolMouseDown, symbolContextMenu, symbolPress, symbolKeyDown, symbolHover, editorPosition, wordAtSide } = await loadEditorModule();

const isMac = /Mac|iPhone|iPad/.test(globalThis.navigator?.platform ?? '');

function lintInput(id, ranges = [{ start: { line: 0, character: 1 }, end: { line: 0, character: 3 } }]) {
  return { tab: id, path: 'x.txt', context: 'host/root/generation', current: true, dirty: false, revision: 'r1',
    findings: ranges.map(range => {
      const diagnostic = { path: 'x.txt', range, severity: 'information', source: '<checker>', code: 'C1', message: '<img onerror=evil>', freshness: 'current', basis: { kind: 'saved', revision: 'r1' } };
      return { diagnostic, entries: [{ providerId: 'checker', diagnostic }] };
    }) };
}

test('Stage 1 real store eligible set agrees with editor lint and source across empty, stale and uncertain draft updates', async () => {
  const lib = name => JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href);
  const { CodeDiagnosticsStore } = await loadServerModule(new URL('../src/lib/stores/codeDiagnostics.svelte.ts', import.meta.url), [
    ["import { api } from '../api';", 'const api = {};'], ["import { epoch } from '../link';", 'const epoch = () => 0;'],
    ["'../codeDiagnostics'", lib('codeDiagnostics')], ["'../codeDiagnosticCounts'", lib('codeDiagnosticCounts')], ["'../codeNavigation'", lib('codeNavigation')],
  ]);
  const id = {}, input = lintInput(id), context = { connectionId: 'c', attachmentId: 'a', generation: 'g' };
  const d = input.findings[0].diagnostic;
  let emit;
  const snapshot = { contractVersion: 1, context, sequence: 0, approval: { state: 'approved' }, providers: [], resultSets: ['a', 'b'].map(providerId => ({ providerId, origin: 'checker', scope: { kind: 'project' }, diagnostics: [{ ...d, source: providerId }] })) };
  const store = new CodeDiagnosticsStore({
    onEvent(name, fn) { if (name === 'codeDiagnostics:updated') emit = fn; return () => {}; },
    async attachCodeDiagnostics() { return { project: { hostId: 'host', root: '/p' }, context, snapshot }; },
    async detachCodeDiagnostics() { return { context }; },
  }, () => 0);
  await store.attach('/p', { phase: 'local', hostId: 'host', generation: 0, capabilities: { codeDiagnostics: true } });
  const mounted = mount({ id, path: 'x.txt', text: 'a😀z\r\n' });
  const before = mounted.view.state.doc.toString();
  const depth = undoDepth(mounted.view.state);
  for (const [sequence, mode] of ['findings', 'dirty', 'uncertain', 'stale', 'empty'].entries()) {
    const next = structuredClone(snapshot); next.sequence = sequence + 1;
    if (mode === 'stale') next.resultSets.forEach(s => s.diagnostics.forEach(d => d.freshness = 'stale'));
    if (mode === 'empty') next.resultSets.forEach(s => s.diagnostics = []);
    emit({ contractVersion: 1, context, sequence: next.sequence, updateId: `u${sequence}`, partIndex: 0, partCount: 1, snapshot: next });
    // FileViewer collapses dirty/uncertain/conflict to the same overlay guard.
    const dirty = mode === 'dirty' || mode === 'uncertain';
    mounted.editor.update({ ...mounted.options, diagnostics: { ...input, dirty, findings: store.findings } });
    const expected = mode === 'findings' ? 1 : 0;
    assert.equal(diagnosticCount(mounted.view.state), expected);
    assert.equal(store.inline('x.txt', { dirty, revision: 'r1' }).length, expected);
    assert.equal(projectSourceDiagnostics('a😀z\r\n', store.findings, { dirty, revision: 'r1' }).segments.length, expected);
    assert.equal(store.findings.length, mode === 'empty' ? 0 : 1);
    assert.equal(mounted.view.state.doc.toString(), before);
    assert.equal(undoDepth(mounted.view.state), depth);
    assert.deepEqual(mounted.calls.changes, []);
  }
  mounted.editor.destroy(); await store.detach();
});

test('lint uses shared verified ranges against LF state, plain attribution and exclusive UTF16 endpoints', () => {
  const id = {}, diagnostics = lintInput(id, [
    { start: { line: 0, character: 1 }, end: { line: 1, character: 1 } },
    { start: { line: 2, character: 0 }, end: { line: 2, character: 0 } },
    { start: { line: 0, character: 2 }, end: { line: 0, character: 3 } },
    undefined,
  ]);
  const { view, editor, options, calls } = mount({ id, text: '\t😀e\u0301\r\nβ\r\n', path: 'x.txt', diagnostics });
  const actual = [];
  forEachDiagnostic(view.state, (d, from, to) => actual.push([from, to, d.severity, d.source, d.message]));
  // EditorState normalizes CRLF; columns remain UTF16, never visual tab widths.
  assert.deepEqual(actual, [[1, 7, 'info', '<checker> C1', '<img onerror=evil>'], [8, 8, 'info', '<checker> C1', '<img onerror=evil>']]);
  assert.deepEqual(markedLines(view.state), [1, 2, 3]);
  assert.deepEqual(calls.changes, []);
  editor.destroy();
});

// Each marked line number names its worst problem's severity.
function lineSeverities(state) {
  const lines = [];
  for (const set of state.facet(lineNumberMarkers)) {
    for (let cursor = set.iter(); cursor.value; cursor.next()) lines.push([state.doc.lineAt(cursor.from).number, cursor.value.elementClass]);
  }
  return lines;
}

test('line numbers take the worst severity and the overview ruler hears each change once', () => {
  const id = {}, diagnostics = lintInput(id, [
    { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
    { start: { line: 0, character: 1 }, end: { line: 0, character: 2 } },
    { start: { line: 1, character: 0 }, end: { line: 1, character: 1 } },
  ]);
  diagnostics.findings[1].diagnostic.severity = 'error';
  diagnostics.findings[2].diagnostic.severity = 'hint';
  const reports = [];
  const { view, editor, options } = mount({ id, text: 'abc\ndef\n', path: 'x.txt', diagnostics, onproblems: problems => reports.push(problems) });
  assert.deepEqual(lineSeverities(view.state), [[1, 'cm-has-diagnostics cm-severity-error'], [2, 'cm-has-diagnostics cm-severity-hint']]);
  assert.equal(reports.length, 1, 'reported once after mounting');
  assert.deepEqual(reports[0].map(p => [p.range.start.line, p.range.start.character, p.range.end.character, p.severity, p.message, p.source]), [
    [0, 0, 1, 'information', '<img onerror=evil>', '<checker> C1'],
    [0, 1, 2, 'error', '<img onerror=evil>', '<checker> C1'],
    [1, 0, 1, 'hint', '<img onerror=evil>', '<checker> C1'],
  ]);
  editor.update({ ...options });
  assert.equal(reports.length, 1, 'the same problems are not reported again');
  type(view, '!');
  assert.equal(reports.length, 2);
  assert.deepEqual(reports[1], [], 'typing clears the saved markers, and the ruler with them');
  assert.equal(editor.lineTop(1), 28, 'a line top includes the document padding');
  assert.equal(editor.lineTop(99), undefined);
  assert.equal(editor.scrollDOM, view.scrollDOM);
  editor.destroy();
});

// Line numbers marked as having diagnostics, as the viewer marks them.
function markedLines(state) {
  const lines = [];
  for (const set of state.facet(lineNumberMarkers)) {
    for (let cursor = set.iter(); cursor.value; cursor.next()) {
      if (cursor.value.elementClass.split(' ').includes('cm-has-diagnostics')) lines.push(state.doc.lineAt(cursor.from).number);
    }
  }
  return lines;
}

test('lint rejects stale, uncertain, document and invalid ranges but uses verified provider fallback', () => {
  const id = {}, diagnostics = lintInput(id);
  const base = diagnostics.findings[0];
  for (const change of [{ freshness: 'stale' }, { freshness: 'uncertain' }, { basis: { kind: 'document', documentId: 'd' } }, { locationIssue: 'invalidRange' }, { range: { start: { line: -1, character: 0 }, end: { line: 0, character: 9 } } }]) {
    const diagnostic = { ...base.diagnostic, ...change };
    const findings = [{ diagnostic, entries: [{ providerId: 'checker', diagnostic }] }];
    const mounted = mount({ id, text: 'a😀z', path: 'x.txt', diagnostics: { ...diagnostics, findings } });
    assert.equal(diagnosticCount(mounted.view.state), 0);
    mounted.editor.destroy();
  }
  const diagnostic = { ...base.diagnostic, freshness: 'stale' };
  const mounted = mount({ id, text: 'a😀z', path: 'x.txt', diagnostics: { ...diagnostics, findings: [{ diagnostic, entries: [{ providerId: 'stale', diagnostic }, ...base.entries] }] } });
  assert.equal(diagnosticCount(mounted.view.state), 1);
  mounted.editor.update({ ...mounted.options, diagnostics: undefined });
  assert.equal(diagnosticCount(mounted.view.state), 0, 'revocation clears an already visible marker');
  mounted.editor.destroy();
});

test('live markers use confirmed displayed identity and clear on typing without resetting cached history', () => {
  const id = {}, diagnostics = lintInput(id);
  const diagnostic = { ...diagnostics.findings[0].diagnostic, basis: { kind: 'document', documentId: 'doc', version: 2 } };
  diagnostics.findings = [{ diagnostic, entries: [{ providerId: 'gopls', diagnostic }] }];
  diagnostics.dirty = true;
  diagnostics.live = { documentId: 'doc', version: 2, synced: true };
  const mounted = mount({ id, text: 'a😀z', path: 'x.txt', diagnostics });
  assert.equal(diagnosticCount(mounted.view.state), 1);
  mounted.view.dispatch({ changes: { from: 4, insert: '!' } });
  assert.equal(diagnosticCount(mounted.view.state), 0);
  const depth = undoDepth(mounted.view.state);
  for (const live of [undefined, { documentId: 'other', version: 2, synced: true }, { documentId: 'doc', version: 1, synced: true }, { documentId: 'doc', version: 2, synced: false }]) {
    mounted.editor.update({ ...mounted.options, text: 'a😀z!', diagnostics: { ...diagnostics, live } });
    assert.equal(diagnosticCount(mounted.view.state), 0);
    assert.equal(undoDepth(mounted.view.state), depth);
  }
  mounted.editor.destroy();
  const remount = mount({ ...mounted.options, text: 'a😀z!', diagnostics: { ...diagnostics, live: undefined } });
  assert.equal(diagnosticCount(remount.view.state), 0);
  assert.equal(undoDepth(remount.view.state), depth);
  remount.editor.destroy();
});

test('lint clears in edit/undo transactions and diagnostic updates preserve history, selection, scroll and owners', () => {
  const id = {}, diagnostics = lintInput(id);
  const first = mount({ id, text: 'a😀z', path: 'x.txt', diagnostics });
  const { editor, view, options, calls } = first;
  assert.equal(diagnosticCount(view.state), 1);
  assert.deepEqual(markedLines(view.state), [1]);
  type(view, '!');
  assert.equal(diagnosticCount(view.state), 0, 'no mapped saved marker even before reactive props');
  assert.deepEqual(markedLines(view.state), []);
  undo(view);
  assert.equal(diagnosticCount(view.state), 0, 'undo is not disk freshness proof');
  const depth = undoDepth(view.state), selection = view.state.selection;
  view.scrollDOM.scrollTop = 80;
  editor.update({ ...options, diagnostics: { ...diagnostics, dirty: true } });
  assert.equal(diagnosticCount(view.state), 0);
  editor.update({ ...options, diagnostics });
  assert.equal(diagnosticCount(view.state), 1, 'verified clean revision can reapply');
  assert.equal(undoDepth(view.state), depth);
  assert.equal(view.state.selection, selection);
  assert.equal(view.scrollDOM.scrollTop, 80);
  assert.equal(calls.changes.length, 2);
  assert.equal(calls.saves, 0);
  editor.destroy();
  for (const change of [{ current: false }, { findings: [] }, { revision: 'r2' }, { tab: {} }, { path: 'other' }, { context: '' }, { dirty: true }]) {
    const remount = mount({ ...options, diagnostics: { ...diagnostics, ...change } });
    assert.equal(diagnosticCount(remount.view.state), 0, JSON.stringify(change));
    remount.editor.destroy();
  }
  const remount = mount(options);
  assert.equal(diagnosticCount(remount.view.state), 1);
  remount.editor.destroy();
  const count = remount.view.dispatches;
  remount.editor.update({ ...options, diagnostics: { ...diagnostics, findings: [] } });
  assert.equal(remount.view.dispatches, count);
});

function key(k, mods = {}) {
  const { mod = false, alt = false, shift = false, prevented = false } = mods;
  const event = {
    key: k,
    metaKey: isMac && mod,
    ctrlKey: !isMac && mod,
    altKey: alt,
    shiftKey: shift,
    defaultPrevented: prevented,
    prevented: 0,
    stopped: 0,
    preventDefault() { this.prevented++; this.defaultPrevented = true; },
    stopPropagation() { this.stopped++; },
  };
  return event;
}

function mount(overrides = {}) {
  const calls = { changes: [], saves: 0 };
  const options = {
    id: {},
    text: 'const a = 1;\n',
    path: 'src/a.ts',
    version: 1,
    wrap: false,
    disabled: false,
    onchange: (text) => calls.changes.push(text),
    onsave: () => { calls.saves++; },
    ...overrides,
  };
  const before = views.length;
  const editor = createFileEditor({ tag: 'host' }, options);
  assert.equal(views.length, before + 1);
  return { editor, view: views[views.length - 1], options, calls };
}

test('viewer position overrides cached pixels and reports the current logical line', () => {
  const positions = [];
  const { editor, view } = mount({
    text: Array.from({ length: 100 }, (_, i) => `line ${i + 1}`).join('\n'),
    position: { line: 50, offset: -4, left: 30 },
    onposition: position => positions.push(position),
  });
  view.flushMeasures();
  assert.equal(view.scrollDOM.scrollTop, 49 * 20 + 8 + 4);
  assert.equal(view.scrollDOM.scrollLeft, 30);
  editor.destroy();
  assert.deepEqual(positions.at(-1), { line: 50, offset: -4, left: 30 });
  assert.equal(view.scrollDOM.listeners.length, 0);
});

// Bounded by time, not turns: the first import of a language package takes
// well over 500 event-loop turns on a slow CI runner.
async function settle(predicate, message = 'condition never became true', timeoutMs = 10_000) {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() > deadline) assert.fail(message);
    await new Promise((resolve) => setImmediate(resolve));
  }
}

// Language support and syntax coloring load independently after async imports.
async function languageOf(view, path) {
  const start = view.dispatches;
  await settle(() => view.state.facet(language) !== null, `language for ${path} never loaded`);
  return view.state.facet(language);
}

function type(view, text, at = view.state.doc.length) {
  view.dispatch({ changes: { from: at, insert: text } });
}

function wrapped(state) {
  return state.facet(EditorView.contentAttributes).some((attrs) => attrs?.class === 'cm-lineWrapping');
}

test('switching modes preserves top padding and partially scrolled line offsets after pending editor scrolls', () => {
  for (const target of [{ line: 1, offset: 8, left: 0 }, { line: 40, offset: -7, left: 24 }]) {
    const { editor, view } = mount({
      text: Array.from({ length: 100 }, (_, i) => `line ${i}`).join('\n'),
      position: target,
    });
    editor.focus();
    view.flushMeasures();
    assert.equal(view.scrollDOM.scrollTop, (target.line - 1) * 20 + 8 - target.offset);
    assert.equal(view.scrollDOM.scrollLeft, target.left);
    editor.destroy();
  }
});

test('indentation is inferred from existing content and used by Tab and Enter', async () => {
  for (const unit of ['  ', '    ', '        ', '\t']) {
    const text = `function outer() {\n${unit}if (true) {\n${unit}${unit}run();\n${unit}}\n}\n`;
    const { editor, view } = mount({ text });
    assert.equal(view.state.facet(indentUnit), unit);
    assert.equal(view.state.tabSize, 4);
    await languageOf(view, 'a.ts');
    view.dispatch({ selection: { anchor: text.indexOf('run();') + 6 } });
    assert.equal(insertNewlineAndIndent(view), true);
    assert.ok(view.state.doc.toString().includes(`run();\n${unit}${unit}\n`), unit);
    view.dispatch({ selection: { anchor: 0 } });
    assert.equal(indentMore(view), true);
    assert.ok(view.state.doc.toString().startsWith(`${unit}function`), unit);
    editor.destroy();
  }
});

test('indent detection ignores blank lines and stays stable until external reload', () => {
  const { editor, view, options } = mount({ text: 'root\n    child\n        nested\n    child\n\nroot' });
  assert.equal(view.state.facet(indentUnit), '    ');
  type(view, '\n  temporary');
  assert.equal(view.state.facet(indentUnit), '    ');
  editor.update({ ...options, version: 2, text: 'root\n\tchild\n\t\tnested' });
  assert.equal(view.state.facet(indentUnit), '\t');
  editor.destroy();
  const empty = mount({ text: '' });
  assert.equal(empty.view.state.facet(indentUnit), '  ', 'no evidence keeps the default');
  empty.editor.destroy();
});

test('line selection crosses viewer/editor mounts and reports the cursor line on exit', () => {
  const selections = [];
  const id = {};
  const first = mount({ id, text: 'one\ntwo\nthree', selection: { anchor: 2, head: 3 }, onselection: value => selections.push(value) });
  assert.equal(first.view.state.selection.main.anchor, 4);
  assert.equal(first.view.state.selection.main.head, 8);
  first.editor.destroy();
  assert.deepEqual(selections, [{ anchor: 2, head: 3 }]);
  const second = mount({ id, text: 'one\ntwo\nthree', selection: { anchor: 1, head: 1 } });
  assert.equal(second.view.state.selection.main.head, 0, 'viewer selection overrides cached editor selection');
  second.editor.destroy();
});

test('editor starts from the given text and reports every edit as a full snapshot', () => {
  const { view, calls } = mount({ text: 'one\n' });
  assert.equal(view.state.doc.toString(), 'one\n');
  assert.equal(calls.changes.length, 0, 'configuration must not count as an edit');
  type(view, 'two\n');
  type(view, 'three\n');
  assert.deepEqual(calls.changes, ['one\ntwo\n', 'one\ntwo\nthree\n']);
});

test('a keystroke builds the document string once; the draft coming back is not compared again', () => {
  const { editor, view, options, calls } = mount({ text: 'a\n'.repeat(1000), diagnostics: lintInput(undefined) });
  const toString = Text.prototype.toString;
  let built = 0;
  Text.prototype.toString = function () { built++; return toString.call(this); };
  try {
    type(view, 'x');
    const draft = calls.changes.at(-1);
    assert.equal(built, 1, 'one string for the reported draft');
    editor.update({ ...options, text: draft, diagnostics: lintInput(options.id) });
    assert.equal(built, 1, 'the reported draft is the document: no rebuild, no scan');
    const depth = undoDepth(view.state);
    // Equal text in another string, or with CRLF line breaks, is still the document.
    editor.update({ ...options, text: draft.replace(/\n/g, '\r\n'), diagnostics: lintInput(options.id) });
    assert.equal(built, 1);
    assert.equal(undoDepth(view.state), depth, 'no adoption for the same text');
    assert.equal(view.state.doc.toString(), draft);
  } finally {
    Text.prototype.toString = toString;
  }
});

test('history: undo/redo work and also report the restored text', () => {
  const { view, calls } = mount({ text: 'base' });
  type(view, '!');
  // Adjacent fast edits may share one history group, so undo until exhausted.
  view.dispatch({ changes: { from: 0, insert: '>' } });
  assert.equal(view.state.doc.toString(), '>base!');
  assert.ok(undoDepth(view.state) >= 1);
  while (undo(view));
  assert.equal(view.state.doc.toString(), 'base');
  assert.equal(calls.changes.at(-1), 'base');
  assert.equal(redo(view), true);
  assert.notEqual(view.state.doc.toString(), 'base');
  assert.equal(calls.changes.at(-1), view.state.doc.toString());
});

test('wrap and disabled options are applied and reconfigured without losing the document or history', () => {
  const { editor, view, options } = mount({ text: 'x', wrap: true, disabled: true });
  assert.equal(wrapped(view.state), true);
  assert.equal(view.state.readOnly, true);
  assert.equal(view.state.facet(EditorView.editable), false);
  assert.equal(undo(view), false, 'read-only editors refuse undo');

  editor.update({ ...options, disabled: false });
  assert.equal(view.state.readOnly, false);
  assert.equal(view.state.facet(EditorView.editable), true);
  type(view, 'y');

  editor.update({ ...options, disabled: false, wrap: false, text: 'xy' });
  assert.equal(wrapped(view.state), false);
  assert.equal(view.state.doc.toString(), 'xy', 'echoing the current draft back must not reset the editor');
  assert.equal(undo(view), true, 'history survives option-only updates');
  assert.equal(view.state.doc.toString(), 'x');
});

// The rules a state's themes mount, as CSS text.
const themeRules = (state) => state.facet(EditorView.styleModule).flatMap((module) => module.rules);

test('the editor font comes from the Appearance variables, falling back to today\'s 0.78rem and --font-mono', () => {
  const { view } = mount();
  const rules = themeRules(view.state).join('\n');
  assert.match(rules, /font-size: var\(--editor-font-size, 0\.78rem\)/);
  assert.match(rules, /font-family: var\(--editor-font-family, var\(--font-mono\)\)/);
  assert.doesNotMatch(rules, /font-size: 0\.78rem/, 'no fixed size left to win over the setting');
  // The viewer's SourceView uses the same two values, so the text does not move between modes.
  const viewer = readFileSync(new URL('lib/components/common/go-to-file/SourceView.svelte', SRC), 'utf8');
  const code = viewer.match(/\n  \.code \{([^}]*)\}/)[1];
  assert.match(code, /font-family: var\(--editor-font-family, var\(--font-mono\)\);/);
  assert.match(code, /font-size: var\(--editor-font-size, 0\.78rem\);/);
  assert.match(code, /line-height: 1\.5;/);
});

test('remeasure swaps between two font themes, so CodeMirror measures again, and keeps the document and history', () => {
  const { editor, view } = mount({ text: 'x' });
  type(view, 'y');
  const selection = view.state.selection.main.head;
  const first = view.state.facet(EditorView.styleModule);
  editor.remeasure();
  const second = view.state.facet(EditorView.styleModule);
  // CodeMirror sets mustMeasureContent when the theme facet changes; that
  // facet is the themes' class names, so a different theme module is enough.
  assert.notDeepEqual(second, first);
  assert.ok(themeRules(view.state).some((r) => /editor-font-size/.test(r)), "the swapped-in theme still sets the font");
  editor.remeasure();
  assert.deepEqual(view.state.facet(EditorView.styleModule), first, 'two themes, reused: no new styles per change');
  assert.equal(view.state.doc.toString(), 'xy');
  assert.equal(view.state.selection.main.head, selection);
  assert.equal(undo(view), true, 'history survives a font change');
  editor.destroy();
  const dispatches = view.dispatches;
  editor.remeasure();
  assert.equal(view.dispatches, dispatches, 'inert after destroy');
});

test('FileEditor remeasures when the editor font or size setting changes, not on mount', () => {
  const source = readFileSync(COMPONENT_URL, 'utf8');
  assert.match(source, /let fontSetting = \$derived\(`\$\{preferences\.editorFont\}\\n\$\{preferences\.editorFontSize\}`\);/);
  assert.match(source, /if \(measuredFont !== undefined && measuredFont !== setting\) editor\.remeasure\(\);/);
  const viewer = readFileSync(new URL('lib/components/common/go-to-file/FileViewer.svelte', SRC), 'utf8');
  assert.match(viewer, /void presentationFontSize; void preferences\.editorFont; void preferences\.editorFontSize;/, 'change navigation gathers its lines again');
  assert.match(viewer, /getFontSetting: \(\) => `\$\{preferences\.editorFont\}\\n\$\{preferences\.editorFontSize\}\\n/, 'find paints again');
});

test('languages load lazily by extension, case-insensitively, with JS/TS/JSX dialects', async () => {
  const cases = [
    ['a.js', javascriptLanguage],
    ['a.mjs', javascriptLanguage],
    ['a.ts', typescriptLanguage],
    ['a.cts', typescriptLanguage],
    ['a.jsx', jsxLanguage],
    ['a.tsx', tsxLanguage],
    ['DATA.JSON', jsonLanguage],
    ['ci.yml', yamlLanguage],
  ];
  for (const [path, expected] of cases) {
    const { editor, view } = mount({ path });
    assert.equal(view.state.facet(language), null, `${path}: language must not be applied synchronously`);
    assert.equal(await languageOf(view, path), expected, path);
    editor.destroy();
  }
  // These packages build their Language per call, so compare by name.
  for (const [path, name] of [['a.py', 'python'], ['main.go', 'go'], ['index.htm', 'html'], ['site.css', 'css'], ['README.md', 'markdown'], ['x.Markdown', 'markdown']]) {
    const { editor, view } = mount({ path });
    assert.equal((await languageOf(view, path))?.name, name, path);
    editor.destroy();
  }
  for (const path of ['Makefile', 'notes.txt', 'archive.tar.unknownext']) {
    const { editor, view } = mount({ path });
    const start = view.dispatches;
    await settle(() => view.dispatches > start);
    assert.equal(view.state.facet(language), null, `${path}: plain text`);
    editor.destroy();
  }
});

test('editor uses viewer tokens for builtins, properties, entities and live edits', async () => {
  const { highlightFile } = await import('../src/lib/codeHighlight.ts');
  for (const [path, lang, text] of [
    ['a.ts', 'typescript', 'const x = console.log("<&😀");'],
    ['a.py', 'python', 'print("<&😀") # comment'],
    ['a.json', 'json', '{"key": "<&😀", "n": true}'],
    ['a.css', 'css', '.item { color: red; }'],
    ['a.md', 'markdown', '`agent-sessions_<version>_windows_amd64_setup.exe`\n\n**`_setup.exe`** installer.\n\nPlain text and *intentional italics*.'],
  ]) {
    const { editor, view } = mount({ path, text });
    const ranges = () => view.state.facet(EditorView.decorations).filter(x => typeof x !== 'function');
    await settle(() => ranges().length > 0);
    for (const suffix of ['', '\n']) {
      if (suffix) type(view, suffix);
      const doc = view.state.doc.toString();
      const actual = Array.from({ length: doc.length }, () => []);
      for (const set of ranges()) set.between(0, doc.length, (from, to, mark) => {
        for (let i = from; i < to; i++) actual[i].push(mark.spec.class);
      });
      const html = await highlightFile(doc, lang);
      const expected = [];
      const stack = [];
      for (const token of html.matchAll(/<span class="([^"]*)">|<\/span>|&[^;]+;|[^<&]+/g)) {
        if (token[1] !== undefined) stack.push(token[1]);
        else if (token[0] === '</span>') stack.pop();
        else for (let i = 0; i < (token[0].startsWith('&') ? 1 : token[0].length); i++) expected.push([...stack].sort());
      }
      assert.deepEqual(actual.map(classes => classes.sort()), expected, path);
    }
    editor.destroy();
  }
});

test('editor leaves text past the highlight limit plain', async () => {
  const { editor, view } = mount({ path: 'a.json', text: '{"key": true}\n'.repeat(40_000) });
  const start = view.dispatches;
  // Both lazy loads, the highlighter and the language, have reported back.
  await settle(() => view.dispatches >= start + 2);
  const marks = view.state.facet(EditorView.decorations).filter(x => typeof x !== 'function').reduce((n, set) => n + set.size, 0);
  assert.equal(marks, 0);
  editor.destroy();
});

test('renaming the path swaps the language; the latest request wins over slower earlier ones', async () => {
  const { editor, view, options } = mount({ path: 'a.ts' });
  assert.equal(await languageOf(view, 'a.ts'), typescriptLanguage);
  // Fire two path changes back to back; the earlier one must never land last.
  editor.update({ ...options, path: 'a.json' });
  editor.update({ ...options, path: 'a.md' });
  await settle(() => view.state.facet(language)?.name === 'markdown');
  for (let i = 0; i < 20; i++) await new Promise((resolve) => setImmediate(resolve));
  assert.equal(view.state.facet(language)?.name, 'markdown');
  assert.equal(view.state.doc.toString(), options.text, 'a rename alone keeps the document');
});

test('a language that finishes loading after destroy is never dispatched', async () => {
  const { editor, view } = mount({ path: 'late.yaml' });
  editor.destroy();
  for (let i = 0; i < 50; i++) await new Promise((resolve) => setImmediate(resolve));
  assert.equal(view.lateDispatches, 0);
  assert.equal(view.destroyed, true);
});

test('remount of the same tab/version/text restores state, undo history and scroll', () => {
  const id = {};
  const first = mount({ id, text: 'hello', version: 3 });
  type(first.view, ' world');
  first.view.scrollDOM.scrollTop = 120;
  first.view.scrollDOM.scrollLeft = 7;
  first.editor.destroy();

  const second = mount({ id, text: 'hello world', version: 3 });
  assert.equal(second.view.state.doc.toString(), 'hello world');
  assert.deepEqual(
    { scrollTop: second.view.scrollDOM.scrollTop, scrollLeft: second.view.scrollDOM.scrollLeft },
    { scrollTop: 120, scrollLeft: 7 },
  );
  // Layout can clobber the immediate assignment; the measure pass re-applies it.
  second.view.scrollDOM.scrollTop = 0;
  second.view.flushMeasures();
  assert.equal(second.view.scrollDOM.scrollTop, 120);
  assert.equal(undo(second.view), true, 'history crossed the unmount');
  assert.equal(second.view.state.doc.toString(), 'hello');
  assert.deepEqual(second.calls.changes, ['hello'], 'the remounted editor reports to its own callbacks');
  assert.deepEqual(first.calls.changes, ['hello world'], 'the unmounted owner is not called again');
});

test('remount after a version bump starts fresh', () => {
  const id = {};
  const first = mount({ id, text: 'draft', version: 1 });
  type(first.view, '!');
  first.view.scrollDOM.scrollTop = 50;
  first.editor.destroy();

  const second = mount({ id, text: 'draft!', version: 2 });
  assert.equal(second.view.state.doc.toString(), 'draft!');
  assert.equal(second.view.scrollDOM.scrollTop, 0);
  assert.equal(undo(second.view), false, 'stale history must not leak');
});

test('a draft changed by the store while unmounted is one undoable change on remount', () => {
  const id = {};
  const first = mount({ id, text: 'one foo\ntwo foo', version: 1 });
  type(first.view, '!');
  first.view.scrollDOM.scrollTop = 50;
  first.editor.destroy();

  // A replace in files changed the draft; the version (history) is the same.
  const second = mount({ id, text: 'one bar\ntwo bar!', version: 1 });
  assert.equal(second.view.state.doc.toString(), 'one bar\ntwo bar!');
  assert.equal(second.view.scrollDOM.scrollTop, 50, 'the tab keeps its place');
  assert.deepEqual(second.calls.changes, [], 'the store already holds the draft');
  assert.equal(undo(second.view), true);
  assert.equal(second.view.state.doc.toString(), 'one foo\ntwo foo!', 'undo takes back exactly the replace');
  assert.equal(undo(second.view), true);
  assert.equal(second.view.state.doc.toString(), 'one foo\ntwo foo', 'typing before it is still in the history');
  assert.deepEqual(second.calls.changes, ['one foo\ntwo foo!', 'one foo\ntwo foo']);
});

test('a draft changed by the store while shown keeps the cursor and is one undoable change', () => {
  const { editor, view, options, calls } = mount({ text: 'foo\nkeep\nfoo', version: 1 });
  view.dispatch({ selection: { anchor: 6 } });
  type(view, 'X', 5);
  editor.update({ ...options, text: 'bar\nkXeep\nbar' });
  assert.equal(view.state.doc.toString(), 'bar\nkXeep\nbar');
  assert.equal(view.state.selection.main.head, 7, 'a change on other lines does not move the cursor');
  assert.deepEqual(calls.changes, ['foo\nkXeep\nfoo']);
  // A CRLF echo of the same text is not a change.
  editor.update({ ...options, text: 'bar\r\nkXeep\r\nbar' });
  assert.equal(view.state.doc.toString(), 'bar\nkXeep\nbar');
  assert.equal(undo(view), true);
  assert.equal(view.state.doc.toString(), 'foo\nkXeep\nfoo');
  assert.equal(undo(view), true);
  assert.equal(view.state.doc.toString(), 'foo\nkeep\nfoo');
});

test('update with a new version or external text resets document and history', () => {
  const { editor, view, options, calls } = mount({ text: 'v1', version: 1 });
  type(view, '+');
  editor.update({ ...options, text: 'v2 from disk', version: 2 });
  assert.equal(view.state.doc.toString(), 'v2 from disk');
  assert.equal(undo(view), false);
  assert.deepEqual(calls.changes, ['v1+'], 'a reset is not reported as a user edit');

  view.scrollDOM.scrollTop = 99;
  editor.update({ ...options, text: 'v2 from disk', version: 3 });
  view.flushMeasures();
  assert.equal(view.scrollDOM.scrollTop, 0, 'a reset scrolls back to the top');
});

test('switching tabs in place keeps each tab\'s own state and routes callbacks to the active tab', () => {
  const a = {};
  const b = {};
  const { editor, view, options, calls } = mount({ id: a, text: 'A', version: 1 });
  type(view, 'a');
  view.scrollDOM.scrollTop = 40;

  const bCalls = [];
  const bOptions = { ...options, id: b, text: 'B', version: 1, path: 'b.md', onchange: (t) => bCalls.push(t) };
  editor.update(bOptions);
  assert.equal(view.state.doc.toString(), 'B');
  view.flushMeasures();
  assert.equal(view.scrollDOM.scrollTop, 0);
  type(view, 'b');
  assert.deepEqual(bCalls, ['Bb']);
  assert.deepEqual(calls.changes, ['Aa']);

  editor.update({ ...options, text: 'Aa' });
  assert.equal(view.state.doc.toString(), 'Aa');
  view.flushMeasures();
  assert.equal(view.scrollDOM.scrollTop, 40);
  assert.equal(undo(view), true);
  assert.equal(view.state.doc.toString(), 'A');
  assert.deepEqual(calls.changes, ['Aa', 'A']);

  editor.update({ ...bOptions, text: 'Bb' });
  assert.equal(view.state.doc.toString(), 'Bb', 'tab B was remembered when switching away');
  assert.equal(undo(view), true);
  assert.equal(view.state.doc.toString(), 'B');
});

test('keyboard: Mod-S saves, Mod-F opens search, Escape closes only an open search', () => {
  const { editor, view, calls } = mount();

  const save = key('s', { mod: true });
  assert.equal(editor.handleKey(save), true);
  assert.equal(calls.saves, 1);
  assert.equal(save.prevented, 1);
  assert.equal(save.stopped, 1);

  assert.equal(editor.handleKey(key('S', { mod: true })), true, 'Caps Lock / uppercase key still saves');
  assert.equal(calls.saves, 2);

  const escapeClosed = key('Escape');
  assert.equal(editor.handleKey(escapeClosed), false, 'Escape without a search panel bubbles to the dialog');
  assert.equal(escapeClosed.prevented, 0);

  assert.equal(editor.handleKey(key('f', { mod: true })), true);
  assert.equal(searchPanelOpen(view.state), true);

  const escapeOpen = key('Escape');
  assert.equal(editor.handleKey(escapeOpen), true);
  assert.equal(escapeOpen.stopped, 1);
  assert.equal(searchPanelOpen(view.state), false);

  for (const ev of [key('s', { mod: true, shift: true }), key('s', { mod: true, alt: true }), key('s'), key('f', { alt: true })]) {
    assert.equal(editor.handleKey(ev), false);
    assert.equal(ev.prevented, 0);
  }
  assert.equal(calls.saves, 2);

  const consumed = key('s', { mod: true, prevented: true });
  assert.equal(editor.handleKey(consumed), true, 'an event already consumed by the editor is reported as handled');
  assert.equal(calls.saves, 2, '...without saving twice');
});

test('find/closeSearch commands drive the real search panel state', () => {
  const { editor, view } = mount();
  assert.equal(editor.closeSearch(), false);
  assert.equal(editor.find(), true);
  assert.equal(searchPanelOpen(view.state), true);
  assert.equal(editor.closeSearch(), true);
  assert.equal(searchPanelOpen(view.state), false);
});

test('the capturing keydown listener saves and is removed on destroy', () => {
  const { editor, view, calls } = mount();
  const event = Object.assign(new Event('keydown', { cancelable: true }), {
    key: 's', metaKey: isMac, ctrlKey: !isMac, altKey: false, shiftKey: false,
  });
  view.dom.dispatchEvent(event);
  assert.equal(calls.saves, 1);
  assert.equal(event.defaultPrevented, true);

  editor.destroy();
  const after = Object.assign(new Event('keydown', { cancelable: true }), {
    key: 's', metaKey: isMac, ctrlKey: !isMac, altKey: false, shiftKey: false,
  });
  view.dom.dispatchEvent(after);
  assert.equal(calls.saves, 1);
  assert.equal(after.defaultPrevented, false);
});

test('after destroy every controller method is an inert no-op and destroy is idempotent', () => {
  const { editor, view, options, calls } = mount();
  editor.focus();
  assert.equal(view.focused, 1);
  editor.destroy();
  editor.destroy();
  const dispatches = view.dispatches;
  editor.focus();
  editor.update({ ...options, text: 'ignored', version: 9 });
  assert.equal(editor.find(), false);
  assert.equal(editor.closeSearch(), false);
  assert.equal(editor.handleKey(key('s', { mod: true })), false);
  assert.equal(view.focused, 1);
  assert.equal(view.dispatches, dispatches);
  assert.equal(view.state.doc.toString(), options.text);
  assert.equal(calls.saves, 0);
});

// ---------------------------------------------------------------------------
// Component: SSR render and the lazy-loading boundary.
// ---------------------------------------------------------------------------

const component = await loadServerComponent(COMPONENT_URL, [
  ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
  ["import { filePrompt } from '../../../stores/filePrompt.svelte';", 'const filePrompt = { request: null };'],
  ["import { preferences } from '../../../stores/preferences.svelte';", "const preferences = { editorFont: '', editorFontSize: null, suggestWhileTyping: true };"],
]);

test('FileEditor server-renders an empty host and inert command exports before the runtime loads', async () => {
  const { render } = await import('svelte/server');
  const bound = {};
  const props = {
    id: {}, text: 'secret draft', path: 'a.ts', version: 1, wrap: false,
    onchange() { assert.fail('no edits during SSR'); },
    onsave() { assert.fail('no saves during SSR'); },
  };
  for (const name of ['focus', 'find', 'closeSearch', 'handleKey']) {
    Object.defineProperty(props, name, { get: () => undefined, set: (fn) => { bound[name] = fn; }, enumerable: true });
  }
  const html = render(component.default, { props }).body;
  assert.match(html, /<div class="file-editor[^"]*"[^>]*><\/div>/);
  assert.match(html, /autocorrect="off"/);
  assert.match(html, /autocapitalize="off"/);
  assert.match(html, /spellcheck="false"/);
  assert.doesNotMatch(html, /role="alert"/);
  assert.doesNotMatch(html, /secret draft/, 'the document is rendered by CodeMirror, not by SSR markup');
  assert.deepEqual(Object.keys(bound).sort(), ['closeSearch', 'find', 'focus', 'handleKey']);
  assert.equal(bound.find(), false);
  assert.equal(bound.closeSearch(), false);
  assert.equal(bound.handleKey(key('s', { mod: true })), false);
  assert.equal(bound.focus(), undefined);
});

function staticImports(code) {
  // import x from '…' / import { … } from '…' / import '…' / export … from '…'
  const re = /^\s*(?:import|export)\s+(type\s+)?(?:[^'";]*?\s+from\s+)?['"]([^'"]+)['"]/gm;
  return [...code.matchAll(re)].map(([, type, spec]) => ({ type: Boolean(type), spec }));
}

test('FileEditor component reaches fileEditor/CodeMirror only through a dynamic import', () => {
  const source = readFileSync(COMPONENT_URL, 'utf8');
  for (const { type, spec } of staticImports(source)) {
    if (/fileEditor|@codemirror/.test(spec)) assert.ok(type, `static value import of ${spec}`);
  }
  const client = compile(source, { generate: 'client', filename: fileURLToPath(COMPONENT_URL) }).js.code;
  const specs = staticImports(client).map((i) => i.spec);
  assert.ok(specs.length > 0, 'import scan found nothing; regex out of date?');
  for (const spec of specs) assert.doesNotMatch(spec, /fileEditor|@codemirror/);
  assert.match(client, /import\(\s*['"]\.\.\/\.\.\/\.\.\/fileEditor['"]\s*\)/);
  assert.match(source, /diagnostics, position/);
  assert.match(source, /createFileEditor\(host, options\(\)\)/, 'lazy resolution reads latest props, not a captured snapshot');
  assert.match(source, /if \(!disposed\)/);
  const viewer = readFileSync(new URL('lib/components/common/go-to-file/FileViewer.svelte', SRC), 'utf8');
  assert.match(viewer, /diagnostics=\{\{ tab: activeTab, path: activeTab.path/);
  assert.match(viewer, /!codeDiagnostics.revoking/);
  assert.match(viewer, /edit.dirty \|\| edit.uncertain \|\| edit.conflict/);
});

test('no other source file statically pulls CodeMirror or fileEditor into the main bundle', () => {
  const root = fileURLToPath(SRC);
  const offenders = [];
  for (const rel of readdirSync(root, { recursive: true })) {
    if (!/\.(ts|js|svelte)$/.test(rel) || rel.replace(/\\/g, '/') === 'lib/fileEditor.ts') continue;
    const code = readFileSync(new URL(rel.replace(/\\/g, '/'), SRC), 'utf8');
    for (const { type, spec } of staticImports(code)) {
      if (!type && (/^@codemirror\//.test(spec) || /(^|\/)fileEditor(\.ts)?$/.test(spec))) offenders.push(`${rel}: ${spec}`);
    }
  }
  assert.deepEqual(offenders, []);
});

test('diagnostic range uses exact UTF16 columns without doc/history/blame changes', () => {
  const { editor, view, calls } = mount({ text: 'a😀z\nβxy', path: 'a.txt' });
  view.dispatch({ changes: { from: 8, insert: '!' } });
  const depth = undoDepth(view.state), text = view.state.doc.toString();
  assert.equal(editor.revealRange({ start: { line: 0, character: 1 }, end: { line: 1, character: 2 } }), true);
  assert.equal(view.state.selection.main.anchor, 1);
  assert.equal(view.state.selection.main.head, 7);
  assert.equal(view.state.doc.toString(), text);
  assert.equal(undoDepth(view.state), depth);
  assert.equal(calls.changes.length, 1);
  assert.equal(editor.revealRange({ start: { line: 0, character: 2 }, end: { line: 0, character: 3 } }), false);
  editor.destroy();
});


// NAV-05: the real CodeMirror keymap, pointer and hover sources. The only
// stand-in is the view's DOM geometry; positions/state are CodeMirror's.
test('navigation keymap has F12/Shift-F12 and asks about the live caret in UTF16 units', () => {
  const asked = [];
  const { editor, view } = mount({ text: '😀 name();\nβeta()', symbols: { ask: (...args) => asked.push(args), hover: async () => undefined } });
  view.dispatch({ selection: { anchor: 6 } });
  const bindings = view.state.facet(keymap).flat();
  const definition = bindings.find(b => b.key === 'F12');
  const references = bindings.find(b => b.key === 'Shift-F12');
  assert.ok(definition && references);
  assert.equal(navigationKeymap.length, 2);
  assert.equal(definition.preventDefault, true);
  assert.equal(references.preventDefault, true);
  assert.equal(definition.run(view), true);
  assert.equal(references.run(view), true);
  assert.deepEqual(asked.map(a => a.slice(0, 3)), [
    ['definition', { line: 0, character: 3 }, { x: 10, y: 20 }],
    ['references', { line: 0, character: 3 }, { x: 10, y: 20 }],
  ]);
  assert.equal(asked[0][3](), true);
  view.dispatch({ changes: { from: 0, insert: 'x' } });
  assert.equal(asked[0][3](), false, 'positions from before typing are no longer current');
  assert.deepEqual(editor.caret(), { line: 0, character: 7 });
  editor.destroy();
  assert.equal(editor.caret(), undefined);
  assert.equal(asked[1][3](), false);
});

test('modifier-click uses posAtCoords, checks the word side and does not place the caret', () => {
  const asked = [];
  const { editor, view } = mount({ text: '😀 name 2', symbols: { ask: (...args) => asked.push(args), hover: async () => undefined } });
  const mouse = (overrides = {}) => ({ button: 0, metaKey: isMac, ctrlKey: !isMac, altKey: false, clientX: 11, clientY: 10,
    prevented: false, preventDefault() { this.prevented = true; }, ...overrides });
  view.pointerPos = 5;
  const event = mouse();
  assert.equal(symbolMouseDown(event, view), true);
  assert.equal(event.prevented, true);
  assert.deepEqual(asked[0].slice(0, 3), ['definition', { line: 0, character: 3 }, { x: 11, y: 10 }]);
  assert.equal(view.state.selection.main.head, 0, 'modifier-click does not move the caret');
  for (const e of [mouse({ button: 2 }), mouse({ metaKey: false, ctrlKey: false }), mouse({ altKey: true }), mouse({ clientX: 100 })]) {
    assert.equal(symbolMouseDown(e, view), false);
    assert.equal(e.prevented, false);
  }
  view.pointerPos = 7;
  assert.equal(symbolMouseDown(mouse(), view), false, 'just after the word is whitespace');
  assert.equal(symbolMouseDown(mouse({ clientX: 9 }), view), true, 'just before the boundary is the word');
  view.pointerPos = 8;
  assert.equal(symbolMouseDown(mouse(), view), false, 'numeric literal');
  assert.deepEqual(wordAtSide(view.state, 3, 1), { from: 3, to: 7 });
  assert.equal(wordAtSide(view.state, 3, -1), undefined);
  assert.deepEqual(editorPosition(view.state, 5), { line: 0, character: 5 });
  editor.destroy();
});

test('edit-mode context menu offers symbols at UTF16 positions and leaves native menus elsewhere', () => {
  const menus = [];
  const { editor, view, options } = mount({ text: '😀 name 2', symbols: { ask() {}, hover: async () => undefined, menu: (...args) => menus.push(args) } });
  const event = (overrides = {}) => ({ button: 2, clientX: 11, clientY: 10, prevented: false, preventDefault() { this.prevented = true; }, ...overrides });
  view.pointerPos = 5;
  const opened = event();
  assert.equal(symbolContextMenu(opened, view), true);
  assert.equal(opened.prevented, true);
  assert.deepEqual(menus[0].slice(0, 3), [{ line: 0, character: 3 }, 'name', { x: 11, y: 10 }]);
  assert.equal(menus[0][3](), true);
  for (const [pos, x] of [[7, 11], [8, 11], [5, 100]]) {
    view.pointerPos = pos;
    const native = event({ clientX: x });
    assert.equal(symbolContextMenu(native, view), false);
    assert.equal(native.prevented, false);
  }
  view.dispatch({ changes: { from: 0, insert: 'x' } });
  assert.equal(menus[0][3](), false, 'an edit while the menu is open makes it stale');
  editor.update({ ...options, symbols: { ask() {}, hover: async () => undefined } });
  view.pointerPos = 6;
  assert.equal(symbolContextMenu(event(), view), false, 'without a menu callback the native menu stays');
  editor.destroy();
});

test('a context menu on selected text copies only, keeps the selection and its focused colour', () => {
  const menus = [], selections = [];
  const { editor, view } = mount({ text: 'one name two', symbols: { ask() {}, hover: async () => undefined, menu: (...args) => menus.push(args), selectionMenu: (...args) => selections.push(args) } });
  const event = (overrides = {}) => ({ button: 2, clientX: 11, clientY: 10, prevented: false, preventDefault() { this.prevented = true; }, ...overrides });
  view.dispatch({ selection: { anchor: 0, head: 8 } });
  view.pointerPos = 5;
  const opened = event();
  symbolPress(opened, view);
  // macOS selects the word under a right-click before the menu opens.
  view.dispatch({ selection: { anchor: 4, head: 8 } });
  assert.equal(symbolContextMenu(opened, view), true);
  assert.equal(opened.prevented, true);
  assert.deepEqual(menus, [], 'no symbol menu on a selection');
  assert.deepEqual(selections[0].slice(0, 2), ['one name', { x: 11, y: 10 }]);
  assert.deepEqual([view.state.selection.main.anchor, view.state.selection.main.head], [0, 8], 'the selection is put back');
  const release = selections[0][2]();
  assert.equal(view.dom.classes.has('cm-selection-held'), true);
  release();
  assert.equal(view.dom.classes.has('cm-selection-held'), false);

  // Outside the selection, or with a word the press selected, the symbol menu.
  view.pointerPos = 10;
  assert.equal(symbolContextMenu(event(), view), true);
  assert.equal(menus.length, 1);
  view.dispatch({ selection: { anchor: 0 } });
  view.pointerPos = 5;
  const word = event();
  symbolPress(word, view);
  view.dispatch({ selection: { anchor: 4, head: 8 } });
  assert.equal(symbolContextMenu(word, view), true);
  assert.equal(menus.length, 2);
  assert.equal(selections.length, 1);
  assert.deepEqual([view.state.selection.main.anchor, view.state.selection.main.head], [0, 0], 'the word the press selected is dropped');
  assert.deepEqual(view.dom.domSelections.at(-1), ['text', 0, 'text', 0], 'in the DOM too, before the editor reads it');
  // So a second right-click on that word is still the symbol menu, even when
  // the editor has not read the browser's selection by the time the menu opens.
  const again = event();
  symbolPress(again, view);
  assert.equal(symbolContextMenu(again, view), true);
  assert.equal(menus.length, 3);
  assert.equal(selections.length, 1);
  // The selection's edges count only from the inside.
  view.dispatch({ selection: { anchor: 4, head: 8 } });
  view.pointerPos = 4;
  assert.equal(symbolContextMenu(event({ clientX: 9 }), view), false, 'just before the selection is whitespace');
  assert.equal(selections.length, 1);
  assert.equal(symbolContextMenu(event({ clientX: 11 }), view), true);
  assert.equal(selections.length, 2);
  editor.destroy();
});

test('a right-click that selects nothing, as on Windows and Linux, leaves the selection untouched; a press without its menu is forgotten', () => {
  const menus = [], selections = [];
  const { editor, view } = mount({ text: 'one name two', symbols: { ask() {}, hover: async () => undefined, menu: (...args) => menus.push(args), selectionMenu: (...args) => selections.push(args) } });
  const event = (overrides = {}) => ({ button: 2, clientX: 11, clientY: 10, preventDefault() {}, ...overrides });
  view.dispatch({ selection: { anchor: 2 } });
  view.dom.domSelection.setBaseAndExtent('text', 2, 'text', 2);
  const writes = view.dom.domSelections.length, dispatches = view.dispatches;
  view.pointerPos = 5;
  const press = event();
  symbolPress(press, view);
  assert.equal(symbolContextMenu(press, view), true);
  assert.equal(menus.length, 1);
  assert.equal(view.dom.domSelections.length, writes, 'the DOM selection is not rewritten');
  assert.equal(view.dispatches, dispatches, 'nor the editor selection');

  // Pressed with a selection whose menu never came, say released outside the
  // editor on Windows: a menu from the keyboard takes the selection as it is.
  view.dispatch({ selection: { anchor: 4, head: 8 } });
  symbolPress(event(), view);
  view.dispatch({ selection: { anchor: 0 } });
  symbolKeyDown({ key: 'Shift' }, view);
  symbolKeyDown({ key: 'F10' }, view);
  assert.equal(symbolContextMenu(event(), view), true);
  assert.equal(selections.length, 0, 'the old selection is not taken for the current one');
  assert.equal(view.state.selection.main.anchor, 0, 'nor put back');
  editor.destroy();
});

test('navigation callbacks are current owners on cached states; old asks and hovers cannot cross tabs or text', async () => {
  const first = [], second = [];
  let resolve;
  const oldSymbols = { ask: (...args) => first.push(args), hover: () => new Promise(r => { resolve = r; }) };
  const a = mount({ text: 'name()', symbols: oldSymbols });
  const oldHover = symbolHover(a.view, 2, 1);
  navigationKeymap[0].run(a.view);
  assert.equal(first.length, 1);
  const current = first[0][3];
  a.editor.destroy();
  assert.equal(current(), false);
  const b = mount({ ...a.options, symbols: { ask: (...args) => second.push(args), hover: async () => ({ html: '<pre>new</pre>', truncated: false }) } });
  navigationKeymap[1].run(b.view);
  assert.equal(first.length, 1, 'cached extensions have no stale callback closure');
  assert.equal(second.length, 1);
  const hover = await symbolHover(b.view, 3, 1);
  assert.equal(hover.pos, 0);
  assert.equal(hover.end, 4);
  assert.equal(hover.above, false);
  resolve({ html: '<pre>old</pre>', truncated: false });
  assert.equal(await oldHover, null, 'destroyed editor cannot show a tooltip');
  assert.equal(await symbolHover(b.view, 4, 1), null, 'no hover over punctuation');
  const pending = symbolHover(b.view, 2, 1);
  b.view.dispatch({ changes: { from: 0, insert: 'x' } });
  assert.equal(await pending, null, 'an asynchronous answer for other text is dropped');
  b.editor.update({ ...b.options, id: {}, path: 'other.ts' });
  assert.equal(second[0][3](), false, 'same text in another tab is not the asked document');
  b.editor.destroy();
});

test('symbol hover creates a bounded scrollable CodeMirror tooltip with the shared safe HTML and notice', async () => {
  const h = mount({ text: 'name', symbols: { ask() {}, hover: async () => ({ html: '<pre class="hover-plain">&lt;safe&gt;</pre>', truncated: true }) } });
  const tooltip = await symbolHover(h.view, 1, 1);
  const previous = globalThis.document;
  try {
    globalThis.document = { createElement: () => ({ children: [], style: {}, appendChild(child) { this.children.push(child); return child; }, addEventListener() {} }) };
    const created = tooltip.create(h.view);
    created.mount({ scrollDOM: { clientHeight: 500 } });
    assert.equal(created.dom.className, 'cm-symbol-hover');
    assert.equal(created.dom.style.maxHeight, '300px');
    assert.match(created.dom.children[0].innerHTML, /&lt;safe&gt;/);
    assert.match(created.dom.children[1].textContent, /cut short/);
  } finally { globalThis.document = previous; h.editor.destroy(); }
});

// ---------------------------------------------------------------------------
// Code completion without a language server (CC-01).
// ---------------------------------------------------------------------------

const typed = (view, text, at = view.state.selection.main.head) =>
  view.dispatch({ changes: { from: at, insert: text }, selection: { anchor: at + text.length }, userEvent: 'input.type' });

function wordsAt(state, pos = state.doc.length, explicit = false) {
  return documentWords(new CompletionContext(state, pos, explicit));
}

test('completion keys: Cmd+I on macOS and Ctrl+Space everywhere open suggestions; Option+` and Option+I stay accents', async () => {
  const opens = completionKeys.filter(b => b.run === startCompletion);
  assert.deepEqual(opens.map(b => [b.key, b.mac]), [[undefined, 'Cmd-i'], ['Ctrl-Space', undefined]]);
  // The keys Settings names (settings-client.test.mjs ties the label to these).
  const keys = await import('../src/lib/editorKeys.ts');
  assert.deepEqual(opens.map(b => b.key ?? b.mac), [keys.SUGGEST_KEY_MAC, keys.SUGGEST_KEY]);
  const names = completionKeys.flatMap(b => [b.key, b.mac, b.win, b.linux]).filter(Boolean);
  for (const dead of ['Alt-`', 'Alt-i']) assert.ok(!names.includes(dead), `${dead} must not be bound`);
  for (const name of ['Escape', 'ArrowDown', 'ArrowUp', 'PageDown', 'PageUp', 'Enter', 'Tab']) assert.ok(names.includes(name), name);

  const { view, editor } = mount();
  const groups = view.state.facet(keymap);
  const all = groups.flat();
  for (const dead of ['Alt-`', 'Alt-i']) assert.ok(!all.some(b => b.key === dead || b.mac === dead), `no keymap binds ${dead}`);
  // Accepting with Tab or Enter runs before indenting or a new line; without a popup they fall through.
  const accept = completionKeys.find(b => b.key === 'Tab');
  const completion = groups.findIndex(g => g.includes(completionKeys[0]));
  const editing = groups.findIndex(g => g.some(b => b.key === 'Tab' && b.run !== accept.run));
  assert.ok(completion >= 0 && editing > completion, 'completion keys come first');
  assert.equal(accept.run(view), false, 'no popup: Tab is left to indentation');
  const enter = completionKeys.find(b => b.key === 'Enter');
  assert.equal(enter.run, acceptOrBreakLine, 'Enter breaks the line after a name typed in full (checked in Chrome)');
  assert.equal(enter.run(view), false, 'no popup: Enter is left to the new line');
  editor.destroy();
});

test('document words: a language source that answers later and fails leaves no unhandled rejection', async () => {
  let rejected = 0;
  const failing = () => { rejected++; return Promise.reject(new Error('gone')); };
  const state = EditorState.create({ doc: 'alpha alphabet\nalp', extensions: EditorState.languageData.of(() => [{ autocomplete: failing }]) });
  const unhandled = [];
  const record = reason => unhandled.push(reason);
  process.on('unhandledRejection', record);
  try {
    assert.deepEqual(wordsAt(state).options.map(o => o.label).sort(), ['alpha', 'alphabet']);
    await new Promise(resolve => setImmediate(resolve));
  } finally {
    process.off('unhandledRejection', record);
  }
  assert.equal(rejected, 1);
  assert.deepEqual(unhandled, []);
});

test('document words: a language source that answers later is run for its labels once, not on every query', () => {
  let runs = 0;
  const later = () => { runs++; return new Promise(() => {}); };
  let syncRuns = 0;
  const now = () => { syncRuns++; return null; };
  const state = EditorState.create({ doc: 'alpha alphabet\nalp', extensions: EditorState.languageData.of(() => [{ autocomplete: later }, { autocomplete: now }]) });
  for (let i = 0; i < 3; i++) assert.deepEqual(wordsAt(state).options.map(o => o.label).sort(), ['alpha', 'alphabet']);
  assert.equal(syncRuns, 1, 'the same text and position: its answer is reused');
  const next = state.update({ changes: { from: state.doc.length, insert: 'h' } }).state;
  for (let i = 0; i < 3; i++) assert.deepEqual(wordsAt(next).options.map(o => o.label).sort(), ['alpha', 'alphabet']);
  assert.equal(runs, 1, 'seen to answer later: not run again');
  assert.equal(syncRuns, 2, 'a source that answers at once still is, once per text');
});

test('each language source runs once per query, and its names still stay out of the words', async () => {
  let runs = 0;
  const counting = (context) => { runs++; return { from: context.pos - 3, options: [{ label: 'alphabet', type: 'keyword' }] }; };
  const state = EditorState.create({ doc: 'alpha alphabet beta\nalp', extensions: EditorState.languageData.of(() => [{ autocomplete: counting }]) });
  // As CodeMirror runs a query (the editor's override): each source with its own context, in order.
  const query = (s, explicit) => Promise.all(completionSources.map(source => source(new CompletionContext(s, s.doc.length, explicit))));
  const results = (await query(state, true)).filter(Boolean);
  assert.equal(runs, 1, 'once, shared by its slot and the words');
  const labels = results.flatMap(r => r.options.map(o => `${o.label}:${o.type}`));
  assert.deepEqual(labels.filter(l => l.startsWith('alphabet')), ['alphabet:keyword'], 'the language item only');
  assert.ok(labels.includes('alpha:text'), 'the other words');
  // The next keystroke is a new query.
  await query(state.update({ changes: { from: state.doc.length, insert: 'h' } }).state, false);
  assert.equal(runs, 2);
  // A slot past the language's sources answers nothing; no language has as many.
  assert.equal(completionSources.length, LANGUAGE_SLOTS + 1);
});

test('document words: prefix matches, the whole typed word and language items left out, none past the highlight limit', () => {
  const state = EditorState.create({ doc: 'alpha alphabet beta\nalp' });
  const result = wordsAt(state);
  assert.equal(result.from, state.doc.length - 3);
  assert.deepEqual(result.options.map(o => o.label).sort(), ['alpha', 'alphabet', 'beta']);
  assert.ok(result.options.every(o => o.type === 'text'));
  assert.equal(result.validFor('alph'), true, 'typing on filters locally');
  assert.equal(result.validFor('alpha'), false, 'a whole word asks again');
  assert.equal(result.validFor('al-'), false);

  // A name the language offers (a keyword, a property) shows once, as the language's item.
  const language = EditorState.create({ doc: 'alpha alphabet beta\nalp', extensions: EditorState.languageData.of(() => [
    { autocomplete: ['alphabet'] },
    { autocomplete: () => ({ from: 20, options: [{ label: 'beta', type: 'keyword' }] }) },
    { autocomplete: async () => ({ from: 20, options: [{ label: 'alpha', type: 'variable' }] }) },
    { autocomplete: () => { throw new Error('broken source'); } },
  ]) });
  assert.deepEqual(wordsAt(language).options.map(o => o.label), ['alpha'], 'only answers given at once are left out');

  const whole = EditorState.create({ doc: 'alpha alphabet\nalpha' });
  assert.deepEqual(wordsAt(whole).options.map(o => o.label), ['alphabet'], 'Enter after a finished word still breaks the line');

  assert.equal(wordsAt(EditorState.create({ doc: 'alpha ' })), null, 'nothing after a space unless asked');
  assert.ok(wordsAt(EditorState.create({ doc: 'alpha ' }), 6, true).options.some(o => o.label === 'alpha'), 'the shortcut lists words anywhere');

  const large = EditorState.create({ doc: 'word '.repeat(100_001) + 'wo' });
  assert.ok(large.doc.length > 500_000);
  assert.equal(wordsAt(large, large.doc.length, true), null);
});

test('typing opens suggestions when the setting is on; off, only the shortcut does; read-only never', () => {
  const { view, editor, options } = mount({ text: 'alpha\n', path: 'notes.txt' });
  typed(view, 'al');
  assert.equal(completionStatus(view.state), 'pending');
  // Options are applied on every keystroke; the same mode leaves an open popup alone.
  editor.update({ ...options, text: view.state.doc.toString() });
  assert.equal(completionStatus(view.state), 'pending');

  assert.equal(editor.handleKey(key('Escape')), true);
  editor.update({ ...options, text: view.state.doc.toString(), suggest: false });
  assert.equal(completionStatus(view.state), null);
  typed(view, 'p');
  assert.equal(completionStatus(view.state), null, 'typing opens nothing with the setting off');
  assert.equal(startCompletion(view), true);
  assert.equal(completionStatus(view.state), 'pending', 'the shortcut still does');

  editor.update({ ...options, text: view.state.doc.toString(), suggest: false, disabled: true });
  assert.equal(startCompletion(view), false, 'read-only: no completion at all');
  assert.equal(editor.handleKey(key('Escape')), false);

  editor.update({ ...options, text: view.state.doc.toString(), disabled: false });
  typed(view, 'h');
  assert.equal(completionStatus(view.state), 'pending', 'editable again, with the setting on');
  editor.destroy();
});

test('Escape closes suggestions first, then the search panel, then reaches the dialog', () => {
  const { view, editor } = mount({ text: 'alpha\nal' });
  assert.equal(editor.find(), true);
  startCompletion(view);
  assert.equal(completionStatus(view.state), 'pending');
  const first = key('Escape');
  assert.equal(editor.handleKey(first), true);
  assert.equal(first.prevented, 1);
  assert.equal(first.stopped, 1);
  assert.equal(completionStatus(view.state), null);
  assert.equal(searchPanelOpen(view.state), true, 'the search panel stays');
  assert.equal(editor.handleKey(key('Escape')), true);
  assert.equal(searchPanelOpen(view.state), false);
  const last = key('Escape');
  assert.equal(editor.handleKey(last), false, 'nothing open: Escape goes to the dialog');
  assert.equal(last.prevented, 0);
  editor.destroy();
});

test('a CSS file offers its properties beside the words, and a property is not repeated as a word', async () => {
  // lang-css reads the property names from the page's own style object.
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'document');
  globalThis.document = { body: { style: { color: '', columnGap: '', cssText: '' } } };
  try {
    const { view, editor } = mount({ text: 'a { color: red; } .collapsed {}\nb { col', path: 'site.css' });
    await languageOf(view, 'site.css');
    const end = view.state.doc.length;
    const css = view.state.languageDataAt('autocomplete', end);
    assert.ok(css.length > 0, 'the CSS package brings its own completions');
    const results = await Promise.all(css.map(s => s(new CompletionContext(view.state, end, false))));
    const labels = results.filter(Boolean).flatMap(r => r.options.map(o => o.label));
    assert.ok(labels.includes('color') && labels.includes('column-gap'), 'properties are offered');
    // CodeMirror filters by the typed prefix; the source gives every other word.
    assert.deepEqual(wordsAt(view.state, end).options.map(o => o.label), ['a', 'red', 'collapsed', 'b'], 'color comes once, as the property');
    editor.destroy();
  } finally {
    if (previous) Object.defineProperty(globalThis, 'document', previous);
    else delete globalThis.document;
  }
});

test('the suggestion popup uses the editor font and the app colours', () => {
  const { view, editor } = mount();
  const css = themeRules(view.state).join('\n');
  assert.match(css, /\.cm-tooltip\.cm-tooltip-autocomplete > ul \{[^}]*font-family: var\(--editor-font-family, var\(--font-mono\)\)/);
  assert.match(css, /\.cm-tooltip\.cm-tooltip-autocomplete > ul > li\[aria-selected\] \{[^}]*var\(--accent-color\)/);
  assert.match(css, /\.cm-tooltip\.cm-tooltip-autocomplete \{[^}]*overflow: visible/);
  assert.match(css, /\.cm-tooltip\.cm-completionInfo \{[^}]*var\(--bg-primary\)/);
  editor.destroy();
});

test('FileEditor passes the suggestion setting, so changing it reconfigures open editors', () => {
  const source = readFileSync(COMPONENT_URL, 'utf8');
  assert.match(source, /suggest: preferences\.suggestWhileTyping/);
  assert.match(source, /\$effect\(\(\) => \{\s*editor\?\.update\(options\(\)\);/);
});

// ---------------------------------------------------------------------------
// Language-server suggestions (CC-06).
// ---------------------------------------------------------------------------

function serverEditor(text, path = 'a.go', reply = async () => ({ items: [], incomplete: false })) {
  const asks = [];
  const h = { asks, reply };
  const symbols = { ask() {}, hover: async () => undefined, complete(position, trigger, current, signal) { asks.push({ position, trigger, current, signal }); return h.reply(position, trigger); } };
  Object.assign(h, mount({ text, path, symbols }));
  h.at = (pos = h.view.state.doc.length, explicit = false) => new CompletionContext(h.view.state, pos, explicit, h.view);
  h.suggest = (pos, explicit) => codeSuggestions(h.at(pos, explicit));
  return h;
}

test('server suggestions: why the server is asked, and trigger characters it names replace the likely ones', async () => {
  const h = serverEditor('fmt.\nx = 1 \nal\nfmt:');
  const { view } = h, line = n => view.state.doc.line(n);
  assert.deepEqual(completionTrigger(h.at(4), view, 'a.go'), { kind: 'character', character: '.' }, 'a likely trigger before the server said');
  assert.deepEqual(completionTrigger(h.at(line(3).to), view, 'a.go'), { kind: 'invoked' }, 'typing a name');
  assert.deepEqual(completionTrigger(h.at(line(2).to, true), view, 'a.go'), { kind: 'invoked' }, 'the shortcut, anywhere');
  assert.equal(completionTrigger(h.at(line(2).to), view, 'a.go'), undefined, 'after a space: words only');
  assert.equal(completionTrigger(h.at(0), view, 'a.go'), undefined);

  h.reply = async () => ({ items: [{ label: 'Println' }], incomplete: false, triggerCharacters: '.' });
  await h.suggest(4);
  assert.deepEqual(h.asks[0].position, { line: 0, character: 4 });
  assert.deepEqual(h.asks[0].trigger, { kind: 'character', character: '.' });
  assert.equal(completionTrigger(h.at(line(4).to), view, 'a.go'), undefined, "':' is not this server's");
  assert.deepEqual(completionTrigger(h.at(4), view, 'a.go'), { kind: 'character', character: '.' });
  assert.deepEqual(completionTrigger(h.at(line(4).to), view, 'b.rb'), { kind: 'character', character: ':' }, 'another file: likely ones again');
  assert.equal(h.asks[0].current(), true);
  typed(view, 'P', 4);
  assert.equal(h.asks[0].current(), false, 'the asked text changed');

  // An incomplete answer: typing on in the same word asks again as such; the shortcut starts over.
  h.reply = async () => ({ items: [{ label: 'alpha' }], incomplete: true });
  const end = view.state.doc.line(3).to;
  const result = await h.suggest(end);
  assert.equal(result.validFor, undefined, 'never filtered locally');
  typed(view, 'p', end);
  assert.deepEqual(completionTrigger(h.at(end + 1), view, 'a.go'), { kind: 'incomplete' });
  assert.deepEqual(completionTrigger(h.at(end + 1, true), view, 'a.go'), { kind: 'invoked' });
  assert.deepEqual(completionTrigger(h.at(end + 1), view, 'a.go'), { kind: 'invoked' });
  h.editor.destroy();
});

test('server suggestions keep the server order past 99 items, where CodeMirror breaks equal scores by sortText', async () => {
  // Labels against the server's order, so a tie broken by label would show.
  const items = Array.from({ length: 500 }, (_, i) => ({ label: `al${String(499 - i).padStart(3, '0')}`, sortText: String(i).padStart(3, '0') }));
  const h = serverEditor('al', 'a.go', async () => ({ incomplete: false, items }));
  const { options } = await h.suggest();
  // CodeMirror's sort for options that match alike (all start with what was typed):
  // the score (match plus boost), then its default compareCompletions.
  const sorted = [...options].sort((a, b) => (b.boost - a.boost) || (a.sortText || a.label).localeCompare(b.sortText || b.label));
  assert.deepEqual(sorted.map(o => o.label), items.map(i => i.label));
  h.editor.destroy();
});

test('server suggestions come above the words they do not repeat, in the server order, with kinds and details', async () => {
  const h = serverEditor('alpha alphabet\nal', 'a.ts', async () => ({ incomplete: false, items: [
    { label: 'alpha', kind: 6, sortText: 'b', detail: 'let alpha: number' },
    { label: 'alert', kind: 3, sortText: 'a', labelDetail: '(message?: any)', deprecated: true },
    { label: 'all', filterText: 'allx', kind: 14, sortText: 'c' },
  ] }));
  const result = await h.suggest();
  assert.equal(result.from, 15);
  assert.equal(result.validFor.test('alp'), true, 'a complete answer filters as the user types on');
  assert.equal(result.validFor.test('al.'), false);
  assert.deepEqual(result.options.map(o => o.label), ['alpha', 'alert', 'allx', 'alphabet'], 'alpha once, as the server item');
  const [alpha, alert, all, word] = result.options;
  assert.ok(alert.boost > alpha.boost && alpha.boost > all.boost && all.boost > 0 && !word.boost);
  assert.deepEqual([alpha.type, alert.type, all.type, word.type], ['variable', 'function', 'keyword', 'text']);
  assert.equal(alpha.detail, 'let alpha: number');
  assert.equal(alert.detail, '(message?: any)');
  assert.equal(all.displayLabel, 'all', 'matched on filterText, shown as the label');
  assert.equal(alpha.displayLabel, undefined);
  assert.equal(alpha.info, undefined, 'no details to show');
  h.editor.destroy();
});

test('words alone, at once, when no server can be asked; asked again next time when it fails or answers nothing', async () => {
  const h = serverEditor('alpha alphabet\nal', 'a.ts');
  h.reply = () => undefined;
  const sync = h.suggest();
  assert.ok(!(sync instanceof Promise), 'no server: no waiting');
  assert.deepEqual(sync.options.map(o => o.label), ['alpha', 'alphabet']);
  assert.equal(typeof sync.validFor, 'function');

  for (const reply of [async () => undefined, async () => { throw new Error('gone'); }]) {
    h.reply = reply;
    const words = await h.suggest();
    assert.deepEqual(words.options.map(o => o.label), ['alpha', 'alphabet']);
    assert.equal(words.validFor, undefined, 'the next keystroke asks the server again');
  }
  h.reply = async () => ({ items: [], incomplete: false });
  const empty = await h.suggest();
  assert.deepEqual(empty.options.map(o => o.label), ['alpha', 'alphabet']);
  assert.equal(empty.validFor, undefined, 'nothing yet (ruby-lsp indexing): the next keystroke asks again');
  assert.equal(await h.suggest(6), null, 'after a space with nothing to offer');

  // Words from another start than the server's edits are left out.
  h.reply = async () => ({ incomplete: false, items: [{ label: '@alpha', edit: { range: { start: { line: 1, character: 0 }, end: { line: 1, character: 2 } }, newText: '@alpha' } }] });
  assert.deepEqual((await h.suggest()).options.map(o => o.label), ['@alpha', 'alpha', 'alphabet'], 'same start: kept');
  h.editor.destroy();
  const ruby = serverEditor('@alpha alphabet\n@al', 'a.rb', async () => ({ incomplete: false, items: [{ label: '@alpha', edit: { range: { start: { line: 1, character: 0 }, end: { line: 1, character: 3 } }, newText: '@alpha' } }] }));
  const ivar = await ruby.suggest();
  assert.equal(ivar.from, 16, 'the edit starts at the sigil');
  assert.deepEqual(ivar.options.map(o => o.label), ['@alpha'], 'words start after it');
  ruby.editor.destroy();
});

test('a slow server is canceled and the words show; CodeMirror ending the query cancels it too', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const h = serverEditor('alpha alphabet\nal', 'a.ts', () => new Promise(() => {}));
  const pending = h.suggest();
  assert.equal(h.asks[0].signal.aborted, false);
  t.mock.timers.tick(SERVER_SUGGESTION_WAIT);
  const words = await pending;
  assert.equal(h.asks[0].signal.aborted, true);
  assert.deepEqual(words.options.map(o => o.label), ['alpha', 'alphabet']);

  const context = h.at();
  void codeSuggestions(context);
  for (const listener of context.abortListeners) listener();
  assert.equal(h.asks[1].signal.aborted, true);
  h.editor.destroy();
});

test('an answer to a query CodeMirror aborted changes neither the trigger characters nor the unfinished word', async () => {
  const h = serverEditor('fmt:\nal', 'a.go');
  let answer;
  h.reply = () => new Promise(resolve => { answer = resolve; });
  const { view } = h, end = view.state.doc.length;
  const aborted = h.at(end);
  const pending = codeSuggestions(aborted);
  // As CodeMirror aborts a query: its listeners, then no more.
  for (const listener of aborted.abortListeners) listener();
  aborted.abortListeners = null;
  answer({ items: [{ label: 'alpha' }], incomplete: true, triggerCharacters: '.' });
  assert.equal(await pending, null);
  typed(view, 'p', end);
  assert.deepEqual(completionTrigger(h.at(end + 1), view, 'a.go'), { kind: 'invoked' }, 'not asked again as incomplete');
  assert.deepEqual(completionTrigger(h.at(4), view, 'a.go'), { kind: 'character', character: ':' }, 'the likely triggers still apply');

  // Typed on in the same word while the server answered: asked again as incomplete.
  const typedOn = codeSuggestions(h.at(end + 1));
  typed(view, 'h', end + 1);
  answer({ items: [{ label: 'alpha' }], incomplete: true });
  await typedOn;
  assert.deepEqual(completionTrigger(h.at(end + 2), view, 'a.go'), { kind: 'incomplete' });

  // The word changed before the answer came: not.
  const changed = codeSuggestions(h.at(end + 2));
  view.dispatch({ changes: { from: end - 2, insert: 'x' } });
  answer({ items: [{ label: 'alpha' }], incomplete: true });
  await changed;
  assert.deepEqual(completionTrigger(h.at(end + 3), view, 'a.go'), { kind: 'invoked' });
  h.editor.destroy();
});

test('accepting applies the edit and its import as one undo step, with the snippet\'s first field selected', async () => {
  const text = 'package main\n\nfunc f() {\n\tfmt.Pr\n}\n';
  const h = serverEditor(text, 'a.go', async () => ({ incomplete: false, items: [{
    label: 'Println', kind: 3, snippet: true,
    edit: { range: { start: { line: 3, character: 5 }, end: { line: 3, character: 7 } }, newText: 'Println(${1:a ...any})$0' },
    extraEdits: [{ range: { start: { line: 1, character: 0 }, end: { line: 1, character: 0 } }, newText: 'import "fmt"\n' }],
  }] }));
  const { view } = h, end = view.state.doc.line(4).to;
  view.dispatch({ selection: { anchor: end } });
  const result = await h.suggest(end);
  typed(view, 'i', end);
  const depth = undoDepth(view.state);
  const option = result.options[0];
  option.apply(view, option, result.from, end + 1);
  assert.equal(view.state.doc.toString(), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)\n}\n');
  const args = view.state.doc.line(5).from + '\tfmt.Println('.length;
  assert.deepEqual([view.state.selection.main.from, view.state.selection.main.to], [args, args + 'a ...any'.length], 'the placeholder is selected');
  assert.equal(undoDepth(view.state), depth + 1);
  assert.equal(hasNextSnippetField(view.state), true);
  assert.equal(nextSnippetField(view), true, 'Tab moves on');
  assert.deepEqual([view.state.selection.main.from, view.state.selection.main.to], [args + 'a ...any)'.length, args + 'a ...any)'.length], 'and ends at $0');
  assert.equal(hasNextSnippetField(view.state), false, 'the snippet is done');
  undo(view);
  assert.equal(view.state.doc.toString(), 'package main\n\nfunc f() {\n\tfmt.Pri\n}\n', 'one undo, the typing stays');

  // An edit elsewhere since asking: only the item's text replaces the typed word, still with its fields.
  typed(view, 'X', 0);
  const word = view.state.doc.line(4);
  option.apply(view, option, word.to - 3, word.to);
  assert.equal(view.state.doc.toString(), 'Xpackage main\n\nfunc f() {\n\tfmt.Println(a ...any)\n}\n');
  assert.equal(view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to), 'a ...any');
  h.editor.destroy();
});

test('server suggestions: a server that names no trigger characters is asked after none', async () => {
  const h = serverEditor('fmt.\nfmt(');
  const { view } = h;
  h.reply = async () => ({ items: [{ label: 'Println' }], incomplete: false, triggerCharacters: '' });
  await h.suggest(4);
  assert.equal(h.asks.length, 1, 'asked after a likely trigger before the server said');
  assert.equal(completionTrigger(h.at(4), view, 'a.go'), undefined, "'.'");
  assert.equal(completionTrigger(h.at(view.state.doc.length), view, 'a.go'), undefined, "'('");
  assert.deepEqual(completionTrigger(h.at(4, true), view, 'a.go'), { kind: 'invoked' }, 'the shortcut still asks');
  // An older host that does not say keeps the likely ones.
  const old = serverEditor('fmt.');
  old.reply = async () => ({ items: [], incomplete: false });
  await old.suggest(4);
  assert.deepEqual(completionTrigger(old.at(4), old.view, 'a.go'), { kind: 'character', character: '.' });
  h.editor.destroy(); old.editor.destroy();
});

// An editor offering helperValue, whose import comes only on resolve (vtsls, CC-12).
async function resolvingEditor(resolveEdits, item = { label: 'helperValue', kind: 6 }) {
  const resolves = [];
  const h = serverEditor('export {};\nhelperVa', 'a.ts', async () => ({ incomplete: false, items: [item],
    resolveEdits: (index, current, signal) => { resolves.push({ index, current, signal }); return resolveEdits(index); } }));
  const end = h.view.state.doc.length;
  h.view.dispatch({ selection: { anchor: end } });
  const result = await h.suggest(end);
  h.resolves = resolves;
  h.end = end;
  h.accept = (to = end) => { const o = result.options[0]; o.apply(h.view, o, result.from, to); };
  return h;
}
const helperImport = { range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }, newText: 'import { helperValue } from "./helper.js";\n' };
const settled = () => new Promise(resolve => setImmediate(resolve));

test('accepting an item whose import comes on resolve waits for it and applies both as one undo step', async () => {
  let answer;
  const h = await resolvingEditor(() => new Promise(resolve => { answer = resolve; }));
  const { view } = h, depth = undoDepth(view.state);
  h.accept();
  assert.equal(view.state.doc.toString(), 'export {};\nhelperVa', 'waits');
  assert.equal(completionStatus(view.state), null, 'the popup closed');
  assert.equal(h.resolves[0].index, 0);
  assert.equal(h.resolves[0].current(), true, 'asked for the text shown');
  answer([helperImport]);
  await settled();
  assert.equal(view.state.doc.toString(), 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue');
  assert.equal(view.state.selection.main.head, view.state.doc.length);
  assert.equal(h.resolves[0].signal.aborted, true, 'done with');
  assert.equal(undoDepth(view.state), depth + 1);
  undo(view);
  assert.equal(view.state.doc.toString(), 'export {};\nhelperVa', 'one undo removes both');
  h.editor.destroy();
});

test('an import known for the text shown applies at once; listed imports never wait', async () => {
  const h = await resolvingEditor(() => [helperImport]);
  h.accept();
  assert.equal(h.view.state.doc.toString(), 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue');
  h.editor.destroy();
  const listed = await resolvingEditor(() => { throw new Error('must not resolve'); }, { label: 'helperValue', extraEdits: [{ ...helperImport, newText: 'import "listed";\n' }] });
  listed.accept();
  assert.equal(listed.view.state.doc.toString(), 'import "listed";\nexport {};\nhelperValue');
  assert.equal(listed.resolves.length, 0);
  listed.editor.destroy();
  const throwing = await resolvingEditor(() => { throw new Error('gone'); });
  throwing.accept();
  assert.equal(throwing.view.state.doc.toString(), 'export {};\nhelperValue', 'a throw: the item alone');
  throwing.editor.destroy();
});

test('a slow, failing or empty resolve inserts the item alone', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  let answer;
  const slow = await resolvingEditor(() => new Promise(resolve => { answer = resolve; }));
  slow.accept();
  t.mock.timers.tick(RESOLVE_ACCEPT_WAIT - 1);
  assert.equal(slow.view.state.doc.toString(), 'export {};\nhelperVa');
  t.mock.timers.tick(1);
  assert.equal(slow.view.state.doc.toString(), 'export {};\nhelperValue');
  assert.equal(slow.resolves[0].signal.aborted, true);
  answer([helperImport]);
  await settled();
  assert.equal(slow.view.state.doc.toString(), 'export {};\nhelperValue', 'a late import is not added');
  slow.editor.destroy();
  for (const resolveEdits of [() => Promise.reject(new Error('staleVersion')), () => Promise.resolve(undefined), () => Promise.resolve([])]) {
    const h = await resolvingEditor(resolveEdits);
    h.accept();
    await settled();
    assert.equal(h.view.state.doc.toString(), 'export {};\nhelperValue');
    assert.equal(h.view.state.selection.main.head, h.view.state.doc.length);
    h.editor.destroy();
  }
});

test('typing while the import is awaited stays, and the item still goes in without it', async () => {
  let answer;
  const h = await resolvingEditor(() => new Promise(resolve => { answer = resolve; }));
  const { view } = h;
  h.accept();
  typed(view, '// note\n', 0);
  typed(view, '(', view.state.doc.length);
  answer([helperImport]);
  await settled();
  assert.equal(view.state.doc.toString(), '// note\nexport {};\nhelperValue(', 'the typed text kept, no import');
  assert.equal(view.state.selection.main.head, view.state.doc.length, 'the caret stays after the typing');
  undo(view);
  assert.equal(view.state.doc.toString(), '// note\nexport {};\nhelperVa(');
  h.editor.destroy();
});

test('an awaited item at an empty range (after a trigger character) goes before what is typed meanwhile', async () => {
  let answer;
  const resolves = [];
  const h = serverEditor('obj.', 'a.ts', async () => ({ incomplete: false, items: [{ label: 'foo', kind: 5 }],
    resolveEdits: (index) => { resolves.push(index); return new Promise(resolve => { answer = resolve; }); } }));
  const { view } = h, end = view.state.doc.length;
  view.dispatch({ selection: { anchor: end } });
  const result = await h.suggest(end);
  assert.equal(result.from, end, 'it replaces nothing');
  const o = result.options[0];
  o.apply(view, o, result.from, end);
  assert.deepEqual(resolves, [0]);
  typed(view, 'x', end);
  answer([]);
  await settled();
  assert.equal(view.state.doc.toString(), 'obj.foox');
  assert.equal(view.state.selection.main.head, view.state.doc.length, 'the caret stays after the typing');
  h.editor.destroy();
});

test('an awaited item whose word was deleted or typed over goes nowhere; a shortened word is still replaced', async () => {
  const backspace = (view, at) => view.dispatch({ changes: { from: at - 1, to: at }, userEvent: 'delete.backward' });
  const cases = [
    ['deleted', (view, w) => view.dispatch({ changes: { from: w.from, to: w.to }, userEvent: 'delete.selection' }), 'export {};\n'],
    ['typed over', (view, w) => view.dispatch({ changes: { from: w.from, to: w.to, insert: 'x' }, userEvent: 'input.type' }), 'export {};\nx'],
    ['deleted one by one', (view, w) => { for (let at = w.to; at > w.from; at--) backspace(view, at); }, 'export {};\n'],
    ['shortened', (view, w) => backspace(view, w.to), 'export {};\nhelperValue'],
  ];
  for (const [what, edit, expected] of cases) {
    let answer;
    const h = await resolvingEditor(() => new Promise(resolve => { answer = resolve; }));
    const { view } = h;
    h.accept();
    edit(view, view.state.doc.line(2));
    const depth = undoDepth(view.state);
    answer([helperImport]);
    await settled();
    assert.equal(view.state.doc.toString(), expected, what);
    assert.equal(undoDepth(view.state), depth + (what === 'shortened' ? 1 : 0), `${what}: undo history`);
    h.editor.destroy();
  }
});

test('accepting again puts an awaited item in first; another file in the editor gets nothing', async () => {
  const answers = [];
  const h = await resolvingEditor(() => new Promise(resolve => answers.push(resolve)));
  const { view } = h;
  h.accept();
  typed(view, ' helperVa', h.end);
  const end = view.state.doc.length, result = await h.suggest(end), o = result.options[0];
  o.apply(view, o, result.from, end);
  assert.equal(view.state.doc.toString(), 'export {};\nhelperValue helperVa', 'the first goes in at once, the second waits');
  answers[1]([helperImport]);
  answers[0]([helperImport]);
  await settled();
  assert.equal(view.state.doc.toString(), 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue helperValue');

  let late;
  const other = await resolvingEditor(() => new Promise(resolve => { late = resolve; }));
  other.accept();
  other.editor.update({ ...other.options, id: {}, path: 'b.ts', text: 'const b = 1;\n' });
  late([helperImport]);
  await settled();
  assert.equal(other.view.state.doc.toString(), 'const b = 1;\n');
  other.editor.destroy();
  h.editor.destroy();
});

test('snippet fields: mirrors change together, Tab and Shift-Tab move, lines take the indentation, Escape leaves', async () => {
  const text = 'func f() {\n\tfo\n}\n';
  const reply = async () => ({ incomplete: false, items: [{
    label: 'for', kind: 15, snippet: true,
    insertText: 'for ${1:i} := 0; $1 < ${2:n}; $1++ {\n\t$0\n}',
  }] });
  const multiple = view => view.state.facet(EditorState.allowMultipleSelections);
  const h = serverEditor(text, 'a.go', reply);
  const { view } = h, end = view.state.doc.line(2).to;
  view.dispatch({ selection: { anchor: end } });
  const result = await h.suggest(end);
  const option = result.options[0];
  assert.equal(multiple(view), false, 'one selection while editing');
  const depth = undoDepth(view.state);
  option.apply(view, option, result.from, end);
  assert.equal(view.state.doc.toString(), 'func f() {\n\tfor i := 0; i < n; i++ {\n\t\t\n\t}\n}\n', 'new lines indented under the first');
  assert.equal(undoDepth(view.state), depth + 1);
  assert.deepEqual(view.state.selection.ranges.map(r => view.state.sliceDoc(r.from, r.to)), ['i', 'i', 'i'], 'every $1 is selected');
  view.dispatch(view.state.replaceSelection('k'));
  assert.equal(view.state.doc.line(2).text, '\tfor k := 0; k < n; k++ {', 'mirrors change together');
  assert.equal(nextSnippetField(view), true);
  assert.deepEqual(view.state.selection.ranges.map(r => view.state.sliceDoc(r.from, r.to)), ['n']);
  assert.equal(prevSnippetField(view), true);
  assert.equal(view.state.selection.ranges.length, 3, 'back on $1');
  nextSnippetField(view);
  nextSnippetField(view);
  assert.equal(view.state.selection.main.head, view.state.doc.line(3).to, '$0 inside the body');
  assert.equal(hasNextSnippetField(view.state), false, 'done at $0');
  assert.equal(multiple(view), false, 'one selection again');
  undo(view);
  assert.equal(view.state.doc.toString(), 'func f() {\n\tfor i := 0; i < n; i++ {\n\t\t\n\t}\n}\n', 'the typing in the field undoes first');
  undo(view);
  assert.equal(view.state.doc.toString(), text, 'then the snippet, in one step');

  // Escape leaves the fields and their cursors; one selection is left.
  const again = await h.suggest(end);
  again.options[0].apply(view, again.options[0], again.from, end);
  assert.equal(view.state.selection.ranges.length, 3);
  assert.equal(clearSnippet(view), true);
  assert.equal(hasNextSnippetField(view.state), false);
  assert.equal(view.state.selection.ranges.length, 1);
  assert.equal(multiple(view), false);
  // A click elsewhere ends it too.
  undo(view);
  const third = await h.suggest(end);
  third.options[0].apply(view, third.options[0], third.from, end);
  assert.equal(hasNextSnippetField(view.state), true);
  view.dispatch({ selection: { anchor: 0 } });
  assert.equal(multiple(view), false);
  assert.equal(hasNextSnippetField(view.state), false, 'the fields are gone');
  assert.equal(view.state.selection.ranges.length, 1);
  h.editor.destroy();
});

test('the details panel shows the signature and the documentation the owner rendered', async () => {
  const asked = [];
  const h = serverEditor('al', 'a.ts', async () => ({ incomplete: false, items: [{ label: 'alpha' }, { label: 'alert' }],
    details: async index => { asked.push(index); return index ? undefined : { detail: 'let alpha: number', html: '<p>Docs</p>', truncated: true }; } }));
  const [alpha, alert] = (await h.suggest()).options;
  const previous = globalThis.document;
  try {
    globalThis.document = { createElement: () => ({ children: [], style: {}, appendChild(child) { this.children.push(child); return child; }, addEventListener() {} }) };
    const dom = await alpha.info(alpha);
    assert.equal(dom.className, 'cm-symbol-hover cm-completion-docs');
    assert.deepEqual(dom.children.map(c => c.className), ['cm-completion-signature', 'hover-body', 'hover-truncated']);
    assert.equal(dom.children[0].textContent, 'let alpha: number', 'the signature is text, never HTML');
    assert.equal(dom.children[1].innerHTML, '<p>Docs</p>');
    assert.equal(await alert.info(alert), null);
    assert.deepEqual(asked, [0, 1], 'by the index in the answer');
  } finally { globalThis.document = previous; h.editor.destroy(); }
});

test('deprecated suggestions are struck through', () => {
  const source = readFileSync(EDITOR_URL, 'utf8');
  assert.match(source, /optionClass/);
  assert.match(source, /'\.cm-completion-deprecated \.cm-completionLabel': \{ textDecoration: 'line-through'/);
});
