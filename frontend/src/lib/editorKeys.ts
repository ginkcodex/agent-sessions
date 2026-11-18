// The editor's key for code suggestions, shared by the editor's keymap
// (fileEditor.ts completionKeys) and Settings, which names it. Kept apart
// from fileEditor.ts, which loads lazily with CodeMirror.

/** On macOS, where Ctrl+Space usually switches the input source (CC-01). */
export const SUGGEST_KEY_MAC = 'Cmd-i';
/** Everywhere, macOS included. */
export const SUGGEST_KEY = 'Ctrl-Space';

/**
 * Whether CodeMirror binds keys as on macOS. This is its own check
 * (`@codemirror/view` reads `navigator.platform`, deprecated as it is), so
 * Settings names the key the editor binds.
 */
export function macKeys(platform: string = typeof navigator === 'undefined' ? '' : navigator.platform): boolean {
  return /Mac/.test(platform);
}

/** The suggestion shortcut as Settings shows it. */
export function suggestShortcutLabel(platform?: string): string {
  return macKeys(platform) ? '⌘I' : 'Ctrl+Space';
}
