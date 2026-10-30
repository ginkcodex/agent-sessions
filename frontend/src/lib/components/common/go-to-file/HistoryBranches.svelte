<script lang="ts">
  import { gitHistory } from '../../../stores/gitHistory.svelte';
  import { gitHistoryActions } from '../../../stores/gitHistoryActions.svelte';
  import ContextMenu, { type ContextMenuItem } from '../ContextMenu.svelte';
  let menu = $state<{ ref: string; x: number; y: number } | null>(null);
  $effect(() => {
    void gitHistory.contextKey; void gitHistory.selectedRepository; void gitHistory.refs;
    void gitHistory.head; void gitHistory.headRef; void gitHistory.revision;
    menu = null;
  });
  function openMenu(event: MouseEvent | KeyboardEvent, ref: string) {
    if (disabled) return;
    event.preventDefault(); event.stopPropagation();
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { ref, x: event instanceof MouseEvent && event.type === 'contextmenu' ? event.clientX : rect.left,
      y: event instanceof MouseEvent && event.type === 'contextmenu' ? event.clientY : rect.bottom };
  }
  function menuKey(event: KeyboardEvent, ref: string) {
    if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) openMenu(event, ref);
  }
  let items = $derived.by((): ContextMenuItem[] => {
    const ref = menu?.ref ?? 'HEAD';
    const name = gitHistory.refs.find(r => r.ref === ref)?.name ?? ref;
    const actions = [
      ['checkout', 'Checkout'], ['compare', 'Show Diff with Working Tree'],
      ['merge', `Merge '${name}' into '${gitHistoryActions.currentBranch}'`],
      ['push', 'Push to origin'], ['delete', 'Delete'],
    ] as const;
    return actions.map(([action, label]) => {
      const reason = gitHistoryActions.blocker(action, ref);
      return { label, separator: action === 'merge' || action === 'delete', disabled: !!reason, title: reason ?? undefined,
        onSelect: () => { if (action === 'compare') void gitHistory.openComparison(ref); else void gitHistoryActions.run(action, ref); } };
    });
  });
  let { compact = false }: { compact?: boolean } = $props();
  let filter = $state('');
  $effect(() => { void gitHistory.contextKey; void gitHistory.selectedRepository; filter = ''; });
  let needle = $derived(filter.trim().toLowerCase());
  let shown = $derived(needle ? gitHistory.refs.filter(ref => ref.name.toLowerCase().includes(needle)) : gitHistory.refs);
  let tags = $derived(shown.filter(ref => ref.ref.startsWith('refs/tags/')));
  let local = $derived(shown.filter(ref => !ref.remote && !ref.ref.startsWith('refs/tags/')));
  let remotes = $derived([...new Set(shown.filter(ref => ref.remote).map(ref => ref.name.split('/')[0]))]);
  let disabled = $derived(!!gitHistory.blockedReason || gitHistory.selectedRepository === null || gitHistory.refsLoading || !!gitHistory.refsError);
  let headLabel = $derived(gitHistory.detached ? 'HEAD (detached)' : gitHistory.unborn ? 'HEAD (unborn)' : 'HEAD');
</script>

