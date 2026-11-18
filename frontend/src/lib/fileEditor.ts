import { Annotation, ChangeSet, CharCategory, Compartment, EditorState, Prec, RangeSet, StateEffect, StateField, type Extension, type SelectionRange, type Text, type Transaction, type TransactionSpec } from '@codemirror/state';
import { forEachDiagnostic, lintKeymap, setDiagnostics, setDiagnosticsEffect, type Diagnostic } from '@codemirror/lint';
import { projectSourceDiagnostics } from './sourceDiagnostics';
import type { AttributedCodeDiagnostic, LiveDocumentBasis } from './codeDiagnostics';
import { EditorView, GutterMarker, closeHoverTooltips, drawSelection, hasHoverTooltips, highlightActiveLine, hoverTooltip, keymap, lineNumberMarkers, lineNumbers, tooltips, type KeyBinding, type Tooltip } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab, isolateHistory } from '@codemirror/commands';
import { LanguageDescription, indentOnInput, indentUnit } from '@codemirror/language';
import { Decoration, type DecorationSet } from '@codemirror/view';
import { HIGHLIGHT_LIMIT, loadFileHighlighter, highlightFileWith } from './codeHighlight';
import type { HLJSApi } from 'highlight.js';
import { keyHandled } from './fullscreen';
import type { FilePosition } from './filePosition';
import type { CodeDiagnosticRange } from './codeDiagnostics';
import { rangeOffsets } from './problemNavigation';
import type { FileLineSelection } from './fileSelection';
import { fileLanguage } from './files';
import { textChanges } from './searchReplace';
import { closeSearchPanel, openSearchPanel, search, searchKeymap, searchPanelOpen } from '@codemirror/search';
import { acceptCompletion, autocompletion, closeCompletion, completeAnyWord, completionKeymap, hasNextSnippetField, hasPrevSnippetField, pickedCompletion, selectedCompletion, snippet, startCompletion, CompletionContext, type Completion, type CompletionResult, type CompletionSource } from '@codemirror/autocomplete';
import type { CodeCompletionItem, CodeCompletionTrigger, CodeDiagnosticPosition, CodeTextEdit } from './codeDiagnostics';
import { IDENTIFIER, LIKELY_TRIGGERS, answerStart, caretAfter, completionChanges, completionDetail, completionType, identifierStart, serverOrder, snippetText, withExtraEdits, type CompletionAccept, type CompletionEdit, type EditorCompleter, type EditorCompletionAnswer, type EditorCompletionDetails } from './editorCompletion';
import { HOVER_DELAY, contextPress, linkModifier, menuKey, wordRange, type SelectionHold, type ViewportPoint } from './sourceNavigation';
import { handleCopyCodeClick } from './copycode';
import { severityRank, type OverviewProblem, type ProblemSeverity } from './problemOverview';

/**
 * Code navigation from the draft. `current` says whether the editor still
 * shows the text the position was taken from; an answer is wanted only
 * while it does.
 */
export interface EditorSymbols {
  /** Go to definition (Cmd/Ctrl+click, F12) or find references (Shift+F12). */
  ask(op: 'definition' | 'references', position: CodeDiagnosticPosition, at: ViewportPoint, current: () => boolean): void;
  /** The hover for the word at position, as HTML; undefined shows nothing. */
  hover(position: CodeDiagnosticPosition, current: () => boolean): Promise<{ html: string; truncated: boolean } | undefined>;
  menu?(position: CodeDiagnosticPosition, word: string, at: ViewportPoint, current: () => boolean): void;
  /** A context menu on selected text: Copy only. */
  selectionMenu?(text: string, at: ViewportPoint, hold: SelectionHold): void;
  /** Language-server suggestions at position (edit mode); see EditorCompleter. */
  complete?: EditorCompleter;
}

export interface FileEditorOptions {
  id: object;
  text: string;
  path: string;
  version: number;
  wrap: boolean;
  disabled?: boolean;
  /** Suggestions open while typing (Settings › General › Editor); the shortcut works either way. */
  suggest?: boolean;
  /** Context-bound snapshot supplied anew by the viewer, never cached as an owner. */
  diagnostics?: { tab: object; path: string; context: string; findings: AttributedCodeDiagnostic[]; dirty: boolean; revision?: string; live?: LiveDocumentBasis; current: boolean };
  position?: FilePosition;
  selection?: FileLineSelection;
  onselection?: (selection: FileLineSelection) => void;
  onposition?: (position: FilePosition) => void;
  onchange: (text: string) => void;
  onsave: () => void;
  /** Read through owners when asked, like every callback here. */
  symbols?: EditorSymbols;
  /** The diagnostics the editor shows, for the overview ruler; called when they change. */
  onproblems?: (problems: OverviewProblem[]) => void;
}

interface CachedEditor {
  state: EditorState;
  version: number;
  top: number;
  left: number;
}

// Only immutable editor state and scroll coordinates survive a tab's unmount.
// In particular, cached extensions must never close over a view or its props.
const tabs = new WeakMap<object, CachedEditor>();
const owners = new WeakMap<EditorView, FileEditorOptions>();
const wrapping = new Compartment();
const editing = new Compartment();
const language = new Compartment();
const coloring = new Compartment();
const font = new Compartment();
const completing = new Compartment();
const editable = (disabled = false): Extension => [
  EditorView.editable.of(!disabled),
  EditorState.readOnly.of(disabled),
];

/**
 * Enter accepts the selected suggestion, unless it is exactly the word just
 * typed (a variable typed out in full): then the popup closes and Enter
 * breaks the line, as it would have without one.
 */
export function acceptOrBreakLine(view: EditorView): boolean {
  const selected = selectedCompletion(view.state);
  const { head, empty } = view.state.selection.main;
  const text = selected && plainText(selected);
  const start = head - (text?.length ?? 0);
  if (selected && text !== undefined && empty && start >= 0
    && view.state.sliceDoc(start, head) === text
    && (start === 0 || view.state.charCategorizer(start)(view.state.sliceDoc(start - 1, start)) !== CharCategory.Word)) {
    closeCompletion(view);
    return false;
  }
  return acceptCompletion(view);
}

/** What accepting option puts in when it changes nothing else; undefined when it may. */
function plainText(option: Completion): string | undefined {
  if (option.apply === undefined) return option.label;
  if (typeof option.apply === 'string') return option.apply;
  // A server item's own text.
  return plainInserts.get(option);
}

/**
 * Completion keys. CodeMirror's own list, except what opens the popup: on
 * macOS Option+` and Option+I are dead keys (accents, and Vietnamese input),
 * and Ctrl+Space switches the input source, so Cmd+I opens it there. Tab
 * accepts like Enter; without a popup both keep their meaning.
 */
export const completionKeys: readonly KeyBinding[] = [
  { mac: 'Cmd-i', run: startCompletion },
  { key: 'Ctrl-Space', run: startCompletion },
  ...completionKeymap.filter(binding => binding.run !== startCompletion && binding.key !== 'Enter'),
  { key: 'Enter', run: acceptOrBreakLine },
  { key: 'Tab', run: acceptCompletion },
];

// Language sources seen to answer with a promise. They are not run again for
// their labels: that run would do the whole work and could not be cancelled.
const asyncSources = new WeakSet<CompletionSource>();

