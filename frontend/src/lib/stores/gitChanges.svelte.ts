import { api } from '../api';
import type { GitCommitRequest, RepoStatus } from '../types';
import { errorText } from '../manage';
import { changeGroups } from '../gitChanges';
import { files, tabHasUnsaved } from './files.svelte';
import { link } from './link.svelte';
import { connectionStore } from './connection.svelte';
import { filePrompt } from './filePrompt.svelte';
import { codeDiagnostics } from './codeDiagnostics.svelte';
import { gitHistory } from './gitHistory.svelte';
import type { GitBranch } from '../types';
import type { GitHistoryRepository } from '../gitHistory';

/** The Changes pane follows the selected project and host, never cached routes. */
export class GitChangesStore {
  status = $state.raw<RepoStatus | null>(null);
  loading = $state(false);
  busy = $state(false);
  error = $state<string | null>(null);
  message = $state('');
  branches = $state.raw<GitBranch[]>([]);
  branchesTruncated = $state(false);
  branchesLoading = $state(false);
  collapsed = $state(new Set<string>());
  syncOperation = $state<'fetch' | 'pull' | 'push' | null>(null);
  syncMessage = $state('');
  /** The repositories inside a project that is in none, as History finds them. */
  repositories = $state.raw<GitHistoryRepository[]>([]);
  repositoriesLoading = $state(false);
  /** '' for the project's own repository; else the chosen one inside it. */
  repository = $state('');
  private syncController: AbortController | null = null;
  private identity = '';
  private sequence = 0;
  private workspace = '';
  private discovered = false;
  private discoveries = 0;
  /** Prior repository choice per project and host; only a hint discovery revalidates. */
  private remembered = new Map<string, string>();

  get groups() { return changeGroups(this.status); }
  get draftPaths(): string[] { return files.tabs.filter(t => t.owner !== 'history' && tabHasUnsaved(t)).map(t => t.path); }
  get writeBlocker(): string | null {
    if (link.blockedReason) return link.blockedReason;
    if (!connectionStore.capabilities.gitWrite) return 'This host does not support Git write actions. Update the server and reconnect.';
    if (this.busy) return 'Wait for the current Git action to finish.';
    if (this.requiresRepository) return 'Choose a repository.';
    // The status shown is of another project or repository until refreshed.
    if (!this.status?.ok || this.identity !== this.key()) return this.status?.reason ?? 'Wait for repository status.';
    return null;
  }
  /** Amending rewrites the last commit, so never one others may have. */
  get amendBlocker(): string | null {
    const status = this.status;
    if (this.writeBlocker || !status) return this.writeBlocker;
    if (!status.hasCommits) return 'There is no commit to amend yet.';
    if (status.upstream && status.ahead < 1) return 'The last commit is already on the upstream.';
    // Older servers do not report remote branches, so they need an upstream to compare.
    if (status.headPublished === undefined && !status.upstream) return 'Update the server and reconnect to amend a branch without an upstream.';
    if (status.headPublished) return 'The last commit is already on a remote branch.';
    return null;
  }
  /** A project in no repository holds several; the pane waits for a choice. */
  get requiresRepository(): boolean { return this.repository === '' && this.repositories.length > 1; }
  private get discoverable(): boolean {
    const caps = connectionStore.capabilities;
    return caps.gitChangesRepository === true && caps.gitHistory === true;
  }
  private key() { return JSON.stringify([files.dir, link.dataHost, files.contextVersion, this.repository]); }
  private syncWorkspace(): void {
    const workspace = JSON.stringify([files.dir, link.dataHost, files.contextVersion]);
    if (workspace === this.workspace) return;
    this.workspace = workspace;
    this.repository = this.remembered.get(JSON.stringify([files.dir, link.dataHost])) ?? '';
    this.repositories = []; this.repositoriesLoading = false; this.discovered = false;
    ++this.discoveries;
  }
  private choose(repository: string): void {
    this.repository = repository;
    const workspace = JSON.stringify([files.dir, link.dataHost]);
    this.remembered.delete(workspace);
    if (repository) this.remembered.set(workspace, repository);
    if (this.remembered.size > 64) this.remembered.delete(this.remembered.keys().next().value!);
  }

  /** rediscover looks again for the repositories inside a project in none. */
  async refresh(rediscover = false): Promise<void> {
    this.syncWorkspace();
    if (rediscover) this.discovered = false;
    const identity = this.key();
    if (identity !== this.identity) {
      this.identity = identity;
      this.status = null; this.message = ''; this.error = null; this.collapsed = new Set();
      this.branches = []; this.branchesTruncated = false; this.syncMessage = '';
    }
    const sequence = ++this.sequence;
    if (!files.dir || !connectionStore.capabilities.gitChanges || link.blockedReason) { this.loading = false; return; }
    const dir = files.dir, repository = this.repository;
    const current = () => sequence === this.sequence && identity === this.key();
    let next: string | null = null;
    this.loading = true;
    try { next = await this.read(dir, repository, current); }
    catch (e) { if (current()) this.error = errorText(e, 'Could not read repository status.'); }
    finally { if (sequence === this.sequence) this.loading = false; }
    if (next !== null && current()) { this.choose(next); await this.refresh(); }
  }

