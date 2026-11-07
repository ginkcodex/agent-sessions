# Font settings — tasks

- **Plan:** [font-settings.md](../../plan/font-settings.md)
- **Progress:** 9/10 done

Planned 2026-11-07. The type scale comes first (FS-01, FS-02): every text
size in the docs preview and session content becomes body size × one of
eight ratios (today's sizes move by at most 0.55 px; inline code grows to
0.85em), before any setting is added. Each task is at
most four developer hours and ends with its tests passing: from
`frontend/` `npm test`, `npm run check` (0 errors) and `npm run build`.
No backend change is planned. SSR and static tests do not show layout or
paint; those are checked by hand in FS-09.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [FS-01](FS-01.md) | ✅ Done | Type scale: tokens, message text and docs preview | 4 h | — |
| [FS-02](FS-02.md) | ✅ Done | Type scale: other session parts, clipping audit, scan test | 4 h | FS-01 |
| [FS-03](FS-03.md) | ✅ Done | Preferences and CSS custom properties | 3 h | — |
| [FS-04](FS-04.md) | ✅ Done | Shared monospace font picker | 2 h | — |
| [FS-05](FS-05.md) | ✅ Done | Settings → Appearance section | 3 h | FS-03, FS-04 |
| [FS-06](FS-06.md) | ✅ Done | Editor and viewer font family and size | 4 h | FS-03 |
| [FS-07](FS-07.md) | ✅ Done | Project docs preview size | 1 h | FS-01, FS-03 |
| [FS-08](FS-08.md) | ✅ Done | Session text size and scroll position | 3 h | FS-02, FS-03 |
| [FS-09](FS-09.md) | 🟡 Partial: desktop apps not checked | Regression gate and manual check | 3 h | FS-05, FS-06, FS-07, FS-08, FS-10 |
| [FS-10](FS-10.md) | ✅ Done | Keep the viewer's place when the editor or docs size changes | 1 h | FS-06, FS-07 |