// What the language's own sources offer here, so words do not repeat them.
// Only answers given at once count; CodeMirror merges the rest as they come.
function languageLabels(context: CompletionContext): Set<string> {
  const labels = new Set<string>();
  const add = (option: string | Completion) => labels.add(typeof option === 'string' ? option : option.label);
  for (const source of context.state.languageDataAt<CompletionSource | readonly (string | Completion)[]>('autocomplete', context.pos)) {
    if (source === documentWords || source === codeSuggestions) continue;
    if (typeof source !== 'function') { source.forEach(add); continue; }
    if (asyncSources.has(source)) continue;
    try {
      const result = source(new CompletionContext(context.state, context.pos, context.explicit, context.view));
      // Not waited for: a rejection is CodeMirror's to report when it runs the source.
      if (result instanceof Promise) { asyncSources.add(source); result.catch(() => {}); }
      else if (result) result.options.forEach(add);
    } catch { /* A failing source offers nothing; CodeMirror reports it when it runs it. */ }
  }
  return labels;
}

/**
 * Words from the document, as CodeMirror's word source finds them, without
 * those the language already offers (a keyword, a CSS property). The word
 * being typed is left out once complete, so Enter after it still breaks the
 * line. Large documents get none, as they get no colours.
 */
export const documentWords: CompletionSource = context => {
  if (context.state.doc.length > HIGHLIGHT_LIMIT) return null;
  const result = completeAnyWord(context);
  if (!result || result instanceof Promise) return result;
  const typed = context.state.sliceDoc(result.from, context.pos);
  const offered = languageLabels(context);
  const words = new Set(result.options.map(option => option.label));
  // completeAnyWord's pattern is anchored at the start only; CodeMirror anchors both ends.
  const pattern = result.validFor instanceof RegExp ? new RegExp(`(?:${result.validFor.source})$`, result.validFor.flags) : null;
  return {
    ...result,
    options: result.options.filter(option => option.label !== typed && !offered.has(option.label)),
    // Typing on narrows the list here; reaching a whole word asks again, so it drops out.
    validFor: text => !words.has(text) && (!pattern || pattern.test(text)),
  };
};

/** How long suggestions wait for the language server before the document's words show alone. */
export const SERVER_SUGGESTION_WAIT = 1500;

// Per view, what its language server said: the characters it suggests after,
// and where an incomplete answer's word starts, so typing on asks again as such.
const learnedTriggers = new WeakMap<EditorView, { path: string; triggers: string }>();
const unfinished = new WeakMap<EditorView, { path: string; start: number }>();
// Server items struck through, and what an item inserts when it changes nothing else.
const deprecatedOptions = new WeakSet<Completion>();
const plainInserts = new WeakMap<Completion, string>();

/**
 * Why the language server is asked at context, or undefined to offer words
 * only: on request; while an identifier is typed (again as `incomplete` after
 * an incomplete answer for the same word); right after one of the server's
 * trigger characters, or a likely one until the server has named its own.
 */
export function completionTrigger(context: CompletionContext, view: EditorView, path: string): CodeCompletionTrigger | undefined {
  if (context.explicit) { unfinished.delete(view); return { kind: 'invoked' }; }
  const { doc } = context.state, pos = context.pos;
  const start = identifierStart(doc, pos);
  if (start < pos) {
    const last = unfinished.get(view);
    return last?.path === path && last.start === start ? { kind: 'incomplete' } : { kind: 'invoked' };
  }
  const line = doc.lineAt(pos);
  const character = Array.from(line.text.slice(0, pos - line.from)).pop();
  const known = learnedTriggers.get(view);
  const triggers = known?.path === path ? known.triggers : LIKELY_TRIGGERS;
  return character && triggers.includes(character) ? { kind: 'character', character } : undefined;
}

function wordsOnly(context: CompletionContext): CompletionResult | null {
  const words = documentWords(context);
  return words instanceof Promise ? null : words;
}

/**
 * Whether the word asked about (from start to pos in asked) is still at the
 * same place in doc: the same document, or one typed on after the word.
 */
function sameWord(doc: Text, asked: Text, start: number, pos: number): boolean {
  return doc === asked || (pos <= doc.length && doc.sliceString(start, pos) === asked.sliceString(start, pos) && identifierStart(doc, pos) === start);
}

/** Resolves undefined, and aborts the request, when the server takes longer than wait. */
function answerWithin(answer: Promise<EditorCompletionAnswer | undefined>, wait: number, controller: AbortController): Promise<EditorCompletionAnswer | undefined> {
  return new Promise(resolve => {
    const timer = setTimeout(() => { controller.abort(); resolve(undefined); }, wait);
    answer.then(a => { clearTimeout(timer); resolve(a); }, () => { clearTimeout(timer); resolve(undefined); });
  });
}

/** The info panel: the item's signature, then its documentation (HTML the owner rendered). */
function completionInfo(d: EditorCompletionDetails | undefined): HTMLElement | null {
  if (!d || (!d.detail && !d.html)) return null;
  const dom = document.createElement('div');
  dom.className = 'cm-symbol-hover cm-completion-docs';
  if (d.detail) {
    const detail = dom.appendChild(document.createElement('div'));
    detail.className = 'cm-completion-signature';
    detail.textContent = d.detail;
  }
  if (d.html) {
    const body = dom.appendChild(document.createElement('div'));
    body.className = 'hover-body';
    body.innerHTML = d.html;
  }
  if (d.truncated) {
    const note = dom.appendChild(document.createElement('div'));
    note.className = 'hover-truncated';
    note.textContent = 'The description was cut short.';
  }
  dom.addEventListener('click', handleCopyCodeClick);
  return dom;
}

// A snippet's mirrors ($1 twice) are edited together as several selections:
// allowed while a snippet is active, back to one selection when it ends.
const mirrors = new Compartment();
const allowMirrors = EditorState.allowMultipleSelections.of(true);
const endMirrors = EditorState.transactionFilter.of(tr => {
  if (mirrors.get(tr.startState) !== allowMirrors) return tr;
  // Only while mirrors are on, which is rare: the new state is needed to know the snippet ended.
  const state = tr.state;
  if (hasNextSnippetField(state) || hasPrevSnippetField(state)) return tr;
  return [tr, { effects: mirrors.reconfigure([]), selection: state.selection.asSingle(), sequential: true }];
});

/**
 * What CodeMirror's snippet() would do to insert template over edit: its
 * change, the first field selected and the fields made active. Undefined,
 * so the plain text goes in, when it fails.
 */
function snippetFields(state: EditorState, template: string, edit: CompletionEdit): Transaction | undefined {
  let made: Transaction | undefined;
  try {
    snippet(template)({ state, dispatch: tr => { made = tr; } }, null, edit.from, edit.to);
  } catch {
    return undefined;
  }
  return made;
}

/**
 * Applies an accepted item as one step to undo, whatever it changed. With
 * `typedSince`, the user typed while its import was awaited: the selection
 * stays theirs, mapped, and a snippet goes in as plain text.
 */
function applyAccept(view: EditorView, completion: Completion, accept: CompletionAccept, typedSince = false, effects: StateEffect<unknown>[] = []): ChangeSet {
  const { changes, main, caret, template } = accept;
  const annotations = [pickedCompletion.of(completion), isolateHistory.of('full')];
  const send = (tr: Transaction) => { view.dispatch(tr); return tr.changes; };
  if (typedSince) return send(view.state.update({ changes, effects, userEvent: 'input.complete', annotations }));
  let fields = template !== undefined ? snippetFields(view.state, template, changes[main]) : undefined;
  if (fields?.selection && fields.selection.ranges.length > 1 && !view.state.facet(EditorState.allowMultipleSelections)) {
    // Selecting every mirror needs several selections before the snippet goes in.
    view.dispatch({ effects: mirrors.reconfigure(allowMirrors) });
    fields = snippetFields(view.state, template!, changes[main]);
  }
  if (fields) {
    // The snippet with its fields, and the other edits in the same transaction.
    return send(view.state.update(
      { changes: fields.changes, selection: fields.selection, effects: [...(fields.effects ? [fields.effects].flat() : []), ...effects], scrollIntoView: true, userEvent: 'input.complete', annotations },
      { changes: changes.filter((_, i) => i !== main) },
    ));
  }
  return send(view.state.update({ changes, selection: { anchor: caretAfter(changes, main, caret) }, effects, scrollIntoView: true, userEvent: 'input.complete', annotations }));
}

