<script lang="ts">
  // What a chat's agent has streamed and the saved transcript does not show
  // yet, below the saved messages, with the chat's state under it.
  import type { SessionRef } from '../../types';
  import type { Chat } from '../../stores/chats.svelte';
  import type { TranscriptMode } from '../../transcript';
  import { tailMessages } from '../../chat';
  import { ALL_AGENTS } from '../../portable';
  import MessageBubble from '../transcript/MessageBubble.svelte';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    sessionRef: SessionRef;
    mode?: TranscriptMode;
    onFileLink?: (value: string) => void;
  }

  let { chat, sessionRef, mode = 'activity', onFileLink }: Props = $props();

  let messages = $derived(tailMessages(chat.tail));
  let agentName = $derived(ALL_AGENTS.find((a) => a.id === chat.agent)?.label ?? chat.agent);
  // A turn that stopped short says why; a normal end says nothing.
  const STOPS: Record<string, string> = {
    cancelled: 'The turn was cancelled.',
    max_tokens: 'The agent stopped: it reached its token limit.',
    max_turn_requests: 'The agent stopped: it reached its limit of requests for one turn.',
    refusal: 'The agent refused to go on.',
  };
  let stopNote = $derived(chat.phase === 'idle' && !chat.error && chat.stopReason ? STOPS[chat.stopReason] : undefined);
  const EXITS: Record<string, string> = {
    closed: 'The chat ended.',
    exited: 'The agent quit.',
    crashed: 'The agent stopped unexpectedly.',
  };
</script>

<div class="live-tail" data-live-tail aria-live="polite">
  {#each messages as message (message.id)}
    <div class="live-message">
      <MessageBubble {message} {sessionRef} {mode} {onFileLink} />
    </div>
  {/each}

  {#if chat.phase === 'starting'}
    <div class="chat-status" role="status">
      <Icon name="loader-circle" size={14} class="spin" />
      <span>{chat.session ? `Opening the session in ${agentName}…` : `Starting ${agentName}…`}</span>
    </div>
  {:else if chat.phase === 'working'}
    {#each chat.permissions as permission (permission.id)}
      <div class="chat-status waiting" role="status">
        <Icon name="shield-check" size={14} />
        <span>{agentName} asks for permission: {permission.toolCall?.title || 'a tool call'}</span>
      </div>
    {/each}
    <div class="chat-status" role="status">
      <Icon name="loader-circle" size={14} class="spin" />
      <span>{agentName} is working…</span>
    </div>
  {/if}

  {#if chat.error}
    <div class="chat-notice error" role="alert">
      <Icon name="circle-alert" size={14} />
      <span>{chat.error}</span>
    </div>
  {:else if stopNote}
    <div class="chat-notice" role="status">
      <Icon name="info" size={14} />
      <span>{stopNote}</span>
    </div>
  {/if}

  {#if chat.exit}
    <div class="chat-notice" class:error={chat.exit.reason === 'crashed'} role="status">
      <Icon name={chat.exit.reason === 'crashed' ? 'triangle-alert' : 'info'} size={14} />
      <div class="exit-text">
        <span>{EXITS[chat.exit.reason] ?? 'The chat ended.'}{chat.exit.error ? ` ${chat.exit.error}` : ''}</span>
        {#if chat.exit.stderrTail}
          <pre class="stderr">{chat.exit.stderrTail}</pre>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .live-tail {
    display: flex;
    flex-direction: column;
  }

  .chat-status,
  .chat-notice {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    margin: 0 0 14px 28px;
    color: var(--text-muted);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .chat-status :global(.icon),
  .chat-notice :global(.icon) {
    margin-top: 2px;
  }

  .chat-status.waiting {
    color: var(--text-secondary);
  }

  .chat-notice {
    padding: 8px 10px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }

  .chat-notice.error {
    border-color: rgba(239, 68, 68, 0.45);
    background: rgba(239, 68, 68, 0.08);
  }

  .exit-text {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }

  .stderr {
    margin: 0;
    max-height: 160px;
    overflow: auto;
    font-family: var(--font-mono);
    font-size: 0.72rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
