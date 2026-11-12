import {
  api,
  subscribeConnectionState,
  subscribeAskpassPrompt,
} from '../api';
import { targetKey } from '../tree';
import { hostLabel, setWindowTitle, windowTitle } from '../hosts';
import { hostInOtherWindow } from '../guidance';
import { toast } from './toast.svelte';
import type {
  AskpassPrompt,
  ConnectionPhase,
  ConnectionState,
  HostCapabilities,
  HostEntry,
  WSLEntry,
} from '../types';
import { appState } from './appState.svelte';
import { manage } from './manage.svelte';
import { search } from './search.svelte';
import { handoff } from './handoff.svelte';
import { exporter } from './export.svelte';
import { importer } from './importer.svelte';
import { files } from './files.svelte';
import { contentSearch } from './contentSearch.svelte';
import { fileActions } from './fileActions.svelte';
import { link } from './link.svelte';
import { terminals } from './terminals.svelte';
import { filePrompt } from './filePrompt.svelte';
import { chats } from './chats.svelte';
import { endingChatsWarning } from '../closeConfirm';

// Local always has every capability; the backend reports none for it.
const LOCAL_CAPABILITIES: HostCapabilities = {
  repairCodexSession: true,
  saveFile: true,
  blame: true,
  gitChanges: true,
  gitChangesRepository: true,
  gitDiff: true,
  gitWrite: true,
  gitSync: true,
  searchFiles: true,
  replaceFiles: true,
  newSession: true,
  trash: true,
  manage: true,
  export: true,
  import: true,
  search: true,
  searchPath: true,
  gitHistory: true,
  gitHistoryActions: true,
  gitHistoryCompare: true,
  acpChat: true,
};

export class ConnectionStore {
  phase = $state<ConnectionPhase>('local');
  host = $state<string | undefined>(undefined);
  generation = $state<number>(0);
  error = $state<string | undefined>(undefined);
  capabilities = $state<HostCapabilities>({ ...LOCAL_CAPABILITIES });
  appVersion = $state<string | undefined>(undefined);

  hosts = $state<HostEntry[]>([]);
  loadingHosts = $state(false);
  hostsError = $state<string | null>(null);
  wslDistros = $state<WSLEntry[]>([]);

  pendingAskpass = $state<AskpassPrompt | null>(null);

  private unsubscribeState: (() => void) | null = null;
  private unsubscribeAskpass: (() => void) | null = null;
  private initialized = false;
  private lifecycle = 0;
  private connectionEvents = 0;
  private title = '';
  private resetCallbacks = new Set<(generation: number, host?: string, sameHost?: boolean) => void>();

  get isRemote(): boolean {
    return this.phase === 'connected' && Boolean(this.host);
  }

  get currentHost(): string {
    return this.host ? hostLabel(this.host) : 'Local';
  }

  get canTrash(): boolean {
    return this.capabilities.trash;
  }

  get canManage(): boolean {
    return this.capabilities.manage;
  }

  get canExport(): boolean {
    return this.capabilities.export;
  }

  get canImport(): boolean {
    return this.capabilities.import;
  }

  get canSearch(): boolean {
    return this.capabilities.search;
  }

  /** Whether chats can start: here, or on a connected host whose server runs them. */
  get canChat(): boolean {
    return (this.phase === 'local' || this.phase === 'connected') && this.capabilities.acpChat === true;
  }

  async init(): Promise<void> {
    if (this.initialized) return;
    this.initialized = true;
    const lifecycle = ++this.lifecycle;
    const active = () => this.initialized && this.lifecycle === lifecycle;

    this.unsubscribeState = subscribeConnectionState((state) => {
      if (!active()) return;
      this.connectionEvents++;
      this.applyState(state);
    });

    this.unsubscribeAskpass = subscribeAskpassPrompt((prompt) => {
      if (active()) this.pendingAskpass = prompt;
    });

    const seen = this.connectionEvents;
    try {
      const state = await api.connectionState();
      // Local identity enrichment can wait behind engine startup. Any live
      // connection event received meanwhile is newer than this snapshot.
      if (active() && seen === this.connectionEvents && state) {
        this.applyState(state, false);
      }
    } catch {
      // Backend may not be available yet or in mock mode
    }

    if (active()) await this.refreshHosts();
  }

