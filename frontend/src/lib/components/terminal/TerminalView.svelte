<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { SearchAddon } from '@xterm/addon-search';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import '@xterm/xterm/css/xterm.css';
  import ContextMenu, { type ContextMenuItem } from '../common/ContextMenu.svelte';
  import { api } from '../../api';
  import { terminals, type PanelTab } from '../../stores/terminals.svelte';
  import { theme } from '../../stores/theme.svelte';
  import { preferences } from '../../stores/preferences.svelte';
  import { terminalFontFamily } from '../../terminalFont';
  import { TerminalResize } from '../../terminalResize';
  import { splitShortcutLabel, terminalFindShortcutLabel, terminalKeyAction, terminalKeySequence, terminalTabShortcut, terminalTheme } from '../../terminal';
  import { TERMINAL_FIND_LIMIT, TerminalFind } from '../../terminalFind.svelte';
  import { copyToClipboard, readClipboard } from '../../portable';
  import { isMac } from '../../../main';
  import { onSplitterDragEnd, splitterDragging } from '../../splitterDrag';
  import { installTerminalFileLinks } from '../../terminalFileLinks';
  import { createTerminalFileOpener } from '../../terminalFileOpen';
  import { messageFileRecovery } from '../../messageFileOpen';
  import { installTerminalColorScheme } from '../../terminalColorScheme';
  import { installTerminalClipboard } from '../../terminalClipboard';
  import { terminalReplacementText } from '../../terminalTextInput';
  import { files } from '../../stores/files.svelte';
  import { link } from '../../stores/link.svelte';
  import { epoch, isStaleReply } from '../../link';
  import { toast } from '../../stores/toast.svelte';

  interface Props {
    tab: PanelTab;
    /** The focused pane, which takes typing; its panel tab shows it. */
    active: boolean;
    /** Opens a shell beside this one. */
    onSplit: () => void;
  }

  let { tab, active, onSplit }: Props = $props();

  let host: HTMLDivElement;
  let term = $state<Terminal | null>(null);
  let fit: FitAddon | null = null;
  let colorScheme: ReturnType<typeof installTerminalColorScheme> | null = null;
  let search: SearchAddon | null = null;
  const find = new TerminalFind({
    addon: () => search,
    selection: () => term?.getSelection() ?? '',
    focus: () => term?.focus(),
    theme: () => theme.resolved,
  });
  let findInput = $state<HTMLInputElement | null>(null);
  let processSize: TerminalResize | null = null;
  let menu = $state<{ x: number; y: number; items: ContextMenuItem[] } | null>(null);

  function cssVar(name: string): string {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  }

  onMount(() => {
    let disposed = false;
    let cleanup = () => {};
    // Cells are measured when the terminal opens, so the font must be there.
    const font = terminalFontFamily(preferences.terminalFont, cssVar('--font-mono'));
    const ready = document.fonts?.load ? document.fonts.load(`13px ${font}`).catch(() => {}) : Promise.resolve();
    void ready.then(() => {
      if (disposed) return;
      cleanup = create(font);
    });
    return () => {
      disposed = true;
      cleanup();
    };
  });

  function create(font: string): () => void {
    const t = new Terminal({
      fontFamily: font,
      fontSize: 13,
      cursorBlink: true,
      scrollback: 5000,
      // Find marks its matches with decorations, which are proposed API.
      allowProposedApi: true,
      // In a program that reads the mouse, Option-drag (Shift-drag elsewhere)
      // still selects text for Copy.
      macOptionClickForcesSelection: true,
      theme: terminalTheme(theme.resolved, cssVar),
    });
    const f = new FitAddon();
    const s = new SearchAddon({ highlightLimit: TERMINAL_FIND_LIMIT });
    t.loadAddon(f);
    t.loadAddon(s);
    const results = s.onDidChangeResults((result) => find.setResults(result));
    t.loadAddon(
      new WebLinksAddon((event, uri) => {
        event.preventDefault();
        void api.openURL(uri);
      }),
    );
    t.attachCustomKeyEventHandler((event) => {
      // Third-party macOS Vietnamese input tools send replacement strings as
      // synthetic keys. Send the complete text once, before xterm truncates it
      // to the keypress charCode; cancel native insertion to avoid duplicates.
      const replacement = isMac ? terminalReplacementText(event) : null;
      if (replacement !== null) {
        event.preventDefault();
        if (event.type === 'keydown') t.input(replacement, true);
        return false;
      }
      const action = terminalKeyAction(event, isMac);
      // The pane's capture listener opens find and stops the shortcut.
      if (action === 'find') return false;
      if (action === 'app') return false;
      // A tab shortcut goes to the app only when that tab exists: programs
      // in the terminal may use Alt+digit themselves.
      const tabNumber = event.type === 'keydown' ? terminalTabShortcut(event, isMac) : null;
      if (tabNumber !== null && tabNumber <= terminals.groups.length) return false;
      if (action === 'copy') {
        event.preventDefault();
        void copySelection();
        return false;
      }
      if (action === 'paste') {
        event.preventDefault();
        void paste();
        return false;
      }
      // Typed on keydown; the keypress that follows must not add a CR.
      const sequence = terminalKeySequence(event);
      if (sequence !== null) {
        event.preventDefault();
        if (event.type === 'keydown') t.input(sequence);
        return false;
      }
      return true;
    });
    t.open(host);
    term = t;
    fit = f;
    search = s;

    const opener = createTerminalFileOpener({
      link: () => link,
      epoch,
      resolve: (cwd, path) => api.resolveTerminalFile(cwd, path),
      files,
      report: (body) => toast.show({ title: 'Could not open terminal file', body, tone: 'error' }),
      isStale: isStaleReply,
    });
    const fileLinks = installTerminalFileLinks(t, {
      isMac,
      cwd: tab.dir,
      onCwd: (cwd) => terminals.setCwd(tab.id, cwd),
      onOpen: (path, cwd) => {
        // A bare name such as run_gui.go is often a file deeper in the
        // project: offer the matches, as message file links do.
        void opener.open(tab.host, path, cwd, cwd ? {
          recover: messageFileRecovery({
            probeFiles: (dir, paths, host) => api.probeFiles(dir, paths, host),
            listFiles: (dir) => api.listFiles(dir),
            toast,
            source: 'terminal',
          }, tab.host, cwd, path),
        } : undefined);
      },
    });
    const id = tab.id;
    const input = t.onData((data) => {
      if (tab.status === 'running') terminals.write(id, data);
    });
    const scheme = installTerminalColorScheme(t, theme.resolved);
    colorScheme = scheme;
    const clipboard = installTerminalClipboard(t, (text) => void copyToClipboard(text));
    processSize = new TerminalResize(async (cols, rows) => {
      if (tab.status !== 'running') { processSize?.dispose(); return; }
      terminals.resized(id);
      await api.terminalResize(id, cols, rows);
    });
    const detach = terminals.attach(id, {
      write: (data) => t.write(data, () => void api.terminalAck(id, data.length).catch(() => {})),
      exit: (code) => t.write(`\r\n\x1b[2m[Process exited with code ${code}]\x1b[0m\r\n`),
      paste: (text) => {
        t.paste(text);
        t.focus();
      },
    });
    const observer = new ResizeObserver((entries) => {
      if (splitterDragging()) keepCursorInView(entries[entries.length - 1].contentRect.height);
      else resize();
    });
    observer.observe(host);
    const offDragEnd = onSplitterDragEnd(() => {
      if (t.element) t.element.style.transform = '';
      resize();
    });
    resize();
    if (active) t.focus();

    return () => {
      processSize?.dispose();
      processSize = null;
      observer.disconnect();
      offDragEnd();
      input.dispose();
      scheme.dispose();
      colorScheme = null;
      clipboard.dispose();
      fileLinks.dispose();
      results.dispose();
      opener.dispose();
      detach();
      t.dispose();
      term = null;
      fit = null;
      search = null;
    };
  }

  // Fits the terminal to its box and tells the process its new size. A
  // hidden tab has no box and keeps its size until it is shown. Refitting
  // at every frame of a splitter drag held each frame up, so the terminal
  // fits once the drag ends.
  function resize() {
    if (!term || !fit || splitterDragging() || host.clientWidth === 0 || host.clientHeight === 0) return;
    const dims = fit.proposeDimensions();
    if (!dims || !Number.isFinite(dims.cols) || !Number.isFinite(dims.rows) || dims.cols < 1 || dims.rows < 1) return;
    if (dims.cols !== term.cols || dims.rows !== term.rows) fit.fit();
    terminals.setSize(term.cols, term.rows, tab.id);
    if (tab.status === 'running') processSize?.request(term.cols, term.rows);
  }

  // Until it refits, the terminal keeps its rows. While the panel is too
  // short for them, they move up as far as the cursor row needs to stay in
  // view, which is where the refit puts them.
  function keepCursorInView(height: number) {
    const el = term?.element;
    const screen = el?.querySelector<HTMLElement>('.xterm-screen');
    if (!term || !el || !screen) return;
    const buffer = term.buffer.active;
    const row = buffer.baseY + buffer.cursorY - buffer.viewportY;
    const rowBottom = ((row + 1) * screen.offsetHeight) / term.rows;
    const shift = row < 0 || row >= term.rows ? 0 : Math.max(0, rowBottom - height);
    el.style.transform = shift ? `translateY(${-shift}px)` : '';
  }

  // xterm clears its selection whenever it reports the mouse to the program,
  // so a forced selection would be gone at the next hover, or at the
  // right-click that opens the menu to copy it. While text is selected,
  // those stay with the terminal.
  function keepSelection(event: MouseEvent) {
    if (!term?.hasSelection() || term.modes.mouseTrackingMode === 'none') return;
    if (event.type === 'mousemove' ? event.buttons === 0 : event.button === 2) event.stopPropagation();
  }

  async function copySelection() {
    const text = term?.getSelection();
    if (text) await copyToClipboard(text);
  }

  async function paste() {
    const text = await readClipboard();
    if (text && tab.status === 'running') term?.paste(text);
  }

  $effect(() => {
    const font = terminalFontFamily(preferences.terminalFont, cssVar('--font-mono'));
    const t = term;
    if (!t || t.options.fontFamily === font) return;
    let current = true;
    const ready = document.fonts?.load ? document.fonts.load(`13px ${font}`).catch(() => {}) : Promise.resolve();
    void ready.then(() => {
      if (!current || term !== t) return;
      t.options.fontFamily = font;
      resize();
    });
    return () => { current = false; };
  });

  $effect(() => {
    const resolved = theme.resolved;
    if (term) {
      term.options.theme = terminalTheme(resolved, cssVar);
      colorScheme?.update(resolved);
      // Only a theme change repaints the marks, not typing in the find bar.
      if (untrack(() => find.open)) untrack(() => find.refresh());
    }
  });

  $effect(() => {
    // A toolbar resize keeps the active tab, but should restore typing focus.
    void terminals.maximized;
    if (!active || !term || !terminals.open) return;
    const t = term;
    const frame = requestAnimationFrame(() => {
      resize();
      t.focus();
    });
    return () => cancelAnimationFrame(frame);
  });

  // Opens the find bar and puts the caret in it. The key or menu item that
  // asked is still being handled, and a closing menu returns focus to the
  // terminal, so the input is focused a frame later.
  function showFind() {
    find.openFind();
    requestAnimationFrame(() => {
      findInput?.focus();
      findInput?.select();
    });
  }

  // The find shortcut is taken in the capture phase so it never reaches the
  // terminal (or the app's own Cmd+Shift+F project search) on this pane.
  // Escape closes the bar no matter which control in it has focus.
  function handlePaneKeydown(event: KeyboardEvent) {
    if (!active) return;
    if (terminalKeyAction(event, isMac) === 'find') {
      event.preventDefault();
      event.stopPropagation();
      if (!event.repeat) showFind();
      return;
    }
    if (event.key === 'Escape' && find.open) {
      event.preventDefault();
      event.stopPropagation();
      find.closeFind();
    }
  }

  function handleFindKey(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      find.search(event.shiftKey ? 'previous' : 'next', false);
    }
  }

  function openMenu(event: MouseEvent) {
    event.preventDefault();
    menu = {
      x: event.clientX,
      y: event.clientY,
      items: [
        { label: 'Copy', disabled: !term?.hasSelection(), onSelect: () => void copySelection() },
        { label: 'Paste', disabled: tab.status !== 'running', onSelect: () => void paste() },
        { label: 'Find…', title: `Find (${terminalFindShortcutLabel(isMac)})`, onSelect: showFind },
        { label: 'Clear', onSelect: () => term?.clear() },
        {
          label: 'Split Terminal',
          separator: true,
          disabled: !terminals.canSplit(tab.id),
          title: `Split terminal (${splitShortcutLabel(isMac)})`,
          onSelect: onSplit,
        },
        { label: 'Close Terminal', onSelect: () => terminals.close(tab.id) },
      ],
    };
  }
