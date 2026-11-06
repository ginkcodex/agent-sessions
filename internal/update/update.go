// Package update finds out whether a newer release of the app is out. It
// only reads GitHub's latest release; users download and install it
// themselves from the release page.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
}

// Checker reads the latest release.
type Checker struct {
	HTTP *http.Client
	// API is the latest-release endpoint; LatestAPI unless a test sets it.
	API string
}

// NewChecker returns a Checker that honours the proxy environment variables
// and gives up after 15 seconds.
func NewChecker() *Checker {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	return &Checker{HTTP: &http.Client{Timeout: 15 * time.Second, Transport: transport}, API: LatestAPI}
}

// Check compares current with the latest release. A build without a release
// version is not compared and makes no request.
func (c *Checker) Check(ctx context.Context, current string) (Info, error) {
	info := Info{Current: current}
	if !semver.IsValid(tag(current)) {
		return info, nil
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
