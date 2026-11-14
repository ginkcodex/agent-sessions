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

function inputRecord(input: unknown): Record<string, unknown> | null {
  let value = input;
  if (typeof value === 'string') {
    try { value = JSON.parse(value); } catch { return null; }
  }
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

export interface ToolFileTarget { path: string; line?: number }

// Tools whose generic `path` names one file. Elsewhere (Grep, Glob, LS) it is
// usually a directory.
const fileTools = new Set(['read', 'edit', 'write', 'multiedit', 'notebookedit']);

/**
 * The file a tool call reads or changes, taken as it is: tool input is not a
 * Markdown destination, so it is never URI-decoded. The backend still decides
 * whether the file may be opened.
 */
export function toolFileTarget(name: string, input: unknown): ToolFileTarget | null {
  const record = inputRecord(input);
  if (!record) return null;
  const keys = ['file_path', 'filePath', 'notebook_path'];
  if (fileTools.has(name.toLowerCase())) keys.push('path');
  const key = keys.find(key => typeof record[key] === 'string');
  const path = key ? (record[key] as string) : '';
  // A trailing separator names a directory; control characters name nothing.
  if (!path.trim() || /[\u0000-\u001f\u007f-\u009f]/.test(path) || /[\\/]$/.test(path)) return null;
  // Claude Code's Read counts its offset in lines from 1.
  const offset = record.offset;
  if (name === 'Read' && typeof offset === 'number' && Number.isSafeInteger(offset) && offset > 0) {
    return { path, line: offset };
  }
  return { path };
}

/** Prefer the useful command or file target to an opaque tool-call identifier. */
export function toolInputSummary(input: unknown): string {
  const record = inputRecord(input);
  if (!record) return '';
  for (const key of ['command', 'cmd', 'file_path', 'filePath', 'path', 'pattern', 'query', 'description']) {
    if (typeof record[key] === 'string') return record[key].replace(/\s+/g, ' ').slice(0, 180);
  }
  return '';
}
