# Rust code checks

Rust compiler diagnostics use an installed Cargo toolchain on the executing host.
The app offers its pinned rust-analyzer language server through the normal
managed-tools path. Project approval can separately cover the backend-owned
preparation steps shown in the proposal: installing an explicitly selected Rust
toolchain via an existing rustup and fetching locked dependencies. The app never
bootstraps rustup and never fetches project dependencies without a reviewed lock.
After project approval, **Check
Project** runs:

```
cargo check --workspace --offline --locked --message-format=json
```

Checks cover saved workspace files, default features and build-selected targets;
they do not save or check editor drafts. Generate and review Cargo.lock separately;
proposal preparation may populate dependency caches with `cargo fetch --locked`.
Cargo.toml has no general equivalent of Cargo's `--locked` CLI flag; the app must
supply the flag on every relevant invocation.
Missing or outdated locks and uncached dependencies fail rather than
silently fetching dependencies or updating locks. Operational failures retain
previous findings as stale.

Approval permits build scripts and procedural macros to execute with the user's
permissions and write build/cache output. Cargo offline mode is not a sandbox:
project code can still access the network.

## Initial discovery limits

Discovery reads metadata without executing Cargo, rustc, rustup or rust-analyzer.
It binds concrete installed tools, toolchain selection, Cargo manifests/locks and
reviewed host/project configuration. Inputs are re-enumerated before execution
and publication. Workspaces and local dependencies must remain inside the opened
authorized folder. Symlinks, external dependencies/workspace paths, ambiguous
selection and unsupported Cargo configuration fail closed. Initially only
`net.offline = true` Cargo configuration is accepted. Recursive `**` member globs,
rustup overrides and custom/path toolchains are not supported.

## Rust live analysis (pinned rust-analyzer only)

Rust navigation, hover and live diagnostics run only the app's exact pinned
managed rust-analyzer. An unrelated installed rust-analyzer, including a newer
user copy, is not a live server; saved Cargo checking stays independent. The
pinned server receives, both as initialize initializationOptions and as the
`rust-analyzer` workspace configuration section:

```json
{"cargo":{"metadataExtraArgs":["--locked"],"extraArgs":["--locked"]},"check":{"extraArgs":[]}}
```

`check.extraArgs` is deliberately empty. The pinned server appends
`cargo.extraArgs` to check arguments (`check_extra_args`), so repeating
`--locked` there would duplicate Cargo's clap flag. Metadata ignores ordinary
cargo extra arguments and gets `--locked` through `metadataExtraArgs`. Sysroot
metadata is hardcoded locked in the pinned project-model source. This is not a
sandbox: approved build scripts and procedural macros still execute with the
user's permissions and can use the network and write caches.

Each Cargo root is one language-server workspace (at most 16). The server owns
opened `.rs` files in that directory and covers the same files for dependency
queries. Lock, manifest and reviewed config changes are re-enumerated before
launch and before publication/status; drift stops the generation rather than
publishing stale analysis. Approval may include backend-owned preparation steps
(toolchain/cache setup). While `snapshot.preparation` is running, failed or
canceled, checks and live analysis stay inactive. Preparation failure never
enables tools, including unrelated providers in the same approval. Reattach the
project to rediscover and review setup after fixing the reported problem; there
is no in-place retry. A remembered approval still runs the reviewed preparation
on reconnect, including locked dependency fetching; completed setup is not cached
as a separate durable state.

There is no real-provider runtime validation in this change: tests use metadata
fixtures and fake processes. Windows cross-build/vet checks are not native
Windows tests; WSL/SSH runtime behavior is also unverified.

## Conditional compilation is not inferred from source

Rust `#[cfg]` attributes, Cargo features, target triples, crate membership and
build-script-provided cfg are not equivalents of Go custom build tags. The app
does not infer a Rust target or enable every feature mentioned in source. Approved
preparation may install targets explicitly declared in rust-toolchain.toml, but
saved checks keep Cargo's default features and build-selected targets; there is
no inferred target installation, feature expansion or cross-compilation.

Future Rust context handling must use a policy-compliant Cargo crate graph and
explicit target/toolchain facts, not speculative source-only inference. It must
also preserve the complete offline + locked metadata boundary before live
analysis or navigation can be offered.
