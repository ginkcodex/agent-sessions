// Monospace font choices (the terminal and the editor). Names come from the
// local OS, not the connected SSH/WSL host. Quote the family as a CSS string
// so punctuation cannot introduce extra font families.
export const SYSTEM_MONO_FONTS = 'ui-monospace, SFMono-Regular, Menlo, Consolas, "DejaVu Sans Mono", monospace';

/** A storable font name: '' (the app default) or an installed family's name. */
export function isTerminalFont(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 200 && !/[\x00-\x1f\x7f]/.test(value);
}

/** A CSS font-family list for a chosen monospace font, or `defaultFamily` for none. */
export function monoFontFamily(name: string, defaultFamily: string): string {
  if (!isTerminalFont(name) || !name) return defaultFamily;
  return `${JSON.stringify(name)}, ${SYSTEM_MONO_FONTS}`;
}

export const terminalFontFamily = monoFontFamily;
