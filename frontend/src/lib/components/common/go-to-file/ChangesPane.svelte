<script lang="ts">
  import { gitChanges } from '../../../stores/gitChanges.svelte';
  import { connectionStore } from '../../../stores/connection.svelte';
  import { branchLabel, type ChangeSection } from '../../../gitChanges';
  import type { GitChange } from '../../../types';
  import FileIcon from '../FileIcon.svelte';

  let { onOpen }: { onOpen: (path: string, staged: boolean) => void } = $props();
  const sections: { key: ChangeSection; label: string }[] = [
    { key: 'conflicts', label: 'Merge conflicts' }, { key: 'staged', label: 'Staged changes' }, { key: 'unstaged', label: 'Changes' },
  ];
  let groups = $derived(gitChanges.groups);
  let blocker = $derived(gitChanges.writeBlocker);
  let status = $derived(gitChanges.status);
  let commitAll = $derived(groups.staged.length === 0);
  let amendChecked = $state(false);
  let amendBlocker = $derived(gitChanges.amendBlocker);
  // A push or another project can make the box stale; it never amends then.
  let amend = $derived(amendChecked && !amendBlocker);
  let branchesOpen = $state(false);
  let branchQuery = $state('');
  let branchName = $state('');
  let filteredBranches = $derived(gitChanges.branches.filter(b => b.name.toLowerCase().includes(branchQuery.toLowerCase())));
  function rowTitle(entry: GitChange): string {
    const path = entry.originalPath ? `${entry.originalPath} → ${entry.path}` : entry.path;
    const sub = entry.submodule;
    if (!sub) return path;
    // Git's own words for a submodule's state.
    const states = [sub.commit && 'new commits', sub.modified && 'modified content', sub.untracked && 'untracked content'].filter(Boolean);
    return `${path} · Submodule${states.length ? ': ' + states.join(', ') : ''}`;
  }
  function rowKey(event: KeyboardEvent, entry: GitChange, section: ChangeSection) {
    if (event.key === 'Enter') { event.preventDefault(); onOpen(entry.path, section === 'staged'); }
    else if (event.key === ' ' && !blocker) {
      event.preventDefault();
      void (section === 'staged' ? gitChanges.unstage([entry.path]) : gitChanges.stage([entry.path]));
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      const root = (event.currentTarget as HTMLElement).closest('.changes-pane');
      const rows = [...root?.querySelectorAll<HTMLButtonElement>('.change-open') ?? []];
      const index = rows.indexOf(event.currentTarget as HTMLButtonElement);
      rows[(index + (event.key === 'ArrowDown' ? 1 : -1) + rows.length) % rows.length]?.focus();
    }
  }
</script>

