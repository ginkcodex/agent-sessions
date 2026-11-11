package engine

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// chatEvents records chat events in emission order.
type chatEvents struct {
	mu      sync.Mutex
	updates []ChatUpdate
	exits   []ChatExit
	changed chan struct{}
}

func (r *chatEvents) Emit(name string, payload any) {
	r.mu.Lock()
	switch name {
	case ChatUpdateEvent:
		r.updates = append(r.updates, payload.(ChatUpdate))
	case ChatExitEvent:
		r.exits = append(r.exits, payload.(ChatExit))
	}
	r.mu.Unlock()
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

func (r *chatEvents) of(chatID string) ([]ChatUpdate, []ChatExit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var u []ChatUpdate
	var x []ChatExit
	for _, e := range r.updates {
		if e.ChatID == chatID {
			u = append(u, e)
		}
	}
	for _, e := range r.exits {
		if e.ChatID == chatID {
			x = append(x, e)
		}
	}
	return u, x
}

// wait returns the chat's updates once one matches.
func (r *chatEvents) wait(t *testing.T, chatID string, match func(ChatUpdate) bool) ChatUpdate {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		u, _ := r.of(chatID)
		if i := slices.IndexFunc(u, match); i >= 0 {
			return u[i]
		}
		select {
		case <-r.changed:
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("no matching update; got %+v", u)
		}
	}
}

func (r *chatEvents) waitExit(t *testing.T, chatID string) ChatExit {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		if _, x := r.of(chatID); len(x) > 0 {
			return x[0]
		}
		select {
		case <-r.changed:
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatal("no chat:exit")
		}
	}
}

// checkSeq fails unless the chat's events, chat:exit last, are numbered
// 1, 2, 3... in emission order.
func (r *chatEvents) checkSeq(t *testing.T, chatID string) {
	t.Helper()
	u, x := r.of(chatID)
	for i, e := range u {
		if e.Seq != uint64(i+1) {
			t.Fatalf("update %d has seq %d: %+v", i, e.Seq, u)
		}
	}
	if len(x) > 1 || len(x) == 1 && x[0].Seq != uint64(len(u)+1) {
		t.Fatalf("exits = %+v after %d updates", x, len(u))
	}
}

func kind(k string) func(ChatUpdate) bool { return func(u ChatUpdate) bool { return u.Kind == k } }

type chatFixture struct {
	m         *ChatManager
	events    *chatEvents
	dir       string
	refreshed chan model.AgentID
}

func newChatFixture(t *testing.T, env ...string) *chatFixture {
	t.Helper()
	f := &chatFixture{events: &chatEvents{changed: make(chan struct{}, 1)}, dir: t.TempDir(), refreshed: make(chan model.AgentID, 16)}
	f.m = NewChatManager(ChatManagerOptions{
		Launch: func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
			return acptest.Command(dir, env...), nil
		},
		Refresh: func(a model.AgentID) {
			select {
			case f.refreshed <- a:
			default:
			}
		},
		Emitter: f.events,
		Grace:   2 * time.Second,
	})
	t.Cleanup(f.m.CloseAll)
	return f
}

func (f *chatFixture) start(t *testing.T, resume *model.SessionRef) ChatState {
	t.Helper()
	st, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: resume})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// turn sends text and waits for the turn to end.
func (f *chatFixture) turn(t *testing.T, chatID, text string) ChatUpdate {
	t.Helper()
	before, _ := f.events.of(chatID)
	if err := f.m.Prompt(chatID, text); err != nil {
		t.Fatal(err)
	}
	return f.events.wait(t, chatID, func(u ChatUpdate) bool { return u.Kind == ChatTurnEnded && u.Seq > uint64(len(before)) })
}

func chatCtx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

func chunks(updates []ChatUpdate) []string {
	var out []string
	for _, u := range updates {
		if u.Kind != ChatSessionUpdate {
			continue
		}
		if c, err := u.Update.AsChunk(); err == nil {
			out = append(out, c.Content.Text)
		}
	}
	return out
}

