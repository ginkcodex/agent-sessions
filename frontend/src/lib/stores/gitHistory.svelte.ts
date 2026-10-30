import { api, GIT_HISTORY_UNSUPPORTED } from '../api';
import type { GitHistoryRepository, GitHistoryRef, GitHistorySummary, GitHistoryCommitDetail, GitHistoryComparison, GitHistoryCompareRequest } from '../gitHistory';
import { errorText } from '../manage';
import { files } from './files.svelte';
import { link } from './link.svelte';
import { connectionStore } from './connection.svelte';

export const HISTORY_PAGE_SIZE = 50;
export const HISTORY_RETAIN_LIMIT = 2000;
export const HISTORY_QUERY_BYTES = 256;
/** UI debounce delay; the store owns no timers. */
export const HISTORY_QUERY_DELAY_MS = 250;

/** Read-only history browser state. Retained file snapshots are owned by files, not this store. */
export class GitHistoryStore {
  repositories = $state.raw<GitHistoryRepository[]>([]);
  repositoriesTruncated = $state(false);
  selectedRepository = $state<string | null>(null);
  refs = $state.raw<GitHistoryRef[]>([]);
  refsTruncated = $state(false);
  selectedRef = $state('HEAD');
  head = $state('');
  headRef = $state('');
  detached = $state(false);
  unborn = $state(false);
  query = $state('');
  commits = $state.raw<GitHistorySummary[]>([]);
  tip = $state('');
  cursor = $state('');
  limited = $state(false);
  shallow = $state(false);
  retainedLimit = $state(false);
  selectedCommit = $state<string | null>(null);
  detail = $state.raw<GitHistoryCommitDetail | null>(null);
  repositoriesLoading = $state(false);
  refsLoading = $state(false);
  pagesLoading = $state(false);
  detailLoading = $state(false);
  discoveryError = $state<string | null>(null);
  refsError = $state<string | null>(null);
  pageError = $state<string | null>(null);
  detailError = $state<string | null>(null);
  stale = $state(true);
  /** Observe alongside contextKey while History is visible; invalidation itself never fetches. */
  revision = $state(0);
  comparison = $state.raw<GitHistoryComparison | null>(null);
  comparisonRequest = $state.raw<GitHistoryCompareRequest | null>(null);
  comparisonLabel = $state('');
  comparisonLoading = $state(false);
  comparisonError = $state<string | null>(null);
  private comparisonGeneration = 0;
  private comparisonHost: string | undefined;

  closeComparison(): void {
    ++this.comparisonGeneration;
    files.invalidateComparisons();
    this.comparison = null; this.comparisonRequest = null; this.comparisonLabel = '';
    this.comparisonLoading = false; this.comparisonError = null;
  }
  async openComparison(value: GitHistoryRef | string): Promise<void> {
    this.syncContext();
    if (this.blockedReason || !files.dir || this.selectedRepository === null) return;
    const ref = typeof value !== 'string' ? value : value === 'HEAD'
      ? { ref: 'HEAD', name: 'HEAD', oid: this.head, current: true, remote: false }
      : this.refs.find(candidate => candidate.ref === value);
    if (!ref?.oid) return;
    this.closeComparison();
    this.comparisonRequest = { dir: files.dir, repo: this.selectedRepository, ref: ref.ref, expectedOID: ref.oid };
    this.comparisonHost = link.dataHost;
    this.comparisonLabel = `${ref.name} → working tree`;
    await this.refreshComparison();
  }
  /** Re-read saved disk contents, never editor buffers. Old mutable tabs are retired. */
  async refreshComparison(): Promise<void> {
    this.syncContext();
    const request = this.comparisonRequest;
    if (!request || this.blockedReason || this.comparisonHost !== link.dataHost) return;
    files.invalidateComparisons();
    this.comparison = null; this.comparisonError = null; this.comparisonLoading = true;
    const identity = this.identity, generation = ++this.comparisonGeneration;
    const current = () => this.current(identity) && generation === this.comparisonGeneration && request === this.comparisonRequest;
    try {
      const result = await api.gitHistoryCompare(request, this.comparisonHost);
      if (!current()) return;
      if (result.ref !== request.ref || result.oid !== request.expectedOID) throw new Error('The comparison reply does not match the selected ref.');
      this.comparison = result;
    } catch (error) {
      if (current()) this.comparisonError = errorText(error, 'Could not compare the working tree. Refresh to retry.');
    } finally { if (current()) this.comparisonLoading = false; }
  }
  private identity = '';
  private workspace = '';
  private discoveryGeneration = 0;
  private refsGeneration = 0;
  private pageGeneration = 0;
  private detailGeneration = 0;
  private discovered = false;
  /** Prior repository choice per workspace/host; only a hint revalidated by discovery. */
  private remembered = new Map<string, string>();
  private refsLoaded = false;
  private pagesLoaded = false;

