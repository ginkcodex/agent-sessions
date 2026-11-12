export interface FilePromptChoice {
  value: string;
  label: string;
  danger?: boolean;
}

/** One row of a prompt's item list: something it ends or discards. */
export interface FilePromptItem {
  /** Main label, simplified under its group: "Codex", a tab's title. */
  name: string;
  /** Small detail line: the host a terminal runs on, a chat's directory. */
  detail?: string;
  /** Row icon: a shell terminal, an agent terminal or a chat. */
  icon: 'terminal' | 'agent' | 'chat';
  /** The agent id whose own glyph the row shows, where known. */
  agent?: string;
  /** A small badge, like "working". */
  badge?: string;
}

/** A prompt's items under one heading, as "Terminals" or "Chats". */
export interface FilePromptGroup {
  title: string;
  /** The heading's icon: terminals or chats. */
  icon: 'terminal' | 'chat';
  items: FilePromptItem[];
  /** Items past the named cap, summed up as "and N more". */
  more?: number;
}

/** One coordinated decision at a time; overlapping destructive actions cancel. */
export class FilePromptStore {
  request = $state<{ title: string; message: string; choices: FilePromptChoice[]; groups: FilePromptGroup[] } | null>(null);
  private resolve: ((value: string | null) => void) | null = null;

  ask(title: string, message: string, choices: FilePromptChoice[], groups: FilePromptGroup[] = []): Promise<string | null> {
    if (this.resolve) return Promise.resolve(null);
    this.request = { title, message, choices, groups };
    return new Promise((resolve) => { this.resolve = resolve; });
  }

  answer(value: string | null): void {
    const resolve = this.resolve;
    this.resolve = null;
    this.request = null;
    resolve?.(value);
  }
}

export const filePrompt = new FilePromptStore();
