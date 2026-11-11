<script lang="ts">
  // What a chat's agent has streamed and the saved transcript does not show
  // yet, below the saved messages, with the chat's state under it.
  import type { SessionRef } from '../../types';
  import type { Chat } from '../../stores/chats.svelte';
  import type { TranscriptMode } from '../../transcript';
  import { tailMessages } from '../../chat';
  import { ALL_AGENTS } from '../../portable';
  import MessageBubble from '../transcript/MessageBubble.svelte';
  import PermissionCard from './PermissionCard.svelte';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    sessionRef: SessionRef;
    mode?: TranscriptMode;
    onFileLink?: (value: string) => void;
    /** Opens a terminal tab in the chat's directory, to log in there. */
    onOpenTerminal?: () => void;
  }

  let { chat, sessionRef, mode = 'activity', onFileLink, onOpenTerminal }: Props = $props();

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
  {:else}
    {#each chat.permissions as permission (permission.id)}
      <PermissionCard {chat} {permission} {agentName} />
    {/each}
    {#if chat.phase === 'working'}
      <div class="chat-status" role="status">
        <Icon name="loader-circle" size={14} class="spin" />
        <span>{chat.permissions.length > 0 ? `${agentName} waits for your answer.` : `${agentName} is working…`}</span>
      </div>
    {/if}
  {/if}

  {#if chat.error}
    <div class="chat-notice error" role="alert">
      <Icon name="circle-alert" size={14} />
      <div class="exit-text">
        <span>{chat.error}</span>
        {#if chat.authMethods?.length}
          <ul class="auth-methods">
            {#each chat.authMethods as method (method.id)}
              <li><strong>{method.name}</strong>{method.description ? `: ${method.description}` : ''}</li>
            {/each}
          </ul>
          {#if onOpenTerminal}
            <div>
              <button type="button" class="notice-action" onclick={onOpenTerminal}>Open a terminal in {chat.dir}</button>
            </div>
          {/if}
        {/if}
      </div>
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

  .auth-methods {
    margin: 0;
    padding-left: 18px;
  }

  .notice-action {
    padding: 4px 10px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font-size: 0.75rem;
    cursor: pointer;
    overflow-wrap: anywhere;
    text-align: left;
  }

  .notice-action:hover {
    filter: brightness(1.08);
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