/** How long accepting an item waits for the import its server gives only on resolve (CC-12). */
export const RESOLVE_ACCEPT_WAIT = 500;

// An accepted item awaiting its import: the text it replaces, followed through
// typing; gone once a change removed or replaced all of that text.
interface PendingPick { id: number; from: number; to: number; gone?: boolean }
const setPendingPick = StateEffect.define<PendingPick | null>();
const pendingPick = StateField.define<PendingPick | null>({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setPendingPick)) value = e.value;
    if (!value || !tr.docChanged) return value;
    let gone = value.gone;
    if (value.from < value.to) tr.changes.iterChangedRanges((fromA, toA) => { if (fromA <= value.from && toA >= value.to) gone = true; });
    // Text typed at either end stays outside it.
    const from = tr.changes.mapPos(value.from, 1), to = tr.changes.mapPos(value.to, -1);
    return { ...value, from, to: Math.max(from, to), gone };
  },
});
let pickIds = 0;
// Per view, how to put its awaited item in now, without the import; the change it made.
const pendingPicks = new WeakMap<EditorView, () => ChangeSet | undefined>();

/** Puts an item still awaiting its import in now; from and to mapped through it. */
function settleEarlierPick(view: EditorView, from: number, to: number): [number, number] {
  const changes = pendingPicks.get(view)?.();
  if (!changes) return [from, to];
  const mapped = changes.mapPos(from, 1);
  return [mapped, Math.max(mapped, changes.mapPos(to, -1))];
}

/**
 * Accepting a server item whose import comes only on resolve (vtsls): with
 * the import when it is known for the text shown. Otherwise the popup
 * closes and the item goes in once its import is resolved, or without it
 * after RESOLVE_ACCEPT_WAIT, a failure, or typing meanwhile; what was typed
 * stays. Accepting again puts an awaited item in at once.
 */
function acceptResolving(view: EditorView, completion: Completion, item: CodeCompletionItem, index: number, answer: EditorCompletionAnswer, askedDoc: Text, pos: number, start: number, from: number, to: number): void {
  const owner = owners.get(view), shown = view.state.doc;
  const accept = (doc: Text, from: number, to: number) => completionChanges(askedDoc, pos, start, doc, from, to, item);
  const controller = new AbortController();
  let got: CodeTextEdit[] | Promise<CodeTextEdit[] | undefined>;
  try { got = answer.resolveEdits!(index, askedText(view), controller.signal); } catch { got = []; }
  if (Array.isArray(got)) { applyAccept(view, completion, withExtraEdits(accept(shown, from, to), shown, got)); return; }
  const id = ++pickIds;
  view.dispatch({ effects: setPendingPick.of({ id, from, to }) });
  closeCompletion(view);
  let done = false;
  const settle = (edits?: CodeTextEdit[]): ChangeSet | undefined => {
    if (done) return undefined;
    done = true;
    clearTimeout(timer);
    controller.abort();
    if (pendingPicks.get(view) === now) pendingPicks.delete(view);
    const pick = view.state.field(pendingPick, false), still = owners.get(view);
    // Another file or tab in the editor, or the editor gone: nothing goes in.
    if (!pick || pick.id !== id || !owner || still?.id !== owner.id || still.path !== owner.path) return undefined;
    // The word it was to replace was deleted or typed over meanwhile: nothing goes in.
    if (pick.gone) {
      view.dispatch({ effects: setPendingPick.of(null) });
      return undefined;
    }
    const doc = view.state.doc, typed = doc !== shown;
    let changes = accept(doc, pick.from, pick.to);
    if (!typed && edits?.length) changes = withExtraEdits(changes, doc, edits);
    return applyAccept(view, completion, changes, typed, [setPendingPick.of(null)]);
  };
  const now = () => settle();
  const timer = setTimeout(now, RESOLVE_ACCEPT_WAIT);
  pendingPicks.set(view, now);
  got.then(settle, () => settle());
}

/** A CodeMirror option for server item index of an answer asked at pos in askedDoc. */
function serverOption(item: CodeCompletionItem, index: number, { boost, sortText }: { boost: number; sortText: string }, answer: EditorCompletionAnswer, askedDoc: Text, pos: number, start: number): Completion {
  const label = item.filterText || item.label;
  const option: Completion = {
    label,
    ...(label !== item.label ? { displayLabel: item.label } : {}),
    detail: completionDetail(item),
    type: completionType(item.kind),
    boost,
    sortText,
    apply(view, completion, from, to) {
      [from, to] = settleEarlierPick(view, from, to);
      // Listed imports go in with the item; others may come on resolve.
      if (!item.extraEdits?.length && answer.resolveEdits) acceptResolving(view, completion, item, index, answer, askedDoc, pos, start, from, to);
      else applyAccept(view, completion, completionChanges(askedDoc, pos, start, view.state.doc, from, to, item));
    },
    ...(answer.details ? { info: async () => completionInfo(await answer.details!(index)) } : {}),
  };
  if (item.deprecated) deprecatedOptions.add(option);
  const plain = itemText(item, answer);
  if (plain !== undefined) plainInserts.set(option, plain);
  return option;
}

/**
 * What accepting item inserts, as plain text, when it changes nothing else;
 * undefined when it may. Listed extra edits do; so may an item that names a
 * module (vtsls's auto-imports, in labelDescription) when its answer
 * resolves edits on accept.
 */
function itemText(item: CodeCompletionItem, answer: EditorCompletionAnswer): string | undefined {
  if (item.extraEdits?.length || (answer.resolveEdits && item.labelDescription)) return undefined;
  const source = item.edit?.newText ?? item.insertText ?? item.label;
  return item.snippet ? snippetText(source).text : source;
}

/**
 * Suggestions in the editor: the language server's, when its owner can ask
 * one (EditorSymbols.complete), above the document's words it did not offer.
 * Words alone while no server can answer, and when it fails or is slow.
 */
export const codeSuggestions: CompletionSource = context => {
  const view = context.view, owner = view && owners.get(view), complete = owner?.symbols?.complete;
  const trigger = view && owner && complete && completionTrigger(context, view, owner.path);
  if (!view || !owner || !complete || !trigger) return wordsOnly(context);
  const askedDoc = context.state.doc, pos = context.pos, path = owner.path;
  const controller = new AbortController();
  context.addEventListener('abort', () => controller.abort());
  const pending = complete(editorPosition(context.state, pos), trigger, () => view.state.doc === askedDoc, controller.signal);
  if (!pending) return wordsOnly(context);
  return answerWithin(pending, SERVER_SUGGESTION_WAIT, controller).then(answer => {
    // CodeMirror dropped the query (another edit, the popup closed): its answer changes nothing.
    if (context.aborted) return null;
    const words = wordsOnly(context);
    // Asked again on the next keystroke: the server may answer then.
    if (!answer) return words && { ...words, validFor: undefined };
    if (answer.triggerCharacters !== undefined) learnedTriggers.set(view, { path, triggers: answer.triggerCharacters });
    const word = identifierStart(askedDoc, pos);
    if (answer.incomplete && sameWord(view.state.doc, askedDoc, word, pos)) unfinished.set(view, { path, start: word });
    else unfinished.delete(view);
    const items = answer.items, start = answerStart(askedDoc, pos, items), placed = serverOrder(items);
    const offered = new Set(items.map(item => item.label));
    const options = items.map((item, index) => serverOption(item, index, placed[index], answer, askedDoc, pos, start));
    if (words && words.from === start) options.push(...words.options.filter(option => !offered.has(option.label)));
    if (!options.length) return null;
    // An incomplete answer is asked again as the user types on, and so is an
    // empty one: a server still indexing (ruby-lsp) answers nothing at first.
    return { from: start, options, ...(answer.incomplete || !items.length ? {} : { validFor: IDENTIFIER }) };
  });
};

