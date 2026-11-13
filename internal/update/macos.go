package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// MacBundleID is the bundle identifier of Agent Sessions. Changing it resets
// the permissions users granted, so it stays the same in every release.
const MacBundleID = "com.wails.agent-sessions"

// Runner runs a command and returns its combined output. The macOS checks go
// through it, so tests can stand in for hdiutil, codesign and the others.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// MacBundle returns the .app bundle that contains the executable exe, or ""
// when exe is not inside one.
func MacBundle(exe string) string {
	for dir := filepath.Dir(exe); ; dir = filepath.Dir(dir) {
		if strings.HasSuffix(dir, ".app") {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			return ""
		}
	}
}

// Mac stages a new version of the installed app bundle: it checks the .app
// inside a downloaded .dmg and copies it next to the installed bundle, ready
// to be swapped in when the app quits.
type Mac struct {
	// Bundle is the installed app, such as /Applications/Agent Sessions.app.
	Bundle string
	// Run runs the system tools; nil runs them for real.
	Run Runner
	// DetachRetries and DetachDelay bound how long a busy image is given to
	// let go before it is detached by force. Zero means 3 tries, 1 s apart.
	DetachRetries int
	DetachDelay   time.Duration

	team string // the installed bundle's Team ID, once read
}

func (m *Mac) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.Run != nil {
		return m.Run(ctx, name, args...)
	}
	return execRunner(ctx, name, args...)
}

// StagedPath is where version waits: next to the installed bundle, so
// swapping it in is a rename on one volume.
func (m *Mac) StagedPath(version string) string {
	return filepath.Join(filepath.Dir(m.Bundle), "."+filepath.Base(m.Bundle)+".update-"+version)
}

// pathReason says why a bundle at path cannot replace itself, judging by the
// path alone. App Translocation runs a quarantined app from a read-only copy
// under a random path, and an app opened from a disk image runs from the
// image; neither is where the user installed it.
func pathReason(path string) error {
	p := filepath.ToSlash(path)
	switch {
	case !strings.HasSuffix(p, ".app"):
		return fmt.Errorf("%s is not an app bundle", path)
	case strings.Contains(p, "/AppTranslocation/"):
		return fmt.Errorf("%s runs from App Translocation: move the app to Applications and open it again", path)
	case strings.HasPrefix(p, "/Volumes/"):
		return fmt.Errorf("%s runs from a disk image or another volume", path)
	}
	return nil
}

// Eligible reports why this install cannot update itself, or nil. It is true
// when the bundle is where the user put it, its folder and the bundle belong
// to the user, and the bundle is signed by a team (the new version must be
// signed by the same one).
func (m *Mac) Eligible(ctx context.Context) error {
	if err := pathReason(m.Bundle); err != nil {
		return err
	}
	parent := filepath.Dir(m.Bundle)
	probe, err := os.CreateTemp(parent, ".agent-sessions-write-test-")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", parent, err)
	}
	probe.Close()
	_ = os.Remove(probe.Name())
	if !ownedByUser(m.Bundle) {
		return fmt.Errorf("%s belongs to another user", m.Bundle)
	}
	id, team, err := m.signature(ctx, m.Bundle)
	if err != nil {
		return err
	}
	if id != MacBundleID {
		return fmt.Errorf("the running app is signed as %q, not %s", id, MacBundleID)
	}
	if team == "" {
		return errors.New("the running app has no Developer ID signature, so a new version cannot be matched to it")
	}
	m.team = team
	return nil
}

