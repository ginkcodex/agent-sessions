import type { Message, SessionRef } from './types';

export type ManualBash =
  | { kind: 'input'; command: string }
  | { kind: 'output'; stdout: string; stderr: string };

/** Recognize complete, dedicated Claude Code ! envelopes, never Markdown/prose. */
export function parseManualBash(text: string): ManualBash | null {
  const trimmed = text.trim();
  if (trimmed.startsWith('<bash-input>') && trimmed.endsWith('</bash-input>')) {
    const command = trimmed.slice('<bash-input>'.length, -'</bash-input>'.length);
    if (!command.includes('</bash-input>')) return { kind: 'input', command };
  }
  if (!trimmed.startsWith('<bash-stdout>')) return null;
  const end = trimmed.indexOf('</bash-stdout>', '<bash-stdout>'.length);
  if (end < 0) return null;
  const rest = trimmed.slice(end + '</bash-stdout>'.length).trim();
  if (!rest.startsWith('<bash-stderr>') || !rest.endsWith('</bash-stderr>')) return null;
  const stderr = rest.slice('<bash-stderr>'.length, -'</bash-stderr>'.length);
  if (stderr.includes('</bash-stderr>')) return null;
  return { kind: 'output', stdout: trimmed.slice('<bash-stdout>'.length, end), stderr };
}

/** Keep raw transcript parts/IDs unchanged; adapt only dedicated user records. */
export function manualBashMessage(message: Message, ref: SessionRef): ManualBash | null {
  if (ref.agent !== 'claude-code' || message.role !== 'user' || !message.parts?.length) return null;
  if (message.parts.some(part => part.kind !== 'text')) return null;
  return parseManualBash(message.parts.map(part => part.text ?? '').join('\n'));
}

function fence(text: string, language = ''): string {
  const runs = text.match(/`+/g) ?? [];
  const marker = '`'.repeat(runs.reduce((length, run) => Math.max(length, run.length + 1), 3));
  return `${marker}${language}\n${text}\n${marker}`;
}

/** Copy/print safe fenced Markdown rather than leaking protocol wrappers. */
export function manualBashMarkdown(record: ManualBash): string {
  if (record.kind === 'input') return fence(record.command, 'bash');
  return [
    record.stdout ? `**stdout**\n\n${fence(record.stdout)}` : '',
    record.stderr ? `**stderr**\n\n${fence(record.stderr)}` : '',
  ].filter(Boolean).join('\n\n') || 'No output';
}

/** Initial DOM stays small even for a single huge output line; full text is opt-in. */
export function bashPreview(text: string): { text: string; clipped: boolean } {
  const maxChars = 6000;
  let end = text.indexOf('\n');
  for (let line = 1; line < 12 && end >= 0; line++) end = text.indexOf('\n', end + 1);
  if (end < 0) end = text.length;
  end = Math.min(end, maxChars);
  return { text: text.slice(0, end), clipped: end < text.length };
}
