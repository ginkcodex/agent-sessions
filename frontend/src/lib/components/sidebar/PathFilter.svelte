<script lang="ts">
  import { onDestroy } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { workspaceStars } from '../../stores/workspaceStars.svelte';
  import Icon from '../common/Icon.svelte';

  let timer: ReturnType<typeof setTimeout> | undefined;

  // Debounce so fast typing doesn't trigger a backend query per keystroke.
  function handleInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    clearTimeout(timer);
    timer = setTimeout(() => {
      appState.setFilter({ path: val });
    }, 250);
  }

  onDestroy(() => {
    clearTimeout(timer);
  });
</script>

<div class="workspace-filters">
  <div class="path-filter text-field">
    <svg
      class="path-icon"
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
    </svg>
    <input
      type="search"
      placeholder="Filter by directory…"
      value={appState.filter.path || ''}
      oninput={handleInput}
      aria-label="Filter by directory"
      autocomplete="off"
      autocorrect="off"
      autocapitalize="off"
      spellcheck="false"
    />
  </div>
  <button
    type="button"
    class="star-filter"
    class:active={workspaceStars.onlyStarred}
    aria-label="Show only starred workspaces"
    aria-pressed={workspaceStars.onlyStarred}
    title={workspaceStars.onlyStarred ? 'Show all workspaces' : 'Show only starred workspaces'}
    onclick={() => { workspaceStars.onlyStarred = !workspaceStars.onlyStarred; }}
  >
    <Icon name="star" size={15} />
  </button>
</div>

<style>
  .workspace-filters {
    display: flex;
    align-items: stretch;
    gap: 6px;
    min-width: 0;
  }

  /* A tone darker than the sidebar it sits on. */
  .path-filter {
    flex: 1;
    min-width: 0;
    background-color: var(--bg-tertiary);
  }

  .star-filter {
    display: grid;
    place-items: center;
    width: 30px;
    flex-shrink: 0;
    border-radius: 5px;
    background-color: var(--bg-tertiary);
    color: var(--text-muted);
    cursor: pointer;
  }

  .star-filter:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .star-filter.active {
    color: var(--accent-color);
    background-color: var(--active-bg);
  }

  .star-filter.active :global(svg) {
    fill: currentColor;
  }

  .star-filter:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }


  .path-icon {
    flex-shrink: 0;
    color: var(--text-muted);
  }
</style>
