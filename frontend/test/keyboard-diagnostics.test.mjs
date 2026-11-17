import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import {
  FLUSH_DELAY,
  KEYBOARD_LOG_STATE_EVENT,
  RECORDED_EVENTS,
  describeTarget,
  keyboardRecord,
  startKeyboardDiagnostics,
} from '../src/lib/keyboardDiagnostics.ts';

function page(html = '<textarea class="xterm-helper-textarea extra"></textarea><input type="password"><div contenteditable="true" class="ProseMirror"></div>') {
  return new JSDOM(`<!doctype html><body>${html}</body>`).window;
}

test('targets are described by tag, type, contenteditable and first class', () => {
  const win = page();
  const doc = win.document;
  assert.equal(describeTarget(doc.querySelector('textarea')), 'textarea.xterm-helper-textarea');
  assert.equal(describeTarget(doc.querySelector('input')), 'input[password]');
  const editable = doc.querySelector('div');
  // jsdom has no isContentEditable; WebKit does.
  Object.defineProperty(editable, 'isContentEditable', { value: true });
  assert.equal(describeTarget(editable), 'div[contenteditable].ProseMirror');
  assert.equal(describeTarget(win), 'window');
  assert.equal(describeTarget(null), '');
});

test('keyboard records carry the key fields and the time', () => {
  const win = page();
  const area = win.document.querySelector('textarea');
  const e = new win.KeyboardEvent('keydown', { key: 'â', code: 'KeyA', keyCode: 65, altKey: true, isComposing: false });
  Object.defineProperty(e, 'target', { value: area });
  Object.defineProperty(e, 'timeStamp', { value: 12.5 });
  const r = keyboardRecord(e, 1000, true);
  assert.deepEqual(r, {
    t: 1012.5, type: 'keydown', target: 'textarea.xterm-helper-textarea', focus: true,
    key: 'â', code: 'KeyA', keyCode: 65, repeat: false, isComposing: false, mods: 'alt',
  });
});

test('input and composition records carry data and input type, never the value', () => {
  const win = page();
  const area = win.document.querySelector('textarea');
  area.value = 'secret field text';
  const input = new win.InputEvent('beforeinput', { data: 'â', inputType: 'insertText' });
  Object.defineProperty(input, 'target', { value: area });
  const r = keyboardRecord(input, 0, false);
  assert.equal(r.data, 'â');
  assert.equal(r.inputType, 'insertText');
  assert.equal(r.isComposing, false);
  assert.ok(!JSON.stringify(r).includes('secret'));

  const comp = new win.CompositionEvent('compositionupdate', { data: 'aa' });
  Object.defineProperty(comp, 'target', { value: area });
  assert.equal(keyboardRecord(comp, 0, false).data, 'aa');
});

test('password inputs keep the event but drop the key and the text', () => {
  const win = page();
  const pw = win.document.querySelector('input');
  const key = new win.KeyboardEvent('keydown', { key: 'x', code: 'KeyX', keyCode: 88 });
  Object.defineProperty(key, 'target', { value: pw });
  const input = new win.InputEvent('input', { data: 'x', inputType: 'insertText' });
  Object.defineProperty(input, 'target', { value: pw });
  const k = keyboardRecord(key, 0, true);
  const i = keyboardRecord(input, 0, true);
  assert.equal(k.code, 'KeyX');
  assert.ok(!('key' in k));
  assert.ok(!('data' in i));
});

test('key order guard corrections are recorded with their action and delete count', () => {
  const win = page();
  assert.ok(RECORDED_EVENTS.includes('keyorderguard'));
  const e = new win.CustomEvent('keyorderguard', { detail: { action: 'repair', deletes: 4 } });
  Object.defineProperty(e, 'target', { value: win });
  Object.defineProperty(e, 'timeStamp', { value: 2 });
  assert.deepEqual(keyboardRecord(e, 1000, true), {
    t: 1002, type: 'keyorderguard', target: 'window', focus: true, action: 'repair', deletes: 4,
  });
});

