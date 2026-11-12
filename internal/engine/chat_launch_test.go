package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/codexbridge"
	"github.com/ginkcode/agent-sessions/internal/launch"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/claude"
	"github.com/ginkcode/agent-sessions/internal/provider/codex"
	"github.com/ginkcode/agent-sessions/internal/provider/opencode"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

// acpFake is a fake provider with an ACP adapter.
type acpFake struct {
	*providertest.Fake
	adapter provider.ACPAdapter
}

func (f acpFake) ACPAdapter() provider.ACPAdapter { return f.adapter }

var fakeAdapter = provider.ACPAdapter{Executable: "fake-acp", CLIEnv: "FAKE_CLI"}

// chatService has a chat-capable Claude fake, a Codex fake without an
// adapter, and one known project directory.
func chatService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	svc := NewService(nil, provider.Set{
		acpFake{providertest.NewFake(model.AgentClaude, "Claude Code"), fakeAdapter},
		providertest.NewFake(model.AgentCodex, "Codex"),
	})
	svc.Catalog().Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "d"}, CWD: dir}}})
	return svc, dir
}

// stubFind answers lookups from found (name → executable); other names are
// not installed.
func stubFind(t *testing.T, found map[string]launch.Executable) *[]string {
	t.Helper()
	var looked []string
	findAgent = func(name string) (launch.Executable, error) {
		looked = append(looked, name)
		if exe, ok := found[name]; ok {
			return exe, nil
		}
		return launch.Executable{}, fmt.Errorf("%w: %s was not found on PATH", launch.ErrAgentNotFound, name)
	}
	t.Cleanup(func() { findAgent = launch.Find })
	return &looked
}

