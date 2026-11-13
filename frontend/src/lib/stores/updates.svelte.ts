import { api, subscribeUpdateState } from '../api';
import { errorText } from '../manage';
import type { UpdateStatus } from '../update';

/** The desktop app's downloaded-update state and the download setting. */
export class UpdatesStore {
  status = $state<UpdateStatus>({ state: 'idle' });
  /** "Download updates automatically". */
  auto = $state(true);
  /** The last daily check failed. */
  checkFailed = $state(false);
  saving = $state(false);
  restarting = $state(false);
  error = $state<string | null>(null);

  private stop: (() => void) | null = null;

  /** Reads the state and follows its changes. onStatus sees the initial state and every change. */
  async init(onStatus: (status: UpdateStatus) => void = () => {}): Promise<void> {
    if (!api.updateState) return;
    this.stop?.();
    let changed = false;
    this.stop = subscribeUpdateState((status) => {
      changed = true;
      this.status = status;
      onStatus(status);
    });
    try {
      const [status, auto] = await Promise.all([api.updateState(), api.autoUpdate?.() ?? Promise.resolve(true)]);
      this.auto = auto;
      // An event that arrived meanwhile is newer.
      if (!changed) {
        this.status = status;
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
