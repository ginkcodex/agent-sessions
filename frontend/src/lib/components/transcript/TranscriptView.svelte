<script lang="ts">
  import { tick, untrack } from 'svelte';
  import type { SessionRef, Message, MessageJump, TerminalOpenMode } from '../../types';
  import { appState } from '../../stores/appState.svelte';
  import { search } from '../../stores/search.svelte';
  import { hasOpenModal, highlightTextNodes, isEditableTarget, jumpOffset } from '../../search';
  import { SessionFind } from '../../sessionFind.svelte';
  import { keyHandled } from '../../fullscreen';
  import { manage } from '../../stores/manage.svelte';
  import { api } from '../../api';
  import { copyToClipboard } from '../../portable';
  import { copiedGuidance, copyFailed, openedGuidance, openFailed } from '../../guidance';
  import { toast } from '../../stores/toast.svelte';
  import { link } from '../../stores/link.svelte';
  import { launcher } from '../../stores/launcher.svelte';
  import { terminals } from '../../stores/terminals.svelte';
  import { files } from '../../stores/files.svelte';
  import { contentSearch } from '../../stores/contentSearch.svelte';
  import { createTerminalFileOpener } from '../../terminalFileOpen';
  import { parseMessageFileLink } from '../../messageFileLinks';
  import { messageFileRecovery } from '../../messageFileOpen';
  import { browserLinkDestination } from '../../linkPreview';
  import { preferences } from '../../stores/preferences.svelte';
  import { epoch, isStaleReply } from '../../link';
  import { mixesRevisions, TranscriptRefresher, windowOf, type TranscriptWindow } from '../../transcriptRefresh';
  import type { TranscriptMode } from '../../transcript';
  import SessionHeader from './SessionHeader.svelte';
  import MessageBubble from './MessageBubble.svelte';
  import SessionFindBar from './SessionFindBar.svelte';
  import LoadingSpinner from '../common/LoadingSpinner.svelte';
  import EmptyState from '../common/EmptyState.svelte';
  import ErrorState from '../common/ErrorState.svelte';
  import DeleteConfirmDialog from '../common/DeleteConfirmDialog.svelte';

  const fileOpener = createTerminalFileOpener({
    link: () => link,
    epoch,
    resolve: (cwd, path) => api.resolveTerminalFile(cwd, path),
    files,
    report: body => toast.show({ title: 'Could not open message file', body, tone: 'error' }),
    isStale: isStaleReply,
  });
  $effect(() => {
    void appState.selectedSessionRef;
    void appState.selectedSessionMeta?.cwd;
    void link.dataHost;
    void link.host;
    void link.phase;
    void link.stale;
    void link.locked;
    return () => fileOpener.dispose();
  });
  function openMessageFile(value: string, ref: SessionRef) {
    if (!refsMatch(appState.selectedSessionRef, ref)) return;
    const target = parseMessageFileLink(value);
    if (!target) return;
    const cwd = appState.selectedSessionMeta?.cwd;
    if (!cwd) {
      toast.show({ title: 'Could not open message file', body: 'The session working directory is not available.', tone: 'error' });
      return;
    }
    const host = link.dataHost ?? '';
    const generation = loadGeneration;
    void fileOpener.open(host, target.path, cwd, {
      recover: messageFileRecovery({
        probeFiles: (dir, paths, expectedHost) => api.probeFiles(dir, paths, expectedHost),
        listFiles: dir => api.listFiles(dir),
        toast,
      }, host, cwd, target.path),
      valid: () => generation === loadGeneration && refsMatch(appState.selectedSessionRef, ref)
        && appState.selectedSessionMeta?.cwd === cwd,
      opened: resolved => {
        contentSearch.setMode('files');
        if (target.line !== undefined) files.revealFileLink(resolved.path, target);
      },
    });
  }

  const PAGE_SIZE = 50;
  const SHOW_BOTTOM_BTN_PX = 400;
  // How near the end the view must be to follow new messages.
  const FOLLOW_SLACK_PX = 40;
  // Scroll room needed before automatic history filling stops.
  const FILL_SCROLL_PX = 300;

  let hoveredLink = $state('');
  let focusedLink = $state('');
  $effect(() => {
    void appState.selectedSessionRef;
    void link.dataHost;
    hoveredLink = ''; focusedLink = '';
  });
  let messages = $state<Message[]>([]);
  let isLoading = $state(false);
  let isLoadingMore = $state(false);
  let hasMore = $state(false);
  let loadError = $state<string | null>(null);
  let mode = $state<TranscriptMode>(preferences.transcriptMode);
  let resumeCopied = $state(false);
  let resumeOpening = $state(false);
  let containerEl: HTMLElement | null = $state(null);
  let listEl: HTMLElement | null = $state(null);
  // Once the view reaches the end of the transcript, it stays there as
  // messages arrive, until the user scrolls up.
  let following = false;
  // Explicit anchoring survives wheel/trackpad input until toggled off.
  let anchoredToBottom = $state(false);
  let nearBottom = $state(false);
  let lastScrollTop = 0;
  let showScrollBottomBtn = $state(false);
  let jumpingToBottom = $state(false);
  let pendingLoad: Promise<void> | null = null;
  let pageOffset = $state(0);
  // The revision the window was refreshed to; spliced pages may be newer.
  let revision: string | undefined;
  const refresher = new TranscriptRefresher(api, epoch);
  let jumpLoading = $state(false);
  let jumpNotice = $state<string | null>(null);
  // The jump this transcript page was loaded for; cleared by normal loads so
  // a meta target is only force-shown while it is the navigation target.
  let activeJump = $state<MessageJump | null>(null);
  let isLoadingEarlier = $state(false);
  let fillFailed = false;
  let handledJumpID = 0;
  let loadGeneration = 0;
  let clearHighlight: (() => void) | null = null;
  let highlightTimer: ReturnType<typeof setTimeout> | null = null;

  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);
  const find = new SessionFind({
    search: (req) => api.searchSession(req),
    ref: () => appState.selectedSessionRef,
    includeHidden: () => mode === 'all',
    topIndex: topMessageIndex,
    jump: (jump) => void loadSearchJump(jump),
    clear: clearFindJump,
    isStale: isStaleReply,
  });

  // A search jump owns its initial page load. Normal selection opens the latest
  // page; guarding by jump id prevents the selection effect racing that load.
  let lastRef: SessionRef | null = null;
  $effect(() => {
    const ref = appState.selectedSessionRef;
    const jump = search.jump;
    const refChanged = ref !== lastRef;
    // The display mode is per session; jumps within it keep the chosen mode.
    if (ref && !refsMatch(lastRef, ref)) mode = untrack(() => preferences.transcriptMode);
    if (!ref || !refsMatch(lastRef, ref)) untrack(() => find.reset());
    lastRef = ref;
    if (ref) {
      if (jump && jump.id > handledJumpID && refsMatch(ref, jump.ref)) {
        handledJumpID = jump.id;
        void loadSearchJump(jump);
      } else if (refChanged) {
        void loadInitialMessages(ref);
      }
    } else {
      setWindow({ messages: [], offset: 0, hasMore: false });
      loadError = null;
    }
  });

  // Changing the default in Settings also applies to the open session.
  $effect(() => {
    mode = preferences.transcriptMode;
  });

  // A refresh or a change event for the open session reloads what is
  // loaded, in place, so new messages appear without losing the scroll.
  let lastVersion = appState.transcriptVersion;
  $effect(() => {
    const version = appState.transcriptVersion;
    // A jump still owns the page through its render/landing tick. Defer the
    // refresh rather than superseding its generation before it can highlight.
    if (version === lastVersion || jumpLoading || isLoading) return;
    lastVersion = version;
    const ref = untrack(() => appState.selectedSessionRef);
    if (ref) void reloadMessages(ref);
  });

  // An open find counts again when the transcript or the display level
  // changes, so new matches of a live session appear.
  $effect(() => {
    void appState.transcriptVersion;
    void mode;
    untrack(() => find.refresh());
  });

  function refsMatch(a: SessionRef | null | undefined, b: SessionRef): boolean {
    return a?.agent === b.agent && a.id === b.id;
  }

  function currentWindow(): TranscriptWindow {
    return { messages, offset: pageOffset, hasMore, revision };
  }

  function setWindow(win: TranscriptWindow): void {
    messages = win.messages;
    pageOffset = win.offset;
    hasMore = win.hasMore;
    revision = win.revision;
  }

  async function loadInitialMessages(ref: SessionRef) {
    const generation = ++loadGeneration;
    isLoading = true;
    jumpLoading = false;
    jumpNotice = null;
    activeJump = null;
    clearJumpHighlight();
    loadError = null;
    setWindow({ messages: [], offset: 0, hasMore: false });
    following = false;
    anchoredToBottom = false;
    nearBottom = false;
    fillFailed = false;

    try {
      const page = await api.getMessages(ref, -1, PAGE_SIZE);
      // Ensure we are still looking at the same session and request.
      if (generation === loadGeneration && refsMatch(appState.selectedSessionRef, ref)) {
        setWindow(windowOf(page));
        following = true;
        isLoading = false;
        await tick();
        if (generation === loadGeneration && containerEl) {
          containerEl.scrollTop = containerEl.scrollHeight;
          lastScrollTop = containerEl.scrollTop;
          nearBottom = atEnd(containerEl);
        }
      }
    } catch (err: any) {
      if (generation === loadGeneration && refsMatch(appState.selectedSessionRef, ref)) {
        loadError = err?.message || 'Failed to load session transcript';
      }
    } finally {
      if (generation === loadGeneration) isLoading = false;
    }
  }

  async function reloadMessages(ref: SessionRef): Promise<void> {
    // A first load in flight reads the current content anyway.
    if (isLoading || jumpLoading) return;
    if (pendingLoad) await pendingLoad;
    if (isLoading || jumpLoading || !refsMatch(appState.selectedSessionRef, ref)) return;
    if (loadError) {
      await loadInitialMessages(ref);
      return;
    }
    // Supersedes in-flight page loads, which would splice into the old list.
    const generation = ++loadGeneration;
    try {
      // Following uses a bounded recent window, even after a large update.
      // Reading history keeps its existing offset and visible position instead.
      // Only messages changed since the window's revision are fetched, where
      // the host supports it; unchanged ones keep their objects and rows.
      const latest = anchoredToBottom || following;
      const win = currentWindow();
      const next = await refresher.refresh(ref, win, latest ? -1 : win.offset, Math.max(win.messages.length, PAGE_SIZE));
      if (generation !== loadGeneration || !refsMatch(appState.selectedSessionRef, ref)) return;
      // The user may have scrolled into history while this request was in flight.
      if (latest && !anchoredToBottom && !following) {
        void reloadMessages(ref);
        return;
      }
      // A find match's marks are put back over the refreshed rows.
      const marked = activeJump?.sticky && clearHighlight ? activeJump : null;
      if (marked) clearJumpHighlight();
      setWindow(next);
      await tick();
      if (generation === loadGeneration && marked && activeJump === marked) markJump(marked, false);
      if (generation === loadGeneration && containerEl && (anchoredToBottom || following)) {
        containerEl.scrollTop = containerEl.scrollHeight;
      }
    } catch (err) {
      if (isStaleReply(err)) return;
      console.error('Failed to reload transcript:', err);
    }
  }

  async function loadSearchJump(jump: MessageJump): Promise<void> {
    // Find steps land in the loaded rows without reloading them, and
    // elsewhere keep showing them until the target's page arrives.
    const inWindow = jump.messageIndex >= pageOffset && jump.messageIndex < pageOffset + messages.length;
    const soft = Boolean(jump.sticky) && !isLoading && !loadError;
    if (soft && inWindow && !jumpLoading) {
      clearJumpHighlight();
      activeJump = jump;
      // State holds a proxy of the jump: a later jump replaces this one.
      const landing = activeJump;
      following = false;
      anchoredToBottom = false;
      await tick();
      if (activeJump === landing && refsMatch(appState.selectedSessionRef, jump.ref)) markJump(jump, true);
      return;
    }
    const generation = ++loadGeneration;
    const offset = jump.messageIndex < 0 ? 0 : jumpOffset(jump.messageIndex);
    if (!soft) isLoading = true;
    jumpLoading = true;
    jumpNotice = null;
    activeJump = jump;
    clearJumpHighlight();
    loadError = null;
    if (!soft) setWindow({ messages: [], offset, hasMore: false });
    following = false;
    anchoredToBottom = false;
    nearBottom = false;
    fillFailed = false;

    try {
      // Only the page around the target is requested; header metadata is
      // awaited too so its arrival doesn't shift the scroll after we land.
      const [page] = await Promise.all([
        api.getMessages(jump.ref, offset, PAGE_SIZE),
        jump.ready?.catch(() => undefined),
      ]);
      if (generation !== loadGeneration || !refsMatch(appState.selectedSessionRef, jump.ref)) return;
      setWindow(windowOf(page));
      isLoading = false;
      await tick();
      if (generation !== loadGeneration || !refsMatch(appState.selectedSessionRef, jump.ref)) return;

      if (jump.messageIndex < 0) {
        containerEl?.scrollTo({ top: 0, behavior: 'smooth' });
        return;
      }

      if (!markJump(jump, true) && !jump.sticky) {
        jumpNotice = 'This search result moved or was removed after it was indexed.';
      }
    } catch (err: any) {
      if (soft) {
        if (generation === loadGeneration && !isStaleReply(err)) {
          toast.show({ title: 'Could not show the match', body: err?.message || 'Please try again.', tone: 'error' });
        }
      } else if (generation === loadGeneration && refsMatch(appState.selectedSessionRef, jump.ref)) {
        loadError = err?.message || 'Failed to load the search result';
      }
    } finally {
      if (generation === loadGeneration) {
        isLoading = false;
        jumpLoading = false;
      }
    }
  }

  // Scrolls to the jump's message and marks its matches. A search result's
  // marks fade; a find match's stay until the next step.
  function markJump(jump: MessageJump, scroll: boolean): boolean {
    const target = containerEl?.querySelector<HTMLElement>(
      `[data-message-index="${jump.messageIndex}"]`
    );
    if (!target) return false;
    target.classList.add('search-jump-target');
    const disposeTextMarks = highlightTextNodes(target, jump.query);
    clearHighlight = () => {
      disposeTextMarks();
      target.classList.remove('search-jump-target');
    };
    if (!jump.sticky) highlightTimer = setTimeout(clearJumpHighlight, 5000);
    if (scroll) {
      // A find match centers its first mark: the message can be long.
      const mark = jump.sticky
        ? Array.from(target.querySelectorAll('mark.search-jump-match')).find((m) => m.getClientRects().length > 0)
        : undefined;
      // Land before pagination can prepend history: it adjusts scrollTop to
      // preserve the viewport, which interrupts a smooth scroll mid-flight.
      (mark ?? target).scrollIntoView({ block: 'center', behavior: 'instant' });
      if (containerEl) lastScrollTop = containerEl.scrollTop;
    }
    return true;
  }

  function clearJumpHighlight(): void {
    if (highlightTimer) clearTimeout(highlightTimer);
    highlightTimer = null;
    clearHighlight?.();
    clearHighlight = null;
  }

  function clearFindJump(): void {
    if (!activeJump?.sticky) return;
    clearJumpHighlight();
    activeJump = null;
  }

  // The first message whose bottom is below the top of the view.
  function topMessageIndex(): number {
    const el = containerEl;
    if (!el) return -1;
    const top = el.getBoundingClientRect().top;
    for (const row of el.querySelectorAll<HTMLElement>('[data-message-index]')) {
      if (row.getBoundingClientRect().bottom > top) return Number(row.dataset.messageIndex);
    }
    return -1;
  }

  function toggleFind(): void {
    if (find.open) find.close();
    else find.show(document.getSelection()?.toString() ?? '');
  }

  // Cmd+F on macOS, Ctrl+F elsewhere, as in browsers: finds in the open
  // session. Text fields, the terminal and editors keep their own.
  function handleWindowKey(e: KeyboardEvent): void {
    if (!appState.selectedSessionRef || hasOpenModal() || keyHandled(e)) return;
    const inFind = e.target instanceof Element && e.target.closest('[data-session-find]') !== null;
    if (isEditableTarget(e.target) && !inFind) return;
    if (e.key === 'Escape') {
      if (!find.open) return;
      e.preventDefault();
      find.close();
      return;
    }
    const command = isMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
    if (!command || e.altKey || e.shiftKey || e.code !== 'KeyF') return;
    e.preventDefault();
    find.show(document.getSelection()?.toString() ?? '');
  }

  function reopenSearch(): void {
    jumpNotice = null;
    search.show();
    void search.runNow();
  }

  // A jump starts mid-transcript; earlier pages load on demand, keeping the
  // visible content anchored while rows are prepended above it.
  // `fill` loads history only to cover a short view, keeping the bottom
  // follow state; user scrolling stops following.
  async function loadEarlierMessages(fill = false): Promise<void> {
    const ref = appState.selectedSessionRef;
    if (!ref || pageOffset <= 0 || isLoadingEarlier || jumpLoading || (anchoredToBottom && !fill)) return;
    if (!fill) following = false;
    const generation = loadGeneration;
    const end = pageOffset;
    const start = Math.max(0, end - PAGE_SIZE);
    let mixed = false;
    isLoadingEarlier = true;
    try {
      const page = await api.getMessages(ref, start, end - start);
      // A page for another window position would not join this one.
      if (generation !== loadGeneration || !refsMatch(appState.selectedSessionRef, ref) || pageOffset !== end) return;
      mixed = mixesRevisions(currentWindow(), page);
      const before = containerEl ? containerEl.scrollHeight - containerEl.scrollTop : 0;
      messages = [...(page.messages || []), ...messages];
      pageOffset = start;
      await tick();
      if (generation === loadGeneration && containerEl) {
        containerEl.scrollTop = following || anchoredToBottom
          ? containerEl.scrollHeight
          : containerEl.scrollHeight - before;
        lastScrollTop = containerEl.scrollTop;
      }
    } catch (err) {
      // Stops automatic filling; the Load earlier button remains a retry.
      if (fill) fillFailed = true;
      console.error('Failed to load earlier messages:', err);
    } finally {
      isLoadingEarlier = false;
    }
    if (mixed) void reloadMessages(ref);
    if (fill) void tick().then(fillViewport);
  }

  // In Chat mode a recent page can be mostly tool activity that renders
  // nothing, leaving no scrollbar to request history with. Load earlier
  // pages until the view can scroll or the transcript start is reached.
  function fillViewport(): void {
    const el = containerEl;
    if (!el || isLoading || isLoadingEarlier || fillFailed || pageOffset <= 0) return;
    if (el.scrollHeight - el.clientHeight > FILL_SCROLL_PX) return;
    void loadEarlierMessages(true);
  }

  // Returns the in-flight load when one is running so callers can await it.
  function loadMoreMessages(limit = PAGE_SIZE): Promise<void> {
    if (pendingLoad) return pendingLoad;
    const ref = appState.selectedSessionRef;
    if (!ref || !hasMore) return Promise.resolve();
    const generation = loadGeneration;
    const end = pageOffset + messages.length;
    let mixed = false;

    isLoadingMore = true;
    pendingLoad = (async () => {
      try {
        const page = await api.getMessages(ref, end, limit);
        if (generation === loadGeneration && refsMatch(appState.selectedSessionRef, ref) && pageOffset + messages.length === end) {
          mixed = mixesRevisions(currentWindow(), page);
          messages = [...messages, ...(page.messages || [])];
          hasMore = page.hasMore;
        }
      } catch (err: any) {
        console.error('Failed to load more messages:', err);
      } finally {
        isLoadingMore = false;
        pendingLoad = null;
      }
      // After pendingLoad clears, which the refresh waits for.
      if (mixed) void reloadMessages(ref);
    })();
    return pendingLoad;
  }

  function distanceFromBottom(el: HTMLElement): number {
    return el.scrollHeight - el.scrollTop - el.clientHeight;
  }

  // The end of what is loaded is not the end while pages remain.
  function atEnd(el: HTMLElement): boolean {
    return !hasMore && distanceFromBottom(el) <= FOLLOW_SLACK_PX;
  }

  function handleScroll(e: Event) {
    const el = e.currentTarget as HTMLElement;
    const scrollingUp = el.scrollTop < lastScrollTop;
    if (anchoredToBottom) el.scrollTop = el.scrollHeight;
    const scrollBottom = distanceFromBottom(el);
    nearBottom = atEnd(el);
    showScrollBottomBtn = scrollBottom > SHOW_BOTTOM_BTN_PX;
    if (anchoredToBottom || nearBottom) following = true;
    else if (el.scrollTop < lastScrollTop && scrollBottom > FOLLOW_SLACK_PX) following = false;
    lastScrollTop = el.scrollTop;

    if (scrollingUp && el.scrollTop < 250 && pageOffset > 0 && !isLoading) {
      void loadEarlierMessages();
    }
    if (scrollBottom < 250 && hasMore && !isLoadingMore && !isLoading) {
      loadMoreMessages();
    }
  }

  // Content can grow or shrink without a scroll event (lazy pages, display
  // mode), so re-evaluate the button and keep loading while it fits on screen.
  // Only new messages re-trigger it, so a failing page load is not retried in
  // a loop; scrolling retries it as before.
  $effect(() => {
    void messages.length;
    void mode;
    void hasMore;
    void containerEl;
    void tick().then(() => {
      if (!containerEl) return;
      if (anchoredToBottom) containerEl.scrollTop = containerEl.scrollHeight;
      const remaining = distanceFromBottom(containerEl);
      nearBottom = atEnd(containerEl);
      showScrollBottomBtn = remaining > SHOW_BOTTOM_BTN_PX;
      // A transcript that fits has no scroll to reach its end with.
      if (atEnd(containerEl)) following = true;
      if (remaining < 250 && hasMore && !isLoadingMore && !isLoading) {
        void loadMoreMessages();
      }
      fillViewport();
    });
  });

  // While following, any growth keeps the end in view: new messages, and
  // diagrams and images that render late. So does a shorter pane.
  $effect(() => {
    const el = containerEl;
    const list = listEl;
    if (!el || !list) return;
    lastScrollTop = el.scrollTop;
    const observer = new ResizeObserver(() => {
      if (anchoredToBottom || following) el.scrollTop = el.scrollHeight;
      nearBottom = atEnd(el);
      showScrollBottomBtn = distanceFromBottom(el) > SHOW_BOTTOM_BTN_PX;
    });
    observer.observe(el);
    observer.observe(list);
    return () => observer.disconnect();
  });

  // Jump directly to a recent page instead of downloading all intervening history.
  async function scrollToBottom() {
    const ref = appState.selectedSessionRef;
    if (jumpingToBottom || !ref) return;
    jumpingToBottom = true;
    const generation = ++loadGeneration;
    // This supersedes a jump still landing, whose cleanup is then skipped.
    jumpLoading = false;
    try {
      if (pendingLoad) await pendingLoad;
      const page = await api.getMessages(ref, -1, PAGE_SIZE);
      if (generation !== loadGeneration || !refsMatch(appState.selectedSessionRef, ref)) return;
      clearJumpHighlight();
      activeJump = null;
      fillFailed = false;
      setWindow(windowOf(page));
      following = true;
      await tick();
      if (containerEl && generation === loadGeneration) {
        containerEl.scrollTop = containerEl.scrollHeight;
        lastScrollTop = containerEl.scrollTop;
        nearBottom = atEnd(containerEl);
      }
    } catch (err) {
      if (generation === loadGeneration && !isStaleReply(err)) {
        toast.show({ title: 'Could not load latest messages', body: 'Please try again.', tone: 'error' });
      }
    } finally {
      jumpingToBottom = false;
    }
  }

  function toggleBottomAnchor(): void {
    anchoredToBottom = !anchoredToBottom;
    // Unlocking preserves normal follow behavior until the user scrolls up.
    following = true;
    if (anchoredToBottom && containerEl) {
      containerEl.scrollTop = containerEl.scrollHeight;
    }
  }

  async function handleCopyResume() {
    if (!appState.selectedSessionRef) return;
    try {
      const cmd = await api.copyResumeCommand(appState.selectedSessionRef);
      if (!(await copyToClipboard(cmd))) {
        toast.show(copyFailed('resume command', 'The clipboard is not available.'));
        return;
      }
      resumeCopied = true;
      setTimeout(() => {
        resumeCopied = false;
      }, 2000);
      toast.show(copiedGuidance('resume', { host: link.dataHost, shell: launcher.copyShell }));
    } catch (err) {
      if (isStaleReply(err)) return;
      toast.show(copyFailed('resume command', err));
    }
  }

  async function handleOpenResume(where: TerminalOpenMode = launcher.openMode) {
    if (!appState.selectedSessionRef || resumeOpening) return;
    resumeOpening = true;
    try {
      if (where === 'panel') {
        const meta = appState.selectedSessionMeta;
        await terminals.runResume(appState.selectedSessionRef, meta?.cwd, meta?.title || meta?.firstPrompt);
      } else {
        await api.openResumeInTerminal(appState.selectedSessionRef);
        toast.show(openedGuidance('resume'));
      }
    } catch (err) {
      if (isStaleReply(err)) return;
      toast.show(openFailed(err));
    } finally {
      resumeOpening = false;
    }
  }
