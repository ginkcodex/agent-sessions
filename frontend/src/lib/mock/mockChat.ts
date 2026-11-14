// A scripted fake ACP agent for the mock backend: it streams thoughts and
// text, runs tool calls, asks for permission and keeps a plan, through the
// same chat:update and chat:exit events the app's chat manager sends.
import type {
  AcpSessionUpdate,
  AgentID,
  ChatAgent,
  ChatAttachment,
  ChatAttachResult,
  ChatConfigOption,
  ChatPlugins,
  ChatDraft,
  ChatPermission,
  ChatPromptResult,
  ChatQuestion,
  ChatQuestionField,
  ChatQueuedPrompt,
  ChatStartRequest,
  ChatState,
  ChatUpdateEvent,
  Message,
  SessionRef,
} from '../types.js';
import { sniffImageFormat } from '../images';

// The app installs its own copy of an adapter with a managed entry, at
// version; older is the version an earlier release would have left.
const AGENTS: { agent: AgentID; name: string; adapter: string; managed?: { version: string; older: string; main: string } }[] = [
  {
    agent: 'claude-code',
    name: 'Claude Code',
    adapter: 'claude-agent-acp',
    managed: { version: '0.106.0', older: '0.105.2', main: 'node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js' },
  },
  {
    agent: 'codex',
    name: 'Codex',
    adapter: 'codex-acp',
    managed: { version: '2.6.0', older: '2.5.1', main: 'node_modules/@agentclientprotocol/codex-acp/dist/index.js' },
  },
  { agent: 'opencode', name: 'OpenCode', adapter: 'opencode' },
];

/** Where the mock "installs" the app's own adapters. */
export const MOCK_MANAGED_TOOLS = '/home/mock/.local/share/agent-sessions/managed-tools';

/** The adapters the app can install, by catalog ID: their agent and version. */
export function mockManagedAdapter(tool: string): { agent: AgentID; version: string; package: string } | undefined {
  const a = AGENTS.find((x) => x.managed && x.adapter === tool);
  return a?.managed && { agent: a.agent, version: a.managed.version, package: `@agentclientprotocol/${a.adapter}` };
}

/** A made-up 24×24 gradient, the image every mock pick and drop attaches. */
export const MOCK_IMAGE_PNG = 'iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAIAAABvFaqvAAAEl0lEQVR42g3TER/3IBAH8OflPDgcDofD4XDwQDAIgiAIgiAIgiA4CIIgCILgYBAM/riX8gw6/H7ud3f9+Utxon2mbaF1pWWjeafpoPGkQGig1HPqJLWaGku1pwqoTFQUyhtlSOmg14+Sl/7785fhxPrM2sLqysrG8s7SweLJgLBAmefMSWY1M5ZpzxQwmZgojDfGkNHBrh8jL/sgjhPvM28LrysvG887TwePJwfCA+Wecye51dxYrj1XwGXionDeOENOB79+nLz8gwROos+iLaKuomwi7yIdIp4CiAhUeC6cFFYLY4X2QoGQSYgieBMMBR3i+gnyig+SOMk+y7bIusqyybzLdMh4SiAyUOm5dFJaLY2V2ksFUiYpiuRNMpR0yOsnySs/SOGk+qzaouqqyqbyrtKh4qmAqEDVl8ZJZbUyVmmvFCiZlCiKN8VQ0aGunyKv+iCNk+6zbouuqy6bzrtOh46nBqLDtyiundRWa/M9rxVombQomjfNUNOhr58mr/4gg5Pps2mLqaspm8m7SYeJpwFiAjWeG/fF0sZYo71RYGQyohjeDENDh7l+hrzmgyxOts+2Lbautmw27zYdNp4WiA3Uem6dtFZb8xVvFViZrCiWN8vQ0mGvnyWv/SCHk+uza4urqyuby7tLh4unA+ICdZ47J53VzlinvVPgZHKiON4cQ0eHu36OvO6DPE6+z74tvq6+bD7vPh0+nh6ID9R77p30XzPGeu29Ai+TF8Xz5hl6Ovz18+T1HxRwCn0ObQl1DWULeQ/pCPEMQEKgwfPgZLA6GBu0DwqCTEGUwFtgGOgI1y+QN3wQ4AR9hrZAXaFskHdIB8QTgECg4PkXBqwGY0F7UAAygSjAGzAEOuD6AXnhgyJOsc+xLbGusWwx7zEdMZ4RSAw0eh6djFZHY6P2UUGUKYoSeYsMIx3x+kXyxg9KOKU+p7akuqaypbyndKR4JiAp0OR5+sZrdTI2aZ8UJJmSKIm3xDDRka5fIm/6oIxT7nNuS65rLlvOe05HjmcGkgPNnmcns9XZ2Kx9VpBlyqJk3jLDTEe+fpm8+YMKTqXPpS2lrqVsJe8lHSWeBUgJ9FtPcbJYXYwt2hcFRaYiSuGtMCx0lOtXyFs+qOJU+1zbUutay1bzXtNR41mB1ECr59XJanU1tmpfFVSZqiiVt8qw0lGvXyVv/aCGU+tza0uraytby3tLR4tnA9ICbd/BONmsbsY27ZuCJlMTpfHWGDY62vVr5G0f1HHqfe5t6XXtZet57+no8exAeqDd8+5kt7ob27XvCrpMXZTOW2fY6ejXr5O3fxDihH3GtmBdsWyYd0wHxhOBfAeHnqOTaDUai9qjApQJRUHekCHSgdcPyYsfdON09/luy13Xu2x33u903PG8gdyB3p7fTt5W38be2t8KbpluUW7eboY3Hff1u8l7f9DAafR5tGXUdZRt5H2kY8RzABnfF/B8ODmsHsYO7YeCIdMQZfA2GA46xvUb5B0f9OD09Plpy1PXp2xP3p90PPF8gDyBPp4/Tj5WP8Y+2j8KHpkeUR7eHoYPHc/1e8j7/PsPnsD5Lu/zGO8AAAAASUVORK5CYII=';

