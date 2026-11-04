GO ?= go
# Desktop builds are plain `go build` with these tags. `desktop` selects the
# Wails v3 entrypoint (cmd/agent-sessions/run_gui.go). `production` is Wails
# v3's release switch: it turns off the web inspector, the debug logger and
# the FRONTEND_DEVSERVER_URL dev-server proxy. `make dev` drops it.
GUI_TAGS ?= desktop,production
DEV_TAGS ?= desktop
# The Wails v3 CLI, only used for `generate icons` and `generate syso`. By
# default it runs at the version go.mod pins, so nothing needs installing.
# It is built without cgo: on Linux a cgo build links GTK 4 and WebKitGTK,
# which the Windows packaging runner does not have, and these two commands
# are pure Go.
WAILS3_VERSION ?= $(shell $(GO) list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)
WAILS3 ?= CGO_ENABLED=0 $(GO) run github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS3_VERSION)
# Release version: the single source is info.productVersion in wails.json.
VERSION ?= $(shell sed -n 's/^ *"productVersion": *"\([^"]*\)".*/\1/p' wails.json)
VERSION_PKG ?= github.com/ginkcode/agent-sessions/internal/version
LDFLAGS ?= -s -w -X $(VERSION_PKG).Version=$(VERSION)
ARCH ?= $(shell $(GO) env GOARCH)
NFPM ?= $(GO) run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0
# Windows/Linux use transparent artwork; macOS uses the Icon Composer tile.
# Keep these sources in sync with frontend/icon*.go (the runtime icons).
APPICON := frontend/assets/Icon-universal.png
MACOS_APPICON := frontend/assets/Icon-macOS.png

.PHONY: test test-ssh lint fmt cli fuzz-smoke golden build clean dev app gui-build check-gui-deps check-gui-linkage \
	package-linux package-macos package-windows windows-app \
	remote-servers conpty windows-setup version tags set-version tag untag release help check-app-icons

# Headless servers the desktop app deploys over SSH. CGO stays off: these
# binaries have no GUI. Cross targets need a Go toolchain that can produce
# them (CI does); `make remote-servers HOST=1` builds only the host pair.
# packaging/windows/agent-sessions.wxs lists one file per platform.
REMOTE_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
REMOTE_DIR ?= build/remote
# The ConPTY the Windows app ships in conpty\ next to its exe: Windows' own
# swallows OSC 10/11 color queries (see internal/term/conpty_windows.go).
# Bumping the version needs the new package's SHA-256.
CONPTY_VERSION ?= 1.25.260930003
CONPTY_SHA256 ?= 02b07b349af66d801159bdf9e440d4a1ce78bb951f37fc8609731665afdae7ee
CONPTY_DIR ?= build/conpty

test:
	$(GO) test -race ./...

# Opt-in SSH integration tests against a throwaway Docker sshd
# (internal/remote/sshtest). Needs docker, ssh and ssh-keygen.
test-ssh:
	AGENT_SESSIONS_SSH_DOCKER=1 $(GO) test -race -count=1 -run 'SSHD' ./internal/remote/... ./internal/app/...

lint:
	$(GO) tool golangci-lint run

fmt:
	$(GO) tool gofumpt -w .
	$(GO) tool golangci-lint fmt

cli:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/agent-sessions-cli ./cmd/agent-sessions-cli