// One extension per mode, made once: options are applied on every keystroke,
// and the same extension again changes nothing (an open popup stays).
const completionParts: Extension = [
  pendingPick,
  Prec.highest(keymap.of(completionKeys)),
  EditorState.languageData.of(() => [{ autocomplete: codeSuggestions }]),
];
const optionClass = (option: Completion) => (deprecatedOptions.has(option) ? 'cm-completion-deprecated' : '');
const suggestionModes = {
  typing: [autocompletion({ defaultKeymap: false, activateOnTyping: true, optionClass }), completionParts],
  shortcut: [autocompletion({ defaultKeymap: false, activateOnTyping: false, optionClass }), completionParts],
};
// Read-only text gets no popup at all.
const suggestions = (options: Pick<FileEditorOptions, 'disabled' | 'suggest'>): Extension =>
  options.disabled ? [] : suggestionModes[options.suggest === false ? 'shortcut' : 'typing'];

/** The editor font (Settings → Appearance), with today's defaults as fallbacks; SourceView uses the same. */
export const EDITOR_FONT_SIZE = 'var(--editor-font-size, 0.78rem)';
export const EDITOR_FONT_FAMILY = 'var(--editor-font-family, var(--font-mono))';

// Two identical font themes. CodeMirror measures its text again only when its
// theme changes, so a font setting swaps one for the other (remeasure below).
// Swapping, rather than making a new theme each time, mounts no new styles.
const fontThemes = [0, 1].map(() => EditorView.theme({
  '&': { fontSize: EDITOR_FONT_SIZE },
  '.cm-scroller': { fontFamily: EDITOR_FONT_FAMILY },
}));

// Matches the viewer's SourceView (font, line height, gutter and line
// padding), so switching between viewing and editing does not move the text.
const theme = EditorView.theme({
  '&': { height: '100%', color: 'var(--text-primary)', backgroundColor: 'var(--bg-primary)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { overflow: 'auto', lineHeight: '1.5', tabSize: '4' },
  '.cm-content': { padding: '8px 0', caretColor: 'var(--text-primary)' },
  '.cm-line': { padding: '0 14px' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text-primary)' },
  '.cm-gutters': { backgroundColor: 'var(--bg-secondary)', color: 'var(--text-muted)', border: 'none', borderRight: '1px solid var(--border-color)' },
  '.cm-gutter': { paddingTop: '0', paddingBottom: '0' },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 10px 0 14px', minWidth: '0' },
  // Line numbers in their worst problem's colour, as in the viewer (SourceView).
  '.cm-lineNumbers .cm-has-diagnostics': { fontWeight: 'bold' },
  '.cm-lineNumbers .cm-severity-error': { color: '#f85149' },
  '.cm-lineNumbers .cm-severity-warning': { color: '#d29922' },
  '.cm-lineNumbers .cm-severity-information': { color: '#2f81f7' },
  '.cm-lineNumbers .cm-severity-hint': { color: '#8250df' },
  ':root:not([data-theme="dark"]) & .cm-lineNumbers .cm-severity-error': { color: '#cf222e' },
  ':root:not([data-theme="dark"]) & .cm-lineNumbers .cm-severity-warning': { color: '#9a6700' },
  // The selection layer sits behind the lines, so the active line's tint must
  // stay translucent or it hides a selection on the cursor's line.
  '.cm-activeLine': { backgroundColor: 'var(--hover-bg)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--bg-secondary)' },
  // Selectors as specific as CodeMirror's base theme, which would otherwise
  // paint its own (light-only) selection colours.
  '& > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'color-mix(in srgb, var(--accent-color) 16%, transparent)',
  },
  // A menu on the selection takes focus but leaves the selection as it was.
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, &.cm-focused .cm-content ::selection, &.cm-selection-held > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--accent-color) 30%, transparent)',
  },
  // Scrolls when shortened to fit the editor (see fitTooltips).
  '.cm-tooltip': { backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border-color)', overflowY: 'auto' },
  // A symbol's hover reads as the viewer's hover card (HoverCard.svelte).
  '.cm-tooltip.cm-symbol-hover': {
    width: 'max-content', maxWidth: 'min(560px, calc(100vw - 8px))', padding: '6px 10px', borderRadius: '6px',
    backgroundColor: 'var(--bg-primary)', boxShadow: '0 4px 16px rgba(0, 0, 0, 0.25)',
    fontFamily: 'var(--font-sans)', fontSize: '12.5px', lineHeight: '1.5', wordBreak: 'break-word', overflow: 'auto',
  },
  '.cm-symbol-hover p, .cm-symbol-hover .code-block-container, .cm-symbol-hover ul, .cm-symbol-hover ol': { margin: '0.4em 0' },
  '.cm-symbol-hover .hover-body > :first-child': { marginTop: '0' },
  '.cm-symbol-hover .hover-body > :last-child': { marginBottom: '0' },
  '.cm-symbol-hover ul, .cm-symbol-hover ol': { paddingLeft: '1.4em' },
  '.cm-symbol-hover .inline-code': { fontFamily: 'var(--font-mono)', fontSize: '0.9em', padding: '0 3px', borderRadius: '3px', backgroundColor: 'var(--bg-tertiary)' },
  '.cm-symbol-hover blockquote': { margin: '0.4em 0', paddingLeft: '0.8em', borderLeft: '2px solid var(--border-color)' },
  '.cm-symbol-hover a': { color: 'var(--accent-color)' },
  '.cm-symbol-hover .hover-plain': { margin: '0', fontFamily: 'var(--font-mono)', whiteSpace: 'pre-wrap' },
  '.cm-symbol-hover .hover-truncated': { marginTop: '4px', fontSize: '11px', color: 'var(--text-secondary)' },
  // Suggestions in the editor font, in the app's colours. The details panel
  // sits beside the list, inside its box, so the box must not clip.
  '.cm-tooltip.cm-tooltip-autocomplete': {
    overflow: 'visible', padding: '2px 0', borderRadius: '6px',
    backgroundColor: 'var(--bg-primary)', boxShadow: '0 4px 16px rgba(0, 0, 0, 0.25)',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': {
    fontFamily: EDITOR_FONT_FAMILY, maxHeight: '15em', minWidth: '14em', maxWidth: 'min(560px, calc(100vw - 8px))',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { padding: '1px 8px', lineHeight: '1.5' },
  // As specific as CodeMirror's base theme, whose light and dark rows are blue.
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'color-mix(in srgb, var(--accent-color) 22%, transparent)', color: 'var(--text-primary)',
  },
  '.cm-tooltip.cm-tooltip-autocomplete-disabled > ul > li[aria-selected]': { backgroundColor: 'var(--hover-bg)' },
  '.cm-completionMatchedText': { textDecoration: 'none', fontWeight: '600', color: 'var(--accent-color)' },
  '.cm-completionDetail': { marginLeft: '0.75em', fontStyle: 'normal', color: 'var(--text-muted)' },
  '.cm-completionIcon': { color: 'var(--text-secondary)' },
  '.cm-completion-deprecated .cm-completionLabel': { textDecoration: 'line-through', opacity: '0.75' },
  '.cm-completion-docs .cm-completion-signature': { fontFamily: EDITOR_FONT_FAMILY, whiteSpace: 'pre-wrap', margin: '0 0 4px' },
  '.cm-completion-docs > :last-child': { marginBottom: '0' },
  '.cm-tooltip.cm-completionInfo': {
    padding: '6px 10px', borderRadius: '6px', backgroundColor: 'var(--bg-primary)', boxShadow: '0 4px 16px rgba(0, 0, 0, 0.25)',
    fontFamily: 'var(--font-sans)', fontSize: '12.5px', lineHeight: '1.5', overflowWrap: 'anywhere',
  },
  '.cm-panels': { backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)' },
  '& .cm-panels-bottom': { borderTop: '1px solid var(--border-color)' },
  '& .cm-panels-top': { borderBottom: '1px solid var(--border-color)' },
  '.cm-textfield, .cm-button': { background: 'var(--bg-primary)', color: 'var(--text-primary)', border: '1px solid var(--border-color)' },
});