// signature reads the identifier and Team ID an app is signed with. The Team
// ID is empty for an ad hoc signature.
func (m *Mac) signature(ctx context.Context, app string) (identifier, team string, err error) {
	out, err := m.run(ctx, "codesign", "-d", "--verbose=2", app)
	if err != nil {
		return "", "", fmt.Errorf("read the signature of %s: %w: %s", app, err, trim(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Identifier="):
			identifier = strings.TrimPrefix(line, "Identifier=")
		case strings.HasPrefix(line, "TeamIdentifier="):
			team = strings.TrimPrefix(line, "TeamIdentifier=")
			if team == "not set" {
				team = ""
			}
		}
	}
	return identifier, team, nil
}

func (m *Mac) plistValue(ctx context.Context, app, key string) (string, error) {
	out, err := m.run(ctx, "plutil", "-extract", key, "raw", "-o", "-", filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return "", fmt.Errorf("read %s of %s: %w: %s", key, app, err, trim(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func trim(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

// verify runs the checks on an app bundle: its signature is intact, it is
// Agent Sessions, from the running app's team, at the expected version.
// gatekeeper adds the system's verdict (Developer ID and notarization).
func (m *Mac) verify(ctx context.Context, app, version string, gatekeeper bool) error {
	if out, err := m.run(ctx, "codesign", "--verify", "--deep", "--strict", app); err != nil {
		return fmt.Errorf("signature of the new app does not verify: %w: %s", err, trim(out))
	}
	id, team, err := m.signature(ctx, app)
	if err != nil {
		return err
	}
	if id != MacBundleID {
		return fmt.Errorf("the new app is signed as %q, not %s", id, MacBundleID)
	}
	if team == "" || team != m.team {
		return fmt.Errorf("the new app is signed by team %q, not the running app's %q", team, m.team)
	}
	for key, want := range map[string]string{"CFBundleIdentifier": MacBundleID, "CFBundleShortVersionString": version} {
		got, err := m.plistValue(ctx, app, key)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("the new app's %s is %q, expected %q", key, got, want)
		}
	}
	if gatekeeper {
		if out, err := m.run(ctx, "spctl", "--assess", "--type", "execute", app); err != nil {
			return fmt.Errorf("Gatekeeper rejects the new app: %w: %s", err, trim(out))
		}
	}
	return nil
}

// Stage makes r's app ready to install and returns where it waits. It mounts
// the .dmg read-only and out of sight, checks the .app inside, copies it
// beside the installed bundle and checks the copy. A version already staged
// and still valid is reused. Nothing is changed in the installed bundle.
func (m *Mac) Stage(ctx context.Context, r Ready) (string, error) {
	if m.team == "" {
		if err := m.Eligible(ctx); err != nil {
			return "", err
		}
	}
	staged := m.StagedPath(r.Version)
	if _, err := os.Stat(staged); err == nil {
		if m.verify(ctx, staged, r.Version, true) == nil {
			return staged, nil
		}
		if err := os.RemoveAll(staged); err != nil {
			return "", err
		}
	}

	src, release, err := m.mount(ctx, r.Path())
	if err != nil {
		return "", err
	}
	defer release()
	if err := m.verify(ctx, src, r.Version, false); err != nil {
		return "", err
	}

	part := staged + ".part"
	if err := os.RemoveAll(part); err != nil {
		return "", err
	}
	if out, err := m.run(ctx, "ditto", src, part); err != nil {
		_ = os.RemoveAll(part)
		return "", fmt.Errorf("copy the new app: %w: %s", err, trim(out))
	}
	if err := m.verify(ctx, part, r.Version, true); err != nil {
		_ = os.RemoveAll(part)
		return "", err
	}
	if err := os.Rename(part, staged); err != nil {
		_ = os.RemoveAll(part)
		return "", err
	}
	return staged, nil
}

// mount attaches the image and returns the single .app at its root, and a
// function that detaches the image again.
func (m *Mac) mount(ctx context.Context, dmg string) (app string, release func(), err error) {
	mp, err := os.MkdirTemp("", "agent-sessions-update-")
	if err != nil {
		return "", nil, err
	}
	out, err := m.run(ctx, "hdiutil", "attach", "-readonly", "-nobrowse", "-noautoopen", "-noverify", "-mountpoint", mp, dmg)
	if err != nil {
		_ = os.Remove(mp)
		return "", nil, fmt.Errorf("mount %s: %w: %s", filepath.Base(dmg), err, trim(out))
	}
	release = func() { m.detach(mp) }

	entries, err := os.ReadDir(mp)
	if err != nil {
		release()
		return "", nil, err
	}
	var apps []string
	for _, e := range entries {
		// A symlink such as Applications is not an app, whatever it points to.
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			apps = append(apps, filepath.Join(mp, e.Name()))
		}
	}
	if len(apps) != 1 {
		release()
		return "", nil, fmt.Errorf("%s holds %d apps, expected one", filepath.Base(dmg), len(apps))
	}
	return apps[0], release, nil
}

// detach unmounts the image, waiting for processes that still have files open
// in it (Spotlight, say) and then forcing it. It always removes the mount
// point folder if the image is gone. A context that has ended must not leave
// the image mounted, so detaching does not use it.
func (m *Mac) detach(mp string) {
	retries, delay := m.DetachRetries, m.DetachDelay
	if retries <= 0 {
		retries = 3
	}
	if delay <= 0 {
		delay = time.Second
	}
	bg := context.Background()
	detached := false
	for i := 0; i < retries && !detached; i++ {
		if i > 0 {
			time.Sleep(delay)
		}
		_, err := m.run(bg, "hdiutil", "detach", "-quiet", mp)
		detached = err == nil
	}
	if !detached {
		if out, err := m.run(bg, "hdiutil", "detach", "-quiet", "-force", mp); err != nil {
			log.Printf("agent-sessions: detach %s: %v: %s", mp, err, trim(out))
			return // still mounted: removing the folder would not help
		}
	}
	_ = os.Remove(mp)
}

// Cleanup removes every staged copy and leftover partial copy beside the
// installed bundle except the one for keep ("" removes them all).
func (m *Mac) Cleanup(keep string) {
	prefix := "." + filepath.Base(m.Bundle) + ".update-"
	entries, _ := os.ReadDir(filepath.Dir(m.Bundle))
	keepName := ""
	if keep != "" {
		keepName = filepath.Base(m.StagedPath(keep))
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || name == keepName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(filepath.Dir(m.Bundle), name)); err != nil {
			log.Printf("agent-sessions: remove %s: %v", name, err)
		}
	}
}
