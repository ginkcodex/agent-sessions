import { api, subscribeChatExit, subscribeChatUpdate } from '../api';
import type {
  ChatAgent,
  ChatAnswerAction,
  ChatAttachment,
  ChatAuthMethod,
  ChatConfigOption,
  ChatExitEvent,
  ChatPermission,
  ChatQuestion,
  ChatStartRequest,
  ChatState,
  ChatUpdateEvent,
  Message,
  SessionRef,
} from '../types';
import { epoch, isStaleReply } from '../link';
import {
  addAnswer,
  applySessionUpdate,
  beginTurn,
  emptyTail,
  endTurn,
  newSequence,
  reconcile,
  sequence,
  type ChatSequence,
  type ChatTail,
  type TailPrompt,
} from '../chatTail';
import { questionForm, summary } from '../chatQuestions';

/**
 * starting: the agent is starting, or loading the session;
 * idle: waiting for a prompt; working: a turn runs; exited: the chat ended.
 */
export type ChatPhase = 'starting' | 'idle' | 'working' | 'exited';

type ChatEvent = (ChatUpdateEvent & { exit?: undefined }) | { seq: number; chatId: string; exit: ChatExitEvent };

/** A question on a chat's card. */
export type ChatPendingQuestion = ChatQuestion & {
  /** The agent went on without the answer: it can only go as a message. */
  late?: boolean;
};

export interface Chat {
  /** This store's key for the chat, set before the backend names it. */
  key: string;
  /** The backend's ID; empty while starting. */
  chatId: string;
  agent: ChatStartRequest['agent'];
  dir: string;
  /** The session the chat drives; the one to resume while starting a resume. */
  session?: SessionRef;
  agentInfo?: ChatState['agentInfo'];
  configOptions: ChatConfigOption[];
  /** What a prompt may carry besides text; unknown while starting and from an older host. */
  promptCapabilities?: ChatState['promptCapabilities'];
  phase: ChatPhase;
  tail: ChatTail;
  /** Permission requests waiting for an answer, oldest first. */
  permissions: ChatPermission[];
  /**
   * The agent's questions waiting for an answer, oldest first, then the late
   * ones it went on without, whose answers can still go as a message.
   */
  questions: ChatPendingQuestion[];
  /** How the last turn ended: end_turn, cancelled, max_tokens… */
  stopReason?: string;
  /** Why the start or the last turn failed. */
  error?: string;
  /** Ways to log in, when the agent needs one. */
  authMethods?: ChatAuthMethod[];
  /** How the chat ended. */
  exit?: ChatExitEvent;
  /** The connection epoch the chat belongs to. */
  epoch: number;
}

interface Internal {
  seq: ChatSequence<ChatEvent>;
  /** The prompt sent and waiting for its turn_start. */
  prompt?: TailPrompt;
  /** The questions asked and not yet resolved, by ID: the resolution reads its answer through the form. */
  asked: Map<string, ChatQuestion>;
}

/** Events kept for chats whose start has not answered yet. */
const MAX_EARLY_EVENTS = 1000;

/**
 * The window's chats: their state, the permission requests waiting on the
 * user, and the live tail the agent streams. Events for chats the store does
 * not know, or from before the last host switch, are ignored.
 */
export class ChatsStore {
  chats = $state<Chat[]>([]);
  agents = $state<ChatAgent[]>([]);
  agentsError = $state<string | null>(null);
  agentsLoading = $state(false);

  private internal = new Map<string, Internal>();
  private nextKey = 1;
  /** Starts waiting for their reply. */
  private starting = 0;
  /** Events that came before their chat's start answered, by chat ID. */
  private early = new Map<string, ChatEvent[]>();
  private earlyCount = 0;
  private unsubscribe: (() => void) | null = null;
  private lifecycle = 0;

  /** Subscribes to chat events and finds the chats a reloaded page left open. */
  async init(): Promise<void> {
    if (this.unsubscribe) return;
    const lifecycle = ++this.lifecycle;
    const offUpdate = subscribeChatUpdate((e) => this.receive({ ...e, exit: undefined }));
    const offExit = subscribeChatExit((e) => this.receive({ seq: e.seq, chatId: e.chatId, exit: e }));
    this.unsubscribe = () => {
      offUpdate();
      offExit();
    };
    let open: ChatState[];
    try {
      open = await api.chatList();
    } catch {
      return;
    }
    if (lifecycle !== this.lifecycle) return;
    for (const st of open) {
      if (this.byChatId(st.chatId)) continue;
      // The seq the chat is at is not known: its next event sets it. The
      // requests still waiting come back as cards.
      const questions = st.questions ?? [];
      const key = this.add(st.agent, st.dir, { chatId: st.chatId, phase: st.working ? 'working' : 'idle', permissions: st.permissions ?? [], questions }, newSequence(), st);
      for (const q of questions) this.internal.get(key)?.asked.set(q.id, q);
    }
  }

