<script lang="ts">
  // Settings › Coding agents › Chat: which agents can chat on the selected
  // host, with Install or Update for the app's own adapter copies.
  import { chats } from '../../stores/chats.svelte';
  import { chatOpen } from '../../stores/chatOpen.svelte';
  import { link } from '../../stores/link.svelte';
  import { hostLabel } from '../../hosts';
  import ChatAdapterRow from './ChatAdapterRow.svelte';

  let host = $derived(link.dataHost ? hostLabel(link.dataHost) : 'this computer');

  $effect(() => {
    void link.dataHost;
    chatOpen.refresh();
  });
</script>

{#if chatOpen.available}
  <section class="chat-adapters" data-chat-adapters>
    <h3>Chat</h3>
    <p class="hint">Chats run each agent through its ACP adapter on {host}.</p>
    {#if chats.agentsError}
      <p class="error" role="alert">{chats.agentsError}</p>
    {:else if chats.agents.length === 0}
      <p class="hint" role="status">{chats.agentsLoading ? 'Looking for adapters…' : 'No agent can chat here.'}</p>
    {/if}
    <ul>
      {#each chats.agents as agent (agent.agent)}
        <li><ChatAdapterRow {agent} {host} /></li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .chat-adapters { margin-top: 24px; }
  h3 { margin: 0 0 6px; font-size: 14px; }
  .hint { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
  ul { list-style: none; margin: 10px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .error { color: var(--error-color, #cf222e); font-size: 12px; }
</style>