  /** Read this and revision in the UI's lifecycle effect, then call ensureLoaded on changes. */
  get contextKey(): string {
    return JSON.stringify([files.dir, link.dataHost, link.host, files.contextVersion,
      link.phase, connectionStore.generation, this.supported]);
  }
  get supported(): boolean {
    // Local advertises it too; an old desktop binding still fails explicitly in the API.
    return connectionStore.capabilities.gitHistory === true;
  }
  get blockedReason(): string | null {
    if (link.blockedReason) return link.blockedReason;
    if (!this.supported) return GIT_HISTORY_UNSUPPORTED;
    if (!files.dir) return 'Choose a workspace to browse Git history.';
    return null;
  }
  get error(): string | null { return this.discoveryError ?? this.refsError ?? this.pageError ?? this.detailError; }
  get loading(): boolean { return this.repositoriesLoading || this.refsLoading || this.pagesLoading; }
  get repository(): GitHistoryRepository | null { return this.repositories.find(r => r.path === this.selectedRepository) ?? null; }
  get requiresRepositorySelection(): boolean { return this.repositories.length > 1 && this.selectedRepository === null; }
  get canLoadMore(): boolean { return !this.blockedReason && !this.stale && !!this.cursor && !this.retainedLimit && !this.pagesLoading; }

