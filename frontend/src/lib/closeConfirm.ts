import type { AgentID, TerminalTab } from './types';
import type { FilePromptChoice } from './stores/filePrompt.svelte';
import { hostLabel } from './hosts';
import { ALL_AGENTS } from './portable';

type Ask = (title: string, message: string, choices: FilePromptChoice[]) => Promise<string | null>;
type RunningTab = Pick<TerminalTab, 'label' | 'host' | 'kind'> & { status: 'running' | 'exited' };
/** A chat as the warning needs it; exited chats are left out. */
export type WindowChat = { agent: AgentID | string; dir: string; phase: string };

/** How many running tabs and chats the warning names before it sums up the rest. */
const NAMED = 6;

/**
 * The warning for closing a window whose terminal tabs still run or whose
 * chats are open, or null when there are none. Closing the window ends them,
 * coding agents included.
 */
export function closeWindowWarning(tabs: readonly RunningTab[], chats: readonly WindowChat[] = []): string | null {
  const running = tabs.filter(tab => tab.status === 'running');
  const open = openChats(chats);
  if (!running.length && !open.length) return null;
  const said: string[] = [];
  if (running.length) {
    const agents = running.filter(tab => tab.kind === 'agent').length;
    const count = running.length > 1 ? `${running.length} terminals are` : agents ? 'A coding agent is' : 'A terminal is';
    const including = agents && running.length > 1 ? `, including ${agents === 1 ? 'a coding agent' : `${agents} coding agents`}` : '';
    said.push(`${count} still running${including}`);
  }
  if (open.length) said.push(chatsOpen(open));
  const names = [
    ...running.map(tab => tab.host ? `${tab.label} on ${hostLabel(tab.host)}` : tab.label),
    ...open.map(chatName),
  ];
  return `${said.join('. ')}. Closing the window ends ${names.length === 1 ? 'it' : 'them'}:\n${listed(names)}`;
}

/**
 * The warning for a host switch that ends the window's open chats, or null
 * when none is open. action says the switch: "Connecting to box".
 */
export function endingChatsWarning(chats: readonly WindowChat[], action: string): string | null {
  const open = openChats(chats);
  if (!open.length) return null;
  return `${chatsOpen(open)}. ${action} ends ${open.length === 1 ? 'it' : 'them'}:\n${listed(open.map(chatName))}`;
}

function openChats(chats: readonly WindowChat[]): WindowChat[] {
  return chats.filter(chat => chat.phase !== 'exited');
}

function chatsOpen(open: readonly WindowChat[]): string {
  const working = open.filter(chat => chat.phase === 'working').length;
  return open.length === 1
    ? `A chat is open${working ? ' and its agent is working' : ''}`
    : `${open.length} chats are open${working ? `, ${working} of them working` : ''}`;
}

/** One name a line, the rest summed up past NAMED. */
function listed(names: string[]): string {
  const total = names.length;
  return (total > NAMED ? [...names.slice(0, NAMED), `and ${total - NAMED} more`] : names).join('\n');
}

function chatName(chat: WindowChat): string {
  const agent = ALL_AGENTS.find(a => a.id === chat.agent)?.label ?? chat.agent;
  const folder = chat.dir.split(/[\\/]/).filter(Boolean).pop();
  return `Chat with ${agent}${folder ? ` in ${folder}` : ''}${chat.phase === 'working' ? ' (working)' : ''}`;
}

/** Asks before a close ends running terminal tabs or open chats; true lets it go ahead. */
export async function confirmClosingWindow(tabs: readonly RunningTab[], chats: readonly WindowChat[], ask: Ask): Promise<boolean> {
  const warning = closeWindowWarning(tabs, chats);
  if (!warning) return true;
  return await ask('Close window?', warning, [{ value: 'close', label: 'Close window', danger: true }]) === 'close';
}