# Gzipped headless servers in $(REMOTE_DIR), plus SHA256SUMS of those archives.
# The checksum covers the .gz bytes: the remote deploy verifies that file.
remote-servers:
	mkdir -p "$(REMOTE_DIR)"
	rm -f "$(REMOTE_DIR)"/SHA256SUMS "$(REMOTE_DIR)"/*.gz
	@set -e; \
	platforms='$(REMOTE_PLATFORMS)'; \
	if [ "$(HOST)" = 1 ]; then platforms="$$($(GO) env GOOS)/$$($(GO) env GOARCH)"; fi; \
	for pair in $$platforms; do \
		os=$${pair%/*}; arch=$${pair#*/}; \
		name="agent-sessions-cli-$$os-$$arch"; \
		echo "building $$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$(REMOTE_DIR)/$$name" ./cmd/agent-sessions-cli; \
		gzip -c -n "$(REMOTE_DIR)/$$name" > "$(REMOTE_DIR)/$$name.gz"; \
		rm -f "$(REMOTE_DIR)/$$name"; \
	done; \
	if command -v sha256sum >/dev/null 2>&1; then \
		(cd "$(REMOTE_DIR)" && sha256sum -- *.gz > SHA256SUMS); \
	else \
		(cd "$(REMOTE_DIR)" && shasum -a 256 -- *.gz > SHA256SUMS); \
	fi; \
	echo "remote servers in $(REMOTE_DIR):"; \
	cat "$(REMOTE_DIR)/SHA256SUMS"

# conpty.dll and OpenConsole.exe for Windows amd64 in $(CONPTY_DIR), with
# their license, from Microsoft's NuGet package. The package is kept in
# build/ and checked against CONPTY_SHA256.
conpty:
	@set -e; \
	pkg="build/Microsoft.Windows.Console.ConPTY.$(CONPTY_VERSION).nupkg"; \
	mkdir -p build "$(CONPTY_DIR)"; \
	if ! { [ -f "$$pkg" ] && echo "$(CONPTY_SHA256)  $$pkg" | sha256sum -c --status; }; then \
		curl -fsSL -o "$$pkg.tmp" "https://www.nuget.org/api/v2/package/Microsoft.Windows.Console.ConPTY/$(CONPTY_VERSION)"; \
		echo "$(CONPTY_SHA256)  $$pkg.tmp" | sha256sum -c --quiet; \
		mv "$$pkg.tmp" "$$pkg"; \
	fi; \
	python3 -c 'import sys, zipfile; z = zipfile.ZipFile(sys.argv[1]); [open(sys.argv[2] + "/" + n.split("/")[-1], "wb").write(z.read(n)) for n in sys.argv[3:]]' \
		"$$pkg" "$(CONPTY_DIR)" runtimes/win-x64/native/conpty.dll build/native/runtimes/x64/OpenConsole.exe; \
	cp packaging/windows/conpty-LICENSE.txt "$(CONPTY_DIR)/LICENSE.txt"; \
	echo "ConPTY $(CONPTY_VERSION) in $(CONPTY_DIR)"

fuzz-smoke:
	$(GO) test ./internal/provider/claude -run='^$$' -fuzz=FuzzRecord -fuzztime=60s

golden:
	$(GO) test ./... -update

build: cli

# The Linux desktop build links GTK 4 and WebKitGTK 6.0 through cgo, so it
# needs a C compiler and their development headers:
#   sudo apt install gcc pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
# (Ubuntu 24.04+, Debian 13+; Fedora 40+: gtk4-devel webkitgtk6.0-devel).
# macOS needs cgo too, for Cocoa/WebKit. Without a C compiler Go silently
# turns cgo off and the Wails build fails with obscure undefined symbols, so
# check first and print an actionable hint.
check-gui-deps:
	@[ "$$($(GO) env CGO_ENABLED)" = 1 ] || { \
		printf '%s\n' 'cgo is off: the desktop build needs a C compiler. Install gcc (or set CC).' >&2; \
		exit 1; \
	}
	@[ "$$($(GO) env GOOS)" != linux ] || pkg-config --exists gtk4 webkitgtk-6.0 || { \
		printf '%s\n' 'Missing GTK 4/WebKitGTK 6.0 development headers. Install libgtk-4-dev libwebkitgtk-6.0-dev.' >&2; \
		exit 1; \
	}

