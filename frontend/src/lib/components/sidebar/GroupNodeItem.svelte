<script lang="ts">
  import Icon from '../common/Icon.svelte';
  import type { GroupNode } from '../../types';
  import GroupNodeItem from './GroupNodeItem.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';
  import ContextMenu, { type ContextMenuItem } from '../common/ContextMenu.svelte';
  import { directoryName, isCollapsed as isKeyCollapsed, nodeKind } from '../../tree';
  import { importer } from '../../stores/importer.svelte';
  import { files } from '../../stores/files.svelte';
  import { newSession } from '../../stores/newSession.svelte';
  import { newSessionMenuItems } from '../../newSession';
  import { workspaces } from '../../stores/workspaces.svelte';
  import { workspaceStars } from '../../stores/workspaceStars.svelte';

  interface Props {
    node: GroupNode;
    level?: number;
    selectedKey: string | null;
    /**
     * The row whose group filters the session list. The highlighted row
     * that is not it only holds the open session, and shows quieter.
     */
    filterKey?: string | null;
    collapsedKeys: Set<string>;
    defaultCollapsed: Set<string>;
    onSelect: (node: GroupNode) => void;
    onToggleCollapse: (key: string) => void;
  }

  let {
    node,
    level = 0,
    selectedKey,
    filterKey = null,
    collapsedKeys,
    defaultCollapsed,
    onSelect,
    onToggleCollapse,
  }: Props = $props();

  let hasChildren = $derived(Boolean(node.children && node.children.length > 0));
  let kind = $derived(nodeKind(node));
  let isCollapsed = $derived(isKeyCollapsed(node.key, collapsedKeys, defaultCollapsed));
  let isSelected = $derived(selectedKey === node.key);
  let isFilter = $derived(filterKey === node.key);
  // Directories show their name, with the whole path in a small line below.
  let name = $derived(kind === 'directory' ? directoryName(node.label) : node.label);
  let path = $derived(kind === 'directory' && name !== node.label ? node.label : '');

  function handleRowClick() {
    onSelect(node);
  }

  // Directory rows have an actions menu, opened by right-click or the ⋯
  // button.
  let menu = $state<{ x: number; y: number } | null>(null);
  let hasMenu = $derived(kind === 'directory' && Boolean(node.cwd));

  let menuItems = $derived.by((): ContextMenuItem[] => [
    {
      label: 'Go to File…',
      disabled: node.cwdMissing,
      title: node.cwdMissing ? 'This directory no longer exists on disk' : undefined,
      onSelect: () => void files.show(node.cwd!),
    },
    {
      label: 'Import Session Bundle Here…',
      disabled: node.cwdMissing,
      title: node.cwdMissing ? 'This directory no longer exists on disk' : undefined,
      onSelect: () => void importer.start(node.cwd),
    },
    ...(workspaces.directories.includes(node.cwd ?? '') ? [{
      label: 'Remove from workspace list',
      onSelect: () => void workspaces.remove(node.cwd!),
      title: 'Only removes the saved workspace. Files and session history are kept.',
    }] : []),
    ...(newSession.available
      ? newSessionMenuItems(newSession.choices, (agent) => void newSession.start(agent, node.cwd!), {
          separator: true,
          disabled: node.cwdMissing ? 'This directory no longer exists on disk' : undefined,
        })
      : []),
  ]);

  function openMenu(e: MouseEvent) {
    if (!hasMenu) return;
    e.preventDefault();
    menu = { x: e.clientX, y: e.clientY };
    void newSession.refresh();
  }

  function handleMenuButton(e: MouseEvent) {
    e.stopPropagation();
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: r.left, y: r.bottom };
    void newSession.refresh();
  }

  function handleDisclosureClick(e: MouseEvent) {
    e.stopPropagation();
    onToggleCollapse(node.key);
  }
</script>

<li
  class="group-node-container"
  role="treeitem"
  aria-selected={isSelected}
  aria-expanded={hasChildren ? !isCollapsed : undefined}
