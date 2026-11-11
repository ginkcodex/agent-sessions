<script lang="ts">
  // An agent's ACP adapter was not found: why, and the command that installs
  // it, to copy and run on the host the chat would run on. The app never
  // installs it itself.
  import type { ChatAgent } from '../../types';
  import { copyToClipboard } from '../../portable';
  import { toast } from '../../stores/toast.svelte';

  interface Props {
    agent: ChatAgent;
    /** Where the adapter is missing: "this computer" or the host's name. */
    host: string;
  }

  let { agent, host }: Props = $props();

  let copied = $state(false);

  async function copy(): Promise<void> {
    if (await copyToClipboard(agent.install)) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } else {
      toast.show({ title: 'Could not copy the command', body: 'Select it and copy it by hand.', tone: 'error' });
    }
  }
</script>

<div class="adapter-missing" data-adapter-missing={agent.agent}>
  <p class="what">
    <strong>{agent.name}</strong> chats need its ACP adapter on {host}.
    {#if agent.problem}<span class="problem">{agent.problem}</span>{/if}
  </p>
  {#if agent.install}
    <p class="how">To install it, run this on {host}:</p>
    <div class="command">
      <code>{agent.install}</code>
      <button type="button" onclick={() => void copy()}>{copied ? '✓ Copied' : 'Copy'}</button>
    </div>
  {/if}
  {#if agent.override}
    <p class="how">The path comes from <code>[chat.adapters]</code> in config.toml: fix or remove it there.</p>
  {/if}
</div>

<style>
  .adapter-missing {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
    line-height: 1.5;
  }

  p {
    margin: 0;
  }

  .problem {
    display: block;
    color: var(--text-secondary);
  }

  .how {
    color: var(--text-secondary);
  }

  .command {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .command code {
    flex: 1;
    min-width: 0;
    padding: 4px 8px;
    border-radius: var(--radius-sm, 4px);
    background: var(--bg-secondary);
    font-family: var(--font-mono);
    font-size: 11.5px;
    overflow-wrap: anywhere;
    user-select: all;
  }

  button {
    padding: 4px 10px;
    border-radius: var(--radius-sm, 4px);
    background: var(--btn-bg);
    color: var(--text-primary);
    font-size: 12px;
    cursor: pointer;
    flex-shrink: 0;
  }

  button:hover {
    background: var(--btn-bg-hover);
  }
</style>