func TestChatNewSessionStreams(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if st.Session.Agent != model.AgentClaude || !strings.HasPrefix(st.Session.ID, "fake-") || st.Dir != f.dir {
		t.Fatalf("state = %+v", st)
	}
	if len(st.ConfigOptions) != 1 || string(st.ConfigOptions[0].CurrentValue) != `"ask"` || st.AgentInfo == nil || st.AgentInfo.Name != "acptest" {
		t.Fatalf("state = %+v", st)
	}
	if owner, ok := f.m.Owned(st.Session); !ok || owner != st.ChatID {
		t.Fatalf("owner = %q, %v", owner, ok)
	}

	end := f.turn(t, st.ChatID, "stream 5")
	if end.StopReason != acp.StopEndTurn || end.Error != "" {
		t.Fatalf("turn end = %+v", end)
	}
	u, _ := f.events.of(st.ChatID)
	var kinds []string
	for _, e := range u {
		kinds = append(kinds, e.Kind)
	}
	want := "started,turn_start,session_update,session_update,session_update,session_update,session_update,turn_end"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("kinds = %s", got)
	}
	if got := chunks(u); strings.Join(got, "") != "0 1 2 3 4 " {
		t.Fatalf("chunks = %q", got)
	}
	select {
	case a := <-f.refreshed:
		if a != model.AgentClaude {
			t.Fatalf("refreshed %s", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no rescan after the turn")
	}
	if got := f.m.Chats(); len(got) != 1 || got[0].ChatID != st.ChatID || got[0].Working {
		t.Fatalf("chats = %+v", got)
	}

	if err := f.m.Close(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if exit := f.events.waitExit(t, st.ChatID); exit.Reason != ChatExitClosed {
		t.Fatalf("exit = %+v", exit)
	}
	f.events.checkSeq(t, st.ChatID)
	if _, ok := f.m.Owned(st.Session); ok {
		t.Fatal("session still owned after close")
	}
	if err := f.m.Prompt(st.ChatID, "x"); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("prompt after close = %v", err)
	}
	if err := f.m.Close(st.ChatID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("second close = %v", err)
	}
}

func TestChatRefusesConcurrentPrompt(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if err := f.m.Prompt(st.ChatID, "hang"); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Prompt(st.ChatID, "stream 1"); !errors.Is(err, ErrChatBusy) {
		t.Fatalf("second prompt = %v", err)
	}
	if got := f.m.Chats(); !got[0].Working {
		t.Fatalf("not working: %+v", got)
	}
	// The cancel may reach the agent before the prompt; repeat until it ends.
	var end ChatUpdate
	for done := false; !done; {
		if err := f.m.Cancel(st.ChatID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		u, _ := f.events.of(st.ChatID)
		if i := slices.IndexFunc(u, kind(ChatTurnEnded)); i >= 0 {
			end, done = u[i], true
		}
	}
	if end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end = %+v", end)
	}
	// The next turn may start as soon as the end is seen.
	if end := f.turn(t, st.ChatID, "stream 1"); end.StopReason != acp.StopEndTurn {
		t.Fatalf("next turn = %+v", end)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatPermissionAnswered(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if err := f.m.Prompt(st.ChatID, "permission"); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	if asked.ID == "" || asked.ToolCall.ToolCallID != "call_p" || len(asked.Options) != 3 {
		t.Fatalf("asked = %+v", asked)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "bogus"); err == nil {
		t.Fatal("an option the request did not offer was accepted")
	}
	if err := f.m.Respond(st.ChatID, "perm-x", "allow-once"); !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("unknown request = %v", err)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("second answer = %v", err)
	}
	end := f.events.wait(t, st.ChatID, kind(ChatTurnEnded))
	if end.StopReason != acp.StopEndTurn {
		t.Fatalf("turn end = %+v", end)
	}
	resolved := f.events.wait(t, st.ChatID, kind(ChatPermissionResolved)).Permission
	if resolved.ID != asked.ID || resolved.Outcome != "selected" || resolved.OptionID != "allow-once" {
		t.Fatalf("resolved = %+v", resolved)
	}
	u, _ := f.events.of(st.ChatID)
	if got := chunks(u); len(got) != 1 || got[0] != "allowed:allow-once" {
		t.Fatalf("chunks = %q", got)
	}
}

func TestChatCancelAnswersPermission(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if err := f.m.Prompt(st.ChatID, "permission"); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	if err := f.m.Cancel(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if end := f.events.wait(t, st.ChatID, kind(ChatTurnEnded)); end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end = %+v", end)
	}
	resolved := f.events.wait(t, st.ChatID, kind(ChatPermissionResolved)).Permission
	if resolved.ID != asked.ID || resolved.Outcome != "cancelled" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("answer after cancel = %v", err)
	}
}

func TestChatCloseDuringPermission(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if err := f.m.Prompt(st.ChatID, "permission"); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	closed := make(chan struct{})
	go func() {
		_ = f.m.Close(st.ChatID)
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(15 * time.Second):
		t.Fatal("Close blocked on a pending permission")
	}
	// Close returns after the chat is gone, so every event is in.
	u, x := f.events.of(st.ChatID)
	if len(x) != 1 || x[0].Reason != ChatExitClosed {
		t.Fatalf("exits = %+v", x)
	}
	r := u[slices.IndexFunc(u, kind(ChatPermissionResolved))]
	if r.Permission.ID != asked.ID || r.Permission.Outcome != "cancelled" {
		t.Fatalf("resolved = %+v", r.Permission)
	}
	if end := u[len(u)-1]; end.Kind != ChatTurnEnded || end.StopReason == acp.StopEndTurn {
		t.Fatalf("last update = %+v", end)
	}
	f.events.checkSeq(t, st.ChatID)
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("answer after close = %v", err)
	}
}

