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
  createUpdateAnnouncer,
  INSTALL_GUIDE_URL,
  linuxUpdateToast,
  pageToast,
  readyToast,
  updateSummary,
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

function start(clock, storage, check, shown = [], checked = []) {
  const checks = startUpdateChecks({
    check,
    notify: (info) => shown.push(info),
    checked: (failed) => checked.push(failed),
    storage,
    now: () => clock.now,
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer,
  });
  return { stop: checks.stop, checkNow: checks.checkNow, shown, checked };
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
  assert.match(app, /announcer\.announce\(status, info\)/);
  assert.match(app, /api\.openURL\(url\)/);
  assert.match(app, /updates\.init\(\(status\) => announcer\.announce\(status\)\)/);
  assert.match(app, /const stopUpdateChecks = checkForUpdates\(\);/);
  assert.match(app, /return \(\) => \{\s*stopUpdateChecks\(\);/);
  assert.match(app, /updates\.setChecker\(\(\) => checks\.checkNow\(\)\);/, 'Settings checks through the window\'s scheduler');
  assert.match(app, /stopMenuChecks\(\);\s*updates\.setChecker\(null\);\s*checks\.stop\(\);/);
  const api = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8');
  assert.match(api, /checkForUpdate\?\(\): Promise<UpdateInfo>;/, 'the browser demo has no updates');
  assert.match(api, /return await this\.binding\.CheckForUpdate\(\);/);
});

function announcer() {
  const log = { shown: [], restarts: 0, opened: [], copied: [] };
  const a = createUpdateAnnouncer({
    show: (m) => log.shown.push(m),
    restart: () => log.restarts++,
    openPage: (url) => log.opened.push(url),
    copy: (command) => log.copied.push(command),
  });
  return { log, ...a };
}

test('a ready update gets a persistent toast with Restart now and Later', () => {
  const m = readyToast('1.6.0', () => {});
  assert.equal(m.persistent, true);
  assert.match(m.body, /1\.6\.0/);
  assert.deepEqual(m.actions.map((a) => a.label), ['Restart now', 'Later']);
});

test('ready: the toast shows once per version; Restart now restarts, Later does nothing', () => {
  const { log, announce } = announcer();
  announce({ state: 'ready', version: '1.6.0', url: 'u' });
  announce({ state: 'ready', version: '1.6.0', url: 'u' });
  assert.equal(log.shown.length, 1, 'repeated events do not bring the toast back after Later');
  log.shown[0].actions[1].onSelect();
  assert.equal(log.restarts, 0);
  log.shown[0].actions[0].onSelect();
  assert.equal(log.restarts, 1);
  announce({ state: 'ready', version: '1.6.1', url: 'u' });
  assert.equal(log.shown.length, 2, 'a newer version is announced again');
});

test('downloading shows nothing; idle shows nothing unless a check just found a release', () => {
  const { log, announce } = announcer();
  announce({ state: 'downloading', version: '1.6.0' });
  announce({ state: 'idle' });
  assert.equal(log.shown.length, 0);
  // A failed download is retried at the next check; the page toast still helps.
  announce({ state: 'idle' }, newer);
  assert.equal(log.shown.length, 1);
  assert.equal(log.shown[0].title, 'Update available');
});

test('manual and unsupported keep the release page toast', () => {
  for (const state of ['manual', 'unsupported']) {
    const { log, announce } = announcer();
    announce({ state, version: '1.3.0', url: newer.url }, newer);
    assert.equal(log.shown.length, 1, state);
    assert.deepEqual(log.shown[0].actions.map((a) => a.label), ['Open release page']);
    log.shown[0].actions[0].onSelect();
    assert.deepEqual(log.opened, [newer.url]);
  }
});

const linuxCommands = [
  ['apt-repo', 'sudo apt update && sudo apt upgrade', /with apt:/],
  ['dnf-repo', 'sudo dnf upgrade', /with dnf:/],
  ['aur', 'paru -Syu agent-sessions-bin', /AUR helper/],
];

test('Linux repository and AUR installs: the toast shows the command and copies it', () => {
  for (const [install, command, how] of linuxCommands) {
    const { log, announce } = announcer();
    announce({ state: 'unsupported', version: '1.3.0', url: newer.url, install }, { ...newer, install });
    assert.equal(log.shown.length, 1, install);
    const m = log.shown[0];
    assert.equal(m.title, 'Update available');
    assert.equal(m.persistent, true);
    assert.match(m.body, /Agent Sessions 1\.3\.0 is out; you have 1\.2\.3\./);
    assert.match(m.body, how);
    assert.equal(m.code, command);
    assert.deepEqual(m.actions.map((a) => a.label), ['Copy command']);
    m.actions[0].onSelect();
    assert.deepEqual(log.copied, [command]);
    assert.deepEqual(log.opened, [], 'no release page');
  }
});

