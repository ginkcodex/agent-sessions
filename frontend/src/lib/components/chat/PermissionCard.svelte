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
    <span class="chip" aria-hidden="true"><Icon name="shield-check" size={14} /></span>
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
  /* The question card's surface in amber; sizes follow the session's text size. */
  .permission-card {
    --fs: var(--session-font-size, 14.5px);
    --tone: #d97706;
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin: 0 0 16px 28px;
    padding: 14px 16px;
    border: 1px solid color-mix(in srgb, var(--tone) 40%, var(--border-color));
    border-radius: 14px;
    background: var(--bg-secondary);
    box-shadow: var(--shadow-card);
    color: var(--text-primary);
    font-size: calc(var(--fs) * var(--type-body));
    line-height: 1.5;
  }

  header {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-small));
  }

  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 8px;
    background: color-mix(in srgb, var(--tone) 15%, transparent);
    color: var(--tone);
  }

  .asks {
    font-weight: 600;
    color: var(--text-primary);
  }

  .kind {
    padding: 0 8px;
    border-radius: 999px;
    background: var(--segment-track);
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-tag));
    line-height: 1.6;
  }

  .title {
    margin: 0;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  pre {
    margin: 0;
    max-height: 220px;
    overflow: auto;
    padding: 8px 10px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--code-bg, var(--bg-primary));
    font-family: var(--font-mono);
    font-size: calc(var(--fs) * var(--type-code) * 0.9);
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
    font-size: calc(var(--fs) * var(--type-small));
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
    padding: 6px 14px;
    border: 1px solid var(--border-color);
    border-radius: 9px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font: inherit;
    font-size: calc(var(--fs) * var(--type-small));
    font-weight: 600;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
  }

  button:hover:not(:disabled) {
    background: var(--hover-bg);
  }

  button:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  button.allow {
    border-color: color-mix(in srgb, #16a34a 55%, var(--border-color));
  }

  button.allow:hover:not(:disabled) {
    background: color-mix(in srgb, #16a34a 10%, var(--bg-primary));
  }

  button.reject {
    border-color: color-mix(in srgb, #dc2626 50%, var(--border-color));
  }

  button.reject:hover:not(:disabled) {
    background: color-mix(in srgb, #dc2626 9%, var(--bg-primary));
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .choice-kind {
    color: var(--text-muted);
    font-size: calc(var(--fs) * var(--type-tag));
    font-weight: 500;
  }

  .lasting {
    margin: 0;
    color: var(--text-muted);
    font-size: calc(var(--fs) * var(--type-small));
  }
</style>
