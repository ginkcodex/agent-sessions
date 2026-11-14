<script lang="ts">
  // The messages sent while the agent works, above the composer: each waits
  // for its turn, in order, and has Send now, Edit and Remove. Send now puts
  // the message into the running turn when the agent can take it there, and
  // sends a paused queue's message at once. A paused queue (the last turn
  // was stopped or failed) says so.
  import type { Chat } from '../../stores/chats.svelte';
  import type { ChatDraft, ChatQueuedPrompt } from '../../types';
  import { chats } from '../../stores/chats.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import { queuedCount, queuedLabel } from '../../chat';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    agentName: string;
    /** Edit moves a message back into the composer, which must be empty. */
    composerEmpty: boolean;
    onEdit: (draft: ChatDraft) => void;
  }

  let { chat, agentName, composerEmpty, onEdit }: Props = $props();

  /** The message a Send now, Edit or Remove waits on. */
  let busy = $state<string | null>(null);

  let paused = $derived(chat.phase === 'idle');
  // While the agent works, only an agent that steers takes a message now.
  let canSendNow = $derived(chat.phase === 'idle' || (chat.phase === 'working' && !!chat.canSteer));
  let sendNowTitle = $derived(chat.phase === 'working' ? `Send to ${agentName} now, into its running turn` : 'Send this message now');

  async function act(q: ChatQueuedPrompt, what: string, run: () => Promise<void>): Promise<void> {
    busy = q.id;
    try {
      await run();
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: `Could not ${what}`, body: errorText(err, 'Please try again.'), tone: 'error' });
    } finally {
      busy = null;
    }
  }

  function sendNow(q: ChatQueuedPrompt): void {
    void act(q, 'send the message', () => chats.sendNow(chat.chatId, q.id));
  }

  function edit(q: ChatQueuedPrompt): void {
    void act(q, 'edit the message', async () => onEdit(await chats.unqueue(chat.chatId, q.id)));
  }

  function remove(q: ChatQueuedPrompt): void {
    void act(q, 'remove the message', async () => void (await chats.unqueue(chat.chatId, q.id)));
  }
</script>

<section class="queued" data-chat-queue aria-label={queuedCount(chat.queue.length)}>
  {#if paused}
    <p class="note" data-chat-queue-paused>Paused. Send now or send a new message to go on.</p>
  {/if}
  <ol>
    {#each chat.queue as q (q.id)}
      {@const label = queuedLabel(q)}
      <li data-queued={q.id}>
        <Icon name="corner-down-right" size={13} />
        <span class="line" title={label.full}>{label.line}</span>
        {#if label.files}<span class="files">{label.files}</span>{/if}
        {#if canSendNow}
          <button type="button" class="text" title={sendNowTitle} disabled={busy !== null} onclick={() => sendNow(q)}>Send now</button>
        {/if}
        <button
          type="button"
          class="text"
          title={composerEmpty ? 'Move the message back into the message box' : 'Send or clear the message box first'}
          disabled={busy !== null || !composerEmpty}
          onclick={() => edit(q)}>Edit</button
        >
        <button type="button" class="icon" title="Remove the message" disabled={busy !== null} onclick={() => remove(q)}
          ><Icon name="x" size={13} /><span class="sr-only">Remove</span></button
        >
      </li>
    {/each}
  </ol>
</section>

<style>
  .queued {
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto 6px;
    box-sizing: border-box;
  }

  .note {
    margin: 0 0 4px 4px;
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  ol {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 30px;
    padding: 2px 4px 2px 10px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-secondary);
    color: var(--text-muted);
    font-size: 0.8rem;
  }

  .line {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .files {
    flex: none;
    font-size: 0.72rem;
  }

  button {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }

  button:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  button:disabled {
    opacity: 0.45;
    cursor: default;
  }

  button.text {
    flex: none;
    padding: 3px 8px;
    border-radius: 7px;
    font-size: 0.75rem;
  }

  button.icon {
    width: 24px;
    height: 24px;
    border-radius: 7px;
  }

  button:hover:not(:disabled) {
    background: var(--hover-bg);
    color: var(--text-primary);
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
