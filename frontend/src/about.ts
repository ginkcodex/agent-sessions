// The About window's page (about.html). It shares the app's styles and theme
// but none of its stores: it only shows the version and opens one link.
import '@fontsource-variable/inter/opsz.css'
import './style.css'
import { mount } from 'svelte'
import AboutPage from './lib/components/common/AboutPage.svelte'
import macIcon from './assets/brand/about-icon-macos.png'
import icon from './assets/brand/about-icon.png'
import { versionLabel } from './lib/about'
import { isDesktop, loadDesktopRuntime, openDesktopURL } from './lib/desktopRuntime'
import { theme } from './lib/stores/theme.svelte'

theme.init()

// Like the app's page, it tells the shell it is ready by loading the runtime.
if (isDesktop()) {
  loadDesktopRuntime().catch((err: unknown) => console.error('The desktop runtime could not be loaded', err))
}

function openURL(url: string): void {
  if (isDesktop()) {
    openDesktopURL(url).catch((err: unknown) => console.error('Could not open the link', err))
  } else {
    window.open(url, '_blank', 'noopener')
  }
}

mount(AboutPage, {
  target: document.getElementById('app')!,
  props: {
    version: versionLabel(window.location.search),
    // macOS shows the icon on its tile; Windows and Linux show the bare mark.
    icon: /Mac/.test(navigator.userAgent) ? macIcon : icon,
    openURL,
  },
})
