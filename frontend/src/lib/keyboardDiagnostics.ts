// Keyboard diagnostics (docs/plan/keyboard-diagnostics.md): while the user
// records from the macOS Help menu, the page's keyboard, input and
// composition events go to the Go recorder, next to AppKit's own, so a lost
// input method replacement shows where it got lost. Nothing is attached
// while recording is off, and field values are never read.
import { callApp, onDesktopEvent } from './desktopRuntime';

/** The Go recorder's state, a bool, sent to every window on each change. */
export const KEYBOARD_LOG_STATE_EVENT = 'keyboard-log:state';

/** Events recorded, all in the capture phase on the window. */
export const RECORDED_EVENTS = [
  'keydown',
  'keyup',
  'keypress',
  'beforeinput',
  'input',
  'compositionstart',
  'compositionupdate',
  'compositionend',
  'focus',
  'blur',
] as const;

/** How long records wait before they are sent, in milliseconds. */
export const FLUSH_DELAY = 250;

export type KeyboardRecord = Record<string, unknown>;

type ElementLike = {
  tagName?: unknown;
  className?: unknown;
  isContentEditable?: boolean;
  getAttribute?: (name: string) => string | null;
};

/** A short name for an event target: tag, type, contenteditable, first class. */
export function describeTarget(target: EventTarget | null): string {
  const el = target as ElementLike | null;
  if (!el || typeof el.tagName !== 'string') {
    if (target && typeof (target as Window).document === 'object') return 'window';
    return target ? 'document' : '';
  }
  let out = el.tagName.toLowerCase();
  const type = el.getAttribute?.('type');
  if (type) out += `[${type}]`;
  if (el.isContentEditable) out += '[contenteditable]';
  const cls = typeof el.className === 'string' ? el.className.trim().split(/\s+/)[0] : '';
  if (cls) out += `.${cls}`;
  return out;
}

function isPassword(target: EventTarget | null): boolean {
  const el = target as ElementLike | null;
  return typeof el?.tagName === 'string' && el.tagName.toUpperCase() === 'INPUT'
    && (el.getAttribute?.('type') ?? '').toLowerCase() === 'password';
}

function modifiers(e: KeyboardEvent): string {
  const mods: string[] = [];
  if (e.metaKey) mods.push('meta');
  if (e.ctrlKey) mods.push('ctrl');
  if (e.altKey) mods.push('alt');
  if (e.shiftKey) mods.push('shift');
  return mods.join('+');
}

/**
 * The record of one event. t is epoch milliseconds: timeOrigin plus the
 * event's timeStamp. Inside password inputs the typed key and text are left
 * out.
 */
export function keyboardRecord(e: Event, timeOrigin: number, focused: boolean): KeyboardRecord {
  const r: KeyboardRecord = {
    t: timeOrigin + e.timeStamp,
    type: e.type,
    target: describeTarget(e.target),
    focus: focused,
  };
  const secret = isPassword(e.target);
  const any = e as unknown as Record<string, unknown>;
  if ('code' in e && 'keyCode' in e) {
    const k = e as KeyboardEvent;
    if (!secret) r.key = k.key;
    r.code = k.code;
    r.keyCode = k.keyCode;
    r.repeat = k.repeat;
    r.isComposing = k.isComposing;
    const mods = modifiers(k);
    if (mods) r.mods = mods;
  }
  if ('inputType' in e) {
    r.inputType = any.inputType;
    r.isComposing = any.isComposing;
  }
  if ('data' in e && !secret) r.data = any.data;
  if (e.defaultPrevented) r.prevented = true;
  return r;
}

export type KeyboardDiagnosticsDeps = {
  call: (method: string, ...args: unknown[]) => Promise<unknown>;
  on: (name: string, callback: (data: unknown) => void) => () => void;
  target: Pick<Window, 'addEventListener' | 'removeEventListener'>;
  doc: Pick<Document, 'addEventListener' | 'removeEventListener' | 'hasFocus' | 'visibilityState'>;
  timeOrigin: () => number;
  now: () => number;
  setTimer: (fn: () => void, ms: number) => unknown;
  clearTimer: (handle: unknown) => void;
};

// Each default is read only when not overridden: tests run without a DOM.
function withDefaults(o: Partial<KeyboardDiagnosticsDeps>): KeyboardDiagnosticsDeps {
  return {
    call: o.call ?? callApp,
    on: o.on ?? onDesktopEvent,
    target: o.target ?? window,
    doc: o.doc ?? document,
    timeOrigin: o.timeOrigin ?? (() => performance.timeOrigin),
    now: o.now ?? (() => performance.now()),
    setTimer: o.setTimer ?? ((fn, ms) => setTimeout(fn, ms)),
    clearTimer: o.clearTimer ?? ((handle) => clearTimeout(handle as ReturnType<typeof setTimeout>)),
  };
}

/**
 * Follows the Go recorder's state and records the page's events while it is
 * on. Returns a function that stops.
 */
export function startKeyboardDiagnostics(overrides: Partial<KeyboardDiagnosticsDeps> = {}): () => void {
  const deps = withDefaults(overrides);
  let on = false;
  let heard = false; // a state event beat the startup query
  let queue: KeyboardRecord[] = [];
  let timer: unknown;

  const flush = () => {
    timer = undefined;
    const batch = queue;
    queue = [];
    if (!on || batch.length === 0) return;
    // A diagnostic never disturbs the page: a failed send loses its batch.
    deps.call('RecordKeyboardEvents', batch).catch(() => {});
  };
  const push = (record: KeyboardRecord) => {
    queue.push(record);
    if (timer === undefined) timer = deps.setTimer(flush, FLUSH_DELAY);
  };
  const listener = (e: Event) => {
    push(keyboardRecord(e, deps.timeOrigin(), deps.doc.hasFocus()));
  };
  const visibility = () => {
    push({ t: deps.timeOrigin() + deps.now(), type: 'visibilitychange', state: deps.doc.visibilityState, focus: deps.doc.hasFocus() });
  };
  const options = { capture: true, passive: true };

  const set = (next: boolean) => {
    if (next === on) return;
    on = next;
    if (on) {
      for (const type of RECORDED_EVENTS) deps.target.addEventListener(type, listener, options);
      deps.doc.addEventListener('visibilitychange', visibility, options);
      return;
    }
    for (const type of RECORDED_EVENTS) deps.target.removeEventListener(type, listener, options);
    deps.doc.removeEventListener('visibilitychange', visibility, options);
    if (timer !== undefined) deps.clearTimer(timer);
    timer = undefined;
    queue = [];
  };

  const off = deps.on(KEYBOARD_LOG_STATE_EVENT, (data) => {
    heard = true;
    set(data === true);
  });
  deps.call('KeyboardLogEnabled').then(
    (enabled) => { if (!heard) set(enabled === true); },
    () => {},
  );
  return () => {
    off();
    set(false);
  };
}
