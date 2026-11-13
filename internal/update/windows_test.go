package update

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
	"github.com/ginkcode/agent-sessions/internal/winmsi"
)

const (
	winUser     = "S-1-5-21-1-2-3-1001"
	winCodeOld  = "{AAAAAAAA-0000-0000-0000-000000000001}"
	winUpgrade  = "{D7786BE1-BF16-4887-9C3B-345C5D87CD95}"
	winOldFile  = "agent-sessions_1.5.5_windows_amd64.msi"
	winNewFile  = "agent-sessions_1.6.0_windows_amd64.msi"
	winOldBytes = "old msi"
	winNewBytes = "new msi"
)

type winEnv struct {
	W        *Windows
	Products []winmsi.Product
	// Infos maps an MSI file name to what it says about itself.
	Infos    map[string]winmsi.Info
	Feeds    []string
	FeedErr  error
	New      Ready
	rollback *installerServer
}

func msiInfo(version string) winmsi.Info {
	return winmsi.Info{
		ProductCode: "{BBBBBBBB-0000-0000-0000-000000000002}", UpgradeCode: winUpgrade,
		ProductName: winmsi.ProductName, ProductVersion: version, PerUser: "1",
	}
}

func newWinEnv(t *testing.T) *winEnv {
	t.Helper()
	local := t.TempDir()
	e := &winEnv{
		Products: []winmsi.Product{{Code: winCodeOld, Version: "1.5.5", SID: winUser, Context: winmsi.UserUnmanaged}},
		Infos:    map[string]winmsi.Info{winNewFile: msiInfo("1.6.0"), winOldFile: msiInfo("1.5.5")},
		rollback: serveInstaller(t, winOldBytes, nil),
	}
	store := &Store{Dir: filepath.Join(t.TempDir(), "updates"), HTTP: http.DefaultClient}
	e.W = &Windows{
		LocalAppData: local, SID: winUser, Version: "1.5.5",
		Products: func() ([]winmsi.Product, error) { return e.Products, nil },
		ReadMSI: func(path string) (winmsi.Info, error) {
			info, ok := e.Infos[filepath.Base(path)]
			if !ok {
				return winmsi.Info{}, errors.New("not an MSI")
			}
			return info, nil
		},
		Store: store,
		Feed: func(_ context.Context, version string) (Release, error) {
			e.Feeds = append(e.Feeds, version)
			if e.FeedErr != nil {
				return Release{}, e.FeedErr
			}
			rel := releaseFor(e.rollback.URL, version, winOldBytes)
			rel.File = winOldFile
			rel.URL = e.rollback.URL + "/" + winOldFile
			return rel, nil
		},
	}
	e.W.Exe = e.W.InstallPath()

	// The new MSI, downloaded and checked.
	newSrv := serveInstaller(t, winNewBytes, nil)
	rel := releaseFor(newSrv.URL, "1.6.0", winNewBytes)
	rel.File = winNewFile
	rel.URL = newSrv.URL + "/" + winNewFile
	r, err := store.Fetch(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	e.New = r
	return e
}

func TestWindowsInstallThatCanUpdateItself(t *testing.T) {
	e := newWinEnv(t)
	if err := e.W.Eligible(); err != nil {
		t.Fatal(err)
	}
	// The exe path is compared without regard to case or a trailing dot.
	e.W.Exe = strings.ToUpper(e.W.InstallPath())
	if err := e.W.Eligible(); err != nil {
		t.Fatalf("path in another case: %v", err)
	}
	// A release candidate has the MSI version of its release.
	e.W.Version = "1.5.5-rc1"
	if err := e.W.Eligible(); err != nil {
		t.Fatalf("pre-release version: %v", err)
	}
	if p, err := e.W.Product(); err != nil || p.Code != winCodeOld {
		t.Fatalf("Product = %+v, %v", p, err)
	}
}

func TestWindowsInstallsThatMustNotUpdateThemselves(t *testing.T) {
	for name, change := range map[string]func(e *winEnv){
		"a test build folder": func(e *winEnv) { e.W.Exe = `C:\Users\Gink\agent-sessions-launch5\agent-sessions.exe` },
		"a portable copy":     func(e *winEnv) { e.W.Exe = filepath.Join(e.W.LocalAppData, "Downloads", "agent-sessions.exe") },
		"another exe in the folder": func(e *winEnv) {
			e.W.Exe = filepath.Join(filepath.Dir(e.W.InstallPath()), "agent-sessions-cli.exe")
		},
		"nothing registered":   func(e *winEnv) { e.Products = nil },
		"two registrations":    func(e *winEnv) { e.Products = append(e.Products, e.Products[0]) },
		"machine-wide install": func(e *winEnv) { e.Products[0].Context = 4 },
		"another user's":       func(e *winEnv) { e.Products[0].SID = "S-1-5-21-9-9-9-1" },
		"managed install":      func(e *winEnv) { e.Products[0].Context = 1 },
		"another version":      func(e *winEnv) { e.Products[0].Version = "1.4.0" },
		"bad product code":     func(e *winEnv) { e.Products[0].Code = "x" },
		"no app data folder":   func(e *winEnv) { e.W.LocalAppData = "" },
	} {
		t.Run(name, func(t *testing.T) {
			e := newWinEnv(t)
			change(e)
			if err := e.W.Eligible(); err == nil {
				t.Fatal("Eligible = nil")
			}
			if err := e.W.Prepare(context.Background(), e.New); err == nil {
				t.Fatal("Prepare = nil")
			}
			if len(e.Feeds) != 0 {
				t.Fatal("downloaded for an install that cannot update")
			}
		})
	}
	e := newWinEnv(t)
	e.W.Products = func() ([]winmsi.Product, error) { return nil, errors.New("registry unreadable") }
	if err := e.W.Eligible(); err == nil {
		t.Fatal("an unreadable registry was eligible")
	}
}

func TestWindowsPrepareKeepsTheNewAndTheRollbackInstaller(t *testing.T) {
	e := newWinEnv(t)
	if err := e.W.Prepare(context.Background(), e.New); err != nil {
		t.Fatal(err)
	}
	rb, ok := e.W.Rollback(e.New)
	if !ok || rb.Version != "1.5.5" || rb.File != winOldFile || rb.Verify() != nil {
		t.Fatalf("Rollback = %+v, %v", rb, ok)
	}
	if got, want := rb.Dir, e.W.Store.RollbackDir("1.6.0"); got != want {
		t.Fatalf("rollback at %s, want %s", got, want)
	}
	if len(e.Feeds) != 1 || e.Feeds[0] != "1.5.5" {
		t.Fatalf("looked up the signed releases of %v", e.Feeds)
	}

	// A later run, offline, uses what is kept.
	e.FeedErr = errors.New("offline")
	if err := e.W.Prepare(context.Background(), e.New); err != nil {
		t.Fatalf("Prepare again, offline: %v", err)
	}
	// Removing the new version removes its rollback with it.
	if err := e.W.Store.Reconcile(""); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.W.Rollback(e.New); ok {
		t.Fatal("the rollback outlived the version it belongs to")
	}
}

func TestWindowsPrepareRefusesPackagesThatAreNotOurs(t *testing.T) {
	for name, change := range map[string]func(e *winEnv){
		"the new MSI is another product": func(e *winEnv) {
			i := e.Infos[winNewFile]
			i.UpgradeCode = "{00000000-0000-0000-0000-000000000000}"
			e.Infos[winNewFile] = i
		},
		"the new MSI has another name": func(e *winEnv) {
			i := e.Infos[winNewFile]
			i.ProductName = "Other"
			e.Infos[winNewFile] = i
		},
		"the new MSI is another version": func(e *winEnv) { e.Infos[winNewFile] = msiInfo("1.6.1") },
		"the new MSI is machine-wide": func(e *winEnv) {
			i := e.Infos[winNewFile]
			i.AllUsers = "1"
			e.Infos[winNewFile] = i
		},
		"the new MSI is unreadable":      func(e *winEnv) { delete(e.Infos, winNewFile) },
		"the rollback MSI is unreadable": func(e *winEnv) { delete(e.Infos, winOldFile) },
		"the rollback MSI is another version": func(e *winEnv) {
			e.Infos[winOldFile] = msiInfo("1.4.0")
		},
		"the rollback MSI is another product": func(e *winEnv) {
			i := e.Infos[winOldFile]
			i.UpgradeCode = "{00000000-0000-0000-0000-000000000000}"
			e.Infos[winOldFile] = i
		},
		"the installed release has no signed feed": func(e *winEnv) { e.FeedErr = ErrUnsigned },
		"the installed release's signature is bad": func(e *winEnv) { e.FeedErr = ErrBadSignature },
	} {
		t.Run(name, func(t *testing.T) {
			e := newWinEnv(t)
			change(e)
			if err := e.W.Prepare(context.Background(), e.New); err == nil {
				t.Fatal("Prepare = nil")
			}
		})
	}

	// Never installs an older version over a newer one.
	e := newWinEnv(t)
	e.W.Version = "1.7.0"
	e.Products[0].Version = "1.7.0"
	if err := e.W.Prepare(context.Background(), e.New); err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("Prepare of an older version = %v", err)
	}
}

