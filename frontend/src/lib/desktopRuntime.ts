// The only importer of @wailsio/runtime. Importing the runtime has side
// effects (it creates window._wails, announces readiness to the shell and
// installs drag listeners), so it is loaded with a dynamic import, and only in
// the desktop app: the browser demo and node tests never load it.
import type * as Wails from '@wailsio/runtime';

/** The Go service every App binding is called through, by full method name. */
export const APP_SERVICE = 'github.com/ginkcode/agent-sessions/internal/app.App';

/**
 * Files dropped on a `data-file-drop-target` element. The desktop shell
 * re-emits v3's FilesDropped window event under this name, because v3 only
 * delivers that event to Go listeners. Payload: `{ filenames, x, y }`, with
 * x and y in CSS pixels relative to the web view.
 */
export const FILES_DROPPED_EVENT = 'files:dropped';

export interface FilesDroppedPayload {
  filenames?: string[] | null;
  x?: number;
  y?: number;
}

/** The parts of @wailsio/runtime the app uses. */
export type DesktopRuntime = {
  Call: Pick<typeof Wails.Call, 'ByName'>;
  Events: Pick<typeof Wails.Events, 'On'>;
  Browser: Pick<typeof Wails.Browser, 'OpenURL'>;
  Window: Pick<typeof Wails.Window, 'SetTitle'>;
  Clipboard: Pick<typeof Wails.Clipboard, 'SetText' | 'Text'>;
};

declare global {
  interface Window {
    /** Set to true by the desktop shell in a classic script before any module runs. */
    __AGENT_SESSIONS_DESKTOP__?: boolean;
  }
}

/**
 * Whether the page runs in the desktop app. The Go asset server injects the
 * marker; the runtime's own `window._wails` cannot be used because importing
 * the runtime creates it.
 */
export function isDesktop(): boolean {
  return typeof window !== 'undefined' && window.__AGENT_SESSIONS_DESKTOP__ === true;
}

/** The desktop marker is set but the runtime bridge could not be loaded. */
export class DesktopBridgeError extends Error {
  constructor(cause: unknown) {
    const detail = cause instanceof Error ? cause.message : String(cause);
    super(`The desktop runtime could not be loaded: ${detail}`, { cause });
    this.name = 'DesktopBridgeError';
  }
}

const importRuntime = (): Promise<DesktopRuntime> => import('@wailsio/runtime');
let loader: () => Promise<DesktopRuntime> = importRuntime;
let loading: Promise<DesktopRuntime> | undefined;

/**
 * Loads the runtime once. Rejects outside the desktop app, and with a
 * DesktopBridgeError when the desktop bridge is broken.
 */
export function loadDesktopRuntime(): Promise<DesktopRuntime> {
  if (!isDesktop()) return Promise.reject(new Error('Not running in the desktop app'));
  loading ??= loader().then(
    (rt) => {
      if (typeof rt?.Call?.ByName !== 'function' || typeof rt?.Events?.On !== 'function') {
        throw new DesktopBridgeError('the runtime has no Call.ByName or Events.On');
      }
      return rt;
    },
    (err) => {
      throw err instanceof DesktopBridgeError ? err : new DesktopBridgeError(err);
    },
  );
  return loading;
}

/**
 * Replaces the runtime loader, for tests. With no argument the real
 * @wailsio/runtime import is restored. Either way the next use reloads.
 */
export function setDesktopRuntimeLoader(next?: () => Promise<DesktopRuntime>): void {
  loader = next ?? importRuntime;
  loading = undefined;
}

/** Calls App.method in the Go backend. */
export async function callApp(method: string, ...args: unknown[]): Promise<any> {
  const rt = await loadDesktopRuntime();
  return rt.Call.ByName(`${APP_SERVICE}.${method}`, ...args);
}

/**
 * Subscribes to a backend event and passes its data to callback. The
 * returned function unsubscribes, also when called before the runtime loaded.
 */
export function onDesktopEvent(name: string, callback: (data: any) => void): () => void {
  let off: (() => void) | undefined;
  let cancelled = false;
  void loadDesktopRuntime().then(
    (rt) => {
      if (cancelled) return;
      off = rt.Events.On(name, (e) => callback(e.data));
    },
    () => {
      // A broken bridge is reported once, at startup (see main.ts).
    },
  );
  return () => {
    cancelled = true;
    off?.();
    off = undefined;
  };
}

/** Subscribes to files dropped on a drop target: (x, y, paths). */
export function onDesktopFileDrop(callback: (x: number, y: number, paths: string[]) => void): () => void {
  return onDesktopEvent(FILES_DROPPED_EVENT, (data: FilesDroppedPayload | null | undefined) => {
    const paths = Array.isArray(data?.filenames) ? data.filenames.filter((p) => typeof p === 'string' && p) : [];
    if (paths.length === 0) return;
    callback(Number(data?.x) || 0, Number(data?.y) || 0, paths);
  });
}

export async function openDesktopURL(url: string): Promise<void> {
  const rt = await loadDesktopRuntime();
  await rt.Browser.OpenURL(url);
}

export async function setDesktopTitle(title: string): Promise<void> {
  const rt = await loadDesktopRuntime();
  await rt.Window.SetTitle(title);
}

/** Puts text on the system clipboard through the desktop shell. */
export async function setDesktopClipboardText(text: string): Promise<void> {
  const rt = await loadDesktopRuntime();
  await rt.Clipboard.SetText(text);
}

/** The system clipboard's text, read through the desktop shell. */
export async function desktopClipboardText(): Promise<string> {
  const rt = await loadDesktopRuntime();
  return (await rt.Clipboard.Text()) || '';
}
