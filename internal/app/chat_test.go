package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
	"github.com/ginkcode/agent-sessions/internal/remote"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// chatLog records the chat events an engine emits.
type chatLog struct {
	mu      sync.Mutex
	updates []engine.ChatUpdate
	exits   []engine.ChatExit
}

func (l *chatLog) Emit(name string, payload any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch name {
	case engine.ChatUpdateEvent:
		l.updates = append(l.updates, payload.(engine.ChatUpdate))
	case engine.ChatExitEvent:
		l.exits = append(l.exits, payload.(engine.ChatExit))
	}
}

// waitFor polls until ok holds for the log.
func (l *chatLog) waitFor(t *testing.T, what string, ok func([]engine.ChatUpdate, []engine.ChatExit) bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		l.mu.Lock()
		done := ok(l.updates, l.exits)
		l.mu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *chatLog) waitTurnEnd(t *testing.T, chatID string) engine.ChatUpdate {
	t.Helper()
	var end engine.ChatUpdate
	l.waitFor(t, "turn end", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		i := slices.IndexFunc(u, func(u engine.ChatUpdate) bool { return u.ChatID == chatID && u.Kind == engine.ChatTurnEnded })
		if i >= 0 {
			end = u[i]
		}
		return i >= 0
	})
	return end
}

func (l *chatLog) waitExit(t *testing.T, chatID string) {
	t.Helper()
	l.waitFor(t, "chat exit", func(_ []engine.ChatUpdate, x []engine.ChatExit) bool {
		return slices.ContainsFunc(x, func(x engine.ChatExit) bool { return x.ChatID == chatID })
	})
}

func chatTestService() *Service {
	return NewService(scan.NewCatalog(), provider.Set{providertest.NewFake(model.AgentClaude, "Claude")})
}

// useChatEngine gives a its own local engine whose chats run the fake ACP
// agent, and returns the chat events it emits.
func useChatEngine(t *testing.T, a *App) *chatLog {
	t.Helper()
	log := &chatLog{}
	eng := engine.NewEngine(
		engine.WithService(a.svc),
		engine.WithRunner(a.runner),
		engine.WithCacheEnabled(false),
		engine.WithEmitter(log),
		engine.WithChatLaunch(func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
			return acptest.Command(dir), nil
		}),
	)
	a.backendMu.Lock()
	a.localEngine, a.backend = eng, eng
	a.backendMu.Unlock()
	t.Cleanup(func() { _ = eng.Close() })
	return log
}

func TestChatBindingsLocal(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)

	if agents, err := a.ChatAgents(); err != nil || len(agents) != 0 {
		t.Fatalf("ChatAgents = %+v, %v; the test providers have no adapter", agents, err)
	}
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if st.ChatID == "" || st.Session.ID == "" || st.Session.Agent != model.AgentClaude {
		t.Fatalf("state = %+v", st)
	}
	if got := a.ChatList(); len(got) != 1 || got[0].ChatID != st.ChatID {
		t.Fatalf("ChatList = %+v", got)
	}

	if err := a.ChatPrompt(st.ChatID, "hello"); err != nil {
		t.Fatal(err)
	}
	if end := log.waitTurnEnd(t, st.ChatID); end.StopReason != "end_turn" {
		t.Fatalf("turn end = %+v", end)
	}

	opts, err := a.ChatSetOption(st.ChatID, "mode", "auto")
	if err != nil || len(opts) == 0 {
		t.Fatalf("ChatSetOption = %+v, %v", opts, err)
	}

	// A permission request is answered through ChatRespond.
	if err := a.ChatPrompt(st.ChatID, "permission"); err != nil {
		t.Fatal(err)
	}
	var perm *engine.ChatPermission
	log.waitFor(t, "permission", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		for _, u := range u {
			if u.ChatID == st.ChatID && u.Kind == engine.ChatPermissionAsked {
				perm = u.Permission
				return true
			}
		}
		return false
	})
	if err := a.ChatRespond(st.ChatID, perm.ID, perm.Options[0].OptionID); err != nil {
		t.Fatal(err)
	}
	log.waitFor(t, "second turn end", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		n := 0
		for _, u := range u {
			if u.ChatID == st.ChatID && u.Kind == engine.ChatTurnEnded {
				n++
			}
		}
		return n == 2
	})

	if err := a.ChatPrompt(st.ChatID, "hang"); err != nil {
		t.Fatal(err)
	}
	if err := a.ChatCancel(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if err := a.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	log.waitExit(t, st.ChatID)
	if got := a.ChatList(); len(got) != 0 {
		t.Fatalf("ChatList after close = %+v", got)
	}
	if err := a.ChatPrompt(st.ChatID, "again"); !errors.Is(err, engine.ErrChatNotFound) {
		t.Fatalf("prompt after close: %v", err)
	}
}

