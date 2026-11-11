import { untrack } from 'svelte';
import type { AgentID, SessionMeta, SessionRef } from '../types';
import { isStaleReply } from '../link';
import { errorText } from '../manage';
import { chatAgentAvailable } from '../chat';
import { appState } from './appState.svelte';
import { chats } from './chats.svelte';
import { connectionStore } from './connection.svelte';
import { link } from './link.svelte';
import { terminals } from './terminals.svelte';
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
    const tab = terminals.tabFor(ref, link.dataHost ?? '');
    if (tab) {
      toast.show({
        title: 'This session runs in a terminal',
        body: 'A session runs in one place at a time. Quit the agent in its terminal tab, then open the session in a chat.',
        actions: [{ label: 'Show terminal', onSelect: () => terminals.show(tab.id) }],
      });
      return false;
    }
    return this.run(async () => {
      await chats.start({ agent: ref.agent, dir: meta.cwd, resume: { agent: ref.agent, id: ref.id } });
    });
  }

  /** Starts a chat with a new session of agent in dir, and shows it. */
  startNew(agent: AgentID, dir: string): Promise<boolean> {
    return this.run(async () => {
      const chat = await chats.start({ agent, dir });
      if (chat.session) await appState.selectSession(chat.session, 'push');
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

  private async run(start: () => Promise<void>): Promise<boolean> {
    if (this.opening) return false;
    this.opening = true;
    try {
      await start();
      this.focus();
      return true;
    } catch (err) {
      if (!isStaleReply(err)) toast.show({ title: 'Could not start the chat', body: errorText(err, 'The agent did not start.'), tone: 'error' });
      return false;
    } finally {
      this.opening = false;
    }
  }
}

export const chatOpen = new ChatOpener();
