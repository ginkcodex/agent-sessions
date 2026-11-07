import type { CodeDiagnosticPosition, CodeDiagnosticRange, CodeHoverText, CodeNavigationLocation } from './codeDiagnostics';
import { renderMarkdown } from './markdown';
import { navigationShortcut, type NavDirection } from './navHistory';

export { SOURCE_HISTORY_LIMIT, SourceHistory, type EditorPlace } from './editorHistory';

/**
 * Code navigation in the file viewer: where the pointer is in the source,
 * the word it is over, the way back, and how an answer reads.
 */

/** A place in a file: a project path, or an absolute host path outside it. */
export interface SourceLocation { path: string; external?: boolean; range: CodeDiagnosticRange }


/** A point in the viewport, where a popup about a symbol opens. */
export interface ViewportPoint { x: number; y: number }

/** What the source view reports about words, for code navigation. */
export interface SourceSymbols {
  /** Words underline under the link modifier only when navigation can be asked. */
  linked: boolean;
  /** Cmd/Ctrl+click on a word; position is the word's start. */
  definition(position: CodeDiagnosticPosition, at: ViewportPoint): void;
  /** The pointer rested on a word. */
  hover(position: CodeDiagnosticPosition, at: ViewportPoint): void;
  /** The pointer left the word, the view scrolled or a button went down. */
  hoverEnd(): void;
  /** A context menu on a word. */
  menu(position: CodeDiagnosticPosition, word: string, at: ViewportPoint): void;
  /** A context menu on selected text; see SelectionHold. */
  selectionMenu(text: string, at: ViewportPoint, hold: SelectionHold): void;
}

/**
 * Runs once the menu has taken focus, which can clear or dim a selection, to
 * keep it as it was; what it returns runs when the menu closes.
 */
export type SelectionHold = () => (() => void) | void;

export const HOVER_DELAY = 400;

/** The word (identifier) around a UTF-16 column, or undefined off a word. */
export function wordRange(line: string, character: number): { start: number; end: number } | undefined {
  const word = /[\p{L}\p{N}_$]/u;
  const at = (i: number) => i >= 0 && i < line.length && word.test(line[i]);
  // The character under the pointer, or the one before it at a word's end.
  const i = at(character) ? character : at(character - 1) ? character - 1 : -1;
  if (i < 0) return undefined;
  let start = i, end = i + 1;
  while (at(start - 1)) start--;
  while (at(end)) end++;
  // A number is a literal, with nothing to go to.
  if (/^\p{N}+$/u.test(line.slice(start, end))) return undefined;
  return { start, end };
}

/** The DOM point under viewport coordinates, in WebKit and Chromium alike. */
export function caretPoint(doc: Document, x: number, y: number): { node: Node; offset: number } | undefined {
  const d = doc as Document & {
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null;
    caretRangeFromPoint?: (x: number, y: number) => Range | null;
  };
  if (typeof d.caretPositionFromPoint === 'function') {
    const p = d.caretPositionFromPoint(x, y);
    return p ? { node: p.offsetNode, offset: p.offset } : undefined;
  }
  if (typeof d.caretRangeFromPoint === 'function') {
    const r = d.caretRangeFromPoint(x, y);
    return r ? { node: r.startContainer, offset: r.startOffset } : undefined;
  }
  return undefined;
}

/**
 * The source position of a DOM point inside root: the line from the nearest
 * `[data-source-line]`, the UTF-16 column from the text before the point.
 * Syntax markup adds no characters, so it never shifts the column. Removed
 * diff lines and gutter numbers have no position.
 */
export function sourcePosition(root: HTMLElement, node: Node, offset: number): CodeDiagnosticPosition | undefined {
  const el = (node.nodeType === 1 ? node as Element : node.parentElement)?.closest<HTMLElement>('[data-source-line]');
  if (!el || !root.contains(el)) return undefined;
  const line = Number(el.dataset.sourceLine) - 1;
  if (!Number.isInteger(line) || line < 0) return undefined;
  const range = root.ownerDocument.createRange();
  range.setStart(el, 0);
  try { range.setEnd(node, offset); } catch { return undefined; }
  return { line, character: range.toString().length };
}

