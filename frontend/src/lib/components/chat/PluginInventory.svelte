<script lang="ts">
  // The answer to /plugin, above the composer: the plugins the agent's CLI
  // reports installed, from the chat's directory and host. It is the app's
  // listing, not an agent message: it is not sent, saved or kept, and has no
  // install or enable actions. Closing it, or the chat, drops it.
  import { onMount } from 'svelte';
  import type { ChatCommand, ChatPlugins } from '../../types';
  import { api } from '../../api';
  import { pluginRows } from '../../chat';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import Icon from '../common/Icon.svelte';

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

<section class="plugins" data-chat-plugins aria-label="{agentName}'s installed plugins">
  <header>
    <span class="title">/plugin</span>
    <span class="note">Installed for {agentName}, read from its CLI. Not sent to the agent.</span>
    <button type="button" class="icon" title="Close the plugin list" onclick={onClose}><Icon name="x" size={13} /><span class="sr-only">Close</span></button>
  </header>
  <div class="body" aria-live="polite">
    {#if failure}
      <p class="status error" data-plugins-state="error">{failure}</p>
    {:else if !result}
      <p class="status" data-plugins-state="loading">Listing {agentName}'s plugins… This can take a minute.</p>
    {:else if !result.supported}
      <p class="status" data-plugins-state="unsupported">{agentName} cannot list its plugins in a form the app reads.</p>
    {:else if rows.length === 0}
      <p class="status" data-plugins-state="empty">No plugins installed.</p>
    {:else}
      <ul data-plugins-state="listed">
        {#each rows as p (p.id)}
          <li>
            <span class="name">{p.name}</span>
            {#if p.marketplace}<span class="meta">@{p.marketplace}</span>{/if}
            {#if p.version}<span class="meta">{p.version}</span>{/if}
            {#if p.scope}<span class="meta">{p.scope}</span>{/if}
            <span class="spacer"></span>
            <span class="badge" class:off={!p.enabled}>{p.enabled ? 'enabled' : 'disabled'}</span>
            {#if p.loaded}<span class="badge" title="The session has this plugin's commands">loaded</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>

<style>
  .plugins {
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto 6px;
    padding: 6px 6px 8px 12px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-secondary);
    font-size: 0.8rem;
    color: var(--text-muted);
    box-sizing: border-box;
  }

  header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .title,
  .name {
    font-family: var(--font-mono);
    color: var(--text-primary);
  }

  .note {
    flex: 1;
    min-width: 0;
    font-size: 0.72rem;
  }

  .body {
    max-height: min(260px, 30vh);
    overflow-y: auto;
  }

  .status {
    margin: 6px 0 0;
  }

  .status.error {
    color: var(--danger);
  }

  ul {
    margin: 4px 0 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding: 3px 4px 3px 0;
  }

  .spacer {
    flex: 1;
  }

  .badge {
    padding: 0 6px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    font-size: 0.7rem;
    color: var(--text-secondary);
  }

  .badge.off {
    color: var(--text-muted);
  }

  button {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  button:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  button:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
    border: 0;
  }
</style>
