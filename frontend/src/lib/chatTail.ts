// The live tail of a chat: what the agent has streamed in ACP session
// updates and the saved transcript does not show yet. Every function here is
// pure; the chats store keeps one tail per chat.
import type { AcpContentBlock, AcpSessionUpdate, AcpToolCall, AcpToolCallContent, Message } from './types';

/** Text the user sent or the agent wrote. */
export interface TailText {
  type: 'text';
  key: string;
  /** The turn the item belongs to, counted from 1. */
  turn: number;
  role: 'user' | 'assistant';
  /** The agent's message ID, when it sent one. */
  messageId?: string;
  text: string;
  /** For a prompt sent from this window: when it was sent, in ms. */
  sentAt?: number;
  /** For a prompt sent from this window: the files it carried. */
  attachments?: TailAttachment[];
}

/** A file sent with a prompt; an image keeps its bytes to show. */
export interface TailAttachment {
  name: string;
  mimeType: string;
  size: number;
  /** An image's bytes in base64. */
  data?: string;
}

/** A prompt sent from this window. */
export interface TailPrompt {
  text: string;
  sentAt: number;
  attachments?: TailAttachment[];
}

/** The agent's thinking. */
export interface TailThought {
  type: 'thought';
  key: string;
  turn: number;
  messageId?: string;
  text: string;
}

export interface TailDiff {
  path: string;
  /** null for a new file. */
  oldText: string | null;
  newText: string;
}

export interface TailTool {
  type: 'tool';
  key: string;
  turn: number;
  toolCallId: string;
  /** The agent's tool name, when the adapter sends it. */
  name?: string;
  title: string;
  /** ACP's tool kind: read, edit, execute, search, fetch, think, other. */
  kind?: string;
  /** pending, in_progress, completed or failed. */
  status: string;
  input?: unknown;
  /** The text of the tool's content blocks. */
  text: string;
  rawOutput?: unknown;
  diffs: TailDiff[];
  /** A command's output and exit code (Codex sends them in _meta). */
  terminal?: { output: string; exitCode?: number | null };
  locations: { path: string; line?: number }[];
}

export interface TailPlanEntry {
  content: string;
  priority?: string;
  /** pending, in_progress or completed. */
  status?: string;
}

/** The agent's plan; each plan update replaces it. */
export interface TailPlan {
  type: 'plan';
  key: string;
  turn: number;
  entries: TailPlanEntry[];
}

export type TailItem = TailText | TailThought | TailTool | TailPlan;

export interface ChatTail {
  /** The latest turn, 0 before the first prompt. */
  turn: number;
  /** True from a turn's start to its end. */
  turnOpen: boolean;
  items: TailItem[];
  /** The next item key's number. */
  nextKey: number;
  /** The session's title, from session_info_update. */
  title?: string;
  /** Context in use, in tokens, from the latest usage_update. */
  usage?: { used: number; size: number };
}

export function emptyTail(): ChatTail {
  return { turn: 0, turnOpen: false, items: [], nextKey: 1 };
}

/** Starts a turn; prompt is what was sent from this window, if anything. */
export function beginTurn(tail: ChatTail, prompt?: TailPrompt): ChatTail {
  const turn = tail.turn + 1;
  const next: ChatTail = { ...tail, turn, turnOpen: true };
  const attachments = prompt?.attachments?.length ? prompt.attachments : undefined;
  if (!prompt || (!prompt.text && !attachments)) return next;
  const item: TailText = { type: 'text', key: `i${tail.nextKey}`, turn, role: 'user', text: prompt.text, sentAt: prompt.sentAt, ...(attachments && { attachments }) };
  return { ...next, items: [...tail.items, item], nextKey: tail.nextKey + 1 };
}

export function endTurn(tail: ChatTail): ChatTail {
  return tail.turnOpen ? { ...tail, turnOpen: false } : tail;
}

/**
 * Applies one ACP session update. Updates the tail does not keep (commands,
 * modes, options) and unknown kinds return the tail unchanged.
 */
