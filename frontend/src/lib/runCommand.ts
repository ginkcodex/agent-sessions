/** Code block languages a shell tab can run as written. */
const SHELL_LANGS = new Set(['bash', 'sh', 'shell', 'zsh']);

import { pasteSafe } from './terminalClipboard';

// Zero-width and bidi format characters: they hide or reorder what the
// command shows, so the line read in the chat is not the one that runs.
const INVISIBLE = /[\u200b-\u200f\u202a-\u202e\u2060-\u2064\u2066-\u2069\ufeff]/g;

// A `$ ` prompt, or a bare `$` line.
const PROMPT = /^\s*\$(?: |$)/;

/**
 * The command a shell code block's Run button types, or '' when the block
 * gets no button: another language, or nothing left to run. A leading `!`
 * (Claude Code's shell-mode prefix, which agents quote) goes, and so do `$ `
 * prompts when every line has one.
 */
export function runnableCommand(lang: string, code: string): string {
  if (!SHELL_LANGS.has(lang.toLowerCase())) return '';
  let command = pasteSafe(code).replace(INVISIBLE, '').trim();
  command = command.replace(/^!\s*/, '');
  const lines = command.split('\n');
  const filled = lines.filter((line) => line.trim());
  if (filled.length > 0 && filled.every((line) => PROMPT.test(line))) {
    command = lines.map((line) => line.replace(PROMPT, '')).join('\n');
  }
  return command.trim();
}
