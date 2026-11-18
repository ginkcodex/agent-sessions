import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  KEY_ORDER_GUARD_EVENT,
  KEY_ORDER_WINDOW,
  createKeyOrderGuard,
  installKeyOrderGuard,
  registerKeyOrderTerminal,
  typedText,
} from '../src/lib/keyOrderGuard.ts';

// The page's keydown/keyup order from the KD-04 fault log (app 1.6.9, chat
// composer): OpenKey's Backspace keydowns arrive after its replacement text.
const fixture = JSON.parse(readFileSync(new URL('./fixtures/openkey-reordered.json', import.meta.url), 'utf8'));

const SP = '\u202F'; // OpenKey's browser-mode spacer

const key = (type, k, overrides = {}) => ({
  type, key: k, keyCode: k === 'Backspace' ? 8 : 65, repeat: false, isComposing: false,
  ctrlKey: false, metaKey: false, altKey: false, ...overrides,
});
const fromFixture = (e) => key(e.type, e.key, {
  keyCode: e.keyCode, metaKey: /meta/.test(e.mods ?? ''), shiftKey: /shift/.test(e.mods ?? ''),
});

/** A text field that applies keydowns the way WebKit's default actions would. */
function field() {
  let value = '';
  let all = false;
  const del = () => {
    value = all ? '' : Array.from(value).slice(0, -1).join('');
    all = false;
  };
  const insert = (text) => {
    value = (all ? '' : value) + text;
    all = false;
  };
  return {
    get value() { return value; },
    apply(e, decision = { kind: 'pass' }) {
      if (decision.kind === 'drop') return;
      if (decision.kind === 'repair') {
        for (let i = 0; i < decision.deletes; i++) del();
        insert(decision.text);
        return;
      }
      if (e.type !== 'keydown') return;
      if (e.metaKey && e.key === 'a') { all = true; return; }
      if (e.key === 'Backspace') { del(); return; }
      const text = typedText(e);
      if (text !== null) insert(text);
    },
  };
}

/** A clock that moves 1 ms per event, as a fast burst would. */
function ticking() {
  let t = 0;
  return { now: () => t, tick: (ms = 1) => { t += ms; } };
}

const visible = (s) => s.replaceAll('\u202F', '');

test('the fault log replays to broken text without the guard and to the typed text with it', () => {
  const broken = field();
  for (const e of fixture.events) broken.apply(fromFixture(e));
  assert.notEqual(visible(broken.value), fixture.native);

  const clock = ticking();
  const guard = createKeyOrderGuard(clock.now);
  const fixed = field();
  let repairs = 0;
  for (const raw of fixture.events) {
    const e = fromFixture(raw);
    const decision = guard.handle(e);
    if (decision.kind === 'repair') repairs++;
    fixed.apply(e, decision);
    clock.tick();
  }
  assert.equal(visible(fixed.value), fixture.native);
  assert.ok(repairs >= 18, `expected a repair for each reordered word, got ${repairs}`);
});

test('one reordered burst: deletes go before the text and the late keydowns are dropped', () => {
  const guard = createKeyOrderGuard(() => 0);
  const f = field();
  const run = (events) => events.map((e) => { const d = guard.handle(e); f.apply(e, d); return d.kind; });
  run('cong'.split('').flatMap((c) => [key('keydown', c), key('keyup', c)]));
  const kinds = run([
    key('keydown', SP), key('keyup', 'a'),
    ...Array(4).fill(key('keyup', 'Backspace')),
    key('keydown', 'ông'), key('keyup', 'a'),
    ...Array(4).fill(key('keydown', 'Backspace')),
  ]);
  assert.deepEqual(kinds, ['pass', 'pass', 'pass', 'pass', 'pass', 'pass', 'repair', 'pass', 'drop', 'drop', 'drop', 'drop']);
  assert.equal(f.value, 'công');
});