const MOCK_MAX_ATTACHMENTS = 10;

/**
 * Files as the app prepares them, made up from their names: images by
 * their extension get MOCK_IMAGE_PNG, documents and archives are refused,
 * and anything else is a text file. Nothing is read.
 */
export function mockAttachFiles(paths: string[]): ChatAttachResult {
  const out: ChatAttachResult = { attachments: [], skipped: [] };
  for (const path of paths) {
    const name = path.split(/[\\/]/).pop() || path;
    if (out.attachments.length === MOCK_MAX_ATTACHMENTS) {
      out.skipped.push({ name, reason: `a message can carry at most ${MOCK_MAX_ATTACHMENTS} attachments` });
    } else if (/\.(png|jpe?g|gif|webp)$/i.test(name)) {
      out.attachments.push({ kind: 'image', name, mimeType: 'image/png', size: base64Size(MOCK_IMAGE_PNG), data: MOCK_IMAGE_PNG, uri: `file://${path}` });
    } else if (/\.(pdf|zip|gz|tar|exe|bin|iso|docx?|xlsx?)$/i.test(name)) {
      out.skipped.push({ name, reason: 'is neither an image nor a text file; only images and text files can be attached' });
    } else {
      const text = `Made-up contents of ${name}, from the mock backend.\n`;
      out.attachments.push({ kind: 'text', name, mimeType: 'text/plain', size: text.length, text, uri: `file://${path}` });
    }
  }
  return out;
}

/** A pasted image as the app prepares it. */
export function mockAttachImage(name: string, data: string): ChatAttachment {
  const label = name || 'Pasted image';
  const format = sniffImageFormat(data);
  if (!format) throw new Error(`${label}: is not a PNG, JPEG, GIF or WebP image`);
  return { kind: 'image', name: label, mimeType: `image/${format}`, size: base64Size(data), data };
}

function base64Size(data: string): number {
  return Math.floor((data.length * 3) / 4) - (data.endsWith('==') ? 2 : data.endsWith('=') ? 1 : 0);
}

export const CHAT_REMOTE_ERROR = "this host's agent-sessions does not support chat; reconnect to update it";

/** A running turn: Stop cancels it; Send now adds steered messages to it. */
interface MockTurn {
  cancelled: boolean;
  steered: MockQueued[];
}

interface MockQueued {
  info: ChatQueuedPrompt;
  text: string;
  attachments: ChatAttachment[];
}

interface MockChat {
  state: ChatState;
  seq: number;
  turn: MockTurn | null;
  /** The messages sent while a turn ran, in sending order. */
  queue: MockQueued[];
  nextQueued: number;
  permission: { request: ChatPermission; answer: (optionId: string) => void } | null;
  nextPerm: number;
  /** The window shows questions (ChatStartRequest.questions). */
  questions: boolean;
  question: { request: ChatQuestion; answer: (answer: MockAnswer) => void } | null;
  nextQuestion: number;
  nextMessage: number;
  closed: boolean;
}

/** How a mock question ended. */
interface MockAnswer {
  outcome: 'accept' | 'decline' | 'cancel' | 'withdrawn';
  content?: Record<string, unknown>;
}

export interface MockChatHost {
  emit(name: string, payload: unknown): void;
  /** False while the window shows a host whose server has no chats, or none. */
  available(): boolean;
  /** Saves a finished turn's messages to the session, as an agent does. */
  save(ref: SessionRef, dir: string, messages: Message[]): void;
}

/**
 * Prompts steer the script:
 * - "permission" or "approve": asks before writing a file;
 * - "run": runs a command, its output in _meta as Codex sends it;
 * - "fail": the turn fails; "login": the turn needs a login;
 * - "crash": the agent dies;
 * - "hang": the turn runs until cancelled, or until Send now steers a
 *   message into it;
 * - "question": the agent asks the user, with a form shaped like
 *   its adapter's; "withdraw": it asks, then goes on without the answer
 *   after withdrawMs, as Codex's auto-resolution does; "async": Codex asks
 *   without waiting, keeps streaming, and withdraws the question when the
 *   turn ends, as its async questions do.
 * The reply says how many files the prompt carried, and which messages
 * Send now steered into the turn.
 *
 * As the engine does, a message sent during a turn is queued; the queue
 * runs after a turn that ends normally and pauses after any other end.
 */
