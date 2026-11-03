<script lang="ts">
  import type { GroupMode } from '../../types';
  import { appState } from '../../stores/appState.svelte';

  const modes: { id: GroupMode; label: string; title: string }[] = [
    { id: 'dir-agent', label: 'Dir', title: 'Group by Directory then Agent' },
    { id: 'agent-dir', label: 'Agent', title: 'Group by Agent then Directory' },
    { id: 'flat', label: 'Flat', title: 'Flat session list' },
  ];
</script>

<div class="mode-selector" role="toolbar" aria-label="Grouping mode selector">
  <div class="segmented-control">
    {#each modes as mode}
      <button
        type="button"
        class="segment-button"
        class:active={appState.groupMode === mode.id}
        title={mode.title}
        aria-pressed={appState.groupMode === mode.id}
        onclick={() => appState.setGroupMode(mode.id)}
      >
        {mode.label}
      </button>
    {/each}
  </div>
</div>

<style>
  .mode-selector {
    display: flex;
    width: 100%;
  }

  .segmented-control {
    display: flex;
    width: 100%;
    gap: 2px;
    background-color: var(--segment-track);
    padding: 2px;
    border-radius: var(--radius-md);
  }

  .segment-button {
    flex: 1;
    padding: 3px 6px;
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--text-secondary);
    border-radius: calc(var(--radius-md) - 2px);
    text-align: center;
    transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
  }

  .segment-button:hover {
    color: var(--text-primary);
  }

  .segment-button.active {
    background-color: var(--segment-thumb);
    color: var(--text-primary);
    font-weight: 600;
    box-shadow: var(--shadow-thumb);
  }
</style>
