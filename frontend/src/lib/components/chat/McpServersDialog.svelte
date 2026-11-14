<script lang="ts">
  // /mcp's answer: the agent's reply to the captured turn (chat.inspection),
  // its servers grouped by status. A reply in another shape shows as
  // Markdown. Nothing here changes a server.
  import type { ChatInspection } from '../../stores/chats.svelte';
  import { parseMcpStatus } from '../../chat';
  import CommandResultDialog from './CommandResultDialog.svelte';
  import TextPart from '../transcript/parts/TextPart.svelte';

  interface Props {
    inspection: ChatInspection;
    agentName: string;
    onClose: () => void;
  }

  let { inspection, agentName, onClose }: Props = $props();
  let status = $derived(inspection.state === 'done' ? parseMcpStatus(inspection.text) : null);

  function tone(title: string): string {
    if (title === 'Connected') return 'ok';
    if (title === 'Failed' || title === 'Cancelled') return 'bad';
    if (title === 'Needs authentication') return 'needs';
    return '';
  }
</script>

<CommandResultDialog title="MCP servers" note="{agentName}'s servers in this chat, as {agentName} reports them." filterPlaceholder="Filter servers…" {onClose}>
  {#snippet children(query: string)}
    {#if inspection.state === 'running'}
      <p class="status" data-mcp-state="loading">Asking {agentName} for its MCP servers…</p>
    {:else if inspection.state === 'failed'}
      <p class="status error" data-mcp-state="failed">{inspection.error || `${agentName} could not list its MCP servers.`}</p>
    {:else if inspection.state === 'cancelled'}
      <p class="status" data-mcp-state="cancelled">Stopped before {agentName} answered.</p>
    {:else if inspection.state === 'exited'}
      <p class="status" data-mcp-state="exited">The chat ended before {agentName} answered.</p>
    {:else if !status}
      <div data-mcp-state="markdown"><TextPart text={inspection.text || `${agentName} sent no answer.`} /></div>
    {:else}
      {@const groups = status.groups.map((g) => ({ ...g, servers: g.servers.filter((s) => s.name.toLowerCase().includes(query)) })).filter((g) => g.servers.length > 0)}
      <p class="summary" data-mcp-state="listed">MCP servers: {status.summary}</p>
      {#if status.groups.length > 0 && groups.length === 0}
        <p class="status" data-mcp-state="no-match">No servers match.</p>
      {/if}
      {#each groups as g (g.title)}
        <section aria-label="{g.title} ({g.servers.length})">
          <h3>{g.title} ({g.servers.length})</h3>
          <ul>
            {#each g.servers as s, i (i)}
              <li>
                <span class="name">{s.name}</span>
                {#if s.scope}<span class="meta">{s.scope}</span>{/if}
                {#if s.detail}<span class="meta detail">{s.detail}</span>{/if}
                <span class="spacer"></span>
                <span class="badge {tone(g.title)}">{g.title === 'Connected' ? '✓ ' : ''}{g.title}</span>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
      {#each status.notes as n, i (i)}
        <div class="notes"><TextPart text={n} /></div>
      {/each}
    {/if}
  {/snippet}
</CommandResultDialog>

<style>
  .status, .summary { margin: 4px 0; color: var(--text-secondary); }
  .status.error { color: var(--danger); }
  h3 { margin: 12px 0 4px; font-size: 11px; font-weight: 600; letter-spacing: 0.02em; text-transform: uppercase; color: var(--text-muted); }
  ul { margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: baseline; gap: 8px; padding: 5px 6px; border-radius: var(--radius-md); }
  li:hover { background: var(--hover-bg); }
  .name { font-family: var(--font-mono); color: var(--text-primary); }
  .meta { font-size: 12px; color: var(--text-muted); }
  .detail { min-width: 0; overflow-wrap: anywhere; }
  .spacer { flex: 1; }
  .badge { flex: none; padding: 0 6px; border: 1px solid var(--border-color); border-radius: 6px; font-size: 11px; color: var(--text-secondary); white-space: nowrap; }
  .badge.ok { color: var(--status-live); border-color: currentColor; }
  .badge.bad { color: var(--danger); border-color: currentColor; }
  .badge.needs { color: var(--status-needs); border-color: currentColor; }
  .notes { margin-top: 12px; font-size: 12px; color: var(--text-secondary); }
</style>
