import { api, subscribeUpdateState } from '../api';
import { errorText } from '../manage';
import type { CheckResult, UpdateInfo, UpdateStatus } from '../update';

/** The desktop app's downloaded-update state and the download setting. */
export class UpdatesStore {
  status = $state<UpdateStatus>({ state: 'idle' });
  /** "Download updates automatically". */
  auto = $state(true);
  /** The last check failed. */
  checkFailed = $state(false);
  /** This window can check by hand: the desktop app's scheduler is running. */
  canCheck = $state(false);
  /** A check asked for here is running. */
  checking = $state(false);
  /** What the last check asked for here found, until the update state moves on. */
  checkResult = $state<CheckResult | null>(null);
  saving = $state(false);
  restarting = $state(false);
  error = $state<string | null>(null);

  private stop: (() => void) | null = null;
  private checker: (() => Promise<UpdateInfo>) | null = null;

  /** Reads the state and follows its changes. onStatus sees the initial state and every change. */
  async init(onStatus: (status: UpdateStatus) => void = () => {}): Promise<void> {
    if (!api.updateState) return;
    this.stop?.();
    let changed = false;
    this.stop = subscribeUpdateState((status) => {
      changed = true;
      this.setStatus(status);
      onStatus(status);
    });
    try {
      const [status, auto] = await Promise.all([api.updateState(), api.autoUpdate?.() ?? Promise.resolve(true)]);
      this.auto = auto;
      // An event that arrived meanwhile is newer.
      if (!changed) {
        this.setStatus(status);
        onStatus(status);
      }
    } catch {
      // Older backends lack the bindings: nothing is downloaded.
    }
  }

  destroy(): void {
    this.stop?.();
    this.stop = null;
  }

  private setStatus(status: UpdateStatus): void {
    this.status = status;
    // A download, a ready version or a release page says more than the check did.
    if (status.state !== 'idle') this.checkResult = null;
  }

  /** Sets the check the button runs; null when this window has none. */
  setChecker(check: (() => Promise<UpdateInfo>) | null): void {
    this.checker = check;
    this.canCheck = check !== null;
  }

  /** Checks for updates now, as the user asked. */
  async checkNow(): Promise<void> {
    if (!this.checker || this.checking) return;
    this.checking = true;
    this.checkResult = null;
    try {
      const info = await this.checker();
      // A release that is being downloaded or is ready shows as the update
      // state; the result only speaks while nothing else does.
      this.checkResult = info.available && this.status.state !== 'idle' ? null : { info };
    } catch (err) {
      this.checkResult = { error: errorText(err, 'The check failed') };
    } finally {
      this.checking = false;
    }
  }

  /** Whether the update setting and state are meaningful here. */
  get available(): boolean {
    return typeof api.updateState === 'function';
  }

  async setAuto(on: boolean): Promise<void> {
    if (!api.setAutoUpdate) return;
    this.saving = true;
    try {
      await api.setAutoUpdate(on);
      this.auto = on;
      this.error = null;
    } catch (err) {
      this.error = errorText(err, 'Failed to save the update setting');
    } finally {
      this.saving = false;
    }
  }

  async restart(): Promise<void> {
    if (!api.restartToUpdate) return;
    this.restarting = true;
    try {
      await api.restartToUpdate();
      this.error = null;
    } catch (err) {
      this.error = errorText(err, 'Could not restart to update');
    } finally {
      this.restarting = false;
    }
  }
}

export const updates = new UpdatesStore();