func executable(t *testing.T, path string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func envValue(env []string, key string) (string, bool) {
	for _, kv := range slices.Backward(env) {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func TestProvidersDeclareACPAdapters(t *testing.T) {
	for _, tc := range []struct {
		prov provider.Provider
		want provider.ACPAdapter
	}{
		{claude.New("", nil), provider.ACPAdapter{Executable: "claude-agent-acp", CLIEnv: "CLAUDE_CODE_EXECUTABLE", Managed: "claude-agent-acp"}},
		{codex.New("", nil), provider.ACPAdapter{Executable: "codex-acp", CLIEnv: "CODEX_PATH", Managed: "codex-acp", DaemonBridge: true}},
		{opencode.New("", nil), provider.ACPAdapter{Executable: "opencode", Args: []string{"acp"}}},
	} {
		l, ok := tc.prov.(provider.ACPLauncher)
		if !ok {
			t.Fatalf("%s has no ACP adapter", tc.prov.ID())
		}
		if got := l.ACPAdapter(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s adapter = %+v, want %+v", tc.prov.ID(), got, tc.want)
		}
	}
}

func TestChatAgents(t *testing.T) {
	svc, _ := chatService(t)
	looked := stubFind(t, nil)
	got := svc.ChatAgents(nil, nil)
	want := []ChatAgent{{
		Agent: model.AgentClaude, Name: "Claude Code",
		Problem: "fake-acp was not found on PATH or in its usual install locations",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing: %+v", got)
	}
	// Detection only looks the adapter up; the CLI is not needed for it.
	if !reflect.DeepEqual(*looked, []string{"fake-acp"}) {
		t.Fatalf("looked up %q", *looked)
	}

	stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	if got := svc.ChatAgents(nil, nil); len(got) != 1 || !got[0].Available || got[0].Path != "/opt/bin/fake-acp" || got[0].Problem != "" {
		t.Fatalf("found: %+v", got)
	}
}

func TestChatAgentsKeepsWSLHint(t *testing.T) {
	svc, _ := chatService(t)
	findAgent = func(name string) (launch.Executable, error) {
		return launch.Executable{}, fmt.Errorf("%w: /mnt/c/npm/%s is a Windows program; install %s inside the WSL distribution", launch.ErrAgentNotFound, name, name)
	}
	t.Cleanup(func() { findAgent = launch.Find })
	if got := svc.ChatAgents(nil, nil); !strings.Contains(got[0].Problem, "is a Windows program") {
		t.Fatalf("problem = %q", got[0].Problem)
	}
}

func TestChatAgentOverrides(t *testing.T) {
	platform.SkipWithoutModeBits(t)
	svc, _ := chatService(t)
	looked := stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	tmp := t.TempDir()
	good := executable(t, filepath.Join(tmp, "bin", "my-acp"), 0o755)
	plain := executable(t, filepath.Join(tmp, "plain"), 0o644)

	for _, tc := range []struct {
		name, path, problem string
	}{
		{"absolute", good, ""},
		{"missing", filepath.Join(tmp, "nope"), "does not exist"},
		{"relative", "bin/my-acp", "is not absolute"},
		{"directory", tmp, "is not a file"},
		{"not executable", plain, "is not executable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			*looked = nil
			got := svc.ChatAgents(ChatAdapters{model.AgentClaude: tc.path}, nil)[0]
			if !got.Override {
				t.Fatalf("override not reported: %+v", got)
			}
			// The override wins over PATH, even when it is unusable.
			if len(*looked) != 0 {
				t.Fatalf("looked up %q despite the override", *looked)
			}
			if tc.problem == "" {
				if !got.Available || got.Path != good {
					t.Fatalf("got %+v", got)
				}
				return
			}
			if got.Available || !strings.Contains(got.Problem, tc.problem) {
				t.Fatalf("got %+v, want problem %q", got, tc.problem)
			}
		})
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	inHome := executable(t, filepath.Join(home, "acp", "fake-acp"), 0o755)
	if got := svc.ChatAgents(ChatAdapters{model.AgentClaude: "~/acp/fake-acp"}, nil)[0]; got.Path != inHome {
		t.Fatalf("~ override: %+v", got)
	}
}

func TestChatLaunch(t *testing.T) {
	svc, dir := chatService(t)
	stubFind(t, map[string]launch.Executable{
		"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true},
		"fake":     {Path: "/usr/bin/fake", OnPath: true},
	})
	t.Setenv("FAKE_CLI", "")
	cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cmd.Argv, []string{"/opt/bin/fake-acp"}) || cmd.Dir != dir {
		t.Fatalf("cmd = %+v", cmd)
	}
	// The adapter drives the CLI new sessions would start.
	if v, _ := envValue(cmd.Env, "FAKE_CLI"); v != "/usr/bin/fake" {
		t.Fatalf("env = %q", cmd.Env)
	}
	// Both are on PATH, so PATH is left alone.
	if _, ok := envValue(cmd.Env, "PATH"); ok {
		t.Fatalf("PATH changed: %q", cmd.Env)
	}

	// The user's own setting wins.
	t.Setenv("FAKE_CLI", "/custom/fake")
	if cmd, _ := svc.ChatLaunch(model.AgentClaude, dir, nil, nil); len(cmd.Env) != 0 {
		t.Fatalf("env = %q", cmd.Env)
	}
}

// Codex's adapter starts this program as its codex, which bridges to the
// Codex daemon and runs the real codex named by AGENT_SESSIONS_CODEX.
func TestChatLaunchDaemonBridge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Codex daemon has no Unix socket on Windows; TestChatLaunch covers the launch")
	}
	dir := t.TempDir()
	bridged := fakeAdapter
	bridged.DaemonBridge = true
	svc := NewService(nil, provider.Set{acpFake{providertest.NewFake(model.AgentClaude, "Claude Code"), bridged}})
	svc.Catalog().Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "d"}, CWD: dir}}})
	stubFind(t, map[string]launch.Executable{
		"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true},
		"fake":     {Path: "/usr/bin/fake", OnPath: true},
	})
	selfExecutable = func() (string, error) { return "/app/agent-sessions", nil }
	t.Cleanup(func() { selfExecutable = os.Executable })
	launchEnv := func() []string {
		t.Helper()
		cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return cmd.Env
	}
	check := func(name, cli, real string) {
		t.Helper()
		env := launchEnv()
		gotCLI, _ := envValue(env, "FAKE_CLI")
		gotReal, _ := envValue(env, codexbridge.EnvCodex)
		if gotCLI != cli || gotReal != real {
			t.Fatalf("%s: env = %q, want FAKE_CLI=%q and %s=%q", name, env, cli, codexbridge.EnvCodex, real)
		}
	}

	t.Setenv("FAKE_CLI", "")
	t.Setenv(codexbridge.EnvDaemon, "")
	check("found codex", "/app/agent-sessions", "/usr/bin/fake")
	t.Setenv("FAKE_CLI", "/custom/fake")
	check("user's codex", "/app/agent-sessions", "/custom/fake")

	t.Setenv(codexbridge.EnvDaemon, "off")
	check("turned off", "", "")
	t.Setenv("FAKE_CLI", "")
	check("turned off, found codex", "/usr/bin/fake", "")
	t.Setenv(codexbridge.EnvDaemon, "")

	// Nothing to bridge to: the adapter finds codex its own way.
	stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	check("no codex", "", "")
	selfExecutable = func() (string, error) { return "", errors.New("gone") }
	stubFind(t, map[string]launch.Executable{
		"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true},
		"fake":     {Path: "/usr/bin/fake", OnPath: true},
	})
	check("no self", "/usr/bin/fake", "")
}

