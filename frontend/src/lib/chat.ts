// What the chat UI shows: the live tail as transcript messages, and which
// agents can chat. Every function here is pure.
import type { AgentID, ChatAgent, Message, Part, ToolStatus } from './types';
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
        parts: [{ kind: 'text', text: it.text }],
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

