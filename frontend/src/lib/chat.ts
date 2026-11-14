// What the chat UI shows: the live tail as transcript messages, and which
// agents can chat. Every function here is pure.
import type { AgentID, ChatAgent, ChatAttachment, ChatAttachResult, ChatCommand, ChatPermission, ChatPlugin, ChatQueuedPrompt, Message, Part, SessionMeta, SessionRef, ToolStatus } from './types';
import { toolOutput, type ChatTail, type TailAnswer, type TailDiff, type TailItem, type TailTool } from './chatTail';

/**
 * What the session header shows for a new chat's session before its agent
 * saves it: the agent, the directory and "New <agent> chat", with no counts,
 * tokens or times yet. SessionHeader takes it with pending set.
 */
export function pendingChatMeta(ref: SessionRef, chat: { dir: string; phase: string }, agentName: string): SessionMeta {
  return {
    ref,
    cwd: chat.dir,
    title: `New ${agentName} chat`,
    createdAt: '',
    updatedAt: '',
    counts: { user: 0, assistant: 0, toolCalls: 0 },
    tokens: { input: 0, output: 0 },
    live: chat.phase !== 'exited',
  };
}

/** A piece of the live tail: a transcript message, or how a question ended. */
export type TailBlock = { kind: 'message'; message: Message } | { kind: 'answer'; answer: TailAnswer };

/**
 * The tail as transcript messages, for MessageBubble: each prompt is a user
 * message, and the agent's items up to the next prompt one assistant message.
 * IDs start with "live:" so they never meet a saved message's.
 */
export function tailMessages(tail: ChatTail): Message[] {
  return tailBlocks(tail).flatMap((b) => (b.kind === 'message' ? [b.message] : []));
}

/**
 * The tail as tailMessages has it, with each question's outcome as a line of
 * its own where it happened: what the agent says after it is a new message.
 */
export function tailBlocks(tail: ChatTail): TailBlock[] {
  const out: TailBlock[] = [];
  let reply: Message | null = null;
  let replyTurn = -1;
  for (const it of tail.items) {
    if (it.type === 'answer') {
      reply = null;
      out.push({ kind: 'answer', answer: it });
      continue;
    }
    if (it.type === 'text' && it.role === 'user') {
      reply = null;
      const message: Message = {
        id: `live:${it.key}`,
        role: 'user',
        time: it.sentAt ? new Date(it.sentAt).toISOString() : '',
        parts: [
          ...(it.text ? [{ kind: 'text' as const, text: it.text }] : []),
          ...(it.attachments ?? []).map((a): Part => ({ kind: 'file', file: { name: a.name, mime: a.mimeType, size: a.size, ...(a.data && { data: a.data }) } })),
        ],
      };
      out.push({ kind: 'message', message });
      continue;
    }
    if (!reply || replyTurn !== it.turn) {
      reply = { id: `live:${it.key}`, role: 'assistant', time: '', parts: [] };
      replyTurn = it.turn;
      out.push({ kind: 'message', message: reply });
    }
    reply.parts.push(...itemParts(it));
  }
  return out;
}

function itemParts(it: Exclude<TailItem, TailAnswer>): Part[] {
  switch (it.type) {
    case 'text':
      return [{ kind: 'text', text: it.text }];
    case 'thought':
      return [{ kind: 'reasoning', text: it.text }];
    case 'plan':
      return [{ kind: 'text', text: planMarkdown(it.entries) }];
    case 'tool': {
      const parts: Part[] = [{ kind: 'tool', tool: { id: it.toolCallId, name: toolName(it), input: it.input, output: toolOutputText(it), status: toolStatus(it.status) } }];
      if (it.diffs.length > 0) parts.push({ kind: 'patch', files: it.diffs.map((d) => d.path), text: it.diffs.map(unifiedDiff).join('\n') });
      return parts;
    }
  }
}

function toolName(t: TailTool): string {
  return t.title || t.name || t.kind || 'Tool';
}

/** ACP's tool statuses as the transcript's. */
export function toolStatus(status: string): ToolStatus {
  switch (status) {
    case 'completed':
      return 'completed';
    case 'failed':
      return 'error';
    case 'pending':
    case 'in_progress':
      return 'pending';
    default:
      return 'unknown';
  }
}

