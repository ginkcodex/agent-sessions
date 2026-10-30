<script lang="ts">
  import type { SessionFind } from '../../sessionFind.svelte';

  interface Props {
    find: SessionFind;
  }

  let { find }: Props = $props();

  let input: HTMLInputElement | undefined = $state();

  $effect(() => {
    void find.focusRequest;
    input?.focus();
    input?.select();
  });

  let note = $derived(
    find.unsupported
      ? "This host's agent-sessions can't search a session; update it."
      : find.error,
  );
</script>

<div class="find-bar" role="search" data-session-find>
  <div class="find-row">
    <div class="find-field text-field">
      <svg class="find-glass" viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
        <circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5" />
        <path d="M10.5 10.5 14 14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
      </svg>
      <input
        bind:this={input}
        class="find-input"
        type="text"
        placeholder="Find in session"
        aria-label="Find in the session"
        value={find.query}
        oninput={(e) => find.setQuery(e.currentTarget.value)}
        onkeydown={(e) => find.handleKey(e)}
        autocomplete="off"
        autocorrect="off"
        autocapitalize="off"
        spellcheck="false"
      />
    </div>
    <span class="find-count" class:none={find.count === 'No results'} aria-live="polite">{find.count}</span>
    <button type="button" class="find-btn" aria-label="Previous match" title="Previous match (Shift+Enter)" disabled={find.hits.length === 0} onclick={() => find.step(-1)}>
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <path d="M8 13V3M3.5 7.5 8 3l4.5 4.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>
    <button type="button" class="find-btn" aria-label="Next match" title="Next match (Enter)" disabled={find.hits.length === 0} onclick={() => find.step(1)}>
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <path d="M8 3v10M3.5 8.5 8 13l4.5-4.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>
    <button type="button" class="find-btn" aria-label="Close find" title="Close (Escape)" onclick={() => find.close()}>
      <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
        <path d="M4 4l8 8M12 4l-8 8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
      </svg>
    </button>
  </div>
  {#if note}
    <p class="find-note" role="status">{note}</p>
  {/if}
</div>

<style>
  /* Over the transcript's top right corner, as the file viewer's find. */
  .find-bar {
    position: absolute;
    top: 6px;
    right: 18px;
    z-index: 5;
    max-width: calc(100% - 36px);
    padding: 5px;
    font-family: var(--font-sans);
    font-size: 0.75rem;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12), 0 1px 3px rgba(15, 23, 42, 0.08);
  }

  .find-row {
    display: flex;
    align-items: center;
    gap: 2px;
  }

  .find-field {
    flex: 1;
  }

  .find-glass {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .find-input {
    width: 200px;
  }

  .find-count {
    /* Room for "999 of 999", so the input stays put while stepping. */
    min-width: 76px;
    padding: 0 4px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    text-align: center;
    white-space: nowrap;
  }

  .find-count.none {
    color: var(--danger);
  }

  .find-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    min-width: 24px;
    height: 24px;
    padding: 0 4px;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .find-btn:hover:not(:disabled) {
    background-color: var(--bg-tertiary);
    color: var(--text-primary);
  }

  .find-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  .find-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .find-note {
    max-width: 340px;
    margin: 5px 4px 1px;
    color: var(--text-secondary);
    line-height: 1.4;
  }
</style>