export class MockChats {
  /** The pause between two streamed steps; 0 in tests. */
  stepMs = 0;
  /** How long a "withdraw" question waits before the agent withdraws it. */
  withdrawMs = 8000;
  /** Agents whose adapter the mock reports missing. */
  missing = new Set<AgentID>();
  /** Agents whose app copy is an older version, until it is installed. */
  outdated = new Set<AgentID>();
  /** Agents whose adapter the app has installed for itself since. */
  installed = new Set<AgentID>();
  /** Agents whose adapter cannot take a message during a turn. */
  noSteer = new Set<AgentID>();
  private chats = new Map<string, MockChat>();
  private nextChat = 1;
  private host: MockChatHost;

  constructor(host: MockChatHost) {
    this.host = host;
  }

  async agents(): Promise<ChatAgent[]> {
    this.requireAvailable();
    return AGENTS.map((a): ChatAgent => {
      const base = { agent: a.agent, name: a.name };
      if (!a.managed) {
        if (this.missing.has(a.agent)) return { ...base, available: false, problem: `${a.adapter} was not found on PATH or in its usual install locations` };
        return { ...base, available: true, path: `/usr/local/bin/${a.adapter}` };
      }
      const m = { installTool: a.adapter, pinned: a.managed.version };
      if (this.missing.has(a.agent) && !this.installed.has(a.agent)) return { ...base, ...m, available: false, problem: `${a.adapter} is not installed` };
      const version = this.outdated.has(a.agent) && !this.installed.has(a.agent) ? a.managed.older : a.managed.version;
      const path = `${MOCK_MANAGED_TOOLS}/${a.adapter}-${version}-linux-x64/${a.managed.main}`;
      return { ...base, ...m, available: true, path, managed: true, version, outdated: version !== a.managed.version };
    });
  }

  async start(req: ChatStartRequest): Promise<ChatState> {
    this.requireAvailable();
    const a = AGENTS.find((x) => x.agent === req.agent);
    if (!a) throw new Error(`chat: ${req.agent} has no ACP adapter`);
    if (this.missing.has(req.agent) && !this.installed.has(req.agent)) throw new Error(`chat adapter not found: ${a.adapter} is not installed. Set it up in Settings › Coding agents`);
    if (!req.dir) throw new Error('chat: no directory');
    if (req.resume && [...this.chats.values()].some((c) => c.state.session.agent === req.resume!.agent && c.state.session.id === req.resume!.id)) {
      throw new Error(`the session is already open in a chat: ${req.resume.agent}:${req.resume.id}`);
    }
    const n = this.nextChat++;
    const chat: MockChat = {
      state: {
        chatId: `chat-${n}`,
        agent: req.agent,
        dir: req.dir,
        // A copy: what the UI passes may be reactive state, which does not clone.
        session: req.resume ? { agent: req.resume.agent, id: req.resume.id } : { agent: req.agent, id: `mock-chat-${n}-${Date.now()}` },
        agentInfo: { name: a.adapter, version: '0.0.0-mock' },
        configOptions: [
          { id: 'mode', name: 'Mode', category: 'mode', type: 'select', currentValue: 'ask', options: [{ value: 'ask', name: 'Ask' }, { value: 'auto', name: 'Auto' }] },
          { id: 'model', name: 'Model', category: 'model', type: 'select', currentValue: 'mock-large', options: [{ value: 'mock-large', name: 'Mock Large' }, { value: 'mock-small', name: 'Mock Small' }] },
        ],
        promptCapabilities: { image: true, embeddedContext: true },
        working: false,
        canQueue: true,
        canSteer: !this.noSteer.has(req.agent),
      },
      seq: 0,
      turn: null,
      queue: [],
      nextQueued: 0,
      permission: null,
      nextPerm: 0,
      questions: !!req.questions,
      question: null,
      nextQuestion: 0,
      nextMessage: 0,
      closed: false,
    };
    this.chats.set(chat.state.chatId, chat);
    // As from the backend, the started event comes before the reply.
    this.update(chat, { kind: 'started', state: structuredClone(chat.state) });
    return structuredClone(chat.state);
  }

  /** Starts a turn, or queues the message while one runs. */
  async prompt(chatId: string, text: string, attachments: ChatAttachment[] = []): Promise<ChatPromptResult> {
    this.requireAvailable();
    const chat = this.get(chatId);
    if (!text && attachments.length === 0) throw new Error('the message is empty');
    if (attachments.length > MOCK_MAX_ATTACHMENTS) throw new Error(`a message can carry at most ${MOCK_MAX_ATTACHMENTS} attachments`);
    // A copy: what the UI passes may be reactive state, which does not clone.
    const atts = attachments.map((a) => ({ ...a }));
    if (chat.turn) {
      const id = `queued-${++chat.nextQueued}`;
      const info: ChatQueuedPrompt = {
        id,
        ...(text && { text }),
        ...(atts.length > 0 && { attachments: atts.map((a) => ({ kind: a.kind, name: a.name, mimeType: a.mimeType, size: a.size })) }),
      };
      chat.queue.push({ info, text, attachments: atts });
      this.queueChanged(chat);
      return { queued: id };
    }
    this.startTurn(chat, text, atts);
    return {};
  }