  destroy(): void {
    this.lifecycle++;
    this.unsubscribe?.();
    this.unsubscribe = null;
  }

  /**
   * The window's connection changed. Another host's chats, or this
   * computer's, are forgotten. When the same host reconnects (sameHost), its
   * chats ended with the old connection: they stay, exited, so they can
   * restart.
   */
  reset(sameHost = false): void {
    // A chat still starting is refused when its start answers.
    const ended = (c: Chat): Chat =>
      c.phase === 'exited' ? c : { ...c, phase: 'exited', exit: { chatId: c.chatId, seq: 0, reason: 'disconnected' }, permissions: [], questions: [], tail: endTurn(c.tail) };
    this.chats = sameHost ? this.chats.filter((c) => c.phase !== 'starting').map((c) => ({ ...ended(c), epoch: epoch() })) : [];
    this.internal.clear();
    this.early.clear();
    this.earlyCount = 0;
    this.agents = [];
    this.agentsError = null;
  }

  byChatId(chatId: string): Chat | undefined {
    return chatId ? this.chats.find((c) => c.chatId === chatId) : undefined;
  }

  byKey(key: string): Chat | undefined {
    return this.chats.find((c) => c.key === key);
  }

  /** The chat that drives the session, if one does. */
  forSession(ref: SessionRef): Chat | undefined {
    return this.chats.find((c) => c.phase !== 'exited' && c.session?.agent === ref.agent && c.session.id === ref.id);
  }

  /**
   * The chat the session's view shows: the one that drives it, else the
   * last one that ended and was not dismissed.
   */
  viewFor(ref: SessionRef): Chat | undefined {
    const live = this.forSession(ref);
    if (live) return live;
    for (let i = this.chats.length - 1; i >= 0; i--) {
      const c = this.chats[i];
      if (c.session?.agent === ref.agent && c.session.id === ref.id) return c;
    }
    return undefined;
  }

  async loadAgents(): Promise<ChatAgent[]> {
    const owner = epoch();
    this.agentsLoading = true;
    try {
      const agents = await api.chatAgents();
      if (owner === epoch()) {
        this.agents = agents;
        this.agentsError = null;
      }
      return agents;
    } catch (e) {
      if (owner === epoch() && !isStaleReply(e)) this.agentsError = errorText(e);
      throw e;
    } finally {
      if (owner === epoch()) this.agentsLoading = false;
    }
  }

  /**
   * Starts a chat. The chat shows as starting at once; it is removed again
   * when the start fails, and the error is thrown.
   */
  async start(req: ChatStartRequest): Promise<Chat> {
    const key = this.add(req.agent, req.dir, { chatId: '', phase: 'starting', session: req.resume }, newSequence(1));
    this.starting++;
    let st: ChatState;
    try {
      st = await api.chatStart(req);
    } catch (e) {
      this.remove(key);
      throw e;
    } finally {
      this.starting--;
    }
    const chat = this.byKey(key);
    if (!chat || chat.epoch !== epoch()) {
      // The window moved to another host while the agent started. The
      // backend ends this computer's chats on that switch; make sure.
      this.remove(key);
      void api.chatClose(st.chatId).catch(() => {});
      throw new Error('The connection changed while the chat started.');
    }
    this.update(key, (c) => ({ ...this.withState(c, st), phase: c.phase === 'starting' ? (st.working ? 'working' : 'idle') : c.phase }));
    const early = this.early.get(st.chatId) ?? [];
    this.early.delete(st.chatId);
    this.earlyCount -= early.length;
    for (const e of early) this.receive(e);
    if (this.starting === 0) this.dropEarly();
    return this.byKey(key)!;
  }

  /**
   * Sends a prompt, its text and attached files. The chat shows as working
   * until the turn ends; the live tail shows the files, an image from its
   * bytes, until the saved message does.
   */
  async prompt(chatId: string, text: string, attachments: ChatAttachment[] = []): Promise<void> {
    const chat = this.byChatId(chatId);
    const state = this.internal.get(chat?.key ?? '');
    if (!chat || !state) throw new Error('The chat is not open.');
    const prompt: TailPrompt = {
      text,
      sentAt: Date.now(),
      ...(attachments.length > 0 && { attachments: attachments.map((a) => ({ name: a.name, mimeType: a.mimeType, size: a.size, ...(a.kind === 'image' && a.data && { data: a.data }) })) }),
    };
    state.prompt = prompt;
    this.update(chat.key, (c) => ({ ...c, phase: c.phase === 'idle' ? 'working' : c.phase, error: undefined, authMethods: undefined }));
    try {
      await api.chatPrompt(chatId, text, attachments);
    } catch (e) {
      // The turn did not start: no turn_start came for this prompt.
      if (state.prompt === prompt) state.prompt = undefined;
      this.update(chat.key, (c) => (c.phase === 'working' && !c.tail.turnOpen ? { ...c, phase: 'idle' } : c));
      throw e;
    }
  }