  destroy(): void {
    this.lifecycle++;
    this.unsubscribeState?.();
    this.unsubscribeState = null;
    this.unsubscribeAskpass?.();
    this.unsubscribeAskpass = null;
    this.initialized = false;
    this.resetCallbacks.clear();
  }

  /**
   * Calls back each time the stores reload for a backend: a host connects,
   * the window returns to this computer, or sameHost, the shown host
   * reconnects.
   */
  onReset(callback: (generation: number, host?: string, sameHost?: boolean) => void): () => void {
    this.resetCallbacks.add(callback);
    return () => {
      this.resetCallbacks.delete(callback);
    };
  }

  async refreshHosts(): Promise<void> {
    this.loadingHosts = true;
    this.hostsError = null;
    // Neither list waits for or fails with the other.
    const wsl = this.refreshWSLDistros();
    try {
      this.hosts = (await api.listHosts()) || [];
    } catch (err: any) {
      this.hostsError = err?.message || 'Failed to list hosts';
    } finally {
      this.loadingHosts = false;
    }
    await wsl;
  }

  private async refreshWSLDistros(): Promise<void> {
    try {
      this.wslDistros = (await api.listWSLDistros()) || [];
    } catch {
      // The section is left out; SSH hosts still connect.
      this.wslDistros = [];
    }
  }

  /**
   * The warning for the window's open chats that connecting to target ends,
   * or null when none would. Any connect or disconnect ends them: the
   * backend ends this computer's chats when the window leaves it, and a
   * host's chats end with its connection. A host that is not serving (a
   * dropped one, or Local while a connect is pending) has no live chats.
   */
  private endingChats(previous: string, target: string): string | null {
    if (link.locked) return null;
    const action = previous === target ? `Reconnecting to ${hostLabel(target)}`
      : target ? `Connecting to ${hostLabel(target)}`
      : `Disconnecting from ${hostLabel(previous)}`;
    return endingChatsWarning(chats.chats, action);
  }

  /** Ask only for explicit host changes, after resolving unsaved file edits. */
  private async changeHost(target: string, action: () => Promise<void>): Promise<void> {
    await fileActions.changeHost(async () => {
      const previous = link.dataHost ?? '';
      const ids = previous === target ? [] : terminals.tabs.filter(tab => tab.host === previous).map(tab => tab.id);
      const ending = this.endingChats(previous, target);
      let close = false;
      if (ids.length > 0) {
        const name = previous ? hostLabel(previous) : 'Local';
        const tabs = `${ids.length} terminal tab(s) belong to ${name}. Keep them open, or close them when switching hosts? Closing ends any running processes, including agents. Split panes count separately.`;
        const choice = await filePrompt.ask('Terminal tabs from previous host', ending ? `${tabs}\n\n${ending}` : tabs, [
          { value: 'keep', label: ending ? 'End chats, keep terminal tabs' : 'Keep terminal tabs' },
          { value: 'close', label: ending ? 'End chats and terminal tabs' : 'End and close terminal tabs', danger: true },
        ]);
        if (choice !== 'keep' && choice !== 'close') return;
        close = choice === 'close';
      } else if (ending && !await this.confirmEndingChats(ending, target)) {
        return;
      }
      // A failed connection must not destroy terminals from the previous host.
      await action();
      if (close) for (const id of ids) terminals.close(id, true);
    });
  }

  private async confirmEndingChats(warning: string, target: string): Promise<boolean> {
    const label = target === (link.dataHost ?? '') ? 'End chats and reconnect' : target ? 'End chats and connect' : 'End chats and disconnect';
    return await filePrompt.ask('End open chats?', warning, [{ value: 'end', label, danger: true }]) === 'end';
  }

