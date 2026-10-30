import type { ContextMenuItem } from './components/common/ContextMenu.svelte';

export const MAX_SELECTION_CONTEXT = 8192;

export function selectionMenuItems(configured: boolean, language: string, copy: () => void, translate: () => void, setup: () => void): ContextMenuItem[] {
  return [
    { label: 'Copy', onSelect: copy },
    ...(configured ? [{ label: `Translate to ${language}`, onSelect: translate }] : [
      { label: 'Translate', onSelect: () => {}, disabled: true, title: 'Set up translation in Settings' },
      { label: 'Set up translation…', onSelect: setup },
    ]),
  ];
}

/** The owner must invalidate this request on replacement or teardown. */
export function requestSelectionTranslation(request: () => Promise<string>, done: (text: string) => void, failed: (error: unknown) => void): () => void {
  let current = true;
  void Promise.resolve().then(request).then(text => { if (current) done(text); }, error => { if (current) failed(error); });
  return () => { current = false; };
}

export interface SelectedText {
  text: string;
  context: string;
  rect: { left: number; right: number; top: number; bottom: number };
  /** A copy of the selected range, held while the menu or popover has focus. */
  range: Range;
}

const HELD = 'held-selection';

/**
 * Focus moving into the menu or popover clears the page selection. Paint the
 * captured range meanwhile; release selects it again unless it left the page or
 * another selection replaced it.
 */
export function holdSelection(range: Range): () => void {
  const highlights = typeof CSS !== 'undefined' && typeof Highlight !== 'undefined' ? CSS.highlights : undefined;
  const highlight = highlights ? new Highlight(range) : undefined;
  if (highlight) highlights!.set(HELD, highlight);
  return () => {
    if (highlight && highlights!.get(HELD) === highlight) highlights!.delete(HELD);
    const selection = window.getSelection();
    if (!selection || !range.startContainer.isConnected || !range.endContainer.isConnected) return;
    if (selection.rangeCount > 0 && !selection.isCollapsed) return;
    selection.removeAllRanges();
    selection.addRange(range);
  };
}

const encoder = new TextEncoder();

/** Keep sentence context near the selected range, bounded in UTF-8 bytes. */
export function sentenceContext(text: string, start: number, end: number): string {
  const Segmenter = Intl.Segmenter;
  const spans = Segmenter
    ? Array.from(new Segmenter(undefined, { granularity: 'sentence' }).segment(text), s => ({ start: s.index, end: s.index + s.segment.length }))
    : Array.from(text.matchAll(/[^.!?。！？\n]+[.!?。！？\n]*|[.!?。！？\n]+/g), s => ({ start: s.index!, end: s.index! + s[0].length }));
  const first = spans.find(s => s.end > start);
  const last = spans.find(s => s.end >= end);
  let left = first?.start ?? 0;
  let right = last?.end ?? text.length;
  // A pathological sentence can be as long as the message. Narrow around the
  // selection first so measuring/copying it remains bounded.
  left = Math.max(left, start - MAX_SELECTION_CONTEXT);
  right = Math.min(right, Math.max(end, start + MAX_SELECTION_CONTEXT));
  // UTF-16 range offsets can put a crop boundary between surrogate halves.
  if (left > 0 && /[\uDC00-\uDFFF]/.test(text[left] ?? '')) left--;
  if (right < text.length && /[\uDC00-\uDFFF]/.test(text[right] ?? '')) right++;
  const points = Array.from(text.slice(left, right));
  let offset = left;
  let bytes = 0;
  const selected = points.findIndex(p => { const at = offset; offset += p.length; return at >= start; });
  const pivot = Math.max(0, selected);
  let lo = pivot;
  let hi = pivot;
  while (lo > 0 || hi < points.length) {
    const forward = hi < points.length && (lo === 0 || hi - pivot <= pivot - lo);
    const point = forward ? points[hi] : points[lo - 1];
    const size = encoder.encode(point).length;
    if (bytes + size > MAX_SELECTION_CONTEXT) break;
    bytes += size;
    if (forward) hi++; else lo--;
  }
  return points.slice(lo, hi).join('').trim();
}

/** The part of a text node inside `range`, or '' when they do not intersect. */
function selectedPart(node: Node, range: Range): string {
  const inside = node === range.startContainer || node === range.endContainer || range.intersectsNode(node);
  if (!inside) return '';
  const from = node === range.startContainer ? range.startOffset : 0;
  const to = node === range.endContainer ? range.endOffset : undefined;
  return (node.textContent ?? '').slice(from, to);
}