  /**
   * Sends a queued message now: it starts a turn when none runs, and joins
   * the running one otherwise.
   */
  async sendNow(chatId: string, queuedId: string): Promise<void> {
    this.requireAvailable();
    const chat = this.get(chatId);
    const i = chat.queue.findIndex((q) => q.info.id === queuedId);
    if (i < 0) throw new Error(`the message is no longer queued: ${queuedId}`);
    if (chat.turn && !chat.state.canSteer) throw new Error('the agent cannot take a message while it works; it is sent when the turn ends');
    const [q] = chat.queue.splice(i, 1);
    this.queueChanged(chat);
    if (!chat.turn) {
      this.startTurn(chat, q.text, q.attachments, q.info);
      return;
    }
    chat.turn.steered.push(q);
    this.update(chat, { kind: 'steered', prompt: structuredClone(q.info) });
  }

  /** Takes a message back from the queue, to edit it. */
  async unqueue(chatId: string, queuedId: string): Promise<ChatDraft> {
    const chat = this.get(chatId);
    const i = chat.queue.findIndex((q) => q.info.id === queuedId);
    if (i < 0) throw new Error(`the message is no longer queued: ${queuedId}`);
    const [q] = chat.queue.splice(i, 1);
    this.queueChanged(chat);
    return { ...(q.text && { text: q.text }), ...(q.attachments.length > 0 && { attachments: q.attachments }) };
  }

  async cancel(chatId: string): Promise<void> {
    const chat = this.get(chatId);
    if (chat.turn) chat.turn.cancelled = true;
    chat.permission?.answer('');
    chat.question?.answer({ outcome: 'cancel' });
  }

  async respond(chatId: string, permissionId: string, optionId: string): Promise<void> {
    const chat = this.get(chatId);
    if (chat.permission?.request.id !== permissionId) throw new Error(`the permission request is no longer pending: ${permissionId}`);
    chat.permission.answer(optionId);
  }

  async answer(chatId: string, questionId: string, action: string, content?: Record<string, unknown>): Promise<void> {
    const chat = this.get(chatId);
    const q = chat.question;
    if (q?.request.id !== questionId) throw new Error(`the question is no longer pending: ${questionId}`);
    if (action !== 'accept' && action !== 'decline' && action !== 'cancel') throw new Error(`chat: unknown answer action ${JSON.stringify(action)}`);
    q.answer(action === 'accept' ? { outcome: action, content: checkAnswer(q.request, content ?? {}) } : { outcome: action });
  }

  async setOption(chatId: string, configId: string, value: unknown): Promise<ChatConfigOption[]> {
    this.requireAvailable();
    const chat = this.get(chatId);
    const opt = chat.state.configOptions.find((o) => o.id === configId);
    if (!opt || !opt.options?.some((o) => o.value === value)) throw new Error(`chat: invalid value for ${configId}`);
    opt.currentValue = value;
    this.update(chat, { kind: 'config_options', configOptions: structuredClone(chat.state.configOptions) });
    return structuredClone(chat.state.configOptions);
  }

  /** Claude's and Codex's CLIs list their plugins; OpenCode's cannot. */
  async plugins(chatId: string): Promise<ChatPlugins> {
    this.requireAvailable();
    const chat = this.get(chatId);
    if (chat.state.agent === 'opencode') return { supported: false, plugins: [] };
    return {
      supported: true,
      plugins: [
        { id: 'ponytail@ponytail', name: 'ponytail', marketplace: 'ponytail', version: '5.3.3', scope: 'user', enabled: true },
        { id: 'gopls-lsp@claude-plugins-official', name: 'gopls-lsp', marketplace: 'claude-plugins-official', version: '1.0.0', scope: 'user', enabled: false },
      ],
    };
  }

  async close(chatId: string): Promise<void> {
    const chat = this.get(chatId);
    this.end(chat, 'closed');
  }

  async list(): Promise<ChatState[]> {
    return [...this.chats.values()].map((c) =>
      structuredClone({
        ...c.state,
        permissions: c.permission ? [c.permission.request] : [],
        questions: c.question ? [c.question.request] : [],
        ...(c.queue.length > 0 && { queue: c.queue.map((q) => q.info) }),
      }),
    );
  }

  /** Ends every chat, as a host switch or the window closing does. */
  closeAll(): void {
    for (const chat of [...this.chats.values()]) this.end(chat, 'closed');
  }

  private requireAvailable(): void {
    if (!this.host.available()) throw new Error(CHAT_REMOTE_ERROR);
  }

  private get(chatId: string): MockChat {
    const chat = this.chats.get(chatId);
    if (!chat) throw new Error(`chat not found: ${chatId}`);
    return chat;
  }

  private update(chat: MockChat, fields: Omit<ChatUpdateEvent, 'chatId' | 'seq'>): void {
    if (chat.closed) return;
    this.host.emit('chat:update', { chatId: chat.state.chatId, seq: ++chat.seq, ...fields });
  }

