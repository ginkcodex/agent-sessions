<script lang="ts">
  // One agent in Settings › Coding agents › Chat: whether its ACP adapter is
  // ready, and Install or Update where the app can install its own pinned
  // copy on the host. One click installs; nothing more is asked.
  import type { ChatAgent } from '../../types';
  import { ADAPTER_TOOLS } from '../../managedTools';
  import { connectionStore } from '../../stores/connection.svelte';
  import { adapterInstall as installer } from '../../stores/adapterInstall.svelte';

  interface Props {
    agent: ChatAgent;
    /** Where the adapter runs: "this computer" or the host's name. */
    host: string;
  }

  let { agent, host }: Props = $props();

  let tool = $derived(ADAPTER_TOOLS.find((t) => t === agent.installTool));
  // Managed installs come with code checks' setup; an older server sends no
  // installTool. An adapter set in config.toml runs before an app copy.
  let installable = $derived(!!tool && !agent.override && connectionStore.capabilities.codeDiagnostics === true);
  let flow = $derived(tool && installer.tool === tool ? installer : undefined);
  let running = $derived(flow?.state?.state === 'installing');
  let working = $derived(!!flow && (flow.pending || running));
  let action = $derived(!installable || working ? undefined : !agent.available ? 'Install' : agent.outdated ? 'Update' : undefined);

  let status = $derived.by(() => {
    if (working) return agent.available ? 'Updating…' : 'Installing…';
    if (!agent.available) return installable ? 'Not installed' : agent.problem || 'Not found';
    if (agent.override) return 'Ready · set in config.toml';
    if (agent.outdated && agent.version) return `${agent.version} · ${agent.pinned} available`;
    return agent.version ? `Ready · ${agent.version}` : 'Ready';
  });
</script>

<div class="row" data-chat-adapter={agent.agent}>
  <strong>{agent.name}</strong>
  <span class="status" class:ready={agent.available && !agent.outdated && !working} title={agent.path || undefined}>{status}</span>
  {#if running}
    <button type="button" disabled={installer.pending} onclick={() => void installer.cancel()}>Cancel</button>
  {:else if action && tool}
    <button
      type="button"
      class="primary"
      title="Installs {tool}{agent.pinned ? ` ${agent.pinned}` : ''} for this app on {host}"
      disabled={installer.busy}
      onclick={() => void installer.run(tool!)}>{action}</button
    >
  {/if}
</div>
{#if flow?.error}<p class="error" role="alert">{flow.error}</p>{/if}
{#if agent.override && !agent.available}
  <p class="how">Fix or remove it in <code>[chat.adapters]</code> in config.toml.</p>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 26px;
    font-size: 12px;
  }

  strong {
    flex: none;
    min-width: 96px;
    font-weight: 600;
  }

  .status {
    flex: 1;
    min-width: 0;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .status.ready {
    color: var(--text-primary);
  }

  p {
    margin: 2px 0 0;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }

  .how {
    color: var(--text-secondary);
  }

  .error {
    color: var(--error-color, #cf222e);
  }

  code {
    font-family: var(--font-mono);
    font-size: 11.5px;
  }

  button {
    flex: none;
    padding: 3px 12px;
    border-radius: var(--radius-sm, 4px);
    background: var(--btn-bg);
    color: var(--text-primary);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
  }

  button:hover:not(:disabled) {
    background: var(--btn-bg-hover);
  }

  button:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  button.primary {
    background: var(--accent-color);
    color: white;
  }

  button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