  async connect(alias: string, envOverrides?: Record<string, string>): Promise<void> {
    this.error = undefined;
    try {
      const connect = async () => {
        const env = envOverrides ?? getSavedHostEnv(alias);
        if (Object.keys(env).length > 0) await api.setHostEnv(env);
        await api.connect(alias);
      };
      if (alias !== link.dataHost) await this.changeHost(alias, connect);
      else {
        // Retrying a dropped host ends nothing; connecting again to the
        // one that serves ends its chats.
        const ending = this.endingChats(alias, alias);
        if (!ending || await this.confirmEndingChats(ending, alias)) await connect();
      }
    } catch (err: any) {
      // The host is open in another window, which was brought to the
      // front; this window stays where it was.
      const elsewhere = hostInOtherWindow(alias, err);
      if (elsewhere) {
        toast.show(elsewhere);
        return;
      }
      this.error = err?.message || 'Failed to connect';
    }
  }

  async disconnect(): Promise<void> {
    this.error = undefined;
    try {
      await this.changeHost('', () => api.disconnect());
    } catch (err: any) {
      this.error = err?.message || 'Failed to disconnect';
    }
  }

  async askpassReply(id: string, answer: string): Promise<boolean> {
    try {
      const ok = await api.askpassReply(id, answer);
      if (this.pendingAskpass?.id === id) {
        this.pendingAskpass = null;
      }
      return ok;
    } catch {
      return false;
    }
  }

  cancelAskpass(id: string): void {
    void this.askpassReply(id, '');
  }

  applyState(state: ConnectionState, triggerResets = true): void {
    this.phase = state.phase;
    this.host = state.host;
    this.generation = state.generation;
    this.error = state.error;
    // Only a live session reports real capabilities. A dropped host keeps
    // its last ones so the stale view still describes that host.
    if (state.phase === 'local') {
      // Code diagnostics are optional even locally: keep what the local
      // backend reports instead of assuming support.
      this.capabilities = {
        ...LOCAL_CAPABILITIES,
        ...(state.capabilities?.codeDiagnostics ? { codeDiagnostics: true } : {}),
        ...(state.capabilities?.codeDiagnostics && state.capabilities.liveCodeDiagnostics ? { liveCodeDiagnostics: true } : {}),
      };
    } else if (state.phase === 'connected' && state.capabilities) {
      this.capabilities = state.capabilities;
    }
    this.appVersion = state.appVersion;
    this.updateTitle();

    if (!triggerResets) {
      link.seed(state.phase, state.host);
      return;
    }
    // Reload only when a host (re)connects or the app returns to Local; a
    // drop or a pending switch leaves the last data up as stale.
    const previousHost = link.dataHost;
    if (link.update(state.phase, state.host)) {
      this.resetStores(link.dataHost, state.phase === 'connected' && !!state.host && previousHost === state.host);
    }
  }

  // The title names the selected host, even while it connects or is down,
  // so each window says which connection it belongs to.
  private updateTitle(): void {
    const title = windowTitle(this.host);
    if (title === this.title) return;
    this.title = title;
    setWindowTitle(title);
  }

  /** The loaded data is from a host that is not serving it right now. */
  get stale(): boolean {
    return link.stale;
  }

  private resetStores(host?: string, sameHost = false): void {
    this.pendingAskpass = null;
    void appState.reset(host, sameHost);
    manage.reset();
    search.reset();
    handoff.reset();
    exporter.reset();
    importer.reset();
    contentSearch.reset();
    void files.connectionChanged(host);
    for (const cb of this.resetCallbacks) {
      try {
        cb(this.generation, host, sameHost);
      } catch {
        // Ignore subscriber errors
      }
    }
  }
}

export const connectionStore = new ConnectionStore();

export function getSavedHostEnv(host?: string): Record<string, string> {
  if (typeof localStorage === 'undefined' || !host) return {};
  try {
    const raw = localStorage.getItem(`agent-sessions:host-env:${targetKey(host)}`);
    if (raw) return JSON.parse(raw);
  } catch {}
  return {};
}

export function saveHostEnv(host: string, env: Record<string, string>): void {
  if (typeof localStorage === 'undefined' || !host) return;
  try {
    localStorage.setItem(`agent-sessions:host-env:${targetKey(host)}`, JSON.stringify(env));
  } catch {}
}
