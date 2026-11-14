package engine

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/codexbridge"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// pluginFake is an ACP fake whose CLI lists plugins as "name,name".
type pluginFake struct {
	acpFake
	timeout time.Duration
}

func (f pluginFake) PluginList() provider.PluginList {
	return provider.PluginList{Args: []string{"plugin", "list", "--json"}, Timeout: f.timeout, Parse: func(b []byte) ([]provider.Plugin, error) {
		if !strings.HasPrefix(string(b), "ok:") {
			return nil, errors.New("unknown output")
		}
		var out []provider.Plugin
		for n := range strings.SplitSeq(strings.TrimPrefix(string(b), "ok:"), ",") {
			out = append(out, provider.Plugin{ID: n, Name: n, Enabled: true})
		}
		return out, nil
	}}
}

type pluginRun struct {
	argv []string
	dir  string
	env  []string
}

// stubPluginCLI answers every listing with run, and records the calls.
func stubPluginCLI(t *testing.T, run func(ctx context.Context) ([]byte, error)) *[]pluginRun {
	t.Helper()
	var calls []pluginRun
	prev := runPluginCLI
	t.Cleanup(func() { runPluginCLI = prev })
	runPluginCLI = func(ctx context.Context, argv []string, dir string, env []string) ([]byte, error) {
		calls = append(calls, pluginRun{argv, dir, env})
		return run(ctx)
	}
	return &calls
}

func TestServiceChatPlugins(t *testing.T) {
	bridge := provider.ACPAdapter{Executable: "fake-acp", CLIEnv: "FAKE_CLI", DaemonBridge: true}
	svc := NewService(nil, provider.Set{
		pluginFake{acpFake{providertest.NewFake(model.AgentClaude, "Claude Code"), fakeAdapter}, time.Minute},
		pluginFake{acpFake{providertest.NewFake(model.AgentCodex, "Codex"), bridge}, 50 * time.Millisecond},
		acpFake{providertest.NewFake(model.AgentOpenCode, "OpenCode"), fakeAdapter},
	})
	ctx := context.Background()
	launch := acp.Command{Dir: "/work/a", Env: []string{"FAKE_CLI=/opt/claude", "PATH=/login/bin"}}

	answer := func(out string, err error) func(context.Context) ([]byte, error) {
		return func(context.Context) ([]byte, error) { return []byte(out), err }
	}
	calls := stubPluginCLI(t, answer("ok:ponytail,gopls-lsp", nil))
	got, err := svc.ChatPlugins(ctx, model.AgentClaude, launch)
	if err != nil || !got.Supported || len(got.Plugins) != 2 || got.Plugins[1].Name != "gopls-lsp" {
		t.Fatalf("ChatPlugins = %+v, %v", got, err)
	}
	// The adapter's CLI, fixed argv, the chat's directory and environment.
	c := (*calls)[0]
	if !slices.Equal(c.argv, []string{"/opt/claude", "plugin", "list", "--json"}) || c.dir != "/work/a" || !slices.Equal(c.env, launch.Env) {
		t.Fatalf("ran %+v", c)
	}

	// The bridge's codex, not the bridge; without a CLI variable, the CLI new sessions start.
	_, _ = svc.ChatPlugins(ctx, model.AgentCodex, acp.Command{Dir: "/work/b", Env: []string{"FAKE_CLI=/self", codexbridge.EnvCodex + "=/opt/codex"}})
	_, _ = svc.ChatPlugins(ctx, model.AgentClaude, acp.Command{Dir: "/work/c"})
	if a, b := (*calls)[1], (*calls)[2]; a.argv[0] != "/opt/codex" || a.dir != "/work/b" || b.argv[0] != "fake" {
		t.Fatalf("ran %+v and %+v", a, b)
	}

	// An agent without a listing is unsupported, and nothing runs.
	got, err = svc.ChatPlugins(ctx, model.AgentOpenCode, launch)
	if err != nil || got.Supported || got.Plugins == nil || len(*calls) != 3 {
		t.Fatalf("OpenCode = %+v, %v after %d runs", got, err, len(*calls))
	}

	for _, tc := range []struct {
		name  string
		agent model.AgentID
		run   func(context.Context) ([]byte, error)
		want  string
	}{
		{"failed", model.AgentClaude, answer("", errors.New("claude exited with status 1: boom")), "could not list Claude Code plugins: claude exited with status 1: boom"},
		{"older CLI", model.AgentClaude, answer("error: unknown option '--json'", nil), "this Claude Code version does not list its plugins in a form the app reads; update it"},
		{"timeout", model.AgentCodex, func(ctx context.Context) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }, "listing Codex plugins took longer than 50ms"},
	} {
		stubPluginCLI(t, tc.run)
		got, err := svc.ChatPlugins(ctx, tc.agent, launch)
		if err == nil || err.Error() != tc.want || got.Supported || got.Plugins != nil {
			t.Errorf("%s: ChatPlugins = %+v, %v; want error %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := svc.ChatPlugins(ctx, "nope", launch); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown agent: %v", err)
	}
}

func TestChatManagerPlugins(t *testing.T) {
	f := newChatFixture(t)
	type call struct {
		agent model.AgentID
		dir   string
	}
	calls := make(chan call, 4)
	block := make(chan struct{})
	f.m.opts.Plugins = func(ctx context.Context, agent model.AgentID, launch acp.Command) (ChatPlugins, error) {
		calls <- call{agent, launch.Dir}
		select {
		case <-block:
			return ChatPlugins{Supported: true, Plugins: []provider.Plugin{{ID: "p@m", Name: "p"}}}, nil
		case <-ctx.Done():
			return ChatPlugins{}, ctx.Err()
		}
	}
	if _, err := f.m.Plugins(chatCtx(t), "chat-404"); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("unknown chat: %v", err)
	}

	st := f.start(t, nil)
	close(block)
	got, err := f.m.Plugins(chatCtx(t), st.ChatID)
	if err != nil || !got.Supported || got.Plugins[0].Name != "p" {
		t.Fatalf("Plugins = %+v, %v", got, err)
	}
	if c := <-calls; c.agent != model.AgentClaude || c.dir != f.dir {
		t.Fatalf("listed %+v, want the chat's agent and directory", c)
	}

	// Closing the chat stops a listing in flight.
	block = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := f.m.Plugins(chatCtx(t), st.ChatID); done <- err }()
	<-calls
	if err := f.m.Close(st.ChatID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrChatClosed) {
			t.Fatalf("listing after close: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the listing did not stop when the chat closed")
	}
}