  private queueChanged(chat: MockChat): void {
    this.update(chat, { kind: 'queue', ...(chat.queue.length > 0 && { queue: structuredClone(chat.queue.map((q) => q.info)) }) });
  }

  /** prompt is the queued message the turn sends, if it came from the queue. */
  private startTurn(chat: MockChat, text: string, attachments: ChatAttachment[], prompt?: ChatQueuedPrompt): void {
    const turn: MockTurn = { cancelled: false, steered: [] };
    chat.turn = turn;
    chat.state.working = true;
    this.update(chat, { kind: 'turn_start', ...(prompt && { prompt: structuredClone(prompt) }) });
    void this.run(chat, turn, text, attachments);
  }

  private session(chat: MockChat, update: AcpSessionUpdate): void {
    this.update(chat, { kind: 'session_update', update });
  }

  private end(chat: MockChat, reason: 'closed' | 'crashed', stderrTail?: string): void {
    if (chat.closed) return;
    chat.permission?.answer('');
    chat.question?.answer({ outcome: 'cancel' });
    if (chat.turn) chat.turn.cancelled = true;
    this.chats.delete(chat.state.chatId);
    this.host.emit('chat:exit', {
      chatId: chat.state.chatId,
      seq: ++chat.seq,
      reason,
      ...(reason === 'crashed' && { error: 'exit status 1' }),
      ...(stderrTail && { stderrTail }),
    });
    chat.closed = true;
  }

