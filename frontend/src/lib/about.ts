// The About window: a page of its own (about.html), opened by the macOS app
// menu's About item, and on Windows and Linux by the sidebar's logo.

export const APP_NAME = 'Agent Sessions';
export const APP_DESCRIPTION = 'Browse and manage agentic coding tool sessions';
export const BUY_ME_A_COFFEE_URL = 'https://buymeacoffee.com/ginkcode';

/**
 * The version line. The shell passes the version in the page's query; a
 * build without one stamped reports "dev".
 */
export function versionLabel(search: string): string {
  const version = new URLSearchParams(search).get('version')?.trim() ?? '';
  if (!version) return '';
  return version === 'dev' ? 'Development build' : `Version ${version}`;
}
