# LFV-03 — Desktop QA with the reported 2.5 MB file

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** LFV-02
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope

In the desktop app (Linux and a Windows test build), open
`.ua/fingerprints.json` (2.5 MB, 107,506 lines) from Go to File.

## Acceptance criteria

- The file opens within about a second and the window stays responsive.
- Scrolling to the end, Ctrl/Cmd+F, wrap on and off, and switching to a small
  file's tab and back all work without stalls.
- Arrowing through the file picker past the large file keeps focus in the
  picker.
- A small file still opens in the normal viewer (gutter line selection, blame).