// A fake desktop: records calls, delivers state events, runs timers by hand.
function harness({ enabled = false, failSend = false } = {}) {
  const win = page();
  const calls = [];
  const listeners = new Map();
  let stateCb;
  let timers = [];
  const deps = {
    call: async (method, ...args) => {
      calls.push([method, ...args]);
      if (method === 'KeyboardLogEnabled') return enabled;
      if (failSend) throw new Error('bridge gone');
      return undefined;
    },
    on: (name, cb) => {
      assert.equal(name, KEYBOARD_LOG_STATE_EVENT);
      stateCb = cb;
      return () => { stateCb = undefined; };
    },
    target: {
      addEventListener: (type, fn) => listeners.set(type, fn),
      removeEventListener: (type, fn) => { if (listeners.get(type) === fn) listeners.delete(type); },
    },
    doc: {
      addEventListener: (type, fn) => listeners.set(`doc:${type}`, fn),
      removeEventListener: (type, fn) => { if (listeners.get(`doc:${type}`) === fn) listeners.delete(`doc:${type}`); },
      hasFocus: () => true,
      visibilityState: 'visible',
    },
    timeOrigin: () => 1000,
    now: () => 5,
    setTimer: (fn, ms) => { const t = { fn, ms }; timers.push(t); return t; },
    clearTimer: (t) => { timers = timers.filter((x) => x !== t); },
  };
  const fire = (type, init = {}) => {
    const e = new win.KeyboardEvent(type, { key: 'a', code: 'KeyA', keyCode: 65, ...init });
    Object.defineProperty(e, 'target', { value: win.document.querySelector('textarea') });
    listeners.get(type)?.(e);
  };
  const runTimers = () => { const due = timers; timers = []; for (const t of due) t.fn(); };
  return {
    deps, calls, listeners, fire, runTimers,
    state: (on) => stateCb?.(on),
    get timers() { return timers; },
    sends: () => calls.filter(([m]) => m === 'RecordKeyboardEvents'),
  };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test('nothing is attached while recording is off', async () => {
  const h = harness();
  const stop = startKeyboardDiagnostics(h.deps);
  await settle();
  assert.deepEqual(h.calls, [['KeyboardLogEnabled']]);
  assert.equal(h.listeners.size, 0);
  stop();
});

test('records are batched while on and dropped once off', async () => {
  const h = harness();
  const stop = startKeyboardDiagnostics(h.deps);
  await settle();
  h.state(true);
  assert.deepEqual([...h.listeners.keys()].sort(), [...RECORDED_EVENTS, 'doc:visibilitychange'].sort());

  h.fire('keydown');
  h.fire('keyup');
  assert.equal(h.timers.length, 1, 'one timer per batch');
  assert.equal(h.timers[0].ms, FLUSH_DELAY);
  h.runTimers();
  const sends = h.sends();
  assert.equal(sends.length, 1);
  assert.deepEqual(sends[0][1].map((r) => r.type), ['keydown', 'keyup']);

  // Turning off detaches and drops what was not sent.
  h.fire('keydown');
  h.state(false);
  assert.equal(h.listeners.size, 0);
  assert.equal(h.timers.length, 0);
  h.runTimers();
  assert.equal(h.sends().length, 1);
  stop();
});

test('a window opened while recording joins in, and a failed send is dropped quietly', async () => {
  const h = harness({ enabled: true, failSend: true });
  const stop = startKeyboardDiagnostics(h.deps);
  await settle();
  assert.ok(h.listeners.has('keydown'));
  h.fire('keydown');
  h.runTimers();
  await settle();
  assert.equal(h.sends().length, 1);
  stop();
  assert.equal(h.listeners.size, 0, 'stop detaches');
});

test('a state event that beats the startup query wins', async () => {
  const h = harness({ enabled: true });
  const stop = startKeyboardDiagnostics(h.deps);
  h.state(false); // arrives before the KeyboardLogEnabled reply
  await settle();
  assert.equal(h.listeners.size, 0);
  stop();
});
