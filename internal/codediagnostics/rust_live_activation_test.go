package codediagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func rustLiveFixture(t *testing.T) (rustFixture, *RustPlan) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Rust live discovery requires macOS/Linux")
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='hello'\nversion='0.1.0'\n")
	f.put(t, "Cargo.lock", "version=3\n")
	f.put(t, "src/main.rs", "fn main() {}\n")
	return f, f.discover(t)
}

func fakeRustLive(p *RustPlan) Registration {
	return rustLiveRegistrationWithPin(p, func(path string) bool { return path != "" && path == p.analyzer })
}

func TestRustLiveWorkspacesOwnershipAndLockedSettings(t *testing.T) {
	f, _ := rustLiveFixture(t)
	f.put(t, "nested/Cargo.toml", "[package]\nname='nested'\nversion='0.1.0'\n[workspace]\n")
	f.put(t, "nested/Cargo.lock", "version=3\n")
	f.put(t, "nested/src/lib.rs", "pub fn nested() {}\n")
	p := f.discover(t)
	r := fakeRustLive(p)
	if r.Availability != "available" || len(r.Live.Workspaces) != 2 || r.Live.Server.Executable != "" {
		t.Fatalf("unexpected registration: %+v", r)
	}
	if err := validateLive(r); err != nil {
		t.Fatal(err)
	}
	docs, err := DiscoveryDocuments(f.root, []Registration{r}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"src/main.rs": "", "nested/src/lib.rs": "nested"} {
		server, ok := docs.Resolve(f.root, file)
		if !ok || server.Config.Directory != want || server.LanguageID != "rust" {
			t.Fatalf("owner %q: %+v %v", file, server, ok)
		}
		cfg := server.Config
		if !slices.Equal(cfg.Env, p.Environment()) || cfg.ValidateStart == nil || cfg.DeclareDiagnostics != true {
			t.Fatal("missing Rust invocation policy")
		}
		if err := cfg.ValidateStart(); err != nil {
			t.Fatal(err)
		}
		var init any
		if err := json.Unmarshal(cfg.InitializationOptions, &init); err != nil {
			t.Fatal(err)
		}
		settings, ok := boundedConfiguration(f.root, json.RawMessage(`{"items":[{"section":"rust-analyzer"}]}`), cfg.Settings)
		if !ok || len(settings) != 1 || !reflect.DeepEqual(init, settings[0]) {
			t.Fatalf("init/config differ: %s %v", cfg.InitializationOptions, settings)
		}
		wantOptions := map[string]any{
			"cargo": map[string]any{"metadataExtraArgs": []any{"--locked"}, "extraArgs": []any{"--locked"}},
			"check": map[string]any{"extraArgs": []any{}},
		}
		if !reflect.DeepEqual(init, wantOptions) {
			t.Fatal(init)
		}
		for _, tool := range cfg.Toolchain {
			if tool.Executable == cfg.Executable {
				t.Fatal("analyzer duplicated in toolchain")
			}
		}
		if cfg.command().ValidateStart == nil || !strings.Contains(cfg.Policy(), "revalidate-inputs") {
			t.Fatal("validation contract not propagated/bound")
		}
	}
	if _, ok := docs.Resolve(f.root, "Cargo.toml"); ok {
		t.Fatal("non-Rust document owned")
	}
	if _, ok := docs.Resolve(f.root, "../escape.rs"); ok {
		t.Fatal("escaping document owned")
	}
	if cover := docs.Cover(f.root, "nested/src/lib.rs"); len(cover) != 2 {
		t.Fatalf("expected both root and nested dependency coverage, got %d", len(cover))
	}
}