// Chats run on this computer only until hosts support them.
func TestChatRefusedForHosts(t *testing.T) {
	stub := &stubRemoteBackend{}
	for name, a := range map[string]*App{"connected": setupRemoteApp(stub), "offline": offlineRemoteApp()} {
		if _, err := a.ChatAgents(); !errors.Is(err, ErrChatRemote) {
			t.Errorf("%s ChatAgents: %v", name, err)
		}
		if _, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: "/p"}); !errors.Is(err, ErrChatRemote) {
			t.Errorf("%s ChatStart: %v", name, err)
		}
		if err := a.ChatPrompt("chat-1", "hi"); !errors.Is(err, ErrChatRemote) {
			t.Errorf("%s ChatPrompt: %v", name, err)
		}
		if _, err := a.ChatSetOption("chat-1", "mode", "plan"); !errors.Is(err, ErrChatRemote) {
			t.Errorf("%s ChatSetOption: %v", name, err)
		}
		// Ending a chat never needs the host.
		if err := a.ChatClose("chat-1"); !errors.Is(err, engine.ErrChatNotFound) {
			t.Errorf("%s ChatClose: %v", name, err)
		}
		if err := a.ChatCancel("chat-1"); !errors.Is(err, engine.ErrChatNotFound) {
			t.Errorf("%s ChatCancel: %v", name, err)
		}
		if err := a.ChatRespond("chat-1", "perm-1", ""); !errors.Is(err, engine.ErrChatNotFound) {
			t.Errorf("%s ChatRespond: %v", name, err)
		}
	}
}

// Closing the window, or quitting, ends its chats' agents and answers
// their permission requests cancelled.
func TestChatEndsWithWindow(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ChatPrompt(st.ChatID, "permission"); err != nil {
		t.Fatal(err)
	}
	log.waitFor(t, "permission", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		return slices.ContainsFunc(u, func(u engine.ChatUpdate) bool { return u.Kind == engine.ChatPermissionAsked })
	})

	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	// close returns once the agent has exited.
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.exits) != 1 || log.exits[0].Reason != engine.ChatExitClosed {
		t.Fatalf("exits = %+v", log.exits)
	}
	resolved := slices.IndexFunc(log.updates, func(u engine.ChatUpdate) bool {
		return u.Kind == engine.ChatPermissionResolved && u.Permission != nil && u.Permission.Outcome == "cancelled"
	})
	if resolved < 0 {
		t.Fatalf("permission not answered cancelled: %+v", log.updates)
	}
	if _, owned := a.localEngine.Chats().Owned(st.Session); owned {
		t.Fatal("session still owned after close")
	}
}

// Connecting to a host ends the window's chats, before the window stops
// showing their events.
func TestChatEndsOnConnect(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	// An alias that is not a host name is refused before anything changes.
	if err := a.Connect("-oProxyCommand=x"); err == nil {
		t.Fatal("bad alias accepted")
	}
	if len(a.ChatList()) != 1 {
		t.Fatal("a refused connect ended the chat")
	}

	a.conn.mu.Lock()
	a.conn.dial = func(context.Context, string, remote.SSHOptions, map[string]string, engine.Emitter) (*remote.Session, error) {
		return nil, errors.New("no hosts in tests")
	}
	a.conn.mu.Unlock()
	t.Cleanup(a.conn.Shutdown)
	_ = a.Connect("example-host")
	log.waitExit(t, st.ChatID)
	if len(a.ChatList()) != 0 {
		t.Fatalf("chats after connect: %+v", a.ChatList())
	}
}

func TestDeleteRefusedWhileChatOwnsSession(t *testing.T) {
	f := newManageTestApp(t, notLive)
	log := useChatEngine(t, f.app)
	if _, err := f.app.SetManageEnabled(true); err != nil {
		t.Fatal(err)
	}
	refs := []model.SessionRef{f.ref}

	st, err := f.app.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir(), Resume: &f.ref})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.app.PreviewDelete(refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || !strings.Contains(p.Items[0].Blocked, "open in a chat") {
		t.Fatalf("owned session not blocked: %+v", p.Items)
	}

	// A preview made before the chat started cannot delete it either.
	if err := f.app.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	log.waitExit(t, st.ChatID)
	p, err = f.app.PreviewDelete(refs)
	if err != nil || len(p.Items) != 1 || p.Items[0].Blocked != "" {
		t.Fatalf("preview after close = %+v, %v", p, err)
	}
	st, err = f.app.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir(), Resume: &f.ref})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := f.app.DeleteSessions(refs, p.Token)
	if err == nil && rep.Deleted != 0 {
		t.Fatalf("deleted an owned session: %+v", rep)
	}
	if len(f.trash.calls) != 0 {
		t.Fatalf("trashed %v", f.trash.calls)
	}

	// Once the chat ends, the session can go.
	if err := f.app.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	log.waitExit(t, st.ChatID)
	p, err = f.app.PreviewDelete(refs)
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := f.app.DeleteSessions(refs, p.Token); err != nil || rep.Deleted != 1 {
		t.Fatalf("delete after close = %+v, %v", rep, err)
	}
}