func TestChatLaunchPrependsInstallDirs(t *testing.T) {
	svc, dir := chatService(t)
	nvm := filepath.FromSlash("/home/me/.nvm/versions/node/v22/bin")
	stubFind(t, map[string]launch.Executable{
		"fake-acp": {Path: filepath.Join(nvm, "fake-acp")},
		"fake":     {Path: filepath.Join("/home/me/.local/bin", "fake")},
	})
	t.Setenv("FAKE_CLI", "")
	t.Setenv("PATH", "/usr/bin")
	cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sep := string(os.PathListSeparator)
	want := nvm + sep + filepath.FromSlash("/home/me/.local/bin") + sep + "/usr/bin"
	if v, _ := envValue(cmd.Env, "PATH"); v != want {
		t.Fatalf("PATH = %q, want %q", v, want)
	}
}

// The adapter starts in the login shell's environment, as a terminal tab's
// agent would: a version manager's PATH from ~/.bashrc reaches it.
func TestChatLaunchLoginEnv(t *testing.T) {
	svc, dir := chatService(t)
	sep := string(os.PathListSeparator)
	join := func(dirs ...string) string {
		for i, d := range dirs {
			dirs[i] = filepath.FromSlash(d)
		}
		return strings.Join(dirs, sep)
	}
	mise := "/home/me/.local/share/mise/installs/node/22/bin"
	t.Setenv("PATH", join("/usr/bin", "/proc/only"))
	t.Setenv("KEEP", "1")
	t.Setenv("FAKE_CLI", "")
	login := []string{"PATH=" + join(mise, "/usr/bin"), "AS_TEST_LOGIN_ONLY=bash", "KEEP=1", "junk", "=x"}
	chatLoginEnv = func() []string { return slices.Clone(login) }
	t.Cleanup(func() { chatLoginEnv = os.Environ })
	nvm := filepath.FromSlash("/home/me/.nvm/versions/node/v22/bin")
	stubFind(t, map[string]launch.Executable{
		"fake-acp": {Path: filepath.Join(nvm, "fake-acp")},
		"fake":     {Path: filepath.FromSlash("/usr/bin/fake"), OnPath: true},
	})
	launchEnv := func() []string {
		t.Helper()
		cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(slices.DeleteFunc(slices.Clone(cmd.Env), func(kv string) bool { return !strings.HasPrefix(kv, "PATH=") })); n != 1 {
			t.Fatalf("%d PATH entries: %q", n, cmd.Env)
		}
		return cmd.Env
	}

	// The adapter's directory, then the login PATH, then what only this
	// process has.
	env := launchEnv()
	if v, _ := envValue(env, "PATH"); v != join(nvm, mise, "/usr/bin", "/proc/only") {
		t.Fatalf("PATH = %q", v)
	}
	if v, _ := envValue(env, "AS_TEST_LOGIN_ONLY"); v != "bash" {
		t.Fatalf("env = %q", env)
	}
	// Unchanged variables are not repeated; malformed entries are dropped.
	for _, kv := range env {
		if strings.HasPrefix(kv, "KEEP=") || kv == "junk" || kv == "=x" {
			t.Fatalf("env = %q", env)
		}
	}
	if v, _ := envValue(env, "FAKE_CLI"); v != filepath.FromSlash("/usr/bin/fake") {
		t.Fatalf("env = %q", env)
	}

	// A CLI path exported in the startup files is the user's own setting.
	login = append(login, "FAKE_CLI=/login/fake")
	looked := stubFind(t, map[string]launch.Executable{"fake-acp": {Path: filepath.FromSlash("/usr/bin/fake-acp"), OnPath: true}})
	env = launchEnv()
	if v, _ := envValue(env, "FAKE_CLI"); v != "/login/fake" || slices.Contains(*looked, "fake") {
		t.Fatalf("env = %q, looked up %q", env, *looked)
	}
	// No adapter directory: the login PATH still replaces this process's.
	if v, _ := envValue(env, "PATH"); v != join(mise, "/usr/bin", "/proc/only") {
		t.Fatalf("PATH = %q", v)
	}
}

