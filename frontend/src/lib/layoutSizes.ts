/**
 * Content-fit layout sizing. The sidebar and session list have proven fixed
 * defaults; the transcript header is the content that runs out of room, so
 * `transcriptNeed` measures what it must show and `fitToWidth` keeps that much
 * window for it when the window narrows.
 */

/** MeasureText returns the width in CSS pixels of text drawn in the given CSS font. */
export type MeasureText = (text: string, font: string) => number;

/** HeaderItem is one visible item in a transcript header row. */
export interface HeaderItem {
  text: string;
  font: string;
  /** Items that shrink with an ellipsis, like the cwd path, count only their
   *  minimum width; the rest count their measured text. */
  flex?: boolean;
  /** The width a flex item keeps when shrunk. */
  minWidth?: number;
}

export interface NeedOptions {
  /** Gap between items on a row. The header-meta-row gap is 12px. */
  gapPx?: number;
  /** The pane's own chrome beside the rows: borders, padding. */
  paddingPx?: number;
  /** The hard floor: the pane never gets less, however little fits. */
  min?: number;
  /** The cap: one long path cannot take over the window. */
  max?: number;
}

/**
 * transcriptNeed is the width the transcript header needs to show its rows
 * unwrapped: the widest row, its items side by side plus gaps, plus the pane's
 * chrome. Shrinkable items count only their minimum. The result sits between
 * min and max.
 */
export function transcriptNeed(
  rows: readonly (readonly HeaderItem[])[],
  measure: MeasureText,
  opts: NeedOptions = {},
): number {
  const gap = opts.gapPx ?? 12;
  const padding = opts.paddingPx ?? 24;
  const min = opts.min ?? 380;
  const max = opts.max ?? 800;
  let widest = 0;
  for (const row of rows) {
    let width = 0;
    for (const [i, item] of row.entries()) {
      if (i > 0) width += gap;
      width += item.flex ? item.minWidth ?? 0 : Math.max(item.minWidth ?? 0, measure(item.text, item.font));
    }
    if (width > widest) widest = width;
  }
  const need = Math.round(widest + padding);
  return Math.max(min, Math.min(max, need));
}

export interface PaneWidths {
  sidebar: number;
  sessionList: number;
}

export type PaneFloors = PaneWidths & { transcript: number };

/**
 * fitToWidth keeps the transcript at its floor when the window narrows: the
 * sidebar and session list give up width, the pane with more room above its
 * floor first. A window too small for all three floors lets the transcript
 * take the hit — the navigation floors are what keep the app usable.
 */
export function fitToWidth(windowWidth: number, widths: PaneWidths, floors: PaneFloors): PaneWidths {
  const out = { sidebar: widths.sidebar, sessionList: widths.sessionList };
  let deficit = out.sidebar + out.sessionList - (windowWidth - floors.transcript);
  if (deficit <= 0) return out;
  // The pane further above its floor gives up width first.
  const order: (keyof PaneWidths)[] =
    out.sessionList - floors.sessionList > out.sidebar - floors.sidebar
      ? ['sessionList', 'sidebar']
      : ['sidebar', 'sessionList'];
  for (const pane of order) {
    const cut = Math.min(deficit, out[pane] - floors[pane]);
    if (cut > 0) {
      out[pane] -= cut;
      deficit -= cut;
    }
    if (deficit <= 0) break;
  }
  return out;
}

/**
 * createMeasure measures text through a canvas in the app's fonts. Without a
 * DOM (tests) every text measures nothing, so needs fall back to their
 * minimums.
 */
export function createMeasure(): MeasureText {
  if (typeof document === 'undefined') return () => 0;
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  if (!ctx) return () => 0;
  return (text: string, font: string) => {
    ctx.font = font;
    return Math.ceil(ctx.measureText(text).width);
  };
}
