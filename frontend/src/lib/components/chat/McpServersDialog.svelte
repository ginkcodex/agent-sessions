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
                <div class="identity">
                  <div class="heading">
                    <span class="name">{s.name}</span>
                    {#if s.scope}<span class="meta scope">{s.scope}</span>{/if}
                  </div>
                  {#if s.detail}<div class="meta detail">{s.detail}</div>{/if}
                </div>
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
  .status, .notes {
    margin: 0;
    padding: 18px 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }
  .status.error { color: var(--danger); }
  .summary { margin: 0 0 18px; color: var(--text-secondary); }
  section + section { margin-top: 22px; }

  h3 {
    margin: 0 0 10px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  ul { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px 16px;
    padding: 16px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--bg-secondary) 55%, var(--bg-primary));
  }

  .identity { flex: 1 1 240px; min-width: 0; }
  .heading { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 10px; }
  .name { min-width: 0; font-family: var(--font-mono); font-weight: 600; color: var(--text-primary); }
  .meta { min-width: 0; font-size: 12px; color: var(--text-muted); }
  .scope { padding: 1px 6px; border-radius: var(--radius-sm); background: var(--bg-secondary); }
  .detail { margin-top: 6px; overflow-wrap: anywhere; }

  .badge {
    max-width: 100%;
    box-sizing: border-box;
    padding: 3px 9px;
    border: 1px solid var(--border-color);
    border-radius: 999px;
    background: var(--bg-secondary);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .badge.ok { --badge-tone: var(--status-live); }
  .badge.bad { --badge-tone: var(--danger); }
  .badge.needs { --badge-tone: var(--status-needs); }
  .badge.ok, .badge.bad, .badge.needs {
    color: var(--badge-tone);
    border-color: color-mix(in srgb, var(--badge-tone) 25%, transparent);
    background: color-mix(in srgb, var(--badge-tone) 10%, var(--bg-primary));
  }

  .notes { margin-top: 18px; font-size: 12px; }
</style>
