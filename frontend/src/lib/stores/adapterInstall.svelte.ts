import type { ConnectionState } from '../types';
import type { AdapterToolName, ManagedToolRequest, ManagedToolState } from '../managedTools';
import { ADAPTER_TOOLS } from '../managedTools';
import { api } from '../api';
import { epoch } from '../link';
import { errorText } from '../manage';
import { chats } from './chats.svelte';

interface Backend {
  connectionState(): Promise<ConnectionState>;
  managedTool?(request: ManagedToolRequest): Promise<ManagedToolState>;
}

/** Polls of a running install, 2 s apart: about ten minutes. */
const MAX_POLLS = 300;

/**
 * Installs or updates a chat agent's ACP adapter as an app-managed tool, on
 * the host the window shows, one at a time. The user's click is the consent:
 * run asks the host for a proposal and installs it at once when it is for
 * that host and tool. A token is used once and never resent; a lost answer
 * is asked about with status.
 */
export class AdapterInstaller {
  /** The adapter this flow is for. */
  tool = $state<AdapterToolName | undefined>(undefined);
  /** The host's last answer: a proposal to review, a running job, or its end. */
  state = $state.raw<ManagedToolState | undefined>(undefined);
  /** A request is on its way. */
  pending = $state(false);
  /** Why the flow stopped, for the user. */
  error = $state('');

  private host = '';
  private owner = -1;
  private life = 0;
  private polls = 0;
  private timer?: ReturnType<typeof setTimeout>;
  private readonly backend: Backend;
  private readonly epoch: () => number;
  private readonly installed: (tool: AdapterToolName) => void | Promise<void>;

  constructor(backend: Backend, currentEpoch: () => number, installed: (tool: AdapterToolName) => void | Promise<void>) {
    this.backend = backend;
    this.epoch = currentEpoch;
    this.installed = installed;
  }

  /** An install runs; no other can start. */
  get busy(): boolean {
    return this.pending || this.state?.state === 'installing';
  }

  /** Installs tool, the pinned version, as the user asked with one click. */
  async run(tool: AdapterToolName): Promise<void> {
    await this.propose(tool);
    if (this.tool === tool && this.state?.state === 'proposed') await this.install();
  }

  /** Asks the host what installing tool would download. */
  async propose(tool: AdapterToolName): Promise<void> {
    if (this.busy || !ADAPTER_TOOLS.includes(tool)) return;
    this.clear();
    const life = this.life;
    this.owner = this.epoch();
    this.tool = tool;
    this.pending = true;
    try {
      if (!this.backend.managedTool) throw new Error('This host cannot install adapters for the app: its agent-sessions is older');
      const conn = await this.backend.connectionState();
      if (!this.current(life)) return;
      if (!conn.hostId || !conn.capabilities?.codeDiagnostics || !['local', 'connected'].includes(conn.phase)) {
        throw new Error('This host offers no app-managed installs');
      }
      this.host = conn.hostId;
      const out = await this.backend.managedTool({ action: 'propose', expectedHostId: this.host, tool });
      if (!this.current(life)) return;
      const p = out.proposal;
      if (out.hostId !== this.host || (out.state === 'proposed' && (!p || p.hostId !== this.host || p.tool !== tool || !p.token || !p.connectionId || !Number.isFinite(Date.parse(p.expires))))) {
        throw new Error('The answer came from another host. Reconnect to the intended host');
      }
      if (out.state === 'proposed') this.state = out;
      else this.error = out.message || 'This host cannot install it';
    } catch (e) {
      if (this.current(life)) this.fail(e);
    } finally {
      if (life === this.life) this.pending = false;
    }
  }

  /** Installs the proposal. Its token goes out once. */
  async install(): Promise<void> {
    const p = this.state?.proposal;
    if (this.pending || this.state?.state !== 'proposed' || !p) return;
    this.state = undefined;
    if (Date.parse(p.expires) <= Date.now()) {
      this.error = 'The offer expired. Choose Install again';
      return;
    }
    await this.send({ action: 'install', token: p.token });
  }

  /** Stops the running install. */
  async cancel(): Promise<void> {
    const job = this.state?.jobId;
    if (this.pending || this.state?.state !== 'installing' || !job) return;
    await this.send({ action: 'cancel', jobId: job });
  }

  /** Forgets the flow: the window shows another backend. */
  reset(): void {
    this.clear();
  }

  private clear(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.life++;
    this.tool = undefined;
    this.state = undefined;
    this.pending = false;
    this.error = '';
    this.polls = 0;
  }

  private current(life: number): boolean {
    return life === this.life && this.owner === this.epoch();
  }

  private fail(e: unknown): void {
    this.state = undefined;
    this.error = errorText(e, 'The install did not start');
  }

  private async send(request: Omit<ManagedToolRequest, 'expectedHostId'>): Promise<void> {
    const life = this.life;
    const job = this.state?.jobId;
    const tool = this.tool;
    clearTimeout(this.timer);
    this.pending = true;
    this.error = '';
    try {
      if (!this.backend.managedTool || !tool) throw new Error('This host cannot install adapters for the app');
      const out = await this.backend.managedTool({ ...request, expectedHostId: this.host });
      if (!this.current(life)) return;
      if (out.hostId !== this.host || out.proposal) throw new Error('The answer came from another host. Reconnect to the intended host');
      if (out.state === 'installing' && (!out.jobId || (job && out.jobId !== job) || (out.tool && out.tool !== tool))) {
        throw new Error('Another install answered. Check Settings › Coding agents again');
      }
      this.state = out;
      await this.settle(out, tool, life);
    } catch (e) {
      if (this.current(life)) this.fail(e);
    } finally {
      if (life === this.life) this.pending = false;
    }
  }

  // Polls a running install, and reloads the chat agents once it ends.
  private async settle(out: ManagedToolState, tool: AdapterToolName, life: number): Promise<void> {
    if (out.state === 'installing') {
      if (++this.polls > MAX_POLLS) {
        this.error = 'Stopped watching the install; it may still be running. Reopen Settings › Coding agents later';
        return;
      }
      this.timer = setTimeout(() => {
        if (this.current(life)) void this.send({ action: 'status', jobId: out.jobId });
      }, 2000);
      return;
    }
    if (out.state === 'failed' || out.state === 'canceled' || out.state === 'unavailable') {
      this.error = out.message ? oneClick(out.message) : 'The install failed';
    }
    // Whatever the end, the agents say what is there now.
    await this.installed(tool);
  }
}

// The installer's messages are shared with code checks, which ask for a fresh
// approval; here the button itself is the approval.
function oneClick(message: string): string {
  return message.replace(/; request fresh approval to retry$/, '. Try again');
}

export const adapterInstall = new AdapterInstaller(api, epoch, async () => {
  await chats.loadAgents().catch(() => {});
});
