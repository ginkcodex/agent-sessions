//go:build darwin

package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests use the real hdiutil, codesign, plutil and ditto on small apps
// signed ad hoc. A Developer ID certificate is not available to tests, so
// teamRunner reports the Team IDs a release build would have.

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>agent-sessions</string>
<key>CFBundleIdentifier</key><string>%s</string>
<key>CFBundleName</key><string>Agent Sessions</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>%s</string>
<key>CFBundleVersion</key><string>%s</string>
</dict></plist>
`

func runTool(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

// makeApp writes a signed bundle with the given identifier and version.
func makeApp(t *testing.T, dir, id, version string) string {
	t.Helper()
	app := filepath.Join(dir, "Agent Sessions.app")
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(fmt.Sprintf(infoPlist, id, version, version)), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Skipf("no /usr/bin/true to build a test app from: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macos, "agent-sessions"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	runTool(t, "codesign", "--force", "--deep", "--sign", "-", app)
	return app
}

// makeDMG puts app and an Applications link, as the release image has, in a
// compressed image.
func makeDMG(t *testing.T, app string) string {
	t.Helper()
	src := t.TempDir()
	runTool(t, "ditto", app, filepath.Join(src, filepath.Base(app)))
	if err := os.Symlink("/Applications", filepath.Join(src, "Applications")); err != nil {
		t.Fatal(err)
	}
	dmg := filepath.Join(t.TempDir(), "agent-sessions_1.6.0_macos_universal.dmg")
	if out, err := exec.Command("hdiutil", "create", "-quiet", "-srcfolder", src, "-volname", "Agent Sessions", "-format", "UDZO", dmg).CombinedOutput(); err != nil {
		t.Skipf("cannot build a disk image here: %v: %s", err, out)
	}
	return dmg
}

// teamRunner runs the real tools but reports Team IDs: running for the
// installed bundle, and next for every other app. Gatekeeper's verdict is
// replaced too, since an ad hoc app is never accepted.
func teamRunner(installed, running, next string) Runner {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "spctl" {
			return nil, nil
		}
		out, err := execRunner(ctx, name, args...)
		if name == "codesign" && len(args) > 0 && args[0] == "-d" {
			team := next
			if args[len(args)-1] == installed {
				team = running
			}
			out = bytes.ReplaceAll(out, []byte("TeamIdentifier=not set"), []byte("TeamIdentifier="+team))
		}
		return out, err
	}
}

type realEnv struct {
	Mac   *Mac
	Dir   string
	Ready Ready
}

func newRealEnv(t *testing.T, newID, newVersion string, run func(installed string) Runner) *realEnv {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := makeApp(t, dir, MacBundleID, "1.5.5")
	dmg := makeDMG(t, makeApp(t, t.TempDir(), newID, newVersion))
	m := &Mac{Bundle: installed}
	if run != nil {
		m.Run = run(installed)
	}
	return &realEnv{Mac: m, Dir: dir, Ready: Ready{Version: "1.6.0", File: filepath.Base(dmg), Dir: filepath.Dir(dmg)}}
}

func mountsOf(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("mount").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func noImageMounted(t *testing.T) {
	t.Helper()
	if strings.Contains(mountsOf(t), "agent-sessions-update-") {
		t.Fatalf("an update image is still mounted:\n%s", mountsOf(t))
	}
}

func TestRealImageStagesAndTheCopyKeepsItsSignature(t *testing.T) {
	e := newRealEnv(t, MacBundleID, "1.6.0", func(installed string) Runner { return teamRunner(installed, "TEAM123456", "TEAM123456") })
	staged, err := e.Mac.Stage(context.Background(), e.Ready)
	if err != nil {
		t.Fatal(err)
	}
	noImageMounted(t)
	// The copy is a whole signed bundle beside the installed one.
	runTool(t, "codesign", "--verify", "--deep", "--strict", staged)
	if v, err := e.Mac.plistValue(context.Background(), staged, "CFBundleShortVersionString"); err != nil || v != "1.6.0" {
		t.Fatalf("staged version = %q, %v", v, err)
	}
	if v, _ := e.Mac.plistValue(context.Background(), e.Mac.Bundle, "CFBundleShortVersionString"); v != "1.5.5" {
		t.Fatalf("installed version = %q: the installed bundle changed", v)
	}
	// The Applications link in the image was not taken for an app.
	if _, err := os.Lstat(filepath.Join(e.Dir, "Applications")); err == nil {
		t.Fatal("copied the Applications link")
	}

	// A second stage reuses the copy.
	if again, err := e.Mac.Stage(context.Background(), e.Ready); err != nil || again != staged {
		t.Fatalf("second Stage = %q, %v", again, err)
	}

	// Tampering with the staged copy is noticed and the copy is made again.
	exe := filepath.Join(staged, "Contents", "MacOS", "agent-sessions")
	f, err := os.OpenFile(exe, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("tampered")
	f.Close()
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	runTool(t, "codesign", "--verify", "--deep", "--strict", staged)
	noImageMounted(t)
}

func TestRealImageSignedAdHocIsRefusedForItsTeam(t *testing.T) {
	// No spoofing: the running app and the new one are both ad hoc signed, as
	// a locally built app or a re-signed one would be.
	e := newRealEnv(t, MacBundleID, "1.6.0", nil)
	_, err := e.Mac.Stage(context.Background(), e.Ready)
	if err == nil || !strings.Contains(err.Error(), "Developer ID") {
		t.Fatalf("Stage = %v, want a refusal for the missing team", err)
	}
	noImageMounted(t)
	if _, err := os.Stat(e.Mac.StagedPath("1.6.0")); !os.IsNotExist(err) {
		t.Fatalf("something was staged: %v", err)
	}
}

func TestRealImageFromAnotherTeamIsRefused(t *testing.T) {
	e := newRealEnv(t, MacBundleID, "1.6.0", func(installed string) Runner { return teamRunner(installed, "TEAM123456", "OTHER99999") })
	_, err := e.Mac.Stage(context.Background(), e.Ready)
	if err == nil || !strings.Contains(err.Error(), "OTHER99999") {
		t.Fatalf("Stage = %v, want a refusal naming the other team", err)
	}
	noImageMounted(t)
	if got, _ := os.ReadDir(e.Dir); len(got) != 1 {
		t.Fatalf("folder = %v", got)
	}
}

func TestRealImageOfAnotherAppOrVersionIsRefused(t *testing.T) {
	team := func(installed string) Runner { return teamRunner(installed, "TEAM123456", "TEAM123456") }
	for name, tc := range map[string]struct{ id, version, want string }{
		"another app":     {"com.example.other", "1.6.0", "com.example.other"},
		"another version": {"com.wails.agent-sessions", "1.5.9", "1.5.9"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newRealEnv(t, tc.id, tc.version, team)
			_, err := e.Mac.Stage(context.Background(), e.Ready)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Stage = %v, want %q", err, tc.want)
			}
			noImageMounted(t)
		})
	}
}

// A release .dmg, set by the person running the tests, is staged for real
// against a copy of its own app: Developer ID, notarization ticket and all.
//
//	AGENT_SESSIONS_TEST_DMG=/path/agent-sessions_1.5.5_macos_universal.dmg \
//	AGENT_SESSIONS_TEST_DMG_VERSION=1.5.5 go test ./internal/update -run RealRelease
func TestRealReleaseImageStages(t *testing.T) {
	dmg, version := os.Getenv("AGENT_SESSIONS_TEST_DMG"), os.Getenv("AGENT_SESSIONS_TEST_DMG_VERSION")
	if dmg == "" || version == "" {
		t.Skip("set AGENT_SESSIONS_TEST_DMG and AGENT_SESSIONS_TEST_DMG_VERSION to try a release image")
	}
	// The "installed" app is a copy taken from the image itself.
	mp := t.TempDir()
	runTool(t, "hdiutil", "attach", "-readonly", "-nobrowse", "-noautoopen", "-mountpoint", mp, dmg)
	dir := filepath.Join(t.TempDir(), "Applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dir, "Agent Sessions.app")
	runTool(t, "ditto", filepath.Join(mp, "Agent Sessions.app"), installed)
	runTool(t, "hdiutil", "detach", "-quiet", mp)

	m := &Mac{Bundle: installed}
	if err := m.Eligible(context.Background()); err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	staged, err := m.Stage(context.Background(), Ready{Version: version, File: filepath.Base(dmg), Dir: filepath.Dir(dmg)})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("staged %s", staged)
	runTool(t, "codesign", "--verify", "--deep", "--strict", staged)
	noImageMounted(t)
}
