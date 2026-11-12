<script lang="ts">
  import type { SessionMeta } from '../../types';
  import { formatRelativeTime, formatAbsoluteTime } from '../../date';
  import CountTooltip from './CountTooltip.svelte';
  import { manage } from '../../stores/manage.svelte';
  import { preferences } from '../../stores/preferences.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { link } from '../../stores/link.svelte';
  import AttentionDot from '../common/AttentionDot.svelte';
  import Icon from '../common/Icon.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';
  import { sessionRename } from '../../stores/sessionRename.svelte';

  interface Props {
    session: SessionMeta;
    isSelected: boolean;
    onSelect: (session: SessionMeta) => void;
    /** Asks the list to open the session menu at viewport coordinates. The
     * list renders it: virtual-list items are transformed and contained, which
     * would clip a fixed-position menu drawn inside the row. */
    onMenu?: (session: SessionMeta, x: number, y: number) => void;
    /** Nesting under the session that spawned it; 0 for a top-level row. */
    depth?: number;
    /** Subagent sessions listed under this one. */
    childCount?: number;
    expanded?: boolean;
    onToggle?: (session: SessionMeta) => void;
    /** Listed only as the parent of a matching subagent; dimmed. */
    context?: boolean;
    /** An ancestor is picked for deletion, which takes this session too. */
    covered?: boolean;
  }

  let {
    session, isSelected, onSelect, onMenu,
    depth = 0, childCount = 0, expanded = false, onToggle, context = false, covered = false,
  }: Props = $props();

  // Indentation stops growing after a few levels; deeper rows still say their
  // level to assistive tech.
  const MAX_INDENT = 3;
  let indent = $derived(Math.min(depth, MAX_INDENT));

  function handleToggle(e: MouseEvent) {
    e.stopPropagation();
    onToggle?.(session);
  }

  // Enter and Space press the button; the row must not also open the session.
  function handleToggleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') e.stopPropagation();
  }

  let showCountTooltip = $state(false);
  function openMenu(event: MouseEvent) {
    if (!onMenu) return;
    event.preventDefault();
    event.stopPropagation();
    onMenu(session, event.clientX, event.clientY);
  }

  let messageTotal = $derived(session.counts.user + session.counts.assistant);
  let displayTitle = $derived(session.title || session.firstPrompt || '(untitled)');
  let relativeTime = $derived(formatRelativeTime(session.updatedAt));
  let absoluteTime = $derived(formatAbsoluteTime(session.updatedAt, preferences.timeZone));
  // Named beside the mark, so the row still says whose session it is while
  // the manage checkbox covers the mark.
  let agentName = $derived.by(() => {
    const agent = session.ref.agent;
    if (agent === 'claude-code') return 'Claude Code';
    if (agent === 'codex') return 'Codex';
    if (agent === 'opencode') return 'OpenCode';
    return agent || 'Session';
  });
  let bulkSelected = $derived(manage.isRefSelected(session.ref));
  // Once any session is picked, every row shows its checkbox; before that a
  // row shows it only under the pointer or focus.
  let selecting = $derived((manage.selectedRefKeys?.size ?? 0) > 0);
  // Where the agent running this session in a terminal tab stands.
  let attention = $derived.by(() => {
    const tab = terminals.tabFor(session.ref, link.dataHost ?? '');
    return tab ? terminals.attention[tab.id] : undefined;
  });

  function handleClick() {
    onSelect(session);
  }

  function handleBulkToggle(e: Event) {
    e.stopPropagation();
    manage.toggleRefSelected(session.ref);
  }

  function handleBulkKeydown(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleBulkToggle(e);
    }
  }
</script>

<div
  class="session-row agent-{session.ref.agent}"
  class:selected={isSelected}
  class:bulk-selected={bulkSelected || covered}
  class:archived={session.archived}
  class:nested={indent > 0}
  class:context
  style="--indent: {indent};"
  title={context ? 'Does not match the filters; shown because a subagent under it does' : undefined}
  role="treeitem"
  tabindex="0"
  aria-selected={isSelected}
  aria-level={depth + 1}
  aria-expanded={childCount > 0 ? expanded : undefined}
  onclick={handleClick}
  oncontextmenu={openMenu}
  onkeydown={(e) => {
    if (e.key === 'F2') { e.preventDefault(); e.stopPropagation(); sessionRename.open(session); return; }
    if (onMenu && (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10'))) {
      e.preventDefault(); e.stopPropagation();
      const rect = e.currentTarget.getBoundingClientRect();
      onMenu(session, rect.left + 12, rect.top + 24);
      return;
    }
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleClick();
    }
  }}
