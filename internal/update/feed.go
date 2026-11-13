package update

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"aead.dev/minisign"
	"golang.org/x/mod/semver"
)

// ErrUnsigned means the signed feed cannot be used: no trusted key is built
// in, the release carries no signature, or it has no installer for this
// platform. Callers fall back to the unsigned release-page notice.
var ErrUnsigned = errors.New("no signed update feed")

// ErrBadSignature means SHA256SUMS or its signature was fetched but does not
// verify with any trusted key. Nothing from that release may be used.
var ErrBadSignature = errors.New("update signature does not verify")

const (
	maxSumsBytes = 256 << 10
	maxSigBytes  = 4 << 10
)

// Release is the newest release for one platform, read from a signed
// SHA256SUMS.
type Release struct {
	// Version is the release version without the tag's "v".
	Version string
	// Page is the release's page.
	Page string
	// File is this platform's installer file name.
	File string
	// URL downloads File.
	URL string
	// SHA256 is File's hash, lower-case hex, from the verified SHA256SUMS.
	SHA256 string
}

// installerSuffix is what follows "agent-sessions_<version>" in the file name
// of the package that carries this platform's updates. Linux has no
// self-update; its package only tells the app which version is out.
func installerSuffix(goos, goarch string) (string, bool) {
	switch {
	case goos == "darwin":
		return "_macos_universal.dmg", true
	case goos == "windows" && goarch == "amd64":
		return "_windows_amd64.msi", true
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return "_" + goarch + ".deb", true
	}
	return "", false
}

var hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// parseSums reads sha256sum output into file name -> hash. Lines with a hash
// that is not 64 hex digits, a repeated name, or no name make the whole file
// invalid: a signed file is expected to be exactly what CI wrote.
func parseSums(data []byte) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		hash, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimPrefix(name, " "), "*")
		if !ok || name == "" || !hexSHA256.MatchString(hash) {
			return nil, fmt.Errorf("malformed SHA256SUMS line %q", line)
		}
		if _, dup := sums[name]; dup {
			return nil, fmt.Errorf("SHA256SUMS lists %s twice", name)
		}
		sums[name] = hash
	}
	return sums, sc.Err()
}

// versionIn finds the single release version whose installer for this
// platform appears in sums.
func versionIn(sums map[string]string, suffix string) (string, error) {
	const prefix = "agent-sessions_"
	version := ""
	for name := range sums {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		if !semver.IsValid(tag(v)) {
			continue
		}
		if version != "" && version != v {
			return "", fmt.Errorf("%w: SHA256SUMS names two versions, %s and %s", ErrUnsigned, version, v)
		}
		version = v
	}
	if version == "" {
		return "", fmt.Errorf("%w: no installer for this platform in SHA256SUMS", ErrUnsigned)
	}
	return version, nil
}

// fetch reads at most limit bytes. found is false for a 404, which is how a
// release without the asset answers.
func (c *Checker) fetch(ctx context.Context, url string, limit int64) (body []byte, found bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "agent-sessions")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("check for updates: %s answered %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, false, fmt.Errorf("check for updates: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, false, fmt.Errorf("check for updates: %s is larger than %d bytes", url, limit)
	}
	return data, true, nil
}

// verifies reports whether any trusted key signed message.
func (c *Checker) verifies(message, signature []byte) bool {
	for _, key := range c.Keys {
		if minisign.Verify(key, message, signature) {
			return true
		}
	}
	return false
}

// Feed reads the latest release's signed SHA256SUMS for this platform.
//
// The newest release is the one GitHub marks Latest. Its SHA256SUMS names the
// version; the signature is then fetched from that exact release, so a
// release published between the two requests cannot pair a signature with
// the wrong file. Only after the signature verifies is anything in the file
// trusted. The result is ErrUnsigned when the feed cannot be used and
// ErrBadSignature when it must not be.
func (c *Checker) Feed(ctx context.Context) (Release, error) {
	goos, goarch := c.GOOS, c.GOARCH
	suffix, ok := installerSuffix(goos, goarch)
	if !ok {
		return Release{}, fmt.Errorf("%w: no installer for %s/%s", ErrUnsigned, goos, goarch)
	}
	if len(c.Keys) == 0 {
		return Release{}, fmt.Errorf("%w: no trusted key", ErrUnsigned)
	}
	base := strings.TrimSuffix(c.Releases, "/")

	sumsData, found, err := c.fetch(ctx, base+"/latest/download/SHA256SUMS", maxSumsBytes)
	if err != nil {
		return Release{}, err
	}
	if !found {
		return Release{}, fmt.Errorf("%w: the latest release has no SHA256SUMS", ErrUnsigned)
	}
	sums, err := parseSums(sumsData)
	if err != nil {
		return Release{}, fmt.Errorf("%w: %v", ErrBadSignature, err)
	}
	// The version only picks which signature to fetch; nothing is used
	// before the signature verifies.
	version, err := versionIn(sums, suffix)
	if err != nil {
		return Release{}, err
	}
	return c.signed(ctx, base, sumsData, sums, version, suffix)
}

// FeedFor reads the signed SHA256SUMS of one release, such as the version
// that is installed, for this platform.
func (c *Checker) FeedFor(ctx context.Context, version string) (Release, error) {
	suffix, ok := installerSuffix(c.GOOS, c.GOARCH)
	if !ok {
		return Release{}, fmt.Errorf("%w: no installer for %s/%s", ErrUnsigned, c.GOOS, c.GOARCH)
	}
	if len(c.Keys) == 0 {
		return Release{}, fmt.Errorf("%w: no trusted key", ErrUnsigned)
	}
	version = strings.TrimPrefix(version, "v")
	if !semver.IsValid(tag(version)) || strings.ContainsAny(version, "/\\") {
		return Release{}, fmt.Errorf("%w: %q is not a release version", ErrUnsigned, version)
	}
	base := strings.TrimSuffix(c.Releases, "/")
	sumsData, found, err := c.fetch(ctx, base+"/download/v"+version+"/SHA256SUMS", maxSumsBytes)
	if err != nil {
		return Release{}, err
	}
	if !found {
		return Release{}, fmt.Errorf("%w: release %s has no SHA256SUMS", ErrUnsigned, version)
	}
	sums, err := parseSums(sumsData)
	if err != nil {
		return Release{}, fmt.Errorf("%w: %v", ErrBadSignature, err)
	}
	if got, err := versionIn(sums, suffix); err != nil || got != version {
		return Release{}, fmt.Errorf("%w: release %s does not list its installer", ErrUnsigned, version)
	}
	return c.signed(ctx, base, sumsData, sums, version, suffix)
}

// signed checks sumsData against the signature published with release
// version, and only then reads its installer from sums.
func (c *Checker) signed(ctx context.Context, base string, sumsData []byte, sums map[string]string, version, suffix string) (Release, error) {
	releaseURL := base + "/download/v" + version + "/"

	sig, found, err := c.fetch(ctx, releaseURL+"SHA256SUMS.minisig", maxSigBytes)
	if err != nil {
		return Release{}, err
	}
	if !found {
		return Release{}, fmt.Errorf("%w: release %s has no signature", ErrUnsigned, version)
	}
	if !c.verifies(sumsData, sig) {
		return Release{}, fmt.Errorf("%w (release %s)", ErrBadSignature, version)
	}

	file := "agent-sessions_" + version + suffix
	return Release{
		Version: version,
		Page:    base + "/tag/v" + version,
		File:    file,
		URL:     releaseURL + file,
		SHA256:  sums[file],
	}, nil
}
