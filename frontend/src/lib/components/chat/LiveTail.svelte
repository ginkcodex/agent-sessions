<script lang="ts">
  // What a chat's agent has streamed and the saved transcript does not show
  // yet, below the saved messages, with the chat's state under it.
  import type { SessionRef } from '../../types';
  import type { Chat } from '../../stores/chats.svelte';
  import type { TranscriptMode } from '../../transcript';
  import { tailBlocks } from '../../chat';
  import { asksWhileWorking } from '../../chatQuestions';
  import { ALL_AGENTS } from '../../portable';
  import MessageBubble from '../transcript/MessageBubble.svelte';
  import PermissionCard from './PermissionCard.svelte';
  import QuestionCard from './QuestionCard.svelte';
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

  let blocks = $derived(tailBlocks(chat.tail));
  // A late question, or one the agent keeps working through, does not hold the turn.
  let waiting = $derived(chat.permissions.length > 0 || chat.questions.some((q) => !q.late && !asksWhileWorking(q)));
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
    disconnected: 'The connection to the host dropped, which ended the chat.',
  };
  // The agent's own log helps only when it stopped by itself; a chat the
  // user ended, or a dropped connection, says so in one line.
  let exitLog = $derived(chat.exit && (chat.exit.reason === 'exited' || chat.exit.reason === 'crashed') ? chat.exit.stderrTail : undefined);
</script>

<div class="live-tail" data-live-tail aria-live="polite">
  {#each blocks as block (block.kind === 'message' ? block.message.id : `answer:${block.answer.key}`)}
    {#if block.kind === 'message'}
      <div class="live-message">
        <MessageBubble message={block.message} {sessionRef} {mode} {onFileLink} />
      </div>
    {:else}
      <div class="answer-line" class:answered={block.answer.outcome === 'accept'} data-answer-line={block.answer.questionId}>
        <Icon name={block.answer.outcome === 'accept' ? 'check' : 'circle-help'} size={14} />
        <span>{block.answer.summary}</span>
      </div>
    {/if}
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
    {#each chat.questions as question (question.id)}
      <QuestionCard {chat} {question} {agentName} />
    {/each}
    {#if chat.phase === 'working'}
      <div class="chat-status" role="status">
        <Icon name="loader-circle" size={14} class="spin" />
        <span>{waiting ? `${agentName} waits for your answer.` : `${agentName} is working…`}</span>
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
        {#if exitLog}
          <details class="exit-log">
            <summary>Show the agent's log</summary>
            <pre class="stderr">{exitLog}</pre>
          </details>
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    line-height: 1.45;
  }

  .answer-line {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    margin: 0 0 14px 28px;
    color: var(--text-muted);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    line-height: 1.45;
    overflow-wrap: anywhere;
  }

  .answer-line.answered {
    color: var(--text-secondary);
  }

  .answer-line.answered :global(.icon) {
    color: #16a34a;
  }

  .answer-line :global(.icon),
  .chat-status :global(.icon),
  .chat-notice :global(.icon) {
    margin-top: calc((1.45em - 14px) / 2);
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    cursor: pointer;
    overflow-wrap: anywhere;
    text-align: left;
  }

  .notice-action:hover {
    filter: brightness(1.08);
  }

  .exit-log summary {
    width: fit-content;
    color: var(--text-muted);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    cursor: pointer;
  }

  .exit-log summary:hover {
    color: var(--text-primary);
  }

  .stderr {
    margin: 6px 0 0;
    max-height: 160px;
    overflow: auto;
    font-family: var(--font-mono);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