// Tooltips (diagnostic messages) fit in the editor's own box. Measured
// against the window, one above the first lines went under the file
// header; now it flips below, or shortens, when there is no room above.
const fitTooltips = tooltips({ tooltipSpace: view => view.scrollDOM.getBoundingClientRect() });

// Use exactly the viewer's tokenizer, not a best-effort mapping of Lezer
// tags: the two parsers disagree on properties, builtins, parameters, etc.
// Language extensions below still provide indentation and editing support.
function fileHighlight(hljs: HLJSApi, path: string): Extension {
  const language = fileLanguage(path);
  function decorate(state: EditorState): DecorationSet {
    if (state.doc.length > HIGHLIGHT_LIMIT) return Decoration.none;
    const html = highlightFileWith(hljs, docText(state.doc), language ?? '');
    const marks = [];
    const open: { from: number; class: string }[] = [];
    let offset = 0;
    // highlight.js emits only escaped text and span tags. Count decoded UTF-16
    // units so entities and non-ASCII text retain the document's exact offsets.
    for (const token of html.matchAll(/<span class="([^"]*)">|<\/span>|&(?:#\d+|#x[\da-f]+|\w+);|[^<&]+/gi)) {
      if (token[1] !== undefined) open.push({ from: offset, class: token[1] });
      else if (token[0] === '</span>') {
        const mark = open.pop();
        if (mark && mark.from < offset) marks.push(Decoration.mark({ class: mark.class }).range(mark.from, offset));
      } else offset += token[0].startsWith('&') ? 1 : token[0].length;
    }
    return Decoration.set(marks, true);
  }
  return StateField.define<DecorationSet>({
    create: decorate,
    update: (value, transaction) => transaction.docChanged ? decorate(transaction.state) : value,
    provide: field => EditorView.decorations.from(field),
  });
}

// Infer the unit once per document load, not on each keystroke (which could
// change the setting halfway through typing). Prefer repeated indent steps
// over absolute columns, so nested blocks and continuation alignment don't
// make a four-space file look like an eight-space file.
function detectIndent(text: string): string {
  let tabs = 0;
  let spaces = 0;
  let previous = 0;
  const widths = new Map<number, number>();
  for (const line of text.split('\n').slice(0, 10000)) {
    if (!line.trim()) continue;
    const prefix = /^[\t ]*/.exec(line)![0];
    if (prefix.includes('\t')) {
      tabs++;
      previous = 0;
      continue;
    }
    const width = prefix.length;
    if (width) spaces++;
    const step = Math.abs(width - previous);
    // Ignore single-column alignment and unusually deep continuation indents.
    if (step >= 2 && step <= 8) widths.set(step, (widths.get(step) ?? 0) + 1);
    previous = width;
  }
  if (tabs > spaces) return '\t';
  let best = 2;
  let count = 0;
  for (const [width, votes] of widths) {
    if (votes > count || (votes === count && width < best)) {
      best = width;
      count = votes;
    }
  }
  return ' '.repeat(best);
}

function lineSelection(text: string, selection: FileLineSelection) {
  const doc = EditorState.create({ doc: text }).doc;
  const line = (number: number) => doc.line(Math.max(1, Math.min(number, doc.lines)));
  return { anchor: line(selection.anchor).from, head: line(selection.head).from };
}

/** Convert against the actual LF-normalized EditorState document, sharing the
 * source viewer's provider fallback, freshness and strict UTF-16 validation. */
export function editorDiagnostics(state: EditorState, options: FileEditorOptions): Diagnostic[] {
  const input = options.diagnostics;
  if (!input || input.tab !== options.id || input.path !== options.path || !input.context
    || !showsText(state.doc, options.text, text => text.replace(/\r\n/g, '\n'))) return [];
  const text = docText(state.doc);
  return projectSourceDiagnostics(text, input.findings.filter(f => f.diagnostic.path === options.path), input, input.current)
    .messages.flatMap(({ finding, notice }) => {
      if (notice) return [];
      const d = finding.diagnostic;
      const offsets = rangeOffsets(text, d.range!);
      return offsets ? [{ from: offsets.anchor, to: offsets.head,
        severity: d.severity === 'information' ? 'info' as const : d.severity,
        source: [d.source || finding.entries.map(e => e.providerId).join(', '), d.code].filter(Boolean).join(' '),
        message: d.message }] : [];
    });
}

const problemSeverity = (severity: Diagnostic['severity']): ProblemSeverity => severity === 'info' ? 'information' : severity;

// Lines with diagnostics are marked on their numbers, in the colour of the
// worst, as in the viewer. A separate lint gutter would widen the gutter and
// move the text.
class DiagnosticLine extends GutterMarker {
  readonly severity: ProblemSeverity;
  constructor(severity: ProblemSeverity) { super(); this.severity = severity; this.elementClass = `cm-has-diagnostics cm-severity-${severity}`; }
  eq(other: GutterMarker) { return other instanceof DiagnosticLine && other.severity === this.severity; }
}
const diagnosticMarkers = Object.fromEntries((['error', 'warning', 'information', 'hint'] as const).map(s => [s, new DiagnosticLine(s)])) as Record<ProblemSeverity, DiagnosticLine>;
const diagnosticLines = StateField.define<RangeSet<GutterMarker>>({
  create: () => RangeSet.empty,
  update(value, tr) {
    if (!tr.docChanged && !tr.effects.some(e => e.is(setDiagnosticsEffect))) return value;
    const { doc } = tr.state;
    const worst = new Map<number, ProblemSeverity>();
    forEachDiagnostic(tr.state, (d, from, to) => {
      const severity = problemSeverity(d.severity);
      // The end is exclusive: a range ending at a line's start does not reach that line.
      for (let line = doc.lineAt(from).number, last = doc.lineAt(Math.max(from, to - 1)).number; line <= last; line++) {
        const start = doc.line(line).from, seen = worst.get(start);
        if (!seen || severityRank[severity] < severityRank[seen]) worst.set(start, severity);
      }
    });
    return RangeSet.of([...worst].map(([from, severity]) => diagnosticMarkers[severity].range(from)), true);
  },
  provide: field => lineNumberMarkers.from(field),
});

