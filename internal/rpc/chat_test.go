package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
	"github.com/ginkcode/agent-sessions/internal/chatattach"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// chatBackend runs chats with the fake ACP agent.
type chatBackend struct {
	*mockBackend
	m *engine.ChatManager
}

func (b *chatBackend) Chats() *engine.ChatManager { return b.m }

// chatEvents records what a client's emitter receives.
type chatEvents struct {
	mu      sync.Mutex
	updates []engine.ChatUpdate
	exits   []engine.ChatExit
}

func (r *chatEvents) Emit(name string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case engine.ChatUpdateEvent:
		r.updates = append(r.updates, payload.(engine.ChatUpdate))
	case engine.ChatExitEvent:
		r.exits = append(r.exits, payload.(engine.ChatExit))
	}
}

func (r *chatEvents) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates) + len(r.exits)
}

func (r *chatEvents) waitUpdate(t *testing.T, chatID string, match func(engine.ChatUpdate) bool) engine.ChatUpdate {
	t.Helper()
	var got engine.ChatUpdate
	eventually(t, "chat update", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		i := slices.IndexFunc(r.updates, func(u engine.ChatUpdate) bool { return u.ChatID == chatID && match(u) })
		if i >= 0 {
			got = r.updates[i]
		}
		return i >= 0
	})
	return got
}

func (r *chatEvents) waitExit(t *testing.T, chatID string) engine.ChatExit {
	t.Helper()
	var got engine.ChatExit
	eventually(t, "chat exit", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		i := slices.IndexFunc(r.exits, func(x engine.ChatExit) bool { return x.ChatID == chatID })
		if i >= 0 {
			got = r.exits[i]
		}
		return i >= 0
	})
	return got
}

func kindIs(k string) func(engine.ChatUpdate) bool {
	return func(u engine.ChatUpdate) bool { return u.Kind == k }
}

// chunkText is an agent_message_chunk's text, or "".
func chunkText(u engine.ChatUpdate) string {
	if u.Kind != engine.ChatSessionUpdate || u.Update == nil || u.Update.Kind != acp.UpdateAgentMessageChunk {
		return ""
	}
	var c acp.ContentChunk
	_ = json.Unmarshal(u.Update.Raw, &c)
	return c.Content.Text
}

// chatConn is one client connection to a server that shares the backend.
type chatConn struct {
	s      *Server
	c      *Client
	events *chatEvents
	served chan struct{}
}

// disconnect ends the connection and waits for the server to finish.
func (x *chatConn) disconnect() {
	_ = x.c.Close()
	_ = x.s.Close()
	<-x.served
}

func newChatBackend(t *testing.T) (*chatBackend, string) {
	t.Helper()
	dir := t.TempDir()
	m := engine.NewChatManager(engine.ChatManagerOptions{
		Launch: func(_ context.Context, _ model.AgentID, d string) (acp.Command, error) {
			return acptest.Command(d), nil
		},
		Agents: func(context.Context) ([]engine.ChatAgent, error) {
			return []engine.ChatAgent{{Agent: model.AgentClaude, Name: "Claude Code", Available: true}}, nil
		},
		Grace: 2 * time.Second,
	})
	t.Cleanup(m.CloseAll)
	return &chatBackend{mockBackend: &mockBackend{}, m: m}, dir
}

func connectChat(t *testing.T, b engine.Backend) *chatConn {
	t.Helper()
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	x := &chatConn{s: NewServer(b, c2sR, s2cW), c: NewClient(s2cR, c2sW), events: &chatEvents{}, served: make(chan struct{})}
	x.c.SetEmitter(x.events)
	go func() {
		defer close(x.served)
		_ = x.s.Serve(context.Background())
	}()
	t.Cleanup(x.disconnect)
	return x
}

