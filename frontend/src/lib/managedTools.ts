/** Closed TI-03 contract. Installer policy is backend-owned, never editable here. */
export type ManagedToolName = 'gopls' | 'ruff' | 'vtsls' | 'pyright' | 'rust-analyzer' | 'svelte-language-server';
export const MANAGED_TOOLS: readonly ManagedToolName[] = ['gopls', 'ruff', 'vtsls', 'pyright', 'rust-analyzer', 'svelte-language-server'];
/** Provider whose missing server a managed install supplies. */
export const MANAGED_TOOL_FOR_PROVIDER: Readonly<Record<string, ManagedToolName>> = { gopls: 'gopls', typescript: 'vtsls', ruff: 'ruff', pyright: 'pyright', 'rust-analyzer': 'rust-analyzer', svelte: 'svelte-language-server' };
/**
 * Chat's ACP adapters, in the same catalog. They are installed only from the
 * chat setup, after review, never automatically, and code checks ignore them.
 */
export type AdapterToolName = 'claude-agent-acp' | 'codex-acp';
export const ADAPTER_TOOLS: readonly AdapterToolName[] = ['claude-agent-acp', 'codex-acp'];
/** Any tool the app can install for itself. */
export type ManagedInstallName = ManagedToolName | AdapterToolName;
export type ManagedToolAvailability = 'available' | 'missingTool' | 'unavailable';
export interface ManagedToolRequest {
  action: 'status' | 'propose' | 'install' | 'dismiss' | 'cancel';
  expectedHostId: string;
  tool?: ManagedInstallName;
  token?: string;
  jobId?: string;
}
export interface ManagedToolProposal {
  token: string;
  hostId: string;
  connectionId: string;
  tool: ManagedInstallName;
  version: string;
  source: string;
  integrity: string;
  /** What else the install uses or downloads (the host Go, Node.js). */
  note?: string;
  destination: string;
  expires: string;
}
export interface ManagedToolState {
  hostId: string;
  state: 'idle' | 'unavailable' | 'available' | 'missingTool' | 'proposed' | 'installing' | 'installed' | 'failed' | 'canceled';
  message?: string;
  jobId?: string;
  tool?: ManagedInstallName;
  proposal?: ManagedToolProposal;
  /** Status only: each app-managed install on the host. */
  tools?: Partial<Record<ManagedInstallName, ManagedToolAvailability>>;
}