function toolOutputText(t: TailTool): string {
  const out = toolOutput(t);
  const code = t.terminal?.exitCode;
  if (typeof code === 'number' && code !== 0) return `${out}${out && !out.endsWith('\n') ? '\n' : ''}Exit code ${code}`;
  return out;
}

const PLAN_MARK: Record<string, string> = { completed: '✓', in_progress: '▸', pending: '○' };

/** The agent's plan as a Markdown list, each entry marked by its status. */
export function planMarkdown(entries: { content: string; status?: string }[]): string {
  const lines = entries.map((e) => `- ${PLAN_MARK[e.status ?? ''] ?? '○'} ${e.content.replace(/\s*\n\s*/g, ' ')}`);
  return ['**Plan**', '', ...lines].join('\n');
}

/** A diff of whole texts, as the patch card shows: old lines out, new lines in. */
export function unifiedDiff(d: TailDiff): string {
  const lines = (text: string) => (text === '' ? [] : text.replace(/\n$/, '').split('\n'));
  const before = d.oldText === null ? [] : lines(d.oldText);
  const after = lines(d.newText);
  const head = [`--- ${d.oldText === null ? '/dev/null' : d.path}`, `+++ ${d.path}`, `@@ -${before.length ? 1 : 0},${before.length} +${after.length ? 1 : 0},${after.length} @@`];
  return [...head, ...before.map((l) => `-${l}`), ...after.map((l) => `+${l}`)].join('\n');
}

/** The agents whose adapter the host has, by ID. */
export function chatAgentAvailable(agents: readonly ChatAgent[], agent: AgentID): boolean {
  return agents.some((a) => a.agent === agent && a.available);
}


/** What a permission request's option does, by its ACP kind. */
export interface PermissionChoice {
  optionId: string;
  /** The agent's own words for the option. */
  name: string;
  /** The kind in words; empty for a kind the app does not know. */
  kindLabel: string;
  tone: 'allow' | 'reject' | 'other';
  /** An allow_always choice: it lasts beyond this chat. */
  lasting: boolean;
}

/** A permission request as its card shows it. */
export interface PermissionView {
  title: string;
  kind: string;
  /** The command the tool runs, when it runs one. */
  command: string;
  /** Unified diffs of the files the tool would change. */
  diffs: { path: string; text: string }[];
  /** The files the tool touches, when it shows no diff of them. */
  paths: string[];
  choices: PermissionChoice[];
}

const KIND_LABELS: Record<string, { label: string; tone: PermissionChoice['tone'] }> = {
  allow_once: { label: 'Allow once', tone: 'allow' },
  allow_always: { label: 'Always allow', tone: 'allow' },
  reject_once: { label: 'Reject once', tone: 'reject' },
  reject_always: { label: 'Always reject', tone: 'reject' },
};

/** The parts of a permission request a card shows. Options keep the agent's order. */
export function permissionView(p: ChatPermission): PermissionView {
  const tool = p.toolCall;
  const diffs = (tool?.content ?? [])
    .filter((c) => c.type === 'diff' && typeof c.path === 'string' && typeof c.newText === 'string')
    .map((c) => ({ path: c.path!, text: unifiedDiff({ path: c.path!, oldText: c.oldText ?? null, newText: c.newText! }) }));
  const shown = new Set(diffs.map((d) => d.path));
  const paths = [...new Set((tool?.locations ?? []).map((l) => l.path).filter((path) => path && !shown.has(path)))];
  return {
    title: tool?.title || tool?.name || 'A tool call',
    kind: tool?.kind ?? '',
    command: commandText(tool?.rawInput),
    diffs,
    paths,
    choices: (p.options ?? []).map((o) => {
      const known = KIND_LABELS[o.kind];
      return { optionId: o.optionId, name: o.name || known?.label || o.optionId, kindLabel: known?.label ?? '', tone: known?.tone ?? 'other', lasting: o.kind === 'allow_always' };
    }),
  };
}

