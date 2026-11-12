// A chat's session options (mode, model, effort) as pickers, and the choices
// the user made, remembered per agent and host. A mode that skips permission
// requests is never remembered or applied: each chat starts asking unless the
// user picks such a mode in it. Apart from the storage helpers, every
// function here is pure.
import type { AgentID, ChatConfigOption, ChatConfigSelectOption } from './types';
import { targetKey } from './tree';

/** Values of a mode option that run tools without asking first. */
const SKIPS_PERMISSIONS: Partial<Record<AgentID, string[]>> = {
  'claude-code': ['bypassPermissions'],
  codex: ['agent-full-access'],
};

export interface PickerChoice {
  value: string;
  name: string;
  description?: string;
  /** The group the agent puts the value in, if any. */
  group?: string;
  /** The mode runs tools without permission requests. */
  skipsPermissions: boolean;
}

export interface Picker {
  id: string;
  name: string;
  description?: string;
  kind: 'select' | 'boolean';
  /** The value the agent reports. */
  value: string | boolean | undefined;
  choices: PickerChoice[];
}

/** A picker's choice as the shared Dropdown shows it. */
export interface PickerOption {
  value: string;
  label: string;
  /** The choice's description, shown on hover in the menu. */
  title?: string;
  /** The group the agent puts the value in; its heading covers the run. */
  group?: string;
}

function isMode(opt: ChatConfigOption): boolean {
  return opt.category === 'mode' || opt.id === 'mode';
}

/** Whether value of opt is a mode that skips permission requests. */
export function skipsPermissions(agent: AgentID, opt: ChatConfigOption, value: unknown): boolean {
  return isMode(opt) && typeof value === 'string' && (SKIPS_PERMISSIONS[agent] ?? []).includes(value);
}

/** The values of a select option, groups flattened. */
export function selectValues(options: ChatConfigSelectOption[] | undefined, group?: string): Omit<PickerChoice, 'skipsPermissions'>[] {
  const out: Omit<PickerChoice, 'skipsPermissions'>[] = [];
  for (const o of options ?? []) {
    if (o.options) out.push(...selectValues(o.options, o.name || o.group));
    else if (typeof o.value === 'string') out.push({ value: o.value, name: o.name || o.value, ...(o.description && { description: o.description }), ...(group && { group }) });
  }
  return out;
}

/**
 * A picker's choices as the shared Dropdown's options: the choice's name,
 * bare — the unguarded warning names what a mode that skips permission
 * requests means — its description as the hover title, and the agent's
 * groups as headings.
 */
export function pickerOptions(picker: Picker): PickerOption[] {
  return picker.choices.map((c) => ({
    value: c.value,
    label: c.name,
    ...(c.description && { title: c.description }),
    ...(c.group && { group: c.group }),
  }));
}

/** The options a picker can show: selects with values, and booleans. */
export function pickers(agent: AgentID, options: ChatConfigOption[]): Picker[] {
  const out: Picker[] = [];
  for (const opt of options) {
    const base = { id: opt.id, name: opt.name || opt.id, ...(opt.description && { description: opt.description }) };
    if (opt.type === 'boolean') {
      out.push({ ...base, kind: 'boolean', value: typeof opt.currentValue === 'boolean' ? opt.currentValue : undefined, choices: [] });
      continue;
    }
    if (opt.type !== 'select') continue;
    const choices = selectValues(opt.options).map((c) => ({ ...c, skipsPermissions: skipsPermissions(agent, opt, c.value) }));
    if (choices.length === 0) continue;
    const value = typeof opt.currentValue === 'string' ? opt.currentValue : undefined;
    // The agent reports a value it does not list: show it rather than another one.
    if (value !== undefined && !choices.some((c) => c.value === value)) choices.unshift({ value, name: value, skipsPermissions: skipsPermissions(agent, opt, value) });
    out.push({ ...base, kind: 'select', value, choices });
  }
  return out;
}

export type Choices = Record<string, string | boolean>;

/**
 * The remembered choices that apply to a chat with these options, in the
 * agent's order: those for options it has, with a value it still offers,
 * that differ from the current one. Never a mode that skips permission
 * requests.
 */
export function choicesToApply(agent: AgentID, options: ChatConfigOption[], saved: Choices): [string, string | boolean][] {
  const out: [string, string | boolean][] = [];
  for (const opt of options) {
    if (!Object.hasOwn(saved, opt.id)) continue;
    const want = saved[opt.id];
    if (want === opt.currentValue || skipsPermissions(agent, opt, want)) continue;
    const offered =
      opt.type === 'boolean' ? typeof want === 'boolean' : opt.type === 'select' && typeof want === 'string' && selectValues(opt.options).some((c) => c.value === want);
    if (offered) out.push([opt.id, want]);
  }
  return out;
}

const STORAGE_PREFIX = 'agent-sessions:chat-options';

function storageKey(agent: AgentID, host?: string): string {
  return `${STORAGE_PREFIX}:${targetKey(host)}:${agent}`;
}

/** The choices remembered for agent on host (undefined: this computer). */
export function loadChoices(agent: AgentID, host?: string): Choices {
  if (typeof localStorage === 'undefined') return {};
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(storageKey(agent, host)) || '{}');
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
    const out: Choices = {};
    for (const [k, v] of Object.entries(parsed)) if (typeof v === 'string' || typeof v === 'boolean') out[k] = v;
    return out;
  } catch {
    return {};
  }
}

/**
 * Remembers a choice the user made for agent on host. Picking a mode that
 * skips permission requests forgets the remembered mode instead, so the next
 * chat starts with the agent's own.
 */
export function saveChoice(agent: AgentID, host: string | undefined, option: ChatConfigOption, value: string | boolean): void {
  if (typeof localStorage === 'undefined') return;
  const choices = loadChoices(agent, host);
  if (skipsPermissions(agent, option, value)) delete choices[option.id];
  else choices[option.id] = value;
  try {
    localStorage.setItem(storageKey(agent, host), JSON.stringify(choices));
  } catch {
    // Storage full or blocked: the choice is not remembered.
  }
}
