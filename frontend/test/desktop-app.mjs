// A stand-in App binding for the desktop backend. The real one calls each Go
// method through the router's Call.ByName (see desktop-runtime.test.mjs);
// these tests pass plain objects, where a missing method stands for an older
// backend.
import { createDesktopAPI } from '../src/lib/api.ts';
import { setDesktopRuntimeLoader } from '../src/lib/desktopRuntime.ts';

let current = {};

/** Makes App the binding desktopAPI() uses, and returns it. */
export function useDesktopApp(App) {
  current = App;
  return App;
}

/** A desktop backend over the binding last passed to useDesktopApp. */
export function desktopAPI() {
  return createDesktopAPI(current);
}

// Fills the parts of the runtime a test does not stub with inert ones. A
// runtime marked `incomplete` keeps its gaps, to stand for a broken bridge.
function completeRuntime(runtime) {
  if (!runtime || runtime.incomplete) return runtime;
  return {
    Call: { ByName: async () => undefined, ...runtime.Call },
    Events: { On: () => () => {}, ...runtime.Events },
    Browser: { OpenURL: async () => {}, ...runtime.Browser },
    Window: { SetTitle: async () => {}, ...runtime.Window },
    Clipboard: { SetText: async () => {}, Text: async () => '', ...runtime.Clipboard },
    ...Object.fromEntries(Object.entries(runtime).filter(([k]) => !['Call', 'Events', 'Browser', 'Window', 'Clipboard'].includes(k))),
  };
}

/**
 * Marks globalThis.window as the desktop app and makes the runtime loader
 * return runtime (an object shaped like @wailsio/runtime, or a rejecting
 * loader result). Returns a function that restores both.
 */
export function installDesktopRuntime(runtime, extraWindow = {}) {
  const hadWindow = 'window' in globalThis;
  const previous = globalThis.window;
  globalThis.window = { ...extraWindow, __AGENT_SESSIONS_DESKTOP__: true };
  setDesktopRuntimeLoader(typeof runtime === 'function' ? runtime : async () => completeRuntime(runtime));
  return () => {
    setDesktopRuntimeLoader();
    if (hadWindow) globalThis.window = previous;
    else delete globalThis.window;
  };
}

/** Lets pending runtime loads and their callbacks run. */
export const flushRuntime = () => new Promise((resolve) => setTimeout(resolve, 0));

/** Runs fn with App as the binding, passing it a desktop backend over it. */
export async function withDesktopApp(App, fn) {
  const previous = current;
  current = App;
  try { return await fn(desktopAPI()); } finally { current = previous; }
}
