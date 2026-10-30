import { api } from '../api';
import type { GitHistoryRefActionRequest } from '../gitHistory';
import { errorText } from '../manage';
import { files } from './files.svelte';
import { link } from './link.svelte';
import { connectionStore } from './connection.svelte';
import { filePrompt } from './filePrompt.svelte';
import { gitHistory } from './gitHistory.svelte';
import { gitChanges } from './gitChanges.svelte';
import { codeDiagnostics } from './codeDiagnostics.svelte';

type Action = GitHistoryRefActionRequest['action'];

/** Explicit ref mutations; selecting a history ref remains strictly read-only. */
export class GitHistoryActionsStore {
  error = $state<string | null>(null);
  message = $state('');
  private controller: AbortController | null = null;

  get busy(): boolean { return gitChanges.busy; }
  get currentBranch(): string { return gitHistory.headRef?.replace(/^refs\/heads\//, '') || (gitHistory.detached ? 'detached HEAD' : 'HEAD'); }
  private identity(): string {
    return JSON.stringify([gitHistory.contextKey, files.dir, link.dataHost, gitHistory.selectedRepository,
      gitHistory.repository?.scope, gitHistory.revision, gitHistory.headRef, gitHistory.head]);
  }
  blocker(action: Action | 'compare', ref: string): string | null {
    if (link.blockedReason) return link.blockedReason;
    if (gitHistory.blockedReason) return gitHistory.blockedReason;
    if (!files.dir || gitHistory.selectedRepository === null || gitHistory.stale || gitHistory.refsLoading || gitHistory.refsError) return 'Refresh repository refs first.';
    const caps = connectionStore.capabilities;
    const target = gitHistory.refs.find(r => r.ref === ref);
    if (ref !== 'HEAD' && !target) return 'This ref is no longer available. Refresh history.';
    if (action === 'compare') {
      if (!caps.gitHistoryCompare) return 'This host does not support working-tree comparisons. Update the server and reconnect.';
      if (ref === 'HEAD' ? !gitHistory.head : target?.kind === 'tag' || ref.startsWith('refs/tags/') ? !target?.commit : !target?.oid) return 'This ref does not resolve to a commit.';
      return null;
    }
    if (!caps.gitHistoryActions || !caps.gitWrite || !caps.saveFile) return 'This host does not support Git ref actions. Update the server and reconnect.';
    if (this.busy) return 'Wait for the current Git action to finish.';
    if (!target) return 'Choose an exact branch or tag, not HEAD.';
    if (action === 'push') {
      if (!caps.gitSync) return 'This host does not support Git sync. Update the server and reconnect.';
      if (target.remote) return 'Remote-tracking refs cannot be pushed.';
    }
    if (action === 'delete' && ref === gitHistory.headRef) return 'The current branch cannot be deleted.';
    if (action === 'checkout' || action === 'merge') {
      if (gitChanges.draftPaths.length) return 'Save or close unsaved drafts before changing the working tree.';
      if (ref.startsWith('refs/tags/') && !target.commit) return 'This tag does not resolve to a commit.';
      if (action === 'merge' && (gitHistory.detached || gitHistory.unborn || !gitHistory.headRef || !gitHistory.head)) return 'Merge requires a current branch with a commit.';
      if (ref === gitHistory.headRef) return 'This is already the current branch.';
    }
    return null;
  }
  cancel(): void { this.controller?.abort(); }

  async run(action: Action, ref: string): Promise<boolean> {
    const blocker = this.blocker(action, ref);
    if (blocker) { this.error = blocker; return false; }
    const dir = files.dir!, repo = gitHistory.selectedRepository!, host = link.dataHost;
    const context = gitHistory.contextKey;
    const sameContext = () => context === gitHistory.contextKey && dir === files.dir && host === link.dataHost && repo === gitHistory.selectedRepository;
    const identity = this.identity(), target = gitHistory.refs.find(r => r.ref === ref)!;
    const expectedOID = target.oid;
    const expectedHead = { symbolicRef: gitHistory.headRef || '', oid: gitHistory.head };
    const changesTree = action === 'checkout' || action === 'merge';
    const scope = changesTree && !!gitHistory.repository?.scope;
    const label = action === 'checkout' ? 'Checkout' : action === 'merge' ? 'Merge' : action === 'push' ? 'Push to origin' : 'Delete';
    const description = action === 'merge' ? `Merge '${target.name}' into '${this.currentBranch}'? Conflicts may require resolving or aborting the merge in a terminal.`
      : action === 'push' ? `Push '${target.name}' to origin? This updates the remote without force and does not change upstream tracking.`
      : action === 'delete' ? `Delete '${target.name}'? ${target.remote ? 'This removes only the local remote-tracking ref, not the branch on the remote.' : ref.startsWith('refs/tags/') ? 'This removes the local tag, not any remote tag.' : 'Only merged branches can be deleted; branches used by another worktree are protected.'}`
      : `Checkout '${target.name}'? ${ref.startsWith('refs/tags/') ? 'This detaches HEAD.' : target.remote ? 'This may create or switch to a local tracking branch.' : 'This switches the working-tree branch.'}`;
    const answer = await filePrompt.ask(label, `${description}${scope ? '\n\nThis workspace is scoped inside the repository. This action affects the WHOLE repository, including files outside the workspace. Continue only if you acknowledge this wider scope.' : ''}`, [{ value: action, label, danger: action === 'delete' }]);
    // A confirmation is permission for this exact snapshot only, never a later host/ref/HEAD.
    if (answer !== action || identity !== this.identity() || this.blocker(action, ref)
      || gitHistory.refs.find(r => r.ref === ref)?.oid !== expectedOID) return false;
    const paths = files.tabs.filter(t => t.owner !== 'history' && !t.external).map(t => t.path);
    const controller = new AbortController();
    this.controller = controller; gitChanges.busy = true; this.error = null; this.message = '';
    let ok = false;
    try {
      const result = await api.gitHistoryRefAction({ dir, repo, action, ref, expectedOID, expectedHead,
        draftPaths: [...gitChanges.draftPaths], acknowledgeWholeRepository: scope }, controller.signal, host);
      ok = true;
      if (sameContext()) this.message = result.summary;
    } catch (e) {
      if (sameContext()) this.error = errorText(e, 'Git ref action failed. Partial effects are preserved; inspect the repository before retrying.');
    } finally {
      // Failure/cancellation can leave moved refs, conflicts or partially changed files.
      const current = sameContext();
      gitHistory.invalidateHost(host);
      const relist = files.invalidateListings(null, host);
      try {
        if (current) {
          // reloadReplaced preserves drafts typed while the operation was in flight.
          if (changesTree) await files.reloadReplaced(paths);
          if (sameContext()) await files.refresh();
          if (sameContext()) await gitChanges.refresh();
          if (sameContext()) await gitChanges.loadBranches();
          if (sameContext() && changesTree) {
            await codeDiagnostics.hide();
            if (sameContext()) await codeDiagnostics.show(dir);
          }
        } else if (relist) void files.refresh();
      } catch (e) {
        if (current && !this.error) this.error = errorText(e, 'Could not refresh after the Git ref action.');
      } finally { this.controller = null; gitChanges.busy = false; }
    }
    return ok;
  }
}

export const gitHistoryActions = new GitHistoryActionsStore();
