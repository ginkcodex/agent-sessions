<script lang="ts">
  import type { GitHistoryChange } from '../../../gitHistory';
  import FileIcon from '../FileIcon.svelte';
  let { changes, truncated = false, selectedPath = null, onOpen }: {
    changes: GitHistoryChange[]; truncated?: boolean; selectedPath?: string | null;
    onOpen: (change: GitHistoryChange, keep: boolean) => void;
  } = $props();
  let collapsed = $state(new Set<string>());
  let tree: HTMLElement | undefined = $state();
  // One level of directory grouping keeps deep paths readable without a nested tree.
  let groups = $derived.by(() => {
    const byDir = new Map<string, GitHistoryChange[]>();
    for (const change of changes) {
      const slash = change.path.lastIndexOf('/');
      const dir = slash < 0 ? '' : change.path.slice(0, slash);
      byDir.set(dir, [...(byDir.get(dir) ?? []), change]);
    }
    return [...byDir].sort(([a], [b]) => a.localeCompare(b));
  });
  function statusLabel(status: string) {
    return ({ A: 'Added', D: 'Deleted', M: 'Modified', R: 'Renamed', T: 'Type changed', C: 'Copied' } as Record<string, string>)[status[0]] ?? status;
  }
  function baseName(path: string) { return path.slice(path.lastIndexOf('/') + 1); }
  function toggle(dir: string) {
    const next = new Set(collapsed);
    if (!next.delete(dir)) next.add(dir);
    collapsed = next;
  }
  /** Arrow keys walk visible rows; Left/Right fold directories like a file tree. */
  function navigate(event: KeyboardEvent, dir: string, change?: GitHistoryChange) {
    const rows = [...(tree?.querySelectorAll<HTMLElement>('[data-tree-row]') ?? [])];
    const index = rows.indexOf(event.currentTarget as HTMLElement);
    const focus = (row: HTMLElement | undefined) => { if (row) { event.preventDefault(); row.focus(); } };
    if (event.key === 'ArrowDown') focus(rows[index + 1]);
    else if (event.key === 'ArrowUp') focus(rows[index - 1]);
    else if (event.key === 'Home') focus(rows[0]);
    else if (event.key === 'End') focus(rows[rows.length - 1]);
    else if (event.key === 'ArrowLeft' && change && dir) focus(rows.find(row => row.dataset.dir === dir && !row.dataset.file));
    else if (!change && (event.key === 'ArrowLeft' || event.key === 'ArrowRight') && collapsed.has(dir) === (event.key === 'ArrowRight')) { event.preventDefault(); toggle(dir); }
    else if (change && event.key === 'Enter') { event.preventDefault(); onOpen(change, event.ctrlKey || event.metaKey); }
  }
</script>
    <section class="changed-files" aria-label="Changed files">
      <h3>Changed files <span class="count">{changes.length}{truncated ? '+' : ''}</span></h3>
      {#if truncated}<p class="notice" role="status">Some changed files are not listed (limit reached or names cannot be represented as UTF-8).</p>{/if}
      {#if changes.length === 0}<p class="state">No changed files in this workspace scope.</p>{/if}
      <div class="tree" role="tree" aria-label="Changed files by directory" bind:this={tree}>
        {#each groups as [dir, changes] (dir)}
          {#if dir}
            <button type="button" class="dir" role="treeitem" aria-level="1" aria-selected="false" aria-expanded={!collapsed.has(dir)} data-tree-row data-dir={dir}
              onclick={() => toggle(dir)} onkeydown={event => navigate(event, dir)}>
              <span class="chevron" class:open={!collapsed.has(dir)} aria-hidden="true">▸</span><FileIcon path={dir} folder expanded={!collapsed.has(dir)} />
              <span class="dir-name" title={dir}>{dir}</span><span class="count">{changes.length}</span>
            </button>
          {/if}
          {#if !collapsed.has(dir)}
            {#each changes as change (change.path)}
              <button type="button" class="changed-file" class:nested={!!dir} class:active={selectedPath === change.path} role="treeitem"
                aria-selected={selectedPath === change.path} aria-level={dir ? 2 : 1} data-tree-row data-dir={dir} data-file={change.path}
                title={`${statusLabel(change.status)}: ${change.oldPath ? change.oldPath + ' → ' : ''}${change.path}. Double-click or Ctrl+Enter keeps the diff open.`}
                onclick={() => onOpen(change, false)} ondblclick={() => onOpen(change, true)} onkeydown={event => navigate(event, dir, change)}>
                <FileIcon path={change.path} />
                <span class="path"><span class="file-name status-{change.status[0]}">{baseName(change.path)}</span>{#if change.oldPath}<span class="old-path">from {change.oldPath}</span>{/if}</span>
                <span class="status status-{change.status[0]}" aria-label={statusLabel(change.status)}>{change.status[0]}</span>
              </button>
            {/each}
          {/if}
        {/each}
      </div>
    </section>
<style>
  button { font: inherit; }
  .details { font-size: .8rem; line-height: 1.4; display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .changed-files { flex: 1 1 50%; min-height: 80px; overflow: auto; }
  h3 { display: flex; align-items: center; gap: 8px; margin: 0; padding: 12px; font-size: .85rem; font-weight: 600; }
  .tree { padding: 0 6px 8px; }
  .changed-file, .dir { display: flex; align-items: center; gap: 8px; width: 100%; border: 0; border-radius: 6px; margin: 2px 0; padding: 7px 8px; background: transparent; color: var(--text-primary); text-align: left; cursor: pointer; font-size: .8rem; }
  .changed-file:hover, .dir:hover { background: var(--hover-bg); }
  .changed-file.active { background: var(--active-bg); }
  .changed-file:focus-visible, .dir:focus-visible { outline: 2px solid var(--accent-color); outline-offset: -2px; }
  .path { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .file-name { display: block; }
  .file-name.status-D { text-decoration: line-through; }
  .status { flex: 0 0 auto; min-width: 22px; padding: 1px 5px; border-radius: 4px; font-family: monospace; font-size: .7rem; font-weight: 600; text-align: center; color: var(--text-secondary); background: color-mix(in srgb, currentColor 10%, transparent); }
  .old-path { display: block; color: var(--text-secondary); margin-top: 2px; font-size: .75rem; }
  .dir { color: var(--text-secondary); font-weight: 500; margin-top: 6px; }
  .dir-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .chevron { flex: 0 0 10px; color: var(--text-muted); font-size: .7rem; }
  .chevron.open { transform: rotate(90deg); }
  .count { flex-shrink: 0; min-width: 1.7em; padding: 1px 5px; border-radius: 5px; color: var(--text-secondary); background: var(--bg-tertiary); font-size: .7rem; font-weight: 500; text-align: center; font-variant-numeric: tabular-nums; }
  .changed-file.nested { padding-left: 26px; }
  .status-A { color: #1a7f37; } .status-D { color: #cf222e; }
  .status-M, .status-R { color: #9a6700; }
  .status-C, .status-T { color: var(--accent-color); }
  :global([data-theme='dark']) .status-A { color: #3fb950; }
  :global([data-theme='dark']) .status-D { color: #f85149; }
  :global([data-theme='dark']) .status-M, :global([data-theme='dark']) .status-R { color: #d29922; }
  .state, .notice { padding: 8px 10px; color: var(--text-secondary); font-size: .8rem; line-height: 1.4; }
</style>