# Fail unless the Linux GUI binary links GTK 4 and WebKitGTK 6.0, and
# neither GTK 3 nor WebKit2GTK. A stray gtk3 tag or old cgo flags would
# otherwise produce a binary the .deb/.rpm dependencies do not cover.
GUI_BIN ?= build/bin/agent-sessions
check-gui-linkage:
	@set -e; \
	libs=$$(ldd "$(GUI_BIN)"); \
	echo "$$libs" | grep -q 'libgtk-4\.so' && echo "$$libs" | grep -q 'libwebkitgtk-6\.0\.so' || { \
		echo "$(GUI_BIN) does not link libgtk-4 and libwebkitgtk-6.0" >&2; exit 1; }; \
	if echo "$$libs" | grep -Eq 'libgtk-3\.so|libwebkit2gtk-'; then \
		echo "$(GUI_BIN) still links GTK 3 or WebKit2GTK:" >&2; \
		echo "$$libs" | grep -E 'libgtk-3\.so|libwebkit2gtk-' >&2; exit 1; \
	fi; \
	echo "$(GUI_BIN) links GTK 4 and WebKitGTK 6.0 only"

# Run the desktop app against the Vite dev server, with hot reload.
# Contract: a Wails v3 build without the `production` tag serves its assets
# from FRONTEND_DEVSERVER_URL when that is set (application.AssetFileServerFS
# proxies to it) and waits up to 5s for it to answer. Release builds carry
# `production`, which compiles that proxy out. frontend/dist must still exist
# because go:embed needs it at compile time; it is only built when missing.
# Vite runs on DEV_PORT (--strictPort: a busy port fails instead of moving).
# The version is stamped like a release, or the remote-server handshake
# would reject a "dev" build.
# NOTE: do not launch in headless CI; use `make gui-build` for compile checks.
DEV_PORT ?= 5173
dev: check-gui-deps
	@test -f frontend/dist/index.html || { cd frontend && npm run build; }
	@set -e; \
	(cd frontend && exec ./node_modules/.bin/vite --port $(DEV_PORT) --strictPort) & vite=$$!; \
	trap 'kill $$vite 2>/dev/null || true' EXIT; \
	trap 'exit 130' HUP INT TERM; \
	FRONTEND_DEVSERVER_URL=http://localhost:$(DEV_PORT) \
		$(GO) run -tags "$(DEV_TAGS)" -ldflags "$(LDFLAGS)" ./cmd/agent-sessions

# Compile the desktop binary at build/bin/agent-sessions with plain go build:
# npm run build must run first so frontend/dist exists for go:embed.
gui-build: check-gui-deps
	cd frontend && npm run check && npm run build
	$(GO) build -tags "$(GUI_TAGS)" -trimpath -ldflags "$(LDFLAGS)" -o build/bin/agent-sessions ./cmd/agent-sessions

app: gui-build

# Verify the icon source before packaging it. It stays 1024px so
# `wails3 generate icons` can derive every macOS/Windows representation.
check-app-icons:
	@for icon in "$(APPICON)" "$(MACOS_APPICON)"; do \
		case "$$(file -b "$$icon")" in \
			'PNG image data, 1024 x 1024, '*'RGBA'*) ;; \
			*) echo "$$icon must be a 1024x1024 RGBA PNG" >&2; exit 1 ;; \
		esac; \
	done

# Linux .deb and .rpm packages in dist/, wrapping the gui-build binary.
# Runtime dependencies are declared in packaging/nfpm.yaml. The package ships
# the headless server for every remote platform, since a Linux desktop can
# connect to a macOS host.
package-linux: check-app-icons gui-build
	$(MAKE) remote-servers
	mkdir -p dist
	VERSION=$(VERSION) ARCH=$(ARCH) $(NFPM) pkg --config packaging/nfpm.yaml --packager deb --target dist/
	VERSION=$(VERSION) ARCH=$(ARCH) $(NFPM) pkg --config packaging/nfpm.yaml --packager rpm --target dist/

