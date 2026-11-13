package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/filelock"
	"golang.org/x/mod/semver"
)

// ErrGaveUp means downloading this version failed MaxFailures times. The app
// shows the release page instead of trying again.
var ErrGaveUp = errors.New("update download failed too many times")

// ErrBusy means another process holds the updates folder.
var ErrBusy = errors.New("another process is working in the updates folder")

const (
	// DefaultMaxFailures is how many failed downloads of one version are
	// tolerated before the app falls back to the release page.
	DefaultMaxFailures = 3
	// DefaultIdleTimeout is how long a download may go without receiving a
	// byte. The whole download has no limit, since installers are large and
	// connections slow.
	DefaultIdleTimeout = 60 * time.Second
	// maxInstallerBytes bounds one download. Installers are around 100 MB.
	maxInstallerBytes = 1 << 30

	lockName     = "updates.lock"
	readyName    = "ready.json"
	failuresName = "failures.json"
)

// Ready is a downloaded installer whose size and SHA256 were checked against
// the signed SHA256SUMS.
type Ready struct {
	Version string `json:"version"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	// Dir is the folder holding File and the ready marker.
	Dir string `json:"-"`
}

// Path is the installer file.
func (r Ready) Path() string { return filepath.Join(r.Dir, r.File) }

// Verify hashes the installer again. Fetch has already done it once; the
// code that installs it does so a second time, since the file sat in a
// user-writable folder in between.
func (r Ready) Verify() error {
	f, err := os.Open(r.Path())
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if n != r.Size || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return fmt.Errorf("%s no longer matches its checksum", r.File)
	}
	return nil
}

// Store is the folder where updates are downloaded:
// <Dir>/<version>/<installer> plus ready.json once the installer is checked.
// All processes of one user share it; the lock Dir/updates.lock allows one to
// download or clean up at a time.
type Store struct {
	Dir string
	// HTTP downloads installers. It must not set an overall Timeout.
	HTTP *http.Client
	// MaxFailures and IdleTimeout default to the constants above when zero.
	MaxFailures int
	IdleTimeout time.Duration

	mu sync.Mutex // one download or cleanup at a time within the process
}

// DefaultDir is <user cache>/agent-sessions/updates.
func DefaultDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "agent-sessions", "updates"), nil
}

// NewStore returns a Store in dir that honours the proxy environment
// variables.
func NewStore(dir string) *Store {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	return &Store{Dir: dir, HTTP: &http.Client{Transport: transport}}
}

func (s *Store) maxFailures() int {
	if s.MaxFailures > 0 {
		return s.MaxFailures
	}
	return DefaultMaxFailures
}

func (s *Store) idleTimeout() time.Duration {
	if s.IdleTimeout > 0 {
		return s.IdleTimeout
	}
	return DefaultIdleTimeout
}

// safeName reports whether v can be a single path element.
func safeName(v string) bool {
	return v != "" && v != "." && v != ".." && !strings.ContainsAny(v, `/\:`) && filepath.Base(v) == v
}

func (s *Store) versionDir(version string) string { return filepath.Join(s.Dir, version) }

// Ready returns the checked installer for version, if there is one. It
// compares sizes only; Ready.Verify hashes.
func (s *Store) Ready(version string) (Ready, bool) {
	if !safeName(version) {
		return Ready{}, false
	}
	dir := s.versionDir(version)
	data, err := os.ReadFile(filepath.Join(dir, readyName))
	if err != nil {
		return Ready{}, false
	}
	var r Ready
	if json.Unmarshal(data, &r) != nil || r.Version != version || !safeName(r.File) || !hexSHA256.MatchString(r.SHA256) {
		return Ready{}, false
	}
	r.Dir = dir
	fi, err := os.Stat(r.Path())
	if err != nil || !fi.Mode().IsRegular() || fi.Size() != r.Size {
		return Ready{}, false
	}
	return r, true
}

// Newest returns the newest ready installer in the folder.
func (s *Store) Newest() (Ready, bool) {
	var best Ready
	found := false
	for _, v := range s.versions() {
		r, ok := s.Ready(v)
		if ok && (!found || semver.Compare(tag(r.Version), tag(best.Version)) > 0) {
			best, found = r, true
		}
	}
	return best, found
}

// versions lists the version folders.
func (s *Store) versions() []string {
	entries, _ := os.ReadDir(s.Dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() && semver.IsValid(tag(e.Name())) {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Store) readFailures() map[string]int {
	counts := map[string]int{}
	if data, err := os.ReadFile(filepath.Join(s.Dir, failuresName)); err == nil {
		if json.Unmarshal(data, &counts) != nil {
			return map[string]int{}
		}
	}
	return counts
}

// writeFailures needs the lock.
func (s *Store) writeFailures(counts map[string]int) error {
	for v, n := range counts {
		if n <= 0 {
			delete(counts, v)
		}
	}
	path := filepath.Join(s.Dir, failuresName)
	if len(counts) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, _ := json.Marshal(counts)
	return writeFileAtomic(path, data)
}

// Failures is how many downloads of version have failed.
func (s *Store) Failures(version string) int { return s.readFailures()[version] }

// GaveUp reports whether version has failed often enough that the app should
// show the release page instead.
func (s *Store) GaveUp(version string) bool { return s.Failures(version) >= s.maxFailures() }

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// lock takes the folder's lock, waiting for another process until ctx ends.
func (s *Store) lock(ctx context.Context, wait bool) (*filelock.Lock, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Dir, lockName)
	for {
		l, err := filelock.TryLock(path)
		if err == nil {
			return l, nil
		}
		if !errors.Is(err, filelock.ErrLocked) {
			return nil, err
		}
		if !wait {
			return nil, ErrBusy
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Fetch makes rel's installer ready: it downloads it, checks its size and
// SHA256 against rel (which came from the verified SHA256SUMS), and only then
// writes the ready marker. If another process is downloading, Fetch waits and
// then finds the result ready. A version that failed MaxFailures times is not
// tried again (ErrGaveUp). A download cut short by ctx is not a failure.
func (s *Store) Fetch(ctx context.Context, rel Release) (Ready, error) {
	if !safeName(rel.Version) || !safeName(rel.File) || !hexSHA256.MatchString(rel.SHA256) || rel.URL == "" {
		return Ready{}, fmt.Errorf("refusing to download an unusable release %+v", rel)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock(ctx, true)
	if err != nil {
		return Ready{}, err
	}
	defer lock.Unlock()

	if r, ok := s.Ready(rel.Version); ok && r.File == rel.File && r.SHA256 == rel.SHA256 {
		return r, nil
	}
	failures := s.readFailures()
	if failures[rel.Version] >= s.maxFailures() {
		return Ready{}, fmt.Errorf("%w (%s)", ErrGaveUp, rel.Version)
	}

	r, err := s.download(ctx, rel)
	if err != nil {
		if ctx.Err() == nil {
			failures[rel.Version]++
			_ = s.writeFailures(failures)
		}
		return Ready{}, err
	}
	if failures[rel.Version] > 0 {
		delete(failures, rel.Version)
		_ = s.writeFailures(failures)
	}
	return r, nil
}

// download needs the lock.
func (s *Store) download(ctx context.Context, rel Release) (Ready, error) {
	dir := s.versionDir(rel.Version)
	// Start clean: a marker for other bytes must not outlive them.
	if err := os.RemoveAll(dir); err != nil {
		return Ready{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Ready{}, err
	}
	part := filepath.Join(dir, rel.File+".part")
	size, sum, err := s.copyTo(ctx, rel.URL, part)
	if err != nil {
		_ = os.Remove(part)
		return Ready{}, err
	}
	if sum != rel.SHA256 {
		_ = os.Remove(part)
		return Ready{}, fmt.Errorf("download of %s does not match its checksum", rel.File)
	}
	if err := os.Rename(part, filepath.Join(dir, rel.File)); err != nil {
		_ = os.Remove(part)
		return Ready{}, err
	}
	r := Ready{Version: rel.Version, File: rel.File, SHA256: sum, Size: size, Dir: dir}
	data, _ := json.Marshal(r)
	if err := writeFileAtomic(filepath.Join(dir, readyName), data); err != nil {
		_ = os.RemoveAll(dir)
		return Ready{}, err
	}
	return r, nil
}

// copyTo downloads url into path and returns the size and SHA256 it read. The
// download is abandoned when no byte arrives for the idle timeout.
func (s *Store) copyTo(ctx context.Context, url, path string) (int64, string, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	idle := s.idleTimeout()
	timer := time.AfterFunc(idle, func() { cancel(fmt.Errorf("no data for %v", idle)) })
	defer timer.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "agent-sessions")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return 0, "", cause(ctx, fmt.Errorf("download update: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("download update: %s answered %s", url, resp.Status)
	}
	if resp.ContentLength > maxInstallerBytes {
		return 0, "", fmt.Errorf("download update: %s is %d bytes, over the limit", url, resp.ContentLength)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	body := io.TeeReader(&idleReader{r: resp.Body, timer: timer, idle: idle}, h)
	n, copyErr := io.Copy(f, io.LimitReader(body, maxInstallerBytes+1))
	closeErr := f.Close()
	switch {
	case copyErr != nil:
		return 0, "", cause(ctx, fmt.Errorf("download update: %w", copyErr))
	case closeErr != nil:
		return 0, "", closeErr
	case n > maxInstallerBytes:
		return 0, "", fmt.Errorf("download update: %s is over the size limit", url)
	case resp.ContentLength >= 0 && n != resp.ContentLength:
		return 0, "", fmt.Errorf("download update: got %d of %d bytes", n, resp.ContentLength)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// cause reports why ctx ended when that is the reason err happened.
func cause(ctx context.Context, err error) error {
	if c := context.Cause(ctx); c != nil && ctx.Err() != nil {
		if errors.Is(c, context.Canceled) {
			return c
		}
		return fmt.Errorf("%w: %v", err, c)
	}
	return err
}

// idleReader restarts the idle timer on every read that returns data.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(i.idle)
	}
	return n, err
}

// Reconcile removes the version folders other than keep, the version the
// latest check found, and the partial files in keep. Pass "" to remove every
// version. This drops older versions, interrupted downloads, and a ready
// version that GitHub no longer offers as latest (a pulled release). Other
// files, such as the installer's log, stay. It returns ErrBusy, having
// removed nothing, while another process is downloading or installing.
func (s *Store) Reconcile(keep string) error {
	if _, err := os.Stat(s.Dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock(context.Background(), false)
	if err != nil {
		return err
	}
	defer lock.Unlock()

	var firstErr error
	for _, v := range s.versions() {
		dir := filepath.Join(s.Dir, v)
		if v == keep {
			s.removePartials(dir)
		} else if err := os.RemoveAll(dir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	failures := s.readFailures()
	for v := range failures {
		if v != keep {
			delete(failures, v)
		}
	}
	if err := s.writeFailures(failures); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *Store) removePartials(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") || strings.HasSuffix(e.Name(), ".tmp") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
