# Automatic Go build contexts

Go live analysis and navigation can use separate automatically inferred build
contexts when a source build constraint requires custom tags. These contexts
apply only to the executing host's GOOS/GOARCH. They do not switch targets,
enable cgo, or claim to describe the project's intended production build.

Discovery reads bounded source headers without running project tools. It keeps
the default context when possible and selects at most three additional canonical
tag sets. At most four gopls processes per project are started lazily, only after
ordinary explicit project approval and when needed by an opened file. No tag
editor or persisted per-project setting is introduced.

## Approval disclosure

The collapsed **What will run and what approval means** disclosure groups tools
by executable and lists inferred tag sets. A tool with tagged contexts is labeled
with its total build-context uses, including the default use, rather than calling
those contexts workspaces. Tools without tag metadata retain workspace labels.
The tag labels are display-only: the exact flags are bound into the existing
approval policy and composite project fingerprint, not authorized by display
text. Opening setup, discovering tags or opening a disclosure never approves or
launches a tool. A new context inventory changes the fingerprint and requires the
ordinary approval buttons after rediscovery.

## Limits

Each opened file has one immutable approved context for its document lifetime.
If its constraints need a different or undiscovered context, use **Rediscover
checkers** and approve the new proposal rather than expecting a running server
to change flags. Unsupported or uncovered constraints retain default analysis
with partial-coverage guidance.

Unsaved dependency changes sent to one context are not synchronized into sibling
gopls processes. Saved changes can inform already-running contexts, but there is
no cross-context draft-consistency guarantee. Satisfying a file's tags also does
not guarantee that its package builds: package, cgo, toolchain and dependency
failures can still prevent navigation. Cross-platform builds are not inferred.
