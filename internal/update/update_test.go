package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"1.2.3", "1.2.2", true},
		{"v1.10.0", "1.9.9", true},
		{"2.0.0", "1.99.0", true},
		{"1.2.3", "1.2.3-rc.1", true},
		{"1.2.2", "1.2.3", false},
		{"v1.2.3", "1.2.3", false},
		{"1.2.3", "dev", false},
		{"", "1.2.3", false},
		{"nightly", "1.2.3", false},
	} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func serve(t *testing.T, status int, body string) *Checker {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub's API turns away requests without a User-Agent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Checker{HTTP: srv.Client(), API: srv.URL}
}

func TestCheckFindsANewerRelease(t *testing.T) {
	c := serve(t, http.StatusOK, `{"tag_name":"v1.3.0","html_url":"https://github.com/ginkcodex/agent-sessions/releases/tag/v1.3.0"}`)
	info, err := c.Check(context.Background(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Current: "1.2.3", Latest: "1.3.0", URL: "https://github.com/ginkcodex/agent-sessions/releases/tag/v1.3.0", Available: true}
	if info != want {
		t.Fatalf("Check = %+v, want %+v", info, want)
	}

	info, err = c.Check(context.Background(), "1.3.0")
	if err != nil || info.Available || info.Latest != "1.3.0" {
		t.Fatalf("Check on the latest = %+v, %v", info, err)
	}
}

func TestCheckOpensOnlyThisRepositorysReleases(t *testing.T) {
	c := serve(t, http.StatusOK, `{"tag_name":"v1.3.0","html_url":"https://example.com/download"}`)
	info, err := c.Check(context.Background(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if want := ReleasesPage + "/tag/v1.3.0"; info.URL != want {
		t.Fatalf("URL = %q, want %q", info.URL, want)
	}
}

func TestCheckSkipsBuildsWithoutAVersion(t *testing.T) {
	c := &Checker{API: "http://127.0.0.1:0/unreachable"}
	info, err := c.Check(context.Background(), "dev")
	if err != nil || info != (Info{Current: "dev"}) {
		t.Fatalf("Check(dev) = %+v, %v", info, err)
	}
}

func TestCheckReportsFailures(t *testing.T) {
	for name, c := range map[string]*Checker{
		"rate limited": serve(t, http.StatusForbidden, `{"message":"API rate limit exceeded"}`),
		"no releases":  serve(t, http.StatusNotFound, `{"message":"Not Found"}`),
		"bad tag":      serve(t, http.StatusOK, `{"tag_name":"latest","html_url":""}`),
		"bad body":     serve(t, http.StatusOK, `<html>`),
	} {
		if info, err := c.Check(context.Background(), "1.2.3"); err == nil || info.Available {
			t.Errorf("%s: Check = %+v, %v; want an error", name, info, err)
		}
	}
}
