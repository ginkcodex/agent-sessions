// Text checking off in every field (docs/plan/keyboard-diagnostics.md,
// KD-06). Spellcheck, autocorrect, autocapitalize and inline predictions can
// rewrite text behind the user's input method, so they are off everywhere.
// The macOS app also turns off WebKit's substitutions natively
// (cmd/agent-sessions/textcheck_darwin.go).

/** Set on each text field when it gets focus. */
export const TEXT_INPUT_ATTRIBUTES: Readonly<Record<string, string>> = {
  spellcheck: 'false',
  autocorrect: 'off',
  autocapitalize: 'off',
  writingsuggestions: 'false',
};

/** Set on `<html>`: descendants inherit these two. */
const INHERITED = ['spellcheck', 'writingsuggestions'];

const TEXT_INPUTS = new Set(['text', 'search', 'url', 'email', 'tel', 'password', '']);

type FieldLike = {
  tagName?: unknown;
  isContentEditable?: boolean;
  getAttribute?: (name: string) => string | null;
};

/** A textarea, a text-like input, or contenteditable content. */
export function isTextEntry(target: EventTarget | null): boolean {
  const el = target as FieldLike | null;
  if (!el || typeof el.tagName !== 'string') return false;
  if (el.isContentEditable) return true;
  const tag = el.tagName.toUpperCase();
  if (tag === 'TEXTAREA') return true;
  if (tag !== 'INPUT') return false;
  return TEXT_INPUTS.has((el.getAttribute?.('type') ?? '').toLowerCase());
}

/** Turns text checking off on one field. Unchanged values are not rewritten. */
export function applyTextInputDefaults(el: Element): void {
  for (const [name, value] of Object.entries(TEXT_INPUT_ATTRIBUTES)) {
    if (el.getAttribute(name) !== value) el.setAttribute(name, value);
  }
}

/**
 * Turns text checking off on the whole page and on each text field as it
 * gets focus. Returns a function that removes the listener.
 */
export function installTextInputDefaults(doc: Document = document): () => void {
  for (const name of INHERITED) doc.documentElement.setAttribute(name, TEXT_INPUT_ATTRIBUTES[name]);
  const onFocus = (event: Event) => {
    if (isTextEntry(event.target)) applyTextInputDefaults(event.target as Element);
  };
  doc.addEventListener('focusin', onFocus, true);
  if (isTextEntry(doc.activeElement)) applyTextInputDefaults(doc.activeElement as Element);
  return () => doc.removeEventListener('focusin', onFocus, true);
}