func TestWindowsRollbackDownloadChecksTheSignedHash(t *testing.T) {
	e := newWinEnv(t)
	e.rollback = serveInstaller(t, "tampered", nil)
	if err := e.W.Prepare(context.Background(), e.New); err == nil {
		t.Fatal("a rollback installer that does not match the signed hash was kept")
	}
	if _, ok := e.W.Rollback(e.New); ok {
		t.Fatal("a rollback installer that failed its check is marked ready")
	}
	// Its failure is not counted against the new version.
	if e.W.Store.Failures("1.6.0") != 0 {
		t.Fatal("the rollback download counted as a failure of the new version")
	}
	if _, err := os.Stat(e.W.Store.versionDir("1.6.0")); err != nil {
		t.Fatalf("the new version was removed: %v", err)
	}
}

func TestFeedForReadsAnEarlierReleaseThroughItsSignature(t *testing.T) {
	pub, priv := newKey(t)
	body15 := []byte(sumsFor("1.5.5"))
	body16 := []byte(sumsFor("1.6.0"))
	sums := map[string][]byte{"1.5.5": body15, "1.6.0": body16}
	sigs := map[string][]byte{"1.5.5": minisign.Sign(priv, body15), "1.6.0": minisign.Sign(priv, body16)}
	fs := newFeed(t, "1.6.0", sums, sigs, "windows", "amd64", pub)

	rel, err := fs.Checker.FeedFor(context.Background(), "1.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "1.5.5" || rel.File != winOldFile || rel.SHA256 != hexSum(winOldFile) || rel.URL != fs.URL+"/download/v1.5.5/"+winOldFile {
		t.Fatalf("FeedFor = %+v", rel)
	}
	if rel, err = fs.Checker.FeedFor(context.Background(), "v1.5.5"); err != nil || rel.Version != "1.5.5" {
		t.Fatalf("FeedFor with a tag = %+v, %v", rel, err)
	}

	// Refused: a signature that does not cover these sums, an unsigned
	// release, a version not in its own sums, and names that are not versions.
	_, otherPriv := newKey(t)
	bad := newFeed(t, "1.6.0", sums, map[string][]byte{"1.5.5": minisign.Sign(otherPriv, body15)}, "windows", "amd64", pub)
	if _, err := bad.Checker.FeedFor(context.Background(), "1.5.5"); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := fs.Checker.FeedFor(context.Background(), "1.4.0"); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("unknown release: %v", err)
	}
	mismatch := newFeed(t, "1.6.0", map[string][]byte{"1.5.5": body16}, map[string][]byte{"1.5.5": minisign.Sign(priv, body16)}, "windows", "amd64", pub)
	if _, err := mismatch.Checker.FeedFor(context.Background(), "1.5.5"); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("sums of another version: %v", err)
	}
	for _, v := range []string{"", "../1.5.5", "1.5.5/../x", "latest"} {
		if _, err := fs.Checker.FeedFor(context.Background(), v); !errors.Is(err, ErrUnsigned) {
			t.Errorf("FeedFor(%q) = %v", v, err)
		}
	}
}