# Universal macOS app wrapped in a .dmg in dist/. Runs on macOS only (cgo,
# lipo, plutil, codesign, hdiutil) and needs jq. Each architecture is a plain
# go build; lipo joins them. The bundle is assembled from
# packaging/macos/Info.plist.in, filled from wails.json's info, with an icon
# from `wails3 generate icons`. It is ad-hoc signed, not notarized.
# hdiutil intermittently fails with "Resource busy" on CI runners, so the
# .dmg step retries once.
# Go 1.26 itself needs macOS 12, so that is the deployment target.
MACOS_MIN ?= 12.0
MACOS_APP := build/bin/Agent Sessions.app
# Fills the Info.plist template's @PLACEHOLDERS@ with XML-escaped values.
MACOS_PLIST_JQ = def x: gsub("&"; "&amp;") | gsub("<"; "&lt;") | gsub(">"; "&gt;"); .info as $$i | $$tpl | gsub("@VERSION@"; $$version | x) | gsub("@MACOS_MIN@"; $$min | x) | gsub("@PRODUCT_NAME@"; $$i.productName | x) | gsub("@COMMENTS@"; $$i.comments | x) | gsub("@COPYRIGHT@"; $$i.copyright | x)
package-macos: check-app-icons check-gui-deps
	@command -v jq >/dev/null 2>&1 || { echo "jq not found: brew install jq" >&2; exit 1; }
	cd frontend && npm run build
	rm -rf build/darwin "$(MACOS_APP)" build/dmg
	mkdir -p build/darwin "$(MACOS_APP)/Contents/MacOS" "$(MACOS_APP)/Contents/Resources"
	@set -e; \
	for arch in arm64 amd64; do \
		echo "building darwin/$$arch"; \
		CGO_ENABLED=1 GOOS=darwin GOARCH=$$arch MACOSX_DEPLOYMENT_TARGET=$(MACOS_MIN) \
		CGO_CFLAGS="-O2 -mmacosx-version-min=$(MACOS_MIN)" CGO_LDFLAGS="-mmacosx-version-min=$(MACOS_MIN)" \
			$(GO) build -tags "$(GUI_TAGS)" -trimpath -ldflags "$(LDFLAGS)" \
			-o "build/darwin/agent-sessions-$$arch" ./cmd/agent-sessions; \
	done
	lipo -create -output "$(MACOS_APP)/Contents/MacOS/agent-sessions" \
		build/darwin/agent-sessions-arm64 build/darwin/agent-sessions-amd64
	$(WAILS3) generate icons -input "$(MACOS_APPICON)" \
		-macfilename "$(MACOS_APP)/Contents/Resources/iconfile.icns" -windowsfilename ""
	# The template keeps v2's bundle identifier, so macOS keeps the Apple
	# Events and notification permissions users already granted. It also
	# carries NSAppleEventsUsageDescription: Open in terminal drives
	# Terminal, iTerm2 or Ghostty with Apple Events, and without a usage
	# description macOS refuses them (-1743) instead of asking the user.
	jq -r --arg version '$(VERSION)' --arg min '$(MACOS_MIN)' --rawfile tpl packaging/macos/Info.plist.in \
		'$(MACOS_PLIST_JQ)' wails.json > "$(MACOS_APP)/Contents/Info.plist"
	plutil -lint "$(MACOS_APP)/Contents/Info.plist"
	# Servers go in before codesign so the signature covers them.
	$(MAKE) remote-servers REMOTE_DIR="$(MACOS_APP)/Contents/Resources/remote"
	codesign --force --deep --sign - "$(MACOS_APP)"
	mkdir -p build/dmg dist
	cp -R "$(MACOS_APP)" build/dmg/
	ln -s /Applications build/dmg/Applications
	# Leave room for filesystem overhead: 25% extra, with a 64 MiB minimum.
	@set -e; \
	staging_kib=$$(du -sk build/dmg | cut -f1); \
	extra_kib=$$(( (staging_kib + 3) / 4 )); \
	if [ "$$extra_kib" -lt 65536 ]; then extra_kib=65536; fi; \
	image_kib=$$((staging_kib + extra_kib)); \
	printf 'DMG staging: %s KiB; image capacity: %s KiB\n' "$$staging_kib" "$$image_kib"; \
	df -h build/dmg dist "$${TMPDIR:-/tmp}"; \
	hdiutil create -volname "Agent Sessions" -srcfolder build/dmg -size "$${image_kib}k" -ov -format UDZO dist/agent-sessions_$(VERSION)_macos_universal.dmg || \
		{ sleep 5; hdiutil create -volname "Agent Sessions" -srcfolder build/dmg -size "$${image_kib}k" -ov -format UDZO dist/agent-sessions_$(VERSION)_macos_universal.dmg; }
	# The staging link to /Applications must not outlive the image: language
	# servers watching this repo would follow it.
	rm -rf build/dmg

