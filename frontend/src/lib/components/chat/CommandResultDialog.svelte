<script lang="ts">
  // The modal an inventory command (/mcp, /plugin) answers in, as VS Code's
  // "MCP servers" list: a title, a filter, and the body the filter narrows.
  // Read-only: Close and Escape are its only actions.
  import type { Snippet } from 'svelte';
  import { modalFocus } from '../../modalFocus';
  import DialogCloseButton from '../common/DialogCloseButton.svelte';

  interface Props {
    title: string;
    /** What the list shows, e.g. whose servers and where from. */
    note: string;
    filterPlaceholder: string;
    onClose: () => void;
    /** The body, given the filter's text, lowercased and trimmed. */
    children: Snippet<[string]>;
  }

  let { title, note, filterPlaceholder, onClose, children }: Props = $props();
  let filter = $state('');
  let filterInput: HTMLInputElement | undefined = $state();
  // The composer still has focus when the dialog opens, so autofocus would not take it.
  $effect(() => filterInput?.focus());
  const titleId = `command-result-${Math.random().toString(36).slice(2)}`;
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="backdrop"
  role="presentation"
  onkeydown={(event) => {
    event.stopPropagation();
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    }
  }}
>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby={titleId} data-command-result use:modalFocus>
    <header>
      <h2 id={titleId}>{title}</h2>
      <DialogCloseButton label="Close {title}" onclick={onClose} />
    </header>
    <p class="note">{note}</p>
    <input class="text-field" type="search" bind:this={filterInput} bind:value={filter} placeholder={filterPlaceholder} aria-label={filterPlaceholder} autocomplete="off" spellcheck="false" />
    <div class="body" aria-live="polite">{@render children(filter.trim().toLowerCase())}</div>
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; z-index: 1100; display: grid; place-items: start center; padding: 12vh 20px 20px; background: var(--scrim); animation: -global-scrim-in var(--dur-med) var(--ease-out); }
  .dialog { display: flex; flex-direction: column; width: min(620px, 100%); max-height: 72vh; padding: 16px 18px 14px; border: 1px solid var(--dialog-border); border-radius: var(--radius-dialog); background: var(--bg-primary); color: var(--text-primary); box-shadow: var(--shadow-dialog); box-sizing: border-box; animation: -global-dialog-in var(--dur-med) var(--ease-out); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: -4px -8px 0 0; }
  h2 { margin: 0; font-size: 15px; font-weight: 600; }
  .note { margin: 4px 0 12px; font-size: 12px; color: var(--text-muted); }
  input { box-sizing: border-box; width: 100%; padding: 7px 10px; font-size: 13px; }
  .body { min-height: 0; margin-top: 10px; overflow-y: auto; font-size: 13px; }
</style>
