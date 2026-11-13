package update

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"aead.dev/minisign"
)

func hexSum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// sumsFor writes a SHA256SUMS in the format `sha256sum` prints.
func sumsFor(version string) string {
	var b strings.Builder
	for _, name := range []string{
		"agent-sessions_" + version + "_amd64.deb",
		"agent-sessions_" + version + "_arm64.deb",
		"agent-sessions_" + version + "_macos_universal.dmg",
		"agent-sessions_" + version + "_windows_amd64.msi",
		"agent-sessions_" + version + "_windows_amd64_setup.exe",
		"agent-sessions-cli-linux-amd64.gz",
	} {
		b.WriteString(hexSum(name) + "  " + name + "\n")
	}
	return b.String()
}

type feedServer struct {
	URL      string
	Checker  *Checker
	Requests atomic.Int32
	API      atomic.Int32
}

// newFeed serves releases at the root. latest is the version GitHub calls
// Latest; signed maps versions to the key (nil = no signature published).
func newFeed(t *testing.T, latest string, sums, sig map[string][]byte, goos, goarch string, keys ...minisign.PublicKey) *feedServer {
	t.Helper()
	fs := &feedServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/download/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		fs.Requests.Add(1)
		body, ok := sums[latest]
		if !ok {
			http.NotFound(w, nil)
			return
		}
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		fs.Requests.Add(1)
		rest := strings.TrimPrefix(r.URL.Path, "/download/v")
		version, file, _ := strings.Cut(rest, "/")
		if file == "SHA256SUMS" {
			if body, ok := sums[version]; ok {
				_, _ = w.Write(body)
				return
			}
			http.NotFound(w, r)
			return
		}
		if file != "SHA256SUMS.minisig" {
			http.NotFound(w, r)
			return
		}
		body, ok := sig[version]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		fs.API.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v` + latest + `","html_url":"` + ReleasesPage + `/tag/v` + latest + `"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fs.URL = srv.URL
	fs.Checker = &Checker{HTTP: srv.Client(), API: srv.URL + "/api", Releases: srv.URL, Keys: keys, GOOS: goos, GOARCH: goarch}
	return fs
}

func newKey(t *testing.T) (minisign.PublicKey, minisign.PrivateKey) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestFeedReturnsTheVerifiedInstallerForEachPlatform(t *testing.T) {
	pub, priv := newKey(t)
	body := []byte(sumsFor("1.6.0"))
	sig := minisign.SignWithComments(priv, body, "version:1.6.0", "untrusted")
	for _, tc := range []struct{ goos, goarch, file string }{
		{"darwin", "arm64", "agent-sessions_1.6.0_macos_universal.dmg"},
		{"darwin", "amd64", "agent-sessions_1.6.0_macos_universal.dmg"},
		{"windows", "amd64", "agent-sessions_1.6.0_windows_amd64.msi"},
		{"linux", "amd64", "agent-sessions_1.6.0_amd64.deb"},
		{"linux", "arm64", "agent-sessions_1.6.0_arm64.deb"},
	} {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			fs := newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": sig}, tc.goos, tc.goarch, pub)
			rel, err := fs.Checker.Feed(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := Release{
				Version: "1.6.0",
				Page:    fs.URL + "/tag/v1.6.0",
				File:    tc.file,
				URL:     fs.URL + "/download/v1.6.0/" + tc.file,
				SHA256:  hexSum(tc.file),
			}
			if rel != want {
				t.Fatalf("Feed = %+v, want %+v", rel, want)
			}
		})
	}
}

