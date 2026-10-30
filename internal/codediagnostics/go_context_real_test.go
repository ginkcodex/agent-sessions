package codediagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestGoContextRealGopls is opt-in and never installs or downloads tools. Set
// AS_TEST_GOPLS to an absolute installed gopls path with its reviewed Go toolchain
// in the same bin directory. Only a fresh dependency-free synthetic module is
// authorized; neither the checkout nor a developer project gets an execution grant.
func TestGoContextRealGopls(t *testing.T) {
	gopls := os.Getenv("AS_TEST_GOPLS")
	if gopls == "" {
		t.Skip("set AS_TEST_GOPLS to an installed gopls with sibling go to run the offline smoke test")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the synthetic tagged fixture and production process-tree launcher require darwin or linux")
	}
	if !filepath.IsAbs(gopls) || filepath.Base(gopls) != "gopls" {
		t.Fatal("AS_TEST_GOPLS must be the absolute path of an installed gopls executable")
	}
	bin := filepath.Dir(gopls)
	goroot := filepath.Dir(bin)
	for _, executable := range []string{gopls, filepath.Join(bin, "go")} {
		if !isExecutable(executable) {
			t.Fatalf("required installed executable is unavailable: %s", executable)
		}
	}

	const ordinary = "package smoke\n\nfunc Ordinary() {}\nfunc UsesOrdinary() { Ordinary() }\n"
	const tagged = "//go:build (linux || darwin) && (webkit2_41 || desktop)\n\npackage smoke\n\nfunc Tagged() { Ordinary() }\n"
	root := goplsRoot(t, map[string]string{
		"go.mod":      "module example.test/isolated-gopls-smoke\n\ngo 1.26\n",
		"ordinary.go": ordinary,
		"tagged.go":   tagged,
	})
	isolation, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := GoplsEnvironment{
		Home: filepath.Join(isolation, "home"), Cache: filepath.Join(isolation, "gocache"),
		ModuleCache: filepath.Join(isolation, "gomodcache"), GoRoot: goroot,
	}
	for _, dir := range []string{env.Home, env.Cache, env.ModuleCache} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := DiscoverGopls(root, []string{bin}, env)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Availability() != "available" {
		t.Fatalf("gopls discovery: %s: %s", plan.Availability(), plan.Provider().Notice)
	}
	registration := plan.Registration()
	if registration.Live == nil || len(registration.Live.Workspaces) != 2 {
		t.Fatalf("want exactly default and inferred desktop contexts, got %+v", registration.Live)
	}
	for i := range registration.Live.Workspaces {
		cfg := &registration.Live.Workspaces[i].Server
		// This synthetic fixture has no cgo. Bind the explicit test-only override
		// into approval before attaching; leave Settings and the transport intact.
		cfg.Env = append(cfg.Env, "CGO_ENABLED=0")
		if i == 0 && (cfg.BuildContext != "" || cfg.Settings != nil) {
			t.Fatalf("default context unexpectedly configured: %+v", cfg)
		}
		if i == 1 && (cfg.BuildContext != "desktop" || string(cfg.Settings) != `{"gopls":{"buildFlags":["-tags=desktop"]}}`) {
			t.Fatalf("unexpected inferred context or settings: %+v", cfg)
		}
	}
	registrations := []Registration{registration}

	trust := NewTrust(nil) // no persistent grants
	manager, err := NewManager("synthetic-real-gopls-smoke", trust, NewSupervisor(trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	live := NewLiveDocuments(trust, manager, SessionOptions{Launch: func(in Invocation) (SessionProcess, SessionStreams, error) {
		// Verify the production invocation remains confined to the fresh fixture,
		// with isolated state and the normal offline/toolchain safety environment.
		if in.Directory != root.Project().Root {
			return nil, SessionStreams{}, fmt.Errorf("refusing non-synthetic working directory %q", in.Directory)
		}
		for _, entry := range []string{
			"HOME=" + env.Home, "GOCACHE=" + env.Cache, "GOMODCACHE=" + env.ModuleCache,
			"CGO_ENABLED=0", "GOENV=off", "GOWORK=off", "GOFLAGS=",
			"GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off",
		} {
			if !slices.Contains(in.Env, entry) {
				return nil, SessionStreams{}, fmt.Errorf("missing synthetic/offline environment entry %q", entry)
			}
		}
		process, streams, err := HostSessionLaunch(in)
		if err == nil {
			launches.Add(1)
		}
		return process, streams, err
	}})
	t.Cleanup(func() {
		manager.Close()
		live.Close() // production shutdown/exit, bounded kill fallback, and Wait
		if resources := live.Sessions.OwnedResources(); resources != (OwnedResources{}) {
			t.Errorf("session resources survived cleanup: %+v", resources)
		}
	})
	manager.SetDocumentProvider(live)
	if err := live.Register(root, registrations); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Attach(root, 1, registrations)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Proposal == nil || snapshot.Approval.State != "required" || launches.Load() != 0 {
		t.Fatal("fresh synthetic module must require explicit approval without launching")
	}
	// Explicit session-only synthetic test grant for the full discovered proposal.
	snapshot, err = manager.Approve(snapshot.Context, snapshot.Proposal.Fingerprint, "session")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Approval.State != "approved" || launches.Load() != 0 {
		t.Fatal("approval should grant the synthetic proposal but not launch a server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, document := range []struct{ path, text string }{{"ordinary.go", ordinary}, {"tagged.go", tagged}} {
		if _, err := manager.OpenDocument(ctx, snapshot.Context, document.path, document.path, 1, document.text, ""); err != nil {
			t.Fatalf("open %s: %v", document.path, err)
		}
	}
	ordinaryOwner, ordinaryGeneration, _, err := live.OwnerSession(root, "ordinary.go")
	if err != nil {
		t.Fatal(err)
	}
	taggedOwner, taggedGeneration, _, err := live.OwnerSession(root, "tagged.go")
	if err != nil {
		t.Fatal(err)
	}
	if ordinaryOwner.ProviderID != goplsProviderID || taggedOwner.ProviderID != goplsProviderID || ordinaryOwner.Policy == taggedOwner.Policy || ordinaryGeneration == taggedGeneration {
		t.Fatal("default and tagged files must have different policy owners and independent live generations")
	}
	if launches.Load() != 2 {
		t.Fatalf("want two coexisting context processes, got %d", launches.Load())
	}

	// Query after both opens: ordinary navigation must still work with the
	// tagged owner alive. Tagged -> Ordinary requires the real tagged package
	// context; a standalone excluded-file parse cannot resolve this other file.
	for _, query := range []struct {
		path string
		pos  Position
	}{{"ordinary.go", Position{3, 22}}, {"tagged.go", Position{4, 17}}, {"ordinary.go", Position{3, 22}}} {
		result, err := manager.Navigate(ctx, snapshot.Context, query.path, 1, NavigateDefinition, query.pos)
		if err != nil {
			t.Fatalf("definition from %s (gopls %s): %v", query.path, plan.Version(), err)
		}
		want := NavigationLocation{Path: "ordinary.go", Range: Range{Start: Position{2, 5}, End: Position{2, 13}}}
		if len(result.Locations) != 1 || result.Locations[0] != want {
			t.Fatalf("definition from %s = %+v, want %+v; tagged Settings must work through existing workspace/configuration", query.path, result.Locations, want)
		}
		t.Logf("gopls %s: %s -> ordinary.go:3:6 (both contexts open)", plan.Version(), query.path)
	}
	for _, cfg := range liveServers(registration.Live) {
		info := live.Sessions.Info(root, cfg)
		if info.Phase != phaseInitialized || info.Generation == "" {
			t.Fatalf("context %q did not coexist: %+v", cfg.BuildContext, info)
		}
	}
	if !strings.Contains(taggedOwner.Policy, `-tags=desktop`) {
		t.Fatal("tagged owner policy does not bind the discovered build flags")
	}
	t.Logf("verified %d coexisting real gopls processes, distinct policy owners, existing Settings transport, isolated state, CGO_ENABLED=0 and offline Go resolution", launches.Load())
}