{#snippet branchIcon()}
  <svg class="icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="4" cy="3" r="1.6" /><circle cx="12" cy="4" r="1.6" /><circle cx="4" cy="13" r="1.6" /><path d="M4 4.6v6.8M12 5.6C12 9 4 7 4 10" /></svg>
{/snippet}
{#snippet repoIcon()}
  <svg class="icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><path d="M3.5 2.5h8a1 1 0 0 1 1 1v8.5h-8.5a1.5 1.5 0 0 0 0 3H12.5M3.5 2.5v11" /></svg>
{/snippet}

<div class="changes-pane">
  {#if !connectionStore.capabilities.gitChanges}
    <p class="note">This host cannot report Git changes. Update agent-sessions on the host, then reconnect.</p>
  {:else}
    <div class="repo-bar">
      {#if gitChanges.repositories.length}
        <label class="repository">Repository
          <select aria-label="Changes repository" value={gitChanges.repository} disabled={gitChanges.busy || gitChanges.repositoriesLoading} onchange={(e) => { branchesOpen = false; void gitChanges.selectRepository(e.currentTarget.value); }}>
            {#if !gitChanges.repository}<option value="" disabled>Choose a repository…</option>{/if}
            {#each gitChanges.repositories as repo (repo.path)}<option value={repo.path}>{repo.path}</option>{/each}
          </select>
        </label>
      {/if}
      <div class="repo-head">
        {#if status?.ok}
          <button type="button" class="branch" aria-expanded={branchesOpen} title="Switch or create branch" onclick={() => { branchesOpen = !branchesOpen; if (branchesOpen) void gitChanges.loadBranches(); }}>
            {@render branchIcon()}<span class="name">{branchLabel(status)}</span><span class="chevron" aria-hidden="true">▾</span>
          </button>
        {:else}<span class="branch idle">{gitChanges.repositories.length ? 'No repository chosen' : 'No repository'}</span>{/if}
        {#if status?.ahead || status?.behind}<span class="count" title="Ahead / behind upstream">↑{status.ahead} ↓{status.behind}</span>{/if}
        <button type="button" class="icon-btn" class:spinning={gitChanges.loading} title="Refresh changes" aria-label="Refresh changes" disabled={gitChanges.loading || gitChanges.busy} onclick={() => void gitChanges.refresh(true)}>
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            <path d="M12.5 1.8v2.7H9.8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
      {#if status?.ok}
        <div class="sync-actions">
          {#if gitChanges.syncOperation}
            <span class="syncing" role="status">{gitChanges.syncOperation === 'fetch' ? 'Fetching' : gitChanges.syncOperation === 'pull' ? 'Pulling' : 'Pushing'}…</span>
            <button type="button" class="action" onclick={() => gitChanges.cancelSync()}>Cancel</button>
          {:else}
            <button type="button" class="action" disabled={Boolean(gitChanges.syncBlocker)} title={gitChanges.syncBlocker ?? 'Fetch remote updates without changing working files'} onclick={() => void gitChanges.sync('fetch')}>Fetch</button>
            {#if status.behind > 0}
              <button type="button" class="action" disabled={Boolean(gitChanges.syncBlocker) || status.detached || Boolean(status.operation) || groups.conflicts.length > 0} title={gitChanges.syncBlocker ?? (status.operation ? `Finish the ${status.operation} first` : 'Pull with fast-forward only; never merge or rebase')} onclick={() => void gitChanges.sync('pull')}>Pull ↓{status.behind}</button>
            {/if}
            {#if status.ahead > 0 || !status.upstream}
              <button type="button" class="action" disabled={Boolean(gitChanges.syncBlocker) || status.detached || !status.hasCommits || Boolean(status.operation) || groups.conflicts.length > 0} title={gitChanges.syncBlocker ?? (status.operation ? `Finish the ${status.operation} first` : 'Push commits; never force')} onclick={() => void gitChanges.sync('push')}>{status.upstream ? `Push ↑${status.ahead}` : 'Publish branch…'}</button>
            {/if}
          {/if}
        </div>
      {/if}
    </div>
    {#if status?.ok}
      {#if gitChanges.syncMessage}<p class="note" role="status">{gitChanges.syncMessage}</p>{/if}
      {#if !connectionStore.capabilities.gitSync}<p class="note">This host does not support Git sync. Update the server and reconnect.</p>{/if}
    {/if}
    {#if branchesOpen && status?.ok}
      <div class="branches">
        <div class="filter-box text-field">
          <svg class="search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3.5 3.5" /></svg>
          <input type="search" class="filter" aria-label="Filter branches" placeholder="Filter branches…" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" bind:value={branchQuery} />
        </div>
        {#if gitChanges.branchesLoading}<p class="note">Reading branches…</p>{/if}
        <div class="branch-list">
          {#each filteredBranches as branch (branch.name + branch.remote)}
            <button type="button" class="row" class:current={branch.current} disabled={Boolean(blocker) || branch.current} title={blocker ?? branch.upstream ?? branch.name} onclick={async () => { if (await gitChanges.switchBranch(branch.name, false, branch.remote)) branchesOpen = false; }}>
              {@render branchIcon()}<span class="name">{branch.name}</span>
              {#if branch.remote}<span class="tag">remote</span>{/if}
              {#if branch.current}<span class="star" role="img" aria-label="Current branch">★</span>{/if}
            </button>
          {/each}
        </div>
        {#if gitChanges.branchesTruncated}<p class="note">Only the first 500 branches are shown.</p>{/if}
        <div class="create">
          <input class="field text-field" aria-label="New branch name" placeholder="New branch name" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" bind:value={branchName} />
          <button type="button" class="action" disabled={Boolean(blocker) || !branchName.trim()} onclick={async () => { if (await gitChanges.switchBranch(branchName.trim(), true)) { branchesOpen = false; branchName = ''; } }}>Create branch…</button>
        </div>
      </div>
    {/if}
    {#if status?.operation}<p class="note">{status.operation} in progress. Resolve conflicts before committing.</p>{/if}
    {#if gitChanges.repositoriesLoading}<p class="note" role="status">Looking for repositories in this workspace…</p>
    {:else if gitChanges.requiresRepository}
      <p class="note" role="status">This workspace contains {gitChanges.repositories.length} repositories. Choose one to review its changes.</p>
      <ul class="repositories" aria-label="Repositories">
        {#each gitChanges.repositories as repo (repo.path)}<li><button type="button" class="row" title={repo.path} onclick={() => void gitChanges.selectRepository(repo.path)}>{@render repoIcon()}<span class="name">{repo.path}</span></button></li>{/each}
      </ul>
    {:else if status && !status.ok}<p class="note">{status.reason}</p>{/if}
    {#if gitChanges.error}<p class="error" role="alert">{gitChanges.error}</p>{/if}
    {#if status?.ok}
      <div class="commit-box">
        <textarea class="text-field" aria-label="Commit message" placeholder="Commit message" rows="3" autocomplete="off" {...{ autocorrect: 'off' }} autocapitalize="off" spellcheck="false" bind:value={gitChanges.message} disabled={gitChanges.busy}
          onkeydown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !e.altKey) { e.preventDefault(); void gitChanges.commit(amend); } }}></textarea>
        <div class="commit-actions">
          <button type="button" class="commit" title={blocker ?? 'Commit saved changes (Cmd/Ctrl+Enter)'} disabled={Boolean(blocker) || !gitChanges.message.trim() || (!amend && !groups.staged.length && !groups.unstaged.length) || groups.conflicts.length > 0}
            onclick={() => void gitChanges.commit(amend)}>{amend ? 'Amend last commit' : commitAll ? 'Commit all' : 'Commit staged'}</button>
          {#if groups.staged.length && groups.unstaged.length}<button type="button" class="action" disabled={Boolean(blocker) || !gitChanges.message.trim() || groups.conflicts.length > 0} onclick={() => void gitChanges.commit(amend, true)}>Commit all</button>{/if}
          <label class="amend" title={amendBlocker ?? 'Rewrite the last commit; allowed while it is not on a remote branch'}><input type="checkbox" bind:checked={amendChecked} disabled={Boolean(amendBlocker)} /> Amend</label>
        </div>
        {#if blocker}<p class="note inline">{blocker}</p>{/if}
      </div>
      <div class="change-groups">
        {#each sections as section}
          {@const entries = groups[section.key]}
          {#if entries.length}
            <div class="section-head">
              <button type="button" class="section-toggle" aria-expanded={!gitChanges.collapsed.has(section.key)} onclick={() => gitChanges.toggle(section.key)}><span class="chevron" aria-hidden="true">▸</span><span class="name">{section.label}</span><span class="count">{entries.length}</span></button>
              {#if section.key === 'staged'}
                <button type="button" class="icon-btn" aria-label="Unstage all" title={blocker ?? 'Unstage all'} disabled={Boolean(blocker)} onclick={() => void gitChanges.unstage(entries.map(e => e.path))}>−</button>
              {:else}
                {@const discardable = entries.filter(e => !e.submodule)}
                {#if section.key === 'unstaged' && discardable.length}<button type="button" class="icon-btn" aria-label="Discard all changes" title={blocker ?? (discardable.length < entries.length ? 'Discard all changes, except in submodules' : 'Discard all changes')} disabled={Boolean(blocker)} onclick={() => void gitChanges.discard(discardable.map(e => e.path))}>↶</button>{/if}
                <button type="button" class="icon-btn" aria-label="Stage all {section.label}" title={blocker ?? 'Stage all'} disabled={Boolean(blocker)} onclick={() => void gitChanges.stage(entries.map(e => e.path))}>+</button>
              {/if}
            </div>
            {#if !gitChanges.collapsed.has(section.key)}
              {#each entries as entry (entry.path)}
                <div class="change-row {section.key}">
                  <button type="button" class="change-open" title={rowTitle(entry)} onclick={() => onOpen(entry.path, section.key === 'staged')} onkeydown={(e) => rowKey(e, entry, section.key)}>
                    {#if entry.submodule}{@render repoIcon()}{:else}<FileIcon path={entry.path} />{/if}<span class="path">{entry.path}</span><span class="status git-{entry.conflict ? 'C' : (section.key === 'staged' ? entry.index : entry.worktree) === '?' ? 'U' : section.key === 'staged' ? entry.index : entry.worktree}">{entry.conflict ? '!' : section.key === 'staged' ? entry.index : entry.worktree}</span>
                  </button>
                  <div class="row-actions">
                    {#if entry.submodule && gitChanges.repositories.some(r => r.path === entry.path)}<button type="button" class="icon-btn" aria-label="Review changes in {entry.path}" title="Review the changes in this submodule" onclick={() => void gitChanges.selectRepository(entry.path)}>
                      <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 3H3v10h10v-3M9 2h5v5M14 2 7.5 8.5" /></svg>
                    </button>{/if}
                    {#if section.key === 'unstaged' && !entry.submodule}<button type="button" class="icon-btn" aria-label="Discard {entry.path}" title={blocker ?? 'Discard changes'} disabled={Boolean(blocker)} onclick={() => void gitChanges.discard([entry.path])}>↶</button>{/if}
                    <button type="button" class="icon-btn" aria-label="{section.key === 'staged' ? 'Unstage' : 'Stage'} {entry.path}" title={blocker ?? (section.key === 'staged' ? 'Unstage' : 'Stage')} disabled={Boolean(blocker)} onclick={() => void (section.key === 'staged' ? gitChanges.unstage([entry.path]) : gitChanges.stage([entry.path]))}>{section.key === 'staged' ? '−' : '+'}</button>
                  </div>
                </div>
              {/each}
            {/if}
          {/if}
        {/each}
        {#if !status.entries.length}<p class="empty">Working tree clean.</p>{/if}
        {#if status.truncated}<p class="note">Only the first 5,000 changes are shown.</p>{/if}
      </div>
    {:else if gitChanges.loading}<p class="note" role="status">Reading changes…</p>{/if}
  {/if}
</div>

<style>
  .changes-pane { flex: 1; min-height: 0; overflow: auto; font-size: .8rem; line-height: 1.4; }
  /* Controls default to the platform's small control font (11px in WebKit).
     Text boxes take the shared .text-field size. */
  button, select { font: inherit; }
  button { cursor: pointer; }
  :disabled { opacity: .5; cursor: default; }
  .icon { width: 16px; height: 16px; flex-shrink: 0; color: var(--text-muted); }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count { flex-shrink: 0; min-width: 1.7em; padding: 1px 5px; border-radius: 5px; background: var(--bg-tertiary); color: var(--text-secondary); text-align: center; font-size: .7rem; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .chevron { flex: 0 0 10px; font-size: .7rem; color: var(--text-muted); }

  .repo-bar { display: flex; flex-direction: column; gap: 8px; padding: 8px 10px 10px; border-bottom: 1px solid var(--border-color); }
  .repository { display: flex; align-items: center; gap: 6px; color: var(--text-secondary); }
  .repository select { flex: 1; min-width: 0; padding: 5px 7px; color: var(--text-primary); background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 6px; }
  .repo-head { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .branch { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; padding: 6px 8px; text-align: left; color: var(--text-primary); background: transparent; border: 0; border-radius: 6px; font-weight: 500; }
  .branch:hover, .branch[aria-expanded='true'] { background: var(--hover-bg); }
  .branch.idle { color: var(--text-secondary); font-weight: 400; }
  .icon-btn { flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 5px; background: transparent; color: var(--text-secondary); }
  .icon-btn:hover:not(:disabled) { background: var(--hover-bg); color: var(--text-primary); }
  .icon-btn.spinning svg { animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .sync-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .syncing { color: var(--text-secondary); }
  .action { padding: 4px 10px; color: var(--text-primary); background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 6px; white-space: nowrap; }
  .action:hover:not(:disabled) { background: var(--hover-bg); }

  .branches { display: flex; flex-direction: column; gap: 6px; padding: 10px 8px; border-bottom: 1px solid var(--border-color); }
  .search-icon { width: 14px; height: 14px; flex-shrink: 0; color: var(--text-muted); }
  .repository select:focus { outline: 2px solid var(--accent-color); outline-offset: -1px; }
  .branch-list { display: flex; flex-direction: column; max-height: 240px; overflow: auto; }
  .row { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; text-align: left; color: var(--text-primary); background: transparent; border: 0; border-radius: 6px; }
  .row:hover:not(:disabled) { background: var(--hover-bg); }
  .row.current { background: var(--active-bg); color: var(--accent-color); opacity: 1; }
  .row.current .icon { color: inherit; }
  .tag { flex-shrink: 0; padding: 0 5px; border-radius: 4px; background: var(--bg-tertiary); color: var(--text-muted); font-size: .7rem; }
  .star { flex-shrink: 0; color: var(--star-color); }
  .create { display: flex; gap: 6px; }
  .create .field { flex: 1; }

  .repositories { list-style: none; margin: 0; padding: 0 8px 8px; }

  .commit-box { padding: 10px; }
  textarea { display: block; width: 100%; resize: vertical; min-height: 60px; max-height: 150px; }
  .commit-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
  .commit { flex: 1; padding: 5px 10px; color: #fff; background: var(--accent-color); border: 0; border-radius: 6px; font-weight: 600; }
  .commit:hover:not(:disabled) { background: var(--accent-hover); }
  .amend { display: flex; align-items: center; gap: 4px; color: var(--text-secondary); cursor: pointer; }
  .amend input { margin: 0; }

  .change-groups { padding: 0 8px 8px; }
  .section-head { display: flex; align-items: center; gap: 2px; margin-top: 4px; }
  .section-toggle { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; padding: 6px; text-align: left; color: var(--text-secondary); background: transparent; border: 0; border-radius: 6px; font-weight: 500; }
  .section-toggle:hover { background: var(--hover-bg); }
  .section-toggle[aria-expanded='true'] .chevron { transform: rotate(90deg); }
  .change-row { display: flex; align-items: center; gap: 2px; margin-left: 12px; border-radius: 6px; }
  .change-row:hover, .change-row:focus-within { background: var(--hover-bg); }
  /* Staged and conflicting rows are tinted with their status colour, so the groups read apart. */
  .change-row.staged { --row-tint: #3fb950; }
  .change-row.conflicts { --row-tint: #db61a2; }
  :global(:root:not([data-theme='dark'])) .change-row.staged { --row-tint: #1a7f37; }
  :global(:root:not([data-theme='dark'])) .change-row.conflicts { --row-tint: #bf3989; }
  .change-row.staged, .change-row.conflicts { background: color-mix(in srgb, var(--row-tint) 10%, transparent); }
  .change-row.staged + .change-row.staged, .change-row.conflicts + .change-row.conflicts { margin-top: 2px; }
  .change-row.staged:hover, .change-row.staged:focus-within, .change-row.conflicts:hover, .change-row.conflicts:focus-within { background: color-mix(in srgb, var(--row-tint) 20%, transparent); }
  .change-open { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; padding: 5px 6px; text-align: left; color: var(--text-primary); background: transparent; border: 0; border-radius: 6px; }
  .path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { flex-shrink: 0; width: 1.2em; text-align: center; font-family: var(--font-mono); font-size: .72rem; font-weight: 600; }
  /* Git status, the file tree's colours. */
  .git-M, .git-R { color: #d29922; }
  .git-A, .git-U { color: #3fb950; }
  .git-D { color: #f85149; }
  .git-C { color: #db61a2; }
  :global(:root:not([data-theme='dark'])) .git-M, :global(:root:not([data-theme='dark'])) .git-R { color: #9a6700; }
  :global(:root:not([data-theme='dark'])) .git-A, :global(:root:not([data-theme='dark'])) .git-U { color: #1a7f37; }
  :global(:root:not([data-theme='dark'])) .git-D { color: #cf222e; }
  :global(:root:not([data-theme='dark'])) .git-C { color: #bf3989; }
  /* Row actions appear on hover or keyboard focus, as in the file tree. */
  .row-actions { display: flex; gap: 2px; padding-right: 2px; visibility: hidden; }
  .change-row:hover .row-actions, .change-row:focus-within .row-actions { visibility: visible; }
  .row:focus-visible, .branch:focus-visible, .section-toggle:focus-visible, .change-open:focus-visible, .icon-btn:focus-visible, .action:focus-visible, .commit:focus-visible { outline: 2px solid var(--accent-color); outline-offset: -2px; }

  .note, .error, .empty { margin: 0; padding: 6px 10px; overflow-wrap: anywhere; font-size: .75rem; line-height: 1.5; color: var(--text-secondary); }
  .note.inline { padding: 6px 0 0; }
  .empty { padding: 12px 6px; color: var(--text-muted); }
  .error { color: var(--danger); }
</style>
