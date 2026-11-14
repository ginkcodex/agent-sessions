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
            <span class="name">{p.name}</span>
            {#if p.marketplace}<span class="meta">@{p.marketplace}</span>{/if}
            {#if p.version}<span class="meta">{p.version}</span>{/if}
            {#if p.scope}<span class="meta">{p.scope}</span>{/if}
            <span class="spacer"></span>
            <span class="badge" class:on={p.enabled}>{p.enabled ? 'enabled' : 'disabled'}</span>
            {#if p.loaded}<span class="badge" title="The session has this plugin's commands">loaded</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</CommandResultDialog>

<style>
  .status { margin: 4px 0; color: var(--text-secondary); }
  .status.error { color: var(--danger); }
  ul { margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: baseline; gap: 8px; padding: 5px 6px; border-radius: var(--radius-md); }
  li:hover { background: var(--hover-bg); }
  .name { font-family: var(--font-mono); color: var(--text-primary); }
  .meta { font-size: 12px; color: var(--text-muted); }
  .spacer { flex: 1; }
  .badge { flex: none; padding: 0 6px; border: 1px solid var(--border-color); border-radius: 6px; font-size: 11px; color: var(--text-muted); white-space: nowrap; }
  .badge.on { color: var(--status-live); border-color: currentColor; }
</style>
