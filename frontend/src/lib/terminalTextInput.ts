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

/**
 * WebKitGTK exposes the IME's exact commit in insertFromComposition. Use it
 * instead of xterm's delayed textarea diffs, which replay stale text after
 * punctuation and read WebKit's NBSP substitutions instead of actual spaces.
 * Native preedit and xterm's composition display remain in control until
 * commit; only clear the scratch textarea once native composition has ended.
 */
export function installTerminalCompositionText(
  textarea: HTMLTextAreaElement | undefined,
  send: (text: string) => void,
) {
  let committed = false;
  const onInput = (event: Event) => {
    const input = event as InputEvent;
    if (input.inputType !== 'insertFromComposition') return;
    committed = true;
    if (input.data) send(input.data);
  };
  const onEnd = () => {
    if (!committed || !textarea) return;
    // xterm still receives compositionend to dismiss its preedit display, but
    // its deferred finalizer has no text to send a second time.
    textarea.value = '';
    committed = false;
  };
  textarea?.addEventListener('input', onInput);
  textarea?.addEventListener('compositionend', onEnd, true);
  return {
    key(event: Pick<KeyboardEvent, 'keyCode'>): boolean {
      // Never start xterm's asynchronous textarea-diff fallback for IME keys.
      return event.keyCode !== 229;
    },
    dispose(): void {
      textarea?.removeEventListener('input', onInput);
      textarea?.removeEventListener('compositionend', onEnd, true);
    },
  };
}