/** The DOM range of columns start..end on a source line, for painting. */
export function sourceRange(root: HTMLElement, line: number, start: number, end: number): Range | undefined {
  const el = root.querySelector<HTMLElement>(`[data-source-line="${line + 1}"]`);
  if (!el) return undefined;
  const walker = root.ownerDocument.createTreeWalker(el, 4 /* SHOW_TEXT */);
  const points: ([Node, number] | undefined)[] = [undefined, undefined];
  let seen = 0, node: Node | null;
  while ((node = walker.nextNode()) && !points[1]) {
    const length = node.textContent?.length ?? 0;
    if (!points[0] && start <= seen + length) points[0] = [node, start - seen];
    if (!points[1] && end <= seen + length) points[1] = [node, end - seen];
    seen += length;
  }
  if (!points[0] || !points[1]) return undefined;
  const range = root.ownerDocument.createRange();
  range.setStart(...points[0]); range.setEnd(...points[1]);
  return range;
}

/** Cmd on macOS, Ctrl elsewhere: the modifier that turns words into links. */
export function linkModifier(event: Pick<MouseEvent | KeyboardEvent, 'metaKey' | 'ctrlKey' | 'altKey'>, mac: boolean): boolean {
  return (mac ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey) && !event.altKey;
}

/**
 * A press that opens a context menu: a right-click, or Ctrl+click on macOS
 * only, since elsewhere Ctrl+click is the link click.
 */
export function contextPress(event: Pick<MouseEvent, 'button' | 'ctrlKey'>, mac: boolean): boolean {
  return event.button === 2 || (mac && event.button === 0 && event.ctrlKey);
}

/**
 * Whether a key could open a context menu (the Menu key, Shift+F10), so a
 * remembered press no longer belongs to the next one; held modifiers repeat
 * on Windows and do not count.
 */
export function menuKey(event: Pick<KeyboardEvent, 'key'>): boolean {
  return !['Shift', 'Control', 'Alt', 'Meta'].includes(event.key);
}

/** F12 and Shift+F12, as in VS Code. */
export function navigationKey(event: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>): 'definition' | 'references' | undefined {
  if (event.key !== 'F12' || event.metaKey || event.ctrlKey || event.altKey) return undefined;
  return event.shiftKey ? 'references' : 'definition';
}

/**
 * Back and forward through code navigation: the app's keys (Cmd+[ / Cmd+],
 * Alt+Left/Right) and, as in VS Code on macOS, Ctrl+- / Ctrl+Shift+-. While
 * typing, the app's keys belong to the field (Cmd+[ outdents in the editor),
 * so only Ctrl+- and the keyboard's Back/Forward keys go.
 */
export function historyKey(event: Pick<KeyboardEvent, 'key' | 'code' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'target'>, mac: boolean): NavDirection | null {
  if (mac && event.ctrlKey && !event.metaKey && !event.altKey && event.code === 'Minus') return event.shiftKey ? 'forward' : 'back';
  const t = event.target as { tagName?: string; isContentEditable?: boolean } | null;
  const typing = Boolean(t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName ?? '')));
  if (typing && !event.key.startsWith('Browser')) return null;
  return navigationShortcut(event, mac);
}

/** How a location reads in a list: path, then 1-based line and column. */
export function locationText(location: CodeNavigationLocation, label: (path: string) => string = p => p): string {
  const { line, character } = location.range.start;
  return `${location.external ? label(location.path) : location.path}:${line + 1}:${character + 1}`;
}

/**
 * How a list of places reads. When every place is outside the project and
 * they share a folder, the folder is named once as the heading and each place
 * reads from there: a Ruby module reopened in 40 files of one gem does not
 * repeat the gem's path 40 times.
 */
export function locationList(locations: CodeNavigationLocation[], label: (path: string) => string = p => p): { heading?: string; labels: string[] } {
  const dir = locations.length > 1 && locations.every(l => l.external) ? commonDirectory(locations.map(l => l.path)) : '';
  if (!dir) return { labels: locations.map(l => locationText(l, label)) };
  return { heading: label(dir), labels: locations.map(l => locationText({ ...l, external: false, path: l.path.slice(dir.length + 1) })) };
}