</script>

<svelte:window onkeydown={handleWindowKey} />

<div class="transcript-pane" role="region" aria-label="Session content"
  onmouseover={(event) => hoveredLink = browserLinkDestination(event.target, event.currentTarget)}
  onmouseout={(event) => hoveredLink = browserLinkDestination(event.relatedTarget, event.currentTarget)}
  onmouseleave={() => hoveredLink = ''}
  onfocusin={(event) => focusedLink = browserLinkDestination(event.target, event.currentTarget)}
  onfocusout={(event) => focusedLink = browserLinkDestination(event.relatedTarget, event.currentTarget)}
>
  {#if !appState.selectedSessionRef}
    <EmptyState
      icon="💬"
      title="No Session Selected"
      description="Select a session from the list to view its full conversation and tool activity."
    />
  {:else if isLoading}
    <div class="center-state">
      <LoadingSpinner size={28} label={jumpLoading ? 'Opening search result…' : 'Loading transcript…'} />
    </div>
  {:else if loadError}
    <div class="center-state">
      <ErrorState
        title="Could not load transcript"
        message={loadError}
        onRetry={() => {
          if (appState.selectedSessionRef) {
            loadInitialMessages(appState.selectedSessionRef);
          }
        }}
      />
    </div>
  {:else if appState.selectedSessionRef}
    {#if appState.selectedSessionMeta}
      <SessionHeader
        meta={appState.selectedSessionMeta}
        {mode}
        onModeChange={(next) => (mode = next)}
        onResume={handleCopyResume}
        onOpenResume={handleOpenResume}
        {resumeOpening}
        {resumeCopied}
        onFind={toggleFind}
        findOpen={find.open}
      />
    {/if}

    {#if jumpNotice}
      <div class="jump-notice" role="status">
        <span>{jumpNotice}</span>
        <button type="button" onclick={reopenSearch}>Search again</button>
        <button type="button" class="dismiss-notice" aria-label="Dismiss notice" onclick={() => (jumpNotice = null)}>×</button>
      </div>
    {/if}

    <div class="messages-frame">
    {#if find.open}
      <SessionFindBar {find} />
    {/if}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      bind:this={containerEl}
      class="messages-scroll-area"
      role="region"
      aria-label="Messages list"
      onscroll={handleScroll}
    >
      <div class="messages-inner-list" bind:this={listEl}>
        {#if messages.length === 0}
          <div class="empty-messages-notice">
            <span class="empty-icon">📭</span>
            <span>No messages found in this session transcript.</span>
          </div>
        {:else}
          {#if pageOffset > 0}
            <div class="load-earlier-bar">
              <button type="button" disabled={isLoadingEarlier} onclick={() => loadEarlierMessages()}>
                {isLoadingEarlier ? 'Loading earlier messages…' : `Load earlier messages (${pageOffset} before)`}
              </button>
            </div>
          {/if}
          <!-- Keyed by position in the transcript, not the window, so loading earlier
               messages keeps the rows below (and a search jump's marks on them). -->
          {#each messages as msg, idx (msg.id || `${msg.role}-${msg.time}-${pageOffset + idx}`)}
            {@const globalIndex = pageOffset + idx}
            <div class="message-index-anchor" data-message-index={globalIndex}>
              <MessageBubble
                message={msg}
                sessionRef={appState.selectedSessionRef}
                onFileLink={(value) => { if (appState.selectedSessionRef) openMessageFile(value, appState.selectedSessionRef); }}
                index={globalIndex}
                {mode}
                forceVisible={activeJump?.messageIndex === globalIndex}
                searchKind={activeJump?.messageIndex === globalIndex ? activeJump.kind : null}
                searchParts={activeJump?.messageIndex === globalIndex ? activeJump.parts : undefined}
                onResetLevel={() => (activeJump = null)}
              />
            </div>
          {/each}
        {/if}

        {#if isLoadingMore}
          <div class="loading-more-bar">
            <LoadingSpinner size={16} label="Loading more messages…" />
          </div>
        {:else if hasMore && messages.length > 0}
          <!-- Fallback when a failed page leaves too little content to scroll. -->
          <div class="load-earlier-bar load-more-bar">
            <button type="button" onclick={() => loadMoreMessages()}>Load more messages</button>
          </div>
        {/if}

        {#if !hasMore && messages.length > 0}
          <div class="end-of-transcript-marker">
            <span>── End of transcript ──</span>
          </div>
        {/if}
      </div>
    </div>
    </div>

    <!-- Outside the scroll area so it stays pinned to the pane corner. -->
    {#if messages.length > 0 && (nearBottom || anchoredToBottom)}
      <button
        type="button"
        class="scroll-bottom-pill bottom-anchor"
        class:anchored={anchoredToBottom}
        aria-pressed={anchoredToBottom}
        aria-label={anchoredToBottom ? 'Release bottom anchor' : 'Anchor to latest message'}
        title={anchoredToBottom ? 'Release anchor to scroll freely' : 'Keep the latest message in view, even when scrolling'}
        onclick={toggleBottomAnchor}
      >
        <span class="anchor-icon" aria-hidden="true"></span>
      </button>
    {:else if showScrollBottomBtn || jumpingToBottom}
      <button
        type="button"
        class="scroll-bottom-pill"
        title="Scroll to bottom"
        disabled={jumpingToBottom}
        onclick={scrollToBottom}
      >
        {jumpingToBottom ? 'Loading…' : '↓ Bottom'}
      </button>
    {/if}
  {/if}

  {#if hoveredLink || focusedLink}
    <div class="link-preview" role="status">{hoveredLink || focusedLink}</div>
  {/if}

  <DeleteConfirmDialog
    open={manage.confirmDialogOpen}
    onClose={() => manage.dismissDialog()}
  />
</div>

<style>
  .transcript-pane {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    background-color: var(--bg-primary);
    overflow: hidden;
    position: relative;
  }

  .link-preview {
    position: absolute;
    bottom: 10px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 10;
    max-width: calc(100% - 32px);
    padding: 5px 10px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    box-shadow: 0 2px 8px #0002;
    font-size: 12px;
    line-height: 1.4;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    pointer-events: none;
  }

  .center-state {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }

  /* Holds the find bar over the messages' top right corner. */
  .messages-frame {
    position: relative;
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .messages-scroll-area {
    flex: 1;
    overflow-y: auto;
    position: relative;
    padding: 16px;
    -webkit-overflow-scrolling: touch;
  }

  .messages-inner-list {
    /* 900px on normal screens; on wide ones the column grows with the pane. */
    max-width: max(900px, 80%);
    margin: 0 auto;
    display: flex;
    flex-direction: column;
  }

  /* Each message remembers its height, so while a splitter is dragged the
     ones off screen can skip layout without anything moving. Laying out a
     long transcript at every width made the drag fall behind the pointer. */
  .message-index-anchor {
    contain-intrinsic-size: auto 300px;
  }

  :global(html.splitter-dragging) .message-index-anchor {
    content-visibility: auto;
  }

  :global(.message-index-anchor.search-jump-target .message-card) {
    outline: 2px solid rgba(245, 158, 11, 0.45);
    outline-offset: 0;
  }

  :global(mark.search-jump-match) {
    padding: 0 1px;
    border-radius: 2px;
    background: rgba(245, 158, 11, 0.55);
    color: inherit;
  }

  .load-earlier-bar {
    display: flex;
    justify-content: center;
    padding: 0 0 14px;
  }

  .load-earlier-bar button {
    padding: 5px 12px;
    border: 1px solid var(--border-color);
    border-radius: 14px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    font-size: 0.75rem;
  }

  .load-earlier-bar button:hover:not(:disabled) { color: var(--text-primary); background: var(--hover-bg); }
  .load-earlier-bar button:disabled { cursor: progress; opacity: 0.7; }

  .jump-notice {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 12px;
    border-bottom: 1px solid rgba(245, 158, 11, 0.4);
    background: rgba(245, 158, 11, 0.1);
    color: var(--text-secondary);
    font-size: 0.75rem;
  }

  .jump-notice span { flex: 1; }
  .jump-notice button {
    padding: 3px 7px;
    border-radius: 4px;
    color: var(--accent-color);
    font-weight: 600;
  }
  .jump-notice button:hover { background: var(--hover-bg); }
  .jump-notice .dismiss-notice { color: var(--text-muted); font-size: 1rem; }

  .empty-messages-notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 60px 20px;
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .loading-more-bar {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }

  .end-of-transcript-marker {
    display: flex;
    justify-content: center;
    padding: 24px 0 12px 0;
    font-size: 0.72rem;
    color: var(--text-muted);
    user-select: none;
  }

  .scroll-bottom-pill {
    position: absolute;
    bottom: 20px;
    right: 24px;
    padding: 6px 12px;
    border-radius: 20px;
    background-color: var(--accent-color);
    color: white;
    font-size: 0.75rem;
    font-weight: 600;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
    border: none;
    cursor: pointer;
    transition: transform 0.15s ease, opacity 0.15s ease;
    z-index: 10;
  }

  .bottom-anchor {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    padding: 0;
    font-size: 1rem;
    background-color: var(--bg-secondary);
    color: var(--text-secondary);
    border: 1px solid var(--border-color);
  }

  .anchor-icon {
    width: 20px;
    height: 20px;
    background-color: currentColor;
    -webkit-mask: url('../../../../assets/anchor.svg') center / contain no-repeat;
    mask: url('../../../../assets/anchor.svg') center / contain no-repeat;
  }

  .bottom-anchor.anchored {
    background-color: var(--accent-color);
    color: white;
    border-color: transparent;
  }

  .scroll-bottom-pill:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 3px;
  }

  .scroll-bottom-pill:disabled {
    cursor: progress;
  }

  .scroll-bottom-pill:hover:not(:disabled) {
    transform: translateY(-2px);
    opacity: 0.95;
  }
</style>