# Wrap an existing MSI without rebuilding the desktop app or modifying sources.
MSI ?= dist/agent-sessions_$(VERSION)_windows_amd64.msi
SETUP_OUT ?= dist/agent-sessions_$(VERSION)_windows_amd64_setup.exe
windows-setup:
	GO="$(GO)" bash packaging/windows/build-setup.sh "$(MSI)" "$(SETUP_OUT)" "$(VERSION)"

# The Windows amd64 desktop app at $(WINDOWS_BUILD_DIR)/bin/agent-sessions.exe,
# cross-compiled without cgo (Wails v3 on Windows needs none). Needs jq.
# $(WINDOWS_BUILD_DIR)/windows is regenerated each run so the resources
# follow the current sources: icon.ico from `wails3 generate icons`, the
# manifest from packaging/windows/agent-sessions.exe.manifest, info.json
# from wails.json's info with VERSION, and the .syso `wails3 generate syso`
# links from them. Go only links a .syso from the package directory, so the
# build copies it to $(WINDOWS_SYSO) for its duration. The file is created
# exclusively and removed on exit; if one is already there, another Windows
# build is running or a killed one left it behind, and the build stops.
WINDOWS_BUILD_DIR ?= build
WINDOWS_SYSO := cmd/agent-sessions/zz_agent_sessions_resources_windows_amd64.syso
windows-app: check-app-icons
	@command -v jq >/dev/null 2>&1 || { echo "jq not found: install it (apt install jq)" >&2; exit 1; }
	cd frontend && npm run build
	@set -e; \
	out='$(abspath $(WINDOWS_BUILD_DIR))'; res="$$out/windows"; syso='$(WINDOWS_SYSO)'; \
	version='$(VERSION)'; \
	rm -rf "$$res"; \
	mkdir -p "$$res" "$$out/bin"; \
	$(WAILS3) generate icons -input "$(APPICON)" -windowsfilename "$$res/icon.ico" -macfilename ""; \
	sed "s/@VERSION@/$${version%%-*}.0/" packaging/windows/agent-sessions.exe.manifest > "$$res/agent-sessions.exe.manifest"; \
	jq --arg v "$$version" '.info | {fixed: {file_version: $$v, product_version: $$v}, info: {"0000": {FileVersion: $$v, ProductVersion: $$v, CompanyName: .companyName, FileDescription: .productName, LegalCopyright: .copyright, ProductName: .productName, Comments: .comments}}}' \
		wails.json > "$$res/info.json"; \
	$(WAILS3) generate syso -arch amd64 -icon "$$res/icon.ico" -manifest "$$res/agent-sessions.exe.manifest" \
		-info "$$res/info.json" -out "$$res/agent-sessions_windows_amd64.syso"; \
	( set -C; cat "$$res/agent-sessions_windows_amd64.syso" > "$$syso" ) 2>/dev/null || { \
		echo "$$syso already exists: another Windows build is running, or a killed one left it. Remove it and retry." >&2; \
		exit 1; }; \
	trap 'rm -f "$$syso"' EXIT; \
	trap 'exit 130' HUP INT TERM; \
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -tags "$(GUI_TAGS)" -trimpath \
		-ldflags "$(LDFLAGS) -H windowsgui" -o "$$out/bin/agent-sessions.exe" ./cmd/agent-sessions; \
	echo "built $$out/bin/agent-sessions.exe"

