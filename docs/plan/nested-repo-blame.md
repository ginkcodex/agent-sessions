# Blame in nested repositories

Planned 2026-10-24. Tasks: [nested-repo-blame](../tasks/nested-repo-blame/README.md).

Report: opening a directory that is not a Git repository but contains several
repositories, line blame never appears.

Cause: the viewer shows blame only when the listing came from Git
(`files.listing.git`). A non-Git root is listed by walking the tree, so `git` is
false and blame is hidden for every file, even inside nested repositories.
`files.BlameFile` already discovers Git from the file's own directory, so the
backend can answer; only the gate is wrong.