test('Backspace keydowns that are late but still before the text keep their place', () => {
  // From the log's second word: two keyups early, three keydowns, then the
  // text with one keydown still to come.
  const guard = createKeyOrderGuard(() => 0);
  const f = field();
  for (const c of 'cong') { f.apply(key('keydown', c), guard.handle(key('keydown', c))); guard.handle(key('keyup', c)); }
  const events = [
    key('keydown', SP), key('keyup', 'a'),
    key('keyup', 'Backspace'), key('keyup', 'Backspace'),
    key('keydown', 'Backspace'), key('keydown', 'Backspace'), key('keydown', 'Backspace'),
    key('keyup', 'Backspace'), key('keyup', 'Backspace'),
    key('keydown', 'ộng'), key('keyup', 'a'),
    key('keydown', 'Backspace'),
  ];
  for (const e of events) f.apply(e, guard.handle(e));
  assert.equal(f.value, 'cộng');
});

test('typing in the normal order is never touched', () => {
  const guard = createKeyOrderGuard(() => 0);
  const kinds = new Set();
  const press = (k, o) => { kinds.add(guard.handle(key('keydown', k, o)).kind); kinds.add(guard.handle(key('keyup', k, o)).kind); };
  for (const c of 'hello ') press(c);
  press('Backspace');
  // A held Backspace repeats keydowns before one keyup.
  guard.handle(key('keydown', 'Backspace'));
  for (let i = 0; i < 5; i++) kinds.add(guard.handle(key('keydown', 'Backspace', { repeat: true })).kind);
  guard.handle(key('keyup', 'Backspace'));
  // OpenKey's burst in the order it was sent.
  for (const e of [key('keydown', SP), key('keyup', 'a'), key('keydown', 'Backspace'), key('keyup', 'Backspace'), key('keydown', 'â'), key('keyup', 'a')]) {
    kinds.add(guard.handle(e).kind);
  }
  for (const c of 'world') press(c);
  assert.deepEqual([...kinds], ['pass']);
});

test('a stray early keyup expires and never deletes later text', () => {
  const clock = ticking();
  const guard = createKeyOrderGuard(clock.now);
  guard.handle(key('keyup', 'Backspace')); // released after switching windows
  clock.tick(KEY_ORDER_WINDOW + 1);
  assert.equal(guard.handle(key('keydown', 'a')).kind, 'pass');
  assert.equal(guard.handle(key('keydown', 'Backspace')).kind, 'pass');

  const owed = createKeyOrderGuard(clock.now);
  owed.handle(key('keyup', 'Backspace'));
  assert.equal(owed.handle(key('keydown', 'â')).kind, 'repair');
  clock.tick(KEY_ORDER_WINDOW + 1);
  // The late keydown never came; the user's own Backspace must still work.
  assert.equal(owed.handle(key('keydown', 'Backspace')).kind, 'pass');
});

test('reset clears the counts, and the spacer, shortcuts and named keys never start a repair', () => {
  const guard = createKeyOrderGuard(() => 0);
  guard.handle(key('keyup', 'Backspace'));
  guard.reset();
  assert.equal(guard.handle(key('keydown', 'a')).kind, 'pass');

  guard.handle(key('keyup', 'Backspace'));
  for (const e of [
    key('keydown', SP),
    key('keydown', 'a', { metaKey: true }),
    key('keydown', 'Enter', { keyCode: 13 }),
    key('keydown', 'ArrowLeft', { keyCode: 37 }),
    key('keydown', 'Process', { keyCode: 229 }),
    key('keydown', 'ô', { isComposing: true }),
  ]) assert.equal(guard.handle(e).kind, 'pass', e.key);
  assert.equal(guard.handle(key('keydown', 'ô')).kind, 'repair');
});