func TestChatAgentCrash(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	end := f.turn(t, st.ChatID, "crash")
	if end.Error == "" || !strings.Contains(end.Error, "fake agent crashed") {
		t.Fatalf("turn end = %+v", end)
	}
	exit := f.events.waitExit(t, st.ChatID)
	if exit.Reason != ChatExitCrashed || exit.Error == "" || !strings.Contains(exit.StderrTail, "fake agent crashed") {
		t.Fatalf("exit = %+v", exit)
	}
	f.events.checkSeq(t, st.ChatID)
	if _, ok := f.m.Owned(st.Session); ok {
		t.Fatal("a crashed chat still owns its session")
	}
	if len(f.m.Chats()) != 0 {
		t.Fatalf("chats = %+v", f.m.Chats())
	}
	// The session is free for a new chat.
	f.start(t, &st.Session)
}

func TestChatAuthRequired(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	end := f.turn(t, st.ChatID, "auth")
	if len(end.AuthMethods) != 1 || end.AuthMethods[0].ID != "fake-login" || !strings.Contains(end.Error, "fake auth login") {
		t.Fatalf("turn end = %+v", end)
	}
	// Other failures are not mistaken for a login.
	if end := f.turn(t, st.ChatID, "echo ok"); end.Error != "" || len(end.AuthMethods) != 0 {
		t.Fatalf("next turn = %+v", end)
	}
	err := &AuthRequiredError{Agent: model.AgentClaude, Methods: []acp.AuthMethod{{ID: "x", Name: "Log in"}}, Err: errors.New("auth")}
	if err.Error() != "claude-code needs you to log in: Log in" || errors.Unwrap(err).Error() != "auth" {
		t.Fatalf("error = %q", err)
	}
}

