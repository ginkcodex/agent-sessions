import { api } from '../api';
import { errorText } from '../manage';
import type { Translation } from './translations.svelte';

export const MAX_FILE_TRANSLATIONS = 20;
export const MAX_FILE_TRANSLATION_BYTES = 16 * 1024 * 1024;

/** Stable across tab/viewer recreation; source equality is checked separately. */
export function fileTranslationKey(host: string | undefined, dir: string | null, path: string, language: string): string {
  return JSON.stringify([host ?? '', dir ?? '', path, language]);
}

interface Entry {
  source: string;
  translation: Translation;
}

/** Window-memory LRU cache. Requests outlive viewers, never the application. */
export class FileTranslationsStore {
  private entries = new Map<string, Entry>();
  private version = $state(0);

  constructor(private translateText: (text: string) => Promise<string> = text => api.translate(text)) {}

  get(key: string, source: string): Translation | undefined {
    void this.version;
    const entry = this.entries.get(key);
    if (!entry || entry.source !== source) return undefined;
    this.touch(key, entry);
    return entry.translation;
  }

  /** Explicit translation only; reopening a file never starts a provider call. */
  async translate(key: string, source: string): Promise<void> {
    const existing = this.entries.get(key);
    if (existing?.source === source && existing.translation.status === 'loading') return;
    const entry: Entry = { source, translation: { status: 'loading', text: '', error: '', showOriginal: false } };
    this.touch(key, entry);
    this.trim();
    this.version++;
    let result: Translation;
    try {
      result = { status: 'done', text: await this.translateText(source), error: '', showOriginal: false };
    } catch (err) {
      result = { status: 'error', text: '', error: errorText(err, 'Translation failed'), showOriginal: false };
    }
    // A changed-source request, eviction or clear owns the slot now.
    if (this.entries.get(key) !== entry) return;
    entry.translation = result;
    this.trim();
    this.version++;
  }

  toggleOriginal(key: string, source: string): void {
    const entry = this.entries.get(key);
    if (entry?.source !== source || entry.translation.status !== 'done') return;
    entry.translation = { ...entry.translation, showOriginal: !entry.translation.showOriginal };
    this.touch(key, entry);
    this.version++;
  }

  clear(): void {
    this.entries.clear();
    this.version++;
  }

  private touch(key: string, entry: Entry): void {
    this.entries.delete(key);
    this.entries.set(key, entry);
  }

  private trim(): void {
    // Conservative UTF-16 string-storage estimate, including keys/source/errors.
    let bytes = 0;
    const sizes = new Map<string, number>();
    for (const [key, entry] of this.entries) {
      const size = 2 * (key.length + entry.source.length + entry.translation.text.length + entry.translation.error.length);
      sizes.set(key, size);
      bytes += size;
    }
    while (this.entries.size > MAX_FILE_TRANSLATIONS || bytes > MAX_FILE_TRANSLATION_BYTES) {
      const oldest = this.entries.keys().next().value;
      if (oldest === undefined) break;
      bytes -= sizes.get(oldest)!;
      this.entries.delete(oldest);
    }
  }
}

export const fileTranslations = new FileTranslationsStore();
