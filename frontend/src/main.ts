// Bundled fonts so the UI looks the same on every platform (see style.css).
// Inter's optical-size axis keeps small UI text open and legible.
import '@fontsource-variable/inter/opsz.css'
import '@fontsource-variable/inter/opsz-italic.css'
import '@fontsource-variable/jetbrains-mono/wght.css'
import '@fontsource-variable/jetbrains-mono/wght-italic.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { api } from './lib/api'
import { isDesktop, loadDesktopRuntime } from './lib/desktopRuntime'
import { startKeyboardDiagnostics } from './lib/keyboardDiagnostics'
import { installKeyOrderGuard } from './lib/keyOrderGuard'
import { installTextInputDefaults } from './lib/textInputDefaults'
import { startLiveDocuments } from './lib/liveDocumentsWiring'
import { setupDropGuard, setupLinkInterceptor } from './lib/navigation'
import { showStartupError } from './lib/startupError'

// The desktop app loads its runtime at once: it tells the shell the page is
// ready and installs the file drop listeners. A broken bridge is shown, never
// replaced by the browser demo's mock data.
if (isDesktop()) {
  loadDesktopRuntime().catch((err: unknown) => showStartupError(err));
  // Records keyboard events only while the Help menu says so.
  startKeyboardDiagnostics();
}

// Spellcheck, autocorrect and inline predictions stay off in every field
// (KD-06).
installTextInputDefaults();
setupLinkInterceptor(api);
setupDropGuard();
startLiveDocuments();

// WebKitGTK paints native GTK scrollbars on a layer above page content, so
// they show through dialogs and floating buttons. style.css replaces them
// with CSS scrollbars on Linux only; macOS keeps its native overlay ones.
export const isMac = /Mac/.test(navigator.userAgent) && !/iPhone/.test(navigator.userAgent) && !/iPad/.test(navigator.userAgent);

// WebKit on macOS sometimes hands OpenKey's Backspaces to the page after its
// replacement text; the guard puts them back in order (KD-05).
if (isDesktop() && isMac) installKeyOrderGuard();

if (/Linux/.test(navigator.userAgent) && !/Android/.test(navigator.userAgent)) {
  document.documentElement.dataset.platform = 'linux';
}

// Svelte 5 mount API
const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
