<script lang="ts">
  // The files attached to the message being written: a thumbnail for an
  // image, an icon for a text file, the name, and a button to remove it.
  import type { ChatAttachment } from '../../types';
  import { formatBytes } from '../../format';
  import Icon from '../common/Icon.svelte';

  interface Props {
    attachments: readonly ChatAttachment[];
    onRemove: (index: number) => void;
  }

  let { attachments, onRemove }: Props = $props();
</script>

<ul class="chips" aria-label="Attached files">
  {#each attachments as a, i (i)}
    <li class="chip" title="{a.name} ({formatBytes(a.size)})">
      {#if a.kind === 'image' && a.data}
        <img src="data:{a.mimeType};base64,{a.data}" alt="" draggable="false" />
      {:else}
        <span class="file-icon"><Icon name="file-text" size={14} /></span>
      {/if}
      <span class="chip-name">{a.name}</span>
      <button type="button" class="remove" title="Remove {a.name}" onclick={() => onRemove(i)}
        ><Icon name="x" size={12} /><span class="sr-only">Remove {a.name}</span></button
      >
    </li>
  {/each}
</ul>

<style>
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 0 0 4px;
    padding: 0;
    list-style: none;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 220px;
    height: 32px;
    padding: 2px;
    border: 1px solid var(--border-color);
    border-radius: 8px;
    background: var(--bg-primary);
    font-size: 0.76rem;
    color: var(--text-secondary);
    box-sizing: border-box;
  }

  .chip img,
  .chip .file-icon {
    flex: none;
    width: 26px;
    height: 26px;
    border-radius: 6px;
  }

  .chip img {
    object-fit: cover;
  }

  .chip .file-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-tertiary);
    color: var(--text-muted);
  }

  .chip-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .remove {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .remove:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  .remove:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
    border: 0;
  }
</style>