func TestChatOwnership(t *testing.T) {
	f := newChatFixture(t)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "old-1"}
	st := f.start(t, &ref)
	if st.Session != ref {
		t.Fatalf("session = %+v", st.Session)
	}
	// Resumed: nothing was replayed.
	if u, _ := f.events.of(st.ChatID); len(chunks(u)) != 0 {
		t.Fatalf("replayed %q", chunks(u))
	}

	_, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref})
	var owned *SessionOwnedError
	if !errors.As(err, &owned) || owned.ChatID != st.ChatID || !errors.Is(err, ErrSessionOwned) {
		t.Fatalf("second chat = %v", err)
	}
	if !strings.Contains(err.Error(), "claude-code:old-1") {
		t.Fatalf("error = %q", err)
	}
	if _, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentCodex, Dir: f.dir, Resume: &ref}); err == nil {
		t.Fatal("continued a Claude session with Codex")
	}
	// A refused start leaves the owner alone.
	if owner, ok := f.m.Owned(ref); !ok || owner != st.ChatID {
		t.Fatalf("owner = %q, %v", owner, ok)
	}

	if err := f.m.Close(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.m.Owned(ref); ok {
		t.Fatal("still owned after close")
	}
	f.start(t, &ref)
}

// A failed start releases the session it reserved.
func TestChatFailedStartReleasesSession(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_PROTOCOL=2")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "old-1"}
	_, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref})
	if err == nil || !strings.Contains(err.Error(), "protocol version 2") {
		t.Fatalf("start = %v", err)
	}
	if _, ok := f.m.Owned(ref); ok || len(f.m.Chats()) != 0 {
		t.Fatal("a failed start kept its chat")
	}

	launchErr := errors.New("no adapter")
	m := NewChatManager(ChatManagerOptions{Launch: func(context.Context, model.AgentID, string) (acp.Command, error) {
		return acp.Command{}, launchErr
	}})
	if _, err := m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref}); !errors.Is(err, launchErr) {
		t.Fatalf("start = %v", err)
	}
	if _, ok := m.Owned(ref); ok {
		t.Fatal("a failed launch kept the session")
	}
}

func TestChatLoadDropsReplay(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_NO_RESUME=1")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "old-1"}
	st := f.start(t, &ref)
	f.turn(t, st.ChatID, "hello there")
	u, _ := f.events.of(st.ChatID)
	if got := chunks(u); len(got) != 1 || got[0] != "hello there" {
		t.Fatalf("chunks = %q", got)
	}
}

func TestChatSetOption(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	opts, err := f.m.SetOption(chatCtx(t), st.ChatID, "mode", "auto")
	if err != nil || string(opts[0].CurrentValue) != `"auto"` {
		t.Fatalf("set = %+v, %v", opts, err)
	}
	if got := f.m.Chats()[0].ConfigOptions; string(got[0].CurrentValue) != `"auto"` {
		t.Fatalf("state options = %+v", got)
	}
	ev := f.events.wait(t, st.ChatID, kind(ChatConfigOptions))
	if string(ev.ConfigOptions[0].CurrentValue) != `"auto"` {
		t.Fatalf("event = %+v", ev)
	}
	if _, err := f.m.SetOption(chatCtx(t), st.ChatID, "mode", "nope"); err == nil {
		t.Fatal("invalid value accepted")
	}
	if _, err := f.m.SetOption(chatCtx(t), "chat-x", "mode", "ask"); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("unknown chat = %v", err)
	}
}

func TestWithMode(t *testing.T) {
	opts := []acp.ConfigOption{
		{ID: "model", CurrentValue: []byte(`"a"`)},
		{ID: "permissions", Category: "mode", CurrentValue: []byte(`"ask"`)},
	}
	got := withMode(opts, "plan")
	if string(got[1].CurrentValue) != `"plan"` || string(opts[1].CurrentValue) != `"ask"` {
		t.Fatalf("got %s, original %s", got[1].CurrentValue, opts[1].CurrentValue)
	}
	if withMode(opts[:1], "plan") != nil {
		t.Fatal("changed options without a mode")
	}
	if got := withMode([]acp.ConfigOption{{ID: "mode"}}, "x"); string(got[0].CurrentValue) != `"x"` {
		t.Fatalf("by ID: %s", got[0].CurrentValue)
	}
}