  cancel(chatId: string): Promise<void> {
    return api.chatCancel(chatId);
  }

  /** Answers a permission request; an empty optionId answers cancelled. */
  async respond(chatId: string, permissionId: string, optionId: string): Promise<void> {
    await api.chatRespond(chatId, permissionId, optionId);
    // The resolution also arrives as an event; drop the card now.
    const chat = this.byChatId(chatId);
    if (chat) this.update(chat.key, (c) => ({ ...c, permissions: c.permissions.filter((p) => p.id !== permissionId) }));
  }

  /**
   * Answers the agent's question: accept with the content built from the
   * draft, or decline (Skip) or cancel. An answer the form refuses throws,
   * and the card stays to fix it.
   */
  async answer(chatId: string, questionId: string, action: ChatAnswerAction, content?: Record<string, unknown>): Promise<void> {
    await api.chatAnswer(chatId, questionId, action, content);
    // The resolution also arrives as an event, with the summary; drop the card now.
    const chat = this.byChatId(chatId);
    if (chat) this.update(chat.key, (c) => ({ ...c, questions: c.questions.filter((q) => q.id !== questionId) }));
  }

  /**
   * Sends the answers to a question the agent went on without as the next
   * prompt, then drops its card. The turn must have ended.
   */
  async sendLate(chatId: string, questionId: string, text: string): Promise<void> {
    await this.prompt(chatId, text);
    this.dismissLate(chatId, questionId);
  }

  /** Drops the card of a question the agent went on without. */
  dismissLate(chatId: string, questionId: string): void {
    const chat = this.byChatId(chatId);
    if (chat) this.update(chat.key, (c) => ({ ...c, questions: c.questions.filter((q) => !(q.late && q.id === questionId)) }));
  }

  async setOption(chatId: string, configId: string, value: unknown): Promise<ChatConfigOption[]> {
    const options = await api.chatSetOption(chatId, configId, value);
    const chat = this.byChatId(chatId);
    if (chat && options.length > 0) this.update(chat.key, (c) => ({ ...c, configOptions: options }));
    return options;
  }

  /** Ends the chat's agent. The chat stays, exited, until dismissed. */
  close(chatId: string): Promise<void> {
    return api.chatClose(chatId);
  }

  /** Removes an exited chat. */
  dismiss(key: string): void {
    if (this.byKey(key)?.phase === 'exited') this.remove(key);
  }

  /** Drops the live items that saved messages now show. */
  reconcile(chatId: string, saved: readonly Message[]): void {
    const chat = this.byChatId(chatId);
    if (!chat) return;
    const tail = reconcile(chat.tail, saved);
    if (tail !== chat.tail) this.update(chat.key, (c) => ({ ...c, tail }));
  }

  private add(agent: Chat['agent'], dir: string, fields: Pick<Chat, 'chatId' | 'phase'> & Partial<Chat>, seq: ChatSequence<ChatEvent>, st?: ChatState): string {
    const key = `chat-${this.nextKey++}`;
    let chat: Chat = { key, agent, dir, configOptions: [], tail: emptyTail(), permissions: [], questions: [], epoch: epoch(), ...fields };
    if (st) chat = this.withState(chat, st);
    this.internal.set(key, { seq, asked: new Map() });
    this.chats = [...this.chats, chat];
    return key;
  }

  private remove(key: string): void {
    this.internal.delete(key);
    this.chats = this.chats.filter((c) => c.key !== key);
  }

  private update(key: string, fn: (c: Chat) => Chat): void {
    this.chats = this.chats.map((c) => (c.key === key ? fn(c) : c));
  }

  private withState(c: Chat, st: ChatState): Chat {
    return {
      ...c,
      chatId: st.chatId,
      agent: st.agent,
      dir: st.dir,
      session: st.session,
      agentInfo: st.agentInfo,
      configOptions: st.configOptions ?? [],
      promptCapabilities: st.promptCapabilities,
    };
  }

