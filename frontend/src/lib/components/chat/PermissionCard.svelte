<script lang="ts">
  // A permission request from a chat's agent: what the tool would do, and one
  // button per option. Nothing is chosen for the user: no option is the
  // default, none takes focus, and Enter does not approve.
  import type { ChatPermission } from '../../types';
  import type { Chat } from '../../stores/chats.svelte';
  import { chats } from '../../stores/chats.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { permissionView, type PermissionChoice } from '../../chat';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    permission: ChatPermission;
    agentName: string;
  }

  let { chat, permission, agentName }: Props = $props();

  let view = $derived(permissionView(permission));
  let answering = $state(false);

  function lineClass(line: string): string {
    if (line.startsWith('+++') || line.startsWith('---') || line.startsWith('@@')) return 'head';
    if (line.startsWith('+')) return 'add';
    if (line.startsWith('-')) return 'del';
    return '';
  }

  // Space and a click still choose; Enter, which a stray key press after
  // Tab or a send could bring, does not approve.
  function onKeydown(e: KeyboardEvent, choice: PermissionChoice): void {
    if (e.key === 'Enter' && choice.tone === 'allow') e.preventDefault();
  }

  async function answer(choice: PermissionChoice): Promise<void> {
    if (answering) return;
    answering = true;
    try {
      await chats.respond(chat.chatId, permission.id, choice.optionId);
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not answer the request', body: errorText(err, 'The agent no longer waits for it.'), tone: 'error' });
    } finally {
      answering = false;
    }
  }
</script>

<section class="permission-card" data-permission-card={permission.id} aria-label="{agentName} asks for permission">
  <header>
    <Icon name="shield-check" size={14} />
    <span class="asks">{agentName} asks for permission</span>
    {#if view.kind}<span class="kind">{view.kind}</span>{/if}
  </header>
  <p class="title">{view.title}</p>

  {#if view.command}
    <pre class="command"><code>{view.command}</code></pre>
  {/if}
  {#each view.diffs as diff (diff.path)}
    <pre class="diff" aria-label="Changes to {diff.path}">{#each diff.text.split('\n') as line, i (i)}<span class={lineClass(line)}>{line}</span>
{/each}</pre>
  {/each}
  {#if view.paths.length > 0}
    <ul class="paths">
      {#each view.paths as path (path)}<li><code>{path}</code></li>{/each}
    </ul>
  {/if}

  <div class="choices" role="group" aria-label="Answer">
    {#each view.choices as choice (choice.optionId)}
      <button
        type="button"
        class={choice.tone}
        disabled={answering}
        title={choice.lasting ? 'Can last beyond this chat: the agent may keep it as a rule for later sessions' : undefined}
        onkeydown={(e) => onKeydown(e, choice)}
        onclick={() => void answer(choice)}
      >
        <span>{choice.name}</span>
        {#if choice.kindLabel && choice.kindLabel.toLowerCase() !== choice.name.toLowerCase()}
          <span class="choice-kind">{choice.kindLabel}</span>
        {/if}
      </button>
    {/each}
  </div>
  {#if view.choices.some((c) => c.lasting)}
    <p class="lasting">An “always” choice can last beyond this chat: the agent may keep it as a rule for later sessions.</p>
  {/if}
  {#if view.choices.length === 0}
    <p class="lasting">The request has no options. Cancel the turn to answer it.</p>
  {/if}
</section>

<style>
  .permission-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin: 0 0 14px 28px;
    padding: 10px 12px;
    border: 1px solid rgba(234, 179, 8, 0.5);
    border-radius: 6px;
    background: rgba(234, 179, 8, 0.07);
    color: var(--text-primary);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  header {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-secondary);
  }

  .asks {
    font-weight: 600;
  }

  .kind {
    padding: 0 6px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  .title {
    margin: 0;
    overflow-wrap: anywhere;
  }

  pre {
    margin: 0;
    max-height: 220px;
    overflow: auto;
    padding: 6px 8px;
    border-radius: 4px;
    background: var(--bg-secondary);
    font-family: var(--font-mono);
    font-size: 0.72rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .diff .add {
    color: #16a34a;
  }

  .diff .del {
    color: #dc2626;
  }

  .diff .head {
    color: var(--text-muted);
  }

  .paths {
    margin: 0;
    padding-left: 18px;
    font-size: 0.75rem;
  }

  .choices {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  button {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    padding: 5px 12px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: 0.78rem;
    cursor: pointer;
  }

  button.allow {
    border-color: rgba(22, 163, 74, 0.55);
  }

  button.reject {
    border-color: rgba(220, 38, 38, 0.55);
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  button:hover:not(:disabled) {
    filter: brightness(1.08);
  }

  .choice-kind {
    color: var(--text-muted);
    font-size: 0.7rem;
  }

  .lasting {
    margin: 0;
    color: var(--text-muted);
    font-size: 0.72rem;
  }
</style>
