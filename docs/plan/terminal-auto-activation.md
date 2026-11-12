# Terminal auto activation setting

## Goal

Keep automatic internal-terminal tab selection enabled by default, but let users disable it so browsing or focusing sessions does not interrupt their chosen shell or agent tab.

## Design

- Add a persistent, default-enabled terminal preference for automatically activating the matching terminal tab when selecting a session/workspace or clicking/focusing its transcript.
- Expose a checkbox in Settings with a clear explanation of what it controls.
- Gate automatic selection through the shared terminal-selection entry point so all browsing/focus callers obey the current preference immediately.
- Preserve explicit terminal actions: selecting a terminal tab, opening/resuming a session in a terminal, creating a shell, and other deliberate terminal controls continue to work.
- Preserve current matching/ranking behavior when enabled. Missing settings in existing installations keep the current enabled behavior.

## Decisions

- One preference covers both shell and agent tabs and applies across hosts.
- Disabling does not close terminals, hide the panel, or change the currently selected tab.
- Re-enabling takes effect on the next normal selection/focus action; no forced selection on setting changes.

## Risks

- An automatic call site might bypass the gated entry point; audit callers and add regression coverage.
- A default-enabled boolean must distinguish an absent stored setting from an explicitly disabled setting.
- Explicit terminal navigation must not accidentally be blocked by the preference.

## Release criteria

- The setting defaults to enabled for new and existing users and persists an explicit disabled value.
- Tests cover enabled and disabled automatic selection, unchanged active tab/panel state when disabled, live preference changes, and explicit terminal selection while disabled.
- Settings UI and automatic caller wiring are covered using existing test conventions.
- Relevant tests, frontend type checking, and frontend production build pass; record any checks not run separately.
