<script lang="ts">
  import { files, tabId, tabOwner, tabHasUnsaved, type FileTab } from '../../../stores/files.svelte';
  import { fileActions } from '../../../stores/fileActions.svelte';
  import { TAB_CLOSE_ACTIONS, tabsToClose } from '../../../tabActions';
  import { externalLabel } from '../../../codeNavigation';
  import FileIcon from '../FileIcon.svelte';
  import ContextMenu from '../ContextMenu.svelte';
  import { TabDrag, revealTab, scrollSideways } from '../../../tabDrag.svelte';
  import { tick } from 'svelte';

  let { presentation, onContext }: { presentation: boolean; onContext: () => void } = $props();
  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);
  const closeShortcut = isMac ? '⌘W' : 'Ctrl+W';
  let tabContext = $state<{ path: string; x: number; y: number } | null>(null);
  let tabItems = $derived.by(() => {
    if (!tabContext) return [];
    const { path } = tabContext;
    return TAB_CLOSE_ACTIONS.map(({ action, label }) => {
      const targets = tabsToClose(files.visibleTabs.map(tabId), path, action);
      return { label, disabled: targets.length === 0, onSelect: () => void fileActions.closeTabs(targets) };
    });
  });

  let bar: HTMLDivElement | undefined = $state();
  let listToggle: HTMLButtonElement | undefined = $state();
  let listMenu: HTMLDivElement | undefined = $state();
  let list = $state<{ top: number; right: number } | null>(null);

  $effect(() => {
    if (!files.open) tabContext = list = null;
  });

  $effect(() => {
    if (files.visibleTabs.length === 0) list = null;
  });

  // Keep the selected tab in view, however it was selected.
  $effect(() => {
    void files.selected;
    void files.visibleTabs.length;
    void tick().then(revealSelected);
  });

  // A dropped tab is shown, as a click on it would.
  const drag = new TabDrag(
    () => bar,
    (id, to) => {
      if (files.moveTab(id, to)) files.selectTab(id);
    },
  );

  export function closeContext() {
    tabContext = list = null;
  }

  function revealSelected() {
    const tab = bar?.querySelector<HTMLElement>('.file-tab.active');
    if (bar && tab) revealTab(bar, tab);
  }

  function toggleList() {
    if (list || !listToggle) {
      list = null;
      return;
    }
    onContext();
    tabContext = null;
    const rect = listToggle.getBoundingClientRect();
    list = { top: rect.bottom + 2, right: Math.max(4, window.innerWidth - rect.right) };
    void tick().then(() => (listMenu?.querySelector<HTMLElement>('.list-item.active') ?? listItems()[0])?.focus());
  }

  function closeList(refocus: boolean) {
    list = null;
    if (refocus) listToggle?.focus();
  }

  function listItems(): HTMLButtonElement[] {
    return listMenu ? Array.from(listMenu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')) : [];
  }

  function pickTab(id: string) {
    closeList(false);
    files.selectTab(id);
  }

  // Stop propagation so the dialog's own shortcuts do not also react.
  function listKeydown(e: KeyboardEvent) {
    e.stopPropagation();
    const items = listItems();
    const at = items.indexOf(document.activeElement as HTMLButtonElement);
    let next = -1;
    if (e.key === 'ArrowDown') next = at + 1 >= items.length ? 0 : at + 1;
    else if (e.key === 'ArrowUp') next = at <= 0 ? items.length - 1 : at - 1;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = items.length - 1;
    else if (e.key === 'Escape' || e.key === 'Tab') {
      e.preventDefault();
      closeList(true);
      return;
    }
    if (next >= 0) {
      e.preventDefault();
      items[next]?.focus();
    }
  }

  function tabDir(tab: FileTab) {
    const path = tab.external ? externalLabel(tab.path, files.dir) : tab.path;
    return path.slice(0, Math.max(0, Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))));
  }

  function fileStatus(path: string) {
    return files.listing?.status?.[path];
  }

  // External files (outside the project) have Windows or POSIX host paths.
  function splitPath(path: string) {
    return { name: path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1) };
  }

  function tabLabel(tab: FileTab) {
    const name = splitPath(tab.path).name;
    if (tab.comparison) return `${name} · ${tab.comparison.label}`;
    return tab.history ? `${name} · ${tab.history.repo} @ ${tab.history.commit.slice(0, 8)}` : name;
  }

  function tabTitle(tab: FileTab) {
    if (tab.comparison) return `${tab.path} · ${tab.comparison.repo} · ${tab.comparison.label} · Saved disk comparison (refresh to update)`;
    if (tab.history) return `${tab.path} · ${tab.history.repo} @ ${tab.history.commit} · Read-only history${tab.history.oldPath ? ' · from ' + tab.history.oldPath : ''}`;
    const path = tab.external ? `${externalLabel(tab.path, files.dir)} · Read-only, outside the project` : tab.path;
    return tab.preview ? `${path} · Preview: double-click to keep it open` : path;
  }

  function closeTab(e: MouseEvent, path: string) {
    e.stopPropagation();
    void fileActions.closeTabs([path]);
  }
