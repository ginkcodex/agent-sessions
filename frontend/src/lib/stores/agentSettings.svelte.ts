import { api } from '../api';
import { epoch } from '../link';
import { link } from './link.svelte';
import type { AgentSettings } from '../types';
import { ALL_AGENTS } from '../portable';

/** Host-owned provider selection; changing it takes effect on backend construction. */
export class AgentSettingsStore {
  settings = $state<AgentSettings | null>(null);
  selected = $state<string[]>([]);
  loading = $state(false);
  saving = $state(false);
  error = $state('');
  private generation = 0;
  private owner = -1;

  /** Current backend selection, not unsaved or restart-pending choices. */
  get activeAgents(): { id: string; label: string }[] {
    if (!this.settings || this.owner !== epoch()) return ALL_AGENTS;
    return this.settings.supported.filter(a => this.settings!.active.includes(a.id)).map(a => ({ id: a.id, label: a.name }));
  }

  /** Saved selection for UI filters, including changes pending a restart. */
  get enabledAgents(): { id: string; label: string }[] {
    if (!this.settings || this.owner !== epoch()) return ALL_AGENTS;
    return this.settings.supported.filter(a => this.settings!.enabled.includes(a.id)).map(a => ({ id: a.id, label: a.name }));
  }

  get dirty(): boolean {
    return !!this.settings && JSON.stringify(this.selected) !== JSON.stringify(this.settings.enabled);
  }

  async refresh(): Promise<void> {
    const id = ++this.generation;
    const owner = epoch();
    this.owner = owner;
    this.settings = null;
    this.selected = [];
    this.error = '';
    this.loading = true;
    try {
      const settings = await api.getAgentSettings();
      if (id !== this.generation || owner !== epoch()) return;
      this.settings = settings;
      this.selected = [...settings.enabled];
    } catch (err) {
      if (id === this.generation && owner === epoch()) this.error = err instanceof Error ? err.message : String(err);
    } finally {
      if (id === this.generation) this.loading = false;
    }
  }

  toggle(id: string, enabled: boolean): void {
    if (!this.settings || this.saving || !this.settings.supported.some(a => a.id === id)) return;
    if (!enabled && this.selected.length === 1 && this.selected.includes(id)) return;
    this.selected = this.settings.supported.filter(a => a.id === id ? enabled : this.selected.includes(a.id)).map(a => a.id);
  }

  async save(): Promise<void> {
    if (!this.settings || !this.selected.length || this.saving || this.owner !== epoch() || link.stale || link.locked) return;
    const owner = this.owner;
    const id = this.generation;
    this.saving = true;
    this.error = '';
    try {
      const settings = await api.setAgentSettings([...this.selected]);
      if (owner !== epoch() || id !== this.generation) return;
      this.settings = settings;
      this.selected = [...settings.enabled];
    } catch (err) {
      if (owner === epoch() && id === this.generation) this.error = err instanceof Error ? err.message : String(err);
    } finally { this.saving = false; }
  }
}

export const agentSettings = new AgentSettingsStore();