# Windows amd64 .msi and _setup.exe in dist/, cross-compiled on Linux. Requires
# jq, python3, x86_64-w64-mingw32-gcc and wixl (apt install
# gcc-mingw-w64-x86-64 wixl, 0.102 or later for
# AllowSameVersionUpgrades). The app comes from windows-app, so the exe's
# version resources and the package agree on VERSION. The package is
# described in packaging/windows/agent-sessions.wxs. It is not code-signed,
# so SmartScreen warns on first run.
package-windows: check-app-icons
	@for tool in jq wixl python3; do \
		command -v $$tool >/dev/null 2>&1 || { echo "$$tool not found: install it (apt install $$tool)" >&2; exit 1; }; \
	done
	@command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 || \
		{ echo "x86_64-w64-mingw32-gcc not found: apt install gcc-mingw-w64-x86-64" >&2; exit 1; }
	$(MAKE) windows-app WINDOWS_BUILD_DIR=build
	$(MAKE) remote-servers
	$(MAKE) conpty
	mkdir -p dist
	x86_64-w64-mingw32-gcc -std=c11 -Wall -Wextra -Werror -O2 -shared \
		-o build/appcompat-ca.dll packaging/windows/appcompat-ca.c -lmsi -ladvapi32
	@set -e; \
	root=$$(pwd); \
	version='$(VERSION)'; \
	set -x; \
	wixl -a x64 -o dist/agent-sessions_$(VERSION)_windows_amd64.msi \
		-D Version="$${version%%-*}" -D BinDir="$$root/build/bin" -D RemoteDir="$$root/build/remote" \
		-D ConptyDir="$(abspath $(CONPTY_DIR))" \
		-D IconFile="$$root/build/windows/icon.ico" -D AppCompatDll="$$root/build/appcompat-ca.dll" \
		packaging/windows/agent-sessions.wxs
	$(MAKE) windows-setup MSI="dist/agent-sessions_$(VERSION)_windows_amd64.msi"

# Releases (see make help). A stable release is a version bump merged into
# main: .github/workflows/release.yml publishes it and creates tag v<VERSION>.
# Tags are pushed by hand only for pre-releases (x.y.z-suffix); the workflow
# refuses a tag that does not match wails.json.
TAG := v$(VERSION)
SEMVER_RE := ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$$

version:
	@echo "wails.json version: $(VERSION)"
	@echo "latest tag:         $$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo none)"
	@if git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null; then \
		echo "local tag:          $(TAG) at $$(git rev-list -n1 --abbrev-commit $(TAG)) (HEAD is $$(git rev-parse --short HEAD))"; \
	else echo "local tag:          $(TAG) not created"; fi
	@if git ls-remote --exit-code --tags origin "refs/tags/$(TAG)" >/dev/null 2>&1; then \
		echo "origin tag:         $(TAG) pushed"; \
	else echo "origin tag:         $(TAG) not pushed"; fi

tags:
	@git tag -l 'v*' --sort=-v:refname \
		--format='%(refname:short)%09%(if)%(*objectname)%(then)%(*objectname:short)%(else)%(objectname:short)%(end)%09%(creatordate:short)%09%(subject)'

set-version:
	@test -n "$(V)" || { echo 'usage: make set-version V=x.y.z' >&2; exit 1; }
	@echo "$(V)" | grep -Eq '$(SEMVER_RE)' || { echo "$(V) is not a semantic version" >&2; exit 1; }
	@! git rev-parse -q --verify "refs/tags/v$(V)" >/dev/null || { echo "v$(V) is already tagged" >&2; exit 1; }
	@git diff --quiet HEAD -- || { echo 'commit or stash your changes first' >&2; exit 1; }
	@if test -d web/src/lib; then \
		tmp=$$(mktemp web/src/lib/app-version.json.XXXXXX) && \
		jq -n --arg version '$(V)' '{version: $$version}' > "$$tmp" && \
		mv "$$tmp" web/src/lib/app-version.json || exit 1; \
		echo 'Updated web demo version; commit and deploy the web repository separately.'; \
	fi
	sed -i.bak 's/^\( *"productVersion": *"\)[^"]*"/\1$(V)"/' wails.json && rm -f wails.json.bak
	git commit -m "chore: release v$(V)" -- wails.json

