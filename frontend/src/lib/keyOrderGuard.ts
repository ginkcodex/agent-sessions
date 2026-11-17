// Key order guard (docs/plan/keyboard-diagnostics.md, KD-05). OpenKey and
// similar macOS tools replace a word by posting Backspaces and then the new
// text. Sometimes WebKit hands those keys to the page out of order: the
// Backspace keyups arrive first, then the text, then some Backspace keydowns,
// whose deletes remove the text just inserted. Normal apps on the same Mac
// are unaffected, and switching apps clears it. The page sees the symptom
// (a Backspace keyup before its keydown), so it puts the deletes back before
// the text.

import { isTextEntry } from './textInputDefaults';

/** How long an early keyup or an owed keydown is kept, in milliseconds. */
export const KEY_ORDER_WINDOW = 100;

/** OpenKey's browser-mode spacer, sent before its Backspaces. */
const SPACER = '\u202F';

/** The event fired on the window for each correction, for the recorder. */
export const KEY_ORDER_GUARD_EVENT = 'keyorderguard';

export type KeyOrderEvent = Pick<
  KeyboardEvent,
  'type' | 'key' | 'keyCode' | 'repeat' | 'isComposing' | 'ctrlKey' | 'metaKey' | 'altKey'
>;

export type KeyOrderDecision =
  | { kind: 'pass' }
  /** A late Backspace keydown whose delete a repair already applied. */
  | { kind: 'drop' }
  /** Cancel this keydown: apply `deletes` Backspaces, then insert `text`. */
  | { kind: 'repair'; deletes: number; text: string };

const PASS: KeyOrderDecision = { kind: 'pass' };
const DROP: KeyOrderDecision = { kind: 'drop' };

/**
 * The text a keydown types, or null for named keys, shortcuts and input
 * method keys. Replacement tools put a whole string in `key`; WebKit's
 * printable key codes are 48–90, and named keys are ASCII words outside them.
 */
export function typedText(e: KeyOrderEvent): string | null {
  if (e.isComposing || e.ctrlKey || e.metaKey || e.altKey) return null;
  if (e.key === 'Dead' || e.key === 'Unidentified' || e.key === 'Process') return null;
  const length = Array.from(e.key).length;
  if (length === 1) return e.key;
  if (length > 1 && ((e.keyCode >= 48 && e.keyCode <= 90) || /[^\x00-\x7f]/.test(e.key))) return e.key;
  return null;
}

export type KeyOrderGuard = {
  handle(e: KeyOrderEvent): KeyOrderDecision;
  reset(): void;
};

/**
 * Pairs Backspace keydowns with their keyups in page order. Key events in
 * the normal order never change the early or owed counts, so they always
 * pass.
 */
export function createKeyOrderGuard(
  now: () => number = () => performance.now(),
  window = KEY_ORDER_WINDOW,
): KeyOrderGuard {
  let held = 0; // pressed Backspaces not yet released
  let early = 0; // Backspace keyups that came before their keydown
  let earlyAt = 0;
  let owed = 0; // late Backspace keydowns a repair already applied
  let owedAt = 0;

  const expire = (t: number) => {
    if (early > 0 && t - earlyAt > window) early = 0;
    if (owed > 0 && t - owedAt > window) owed = 0;
  };

  return {
    handle(e) {
      const t = now();
      expire(t);
      if (e.key === 'Backspace') {
        if (e.type === 'keyup') {
          if (held > 0) held--;
          else {
            early++;
            earlyAt = t;
          }
          return PASS;
        }
        if (e.type !== 'keydown' || e.repeat) return PASS;
        if (owed > 0) {
          owed--;
          return DROP;
        }
        // Late, but still before any text: its delete lands in the right place.
        if (early > 0) {
          early--;
          return PASS;
        }
        held++;
        return PASS;
      }
      if (e.type !== 'keydown' || early === 0) return PASS;
      const text = typedText(e);
      if (text === null || text === SPACER) return PASS;
      const deletes = early;
      early = 0;
      owed += deletes;
      owedAt = t;
      return { kind: 'repair', deletes, text };
    },
    reset() {
      held = 0;
      early = 0;
      owed = 0;
    },
  };
}

/** Terminal helper textareas, each with the function that sends its input. */
const terminals = new Map<EventTarget, (data: string) => void>();

/**
 * A terminal takes a repair as DELs followed by the text. Returns a function
 * that unregisters it.
 */
export function registerKeyOrderTerminal(textarea: EventTarget | undefined, send: (data: string) => void): () => void {
  if (!textarea) return () => {};
  terminals.set(textarea, send);
  return () => {
    if (terminals.get(textarea) === send) terminals.delete(textarea);
  };
}

/** A text field the guard may edit: not read-only or disabled. */
function isEditable(target: EventTarget | null): boolean {
  if (!isTextEntry(target)) return false;
  const el = target as { readOnly?: boolean; disabled?: boolean; isContentEditable?: boolean };
  return !!el.isContentEditable || (!el.readOnly && !el.disabled);
}

export type KeyOrderDeps = {
  target: Pick<Window, 'addEventListener' | 'removeEventListener' | 'dispatchEvent'>;
  doc: Pick<Document, 'execCommand'>;
  guard: KeyOrderGuard;
};

/**
 * Applies the guard's decisions on the window, in the capture phase, before
 * the page's own handlers and xterm. Returns a function that removes it.
 */
export function installKeyOrderGuard(overrides: Partial<KeyOrderDeps> = {}): () => void {
  const target = overrides.target ?? window;
  const doc = overrides.doc ?? document;
  const guard = overrides.guard ?? createKeyOrderGuard();

  const report = (action: 'repair' | 'drop', deletes: number) => {
    target.dispatchEvent(new CustomEvent(KEY_ORDER_GUARD_EVENT, { detail: { action, deletes } }));
  };

  const onKey = (event: Event) => {
    const e = event as KeyboardEvent;
    const send = terminals.get(e.target as EventTarget);
    // Only fields the guard can edit; elsewhere it just keeps count.
    if (!send && !isEditable(e.target)) {
      guard.handle(e);
      return;
    }
    const decision = guard.handle(e);
    if (decision.kind === 'pass') return;
    // stopPropagation, not stopImmediatePropagation: the keyboard recorder
    // listens on the window too and should still see the cancelled event.
    e.preventDefault();
    e.stopPropagation();
    if (decision.kind === 'drop') {
      report('drop', 1);
      return;
    }
    if (send) send('\x7f'.repeat(decision.deletes) + decision.text);
    else {
      for (let i = 0; i < decision.deletes; i++) doc.execCommand('delete');
      doc.execCommand('insertText', false, decision.text);
    }
    report('repair', decision.deletes);
  };
  const onBlur = () => guard.reset();

  target.addEventListener('keydown', onKey, true);
  target.addEventListener('keyup', onKey, true);
  target.addEventListener('blur', onBlur);
  return () => {
    target.removeEventListener('keydown', onKey, true);
    target.removeEventListener('keyup', onKey, true);
    target.removeEventListener('blur', onBlur);
  };
}
