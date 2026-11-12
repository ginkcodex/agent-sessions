/** Extract only known text envelopes; never escape-decode arbitrary tool text. */
export function readableToolOutput(raw: string): string {
  if (!/^[\s]*[\[{]/.test(raw)) return raw;
  try {
    const value: unknown = JSON.parse(raw);
    const envelope = value && typeof value === 'object' && !Array.isArray(value)
      && 'content' in value && Object.keys(value).every(key => key === 'content');
    const blocks = Array.isArray(value) ? value : envelope ? value.content : null;
    if (Array.isArray(blocks) && blocks.length && blocks.every(block =>
      block && typeof block === 'object' &&
      (block.type === 'text' || block.type === 'input_text' || block.type === 'output_text') &&
      typeof block.text === 'string' && Object.keys(block).every(key => key === 'type' || key === 'text'))) {
      return blocks.map(block => block.text).join('\n\n');
    }
    // Re-serializing arbitrary JSON can round large integers or discard duplicate keys.
    return raw;
  } catch {
    return raw;
  }
}

/** A bounded preview prevents large tool results from overwhelming the transcript. */
export function toolOutputPreview(text: string): { text: string; shortened: boolean } {
  const maxChars = 1200;
  let end = Math.min(text.length, maxChars);
  let start = 0;
  for (let line = 0; line < 8; line++) {
    const next = text.indexOf('\n', start);
    if (next < 0) break;
    if (line === 7) end = Math.min(end, next);
    start = next + 1;
  }
  // Do not leave half an emoji at a character-budget boundary.
  if (end < text.length && end > 0 && /[\uD800-\uDBFF]/.test(text[end - 1])) end--;
  return { text: text.slice(0, end), shortened: end < text.length };
}

/** Prefer the useful command or file target to an opaque tool-call identifier. */
export function toolInputSummary(input: unknown): string {
  let value = input;
  if (typeof value === 'string') {
    try { value = JSON.parse(value); } catch { return ''; }
  }
  if (!value || typeof value !== 'object') return '';
  const record = value as Record<string, unknown>;
  for (const key of ['command', 'cmd', 'file_path', 'filePath', 'path', 'pattern', 'query', 'description']) {
    if (typeof record[key] === 'string') return record[key].replace(/\s+/g, ' ').slice(0, 180);
  }
  return '';
}