  private pause(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, this.stepMs));
  }

  private async stream(chat: MockChat, turn: MockTurn, kind: 'agent_message_chunk' | 'agent_thought_chunk', messageId: string, text: string): Promise<void> {
    for (const word of text.match(/\S+\s*/g) ?? []) {
      if (turn.cancelled) return;
      this.session(chat, { sessionUpdate: kind, messageId, content: { type: 'text', text: word } });
      await this.pause();
    }
  }

  private ask(chat: MockChat, toolCall: ChatPermission['toolCall']): Promise<string> {
    const id = `perm-${++chat.nextPerm}`;
    const permission: ChatPermission = {
      id,
      toolCall,
      options: [
        { optionId: 'allow-once', name: 'Allow', kind: 'allow_once' },
        { optionId: 'allow-always', name: 'Always allow edits', kind: 'allow_always' },
        { optionId: 'reject', name: 'Reject', kind: 'reject_once' },
      ],
    };
    return new Promise((resolve) => {
      chat.permission = {
        request: permission,
        answer: (optionId) => {
          if (chat.permission?.request.id !== id) return;
          chat.permission = null;
          this.update(chat, { kind: 'permission_resolved', permission: { id, outcome: optionId ? 'selected' : 'cancelled', ...(optionId && { optionId }) } });
          resolve(optionId);
        },
      };
      this.update(chat, { kind: 'permission', permission });
    });
  }

  /** Asks a question and waits for its answer; withdrawMs withdraws it after that long. */
  private askQuestion(chat: MockChat, question: Omit<ChatQuestion, 'id'>, withdrawMs?: number): Promise<MockAnswer> {
    const id = `question-${++chat.nextQuestion}`;
    const request: ChatQuestion = { id, ...question };
    return new Promise((resolve) => {
      let timer: ReturnType<typeof setTimeout> | undefined;
      chat.question = {
        request,
        answer: (a) => {
          if (chat.question?.request.id !== id) return;
          chat.question = null;
          if (timer) clearTimeout(timer);
          this.update(chat, { kind: 'question_resolved', question: { id, outcome: a.outcome, ...(a.content && { content: a.content }) } });
          resolve(a);
        },
      };
      this.update(chat, { kind: 'question', question: request });
      if (withdrawMs !== undefined) timer = setTimeout(() => chat.question?.request.id === id && chat.question.answer({ outcome: 'withdrawn' }), withdrawMs);
    });
  }

  private async run(chat: MockChat, turn: MockTurn, text: string, attachments: ChatAttachment[]): Promise<void> {
    const n = ++chat.nextMessage;
    const msg = `msg_mock_${chat.state.chatId}_${n}`;
    const tool = `toolu_mock_${chat.state.chatId}_${n}`;
    // The agent saves the files with the prompt; the mock keeps an image's
    // bytes in the part, where an agent's log would have them.
    const saved: Message[] = [{ id: `user_mock_${chat.state.chatId}_${n}`, role: 'user', time: new Date().toISOString(), parts: userParts(text, attachments) }];
    // The turn runs on the model the session has now, as an agent's does.
    const model = chat.state.configOptions.find((o) => o.id === 'model')?.currentValue;
    const reply: Message = { id: msg, role: 'assistant', time: new Date().toISOString(), model: typeof model === 'string' ? model : 'mock-large', parts: [] };
    let stopReason = 'end_turn';
    let error: string | undefined;
    let authMethods: ChatUpdateEvent['authMethods'];
    try {
      await this.pause();
      if (/crash/i.test(text)) {
        this.end(chat, 'crashed', 'mock agent: panic: the mock was asked to crash\n');
        return;
      }
      if (/login/i.test(text)) {
        error = 'Authentication required: provider authentication required';
        authMethods = [{ id: 'mock-login', name: 'Log in', description: 'Run `mock auth login` in the terminal' }];
        return;
      }
      if (/fail/i.test(text)) {
        error = 'Internal error: the mock was asked to fail';
        return;
      }
      const plan = (done: number) => ({
        sessionUpdate: 'plan',
        entries: ['Read the project', 'Make the change', 'Report back'].map((content, i) => ({ content, priority: 'medium', status: i < done ? 'completed' : i === done ? 'in_progress' : 'pending' })),
      });
      this.session(chat, plan(0));
      const thought = `The user asked: "${text}". Let me look at the project first.`;
      await this.stream(chat, turn, 'agent_thought_chunk', msg, thought);
      reply.parts.push({ kind: 'reasoning', text: thought });

      this.session(chat, { sessionUpdate: 'tool_call', toolCallId: `${tool}_read`, name: 'Read', title: 'Read README.md', kind: 'read', status: 'pending', locations: [{ path: `${chat.state.dir}/README.md` }], rawInput: {} });
      await this.pause();
      this.session(chat, { sessionUpdate: 'tool_call_update', toolCallId: `${tool}_read`, status: 'in_progress', rawInput: { file_path: `${chat.state.dir}/README.md` } });
      await this.pause();
      const readme = '# Mock project\n\nA project the mock agent reads.\n';
      this.session(chat, { sessionUpdate: 'tool_call_update', toolCallId: `${tool}_read`, status: 'completed', content: [{ type: 'content', content: { type: 'text', text: readme } }] });
      reply.parts.push({ kind: 'tool', tool: { id: `${tool}_read`, name: 'Read', input: { file_path: `${chat.state.dir}/README.md` }, output: readme, status: 'completed' } });
      this.session(chat, plan(1));

      if (/run/i.test(text)) {
        const exec = `exec-mock-${chat.state.chatId}-${n}`;
        const command = 'ls';
        this.session(chat, { sessionUpdate: 'tool_call', toolCallId: exec, name: 'exec_command', title: command, kind: 'execute', status: 'in_progress', content: [{ type: 'terminal', terminalId: exec }], rawInput: { command } });
        await this.pause();
        const out = 'README.md\nmain.go\n';
        this.session(chat, { sessionUpdate: 'tool_call_update', toolCallId: exec, status: 'completed', _meta: { terminal_output_delta: { data: out, terminal_id: exec }, terminal_exit: { exit_code: 0, signal: null, terminal_id: exec } } });
        // Codex saves the command under another ID.
        reply.parts.push({ kind: 'tool', tool: { id: `call_mock_${n}`, name: 'exec_command', input: { command }, output: out, status: 'completed' } });
      }

      if (/permission|approve/i.test(text)) {
        const path = `${chat.state.dir}/NOTES.md`;
        const write = `${tool}_write`;
        this.session(chat, { sessionUpdate: 'tool_call', toolCallId: write, name: 'Write', title: 'Write NOTES.md', kind: 'edit', status: 'pending', locations: [{ path }], rawInput: { file_path: path } });
        const content = '# Notes\n\nWritten by the mock agent.\n';
        const answer = await this.ask(chat, {
          toolCallId: write,
          title: 'Write NOTES.md',
          kind: 'edit',
          status: 'pending',
          rawInput: { file_path: path, content },
          content: [{ type: 'diff', path, oldText: null, newText: content }],
        });
        if (turn.cancelled || !answer) {
          stopReason = 'cancelled';
          return;
        }
        if (answer === 'reject') {
          this.session(chat, { sessionUpdate: 'tool_call_update', toolCallId: write, status: 'failed', content: [{ type: 'content', content: { type: 'text', text: 'The user declined this tool call' } }] });
          reply.parts.push({ kind: 'tool', tool: { id: write, name: 'Write', input: { file_path: path }, output: 'The user declined this tool call', status: 'error' } });
        } else {
          this.session(chat, { sessionUpdate: 'tool_call_update', toolCallId: write, status: 'completed', content: [{ type: 'diff', path, oldText: null, newText: content }] });
          reply.parts.push({ kind: 'tool', tool: { id: write, name: 'Write', input: { file_path: path, content }, output: 'File created', status: 'completed' } });
        }
      }

      // A late answer (QuestionCard's "Send as message") is a plain prompt.
      const lateAnswer = /^My answers? to your questions?:/.test(text);
      if (!lateAnswer && /async/i.test(text) && chat.questions) {
        // Codex's request_user_input_async: the turn goes on, and the adapter
        // withdraws the question when the turn completes.
        let got: MockAnswer | undefined;
        void this.askQuestion(chat, mockAsyncQuestion()).then((a) => (got = a));
        await this.stream(chat, turn, 'agent_message_chunk', msg, 'I asked which approach you prefer, and I keep working while you answer: reading the module, checking its tests, and drafting both approaches. ');
        if (turn.cancelled) {
          stopReason = 'cancelled';
          return;
        }
        chat.question?.answer({ outcome: 'withdrawn' });
        const said = got?.outcome === 'accept' ? `You answered ${JSON.stringify(got.content ?? {})} in time.` : 'No answer came before I finished.';
        await this.stream(chat, turn, 'agent_message_chunk', msg, said + ' ');
        reply.parts.push({ kind: 'text', text: said });
      } else if (!lateAnswer && /question|withdraw/i.test(text)) {
        const ask = `${tool}_ask`;
        let said: string;
        if (!chat.questions) {
          // Each adapter's fallback when the client shows no forms.
          said = 'This window does not show questions, so I went on with the recommended approach.';
        } else {
          const withdraw = /withdraw/i.test(text);
          const form = mockQuestion(chat.state.agent, ask, withdraw ? this.withdrawMs : undefined);
          const a = await this.askQuestion(chat, form, withdraw ? this.withdrawMs : undefined);
          if (turn.cancelled || a.outcome === 'cancel') {
            stopReason = 'cancelled';
            return;
          }
          said =
            a.outcome === 'accept'
              ? `You answered ${JSON.stringify(a.content ?? {})}.`
              : a.outcome === 'decline'
                ? 'You skipped the question, so I chose for you.'
                : 'Nobody answered in time, so I went on with the recommended approach.';
          reply.parts.push({ kind: 'tool', tool: { id: ask, name: 'AskUserQuestion', input: { message: form.message }, output: said, status: 'completed' } });
        }
        await this.stream(chat, turn, 'agent_message_chunk', msg, said + ' ');
        reply.parts.push({ kind: 'text', text: said });
      }

      if (/hang/i.test(text)) {
        while (!turn.cancelled && turn.steered.length === 0) await new Promise((resolve) => setTimeout(resolve, Math.max(this.stepMs, 10)));
      }
      this.session(chat, plan(2));
      const files = `${attachments.length} file${attachments.length === 1 ? '' : 's'} (${attachments.map((a) => a.name).join(', ')})`;
      const said = attachments.length === 0 ? `You said "${text}"` : text ? `You said "${text}", with ${files} attached` : `You sent ${files}`;
      const also = turn.steered.map((q) => ` While I worked you added "${q.text || q.attachments.map((a) => a.name).join(', ')}".`).join('');
      const answer = `Done. ${said}, and the mock project has a README.${also} This reply streams a word at a time, as an agent's would.`;
      await this.stream(chat, turn, 'agent_message_chunk', msg, answer);
      if (turn.cancelled) {
        stopReason = 'cancelled';
        return;
      }
      reply.parts.push({ kind: 'text', text: answer });
      this.session(chat, plan(3));
      // As the adapters do after each reply: the context the session uses.
      this.session(chat, { sessionUpdate: 'usage_update', used: 12000 + saved.length * 4200, size: 200000 });
    } finally {
      if (!chat.closed) {
        chat.turn = null;
        chat.state.working = false;
        if (turn.cancelled) stopReason = 'cancelled';
        // The agent saves a steered message after the prompt it joined.
        for (const [i, q] of turn.steered.entries()) {
          saved.push({ id: `user_mock_${chat.state.chatId}_${n}_steered_${i + 1}`, role: 'user', time: new Date().toISOString(), parts: userParts(q.text, q.attachments) });
        }
        if (reply.parts.length > 0) saved.push(reply);
        if (!error) this.host.save(chat.state.session, chat.state.dir, saved);
        this.update(chat, error ? { kind: 'turn_end', error, ...(authMethods && { authMethods }) } : { kind: 'turn_end', stopReason });
        // The queue goes on after a normal end, and pauses after any other.
        const next = !error && stopReason === 'end_turn' ? chat.queue.shift() : undefined;
        if (next) {
          this.queueChanged(chat);
          this.startTurn(chat, next.text, next.attachments, next.info);
        }
      }
    }
  }
}