export function applySessionUpdate(tail: ChatTail, update: AcpSessionUpdate | undefined): ChatTail {
  if (!update || typeof update !== 'object') return tail;
  switch (update.sessionUpdate) {
    case 'agent_message_chunk':
      return applyChunk(tail, update, 'text', 'assistant');
    case 'user_message_chunk':
      return applyChunk(tail, update, 'text', 'user');
    case 'agent_thought_chunk':
      return applyChunk(tail, update, 'thought', 'assistant');
    case 'tool_call':
    case 'tool_call_update':
      return applyToolCall(tail, update as unknown as AcpToolCall);
    case 'plan':
      return applyPlan(tail, update.entries);
    case 'session_info_update': {
      const title = update.title;
      return typeof title === 'string' && title !== tail.title ? { ...tail, title } : tail;
    }
    case 'usage_update': {
      const { used, size } = update;
      if (typeof used !== 'number' || typeof size !== 'number' || !Number.isFinite(used) || !Number.isFinite(size) || used < 0 || size <= 0) return tail;
      return tail.usage?.used === used && tail.usage.size === size ? tail : { ...tail, usage: { used, size } };
    }
    default:
      return tail;
  }
}

function blockText(block: unknown): string {
  const b = block as AcpContentBlock | undefined;
  return b && b.type === 'text' && typeof b.text === 'string' ? b.text : '';
}

function applyChunk(tail: ChatTail, update: AcpSessionUpdate, type: 'text' | 'thought', role: 'user' | 'assistant'): ChatTail {
  const text = blockText(update.content);
  if (!text) return tail;
  const messageId = typeof update.messageId === 'string' && update.messageId ? update.messageId : undefined;
  const same = (it: TailItem) => it.type === type && (it.type !== 'text' || it.role === role);
  let at = -1;
  if (messageId) {
    at = findLastIndex(tail.items, (it) => same(it) && (it as TailText | TailThought).messageId === messageId);
  } else {
    // Without IDs, a chunk continues the item it follows.
    const last = tail.items.length - 1;
    const it = tail.items[last];
    if (it && same(it) && !(it as TailText | TailThought).messageId && it.turn === tail.turn) at = last;
  }
  if (at >= 0) {
    const it = tail.items[at] as TailText | TailThought;
    return { ...tail, items: replaceAt(tail.items, at, { ...it, text: it.text + text }) };
  }
  const key = `i${tail.nextKey}`;
  const item: TailText | TailThought =
    type === 'text' ? { type, key, turn: tail.turn, role, text, ...(messageId && { messageId }) } : { type, key, turn: tail.turn, text, ...(messageId && { messageId }) };
  return { ...tail, items: [...tail.items, item], nextKey: tail.nextKey + 1 };
}

function applyToolCall(tail: ChatTail, update: AcpToolCall): ChatTail {
  const id = update.toolCallId;
  if (typeof id !== 'string' || !id) return tail;
  const at = findLastIndex(tail.items, (it) => it.type === 'tool' && it.toolCallId === id);
  const base: TailTool =
    at >= 0
      ? (tail.items[at] as TailTool)
      : { type: 'tool', key: `i${tail.nextKey}`, turn: tail.turn, toolCallId: id, title: '', status: 'pending', text: '', diffs: [], locations: [] };
  const tool = mergeTool(base, update);
  if (at >= 0) return { ...tail, items: replaceAt(tail.items, at, tool) };
  return { ...tail, items: [...tail.items, tool], nextKey: tail.nextKey + 1 };
}

// An update sets the fields it carries; content replaces the old content.
function mergeTool(t: TailTool, u: AcpToolCall): TailTool {
  const out: TailTool = { ...t };
  if (typeof u.title === 'string') out.title = u.title;
  if (typeof u.kind === 'string') out.kind = u.kind;
  if (typeof u.status === 'string') out.status = u.status;
  if (typeof u.name === 'string' && u.name) out.name = u.name;
  if (u.rawInput !== undefined && !isEmptyObject(u.rawInput)) out.input = u.rawInput;
  if (u.rawOutput !== undefined) out.rawOutput = u.rawOutput;
  if (Array.isArray(u.locations)) out.locations = u.locations.filter((l) => l && typeof l.path === 'string');
  if (Array.isArray(u.content)) {
    out.text = u.content.map(contentText).filter(Boolean).join('\n');
    out.diffs = u.content.filter((c) => c?.type === 'diff' && typeof c.path === 'string').map((c) => ({
      path: c.path!,
      oldText: typeof c.oldText === 'string' ? c.oldText : null,
      newText: typeof c.newText === 'string' ? c.newText : '',
    }));
    if (!out.terminal && u.content.some((c) => c?.type === 'terminal')) out.terminal = { output: '' };
  }
  const meta = u._meta;
  if (meta && typeof meta === 'object') {
    const delta = (meta.terminal_output_delta as { data?: unknown } | undefined)?.data;
    if (typeof delta === 'string' && delta) out.terminal = { ...out.terminal, output: (out.terminal?.output ?? '') + delta };
    const exit = meta.terminal_exit as { exit_code?: unknown } | undefined;
    if (exit && typeof exit === 'object') {
      const code = typeof exit.exit_code === 'number' ? exit.exit_code : null;
      out.terminal = { output: out.terminal?.output ?? '', exitCode: code };
    }
  }
  return out;
}