{#snippet branchIcon()}
  <svg class="branch-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="4" cy="3" r="1.6" /><circle cx="12" cy="4" r="1.6" /><circle cx="4" cy="13" r="1.6" /><path d="M4 4.6v6.8M12 5.6C12 9 4 7 4 10" /></svg>
{/snippet}

{#if compact}
  <label>Branch <select aria-label="History branch" value={gitHistory.selectedRef} {disabled} oncontextmenu={(e) => openMenu(e, gitHistory.selectedRef)} onkeydown={(e) => menuKey(e, gitHistory.selectedRef)} onchange={(e) => void gitHistory.selectRef(e.currentTarget.value)}>
    <option value="HEAD">{headLabel}</option>
    <optgroup label="Local">{#each local as ref (ref.ref)}<option value={ref.ref}>{ref.name}</option>{/each}</optgroup>
    {#each remotes as remote}<optgroup label={`Remote · ${remote}`}>
      {#each gitHistory.refs.filter(ref => ref.remote && ref.name.split('/')[0] === remote) as ref (ref.ref)}<option value={ref.ref}>{ref.name}</option>{/each}
    </optgroup>{/each}
    {#if tags.length}<optgroup label="Tags">{#each tags as ref (ref.ref)}<option value={ref.ref} disabled={!ref.commit}>{ref.name}{!ref.commit ? ' (not a commit)' : ''}</option>{/each}</optgroup>{/if}
  </select><button type="button" class="actions" aria-label="Actions for selected history ref" aria-haspopup="menu" {disabled} onclick={(e) => openMenu(e, gitHistory.selectedRef)} onkeydown={(e) => menuKey(e, gitHistory.selectedRef)}>⋯</button></label>
{:else}
  <nav aria-label="History refs" class="branches">
    <h3>Branches</h3>
    <div class="filter-box text-field">
      <svg class="search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3.5 3.5" /></svg>
      <input type="search" class="filter" placeholder="Filter branches…" aria-label="Filter branches" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" bind:value={filter} />
    </div>
    <button type="button" class="ref head" class:selected={gitHistory.selectedRef === 'HEAD'} aria-current={gitHistory.selectedRef === 'HEAD' ? 'true' : undefined} {disabled} oncontextmenu={(e) => openMenu(e, 'HEAD')} onkeydown={(e) => menuKey(e, 'HEAD')} onclick={() => void gitHistory.selectRef('HEAD')}>
      <svg class="branch-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="8" cy="8" r="2.6" /><path d="M1.5 8h4M10.6 8h3.9" /></svg><span class="name">{headLabel}</span>
    </button>
    {#if local.length}<details open><summary><span class="chevron" aria-hidden="true">▸</span><span class="label">Local</span><span class="count">{local.length}</span></summary>
      {#each local as ref (ref.ref)}<button type="button" class="ref" class:selected={gitHistory.selectedRef === ref.ref} aria-current={gitHistory.selectedRef === ref.ref ? 'true' : undefined} title={ref.current ? `${ref.ref} (current branch)` : ref.ref} {disabled} oncontextmenu={(e) => openMenu(e, ref.ref)} onkeydown={(e) => menuKey(e, ref.ref)} onclick={() => void gitHistory.selectRef(ref.ref)}>{@render branchIcon()}<span class="name">{ref.name}</span>{#if ref.current}<span class="star" role="img" aria-label="Current branch">★</span>{/if}</button>{/each}
    </details>{/if}
    {#if remotes.length}<details open><summary><span class="chevron" aria-hidden="true">▸</span><span class="label">Remote</span><span class="count">{shown.length - local.length - tags.length}</span></summary>
      {#each remotes as remote}{@const refs = shown.filter(ref => ref.remote && ref.name.split('/')[0] === remote)}<details open class="remote"><summary><span class="chevron" aria-hidden="true">▸</span><span class="label">{remote}</span><span class="count">{refs.length}</span></summary>
        {#each refs as ref (ref.ref)}<button type="button" class="ref" class:selected={gitHistory.selectedRef === ref.ref} aria-current={gitHistory.selectedRef === ref.ref ? 'true' : undefined} title={ref.ref} {disabled} oncontextmenu={(e) => openMenu(e, ref.ref)} onkeydown={(e) => menuKey(e, ref.ref)} onclick={() => void gitHistory.selectRef(ref.ref)}>{@render branchIcon()}<span class="name">{ref.name.slice(remote.length + 1)}</span></button>{/each}
      </details>{/each}
    </details>{/if}
    {#if tags.length}<details open><summary><span class="chevron" aria-hidden="true">▸</span><span class="label">Tags</span><span class="count">{tags.length}</span></summary>
      {#each tags as ref (ref.ref)}<button type="button" class="ref" class:selected={gitHistory.selectedRef === ref.ref} aria-current={gitHistory.selectedRef === ref.ref ? 'true' : undefined} title={ref.commit ? ref.ref : `${ref.ref} (not a commit)`} {disabled} oncontextmenu={(e) => openMenu(e, ref.ref)} onkeydown={(e) => menuKey(e, ref.ref)} onclick={() => { if (ref.commit) void gitHistory.selectRef(ref.ref); }}>
        <svg class="branch-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><path d="M8 2h6v6l-6 6-6-6z" /><circle cx="11" cy="5" r="1" /></svg><span class="name">{ref.name}</span>
      </button>{/each}
    </details>{/if}
    {#if needle && !shown.length}<p class="empty" role="status">No branches match “{filter.trim()}”.</p>{/if}
    <p class="hint">Remote refs are already fetched. Selection never switches or fetches a ref. Explicit context-menu actions may change the repository or remote.</p>
  </nav>
{/if}

{#if gitHistoryActions.error}<p class="action-error" role="alert">{gitHistoryActions.error}</p>{/if}
{#if gitHistoryActions.message}<p role="status">{gitHistoryActions.message}</p>{/if}
{#if menu}<ContextMenu x={menu.x} y={menu.y} {items} ariaLabel="History ref actions" onClose={() => menu = null} />{/if}

<style>
  .actions { padding: 3px 8px; border: 1px solid var(--border-color); border-radius: 5px; color: var(--text-primary); background: var(--bg-secondary); cursor: pointer; }
  .action-error { color: var(--error-color, var(--text-primary)); }
  .branches { padding: 12px 8px; font-size: .8rem; line-height: 1.4; }
  h3 { margin: 0 4px 10px; font-size: .85rem; font-weight: 600; color: var(--text-primary); }
  .filter-box { margin: 0 0 10px; }
  .search-icon { width: 14px; height: 14px; flex-shrink: 0; color: var(--text-muted); }
  .ref { font: inherit; display: flex; align-items: center; gap: 8px; width: calc(100% - 12px); margin: 2px 0 2px 12px; text-align: left; padding: 7px 8px; border: 0; background: transparent; color: var(--text-primary); border-radius: 6px; cursor: pointer; }
  .ref.head { width: 100%; margin: 0 0 8px; font-weight: 500; }
  .branch-icon { width: 16px; height: 16px; flex-shrink: 0; color: var(--text-muted); }
  .name, .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ref:hover, summary:hover { background: var(--hover-bg); }
  .ref.selected { background: var(--active-bg); color: var(--accent-color); }
  .ref.selected .branch-icon { color: inherit; }
  .ref:focus-visible, summary:focus-visible { outline: 2px solid var(--accent-color); outline-offset: -2px; }
  :disabled { opacity: .6; cursor: default; }
  details { margin-top: 4px; }
  summary { display: flex; align-items: center; gap: 6px; padding: 6px; border-radius: 6px; color: var(--text-secondary); cursor: pointer; list-style: none; font-weight: 500; }
  summary::-webkit-details-marker { display: none; }
  .chevron { flex: 0 0 10px; font-size: .7rem; color: var(--text-muted); }
  details[open] > summary .chevron { transform: rotate(90deg); }
  .remote { margin-left: 12px; }
  .count { flex-shrink: 0; min-width: 1.7em; padding: 1px 5px; border-radius: 5px; background: var(--bg-tertiary); color: var(--text-secondary); text-align: center; font-size: .7rem; font-variant-numeric: tabular-nums; }
  .star { flex-shrink: 0; color: var(--star-color); }
  p { color: var(--text-secondary); font-size: .75rem; line-height: 1.5; }
  .hint { margin: 14px 4px 0; padding-top: 10px; border-top: 1px solid var(--border-color); }
  .empty { margin: 12px 4px; }
  label { display: flex; align-items: center; gap: 6px; font-size: .8rem; line-height: 1.4; }
  select { min-width: 0; max-width: 220px; color: var(--text-primary); background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 6px; padding: 5px 7px; font: inherit; }
</style>
