// A scripted fake ACP agent for the mock backend: it streams thoughts and
// text, runs tool calls, asks for permission and keeps a plan, through the
// same chat:update and chat:exit events the app's chat manager sends.
import type {
  AcpSessionUpdate,
  AgentID,
  ChatAgent,
  ChatConfigOption,
  ChatPermission,
  ChatStartRequest,
  ChatState,
  ChatUpdateEvent,
  Message,
  SessionRef,
} from '../types.js';

const AGENTS: { agent: AgentID; name: string; install: string; adapter: string }[] = [
  { agent: 'claude-code', name: 'Claude Code', install: 'npm install -g @agentclientprotocol/claude-agent-acp', adapter: 'claude-agent-acp' },
  { agent: 'codex', name: 'Codex', install: 'npm install -g @agentclientprotocol/codex-acp', adapter: 'codex-acp' },
  { agent: 'opencode', name: 'OpenCode', install: 'npm install -g opencode-ai', adapter: 'opencode' },
];

export const CHAT_REMOTE_ERROR = "chat works for this computer's sessions only; connected hosts do not support it yet";

interface MockChat {
  state: ChatState;
  seq: number;
  turn: { cancelled: boolean } | null;
  permission: { request: ChatPermission; answer: (optionId: string) => void } | null;
  nextPerm: number;
  nextMessage: number;
  closed: boolean;
}

export interface MockChatHost {
  emit(name: string, payload: unknown): void;
  /** False while the window shows a host: chats run locally only. */
  local(): boolean;
  /** Saves a finished turn's messages to the session, as an agent does. */
  save(ref: SessionRef, dir: string, messages: Message[]): void;
}

/**
 * Prompts steer the script:
 * - "permission" or "approve": asks before writing a file;
 * - "run": runs a command, its output in _meta as Codex sends it;
 * - "fail": the turn fails; "login": the turn needs a login;
 * - "crash": the agent dies;
 * - "hang": the turn runs until cancelled.
 */
export class MockChats {
  /** The pause between two streamed steps; 0 in tests. */
  stepMs = 0;
  /** Agents whose adapter the mock reports missing. */
  missing = new Set<AgentID>();
  private chats = new Map<string, MockChat>();
  private nextChat = 1;
  private host: MockChatHost;

  constructor(host: MockChatHost) {
    this.host = host;
  }

  async agents(): Promise<ChatAgent[]> {
    this.requireLocal();
    return AGENTS.map((a) =>
      this.missing.has(a.agent)
        ? { agent: a.agent, name: a.name, available: false, install: a.install, problem: `${a.adapter} was not found on PATH` }
        : { agent: a.agent, name: a.name, available: true, path: `/usr/local/bin/${a.adapter}`, install: a.install },
    );
  }

  async start(req: ChatStartRequest): Promise<ChatState> {
    this.requireLocal();
    const a = AGENTS.find((x) => x.agent === req.agent);
    if (!a) throw new Error(`chat: ${req.agent} has no ACP adapter`);
    if (this.missing.has(req.agent)) throw new Error(`chat: ${a.adapter} was not found on PATH`);
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
        working: false,
      },
      seq: 0,
      turn: null,
      permission: null,
      nextPerm: 0,
      nextMessage: 0,
      closed: false,
    };
    this.chats.set(chat.state.chatId, chat);
    // As from the backend, the started event comes before the reply.
    this.update(chat, { kind: 'started', state: structuredClone(chat.state) });
    return structuredClone(chat.state);
  }

  async prompt(chatId: string, text: string): Promise<void> {
    this.requireLocal();
    const chat = this.get(chatId);
    if (chat.turn) throw new Error('chat: a turn is already running');
    const turn = { cancelled: false };
    chat.turn = turn;
    chat.state.working = true;
    this.update(chat, { kind: 'turn_start' });
    void this.run(chat, turn, text);
  }

  async cancel(chatId: string): Promise<void> {
    const chat = this.get(chatId);
    if (chat.turn) chat.turn.cancelled = true;
    chat.permission?.answer('');
  }

  async respond(chatId: string, permissionId: string, optionId: string): Promise<void> {
    const chat = this.get(chatId);
    if (chat.permission?.request.id !== permissionId) throw new Error(`the permission request is no longer pending: ${permissionId}`);
    chat.permission.answer(optionId);
  }

  async setOption(chatId: string, configId: string, value: unknown): Promise<ChatConfigOption[]> {
    this.requireLocal();
    const chat = this.get(chatId);
    const opt = chat.state.configOptions.find((o) => o.id === configId);
    if (!opt || !opt.options?.some((o) => o.value === value)) throw new Error(`chat: invalid value for ${configId}`);
    opt.currentValue = value;
    this.update(chat, { kind: 'config_options', configOptions: structuredClone(chat.state.configOptions) });
    return structuredClone(chat.state.configOptions);
  }

  async close(chatId: string): Promise<void> {
    const chat = this.get(chatId);
    this.end(chat, 'closed');
  }

  async list(): Promise<ChatState[]> {
    return [...this.chats.values()].map((c) => structuredClone({ ...c.state, permissions: c.permission ? [c.permission.request] : [] }));
  }

  /** Ends every chat, as a host switch or the window closing does. */
  closeAll(): void {
    for (const chat of [...this.chats.values()]) this.end(chat, 'closed');
  }

  private requireLocal(): void {
    if (!this.host.local()) throw new Error(CHAT_REMOTE_ERROR);
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

  private session(chat: MockChat, update: AcpSessionUpdate): void {
    this.update(chat, { kind: 'session_update', update });
  }

  private end(chat: MockChat, reason: 'closed' | 'crashed', stderrTail?: string): void {
    if (chat.closed) return;
    chat.permission?.answer('');
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

  private async stream(chat: MockChat, turn: { cancelled: boolean }, kind: 'agent_message_chunk' | 'agent_thought_chunk', messageId: string, text: string): Promise<void> {
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

  private async run(chat: MockChat, turn: { cancelled: boolean }, text: string): Promise<void> {
    const n = ++chat.nextMessage;
    const msg = `msg_mock_${chat.state.chatId}_${n}`;
    const tool = `toolu_mock_${chat.state.chatId}_${n}`;
    const saved: Message[] = [{ id: `user_mock_${chat.state.chatId}_${n}`, role: 'user', time: new Date().toISOString(), parts: [{ kind: 'text', text }] }];
    const reply: Message = { id: msg, role: 'assistant', time: new Date().toISOString(), model: 'mock-large', parts: [] };
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

      if (/hang/i.test(text)) {
        while (!turn.cancelled) await new Promise((resolve) => setTimeout(resolve, Math.max(this.stepMs, 10)));
      }
      this.session(chat, plan(2));
      const answer = `Done. You said "${text}", and the mock project has a README. This reply streams a word at a time, as an agent's would.`;
      await this.stream(chat, turn, 'agent_message_chunk', msg, answer);
      if (turn.cancelled) {
        stopReason = 'cancelled';
        return;
      }
      reply.parts.push({ kind: 'text', text: answer });
      this.session(chat, plan(3));
    } finally {
      if (!chat.closed) {
        chat.turn = null;
        chat.state.working = false;
        if (turn.cancelled) stopReason = 'cancelled';
        if (reply.parts.length > 0) saved.push(reply);
        if (!error) this.host.save(chat.state.session, chat.state.dir, saved);
        this.update(chat, error ? { kind: 'turn_end', error, ...(authMethods && { authMethods }) } : { kind: 'turn_end', stopReason });
      }
    }
  }
}
