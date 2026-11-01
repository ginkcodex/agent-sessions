/**
 * macOS input tools such as OpenKey can inject a whole replacement string in
 * one native key event. WebKit puts it in `key`, but keyCode/charCode describe
 * only its first character. xterm 6's keypress handler sends that character
 * and suppresses the following input event, losing the rest of the string.
 *
 * Only intercept printable-key events carrying multiple Unicode code points.
 * The 48–90 keyCode range is WebKit's printable letter/digit mapping; named
 * keys (arrows, Delete, function keys, Process/229, etc.) must stay with xterm.
 * A single supplementary character stays on xterm's normal Unicode path too.
 */
export function terminalReplacementText(
  event: Pick<KeyboardEvent, 'key' | 'keyCode' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing'>,
): string | null {
  if (event.isComposing || event.ctrlKey || event.metaKey || event.altKey) return null;
  if (event.keyCode < 48 || event.keyCode > 90) return null;
  if (event.key === 'Dead' || event.key === 'Unidentified' || event.key === 'Process') return null;
  return Array.from(event.key).length > 1 ? event.key : null;
}