func initChat(t *testing.T, x *chatConn) {
	t.Helper()
	res, err := x.c.Initialize(t.Context(), InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Capabilities.ACPChat {
		t.Fatal("the host did not offer chats")
	}
}

// agentPID is the fake agent's process ID, which it puts in its session IDs.
func agentPID(t *testing.T, st engine.ChatState) int {
	t.Helper()
	parts := strings.Split(st.Session.ID, "-")
	pid, err := strconv.Atoi(parts[1])
	if len(parts) != 3 || err != nil {
		t.Fatalf("session ID %q", st.Session.ID)
	}
	return pid
}

func TestChatRPCRoundTrip(t *testing.T) {
	b, dir := newChatBackend(t)
	x := connectChat(t, b)
	initChat(t, x)
	ctx := t.Context()

	agents, err := x.c.ChatAgents(ctx)
	if err != nil || len(agents) != 1 || !agents[0].Available {
		t.Fatal(agents, err)
	}
	st, err := x.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil || st.ChatID == "" || st.Session.ID == "" {
		t.Fatal(st, err)
	}
	started := x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatStarted))
	if started.Seq != 1 || started.State == nil || started.State.Session != st.Session {
		t.Fatalf("started = %+v", started)
	}

	if err := x.c.ChatPrompt(ctx, st.ChatID, "stream 3", nil); err != nil {
		t.Fatal(err)
	}
	x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatTurnEnded))
	var text string
	x.events.mu.Lock()
	for _, u := range x.events.updates {
		text += chunkText(u)
	}
	x.events.mu.Unlock()
	if text != "0 1 2 " {
		t.Fatalf("streamed %q", text)
	}

	list, err := x.c.ChatList(ctx)
	if err != nil || len(list) != 1 || list[0].ChatID != st.ChatID {
		t.Fatal(list, err)
	}
	opts, err := x.c.ChatSetOption(ctx, st.ChatID, "mode", "auto")
	if err != nil || len(opts) != 1 || string(opts[0].CurrentValue) != `"auto"` {
		t.Fatal(opts, err)
	}

	// A permission request comes back as an event and is answered by ID.
	if err := x.c.ChatPrompt(ctx, st.ChatID, "permission", nil); err != nil {
		t.Fatal(err)
	}
	perm := x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatPermissionAsked))
	if perm.Permission == nil || len(perm.Permission.Options) != 3 {
		t.Fatalf("permission = %+v", perm)
	}
	if err := x.c.ChatPrompt(ctx, st.ChatID, "again", nil); !errors.Is(err, engine.ErrChatBusy) {
		t.Fatalf("second prompt: %v", err)
	}
	if err := x.c.ChatRespond(ctx, st.ChatID, "nope", "allow-once"); !errors.Is(err, engine.ErrPermissionNotFound) {
		t.Fatalf("unknown permission: %v", err)
	}
	if err := x.c.ChatRespond(ctx, st.ChatID, perm.Permission.ID, "allow-once"); err != nil {
		t.Fatal(err)
	}
	x.events.waitUpdate(t, st.ChatID, func(u engine.ChatUpdate) bool { return chunkText(u) == "allowed:allow-once" })

	if err := x.c.ChatPrompt(ctx, st.ChatID, "hang", nil); err != nil {
		t.Fatal(err)
	}
	// The cancel may reach the agent before the prompt; repeat until it ends.
	cancelled := func(u engine.ChatUpdate) bool {
		return u.Kind == engine.ChatTurnEnded && u.StopReason == acp.StopCancelled
	}
	eventually(t, "cancelled turn", func() bool {
		if err := x.c.ChatCancel(ctx, st.ChatID); err != nil {
			t.Error(err)
			return true
		}
		time.Sleep(50 * time.Millisecond)
		x.events.mu.Lock()
		defer x.events.mu.Unlock()
		return slices.ContainsFunc(x.events.updates, cancelled)
	})

	if err := x.c.ChatClose(ctx, st.ChatID); err != nil {
		t.Fatal(err)
	}
	exit := x.events.waitExit(t, st.ChatID)
	if exit.Reason != engine.ChatExitClosed {
		t.Fatalf("exit = %+v", exit)
	}
	assertProcessStopped(t, agentPID(t, st))

	// Events keep their order and numbers across the connection.
	x.events.mu.Lock()
	for i, u := range x.events.updates {
		if u.Seq != uint64(i+1) {
			t.Errorf("update %d has seq %d", i, u.Seq)
		}
	}
	if exit.Seq != uint64(len(x.events.updates)+1) {
		t.Errorf("exit seq %d after %d updates", exit.Seq, len(x.events.updates))
	}
	x.events.mu.Unlock()

	// A closed chat is forgotten: the connection no longer reaches it.
	if err := x.c.ChatPrompt(ctx, st.ChatID, "hi", nil); !errors.Is(err, engine.ErrChatNotFound) {
		t.Fatalf("prompt after close: %v", err)
	}
	if list, err := x.c.ChatList(ctx); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
}

