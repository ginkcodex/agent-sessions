import type { IDisposable, Terminal } from '@xterm/xterm';

/**
 * The text an OSC 52 sequence asks to put on the clipboard: `Pc;Pd`, where
 * Pd is base64. Queries (`?`), empty and malformed payloads give undefined:
 * programs may set the clipboard but never read it.
 */
export function parseOsc52(data: string): string | undefined {
  const sep = data.indexOf(';');
  if (sep < 0) return undefined;
  const payload = data.slice(sep + 1);
  if (payload === '' || payload === '?') return undefined;
  let binary: string;
  try {
    binary = atob(payload);
  } catch {
    return undefined;
  }
  return new TextDecoder().decode(Uint8Array.from(binary, (c) => c.charCodeAt(0)));
}

/**
 * Programs that read the mouse (Claude Code's fullscreen view, Codex,
 * OpenCode) select text themselves and copy it with OSC 52, which xterm
 * leaves to its host. Every selection target goes to the one clipboard.
 */
export function installTerminalClipboard(terminal: Terminal, write: (text: string) => void): IDisposable {
  return terminal.parser.registerOscHandler(52, (data) => {
    const text = parseOsc52(data);
    if (text) write(text);
    return true;
  });
}