/** The deepest folder holding every path, or '' when that is only a root. */
function commonDirectory(paths: string[]): string {
  let end = paths[0].length;
  for (const p of paths) {
    end = Math.min(end, p.length);
    for (let i = 0; i < end; i++) if (p[i] !== paths[0][i]) { end = i; break; }
  }
  const prefix = paths[0].slice(0, end);
  const dir = prefix.slice(0, Math.max(prefix.lastIndexOf('/'), prefix.lastIndexOf('\\'), 0));
  // "/" or "C:" says nothing about where the places are.
  return /^([A-Za-z]:)?$/.test(dir) ? '' : dir;
}

/** A Ruby constant's file name, as ActiveSupport's underscore: HTTPClient → http_client. */
export function rubyFileName(constant: string): string {
  return constant.replace(/([A-Z\d]+)([A-Z][a-z])/g, '$1_$2').replace(/([a-z\d])([A-Z])/g, '$1_$2').toLowerCase();
}

/**
 * Definitions of a Ruby constant, the likely file first. A module or class
 * is reopened in many files and ruby-lsp sends every reopening in index
 * order; by the naming rule gems and Rails apps follow (Zeitwerk),
 * `ActiveJob::Base` is defined in `active_job/base.rb`. So first the file
 * named after the constant's whole path (the `A::B::` written before the
 * word, then the word), then files named after the word alone, then the rest.
 * Nothing is dropped.
 */
export function rankRubyDefinitions<T extends { path: string }>(locations: T[], word: string, before: string): T[] {
  if (!/^[A-Z]/.test(word)) return locations;
  const qualifier = before.match(/(?<![\p{L}\p{N}_$])(?:[A-Z][\p{L}\p{N}_]*::)+$/u)?.[0] ?? '';
  const own = rubyFileName(word);
  const full = [...qualifier.split('::').filter(Boolean).map(rubyFileName), own].join('/');
  const rank = (path: string) => {
    const stem = '/' + path.replace(/\\/g, '/').replace(/\.rbs?$/, '');
    return stem.endsWith('/' + full) ? 0 : stem.endsWith('/' + own) ? 1 : 2;
  };
  // Array sort is stable, so ties keep the server's order.
  return locations.map(l => ({ l, r: rank(l.path) })).sort((a, b) => a.r - b.r).map(x => x.l);
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Language servers may pass through Python-Markdown documentation directives.
// Adapt only their indented bodies, never literal examples or plaintext hovers.
function hoverMarkdown(source: string): string {
  const lines = source.split(/\r?\n/);
  const out: string[] = [];
  let fenced = false;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trimStart().startsWith('```')) fenced = !fenced;
    const header = !fenced && line.match(/^!!! ([a-z][\w-]*)(?: "([^"\n]+)")?\s*$/i);
    if (!header) {
      out.push(line);
      continue;
    }
    // Some servers encode the four-space indent as nonbreaking spaces.
    const indent = /^(?: {4}|\t|(?:&nbsp;){4}| {4})/;
    const body: string[] = [];
    let next = i + 1;
    while (next < lines.length) {
      if (indent.test(lines[next])) {
        body.push(lines[next].replace(indent, ''));
        next++;
      } else if (!lines[next].trim()) {
        body.push('');
        next++;
      } else break;
    }
    if (!body.some((l) => l.trim())) {
      out.push(line);
      continue;
    }
    const title = (header[2] ?? header[1]).replace(/[\\`*_[\]<>]/g, '\\$&');
    out.push('', `> **${title}**`, '>', ...body.map((l) => `> ${l}`), '');
    i = next - 1;
  }
  return out.join('\n');
}

/** The hover text as HTML: Markdown through the transcript renderer, plain text escaped. */
export function hoverHtml(hover: CodeHoverText): string {
  return hover.kind === 'markdown' ? renderMarkdown(hoverMarkdown(hover.value)) : `<pre class="hover-plain">${escapeHtml(hover.value)}</pre>`;
}
