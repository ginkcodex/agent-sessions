<script lang="ts">
  // Where the user writes to a chat's agent: send, cancel the running turn,
  // end the chat, and pick the session's options.
  import type { Chat } from '../../stores/chats.svelte';
  import { chats } from '../../stores/chats.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import { ALL_AGENTS } from '../../portable';
  import ChatOptions from './ChatOptions.svelte';

  interface Props {
    chat: Chat;
    /**
     * Asks for focus: the input takes it while focusRequest is ahead of
     * focusHandled, even when the ask came before this composer mounted.
     */
    focus?: { focusRequest: number; focusHandled: number };
    /** Starts the agent again once the chat ended. */
    onRestart?: () => void;
    /** Sets one of the session's options; without it no pickers show. */
    onOption?: (configId: string, value: string | boolean) => Promise<void>;
  }

  let { chat, focus, onRestart, onOption }: Props = $props();

  let text = $state('');
  let sending = $state(false);
  let input: HTMLTextAreaElement | undefined = $state();

  let agentName = $derived(ALL_AGENTS.find((a) => a.id === chat.agent)?.label ?? chat.agent);
  let closed = $derived(chat.phase === 'exited');
  let canSend = $derived(chat.phase === 'idle' && !sending && text.trim() !== '');
  let placeholder = $derived(
    chat.phase === 'starting'
      ? `Starting ${agentName}…`
      : closed
        ? 'The chat ended.'
        : `Message ${agentName}. Enter sends, Shift+Enter starts a new line.`,
  );

  $effect(() => {
    const want = focus?.focusRequest ?? 0;
    // The input is disabled while the chat starts: focus once it is not.
    void chat.phase;
    if (!input || !focus || want <= focus.focusHandled || input.disabled) return;
    focus.focusHandled = want;
    input.focus();
  });

  async function send(): Promise<void> {
    if (!canSend) return;
    const prompt = text;
    text = '';
    sending = true;
    try {
      await chats.prompt(chat.chatId, prompt);
    } catch (err) {
      // Nothing was sent: give the text back unless a new one was started.
      if (text === '') text = prompt;
      if (!isStaleReply(err)) toast.show({ title: 'Could not send the message', body: errorText(err, 'The agent did not take it.'), tone: 'error' });
    } finally {
      sending = false;
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key !== 'Enter' || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || e.isComposing) return;
    e.preventDefault();
    void send();
  }

  async function act(what: string, run: () => Promise<void>): Promise<void> {
    try {
      await run();
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: `Could not ${what}`, body: errorText(err, 'Please try again.'), tone: 'error' });
    }
  }
</script>

<form
  class="chat-composer"
  data-chat-composer
  aria-label="Chat with {agentName}"
  onsubmit={(e) => {
    e.preventDefault();
    void send();
  }}
>
  {#if onOption && !closed}
    <ChatOptions {chat} onChange={onOption} />
  {/if}
  <textarea
    bind:this={input}
    bind:value={text}
    rows="3"
    {placeholder}
    aria-label="Message to {agentName}"
    disabled={chat.phase === 'starting' || closed}
    onkeydown={onKeydown}
  ></textarea>
  <div class="composer-actions">
    {#if closed}
      <button type="button" class="secondary" onclick={() => chats.dismiss(chat.key)}>Dismiss</button>
      {#if onRestart}
        <span class="spacer"></span>
        <button type="button" class="primary" title="Start {agentName} again for this session" onclick={onRestart}>Restart</button>
      {/if}
    {:else}
      <button
        type="button"
        class="secondary"
        title="End the chat and stop {agentName}"
        disabled={chat.phase === 'starting'}
        onclick={() => void act('end the chat', () => chats.close(chat.chatId))}>End chat</button
      >
      <span class="spacer"></span>
      {#if chat.phase === 'working'}
        <button type="button" class="secondary" onclick={() => void act('cancel the turn', () => chats.cancel(chat.chatId))}>Cancel</button>
      {/if}
      <button type="submit" class="primary" disabled={!canSend}>Send</button>
    {/if}
  </div>
</form>

<style>
  .chat-composer {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 10px 16px 12px;
    border-top: 1px solid var(--border-color);
    background: var(--bg-primary);
  }

  textarea {
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto;
    min-height: 56px;
    max-height: 40vh;
    padding: 8px 10px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font: inherit;
    font-size: 0.85rem;
    line-height: 1.45;
    resize: vertical;
    box-sizing: border-box;
  }

  textarea:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: -1px;
  }

  textarea:disabled {
    opacity: 0.6;
  }

  .composer-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto;
  }

  .spacer {
    flex: 1;
  }

  button {
    padding: 5px 12px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: 0.78rem;
    cursor: pointer;
  }

  button.primary {
    border-color: transparent;
    background: var(--accent-color);
    color: white;
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  button:hover:not(:disabled) {
    filter: brightness(1.08);
  }
</style>