test('a Linux package installed by hand: the toast opens the install guide or the release page', () => {
  const { log, announce } = announcer();
  announce({ state: 'unsupported', version: '1.3.0', url: newer.url, install: 'manual' }, { ...newer, install: 'manual' });
  const m = log.shown[0];
  assert.equal(m.code, undefined);
  assert.deepEqual(m.actions.map((a) => a.label), ['Open install guide', 'Open release page']);
  m.actions[0].onSelect();
  m.actions[1].onSelect();
  assert.deepEqual(log.opened, [INSTALL_GUIDE_URL, newer.url]);
  assert.equal(INSTALL_GUIDE_URL, 'https://agent-sessions.ginklab.cv/download');
});

test('Linux: the install method in the update state alone is enough, and shows once', () => {
  const { log, announce } = announcer();
  announce({ state: 'unsupported', version: '1.3.0', url: newer.url, install: 'dnf-repo' });
  announce({ state: 'unsupported', version: '1.3.0', url: newer.url, install: 'dnf-repo' });
  assert.equal(log.shown.length, 1);
  assert.match(log.shown[0].body, /^Agent Sessions 1\.3\.0 is out\. Update it with dnf:$/);
  // A check that found it while idle (a download that failed elsewhere) too.
  announce({ state: 'idle', install: 'apt-repo' }, newer);
  assert.equal(log.shown[1].code, 'sudo apt update && sudo apt upgrade');
});

test('macOS and Windows toasts are unchanged: no install method, no Linux toast', () => {
  assert.equal(linuxUpdateToast(undefined, '1.3.0', '1.2.3', { copy() {}, openPage() {}, releasePage: '' }), null);
  for (const state of ['manual', 'unsupported', 'idle']) {
    const { log, announce } = announcer();
    announce({ state, version: '1.3.0', url: newer.url }, newer);
    assert.deepEqual(log.shown[0].actions.map((a) => a.label), ['Open release page'], state);
    assert.equal(log.shown[0].code, undefined);
  }
  const { log, announce } = announcer();
  announce({ state: 'ready', version: '1.6.0', url: 'u', install: 'apt-repo' });
  assert.deepEqual(log.shown[0].actions.map((a) => a.label), ['Restart now', 'Later'], 'a ready update is never a Linux toast');
});

test('the desktop app copies the Linux command through the native clipboard', () => {
  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
  assert.match(app, /copy: \(command\) =>\s*void copyToClipboard\(command\)/);
});

test('a state change to manual without a check still shows the page, once', () => {
  const { log, announce } = announcer();
  announce({ state: 'manual', version: '1.3.0', url: newer.url });
  announce({ state: 'manual', version: '1.3.0', url: newer.url });
  assert.equal(log.shown.length, 1);
  assert.match(log.shown[0].body, /1\.3\.0/);
  log.shown[0].actions[0].onSelect();
  assert.deepEqual(log.opened, [newer.url]);
  const m = pageToast({ state: 'manual' }, () => {});
  assert.doesNotMatch(m.body, /undefined/);
});

test('Settings says where the update is', () => {
  assert.equal(updateSummary({ state: 'idle' }, false), 'Up to date');
  assert.equal(updateSummary({ state: 'idle' }, true), 'Last check failed');
  assert.equal(updateSummary({ state: 'ready', version: '1.6.0' }, false), 'Ready: restart to install 1.6.0');
  assert.equal(updateSummary({ state: 'downloading', version: '1.6.0' }, true), 'Downloading 1.6.0…');
  assert.equal(updateSummary({ state: 'manual', version: '1.6.0' }, false), '1.6.0 is available');
  assert.equal(updateSummary({ state: 'unsupported', version: '1.6.0' }, false), '1.6.0 is available');
});

test('checks report whether they failed', async () => {
  const clock = fakeTime();
  const results = [];
  let fail = true;
  startUpdateChecks({
    check: async () => { if (fail) throw new Error('offline'); return same; },
    notify() {},
    checked: (failed) => results.push(failed),
    storage: memoryStorage(),
    now: () => clock.now,
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer,
  });
  await clock.next();
  fail = false;
  await clock.next();
  assert.deepEqual(results, [true, false]);
});