  private receive(event: ChatEvent): void {
    // Chats from before a host switch are gone.
    const stale = this.chats.filter((c) => c.epoch !== epoch());
    for (const c of stale) this.remove(c.key);

    const chat = this.byChatId(event.chatId);
    const state = chat && this.internal.get(chat.key);
    if (!chat || !state) {
      this.keepEarly(event);
      return;
    }
    const { seq, ready } = sequence(state.seq, event);
    state.seq = seq;
    for (const e of ready) this.apply(chat.key, state, e);
  }

  // An event can come before the reply of the start that made its chat.
  // Others are for chats this store does not know: ignored.
  private keepEarly(event: ChatEvent): void {
    if (this.starting === 0 || this.earlyCount >= MAX_EARLY_EVENTS) return;
    const list = this.early.get(event.chatId) ?? [];
    list.push(event);
    this.early.set(event.chatId, list);
    this.earlyCount++;
  }

  private dropEarly(): void {
    this.early.clear();
    this.earlyCount = 0;
  }

  private apply(key: string, state: Internal, e: ChatEvent): void {
    if (e.exit) {
      const exit = e.exit;
      state.prompt = undefined;
      state.asked.clear();
      this.update(key, (c) => ({ ...c, phase: 'exited', exit, permissions: [], questions: [], tail: endTurn(c.tail) }));
      return;
    }
    switch (e.kind) {
      case 'started':
        if (e.state) {
          const st = e.state;
          this.update(key, (c) => ({ ...this.withState(c, st), phase: c.phase === 'starting' ? (st.working ? 'working' : 'idle') : c.phase }));
        }
        return;
      case 'session_update': {
        const u = e.update;
        this.update(key, (c) => {
          const tail = applySessionUpdate(c.tail, u);
          return tail === c.tail ? c : { ...c, tail };
        });
        return;
      }
      case 'permission': {
        const p = e.permission;
        if (!p?.id) return;
        this.update(key, (c) => (c.permissions.some((q) => q.id === p.id) ? c : { ...c, permissions: [...c.permissions, p] }));
        return;
      }
      case 'permission_resolved': {
        const id = e.permission?.id;
        if (!id) return;
        this.update(key, (c) => ({ ...c, permissions: c.permissions.filter((q) => q.id !== id) }));
        return;
      }
      case 'question': {
        const q = e.question;
        if (!q?.id || state.asked.has(q.id)) return;
        state.asked.set(q.id, q);
        this.update(key, (c) => ({ ...c, questions: [...c.questions, q] }));
        return;
      }
      case 'question_resolved': {
        const id = e.question?.id;
        if (!id) return;
        const asked = state.asked.get(id);
        state.asked.delete(id);
        const outcome = e.question?.outcome;
        // A cancelled question goes with its turn; the others leave a line.
        // One the agent went on without stays, late, so the user can still
        // send the answers as a message.
        const line = asked && outcome && outcome !== 'cancel' ? summary(questionForm(asked), outcome, e.question?.content) : undefined;
        this.update(key, (c) => ({
          ...c,
          questions:
            outcome === 'withdrawn' ? c.questions.map((q) => (q.id === id ? { ...q, late: true } : q)) : c.questions.filter((q) => q.id !== id),
          tail: line ? addAnswer(c.tail, { questionId: id, ...(asked?.toolCallId && { toolCallId: asked.toolCallId }), outcome: outcome!, summary: line }) : c.tail,
        }));
        return;
      }
      case 'turn_start': {
        const prompt = state.prompt;
        state.prompt = undefined;
        this.update(key, (c) => ({
          ...c,
          phase: c.phase === 'exited' ? c.phase : 'working',
          tail: beginTurn(c.tail, prompt),
          stopReason: undefined,
          error: undefined,
          authMethods: undefined,
        }));
        return;
      }
      case 'turn_end':
        // Permission requests end with their turn; the backend has answered
        // them. A question goes only with its question_resolved: the agent
        // may still wait for it (Codex's async questions outlive the prompt
        // until the adapter withdraws them), and a late one stays.
        this.update(key, (c) => ({
          ...c,
          phase: c.phase === 'exited' ? c.phase : 'idle',
          tail: endTurn(c.tail),
          permissions: [],
          stopReason: e.stopReason || undefined,
          error: e.error || undefined,
          authMethods: e.authMethods?.length ? e.authMethods : undefined,
        }));
        return;
      case 'config_options':
        if (e.configOptions) {
          const options = e.configOptions;
          this.update(key, (c) => ({ ...c, configOptions: options }));
        }
        return;
      default:
        // A kind from a newer backend.
        return;
    }
  }
}

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export const chats = new ChatsStore();