func TestFeedAcceptsTheBackupKey(t *testing.T) {
	primary, _ := newKey(t)
	backup, backupPriv := newKey(t)
	body := []byte(sumsFor("1.6.0"))
	sig := minisign.Sign(backupPriv, body)
	fs := newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": sig}, "darwin", "arm64", primary, backup)
	if _, err := fs.Checker.Feed(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFeedRefusesWhatNothingVouchesFor(t *testing.T) {
	pub, priv := newKey(t)
	_, otherPriv := newKey(t)
	body := []byte(sumsFor("1.6.0"))
	good := minisign.Sign(priv, body)

	tampered := []byte(strings.Replace(sumsFor("1.6.0"), hexSum("agent-sessions_1.6.0_macos_universal.dmg"), strings.Repeat("0", 64), 1))

	for name, tc := range map[string]struct {
		sums map[string][]byte
		sig  map[string][]byte
		keys []minisign.PublicKey
		want error
	}{
		"signed by an unknown key":   {map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": minisign.Sign(otherPriv, body)}, []minisign.PublicKey{pub}, ErrBadSignature},
		"sums changed after signing": {map[string][]byte{"1.6.0": tampered}, map[string][]byte{"1.6.0": good}, []minisign.PublicKey{pub}, ErrBadSignature},
		"garbage signature":          {map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": []byte("not a signature")}, []minisign.PublicKey{pub}, ErrBadSignature},
		"signature of an older release": {
			map[string][]byte{"1.6.0": body},
			map[string][]byte{"1.6.0": minisign.Sign(priv, []byte(sumsFor("1.5.0")))},
			[]minisign.PublicKey{pub}, ErrBadSignature,
		},
		"no signature published": {map[string][]byte{"1.6.0": body}, map[string][]byte{}, []minisign.PublicKey{pub}, ErrUnsigned},
		"no trusted key":         {map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": good}, nil, ErrUnsigned},
		"no SHA256SUMS":          {map[string][]byte{}, map[string][]byte{"1.6.0": good}, []minisign.PublicKey{pub}, ErrUnsigned},
		"malformed SHA256SUMS":   {map[string][]byte{"1.6.0": []byte("zzzz  file\n")}, map[string][]byte{"1.6.0": good}, []minisign.PublicKey{pub}, ErrBadSignature},
		"two versions in sums": {
			map[string][]byte{"1.6.0": []byte(sumsFor("1.6.0") + hexSum("x") + "  agent-sessions_1.7.0_macos_universal.dmg\n")},
			map[string][]byte{"1.6.0": good}, []minisign.PublicKey{pub}, ErrUnsigned,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fs := newFeed(t, "1.6.0", tc.sums, tc.sig, "darwin", "arm64", tc.keys...)
			rel, err := fs.Checker.Feed(context.Background())
			if !errors.Is(err, tc.want) || rel != (Release{}) {
				t.Fatalf("Feed = %+v, %v; want %v", rel, err, tc.want)
			}
		})
	}
}

func TestFeedHasNoInstallerForOtherPlatforms(t *testing.T) {
	pub, priv := newKey(t)
	body := []byte(sumsFor("1.6.0"))
	sig := minisign.Sign(priv, body)
	for _, tc := range []struct{ goos, goarch string }{{"windows", "arm64"}, {"freebsd", "amd64"}, {"", ""}} {
		fs := newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": sig}, tc.goos, tc.goarch, pub)
		if _, err := fs.Checker.Feed(context.Background()); !errors.Is(err, ErrUnsigned) {
			t.Errorf("%s/%s: Feed error = %v, want ErrUnsigned", tc.goos, tc.goarch, err)
		}
		if n := fs.Requests.Load(); n != 0 {
			t.Errorf("%s/%s: made %d requests for a platform with no installer", tc.goos, tc.goarch, n)
		}
	}
}

func TestCheckUsesTheSignedFeedAndNeverTheAPI(t *testing.T) {
	pub, priv := newKey(t)
	body := []byte(sumsFor("1.6.0"))
	sig := minisign.Sign(priv, body)
	fs := newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": sig}, "darwin", "arm64", pub)

	info, err := fs.Checker.Check(context.Background(), "1.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "1.6.0" || info.URL != fs.URL+"/tag/v1.6.0" || info.Release == nil || info.Release.File != "agent-sessions_1.6.0_macos_universal.dmg" {
		t.Fatalf("Check = %+v (release %+v)", info, info.Release)
	}
	if n := fs.API.Load(); n != 0 {
		t.Fatalf("a signed feed made %d API requests", n)
	}

	// Already up to date: nothing to install.
	info, err = fs.Checker.Check(context.Background(), "1.6.0")
	if err != nil || info.Available || info.Release != nil || info.Latest != "1.6.0" {
		t.Fatalf("Check on the latest = %+v, %v", info, err)
	}
	// A newer build than the release (a tagged pre-release in testing).
	info, err = fs.Checker.Check(context.Background(), "1.7.0")
	if err != nil || info.Available || info.Release != nil {
		t.Fatalf("Check on a newer build = %+v, %v", info, err)
	}
}

func TestCheckFallsBackToTheAPIOnlyWhenThereIsNoSignedFeed(t *testing.T) {
	pub, _ := newKey(t)
	body := []byte(sumsFor("1.6.0"))

	// Unsigned release: the old notice still works.
	fs := newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{}, "darwin", "arm64", pub)
	info, err := fs.Checker.Check(context.Background(), "1.5.5")
	if err != nil || !info.Available || info.Latest != "1.6.0" || info.Release != nil {
		t.Fatalf("Check = %+v, %v", info, err)
	}
	if fs.API.Load() != 1 {
		t.Fatalf("API requests = %d, want 1", fs.API.Load())
	}

	// A signature that does not verify is an error, with no fallback to an
	// answer nothing vouches for.
	_, otherPriv := newKey(t)
	fs = newFeed(t, "1.6.0", map[string][]byte{"1.6.0": body}, map[string][]byte{"1.6.0": minisign.Sign(otherPriv, body)}, "darwin", "arm64", pub)
	info, err = fs.Checker.Check(context.Background(), "1.5.5")
	if !errors.Is(err, ErrBadSignature) || info.Available {
		t.Fatalf("Check = %+v, %v; want ErrBadSignature", info, err)
	}
	if fs.API.Load() != 0 {
		t.Fatalf("a bad signature fell back to the API (%d requests)", fs.API.Load())
	}
}

func TestExtraKeysAddQATrustAtBuildTime(t *testing.T) {
	pub, _ := newKey(t)
	text, err := pub.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	savedKeys, savedExtra := trustedKeys, extraKeys
	t.Cleanup(func() { trustedKeys, extraKeys = savedKeys, savedExtra })

	trustedKeys, extraKeys = nil, ""
	if got := len(builtinKeys()); got != 0 {
		t.Fatalf("no keys configured gave %d", got)
	}
	extraKeys = " " + string(text) + " , not-a-key ,"
	keys := builtinKeys()
	if len(keys) != 1 || keys[0].ID() != pub.ID() {
		t.Fatalf("builtinKeys = %v, want only the valid extra key", keys)
	}
}

func TestBuiltInKeysParse(t *testing.T) {
	if got, want := len(builtinKeys()), len(trustedKeys); got != want {
		t.Fatalf("%d of the %d built-in keys parse; an entry is mistyped", got, want)
	}
	seen := map[uint64]bool{}
	for _, key := range builtinKeys() {
		if seen[key.ID()] {
			t.Fatalf("key %X is listed twice", key.ID())
		}
		seen[key.ID()] = true
	}
	if seen[0] {
		t.Fatal("a built-in key has no ID")
	}
}