// Chats belong to the connection that started them: another connection to
// the same host neither reaches them nor hears their events.
func TestChatRPCOwnership(t *testing.T) {
	b, dir := newChatBackend(t)
	a, other := connectChat(t, b), connectChat(t, b)
	initChat(t, a)
	initChat(t, other)
	ctx := t.Context()

	st, err := a.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.c.ChatPrompt(ctx, st.ChatID, "permission", nil); err != nil {
		t.Fatal(err)
	}
	perm := a.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatPermissionAsked))

	for name, call := range map[string]func() error{
		"prompt":  func() error { return other.c.ChatPrompt(ctx, st.ChatID, "hi", nil) },
		"cancel":  func() error { return other.c.ChatCancel(ctx, st.ChatID) },
		"respond": func() error { return other.c.ChatRespond(ctx, st.ChatID, perm.Permission.ID, "allow-once") },
		"option": func() error {
			_, err := other.c.ChatSetOption(ctx, st.ChatID, "mode", "auto")
			return err
		},
		"close": func() error { return other.c.ChatClose(ctx, st.ChatID) },
	} {
		if err := call(); !errors.Is(err, engine.ErrChatNotFound) {
			t.Errorf("%s from another connection: %v", name, err)
		}
	}
	if list, err := other.c.ChatList(ctx); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	// Its own session is refused too: one chat per session.
	_, err = other.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir, Resume: &st.Session})
	if !errors.Is(err, engine.ErrSessionOwned) {
		t.Fatalf("second chat for the session: %v", err)
	}

	// The owner still answers, and only it hears the chat.
	if err := a.c.ChatRespond(ctx, st.ChatID, perm.Permission.ID, "reject"); err != nil {
		t.Fatal(err)
	}
	a.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatTurnEnded))
	if err := a.c.ChatClose(ctx, st.ChatID); err != nil {
		t.Fatal(err)
	}
	a.events.waitExit(t, st.ChatID)
	if n := other.events.count(); n != 0 {
		t.Fatalf("another connection heard %d chat events", n)
	}
}

// A connection that ends takes its chats with it: their agents and the
// agents' children stop, and their permission requests are answered
// cancelled.
func TestChatRPCDisconnectClosesChats(t *testing.T) {
	b, dir := newChatBackend(t)
	x, other := connectChat(t, b), connectChat(t, b)
	initChat(t, x)
	initChat(t, other)
	ctx := t.Context()

	var pids []int
	var chats []engine.ChatState
	for range 2 {
		st, err := x.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		chats = append(chats, st)
		pids = append(pids, agentPID(t, st))
		if err := x.c.ChatPrompt(ctx, st.ChatID, "spawn", nil); err != nil {
			t.Fatal(err)
		}
		u := x.events.waitUpdate(t, st.ChatID, func(u engine.ChatUpdate) bool { return strings.HasPrefix(chunkText(u), "child ") })
		child, err := strconv.Atoi(strings.TrimPrefix(chunkText(u), "child "))
		if err != nil {
			t.Fatal(err)
		}
		pids = append(pids, child)
		x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatTurnEnded))
	}
	// One chat waits on a permission request when the connection goes.
	if err := x.c.ChatPrompt(ctx, chats[0].ChatID, "permission", nil); err != nil {
		t.Fatal(err)
	}
	x.events.waitUpdate(t, chats[0].ChatID, kindIs(engine.ChatPermissionAsked))
	// The other connection's chat is not this connection's to close.
	kept, err := other.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}

	x.disconnect()
	for _, st := range chats {
		if _, ok := b.m.Owned(st.Session); ok {
			t.Errorf("%s is still owned after the disconnect", st.Session.Key())
		}
	}
	if left := b.m.Chats(); len(left) != 1 || left[0].ChatID != kept.ChatID {
		t.Fatalf("chats after the disconnect: %+v", left)
	}
	for _, pid := range pids {
		assertProcessStopped(t, pid)
	}
	if list, err := other.c.ChatList(ctx); err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
}

