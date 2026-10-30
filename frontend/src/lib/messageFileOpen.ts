import type { FileListing, FileProbeResult } from './types';
import type { ToastMessage } from './guidance';

interface MessageFileRecoveryDependencies {
  probeFiles(dir: string, paths: string[], host: string): Promise<FileProbeResult>;
  listFiles(dir: string): Promise<FileListing>;
  toast: { show(message: ToastMessage): number; dismiss(id: number): void };
  /** Where the clicked path came from, for the toast wording. */
  source?: 'message' | 'terminal';
}

function isRelativeFilePath(path: string): boolean {
  return !/[\\:\x00-\x1f\x7f-\x9f]/.test(path)
    && path.split('/').every(part => part !== '' && part !== '.' && part !== '..');
}

export function messageFileProbePaths(path: string): string[] {
  if (!isRelativeFilePath(path) || new TextEncoder().encode(path).length > 4096) return [];
  const parts = path.split('/');
  if (parts.length > 32) return [];
  return parts.map((_, index) => parts.slice(0, index + 1).join('/'));
}

export function matchingMessageFiles(listing: FileListing, requested: string): string[] {
  if (!isRelativeFilePath(requested)) return [];
  const directories = new Set(listing.directories ?? []);
  return [...new Set([...listing.files, ...(listing.ignored ?? [])])].filter(path => {
    return isRelativeFilePath(path) && (path === requested || path.endsWith('/' + requested))
      && !directories.has(path) && listing.status?.[path] !== 'D';
  }).sort();
}

/** Discovery only offers choices; the opener must authorize the selected path again. */
export function messageFileRecovery(deps: MessageFileRecoveryDependencies, host: string, cwd: string, requested: string) {
  return async (_error: unknown, current: () => boolean, own: (dispose: () => void) => void): Promise<string | null | undefined> => {
    const prefixes = messageFileProbePaths(requested);
    if (!prefixes.length || !current()) return undefined;
    // A missing parent makes the full path unknown. Probe each prefix as a
    // leaf so an explicitly absent ancestor can still permit discovery.
    // A failed probe (say, a terminal cwd that is no session directory) is
    // no evidence of absence: keep the original failure.
    const probe = await deps.probeFiles(cwd, prefixes, host).catch(() => null);
    if (!current()) return null;
    if (!probe?.observations.some(item => prefixes.includes(item.path) && item.state === 'absent')) return undefined;
    const listing = await deps.listFiles(cwd);
    if (!current()) return null;
    const candidates = matchingMessageFiles(listing, requested);
    if (!candidates.length) {
      deps.toast.show({
        title: `Could not find ${deps.source ?? 'message'} file`,
        body: `No matching ${requested} in the available listing for ${cwd}${host ? ` on ${host}` : ''}.${listing.truncated ? ' The listing is truncated; more files may exist.' : ''}`,
        tone: 'error',
      });
      return null;
    }
    return new Promise(resolve => {
      let settled = false;
      const finish = (path: string | null) => {
        if (settled) return;
        settled = true;
        resolve(path);
      };
      const id = deps.toast.show({
        title: `Choose a file for ${requested}`,
        body: `Matches in ${cwd}${host ? ` on ${host}` : ''}. Select a path to open.${listing.truncated ? ' The listing is truncated; more matches may exist.' : ''}`,
        persistent: true,
        actions: candidates.map(path => ({ label: path, onSelect: () => finish(current() ? path : null) })),
        onDismiss: reason => { if (reason !== 'select') finish(null); },
      });
      own(() => { deps.toast.dismiss(id); finish(null); });
    });
  };
}