const settle = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve();
};

test('checking by hand runs at once and counts as the day\'s check in every window', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  let checks = 0;
  const check = async () => (checks++, newer);
  const a = start(clock, storage, check);
  const b = start(clock, storage, check);

  clock.now += 1000; // before the startup check
  assert.deepEqual(await a.checkNow(), newer);
  assert.equal(checks, 1);
  assert.deepEqual(a.shown, [newer], 'announced as the daily check does');
  assert.deepEqual(a.checked, [false]);
  const nextDay = clock.now + CHECK_INTERVAL_MS;
  assert.equal(Number(storage.getItem(UPDATE_CHECK_KEY)), nextDay);
  // This window's startup timer was replaced by one a day away.
  assert.equal(clock.timers.filter((t) => t.at === nextDay).length, 1);

  // The other window's startup check finds it done and waits for the day.
  await clock.next();
  assert.equal(checks, 1);
  assert.deepEqual(clock.timers.map((t) => t.at), [nextDay, nextDay]);
  a.stop();
  b.stop();
});

test('checking by hand during a check shares it', async () => {
  const clock = fakeTime();
  let finish;
  let checks = 0;
  const { stop, checkNow } = start(clock, memoryStorage(), () => (checks++, new Promise((resolve) => (finish = resolve))));
  void clock.next(); // the startup check begins
  await settle();
  const first = checkNow();
  const second = checkNow();
  finish(same);
  assert.deepEqual(await first, same);
  assert.deepEqual(await second, same);
  assert.equal(checks, 1);
  assert.equal(clock.timers.length, 1, 'one timer, a day away');
  assert.equal(clock.timers[0].at, clock.now + CHECK_INTERVAL_MS);
  stop();
});

test('a failed check by hand keeps the schedule and reports the failure', async () => {
  const clock = fakeTime();
  const storage = memoryStorage();
  const due = clock.now + 6 * 60 * 60 * 1000;
  storage.setItem(UPDATE_CHECK_KEY, String(due));
  const { stop, checkNow, shown, checked } = start(clock, storage, async () => { throw new Error('offline'); });
  await assert.rejects(checkNow(), /offline/);
  assert.equal(Number(storage.getItem(UPDATE_CHECK_KEY)), due);
  assert.deepEqual(checked, [true]);
  assert.deepEqual(shown, []);
  assert.deepEqual(clock.timers.map((t) => t.at), [due]);
  stop();
  await assert.rejects(checkNow(), /stopped/, 'a closed window checks nothing');
});

test('Settings reports what a check by hand found while nothing else is going on', () => {
  const idle = { state: 'idle' };
  assert.equal(updateSummary(idle, false, { info: same }), 'Up to date: 1.2.3 is the latest version');
  assert.equal(updateSummary(idle, false, { info: { current: 'dev', latest: '', url: '', available: false } }),
    'Development build: there is no release to compare it with');
  assert.equal(updateSummary(idle, false, { info: newer }), '1.3.0 is available');
  assert.equal(updateSummary(idle, true, { error: 'offline' }), 'Last check failed');
  assert.equal(updateSummary({ state: 'ready', version: '1.3.0' }, false, { info: newer }), 'Ready: restart to install 1.3.0');
});

test('the macOS menu\'s Check for Updates… opens Settings and checks there', async () => {
  const { subscribeUpdateCheck, UPDATE_CHECK_EVENT } = await import('../src/lib/api.ts');
  assert.equal(UPDATE_CHECK_EVENT, 'update:check', 'the name cmd/agent-sessions/about.go sends');
  const handlers = new Map();
  const backend = { onEvent: (name, fn) => (handlers.set(name, fn), () => handlers.delete(name)) };
  let requests = 0;
  const off = subscribeUpdateCheck(() => requests++, backend);
  handlers.get(UPDATE_CHECK_EVENT)();
  assert.equal(requests, 1);
  off();
  assert.equal(handlers.size, 0);

  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
  assert.match(app, /subscribeUpdateCheck\(\(\) => \{\s*manage\.openSettings\('general'\);\s*void updates\.checkNow\(\);\s*\}\);/);
  const go = readFileSync(new URL('../../cmd/agent-sessions/about.go', import.meta.url), 'utf8');
  assert.match(go, /const checkUpdatesEvent = "update:check"/);
});