>
  <div class="row-top">
    {#if attention === 'working' || attention === 'needs' || attention === 'done'}
      <AttentionDot state={attention} size={6} announce />
    {:else if session.live}
      <span class="live-dot" title="Active session"></span>
    {/if}
    <!-- The agent's mark; in manage mode its checkbox takes its place. -->
    <span class="lead" class:checkable={manage.settings.enabled} class:checking={bulkSelected || covered || selecting}>
      <AgentIcon agent={session.ref.agent} size={12} />
      {#if manage.settings.enabled}
        <input
          type="checkbox"
          class="bulk-check"
          checked={bulkSelected || covered}
          disabled={covered}
          title={covered ? 'Deleted along with its parent' : undefined}
          onclick={handleBulkToggle}
          onkeydown={handleBulkKeydown}
          aria-label="Select session {session.title || session.ref.id}"
        />
      {/if}
    </span>
    <span class="agent-name" class:subagent={Boolean(session.parentId)}>{agentName}</span>
    {#if childCount > 0}
      <button
        type="button"
        class="subagent-toggle"
        class:open={expanded}
        aria-expanded={expanded}
        title={expanded ? 'Hide its subagent sessions' : 'Show its subagent sessions'}
        onclick={handleToggle}
        onkeydown={handleToggleKeydown}
      >
        <Icon name="chevron-right" size={11} class="toggle-chevron" />
        {childCount} {childCount === 1 ? 'subagent' : 'subagents'}
      </button>
    {/if}
    {#if session.archived}
      <span class="archived-tag">Archived</span>
    {/if}
    <span
      class="time-label"
      title={absoluteTime}
    >
      {relativeTime}
    </span>
  </div>

  <div class="row-middle">
    <span class="title-text" class:subagent={Boolean(session.parentId)} title={displayTitle}>
      {#if session.parentId}<Icon name="corner-down-right" size={12} class="subagent-icon" label="Subagent" />{/if}
      {displayTitle}
    </span>
  </div>

  <div class="row-bottom">
    <div
      class="count-wrapper"
      role="region"
      aria-label="Message count details"
      onmouseenter={() => (showCountTooltip = true)}
      onmouseleave={() => (showCountTooltip = false)}
    >
      <span class="count-badge">
        <Icon name="message-square" size={11} class="count-icon" />
        <span class="count-num">{messageTotal}</span>
      </span>
      {#if showCountTooltip}
        <div class="tooltip-container">
          <CountTooltip counts={session.counts} />
        </div>
      {/if}
    </div>

    {#if session.model}
      <span class="model-tag" title="Model: {session.model}">{session.model}</span>
    {/if}

    {#if session.gitBranch}
      <span class="branch-tag" title="Git Branch: {session.gitBranch}"><Icon name="git-branch" size={11} /> {session.gitBranch}</span>
    {/if}
  </div>
</div>

<style>
  .session-row {
    /* Height plus margins must match the itemHeight passed to VirtualList.
       The side margins inset the row so the selection reads as a pill. */
    height: 80px;
    margin: 2px 6px 2px calc(6px + var(--indent, 0) * var(--indent-step));
    --indent-step: 16px;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: 8px 10px;
    border-radius: var(--radius-md);
    background-color: transparent;
    cursor: pointer;
    user-select: none;
    position: relative;
    transition: background-color var(--dur-fast) var(--ease-out);
    /* The selection takes the agent's color: Claude orange, Codex green,
       OpenCode blue. */
    --row-agent: var(--accent-color);
  }

  .session-row.agent-claude-code { --row-agent: var(--agent-claude); }
  .session-row.agent-codex { --row-agent: var(--agent-codex); }
  .session-row.agent-opencode { --row-agent: var(--agent-opencode); }

  .session-row:hover {
    background-color: var(--hover-bg);
  }

  .session-row.selected {
    background-color: color-mix(in srgb, var(--row-agent) 14%, transparent);
  }

  .session-row.bulk-selected:not(.selected) {
    background-color: color-mix(in srgb, var(--danger) 8%, transparent);
  }

  .session-row:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: -2px;
  }

  .session-row.archived {
    opacity: 0.75;
  }

  /* One guide line per level in the indent, running through the gaps
     between rows so a family reads as one column. */
  .session-row.nested::before {
    content: '';
    position: absolute;
    top: -2px;
    bottom: -2px;
    left: calc(-1 * var(--indent) * var(--indent-step));
    width: calc(var(--indent) * var(--indent-step));
    background: repeating-linear-gradient(
      to right,
      transparent 0 7px,
      var(--border-color) 7px 8px,
      transparent 8px var(--indent-step)
    );
    pointer-events: none;
  }

  /* A parent listed only for its matching subagents. */
  .session-row.context {
    opacity: 0.6;
  }

  .row-top {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.68rem;
  }

  .live-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background-color: var(--status-live);
    box-shadow: 0 0 6px var(--status-live);
    flex-shrink: 0;
    /* Liveness is presence, not work in progress. No idle pulse. */
  }

  /* One 16px slot for the agent's mark, or the checkbox over it. */
  .lead {
    position: relative;
    display: inline-grid;
    place-items: center;
    width: 16px;
    height: 16px;
    flex-shrink: 0;
  }

  .lead .bulk-check {
    position: absolute;
    inset: 0;
    opacity: 0;
    transition: opacity 0.1s ease;
  }

  .session-row:hover .lead.checkable .bulk-check,
  .session-row:focus-within .lead.checkable .bulk-check,
  .lead.checking .bulk-check {
    opacity: 1;
  }

  .session-row:hover .lead.checkable :global(.agent-icon),
  .session-row:focus-within .lead.checkable :global(.agent-icon),
  .lead.checking :global(.agent-icon) {
    visibility: hidden;
  }

  /* Tinted in the agent's color, like the badge the mark replaced. */
  .agent-name {
    margin-left: -2px;
    padding: 1px 4px;
    border-radius: 3px;
    font-weight: 600;
    white-space: nowrap;
    color: var(--row-agent);
    background-color: color-mix(in srgb, var(--row-agent) 15%, transparent);
  }

  /* A subagent's name in the gray of its parent's subagent count, so the
     colored names pick out the main sessions. */
  .agent-name.subagent {
    color: var(--text-secondary);
    background-color: var(--segment-track);
  }

  .subagent-toggle {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    height: 16px;
    padding: 0 5px 0 2px;
    border: none;
    border-radius: 4px;
    font-size: 0.62rem;
    font-weight: 500;
    white-space: nowrap;
    color: var(--text-secondary);
    background-color: var(--segment-track);
    cursor: pointer;
  }

  .subagent-toggle:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .subagent-toggle :global(.toggle-chevron) {
    transition: transform var(--dur-fast) var(--ease-out);
  }

  .subagent-toggle.open :global(.toggle-chevron) {
    transform: rotate(90deg);
  }

  .archived-tag {
    padding: 0 5px;
    border-radius: 4px;
    font-size: 0.62rem;
    font-weight: 500;
    color: var(--text-muted);
    background-color: var(--segment-track);
  }

  .time-label {
    margin-left: auto;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .row-middle {
    display: flex;
    align-items: center;
    overflow: hidden;
  }

  .title-text {
    font-size: 0.825rem;
    font-weight: 500;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    width: 100%;
  }

  .session-row.selected .title-text {
    font-weight: 600;
  }

  /* A subagent's session: led by a branch-off arrow, in a quieter color. */
  .title-text.subagent {
    color: var(--text-secondary);
  }

  .title-text :global(.subagent-icon) {
    margin-right: 3px;
    color: var(--text-muted);
  }

  .row-bottom {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.68rem;
    color: var(--text-secondary);
  }

  .count-wrapper {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .count-badge {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-variant-numeric: tabular-nums;
  }

  .count-badge :global(.count-icon) {
    opacity: 0.7;
  }

  .tooltip-container {
    position: absolute;
    bottom: calc(100% + 4px);
    left: 0;
  }

  .model-tag, .branch-tag {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 120px;
    font-family: var(--font-mono);
    opacity: 0.75;
  }
</style>
