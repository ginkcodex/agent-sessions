<script lang="ts">
  // Where the user writes to a chat's agent: send, cancel the running turn,
  // end the chat, and pick the session's options. One card: the text box,
  // then a toolbar with End chat on the left and, on the right, the context
  // in use, the pickers and a round send (or, while a turn runs, stop) button.
  import type { Chat } from '../../stores/chats.svelte';
  import { chats } from '../../stores/chats.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import { ALL_AGENTS } from '../../portable';
  import ChatOptions from './ChatOptions.svelte';
  import Icon from '../common/Icon.svelte';

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
        : `Message ${agentName}…`,
  );
  let hintId = $derived(`chat-hint-${chat.key}`);

  // The ring's share of the context in use, from the agent's usage_update.
  const RING = 2 * Math.PI * 7;
  let usage = $derived(chat.tail.usage);
  let usedShare = $derived(usage ? Math.min(1, usage.used / usage.size) : 0);
  let usageTitle = $derived(
    usage ? `Context: ${usage.used.toLocaleString('en-US')} of ${usage.size.toLocaleString('en-US')} tokens in use` : '',
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
  <div class="card" class:disabled={chat.phase === 'starting' || closed}>
    <textarea
      bind:this={input}
      bind:value={text}
      rows="2"
      {placeholder}
      aria-label="Message to {agentName}"
      aria-describedby={hintId}
      disabled={chat.phase === 'starting' || closed}
      onkeydown={onKeydown}
    ></textarea>
    <span id={hintId} class="sr-only">Enter sends, Shift+Enter starts a new line.</span>
    <div class="toolbar">
      {#if closed}
        <button type="button" class="text" onclick={() => chats.dismiss(chat.key)}>Dismiss</button>
        <span class="spacer"></span>
        {#if onRestart}
          <button type="button" class="text primary" title="Start {agentName} again for this session" onclick={onRestart}>Restart</button>
        {/if}
      {:else}
        <button
          type="button"
          class="icon"
          title="End the chat and stop {agentName}"
          disabled={chat.phase === 'starting'}
          onclick={() => void act('end the chat', () => chats.close(chat.chatId))}
          ><Icon name="power" size={15} /><span class="sr-only">End chat</span></button
        >
        <span class="spacer"></span>
        {#if usage}
          <span class="usage" data-chat-usage title={usageTitle}>
            <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
              <circle cx="9" cy="9" r="7" class="track" />
              <circle cx="9" cy="9" r="7" class="used" stroke-dasharray="{RING * usedShare} {RING}" transform="rotate(-90 9 9)" />
            </svg>
            {Math.round(usedShare * 100)}%<span class="sr-only">{' of the context in use'}</span>
          </span>
        {/if}
        {#if onOption}
          <ChatOptions {chat} onChange={onOption} />
        {/if}
        {#if chat.phase === 'working'}
          <button type="button" class="round stop" title="Stop {agentName}'s turn" onclick={() => void act('cancel the turn', () => chats.cancel(chat.chatId))}
            ><Icon name="square" size={12} class="filled" /><span class="sr-only">Cancel</span></button
          >
        {:else}
          <button type="submit" class="round send" title="Send (Enter)" disabled={!canSend}
            ><Icon name="arrow-up" size={16} strokeWidth={2.5} /><span class="sr-only">Send</span></button
          >
        {/if}
      {/if}
    </div>
  </div>
</form>

<style>
  .chat-composer {
    padding: 8px 16px 14px;
    background: var(--bg-primary);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 4px;
    width: 100%;
    max-width: max(900px, 80%);
    margin: 0 auto;
    padding: 10px 10px 8px 14px;
    border: 1px solid var(--border-strong);
    border-radius: 16px;
    background: var(--bg-secondary);
    box-shadow: var(--shadow-card);
    box-sizing: border-box;
    transition: border-color var(--dur-fast) var(--ease-out);
  }

  .card:focus-within:not(.disabled) {
    border-color: color-mix(in srgb, var(--accent-color) 55%, var(--border-strong));
  }

  textarea {
    width: 100%;
    field-sizing: content;
    min-height: 2.9em;
    max-height: 40vh;
    padding: 2px 4px 2px 0;
    border: none;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: 0.88rem;
    line-height: 1.45;
    resize: none;
    box-sizing: border-box;
  }

  textarea:focus {
    outline: none;
  }

  textarea::placeholder {
    color: var(--text-muted);
  }

  textarea:disabled {
    opacity: 0.6;
  }

  .toolbar {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 30px;
  }

  .spacer {
    flex: 1;
  }

  button {
    display: inline-flex;
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

  button.icon {
    width: 28px;
    height: 28px;
    border-radius: 8px;
  }

  button.icon:hover:not(:disabled),
  button.text:hover:not(:disabled) {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  button.text {
    padding: 4px 10px;
    border-radius: 8px;
    font-size: 0.78rem;
  }

  button.text.primary {
    background: var(--accent-color);
    color: white;
  }

  button.round {
    flex: none;
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--text-primary);
    color: var(--bg-primary);
  }

  button.round.send:disabled {
    background: var(--btn-bg-hover);
    color: var(--text-muted);
    opacity: 1;
  }

  button.round:hover:not(:disabled),
  button.text.primary:hover {
    filter: brightness(1.15);
  }

  button.round :global(.filled) {
    fill: currentColor;
  }

  .usage {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 0 4px;
    font-size: 0.72rem;
    font-variant-numeric: tabular-nums;
    color: var(--text-muted);
  }

  .usage circle {
    fill: none;
    stroke-width: 2.5;
  }

  .usage .track {
    stroke: var(--border-strong);
  }

  .usage .used {
    stroke: var(--text-secondary);
    stroke-linecap: round;
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
