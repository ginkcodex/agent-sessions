<script lang="ts">
  import type { AgentID, SessionMeta, TerminalOpenMode } from '../../types';
  import { formatTokens, formatCost } from '../../format';
  import { formatAbsoluteTime, formatAgo, isKnownTime } from '../../date';
  import { appState } from '../../stores/appState.svelte';
  import { handoff } from '../../stores/handoff.svelte';
  import { exporter } from '../../stores/export.svelte';
  import { ALL_AGENTS, copyToClipboard } from '../../portable';
  import { agentSettings } from '../../stores/agentSettings.svelte';
  import { copyFailed } from '../../guidance';
  import { toast } from '../../stores/toast.svelte';
  import AgentIcon from '../common/AgentIcon.svelte';
  import Icon from '../common/Icon.svelte';
  import NavButtons from './NavButtons.svelte';
  import { link } from '../../stores/link.svelte';
  import { launcher } from '../../stores/launcher.svelte';
  import { preferences } from '../../stores/preferences.svelte';
  import { resumeButtonTitle } from '../../link';
  import { transcriptModes, type TranscriptMode } from '../../transcript';
  import { newSession } from '../../stores/newSession.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { newSessionShortcutLabel } from '../../terminal';
  import { chats } from '../../stores/chats.svelte';
  import { chatOpen } from '../../stores/chatOpen.svelte';
  import { manage } from '../../stores/manage.svelte';

  interface Props {
    meta: SessionMeta;
    mode: TranscriptMode;
    onModeChange: (mode: TranscriptMode) => void;
    /** Copies the resume command. */
    onResume: () => void;
    /**
     * Resumes in a terminal: where agents open by default, or the other
     * place when where is given. Offered where launcher can open it.
     */
    onOpenResume?: (where?: TerminalOpenMode) => void;
    resumeCopied?: boolean;
    resumeOpening?: boolean;
    /** Shows or hides the find bar; without it there is no Find button. */
    onFind?: () => void;
    findOpen?: boolean;
    /** Prepares the full Chat-level conversation in the shared print editor. */
    onPrint?: () => void;
    printPreparing?: boolean;
  }

  let {
    meta,
    mode,
    onModeChange,
    onResume,
    onOpenResume = () => {},
    resumeCopied = false,
    resumeOpening = false,
    onFind,
    findOpen = false,
    onPrint,
    printPreparing = false,
  }: Props = $props();

  // The agent's name, for the icon badge's tooltip and screen readers.
  let agentName = $derived(ALL_AGENTS.find((a) => a.id === meta.ref.agent)?.label ?? meta.ref.agent);

  let messageTotal = $derived(meta.counts.user + meta.counts.assistant);
  // The ▾ menu offers the place agents do not open by default.
  let otherOpen = $derived<TerminalOpenMode>(launcher.openMode === 'panel' ? 'external' : 'panel');
  let canOpenOther = $derived(Boolean(otherOpen === 'panel' ? launcher.canOpenPanel : launcher.canOpenExternal));
  const OPEN_IN: Record<TerminalOpenMode, { label: string; title: string }> = {
    panel: { label: 'Open in built-in terminal', title: 'Resume this session in the terminal panel below' },
    external: { label: 'Open in external terminal', title: 'Resume this session in a new terminal window' },
  };
  // A panel tab already runs this session: Resume shows it rather than
  // starting a second copy, which the ▾ menu still offers.
  let runningTab = $derived(terminals.tabFor(meta.ref, link.dataHost ?? ''));

  // New session: the main part starts this session's agent in its directory,
  // ▾ offers every agent, those the host lacks disabled with what to install.
  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);
  let newChoices = $derived(newSession.choices);
  let sameAgent = $derived(newChoices.find((c) => c.agent === meta.ref.agent));
  let newDisabledReason = $derived(meta.cwdMissing ? 'This directory no longer exists on disk' : '');
  let newMainTitle = $derived(
    newDisabledReason ||
      sameAgent?.missing ||
      `Start a new ${agentName} session in ${meta.cwd} (${newSessionShortcutLabel(isMac)} starts the agent last used)`,
  );
  let newMenuOpen = $state(false);
  let newMenuRoot: HTMLDivElement | undefined = $state();

  $effect(() => {
    void link.dataHost;
    if (newSession.available) void newSession.refresh();
    chatOpen.refresh();
  });

  // A chat drives this session: the terminal does not resume it meanwhile.
  let liveChat = $derived(chats.forSession(meta.ref));
  let canChatHere = $derived(chatOpen.canChat(meta.ref.agent) && !meta.cwdMissing);
  // New chats: every agent whose ACP adapter the host has.
  let chatChoices = $derived(ALL_AGENTS.filter((a) => chatOpen.canChat(a.id)));
  // Agents the host could chat with once their adapter is installed.
  let chatMissing = $derived(chatOpen.available && chats.agents.some((a) => !a.available));

  function startChat(agent: AgentID) {
    newMenuOpen = false;
    void chatOpen.startNew(agent, meta.cwd);
  }

  function startNew(agent: AgentID) {
    newMenuOpen = false;
    void newSession.start(agent, meta.cwd);
  }

  let resumeMenuOpen = $state(false);
  // Continue in and Export share one ⋯ menu, so Resume stays the one
  // strong action in the toolbar.
  let moreMenuOpen = $state(false);
  let resumeMenuRoot: HTMLDivElement | undefined = $state();
  let moreMenuRoot: HTMLDivElement | undefined = $state();

  // The copy button shows a tick for a moment after copying the path.
  let cwdCopied = $state(false);
  let cwdCopiedTimer: ReturnType<typeof setTimeout> | undefined;

  async function copyCwd() {
    try {
      if (!(await copyToClipboard(meta.cwd))) {
        toast.show(copyFailed('the path', 'The clipboard is not available.'));
        return;
      }
      cwdCopied = true;
      clearTimeout(cwdCopiedTimer);
      cwdCopiedTimer = setTimeout(() => (cwdCopied = false), 1500);
    } catch (err) {
      toast.show(copyFailed('the path', err));
    }
  }

  $effect(() => () => clearTimeout(cwdCopiedTimer));

  function handleNavigateToParent() {
    if (meta.parentId) {
      appState.selectSession({ agent: meta.ref.agent, id: meta.parentId }, 'push');
    }
  }

  function handleWindowPointerDown(e: PointerEvent) {
    const target = e.target as Node;
    if (resumeMenuOpen && resumeMenuRoot && !resumeMenuRoot.contains(target)) {
      resumeMenuOpen = false;
    }
    if (moreMenuOpen && moreMenuRoot && !moreMenuRoot.contains(target)) {
      moreMenuOpen = false;
    }
    if (newMenuOpen && newMenuRoot && !newMenuRoot.contains(target)) {
      newMenuOpen = false;
    }
  }
