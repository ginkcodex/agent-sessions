<script lang="ts">
  // Where the user writes to a chat's agent: send, cancel the running turn,
  // end the chat or switch it to a terminal, and pick the session's options.
  // One card: the attached files, the text box, then a toolbar with Attach
  // and the end menu on the left and, on the right, the context in use, the
  // pickers and a round send (or, while a turn runs, stop) button. Files
  // dropped on the card and images pasted into it are attached too. While
  // the agent works a message is queued, and the queue shows above the card.
  import { onMount } from 'svelte';
  import type { Chat } from '../../stores/chats.svelte';
  import type { ChatAttachment, ChatAttachResult, ChatDraft } from '../../types';
  import { api } from '../../api';
  import { addAttachments, canSendPrompt, endChatQuestion, skippedToast, switchToTerminalQuestion } from '../../chat';
  import AttachmentChips from './AttachmentChips.svelte';
  import { chats } from '../../stores/chats.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { filePrompt } from '../../stores/filePrompt.svelte';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import { ALL_AGENTS } from '../../portable';
  import ChatOptions from './ChatOptions.svelte';
  import QueuedMessages from './QueuedMessages.svelte';
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
    /** Ends the chat and continues its session in a terminal tab. */
    onSwitchToTerminal?: () => Promise<void>;
    /**
     * The agent saved the chat's session where the index finds it. A new
     * chat's agent names a session ID at once but saves nothing before its
     * first turn, so there is nothing to resume in a terminal until then.
     */
    sessionSaved?: boolean;
    /** Sets one of the session's options; without it no pickers show. */
    onOption?: (configId: string, value: string | boolean) => Promise<void>;
  }

  let { chat, focus, onRestart, onSwitchToTerminal, sessionSaved = false, onOption }: Props = $props();

  let text = $state('');
  let attachments = $state<ChatAttachment[]>([]);
  let sending = $state(false);
  let attaching = $state(false);
  let endMenuOpen = $state(false);
  let endMenuRoot: HTMLDivElement | undefined = $state();
  let input: HTMLTextAreaElement | undefined = $state();
  let form: HTMLFormElement | undefined = $state();

  let agentName = $derived(ALL_AGENTS.find((a) => a.id === chat.agent)?.label ?? chat.agent);
  let closed = $derived(chat.phase === 'exited');
  let canSwitch = $derived(!!chat.session && sessionSaved);
  // A chat whose agent (or host) does not say what a prompt may carry takes
  // text only.
  let caps = $derived(chat.promptCapabilities);
  let canAttach = $derived(!!caps && !closed && chat.phase !== 'starting');
  // While the agent works, a message waits in the queue for the turn's end.
  let queues = $derived(chat.phase === 'working' && !!chat.canQueue);
  let canSend = $derived(canSendPrompt(chat.phase, sending, text, attachments.length, queues));
  let empty = $derived(text.trim() === '' && attachments.length === 0);
  let placeholder = $derived(
    chat.phase === 'starting'
      ? `Starting ${agentName}…`
      : closed
        ? 'The chat ended.'
        : queues
          ? `Queue a message for ${agentName}…`
          : `Message ${agentName}…`,
  );
  let hintId = $derived(`chat-hint-${chat.key}`);
  // WebKit (macOS and Linux) reads autocorrect on a textarea; Svelte's types
  // list it for inputs only.
  const SAFARI_NO_AUTOCORRECT: Record<string, string> = { autocorrect: 'off' };

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
    const prompt = text.trim() === '' ? '' : text;
    const files = $state.snapshot(attachments) as ChatAttachment[];
    text = '';
    attachments = [];
    sending = true;
    try {
      await chats.prompt(chat.chatId, prompt, files);
    } catch (err) {
      // Nothing was sent: give the text and the files back unless new ones
      // were started.
      if (text === '') text = prompt;
      if (attachments.length === 0) attachments = files;
      if (!isStaleReply(err)) toast.show({ title: 'Could not send the message', body: errorText(err, 'The agent did not take it.'), tone: 'error' });
    } finally {
      sending = false;
    }
  }

  /** Edit: a queued message comes back into the empty composer. */
  function edit(draft: ChatDraft): void {
    text = draft.text ?? '';
    attachments = draft.attachments ?? [];
    input?.focus();
  }

  /** Adds prepared files, and says which could not be attached and why. */
  function add(result: ChatAttachResult): void {
    const next = addAttachments(attachments, result, { name: agentName, images: !!caps?.image });
    attachments = next.attachments;
    const note = skippedToast(next.skipped);
    if (note) toast.show({ ...note, tone: 'error' });
  }

  async function attach(prepare: () => Promise<ChatAttachResult>): Promise<void> {
    attaching = true;
    try {
      add(await prepare());
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not attach the files', body: errorText(err, 'Please try again.'), tone: 'error' });
    } finally {
      attaching = false;
    }
  }

  function pick(): void {
    if (canAttach && !attaching) void attach(() => api.chatPickAttachments(chat.chatId)).then(() => input?.focus());
  }

  function remove(index: number): void {
    attachments = attachments.filter((_, i) => i !== index);
    input?.focus();
  }

  // Files dropped on this composer; the terminal panel leaves them alone.
  onMount(() =>
    api.onFileDrop((x, y, paths) => {
      if (!form || !canAttach) return;
      if (document.elementFromPoint(x, y)?.closest('[data-file-drop-target="chat"]') !== form) return;
      void attach(() => api.chatAttachFiles(paths));
    }),
  );

  // A pasted image is attached; pasted text, even with a picture of it
  // beside (as office apps copy), is pasted as text.
  function onPaste(e: ClipboardEvent): void {
    const data = e.clipboardData;
    if (!canAttach || !data || data.types.includes('text/plain')) return;
    const images = [...data.files].filter((f) => f.type.startsWith('image/'));
    if (images.length === 0) return;
    e.preventDefault();
    void attach(async () => {
      const out: ChatAttachResult = { attachments: [], skipped: [] };
      for (const f of images) {
        const name = f.name || 'Pasted image';
        try {
          out.attachments.push(await api.chatAttachImage(name, await fileBase64(f)));
        } catch (err) {
          const reason = errorText(err, 'could not be read');
          out.skipped.push({ name, reason: reason.startsWith(`${name}: `) ? reason.slice(name.length + 2) : reason });
        }
      }
      return out;
    });
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

  // Ending a chat whose agent works stops its turn, and queued messages are
  // never sent: ask first.
  async function endChat(): Promise<void> {
    const question = endChatQuestion(chat.phase, agentName, chat.queue.length);
    if (question && (await filePrompt.ask(question.title, question.message, question.choices)) !== 'end') return;
    await act('end the chat', () => chats.close(chat.chatId));
  }

  // Switching stops the chat as End chat does — a working turn with it —
  // and the session continues in a terminal tab: ask first while it works.
  async function switchToTerminal(): Promise<void> {
    const question = switchToTerminalQuestion(chat.phase, agentName, chat.queue.length);
    if (question && (await filePrompt.ask(question.title, question.message, question.choices)) !== 'switch') return;
    await onSwitchToTerminal?.();
  }

  // The end menu closes on a click outside it, and on Escape, which puts
  // the focus back on its trigger.
  function handleWindowPointerDown(e: PointerEvent): void {
    if (endMenuOpen && endMenuRoot && !endMenuRoot.contains(e.target as Node)) endMenuOpen = false;
  }

  function handleWindowKeydown(e: KeyboardEvent): void {
    if (!endMenuOpen || e.key !== 'Escape') return;
    endMenuOpen = false;
    endMenuRoot?.querySelector('button')?.focus();
  }

  /** A file's bytes in base64. */
  async function fileBase64(f: Blob): Promise<string> {
    const bytes = new Uint8Array(await f.arrayBuffer());
    let bin = '';
    for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    return btoa(bin);
  }
</script>

<svelte:window onpointerdown={handleWindowPointerDown} onkeydown={handleWindowKeydown} />

<form
  bind:this={form}
  class="chat-composer"
  data-chat-composer
  data-file-drop-target={canAttach ? 'chat' : undefined}
  aria-label="Chat with {agentName}"
  onsubmit={(e) => {
    e.preventDefault();
    void send();
  }}
>
  {#if chat.queue.length > 0 && !closed}
    <QueuedMessages {chat} {agentName} composerEmpty={empty} onEdit={edit} />
  {/if}
  <div class="card" class:disabled={chat.phase === 'starting' || closed}>
    {#if attachments.length > 0}
      <AttachmentChips {attachments} onRemove={remove} />
    {/if}
    <textarea
      bind:this={input}
      bind:value={text}
      rows="2"
      {placeholder}
      aria-label="Message to {agentName}"
      aria-describedby={hintId}
      spellcheck="false"
      autocomplete="off"
      {...SAFARI_NO_AUTOCORRECT}
      autocapitalize="off"
      disabled={chat.phase === 'starting' || closed}
      onkeydown={onKeydown}
      onpaste={onPaste}
    ></textarea>
    <span id={hintId} class="sr-only"
      >{queues ? `Enter queues the message until ${agentName} finishes, Shift+Enter starts a new line.` : 'Enter sends, Shift+Enter starts a new line.'}</span
    >
    <div class="toolbar">
      {#if closed}
        <button type="button" class="text" onclick={() => chats.dismiss(chat.key)}>Dismiss</button>
        <span class="spacer"></span>
        {#if onRestart}
          <button type="button" class="text primary" title="Start {agentName} again for this session" onclick={onRestart}>Restart</button>
        {/if}
      {:else}
        {#if caps}
          <button type="button" class="icon" data-chat-attach title="Attach images or files" disabled={!canAttach || attaching} onclick={pick}
            ><Icon name="plus" size={17} /><span class="sr-only">Attach images or files</span></button
          >
        {/if}
        <div class="menu-container" bind:this={endMenuRoot}>
          <button
            type="button"
            class="icon"
            title="End the chat and stop {agentName}"
            aria-haspopup="menu"
            aria-expanded={endMenuOpen}
            disabled={chat.phase === 'starting'}
            onclick={() => (endMenuOpen = !endMenuOpen)}
            ><Icon name="power" size={15} /><span class="sr-only">End chat</span></button
          >
          {#if endMenuOpen}
            <div class="action-dropdown-menu" role="menu">
              <button type="button" class="action-menu-item" role="menuitem" onclick={() => { endMenuOpen = false; void endChat(); }}>End the chat</button>
              {#if onSwitchToTerminal}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title={canSwitch ? 'Stop the chat and continue the session in a terminal tab' : 'Available once the agent saves this session'}
                  disabled={!canSwitch}
                  onclick={() => { endMenuOpen = false; void switchToTerminal(); }}
                  >Switch to terminal</button
                >
              {/if}
            </div>
          {/if}
        </div>
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
        {#if queues && !empty}
          <button type="submit" class="round send" title="Queue the message until {agentName} finishes (Enter)" disabled={!canSend}
            ><Icon name="arrow-up" size={16} strokeWidth={2.5} /><span class="sr-only">Queue</span></button
          >
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

  .card:focus-within:not(.disabled),
  .chat-composer:global(.file-drop-target-active) .card {
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
    /* The session's text size, as the messages it answers. */
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-body));
    line-height: 1.5;
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

  /* The end menu, in SessionHeader's action-menu style. The composer sits
     at the window's bottom edge, so the menu opens upward. */
  .menu-container {
    position: relative;
    display: inline-flex;
  }

  .action-dropdown-menu {
    position: absolute;
    bottom: calc(100% + 4px);
    left: 0;
    z-index: 100;
    min-width: 150px;
    padding: 4px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    background: var(--bg-primary);
    box-shadow: var(--shadow-popover);
  }

  .action-menu-item {
    justify-content: flex-start;
    width: 100%;
    padding: 6px 10px;
    border-radius: 6px;
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .action-menu-item:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  .action-menu-item:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .action-menu-item:disabled:hover {
    background: transparent;
    color: var(--text-secondary);
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