/** Text the user could see selected: rendered and not `user-select: none`. */
function selectable(el: Element | null): boolean {
  for (let e = el; e; e = e.parentElement) {
    const style = getComputedStyle(e) as CSSStyleDeclaration & { webkitUserSelect?: string };
    if (style.display === 'none' || style.visibility === 'hidden' || style.userSelect === 'none' || style.webkitUserSelect === 'none') return false;
  }
  return true;
}

function hasSelectableText(range: Range): boolean {
  if (range.collapsed) return false;
  const top = range.commonAncestorContainer;
  const walker = (top.ownerDocument ?? document).createTreeWalker(top, NodeFilter.SHOW_TEXT);
  for (let node: Node | null = top.nodeType === 3 ? top : walker.nextNode(); node; node = node === top ? null : walker.nextNode()) {
    if (selectedPart(node, range).trim() && selectable(node.parentElement)) return true;
  }
  return false;
}

/**
 * Line and paragraph selection (multi-click, or a drag past the line end) can
 * end at an element boundary: offset 0 of the next message's header, or of a
 * control after the text. Narrow such a range to the selected text in `root`,
 * unless the dropped parts hold text the user sees selected.
 */
function selectedTextRange(root: Node, range: Range): Range | null {
  const { startContainer: start, endContainer: end } = range;
  if (start.nodeType === 3 && end.nodeType === 3 && root.contains(start) && root.contains(end)) return range;
  let first: [Node, number] | undefined;
  let last: [Node, number] | undefined;
  (function walk(node: Node) {
    if (node.nodeType !== 3) return node.childNodes.forEach(walk);
    if (!selectedPart(node, range).trim() || !selectable(node.parentElement)) return;
    first ??= [node, node === start ? range.startOffset : 0];
    last = [node, node === end ? range.endOffset : node.textContent?.length ?? 0];
  })(root);
  if (!first || !last) return null;
  const narrowed = range.cloneRange();
  narrowed.setStart(...first);
  narrowed.setEnd(...last);
  const before = range.cloneRange();
  before.setEnd(...first);
  const after = range.cloneRange();
  after.setStart(...last);
  return hasSelectableText(before) || hasSelectableText(after) ? null : narrowed;
}

/** Capture before the menu takes focus. Cross-message and control ranges stay native. */
export function captureSelection(root: HTMLElement, selection: Selection | null): SelectedText | null {
  if (!selection || selection.isCollapsed || selection.rangeCount !== 1) return null;
  const narrowed = selectedTextRange(root, selection.getRangeAt(0));
  if (!narrowed) return null;
  const range: Range = narrowed;
  const positions = new Map<Node, number[]>();
  let text = '';
  let invalid = false;
  const block = /^(P|DIV|LI|PRE|BLOCKQUOTE|H[1-6]|TR|SECTION|DETAILS)$/;
  function walk(node: Node) {
    if (node.nodeType === 3) {
      positions.set(node, [text.length]);
      text += node.textContent ?? '';
      return;
    }
    if (node.nodeType !== 1) return;
    const el = node as HTMLElement;
    const style = getComputedStyle(el);
    if (el.matches('button,input,textarea,select,[hidden],[aria-hidden="true"]') || style.display === 'none' || style.visibility === 'hidden') {
      if (range.intersectsNode(el)) invalid = true;
      return;
    }
    const newline = () => { if (text && !text.endsWith('\n')) text += '\n'; };
    if (block.test(el.tagName) || el.tagName === 'BR') newline();
    const offsets = [text.length];
    for (const child of el.childNodes) {
      walk(child);
      offsets.push(text.length);
    }
    positions.set(node, offsets);
    if (block.test(el.tagName)) newline();
  }
  walk(root);
  const offset = (node: Node, at: number) => node.nodeType === 3
    ? (positions.get(node)?.[0] ?? NaN) + at : positions.get(node)?.[at] ?? NaN;
  const start = offset(range.startContainer, range.startOffset);
  const end = offset(range.endContainer, range.endOffset);
  const selected = range.toString();
  if (invalid || !Number.isFinite(start) || !Number.isFinite(end) || !selected.trim()) return null;
  const rect = range.getBoundingClientRect();
  return { text: selected, context: sentenceContext(text, start, end), rect: { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom }, range: range.cloneRange() };
}