/** The diagnostics a state shows, as the overview ruler takes them. */
export function editorProblems(state: EditorState): OverviewProblem[] {
  const problems: OverviewProblem[] = [];
  forEachDiagnostic(state, (d, from, to) => problems.push({
    range: { start: editorPosition(state, from), end: editorPosition(state, to) },
    severity: problemSeverity(d.severity), message: d.message, source: d.source ?? '',
  }));
  return problems;
}
// What each view last reported, so an unchanged set is not reported again.
const reported = new WeakMap<EditorView, string>();
function reportProblems(view: EditorView) {
  const report = owners.get(view)?.onproblems;
  if (!report) return;
  const problems = editorProblems(view.state);
  const key = JSON.stringify(problems);
  if (reported.get(view) === key) return;
  reported.set(view, key);
  report(problems);
}

// Clearing in the same transaction prevents even one frame of automatically
// mapped saved markers. Undo is a document change too, not freshness evidence.
const clearSavedMarkers = EditorState.transactionExtender.of(tr => tr.docChanged
  ? { effects: setDiagnosticsEffect.of([]) } : null);
function withDiagnostics(state: EditorState, options: FileEditorOptions): EditorState {
  return state.update(setDiagnostics(state, editorDiagnostics(state, options))).state;
}

// F8 still steps through diagnostics. Mod-Shift-m would open CodeMirror's own
// lint panel; Ctrl+Shift+M toggles the dialog's problems panel instead.
const editorLintKeymap = lintKeymap.filter(binding => binding.key !== 'Mod-Shift-m');

// Marks a draft the store changed (a replace in files), already in the draft.
const external = Annotation.define<boolean>();

// Code navigation: Cmd/Ctrl+click, F12, Shift+F12 and hover. Positions are
// LSP's (0-based line, UTF-16 column), taken from the document shown, which
// counts UTF-16 units as LSP does. Like every handler here, these find the
// view's current options through owners, never through a closure.

/** The 0-based line and UTF-16 column of a document offset. */
export function editorPosition(state: EditorState, pos: number): CodeDiagnosticPosition {
  const line = state.doc.lineAt(pos);
  return { line: line.number - 1, character: pos - line.from };
}

/** The word (identifier) the pointer is over: the character after pos when side > 0, else before it. */
export function wordAtSide(state: EditorState, pos: number, side: number): { from: number; to: number } | undefined {
  const line = state.doc.lineAt(pos);
  const at = side < 0 ? pos - 1 : pos;
  if (at < line.from || at >= line.to) return undefined;
  const word = wordRange(line.text, at - line.from);
  // wordRange falls back to the character before; under the pointer only.
  if (!word || at - line.from < word.start || at - line.from >= word.end) return undefined;
  return { from: line.from + word.start, to: line.from + word.end };
}

/**
 * Whether the editor still shows the text it showed now, in the same tab and
 * path. While it does, a synced live document holds exactly this text: every
 * edit goes to the draft (and its document) as it is made.
 */
function askedText(view: EditorView): () => boolean {
  const owner = owners.get(view), doc = view.state.doc;
  return () => {
    const now = owners.get(view);
    return Boolean(owner && now && now.id === owner.id && now.path === owner.path && now.version === owner.version
      && view.state.doc.eq(doc) && showsText(doc, now.text));
  };
}

/** F12 and Shift+F12 at the caret: the word around it, else the caret itself. */
function navigateAtCaret(op: 'definition' | 'references') {
  return (view: EditorView): boolean => {
    const symbols = owners.get(view)?.symbols;
    if (!symbols) return false;
    const head = view.state.selection.main.head;
    const line = view.state.doc.lineAt(head);
    const word = wordRange(line.text, head - line.from);
    const coords = view.coordsAtPos(head);
    symbols.ask(op, editorPosition(view.state, word ? line.from + word.start : head), { x: coords?.left ?? 0, y: coords?.bottom ?? 0 }, askedText(view));
    return true;
  };
}

export const navigationKeymap: readonly KeyBinding[] = [
  { key: 'F12', run: navigateAtCaret('definition'), preventDefault: true },
  { key: 'Shift-F12', run: navigateAtCaret('references'), preventDefault: true },
];

/** The word under viewport coordinates, judged as CodeMirror judges a hover. */
function wordAtCoords(view: EditorView, x: number, y: number): { from: number; to: number } | undefined {
  const pos = view.posAtCoords({ x, y });
  if (pos === null) return undefined;
  const coords = view.coordsAtPos(pos);
  if (!coords || y < coords.top || y > coords.bottom
    || x < coords.left - view.defaultCharacterWidth || x > coords.right + view.defaultCharacterWidth) return undefined;
  return wordAtSide(view.state, pos, x < coords.left ? -1 : 1);
}

/** Cmd (macOS) or Ctrl+click on a word goes to its definition instead of placing the caret. */
export function symbolMouseDown(event: MouseEvent, view: EditorView): boolean {
  const symbols = owners.get(view)?.symbols;
  if (!symbols || event.button !== 0 || !linkModifier(event, /Mac|iPhone|iPad/.test(navigator.platform))) return false;
  const word = wordAtCoords(view, event.clientX, event.clientY);
  if (!word) return false;
  event.preventDefault();
  symbols.ask('definition', editorPosition(view.state, word.from), { x: event.clientX, y: event.clientY }, askedText(view));
  return true;
}

// The selection as it was before a context-menu press: on macOS a
// right-click selects the word under the pointer before the menu opens.
const pressedSelections = new WeakMap<EditorView, SelectionRange>();

/** A right-click, or Ctrl+click on macOS, opens a context menu: remember the selection first. */
export function symbolPress(event: MouseEvent, view: EditorView): void {
  if (contextPress(event, /Mac|iPhone|iPad/.test(navigator.platform))) pressedSelections.set(view, view.state.selection.main);
  else pressedSelections.delete(view);
}

/** A menu from the keyboard has no press: forget one whose menu never came. */
export function symbolKeyDown(event: KeyboardEvent, view: EditorView): void {
  if (menuKey(event)) pressedSelections.delete(view);
}

/** Whether viewport coordinates fall on the selection range, not just next to it. */
function onSelection(view: EditorView, range: SelectionRange, x: number, y: number): boolean {
  const pos = view.posAtCoords({ x, y });
  const coords = pos === null ? null : view.coordsAtPos(pos);
  if (range.empty || pos === null || !coords || y < coords.top || y > coords.bottom || pos < range.from || pos > range.to) return false;
  return (pos !== range.from || x >= coords.left) && (pos !== range.to || x <= coords.left);
}

/**
 * On selected text, a menu with Copy only, keeping the selection; on a word,
 * the symbol menu. Elsewhere the native editing menu stays.
 */
