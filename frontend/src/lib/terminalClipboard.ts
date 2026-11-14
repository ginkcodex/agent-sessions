import type { IDisposable, Terminal } from '@xterm/xterm';

// C0 controls except tab and newline, DEL and C1. xterm's paste keeps them,
// and an ESC could end a bracketed paste early (`\e[201~`) and run what
// follows as typed input.
const CONTROLS = /[\x00-\x08\x0b-\x1f\x7f-\x9f]/g;

/** OSC 52 payloads over this many base64 characters (1 MiB decoded) are dropped. */
export const OSC52_MAX_PAYLOAD = Math.ceil((1024 * 1024) / 3) * 4;

/**
 * Text as it may reach a terminal as a paste: line breaks become newlines
 * (xterm sends them as CR) and every other control character goes.
 */
export function pasteSafe(text: string): string {
  return text.replace(/\r\n?/g, '\n').replace(CONTROLS, '');
}

/**
 * Pastes text into the terminal, without the controls that could escape
 * bracketed paste. Every paste into a panel terminal goes through here.
 */
export function pasteInto(terminal: Terminal, text: string): void {
  const safe = pasteSafe(text);
  if (safe) terminal.paste(safe);
}

/**
 * Takes the browser's paste event (Shift+Insert, middle click, the Edit menu)
 * from xterm, which would paste the text unfiltered, and pastes it through
 * pasteInto. host is the element xterm was opened in.
 */
export function installTerminalPaste(host: HTMLElement, terminal: Terminal): IDisposable {
  const onPaste = (event: ClipboardEvent) => {
    event.preventDefault();
    event.stopPropagation();
    const text = event.clipboardData?.getData('text/plain');
    if (text) pasteInto(terminal, text);
  };
  host.addEventListener('paste', onPaste, true);
  return { dispose: () => host.removeEventListener('paste', onPaste, true) };
}

/**
 * The text an OSC 52 sequence asks to put on the clipboard: `Pc;Pd`, where
 * Pd is base64. Queries (`?`), empty, oversize and malformed payloads give
 * undefined: programs may set the clipboard but never read it. Control
 * characters are dropped (see pasteSafe), since the text is pasted later.
 */
export function parseOsc52(data: string): string | undefined {
  const sep = data.indexOf(';');
  if (sep < 0) return undefined;
  const payload = data.slice(sep + 1);
  if (payload === '' || payload === '?' || payload.length > OSC52_MAX_PAYLOAD) return undefined;
  let binary: string;
  try {
    binary = atob(payload);
  } catch {
    return undefined;
  }
  return pasteSafe(new TextDecoder().decode(Uint8Array.from(binary, (c) => c.charCodeAt(0)))) || undefined;
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
