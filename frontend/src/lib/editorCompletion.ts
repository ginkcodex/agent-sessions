import type { Text } from '@codemirror/state';
import type { CodeCompletionItem, CodeCompletionTrigger, CodeDiagnosticPosition, CodeDiagnosticRange } from './codeDiagnostics';

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
  order.sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : a.index - b.index));
  const boosts = new Array<number>(items.length);
  order.forEach(({ index }, rank) => { boosts[index] = 99 - Math.floor(rank * 98 / Math.max(1, items.length)); });
  return boosts;
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

/**
 * Plain text for an LSP snippet: placeholders keep their default text (the
 * first choice), variables theirs, and the caret goes to the first tab stop
 * ($1, else $0, else the end). Snippets with tab stops come in CC-07.
 */
export function snippetText(snippet: string): { text: string; caret: number } {
  let out = '', i = 0;
  const stops: { n: number; at: number }[] = [];
  const parse = (until?: string): void => {
    while (i < snippet.length) {
      const c = snippet[i];
      if (until && until.includes(c)) return;
      if (c === '\\' && i + 1 < snippet.length && '$}\\,|'.includes(snippet[i + 1])) { out += snippet[i + 1]; i += 2; continue; }
      if (c !== '$') { out += c; i++; continue; }
      const simple = /^\$(\d+|[A-Za-z_]\w*)/.exec(snippet.slice(i));
      if (simple) {
        if (/^\d/.test(simple[1])) stops.push({ n: Number(simple[1]), at: out.length });
        i += simple[0].length;
        continue;
      }
      const open = /^\$\{(\d+|[A-Za-z_]\w*)/.exec(snippet.slice(i));
      if (!open) { out += c; i++; continue; }
      i += open[0].length;
      if (/^\d/.test(open[1])) stops.push({ n: Number(open[1]), at: out.length });
      if (snippet[i] === ':') { i++; parse('}'); }
      else if (snippet[i] === '|') {
        const choices = /^\|((?:[^|\\]|\\.)*)\|/.exec(snippet.slice(i));
        if (choices) { out += choices[1].split(/(?<!\\),/)[0].replace(/\\(.)/g, '$1'); i += choices[0].length; }
      } else if (snippet[i] === '/') {
        // A transform: the variable's value is not known here.
        const end = snippet.indexOf('}', i);
        i = end < 0 ? snippet.length : end;
      }
      if (snippet[i] === '}') i++;
    }
  };
  parse();
  const first = stops.filter(s => s.n > 0).sort((a, b) => a.n - b.n)[0] ?? stops.find(s => s.n === 0);
  return { text: out, caret: first?.at ?? out.length };
}

export interface CompletionEdit { from: number; to: number; insert: string }

/**
 * Edits for accepting `item` from an answer asked at `asked` (the caret) in
 * `askedDoc`, whose filter text started at `start`. `from` and `to` are where
 * that filter text is now in `doc`. When nothing else changed since (the text
 * before `from` and after `to` is as it was), the server's edit and its extra
 * edits (imports) apply as asked; otherwise only the item's text replaces the
 * filter text (the label, when the edit was for other text), as no other
 * offset can be trusted. Changes are sorted and do
 * not overlap; `caret` is where the caret goes, counted from the main edit's
 * start, and `main` is the main edit's index.
 */
export function completionChanges(askedDoc: Text, asked: number, start: number, doc: Text, from: number, to: number, item: CodeCompletionItem): { changes: CompletionEdit[]; main: number; caret: number } {
  const plain = item.snippet ? snippetText(item.edit?.newText ?? item.insertText ?? item.label) : undefined;
  const insert = plain?.text ?? item.edit?.newText ?? item.insertText ?? item.label;
  const caret = plain ? plain.caret : insert.length;
  const range = item.edit && rangeOffsets(askedDoc, item.edit.range);
  // Without the edit's own range, its text fits the filter text only when it replaced exactly that.
  const fits = !item.edit || (range !== undefined && range[0] === start && range[1] === asked);
  const typedOnly = { from, to, insert: fits ? insert : item.label };
  const typedCaret = fits ? caret : item.label.length;
  const clean = start <= asked && from <= to && askedDoc.slice(0, start).eq(doc.slice(0, from)) && askedDoc.slice(asked).eq(doc.slice(to));
  if (!clean) return { changes: [typedOnly], main: 0, caret: typedCaret };
  // Asked offsets to now: up to the filter text as they were, after the caret shifted with it.
  const map = (x: number) => (x <= start ? x : x >= asked ? x + (to - asked) : undefined);
  let main = typedOnly, mainCaret = typedCaret;
  if (range) {
    const ms = range[0] === start ? from : map(range[0]), me = range[1] === asked ? to : map(range[1]);
    // An edit starting or ending inside the typed text is the server's guess at text it never saw.
    if (ms !== undefined && me !== undefined && ms <= from && me >= to) { main = { from: ms, to: me, insert }; mainCaret = caret; }
  }
  const changes = [main];
  for (const extra of item.extraEdits ?? []) {
    const r = rangeOffsets(askedDoc, extra.range);
    const ms = r && map(r[0]), me = r && map(r[1]);
    if (ms === undefined || me === undefined) continue;
    // Extra edits are elsewhere; one touching another edit is dropped.
    if (changes.some(c => (ms < c.to && me > c.from) || (ms === me && ms >= c.from && ms <= c.to) || (c.from === c.to && c.from >= ms && c.from <= me))) continue;
    changes.push({ from: ms, to: me, insert: extra.newText });
  }
  changes.sort((a, b) => a.from - b.from);
  return { changes, main: changes.indexOf(main), caret: mainCaret };
}

/** Where the caret goes once `changes` (sorted) are applied: `caret` past the start of change `main`. */
export function caretAfter(changes: readonly CompletionEdit[], main: number, caret: number): number {
  let shift = 0;
  for (let i = 0; i < main; i++) shift += changes[i].insert.length - (changes[i].to - changes[i].from);
  return changes[main].from + shift + caret;
}
