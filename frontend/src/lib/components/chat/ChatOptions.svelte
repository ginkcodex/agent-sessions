<script lang="ts">
  // The chat's session options (mode, model, effort…) as the agent offers
  // them. Each picker shows the value the agent reports; a change goes to
  // the agent, and the picker follows its answer. They sit in the composer's
  // toolbar: a picker shows only its value, its name is its tooltip. The
  // selects are the shared Dropdown, not native selects whose popups follow
  // the platform's theme; their menus open upward, over the window's bottom
  // edge.
  import type { Chat } from '../../stores/chats.svelte';
  import { pickers, pickerOptions, type Picker } from '../../chatOptions';
  import Dropdown from '../common/Dropdown.svelte';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    /** Sets an option; the chat's options then carry the agent's answer. */
    onChange: (configId: string, value: string | boolean) => Promise<void>;
  }

  let { chat, onChange }: Props = $props();

  let busy = $state<string | null>(null);
  let list = $derived(pickers(chat.agent, chat.configOptions));
  let disabled = $derived(chat.phase === 'starting' || chat.phase === 'exited');
  let unguarded = $derived(list.find((p) => p.kind === 'select' && p.choices.some((c) => c.value === p.value && c.skipsPermissions)));

  async function change(configId: string, value: string | boolean, box?: HTMLInputElement): Promise<void> {
    busy = configId;
    try {
      await onChange(configId, value);
    } finally {
      busy = null;
      // A refused change leaves the agent's value: a picker shows it again on
      // its own, but a checkbox needs putting back by hand.
      if (box) box.checked = list.find((p) => p.id === configId)?.value === true;
    }
  }

  /** The picker's tooltip: its name and, when the agent gives one, its description. */
  function tip(p: Picker): string {
    return p.description ? `${p.name}: ${p.description}` : p.name;
  }
</script>

{#if list.length > 0}
  <div class="chat-options" data-chat-options>
    {#if unguarded}
      <span class="unguarded" role="note"><Icon name="triangle-alert" size={12} /> {unguarded.name}: the agent runs tools without asking.</span>
    {/if}
    {#each list as p (p.id)}
      {#if p.kind === 'boolean'}
        <label class="picker toggle" title={tip(p)}>
          <span class="picker-name">{p.name}</span>
          <input
            type="checkbox"
            checked={p.value === true}
            disabled={disabled || busy !== null}
            onchange={(e) => void change(p.id, e.currentTarget.checked, e.currentTarget)}
          />
        </label>
      {:else}
        <span class="picker select">
          <Dropdown
            ariaLabel={p.name}
            title={tip(p)}
            options={pickerOptions(p)}
            value={typeof p.value === 'string' ? p.value : ''}
            placeholder="Not set"
            align="right"
            placement="top"
            disabled={disabled || busy !== null}
            onChange={(value) => void change(p.id, value)}
          />
        </span>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .chat-options {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 2px 4px;
    min-width: 0;
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .picker {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border-radius: 6px;
    color: var(--text-secondary);
  }

  .picker.toggle {
    padding: 3px 6px;
  }

  .picker.toggle:hover:not(:has(:disabled)) {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  /* Long values truncate, as the select's did. */
  .picker.select {
    min-width: 0;
    max-width: 14rem;
  }

  input:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 0;
  }

  input:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .unguarded {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-right: 4px;
    color: var(--danger);
  }
</style>