tag:
	@echo "$(VERSION)" | grep -Eq '$(SEMVER_RE)' || { echo "wails.json version '$(VERSION)' is not a semantic version" >&2; exit 1; }
	@case "$(VERSION)" in *-*) ;; *) echo "$(VERSION) is a stable version: merge it into main and the release workflow tags it" >&2; exit 1 ;; esac
	@git diff --quiet HEAD -- || { echo 'commit or stash your changes before tagging' >&2; exit 1; }
	@! git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null || { echo "$(TAG) already exists; bump with make set-version V=..." >&2; exit 1; }
	git tag -a "$(TAG)" -m "Agent Sessions $(VERSION)"
	@echo "Created $(TAG) at $$(git rev-parse --short HEAD). Publish it with: make release"

untag:
	git tag -d "$(TAG)"
	@! git ls-remote --exit-code --tags origin "refs/tags/$(TAG)" >/dev/null 2>&1 || \
		echo "$(TAG) is still on origin; releases are not withdrawn automatically."

release:
	@git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null || $(MAKE) --no-print-directory tag
	git push origin "$(TAG)"

help:
	@printf 'Development\n'
	@printf '  %-22s %s\n' 'test' 'Run Go tests with the race detector'
	@printf '  %-22s %s\n' 'test-ssh' 'Run the SSH integration tests against a Docker sshd'
	@printf '  %-22s %s\n' 'lint' 'Run golangci-lint'
	@printf '  %-22s %s\n' 'fmt' 'Format Go code'
	@printf '  %-22s %s\n' 'cli / build' 'Build bin/agent-sessions-cli'
	@printf '  %-22s %s\n' 'remote-servers' 'Build gzipped headless servers (HOST=1: this machine only)'
	@printf '  %-22s %s\n' 'conpty' 'Fetch the ConPTY the Windows app ships'
	@printf '  %-22s %s\n' 'fuzz-smoke' 'Fuzz the Claude record parser for 60s'
	@printf '  %-22s %s\n' 'golden' 'Regenerate golden test files'
	@printf '  %-22s %s\n' 'dev' 'Run the desktop app against Vite with hot reload (DEV_PORT=5173)'
	@printf '  %-22s %s\n' 'app / gui-build' 'Build build/bin/agent-sessions'
	@printf '  %-22s %s\n' 'check-gui-linkage' 'Check the Linux app links GTK 4/WebKitGTK 6.0 only'
	@printf '  %-22s %s\n' 'windows-app' 'Cross-build the Windows exe (WINDOWS_BUILD_DIR=build)'
	@printf '  %-22s %s\n' 'clean' 'Remove build outputs'
	@printf '\nPackaging (writes dist/)\n'
	@printf '  %-22s %s\n' 'package-linux' '.deb and .rpm for ARCH (default: host)'
	@printf '  %-22s %s\n' 'package-macos' 'Universal .dmg (macOS only, needs jq)'
	@printf '  %-22s %s\n' 'package-windows' 'amd64 .msi + _setup.exe (needs wixl, jq, python3)'
	@printf '  %-22s %s\n' 'windows-setup MSI=...' 'Wrap an existing MSI (optional SETUP_OUT=...)'
	@printf '\nReleases (current: v$(VERSION))\n'
	@printf '  %-22s %s\n' 'version' 'Show the version and whether its tag exists'
	@printf '  %-22s %s\n' 'tags' 'List release tags, newest first'
	@printf '  %-22s %s\n' 'set-version V=x.y.z' 'Bump wails.json and commit it; merged into main, it releases'
	@printf '  %-22s %s\n' 'tag' 'Pre-releases only: create tag v<version> at HEAD (local)'
	@printf '  %-22s %s\n' 'untag' 'Delete the local tag'
	@printf '  %-22s %s\n' 'release' 'Pre-releases only: create the tag if needed and push it; CI publishes'

clean:
	rm -rf bin build/bin build/darwin build/dmg build/remote build/windows dist frontend/dist "$(WINDOWS_SYSO)"