/** A user message's parts as the agent saves them. */
function userParts(text: string, attachments: ChatAttachment[]): Message['parts'] {
  return [
    ...(text ? [{ kind: 'text' as const, text }] : []),
    ...attachments.map((a): Message['parts'][number] => ({ kind: 'file', file: { name: a.name, mime: a.mimeType, size: a.size, ...(a.data && { data: a.data }) } })),
  ];
}

const APPROACHES = [
  { const: 'Extend existing module (Recommended)', title: 'Extend existing module (Recommended)', description: 'Add the setting to src/config.ts, next to the others.' },
  { const: 'New module', title: 'New module', description: 'Put it in its own file and import it where it is needed.' },
  { const: 'Rewrite the loader', title: 'Rewrite the loader', description: 'Replace how settings are read; a larger change.' },
];
const EXTRAS = [
  { const: 'Unit tests', title: 'Unit tests', description: 'Cover the new setting.' },
  { const: 'Documentation', title: 'Documentation', description: 'Describe it in the README.' },
  { const: 'Changelog entry', title: 'Changelog entry' },
];
const OVERWRITE = 'This will overwrite src/config.ts. Continue?';

/** A made-up Codex async question, as codex-acp asks it (docs/plan/chat-questions-adapters.md). */
function mockAsyncQuestion(): Omit<ChatQuestion, 'id'> {
  const meta = (extra: Record<string, unknown>) => ({ codex: { isSecret: false, ...extra } });
  return {
    message: 'Codex keeps working while you answer. An answer sent after it finishes is not delivered.',
    fields: [
      { key: 'question_0', type: 'string', title: 'Which implementation approach should I take?', oneOf: [...APPROACHES, { const: 'Other', title: 'Other', description: 'Provide a different answer in the note field.' }], _meta: meta({ isOther: true }) },
      { key: 'question_0_note', type: 'string', title: 'Additional answer or note', _meta: meta({ questionId: 'question_0', role: 'user_note' }) },
    ],
    meta: { codex: { delivery: 'async' } },
  };
}

