// Update checks: the desktop app compares itself with the latest GitHub
// release once a day and, when a newer one is out, offers its page. Users
// download and install it themselves.
import type { ToastMessage } from './guidance';

/** What the backend's CheckForUpdate answers. */
export interface UpdateInfo {
  /** The running version; "dev" for a build without one. */
  current: string;
  /** The latest release's version, without the tag's "v". */
  latest: string;
  /** The latest release's page. */
  url: string;
  available: boolean;
}

/**
 * Where a downloaded update is, from the backend's UpdateState and its
 * update:state event.
 *
 *  - idle: nothing to show.
 *  - downloading: a new version is being fetched; show nothing.
 *  - ready: it is downloaded and checked, and installs when the app quits.
 *  - manual: a newer release is out; the app will not fetch it itself.
 *  - unsupported: this install cannot update itself.
 *
 * manual and unsupported show today's toast with the release page.
 */
export type UpdateState = 'idle' | 'downloading' | 'ready' | 'manual' | 'unsupported';

export interface UpdateStatus {
  state: UpdateState;
  /** The newer release's version; absent when idle. */
  version?: string;
  /** Its release page. */
  url?: string;
}

/** When the next check is due, in epoch milliseconds, shared by every window. */
export const UPDATE_CHECK_KEY = 'agent-sessions:next-update-check';
/** The first check waits for startup to settle. */
export const STARTUP_DELAY_MS = 10_000;
export const CHECK_INTERVAL_MS = 24 * 60 * 60 * 1000;
/** After a failed check, or while another window checks. */
export const RETRY_MS = 60 * 60 * 1000;

/**
 * How long to wait for the next check. Never sooner than the startup delay,
 * and never later than a day, even when the clock has gone back.
 */
export function checkDelay(dueAt: number | null, now: number): number {
  if (dueAt === null || !Number.isFinite(dueAt)) return STARTUP_DELAY_MS;
  return Math.min(CHECK_INTERVAL_MS, Math.max(STARTUP_DELAY_MS, dueAt - now));
}

/** The toast offering a newer release's page. */
export function updateToast(info: UpdateInfo, openPage: () => void): ToastMessage {
  return {
    title: 'Update available',
    body: `Agent Sessions ${info.latest} is out; you have ${info.current}. Download it from its release page.`,
    actions: [{ label: 'Open release page', onSelect: openPage }],
    persistent: true,
  };
}

export interface UpdateCheckDeps {
  check: () => Promise<UpdateInfo>;
  /** Called with a check that found a newer release. */
  notify: (info: UpdateInfo) => void;
  storage?: Pick<Storage, 'getItem' | 'setItem'> | null;
  now?: () => number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (handle: unknown) => void;
}

/**
 * Checks for updates now and then, until the returned stop is called. Every
 * window runs one; the due time in storage makes one window check for all,
 * and a window that finds an update shows it.
 */
export function startUpdateChecks(deps: UpdateCheckDeps): () => void {
  const now = deps.now ?? Date.now;
  const setTimer = deps.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
  const clearTimer = deps.clearTimer ?? ((handle) => clearTimeout(handle as ReturnType<typeof setTimeout>));
  let timer: unknown = null;
  let stopped = false;

  const readDue = (): number | null => {
    try {
      const raw = deps.storage?.getItem(UPDATE_CHECK_KEY);
      const due = raw ? Number(raw) : NaN;
      return Number.isFinite(due) ? due : null;
    } catch {
      return null;
    }
  };
  let localDue: number | null = null;
  const writeDue = (due: number) => {
    localDue = due;
    try {
      deps.storage?.setItem(UPDATE_CHECK_KEY, String(due));
    } catch {
      // Storage disabled: this window keeps its own schedule.
    }
  };
  const due = () => readDue() ?? localDue;

  const schedule = () => {
    if (stopped) return;
    timer = setTimer(() => void run(), checkDelay(due(), now()));
  };

  const run = async () => {
    timer = null;
    if (stopped) return;
    const at = due();
    // Another window checked meanwhile.
    if (at !== null && at > now()) return schedule();
    // Claim the check, so other windows wait for this one.
    writeDue(now() + RETRY_MS);
    try {
      const info = await deps.check();
      writeDue(now() + CHECK_INTERVAL_MS);
      if (info.available && !stopped) deps.notify(info);
    } catch {
      // Offline or rate limited: the claim retries in an hour.
    }
    schedule();
  };

  schedule();
  return () => {
    stopped = true;
    if (timer !== null) clearTimer(timer);
    timer = null;
  };
}
