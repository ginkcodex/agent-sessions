// Update checks: once a day, shared by every window, and a toast that opens
// the release page.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  CHECK_INTERVAL_MS,
  RETRY_MS,
  STARTUP_DELAY_MS,
  UPDATE_CHECK_KEY,
  checkDelay,
  startUpdateChecks,
  updateToast,
} from '../src/lib/update.ts';

const newer = { current: '1.2.3', latest: '1.3.0', url: 'https://github.com/ginkcodex/agent-sessions/releases/tag/v1.3.0', available: true };
const same = { ...newer, latest: '1.2.3', available: false };

function memoryStorage() {
  const items = new Map();
  return {
    items,
    getItem: (key) => (items.has(key) ? items.get(key) : null),
    setItem: (key, value) => items.set(key, String(value)),
  };
}

// A clock and timers the test moves by hand.
function fakeTime(start = 1_000_000) {
  const clock = { now: start, timers: [] };
  clock.setTimer = (fn, ms) => {
    const timer = { fn, at: clock.now + ms };
    clock.timers.push(timer);
    return timer;
  };
  clock.clearTimer = (timer) => {
    clock.timers = clock.timers.filter((t) => t !== timer);
  };
  // Runs the next timer and lets the check it starts finish.
  clock.next = async () => {
    clock.timers.sort((a, b) => a.at - b.at);
    const timer = clock.timers.shift();
    assert.ok(timer, 'a check is scheduled');
    clock.now = Math.max(clock.now, timer.at);
    timer.fn();
    for (let i = 0; i < 5; i++) await Promise.resolve();
    return timer;
  };
  return clock;
}

function start(clock, storage, check, shown = []) {
  const stop = startUpdateChecks({
    check,
    notify: (info) => shown.push(info),
    storage,
    now: () => clock.now,
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer,
  });
  return { stop, shown };
}

test('the next check waits for startup, then for its due time, and never more than a day', () => {
  assert.equal(checkDelay(null, 0), STARTUP_DELAY_MS);
  assert.equal(checkDelay(100, 5000), STARTUP_DELAY_MS, 'overdue');
  assert.equal(checkDelay(5 * 60_000, 0), 5 * 60_000);
  assert.equal(checkDelay(0 + 30 * CHECK_INTERVAL_MS, 0), CHECK_INTERVAL_MS, 'the clock went back');
});

test('the toast names both versions, stays up, and opens the release page', () => {
  let opened = 0;
  const message = updateToast(newer, () => opened++);
  assert.equal(message.title, 'Update available');
  assert.match(message.body, /Agent Sessions 1\.3\.0 is out; you have 1\.2\.3\./);
  assert.equal(message.persistent, true);
  assert.deepEqual(message.actions.map((a) => a.label), ['Open release page']);
  message.actions[0].onSelect();
  assert.equal(opened, 1);
});

test('a newer release is shown after startup, and the next check is a day later', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  let checks = 0;
  const { stop, shown } = start(clock, storage, async () => (checks++, newer));

  const first = await clock.next();
  assert.equal(first.at, 1_000_000 + STARTUP_DELAY_MS);
  assert.equal(checks, 1);
  assert.deepEqual(shown, [newer]);
  assert.equal(Number(storage.getItem(UPDATE_CHECK_KEY)), clock.now + CHECK_INTERVAL_MS);
  assert.equal(clock.timers[0].at, clock.now + CHECK_INTERVAL_MS);
  stop();
  assert.equal(clock.timers.length, 0);
});

test('being up to date shows nothing', async () => {
  const clock = fakeTime();
  const { stop, shown } = start(clock, memoryStorage(), async () => same);
  await clock.next();
  assert.deepEqual(shown, []);
  stop();
});

test('a restart within the day does not check again', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  storage.setItem(UPDATE_CHECK_KEY, String(clock.now + 6 * 60 * 60 * 1000));
  let checks = 0;
  const { stop } = start(clock, storage, async () => (checks++, same));
  assert.equal(clock.timers[0].at, clock.now + 6 * 60 * 60 * 1000);
  await clock.next();
  assert.equal(checks, 1);
  stop();
});

test('one window checks for all: another window waits for its due time', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  let checks = 0;
  const check = async () => (checks++, same);
  const a = start(clock, storage, check);
  const b = start(clock, storage, check);
  await clock.next(); // a checks
  await clock.next(); // b finds it done and waits
  assert.equal(checks, 1);
  assert.deepEqual(clock.timers.map((t) => t.at), [clock.now + CHECK_INTERVAL_MS, clock.now + CHECK_INTERVAL_MS]);
  a.stop();
  b.stop();
});

test('a failed check is tried again in an hour', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  let fail = true;
  const { stop, shown } = start(clock, storage, async () => {
    if (fail) throw new Error('offline');
    return newer;
  });
  await clock.next();
  assert.equal(Number(storage.getItem(UPDATE_CHECK_KEY)), clock.now + RETRY_MS);
  fail = false;
  const retry = await clock.next();
  assert.equal(retry.at, 1_000_000 + STARTUP_DELAY_MS + RETRY_MS);
  assert.deepEqual(shown, [newer]);
  stop();
});

test('a window closed while checking shows nothing', async () => {
  const clock = fakeTime();
  let finish;
  const { stop, shown } = start(clock, memoryStorage(), () => new Promise((resolve) => (finish = resolve)));
  await clock.next();
  stop();
  finish(newer);
  for (let i = 0; i < 5; i++) await Promise.resolve();
  assert.deepEqual(shown, []);
  assert.equal(clock.timers.length, 0);
});

test('without storage a window keeps its own schedule', async () => {
  const clock = fakeTime();
  const broken = { getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); } };
  let checks = 0;
  const { stop } = start(clock, broken, async () => (checks++, same));
  await clock.next();
  assert.equal(checks, 1);
  assert.equal(clock.timers[0].at, clock.now + CHECK_INTERVAL_MS);
  stop();
});

test('only the desktop app checks, through the backend, and opens the page in the browser', () => {
  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
  assert.match(app, /const check = api\.checkForUpdate\?\.bind\(api\);\s*if \(!isWails \|\| !check\) return \(\) => \{\};/);
  assert.match(app, /notify: \(info\) => toast\.show\(updateToast\(info, \(\) => openReleasePage\(info\)\)\)/);
  assert.match(app, /api\.openURL\(info\.url\)/);
  assert.match(app, /const stopUpdateChecks = checkForUpdates\(\);/);
  assert.match(app, /return \(\) => \{\s*stopUpdateChecks\(\);/);
  const api = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8');
  assert.match(api, /checkForUpdate\?\(\): Promise<UpdateInfo>;/, 'the browser demo has no updates');
  assert.match(api, /return await this\.binding\.CheckForUpdate\(\);/);
});
