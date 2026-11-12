import { untrack } from 'svelte';
import type { AgentID, SessionMeta, SessionRef } from '../types';
import { isSessionInUseError, isStaleReply } from '../link';
import { errorText } from '../manage';
import { chatAgentAvailable } from '../chat';
import { choicesToApply, loadChoices, saveChoice } from '../chatOptions';
import { appState } from './appState.svelte';
import { chats, type Chat } from './chats.svelte';
import { connectionStore } from './connection.svelte';
import { link } from './link.svelte';
import { terminals, type PanelTab } from './terminals.svelte';
import { toast } from './toast.svelte';

/**
 * Opens sessions in chats from the UI: a new chat, or a saved session
 * continued in one. A session runs in one place at a time, so a session a
 * terminal tab runs does not open in a chat, nor the reverse.
 */
export class ChatOpener {
  /** A chat is starting from here. */
  opening = $state(false);
  /** Counts up to ask the shown chat's composer for focus. */
  focusRequest = $state(0);
  /** The last request a composer took. */
  focusHandled = 0;

  get available(): boolean {
    return connectionStore.canChat;
  }

  /** Whether the agent's adapter was found; false until the agents load. */
  canChat(agent: AgentID): boolean {
    return this.available && chatAgentAvailable(chats.agents, agent);
  }

  /**
   * Asks which agents have an adapter; adapters come and go. Effects call
   * this: it reads no state for them, or each load would start the next.
   */
  refresh(): void {
    untrack(() => {
      if (this.available && !chats.agentsLoading) void chats.loadAgents().catch(() => {});
    });
  }

  focus(): void {
    this.focusRequest++;
  }

  /** Continues a saved session in a chat, or shows the chat already driving it. */
  async openSession(meta: SessionMeta): Promise<boolean> {
    const ref = meta.ref;
    if (chats.forSession(ref)) {
      this.focus();
      return true;
    }
    if (this.refuseChat(ref)) return false;
    return this.run(
      async () => {
        await chats.start({ agent: ref.agent, dir: meta.cwd, resume: { agent: ref.agent, id: ref.id }, questions: true });
      },
      () => this.openSession(meta),
    );
  }

  /**
   * Starts a chat with a new session of agent in dir, and shows it. The
   * options the user last chose for the agent on this host apply first,
   * except a mode that skips permission requests.
   */
  startNew(agent: AgentID, dir: string): Promise<boolean> {
    return this.run(() => this.startFresh(agent, dir));
  }

  /**
   * Sets one of the chat's session options as the user chose it, and
   * remembers the choice for the agent's next new chat on this host.
   */
  async choose(chat: Chat, configId: string, value: string | boolean): Promise<void> {
    const host = link.dataHost;
    const option = chat.configOptions.find((o) => o.id === configId);
    try {
      await chats.setOption(chat.chatId, configId, value);
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not change the option', body: errorText(err, 'The agent kept the current value.'), tone: 'error' });
      return;
    }
    if (option) saveChoice(chat.agent, host, option, value);
  }

  private async startFresh(agent: AgentID, dir: string): Promise<void> {
    const host = link.dataHost;
    const chat = await chats.start({ agent, dir, questions: true });
    await this.applyChoices(chat, host);
    if (chat.session) await appState.selectSession(chat.session, 'push');
  }

  // Applies the remembered choices the agent still offers, before the first
  // prompt, one at a time: a model change can change the efforts on offer.
  // One the agent refuses is skipped.
  private async applyChoices(chat: Chat, host: string | undefined): Promise<void> {
    const saved = loadChoices(chat.agent, host);
    let options = chat.configOptions;
    const tried = new Set<string>();
    for (;;) {
      const next = choicesToApply(chat.agent, options, saved).find(([id]) => !tried.has(id));
      if (!next) return;
      const [id, value] = next;
      tried.add(id);
      try {
        const got = await chats.setOption(chat.chatId, id, value);
        if (got.length > 0) options = got;
      } catch (err) {
        if (isStaleReply(err)) return;
        const name = options.find((o) => o.id === id)?.name ?? id;
        toast.show({ title: `Could not apply your ${name} choice`, body: errorText(err, 'The agent kept its own.'), tone: 'error' });
      }
    }
  }