  private current(identity: string): boolean { return identity === this.identity && identity === this.contextKey && !this.blockedReason; }
  private clearDetail(): void {
    ++this.detailGeneration;
    this.selectedCommit = null; this.detail = null; this.detailLoading = false; this.detailError = null;
  }
  private clearPages(): void {
    ++this.pageGeneration;
    this.commits = []; this.tip = ''; this.cursor = ''; this.limited = false; this.shallow = false;
    this.retainedLimit = false; this.pagesLoading = false; this.pagesLoaded = false; this.pageError = null;
    this.clearDetail();
  }
  private clearRefs(): void {
    this.closeComparison();
    ++this.refsGeneration;
    this.refs = []; this.head = ''; this.headRef = ''; this.detached = false; this.unborn = false; this.refsTruncated = false;
    this.refsLoading = false; this.refsLoaded = false; this.refsError = null;
    this.clearPages();
  }
  /** Observe context even when requests are blocked, so reconnect cannot revive old requests. */
  private syncContext(): void {
    const identity = this.contextKey;
    if (identity === this.identity) return;
    const workspace = JSON.stringify([files.dir, link.dataHost, link.host]);
    if (workspace !== this.workspace) {
      this.selectedRepository = this.remembered.get(workspace) ?? null; this.selectedRef = 'HEAD'; this.query = '';
    }
    this.workspace = workspace; this.identity = identity;
    this.reset(); // contextKey itself already re-runs the UI effect.
  }
  /**
   * Git writes may affect overlapping/scoped workspaces and linked worktrees.
   * Match the existing listing invalidation's conservative host-wide Git scope.
   * Requests for another host must never clear this host's browser.
   */
  invalidateHost(host: string | undefined): void {
    if (link.dataHost !== host) return;
    this.invalidate();
  }
  /**
   * Reopen revalidates refs; mode switches within one opening do not repin. A load
   * already in flight (e.g. History effect ran first in the same flush) is fresh enough.
   */
  dialogOpened(): void { this.syncContext(); if (!this.loading) this.invalidate(); }
  /** Focus events during an existing refresh coalesce rather than restart it. */
  focusRefresh(): void {
    this.syncContext();
    if (!this.loading && !this.stale) this.invalidate();
  }
  /**
   * External invalidation is lazy: it retires pending replies and bumps revision so a
   * visible History effect calls ensureLoaded once. Internal resets never bump it.
   */
  invalidate(): void { this.reset(); ++this.revision; }
  private reset(): void {
    ++this.discoveryGeneration;
    this.discovered = false; this.repositories = []; this.repositoriesTruncated = false;
    this.repositoriesLoading = false; this.discoveryError = null; this.stale = true;
    this.clearRefs();
  }
  async ensureLoaded(): Promise<void> {
    this.syncContext();
    if (this.blockedReason) return;
    if (!this.discovered) { if (!this.repositoriesLoading) await this.refresh(); return; }
    if (this.selectedRepository === null) return;
    if (!this.refsLoaded) { if (!this.refsLoading) await this.loadRefs(); return; }
    if (!this.pagesLoaded && !this.pagesLoading) await this.loadPage(false);
  }
  /** Explicit refresh always starts a new discovery/ref/list snapshot; old details cannot land. */
  async refresh(): Promise<void> {
    this.syncContext();
    this.reset();
    if (this.blockedReason) return;
    const identity = this.identity, generation = ++this.discoveryGeneration;
    const prior = this.selectedRepository;
    this.repositoriesLoading = true;
    try {
      const result = await api.gitHistoryRepositories(files.dir!);
      if (!this.current(identity) || generation !== this.discoveryGeneration) return;
      this.repositories = result.repositories ?? []; this.repositoriesTruncated = result.truncated;
      this.discovered = true; this.stale = false;
      this.selectedRepository = this.repositories.some(r => r.path === prior) ? prior
        : this.repositories.length === 1 ? this.repositories[0].path : null;
      if (this.selectedRepository !== prior) this.selectedRef = 'HEAD';
      this.remember();
    } catch (e) {
      if (this.current(identity) && generation === this.discoveryGeneration) this.discoveryError = errorText(e, 'Could not discover Git repositories.');
      return;
    } finally {
      if (identity === this.contextKey && generation === this.discoveryGeneration) this.repositoriesLoading = false;
    }
    if (this.current(identity) && generation === this.discoveryGeneration && this.selectedRepository !== null) await this.loadRefs();
  }
  private remember(): void {
    this.remembered.delete(this.workspace);
    if (this.selectedRepository === null) return;
    this.remembered.set(this.workspace, this.selectedRepository);
    if (this.remembered.size > 64) this.remembered.delete(this.remembered.keys().next().value!);
  }
  async selectRepository(path: string): Promise<void> {
    this.syncContext();
    if (this.blockedReason || !this.repositories.some(r => r.path === path)) return;
    if (path === this.selectedRepository) { await this.ensureLoaded(); return; }
    this.selectedRepository = path; this.selectedRef = 'HEAD';
    this.remember();
    this.clearRefs();
    await this.loadRefs();
  }
  private async loadRefs(): Promise<void> {
    const repo = this.selectedRepository;
    if (repo === null || this.blockedReason) return;
    const identity = this.identity, generation = ++this.refsGeneration;
    this.refsLoading = true; this.refsError = null;
    try {
      const result = await api.gitHistoryBranches({ dir: files.dir!, repo });
      if (!this.current(identity) || generation !== this.refsGeneration) return;
      this.refs = result.refs ?? []; this.refsTruncated = result.truncated;
      this.head = result.head; this.headRef = result.headRef ?? this.refs.find(ref => ref.current)?.ref ?? ''; this.detached = result.detached; this.unborn = result.unborn;
      this.refsLoaded = true;
      if (this.selectedRef !== 'HEAD' && !this.refs.some(r => r.ref === this.selectedRef)) this.selectedRef = 'HEAD';
    } catch (e) {
      if (this.current(identity) && generation === this.refsGeneration) this.refsError = errorText(e, 'Could not list history refs.');
      return;
    } finally {
      if (identity === this.contextKey && generation === this.refsGeneration) this.refsLoading = false;
    }
    if (this.current(identity) && generation === this.refsGeneration) await this.loadPage(false);
  }
  async selectRef(ref: string): Promise<void> {
    this.syncContext();
    if (this.blockedReason || this.selectedRepository === null || !this.refsLoaded) return;
    if (ref !== 'HEAD' && !this.refs.some(r => r.ref === ref)) return;
    if (ref === this.selectedRef) { await this.ensureLoaded(); return; }
    this.selectedRef = ref; this.clearPages();
    await this.loadPage(false);
  }
  /** Call immediately to invalidate old replies. UI may debounce ensureLoaded using defer. */
  async setQuery(query: string, { defer = false }: { defer?: boolean } = {}): Promise<void> {
    this.syncContext();
    if (query === this.query) { if (!defer) await this.ensureLoaded(); return; }
    this.query = query; this.clearPages();
    if (!defer) await this.ensureLoaded();
  }
  async loadMore(): Promise<void> {
    this.syncContext();
    if (this.canLoadMore) await this.loadPage(true);
  }
  private async loadPage(append: boolean): Promise<void> {
    const repo = this.selectedRepository;
    if (repo === null || !this.refsLoaded || this.blockedReason) return;
    if (new TextEncoder().encode(this.query).length > HISTORY_QUERY_BYTES) {
      this.pageError = `Search is limited to ${HISTORY_QUERY_BYTES} UTF-8 bytes.`;
      return;
    }
    const identity = this.identity, generation = ++this.pageGeneration;
    const tip = this.tip, cursor = append ? this.cursor : '';
    this.pagesLoading = true; this.pageError = null;
    try {
      // The opaque cursor binds target, ref, query, page size AND pinned tip. Never repin load-more.
      const page = await api.gitHistoryList({ dir: files.dir!, repo, ref: this.selectedRef, query: this.query, cursor, limit: HISTORY_PAGE_SIZE });
      if (!this.current(identity) || generation !== this.pageGeneration) return;
      if (append && page.tip !== tip) throw new Error('History snapshot changed. Refresh history to continue.');
      if (append && page.cursor && page.cursor === cursor) throw new Error('History continuation did not advance. Refresh history to continue.');
      const rows = append ? [...this.commits] : [];
      const seen = new Set(rows.map(c => c.oid));
      for (const commit of page.commits ?? []) if (!seen.has(commit.oid)) { seen.add(commit.oid); rows.push(commit); }
      this.commits = rows.slice(0, HISTORY_RETAIN_LIMIT);
      this.tip = page.tip; this.cursor = page.cursor; this.limited = page.limited; this.shallow = page.shallow;
      this.retainedLimit = rows.length > HISTORY_RETAIN_LIMIT || (rows.length === HISTORY_RETAIN_LIMIT && !!page.cursor);
      this.pagesLoaded = true;
    } catch (e) {
      if (this.current(identity) && generation === this.pageGeneration) this.pageError = errorText(e, 'Could not read Git history. Refresh to retry missing snapshots.');
    } finally {
      if (identity === this.contextKey && generation === this.pageGeneration) this.pagesLoading = false;
    }
  }
  async selectCommit(oid: string): Promise<void> {
    this.syncContext();
    if (this.blockedReason || !this.tip || this.selectedRepository === null || !this.commits.some(c => c.oid === oid)) return;
    this.closeComparison();
    if (oid === this.selectedCommit && (this.detail || this.detailLoading)) return;
    this.clearDetail(); this.selectedCommit = oid;
    const identity = this.identity, generation = ++this.detailGeneration;
    this.detailLoading = true;
    try {
      const detail = await api.gitHistoryCommit({ dir: files.dir!, repo: this.selectedRepository, tip: this.tip, commit: oid });
      if (this.current(identity) && generation === this.detailGeneration) this.detail = detail;
    } catch (e) {
      if (this.current(identity) && generation === this.detailGeneration) this.detailError = errorText(e, 'Could not read commit details.');
    } finally {
      if (identity === this.contextKey && generation === this.detailGeneration) this.detailLoading = false;
    }
  }
}

export const gitHistory = new GitHistoryStore();