// Claude and OpenCode send the command as a string; Codex as argv.
function commandText(input: unknown): string {
  if (!input || typeof input !== 'object') return '';
  const command = (input as { command?: unknown }).command;
  if (typeof command === 'string') return command;
  if (Array.isArray(command) && command.every((a) => typeof a === 'string')) return command.map(shellWord).join(' ');
  return '';
}

function shellWord(arg: string): string {
  return /^[\w@%+=:,./-]+$/.test(arg) ? arg : `'${arg.replace(/'/g, `'\\''`)}'`;
}

/** The most files one message may carry, as the backend checks. */
export const MAX_CHAT_ATTACHMENTS = 10;

/**
 * The composer's files with newly prepared ones added, in order: images are
 * skipped for an agent that takes none, and files past the limit, each with
 * why, after those the backend skipped.
 */
export function addAttachments(
  current: readonly ChatAttachment[],
  result: ChatAttachResult,
  agent: { name: string; images: boolean },
): ChatAttachResult {
  const attachments = [...current];
  const skipped = [...result.skipped];
  for (const a of result.attachments) {
    if (a.kind === 'image' && !agent.images) skipped.push({ name: a.name, reason: `${agent.name} does not take images` });
    else if (attachments.length >= MAX_CHAT_ATTACHMENTS) skipped.push({ name: a.name, reason: `a message can carry at most ${MAX_CHAT_ATTACHMENTS} attachments` });
    else attachments.push(a);
  }
  return { attachments, skipped };
}

/** The toast for files that could not be attached, or null when none. */
export function skippedToast(skipped: ChatAttachResult['skipped']): { title: string; body: string } | null {
  if (skipped.length === 0) return null;
  if (skipped.length === 1) {
    const { name, reason } = skipped[0];
    return { title: `Could not attach ${name}`, body: reason.charAt(0).toUpperCase() + reason.slice(1) };
  }
  return { title: `Could not attach ${skipped.length} files`, body: skipped.map((s) => `${s.name}: ${s.reason}`).join('; ') };
}

/**
 * Whether the composer can send: text or a file, to an idle chat, or to a
 * working one that queues it (queues).
 */
export function canSendPrompt(phase: string, sending: boolean, text: string, attachments: number, queues = false): boolean {
  return (phase === 'idle' || (phase === 'working' && queues)) && !sending && (text.trim() !== '' || attachments > 0);
}

/** A command the composer's picker offers: the text it inserts, and what the agent says of it. */
export interface CommandChoice {
  text: string;
  description: string;
  hint: string;
}

/**
 * The agent's advertised commands that the composer's text, while it is one
 * word after / or $ (Codex's skills), could name. A name matches by its start
 * or by the part after its plugin prefix (/understand finds
 * /understand-anything:understand), and every match is listed, so a short
 * name two plugins share is the user's choice. An exact name comes first.
 * The text inserted is the advertised name, with a space when it takes
 * arguments.
 */
export function commandChoices(commands: readonly ChatCommand[], text: string): CommandChoice[] {
  if (!/^[/$]\S*$/.test(text)) return [];
  const q = text.toLowerCase();
  const out: (CommandChoice & { exact: boolean })[] = [];
  for (const c of commands) {
    const full = c.name.startsWith('$') ? c.name : `/${c.name}`;
    const colon = full.lastIndexOf(':');
    const short = colon < 0 ? full : full[0] + full.slice(colon + 1);
    if (!full.toLowerCase().startsWith(q) && !short.toLowerCase().startsWith(q)) continue;
    // /mcp's arguments reconnect or turn servers off: the picker offers the listing only.
    const args = !!c.input && c.name !== 'mcp';
    out.push({ text: args ? `${full} ` : full, description: c.description ?? '', hint: args ? (c.input?.hint ?? '') : '', exact: full.toLowerCase() === q });
  }
  return out.sort((a, b) => Number(b.exact) - Number(a.exact)).map(({ exact: _, ...c }) => c);
}

/**
 * The app's own /plugin: no adapter advertises it, so the composer lists the
 * installed plugins itself instead of sending it to the agent.
 */
export const PLUGIN_COMMAND: ChatCommand = { name: 'plugin', description: 'List the installed plugins (shown here, not sent to the agent)', input: null };

/** Whether text is /plugin for the app: the agent does not have its own. */
export function isPluginListing(text: string, commands: readonly ChatCommand[]): boolean {
  return text.trim() === '/plugin' && !commands.some((c) => c.name === 'plugin');
}

/**
 * Whether text is the agent's bare /mcp, the status listing the app captures
 * for its modal. /mcp with arguments changes servers: an ordinary turn.
 */
export function isMcpListing(text: string, commands: readonly ChatCommand[]): boolean {
  return text.trim() === '/mcp' && commands.some((c) => c.name === 'mcp');
}

/** A server row of /mcp's reply. */
export interface McpServerRow {
  name: string;
  /** Claude's scope, given only to tell apart two servers of one name. */
  scope?: string;
  /** Tool counts and the error, as plain text. */
  detail: string;
}

/** /mcp's reply as the modal lists it: groups in the adapter's order, Disabled last. */
export interface McpStatus {
  /** The summary, e.g. "3 (1 failed, 2 connected)", or that none are configured. */
  summary: string;
  groups: { title: string; servers: McpServerRow[] }[];
  /** The other blocks (Codex's notes, the reconnect hint), as Markdown. */
  notes: string[];
}

const NO_MCP_SERVERS = 'No MCP servers are configured.';

/**
 * Reads the Markdown both adapters answer a bare /mcp with: a
 * "**MCP servers:** N (…)" line, "**<Status>**" blocks of
 * "- `name` (scope): detail" items and a "**Disabled:** `a`, `b`" line,
 * between other notes. Null when it is not that shape, for the modal to
 * show the Markdown itself.
 */
export function parseMcpStatus(markdown: string): McpStatus | null {
  const out: McpStatus = { summary: '', groups: [], notes: [] };
  for (const block of markdown.trim().split(/\n\s*\n/)) {
    const b = block.trim();
    const summary = /^\*\*MCP servers:\*\*\s*(.*)$/s.exec(b);
    const disabled = /^\*\*Disabled:\*\*\s*(.+)$/s.exec(b);
    const group = /^\*\*(.+?)\*\*\n(- [^]*)$/.exec(b);
    if (b === NO_MCP_SERVERS || summary) {
      if (out.summary) return null;
      out.summary = summary ? unescapeMarkdown(summary[1]) : b;
    } else if (disabled) {
      const servers: McpServerRow[] = [];
      let rest = disabled[1];
      for (;;) {
        const name = serverName(rest);
        if (!name) return null;
        servers.push({ name: name.text, detail: '' });
        rest = rest.slice(name.length);
        if (rest === '') break;
        if (!rest.startsWith(', ')) return null;
        rest = rest.slice(2);
      }
      out.groups.push({ title: 'Disabled', servers });
    } else if (group) {
      const servers: McpServerRow[] = [];
      for (const line of group[2].split('\n')) {
        const item = /^- (.*)$/.exec(line);
        const name = item && serverName(item[1]);
        if (!item || !name) return null;
        const after = /^(?: \(((?:\\.|[^)\\])*)\))?(?:: (.*))?$/.exec(item[1].slice(name.length));
        if (!after) return null;
        servers.push({ name: name.text, ...(after[1] && { scope: unescapeMarkdown(after[1]) }), detail: unescapeMarkdown(after[2] ?? '') });
      }
      out.groups.push({ title: unescapeMarkdown(group[1]), servers });
    } else if (b) {
      out.notes.push(b);
    }
  }
  return out.summary ? out : null;
}

/** A server name at the start of text: a code span, or Codex's "(unnamed)". */
function serverName(text: string): { text: string; length: number } | null {
  if (text.startsWith('(unnamed)')) return { text: '(unnamed)', length: 9 };
  const fence = /^`+/.exec(text)?.[0];
  if (!fence) return null;
  // The span closes at the first run of exactly as many backticks.
  const close = new RegExp(`(?<!\`)${fence}(?!\`)`, 'g');
  close.lastIndex = fence.length;
  const end = close.exec(text);
  if (!end) return null;
  let name = text.slice(fence.length, end.index);
  if (name.length > 2 && name.startsWith(' ') && name.endsWith(' ') && name.trim()) name = name.slice(1, -1);
  return { text: name, length: end.index + fence.length };
}

