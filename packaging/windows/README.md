# Windows setup and MSI packaging

## Install, update, and recover

For normal installs and updates, download `agent-sessions_<version>_windows_amd64_setup.exe`
from the release. Close Agent Sessions (including other running copies) before
starting setup, and run it as your normal Windows user, not as administrator.
The unsigned download may trigger SmartScreen; choose **More info → Run anyway**
only for a release you trust. Setup is per-user and requires the WebView2 Runtime
(included with Windows 11).

Setup validates its bundled MSI, then removes the existing per-user installation
in a **separate Windows Installer transaction** before installing the new MSI.
An already-absent product is accepted, but a removal failure stops the update.
This is deliberately not an atomic MSI major upgrade: if removal succeeds and
installation subsequently fails, the previous version may no longer be installed.
Do not assume it will be restored automatically. Setup does not forcibly kill a
running app; close it first and address any Windows Installer file-in-use prompts.

Each attempt keeps the extracted `payload.msi` and diagnostic logs in a unique
run directory beneath `%LOCALAPPDATA%\ginkcode\Agent Sessions\Setup`, on both
success and failure. Use the location reported by setup to find the exact run.
On failure, retain that directory, fix the reported cause, and retry setup or
manually install the retained MSI. You can also download the release's `.msi`
for manual installation/recovery. To capture a manual installation log:

```powershell
msiexec.exe /i "C:\path\to\agent-sessions_<version>_windows_amd64.msi" /L*v "$env:TEMP\agent-sessions-install.log"
```

The standalone MSI remains a supported manual fallback with its own normal
Windows Installer upgrade behavior; it does not perform the wrapper's separate
removal flow. Both artifacts install in `%LOCALAPPDATA%\Programs\Agent Sessions`.
Neither setup nor the MSI is code-signed.

## Build

`make package-windows` cross-compiles the desktop app, builds a per-user MSI
with wixl, and wraps that exact MSI in `*_setup.exe` in `dist/`. In addition to
the existing Wails/Go/Node tools, install `wixl` (0.102 or newer), `jq`, `python3`,
and `gcc-mingw-w64-x86-64` (including `x86_64-w64-mingw32-windres`).

To wrap an existing MSI without rebuilding the app:

```sh
make windows-setup MSI=/path/to/agent-sessions.msi VERSION=0.9.36 \
  SETUP_OUT=dist/agent-sessions_0.9.36_windows_amd64_setup.exe
# Equivalent script interface:
bash packaging/windows/build-setup.sh /path/to/agent-sessions.msi /path/to/setup.exe 0.9.36
```

The script stages the root `go.mod`/`go.sum`, non-test setup sources, embedded MSI,
SHA-256/version metadata and an explicit `asInvoker` manifest in a temporary
module tree. It cross-compiles a Windows amd64 GUI executable with CGO disabled
and cleans staging on exit; no payload or generated metadata is left in the
repository. `GO` and `WINDRES` can override the build tools. The expected MSI
version is the numeric `major.minor.patch` after stripping a leading `v` and any
release suffix; it must match the supplied MSI's `ProductVersion`. Setup checks
that match at runtime before removal, so building requires no host `msiinfo` tool.

Release publication includes both the MSI and setup executable, and `SHA256SUMS`
covers each artifact. Root `go test ./...` includes the platform-neutral setup
state-machine tests. CI also builds the Windows adapters natively and a staged,
bundled wrapper using a non-MSI fixture on Linux; it never executes setup or
installs/uninstalls a product. These compile checks do not replace the isolated
Windows transaction tests below.

## Fullscreen compatibility default

The MSI enables **Disable fullscreen optimizations** for the installed
`agent-sessions.exe`. This avoids a fullscreen cursor freeze observed on a
multi-monitor Windows system. There is no change to fullscreen window dimensions
or application rendering code.

The setting is a `REG_SZ` value under the current user's
`Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers` key,
named with the full executable path. A new value contains
`~ DISABLEDXMAXIMIZEDWINDOWEDMODE`. Existing unrelated compatibility flags are
preserved. Uninstall removes only this token, even if it was originally set
manually, and deletes that executable's value only if no other settings remain.
Other executables and the Layers key itself are never deleted.

This applies to MSI installations/updates, not portable builds in other paths.
An already-running app needs to be restarted. Users can subsequently change the
checkbox in the executable's Compatibility properties.

`appcompat-ca.c` is an installer-only x64 DLL embedded in the MSI, not an app
runtime dependency. Immediate preparation captures the existing value;
impersonated deferred actions update HKCU and rollback actions restore the exact
snapshot on failure. Early transactional major-upgrade removal precedes the new
snapshot, so old uninstall cleanup cannot remove the new installation's setting.
Concurrent changes are detected where possible rather than blindly overwritten.

## Validation

Before shipping the wrapper, use an isolated Windows account/VM and test fresh
installation, an already-absent product, same-version reinstall, upgrade, removal
failure, install failure after successful removal, and a reboot-required result.
Confirm malformed/wrong-version payloads stop before removal, log/recovery paths
remain available, and retrying the retained MSI works. Test with the app closed
and with file-in-use prompts, and confirm setup does not request elevation.
Never execute an installer fixture against a live installation.

The portable tokenizer/snapshot tests need only a native C compiler:

```sh
gcc -std=c11 -Wall -Wextra -Werror -pedantic -fsanitize=address,undefined \
  packaging/windows/appcompat-ca-test.c -o /tmp/appcompat-ca-test
/tmp/appcompat-ca-test
```

Build the production DLL separately when checking packaging without rebuilding
the desktop app:

```sh
x86_64-w64-mingw32-gcc -std=c11 -Wall -Wextra -Werror -O2 -shared \
  -o /tmp/appcompat-ca.dll packaging/windows/appcompat-ca.c -lmsi -ladvapi32
```

Pass `-D AppCompatDll=/tmp/appcompat-ca.dll` alongside the existing payload,
icon, and version definitions to wixl (see `make package-windows`). Inspect the
MSI's `CustomAction` and `InstallExecuteSequence` tables, not only compiler exit
status. Expected DLL action types: preparation `1`, rollback `1281`, apply `1025`
(impersonated and synchronous). `RemoveExistingProducts` must follow
`InstallInitialize`, preparation must follow old-product removal, and rollback
must precede apply, all before `InstallFinalize`.

Before release, exercise real MSI transactions on an isolated Windows test
account/VM, not over a live installation. For registry-isolated fixtures the DLL
supports a **compile-time only** `APPCOMPAT_REGISTRY_KEY` override; use a
throwaway HKCU test key and an MSI with distinct product/upgrade identities and
install directories. Never run fixture uninstall against the real product ID.

Check:

- Fresh install: missing, empty, and marker-only values produce the default.
- Existing unrelated flags survive; existing target tokens are not duplicated.
- Uninstall removes only the complete target token and preserves flags added
  after installation; missing values are harmless.
- Same-version and newer-version major upgrades keep the setting; old-path
  cleanup occurs before application at the new path.
- Force failure after apply and check exact rollback restoration for install,
  uninstall, and upgrade (including absence versus empty values).
- Invalid registry types fail without overwriting; neighboring values survive.
- Paths containing spaces/non-ASCII characters work, and the registry entry is
  written to the installing user's hive without elevation.
- Finally install the actual package, confirm the checkbox, relaunch, and test
  F11 cursor movement and exit on the affected monitor.

Successful portable tests or a placeholder-payload MSI compile do not establish
that Windows registry writes or MSI transaction rollback work end to end.
