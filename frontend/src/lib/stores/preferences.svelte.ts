import { isTranscriptMode, type TranscriptMode } from '../transcript';
import { isTimeZoneMode, type TimeZoneMode } from '../date';
import { isTerminalFont } from '../terminalFont';
import { codeLanguageIds, type CodeLanguageId } from '../codeLanguages';

const TERMINAL_FONT_KEY = 'agent-sessions:terminal-font';

const STORAGE_KEY = 'agent-sessions:pane-widths';
const TIME_ZONE_KEY = 'agent-sessions:time-zone';
// Separate key so the pane-widths JSON format stays unchanged.
const TRANSCRIPT_MODE_KEY = 'agent-sessions:transcript-mode';
const AUTO_INSTALL_KEY = 'agent-sessions:auto-install-language-servers';
const DISABLED_LANGUAGES_KEY = 'agent-sessions:disabled-code-languages';

export interface PaneWidths {
  sidebar: number;
  sessionList: number;
}

export const LIMITS = {
  sidebar: { min: 200, default: 260 },
  sessionList: { min: 260, default: 340 },
  transcriptMin: 380,
};

export class PreferencesStore {
  widths = $state<PaneWidths>({
    sidebar: LIMITS.sidebar.default,
    sessionList: LIMITS.sessionList.default,
  });
  // The level each session opens at; the header selector overrides it per session.
  transcriptMode = $state<TranscriptMode>('activity');
  // Zone for message and session timestamps.
  timeZone = $state<TimeZoneMode>('utc');
  terminalFont = $state('');
  // Like Zed, download a missing language server a project needs without a click.
  autoInstallLanguageServers = $state(true);
  // Languages that get no code checks or navigation; every language is on by default.
  disabledLanguages = $state.raw<CodeLanguageId[]>([]);

  init(): void {
    if (typeof localStorage === 'undefined') return;

    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (typeof parsed.sidebar === 'number' && typeof parsed.sessionList === 'number') {
          this.widths = {
            sidebar: this.clamp(parsed.sidebar, LIMITS.sidebar.min),
            sessionList: this.clamp(parsed.sessionList, LIMITS.sessionList.min),
          };
        }
      }
    } catch {
      // Ignore JSON parse errors and keep defaults
    }

    try {
      const mode = localStorage.getItem(TRANSCRIPT_MODE_KEY);
      if (isTranscriptMode(mode)) this.transcriptMode = mode;
    } catch {
      // Storage disabled; keep Activity
    }

    try {
      const zone = localStorage.getItem(TIME_ZONE_KEY);
      if (isTimeZoneMode(zone)) this.timeZone = zone;
    } catch {
      // Storage disabled; keep UTC
    }

    try {
      const font = localStorage.getItem(TERMINAL_FONT_KEY);
      if (isTerminalFont(font)) this.terminalFont = font;
    } catch {
      // Storage disabled; keep the app default.
    }

    try {
      this.autoInstallLanguageServers = localStorage.getItem(AUTO_INSTALL_KEY) !== 'off';
    } catch {
      // Storage disabled; keep automatic downloads on.
    }

    try {
      const saved = JSON.parse(localStorage.getItem(DISABLED_LANGUAGES_KEY) ?? '[]');
      if (Array.isArray(saved)) this.disabledLanguages = codeLanguageIds(saved);
    } catch {
      // Unreadable or disabled storage; keep every language on.
    }

    if (typeof window !== 'undefined') {
      window.addEventListener('resize', () => this.handleWindowResize());
      this.handleWindowResize();
    }
  }

  equalizeSidebarAndSessionList(): void {
    const width = this.clamp(
      (this.widths.sidebar + this.widths.sessionList) / 2,
      Math.max(LIMITS.sidebar.min, LIMITS.sessionList.min),
    );
    this.widths = { sidebar: width, sessionList: width };
    this.ensureTranscriptVisible();
    this.save();
  }

  equalizeNavigationAndTranscript(totalWidth: number): void {
    const navigationWidth = this.clamp(
      totalWidth / 2,
      LIMITS.sidebar.min + LIMITS.sessionList.min,
      totalWidth - LIMITS.transcriptMin,
    );
    // Keep the directory width unless the session list would hit its minimum.
    const sidebar = this.clamp(
      this.widths.sidebar,
      LIMITS.sidebar.min,
      navigationWidth - LIMITS.sessionList.min,
    );
    this.widths = { sidebar, sessionList: navigationWidth - sidebar };
    this.save();
  }

  setSidebarWidth(width: number): void {
    this.widths.sidebar = this.clamp(width, LIMITS.sidebar.min);
    this.ensureTranscriptVisible();
    this.save();
  }

  setSessionListWidth(width: number): void {
    this.widths.sessionList = this.clamp(width, LIMITS.sessionList.min);
    this.ensureTranscriptVisible();
    this.save();
  }

  setTranscriptMode(mode: TranscriptMode): void {
    this.transcriptMode = mode;
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(TRANSCRIPT_MODE_KEY, mode);
    } catch {
      // Storage quota or disabled
    }
  }

  setTerminalFont(font: string): void {
    if (!isTerminalFont(font)) return;
    this.terminalFont = font;
    try {
      localStorage.setItem(TERMINAL_FONT_KEY, font);
    } catch {
      // Storage disabled; selection still applies for this run.
    }
  }

  setAutoInstallLanguageServers(on: boolean): void {
    this.autoInstallLanguageServers = on;
    try {
      localStorage.setItem(AUTO_INSTALL_KEY, on ? 'on' : 'off');
    } catch {
      // Storage disabled; the choice still applies for this run.
    }
  }

  setLanguageEnabled(id: CodeLanguageId, on: boolean): void {
    const off = this.disabledLanguages.filter(l => l !== id);
    this.disabledLanguages = codeLanguageIds(on ? off : [...off, id]);
    try {
      localStorage.setItem(DISABLED_LANGUAGES_KEY, JSON.stringify(this.disabledLanguages));
    } catch {
      // Storage disabled; the choice still applies for this run.
    }
  }

  setTimeZone(zone: TimeZoneMode): void {
    this.timeZone = zone;
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(TIME_ZONE_KEY, zone);
    } catch {
      // Storage quota or disabled
    }
  }

  private handleWindowResize(): void {
    this.ensureTranscriptVisible();
  }

  private ensureTranscriptVisible(): void {
    if (typeof window === 'undefined') return;
    const windowWidth = window.innerWidth;
    const available = windowWidth - LIMITS.transcriptMin;

    if (available > 0 && this.widths.sidebar + this.widths.sessionList > available) {
      const ratio = this.widths.sidebar / (this.widths.sidebar + this.widths.sessionList);
      this.widths.sidebar = Math.max(LIMITS.sidebar.min, Math.round(available * ratio));
      this.widths.sessionList = Math.max(LIMITS.sessionList.min, available - this.widths.sidebar);
    }
  }

  private clamp(val: number, min: number, max = Infinity): number {
    return Math.max(min, Math.min(max, val));
  }

  private save(): void {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(this.widths));
    } catch {
      // Storage quota or disabled
    }
  }
}

export const preferences = new PreferencesStore();