function unescapeMarkdown(text: string): string {
  return text.replace(/\\([\\`*_[\]<>|~])/g, '$1');
}

/**
 * /plugin's rows. The CLI says what is installed and enabled on disk; a
 * plugin is loaded in the chat only when its catalog has a command named
 * "<plugin>:…" (Codex: "$<plugin>:…"). mcp: names are MCP prompts.
 */
export function pluginRows(plugins: readonly ChatPlugin[], commands: readonly ChatCommand[]): (ChatPlugin & { loaded: boolean })[] {
  const names = commands.map((c) => c.name.replace(/^\$/, ''));
  return plugins.map((p) => ({ ...p, loaded: p.name !== 'mcp' && names.some((n) => n.startsWith(`${p.name}:`)) }));
}

/** "2 queued messages", the end questions' and the queue's way to count them. */
export function queuedCount(n: number): string {
  return n === 1 ? '1 queued message' : `${n} queued messages`;
}

/**
 * How the queue shows a message: its first line (or, without text, its
 * files' names), the whole text for its tooltip, and a count of its files.
 */
export function queuedLabel(q: ChatQueuedPrompt): { line: string; full: string; files: string } {
  const text = (q.text ?? '').trim();
  const names = (q.attachments ?? []).map((a) => a.name);
  const n = names.length;
  return {
    line: text ? text.split('\n', 1)[0].trim() : names.join(', '),
    full: text || names.join(', '),
    files: text && n > 0 ? (n === 1 ? '1 file' : `${n} files`) : '',
  };
}

/** What ending the chat costs: the running turn, and the messages never sent. */
function endLoss(phase: string, agentName: string, queued: number, stops: string): string | null {
  const lost = queued > 0 ? `${queuedCount(queued).replace(/^./, (c) => c.toUpperCase())} will not be sent.` : '';
  if (phase === 'working') return `${agentName} is still working. ${stops}${lost ? ` ${lost}` : ''}`;
  return lost || null;
}

/**
 * What End chat asks while the agent works or messages are queued, or null
 * when the chat may end at once: an idle chat loses nothing, its session is
 * saved.
 */
export function endChatQuestion(phase: string, agentName: string, queued = 0): { title: string; message: string; choices: { value: string; label: string; danger?: boolean }[] } | null {
  if (phase !== 'working' && phase !== 'idle') return null;
  const message = endLoss(phase, agentName, queued, 'Ending the chat stops its turn.');
  if (!message) return null;
  return { title: 'End the chat?', message, choices: [{ value: 'end', label: 'End chat', danger: true }] };
}

/**
 * What Switch to terminal asks while the agent works or messages are
 * queued, or null when the chat may stop at once: switching ends the chat,
 * so it stops a running turn just as End chat does.
 */
export function switchToTerminalQuestion(phase: string, agentName: string, queued = 0): { title: string; message: string; choices: { value: string; label: string; danger?: boolean }[] } | null {
  if (phase !== 'working' && phase !== 'idle') return null;
  const message = endLoss(phase, agentName, queued, 'Switching stops its turn.');
  if (!message) return null;
  return { title: 'Switch to terminal?', message, choices: [{ value: 'switch', label: 'Switch', danger: true }] };
}

/**
 * What Switch to chat asks before it ends a terminal tab's agent, or null
 * when the tab may switch at once. A terminal tab knows its agent only by
 * attention: working draws output (or its session's files say a turn
 * runs), needs waits for the user, and idle, done and exited have no turn
 * to stop.
 */
export function switchToChatQuestion(attention: string, agentName: string): { title: string; message: string; choices: { value: string; label: string; danger?: boolean }[] } | null {
  if (attention === 'working') return {
    title: 'Switch to chat?',
    message: `${agentName} is still working. Switching stops its turn.`,
    choices: [{ value: 'switch', label: 'Switch', danger: true }],
  };
  if (attention === 'needs') return {
    title: 'Switch to chat?',
    message: `${agentName} is waiting for your answer. Switching discards it.`,
    choices: [{ value: 'switch', label: 'Switch', danger: true }],
  };
  return null;
}