func TestEnvHelpers(t *testing.T) {
	over := envOverlay([]string{"A=1", "B=2", "B=3"}, []string{"A=1", "B=2", "C=", "D"})
	if !slices.Equal(over, []string{"B=2", "C="}) {
		t.Fatalf("overlay = %q", over)
	}
	if v := envLookup([]string{"A=1", "A=2"}, "A"); v != "2" {
		t.Fatal(v)
	}
	if got := envSet([]string{"PATH=a", "X=1", "PATH=b"}, "PATH", "c"); !slices.Equal(got, []string{"X=1", "PATH=c"}) {
		t.Fatal(got)
	}
	if runtime.GOOS == "windows" {
		if v := envLookup([]string{"Path=w"}, "PATH"); v != "w" {
			t.Fatal(v)
		}
		if got := envSet([]string{"Path=w"}, "PATH", "c"); !slices.Equal(got, []string{"PATH=c"}) {
			t.Fatal(got)
		}
	}
}

func TestChatLaunchRefusals(t *testing.T) {
	svc, dir := chatService(t)
	stubFind(t, nil)
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil); !errors.Is(err, ErrAdapterNotFound) || !strings.Contains(err.Error(), "fake-acp was not found on PATH or in its usual install locations. Set it up in Settings › Coding agents") {
		t.Fatalf("missing adapter: %v", err)
	}
	stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	if _, err := svc.ChatLaunch(model.AgentClaude, filepath.Dir(dir), nil, nil); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("unknown directory: %v", err)
	}
	if _, err := svc.ChatLaunch(model.AgentCodex, dir, nil, nil); !errors.Is(err, ErrChatUnsupported) {
		t.Fatalf("no adapter: %v", err)
	}
	if _, err := svc.ChatLaunch(model.AgentOpenCode, dir, nil, nil); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, nil, nil); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("removed directory: %v", err)
	}
	// An unusable override is reported, not replaced by the lookup.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, ChatAdapters{model.AgentClaude: "/nope/fake-acp"}, nil); !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("bad override: %v", err)
	}
}

func TestDecodeChatAdapters(t *testing.T) {
	got, err := decodeChatAdapters([]byte(`
[agents]
enabled = ["claude-code"]

[chat.adapters]
claude-code = "/opt/claude-agent-acp"
codex = "  "
`))
	if err != nil || !reflect.DeepEqual(got, ChatAdapters{model.AgentClaude: "/opt/claude-agent-acp"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := decodeChatAdapters([]byte("[chat.adapters]\ncursor = \"/x\"\n")); err == nil || !strings.Contains(err.Error(), `unknown agent "cursor"`) {
		t.Fatalf("unknown agent: %v", err)
	}
	if _, err := decodeChatAdapters([]byte("[chat.adapters\n")); err == nil {
		t.Fatal("malformed config accepted")
	}
	if got, err := decodeChatAdapters(nil); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v, %v", got, err)
	}
}

// The engine reads the overrides from config.toml, and fake executables on a
// temporary PATH are found by the real lookup.
func TestEngineChatAgentsReadsConfig(t *testing.T) {
	platform.SkipWithoutModeBits(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executables")
	}
	cfg := t.TempDir()
	svc, dir := chatService(t)
	eng := NewEngine(WithService(svc), WithRoots(paths.Roots{Config: cfg}))
	ctx := context.Background()

	bin := t.TempDir()
	onPath := executable(t, filepath.Join(bin, "fake-acp"), 0o755)
	t.Setenv("PATH", bin)
	t.Setenv("HOME", t.TempDir())
	got, err := eng.ChatAgents(ctx)
	if err != nil || len(got) != 1 || got[0].Path != onPath || got[0].Override {
		t.Fatalf("PATH lookup: %+v, %v", got, err)
	}

	override := executable(t, filepath.Join(t.TempDir(), "fake-acp"), 0o755)
	if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("[chat.adapters]\nclaude-code = \""+override+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = eng.ChatAgents(ctx)
	if err != nil || got[0].Path != override || !got[0].Override {
		t.Fatalf("override: %+v, %v", got, err)
	}
	cmd, err := eng.ChatLaunch(ctx, model.AgentClaude, dir)
	if err != nil || cmd.Argv[0] != override {
		t.Fatalf("launch: %+v, %v", cmd, err)
	}

	if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("[chat.adapters\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.ChatAgents(ctx); err == nil {
		t.Fatal("malformed config accepted")
	}
	if _, err := eng.ChatLaunch(ctx, model.AgentClaude, dir); err == nil {
		t.Fatal("launch with a malformed config")
	}
}

// fakeManaged is a managed-tools view with one adapter install, or none.
// The install is the pinned version 1.2.0 unless version says otherwise.
type fakeManaged struct {
	main, node, version string
	err, nodeErr        error
	looked, nodes       []string
}

func (f *fakeManaged) Adapter(tool string) (string, string, error) {
	f.looked = append(f.looked, tool)
	if f.main == "" && f.err == nil {
		return "", "", os.ErrNotExist
	}
	if f.version == "" {
		return f.main, "1.2.0", f.err
	}
	return f.main, f.version, f.err
}

func (f *fakeManaged) Pinned(string) string { return "1.2.0" }

func (f *fakeManaged) Runtime(tool string) (string, error) {
	f.nodes = append(f.nodes, tool)
	return f.node, f.nodeErr
}

// managedChatService is chatService with an adapter the app can install as
// the catalog entry id.
func managedChatService(t *testing.T, id string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	a := fakeAdapter
	a.Managed = id
	svc := NewService(nil, provider.Set{acpFake{providertest.NewFake(model.AgentClaude, "Claude Code"), a}})
	svc.Catalog().Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "d"}, CWD: dir}}})
	return svc, dir
}