</script>

<svelte:window onpointerdown={handleWindowPointerDown} />

<header class="session-header">
  {#if meta.parentId}
    <div class="parent-banner">
      <span>Subagent session of parent:</span>
      <button
        type="button"
        class="parent-link-btn"
        onclick={handleNavigateToParent}
      >
        {meta.parentId} → View Parent
      </button>
    </div>
  {/if}

  <div class="header-top-row">
    <div class="header-badges">
      <NavButtons />
      <span
        class="agent-badge agent-{meta.ref.agent}"
        role="img"
        aria-label={agentName}
        title={agentName}
      >
        <AgentIcon agent={meta.ref.agent} size={14} />
      </span>
      {#if meta.live}
        <span class="live-badge"><span class="live-pulse" aria-hidden="true"></span>Live</span>
      {/if}
      {#if meta.archived}
        <span class="archived-badge">Archived</span>
      {/if}
      {#if meta.model}
        <span class="model-badge">{meta.model}</span>
      {/if}
      {#if meta.agentVersion}
        <span class="version-badge">v{meta.agentVersion}</span>
      {/if}
    </div>

    <div class="header-actions">
      {#if liveChat}
        <span class="running-badge" title="This session is running in a chat below">● Live in chat</span>
      {:else if runningTab}
        <span class="running-badge" title="This session is running in the terminal panel">● Running</span>
      {/if}
      {#if launcher.canOpen || canOpenOther}
        <div class="menu-container resume-split" bind:this={resumeMenuRoot}>
          {#if runningTab}
            {@const tabId = runningTab.id}
            <button
              type="button"
              class="action-btn resume-btn resume-main"
              title="Show the terminal tab running this session"
              onclick={() => terminals.show(tabId)}
            >
              Show terminal
            </button>
          {:else}
            <button
              type="button"
              class="action-btn resume-btn resume-main"
              title={launcher.canOpen ? resumeButtonTitle(link.dataHost, true, launcher.openMode) : resumeButtonTitle(link.dataHost)}
              disabled={resumeOpening}
              onclick={() => (launcher.canOpen ? onOpenResume() : onResume())}
            >
              {resumeCopied ? '✓ Copied' : resumeOpening ? 'Opening…' : 'Resume'}
            </button>
          {/if}
          <button
            type="button"
            class="action-btn resume-btn resume-toggle"
            title="More ways to resume"
            aria-label="More ways to resume"
            aria-haspopup="menu"
            aria-expanded={resumeMenuOpen}
            onclick={() => {
              resumeMenuOpen = !resumeMenuOpen;
              if (resumeMenuOpen) {
                moreMenuOpen = false;
                newMenuOpen = false;
                chatOpen.refresh();
              }
            }}
          >
            <Icon name="chevron-down" size={13} strokeWidth={2.25} />
          </button>
          {#if resumeMenuOpen}
            <div class="action-dropdown-menu" role="menu">
              {#if runningTab && launcher.openMode === 'panel' && launcher.canOpen}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title="Resume this session again in another terminal tab"
                  disabled={resumeOpening}
                  onclick={() => {
                    resumeMenuOpen = false;
                    onOpenResume();
                  }}
                >
                  <span>Resume in a new tab</span>
                </button>
              {/if}
              {#if canOpenOther}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title={OPEN_IN[otherOpen].title}
                  onclick={() => {
                    resumeMenuOpen = false;
                    onOpenResume(otherOpen);
                  }}
                >
                  <span>{OPEN_IN[otherOpen].label}</span>
                </button>
              {/if}
              {#if launcher.canOpen}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title="Copy the resume command to run it yourself"
                  onclick={() => {
                    resumeMenuOpen = false;
                    onResume();
                  }}
                >
                  <span>Copy command</span>
                </button>
              {/if}
              {#if canChatHere && !liveChat}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title="Continue this session in a chat in this window, which shows the agent's reply as it streams"
                  disabled={chatOpen.opening}
                  onclick={() => {
                    resumeMenuOpen = false;
                    void chatOpen.openSession(meta);
                  }}
                >
                  <Icon name="message-square" size={14} />
                  <span>Open in chat</span>
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {:else}
        <button
          type="button"
          class="action-btn resume-btn"
          title={resumeButtonTitle(link.dataHost)}
          onclick={onResume}
        >
          {resumeCopied ? '✓ Copied' : 'Resume'}
        </button>
      {/if}

      {#if newSession.available && meta.cwd}
        <div class="menu-container new-split" bind:this={newMenuRoot}>
          <button
            type="button"
            class="action-btn new-btn new-main"
            title={newMainTitle}
            disabled={newSession.starting || Boolean(newDisabledReason || sameAgent?.missing)}
            onclick={() => startNew(meta.ref.agent)}
          >
            {newSession.starting ? 'Starting…' : 'New'}
          </button>
          <button
            type="button"
            class="action-btn new-btn new-toggle"
            title="Start a new session with another agent"
            aria-label="New session with another agent"
            aria-haspopup="menu"
            aria-expanded={newMenuOpen}
            disabled={Boolean(newDisabledReason)}
            onclick={() => {
              newMenuOpen = !newMenuOpen;
              if (newMenuOpen) {
                resumeMenuOpen = false;
                moreMenuOpen = false;
                void newSession.refresh();
                chatOpen.refresh();
              }
            }}
          >
            <Icon name="chevron-down" size={13} strokeWidth={2.25} />
          </button>
          {#if newMenuOpen}
            <div class="action-dropdown-menu" role="menu">
              {#each newChoices as choice (choice.agent)}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title={choice.missing || `Start a new ${choice.name} session in ${meta.cwd}`}
                  disabled={Boolean(choice.missing) || newSession.starting}
                  onclick={() => startNew(choice.agent)}
                >
                  <AgentIcon agent={choice.agent} size={14} />
                  <span>New {choice.name} session</span>
                  {#if choice.missing}
                    <span class="menu-note">not installed</span>
                  {/if}
                </button>
              {/each}
              {#if chatChoices.length > 0}
                <div class="menu-heading" role="presentation">Chat</div>
                {#each chatChoices as choice (choice.id)}
                  <button
                    type="button"
                    class="action-menu-item"
                    role="menuitem"
                    title="Start a new {choice.label} session in a chat in this window, in {meta.cwd}"
                    disabled={chatOpen.opening}
                    onclick={() => startChat(choice.id)}
                  >
                    <AgentIcon agent={choice.id} size={14} />
                    <span>New {choice.label} chat</span>
                  </button>
                {/each}
              {/if}
              {#if chatMissing}
                {#if chatChoices.length === 0}
                  <div class="menu-heading" role="presentation">Chat</div>
                {/if}
                <button
                  type="button"
                  class="action-menu-item"
                  role="menuitem"
                  title="Some agents need their ACP adapter to chat: see how to install it"
                  onclick={() => {
                    newMenuOpen = false;
                    manage.openSettings('agents');
                  }}
                >
                  <Icon name="settings" size={14} />
                  <span>Set up chat…</span>
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      {#if onFind}
        <button
          type="button"
          class="action-btn find-btn icon-btn"
          class:active={findOpen}
          aria-pressed={findOpen}
          aria-label="Find in session"
          title="Find in session ({isMac ? '⌘F' : 'Ctrl+F'})"
          onclick={onFind}
        >
          <span class="find-icon" aria-hidden="true"></span>
        </button>
      {/if}

      <div class="menu-container" bind:this={moreMenuRoot}>
        <button
          type="button"
          class="action-btn icon-btn more-btn"
          class:active={moreMenuOpen}
          title={printPreparing ? 'Preparing conversation for printing…' : 'Continue in another agent, export, or print'}
          aria-busy={printPreparing}
          aria-label="More actions"
          aria-haspopup="menu"
          aria-expanded={moreMenuOpen}
          onclick={() => {
            moreMenuOpen = !moreMenuOpen;
            if (moreMenuOpen) {
              resumeMenuOpen = false;
              newMenuOpen = false;
            }
          }}
        >
          <Icon name="ellipsis" size={16} />
        </button>
        {#if moreMenuOpen}
          <div class="action-dropdown-menu more-menu" role="menu">
            <div class="menu-heading" role="presentation">Continue in</div>
            {#each agentSettings.activeAgents as agent (agent.id)}
              <button
                type="button"
                class="action-menu-item"
                role="menuitem"
                title="Continue this session in {agent.label}"
                onclick={() => {
                  moreMenuOpen = false;
                  handoff.open(meta, agent.id);
                }}
              >
                <AgentIcon agent={agent.id} size={14} />
                <span>{agent.label}</span>
              </button>
            {/each}
            <div class="menu-separator" role="separator"></div>
            {#if onPrint}
              <button
                type="button"
                class="action-menu-item"
                role="menuitem"
                title="Print the entire conversation at Chat level, including messages not currently shown"
                disabled={printPreparing || messageTotal === 0 || link.locked}
                onclick={() => {
                  moreMenuOpen = false;
                  onPrint?.();
                }}
              >
                <Icon name="file-text" size={14} />
                <span>{printPreparing ? 'Preparing conversation…' : 'Print conversation to PDF…'}</span>
              </button>
              <div class="menu-separator" role="separator"></div>
            {/if}
            <div class="menu-heading" role="presentation">Export</div>
            <button
              type="button"
              class="action-menu-item"
              role="menuitem"
              title="Export a bundle that can restore this session"
              onclick={() => {
                moreMenuOpen = false;
                exporter.open(meta, 'complete');
              }}
            >
              <Icon name="package" size={14} />
              <span>Complete (Restorable)…</span>
            </button>
            <button
              type="button"
              class="action-menu-item"
              role="menuitem"
              title="Export a redacted bundle that is safe to share"
              onclick={() => {
                moreMenuOpen = false;
                exporter.open(meta, 'share-safe');
              }}
            >
              <Icon name="shield-check" size={14} />
              <span>Share-safe (Redacted)…</span>
            </button>
          </div>
        {/if}
      </div>

      <div class="mode-switch" role="group" aria-label="Transcript display">
        {#each transcriptModes as option (option.value)}
          <button
            type="button"
            class="action-btn mode-btn"
            class:active={mode === option.value}
            aria-pressed={mode === option.value}
            title={option.title}
            onclick={() => onModeChange(option.value)}
          >
            {option.label}
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="header-title-row">
    <h2 class="title-text" title={meta.title || meta.ref.id}>
      {meta.title || meta.firstPrompt || meta.ref.id}
    </h2>
  </div>

  <div class="header-meta-row">
    <div class="meta-item cwd-item" title={meta.cwd}>
      <Icon name="folder" size={13} class="meta-icon" />
      <span class="cwd-text">{meta.cwd}</span>
      {#if meta.cwdMissing}
        <span class="missing-badge">[missing]</span>
      {/if}
      {#if meta.cwd}
        <button
          type="button"
          class="cwd-copy-btn"
          class:copied={cwdCopied}
          title={cwdCopied ? 'Copied' : 'Copy path'}
          aria-label={cwdCopied ? 'Path copied' : 'Copy path'}
          onclick={() => void copyCwd()}
        >
          {#if cwdCopied}
            <svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true">
              <path d="M3 8.5 6.5 12 13 4.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          {:else}
            <svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true">
              <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.4" />
              <path d="M10.5 3.5V3A1.5 1.5 0 0 0 9 1.5H3A1.5 1.5 0 0 0 1.5 3v6A1.5 1.5 0 0 0 3 10.5h.5" fill="none" stroke="currentColor" stroke-width="1.4" />
            </svg>
          {/if}
        </button>
      {/if}
    </div>

    {#if meta.gitBranch}
      <div class="meta-item branch-item" title="Git Branch: {meta.gitBranch}">
        <Icon name="git-branch" size={13} class="meta-icon" />
        <span class="branch-text">{meta.gitBranch}</span>
      </div>
    {/if}

    <div class="meta-item count-item">
      <Icon name="message-square" size={13} class="meta-icon" />
      <span class="count-val">{messageTotal} msgs</span>
    </div>

    <!-- Token use, on the path's line; it wraps below when space runs out. -->
    <div class="stats-group" aria-label="Token use">
      <span class="stat-pill" title="Input tokens">
        <span class="stat-val">{formatTokens(meta.tokens.input)}</span>
        <span class="stat-label">in</span>
      </span>
      <span class="stat-pill" title="Output tokens">
        <span class="stat-val">{formatTokens(meta.tokens.output)}</span>
        <span class="stat-label">out</span>
      </span>
      {#if meta.tokens.cacheRead && meta.tokens.cacheRead > 0}
        <span class="stat-pill" title="Tokens read from the prompt cache">
          <span class="stat-val">{formatTokens(meta.tokens.cacheRead)}</span>
          <span class="stat-label">cached</span>
        </span>
      {/if}
      {#if meta.costUsd && meta.costUsd > 0}
        <span class="stat-pill cost" title="Estimated cost">
          <span class="stat-val">{formatCost(meta.costUsd)}</span>
        </span>
      {/if}
    </div>
  </div>

  <div class="header-time-row">
    {#each [{ label: 'Created', value: meta.createdAt }, { label: 'Updated', value: meta.updatedAt }] as stamp (stamp.label)}
      {#if isKnownTime(stamp.value)}
        <div class="meta-item time-item" title="{stamp.label} {formatAgo(stamp.value)}">
          <span class="stat-label">{stamp.label}:</span>
          <span class="time-val">{formatAbsoluteTime(stamp.value, preferences.timeZone)}</span>
        </div>
      {/if}
    {/each}
  </div>
</header>

<style>
  .session-header {
    container-type: inline-size;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 16px;
    background-color: var(--bg-secondary);
  }

  .parent-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    border-radius: 4px;
    background: rgba(168, 85, 247, 0.1);
    border: 1px solid rgba(168, 85, 247, 0.25);
    font-size: 0.72rem;
    color: #a855f7;
  }

  .parent-link-btn {
    font-family: var(--font-mono);
    font-size: inherit;
    font-weight: 600;
    background: none;
    border: none;
    color: #a855f7;
    cursor: pointer;
    text-decoration: underline;
  }

  .header-top-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    row-gap: 8px;
  }

  .header-badges {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  /* Keep individual painted surfaces composited on WebKitGTK (SS-10,
     user-confirmed). A shared group layer did not stop rapid-scroll flashing. */
  :global([data-platform="linux"]) .agent-badge,
  :global([data-platform="linux"]) .live-badge,
  :global([data-platform="linux"]) .archived-badge,
  :global([data-platform="linux"]) .model-badge,
  :global([data-platform="linux"]) .version-badge {
    transform: translateZ(0);
  }

  .agent-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 20px;
    border-radius: 4px;
  }

  .agent-badge.agent-claude-code {
    background: rgba(217, 119, 87, 0.15);
    color: var(--agent-claude);
  }

  .agent-badge.agent-codex {
    background: rgba(16, 163, 127, 0.15);
    color: var(--agent-codex);
  }

  .agent-badge.agent-opencode {
    background: rgba(59, 130, 246, 0.15);
    color: var(--agent-opencode);
  }

  /* Live: a soft green pill whose dot breathes. */
  .live-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 1px 8px 1px 7px;
    border-radius: 999px;
    font-size: 0.66rem;
    font-weight: 600;
    color: var(--status-live);
    background-color: color-mix(in srgb, var(--status-live) 13%, transparent);
  }

  .live-pulse {
    position: relative;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background-color: currentColor;
  }

  /* Live means an open process, including one waiting for input. Keep its
     presence indicator static rather than compositing an idle animation. */

  .archived-badge {
    padding: 1px 7px;
    border-radius: 999px;
    font-size: 0.66rem;
    font-weight: 500;
    color: var(--text-muted);
    background-color: var(--segment-track);
  }

  .model-badge, .version-badge {
    font-size: 0.7rem;
    font-family: var(--font-mono);
    color: var(--text-secondary);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 auto;
    justify-content: flex-end;
    flex-wrap: wrap;
  }

  @container (max-width: 460px) {
    .header-actions {
      flex-basis: 100%;
      justify-content: flex-start;
    }
  }

  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 3px 8px;
    border-radius: var(--radius-sm, 6px);
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    background-color: transparent;
    color: var(--text-secondary);
    border: none;
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }

  .action-btn:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .action-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 1px;
  }

  /* The one filled button in the toolbar. */
  .action-btn.resume-btn {
    background-color: var(--accent-color);
    color: white;
    box-shadow: 0 1px 2px color-mix(in srgb, var(--accent-color) 35%, transparent);
  }

  .action-btn.resume-btn:hover {
    background-color: var(--accent-hover);
    color: white;
  }

  .action-btn.resume-btn:disabled {
    opacity: 0.7;
    cursor: progress;
  }

  .resume-split .resume-main {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }

  .resume-split .resume-toggle {
    padding: 3px 6px;
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    border-left: 1px solid rgba(255, 255, 255, 0.35);
  }

  .running-badge {
    align-self: center;
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--status-live);
    white-space: nowrap;
  }

  /* New session: a tonal button, so Resume stays the main action. */
  .action-btn.new-btn {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  .action-btn.new-btn:hover:not(:disabled) {
    background-color: var(--bg-tertiary);
  }

  /* Keep filled action surfaces in their own layers on WebKitGTK when the
     adjacent composited list rapidly repaints under the pointer (SS-08).
     Only transform the buttons: menus must remain outside these layers. */
  :global([data-platform="linux"]) .action-btn.resume-btn,
  :global([data-platform="linux"]) .action-btn.new-btn {
    transform: translateZ(0);
  }

  .action-btn.new-btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .new-split .new-main {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }

  .new-split .new-toggle {
    padding: 3px 6px;
    border-left: 1px solid var(--border-color);
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
  }

  .action-menu-item:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .action-menu-item:disabled:hover {
    background: transparent;
    color: var(--text-secondary);
  }

  .menu-note {
    margin-left: auto;
    padding-left: 8px;
    font-size: 0.68rem;
    color: var(--text-muted);
  }

  /* A segmented control: a filled track with the chosen level raised on
     it, so the levels separate by tone instead of outlines. */
  .mode-switch {
    display: inline-flex;
    gap: 2px;
    padding: 2px;
    border-radius: var(--radius-md);
    background-color: var(--segment-track);
  }

  .mode-switch .mode-btn {
    padding: 2px 10px;
    border-radius: calc(var(--radius-md) - 2px);
    transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
  }

  .mode-switch .mode-btn:hover {
    background-color: var(--hover-bg);
  }

  .mode-switch .mode-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: -2px;
  }

  .action-btn.mode-btn.active,
  .action-btn.mode-btn.active:hover {
    background-color: var(--segment-thumb);
    color: var(--text-primary);
    box-shadow: var(--shadow-thumb);
  }

  /* Square icon buttons: Find and the ⋯ menu. */
  .action-btn.icon-btn {
    justify-content: center;
    width: 26px;
    height: 24px;
    padding: 0;
  }

  .action-btn.find-btn.active,
  .action-btn.find-btn.active:hover {
    background-color: var(--active-bg);
    color: var(--accent-color);
  }

  .action-btn.more-btn.active {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  .find-icon {
    width: 14px;
    height: 14px;
    background-color: currentColor;
    -webkit-mask: url('../../../../assets/find.svg') center / contain no-repeat;
    mask: url('../../../../assets/find.svg') center / contain no-repeat;
  }

  .menu-container {
    position: relative;
    display: inline-flex;
  }

  .action-dropdown-menu {
    animation: -global-pop-in var(--dur-fast) var(--ease-out);
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 100;
    min-width: 140px;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-popover);
    padding: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .action-menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border: none;
    background: transparent;
    border-radius: var(--radius-sm, 4px);
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--text-secondary);
    cursor: pointer;
    text-align: left;
    width: 100%;
    box-sizing: border-box;
    /* Menus hang from narrow buttons; one line per item makes them as wide
       as their longest item instead of the button's width. */
    white-space: nowrap;
  }

  .action-menu-item:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  .more-menu {
    min-width: 220px;
  }

  .menu-heading {
    padding: 6px 10px 2px;
    font-size: 0.64rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    color: var(--text-muted);
  }

  .menu-separator {
    height: 1px;
    margin: 3px 6px;
    background-color: var(--border-color);
  }

  .header-title-row {
    margin: 2px 0;
  }

  .title-text {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 650;
    letter-spacing: -0.015em;
    color: var(--text-primary);
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .header-meta-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 12px;
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .meta-item {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .meta-item :global(.meta-icon) {
    color: var(--text-muted);
  }

  .cwd-item {
    font-family: var(--font-mono);
    max-width: 500px;
    min-width: 0;
    white-space: nowrap;
  }

  /* One line: the path gives up room (ending in …, full on hover) so the
     branch, message count and token use stay beside it. Narrow panes wrap. */
  @container (min-width: 641px) {
    .header-meta-row {
      flex-wrap: nowrap;
    }
    .cwd-item {
      flex: 0 1 auto;
      min-width: 140px;
    }
    .header-meta-row > .meta-item:not(.cwd-item),
    .stats-group {
      flex-shrink: 0;
    }
    .branch-item {
      flex-shrink: 1;
      min-width: 0;
      max-width: 180px;
    }
    .branch-text {
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  /* The path is cut short, never the copy button after it. */
  .cwd-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cwd-copy-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 18px;
    height: 18px;
    padding: 0;
    border-radius: 3px;
    color: var(--text-muted);
    cursor: pointer;
  }

  .cwd-copy-btn:hover {
    color: var(--text-primary);
    background-color: var(--hover-bg);
  }

  .cwd-copy-btn.copied {
    color: var(--success, #10b981);
  }

  .missing-badge {
    color: var(--danger);
    font-weight: 600;
    font-size: 0.7rem;
  }

  .branch-item {
    font-family: var(--font-mono);
  }

  /* Token use as small tonal chips: the number, then a quiet unit. */
  .stats-group {
    display: flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .header-time-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 12px;
    font-size: 0.72rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .stat-pill {
    display: inline-flex;
    align-items: baseline;
    gap: 3px;
    padding: 1px 7px;
    border-radius: 999px;
    background-color: var(--segment-track);
  }

  .stat-label {
    font-weight: 500;
  }

  .time-item {
    white-space: nowrap;
  }

  .stat-val {
    font-weight: 600;
    color: var(--text-secondary);
  }

</style>
