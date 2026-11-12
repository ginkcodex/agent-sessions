<script lang="ts">
  // The chat's session options (mode, model, effort…) as the agent offers
  // them. Each picker shows the value the agent reports; a change goes to
  // the agent, and the picker follows its answer. They sit in the composer's
  // toolbar: a select shows only its value, its name is its tooltip.
  import type { Chat } from '../../stores/chats.svelte';
  import { pickers } from '../../chatOptions';
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

  async function change(configId: string, value: string | boolean, target: HTMLSelectElement | HTMLInputElement): Promise<void> {
    busy = configId;
    try {
      await onChange(configId, value);
    } finally {
      busy = null;
      // A refused change leaves the agent's value: show it again.
      const now = list.find((p) => p.id === configId)?.value;
      if (target instanceof HTMLInputElement) target.checked = now === true;
      else if (typeof now === 'string') target.value = now;
    }
  }

  // Groups in the agent's order, values in each.
  function groups(choices: (typeof list)[number]['choices']): { group?: string; choices: typeof choices }[] {
    const out: { group?: string; choices: typeof choices }[] = [];
    for (const c of choices) {
      const last = out[out.length - 1];
      if (last && last.group === c.group) last.choices.push(c);
      else out.push({ group: c.group, choices: [c] });
    }
    return out;
  }
</script>

{#if list.length > 0}
  <div class="chat-options" data-chat-options>
    {#if unguarded}
      <span class="unguarded" role="note"><Icon name="triangle-alert" size={12} /> {unguarded.name}: the agent runs tools without asking.</span>
    {/if}
    {#each list as p (p.id)}
      <label class="picker" class:toggle={p.kind === 'boolean'} title={p.description ? `${p.name}: ${p.description}` : p.name}>
        {#if p.kind === 'boolean'}
          <span class="picker-name">{p.name}</span>
          <input
            type="checkbox"
            checked={p.value === true}
            disabled={disabled || busy !== null}
            onchange={(e) => void change(p.id, e.currentTarget.checked, e.currentTarget)}
          />
        {:else}
          <select
            aria-label={p.name}
            value={p.value ?? ''}
            disabled={disabled || busy !== null}
            onchange={(e) => void change(p.id, e.currentTarget.value, e.currentTarget)}
          >
            {#if p.value === undefined}
              <option value="" disabled>Not set</option>
            {/if}
            {#each groups(p.choices) as g, gi (gi)}
              {#if g.group}
                <optgroup label={g.group}>
                  {#each g.choices as c (c.value)}
                    <option value={c.value} title={c.description}>{c.name}{c.skipsPermissions ? ' (no permission requests)' : ''}</option>
                  {/each}
                </optgroup>
              {:else}
                {#each g.choices as c (c.value)}
                  <option value={c.value} title={c.description}>{c.name}{c.skipsPermissions ? ' (no permission requests)' : ''}</option>
                {/each}
              {/if}
            {/each}
          </select>
          <Icon name="chevron-down" size={12} class="chevron" />
        {/if}
      </label>
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

  .picker:hover:not(:has(:disabled)) {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  select {
    appearance: none;
    field-sizing: content;
    min-width: 0;
    max-width: 14rem;
    padding: 3px 20px 3px 6px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: inherit;
    font: inherit;
    text-overflow: ellipsis;
    cursor: pointer;
  }

  select:focus-visible,
  input:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 0;
  }

  /* The select's own arrow is gone with appearance: none. */
  .picker :global(.chevron) {
    position: absolute;
    right: 5px;
    pointer-events: none;
  }

  /* The list follows the theme, not the transparent select. */
  select :global(option),
  select :global(optgroup) {
    background: var(--bg-secondary);
    color: var(--text-primary);
  }

  select:disabled,
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