</script>

<!-- A running tab takes dropped files: terminals.dropFiles types their paths.
     The panel hides the panes of tabs not shown. -->
<div
  class="terminal-view"
  style:--wails-drop-target={tab.status === 'running' ? 'drop' : undefined}
  oncontextmenu={openMenu}
  onkeydowncapture={handlePaneKeydown}
  role="presentation"
>
  <div class="xterm-host" bind:this={host} onmousedowncapture={keepSelection} onmousemovecapture={keepSelection}></div>
  {#if find.open}
    <!-- The terminal's own menu would hide the input's Cut, Copy and Paste. -->
    <div class="find-bar" role="search" aria-label="Find in terminal" oncontextmenu={(e) => e.stopPropagation()}>
      <div class="find-field text-field">
        <input
          bind:this={findInput}
          class="find-input"
          type="text"
          placeholder="Find"
          aria-label="Find in terminal"
          value={find.query}
          oninput={(e) => find.setQuery(e.currentTarget.value)}
          onkeydown={handleFindKey}
          autocomplete="off"
          autocorrect="off"
          autocapitalize="off"
          spellcheck="false"
        />
        <span class="find-count" class:none={find.query !== '' && !find.found}>{find.count}</span>
        <button
          type="button"
          class="find-btn case"
          class:on={find.matchCase}
          aria-pressed={find.matchCase}
          aria-label="Match case"
          title="Match case"
          onclick={() => find.toggleCase()}
        >Aa</button>
      </div>
      <button type="button" class="find-btn" title="Previous match (Shift+Enter)" aria-label="Previous match" onclick={() => find.search('previous', false)}>↑</button>
      <button type="button" class="find-btn" title="Next match (Enter)" aria-label="Next match" onclick={() => find.search('next', false)}>↓</button>
      <button type="button" class="find-btn" title="Close (Escape)" aria-label="Close find" onclick={() => find.closeFind()}>×</button>
    </div>
  {/if}
</div>

{#if menu}
  <ContextMenu
    x={menu.x}
    y={menu.y}
    items={menu.items}
    ariaLabel="Terminal actions"
    onClose={() => {
      menu = null;
      term?.focus();
    }}
  />
{/if}

<style>
  .terminal-view {
    position: absolute;
    inset: 0;
    padding: 4px 0 0 8px;
    background: var(--bg-primary);
  }

  /* The runtime marks the drop target while files are dragged over it. */
  .terminal-view:global(.wails-drop-target-active)::after {
    content: '';
    position: absolute;
    inset: 4px;
    border: 2px dashed var(--accent-color);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-color) 8%, transparent);
    pointer-events: none;
  }

  .xterm-host {
    width: 100%;
    height: 100%;
  }

  .xterm-host :global(.xterm) {
    height: 100%;
  }

  /* Until the terminal refits after a splitter drag, it keeps its rows and
     the panel clips them; keepCursorInView moves them. */
  :global(html.splitter-dragging) .xterm-host {
    overflow: hidden;
  }

  :global(html.splitter-dragging) .xterm-host :global(.xterm) {
    height: auto;
  }

  /* xterm.css paints the viewport black; xterm 6 themes only the scrollable
     element inside it, which stops at the last whole row. */
  .xterm-host :global(.xterm .xterm-viewport) {
    background-color: var(--bg-primary);
  }

  /* Over the terminal's top right corner, as the file viewer's find bar is. */
  .find-bar {
    position: absolute;
    top: 6px;
    right: 18px;
    z-index: 20;
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 5px;
    font-family: var(--font-sans);
    font-size: 0.75rem;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12), 0 1px 3px rgba(15, 23, 42, 0.08);
  }

  /* Room for the case toggle at the end of the field. */
  .find-field {
    gap: 4px;
    padding: 2px 2px 2px 8px;
  }

  .find-input {
    width: 180px;
  }

  .find-count {
    min-width: 76px;
    padding: 0 4px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    text-align: center;
    white-space: nowrap;
  }

  .find-count.none {
    color: var(--danger);
  }

  .find-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    min-width: 24px;
    height: 24px;
    padding: 0 4px;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    cursor: pointer;
  }

  .find-btn:hover {
    background-color: var(--bg-tertiary);
    color: var(--text-primary);
  }

  .find-btn.case {
    min-width: 22px;
    height: 20px;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
  }

  .find-btn.on {
    color: var(--accent-color);
    background-color: var(--active-bg);
  }
</style>
