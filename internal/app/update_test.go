package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/update"
	"github.com/ginkcode/agent-sessions/internal/version"
)

func TestCheckForUpdateComparesThisAppWithTheLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://github.com/ginkcodex/agent-sessions/releases/tag/v9.0.0"}`))
	}))
	defer srv.Close()
	a := &App{updates: &update.Checker{HTTP: srv.Client(), API: srv.URL}}

	saved := version.Version
	t.Cleanup(func() { version.Version = saved })
	version.Version = "1.2.3"
	info, err := a.CheckForUpdate()
	if err != nil || !info.Available || info.Current != "1.2.3" || info.Latest != "9.0.0" {
		t.Fatalf("CheckForUpdate = %+v, %v", info, err)
	}

	// A development build has nothing to update to.
	version.Version = "dev"
	if info, err := a.CheckForUpdate(); err != nil || info.Available || info.Latest != "" {
		t.Fatalf("CheckForUpdate on dev = %+v, %v", info, err)
	}
}
