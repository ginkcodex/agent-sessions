<script lang="ts">
  import type { Message, SessionRef, SearchHitKind } from '../../types';
  import { formatAbsoluteTime, formatShortTime } from '../../date';
  import { preferences } from '../../stores/preferences.svelte';
  import { manage } from '../../stores/manage.svelte';
  import { toast } from '../../stores/toast.svelte';
  import { translateSettings } from '../../stores/translate.svelte';
  import { translations, translationKey } from '../../stores/translations.svelte';
  import { copyToClipboard } from '../../portable';
  import { copyFailed } from '../../guidance';
  import { appState } from '../../stores/appState.svelte';
  import { isSlashCommand, messageMarkdown, messageText, visibleTranscriptParts, withTranslation, type TranscriptMode } from '../../transcript';
  import { printMessage } from '../../print';
  import ContextMenu, { type ContextMenuItem } from '../common/ContextMenu.svelte';
  import Icon from '../common/Icon.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';
  import TextPart from './parts/TextPart.svelte';
  import ManualBashPart from './parts/ManualBashPart.svelte';
  import { manualBashMessage, manualBashMarkdown } from '../../manualBash';
  import ReasoningPart from './parts/ReasoningPart.svelte';
  import ToolPart from './parts/ToolPart.svelte';
  import type { ToolFileTarget } from '../../toolPresentation';
  import PatchPart from './parts/PatchPart.svelte';
  import CompactionPart from './parts/CompactionPart.svelte';
  import NoticePart from './parts/NoticePart.svelte';
  import ImagePart from './parts/ImagePart.svelte';
  import { isImageFile } from '../../images';
  import { captureSelection, holdSelection, selectionMenuItems, type SelectedText } from '../../selectionTranslation';
  import SelectionTranslation from './SelectionTranslation.svelte';

  interface Props {
    message: Message;
    sessionRef: SessionRef;
    /** Position in the transcript; keys the translation of a message without an id. */
    index?: number;
    mode?: TranscriptMode;
    forceVisible?: boolean;
    searchKind?: SearchHitKind | null;
    /** Indexes of parts holding a match, opened to show it. */
    searchParts?: number[];
    /** Drops the search result's exception, so the message follows the display level again. */
    onResetLevel?: () => void;
    onFileLink?: (value: string) => void;
    /** Opens the file a tool call reads or changes. */
    onOpenFile?: (target: ToolFileTarget) => void;
    /** Runs a shell block of the assistant's text in a terminal tab. */
    onRunCommand?: (command: string) => void;
  }

  let {
    message,
    sessionRef,
    index = 0,
    mode = 'activity',
    forceVisible = false,
    searchKind = null,
    searchParts = [],
    onResetLevel,
    onFileLink,
    onOpenFile,
    onRunCommand,
  }: Props = $props();

  // A short time beside the role; the full one, with its zone, on hover.
  let shortTime = $derived(message.time ? formatShortTime(message.time, preferences.timeZone) : '');
  let fullTime = $derived(message.time ? formatAbsoluteTime(message.time, preferences.timeZone) : '');
  let isSystem = $derived(message.role === 'system');
  let isUser = $derived(message.role === 'user');
  let isAssistant = $derived(message.role === 'assistant');
  let shownParts = $derived(visibleTranscriptParts(message, mode, forceVisible));
  let matchedParts = $derived(new Set(searchParts.map((i) => message.parts?.[i])));
  // A search result shows more than the display level would.
  let revealed = $derived(forceVisible && shownParts.length > visibleTranscriptParts(message, mode).length);

  // Copy and Translate work on what Chat level shows of user and assistant
  // messages; other messages keep the default context menu.
  let manualBash = $derived(manualBashMessage(message, sessionRef));
  let markdown = $derived(manualBash ? manualBashMarkdown(manualBash) : isUser || isAssistant ? messageMarkdown(message) : '');
  let slashCommand = $derived(isUser && !manualBash && isSlashCommand(messageText(message)));
  // Commands/output are code, not prose to translate.
  let text = $derived(!manualBash && !slashCommand && markdown ? messageText(message) : '');
  let key = $derived(translationKey(sessionRef, message, index, translateSettings.language));
  let translation = $derived(translations.get(key));
  let translated = $derived(translation?.status === 'done' && !translation.showOriginal);

  // A translation replaces the text parts with one, where the first was.
  let visibleParts = $derived(translated && translation ? withTranslation(shownParts, translation.text) : shownParts);

  let body: HTMLDivElement | undefined = $state();
  let menu = $state<{ x: number; y: number; selection?: SelectedText } | null>(null);
  let snippet = $state<SelectedText | null>(null);

  $effect(() => {
    void message;
    void sessionRef;
    void translateSettings.language;
    snippet = null;
    menu = null;
  });

  // The menu and then its popover take focus, which clears the selection.
  // Menu → popover is the same range, so the hold continues across it.
  let held = $derived(menu?.selection?.range ?? snippet?.range);
  $effect(() => {
    const range = held;
    return range ? holdSelection(range) : undefined;
  });

  function openMenu(e: MouseEvent) {
    snippet = null;
    menu = null;
    const selection = window.getSelection();
    if (selection && !selection.isCollapsed) {
      if (!body?.contains(e.target as Node)) return;
      const captured = captureSelection(body, selection);
      if (!captured) return;
      e.preventDefault();
      menu = { x: e.clientX, y: e.clientY, selection: captured };
      return;
    }
    if (!markdown) return;
    e.preventDefault();
    menu = { x: e.clientX, y: e.clientY };
  }

  async function copySelection(text: string) {
    try {
      if (!(await copyToClipboard(text))) throw new Error('The clipboard is not available.');
      toast.show({ title: 'Selection copied', body: 'Copied the selected text.' });
    } catch (err) {
      toast.show(copyFailed('the selection', err));
    }
  }

  async function copyMessage() {
    try {
      if (!(await copyToClipboard(markdown))) {
        toast.show(copyFailed('the message', 'The clipboard is not available.'));
        return;
      }
      toast.show({ title: 'Message copied', body: 'Copied as Markdown, as Chat level shows it.' });
    } catch (err) {
      toast.show(copyFailed('the message', err));
    }
  }

  // Opens what Chat level shows, in the language shown, for editing and printing.
  function print() {
    const parts = visibleTranscriptParts(message, 'chat');
    printMessage({
      sessionRef,
      title: appState.selectedSessionMeta?.title ?? '',
      parts: manualBash ? [{ kind: 'text', text: markdown }] : translated && translation ? withTranslation(parts, translation.text) : parts,
    });
  }

  function translate() {
    void translations.translate(key, text);
  }

  let menuItems = $derived.by((): ContextMenuItem[] => {
    const selected = menu?.selection;
    if (selected) {
      return selectionMenuItems(translateSettings.configured, translateSettings.language,
        () => void copySelection(selected.text),
        () => (snippet = selected),
        () => manage.openSettings('translation'));
    }
    const items: ContextMenuItem[] = [
      { label: 'Copy', onSelect: () => void copyMessage() },
    ];
    if (!translateSettings.configured) {
      items.push(
        { label: 'Translate', onSelect: () => {}, disabled: true, title: 'Set up translation in Settings' },
        { label: 'Set up translation…', onSelect: () => manage.openSettings('translation') },
      );
    } else if (!text) {
      items.push({ label: 'Translate', onSelect: () => {}, disabled: true, title: 'This message has no text to translate' });
    } else if (translation?.status === 'done') {
      items.push(
        { label: translation.showOriginal ? 'Show translation' : 'Show original', onSelect: () => translations.toggleOriginal(key) },
        { label: 'Retranslate', onSelect: translate },
      );
    } else if (translation?.status === 'loading') {
      items.push({ label: 'Translating…', onSelect: () => {}, disabled: true });
    } else {
      items.push({ label: `Translate to ${translateSettings.language}`, onSelect: translate });
    }
    items.push({ label: 'Print / Save as PDF…', onSelect: print });
    return items;
  });
