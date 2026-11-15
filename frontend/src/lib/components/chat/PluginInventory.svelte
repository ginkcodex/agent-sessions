<script lang="ts">
  // The answer to /plugin, in the command result modal: the plugins the
  // agent's CLI reports installed, from the chat's directory and host. It is
  // the app's listing, not an agent message: it is not sent, saved or kept,
  // and has no install or enable actions. Closing it, or the chat, drops it.
  import { onMount } from 'svelte';
  import type { ChatCommand, ChatPlugins } from '../../types';
  import { api } from '../../api';
  import { pluginRows } from '../../chat';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import CommandResultDialog from './CommandResultDialog.svelte';

  interface Props {
    chatId: string;
    agentName: string;
    /** The chat's command catalog, which says what the session loaded. */
    commands: readonly ChatCommand[];
    onClose: () => void;
  }

  let { chatId, agentName, commands, onClose }: Props = $props();

  let result = $state<ChatPlugins | null>(null);
  let failure = $state('');
  let rows = $derived(result ? pluginRows(result.plugins, commands) : []);

  onMount(() => {
    let live = true;
    api.chatPlugins(chatId).then(
      (r) => {
        if (live) result = r;
      },
      (err) => {
        if (live && !isStaleReply(err)) failure = errorText(err, `Could not list ${agentName}'s plugins.`);
      },
    );
    return () => {
      live = false;
    };
  });
</script>

<CommandResultDialog title="Plugins" note="Installed for {agentName}, read from its CLI. Not sent to the agent." filterPlaceholder="Filter plugins…" {onClose}>
  {#snippet children(query: string)}
    {@const shown = rows.filter((p) => p.name.toLowerCase().includes(query) || (p.marketplace ?? '').toLowerCase().includes(query))}
    {#if failure}
      <p class="status error" data-plugins-state="error">{failure}</p>
    {:else if !result}
      <p class="status" data-plugins-state="loading">Listing {agentName}'s plugins… This can take a minute.</p>
    {:else if !result.supported}
      <p class="status" data-plugins-state="unsupported">{agentName} cannot list its plugins in a form the app reads.</p>
    {:else if rows.length === 0}
      <p class="status" data-plugins-state="empty">No plugins installed.</p>
    {:else if shown.length === 0}
      <p class="status" data-plugins-state="no-match">No plugins match.</p>
    {:else}
      <ul data-plugins-state="listed">
        {#each shown as p (p.id)}
          <li>
            <div class="identity">
              <span class="name">{p.name}</span>
              <div class="metadata">
                {#if p.marketplace}<span class="meta">@{p.marketplace}</span>{/if}
                {#if p.version}<span class="meta">{p.version}</span>{/if}
                {#if p.scope}<span class="meta">{p.scope}</span>{/if}
              </div>
            </div>
            <div class="badges">
              <span class="badge" class:on={p.enabled}>{p.enabled ? 'enabled' : 'disabled'}</span>
              {#if p.loaded}<span class="badge loaded" title="The session has this plugin's commands">loaded</span>{/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</CommandResultDialog>

<style>
  .status {
    margin: 0;
    padding: 18px 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }
  .status.error { color: var(--danger); }

  ul { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px 16px;
    padding: 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--bg-secondary) 55%, var(--bg-primary));
  }

  .identity { flex: 1 1 240px; min-width: 0; }
  .name { font-family: var(--font-mono); font-weight: 600; color: var(--text-primary); }
  .metadata { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 6px; }
  .meta { min-width: 0; font-size: 12px; color: var(--text-muted); }
  .badges { display: flex; flex-wrap: wrap; gap: 6px; }

  .badge {
    padding: 3px 9px;
    border: 1px solid var(--border-color);
    border-radius: 999px;
    background: var(--bg-secondary);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .badge.on {
    color: var(--status-live);
    border-color: color-mix(in srgb, var(--status-live) 25%, transparent);
    background: color-mix(in srgb, var(--status-live) 10%, var(--bg-primary));
  }

  .badge.loaded {
    color: var(--accent-hover);
    border-color: color-mix(in srgb, var(--accent-color) 25%, transparent);
    background: color-mix(in srgb, var(--accent-color) 10%, var(--bg-primary));
  }
</style>