// tree bytes include all manifests, locks and source files, not just the inputs
// used by discovery. A fake startup must not write even unrelated project files.
func rustLiveTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRustLiveNestedFirstOpenServesConfiguration(t *testing.T) {
	f, _ := rustLiveFixture(t)
	f.put(t, "nested/Cargo.toml", "[workspace]\nmembers=['member']\n")
	f.put(t, "nested/Cargo.lock", "version=3\n")
	f.put(t, "nested/member/Cargo.toml", "[package]\nname='member'\nversion='0.1.0'\n")
	f.put(t, "nested/member/src/lib.rs", "pub fn helper() {}\n")
	f.put(t, "nested/member/src/other.rs", "pub fn other() {}\n")
	before := rustLiveTree(t, f.root.Project().Root)
	reg := fakeRustLive(f.discover(t))
	e := newEnv(t, SessionOptions{})
	opened := make(chan map[string]any, 2)
	initialized := make(chan map[string]any, 1)
	configurations := make(chan map[string]any, 1)
	e.script = func(srv *fakeSrv) {
		for {
			msg, ok := srv.read()
			if !ok {
				return
			}
			id, _ := json.Marshal(msg["id"])
			switch msg["method"] {
			case "initialize":
				initialized <- msg
				configurations <- srv.request(`"config"`, "workspace/configuration", `{"items":[{"section":"rust-analyzer"}]}`)
				srv.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + navigationInit + `}`)
			case "textDocument/didOpen":
				opened <- msg
			case "textDocument/definition":
				srv.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":{"uri":"` + uriOf(f.root, "nested/member/src/lib.rs") + `","range":` + navRange(0, 7, 0, 13) + `}}`)
			case "shutdown":
				srv.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":null}`)
			case "exit":
				_ = srv.Terminate()
				return
			}
		}
	}
	manager, err := NewManager("rust-nested-startup", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	live := NewLiveDocuments(e.trust, manager, e.mgr.options)
	t.Cleanup(func() { manager.Close(); live.Close() })
	if err := live.Register(f.root, []Registration{reg}); err != nil {
		t.Fatal(err)
	}
	manager.SetDocumentProvider(live)
	snapshot, err := manager.Attach(f.root, 1, []Registration{reg})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), documentIOTimeout)
	defer cancel()
	_, err = manager.OpenDocument(ctx, snapshot.Context, "nested/member/src/lib.rs", "unapproved", 1, "draft", "")
	code(t, err, "approvalRequired")
	if e.launches() != 0 {
		t.Fatal("unapproved open launched a server")
	}
	if _, err := manager.Approve(snapshot.Context, snapshot.Proposal.Fingerprint, "session"); err != nil {
		t.Fatal(err)
	}
	for i, file := range []string{"nested/member/src/lib.rs", "nested/member/src/other.rs"} {
		if _, err := manager.OpenDocument(ctx, snapshot.Context, file, fmt.Sprintf("d%d", i), 1, "displayed draft", ""); err != nil {
			t.Fatal(err)
		}
		msg := <-opened
		item := msg["params"].(map[string]any)["textDocument"].(map[string]any)
		if item["uri"] != uriOf(f.root, file) || item["text"] != "displayed draft" {
			t.Fatal(item)
		}
	}
	srv := e.next()
	if e.launches() != 1 || srv.inv.Directory != filepath.Join(f.root.Project().Root, "nested") {
		t.Fatalf("wrong workspace or duplicate server: %+v launches=%d", srv.inv, e.launches())
	}
	params := (<-initialized)["params"].(map[string]any)
	wantURI := uriOf(f.root, "nested")
	if params["rootUri"] != wantURI || params["workspaceFolders"].([]any)[0].(map[string]any)["uri"] != wantURI {
		t.Fatalf("wrong initialize scope: %v", params)
	}
	want, _ := boundedConfiguration(f.root, json.RawMessage(`{"items":[{"section":"rust-analyzer"}]}`), reg.Live.Workspaces[0].Server.Settings)
	if reply := <-configurations; !reflect.DeepEqual(reply["result"], want) {
		t.Fatalf("wrong locked configuration: %v", reply)
	}
	result, err := manager.Navigate(ctx, snapshot.Context, "d0", 1, NavigateDefinition, Position{})
	if err != nil || len(result.Locations) != 1 || result.Locations[0].Path != "nested/member/src/lib.rs" {
		t.Fatalf("nested navigation: %+v %v", result, err)
	}
	if after := rustLiveTree(t, f.root.Project().Root); !reflect.DeepEqual(before, after) {
		t.Fatal("startup changed project files")
	}
}

func TestRustLiveWorkspaceBudget(t *testing.T) {
	_, p := rustLiveFixture(t)
	p.roots = make([]string, maxLiveWorkspaces+1)
	for i := range p.roots {
		p.roots[i] = fmt.Sprintf("root%d", i)
	}
	r := fakeRustLive(p)
	if r.Availability != "unsupported" || len(r.Live.Workspaces) != 0 || !r.Live.State.Coverage.Partial {
		t.Fatal(r)
	}
}

func TestRustLiveApprovalAndPublicationRevalidateInputs(t *testing.T) {
	for _, mutation := range []string{"lock", "new config"} {
		t.Run(mutation, func(t *testing.T) {
			f, p := rustLiveFixture(t)
			r := fakeRustLive(p)
			e := newEnv(t, SessionOptions{})
			docs, err := DiscoveryDocuments(f.root, []Registration{r}, e.mgr)
			if err != nil {
				t.Fatal(err)
			}
			server, ok := docs.Resolve(f.root, "src/main.rs")
			if !ok {
				t.Fatal("missing owner")
			}
			cfg := server.Config
			if _, err := e.mgr.Attach(context.Background(), f.root, cfg); errCode(err) != "approvalRequired" || e.launches() != 0 {
				t.Fatalf("unapproved Rust launch: %v", err)
			}
			fp, err := Fingerprint(f.root, cfg.ApprovalTools)
			if err != nil {
				t.Fatal(err)
			}
			approve(t, e.trust, f.root, cfg.ApprovalTools, fp, "session")
			info, err := e.mgr.Attach(context.Background(), f.root, cfg)
			if err != nil {
				t.Fatal(err)
			}
			e.next()
			if mutation == "lock" {
				f.put(t, "Cargo.lock", "version=3\n# changed\n")
			} else {
				f.put(t, ".cargo/config.toml", "[net]\noffline=true\n")
			}
			if err := cfg.ValidateStart(); err == nil {
				t.Fatal("input drift not detected")
			}
			if e.mgr.CurrentGeneration(f.root, cfg, info.Generation) {
				t.Fatal("changed Rust inputs allowed publication")
			}
			if err := docs.ValidateDocument(f.root, "src/main.rs", DocumentOwnerIdentity{}, "fn main() {}"); err == nil {
				t.Fatal("changed Rust inputs allowed document status")
			}
		})
	}
}
