// Update checks: the desktop app compares itself with the latest GitHub
// release once a day, or when the user asks. Where it can, the backend
// downloads the new version and offers a restart; elsewhere the toast offers
// the release page.
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
  /** This install can update itself, so the download setting applies. */
  supported?: boolean;
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

/** The toast for a downloaded update. Later (or closing it) keeps it hidden until the next launch. */
export function readyToast(version: string, restart: () => void): ToastMessage {
  return {
    title: 'Update ready',
    body: `Agent Sessions ${version} is downloaded. Restart to install it, or it installs when you quit.`,
    actions: [
      { label: 'Restart now', onSelect: restart },
      { label: 'Later', onSelect: () => {} },
    ],
    persistent: true,
  };
}

/** The toast for a newer release this install will not fetch by itself, from the update state alone. */
export function pageToast(status: UpdateStatus, openPage: () => void): ToastMessage {
  return {
    title: 'Update available',
    body: `${status.version ? `Agent Sessions ${status.version}` : 'A newer Agent Sessions'} is out. Download it from its release page.`,
    actions: [{ label: 'Open release page', onSelect: openPage }],
    persistent: true,
  };
}

/** The answer to a check the user asked for in this window. */
export interface CheckResult {
  info?: UpdateInfo;
  error?: string;
}

/**
 * What the Settings page says about updates. result, from a check the user
 * asked for, adds what that check found while nothing else is going on.
 */
export function updateSummary(status: UpdateStatus, checkFailed: boolean, result?: CheckResult | null): string {
  switch (status.state) {
    case 'ready':
      return `Ready: restart to install ${status.version ?? 'the new version'}`;
    case 'downloading':
      return `Downloading ${status.version ?? 'the new version'}…`;
    case 'manual':
    case 'unsupported':
      return `${status.version ?? 'A newer version'} is available`;
    default: {
      const info = result?.info;
      if (info && !info.latest) return 'Development build: there is no release to compare it with';
      if (info?.available) return `${info.latest} is available`;
      if (info) return `Up to date: ${info.current} is the latest version`;
      return checkFailed ? 'Last check failed' : 'Up to date';
    }
  }
}

export interface UpdateAnnouncerDeps {
  show: (message: ToastMessage) => void;
  /** Quits through the usual path and installs. */
  restart: () => void;
  openPage: (url: string) => void;
}

/**
 * Decides which toast an update state deserves. A ready update is announced
 * once per launch and version; Later or closing the toast keeps it hidden.
 * A check that found a release passes its info, which shows the release page
 * toast as before for the states that cannot download.
 */
export function createUpdateAnnouncer(deps: UpdateAnnouncerDeps) {
  let readyFor = '';
  let pageFor = '';
  return {
    announce(status: UpdateStatus, info?: UpdateInfo): void {
      const version = status.version ?? info?.latest ?? '';
      switch (status.state) {
        case 'ready':
          if (!version || readyFor === version) return;
          readyFor = version;
          deps.show(readyToast(version, deps.restart));
          return;
        case 'downloading':
          return;
        case 'idle':
          // A failed download is retried at the next check; the page still helps.
          if (info) deps.show(updateToast(info, () => deps.openPage(info.url)));
          return;
        default: {
          const url = status.url || info?.url || '';
          if (info) {
            pageFor = info.latest;
            deps.show(updateToast(info, () => deps.openPage(url || info.url)));
          } else if (version && pageFor !== version) {
            pageFor = version;
            deps.show(pageToast(status, () => deps.openPage(url)));
          }
        }
      }
    },
  };
}

export interface UpdateCheckDeps {
  check: () => Promise<UpdateInfo>;
  /** Called with a check that found a newer release. */
  notify: (info: UpdateInfo) => void;
  /** Called after each check with whether it failed. */
  checked?: (failed: boolean) => void;
  storage?: Pick<Storage, 'getItem' | 'setItem'> | null;
  now?: () => number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (handle: unknown) => void;
}

export interface UpdateChecks {
  /** Ends the checks; a check already running reports nothing. */
  stop(): void;
  /**
   * Checks at once, as the user asked. A check already running is shared.
   * It counts as the day's check: on success the next one is a day away in
   * every window; a failure keeps the schedule. It reports through the same
   * callbacks as a scheduled check, and also answers the caller.
   */
  checkNow(): Promise<UpdateInfo>;
}

/**
 * Checks for updates now and then, until stop is called. Every window runs
 * one; the due time in storage makes one window check for all, and a window
 * that finds an update shows it.
 */
export function startUpdateChecks(deps: UpdateCheckDeps): UpdateChecks {
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

  let running: Promise<UpdateInfo> | null = null;

  const schedule = () => {
    if (stopped) return;
    if (timer !== null) clearTimer(timer);
    timer = setTimer(() => void run(), checkDelay(due(), now()));
  };

  // One check at a time in this window. A failure is reported, then thrown.
  const perform = (): Promise<UpdateInfo> => {
    if (running) return running;
    const current = (async () => {
      try {
        const info = await deps.check();
        writeDue(now() + CHECK_INTERVAL_MS);
        if (!stopped) {
          deps.checked?.(false);
          if (info.available) deps.notify(info);
        }
        return info;
      } catch (err) {
        if (!stopped) deps.checked?.(true);
        throw err;
      } finally {
        running = null;
        schedule();
      }
    })();
    running = current;
    return current;
  };

  const run = async () => {
    timer = null;
    if (stopped || running) return;
    const at = due();
    // Another window checked meanwhile.
    if (at !== null && at > now()) return schedule();
    // Claim the check, so other windows wait for this one. If it fails, the
    // claim retries in an hour.
    writeDue(now() + RETRY_MS);
    await perform().catch(() => {});
  };

  schedule();
  return {
    stop() {
      stopped = true;
      if (timer !== null) clearTimer(timer);
      timer = null;
    },
    checkNow() {
      if (stopped) return Promise.reject(new Error('Update checks have stopped'));
      return perform();
    },
  };
}
