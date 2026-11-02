<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  import { agentSettings } from '../../stores/agentSettings.svelte';

  $effect(() => {
    const agent = appState.filter.agent;
    if (agent && !agentSettings.enabledAgents.some(a => a.id === agent)) {
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
    {#each agentSettings.enabledAgents as agent (agent.id)}
      <button
        type="button"
        class="chip agent-{agent.id}"
        class:active={appState.filter.agent === agent.id}
        title="Filter {agent.label} sessions ({appState.agentCounts[agent.id] ?? 0})"
        onclick={() => toggleAgentFilter(agent.id)}
      >
        <span class="chip-name">{agent.id === 'claude-code' ? 'Claude' : agent.label}</span>
        <span class="chip-count">{appState.agentCounts[agent.id] ?? 0}</span>
      </button>
    {/each}
  </div>
</div>

<style>
  .agent-summary {
    width: 100%;
    padding: 6px 8px;
    border-top: 1px solid var(--border-color);
    background-color: var(--bg-secondary);
  }

  .summary-chips {
    display: flex;
    gap: 4px;
    justify-content: space-between;
  }

  .chip {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
    padding: 3px 6px;
    border-radius: 4px;
    font-size: 0.72rem;
    font-weight: 500;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    color: var(--text-secondary);
    transition: all 0.15s ease;
  }

  .chip:hover {
    color: var(--text-primary);
    border-color: var(--text-muted);
  }

  .chip.active {
    border-color: var(--accent-color);
    background-color: var(--active-bg);
  }

  .chip.agent-claude-code.active {
    color: var(--agent-claude);
    border-color: var(--agent-claude);
  }

  .chip.agent-codex.active {
    color: var(--agent-codex);
    border-color: var(--agent-codex);
  }

  .chip.agent-opencode.active {
    color: var(--agent-opencode);
    border-color: var(--agent-opencode);
  }

  .chip-name {
    font-weight: 600;
  }

  .chip-count {
    font-variant-numeric: tabular-nums;
    font-size: 0.7rem;
    opacity: 0.8;
  }
</style>
