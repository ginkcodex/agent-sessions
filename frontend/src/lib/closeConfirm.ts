import type { TerminalTab } from './types';
import type { FilePromptChoice } from './stores/filePrompt.svelte';
import { hostLabel } from './hosts';

type Ask = (title: string, message: string, choices: FilePromptChoice[]) => Promise<string | null>;
type RunningTab = Pick<TerminalTab, 'label' | 'host' | 'kind'> & { status: 'running' | 'exited' };

/** How many running tabs the warning names before it sums up the rest. */
const NAMED = 6;

/**
 * The warning for closing a window whose terminal tabs still run, or null
 * when none do. Closing the window ends them, coding agents included.
 */
export function runningTerminalsWarning(tabs: readonly RunningTab[]): string | null {
  const running = tabs.filter(tab => tab.status === 'running');
  if (!running.length) return null;
  const agents = running.filter(tab => tab.kind === 'agent').length;
  const count = running.length > 1 ? `${running.length} terminals are` : agents ? 'A coding agent is' : 'A terminal is';
  const including = agents && running.length > 1 ? `, including ${agents === 1 ? 'a coding agent' : `${agents} coding agents`}` : '';
  const names = running.slice(0, NAMED).map(tab => tab.host ? `${tab.label} on ${hostLabel(tab.host)}` : tab.label);
  if (running.length > NAMED) names.push(`and ${running.length - NAMED} more`);
  return `${count} still running${including}. Closing the window ends ${running.length === 1 ? 'it' : 'them'}:\n${names.join('\n')}`;
}

/** Asks before a close ends running terminal tabs; true lets it go ahead. */
export async function confirmEndingTerminals(tabs: readonly RunningTab[], ask: Ask): Promise<boolean> {
  const warning = runningTerminalsWarning(tabs);
  if (!warning) return true;
  return await ask('Close window?', warning, [{ value: 'close', label: 'Close window', danger: true }]) === 'close';
}
