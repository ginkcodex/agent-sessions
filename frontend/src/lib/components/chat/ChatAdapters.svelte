<script lang="ts">
  // Settings › Coding agents: which agents can chat on the selected host,
  // and how to install the adapters that are missing.
  import { chats } from '../../stores/chats.svelte';
  import { chatOpen } from '../../stores/chatOpen.svelte';
  import { link } from '../../stores/link.svelte';
  import { hostLabel } from '../../hosts';
  import AdapterMissing from './AdapterMissing.svelte';

  let host = $derived(link.dataHost ? hostLabel(link.dataHost) : 'this computer');

  $effect(() => {
    void link.dataHost;
    chatOpen.refresh();
  });
</script>

{#if chatOpen.available}
  <section class="chat-adapters" data-chat-adapters>
    <h3>Chat</h3>
    <p class="hint">A chat drives an agent through its ACP adapter and shows its reply as it streams. Adapters are found on {host} like the agents.</p>
    {#if chats.agentsError}
      <p class="error" role="alert">{chats.agentsError}</p>
    {:else if chats.agents.length === 0}
      <p class="hint" role="status">{chats.agentsLoading ? 'Looking for adapters…' : 'No agent can chat here.'}</p>
    {/if}
    <ul>
      {#each chats.agents as agent (agent.agent)}
        <li>
          {#if agent.available}
            <span><strong>{agent.name}</strong>: ready{#if agent.path}, <code>{agent.path}</code>{/if}</span>
          {:else}
            <AdapterMissing {agent} {host} />
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .chat-adapters { margin-top: 24px; }
  h3 { margin: 0 0 10px; font-size: 14px; }
  .hint { color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
  ul { list-style: none; margin: 12px 0 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
  li { font-size: 12px; line-height: 1.5; }
  code { font-family: var(--font-mono); font-size: 11.5px; overflow-wrap: anywhere; }
  .error { color: var(--error-color, #cf222e); font-size: 12px; }
</style>
