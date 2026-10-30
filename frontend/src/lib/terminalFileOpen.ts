import type { ConnectionPhase } from './types';

export interface TerminalFileTarget { dir: string; path: string }

interface LinkSnapshot {
  host?: string;
  dataHost?: string;
  phase: ConnectionPhase;
  stale: boolean;
  locked: boolean;
}

export interface TerminalFileOpenDependencies {
  link(): LinkSnapshot;
  epoch(): number;
  resolve(dir: string, path: string): Promise<TerminalFileTarget>;
  files: {
    dir: string | null;
    listError: string | null;
    show(dir: string): Promise<void>;
    openFile(path: string, opts: { keep: boolean }): void;
  };
  report(message: string): void;
  isStale(error: unknown): boolean;
}

export function terminalFileHostReady(host: string, state: LinkSnapshot): boolean {
  return !state.stale && !state.locked && (state.host ?? '') === host && (state.dataHost ?? '') === host
    && (host ? state.phase === 'connected' : state.phase === 'local');
}

/** One opener per terminal view. Late clicks/replies must not replace a newer destination. */
export function createTerminalFileOpener(deps: TerminalFileOpenDependencies) {
  let sequence = 0;
  let cleanup: (() => void) | undefined;
  const retire = () => { sequence++; const previous = cleanup; cleanup = undefined; previous?.(); };
  return {
    dispose() { retire(); },
    async open(host: string, path: string, cwd?: string, options?: {
      valid?: () => boolean;
      opened?: (target: TerminalFileTarget) => void;
      recover?: (error: unknown, current: () => boolean, own: (dispose: () => void) => void) => Promise<string | null | undefined>;
    }): Promise<void> {
      retire();
      const request = sequence;
      if (options?.valid && !options.valid()) return;
      if (!terminalFileHostReady(host, deps.link())) {
        deps.report('Select/connect to this terminal’s host first to open its files.');
        return;
      }
      const started = deps.epoch();
      const current = () => request === sequence && started === deps.epoch() && terminalFileHostReady(host, deps.link())
        && (!options?.valid || options.valid());
      try {
        let target: TerminalFileTarget;
        try {
          target = await deps.resolve(cwd ?? '', path);
        } catch (error) {
          if (!current() || deps.isStale(error)) return;
          if (!options?.recover) throw error;
          const selected = await options.recover(error, current, dispose => {
            if (current()) cleanup = dispose; else dispose();
          });
          if (!current() || selected === null) return;
          if (selected === undefined) throw error;
          target = await deps.resolve(cwd ?? '', selected);
        }
        if (!current()) return;
        await deps.files.show(target.dir);
        if (!current() || deps.files.dir !== target.dir) return;
        if (deps.files.listError) {
          deps.report(deps.files.listError);
          return;
        }
        deps.files.openFile(target.path, { keep: true });
        if (current()) options?.opened?.(target);
      } catch (error) {
        if (current() && !deps.isStale(error)) {
          deps.report(error instanceof Error ? error.message : String(error));
        }
      } finally {
        if (request === sequence) {
          const previous = cleanup;
          cleanup = undefined;
          previous?.();
        }
      }
    },
  };
}
