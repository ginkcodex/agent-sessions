package codediagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Integration regressions for automatic Go build contexts: discovery, approval
// and watcher fan-out. Everything is offline and uses fake language servers; no
// project tool is executed.

const goReportedConstraint = "//go:build (linux || darwin) && (webkit2_41 || desktop)\n\npackage p\n\nfunc Tagged() {}\n"

func skipUnlessGoContextHost(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the reported constraint selects a custom tag only on darwin or linux")
	}
}

func goContextPlan(t *testing.T, files map[string]string) (AuthorizedRoot, *GoplsPlan) {
	t.Helper()
	root, plan, _ := goContextPlanEnv(t, files)
	return root, plan
}

func goContextPlanEnv(t *testing.T, files map[string]string) (AuthorizedRoot, *GoplsPlan, GoplsEnvironment) {
	t.Helper()
	all := map[string]string{"go.mod": "module example.test/ctx\n\ngo 1.26\n"}
	for name, body := range files {
		all[name] = body
	}
	root := goplsRoot(t, all)
	bin, env := goplsHost(t, "v0.24.0", goplsModulePath)
	if err := os.WriteFile(filepath.Join(env.GoRoot, "VERSION"), []byte("go1.26.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := DiscoverGopls(root, []string{bin}, env)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Availability() != "available" {
		t.Fatalf("gopls unavailable: %s %s", plan.Availability(), plan.Provider().Notice)
	}
	return root, plan, env
}

func contextIDs(plan *GoplsPlan) []string {
	ids := []string{}
	for _, c := range plan.contexts {
		ids = append(ids, c.config.BuildContext)
	}
	return ids
}

// ownerOf resolves the single exclusive owner of a file and returns its context id.
func ownerOf(t *testing.T, docs SessionDocuments, root AuthorizedRoot, file string) string {
	t.Helper()
	all := docs.ResolveAll(root, file)
	if len(all) != 1 {
		t.Fatalf("%s: want exactly one owner, got %d", file, len(all))
	}
	return all[0].Config.BuildContext
}

func TestGoContextDiscoveryReportedSample(t *testing.T) {
	skipUnlessGoContextHost(t)
	root, plan := goContextPlan(t, map[string]string{
		"ordinary.go": "package p\n\nfunc Ordinary() {}\n",
		"tagged.go":   goReportedConstraint,
	})
	if ids := contextIDs(plan); !slices.Equal(ids, []string{"desktop"}) {
		t.Fatalf("contexts = %v, want exactly [desktop]", ids)
	}
	if plan.contextPartial || plan.Provider().Coverage.Partial {
		t.Fatalf("a fully understood module must not be partial: %+v", plan.Provider())
	}
	reg := plan.Registration()
	if reg.Live == nil || len(reg.Live.Workspaces) != 2 {
		t.Fatalf("want default plus desktop workspaces: %+v", reg.Live)
	}
	def, tagged := reg.Live.Workspaces[0].Server, reg.Live.Workspaces[1].Server
	if def.BuildContext != "" || def.Settings != nil {
		t.Fatalf("default context must stay unconfigured: %+v", def)
	}
	if tagged.BuildContext != "desktop" || string(tagged.Settings) != `{"gopls":{"buildFlags":["-tags=desktop"]}}` {
		t.Fatalf("desktop context: %q %s", tagged.BuildContext, tagged.Settings)
	}
	if !strings.Contains(plan.Provider().Notice, "desktop") || !strings.Contains(plan.Provider().Notice, "four language server processes") {
		t.Fatalf("notice does not disclose the inferred context: %q", plan.Provider().Notice)
	}
	docs, err := DiscoveryDocuments(root, []Registration{reg}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"ordinary.go": "", "tagged.go": "desktop"} {
		if got := ownerOf(t, docs, root, file); got != want {
			t.Errorf("%s owner = %q, want %q", file, got, want)
		}
	}
	if _, ok := docs.Resolve(root, "notes.txt"); ok {
		t.Fatal("non-Go file resolved")
	}
	// Coverage is broader than ownership: every context can depend on a Go file.
	if cover := docs.Cover(root, "tagged.go"); len(cover) != 2 {
		t.Fatalf("coverage = %d servers, want both contexts", len(cover))
	}
	// No inferred context at all keeps the legacy single-workspace registration.
	root2, plain := goContextPlan(t, map[string]string{"ordinary.go": "package p\n"})
	if len(plain.contexts) != 0 || len(plain.Registration().Live.Workspaces) != 0 || len(liveServers(plain.Registration().Live)) != 1 {
		t.Fatalf("legacy registration changed: %d contexts", len(plain.contexts))
	}
	cfg, _ := plain.Server()
	if cfg.Tool().ProviderID != goplsProviderID || cfg.Tool().BuildContext != "" {
		t.Fatalf("default tool identity changed: %+v", cfg.Tool())
	}
	_ = root2
}

func TestGoContextDiscoveryCapDedupAndOwnersAreDeterministic(t *testing.T) {
	skipUnlessGoContextHost(t)
	files := map[string]string{
		"a1.go": "//go:build alpha\n\npackage p\n",
		"a2.go": "//go:build alpha\n\npackage p\n",
		"b.go":  "//go:build bravo\n\npackage p\n",
		"c.go":  "//go:build charlie\n\npackage p\n",
		"d.go":  "//go:build delta\n\npackage p\n",
		"e.go":  "//go:build echo\n\npackage p\n",
		"z.go":  "package p\n",
		"x.go":  "//go:build cgo && desktop\n\npackage p\n", // depends on an unknown reserved fact
		"w.go":  "//go:build windows\n\npackage p\n",        // provably excluded on this host
	}
	var baseline []string
	var baselineOwners map[string]string
	var baselineFingerprint string
	for run := range 6 {
		root, plan := goContextPlan(t, files)
		ids := contextIDs(plan)
		if !slices.Equal(ids, []string{"alpha", "bravo", "charlie"}) {
			t.Fatalf("run %d contexts = %v, want the first three distinct sets in lexicographic order", run, ids)
		}
		if !plan.contextPartial || !plan.Provider().Coverage.Partial || !strings.Contains(plan.Provider().Notice, goContextPartial) {
			t.Fatalf("run %d: capped inventory must carry the partial notice: %+v", run, plan.Provider())
		}
		docs, err := DiscoveryDocuments(root, []Registration{plan.Registration()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		owners := map[string]string{}
		for file := range files {
			owners[file] = ownerOf(t, docs, root, file) // exactly one owner each
		}
		want := map[string]string{"a1.go": "alpha", "a2.go": "alpha", "b.go": "bravo", "c.go": "charlie", "d.go": "", "e.go": "", "z.go": "", "x.go": "", "w.go": ""}
		for file, id := range want {
			if owners[file] != id {
				t.Errorf("run %d: %s owner = %q, want %q", run, file, owners[file], id)
			}
		}
		fp, err := Fingerprint(root, liveTools(plan.Registration().Live))
		if err != nil {
			t.Fatal(err)
		}
		if run == 0 {
			baseline, baselineOwners, baselineFingerprint = ids, owners, fp
			continue
		}
		if !slices.Equal(ids, baseline) || len(owners) != len(baselineOwners) {
			t.Fatalf("run %d is not deterministic", run)
		}
		// Different temp roots have different fingerprints; only same-run structure is compared.
		if fp == "" || baselineFingerprint == "" {
			t.Fatal("empty fingerprint")
		}
	}
}

func TestGoContextConfigPolicyAndCompositeFingerprint(t *testing.T) {
	skipUnlessGoContextHost(t)
	root, plan := goContextPlan(t, map[string]string{"tagged.go": goReportedConstraint, "other.go": "//go:build alpha\n\npackage p\n"})
	reg := plan.Registration()
	servers := liveServers(reg.Live)
	if len(servers) != 3 {
		t.Fatalf("servers = %d", len(servers))
	}
	policies, ids := map[string]bool{}, map[string]bool{}
	for _, cfg := range servers {
		policies[cfg.Policy()] = true
		for _, tool := range cfg.Tools() {
			if ids[tool.ProviderID] {
				t.Fatalf("duplicate approval tool id %q across contexts", tool.ProviderID)
			}
			ids[tool.ProviderID] = true
		}
	}
	if len(policies) != 3 {
		t.Fatal("contexts must have distinct policies (exact flags are bound into policy)")
	}
	def := servers[0]
	if strings.Contains(def.Policy(), "settings:") || def.Tool().ProviderID != goplsProviderID {
		t.Fatalf("default context must keep legacy policy and tool id: %q %q", def.Policy(), def.Tool().ProviderID)
	}
	for _, cfg := range servers[1:] {
		want := `-tags=` + cfg.BuildContext
		if !strings.Contains(cfg.Policy(), want) {
			t.Fatalf("policy %q does not bind %s", cfg.Policy(), want)
		}
		var settings struct {
			Gopls struct {
				BuildFlags []string `json:"buildFlags"`
			} `json:"gopls"`
		}
		if err := json.Unmarshal(cfg.Settings, &settings); err != nil || !slices.Equal(settings.Gopls.BuildFlags, []string{"-tags=" + cfg.BuildContext}) {
			t.Fatalf("settings %s: %v", cfg.Settings, err)
		}
		if cfg.Tool().BuildContext != cfg.BuildContext || cfg.Tool().ProviderID == goplsProviderID {
			t.Fatalf("context tool identity: %+v", cfg.Tool())
		}
		// Clones are deep: mutating the copy cannot change the approved settings.
		clone := cloneServer(cfg)
		clone.Settings[0] = '['
		if cfg.Settings[0] != '{' {
			t.Fatal("cloneServer shares Settings")
		}
	}

	tools := liveTools(reg.Live)
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	// Display metadata never authorizes: changing or dropping it keeps the fingerprint.
	display := cloneTools(tools)
	for i := range display {
		display[i].BuildContext = "different,display"
	}
	if got, err := Fingerprint(root, display); err != nil || got != fp {
		t.Fatalf("display metadata changed the fingerprint: %v %v", got, err)
	}
	for i := range display {
		display[i].BuildContext = ""
	}
	if got, err := Fingerprint(root, display); err != nil || got != fp {
		t.Fatalf("dropping display metadata changed the fingerprint: %v %v", got, err)
	}
	// ...while the exact flags (Policy) are bound.
	flags := cloneTools(tools)
	for i := range flags {
		if flags[i].BuildContext == "alpha" {
			flags[i].Policy = strings.ReplaceAll(flags[i].Policy, "-tags=alpha", "-tags=other")
		}
	}
	if got, err := Fingerprint(root, flags); err != nil || got == fp {
		t.Fatalf("tampered flags kept the fingerprint: %v %v", got, err)
	}

	// The manager offers one composite proposal with display-only context metadata.
	trust := NewTrust(nil)
	mgr, err := NewManager("ctx", trust, NewSupervisor(trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	snap, err := mgr.Attach(root, 1, []Registration{reg})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Proposal == nil || snap.Proposal.Fingerprint != fp || snap.Approval.State != "required" {
		t.Fatalf("proposal %+v approval %+v", snap.Proposal, snap.Approval)
	}
	var shown []string
	for _, tool := range snap.Proposal.Tools {
		if tool.BuildContext != "" {
			shown = append(shown, tool.BuildContext)
			if tool.ProviderID == goplsProviderID {
				t.Fatalf("context tool reused the default id: %+v", tool)
			}
		}
	}
	slices.Sort(shown)
	if !slices.Equal(shown, []string{"alpha", "desktop"}) {
		t.Fatalf("proposal display contexts = %v", shown)
	}
	raw, _ := json.Marshal(snap.Proposal)
	if strings.Contains(string(raw), "buildFlags") || strings.Contains(string(raw), "-tags=") {
		t.Fatalf("flags leaked into display metadata: %s", raw)
	}
}

func TestGoContextInventoryChangeNeedsRenewedApproval(t *testing.T) {
	skipUnlessGoContextHost(t)
	root, first, env := goContextPlanEnv(t, map[string]string{"a.go": "//go:build alpha\n\npackage p\n"})
	fp1, err := Fingerprint(root, liveTools(first.Registration().Live))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root.Project().Root, "a.go"), []byte("//go:build bravo\n\npackage p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(filepath.Dir(env.GoRoot), "bin")
	second, err := DiscoverGopls(root, []string{bin}, env)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(contextIDs(second), []string{"bravo"}) {
		t.Fatalf("contexts = %v", contextIDs(second))
	}
	fp2, err := Fingerprint(root, liveTools(second.Registration().Live))
	if err != nil || fp1 == fp2 {
		t.Fatalf("inventory change kept the proposal fingerprint: %v %v", fp1 == fp2, err)
	}
	// Unchanged discovery is stable.
	third, err := DiscoverGopls(root, []string{bin}, env)
	if err != nil {
		t.Fatal(err)
	}
	fp3, err := Fingerprint(root, liveTools(third.Registration().Live))
	if err != nil || fp3 != fp2 {
		t.Fatalf("repeat discovery changed the fingerprint: %v", err)
	}
}

func TestGoContextHeaderValidation(t *testing.T) {
	skipUnlessGoContextHost(t)
	disk := goReportedConstraint
	root, plan := goContextPlan(t, map[string]string{"ordinary.go": "package p\n\nfunc O() {}\n", "tagged.go": disk})
	docs, err := DiscoveryDocuments(root, []Registration{plan.Registration()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	validate := func(file, text string) error {
		return docs.ValidateDocument(root, file, DocumentOwnerIdentity{}, text)
	}
	write := func(file, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root.Project().Root, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validate("tagged.go", disk); err != nil {
		t.Fatalf("unchanged tagged file rejected: %v", err)
	}
	if err := validate("ordinary.go", "package p\n\nfunc O() {}\n"); err != nil {
		t.Fatalf("unchanged ordinary file rejected: %v", err)
	}
	// Ordinary body edits never change approval.
	if err := validate("tagged.go", disk+"\nfunc More() { _ = 1 }\n"); err != nil {
		t.Fatalf("body edit rejected: %v", err)
	}
	// A displayed header that selects another context fails with bounded guidance.
	for name, text := range map[string]string{
		"removed constraint": "package p\n",
		"other tag":          "//go:build alpha\n\npackage p\n",
		"unknown dependency": "//go:build cgo && desktop\n\npackage p\n",
		"malformed":          "//go:build desktop &&\n\npackage p\n",
		"truncated":          "//go:build desktop\n",
	} {
		err := validate("tagged.go", text)
		if errCode(err) != "unsupported" || !strings.Contains(err.Error(), "Rediscover checkers") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A default-owned file whose displayed header now needs a context is also refused.
	if errCode(validate("ordinary.go", "//go:build desktop\n\npackage p\n")) != "unsupported" {
		t.Error("default file gaining a constraint accepted")
	}
	// The saved header must match as well, even if the displayed text still does.
	write("tagged.go", "//go:build alpha\n\npackage p\n")
	if errCode(validate("tagged.go", disk)) != "unsupported" {
		t.Error("changed saved header accepted")
	}
	write("tagged.go", disk)
	if err := validate("tagged.go", disk); err != nil {
		t.Fatalf("restored header rejected: %v", err)
	}
	// A deleted file resolves no owner, so there is nothing to validate; opening
	// it for synchronization fails at path resolution instead.
	if err := os.Remove(filepath.Join(root.Project().Root, "tagged.go")); err != nil {
		t.Fatal(err)
	}
	if len(docs.ResolveAll(root, "tagged.go")) != 0 {
		t.Error("deleted file still has an owner")
	}
}

func TestGoContextHeaderValidationDefaultFallback(t *testing.T) {
	skipUnlessGoContextHost(t)
	// A constraint that begins after the 16 KiB header budget is not understood at
	// discovery: the file stays on the default context with a partial notice, and
	// validation must agree rather than demand a context nobody offered.
	pad := strings.Repeat("// padding padding padding padding\n", (maxGoBuildHeaderBytes/36)+16)
	late := pad + "//go:build desktop\n\npackage p\n"
	root, plan := goContextPlan(t, map[string]string{"late.go": late, "x.go": "//go:build cgo && desktop\n\npackage p\n", "w.go": "//go:build windows\n\npackage p\n"})
	if len(plan.contexts) != 0 || !plan.contextPartial {
		t.Fatalf("contexts %v partial %v", contextIDs(plan), plan.contextPartial)
	}
	docs, err := DiscoveryDocuments(root, []Registration{plan.Registration()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, text := range map[string]string{"late.go": late, "x.go": "//go:build cgo && desktop\n\npackage p\n", "w.go": "//go:build windows\n\npackage p\n"} {
		if got := ownerOf(t, docs, root, file); got != "" {
			t.Errorf("%s owner %q, want default", file, got)
		}
		if err := docs.ValidateDocument(root, file, DocumentOwnerIdentity{}, text); err != nil {
			t.Errorf("%s: default fallback rejected: %v", file, err)
		}
	}
}

func TestGoContextHeaderValidationCappedFileStaysOnDefault(t *testing.T) {
	skipUnlessGoContextHost(t)
	files := map[string]string{
		"a.go": "//go:build alpha\n\npackage p\n", "b.go": "//go:build bravo\n\npackage p\n",
		"c.go": "//go:build charlie\n\npackage p\n", "d.go": "//go:build delta\n\npackage p\n",
	}
	root, plan := goContextPlan(t, files)
	if !slices.Equal(contextIDs(plan), []string{"alpha", "bravo", "charlie"}) {
		t.Fatalf("contexts = %v", contextIDs(plan))
	}
	docs, err := DiscoveryDocuments(root, []Registration{plan.Registration()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownerOf(t, docs, root, "d.go"); got != "" {
		t.Fatalf("capped file owner = %q, want default", got)
	}
	// The plan says capped files "stay on default analysis". Validation must not
	// turn that into a permanent refusal for an unchanged file.
	if err := docs.ValidateDocument(root, "d.go", DocumentOwnerIdentity{}, files["d.go"]); err != nil {
		t.Errorf("unchanged capped file refused on its default fallback: %v", err)
	}
	if err := docs.ValidateDocument(root, "a.go", DocumentOwnerIdentity{}, files["a.go"]); err != nil {
		t.Errorf("offered context rejected its own file: %v", err)
	}
}

// ---- watcher fan-out against fake sessions -------------------------------------

type watchRecorder struct {
	mu     sync.Mutex
	frames map[*fakeSrv][]map[string]any
}

func (w *watchRecorder) script(f *fakeSrv) {
	for {
		m, ok := f.read()
		if !ok {
			return
		}
		id, _ := json.Marshal(m["id"])
		switch m["method"] {
		case "initialize":
			f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + defaultInit + `}`)
		case "shutdown":
			f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":null}`)
		case "workspace/didChangeWatchedFiles":
			w.mu.Lock()
			w.frames[f] = append(w.frames[f], m)
			w.mu.Unlock()
		case "exit":
			_ = f.Terminate()
			return
		}
	}
}

func (w *watchRecorder) uris(f *fakeSrv) (frames int, uris []string, kinds []float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, m := range w.frames[f] {
		frames++
		for _, c := range m["params"].(map[string]any)["changes"].([]any) {
			ev := c.(map[string]any)
			uris = append(uris, ev["uri"].(string))
			kinds = append(kinds, ev["type"].(float64))
		}
	}
	return
}

func TestGoContextWatcherFanOutExactSessions(t *testing.T) {
	skipUnlessGoContextHost(t)
	root, plan := goContextPlan(t, map[string]string{
		"ordinary.go": "package p\n", "tagged.go": goReportedConstraint, "other.go": "//go:build alpha\n\npackage p\n",
	})
	if !slices.Equal(contextIDs(plan), []string{"alpha", "desktop"}) {
		t.Fatalf("contexts = %v", contextIDs(plan))
	}
	e := newEnv(t, SessionOptions{})
	rec := &watchRecorder{frames: map[*fakeSrv][]map[string]any{}}
	e.script = rec.script
	reg := plan.Registration()
	tools := liveTools(reg.Live)
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	// One whole grant covers every context; nothing starts before it is used.
	approve(t, e.trust, root, tools, fp, "session")
	docs, err := DiscoveryDocuments(root, []Registration{reg}, e.mgr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.mgr.Close()
		assertOwnedClean(t, "go context cleanup", e.mgr, nil, nil)
	})
	cfgOf := func(file string) ServerConfig { return docs.ResolveAll(root, file)[0].Config }
	changes := []FileChange{
		{Path: "tagged.go", Kind: 2}, {Path: "ordinary.go", Kind: 1}, {Path: "gone.go", Kind: 3},
		{Path: "notes.txt", Kind: 2}, {Path: "../escape.go", Kind: 2}, {Path: "tagged.go", Kind: 9},
	}
	// No session runs: the watcher never launches one.
	if err := docs.WatchedFiles(context.Background(), root, changes); err == nil || e.launches() != 0 {
		t.Fatalf("watcher launched or succeeded without a session: %v %d", err, e.launches())
	}

	// Start only the default and desktop contexts; alpha stays stopped.
	ctx := context.Background()
	if _, err := e.mgr.Attach(ctx, root, cfgOf("ordinary.go")); err != nil {
		t.Fatal(err)
	}
	defSrv := e.next()
	if _, err := e.mgr.Attach(ctx, root, cfgOf("tagged.go")); err != nil {
		t.Fatal(err)
	}
	desktopSrv := e.next()
	if e.launches() != 2 {
		t.Fatalf("launches = %d", e.launches())
	}
	if err := os.WriteFile(filepath.Join(root.Project().Root, "tagged.go"), []byte(goReportedConstraint+"// edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := docs.WatchedFiles(ctx, root, changes); err != nil {
		t.Fatal(err)
	}
	base := fileURI(root.Project().Root)
	for name, srv := range map[string]*fakeSrv{"default": defSrv, "desktop": desktopSrv} {
		eventually(t, name+" watched frame", func() bool { n, _, _ := rec.uris(srv); return n > 0 })
		frames, uris, kinds := rec.uris(srv)
		// One batched notification per exact running session, with contained events only.
		if frames != 1 || len(uris) != 3 || !slices.Equal(kinds, []float64{2, 1, 3}) {
			t.Fatalf("%s: frames=%d uris=%v kinds=%v", name, frames, uris, kinds)
		}
		for i, suffix := range []string{"/tagged.go", "/ordinary.go", "/gone.go"} {
			if !strings.HasPrefix(uris[i], base) || !strings.HasSuffix(uris[i], suffix) {
				t.Fatalf("%s: uri %d = %s", name, i, uris[i])
			}
		}
	}
	if e.launches() != 2 {
		t.Fatalf("watcher launched a context: %d", e.launches())
	}

	// A deletion alone is portable (the file need not exist) and still fans out once.
	if err := docs.WatchedFiles(ctx, root, []FileChange{{Path: "removed.go", Kind: 3}}); err != nil {
		t.Fatal(err)
	}
	for name, srv := range map[string]*fakeSrv{"default": defSrv, "desktop": desktopSrv} {
		eventually(t, name+" deletion frame", func() bool { n, _, _ := rec.uris(srv); return n == 2 })
	}

	// Stop the default session: only the remaining running session is addressed.
	e.mgr.Hide(root, cfgOf("ordinary.go"))
	e.clock.Advance(idleGrace)
	eventually(t, "default stopped", func() bool { return e.mgr.Info(root, cfgOf("ordinary.go")).Phase == phaseStopped })
	if err := docs.WatchedFiles(ctx, root, []FileChange{{Path: "tagged.go", Kind: 2}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "desktop third frame", func() bool { n, _, _ := rec.uris(desktopSrv); return n == 3 })
	if n, _, _ := rec.uris(defSrv); n != 2 {
		t.Fatalf("stopped session received a change: %d", n)
	}
	if e.launches() != 2 {
		t.Fatalf("watcher restarted a stopped context: %d", e.launches())
	}

	// With every session gone the watcher reports instead of starting anything.
	e.mgr.Revoke(root)
	if err := docs.WatchedFiles(ctx, root, []FileChange{{Path: "tagged.go", Kind: 2}}); err == nil || e.launches() != 2 {
		t.Fatalf("revoked sessions: %v %d", err, e.launches())
	}
}

func TestGoContextCompositeApprovalAndIndependentOwners(t *testing.T) {
	skipUnlessGoContextHost(t)
	root, plan := goContextPlan(t, map[string]string{"ordinary.go": "package p\n", "tagged.go": goReportedConstraint, "other.go": "//go:build alpha\n\npackage p\n"})
	reg := plan.Registration()
	e := newEnv(t, SessionOptions{})
	mgr, err := NewManager("ctx", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mgr.Close()
		e.mgr.Close()
		assertOwnedClean(t, "go context cleanup", e.mgr, nil, nil)
	})
	snap, err := mgr.Attach(root, 1, []Registration{reg})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := DiscoveryDocuments(root, []Registration{reg}, e.mgr)
	if err != nil {
		t.Fatal(err)
	}
	cfgOf := func(file string) ServerConfig { return docs.ResolveAll(root, file)[0].Config }
	for _, file := range []string{"ordinary.go", "tagged.go", "other.go"} {
		_, err := e.mgr.Attach(context.Background(), root, cfgOf(file))
		code(t, err, "approvalRequired")
	}
	if e.launches() != 0 {
		t.Fatal("started before approval")
	}
	if _, err := mgr.Approve(snap.Context, snap.Proposal.Fingerprint, "session"); err != nil {
		t.Fatal(err)
	}
	generations := map[string]string{}
	for _, file := range []string{"ordinary.go", "tagged.go", "other.go"} {
		if _, err := e.mgr.Attach(context.Background(), root, cfgOf(file)); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		generations[file] = e.mgr.Info(root, cfgOf(file)).Generation
	}
	if e.launches() != 3 {
		t.Fatalf("contexts did not start independently: %d", e.launches())
	}
	for _, file := range []string{"ordinary.go", "tagged.go", "other.go"} {
		if _, err := e.mgr.Attach(context.Background(), root, cfgOf(file)); err != nil || e.launches() != 3 {
			t.Fatalf("reuse of %s started another session: %v %d", file, err, e.launches())
		}
	}
	if generations["ordinary.go"] == generations["tagged.go"] || generations["tagged.go"] == generations["other.go"] || generations["ordinary.go"] == generations["other.go"] || generations["ordinary.go"] == "" {
		t.Fatalf("contexts do not have independent generations: %v", generations)
	}
	owner, generation, _, err := docs.OwnerSession(root, "tagged.go")
	if err != nil || owner.ProviderID != goplsProviderID || owner.Policy != cfgOf("tagged.go").Policy() || generation != generations["tagged.go"] {
		t.Fatalf("owner %+v generation %s: %v", owner, generation, err)
	}
	changed := cloneServer(cfgOf("tagged.go"))
	changed.Settings = []byte(`{"gopls":{"buildFlags":["-tags=other"]}}`)
	if _, err := e.mgr.Attach(context.Background(), root, changed); err == nil || e.launches() != 3 {
		t.Fatalf("unapproved flags accepted: %v %d", err, e.launches())
	}
}

func TestGoContextSessionsAreCappedAtDefaultPlusThree(t *testing.T) {
	skipUnlessGoContextHost(t)
	files := map[string]string{"ordinary.go": "package p\n"}
	for _, tag := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		files[tag+".go"] = "//go:build " + tag + "\n\npackage p\n"
	}
	root, plan := goContextPlan(t, files)
	reg := plan.Registration()
	if got := len(reg.Live.Workspaces); got != 1+maxGoContexts {
		t.Fatalf("workspaces = %d, want default plus %d", got, maxGoContexts)
	}
	e := newEnv(t, SessionOptions{})
	tools := liveTools(reg.Live)
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	approve(t, e.trust, root, tools, fp, "session")
	docs, err := DiscoveryDocuments(root, []Registration{reg}, e.mgr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.mgr.Close()
		assertOwnedClean(t, "go context cleanup", e.mgr, nil, nil)
	})
	for _, cfg := range []ServerConfig{docs.ResolveAll(root, "ordinary.go")[0].Config} {
		if _, err := e.mgr.Attach(context.Background(), root, cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, tag := range []string{"alpha", "bravo", "charlie"} {
		if _, err := e.mgr.Attach(context.Background(), root, docs.ResolveAll(root, tag+".go")[0].Config); err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
	}
	if e.launches() != 4 {
		t.Fatalf("launches = %d, want four lazily started contexts", e.launches())
	}
	// A context that was not offered cannot be smuggled in by hand.
	rogue := cloneServer(docs.ResolveAll(root, "alpha.go")[0].Config)
	rogue.BuildContext = "delta"
	rogue.Settings = []byte(`{"gopls":{"buildFlags":["-tags=delta"]}}`)
	if _, err := e.mgr.Attach(context.Background(), root, rogue); err == nil || e.launches() != 4 {
		t.Fatalf("unoffered context started: %v %d", err, e.launches())
	}
}
