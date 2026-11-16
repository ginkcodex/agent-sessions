# Linux native window theme

## Goal

Give Linux the same Light/Dark/System behavior as macOS while retaining native title-bar controls and respecting the user's GTK theme. Use the existing `SetWindowTheme` bridge; no new preference or dependency.

## Design

- Implement the Linux branch of `setWindowTheme` using GTK 4 on the main thread. The frontend already sends saved modes and serializes explicit changes.
- Request the current GTK theme's matching appearance rather than choosing a different desktop theme or drawing replacement window controls.
- Use `gtk-application-prefer-dark-theme`: true for Dark, false for Light. System calls `gtk_settings_reset_property` to remove the override and resume current/future session values. This compatibility API remains functional in GTK 4.20+ despite deprecation.
- GTK's preference is process/display-wide: the latest explicit selection or initialization wins for native chrome across app windows on that display. The router still targets the calling window's callback; Linux applies that request at GTK's supported scope. No cross-window web-theme propagation is added.
- Support the project's GTK 4 baseline (Ubuntu 24.04+) as well as newer GTK versions. Native theme APIs must not make the minimum runtime version newer.
- Keep macOS behavior unchanged and Windows a no-op. Retain the existing Go mode validation and error handling.

## Decisions and scope

Linux decorations may be GTK client-side decorations or compositor/window-manager decorations. GTK styling can be controlled; external decorations may ignore appearance requests. Do not force custom colors, replace decorations, change system settings, or install a different theme. Prefer per-window behavior; if GTK exposes only application/display-wide appearance, document that limitation explicitly rather than claiming window isolation.

## Risks

GTK theme and API versions differ; older themes may lack distinct variants. Light requests the theme's default variant, which may itself be dark for explicitly dark-only theme names (for example Yaru-dark). Missing dark variants may fall back to GTK's default dark theme. We do not rename or replace the selected theme to work around this. Application-wide GTK preferences could affect other windows in this process. Native appearance may affect webview media queries, so preserve the existing no-feedback-loop behavior. X11/Wayland and different desktops require actual native validation; headless probes do not establish visual parity.

## Release criteria

- Native tests exercise Light/Dark/System, reset behavior, repeated toggles, theme preservation and relevant GTK scope/version behavior using the actual helper.
- Existing Go/frontend bridge tests remain green; Linux production desktop compiles; Windows no-op still compiles.
- Record actual checks and limitations in tasks. Visually test native title bars and controls on Linux desktops, including System changes, restart and multiple windows; do not count planned checks as passes.

## Tasks

See [Linux native window theme tasks](../tasks/linux-window-theme/README.md).