</script>

{#if visibleParts.length > 0}
  <!-- The message actions menu opens on right-click or the context menu key. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="message-bubble-wrapper"
    class:role-user={isUser}
    class:role-assistant={isAssistant}
    class:role-system={isSystem}
    class:is-meta={message.isMeta}
    class:manual-bash={!!manualBash}
    class:slash-command={slashCommand}
    oncontextmenu={openMenu}
  >
    <!-- The role, model and time sit above the message rather than in a card
         header: user messages are a bubble on the right, the agent's reply
         is open text under its mark. -->
    <div class="message-meta-header">
      <div class="meta-left">
        {#if isUser}
          <span class="role-icon user"><Icon name="user" size={12} /></span>
          <span class="role-label">You</span>
          {#if slashCommand}
            <span class="command-tag" title="A slash command for the agent">cmd</span>
          {/if}
        {:else if isAssistant}
          <span class="role-icon assistant"><AgentIcon agent={sessionRef?.agent} size={11} /></span>
          <span class="role-label">Assistant</span>
          {#if message.model}
            <span class="model-tag">{message.model}</span>
          {/if}
        {:else}
          <span class="role-icon system"><Icon name="settings" size={12} /></span>
          <span class="role-label">System</span>
        {/if}
        {#if message.isSidechain}
          <span class="sidechain-tag">sidechain</span>
        {/if}
        {#if translation?.status === 'loading'}
          <span class="translation-tag" role="status">Translating…</span>
        {:else if translation?.status === 'done'}
          <span class="translation-tag">
            {translated ? translateSettings.language : 'Original'} ·
            <button type="button" class="translation-toggle" onclick={() => translations.toggleOriginal(key)}>
              {translated ? 'Show original' : 'Show translation'}
            </button>
          </span>
        {:else if translation?.status === 'error'}
          <span class="translation-tag failed" title={translation.error}>
            Translation failed ·
            <button type="button" class="translation-toggle" onclick={translate}>Retry</button>
          </span>
        {/if}
      </div>

      {#if shortTime || (revealed && onResetLevel)}
        <div class="meta-right">
          {#if shortTime}
            <time class="time-label" datetime={message.time} title={fullTime}>{shortTime}</time>
          {/if}
          {#if revealed && onResetLevel}
            <button
              type="button"
              class="reset-level"
              aria-label="Show this message at the current display level"
              title="Shown in full for this search result. Click to show it at the current display level."
              onclick={onResetLevel}
            >↻</button>
          {/if}
        </div>
      {/if}
    </div>

    <div class="message-card">
      <div class="message-parts-body" bind:this={body}>
        {#if manualBash}
          <ManualBashPart record={manualBash} defaultExpanded={forceVisible} />
        {:else}
          {#each visibleParts as part, idx (idx)}
            {#if part.kind === 'text' && part.text}
              <TextPart text={part.text} {onFileLink} onRunCommand={isAssistant ? onRunCommand : undefined} />
            {:else if part.kind === 'reasoning' && part.text}
              <ReasoningPart text={part.text} {onFileLink} defaultExpanded={searchKind === 'reasoning' || matchedParts.has(part)} />
            {:else if part.kind === 'tool' && part.tool}
              <ToolPart tool={part.tool} {sessionRef} {onOpenFile} defaultExpanded={searchKind === 'tool' || matchedParts.has(part)} />
            {:else if part.kind === 'patch'}
              <PatchPart files={part.files} text={part.text} />
            {:else if part.kind === 'compaction'}
              <CompactionPart text={part.text} />
            {:else if part.kind === 'notice' && part.text}
              <NoticePart text={part.text} />
            {:else if part.kind === 'file' && part.file && isImageFile(part.file)}
              <ImagePart file={part.file} {sessionRef} />
            {:else if part.kind === 'file' && part.file}
              <div class="file-attachment-pill">
                <Icon name="paperclip" size={13} />
                <span>{part.file.name || part.file.ref || 'Attachment'}</span>
              </div>
            {/if}
          {/each}
        {/if}
      </div>
    </div>
  </div>
  {#if menu}
    <ContextMenu x={menu.x} y={menu.y} items={menuItems} ariaLabel={menu.selection ? 'Selection actions' : 'Message actions'} onClose={() => (menu = null)} />
  {/if}
  {#if snippet}
    <SelectionTranslation selection={snippet} language={translateSettings.language} onClose={() => (snippet = null)} />
  {/if}
{/if}

<style>
  .message-bubble-wrapper {
    /* The avatar's width plus its gap: replies line up with the role label. */
    --gutter: 28px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 22px;
    width: 100%;
  }

  /* The user's message: a soft bubble on the right, its label above it. */
  .message-bubble-wrapper.role-user {
    align-items: flex-end;
  }

  .role-user .message-meta-header {
    flex-direction: row-reverse;
  }

  .role-user .meta-left {
    flex-direction: row-reverse;
  }

  .role-user .message-card {
    max-width: min(85%, 720px);
    background-color: var(--bg-secondary);
    border-radius: 16px 4px 16px 16px;
  }

  .role-user .message-parts-body {
    padding: 10px 14px;
  }

  /* Local shell activity gets a full-width terminal card, not a prose bubble. */
  .role-user.manual-bash .message-card {
    width: calc(100% - var(--gutter));
    max-width: none;
    background: transparent;
    border-radius: 0;
  }

  .role-user.manual-bash .message-parts-body {
    padding: 0;
  }

  /* The agent's reply: open text under its mark, no card. */
  .role-assistant .message-card,
  .role-system .message-card {
    margin-left: var(--gutter);
  }

  /* Open text sits on the pane's own surface, so code blocks take a tone. */
  .role-assistant,
  .role-system {
    --code-bg: var(--bg-secondary);
  }

  .role-system .message-card {
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-md);
  }

  .role-system .message-parts-body {
    padding: 8px 12px;
  }

  .message-card {
    min-width: 0;
    border-radius: var(--radius-md);
  }

  .message-meta-header {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px 10px;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    color: var(--text-muted);
  }

  .meta-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  /* A small round avatar holding the role's icon. */
  .role-icon {
    display: inline-grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background-color: var(--bg-tertiary);
    color: var(--text-secondary);
  }

  .role-icon.user {
    background-color: var(--active-bg);
    color: var(--accent-color);
  }

  .role-label {
    font-weight: 600;
    color: var(--text-primary);
  }

  .model-tag {
    font-family: var(--font-mono);
    color: var(--text-muted);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
  }

  .command-tag {
    padding: 0 5px;
    border: 1px solid color-mix(in srgb, var(--accent-color) 45%, transparent);
    border-radius: 6px;
    background: color-mix(in srgb, var(--accent-color) 12%, transparent);
    font-family: var(--font-mono);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    font-weight: 600;
    text-transform: uppercase;
    color: var(--accent-color);
  }

  .role-user.slash-command .message-parts-body {
    font-family: var(--font-mono);
  }

  .sidechain-tag {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    color: #a855f7;
  }

  .translation-tag {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    color: var(--text-muted);
  }

  .translation-tag.failed {
    color: var(--danger, #ef4444);
  }

  .translation-toggle {
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    color: var(--accent-color);
    cursor: pointer;
  }

  .translation-toggle:hover {
    text-decoration: underline;
  }

  .meta-right {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  /* An icon glyph in a fixed 20px button, so it keeps its size: not part
     of the type scale (docs/plan/font-settings.md). */
  .reset-level {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 14px;
    line-height: 1;
    cursor: pointer;
  }

  .reset-level:hover {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  /* Paragraph margins inside the body don't push it away from its label. */
  .message-parts-body > :global(:first-child) {
    margin-top: 0;
  }

  .message-parts-body > :global(:last-child) {
    margin-bottom: 0;
  }

  .file-attachment-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border-radius: var(--radius-sm);
    background: var(--segment-track);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    font-family: var(--font-mono);
    margin: 4px 0;
  }
</style>
