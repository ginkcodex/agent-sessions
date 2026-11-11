<script lang="ts">
  // The chat's session options (mode, model, effort…) as the agent offers
  // them. Each picker shows the value the agent reports; a change goes to
  // the agent, and the picker follows its answer.
  import type { Chat } from '../../stores/chats.svelte';
  import { pickers } from '../../chatOptions';

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
    {#each list as p (p.id)}
      <label class="picker" title={p.description ?? p.name}>
        <span class="picker-name">{p.name}</span>
        {#if p.kind === 'boolean'}
          <input
            type="checkbox"
            checked={p.value === true}
            disabled={disabled || busy !== null}
            onchange={(e) => void change(p.id, e.currentTarget.checked, e.currentTarget)}
          />
        {:else}
          <select
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
        {/if}
      </label>
    {/each}
    {#if unguarded}
      <span class="unguarded" role="note">{unguarded.name}: the agent runs tools without asking.</span>
    {/if}
  </div>
{/if}

<style>
  .chat-options {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 12px;
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto;
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .picker {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }

  select {
    max-width: 16rem;
    padding: 2px 4px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font: inherit;
  }

  select:disabled,
  input:disabled {
    opacity: 0.6;
  }

  .unguarded {
    color: var(--danger);
  }
</style>
