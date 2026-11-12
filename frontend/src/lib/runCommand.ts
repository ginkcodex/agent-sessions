/** Code block languages a shell tab can run as written. */
const SHELL_LANGS = new Set(['bash', 'sh', 'shell', 'zsh']);

// C0 controls except tab and newline, DEL and C1. xterm's paste keeps them,
// and an ESC could end a bracketed paste early and run what follows.
const CONTROLS = /[\x00-\x08\x0b-\x1f\x7f-\x9f]/g;

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
  let command = code.replace(CONTROLS, '').trim();
  command = command.replace(/^!\s*/, '');
  const lines = command.split('\n');
  const filled = lines.filter((line) => line.trim());
  if (filled.length > 0 && filled.every((line) => PROMPT.test(line))) {
    command = lines.map((line) => line.replace(PROMPT, '')).join('\n');
  }
  return command.trim();
}
