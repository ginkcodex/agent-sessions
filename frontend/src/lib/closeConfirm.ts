import type { AgentID, TerminalTab } from './types';
import type { FilePromptChoice, FilePromptGroup, FilePromptItem } from './stores/filePrompt.svelte';
import { hostLabel } from './hosts';
import { ALL_AGENTS } from './portable';

type Ask = (title: string, message: string, choices: FilePromptChoice[], groups?: FilePromptGroup[]) => Promise<string | null>;
type RunningTab = Pick<TerminalTab, 'label' | 'host' | 'kind' | 'agent'> & { status: 'running' | 'exited' };
/** A chat as the warning needs it; exited chats are left out. */
export type WindowChat = { agent: AgentID | string; dir: string; phase: string };
/** A dirty editor tab as the unsaved-changes question needs it. */
export type DirtyDraft = { dir: string; tab: { path: string } };

/** What a closing dialog warns about: the sentence and the items it ends. */
export interface CloseWarning {
  message: string;
  /** The items as Terminals and Chats cards. */
  groups: FilePromptGroup[];
}

/** How many items a group names before it sums up the rest. */
const NAMED = 6;

/**
 * The warning for closing a window whose terminal tabs still run or whose
 * chats are open, or null when there are none. Closing the window ends them,
 * coding agents included.
 */
export function closeWindowWarning(tabs: readonly RunningTab[], chats: readonly WindowChat[] = []): CloseWarning | null {
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
  const total = running.length + open.length;
  return {
    message: `${said.join('. ')}. Closing the window ends ${total === 1 ? 'it' : 'them'}:`,
    groups: groups([
      running.length ? named('Terminals', 'terminal', running.map(terminalItem)) : null,
      open.length ? named('Chats', 'chat', open.map(chatItem)) : null,
    ]),
  };
}

/**
 * The warning for a host switch that ends the window's open chats, or null
 * when none is open. action says the switch: "Connecting to box".
 */
export function endingChatsWarning(chats: readonly WindowChat[], action: string): CloseWarning | null {
  const open = openChats(chats);
  if (!open.length) return null;
  return {
    message: `${chatsOpen(open)}. ${action} ends ${open.length === 1 ? 'it' : 'them'}:`,
    groups: [named('Chats', 'chat', open.map(chatItem))],
  };
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

function groups(built: (FilePromptGroup | null)[]): FilePromptGroup[] {
  return built.filter((group): group is FilePromptGroup => group !== null);
}

/** A group naming its first NAMED items and summing up the rest. */
function named(title: string, icon: 'terminal' | 'chat', items: FilePromptItem[]): FilePromptGroup {
  return items.length > NAMED
    ? { title, icon, items: items.slice(0, NAMED), more: items.length - NAMED }
    : { title, icon, items };
}

function terminalItem(tab: RunningTab): FilePromptItem {
  return {
    name: tab.label,
    detail: tab.host ? `on ${hostLabel(tab.host)}` : undefined,
    icon: tab.kind === 'agent' ? 'agent' : 'terminal',
    agent: tab.agent,
  };
}

function chatItem(chat: WindowChat): FilePromptItem {
  const agent = ALL_AGENTS.find(a => a.id === chat.agent)?.label ?? String(chat.agent);
  const folder = chat.dir.split(/[\\/]/).filter(Boolean).pop();
  return {
    name: agent,
    detail: folder ? `in ${folder}` : undefined,
    icon: 'chat',
    agent: String(chat.agent),
    badge: chat.phase === 'working' ? 'working' : undefined,
  };
}

/**
 * The warning for closing or switching away while files hold unsaved edits,
 * listing them all. Unlike the terminal and chat cards nothing is summed up:
 * the decision (save or discard) is taken file by file.
 */
export function unsavedDrafts(dirty: readonly DirtyDraft[]): CloseWarning {
  return {
    message: 'Unsaved changes in:',
    groups: [{ title: 'Unsaved files', icon: 'file', items: dirty.map(fileItem) }],
  };
}

function fileItem({ dir, tab }: DirtyDraft): FilePromptItem {
  return { name: tab.path, detail: `in ${dir}`, icon: 'file' };
}

/** Asks before a close ends running terminal tabs or open chats; true lets it go ahead. */
export async function confirmClosingWindow(tabs: readonly RunningTab[], chats: readonly WindowChat[], ask: Ask): Promise<boolean> {
  const warning = closeWindowWarning(tabs, chats);
  if (!warning) return true;
  return await ask('Close window?', warning.message, [{ value: 'close', label: 'Close window', danger: true }], warning.groups) === 'close';
}
