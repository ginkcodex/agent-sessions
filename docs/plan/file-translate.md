# Whole-file translation

## Goal

Translate an entire text file from the file preview's right-click menu without
selecting text, using the same translation settings and result UI as session
content.

## Design

- Add a `Translate entire file` action to the file preview context menu when no
  text is selected. Keep selected-text Copy/Translate behavior unchanged.
- Pass the complete loaded source text, rather than rendered HTML or only the
  visible viewport, to the existing whole-content translation flow.
- Reuse the whole-content translation API and message-style loading/error,
  retry, original/translation toggle and retranslate semantics. Display the
  translated Markdown in place, without a new backend endpoint or file writes.
- Offer the action in rendered Markdown previews, the same surface as existing
  selected-text translation. Source, diff, image and editing views stay unchanged.
- Keep file translations in a window/session-memory cache independent of viewer
  lifetime (FT-02). Scope entries by host, project, path and target language;
  require exact original source equality before displaying or reusing a result.
  Closing with Esc, switching tabs or leaving preview does not cancel pending
  requests or discard completed results. No automatic requests on reopen.
- Bound the cache to 20 entries and 16 MiB of estimated retained string storage,
  evicting least recently used entries. App restart clears it; no disk storage.
  Changed source replaces the cached version when explicitly translated, and
  an older in-flight reply cannot overwrite the newer entry.
- Copy/print use displayed Markdown in preview; Find already observes the
  rendered preview DOM. Preserve original/translation toggle state on reopen.

## Decisions

- This is a preview action, not a file-tree action. No file reread is needed.
- Preserve the existing translation input limits and settings guidance. Never
  silently truncate a file or split it into independent translations.
- Translation is read-only and does not replace the original file.

## Risks

Large files may exceed existing translation limits or provider context limits;
report the existing error rather than sending partial content. Selection menus
must keep priority. An in-flight result must remain associated with the original
request even if the user changes files.

## Release criteria

- Right-click with no selection can translate a complete rendered Markdown
  preview from its source, independent of viewport/rendered DOM.
- Selected-text translation and existing preview menu actions still work.
- Closing/reopening preview reuses completed and pending translations without
  another request when host/project/path/language and source match. Source
  changes hide old results immediately; cache bounds and stale reply guards hold.
- Unavailable/non-text/empty/editing content does not expose whole-file translation.
- Regression tests cover eligibility, complete request payload, loading/result and
  error behavior. Frontend tests, type/component check and production build pass.
- Record any unperformed desktop/provider checks explicitly in task evidence.

Tasks: [file-translate](../tasks/file-translate/README.md).
