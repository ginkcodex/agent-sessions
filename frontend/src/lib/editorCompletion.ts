import type { Text } from '@codemirror/state';
import type { CodeCompletionItem, CodeCompletionTrigger, CodeDiagnosticPosition, CodeDiagnosticRange, CodeTextEdit } from './codeDiagnostics';

/**
 * Language-server completion for the file editor (CC-06): what the editor
 * gets from its owner, and the pure parts of turning an answer into
 * CodeMirror suggestions and edits. Nothing here holds a store.
 */

/** An item's detail and documentation for the info panel; `html` is rendered Markdown or escaped text. */
export interface EditorCompletionDetails { detail?: string; html?: string; truncated?: boolean }

/** A language server's answer, as the editor's owner hands it over. */
export interface EditorCompletionAnswer {
  items: CodeCompletionItem[];
  incomplete: boolean;
  /** The server's trigger characters, when the answer says. */
  triggerCharacters?: string;
  /** Item `index`'s details; undefined shows none. */
  details?(index: number): Promise<EditorCompletionDetails | undefined>;
  /**
   * Item `index`'s extra edits (imports) that its server gives only on
   * resolve (CC-12), in the offsets of the text the editor shows when asked;
   * `current` says it still does. An array, at once, when they are already
   * known (empty for none); else a promise, undefined when they cannot be had
   * for that text. Absent when the answer cannot be resolved.
   */
  resolveEdits?(index: number, current: () => boolean, signal: AbortSignal): CodeTextEdit[] | Promise<CodeTextEdit[] | undefined>;
}

/** Asks for completions; undefined, without a promise, when no server can answer. */
export type EditorCompleter = (position: CodeDiagnosticPosition, trigger: CodeCompletionTrigger, current: () => boolean, signal: AbortSignal) => Promise<EditorCompletionAnswer | undefined> | undefined;

/**
 * Characters asked after before the server's own are known: those that
 * gopls, vtsls, Pyright, ruby-lsp and the others use. The host answers
 * one its server did not name with nothing.
 */
export const LIKELY_TRIGGERS = '.:>/@<#"\'`$(';

