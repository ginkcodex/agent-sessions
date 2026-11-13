package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMac stands in for hdiutil, codesign, plutil, spctl and ditto. Its
// image holds one app; the installed bundle is any path not under a mount.
type fakeMac struct {
	t *testing.T

	mu        sync.Mutex
	calls     []string
	mounted   map[string]bool
	detaches  int
	appName   string // the app inside the image
	extraApps []string

	runningTeam, newTeam string
	newID, newVersion    string
	badSignature         bool
	gatekeeperNo         bool
	attachFails          bool
	dittoFails           bool
	busyDetaches         int // detach fails this many times before it works
	forceNeeded          bool
}

func newFake(t *testing.T) *fakeMac {
	return &fakeMac{
		t: t, mounted: map[string]bool{}, appName: "Agent Sessions.app",
		runningTeam: "TEAM123456", newTeam: "TEAM123456", newID: MacBundleID, newVersion: "1.6.0",
	}
}

func (f *fakeMac) log(name string, args []string) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.mu.Unlock()
}

func (f *fakeMac) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// inMount reports whether path is in a mounted image.
func (f *fakeMac) inMount(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for mp := range f.mounted {
		if strings.HasPrefix(path, mp) {
			return true
		}
	}
	return false
}

func (f *fakeMac) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.log(name, args)
	last := ""
	if len(args) > 0 {
		last = args[len(args)-1]
	}
	switch name {
	case "hdiutil":
		switch args[0] {
		case "attach":
			if f.attachFails {
				return []byte("hdiutil: attach failed - no mountable file systems"), errors.New("exit 1")
			}
			var mp string
			for i, a := range args {
				if a == "-mountpoint" {
					mp = args[i+1]
				}
			}
			for _, app := range append([]string{f.appName}, f.extraApps...) {
				if err := os.MkdirAll(filepath.Join(mp, app, "Contents"), 0o755); err != nil {
					f.t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(mp, app, "Contents", "marker"), []byte("new"), 0o644); err != nil {
					f.t.Fatal(err)
				}
			}
			f.mu.Lock()
			f.mounted[mp] = true
			f.mu.Unlock()
			return nil, nil
		case "detach":
			f.mu.Lock()
			defer f.mu.Unlock()
			force := false
			for _, a := range args {
				force = force || a == "-force"
			}
			if f.busyDetaches > 0 && !force {
				f.busyDetaches--
				return []byte("Resource busy"), errors.New("exit 16")
			}
			if f.forceNeeded && !force {
				return []byte("Resource busy"), errors.New("exit 16")
			}
			f.detaches++
			delete(f.mounted, last)
			_ = os.RemoveAll(last) // a real mount point is empty again once detached
			os.MkdirAll(last, 0o755)
			return nil, nil
		}
	case "codesign":
		switch args[0] {
		case "--verify":
			if f.badSignature && f.inMountOrStaged(last) {
				return []byte("a sealed resource is missing or invalid"), errors.New("exit 1")
			}
			return nil, nil
		case "-d":
			if f.inMountOrStaged(last) {
				return []byte(fmt.Sprintf("Identifier=%s\nTeamIdentifier=%s\n", f.newID, orNotSet(f.newTeam))), nil
			}
			return []byte(fmt.Sprintf("Identifier=%s\nTeamIdentifier=%s\n", MacBundleID, orNotSet(f.runningTeam))), nil
		}
	case "plutil":
		plist := last
		switch args[1] {
		case "CFBundleIdentifier":
			return []byte(f.newID + "\n"), nil
		case "CFBundleShortVersionString":
			_ = plist
			return []byte(f.newVersion + "\n"), nil
		}
	case "spctl":
		if f.gatekeeperNo {
			return []byte("rejected"), errors.New("exit 3")
		}
		return nil, nil
	case "ditto":
		if f.dittoFails {
			return []byte("ditto: no space left on device"), errors.New("exit 1")
		}
		if err := os.CopyFS(args[1], os.DirFS(args[0])); err != nil {
			return []byte(err.Error()), err
		}
		return nil, nil
	}
	f.t.Fatalf("unexpected command %s %v", name, args)
	return nil, nil
}

// inMountOrStaged: the new app is anything that is not the installed bundle.
func (f *fakeMac) inMountOrStaged(path string) bool {
	return !strings.HasSuffix(path, "installed"+string(filepath.Separator)+"Agent Sessions.app")
}

func orNotSet(s string) string {
	if s == "" {
		return "not set"
	}
	return s
}

// macEnv is an installed bundle in a folder the user owns, and a downloaded
// "dmg" to stage.
type macEnv struct {
	Fake  *fakeMac
	Mac   *Mac
	Dir   string // holds the installed bundle
	Ready Ready
}

