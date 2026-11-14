# CPC-08 — Desktop, provider and remote QA

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CPC-05, CPC-07
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Exercise inventory and invocation display end to end and record results in
`docs/plan/chat-plugin-commands-qa.md`. Cover the CPC-02 support matrix, local
and available WSL/SSH hosts. Use fixture/fake-agent scenarios before any real
plugin execution; obtain approval for a real plugin/model turn in a disposable
workspace because plugin commands may change files or spend tokens.

## Likely files

- `docs/plan/chat-plugin-commands-qa.md`
- This task/group README and all progress indexes

## Acceptance criteria and tests

- Drive the app: inventory listings, command discovery/arguments, keyboard
  selection, cancel, reattach/reopen and host switching.
- Verify commandless and disabled plugins, stale loaded-vs-installed state,
  errors and unsupported providers; listing does not mutate configuration.
- Check user invocation visibility once live and saved, without expanded
  skill instructions. Exercise real commands only with explicit consent.
- Run affected Go suites, `make test` where available and frontend
  tests/check/build; record output and distinguish failures from unavailable
  platform/environment checks. Unrun QA is never marked a pass.
