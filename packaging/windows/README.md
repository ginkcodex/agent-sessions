# Windows MSI packaging

`make package-windows` cross-compiles the desktop app and builds a per-user MSI
with wixl. In addition to the existing Wails/Go/Node tools, install `wixl`
(0.102 or newer), `jq`, `python3`, and `gcc-mingw-w64-x86-64`.

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