func newMacEnv(t *testing.T) *macEnv {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "installed")
	bundle := filepath.Join(dir, "Agent Sessions.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	dl := t.TempDir()
	if err := os.WriteFile(filepath.Join(dl, "agent-sessions_1.6.0_macos_universal.dmg"), []byte("dmg"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFake(t)
	return &macEnv{
		Fake:  f,
		Mac:   &Mac{Bundle: bundle, Run: f.Run, DetachDelay: time.Millisecond},
		Dir:   dir,
		Ready: Ready{Version: "1.6.0", File: "agent-sessions_1.6.0_macos_universal.dmg", Dir: dl},
	}
}

func (e *macEnv) siblings() []string {
	entries, _ := os.ReadDir(e.Dir)
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}
	return names
}

func (e *macEnv) wantUntouched(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.Mac.Bundle, "Contents", "marker"))
	if err != nil || string(data) != "old" {
		t.Fatalf("installed bundle changed: %q, %v", data, err)
	}
	if got := e.siblings(); len(got) != 1 {
		t.Fatalf("leftovers next to the bundle: %v", got)
	}
	if e.Fake.mountedCount() != 0 {
		t.Fatalf("image still mounted: %v", e.Fake.mounted)
	}
}

func (f *fakeMac) mountedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.mounted)
}

func TestMacBundleFindsTheAppAroundTheExecutable(t *testing.T) {
	sep := string(filepath.Separator)
	exe := filepath.Join(sep+"Applications", "Agent Sessions.app", "Contents", "MacOS", "agent-sessions")
	if got, want := MacBundle(exe), filepath.Join(sep+"Applications", "Agent Sessions.app"); got != want {
		t.Fatalf("MacBundle = %q, want %q", got, want)
	}
	if got := MacBundle(filepath.Join(sep+"usr", "local", "bin", "agent-sessions")); got != "" {
		t.Fatalf("MacBundle outside a bundle = %q", got)
	}
}

func TestUpdatesAreOffWhereTheAppCannotReplaceItself(t *testing.T) {
	for name, path := range map[string]string{
		"translocated":     "/private/var/folders/ab/xyz/AppTranslocation/1234-5678/d/Agent Sessions.app",
		"disk image":       "/Volumes/Agent Sessions/Agent Sessions.app",
		"another volume":   "/Volumes/External/Applications/Agent Sessions.app",
		"not a bundle":     "/usr/local/bin/agent-sessions",
		"development tree": "/home/me/src/build/bin/agent-sessions",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			m := &Mac{Bundle: path, Run: f.Run}
			if err := m.Eligible(context.Background()); err == nil {
				t.Fatal("Eligible = nil")
			}
			if len(f.calls) != 0 {
				t.Fatalf("ran %v for a path that is refused outright", f.calls)
			}
		})
	}
}

func TestEligibleNeedsAWritableFolderAndATeamSignature(t *testing.T) {
	e := newMacEnv(t)
	if err := e.Mac.Eligible(context.Background()); err != nil {
		t.Fatalf("a bundle in a writable folder: %v", err)
	}

	missing := &Mac{Bundle: filepath.Join(e.Dir, "gone", "Agent Sessions.app"), Run: e.Fake.Run}
	if err := missing.Eligible(context.Background()); err == nil || !strings.Contains(err.Error(), "cannot write") {
		t.Fatalf("an unwritable folder: %v", err)
	}

	e.Fake.runningTeam = ""
	if err := (&Mac{Bundle: e.Mac.Bundle, Run: e.Fake.Run}).Eligible(context.Background()); err == nil || !strings.Contains(err.Error(), "no Developer ID") {
		t.Fatalf("an ad hoc signed app: %v", err)
	}
}

func TestStageChecksAndCopiesTheNewApp(t *testing.T) {
	e := newMacEnv(t)
	staged, err := e.Mac.Stage(context.Background(), e.Ready)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(e.Dir, ".Agent Sessions.app.update-1.6.0"); staged != want {
		t.Fatalf("staged at %q, want %q", staged, want)
	}
	if data, _ := os.ReadFile(filepath.Join(staged, "Contents", "marker")); string(data) != "new" {
		t.Fatalf("staged marker = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(e.Mac.Bundle, "Contents", "marker")); string(data) != "old" {
		t.Fatal("the installed bundle changed")
	}
	if got := e.siblings(); len(got) != 2 {
		t.Fatalf("folder = %v, want the bundle and the staged copy only", got)
	}
	if e.Fake.mountedCount() != 0 || e.Fake.detaches != 1 {
		t.Fatalf("mounted %d, detached %d", e.Fake.mountedCount(), e.Fake.detaches)
	}
	for _, want := range []string{"hdiutil attach -readonly -nobrowse", "spctl --assess --type execute", "codesign --verify --deep --strict"} {
		if e.Fake.count(want) == 0 {
			t.Errorf("never ran %q; calls: %v", want, e.Fake.calls)
		}
	}
}

func TestStageRefusesWhatIsNotTheRunningAppsNextVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(f *fakeMac)
		want   string
	}{
		"a bad signature":    {func(f *fakeMac) { f.badSignature = true }, "does not verify"},
		"another team":       {func(f *fakeMac) { f.newTeam = "OTHER99999" }, "team"},
		"no team (ad hoc)":   {func(f *fakeMac) { f.newTeam = "" }, "team"},
		"another app":        {func(f *fakeMac) { f.newID = "com.example.other" }, "com.example.other"},
		"another version":    {func(f *fakeMac) { f.newVersion = "1.5.9" }, "1.5.9"},
		"Gatekeeper says no": {func(f *fakeMac) { f.gatekeeperNo = true }, "Gatekeeper"},
		"no space to copy":   {func(f *fakeMac) { f.dittoFails = true }, "no space"},
		"cannot mount":       {func(f *fakeMac) { f.attachFails = true }, "mount"},
		"two apps":           {func(f *fakeMac) { f.extraApps = []string{"Other.app"} }, "2 apps"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newMacEnv(t)
			tc.change(e.Fake)
			_, err := e.Mac.Stage(context.Background(), e.Ready)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Stage error = %v, want one naming %q", err, tc.want)
			}
			e.wantUntouched(t)
		})
	}
}