  /**
   * Reads the status, or returns the repository to read instead. A project in
   * no repository may hold some: like History, Changes then works on one of
   * them, the only one or the one chosen.
   */
  private async read(dir: string, repository: string, current: () => boolean): Promise<string | null> {
    if (repository) {
      if (!this.discoverable) return '';
      if (!this.discovered) {
        const found = await this.discover(dir, current);
        if (!current()) return null;
        if (found && !found.some(r => r.path === repository)) return '';
      }
    }
    const status = await api.gitStatus(dir, repository);
    if (!current()) return null;
    if (!repository && status.ok) { this.repositories = []; this.discovered = false; }
    this.status = status; this.error = null;
    if (repository || status.ok || !this.discoverable) return null;
    const found = await this.discover(dir, current);
    return current() && found?.length === 1 ? found[0].path : null;
  }

  private async discover(dir: string, current: () => boolean): Promise<GitHistoryRepository[] | null> {
    const discovery = ++this.discoveries;
    this.repositoriesLoading = true;
    try {
      const result = await api.gitHistoryRepositories(dir);
      if (!current()) return null;
      // '.' is the project's own repository, which Git could not read.
      this.repositories = (result.repositories ?? []).filter(r => r.path !== '.');
      this.discovered = true;
      return this.repositories;
    } catch (e) {
      if (current()) this.error = errorText(e, 'Could not discover Git repositories.');
      return null;
    } finally { if (discovery === this.discoveries) this.repositoriesLoading = false; }
  }

  async selectRepository(path: string): Promise<void> {
    if (path === this.repository || !this.repositories.some(r => r.path === path)) return;
    this.choose(path);
    await this.refresh();
  }

  async loadBranches(): Promise<void> {
    const dir = files.dir, repository = this.repository, identity = this.key();
    // No repository, no branches: Git would only fail.
    if (!dir || this.branchesLoading || !connectionStore.capabilities.gitChanges || !this.status?.ok || identity !== this.identity) return;
    this.branchesLoading = true;
    try {
      const result = await api.gitBranches(dir, repository);
      if (identity === this.key()) { this.branches = result.branches; this.branchesTruncated = result.truncated; }
    } catch (e) { if (identity === this.key()) this.error = errorText(e, 'Could not list branches.'); }
    finally { this.branchesLoading = false; }
  }

  async switchBranch(name: string, create = false, remote = false): Promise<boolean> {
    if (this.draftPaths.length) { this.error = 'Save or close unsaved drafts before switching branches.'; return false; }
    const identity = this.key();
    const paths = files.tabs.filter(t => t.owner !== 'history' && !t.external).map(t => t.path);
    const ok = await this.write((dir, repo, host) => api.gitSwitch(dir, repo, { name, create, remote, draftPaths: this.draftPaths }, host), paths, true);
    if (ok && files.dir && identity === this.key()) { await codeDiagnostics.hide(); if (identity === this.key() && files.dir) await codeDiagnostics.show(files.dir); await this.loadBranches(); }
    return ok;
  }

  get syncBlocker(): string | null {
    if (!connectionStore.capabilities.gitSync) return 'This host does not support Git sync. Update the server and reconnect.';
    return this.writeBlocker;
  }

  cancelSync(): void { this.syncController?.abort(); }

