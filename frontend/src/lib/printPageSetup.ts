/** Page dimensions shared by the print editor, CSS and native print adapters. */
export interface PrintPageSetup {
  paper: 'a4' | 'letter';
  orientation: 'portrait' | 'landscape';
  /** Uniform physical margin, in millimetres, on all four page edges. */
  marginMm: number;
}

export const DEFAULT_PRINT_PAGE_SETUP: Readonly<PrintPageSetup> = Object.freeze({
  paper: 'a4', orientation: 'portrait', marginMm: 14,
});

export function isPrintPageSetup(value: unknown): value is PrintPageSetup {
  if (!value || typeof value !== 'object') return false;
  const setup = value as Partial<PrintPageSetup>;
  return (setup.paper === 'a4' || setup.paper === 'letter')
    && (setup.orientation === 'portrait' || setup.orientation === 'landscape')
    && typeof setup.marginMm === 'number' && Number.isFinite(setup.marginMm)
    && setup.marginMm >= 0 && setup.marginMm <= 40;
}

/** Copy and validate at dispatch time so later control edits cannot mutate a job. */
export function printPageSetup(value: unknown = DEFAULT_PRINT_PAGE_SETUP): PrintPageSetup {
  if (!isPrintPageSetup(value)) throw new Error('Choose A4 or Letter, portrait or landscape, and a margin from 0 to 40 mm.');
  return { paper: value.paper, orientation: value.orientation, marginMm: value.marginMm };
}

/** Physical paper and printable area, independent of the screen's pixel density. */
export function printPageDimensions(value: PrintPageSetup): { width: number; height: number; contentWidth: number; contentHeight: number } {
  const setup = printPageSetup(value);
  const [short, long] = setup.paper === 'letter' ? [215.9, 279.4] : [210, 297];
  const [width, height] = setup.orientation === 'landscape' ? [long, short] : [short, long];
  // Round away binary floating-point noise before using millimetres in CSS.
  const subtractMargins = (length: number) => Math.round((length - 2 * setup.marginMm) * 1000) / 1000;
  return { width, height, contentWidth: subtractMargins(width), contentHeight: subtractMargins(height) };
}

/** Validated values only: no user-provided CSS strings enter the print document. */
export function printPageCSS(value: PrintPageSetup): string {
  const setup = printPageSetup(value);
  const { width, height } = printPageDimensions(setup);
  return `@page { size: ${width}mm ${height}mm; margin: ${setup.marginMm}mm; }`;
}

/** A temporary page rule; remove it on print completion, failure or teardown. */
export function installPrintPageSetup(document: Document, setup: PrintPageSetup): () => void {
  const style = document.createElement('style');
  style.dataset.printPageSetup = '';
  style.textContent = printPageCSS(setup);
  document.head.appendChild(style);
  return () => style.remove();
}
