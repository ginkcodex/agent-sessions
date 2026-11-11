<script lang="ts">
  import Icon from '../common/Icon.svelte';
  import { untrack } from 'svelte';
  import { newSession } from '../../stores/newSession.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { files } from '../../stores/files.svelte';
  import { link } from '../../stores/link.svelte';
  import { workspaceStars } from '../../stores/workspaceStars.svelte';
  import { chatOpen } from '../../stores/chatOpen.svelte';
  import type { AgentID } from '../../types';

  let { cwd }: { cwd: string } = $props();
  let agent = $state<AgentID>('claude-code');
  $effect(() => {
    void cwd;
    untrack(() => {
      void newSession.refresh().then(() => { agent = newSession.defaultAgent; });
      agent = newSession.defaultAgent;
      chatOpen.refresh();
    });
  });
  let blocked = $derived(link.stale || link.locked);
</script>

<div class="workspace-start">
  <span class="icon" aria-hidden="true"><Icon name="folder" size={22} strokeWidth={1.75} /></span>
  <h3>No sessions yet</h3>
  <p class="path">{cwd}</p>
  <button
    type="button"
    class="workspace-star"
    class:starred={workspaceStars.isStarred(cwd)}
    aria-label={workspaceStars.isStarred(cwd) ? 'Unstar workspace' : 'Star workspace'}
    aria-pressed={workspaceStars.isStarred(cwd)}
    title={workspaceStars.isStarred(cwd) ? 'Unstar workspace' : 'Star workspace'}
    disabled={blocked}
    onclick={() => workspaceStars.toggle(cwd)}
  >
    <Icon name="star" size={14} />
    {workspaceStars.isStarred(cwd) ? 'Unstar workspace' : 'Star workspace'}
  </button>
  <p>Start a coding agent in this workspace. New sessions appear here once discovered.</p>
  <label for="workspace-agent">Coding agent</label>
  <select id="workspace-agent" bind:value={agent} disabled={blocked || newSession.starting}>
    {#each newSession.choices as choice (choice.agent)}
      <option value={choice.agent} disabled={!!choice.missing}>{choice.name}{choice.missing ? ` — ${choice.missing}` : ''}</option>
    {/each}
  </select>
  <button type="button" class="primary" disabled={blocked || !newSession.available || newSession.starting || !!newSession.choices.find(c => c.agent === agent)?.missing} onclick={() => void newSession.start(agent, cwd)}>New session</button>
  <div class="secondary">
    {#if chatOpen.canChat(agent)}
      <button type="button" disabled={blocked || chatOpen.opening} title="Start a new session in a chat in this window" onclick={() => void chatOpen.startNew(agent, cwd)}>New chat</button>
    {/if}
    <button type="button" disabled={blocked} onclick={() => void terminals.newShell(cwd)}>Open shell</button>
    <button type="button" disabled={blocked} onclick={() => void files.show(cwd)}>Go to File</button>
  </div>
</div>

<style>
  .workspace-start { padding: 32px 20px; text-align: center; display: flex; flex-direction: column; align-items: center; gap: 10px; }
  h3, p { margin: 0; } h3 { font-size: 16px; } p { color: var(--text-secondary); font-size: 13px; line-height: 1.5; }
  .path { font-family: var(--font-mono); overflow-wrap: anywhere; } .icon { display: grid; place-items: center; width: 48px; height: 48px; border-radius: var(--radius-lg); background: var(--bg-secondary); color: var(--text-muted); }
  label { margin-top: 10px; font-size: 12px; color: var(--text-secondary); }
  button, select { padding: 7px 12px; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); }
  button { cursor: pointer; } button:disabled { opacity: .5; cursor: default; } .primary { background: var(--accent-color); color: white; }
  .secondary { display: flex; gap: 8px; }
  .workspace-star { display: inline-flex; align-items: center; gap: 6px; }
  .workspace-star.starred { color: var(--accent-color); }
  .workspace-star.starred :global(svg) { fill: currentColor; }
  .workspace-star:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
</style>