>
  <div
    class="node-row"
    class:dir={kind === 'directory'}
    class:group={kind === 'agent'}
    class:selected={isSelected && isFilter}
    class:current={isSelected && !isFilter}
    class:missing={node.cwdMissing}
    style="padding-left: {level * 12 + 6}px"
    onclick={handleRowClick}
    oncontextmenu={openMenu}
    role="button"
    tabindex="0"
    onkeydown={(e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        handleRowClick();
      }
    }}
  >
    {#if hasChildren}
      <button
        type="button"
        class="disclosure-btn"
        aria-label={isCollapsed ? 'Expand' : 'Collapse'}
        onclick={handleDisclosureClick}
      >
        <span class="disclosure-arrow" class:open={!isCollapsed}><Icon name="chevron-right" size={12} strokeWidth={2.25} /></span>
      </button>
    {:else}
      <span class="disclosure-spacer"></span>
    {/if}

    <div class="icon-area">
      {#if kind === 'agent'}
        <AgentIcon agent={node.agent} size={15} />
      {:else if kind === 'directory' && node.cwdMissing}
        <span class="node-icon missing" title="Directory no longer exists on disk"><Icon name="folder-x" size={15} /></span>
      {:else if kind === 'directory'}
        <span class="node-icon folder"><Icon name="folder" size={15} /></span>
      {:else}
        <AgentIcon agent={node.agent} />
      {/if}
    </div>

    <span class="node-label" title={node.secondary || node.label}>
      <span class="node-name">
        {name}
        {#if node.cwdMissing}
          <span class="missing-badge">[missing]</span>
        {/if}
      </span>
      {#if path}
        <span class="node-path">{path}</span>
      {/if}
    </span>

    {#if hasMenu}
      <button
        type="button"
        class="menu-btn"
        class:open={menu !== null}
        title="Directory actions"
        aria-label="Directory actions"
        aria-haspopup="menu"
        onclick={handleMenuButton}
        onkeydown={(e) => e.stopPropagation()}
      >
        ⋯
      </button>
    {/if}

    <span class="count-badge" title="{node.sessionCount} sessions">
      {node.sessionCount}
    </span>

    {#if kind === 'directory' && node.cwd}
      <button
        type="button"
        class="star-btn"
        class:starred={workspaceStars.isStarred(node.cwd)}
        aria-label={workspaceStars.isStarred(node.cwd) ? `Unstar workspace ${name}` : `Star workspace ${name}`}
        aria-pressed={workspaceStars.isStarred(node.cwd)}
        title={workspaceStars.isStarred(node.cwd) ? 'Unstar workspace' : 'Star workspace'}
        onclick={(e) => { e.stopPropagation(); workspaceStars.toggle(node.cwd!); }}
        onkeydown={(e) => e.stopPropagation()}
      >
        <Icon name="star" size={14} />
      </button>
    {/if}
  </div>

  {#if menu}
    <ContextMenu x={menu.x} y={menu.y} items={menuItems} ariaLabel="Directory actions" onClose={() => (menu = null)} />
  {/if}

  {#if hasChildren && !isCollapsed && node.children}
    <ul class="children-list" role="group">
      {#each node.children as child (child.key)}
        <GroupNodeItem
          node={child}
          level={level + 1}
          {selectedKey}
          {filterKey}
          {collapsedKeys}
          {defaultCollapsed}
          {onSelect}
          {onToggleCollapse}
        />
      {/each}
    </ul>
  {/if}
</li>

<style>
  .group-node-container {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .node-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-top: 7px;
    padding-bottom: 7px;
    padding-right: 8px;
    margin-bottom: 2px;
    border-radius: 4px;
    cursor: pointer;
    user-select: none;
    font-size: 0.825rem;
    color: var(--text-secondary);
    transition: background-color 0.1s ease, color 0.1s ease;
  }

  /* Directories read as section headers; agent groups sit beneath them. */
  .node-row.dir {
    font-weight: 600;
    color: var(--text-primary);
  }

  .node-row.group {
    font-weight: 500;
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .node-row:hover {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  .node-row.selected {
    background-color: var(--active-bg);
    color: var(--accent-color);
    font-weight: 600;
  }

  /* Holds the open session without filtering the list. */
  .node-row.current {
    background-color: var(--hover-bg);
    color: var(--text-primary);
    font-weight: 600;
  }

  .node-row.missing {
    opacity: 0.75;
  }

  .disclosure-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    padding: 0;
    cursor: pointer;
    color: var(--text-muted);
    border-radius: 2px;
  }

  .disclosure-btn:hover {
    color: var(--text-primary);
  }

  .disclosure-arrow {
    display: inline-flex;
    transition: transform 0.15s ease;
  }

  .disclosure-arrow.open {
    transform: rotate(90deg);
  }

  .disclosure-spacer {
    width: 16px;
    height: 16px;
    flex-shrink: 0;
  }

  .icon-area {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .node-icon {
    display: inline-flex;
    color: var(--text-muted);
  }

  .node-icon.missing {
    color: var(--status-needs);
  }

  .node-label {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .node-name,
  .node-path {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .node-path {
    margin-top: 1px;
    font-size: 0.68rem;
    font-weight: 400;
    color: var(--text-muted);
  }

  .missing-badge {
    margin-left: 4px;
    font-size: 0.7rem;
    color: var(--danger);
    font-weight: 500;
  }

  .star-btn {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    padding: 0;
    flex-shrink: 0;
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
  }

  .star-btn:hover {
    background-color: var(--bg-tertiary);
    color: var(--text-primary);
  }

  .star-btn.starred {
    color: var(--accent-color);
  }

  .star-btn.starred :global(svg) {
    fill: currentColor;
  }

  .star-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  .menu-btn {
    display: none;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    padding: 0;
    flex-shrink: 0;
    border-radius: 3px;
    color: var(--text-muted);
    font-size: 0.85rem;
    line-height: 1;
    cursor: pointer;
  }

  .node-row:hover .menu-btn,
  .node-row:focus-within .menu-btn,
  .menu-btn.open {
    display: flex;
  }

  .menu-btn:hover,
  .menu-btn.open {
    color: var(--text-primary);
    background-color: var(--bg-tertiary);
  }

  .count-badge {
    font-size: 0.7rem;
    padding: 1px 6px;
    border-radius: 10px;
    background-color: var(--bg-tertiary);
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .selected .count-badge {
    background-color: rgba(37, 99, 235, 0.2);
    color: var(--accent-color);
  }

  .children-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
</style>
