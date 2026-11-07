// Font settings (docs/plan/font-settings.md): the editor font, the body
// text sizes of the editor, the project docs preview and session content,
// and the built-in terminal's size (an xterm.js option, not a property).
// `null` / '' mean "default": no custom property is set and the CSS
// fallbacks apply, so the default look cannot drift.
import { isTerminalFont, monoFontFamily } from './terminalFont';

export type FontSizeKind = 'editor' | 'docs' | 'session' | 'terminal';

export interface FontSizeRange {
  min: number;
  max: number;
  /** The size the CSS falls back to when nothing is set. */
  default: number;
}

export const FONT_SIZES: Record<FontSizeKind, FontSizeRange> = {
  // 0.78rem on the 18px app base.
  editor: { min: 10, max: 28, default: 14.04 },
  docs: { min: 11, max: 28, default: 14.5 },
  session: { min: 11, max: 24, default: 14.5 },
  // xterm.js's size before this setting.
  terminal: { min: 9, max: 28, default: 13 },
};

/** Custom properties set on the document root; each is unset at its default. */
export const ROOT_VARS = ['--editor-font-family', '--editor-font-size', '--docs-font-size'] as const;

export interface AppearancePrefs {
  editorFont: string;
  editorFontSize: number | null;
  docsFontSize: number | null;
}

/**
 * A valid size for `kind`, kept to 0.5px, or null (the default) for anything
 * else: non-numbers, out-of-range values, or stored text that is not a plain
 * number.
 */
export function fontSize(kind: FontSizeKind, value: unknown): number | null {
  let n: number;
  if (typeof value === 'number') n = value;
  else if (typeof value === 'string' && /^\d{1,3}(\.\d+)?$/.test(value)) n = Number(value);
  else return null;
  if (!Number.isFinite(n)) return null;
  n = Math.round(n * 2) / 2;
  const { min, max } = FONT_SIZES[kind];
  return n >= min && n <= max ? n : null;
}

/**
 * One step up (+1) or down (-1) from `value` (null: the default), in whole
 * pixels: a fractional size goes to the next whole number (14.5 → 15 or 14).
 * Stays within the range.
 */
export function stepSize(kind: FontSizeKind, value: number | null, direction: 1 | -1): number {
  const { min, max, default: fallback } = FONT_SIZES[kind];
  const current = value ?? fallback;
  const next = direction > 0 ? Math.floor(current) + 1 : Math.ceil(current) - 1;
  return Math.min(max, Math.max(min, next));
}

/** Root custom properties for the preferences; defaults are left out. */
export function appearanceVars(prefs: AppearancePrefs): Partial<Record<(typeof ROOT_VARS)[number], string>> {
  const vars: Partial<Record<(typeof ROOT_VARS)[number], string>> = {};
  if (isTerminalFont(prefs.editorFont) && prefs.editorFont) {
    vars['--editor-font-family'] = monoFontFamily(prefs.editorFont, 'var(--font-mono)');
  }
  const editor = fontSize('editor', prefs.editorFontSize);
  if (editor !== null) vars['--editor-font-size'] = `${editor}px`;
  const docs = fontSize('docs', prefs.docsFontSize);
  if (docs !== null) vars['--docs-font-size'] = `${docs}px`;
  return vars;
}

/** The `--session-font-size` value for the transcript, or undefined at the default. */
export function sessionFontVar(size: number | null): string | undefined {
  const valid = fontSize('session', size);
  return valid === null ? undefined : `${valid}px`;
}

/** Sets the root properties and removes those at their default. */
export function applyRootVars(style: Pick<CSSStyleDeclaration, 'setProperty' | 'removeProperty'>, vars: ReturnType<typeof appearanceVars>): void {
  for (const name of ROOT_VARS) {
    const value = vars[name];
    if (value) style.setProperty(name, value);
    else style.removeProperty(name);
  }
}
