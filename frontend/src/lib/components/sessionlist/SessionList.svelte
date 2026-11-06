<script lang="ts">
  import type { SessionMeta, SortOpts } from '../../types';
  import { appState } from '../../stores/appState.svelte';
  import { discovery } from '../../stores/discovery.svelte';
  import { catalogNotice, emptyCatalogState, discoveryProblem } from '../../discovery';
  import { onDestroy, tick, untrack } from 'svelte';
  import VirtualList from './VirtualList.svelte';
  import SessionListSkeleton from './SessionListSkeleton.svelte';
  import SessionRow from './SessionRow.svelte';
  import EmptyState from '../common/EmptyState.svelte';
  import Icon from '../common/Icon.svelte';
  import ErrorState from '../common/ErrorState.svelte';
  import { manage } from '../../stores/manage.svelte';
  import { AGE_FILTERS, filterSessionsByAge, isAgeFilter, refKey } from '../../manage';
  import {
    ancestorKeys, buildSessionTree, collectContext, expansionKey, flattenSessionTree, hasPickedAncestor,
    isExpanded, pickedForDelete, type SessionTreeNode, type SessionTreeRow,
  } from '../../sessionTree';
  import Dropdown from '../common/Dropdown.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { link } from '../../stores/link.svelte';
  import ContextMenu, { type ContextMenuItem } from '../common/ContextMenu.svelte';
  import { sessionRename } from '../../stores/sessionRename.svelte';

  // Rendered outside VirtualList: its transformed, contained items would become
  // the containing block of a fixed-position menu and clip it.
  let sessionMenu = $state<{ session: SessionMeta; x: number; y: number } | null>(null);
  function openSessionMenu(session: SessionMeta, x: number, y: number) {
    sessionMenu = { session, x, y };
  }
  // ContextMenu closes (clearing sessionMenu) before running an action, so each
  // action must capture its session here rather than read sessionMenu later.
  function sessionMenuItems(session: SessionMeta): ContextMenuItem[] {
    return [
      { label: 'Rename session…', disabled: link.stale || link.locked || sessionRename.busy, onSelect: () => sessionRename.open(session) },
    ];
  }

  import { nodeDirectory } from '../../tree';
  import { workspaces } from '../../stores/workspaces.svelte';
  import WorkspaceStart from './WorkspaceStart.svelte';

  let selectedWorkspace = $derived(nodeDirectory(appState.groups, appState.selectedGroupKey ?? ''));
  let emptyWorkspace = $derived(selectedWorkspace && workspaces.directories.includes(selectedWorkspace) && appState.sessions.length === 0 && !appState.filter.query && !appState.filter.agent && !appState.filter.liveOnly && !appState.filter.archived && !appState.filter.hasSubagents && !appState.filter.hideSubagents);

  const AGE_OPTIONS = AGE_FILTERS.map((f) => ({ value: f.id, label: f.label }));

  const SORT_OPTIONS = [
    { value: 'updated', label: 'Updated' },
    { value: 'created', label: 'Created' },
    { value: 'title', label: 'Title' },
    { value: 'messages', label: 'Messages' },
    { value: 'tokens', label: 'Tokens' },
    { value: 'cost', label: 'Cost' },
  ];

  let listEl: HTMLElement | null = $state(null);

  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let visibleSessions = $derived(
    filterSessionsByAge(appState.sessions, manage.ageFilter)
  );

  // Subagents sit under the session that spawned it. When a subagent matches
  // the filters and its parent does not, the parent is listed too, dimmed:
  // from the loaded list when only the age filter left it out, otherwise
  // fetched. Null marks a parent the backend does not have.
  let fetchedContext = $state(new Map<string, SessionMeta | null>());
  let contextRequests = new Set<string>();
  let loadedByKey = $derived(new Map(appState.sessions.map((s) => [refKey(s.ref), s])));
  let contextScan = $derived(
    collectContext(visibleSessions, (key) => loadedByKey.get(key) ?? fetchedContext.get(key))
  );

  // Another filter, group or host can bring other parents; refetch them.
  $effect(() => {
    void [appState.filter, appState.selectedGroupKey, link.dataHost];
    untrack(() => {
      fetchedContext = new Map();
      contextRequests = new Set();
    });
  });

  $effect(() => {
    const requests = contextRequests;
    for (const ref of contextScan.missing) {
      const key = refKey(ref);
      if (requests.has(key)) continue;
      requests.add(key);
      void appState.lookupSession(ref).then((meta) => {
        if (requests !== contextRequests) return;
        fetchedContext = new Map(fetchedContext).set(key, meta);
      });
    }
  });

  let tree = $derived(buildSessionTree(visibleSessions, contextScan.context));
  // Open or closed per family, overriding the default: closed, except a
  // context parent, which is listed only to show its subagents.
  let expansion = $state(new Map<string, boolean>());
  let rows = $derived(flattenSessionTree(tree, expansion));
  let subagentCount = $derived(visibleSessions.filter((s) => s.parentId).length);
  let anyExpanded = $derived(rows.some((r) => r.expanded));

  function setExpanded(node: SessionTreeNode, open: boolean) {
    expansion = new Map(expansion).set(expansionKey(node), open);
  }

  function toggleFamily(session: SessionMeta) {
    const node = tree.byKey.get(refKey(session.ref));
    if (node) setExpanded(node, !isExpanded(node, expansion));
  }

  function collapseAll() {
    const next = new Map<string, boolean>();
    for (const node of tree.byKey.values()) {
      if (node.children.length > 0) next.set(expansionKey(node), false);
    }
    expansion = next;
  }

  let virtualList: { ensureVisible(index: number): void } | undefined = $state();

  // The row standing for the selected session: its own, or, while its
  // family is closed, its nearest listed ancestor's.
  function selectedRowIndex(): number {
    const ref = appState.selectedSessionRef;
    if (!ref) return -1;
    const key = refKey(ref);
    for (const k of [key, ...ancestorKeys(tree, key)]) {
      const i = rows.findIndex((r) => r.node.key === k);
      if (i >= 0) return i;
    }
    return -1;
  }

  // A newly selected session is opened up to and scrolled into view, once:
  // closing its family afterwards, or a background reload, leaves it be.
  let revealedKey = '';
  $effect(() => {
    const ref = appState.selectedSessionRef;
    const key = ref ? refKey(ref) : '';
    if (key === revealedKey) return;
    if (!key) {
      revealedKey = '';
      return;
    }
    if (!tree.byKey.has(key)) return;
    revealedKey = key;
    untrack(() => {
      const closed = ancestorKeys(tree, key)
        .map((k) => tree.byKey.get(k)!)
        .filter((node) => !isExpanded(node, expansion));
      if (closed.length > 0) {
        const next = new Map(expansion);
        for (const node of closed) next.set(expansionKey(node), true);
        expansion = next;
      }
    });
    void tick().then(() => virtualList?.ensureVisible(selectedRowIndex()));
  });

  let narrowed = $derived(Boolean(
    appState.filter.query || appState.filter.path || appState.filter.agent ||
    appState.filter.liveOnly || appState.filter.hasSubagents || appState.filter.hideSubagents || appState.filter.archived ||
    appState.selectedGroupKey || manage.ageFilter !== 'any'
  ));
  let emptyState = $derived(
    emptyCatalogState(discovery.outcome, discovery.status, narrowed, appState.discoveryReloading)
  );
  let notice = $derived(catalogNotice(discovery.outcome, discovery.status));
  let problem = $derived(
    emptyState === 'failed' || emptyState === 'degraded'
      ? discoveryProblem(emptyState, discovery.status)
      : null
  );

  // Picking a parent covers its subagents: its delete takes them along.
  let picked = $derived<ReadonlySet<string>>(manage.selectedRefKeys ?? new Set());
  let toDelete = $derived(pickedForDelete(tree, picked));
  let selectedVisibleCount = $derived(toDelete.length);
  let allPicked = $derived(
    visibleSessions.length > 0 &&
      visibleSessions.every((s) => {
        const key = refKey(s.ref);
        return picked.has(key) || hasPickedAncestor(tree, key, picked);
      })
  );

  // Debounce so fast typing doesn't trigger a backend query per keystroke.
  function handleSearchInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      appState.setFilter({ query: val });
    }, 250);
  }

  onDestroy(() => {
    clearTimeout(searchTimer);
  });

  function toggleLiveOnly() {
    appState.setFilter({ liveOnly: !appState.filter.liveOnly });
  }

  function toggleArchived() {
    appState.setFilter({ archived: !appState.filter.archived });
  }

  function handleSortChange(field: string) {
    appState.setSort({ field, desc: appState.sort.desc });
  }

  function toggleSortDirection() {
    appState.setSort({ field: appState.sort.field, desc: !appState.sort.desc });
  }

  function handleSelectSession(session: SessionMeta) {
    if (!session.cwdMissing) terminals.selectDirectory(session.cwd, link.dataHost ?? '');
    appState.selectSession(session.ref, 'push');
  }

  function handleClearFilters() {
    clearTimeout(searchTimer);
    appState.setFilter({ query: '', liveOnly: false, archived: false, hideSubagents: false });
    manage.setAgeFilter('any');
  }

  function handleAgeChange(value: string) {
    manage.setAgeFilter(isAgeFilter(value) ? value : 'any');
    manage.clearSelection();
  }

  function toggleSelectAll() {
    if (allPicked) {
      manage.clearSelection();
    } else {
      manage.selectAll(visibleSessions);
    }
  }

  function handleBulkDelete() {
    if (!toDelete.length) return;
    void manage.requestDelete(toDelete);
  }

  function stepTo(index: number) {
    const row = rows[index];
    if (row) void appState.selectSession(row.node.session.ref, 'step');
  }

  // Left and Right open and close families, but only from the rows: in the
  // search field they move the caret.
  function fromRows(target: EventTarget | null): boolean {
    return target === listEl || (target instanceof Element && target.closest('.list-body') !== null);
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (rows.length === 0) return;
    const current = selectedRowIndex();

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      stepTo(Math.min(rows.length - 1, current + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      stepTo(Math.max(0, current - 1));
    } else if ((e.key === 'ArrowRight' || e.key === 'ArrowLeft') && fromRows(e.target)) {
      const row = rows[current];
      if (!row) return;
      e.preventDefault();
      if (e.key === 'ArrowRight') {
        if (row.node.children.length === 0) return;
        if (row.expanded) stepTo(current + 1);
        else setExpanded(row.node, true);
      } else if (row.expanded) {
        setExpanded(row.node, false);
      } else if (row.node.parentKey) {
        stepTo(rows.findIndex((r) => r.node.key === row.node.parentKey));
      }
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  bind:this={listEl}
  class="session-list-pane"
  role="region"
  aria-label="Session list"
  onkeydown={handleKeyDown}
  tabindex="-1"
>
  <div class="search-toolbar">
    <div class="text-field">
      <Icon name="search" size={14} class="search-icon" />
      <input
        type="search"
        placeholder="Filter sessions (title, prompt, cwd, model)…"
        value={appState.filter.query || ''}
        oninput={handleSearchInput}
        aria-label="Search sessions"
        autocomplete="off"
        autocorrect="off"
        autocapitalize="off"
        spellcheck="false"
      />
    </div>

    <div class="filter-controls">
      <button
        type="button"
        class="filter-btn live-btn"
        class:active={appState.filter.liveOnly}
        title="Show only active sessions"
        onclick={toggleLiveOnly}
      >
        <span class="live-indicator">●</span> Live
      </button>

      <button
        type="button"
        class="filter-btn"
        class:active={appState.filter.archived}
        title="Include archived sessions"
        onclick={toggleArchived}
      >
        Archived
      </button>

      <!-- Icon only: the row has no room for another label. Subagents start
           tucked under their parents; this tucks every open family back. -->
      <button
        type="button"
        class="filter-btn icon-btn"
        aria-label="Collapse all subagent sessions"
        title="Collapse all subagent sessions"
        disabled={!anyExpanded}
        onclick={collapseAll}
      >
        <Icon name="chevrons-down-up" size={13} />
      </button>

      <!-- A view filter, so it stays visible on hosts without session management. -->
      <Dropdown
        options={AGE_OPTIONS}
        value={manage.ageFilter}
        onChange={handleAgeChange}
        ariaLabel="Filter sessions by age"
      />

      <div class="sort-wrapper">
        <Dropdown
          options={SORT_OPTIONS}
          value={appState.sort.field}
          onChange={handleSortChange}
          ariaLabel="Sort sessions by"
          align="right"
        />
        <button
          type="button"
          class="sort-dir-btn"
          title={appState.sort.desc ? 'Descending (click for ascending)' : 'Ascending (click for descending)'}
          onclick={toggleSortDirection}
          aria-label="Toggle sort order"
        >
          {appState.sort.desc ? '↓' : '↑'}
        </button>
      </div>
    </div>
  </div>

  <div class="list-subhead">
    <span class="session-count">
      {visibleSessions.length - subagentCount} {visibleSessions.length - subagentCount === 1 ? 'session' : 'sessions'}
      {#if subagentCount > 0}
        · {subagentCount} {subagentCount === 1 ? 'subagent' : 'subagents'}
      {/if}
    </span>

    {#if notice && visibleSessions.length > 0}
      <span class="discovery-notice" class:warning={notice.tone === 'warning'} role="status" title={notice.title}>
        {notice.text}
      </span>
    {/if}

    {#if manage.settings.enabled}
      <label class="select-all-label">
        <input
          type="checkbox"
          class="bulk-check"
          checked={allPicked}
          onclick={toggleSelectAll}
        />
        Select all
      </label>

      <button
        type="button"
        class="bulk-delete-btn"
        disabled={selectedVisibleCount === 0 || manage.deleting}
        title="Delete the selected sessions"
        onclick={handleBulkDelete}
      >
        Delete ({selectedVisibleCount})
      </button>
    {/if}
  </div>

  <div class="list-body">
    {#if appState.loadingSessions}
      <SessionListSkeleton />
    {:else if appState.error}
      <ErrorState
        title="Could not load sessions"
        message="The session list could not be loaded."
        details={appState.error}
        onRetry={() => appState.reload()}
      />
    {:else if visibleSessions.length === 0}
      {#if emptyWorkspace && selectedWorkspace}
        <WorkspaceStart cwd={selectedWorkspace} />
      {:else if emptyState === 'discovering'}
        <div class="discovering" role="status">
          <EmptyState
            icon="loader-circle"
            title="Discovering sessions…"
            description="Looking for agent session history. Sessions appear here as they are found."
          />
        </div>
      {:else if emptyState === 'no-agents' || emptyState === 'no-sessions'}
        <EmptyState
          icon="inbox"
          title="No sessions found"
          description={emptyState === 'no-agents'
            ? 'No supported agents were detected.'
            : 'Discovery finished without finding any session history.'}
        />
      {:else if problem}
        <EmptyState icon="triangle-alert" title={problem.title} description={problem.description} />
      {:else}
        <EmptyState
          icon="search"
          title="No matching sessions"
          description="No sessions match the current search and filters."
          actionText="Clear filters"
          onAction={handleClearFilters}
        />
      {/if}
    {:else}
      <VirtualList bind:this={virtualList} items={rows} itemHeight={84} overscan={5} role="tree" label="Sessions">
        {#snippet children(row: SessionTreeRow)}
          <SessionRow
            session={row.node.session}
            isSelected={appState.selectedSessionRef?.agent === row.node.session.ref.agent &&
              appState.selectedSessionRef?.id === row.node.session.ref.id}
            onSelect={handleSelectSession}
            onMenu={openSessionMenu}
            depth={row.depth}
            childCount={row.node.children.length}
            expanded={row.expanded}
            onToggle={toggleFamily}
            context={row.node.context}
            covered={hasPickedAncestor(tree, row.node.key, picked)}
          />
        {/snippet}
      </VirtualList>
    {/if}
    {#if sessionMenu}
      <ContextMenu x={sessionMenu.x} y={sessionMenu.y} ariaLabel="Session actions"
        items={sessionMenuItems(sessionMenu.session)} onClose={() => (sessionMenu = null)} />
    {/if}
  </div>
</div>

<style>
  .session-list-pane {
    outline: none;
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    background-color: var(--bg-primary);
    overflow: hidden;
  }

  .search-toolbar {
    padding: 8px 10px;
    border-bottom: 1px solid var(--border-color);
    background-color: var(--bg-primary);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .search-toolbar :global(.search-icon) {
    color: var(--text-muted);
  }

  /* Every control in the row shares one height (also read by Dropdown). */
  /* Wraps instead of clipping in a narrow pane; sorting then keeps right. */
  .filter-controls {
    --control-height: 24px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }

  .filter-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: var(--control-height);
    padding: 0 7px;
    white-space: nowrap;
    border-radius: var(--radius-sm);
    font-size: 0.72rem;
    font-weight: 500;
    background-color: var(--bg-secondary);
    border: none;
    color: var(--text-secondary);
    transition: all 0.15s ease;
  }

  .filter-btn:hover:not(:disabled) {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .filter-btn:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .filter-btn.active {
    background-color: var(--active-bg);
    color: var(--accent-color);
    font-weight: 600;
  }

  .icon-btn {
    position: relative;
    justify-content: center;
    flex: none;
    width: var(--control-height);
    padding: 0;
  }

  .live-btn .live-indicator {
    color: var(--status-live);
    font-size: 0.75rem;
  }

  .sort-wrapper {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .sort-dir-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-height);
    height: var(--control-height);
    padding: 0;
    font-size: 0.75rem;
    font-weight: bold;
    border-radius: var(--radius-sm);
    background-color: var(--bg-secondary);
    border: none;
    color: var(--text-secondary);
    transition: all 0.15s ease;
  }

  .sort-dir-btn:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .list-subhead {
    padding: 4px 10px;
    background-color: var(--bg-secondary);
    border-bottom: 1px solid var(--border-color);
    font-size: 0.7rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .select-all-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 0.7rem;
    cursor: pointer;
  }

  .bulk-delete-btn {
    margin-left: auto;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    font-size: 0.7rem;
    font-weight: 500;
    background-color: color-mix(in srgb, var(--danger) 12%, transparent);
    color: var(--danger);
    border: none;
    cursor: pointer;
  }

  .bulk-delete-btn:hover:not(:disabled) {
    background-color: color-mix(in srgb, var(--danger) 20%, transparent);
  }

  .bulk-delete-btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .discovery-notice {
    white-space: nowrap;
  }

  .discovery-notice.warning {
    color: var(--text-secondary);
  }

  .discovering {
    height: 100%;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.5;
    }
  }

  .list-body {
    flex: 1;
    overflow: hidden;
    position: relative;
  }
</style>
