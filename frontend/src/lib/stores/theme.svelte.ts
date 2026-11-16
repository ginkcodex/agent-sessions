import { isThemeMode, nextThemeMode, resolveTheme, type ResolvedTheme, type ThemeMode } from '../theme';
import { callApp, isDesktop } from '../desktopRuntime';

export type { ThemeMode } from '../theme';

const STORAGE_KEY = 'agent-sessions:theme';

export class ThemeStore {
  mode = $state<ThemeMode>('system');
  resolved = $state<ResolvedTheme>('dark');

  private mediaQuery: MediaQueryList | null = null;
  private listener: ((e: MediaQueryListEvent) => void) | null = null;
  private nativeUpdate: Promise<void> = Promise.resolve();

  init(): void {
    if (typeof window === 'undefined') return;

    const saved = localStorage.getItem(STORAGE_KEY);
    if (isThemeMode(saved)) {
      this.mode = saved;
    }

    // System mode follows the OS live, e.g. when it switches at sunset.
    this.mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    this.listener = () => {
      // Native appearance can change this query too; never feed it back to the window.
      if (this.mode === 'system') {
        this.updateResolved();
        this.applyDOM();
      }
    };
    this.mediaQuery.addEventListener('change', this.listener);

    this.updateResolved();
    this.applyDOM();
    this.applyNative();
  }

  setMode(mode: ThemeMode): void {
    this.mode = mode;
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, mode);
    }
    this.updateResolved();
    this.applyDOM();
    this.applyNative();
  }

  // Steps System → Light → Dark → System.
  toggle(): void {
    this.setMode(nextThemeMode(this.mode));
  }

  private updateResolved(): void {
    this.resolved = resolveTheme(this.mode, this.mediaQuery?.matches ?? false);
  }

  private applyDOM(): void {
    if (typeof document !== 'undefined') {
      document.documentElement.setAttribute('data-theme', this.resolved);
    }
  }

  private applyNative(): void {
    if (!isDesktop()) return;
    const mode = this.mode;
    // Serialize changes: an older native call must not land after a newer one.
    this.nativeUpdate = this.nativeUpdate
      .then(() => callApp('SetWindowTheme', mode))
      .catch((err: unknown) => console.warn('Could not update the native window theme', err));
  }

  destroy(): void {
    if (this.mediaQuery && this.listener) {
      this.mediaQuery.removeEventListener('change', this.listener);
    }
  }
}

export const theme = new ThemeStore();
