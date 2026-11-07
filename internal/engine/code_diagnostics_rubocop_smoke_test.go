package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// Real RuboCop smoke on a disposable synthetic project through the production
// adapter path: DiscoverLocalLive (Ruby and gem metadata only) +
// HostSessionLaunch, composite approval, an unsaved draft with a syntax error,
// then offenses, then the fixed text and its empty publication. The Ruby is
// supplied explicitly, never searched for: AGENT_SESSIONS_RUBOCOP_DIR names
// the bin directory of a native Ruby whose own gem directory has the rubocop
// gem (for example ~/.local/share/mise/installs/ruby/4.0.9/bin after
// gem install rubocop). HOME is an empty temp directory, so no version
// manager, user gem or user RuboCop configuration applies and RuboCop runs in
// standalone mode. Nothing is installed, downloaded or saved.
func TestLocalLiveRealRubocopSmoke(t *testing.T) {
	rubyDir := os.Getenv("AGENT_SESSIONS_RUBOCOP_DIR")
	if testing.Short() || rubyDir == "" {
		t.Skip("real language server: set AGENT_SESSIONS_RUBOCOP_DIR")
	}
	if !localLiveSupported() {
		t.Skip("local live platform gate")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, _ := filepath.EvalSymlinks(t.TempDir())
	files := map[string]string{
		".rubocop.yml": "AllCops:\n  NewCops: enable\n  SuggestExtensions: false\n",
		"app.rb":       "# frozen_string_literal: true\n\nputs 'ok'\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	discover := func(root cd.AuthorizedRoot) ([]cd.Registration, error) {
		return cd.DiscoverLocalLive(root, cd.LocalLiveEnvironment{
			Path: []string{rubyDir},
			Ruby: cd.RubyEnvironment{Home: home},
			// Only Ruby is looked up; the other languages need hosts of their own.
			Disabled: cd.NewDisabledLanguages([]string{"go", "typescript", "svelte", "python", "rust"}),
		})
	}
	root, err := cd.AuthorizeRoot("local:smoke", dir, func(p string) bool { return p == dir })
	if err != nil {
		t.Fatal(err)
	}
	regs, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(regs, func(r cd.Registration) bool { return r.ProviderID == "rubocop" })
	if i < 0 {
		t.Fatal("no rubocop registration")
	}
	rubocop := regs[i]
	if rubocop.Availability != "available" {
		t.Fatalf("rubocop not available: %s (%s)", rubocop.Availability, rubocop.Live.State.Notice)
	}
	t.Logf("launch %s %v env %v", rubocop.Live.Server.Executable, rubocop.Live.Server.Args, rubocop.Live.Server.Env)
	for _, tool := range rubocop.Live.Server.Tools() {
		t.Logf("approval input %s %s", tool.ProviderID, tool.Executable)
	}

	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "smoke"}, CWD: dir, RepoRoot: dir}})
	e := NewEngine(WithService(NewService(catalog, nil)), WithCacheEnabled(false), WithRoots(paths.Roots{Config: t.TempDir()}),
		WithInjectedLiveCodeDiagnostics(LiveCodeDiagnosticsInjection{Discover: discover, Session: cd.SessionOptions{Launch: cd.HostSessionLaunch}}))
	t.Cleanup(func() { _ = e.Close() })
	ctx := context.Background()
	att, err := e.AttachCodeDiagnostics(ctx, dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := att.Context
	var s cd.Snapshot
	waitLocal(t, 30*time.Second, "proposal", func() bool {
		s, err = e.GetCodeDiagnosticsSnapshot(ctx, c)
		return err == nil && s.Proposal != nil
	})
	syntaxError := "def broken(\n  1 +\n"
	if _, err = e.OpenCodeDiagnosticDocument(ctx, c, "app.rb", "rb-draft", 1, syntaxError, ""); err == nil {
		t.Fatal("opened before approval")
	} else {
		requireDiagnosticCode(t, err, "approvalRequired")
	}
	if _, err = e.ApproveCodeDiagnostics(ctx, c, "session", s.Proposal.Fingerprint); err != nil {
		t.Fatal(err)
	}
	result := func(s cd.Snapshot, path string) *cd.ResultSet {
		for i := range s.ResultSets {
			if s.ResultSets[i].ProviderID == "rubocop" && s.ResultSets[i].Scope.Path == path {
				return &s.ResultSets[i]
			}
		}
		return nil
	}
	// wait reports the provider state on timeout so a real-server failure is legible.
	wait := func(limit time.Duration, what string, cond func(cd.Snapshot) bool) {
		t.Helper()
		deadline := time.Now().Add(limit)
		for time.Now().Before(deadline) {
			if s, err = e.GetCodeDiagnosticsSnapshot(ctx, c); err == nil && cond(s) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		for _, p := range s.Providers {
			t.Logf("provider %s %s phase=%s freshness=%s notice=%q failure=%+v", p.ProviderID, p.Availability, p.Phase, p.Freshness, p.Notice, p.Failure)
		}
		t.Logf("result sets %+v", s.ResultSets)
		t.Fatalf("timed out: %s (%v)", what, err)
	}
	// RuboCop pushes are unversioned: observed at receipt, kept positioned,
	// and uncertain rather than current.
	codes := func(version int64, severity string, want ...string) {
		t.Helper()
		start := time.Now()
		var ds []cd.Diagnostic
		wait(60*time.Second, want[0], func(s cd.Snapshot) bool {
			r := result(s, "app.rb")
			if r == nil || len(r.Diagnostics) == 0 || r.Diagnostics[0].Code != want[0] {
				return false
			}
			ds = r.Diagnostics
			return true
		})
		got := []string{}
		for _, d := range ds {
			t.Logf("v%d after %s: %q code=%s source=%s severity=%s freshness=%s", version, time.Since(start).Round(time.Millisecond), d.Message, d.Code, d.Source, d.Severity, d.Freshness)
			if d.Source != "rubocop/RuboCop" || d.Severity != severity || d.Freshness != "uncertain" || d.Range == nil ||
				d.Basis.Version != nil || d.Basis.ObservedVersion == nil || *d.Basis.ObservedVersion != version {
				t.Fatalf("%+v", d)
			}
			got = append(got, d.Code)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("v%d codes %v, want %v", version, got, want)
		}
	}
	if _, err = e.OpenCodeDiagnosticDocument(ctx, c, "app.rb", "rb-draft", 1, syntaxError, ""); err != nil {
		t.Fatal(err)
	}
	codes(1, "error", "Lint/Syntax")
	if _, err = e.ChangeCodeDiagnosticDocument(ctx, c, "rb-draft", 2, "x = \"double\"\nputs(x)  \n"); err != nil {
		t.Fatal(err)
	}
	codes(2, "information", "Style/FrozenStringLiteralComment", "Style/StringLiterals", "Layout/TrailingWhitespace")
	if _, err = e.ChangeCodeDiagnosticDocument(ctx, c, "rb-draft", 3, files["app.rb"]); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	wait(60*time.Second, "empty clear", func(s cd.Snapshot) bool {
		r := result(s, "app.rb")
		return r == nil || len(r.Diagnostics) == 0
	})
	t.Logf("fixed draft cleared after %s", time.Since(start).Round(time.Millisecond))
	if _, err = e.CloseCodeDiagnosticDocument(ctx, c, "rb-draft"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != len(files) {
		t.Fatalf("RuboCop wrote into the project: %v", entries)
	}
	for name, body := range files {
		if data, _ := os.ReadFile(filepath.Join(dir, name)); string(data) != body {
			t.Fatal("draft written to disk", name)
		}
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("RuboCop wrote to HOME: %v", entries)
	}
}
