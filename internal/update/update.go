// Package update finds out whether a newer release of the app is out.
//
// The preferred source is the latest release's minisign-signed SHA256SUMS
// (Checker.Feed), which names the version and the installer and cannot be
// forged by whoever answers the request. Until a release is signed, or when
// no key is built in, it falls back to GitHub's latest-release API, which only
// supports a notice with the release page.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"aead.dev/minisign"
	"golang.org/x/mod/semver"
)

const (
	// ReleasesPage lists every release, with its downloads.
	ReleasesPage = "https://github.com/ginkcodex/agent-sessions/releases"
	// LatestAPI answers with the newest release that is neither a draft nor
	// a prerelease.
	LatestAPI = "https://api.github.com/repos/ginkcodex/agent-sessions/releases/latest"
)

// Info is the outcome of a check.
type Info struct {
	// Current is the running version, as the app reports it.
	Current string `json:"current"`
	// Latest is the newest release's version, without the tag's "v".
	Latest string `json:"latest"`
	// URL is the newest release's page.
	URL string `json:"url"`
	// Available is set when Latest is newer than Current. A build without a
	// release version ("dev") never has an update.
	Available bool `json:"available"`
	// Install says how a Linux install gets updates (InstallAptRepo, ...).
	// It is empty on other systems.
	Install string `json:"install,omitempty"`
	// Release is the verified installer for this platform. It is set only
	// when the answer came from the signed feed, and is not sent to the UI.
	Release *Release `json:"-"`
}

// Checker reads the latest release.
type Checker struct {
	HTTP *http.Client
	// API is the latest-release endpoint; LatestAPI unless a test sets it.
	API string
	// Releases is the releases page whose latest/download/ and download/
	// paths serve the signed files; ReleasesPage unless a test or
	// AGENT_SESSIONS_UPDATE_URL sets it.
	Releases string
	// Keys are the trusted minisign public keys. With none, only the
	// unsigned API check runs.
	Keys []minisign.PublicKey
	// GOOS and GOARCH pick the installer; the running platform unless a test
	// sets them.
	GOOS, GOARCH string
}

// NewChecker returns a Checker that honours the proxy environment variables
// and gives up after 15 seconds.
func NewChecker() *Checker {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	releases := ReleasesPage
	if override := strings.TrimSpace(os.Getenv("AGENT_SESSIONS_UPDATE_URL")); override != "" {
		releases = override
	}
	return &Checker{
		HTTP:     &http.Client{Timeout: 15 * time.Second, Transport: transport},
		API:      LatestAPI,
		Releases: releases,
		Keys:     builtinKeys(),
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
	}
}

// Check compares current with the latest release. A build without a release
// version is not compared and makes no request.
func (c *Checker) Check(ctx context.Context, current string) (Info, error) {
	info := Info{Current: current}
	if !semver.IsValid(tag(current)) {
		return info, nil
	}
	rel, err := c.Feed(ctx)
	switch {
	case err == nil:
		info.Latest, info.URL = rel.Version, rel.Page
		info.Available = Newer(rel.Version, current)
		if info.Available {
			info.Release = &rel
		}
		return info, nil
	case errors.Is(err, ErrUnsigned):
		// No signed feed to use: ask the API, for a notice only.
	default:
		// A bad signature, or the network failed: report it, and do not
		// fall back to an answer that nothing vouches for.
		return info, err
	}
	latest, url, err := c.latest(ctx)
	if err != nil {
		return info, err
	}
	info.Latest, info.URL = latest, url
	info.Available = Newer(latest, current)
	return info, nil
}

// Newer reports whether latest is a later version than current. Either may
// carry a "v"; a version that does not parse is never newer, nor older.
func Newer(latest, current string) bool {
	l, c := tag(latest), tag(current)
	return semver.IsValid(l) && semver.IsValid(c) && semver.Compare(l, c) > 0
}

func tag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func (c *Checker) latest(ctx context.Context) (version, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "agent-sessions")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("check for updates: GitHub answered %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", "", fmt.Errorf("check for updates: %w", err)
	}
	t := tag(release.TagName)
	if !semver.IsValid(t) {
		return "", "", errors.New("check for updates: the latest release has no version tag")
	}
	return strings.TrimPrefix(t, "v"), releasePage(release.HTMLURL, t), nil
}

// releasePage is the page the app opens. It only trusts a page of this
// repository's releases, so the response cannot send users anywhere else.
func releasePage(htmlURL, tag string) string {
	if strings.HasPrefix(htmlURL, ReleasesPage+"/") {
		return htmlURL
	}
	return ReleasesPage + "/tag/" + tag
}
