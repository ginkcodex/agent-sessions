import type { SessionMeta } from './types';

export interface CodexRepairResult {
  path: string;
  backupPath: string;
}

/** Only the exact legacy name emitted by the old importer needs this repair. */
export function needsCodexRepair(meta: SessionMeta): boolean {
  return meta.ref.agent === 'codex'
    && /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(meta.ref.id)
    && meta.sourcePath?.split(/[\\/]/).pop() === `rollout-restored-${meta.ref.id}.jsonl`;
}