// A chat whose start finishes after its connection ended does not outlive it.
func TestChatRPCStartAfterDisconnect(t *testing.T) {
	b, dir := newChatBackend(t)
	x := connectChat(t, b)
	initChat(t, x)
	x.s.closeChats()
	if _, err := x.c.ChatStart(t.Context(), engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir}); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("start on a closed connection: %v", err)
	}
	// The started chat was closed again: the manager holds no chat.
	x.s.chats.mu.Lock()
	x.s.chats.closed = false
	x.s.chats.mu.Unlock()
	st, err := x.c.ChatStart(t.Context(), engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	x.s.chats.mu.Lock()
	x.s.chats.closed = true
	x.s.chats.mu.Unlock()
	eventually(t, "no chat left but the last", func() bool {
		left := b.m.Chats()
		return len(left) == 1 && left[0].ChatID == st.ChatID
	})
}

func TestChatRPCAttachments(t *testing.T) {
	b, dir := newChatBackend(t)
	x := connectChat(t, b)
	res, err := x.c.Initialize(t.Context(), InitializeRequest{})
	if err != nil || !res.Capabilities.ChatAttachments {
		t.Fatal(res, err)
	}
	ctx := t.Context()
	st, err := x.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if st.PromptCapabilities == nil || !st.PromptCapabilities.Image {
		t.Fatalf("prompt capabilities = %+v", st.PromptCapabilities)
	}
	txt := chatattach.Attachment{Kind: chatattach.KindText, Name: "a.txt", MimeType: "text/plain", Text: "hi", URI: "file:///c/a.txt"}
	if err := x.c.ChatPrompt(ctx, st.ChatID, "blocks", []chatattach.Attachment{txt}); err != nil {
		t.Fatal(err)
	}
	x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatTurnEnded))
	var text string
	x.events.mu.Lock()
	for _, u := range x.events.updates {
		text += chunkText(u)
	}
	x.events.mu.Unlock()
	if want := "text blocks\nresource file:///c/a.txt text/plain hi"; text != want {
		t.Fatalf("blocks = %q, want %q", text, want)
	}
	// The server checks what a peer sends.
	bad := txt
	bad.Text = "a\x00b"
	if err := x.c.ChatPrompt(ctx, st.ChatID, "x", []chatattach.Attachment{bad}); err == nil || !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("bad attachment: %v", err)
	}

	// A host too old for attachments: text still goes, attachments do not.
	x.c.mu.Lock()
	x.c.caps.ChatAttachments = false
	x.c.mu.Unlock()
	if err := x.c.ChatPrompt(ctx, st.ChatID, "x", []chatattach.Attachment{txt}); !errors.Is(err, ErrNoChatAttachments) {
		t.Fatalf("old host: %v", err)
	}
	if err := x.c.ChatPrompt(ctx, st.ChatID, "stream 1", nil); err != nil {
		t.Fatalf("old host, text only: %v", err)
	}
}

// The agent's questions reach the client in order and its answers go back;
// a host too old for questions starts chats without them.
func TestChatRPCQuestions(t *testing.T) {
	b, dir := newChatBackend(t)
	x := connectChat(t, b)
	res, err := x.c.Initialize(t.Context(), InitializeRequest{})
	if err != nil || !res.Capabilities.ChatQuestions {
		t.Fatal(res, err)
	}
	ctx := t.Context()
	st, err := x.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir, Questions: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := x.c.ChatPrompt(ctx, st.ChatID, "question claude", nil); err != nil {
		t.Fatal(err)
	}
	asked := x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatQuestionAsked)).Question
	var keys []string
	for _, f := range asked.Fields {
		keys = append(keys, f.Key)
	}
	if strings.Join(keys, " ") != "question_0 question_0_custom question_1 question_1_custom" || len(asked.Fields[0].OneOf) != 2 || asked.Fields[2].Items == nil {
		t.Fatalf("asked = %+v", asked)
	}
	if err := x.c.ChatAnswer(ctx, st.ChatID, "question-x", acp.ElicitationDecline, nil); !errors.Is(err, engine.ErrQuestionNotFound) {
		t.Fatalf("unknown question: %v", err)
	}
	if err := x.c.ChatAnswer(ctx, st.ChatID, asked.ID, acp.ElicitationAccept, map[string]any{"question_0": "Something else"}); err == nil || !strings.Contains(err.Error(), "not one of the options") {
		t.Fatalf("bad answer: %v", err)
	}
	answer := map[string]any{"question_0": "New module", "question_1": []string{"Unit tests"}}
	if err := x.c.ChatAnswer(ctx, st.ChatID, asked.ID, acp.ElicitationAccept, answer); err != nil {
		t.Fatal(err)
	}
	resolved := x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatQuestionResolved)).Question
	if resolved.ID != asked.ID || resolved.Outcome != acp.ElicitationAccept || resolved.Content["question_0"] != "New module" {
		t.Fatalf("resolved = %+v", resolved)
	}
	got := x.events.waitUpdate(t, st.ChatID, func(u engine.ChatUpdate) bool { return chunkText(u) != "" })
	if want := `accept {"question_0":"New module","question_1":["Unit tests"]}`; chunkText(got) != want {
		t.Fatalf("agent got %q, want %q", chunkText(got), want)
	}
	x.events.waitUpdate(t, st.ChatID, kindIs(engine.ChatTurnEnded))

	// A host too old for questions: answers are refused, and a new chat
	// does not ask the agent to send them.
	x.c.mu.Lock()
	x.c.caps.ChatQuestions = false
	x.c.mu.Unlock()
	if err := x.c.ChatAnswer(ctx, st.ChatID, asked.ID, acp.ElicitationDecline, nil); !errors.Is(err, ErrNoChatQuestions) {
		t.Fatalf("old host: %v", err)
	}
	old, err := x.c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir(), Questions: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := x.c.ChatPrompt(ctx, old.ChatID, "question claude", nil); err != nil {
		t.Fatal(err)
	}
	x.events.waitUpdate(t, old.ChatID, kindIs(engine.ChatTurnEnded))
	if got := x.events.waitUpdate(t, old.ChatID, func(u engine.ChatUpdate) bool { return chunkText(u) != "" }); chunkText(got) != "no elicitation" {
		t.Fatalf("old host's agent: %q", chunkText(got))
	}
}