function contentText(c: AcpToolCallContent | undefined): string {
  return c?.type === 'content' ? blockText(c.content) : '';
}

function isEmptyObject(v: unknown): boolean {
  return !!v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length === 0;
}

function applyPlan(tail: ChatTail, raw: unknown): ChatTail {
  if (!Array.isArray(raw)) return tail;
  const entries: TailPlanEntry[] = raw
    .filter((e) => e && typeof e.content === 'string')
    .map((e) => ({
      content: e.content,
      ...(typeof e.priority === 'string' && { priority: e.priority }),
      ...(typeof e.status === 'string' && { status: e.status }),
    }));
  const at = tail.items.findIndex((it) => it.type === 'plan');
  // A plan stays where it first showed during its turn; a later turn's plan
  // moves to the end.
  if (at >= 0 && tail.items[at].turn === tail.turn) {
    return { ...tail, items: replaceAt(tail.items, at, { ...(tail.items[at] as TailPlan), entries }) };
  }
  const items = at >= 0 ? tail.items.filter((_, i) => i !== at) : tail.items;
  const plan: TailPlan = { type: 'plan', key: `i${tail.nextKey}`, turn: tail.turn, entries };
  return { ...tail, items: [...items, plan], nextKey: tail.nextKey + 1 };
}

/** The tool's output: its content text, else its raw output when that is text. */
export function toolOutput(t: TailTool): string {
  if (t.text) return t.text;
  if (t.terminal?.output) return t.terminal.output;
  return typeof t.rawOutput === 'string' ? t.rawOutput : '';
}

/**
 * The saved message ID a streamed message ID stands for. OpenCode sends its
 * thoughts as `<message ID>:reasoning:<n>`.
 */
export function savedMessageId(messageId: string): string {
  return messageId.replace(/:reasoning:\d+$/, '');
}

// A saved prompt this much older than the live one is another prompt.
const PROMPT_CLOCK_SLACK_MS = 60_000;

/**
 * Drops the items that saved messages now show:
 * - text and thoughts whose message is saved with that kind of part, once
 *   they are complete (something follows them, or their turn ended);
 * - tool calls saved under the same ID, once they have a result or their
 *   turn ended;
 * - a prompt sent from this window, once a saved user message has its text;
 * - every item of an ended turn whose last agent message is saved. This
 *   covers tool calls the agent saves under other IDs (Codex) and plans;
 * - every item of a turn before one the saved messages reach by ID. An agent
 *   saves turns in order, so what an earlier turn left unsaved (a cancelled
 *   reply, a failed prompt) never will be, and would show out of order.
 */