/** An identifier being typed, with a leading sigil (`@ivar`, `$var`, `#private`). */
export const IDENTIFIER = /^[@$#]?[\p{L}\p{N}_$]*$/u;
const IDENTIFIER_END = /[\p{L}\p{N}_$]*$/u;

/** CodeMirror icon types for LSP CompletionItemKind 1-25. */
const KIND_TYPES: Record<number, string> = {
  1: 'text', 2: 'method', 3: 'function', 4: 'function', 5: 'property', 6: 'variable', 7: 'class', 8: 'interface',
  9: 'namespace', 10: 'property', 11: 'constant', 12: 'constant', 13: 'enum', 14: 'keyword', 15: 'text', 16: 'constant',
  17: 'text', 18: 'variable', 19: 'text', 20: 'enum', 21: 'constant', 22: 'class', 23: 'property', 24: 'keyword', 25: 'type',
};
export const completionType = (kind?: number): string | undefined => (kind ? KIND_TYPES[kind] : undefined);

/** What is shown after the label: its details, else the description or the detail. */
export function completionDetail(item: CodeCompletionItem): string | undefined {
  const text = item.labelDetail
    ? item.labelDetail + (item.labelDescription ? ` ${item.labelDescription}` : '')
    : item.labelDescription || item.detail;
  if (!text) return undefined;
  const line = text.replace(/\s+/g, ' ').trim();
  return line.length > 120 ? line.slice(0, 119) + '…' : line;
}

/**
 * Boosts that keep the server's order (sortText, then its own) and put
 * every server item above the editor's words: 1 to 99, highest first.
 */
export function serverBoosts(items: readonly CodeCompletionItem[]): number[] {
  const order = items.map((item, index) => ({ key: item.sortText ?? item.label, index }));
  order.sort((a, b) => compareKeys(a.key, b.key) || a.index - b.index);
  const boosts = new Array<number>(items.length);
  order.forEach(({ index }, rank) => { boosts[index] = 99 - Math.floor(rank * 98 / Math.max(1, items.length)); });
  return boosts;
}

/** Orders strings by code unit, as servers' sortText is meant to be read. */
function compareKeys(a: string, b: string): number {
  if (a < b) return -1;
  return a > b ? 1 : 0;
}

/** The document offset of an LSP position; undefined when it is not inside the document. */
export function positionOffset(doc: Text, p: CodeDiagnosticPosition): number | undefined {
  if (p.line < 0 || p.line >= doc.lines || p.character < 0) return undefined;
  const line = doc.line(p.line + 1);
  return p.character <= line.length ? line.from + p.character : undefined;
}

function rangeOffsets(doc: Text, r: CodeDiagnosticRange): [number, number] | undefined {
  const from = positionOffset(doc, r.start), to = positionOffset(doc, r.end);
  return from !== undefined && to !== undefined && from <= to ? [from, to] : undefined;
}

/** Where the identifier before pos starts. */
export function identifierStart(doc: Text, pos: number): number {
  const line = doc.lineAt(pos);
  return pos - IDENTIFIER_END.exec(line.text.slice(0, pos - line.from))![0].length;
}

/**
 * Where the suggestions' filter text starts: the start all main edits share,
 * when it is on the asked line before the caret, else the identifier's start.
 */
export function answerStart(doc: Text, pos: number, items: readonly CodeCompletionItem[]): number {
  const line = doc.lineAt(pos);
  let shared: number | undefined;
  for (const item of items) {
    if (!item.edit) continue;
    const start = positionOffset(doc, item.edit.range.start);
    if (start === undefined || (shared !== undefined && start !== shared)) return identifierStart(doc, pos);
    shared = start;
  }
  return shared !== undefined && shared >= line.from && shared <= pos ? shared : identifierStart(doc, pos);
}

/** A parsed LSP snippet: text, and tab stops holding their default parts. */
type SnippetPart = string | { stop: number; parts: SnippetPart[] };

/**
 * Parses LSP snippet syntax leniently: `$1`, `${1}`, `${1:default}` (nested),
 * `${1|a,b|}` (the first choice), variables (`$NAME`, `${NAME:default}`:
 * their default, as their value is not known here), escapes (`\$`, `\}`,
 * `\\`). A transform inserts nothing; anything that does not parse is text.
 */
function parseSnippet(snippet: string): SnippetPart[] {
  let i = 0;
  const parse = (inside: boolean): SnippetPart[] => {
    const parts: SnippetPart[] = [];
    let text = '';
    const flush = () => { if (text) { parts.push(text); text = ''; } };
    while (i < snippet.length) {
      const c = snippet[i];
      if (inside && c === '}') break;
      if (c === '\\' && i + 1 < snippet.length && '$}\\,|'.includes(snippet[i + 1])) { text += snippet[i + 1]; i += 2; continue; }
      if (c !== '$') { text += c; i++; continue; }
      const simple = /^\$(\d+|[A-Za-z_]\w*)/.exec(snippet.slice(i));
      if (simple) {
        i += simple[0].length;
        if (/^\d/.test(simple[1])) { flush(); parts.push({ stop: Number(simple[1]), parts: [] }); }
        continue;
      }
      const open = /^\$\{(\d+|[A-Za-z_]\w*)/.exec(snippet.slice(i));
      // `${name` not followed by what can come next is text.
      if (!open || i + open[0].length >= snippet.length || !':|/}'.includes(snippet[i + open[0].length])) { text += c; i++; continue; }
      i += open[0].length;
      let inner: SnippetPart[] = [], transform = false;
      if (snippet[i] === ':') { i++; inner = parse(true); }
      else if (snippet[i] === '|') {
        const choices = /^\|((?:[^|\\]|\\.)*)\|/.exec(snippet.slice(i));
        if (choices) {
          const first = choices[1].split(/(?<!\\),/)[0].replace(/\\(.)/g, '$1');
          if (first) inner = [first];
          i += choices[0].length;
        }
      } else if (snippet[i] === '/') {
        // A transform: the text it would change is not known here.
        transform = true;
        const end = snippet.indexOf('}', i);
        i = end < 0 ? snippet.length : end;
      }
      if (snippet[i] === '}') i++;
      flush();
      if (transform) continue;
      if (/^\d/.test(open[1])) parts.push({ stop: Number(open[1]), parts: inner });
      else parts.push(...inner);
    }
    flush();
    return parts;
  };
  return parse(false);
}

const partsText = (parts: readonly SnippetPart[]): string => parts.map(p => (typeof p === 'string' ? p : partsText(p.parts))).join('');

/**
 * Plain text for an LSP snippet: placeholders keep their default text, and
 * the caret goes to the first tab stop ($1, else $0, else the end).
 */
export function snippetText(snippet: string): { text: string; caret: number } {
  let out = '';
  const stops: { n: number; at: number }[] = [];
  const walk = (parts: readonly SnippetPart[]): void => {
    for (const p of parts) {
      if (typeof p === 'string') { out += p; continue; }
      stops.push({ n: p.stop, at: out.length });
      walk(p.parts);
    }
  };
  walk(parseSnippet(snippet));
  const first = stops.filter(s => s.n > 0).sort((a, b) => a.n - b.n)[0] ?? stops.find(s => s.n === 0);
  return { text: out, caret: first?.at ?? out.length };
}

/**
 * A CodeMirror `snippet()` template for an LSP snippet: tab stops become
 * fields, with their default text when it is one line without braces (a
 * CodeMirror field cannot hold those; the stop then sits before its text,
 * and nested stops keep their own fields). The last stop is `$0`, at the end
 * when the snippet has none, so Tab ends there as in VS Code.
 *
 * CodeMirror reads `${` and `#{` as fields and drops a backslash before a
 * brace, so a brace after `$`, `#` or `\` is escaped, and only then. Its
 * escapes shift the fields after them wrongly from the third on one line;
 * such a snippet has no template (undefined) and goes in as plain text.
 */
export function snippetTemplate(snippet: string): string | undefined {
  // A field's text, when CodeMirror can hold it: one line, no braces, no nested stops,
  // and no backslash at the end, which would escape a brace after it.
  const fieldText = (parts: readonly SnippetPart[]) => {
    const flat = parts.every(q => typeof q === 'string') ? partsText(parts) : undefined;
    return flat !== undefined && !/[{}\r\n]|\\$/.test(flat) ? flat : undefined;
  };
  // Mirrors (`$1` again) show the text of the stop's placeholder.
  const defaults = new Map<number, string>();
  const collect = (parts: readonly SnippetPart[]): void => {
    for (const p of parts) {
      if (typeof p === 'string') continue;
      const text = fieldText(p.parts);
      if (text && !defaults.has(p.stop)) defaults.set(p.stop, text);
      collect(p.parts);
    }
  };
  const parts = parseSnippet(snippet);
  collect(parts);
  // The text's last character so far, and the escapes on the line so far.
  let prev = '', escapes = 0, final = false, safe = true;
  const literal = (text: string): string => {
    let out = '';
    for (const c of text) {
      if (c === '\n' || c === '\r') escapes = 0;
      if ((c === '{' && (prev === '$' || prev === '#')) || ((c === '{' || c === '}') && prev === '\\')) { out += '\\'; escapes++; }
      out += c;
      prev = c;
    }
    return out;
  };
  const field = (stop: number, text: string): string => {
    if (escapes > 2) safe = false;
    if (text) prev = text[text.length - 1];
    return text ? `\${${stop}:${text}}` : `\${${stop}}`;
  };
  const render = (parts: readonly SnippetPart[]): string => parts.map(p => {
    if (typeof p === 'string') return literal(p);
    if (p.stop === 0) final = true;
    const text = p.parts.length ? fieldText(p.parts) : defaults.get(p.stop) ?? '';
    return text !== undefined ? field(p.stop, text) : field(p.stop, '') + render(p.parts);
  }).join('');
  let template = render(parts);
  if (!final) template += field(0, '');
  return safe ? template : undefined;
}

export interface CompletionEdit { from: number; to: number; insert: string }

/** What accepting an item changes: see completionChanges. */
export interface CompletionAccept { changes: CompletionEdit[]; main: number; caret: number; template?: string }

/** Whether from..to touches one of changes: overlaps it, is an insert at or in it, or holds its insert. */
function touches(changes: readonly CompletionEdit[], from: number, to: number): boolean {
  return changes.some(c => (from < c.to && to > c.from) || (from === to && from >= c.from && from <= c.to) || (c.from === c.to && c.from >= from && c.from <= to));
}

/**
 * Edits for accepting `item` from an answer asked at `asked` (the caret) in
 * `askedDoc`, whose filter text started at `start`. `from` and `to` are where
 * that filter text is now in `doc`. When nothing else changed since (the text
 * before `from` and after `to` is as it was), the server's edit and its extra
 * edits (imports) apply as asked; otherwise only the item's text replaces the
 * filter text (the label, when the edit was for other text), as no other
 * offset can be trusted. Changes are sorted and do
 * not overlap; `caret` is where the caret goes, counted from the main edit's
 * start, and `main` is the main edit's index. A snippet item's main edit
 * inserts its plain text; `template` is then its `snippet()` template, for
 * the editor to insert in its place with tab stops.
 */
export function completionChanges(askedDoc: Text, asked: number, start: number, doc: Text, from: number, to: number, item: CodeCompletionItem): CompletionAccept {
  const source = item.edit?.newText ?? item.insertText ?? item.label;
  const plain = item.snippet ? snippetText(source) : undefined;
  const insert = plain?.text ?? source;
  const caret = plain ? plain.caret : insert.length;
  const range = item.edit && rangeOffsets(askedDoc, item.edit.range);
  // Without the edit's own range, its text fits the filter text only when it replaced exactly that.
  const fits = !item.edit || (range !== undefined && range[0] === start && range[1] === asked);
  const typedOnly = { from, to, insert: fits ? insert : item.label };
  const typedCaret = fits ? caret : item.label.length;
  const clean = start <= asked && from <= to && askedDoc.slice(0, start).eq(doc.slice(0, from)) && askedDoc.slice(asked).eq(doc.slice(to));
  // The label, when the item's own text did not fit, is no snippet.
  const template = (main: CompletionEdit) => {
    const t = item.snippet && (main !== typedOnly || fits) ? snippetTemplate(source) : undefined;
    return t === undefined ? {} : { template: t };
  };
  if (!clean) return { changes: [typedOnly], main: 0, caret: typedCaret, ...template(typedOnly) };
  // Asked offsets to now: up to the filter text as they were, after the caret shifted with it.
  const map = (x: number) => (x <= start ? x : x >= asked ? x + (to - asked) : undefined);
  let main = typedOnly, mainCaret = typedCaret;
  if (range) {
    const ms = range[0] === start ? from : map(range[0]), me = range[1] === asked ? to : map(range[1]);
    // An edit starting or ending inside the typed text is the server's guess at text it never saw.
    if (ms !== undefined && me !== undefined && ms <= from && me >= to) { main = { from: ms, to: me, insert }; mainCaret = caret; }
  }
  const changes = withEdits([main], (item.extraEdits ?? []).map(extra => {
    const r = rangeOffsets(askedDoc, extra.range);
    const ms = r && map(r[0]), me = r && map(r[1]);
    return ms === undefined || me === undefined ? undefined : { from: ms, to: me, insert: extra.newText };
  }));
  return { changes, main: changes.indexOf(main), caret: mainCaret, ...template(main) };
}

/**
 * changes with extra, sorted. Extra edits are elsewhere, and part of an import
 * can break the code: when one is undefined (it does not fit the text) or
 * touches another edit, none is added.
 */
function withEdits(changes: readonly CompletionEdit[], extra: readonly (CompletionEdit | undefined)[]): CompletionEdit[] {
  const out = [...changes];
  for (const e of extra) {
    if (!e || touches(out, e.from, e.to)) return [...changes];
    out.push(e);
  }
  return out.sort((a, b) => a.from - b.from);
}

/**
 * accept with the extra edits an item resolved to (CC-12), given in the
 * offsets of doc, the text accept applies to. As with listed ones, all or
 * none: an edit outside doc, or touching the main edit or another, drops them.
 */
export function withExtraEdits(accept: CompletionAccept, doc: Text, edits: readonly CodeTextEdit[]): CompletionAccept {
  const main = accept.changes[accept.main];
  const changes = withEdits(accept.changes, edits.map(extra => {
    const r = rangeOffsets(doc, extra.range);
    return r && { from: r[0], to: r[1], insert: extra.newText };
  }));
  return { ...accept, changes, main: changes.indexOf(main) };
}

/** Where the caret goes once `changes` (sorted) are applied: `caret` past the start of change `main`. */
export function caretAfter(changes: readonly CompletionEdit[], main: number, caret: number): number {
  let shift = 0;
  for (let i = 0; i < main; i++) shift += changes[i].insert.length - (changes[i].to - changes[i].from);
  return changes[main].from + shift + caret;
}