</script>

{#if tabContext}
  <ContextMenu x={tabContext.x} y={tabContext.y} items={tabItems} ariaLabel="File tab actions" onClose={closeContext} />
{/if}
<svelte:window
  onpointerdown={(e) => {
    if (list && !listMenu?.contains(e.target as Node) && !listToggle?.contains(e.target as Node)) closeList(false);
  }}
  onresize={() => closeList(false)}
  onblur={() => closeList(false)}
/>
{#if list}
  <div
    bind:this={listMenu}
    class="tab-list"
    role="menu"
    tabindex="-1"
    aria-label="Open files"
    style:top="{list.top}px"
    style:right="{list.right}px"
    onkeydown={listKeydown}
  >
    {#each files.visibleTabs as tab (tabId(tab))}
      {@const dir = tabDir(tab)}
      <button
        type="button"
        class="list-item"
        class:active={tabId(tab) === files.selected}
        class:preview={tab.preview}
        role="menuitem"
        title={tabTitle(tab)}
        onpointerenter={(e) => e.currentTarget.focus()}
        onclick={() => pickTab(tabId(tab))}
      >
        <FileIcon path={tab.path} />
        <span class="list-name">{tabLabel(tab)}</span>
        {#if dir}<span class="list-dir">{dir}</span>{/if}
        {#if tabHasUnsaved(tab)}<span class="list-unsaved" aria-label="Unsaved changes">●</span>{/if}
      </button>
    {/each}
  </div>
{/if}
{#if files.visibleTabs.length > 0}
  <div class="tab-strip" class:presentation>
  <div
    class="tab-bar"
    class:reordering={drag.id !== null}
    bind:this={bar}
    role="tablist"
    aria-label="Open files"
    onwheel={(e) => bar && scrollSideways(bar, e)}
  >
    {#each files.visibleTabs as tab, index (tabId(tab))}
      {@const st = tabOwner(tab) === 'history' || tab.external ? undefined : fileStatus(tab.path)}
      <div
        class="file-tab"
        class:active={tabId(tab) === files.selected}
        class:preview={tab.preview}
        role="tab"
        tabindex="0"
        aria-selected={tabId(tab) === files.selected}
        class:external={tab.external}
        class:dragging={drag.id === tabId(tab)}
        data-tab-id={tabId(tab)}
        style:transform={drag.id === null ? null : `translateX(${drag.offset(tabId(tab), index)}px)`}
        title={tabTitle(tab)}
        onpointerdown={(e) => drag.press(e, tabId(tab))}
        onclick={() => files.selectTab(tabId(tab))}
        ondblclick={() => files.keepTab(tabId(tab))}
        oncontextmenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onContext();
          tabContext = { path: tabId(tab), x: e.clientX, y: e.clientY };
        }}
        onauxclick={(e) => e.button === 1 && closeTab(e, tabId(tab))}
        onkeydown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            files.selectTab(tabId(tab));
          }
        }}
      >
        <FileIcon path={tab.path} />
        <span class="tab-name git-{st ?? 'none'}">{tabLabel(tab)}</span>
        {#if tab.external || tabOwner(tab) === 'history'}
          <svg class="tab-lock" viewBox="0 0 16 16" width="10" height="10" role="img" aria-label="Read-only">
            <rect x="3" y="7" width="10" height="7.5" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.4" />
            <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" fill="none" stroke="currentColor" stroke-width="1.4" />
          </svg>
        {/if}
        {#if tabHasUnsaved(tab)}<span aria-label="Unsaved changes" title="Unsaved changes">●</span>{/if}
        <button
          type="button"
          class="tab-close"
          aria-label="Close {splitPath(tab.path).name}"
          title="Close ({closeShortcut})"
          onclick={(e) => closeTab(e, tabId(tab))}>×</button
        >
      </div>
    {/each}
  </div>
  <button
    bind:this={listToggle}
    type="button"
    class="tab-list-toggle"
    class:open={list}
    aria-haspopup="menu"
    aria-expanded={!!list}
    aria-label="Show open files"
    title="Show open files"
    onclick={toggleList}
  >
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
      <path d="M2.5 4h11M2.5 8h11M2.5 12h11" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
    </svg>
  </button>
  </div>
{/if}

<style>
  .tab-strip.presentation { display: none; }
  .tab-strip {
    display: flex;
    flex-shrink: 0;
    border-bottom: 1px solid var(--border-color);
    background-color: var(--bg-secondary);
  }

  /* Open files, as VS Code's editor tabs. A preview tab is in italics. */
  .tab-bar {
    position: relative;
    display: flex;
    flex: 1;
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: thin;
  }

  .tab-list-toggle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 30px;
    border-left: 1px solid var(--border-color);
    color: var(--text-muted);
    cursor: pointer;
  }

  .tab-list-toggle:hover,
  .tab-list-toggle.open {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  .tab-list {
    position: fixed;
    z-index: 120;
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 220px;
    max-width: min(520px, calc(100vw - 16px));
    max-height: min(420px, calc(100vh - 80px));
    overflow-y: auto;
    padding: 4px;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-sm, 6px);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
    outline: none;
  }

  .list-item {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    width: 100%;
    padding: 5px 8px;
    border-radius: var(--radius-sm, 4px);
    font-size: 0.75rem;
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
    outline: none;
  }

  .list-item:focus {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }

  .list-item.active .list-name {
    color: var(--accent-color);
    font-weight: 600;
  }

  .list-item.preview .list-name {
    font-style: italic;
  }

  .list-name {
    flex-shrink: 0;
    max-width: 60%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .list-dir {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
  }

  .list-unsaved {
    flex-shrink: 0;
    margin-left: auto;
  }

  .file-tab {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    max-width: 220px;
    padding: 6px 6px 6px 12px;
    border-right: 1px solid var(--border-color);
    border-top: 2px solid transparent;
    font-size: 0.75rem;
    color: var(--text-secondary);
    cursor: pointer;
    user-select: none;
  }

  .file-tab:hover {
    background-color: var(--hover-bg);
  }

  /* Pressed on, the icon would start the webview's own image drag, which
     cancels the tab's. */
  .file-tab :global(img) {
    -webkit-user-drag: none;
  }

  /* The other tabs slide aside for a dragged one. */
  .tab-bar.reordering .file-tab:not(.dragging) {
    transition: transform 150ms ease;
  }

  @media (prefers-reduced-motion: reduce) {
    .tab-bar.reordering .file-tab:not(.dragging) {
      transition: none;
    }
  }

  .file-tab.active {
    background-color: var(--bg-primary);
    color: var(--text-primary);
    border-top-color: var(--accent-color);
  }

  /* A dragged tab is lifted above the ones it passes. */
  .file-tab.dragging {
    position: relative;
    z-index: 1;
    background-color: var(--bg-primary);
    box-shadow: 0 0 0 1px var(--border-color), 0 2px 8px rgba(0, 0, 0, 0.2);
  }

  .file-tab.preview .tab-name {
    font-style: italic;
  }

  .file-tab.external .tab-name {
    color: var(--text-muted);
  }

  .tab-lock {
    flex-shrink: 0;
    opacity: 0.7;
  }

  .tab-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tab-close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 18px;
    height: 18px;
    border-radius: 4px;
    font-size: 0.9rem;
    line-height: 1;
    color: var(--text-muted);
    visibility: hidden;
    cursor: pointer;
  }

  .file-tab.active .tab-close,
  .file-tab:hover .tab-close,
  .tab-close:focus-visible {
    visibility: visible;
  }

  .tab-close:hover {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  /* Git status, VS Code's colours. */
  .git-M,
  .git-R {
    color: #d29922;
  }
  .git-A,
  .git-U {
    color: #3fb950;
  }
  .git-D {
    color: #f85149;
  }
  .git-C {
    color: #db61a2;
  }
  :global(:root:not([data-theme='dark'])) .git-M,
  :global(:root:not([data-theme='dark'])) .git-R {
    color: #9a6700;
  }
  :global(:root:not([data-theme='dark'])) .git-A,
  :global(:root:not([data-theme='dark'])) .git-U {
    color: #1a7f37;
  }
  :global(:root:not([data-theme='dark'])) .git-D {
    color: #cf222e;
  }
  :global(:root:not([data-theme='dark'])) .git-C {
    color: #bf3989;
  }
</style>
