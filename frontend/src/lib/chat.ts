// What the chat UI shows: the live tail as transcript messages, and which
// agents can chat. Every function here is pure.
import type { AgentID, ChatAgent, ChatAttachment, ChatAttachResult, ChatPermission, Message, Part, ToolStatus } from './types';
import { toolOutput, type ChatTail, type TailDiff, type TailItem, type TailTool } from './chatTail';

/**
 * The tail as transcript messages, for MessageBubble: each prompt is a user
 * message, and the agent's items up to the next prompt one assistant message.
 * IDs start with "live:" so they never meet a saved message's.
 */
export function tailMessages(tail: ChatTail): Message[] {
  const out: Message[] = [];
  let reply: Message | null = null;
  let replyTurn = -1;
  for (const it of tail.items) {
    if (it.type === 'text' && it.role === 'user') {
      reply = null;
      out.push({
        id: `live:${it.key}`,
        role: 'user',
        time: it.sentAt ? new Date(it.sentAt).toISOString() : '',
        parts: [
          ...(it.text ? [{ kind: 'text' as const, text: it.text }] : []),
          ...(it.attachments ?? []).map((a): Part => ({ kind: 'file', file: { name: a.name, mime: a.mimeType, size: a.size, ...(a.data && { data: a.data }) } })),
        ],
      });
      continue;
    }
    if (!reply || replyTurn !== it.turn) {
      reply = { id: `live:${it.key}`, role: 'assistant', time: '', parts: [] };
      replyTurn = it.turn;
      out.push(reply);
    }
    reply.parts.push(...itemParts(it));
  }
  return out;
}

function itemParts(it: TailItem): Part[] {
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
    case 'answer':
      return [{ kind: 'text', text: it.summary }];
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

/** Whether the composer can send: an idle chat, and text or a file. */
export function canSendPrompt(phase: string, sending: boolean, text: string, attachments: number): boolean {
  return phase === 'idle' && !sending && (text.trim() !== '' || attachments > 0);
}

/**
 * What End chat asks while the agent works, or null when the chat may end
 * at once: an idle chat loses nothing, its session is saved.
 */
export function endChatQuestion(phase: string, agentName: string): { title: string; message: string; choices: { value: string; label: string; danger?: boolean }[] } | null {
  if (phase !== 'working') return null;
  return {
    title: 'End the chat?',
    message: `${agentName} is still working. Ending the chat stops its turn.`,
    choices: [{ value: 'end', label: 'End chat', danger: true }],
  };
}