func TestStageDoesNothingToTheBundleWhenTheDeviceIsNotEligible(t *testing.T) {
	e := newMacEnv(t)
	e.Fake.runningTeam = ""
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err == nil {
		t.Fatal("staged for an app with no team")
	}
	if e.Fake.count("hdiutil") != 0 {
		t.Fatal("mounted an image for an ineligible install")
	}
	e.wantUntouched(t)
}

func TestStageReusesAValidCopyAndReplacesABrokenOne(t *testing.T) {
	e := newMacEnv(t)
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	if n := e.Fake.count("hdiutil attach"); n != 1 {
		t.Fatalf("mounted %d times; a valid staged copy needs no image", n)
	}

	// A copy that no longer verifies is replaced from the image.
	e.Fake.badSignature = true
	staged := e.Mac.StagedPath("1.6.0")
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err == nil {
		t.Fatal("a broken copy was accepted")
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("the broken copy was kept: %v", err)
	}
}

func TestStageReplacesAPartialCopyLeftByACrash(t *testing.T) {
	e := newMacEnv(t)
	part := e.Mac.StagedPath("1.6.0") + ".part"
	if err := os.MkdirAll(filepath.Join(part, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "Contents", "junk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged, err := e.Mac.Stage(context.Background(), e.Ready)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staged, "Contents", "junk")); !os.IsNotExist(err) {
		t.Fatal("the partial copy leaked into the staged app")
	}
	if got := e.siblings(); len(got) != 2 {
		t.Fatalf("folder = %v", got)
	}
}

func TestADetachThatIsBusyIsRetriedThenForced(t *testing.T) {
	e := newMacEnv(t)
	e.Fake.busyDetaches = 2
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	if e.Fake.count("hdiutil detach -quiet -force") != 0 || e.Fake.mountedCount() != 0 {
		t.Fatalf("calls: %v", e.Fake.calls)
	}

	e = newMacEnv(t)
	e.Fake.forceNeeded = true
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	if e.Fake.count("hdiutil detach -quiet -force") != 1 || e.Fake.mountedCount() != 0 {
		t.Fatalf("calls: %v", e.Fake.calls)
	}
}

func TestACancelledStageStillDetaches(t *testing.T) {
	e := newMacEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	run := e.Fake.Run
	e.Mac.Run = func(c context.Context, name string, args ...string) ([]byte, error) {
		out, err := run(c, name, args...)
		if name == "codesign" && args[0] == "--verify" {
			cancel() // the user quits mid-check
			return nil, ctx.Err()
		}
		return out, err
	}
	if _, err := e.Mac.Stage(ctx, e.Ready); err == nil {
		t.Fatal("Stage ignored the cancellation")
	}
	e.wantUntouched(t)
}

func TestCleanupKeepsOnlyTheWantedStagedVersion(t *testing.T) {
	e := newMacEnv(t)
	for _, v := range []string{"1.5.9", "1.6.0", "1.6.1"} {
		if err := os.MkdirAll(e.Mac.StagedPath(v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(e.Mac.StagedPath("1.6.1")+".part", 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(e.Dir, "Other.app")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	e.Mac.Cleanup("1.6.1")
	var left []string
	for _, n := range e.siblings() {
		left = append(left, n)
	}
	want := map[string]bool{"Agent Sessions.app": true, "Other.app": true, ".Agent Sessions.app.update-1.6.1": true}
	if len(left) != len(want) {
		t.Fatalf("left %v", left)
	}
	for _, n := range left {
		if !want[n] {
			t.Fatalf("left %v", left)
		}
	}
	e.Mac.Cleanup("")
	if got := e.siblings(); len(got) != 2 {
		t.Fatalf("after Cleanup(\"\"): %v", got)
	}
}
