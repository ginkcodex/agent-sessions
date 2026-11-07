# Selected-text menu and contextual translation

Planned 2026-10-24. Tasks: [selection-translate](../tasks/selection-translate/README.md).

Right-clicking a transcript message offers Copy, Translate and Print / Save as
PDF. With text selected the app returned to the native menu, which offers little
in the desktop webview. A selection inside one message's rendered body now gets
its own menu: **Copy** (the selection) and **Translate to {language}**. It has no
Print/Save and no whole-message translation actions.

A short selection is ambiguous on its own, so the sentence(s) around it go to the
model as context. Only the selected text is translated. The result appears in a
popover at the selection; the message itself is not changed, and nothing is kept
after the popover closes.

Invariants:
- No selection → the existing message menu is unchanged.
- A selection crossing messages, touching the header or its controls, or outside
  the rendered body keeps the native menu.
- Selection actions never call or mutate the whole-message translations store.
- Context is bounded (8 KiB UTF-8 client-side, checked again by the backend).
- Automated tests do not establish native selection, focus or placement
  behavior; report them separately.
