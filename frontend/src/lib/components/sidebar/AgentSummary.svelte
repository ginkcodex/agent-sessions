<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  import { agentSettings } from '../../stores/agentSettings.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';

  $effect(() => {
    const agent = appState.filter.agent;
    if (agent && !agentSettings.activeAgents.some(a => a.id === agent)) {
      void appState.setFilter({ agent: undefined });
    }
  });

  function toggleAgentFilter(agent: string) {
    if (appState.filter.agent === agent) {
      appState.setFilter({ agent: undefined });
    } else {
      appState.setFilter({ agent });
    }
  }
</script>

<div class="agent-summary" aria-label="Session summary by agent">
  <div class="summary-chips">
    {#each agentSettings.activeAgents as agent (agent.id)}
      <button
        type="button"
        class="chip agent-{agent.id}"
        class:active={appState.filter.agent === agent.id}
        title="Filter {agent.label} sessions ({appState.agentCounts[agent.id] ?? 0})"
        onclick={() => toggleAgentFilter(agent.id)}
      >
        <AgentIcon agent={agent.id} size={11} />
        <span class="chip-name">{agent.id === 'claude-code' ? 'Claude' : agent.label}</span>
        <span class="chip-count">{appState.agentCounts[agent.id] ?? 0}</span>
      </button>
    {/each}
  </div>
</div>

<style>
  .agent-summary {
    container-type: inline-size;
    width: 100%;
    padding: 6px 8px;
    border-top: 1px solid var(--border-color);
  }

  .summary-chips {
    display: flex;
    gap: 4px;
    justify-content: space-between;
  }

  /* Filled, borderless chips; the chosen agent fills with its own color. */
  .chip {
    --chip-color: var(--accent-color);
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 4px 7px;
    border-radius: var(--radius-sm);
    font-size: 0.72rem;
    font-weight: 500;
    background-color: var(--segment-track);
    color: var(--text-secondary);
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .chip.agent-claude-code { --chip-color: var(--agent-claude); }
  .chip.agent-codex { --chip-color: var(--agent-codex); }
  .chip.agent-opencode { --chip-color: var(--agent-opencode); }

  .chip:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .chip.active,
  .chip.active:hover {
    background-color: color-mix(in srgb, var(--chip-color) 16%, transparent);
    color: var(--chip-color);
  }

  .chip-name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chip-count {
    margin-left: auto;
    font-variant-numeric: tabular-nums;
    font-size: 0.7rem;
    opacity: 0.75;
  }

  /* A narrow sidebar keeps each agent's mark and count, not a clipped name. */
  @container (max-width: 300px) {
    .chip-name {
      display: none;
    }
  }
</style>