  /**
   * Starts an ended chat's agent again: it continues the session when the
   * agent saved it (saved), else it starts a new one in the same directory.
   * The ended chat goes once the new one runs.
   */
  restart(chat: Chat, saved: boolean): Promise<boolean> {
    if (chat.phase !== 'exited') return Promise.resolve(false);
    const ref = chat.session;
    if (saved && ref && this.refuseChat(ref)) return Promise.resolve(false);
    return this.run(
      async () => {
        if (saved && ref) {
          await chats.start({ agent: chat.agent, dir: chat.dir, resume: { agent: ref.agent, id: ref.id }, questions: true });
        } else {
          await this.startFresh(chat.agent, chat.dir);
        }
        chats.dismiss(chat.key);
      },
      () => this.restart(chat, saved),
    );
  }

  /**
   * Stops a live chat and continues its session in a panel terminal tab.
   * The ended chat goes at once: the terminal tab is the continuation, so
   * Dismiss and Restart are not offered. False when the chat has no saved
   * session to resume, or when something failed. A session without meta is
   * not saved yet (a new chat before its first turn): its ID resumes
   * nothing, so the chat stays.
   */
  async switchToTerminal(chat: Chat, meta?: SessionMeta | null): Promise<boolean> {
    const ref = chat.session;
    if (!ref || !meta) return false;
    try {
      await chats.close(chat.chatId);
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not end the chat', body: errorText(err, 'Please try again.'), tone: 'error' });
      return false;
    }
    // The engine ends the chat before close answers, so the chat is known
    // to be over and can go; a late exit event for it is ignored.
    chats.dismiss(chat.key);
    try {
      await terminals.runResume(ref, meta?.cwd || chat.dir, meta?.title || meta?.firstPrompt || '');
      return true;
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not open the terminal', body: `${errorText(err, 'The terminal did not start.')} The session is saved; resume it from the session header.`, tone: 'error' });
      return false;
    }
  }

  /**
   * Stops a terminal tab's agent and continues its session in a chat. Only
   * the pane that runs the session goes: a split tab's siblings stay. The
   * pane goes at once, so refuseChat lets the chat start. False when the
   * tab has no saved session to continue.
   */
  async switchToChat(tab: PanelTab): Promise<boolean> {
    const ref = tab.session;
    if (!ref) return false;
    // The question that asked before this was the caller's; no second
    // confirm bar. A pane whose agent already exited just goes.
    terminals.close(tab.id, true);
    return this.openSession({
      ref,
      cwd: tab.cwd || tab.dir || '',
      title: tab.label,
      createdAt: '',
      updatedAt: '',
      counts: { user: 0, assistant: 0, toolCalls: 0 },
      tokens: { input: 0, output: 0 },
    });
  }

  /**
   * Refuses to run a session in a terminal while a chat drives it, and
   * says so. True when refused.
   */
  refuseTerminal(ref: SessionRef): boolean {
    if (!chats.forSession(ref)) return false;
    toast.show({
      title: 'This session is open in a chat',
      body: 'A session runs in one place at a time. End the chat, then resume the session in a terminal.',
      actions: [
        {
          label: 'Show chat',
          onSelect: () => {
            void appState.selectSession(ref, 'push');
            this.focus();
          },
        },
      ],
    });
    return true;
  }

  // A terminal tab runs the session: says so. True when refused.
  private refuseChat(ref: SessionRef): boolean {
    const tab = terminals.tabFor(ref, link.dataHost ?? '');
    if (!tab) return false;
    toast.show({
      title: 'This session runs in a terminal',
      body: 'A session runs in one place at a time. Quit the agent in its terminal tab, then open the session in a chat.',
      actions: [{ label: 'Show terminal', onSelect: () => terminals.show(tab.id) }],
    });
    return true;
  }

  // Runs start, and says why it failed. again opens the same session again,
  // offered when the agent says another of its clients has the session.
  private async run(start: () => Promise<void>, again?: () => Promise<boolean>): Promise<boolean> {
    if (this.opening) return false;
    this.opening = true;
    try {
      await start();
      this.focus();
      return true;
    } catch (err) {
      if (isStaleReply(err)) return false;
      if (again && isSessionInUseError(err)) {
        toast.show({
          title: 'Codex is still using this session',
          body: 'Codex keeps a session open for about a minute after its CLI, app or IDE extension closes it, and refuses the chat until then. Close it there, or try again in a minute.',
          tone: 'error',
          actions: [{ label: 'Try again', onSelect: () => void again() }],
        });
      } else {
        toast.show({ title: 'Could not start the chat', body: errorText(err, 'The agent did not start.'), tone: 'error' });
      }
      return false;
    } finally {
      this.opening = false;
    }
  }
}

export const chatOpen = new ChatOpener();