export function reconcile(tail: ChatTail, saved: readonly Message[]): ChatTail {
  if (tail.items.length === 0 || saved.length === 0) return tail;
  const byId = new Map<string, Message>();
  const tools = new Map<string, string>();
  const prompts: { text: string; time: number }[] = [];
  for (const m of saved) {
    byId.set(m.id, m);
    for (const p of m.parts ?? []) {
      if (p.tool?.id) tools.set(p.tool.id, p.tool.status);
    }
    if (m.role === 'user') {
      const text = (m.parts ?? []).filter((p) => p.kind === 'text').map((p) => p.text ?? '').join('\n').trim();
      if (text) prompts.push({ text, time: Date.parse(m.time) });
    }
  }
  const ended = (turn: number) => turn < tail.turn || !tail.turnOpen;
  const lastIndex = tail.items.length - 1;
  const complete = (i: number) => ended(tail.items[i].turn) || i < lastIndex;

  const covered = (it: TailItem, i: number): boolean => {
    switch (it.type) {
      case 'text': {
        if (it.role === 'user') {
          // Claude saves prompts without an ID: fall back to the text.
          if (it.messageId && byId.has(savedMessageId(it.messageId))) return true;
          const text = it.text.trim();
          // A prompt of only files waits for its turn to be saved.
          if (!text) return false;
          return prompts.some((p) => p.text === text && (Number.isNaN(p.time) || it.sentAt === undefined || p.time >= it.sentAt - PROMPT_CLOCK_SLACK_MS));
        }
        if (!it.messageId || !complete(i)) return false;
        return hasPart(byId.get(savedMessageId(it.messageId)), 'text');
      }
      case 'thought':
        if (!it.messageId || !complete(i)) return false;
        return hasPart(byId.get(savedMessageId(it.messageId)), 'reasoning');
      case 'tool': {
        const status = tools.get(it.toolCallId);
        return status !== undefined && (status !== 'pending' || ended(it.turn));
      }
      default:
        return false;
    }
  };

  // Turns that ended with their last agent message saved.
  const savedTurns = new Set<number>();
  const lastMessage = new Map<number, number>();
  tail.items.forEach((it, i) => {
    if ((it.type === 'text' && it.role === 'assistant') || it.type === 'thought') lastMessage.set(it.turn, i);
  });
  for (const [turn, i] of lastMessage) {
    if (ended(turn) && covered(tail.items[i], i)) savedTurns.add(turn);
  }

  // The latest turn the saved messages reach by an agent's own ID.
  let reached = 0;
  for (const it of tail.items) {
    if (it.turn <= reached) continue;
    const id = it.type === 'tool' ? it.toolCallId : it.type !== 'plan' && it.messageId ? savedMessageId(it.messageId) : '';
    if (id && (it.type === 'tool' ? tools.has(id) : byId.has(id))) reached = it.turn;
  }

  const items = tail.items.filter((it, i) => it.turn >= reached && !savedTurns.has(it.turn) && !covered(it, i));
  return items.length === tail.items.length ? tail : { ...tail, items };
}

function hasPart(m: Message | undefined, kind: 'text' | 'reasoning'): boolean {
  return !!m && (m.parts ?? []).some((p) => p.kind === kind && !!p.text);
}

function findLastIndex<T>(list: readonly T[], ok: (it: T) => boolean): number {
  for (let i = list.length - 1; i >= 0; i--) if (ok(list[i])) return i;
  return -1;
}

function replaceAt<T>(list: readonly T[], i: number, value: T): T[] {
  const out = list.slice();
  out[i] = value;
  return out;
}

/** A chat's events put back in seq order. */
export interface ChatSequence<E extends { seq: number }> {
  /** The seq of the next event to apply; 0 until the first event sets it. */
  next: number;
  /** Events that arrived ahead of next, in seq order. */
  held: E[];
}

/** Held events past this mean the missing one is lost: skip it. */
export const MAX_HELD_EVENTS = 256;

export function newSequence<E extends { seq: number }>(next = 0): ChatSequence<E> {
  return { next, held: [] };
}

/**
 * Takes one event and returns the events now ready, in order. Events older
 * than next, and duplicates, are dropped. When the first seq is not known
 * (a chat found after a reload), the first event sets it.
 */
export function sequence<E extends { seq: number }>(s: ChatSequence<E>, event: E): { seq: ChatSequence<E>; ready: E[] } {
  const n = event.seq;
  if (!Number.isInteger(n) || n < 1) return { seq: s, ready: [] };
  let next = s.next === 0 ? n : s.next;
  if (n < next || s.held.some((e) => e.seq === n)) return { seq: s, ready: [] };
  let held = [...s.held, event].sort((a, b) => a.seq - b.seq);
  if (held.length > MAX_HELD_EVENTS) next = held[0].seq;
  const ready: E[] = [];
  while (held.length > 0 && held[0].seq === next) {
    ready.push(held[0]);
    held = held.slice(1);
    next++;
  }
  return { seq: { next, held }, ready };
}
