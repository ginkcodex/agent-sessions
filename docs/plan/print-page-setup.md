# Print page setup

## Goal

Make printed messages, conversations and Markdown files look similar across
Linux, macOS and Windows, with configurable page setup in the existing print
editor. Exact cross-engine pagination is not required.

## Design

- Add A4/Letter, portrait/landscape and a uniform margin (0–40 mm) control to
  Print / Save as PDF. Defaults: A4, portrait, 14 mm. Keep the last selection
  while the editor remains mounted; no new persistent preference schema.
- Carry `PrintPageSetup { paper: 'a4' | 'letter', orientation: 'portrait' |
  'landscape', marginMm: number }` on `PrintDocument`. Generate a document-level
  `@page` rule with explicit size/margins before opening the system print dialog.
- Add `App.PrintWindowWithSetup(setup)` and optional `Desktop.PrintWithSetup`.
  Preserve the argumentless PrintWindow endpoint for existing callers. Validate
  values in Go and TypeScript; pass validated setup through the window router.
  Frontend uses the new endpoint when available, falling back for older apps.
- Linux: use WebKitPrintOperation with a fresh GtkPageSetup and GtkPrintSettings,
  selected paper/orientation/margins and scale 100%, attached to the calling
  window. Do not modify global printer settings. CSS and native margins describe
  the same page area, not extra content padding.
- macOS: adapt the existing WKWebView print bridge to selected paper dimensions,
  orientation, margins and 100% scaling instead of inherited print defaults.
- Windows: retain WebView2's interactive window.print() path and use the shared
  CSS page setup. Native dialogs can override CSS; show concise guidance to
  match paper/orientation, keep scale 100%, and turn off browser headers/footers.
- Preview shows the selected printable width (not a simulated paginated PDF).
  The actual sheet uses the selected width too, avoiding viewport-width layout.

## Decisions

- Uniform margins keep the initial UI small; no independent per-edge controls.
- Scale stays at 100%; no custom rendering engine or fixed pagination engine.
- Headers/footers are not exposed as an app toggle since not every native
  backend supports controlling them. Explain native overrides rather than
  promise settings that the webview cannot enforce.

## Risks

Native dialogs/printer hardware may override requested margins or paper. CSS
paged-media support varies between webviews. Font metrics remain platform
specific. GTK print operations are asynchronous; keep their ownership valid
through completion/cancellation. Native visual checks must be recorded honestly.

## Release criteria

- Setup works for conversation, single-message and file printing.
- Validation rejects invalid/non-finite sizes; old print calls remain compatible.
- CSS and native adapters receive matching setup without additive padding.
- Tests cover defaults, dimensions, validation, UI changes, document dispatch,
  style lifecycle and per-window native routing. Frontend tests/check/build and
  Go tests pass; compile Linux desktop and Windows adapters where available.
- Native visual comparison on all three systems is recorded separately; do
  not claim consistent PDF output solely from automated tests.

Tasks: [print-page-setup](../tasks/print-page-setup/README.md).
