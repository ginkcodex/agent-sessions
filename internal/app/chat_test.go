package app

import (
	"context"
	"errors"
	"io"
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
	"github.com/ginkcode/agent-sessions/internal/rpc"
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

	if err := a.ChatPrompt(st.ChatID, "hello", nil); err != nil {
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
	if err := a.ChatPrompt(st.ChatID, "permission", nil); err != nil {
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

	if err := a.ChatPrompt(st.ChatID, "hang", nil); err != nil {
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
	if err := a.ChatPrompt(st.ChatID, "again", nil); !errors.Is(err, engine.ErrChatNotFound) {
		t.Fatalf("prompt after close: %v", err)
	}
}

// The agent's question reaches the window, and ChatAnswer sends the
// answer back, on this computer and on a host.
func TestChatAnswerBinding(t *testing.T) {
	for _, where := range []string{"local", "remote"} {
		t.Run(where, func(t *testing.T) {
			var a *App
			var log *chatLog
			if where == "local" {
				a = NewAppWithService(chatTestService())
				log = useChatEngine(t, a)
			} else {
				a, log = remoteChatApp(t)
			}
			st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir(), Questions: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := a.ChatPrompt(st.ChatID, "question claude", nil); err != nil {
				t.Fatal(err)
			}
			var asked *engine.ChatQuestion
			log.waitFor(t, "question", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
				i := slices.IndexFunc(u, func(u engine.ChatUpdate) bool { return u.ChatID == st.ChatID && u.Kind == engine.ChatQuestionAsked })
				if i >= 0 {
					asked = u[i].Question
				}
				return i >= 0
			})
			if got := a.ChatList(); len(got) != 1 || len(got[0].Questions) != 1 || got[0].Questions[0].ID != asked.ID {
				t.Fatalf("ChatList = %+v", got)
			}
			if err := a.ChatAnswer(st.ChatID, "question-x", acp.ElicitationDecline, nil); !errors.Is(err, engine.ErrQuestionNotFound) {
				t.Fatalf("unknown question: %v", err)
			}
			if err := a.ChatAnswer(st.ChatID, asked.ID, acp.ElicitationAccept, map[string]any{"question_0": "New module"}); err != nil {
				t.Fatal(err)
			}
			if end := log.waitTurnEnd(t, st.ChatID); end.StopReason != "end_turn" {
				t.Fatalf("turn end = %+v", end)
			}
			log.waitFor(t, "the answer", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
				return slices.ContainsFunc(u, func(u engine.ChatUpdate) bool {
					return u.ChatID == st.ChatID && u.Kind == engine.ChatQuestionResolved && u.Question.Outcome == acp.ElicitationAccept
				})
			})
			if got := a.ChatList(); len(got) != 1 || len(got[0].Questions) != 0 {
				t.Fatalf("ChatList after the answer = %+v", got)
			}
		})
	}
}

// remoteChatApp shows a host whose server runs chats with the fake agent,
// over a real rpc pair. Its events reach the returned log through the
// generation-gated emitter, as a dialled host's do.
func remoteChatApp(t *testing.T) (*App, *chatLog) {
	t.Helper()
	eng := engine.NewEngine(
		engine.WithService(engine.NewService(scan.NewCatalog(), nil)),
		engine.WithCacheEnabled(false),
		engine.WithChatLaunch(func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
			return acptest.Command(dir), nil
		}),
	)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	server, client := rpc.NewServer(eng, inR, outW), rpc.NewClient(outR, inW)
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(context.Background()) }()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close(); <-done; _ = eng.Close() })

	a := NewAppWithService(chatTestService())
	useChatEngine(t, a)
	a.conn = newConnection(nil)
	a.conn.wantHost = "remote-server"
	a.conn.localActive = false
	a.conn.gen = 1
	a.conn.state = ConnectionState{Phase: ConnConnected, Host: "remote-server", Generation: 1}
	log := &chatLog{}
	client.SetEmitter(generationEmitter(log, 1, a.conn))
	init, err := client.Initialize(t.Context(), rpc.InitializeRequest{})
	if err != nil || !init.Capabilities.ACPChat {
		t.Fatalf("capabilities %+v %v", init, err)
	}
	a.conn.client = client
	return a, log
}

