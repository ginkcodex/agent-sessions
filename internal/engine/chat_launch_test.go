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

var fakeAdapter = provider.ACPAdapter{Executable: "fake-acp", Install: "npm install -g fake-acp", CLIEnv: "FAKE_CLI"}

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
		{claude.New("", nil), provider.ACPAdapter{Executable: "claude-agent-acp", Install: "npm install -g @agentclientprotocol/claude-agent-acp", CLIEnv: "CLAUDE_CODE_EXECUTABLE"}},
		{codex.New("", nil), provider.ACPAdapter{Executable: "codex-acp", Install: "npm install -g @agentclientprotocol/codex-acp", CLIEnv: "CODEX_PATH"}},
		{opencode.New("", nil), provider.ACPAdapter{Executable: "opencode", Args: []string{"acp"}, Install: "npm install -g opencode-ai"}},
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
	got := svc.ChatAgents(nil)
	want := []ChatAgent{{
		Agent: model.AgentClaude, Name: "Claude Code", Install: "npm install -g fake-acp",
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
	if got := svc.ChatAgents(nil); len(got) != 1 || !got[0].Available || got[0].Path != "/opt/bin/fake-acp" || got[0].Problem != "" {
		t.Fatalf("found: %+v", got)
	}
}

func TestChatAgentsKeepsWSLHint(t *testing.T) {
	svc, _ := chatService(t)
	findAgent = func(name string) (launch.Executable, error) {
		return launch.Executable{}, fmt.Errorf("%w: /mnt/c/npm/%s is a Windows program; install %s inside the WSL distribution", launch.ErrAgentNotFound, name, name)
	}
	t.Cleanup(func() { findAgent = launch.Find })
	if got := svc.ChatAgents(nil); !strings.Contains(got[0].Problem, "is a Windows program") {
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
			got := svc.ChatAgents(ChatAdapters{model.AgentClaude: tc.path})[0]
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
	if got := svc.ChatAgents(ChatAdapters{model.AgentClaude: "~/acp/fake-acp"})[0]; got.Path != inHome {
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
	cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil)
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
	if cmd, _ := svc.ChatLaunch(model.AgentClaude, dir, nil); len(cmd.Env) != 0 {
		t.Fatalf("env = %q", cmd.Env)
	}
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
	cmd, err := svc.ChatLaunch(model.AgentClaude, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	sep := string(os.PathListSeparator)
	want := nvm + sep + filepath.FromSlash("/home/me/.local/bin") + sep + "/usr/bin"
	if v, _ := envValue(cmd.Env, "PATH"); v != want {
		t.Fatalf("PATH = %q, want %q", v, want)
	}
}

func TestChatLaunchRefusals(t *testing.T) {
	svc, dir := chatService(t)
	stubFind(t, nil)
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, nil); !errors.Is(err, ErrAdapterNotFound) || !strings.Contains(err.Error(), "npm install -g fake-acp") {
		t.Fatalf("missing adapter: %v", err)
	}
	stubFind(t, map[string]launch.Executable{"fake-acp": {Path: "/opt/bin/fake-acp", OnPath: true}})
	if _, err := svc.ChatLaunch(model.AgentClaude, filepath.Dir(dir), nil); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("unknown directory: %v", err)
	}
	if _, err := svc.ChatLaunch(model.AgentCodex, dir, nil); !errors.Is(err, ErrChatUnsupported) {
		t.Fatalf("no adapter: %v", err)
	}
	if _, err := svc.ChatLaunch(model.AgentOpenCode, dir, nil); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, nil); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("removed directory: %v", err)
	}
	// An unusable override is reported, not replaced by the lookup.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChatLaunch(model.AgentClaude, dir, ChatAdapters{model.AgentClaude: "/nope/fake-acp"}); !errors.Is(err, ErrAdapterNotFound) {
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
