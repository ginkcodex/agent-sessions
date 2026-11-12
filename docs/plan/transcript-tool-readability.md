# Transcript tool readability

## Goal
Make session content at Activity and All levels scannable without losing tool details.

## Design and decisions
- Shared tool cards show a useful command/path summary instead of a prominent opaque call ID (the ID remains available on hover).
- Recognize JSON text-content envelopes, including Codex input_text blocks, and display their actual text/newlines. Unknown or mixed JSON stays verbatim to avoid losing metadata or rounding large integers; plain text is never escape-decoded. Session search includes decoded output values alongside raw output.
- Long outputs start as a short preview with Show more/Show less, instead of a tall nested scroll area on every call. Search-matched cards open fully. Parameters remain independently collapsible with an explicit disclosure indicator.
- Use the session font scale, more comfortable line spacing and word wrapping without breaking every word. Preserve raw output copying and truncated-blob loading.
- Both levels share this presentation; their existing filtering semantics stay unchanged.

## Risks
Provider JSON shapes vary: unwrap only recognized all-text envelopes, preserving unknown or mixed payloads. Large-output rendering must stay bounded when collapsed. Expansion must not hide search matches. Native WebKit sizing and light/dark visual checks need manual validation.

## Release criteria
Unit/DOM tests cover envelopes, previews, expansion and search opening; frontend checks/build pass. Native visual QA is recorded honestly rather than assumed from DOM tests.
