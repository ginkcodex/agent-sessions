<script lang="ts">
  import { tick } from 'svelte';
  import type { GroupNode, SessionMeta } from '../../types';
  import Icon from '../common/Icon.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';
  import { api } from '../../api';
  import { appState } from '../../stores/appState.svelte';
  import { manage } from '../../stores/manage.svelte';
  import { workspaceStars } from '../../stores/workspaceStars.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { link } from '../../stores/link.svelte';
  import { errorText, filterSessionsByAge, refsEqual } from '../../manage';
  import { isStaleReply } from '../../link';
  import { directoryName, switcherWorkspaces } from '../../tree';
  import { formatAgo } from '../../date';

  let { disabled = false }: { disabled?: boolean } = $props();

  type Loaded = { sessions?: SessionMeta[]; error?: string };

  let open = $state(false);
  // Bumped on every open and close, so replies for an older panel are dropped.
  let generation = 0;
  let top = $state(0);
  let workspaces = $state<GroupNode[] | null>(null);
  let workspacesError = $state('');
  let expanded = $state(new Set<string>());
  let loaded = $state<Record<string, Loaded>>({});
  let button = $state<HTMLButtonElement>();
  let panel = $state<HTMLDivElement>();

  function holdsOpenSession(node: GroupNode): boolean {
    const cwd = appState.selectedSessionMeta?.cwd;
    return Boolean(cwd && node.cwd && (cwd === node.cwd || cwd.startsWith(`${node.cwd}/`)));
  }

  async function show() {
    const gen = ++generation;
    top = button?.getBoundingClientRect().top ?? 0;
    workspaces = null;
    workspacesError = '';
    expanded = new Set();
    loaded = {};
    open = true;
    // Focus inside so Tab reaches the rows and Escape is heard.
    void tick().then(() => panel?.focus());
    try {
      const groups = await api.listGroups('dir-agent', appState.filter);
      if (gen !== generation) return;
      workspaces = switcherWorkspaces(workspaceStars.visibleGroups(groups));
      const current = workspaces.find(holdsOpenSession);
      if (current) toggle(current);
    } catch (err) {
      if (gen === generation && !isStaleReply(err)) workspacesError = errorText(err, 'Could not list workspaces.');
    }
  }

  async function close(refocus = true) {
    generation++;
    open = false;
    if (refocus) {
      await tick();
      button?.focus();
    }
  }

  async function toggle(node: GroupNode) {
    const next = new Set(expanded);
    if (next.delete(node.key)) {
      expanded = next;
      return;
    }
    next.add(node.key);
    expanded = next;
    if (loaded[node.key]?.sessions) return;
    const gen = generation;
    loaded[node.key] = {};
    try {
      const sessions = await api.listSessions(node.key, { ...appState.filter, hideSubagents: true }, appState.sort);
      if (gen === generation) loaded[node.key] = { sessions: filterSessionsByAge(sessions, manage.ageFilter) };
    } catch (err) {
      if (gen === generation && !isStaleReply(err)) loaded[node.key] = { error: errorText(err, 'Could not list sessions.') };
    }
  }

  // Same as a click in the session list.
  function pick(session: SessionMeta) {
    void close();
    if (!session.cwdMissing) terminals.selectDirectory(session.cwd, link.dataHost ?? '', session);
    void appState.selectSession(session.ref, 'push');
  }

  $effect(() => {
    if (disabled && open) void close(false);
  });

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as Node;
    if (open && !panel?.contains(target) && !button?.contains(target)) void close(false);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) {
      e.stopPropagation();
      void close();
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<button
  type="button"
  class="switch-button"
  title="Switch session"
  aria-label="Switch session"
  aria-haspopup="dialog"
  aria-expanded={open}
  {disabled}
  bind:this={button}
  onclick={() => (open ? close() : show())}
>
  <Icon name="folder" size={16} />
</button>

{#if open}
  <div
    class="switcher"
    role="dialog"
    tabindex="-1"
    aria-label="Switch session"
    style:top="{top}px"
    style:max-height="calc(100vh - {top}px - 8px)"
    bind:this={panel}
    onkeydown={handleKeydown}
  >
    {#if workspacesError}
      <p class="note error">{workspacesError}</p>
    {:else if !workspaces}
      <p class="note">Loading…</p>
    {:else if workspaces.length === 0}
      <p class="note">No sessions match the sidebar's filters.</p>
    {:else}
      <ul>
        {#each workspaces as node (node.key)}
          {@const isOpen = expanded.has(node.key)}
          {@const state = loaded[node.key]}
          <li>
            <button
              type="button"
              class="row workspace"
              aria-expanded={isOpen}
              title={node.secondary || node.label}
              onclick={() => toggle(node)}
            >
              <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} size={12} />
              <span class="label">{directoryName(node.label)}</span>
              <span class="meta">{node.sessionCount}</span>
            </button>
            {#if isOpen}
              {#if state?.error}
                <p class="note error nested">{state.error}</p>
              {:else if !state?.sessions}
                <p class="note nested">Loading…</p>
              {:else if state.sessions.length === 0}
                <p class="note nested">No sessions.</p>
              {:else}
                <ul>
                  {#each state.sessions as session (`${session.ref.agent}:${session.ref.id}`)}
                    {@const title = session.title || session.firstPrompt || '(untitled)'}
                    <li>
                      <button
                        type="button"
                        class="row session"
                        aria-current={refsEqual(session.ref, appState.selectedSessionRef) ? 'true' : undefined}
                        title={title}
                        onclick={() => pick(session)}
                      >
                        <AgentIcon agent={session.ref.agent} size={12} />
                        <span class="label">{title}</span>
                        <span class="meta">{formatAgo(session.updatedAt)}</span>
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  /* App's .icon-button, which is scoped to App.svelte. */
  .switch-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border-radius: 4px;
    color: var(--text-secondary);
  }

  .switch-button:hover:not(:disabled),
  .switch-button[aria-expanded='true'] {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .switcher {
    position: fixed;
    left: 44px;
    width: 320px;
    /* The panel starts 44px from the left; keep an 8px margin on the right. */
    max-width: calc(100vw - 52px);
    overflow-y: auto;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    padding: 6px 0;
    z-index: 105;
  }

  .switcher:focus {
    outline: none;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 6px 10px;
    border: none;
    background: none;
    color: var(--text-primary);
    font-size: 0.78rem;
    text-align: left;
    cursor: pointer;
  }

  .row:hover,
  .row:focus-visible {
    background: var(--hover-bg);
  }

  .row.session {
    padding-left: 28px;
  }

  .row[aria-current='true'] {
    background: color-mix(in srgb, var(--accent-color) 14%, transparent);
    font-weight: 600;
  }

  .label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    flex-shrink: 0;
    color: var(--text-secondary);
    font-size: 0.72rem;
  }

  .note {
    margin: 0;
    padding: 6px 10px;
    color: var(--text-secondary);
    font-size: 0.78rem;
  }

  .note.nested {
    padding-left: 28px;
  }

  .note.error {
    color: var(--danger);
  }
</style>