/** A made-up question in the shape the agent's adapter sends (docs/plan/chat-questions-adapters.md). */
function mockQuestion(agent: AgentID, toolCallId: string, withdrawMs?: number): Omit<ChatQuestion, 'id'> {
  const codex = (extra: Record<string, unknown>) => ({ codex: { isSecret: false, ...extra } });
  switch (agent) {
    case 'codex': {
      const other = { const: 'Other', title: 'Other', description: 'Provide a different answer in the note field.' };
      const note = (questionId: string): ChatQuestionField => ({ key: `${questionId}_note`, type: 'string', title: 'Additional answer or note', _meta: codex({ questionId, role: 'user_note' }) });
      return {
        toolCallId,
        message: 'Codex needs your input to continue.',
        fields: [
          { key: 'approach', type: 'string', title: 'Which implementation approach should I take?', description: 'Approach', oneOf: [...APPROACHES, other], _meta: codex({ isOther: true }) },
          note('approach'),
          { key: 'confirm', type: 'string', title: OVERWRITE, description: 'Confirm', oneOf: [{ const: 'Yes', title: 'Yes' }, { const: 'No', title: 'No' }, other], _meta: codex({ isOther: true }) },
          note('confirm'),
        ],
        required: ['approach', 'confirm'],
        ...(withdrawMs !== undefined && { meta: { codex: { autoResolutionMs: withdrawMs } } }),
      };
    }
    case 'opencode':
      return {
        toolCallId,
        message: 'Answer the questions to continue.',
        fields: [
          { key: 'approach', type: 'string', title: 'Approach', description: 'Which implementation approach should I take?', oneOf: APPROACHES, default: APPROACHES[0].const },
          { key: 'extras', type: 'array', title: 'Extras', description: 'Which extras should I include?', items: { anyOf: EXTRAS } },
          { key: 'extras_custom', type: 'string', title: 'Extras (other)', description: 'Add your own answer' },
          { key: 'overwrite', type: 'boolean', title: 'Overwrite', description: OVERWRITE },
        ],
        required: ['approach', 'overwrite'],
      };
    default: {
      const custom = (n: number): ChatQuestionField => ({ key: `question_${n}_custom`, type: 'string', title: 'Other', description: 'Type your own answer' });
      return {
        toolCallId,
        message: 'Please answer the following questions.',
        fields: [
          { key: 'question_0', type: 'string', title: 'Implementation approach', description: 'Which implementation approach should I take?', oneOf: APPROACHES },
          custom(0),
          { key: 'question_1', type: 'array', title: 'Extras to include', description: 'Which extras should I include?', items: { anyOf: EXTRAS } },
          custom(1),
          { key: 'question_2', type: 'string', title: 'Confirm destructive action', description: OVERWRITE, oneOf: [{ const: 'Yes, overwrite', title: 'Yes, overwrite' }, { const: 'No, keep it', title: 'No, keep it' }] },
          custom(2),
        ],
      };
    }
  }
}

function optionValues(f: ChatQuestionField): string[] {
  const opts = f.type === 'array' ? (f.items?.anyOf?.map((o) => o.const) ?? f.items?.enum) : (f.oneOf?.map((o) => o.const) ?? f.enum);
  return opts ?? [];
}

/** The answer as the backend checks it: known fields, valid options, required answers. Empty values are no answer. */
function checkAnswer(q: ChatQuestion, content: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, v] of Object.entries(content)) {
    const f = q.fields?.find((x) => x.key === key);
    if (!f) throw new Error(`the form has no field "${key}"`);
    if (v === null || v === undefined || v === '') continue;
    const name = JSON.stringify(f.title || key);
    const opts = optionValues(f);
    if (f.type === 'string' && (typeof v !== 'string' || (opts.length > 0 && !opts.includes(v)))) throw new Error(`${name}: ${JSON.stringify(v)} is not one of the options`);
    if (f.type === 'array' && (!Array.isArray(v) || v.some((x) => !opts.includes(x)))) throw new Error(`${name}: the answer is not a list of options`);
    if (f.type === 'boolean' && typeof v !== 'boolean') throw new Error(`${name}: the answer is not yes or no`);
    if ((f.type === 'number' || f.type === 'integer') && typeof v !== 'number') throw new Error(`${name}: the answer is not a number`);
    out[key] = v;
  }
  for (const key of q.required ?? []) {
    if (!(key in out)) throw new Error(`${JSON.stringify(q.fields?.find((x) => x.key === key)?.title || key)} needs an answer`);
  }
  return out;
}
