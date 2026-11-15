<script lang="ts">
  // OpenCode's CLI inventories and Codex's live skill catalog, shown in
  // the command result modal for the chat's directory and host. It is the
  // app's listing, not an agent message: it is not sent, saved or kept, and
  // has no connect or enable actions. Closing it, or the chat, drops it.
  import { onMount } from 'svelte';
  import type { ChatInventory, ChatInventoryKind, ChatSkill } from '../../types';
  import type { ChatInspection } from '../../stores/chats.svelte';
  import { api } from '../../api';
  import { mcpStatusFromServers } from '../../chat';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import CommandResultDialog from './CommandResultDialog.svelte';
  import McpServersDialog from './McpServersDialog.svelte';

  interface Props {
    chatId: string;
    kind: ChatInventoryKind;
    agentName: string;
    // Supplied for Codex from its live ACP $skill commands; skips the CLI read.
    skills?: ChatSkill[];
    onClose: () => void;
  }

  let { chatId, kind, agentName, skills, onClose }: Props = $props();

  let result = $state<ChatInventory | null>(null);
  let failure = $state('');
  let inventoryResult = $derived(skills !== undefined ? { kind, supported: true, skills } : result);
  let what = $derived(kind === 'mcp' ? 'MCP servers' : 'skills');

  // The MCP servers show in /mcp's modal, as one finished inspection.
  let inspection = $derived<ChatInspection>(
    failure
      ? { command: '/mcps', text: '', state: 'failed', error: failure }
      : result && !result.supported
        ? { command: '/mcps', text: '', state: 'failed', error: `${agentName} cannot list its MCP servers in a form the app reads.` }
        : { command: '/mcps', text: '', state: result ? 'done' : 'running' },
  );
  let listed = $derived(result?.supported ? mcpStatusFromServers(result.mcpServers ?? []) : undefined);

  onMount(() => {
    if (skills !== undefined) return;
    let live = true;
    api.chatInventory(chatId, kind).then(
      (r) => {
        if (live) result = r;
      },
      (err) => {
        if (live && !isStaleReply(err)) failure = errorText(err, `Could not list ${agentName}'s ${what}.`);
      },
    );
    return () => {
      live = false;
    };
  });
</script>

{#if kind === 'mcp'}
  <McpServersDialog {inspection} {agentName} {listed} note="{agentName}'s servers for this chat's directory, read from its CLI. Not sent to the agent." {onClose} />
{:else}
  <CommandResultDialog title="Skills" note={skills !== undefined
    ? `${agentName}'s skills available in this chat. Not sent to the agent.`
    : `${agentName}'s skills for this chat's directory, read from its CLI. Not sent to the agent.`} filterPlaceholder="Filter skills…" {onClose}>
    {#snippet children(query: string)}
      {@const skills = inventoryResult?.skills ?? []}
      {@const shown = skills.filter((s) => s.name.toLowerCase().includes(query) || s.id.toLowerCase().includes(query) || (s.description ?? '').toLowerCase().includes(query))}
      {#if failure}
        <p class="status error" data-skills-state="error">{failure}</p>
      {:else if !inventoryResult}
        <p class="status" data-skills-state="loading">Listing {agentName}'s skills…</p>
      {:else if !inventoryResult.supported}
        <p class="status" data-skills-state="unsupported">{agentName} cannot list its skills in a form the app reads.</p>
      {:else if skills.length === 0}
        <p class="status" data-skills-state="empty">No skills available.</p>
      {:else if shown.length === 0}
        <p class="status" data-skills-state="no-match">No skills match.</p>
      {:else}
        <ul data-skills-state="listed">
          {#each shown as s (s.id)}
            <li>
              <div class="heading">
                <span class="name">{s.name}</span>
                {#if s.id !== s.name}<span class="meta">{s.id}</span>{/if}
              </div>
              {#if s.description}<p class="description">{s.description}</p>{/if}
            </li>
          {/each}
        </ul>
      {/if}
    {/snippet}
  </CommandResultDialog>
{/if}

<style>
  .status {
    margin: 0;
    padding: 18px 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }
  .status.error { color: var(--danger); }

  ul { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }

  li {
    padding: 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--bg-secondary) 55%, var(--bg-primary));
  }

  .heading { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 10px; }
  .name { min-width: 0; font-family: var(--font-mono); font-weight: 600; color: var(--text-primary); }
  .meta { min-width: 0; font-size: 12px; color: var(--text-muted); }
  .description { margin: 6px 0 0; font-size: 12px; line-height: 1.5; color: var(--text-secondary); overflow-wrap: anywhere; }
</style>
