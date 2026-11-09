# Java and PHP code intelligence

## Goal

Add Java and PHP to Settings → Code checks, with Go to Definition and live
opened-document diagnostics through the existing approved local/SSH/WSL server
pipeline. Preserve existing language behavior and honest coverage reporting.

## Design and decisions

- **Java:** Eclipse JDT Language Server (`jdtls` distribution), launched directly
  with a reviewed native Java runtime, not its Python/shell wrapper. Discover the
  distribution from a resolved `jdtls` command or conventional installation
  locations. Use a separate app-cache workspace per host project; do not write
  discovery state or server cache into the project or installed distribution.
  Inspect the JDK `release` file instead of executing a version probe. Require
  Java 25 for current JDT LS; the project's source level can be older.
- **PHP:** Intelephense's npm server, launched directly by reviewed Node.js with
  `--stdio` on Node.js 22+. Definition, references, hover and opened-file diagnostics are free;
  the server is proprietary and premium features are outside this feature.
  No PHP runtime, Composer execution or paid licence is required by the adapter.
- First release uses user-installed tools, as Ruby does. No background install,
  download, version probe or package/build command during discovery. Managed
  server/JDK installation is deferred; show actionable manual setup text.
- Reuse server lifecycle, push normalization, UTF-16 document synchronization,
  navigation freshness and external-file read grants. No new RPC contract.
  Java archive/decompiled `jdt:` URIs remain unsupported; source-file definition
  targets are supported. Do not convert virtual URIs into filesystem authority.
- Share the bounded nested-workspace scan. PHP boundaries are `composer.json`;
  standalone PHP files retain root fallback. Group nested Maven/Gradle module
  manifests under the outermost Java build boundary so JDT LS imports one build;
  independent sibling projects use separate instances. Markerless Java uses root
  fallback. Exclude dependency/generated trees (`vendor`, `target`, `.gradle`).
- Java build import can execute Maven/Gradle plugins, wrappers and annotation
  processors and can download dependencies after project execution approval.
  Make this visible in provider/setup text. Trust is consent, not a sandbox.
- Checks cover opened documents, not a saved whole-project `mvn`, `gradle`,
  `php -l` or PHPStan checker. Unopened/external changes remain honestly unwatched.
  Forward app-observed saved-file changes to the new servers for index freshness.
- Java/PHP settings default on like other new registry entries. Disabling a
  language skips tool discovery and prevents execution/navigation for it.

## Work breakdown

See [tasks](../tasks/code-diagnostics-java-php/README.md). Each implementation
commit includes its task evidence and all progress indexes. Do not mark real
server or native/remote QA passed on the strength of fake-server tests.

## Risks

- JDT LS cold initialization is slower than other servers; check the current
  timeout against a real server and add a bounded adapter-specific allowance if
  necessary, without relaxing every server's timeout.
- Distribution layouts/runtime versions vary. Unsupported/incomplete layouts
  must be reported without launching arbitrary wrappers. Bind launch inputs and
  loaded server/runtime identities into existing approval and revalidation.
- JDT LS protocol extensions and Java archive URIs exceed the existing client.
  Only acknowledge safe, bounded requests needed for source navigation; deny
  edits, commands and arbitrary server requests as before.
- Intelephense indexing can take time; fake protocol coverage cannot establish
  real diagnostics/navigation behavior. Retain known external-file limitations.
- Native Windows local live servers remain unsupported by the existing route;
  Windows users use a WSL/SSH workspace. Remote servers must be rebuilt to deploy
  this backend change. No Windows build is requested by this task.

## Release criteria

1. Java/PHP appear in language settings and setup guidance with accurate tools,
   runtime requirements, trust notices and coverage limitations.
2. Metadata-only discovery handles absent tools, incomplete installs, runtime
   selection, contained roots, nested boundaries and disabled languages.
3. Approved sessions synchronize Java/PHP documents, publish diagnostics and
   route source-file definitions; existing languages retain their behavior.
4. Targeted backend/frontend tests, full Go race tests, frontend check/build and
   lint have recorded outcomes. Failures and unrun checks are explicit.
5. Real-server Java/PHP checks demonstrate initialization, source definition and
   error diagnostics where available; remaining GUI/SSH/WSL/platform QA is
   tracked, not silently counted as complete.

## References

- [JDT LS requirements/startup](https://github.com/eclipse-jdtls/eclipse.jdt.ls)
- [Intelephense installation](https://github.com/bmewburn/intelephense-docs/blob/master/installation.md)
- [Intelephense free/premium features](https://github.com/bmewburn/vscode-intelephense)
- [Existing workspace model](code-diagnostics-workspaces.md)
- [Existing execution trust](code-diagnostics-trust.md)