test('typedText keeps text and leaves named keys alone', () => {
  for (const k of ['a', ' ', 'â', 'ộng', 'Cộng hòa', '😀']) assert.equal(typedText(key('keydown', k)), k);
  assert.equal(typedText(key('keydown', 'ng', { keyCode: 78 })), 'ng');
  for (const [k, code] of [['Enter', 13], ['Backspace', 8], ['Shift', 16], ['Meta', 91], ['F1', 112], ['Dead', 69], ['Unidentified', 0]]) {
    assert.equal(typedText(key('keydown', k, { keyCode: code })), null, k);
  }
});

/** A window stand-in that runs capture listeners the way the guard uses them. */
function fakeWindow() {
  const listeners = new Map();
  const fired = [];
  return {
    fired,
    addEventListener(type, fn) { listeners.set(type, [...(listeners.get(type) ?? []), fn]); },
    removeEventListener(type, fn) { listeners.set(type, (listeners.get(type) ?? []).filter((x) => x !== fn)); },
    dispatchEvent(e) { fired.push(e); return true; },
    send(type, props) {
      const e = {
        ...key(type, props.key, props), target: props.target, prevented: false, stopped: false,
        preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; },
      };
      for (const fn of listeners.get(type) ?? []) fn(e);
      return e;
    },
    blur() { for (const fn of listeners.get('blur') ?? []) fn(); },
  };
}

const reorderedBurst = (win, target, text) => [
  win.send('keydown', { key: SP, target }),
  win.send('keyup', { key: 'a', target }),
  win.send('keyup', { key: 'Backspace', target }),
  win.send('keyup', { key: 'Backspace', target }),
  win.send('keydown', { key: text, target }),
  win.send('keydown', { key: 'Backspace', target }),
  win.send('keydown', { key: 'Backspace', target }),
];

test('a terminal gets DELs before the text, and the late Backspaces never reach xterm', () => {
  const win = fakeWindow();
  const textarea = { tagName: 'TEXTAREA' };
  const sent = [];
  const unregister = registerKeyOrderTerminal(textarea, (data) => sent.push(data));
  const stop = installKeyOrderGuard({ target: win, doc: { execCommand: () => assert.fail('no execCommand for terminals') } });
  const events = reorderedBurst(win, textarea, 'ông');
  assert.deepEqual(sent, ['\x7f\x7fông']);
  assert.deepEqual(events.map((e) => e.prevented), [false, false, false, false, true, true, true]);
  assert.ok(events.slice(4).every((e) => e.stopped));
  assert.deepEqual(win.fired.map((e) => [e.type, e.detail.action, e.detail.deletes]), [
    [KEY_ORDER_GUARD_EVENT, 'repair', 2], [KEY_ORDER_GUARD_EVENT, 'drop', 1], [KEY_ORDER_GUARD_EVENT, 'drop', 1],
  ]);
  stop();
  unregister();
});

test('a text field gets the deletes and the text through execCommand', () => {
  const win = fakeWindow();
  const commands = [];
  const stop = installKeyOrderGuard({ target: win, doc: { execCommand: (...args) => { commands.push(args); return true; } } });
  reorderedBurst(win, { tagName: 'TEXTAREA' }, 'ông');
  assert.deepEqual(commands, [['delete'], ['delete'], ['insertText', false, 'ông']]);

  // Read-only fields, buttons and other targets are left alone.
  commands.length = 0;
  for (const target of [{ tagName: 'TEXTAREA', readOnly: true }, { tagName: 'BUTTON' }, { tagName: 'INPUT', getAttribute: (name) => (name === 'type' ? 'checkbox' : null) }]) {
    const events = reorderedBurst(win, target, 'ông');
    assert.ok(events.every((e) => !e.prevented));
    win.blur();
  }
  assert.deepEqual(commands, []);
  stop();
});

test('window blur forgets an early keyup', () => {
  const win = fakeWindow();
  const stop = installKeyOrderGuard({ target: win, doc: { execCommand: () => assert.fail('nothing to repair') } });
  const target = { tagName: 'TEXTAREA' };
  win.send('keyup', { key: 'Backspace', target });
  win.blur();
  assert.equal(win.send('keydown', { key: 'â', target }).prevented, false);
  stop();
});