export function symbolContextMenu(event: MouseEvent, view: EditorView): boolean {
  const symbols = owners.get(view)?.symbols;
  const at = { x: event.clientX, y: event.clientY };
  // Without a press, from the keyboard, the selection is the user's.
  const before = pressedSelections.get(view);
  const pressed = before ?? view.state.selection.main;
  pressedSelections.delete(view);
  // The editor may not have read the browser's word selection yet and would
  // later, so put back the DOM's selection as well as the editor's. Where a
  // right-click selects nothing, as on Windows and Linux, both are as they were.
  const putBack = () => {
    if (!before) return;
    const dom = view.dom.ownerDocument.getSelection();
    const at = (node: Node | null | undefined, offset: number) => node && view.contentDOM.contains(node) ? view.posAtDOM(node, offset) : -1;
    if (dom && (at(dom.anchorNode, dom.anchorOffset) !== before.anchor || at(dom.focusNode, dom.focusOffset) !== before.head)) {
      const anchor = view.domAtPos(before.anchor), head = view.domAtPos(before.head);
      dom.setBaseAndExtent(anchor.node, anchor.offset, head.node, head.offset);
    }
    if (!view.state.selection.main.eq(before)) view.dispatch({ selection: { anchor: before.anchor, head: before.head } });
  };
  if (symbols?.selectionMenu && onSelection(view, pressed, at.x, at.y)) {
    event.preventDefault();
    putBack();
    symbols.selectionMenu(view.state.sliceDoc(pressed.from, pressed.to), at, () => {
      view.dom.classList.add('cm-selection-held');
      return () => view.dom.classList.remove('cm-selection-held');
    });
    return true;
  }
  if (!symbols?.menu) return false;
  const word = wordAtCoords(view, at.x, at.y);
  if (!word) return false;
  event.preventDefault();
  // Drop the word the right-click selected, or a later right-click on it
  // would take it for the user's selection.
  putBack();
  symbols.menu(editorPosition(view.state, word.from), view.state.sliceDoc(word.from, word.to), at, askedText(view));
  return true;
}

/** The hover tooltip of the word under the pointer, from the language server. */
export async function symbolHover(view: EditorView, pos: number, side: -1 | 1): Promise<Tooltip | null> {
  const symbols = owners.get(view)?.symbols;
  const word = symbols && wordAtSide(view.state, pos, side);
  if (!symbols || !word) return null;
  const sameText = askedText(view), top = view.scrollDOM.scrollTop, left = view.scrollDOM.scrollLeft;
  const current = () => sameText() && view.scrollDOM.scrollTop === top && view.scrollDOM.scrollLeft === left;
  const hover = await symbols.hover(editorPosition(view.state, word.from), current);
  if (!hover || !current()) return null;
  return {
    pos: word.from, end: word.to, above: false,
    create() {
      const dom = document.createElement('div');
      dom.className = 'cm-symbol-hover';
      const body = dom.appendChild(document.createElement('div'));
      body.className = 'hover-body';
      // Markdown through the transcript renderer, or escaped plain text (hoverHtml).
      body.innerHTML = hover.html;
      if (hover.truncated) {
        const note = dom.appendChild(document.createElement('div'));
        note.className = 'hover-truncated';
        note.textContent = 'The description was cut short.';
      }
      dom.addEventListener('click', handleCopyCodeClick);
      return {
        dom,
        // At most 60% of the editor's height, as in the viewer; it scrolls.
        mount(view) { dom.style.maxHeight = `${Math.max(80, view.scrollDOM.clientHeight * 0.6)}px`; },
      };
    },
  };
}

const navigation: Extension = [
  keymap.of(navigationKeymap),
  EditorView.domEventHandlers({
    contextmenu: symbolContextMenu,
    mousedown(event, view) {
      symbolPress(event, view);
      if (hasHoverTooltips(view.state)) view.dispatch({ effects: closeHoverTooltips });
      return symbolMouseDown(event, view);
    },
    scroll(_event, view) {
      if (hasHoverTooltips(view.state)) view.dispatch({ effects: closeHoverTooltips });
      return false;
    },
  }),
  // An observer: it must see keys the keymap handles too.
  EditorView.domEventObservers({ keydown: symbolKeyDown }),
  hoverTooltip(symbolHover, { hoverTime: HOVER_DELAY, hideOnChange: true }),
];

function freshState(options: FileEditorOptions): EditorState {
  return EditorState.create({
    doc: options.text,
    selection: options.selection ? lineSelection(options.text, options.selection) : undefined,
    extensions: [
      lineNumbers(), history(), drawSelection(), highlightActiveLine(), indentOnInput(),
      diagnosticLines, clearSavedMarkers, navigation,
      search(), theme, font.of(fontThemes[0]), fitTooltips, indentUnit.of(detectIndent(options.text)), EditorState.tabSize.of(4),
      wrapping.of(options.wrap ? EditorView.lineWrapping : []),
      editing.of(editable(options.disabled)), language.of([]), coloring.of([]), completing.of(suggestions(options)),
      mirrors.of([]), endMirrors,
      keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap, ...searchKeymap, ...editorLintKeymap]),
      EditorView.updateListener.of(update => {
        if (update.docChanged && !update.transactions.every(tr => tr.annotation(external)))
          owners.get(update.view)?.onchange(docText(update.state.doc));
        if (update.docChanged || update.transactions.some(tr => tr.effects.some(e => e.is(setDiagnosticsEffect)))) reportProblems(update.view);
      }),
    ],
  });
}

/**
 * The state with its document changed to text as one undoable step of its
 * own, so a replace in files keeps the history before it and undo takes
 * back exactly the replace.
 */
function adoption(state: EditorState, text: string): TransactionSpec {
  return { changes: textChanges(docText(state.doc), lf(text)), annotations: [external.of(true), isolateHistory.of('full')] };
}

function adopt(state: EditorState, text: string): EditorState {
  return showsText(state.doc, text) ? state : state.update(adoption(state, text)).state;
}

// The document as CodeMirror holds it: every line break is \n.
const lf = (text: string) => text.replace(/\r\n?/g, '\n');

// Documents are immutable: build each one's string once. On a large file each
// build or compare costs milliseconds, and a keystroke asked for several.
const docStrings = new WeakMap<Text, string>();
function docText(doc: Text): string {
  let text = docStrings.get(doc);
  if (text === undefined) docStrings.set(doc, text = doc.toString());
  return text;
}

/** Whether text is the document, with its line breaks as CodeMirror holds them. */
function showsText(doc: Text, text: string, normalize = lf): boolean {
  const own = docText(doc);
  // The draft is usually the very string the editor reported: no scan then.
  return text === own || normalize(text) === own;
}

function handleKey(view: EditorView, event: KeyboardEvent): boolean {
  // A parent window handler can see an already-consumed event.
  if (keyHandled(event)) return true;
  const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? event.metaKey : event.ctrlKey;
  const key = event.key.toLowerCase();
  let consumed = false;
  if (mod && !event.altKey && !event.shiftKey && key === 's') {
    owners.get(view)?.onsave();
    consumed = true;
  } else if (mod && !event.altKey && !event.shiftKey && key === 'f') {
    openSearchPanel(view);
    consumed = true;
  } else if (event.key === 'Escape' && closeCompletion(view)) {
    // Suggestions close first: the hover, the search panel and the dialog stay.
    consumed = true;
  } else if (event.key === 'Escape' && hasHoverTooltips(view.state)) {
    // A hover (a symbol's, or a diagnostic's) closes before the search panel or the dialog.
    view.dispatch({ effects: closeHoverTooltips });
    consumed = true;
  } else if (event.key === 'Escape' && searchPanelOpen(view.state)) {
    closeSearchPanel(view);
    consumed = true;
  }
  if (consumed) {
    event.preventDefault();
    event.stopPropagation();
  }
  return consumed;
}