func TestChatAgentsManagedAdapter(t *testing.T) {
	svc, _ := managedChatService(t, "fake-managed")
	stubFind(t, nil)
	main := filepath.FromSlash("/data/managed-tools/fake-managed-1/node_modules/fake/dist/index.js")

	// No managed installs on this host: nothing to offer.
	if got := svc.ChatAgents(nil, nil)[0]; got.InstallTool != "" || got.Available || got.Managed {
		t.Fatalf("no managed tools: %+v", got)
	}
	// Installable but not installed: says so.
	none := &fakeManaged{}
	got := svc.ChatAgents(nil, none)[0]
	if got.InstallTool != "fake-managed" || got.Available || got.Managed || got.Problem != "fake-acp is not installed" {
		t.Fatalf("not installed: %+v", got)
	}
	// Installed: available as the app's copy, without running anything.
	installed := &fakeManaged{main: main, node: "/data/managed-tools/node/bin/node"}
	got = svc.ChatAgents(nil, installed)[0]
	if !got.Available || !got.Managed || got.Path != main || got.InstallTool != "fake-managed" || got.Problem != "" || got.Version != "1.2.0" || got.Pinned != "1.2.0" || got.Outdated {
		t.Fatalf("installed: %+v", got)
	}
	// An older copy runs, and says it is outdated.
	got = svc.ChatAgents(nil, &fakeManaged{main: main, node: "/data/managed-tools/node/bin/node", version: "1.1.9"})[0]
	if !got.Available || !got.Managed || got.Version != "1.1.9" || got.Pinned != "1.2.0" || !got.Outdated {
		t.Fatalf("older copy: %+v", got)
	}
	if got := svc.ChatAgents(nil, none)[0]; got.Pinned != "1.2.0" || got.Version != "" || got.Outdated {
		t.Fatalf("pinned when missing: %+v", got)
	}
	// A damaged install, or one without a Node.js, is reported as such.
	if got := svc.ChatAgents(nil, &fakeManaged{err: errors.New("invalid receipt")})[0]; got.Available || !strings.Contains(got.Problem, "is damaged; install it again") {
		t.Fatalf("damaged: %+v", got)
	}
	if got := svc.ChatAgents(nil, &fakeManaged{main: main, nodeErr: os.ErrNotExist})[0]; got.Available || !strings.Contains(got.Problem, "has no Node.js") {
		t.Fatalf("no node: %+v", got)
	}

	// The app's copy runs before one on PATH, and without it one on PATH is
	// not used: chats run a version this app was tested with.
	looked := stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	if got := svc.ChatAgents(nil, installed)[0]; !got.Available || !got.Managed || got.Path != main {
		t.Fatalf("app's copy first: %+v", got)
	}
	if got := svc.ChatAgents(nil, none)[0]; got.Available || got.Problem != "fake-acp is not installed" || len(*looked) != 0 {
		t.Fatalf("PATH used: %+v, looked %q", got, *looked)
	}
	// Where the app cannot install, PATH is searched as before.
	if got := svc.ChatAgents(nil, nil)[0]; !got.Available || got.Managed || got.Path != "/opt/bin/fake-acp" || got.Pinned != "" || got.Version != "" {
		t.Fatalf("no managed tools: %+v", got)
	}
	// An override wins over the app's copy, even an unusable one.
	installed.looked = nil
	stubFind(t, nil)
	missing := filepath.Join(t.TempDir(), "nope", "fake-acp")
	if got := svc.ChatAgents(ChatAdapters{model.AgentClaude: missing}, installed)[0]; got.Available || got.Managed || !strings.Contains(got.Problem, "does not exist") || len(installed.looked) != 0 {
		t.Fatalf("override wins: %+v, looked %q", got, installed.looked)
	}

	// An adapter the app cannot install (OpenCode's) offers nothing.
	svc2, _ := chatService(t)
	if got := svc2.ChatAgents(nil, none)[0]; got.InstallTool != "" || got.Managed {
		t.Fatalf("no catalog entry: %+v", got)
	}
}