  async sync(op: 'fetch' | 'pull' | 'push'): Promise<boolean> {
    const dir = files.dir, repository = this.repository, host = link.dataHost, identity = this.key();
    if (!dir || this.syncBlocker) return false;
    if (op !== 'fetch' && this.status?.detached) { this.error = 'Switch to a branch before pulling or pushing.'; return false; }
    if (op === 'pull' && this.draftPaths.length) { this.error = 'Save or close unsaved drafts before pulling.'; return false; }
    // Publishing creates a remote branch, so it asks; a push to the upstream does not.
    if (op === 'push' && !this.status?.upstream) {
      const answer = await filePrompt.ask('Publish branch', `Publish ${this.status?.branch} to the remote and track it as its upstream?`, [{ value: 'push', label: 'Publish' }]);
      if (answer !== 'push' || identity !== this.key() || this.syncBlocker) return false;
    }
    const controller = new AbortController();
    this.syncController = controller; this.syncOperation = op;
    this.busy = true; this.error = null; this.syncMessage = '';
    let ok = false;
    try {
      const result = await api.gitSync(dir, repository, op, host, controller.signal);
      ok = true;
      if (identity === this.key()) { this.status = result.status; this.syncMessage = result.summary; }
    } catch (e) {
      if (identity !== this.key()) { /* another project now */ }
      else if ((e as { code?: string } | null)?.code === 'canceled') this.syncMessage = `Canceled ${op}. Anything Git already finished is kept.`;
      else this.error = errorText(e, 'Git sync failed.');
    } finally {
      // Cancellation or a failed pull can still have fetched refs or changed disk,
      // including under hidden or overlapping roots of this host. Push moves tracking refs.
      gitHistory.invalidateHost(host);
      const relist = op === 'pull' && files.invalidateListings(null, host);
      if (identity === this.key()) {
        const failure = this.error;
        try {
          if (op === 'pull') {
            await files.reloadReplaced(files.tabs.filter(t => t.owner !== 'history' && !t.external).map(t => t.path));
            if (identity === this.key()) await files.refresh();
            if (identity === this.key()) await codeDiagnostics.hide();
            if (identity === this.key() && files.dir) await codeDiagnostics.show(files.dir);
          }
          if (identity === this.key()) await this.refresh();
        } catch (e) { if (identity === this.key()) this.error = errorText(e, 'Could not refresh after Git sync.'); }
        if (failure && identity === this.key()) this.error = failure;
      } else if (relist) void files.refresh();
      this.syncController = null; this.syncOperation = null; this.busy = false;
    }
    return ok;
  }

  toggle(section: string): void {
    const next = new Set(this.collapsed);
    if (next.has(section)) next.delete(section); else next.add(section);
    this.collapsed = next;
  }

  /** moveRefs: commit/amend/switch can move refs, so retained history must repin. */
  private async write(work: (dir: string, repo: string, host: string | undefined) => Promise<void>, reload: string[] = [], moveRefs = false): Promise<boolean> {
    const dir = files.dir, repo = this.repository, host = link.dataHost, identity = this.key();
    if (!dir || this.writeBlocker) return false;
    this.busy = true; this.error = null;
    let ok = false, relist = false;
    try {
      // Before any reload: every retained listing of this host may show older Git
      // status, even after a failure (hooks may stage or rewrite files).
      try { await work(dir, repo, host); } finally {
        relist = files.invalidateListings(null, host);
        // Hooks can move refs even when the requested action ultimately fails.
        if (moveRefs) gitHistory.invalidateHost(host);
      }
      ok = true;
      if (identity === this.key() && reload.length) await files.reloadReplaced(reload);
    } catch (e) { if (identity === this.key()) this.error = errorText(e, 'Git action failed.'); }
    finally {
      this.busy = false;
      // Failed hooks may still stage files; failed discards may be partial.
      if (identity === this.key()) {
        const failure = this.error;
        await files.refresh();
        await this.refresh();
        if (failure) this.error = failure;
      } else if (relist) void files.refresh();
    }
    return ok;
  }

  stage(paths: string[]): Promise<boolean> { return this.write((dir, repo, host) => api.gitStage(dir, repo, paths, host)); }
  unstage(paths: string[]): Promise<boolean> { return this.write((dir, repo, host) => api.gitUnstage(dir, repo, paths, host)); }

  async discard(paths: string[]): Promise<boolean> {
    // Git cannot discard in a submodule from here; it is a repository of its own.
    const submodules = new Set(this.status?.entries.filter(e => e.submodule).map(e => e.path));
    const chosen = (paths.length ? paths : this.groups.unstaged.map(e => e.path)).filter(path => !submodules.has(path));
    if (!chosen.length || this.writeBlocker) return false;
    if (chosen.some(path => this.draftPaths.includes(path))) { this.error = 'Save or close the unsaved drafts before discarding their changes.'; return false; }
    const identity = this.key();
    const answer = await filePrompt.ask('Discard changes', `Permanently discard changes in ${chosen.join(', ')}? Untracked files will be deleted. This cannot be undone.`, [{ value: 'discard', label: 'Discard', danger: true }]);
    if (answer !== 'discard' || identity !== this.key()) return false;
    return this.write((dir, repo, host) => api.gitDiscard(dir, repo, { paths: chosen, draftPaths: this.draftPaths }, host), chosen);
  }

  async commit(amend = false, stageAll = this.groups.staged.length === 0): Promise<boolean> {
    if (this.groups.conflicts.length) { this.error = 'Resolve and stage merge conflicts before committing.'; return false; }
    if (!this.message.trim()) { this.error = 'Enter a commit message.'; return false; }
    if (stageAll && this.draftPaths.length) { this.error = 'Save or close unsaved drafts before committing all. Git commits saved files only.'; return false; }
    const message = this.message, identity = this.key();
    const request: GitCommitRequest = { message, amend, stageAll };
    const ok = await this.write((dir, repo, host) => api.gitCommit(dir, repo, request, host), [], true);
    if (ok && identity === this.key() && this.message === message) this.message = '';
    return ok;
  }
}

export const gitChanges = new GitChangesStore();