// Picks the editor's language by file name from @codemirror/language-data,
// which loads each language only when a file needs it. Svelte is not in that
// list, so it has its own package. Names the list does not know (.zshrc,
// Brewfile) fall back to the viewer's highlight.js language, looked up by name.
async function loadLanguage(path: string): Promise<Extension> {
  const name = path.slice(path.lastIndexOf('/') + 1);
  if (/\.svelte$/i.test(name)) return (await import('@replit/codemirror-lang-svelte')).svelte();
  const { languages } = await import('@codemirror/language-data');
  const hljsName = fileLanguage(path);
  const description =
    LanguageDescription.matchFilename(languages, name) ??
    (hljsName ? LanguageDescription.matchLanguageName(languages, hljsName, false) : null);
  return description ? await description.load() : [];
}

export function createFileEditor(parent: HTMLElement, initial: FileEditorOptions) {
  let options = initial;
  let destroyed = false;
  let languageRequest = 0;
  let scrollRequest = 0;
  const cached = tabs.get(options.id);
  // The same version is the same history: a draft changed meanwhile (a
  // replace in files) is taken in as one undoable change.
  const reuse = cached?.version === options.version;
  const view = new EditorView({ parent, state: withDiagnostics(reuse ? adopt(cached.state, options.text) : freshState(options), options) });
  owners.set(view, options);
  // Capture also covers the search panel: its own keymap otherwise closes the
  // panel before a bubbling parent handler can recognize and consume Escape.
  const onKeyDown = (event: KeyboardEvent) => { handleKey(view, event); };
  view.dom.addEventListener('keydown', onKeyDown, true);

  function configure() {
    view.dispatch({ effects: [
      wrapping.reconfigure(options.wrap ? EditorView.lineWrapping : []),
      editing.reconfigure(editable(options.disabled)),
      completing.reconfigure(suggestions(options)),
    ] });
  }
  function setLanguage() {
    const request = ++languageRequest;
    const path = options.path;
    void loadFileHighlighter().then(hljs => {
      if (!destroyed && request === languageRequest) {
        view.dispatch({ effects: coloring.reconfigure(fileHighlight(hljs, path)) });
      }
    }).catch(() => { /* Keep plain text if the lazy highlighter cannot load. */ });
    void loadLanguage(path).catch((): Extension => []).then(extension => {
      if (!destroyed && request === languageRequest) view.dispatch({ effects: language.reconfigure(extension) });
    });
  }
  function restore(top: number, left: number) {
    const request = ++scrollRequest;
    view.scrollDOM.scrollTop = top;
    view.scrollDOM.scrollLeft = left;
    view.requestMeasure({ read: () => null, write() {
      if (!destroyed && request === scrollRequest) {
        view.scrollDOM.scrollTop = top;
        view.scrollDOM.scrollLeft = left;
      }
    } });
  }
  function position(): FilePosition {
    const top = view.scrollDOM.scrollTop;
    const block = view.lineBlockAtHeight(Math.max(0, top - view.documentPadding.top));
    return {
      line: view.state.doc.lineAt(block.from).number,
      offset: block.top + view.documentPadding.top - top,
      left: view.scrollDOM.scrollLeft,
    };
  }
  function restorePosition(target: FilePosition) {
    const request = ++scrollRequest;
    const line = view.state.doc.line(Math.max(1, Math.min(target.line, view.state.doc.lines)));
    // Scroll to the logical line first so CodeMirror renders and measures it,
    // including wrapped lines outside the initial virtual viewport.
    view.dispatch({ effects: EditorView.scrollIntoView(line.from, { y: 'start', yMargin: 0 }) });
    // Flush CodeMirror's pending scroll before applying the exact viewer
    // offset. It processes scrollIntoView AFTER requestMeasure writes, so
    // scheduling both together lets its scroll overwrite our restoration.
    view.lineBlockAtHeight(0);
    view.requestMeasure({
      read: () => view.lineBlockAt(line.from).top + view.documentPadding.top - target.offset,
      write(top) {
        if (!destroyed && request === scrollRequest) {
          view.scrollDOM.scrollTop = top;
          view.scrollDOM.scrollLeft = target.left;
        }
      },
    });
  }
  function reportPosition() { options.onposition?.(position()); }
  view.scrollDOM.addEventListener('scroll', reportPosition);
  function remember() {
    reportPosition();
    const range = view.state.selection.main;
    options.onselection?.({ anchor: view.state.doc.lineAt(range.anchor).number, head: view.state.doc.lineAt(range.head).number });
    tabs.set(options.id, { state: view.state, version: options.version, top: view.scrollDOM.scrollTop, left: view.scrollDOM.scrollLeft });
  }
  if (reuse && options.selection) view.dispatch({ selection: lineSelection(options.text, options.selection) });
  configure();
  setLanguage();
  if (options.position) restorePosition(options.position);
  else if (reuse) restore(cached.top, cached.left);
  reportProblems(view);

  return {
    update(next: FileEditorOptions) {
      if (destroyed) return;
      const changedTab = next.id !== options.id;
      const reset = changedTab || next.version !== options.version;
      const adopted = !reset && !showsText(view.state.doc, next.text);
      const changedPath = next.path !== options.path;
      if (changedTab) remember();
      options = next;
      owners.set(view, options);
      if (reset) {
        const saved = changedTab ? tabs.get(options.id) : undefined;
        const valid = saved?.version === options.version;
        view.setState(withDiagnostics(valid ? adopt(saved.state, options.text) : freshState(options), options));
        if (options.position) restorePosition(options.position);
        else restore(valid ? saved.top : 0, valid ? saved.left : 0);
      } else if (adopted) view.dispatch(adoption(view.state, options.text));
      configure();
      view.dispatch(setDiagnostics(view.state, editorDiagnostics(view.state, options)));
      if (reset || changedPath) setLanguage();
      reportProblems(view);
    },
    /** The element that scrolls the text. */
    scrollDOM: view.scrollDOM,
    /** Where a 0-based line starts, in px from the top of the scrolled text; estimated outside the viewport. */
    lineTop(line: number): number | undefined {
      if (destroyed || line < 0 || line >= view.state.doc.lines) return undefined;
      return view.lineBlockAt(view.state.doc.line(line + 1).from).top + view.documentPadding.top;
    },
    revealRange(range: CodeDiagnosticRange): boolean {
      if (destroyed) return false;
      const selection = rangeOffsets(docText(view.state.doc), range);
      if (!selection) return false;
      ++scrollRequest; // Supersede pending scroll restoration, not document/history.
      view.dispatch({ selection, effects: EditorView.scrollIntoView(selection.anchor, { y: 'center' }) });
      return true;
    },
    focus() { if (!destroyed) view.focus(); },
    /** Where the selection starts, for navigation history. */
    caret(): CodeDiagnosticPosition | undefined {
      return destroyed ? undefined : editorPosition(view.state, view.state.selection.main.from);
    },
    selectedText() {
      if (destroyed) return '';
      const { from, to } = view.state.selection.main;
      return view.state.sliceDoc(from, to);
    },
    find() { return !destroyed && openSearchPanel(view); },
    closeSearch() { return !destroyed && closeSearchPanel(view); },
    handleKey(event: KeyboardEvent) { return !destroyed && handleKey(view, event); },
    /** Measures the text again after the editor font or size changed. */
    remeasure() {
      if (destroyed) return;
      const current = font.get(view.state);
      view.dispatch({ effects: font.reconfigure(fontThemes[current === fontThemes[0] ? 1 : 0]) });
    },
    destroy() {
      if (destroyed) return;
      remember();
      destroyed = true;
      languageRequest++;
      owners.delete(view);
      view.dom.removeEventListener('keydown', onKeyDown, true);
      view.scrollDOM.removeEventListener('scroll', reportPosition);
      view.destroy();
    },
  };
}

export type FileEditorController = ReturnType<typeof createFileEditor>;