func TestChatLaunchManagedAdapter(t *testing.T) {
	svc, dir := managedChatService(t, "fake-managed")
	stubFind(t, map[string]launch.Executable{"fake": {Path: "/usr/bin/fake", OnPath: true}})
	t.Setenv("FAKE_CLI", "")
	t.Setenv("PATH", "/usr/bin")
	nodeBin := filepath.FromSlash("/data/managed-tools/node/bin")
	main := filepath.FromSlash("/data/managed-tools/fake-managed-1/node_modules/fake/dist/index.js")
	m := &fakeManaged{main: main, node: filepath.Join(nodeBin, "node")}
	cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cmd.Argv, []string{filepath.Join(nodeBin, "node"), main}) || cmd.Dir != dir {
		t.Fatalf("cmd = %+v", cmd)
	}
	if !reflect.DeepEqual(m.looked, []string{"fake-managed"}) || !reflect.DeepEqual(m.nodes, []string{"fake-managed"}) {
		t.Fatalf("looked up %q, %q", m.looked, m.nodes)
	}
	// The adapter's own node calls find the same Node.js, and it drives the
	// user's CLI as before.
	if v, _ := envValue(cmd.Env, "PATH"); v != nodeBin+string(os.PathListSeparator)+"/usr/bin" {
		t.Fatalf("PATH = %q", v)
	}
	if v, _ := envValue(cmd.Env, "FAKE_CLI"); v != "/usr/bin/fake" {
		t.Fatalf("env = %q", cmd.Env)
	}
	// An older copy runs until the user updates it.
	old := filepath.FromSlash("/data/managed-tools/fake-managed-0/node_modules/fake/dist/index.js")
	cmd, err = svc.ChatLaunch(model.AgentClaude, dir, nil, &fakeManaged{main: old, node: filepath.Join(nodeBin, "node"), version: "1.1.9"})
	if err != nil || !reflect.DeepEqual(cmd.Argv, []string{filepath.Join(nodeBin, "node"), old}) {
		t.Fatalf("older copy: %+v, %v", cmd, err)
	}
	// Not installed: refused, pointing to the settings, even with a copy on
	// PATH.
	stubFind(t, map[string]launch.Executable{"fake": {Path: "/usr/bin/fake", OnPath: true}, "fake-acp": {Path: "/usr/bin/fake-acp", OnPath: true}})
	_, err = svc.ChatLaunch(model.AgentClaude, dir, nil, &fakeManaged{})
	if !errors.Is(err, ErrAdapterNotFound) || !strings.Contains(err.Error(), "fake-acp is not installed. Set it up in Settings › Coding agents") || strings.Contains(err.Error(), "npm") {
		t.Fatalf("missing: %v", err)
	}
}

// The engine offers managed installs where the installer runs and reads
// them from its own app data, writing nothing.
func TestEngineChatAgentsOffersManagedInstall(t *testing.T) {
	svc, _ := managedChatService(t, "claude-agent-acp")
	stubFind(t, nil)
	cfg, data := t.TempDir(), t.TempDir()
	eng := NewEngine(WithService(svc), WithRoots(paths.Roots{Config: cfg, Data: data}))
	got, err := eng.ChatAgents(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	want := "claude-agent-acp"
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		want = ""
	}
	problem := "fake-acp is not installed"
	if want == "" {
		problem = "fake-acp was not found on PATH"
	}
	if got[0].InstallTool != want || got[0].Available || got[0].Managed || !strings.Contains(got[0].Problem, problem) || (got[0].Pinned != "") != (want != "") {
		t.Fatalf("got %+v", got[0])
	}
	for _, dir := range []string{cfg, data} {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("%s written: %v", dir, entries)
		}
	}
}