func TestChatRPCCapability(t *testing.T) {
	ctx := t.Context()
	calls := func(c *Client) map[string]error {
		_, agents := c.ChatAgents(ctx)
		_, start := c.ChatStart(ctx, engine.ChatStartRequest{Agent: model.AgentClaude, Dir: "/"})
		_, option := c.ChatSetOption(ctx, "c", "mode", "auto")
		_, list := c.ChatList(ctx)
		return map[string]error{
			"agents": agents, "start": start, "option": option, "list": list,
			"prompt":  c.ChatPrompt(ctx, "c", "hi", nil),
			"cancel":  c.ChatCancel(ctx, "c"),
			"respond": c.ChatRespond(ctx, "c", "p", "o"),
			"answer":  c.ChatAnswer(ctx, "c", "q", acp.ElicitationDecline, nil),
			"close":   c.ChatClose(ctx, "c"),
		}
	}
	for _, tc := range []struct {
		name    string
		backend engine.Backend
		force   bool // the client calls though the host did not offer chats
	}{
		{"no chat backend", &mockBackend{}, false},
		{"no chat backend, forced", &mockBackend{}, true},
		{"no chat manager", &chatBackend{mockBackend: &mockBackend{}}, false},
		{"no chat manager, forced", &chatBackend{mockBackend: &mockBackend{}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := connectChat(t, tc.backend)
			res, err := x.c.Initialize(ctx, InitializeRequest{})
			if err != nil || res.Capabilities.ACPChat {
				t.Fatal(res, err)
			}
			if tc.force {
				x.c.mu.Lock()
				x.c.caps.ACPChat = true
				x.c.caps.ChatQuestions = true
				x.c.mu.Unlock()
			}
			for name, err := range calls(x.c) {
				if !errors.Is(err, engine.ErrUnsupportedAction) {
					t.Errorf("%s: %v", name, err)
				}
			}
		})
	}
	// An old host answers method-not-found; the client reads it as no chats.
	t.Run("old host", func(t *testing.T) {
		x := connectChat(t, &mockBackend{})
		if _, err := x.c.Initialize(ctx, InitializeRequest{}); err != nil {
			t.Fatal(err)
		}
		x.c.mu.Lock()
		x.c.caps.ACPChat = true
		x.c.mu.Unlock()
		err := x.c.chatCall(ctx, "chatFromTheFuture", struct{}{}, nil)
		if !errors.Is(err, engine.ErrUnsupportedAction) {
			t.Fatal(err)
		}
	})
	// An absent capability is omitted, so old peers see the same JSON.
	raw, err := json.Marshal(Capabilities{})
	if err != nil || strings.Contains(string(raw), "acpChat") || strings.Contains(string(raw), "chatQuestions") {
		t.Fatal(string(raw), err)
	}
	if _, err := (&Client{}).ChatStart(ctx, engine.ChatStartRequest{}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}
