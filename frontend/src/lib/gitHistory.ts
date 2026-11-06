import type { FileContent } from './types';

export interface GitHistoryTarget { dir: string; repo: string }
export interface GitHistoryRepository { path: string; name: string; scope: string }
export interface GitHistoryRepositoryList { repositories: GitHistoryRepository[]; truncated: boolean }
export interface GitHistoryRef { ref: string; name: string; remote: boolean; current: boolean; oid: string; kind?: 'local' | 'remote' | 'tag'; commit?: string }
export interface GitHistoryRefList { refs: GitHistoryRef[]; head: string; headRef?: string; detached: boolean; unborn: boolean; shallow: boolean; truncated: boolean }
export interface GitHistoryHeadIdentity { symbolicRef: string; oid: string }
export interface GitHistoryRefActionRequest extends GitHistoryTarget { action: 'checkout' | 'merge' | 'push' | 'delete'; ref: string; expectedOID: string; expectedHead: GitHistoryHeadIdentity; draftPaths: string[]; acknowledgeWholeRepository: boolean }
export interface GitHistoryRefActionResult { action: string; ref: string; summary: string }
export interface GitHistoryCompareRequest extends GitHistoryTarget { ref: string; expectedOID: string }
export interface GitHistoryComparison { ref: string; oid: string; commit: string; token: string; changes: GitHistoryChange[]; truncated: boolean }
export interface GitHistoryCompareDiffRequest extends GitHistoryCompareRequest { token: string; path: string }
export interface GitHistoryCompareDiff { content: FileContent; ref: string; oid: string; commit: string; token: string; change: GitHistoryChange; deleted: boolean; kind: GitHistoryDiff['kind'] }
export interface GitHistoryPerson { name: string; email: string; time: number }
export interface GitHistorySummary { oid: string; subject: string; author: GitHistoryPerson; parents: string[] }
export interface GitHistoryPage { tip: string; commits: GitHistorySummary[]; cursor: string; limited: boolean; shallow: boolean }
export interface GitHistoryListRequest extends GitHistoryTarget { ref: string; query: string; cursor: string; limit: number }
export interface GitHistoryCommitRequest extends GitHistoryTarget { tip: string; commit: string }
export interface GitHistoryChange { path: string; oldPath?: string; status: string; oldMode: string; newMode: string }
export interface GitHistoryCommitDetail extends GitHistorySummary { committer: GitHistoryPerson; message: string; base: string; changes: GitHistoryChange[]; truncated: boolean }
export interface GitHistoryDiffRequest extends GitHistoryCommitRequest { path: string }
export interface GitHistoryDiff { content: FileContent; commit: string; base: string; change: GitHistoryChange; deleted: boolean; kind: 'text' | 'binary' | 'tooLarge' | 'symlink' | 'submodule' }

export interface RepositoryOption { value: string; label: string; title: string }

/** Options for a Repository picker. The project's own repository ('.') goes
 *  by its name, the others by their path. Changes passes its own repository
 *  apart, as the empty choice. */
export function repositoryOptions(repositories: GitHistoryRepository[], own?: GitHistoryRepository | null): RepositoryOption[] {
  const option = (r: GitHistoryRepository, value = r.path): RepositoryOption => {
    const label = r.path === '.' ? r.name : r.path;
    // Long paths are cut short; the tooltip has them whole.
    return { value, label, title: label };
  };
  return [...(own ? [option(own, '')] : []), ...repositories.map(r => option(r))];
}
