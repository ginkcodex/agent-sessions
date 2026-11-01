#!/usr/bin/env bash
# Build the Windows GUI setup wrapper without generated files in the source tree.
set -euo pipefail

if [ "$#" -ne 3 ]; then
    printf 'Usage: %s MSI OUTPUT VERSION\n' "$0" >&2
    exit 2
fi

msi=$1
output=$2
version=${3#v}
# Windows Installer compares the numeric ProductVersion, not a release suffix.
version=${version%%[-+]*}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf 'VERSION must have a numeric major.minor.patch (optional release suffix): %s\n' "$3" >&2
    exit 2
fi
if [ ! -f "$msi" ]; then
    printf 'MSI not found: %s\n' "$msi" >&2
    exit 1
fi

# An explicit asInvoker manifest prevents Windows' filename-based installer
# detection from elevating this per-user wrapper. Resources stay in staging.
windres=${WINDRES:-}
if [ -z "$windres" ]; then
    for candidate in x86_64-w64-mingw32-windres windres; do
        if command -v "$candidate" >/dev/null 2>&1; then
            windres=$candidate
            break
        fi
    done
fi
if [ -z "$windres" ] || ! command -v "$windres" >/dev/null 2>&1; then
    printf 'Windows resource compiler not found: install gcc-mingw-w64-x86-64 or set WINDRES.\n' >&2
    exit 1
fi

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
mkdir -p -- "$(dirname -- "$output")"
output=$(cd -- "$(dirname -- "$output")" && pwd)/$(basename -- "$output")
stage=$(mktemp -d "${TMPDIR:-/tmp}/agent-sessions-setup.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
source_dir="$root/packaging/windows/setup"
bundle_dir="$stage/packaging/windows/setup"
mkdir -p -- "$bundle_dir"
cp -- "$root/go.mod" "$root/go.sum" "$stage/"
for source in "$source_dir"/*.go; do
    case "$source" in *_test.go) continue ;; esac
    cp -- "$source" "$bundle_dir/"
done
cp -- "$msi" "$bundle_dir/payload.msi"
cp -- "$source_dir/setup.ico" "$bundle_dir/setup.ico"
cat > "$bundle_dir/setup.manifest" <<'MANIFEST'
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="amd64" name="ginkcode.AgentSessions.Setup" type="win32"/>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security><requestedPrivileges>
      <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
    </requestedPrivileges></security>
  </trustInfo>
  <dependency><dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
  </dependentAssembly></dependency>
  <application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings>
    <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
    <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
  </windowsSettings></application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application>
    <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
  </application></compatibility>
</assembly>
MANIFEST
printf '1 24 "setup.manifest"\n2 ICON "setup.ico"\n' > "$bundle_dir/setup.rc"
(cd -- "$bundle_dir" && "$windres" -O coff -F pe-x86-64 -i setup.rc -o rsrc_windows_amd64.syso)
if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum "$bundle_dir/payload.msi")
else
    digest=$(shasum -a 256 "$bundle_dir/payload.msi")
fi
digest=${digest%% *}
printf '//go:build setupbundle\n\npackage main\n\nconst payloadSHA256 = "%s"\nconst payloadVersion = "%s"\n' \
    "$digest" "$version" > "$bundle_dir/bundle_metadata.go"

# Runtime checks the embedded MSI ProductVersion against this expected version;
# no host-side MSI inspection tool is required to build the wrapper.
(cd -- "$stage" && GOWORK=off CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "${GO:-go}" build \
    -mod=readonly -tags setupbundle -trimpath -ldflags '-H windowsgui -s -w' \
    -o "$output" ./packaging/windows/setup)
printf 'Windows setup wrapper: %s (MSI version %s, SHA-256 %s)\n' "$output" "$version" "$digest"