// A host's chats run on the host: the bindings reach its server and its
// events come back through the connection, until the window moves on.
func TestChatBindingsRemote(t *testing.T) {
	a, log := remoteChatApp(t)
	local, err := a.ownChats()
	if err != nil {
		t.Fatal(err)
	}

	if agents, err := a.ChatAgents(); err != nil || len(agents) != 0 {
		t.Fatalf("ChatAgents = %+v, %v", agents, err)
	}
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(local.Chats()) != 0 {
		t.Fatal("the chat ran on this computer")
	}
	if got := a.ChatList(); len(got) != 1 || got[0].ChatID != st.ChatID {
		t.Fatalf("ChatList = %+v", got)
	}
	if err := a.ChatPrompt(st.ChatID, "hello", nil); err != nil {
		t.Fatal(err)
	}
	if end := log.waitTurnEnd(t, st.ChatID); end.StopReason != "end_turn" {
		t.Fatalf("turn end %+v", end)
	}
	log.waitFor(t, "streamed text", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		return slices.ContainsFunc(u, func(u engine.ChatUpdate) bool {
			return u.ChatID == st.ChatID && u.Kind == engine.ChatSessionUpdate && u.Update.Kind == "agent_message_chunk"
		})
	})
	if _, err := a.ChatSetOption(st.ChatID, "nope", "x"); err == nil {
		t.Fatal("an unknown option was set")
	}

	// The window moves on (another host, or a reconnect): the old
	// connection's events no longer reach it.
	a.conn.mu.Lock()
	a.conn.gen++
	a.conn.mu.Unlock()
	log.mu.Lock()
	before := len(log.updates)
	log.mu.Unlock()
	if err := a.ChatPrompt(st.ChatID, "again", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	log.mu.Lock()
	after, exits := len(log.updates), len(log.exits)
	log.mu.Unlock()
	if after != before || exits != 0 {
		t.Fatalf("a stale connection's events arrived: %d updates (had %d), %d exits", after, before, exits)
	}
}

// A host whose server predates chats, or one that is offline, says so.
func TestChatRefusedForHosts(t *testing.T) {
	for name, a := range map[string]*App{"no capability": setupRemoteApp(&stubRemoteBackend{}), "offline": offlineRemoteApp()} {
		want := ErrChatRemote
		if name == "offline" {
			want = rpc.ErrDisconnected
		}
		if _, err := a.ChatAgents(); !errors.Is(err, want) {
			t.Errorf("%s ChatAgents: %v", name, err)
		}
		if _, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: "/p"}); !errors.Is(err, want) {
			t.Errorf("%s ChatStart: %v", name, err)
		}
		if err := a.ChatPrompt("chat-1", "hi", nil); !errors.Is(err, want) {
			t.Errorf("%s ChatPrompt: %v", name, err)
		}
		if _, err := a.ChatSetOption("chat-1", "mode", "plan"); !errors.Is(err, want) {
			t.Errorf("%s ChatSetOption: %v", name, err)
		}
		if err := a.ChatAnswer("chat-1", "question-1", acp.ElicitationDecline, nil); !errors.Is(err, want) {
			t.Errorf("%s ChatAnswer: %v", name, err)
		}
		if err := a.ChatClose("chat-1"); !errors.Is(err, want) {
			t.Errorf("%s ChatClose: %v", name, err)
		}
		if got := a.ChatList(); got == nil || len(got) != 0 {
			t.Errorf("%s ChatList = %#v", name, got)
		}
	}
}

// A connected server without the capability answers ErrChatRemote too.
func TestChatRefusedWithoutCapability(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	eng := engine.NewEngine(engine.WithService(engine.NewService(scan.NewCatalog(), nil)), engine.WithCacheEnabled(false))
	server, client := rpc.NewServer(noChats{eng}, inR, outW), rpc.NewClient(outR, inW)
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(context.Background()) }()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close(); <-done; _ = eng.Close() })
	if init, err := client.Initialize(t.Context(), rpc.InitializeRequest{}); err != nil || init.Capabilities.ACPChat {
		t.Fatalf("capabilities %+v %v", init, err)
	}
	a := setupRemoteApp(&stubRemoteBackend{})
	a.conn.client = client
	if _, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: "/p"}); !errors.Is(err, ErrChatRemote) {
		t.Fatalf("ChatStart: %v", err)
	}
}

// noChats is a backend without a chat manager, as an older server is.
type noChats struct{ engine.Backend }

// Closing the window, or quitting, ends its chats' agents and answers
// their permission requests cancelled.
func TestChatEndsWithWindow(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ChatPrompt(st.ChatID, "permission", nil); err != nil {
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

// A window with an open chat asks before it closes or the app quits: this
// computer's chats from the chat manager, a host's from what the frontend
// says (SetChatsOpen).
func TestOpenChatsGuardTheWindowClose(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)
	a.fileClose.busy = a.windowBusy
	closes := func() bool {
		prevent, id := a.fileClose.request()
		if id != 0 {
			a.fileClose.complete(id, false)
		}
		return !prevent
	}
	if !closes() {
		t.Fatal("a window without chats asked before closing")
	}

	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if closes() {
		t.Fatal("a window with a local chat closed unasked")
	}
	if id, answer := a.fileClose.requestHeld(); id == 0 || answer == nil {
		t.Fatal("a quit with a local chat went ahead unasked")
	} else {
		a.fileClose.complete(id, false)
	}
	if err := a.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	log.waitExit(t, st.ChatID)
	if !closes() {
		t.Fatal("the window still asked after its chat ended")
	}

	// A chat on a host: only the frontend knows.
	a.SetChatsOpen(true)
	if closes() {
		t.Fatal("a window with a host's chat closed unasked")
	}
	a.SetChatsOpen(false)
	if !closes() {
		t.Fatal("the window still asked once its chats ended")
	}
}