func TestChatCloseAll(t *testing.T) {
	baseline := runtime.NumGoroutine()
	f := newChatFixture(t)
	var clients []*acp.Client
	var ids []string
	for i, text := range []string{"hang", "permission", ""} {
		st := f.start(t, nil)
		ids = append(ids, st.ChatID)
		if text != "" {
			if err := f.m.Prompt(st.ChatID, text); err != nil {
				t.Fatal(err)
			}
		}
		if i == 1 {
			f.events.wait(t, st.ChatID, kind(ChatPermissionAsked))
		}
		f.m.mu.Lock()
		clients = append(clients, f.m.chats[st.ChatID].client)
		f.m.mu.Unlock()
	}
	if len(f.m.Chats()) != 3 {
		t.Fatalf("chats = %+v", f.m.Chats())
	}

	f.m.CloseAll()
	for _, c := range clients {
		select {
		case <-c.Done():
		default:
			t.Fatal("an agent outlived CloseAll")
		}
	}
	for _, id := range ids {
		if _, x := f.events.of(id); len(x) != 1 || x[0].Reason != ChatExitClosed {
			t.Fatalf("%s exits = %+v", id, x)
		}
		f.events.checkSeq(t, id)
	}
	if len(f.m.Chats()) != 0 || len(f.m.owners) != 0 {
		t.Fatalf("left: %+v, owners %v", f.m.Chats(), f.m.owners)
	}

	// The clients' own cleanup ends right after the exit.
	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > baseline {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<20)
			t.Fatalf("%d goroutines, %d before:\n%s", runtime.NumGoroutine(), baseline, buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Close during the start stops it.
func TestChatCloseDuringStart(t *testing.T) {
	f := newChatFixture(t)
	launched := make(chan struct{})
	release := make(chan struct{})
	f.m.opts.Launch = func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
		close(launched)
		<-release
		return acptest.Command(dir), nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir})
		result <- err
	}()
	<-launched
	closed := make(chan struct{})
	go func() {
		f.m.CloseAll()
		close(closed)
	}()
	// CloseAll waits for the start, which is still launching.
	time.Sleep(50 * time.Millisecond)
	close(release)
	<-closed
	if err := <-result; !errors.Is(err, ErrChatClosed) {
		t.Fatalf("start = %v", err)
	}
	if len(f.m.Chats()) != 0 || len(f.m.owners) != 0 {
		t.Fatal("the closed start left state behind")
	}
}

// The engine closes its chats and passes its emitter on.
func TestEngineClosesChats(t *testing.T) {
	svc, dir := chatService(t)
	eng := NewEngine(WithService(svc))
	events := &chatEvents{changed: make(chan struct{}, 1)}
	eng.SetEmitter(events)
	eng.Chats().opts.Launch = func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
		return acptest.Command(dir), nil
	}
	st, err := eng.Chats().Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if _, x := events.of(st.ChatID); len(x) != 1 || x[0].Reason != ChatExitClosed {
		t.Fatalf("exits = %+v", x)
	}
	// Without a running cache there is nothing to rescan.
	eng.refreshAgent(model.AgentClaude)
}

func TestChatManagerAgents(t *testing.T) {
	if got, err := NewChatManager(ChatManagerOptions{}).Agents(chatCtx(t)); err != nil || len(got) != 0 {
		t.Fatalf("no lookup: %+v, %v", got, err)
	}
	svc, _ := chatService(t)
	stubFind(t, nil)
	got, err := NewEngine(WithService(svc)).Chats().Agents(chatCtx(t))
	if err != nil || len(got) != 1 || got[0].Agent != model.AgentClaude || got[0].Available {
		t.Fatalf("agents = %+v, %v", got, err)
	}
}
