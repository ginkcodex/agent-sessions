package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
	"github.com/ginkcode/agent-sessions/internal/chatattach"
	"github.com/ginkcode/agent-sessions/internal/manage"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
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
		owners:  &chatOwners{m: map[string]string{}},
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
	if _, err := f.m.Prompt(chatID, text, nil); err != nil {
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
	want := "commands,started,turn_start,session_update,session_update,session_update,session_update,session_update,turn_end"
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
	if _, err := f.m.Prompt(st.ChatID, "x", nil); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("prompt after close = %v", err)
	}
	if err := f.m.Close(st.ChatID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("second close = %v", err)
	}
}

func TestChatPromptWithAttachments(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if st.PromptCapabilities == nil || !st.PromptCapabilities.Image || !st.PromptCapabilities.EmbeddedContext {
		t.Fatalf("prompt capabilities = %+v", st.PromptCapabilities)
	}
	img := chatattach.Attachment{Kind: chatattach.KindImage, Name: "dot.png", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nxxxx"))}
	txt := chatattach.Attachment{Kind: chatattach.KindText, Name: "a.go", MimeType: "text/x-go", Text: "package a", URI: "file:///p/a.go"}

	// The agent gets the text, then each attachment in order.
	before, _ := f.events.of(st.ChatID)
	if _, err := f.m.Prompt(st.ChatID, "blocks please", []chatattach.Attachment{txt, img}); err != nil {
		t.Fatal(err)
	}
	f.events.wait(t, st.ChatID, func(u ChatUpdate) bool { return u.Kind == ChatTurnEnded && u.Seq > uint64(len(before)) })
	u, _ := f.events.of(st.ChatID)
	want := "text blocks please\nresource file:///p/a.go text/x-go package a\nimage image/png 16"
	if got := chunks(u[len(before):]); len(got) != 1 || got[0] != want {
		t.Fatalf("blocks = %q, want %q", got, want)
	}

	// Attachments alone are a prompt; nothing at all is not.
	before, _ = f.events.of(st.ChatID)
	if _, err := f.m.Prompt(st.ChatID, "", []chatattach.Attachment{img}); err != nil {
		t.Fatal(err)
	}
	f.events.wait(t, st.ChatID, func(u ChatUpdate) bool { return u.Kind == ChatTurnEnded && u.Seq > uint64(len(before)) })
	u, _ = f.events.of(st.ChatID)
	if got := chunks(u[len(before):]); len(got) != 1 || got[0] != "image image/png 16" {
		t.Fatalf("blocks = %q", got)
	}
	if _, err := f.m.Prompt(st.ChatID, "", nil); !errors.Is(err, ErrEmptyPrompt) {
		t.Fatalf("empty prompt: %v", err)
	}
	// A malformed attachment refuses the prompt before any turn starts.
	bad := img
	bad.MimeType = "image/jpeg"
	before, _ = f.events.of(st.ChatID)
	if _, err := f.m.Prompt(st.ChatID, "x", []chatattach.Attachment{bad}); err == nil || !strings.Contains(err.Error(), "dot.png") {
		t.Fatalf("bad attachment: %v", err)
	}
	if after, _ := f.events.of(st.ChatID); len(after) != len(before) {
		t.Fatalf("a refused prompt sent %d events", len(after)-len(before))
	}
}

func TestChatAgentWithoutImages(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_TEXT_ONLY=1")
	st := f.start(t, nil)
	if st.PromptCapabilities == nil || st.PromptCapabilities.Image || st.PromptCapabilities.EmbeddedContext {
		t.Fatalf("prompt capabilities = %+v", st.PromptCapabilities)
	}
	img := chatattach.Attachment{Kind: chatattach.KindImage, Name: "dot.png", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nxxxx"))}
	if _, err := f.m.Prompt(st.ChatID, "look", []chatattach.Attachment{img}); !errors.Is(err, chatattach.ErrNoImages) {
		t.Fatalf("image to a text-only agent: %v", err)
	}
	if got := f.m.Chats(); len(got) != 1 || got[0].Working {
		t.Fatalf("a refused prompt left the chat working: %+v", got)
	}
	// A text file goes in as text instead.
	txt := chatattach.Attachment{Kind: chatattach.KindText, Name: "a.txt", MimeType: "text/plain", Text: "hi"}
	before, _ := f.events.of(st.ChatID)
	if _, err := f.m.Prompt(st.ChatID, "blocks", []chatattach.Attachment{txt}); err != nil {
		t.Fatal(err)
	}
	f.events.wait(t, st.ChatID, func(u ChatUpdate) bool { return u.Kind == ChatTurnEnded && u.Seq > uint64(len(before)) })
	u, _ := f.events.of(st.ChatID)
	want := "text blocks\ntext Attached file a.txt:\n\n```\nhi\n```"
	if got := chunks(u[len(before):]); len(got) != 1 || got[0] != want {
		t.Fatalf("blocks = %q, want %q", got, want)
	}
}

// cancelTurn cancels the running turn and returns its end. The cancel may
// reach the agent before the prompt; it is repeated until the turn ends.
func (f *chatFixture) cancelTurn(t *testing.T, chatID string) ChatUpdate {
	t.Helper()
	before, _ := f.events.of(chatID)
	for {
		if err := f.m.Cancel(chatID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		u, _ := f.events.of(chatID)
		if i := slices.IndexFunc(u[len(before):], kind(ChatTurnEnded)); i >= 0 {
			return u[len(before)+i]
		}
	}
}

// queue sends text to a working chat and returns the queued ID.
func (f *chatFixture) queue(t *testing.T, chatID, text string, atts ...chatattach.Attachment) string {
	t.Helper()
	r, err := f.m.Prompt(chatID, text, atts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Queued == "" {
		t.Fatalf("%q was not queued", text)
	}
	return r.Queued
}

// turnEnds waits until the chat has emitted n turn ends.
func (f *chatFixture) turnEnds(t *testing.T, chatID string, n int) []ChatUpdate {
	t.Helper()
	f.events.wait(t, chatID, func(u ChatUpdate) bool {
		all, _ := f.events.of(chatID)
		ends := 0
		for _, e := range all {
			if e.Kind == ChatTurnEnded {
				ends++
			}
		}
		return ends >= n
	})
	u, _ := f.events.of(chatID)
	return u
}

// story names the turn, queue and steering events: "start", "start:<text>"
// for a turn from the queue, "queue:<n>", "steered:<text>", "end:<reason>".
func story(updates []ChatUpdate) []string {
	var out []string
	for _, u := range updates {
		switch u.Kind {
		case ChatTurnStarted:
			if u.Prompt != nil {
				out = append(out, "start:"+u.Prompt.Text)
			} else {
				out = append(out, "start")
			}
		case ChatQueueChanged:
			out = append(out, "queue:"+strconv.Itoa(len(u.Queue)))
		case ChatSteered:
			out = append(out, "steered:"+u.Prompt.Text)
		case ChatTurnEnded:
			out = append(out, "end:"+u.StopReason)
		}
	}
	return out
}

func TestChatQueueSendsAfterNormalEnd(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if !st.CanQueue || !st.CanSteer {
		t.Fatalf("state = %+v", st)
	}
	if r, err := f.m.Prompt(st.ChatID, "permission", nil); err != nil || r.Queued != "" {
		t.Fatalf("prompt = %+v, %v", r, err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	first := f.queue(t, st.ChatID, "stream 1")
	second := f.queue(t, st.ChatID, "second")
	if first == second {
		t.Fatalf("both queued as %q", first)
	}
	// A reloaded window finds the queue in the chat's state.
	if open := f.m.Chats(); len(open[0].Queue) != 2 || open[0].Queue[0].ID != first || open[0].Queue[1].Text != "second" {
		t.Fatalf("chats = %+v", open)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); err != nil {
		t.Fatal(err)
	}
	u := f.turnEnds(t, st.ChatID, 3)
	want := []string{"start", "queue:1", "queue:2", "end:end_turn", "queue:1", "start:stream 1", "end:end_turn", "queue:0", "start:second", "end:end_turn"}
	if got := story(u); !slices.Equal(got, want) {
		t.Fatalf("story = %q, want %q", got, want)
	}
	if got := chunks(u); !slices.Equal(got, []string{"allowed:allow-once", "0 ", "second"}) {
		t.Fatalf("chunks = %q", got)
	}
	if open := f.m.Chats(); open[0].Working || len(open[0].Queue) != 0 {
		t.Fatalf("chats = %+v", open)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatQueuePausesAfterCancel(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "hang", nil); err != nil {
		t.Fatal(err)
	}
	if got := f.m.Chats(); !got[0].Working {
		t.Fatalf("not working: %+v", got)
	}
	queued := f.queue(t, st.ChatID, "stream 1")
	if end := f.cancelTurn(t, st.ChatID); end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end = %+v", end)
	}
	// Paused: nothing starts, and the message stays queued.
	time.Sleep(100 * time.Millisecond)
	if open := f.m.Chats(); open[0].Working || len(open[0].Queue) != 1 || open[0].Queue[0].ID != queued {
		t.Fatalf("chats = %+v", open)
	}
	// A new message starts at once; the queue goes on after it.
	if r, err := f.m.Prompt(st.ChatID, "fresh words", nil); err != nil || r.Queued != "" {
		t.Fatalf("prompt = %+v, %v", r, err)
	}
	u := f.turnEnds(t, st.ChatID, 3)
	want := []string{"start", "queue:1", "end:cancelled", "start", "end:end_turn", "queue:0", "start:stream 1", "end:end_turn"}
	if got := story(u); !slices.Equal(got, want) {
		t.Fatalf("story = %q, want %q", got, want)
	}
	if got := chunks(u); !slices.Equal(got, []string{"fresh words", "0 "}) {
		t.Fatalf("chunks = %q", got)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatQueueChecksAttachments(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_TEXT_ONLY=1")
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "hang", nil); err != nil {
		t.Fatal(err)
	}
	img := chatattach.Attachment{Kind: chatattach.KindImage, Name: "dot.png", MimeType: "image/png", Size: 12, Data: base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nxxxx"))}
	if _, err := f.m.Prompt(st.ChatID, "look", []chatattach.Attachment{img}); !errors.Is(err, chatattach.ErrNoImages) {
		t.Fatalf("queued image = %v", err)
	}
	if open := f.m.Chats(); len(open[0].Queue) != 0 {
		t.Fatalf("chats = %+v", open)
	}
	f.cancelTurn(t, st.ChatID)
}

func TestChatUnqueue(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "permission", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	img := chatattach.Attachment{Kind: chatattach.KindImage, Name: "dot.png", MimeType: "image/png", Size: 12, Data: base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nxxxx"))}
	id := f.queue(t, st.ChatID, "look", img)
	queued := f.events.wait(t, st.ChatID, kind(ChatQueueChanged)).Queue
	if len(queued) != 1 || len(queued[0].Attachments) != 1 ||
		queued[0].Attachments[0] != (ChatAttachmentInfo{Kind: "image", Name: "dot.png", MimeType: "image/png", Size: 12}) {
		t.Fatalf("queue = %+v", queued)
	}
	draft, err := f.m.Unqueue(st.ChatID, id)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Text != "look" || len(draft.Attachments) != 1 || draft.Attachments[0] != img {
		t.Fatalf("draft = %+v", draft)
	}
	if _, err := f.m.Unqueue(st.ChatID, id); !errors.Is(err, ErrQueuedNotFound) {
		t.Fatalf("second unqueue = %v", err)
	}
	if err := f.m.Respond(st.ChatID, asked.ID, "allow-once"); err != nil {
		t.Fatal(err)
	}
	f.events.wait(t, st.ChatID, kind(ChatTurnEnded))
	time.Sleep(100 * time.Millisecond)
	u, _ := f.events.of(st.ChatID)
	if got, want := story(u), []string{"start", "queue:1", "queue:0", "end:end_turn"}; !slices.Equal(got, want) {
		t.Fatalf("story = %q, want %q", got, want)
	}
}

func TestChatSendNowSteers(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "hold", nil); err != nil {
		t.Fatal(err)
	}
	id := f.queue(t, st.ChatID, "look at b.txt")
	// The steer may reach the agent before the prompt: it then answers
	// promptRequired and the message goes back to the queue. Repeat.
	for {
		if err := f.m.SendNow(chatCtx(t), st.ChatID, id); err != nil {
			t.Fatal(err)
		}
		if u, _ := f.events.of(st.ChatID); slices.ContainsFunc(u, kind(ChatSteered)) {
			break
		}
		if open := f.m.Chats(); len(open[0].Queue) != 1 || open[0].Queue[0].ID != id {
			t.Fatalf("not steered and not queued: %+v", open)
		}
		time.Sleep(20 * time.Millisecond)
	}
	u := f.turnEnds(t, st.ChatID, 1)
	got := story(u)
	if got[len(got)-2] != "steered:look at b.txt" || got[len(got)-1] != "end:end_turn" {
		t.Fatalf("story = %q", got)
	}
	if got := chunks(u); !slices.Equal(got, []string{"steered:look at b.txt"}) {
		t.Fatalf("chunks = %q", got)
	}
	time.Sleep(100 * time.Millisecond)
	if open := f.m.Chats(); open[0].Working || len(open[0].Queue) != 0 {
		t.Fatalf("chats = %+v", open)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatSendNowStartsPausedQueue(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "hang", nil); err != nil {
		t.Fatal(err)
	}
	f.queue(t, st.ChatID, "first")
	id := f.queue(t, st.ChatID, "stream 1")
	f.cancelTurn(t, st.ChatID)
	if err := f.m.SendNow(chatCtx(t), st.ChatID, id); err != nil {
		t.Fatal(err)
	}
	u := f.turnEnds(t, st.ChatID, 3)
	want := []string{"start", "queue:1", "queue:2", "end:cancelled", "queue:1", "start:stream 1", "end:end_turn", "queue:0", "start:first", "end:end_turn"}
	if got := story(u); !slices.Equal(got, want) {
		t.Fatalf("story = %q, want %q", got, want)
	}
	if err := f.m.SendNow(chatCtx(t), st.ChatID, id); !errors.Is(err, ErrQueuedNotFound) {
		t.Fatalf("sent twice = %v", err)
	}
}

func TestChatSendNowWithoutSteering(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_NO_STEER=1")
	st := f.start(t, nil)
	if !st.CanQueue || st.CanSteer {
		t.Fatalf("state = %+v", st)
	}
	if _, err := f.m.Prompt(st.ChatID, "hang", nil); err != nil {
		t.Fatal(err)
	}
	id := f.queue(t, st.ChatID, "later")
	if err := f.m.SendNow(chatCtx(t), st.ChatID, id); !errors.Is(err, ErrSteerUnsupported) {
		t.Fatalf("send now = %v", err)
	}
	if open := f.m.Chats(); len(open[0].Queue) != 1 || open[0].Queue[0].ID != id {
		t.Fatalf("chats = %+v", open)
	}
	f.cancelTurn(t, st.ChatID)
}

func TestChatPermissionAnswered(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "permission", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatPermissionAsked)).Permission
	if asked.ID == "" || asked.ToolCall.ToolCallID != "call_p" || len(asked.Options) != 3 {
		t.Fatalf("asked = %+v", asked)
	}
	// A reloaded window finds the waiting request in the chat's state.
	open := f.m.Chats()
	if len(open) != 1 || len(open[0].Permissions) != 1 || open[0].Permissions[0].ID != asked.ID ||
		open[0].Permissions[0].ToolCall.ToolCallID != "call_p" || len(open[0].Permissions[0].Options) != 3 {
		t.Fatalf("chats = %+v", open)
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
	if open := f.m.Chats(); len(open) != 1 || len(open[0].Permissions) != 0 {
		t.Fatalf("answered request still listed: %+v", open)
	}
}

func TestChatCancelAnswersPermission(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if _, err := f.m.Prompt(st.ChatID, "permission", nil); err != nil {
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
	if _, err := f.m.Prompt(st.ChatID, "permission", nil); err != nil {
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

// An agent that writes a message over the frame limit is stopped, and the
// chat ends with that error instead of staying live.
func TestChatOversizedMessageEndsChat(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	if end := f.turn(t, st.ChatID, "flood"); end.Error == "" {
		t.Fatalf("turn end = %+v", end)
	}
	exit := f.events.waitExit(t, st.ChatID)
	if exit.Reason != ChatExitCrashed || !strings.Contains(exit.Error, acp.ErrFrameTooLarge.Error()) {
		t.Fatalf("exit = %+v", exit)
	}
	f.events.checkSeq(t, st.ChatID)
	if _, ok := f.m.Owned(st.Session); ok {
		t.Fatal("the chat still owns its session")
	}
}

func TestChatAuthRequired(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	end := f.turn(t, st.ChatID, "auth")
	if len(end.AuthMethods) != 1 || end.AuthMethods[0].ID != "fake-login" || !strings.Contains(end.Error, "fake auth login") {
		t.Fatalf("turn end = %+v", end)
	}
	// A failed turn still asks for a rescan: the agent may have saved the prompt.
	select {
	case <-f.refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("no rescan after a failed turn")
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

// A session another Codex process holds is refused with a plain message, and
// the refused chat lets go of the session.
func TestChatSessionInUse(t *testing.T) {
	f := newChatFixture(t)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "in-use"}
	_, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref})
	var inUse *SessionInUseError
	if !errors.As(err, &inUse) || !errors.Is(err, ErrSessionInUse) || inUse.Agent != model.AgentClaude {
		t.Fatalf("start = %v", err)
	}
	if err.Error() != ErrSessionInUse.Error() {
		t.Fatalf("error = %q", err)
	}
	var agentErr *acp.Error
	if !errors.As(err, &agentErr) || agentErr.Reason() != acp.ReasonThreadActiveWriter {
		t.Fatalf("agent error = %v", agentErr)
	}
	// Other agent errors pass through unchanged.
	if (&acp.Error{Code: acp.CodeInvalidRequest, Message: "x", Data: json.RawMessage(`{"reason":"other"}`)}).Reason() != "other" ||
		acp.IsSessionInUse(&acp.Error{Code: acp.CodeInvalidRequest, Message: "x"}) {
		t.Fatal("reason misread")
	}
	// The session is free for the next try.
	ok := model.SessionRef{Agent: model.AgentClaude, ID: "free"}
	f.start(t, &ok)
	if _, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref}); !errors.Is(err, ErrSessionInUse) {
		t.Fatalf("retry = %v", err)
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
	f.turn(t, st.ChatID, "opened")
	u, _ := f.events.of(st.ChatID)
	if got := chunks(u); strings.Join(got, "|") != "hello there|session/load" {
		t.Fatalf("chunks = %q", got)
	}
}

// The command catalog sent during setup, or among a load's dropped replay,
// is in the state; each update replaces it, an empty one clears it, and none
// shows as a session update.
func TestChatCommands(t *testing.T) {
	names := func(cs []acp.AvailableCommand) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return strings.Join(out, "|")
	}
	stateOf := func(f *chatFixture, id string) ChatState {
		for _, st := range f.m.Chats() {
			if st.ChatID == id {
				return st
			}
		}
		t.Fatalf("no chat %s", id)
		return ChatState{}
	}
	for _, tc := range []struct {
		name   string
		env    []string
		resume *model.SessionRef
	}{
		{"new", nil, nil},
		{"load", []string{"ACPTEST_NO_RESUME=1"}, &model.SessionRef{Agent: model.AgentClaude, ID: "old-cmd"}},
	} {
		f := newChatFixture(t, tc.env...)
		st := f.start(t, tc.resume)
		if got := names(st.Commands); got != "plugin:cmd1|cmd2" {
			t.Fatalf("%s: commands = %q", tc.name, got)
		}
		if c := st.Commands[0]; c.Description != "Command 1" || c.Input == nil || c.Input.Hint != "[args]" {
			t.Fatalf("%s: command = %+v", tc.name, c)
		}
		if tc.resume != nil {
			f.turn(t, st.ChatID, "opened")
			if u, _ := f.events.of(st.ChatID); strings.Join(chunks(u), "|") != "session/load" {
				t.Fatalf("replayed history leaked: %q", chunks(u))
			}
		}
	}

	f := newChatFixture(t)
	st := f.start(t, nil)
	f.turn(t, st.ChatID, "commands 3")
	if got := names(stateOf(f, st.ChatID).Commands); got != "plugin:cmd1|cmd2|cmd3" {
		t.Fatalf("after update: %q", got)
	}
	f.turn(t, st.ChatID, "commands 0")
	if got := stateOf(f, st.ChatID).Commands; len(got) != 0 {
		t.Fatalf("an empty update kept %+v", got)
	}
	u, _ := f.events.of(st.ChatID)
	var sizes []int
	for _, e := range u {
		if e.Kind == ChatCommands {
			sizes = append(sizes, len(e.Commands))
		}
		if e.Kind == ChatSessionUpdate && e.Update.Kind == acp.UpdateAvailableCommands {
			t.Fatalf("catalog sent as a session update: %+v", e)
		}
	}
	if !slices.Equal(sizes, []int{2, 3, 0}) {
		t.Fatalf("commands events = %v", sizes)
	}
	f.events.checkSeq(t, st.ChatID)
}

// A saved session continues with session/resume when the agent offers it,
// else with session/load; an agent with neither refuses it plainly.
func TestChatContinuesSession(t *testing.T) {
	opened := func(t *testing.T, f *chatFixture, resume *model.SessionRef) string {
		t.Helper()
		st := f.start(t, resume)
		f.turn(t, st.ChatID, "opened")
		u, _ := f.events.of(st.ChatID)
		if err := f.m.Close(st.ChatID); err != nil {
			t.Fatal(err)
		}
		return strings.Join(chunks(u), "|")
	}
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "old-2"}
	if got := opened(t, newChatFixture(t), &ref); got != "session/resume" {
		t.Fatalf("with resume: chunks = %q", got)
	}
	if got := opened(t, newChatFixture(t, "ACPTEST_NO_RESUME=1"), &ref); got != "session/load" {
		t.Fatalf("with load only: chunks = %q", got)
	}
	if got := opened(t, newChatFixture(t, "ACPTEST_NO_RESUME=1", "ACPTEST_NO_LOAD=1"), nil); got != "session/new" {
		t.Fatalf("new session: chunks = %q", got)
	}

	f := newChatFixture(t, "ACPTEST_NO_RESUME=1", "ACPTEST_NO_LOAD=1")
	_, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Resume: &ref})
	if !errors.Is(err, ErrChatCannotContinue) {
		t.Fatalf("start = %v", err)
	}
	if _, ok := f.m.Owned(ref); ok || len(f.m.Chats()) != 0 {
		t.Fatalf("a refused chat stays: owned %v, chats %+v", ok, f.m.Chats())
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
			if _, err := f.m.Prompt(st.ChatID, text, nil); err != nil {
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
	if len(f.m.Chats()) != 0 || f.m.owners.len() != 0 {
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
	if len(f.m.Chats()) != 0 || f.m.owners.len() != 0 {
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

// The engine's own manage manager refuses sessions any chat in the process
// owns, and allows them again once the chat ends.
func TestEngineManageRefusesOwnedSession(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "0123abcd-0000-4000-8000-0000000000c7"}
	source := filepath.Join(claudeRoot, "projects", "p", ref.ID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, provider.Set{providertest.NewFake(model.AgentClaude, "Claude")})
	svc.Catalog().Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: ref, SourcePath: source, UpdatedAt: time.Now().Add(-48 * time.Hour)}}})
	events := &chatEvents{changed: make(chan struct{}, 1)}
	e := NewEngine(
		WithService(svc),
		WithCacheEnabled(false),
		WithRoots(paths.Roots{Config: filepath.Join(root, "config"), Claude: claudeRoot}),
		WithEmitter(events),
		WithChatLaunch(func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
			return acptest.Command(dir), nil
		}),
	)
	t.Cleanup(func() { _ = e.Close() })
	mgr, err := e.ensureManage()
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetConfig(manage.Config{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	blocked := func() string {
		t.Helper()
		p, err := e.PreviewDelete(context.Background(), []model.SessionRef{ref})
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("preview: %+v, %v", p, err)
		}
		return p.Items[0].Blocked
	}

	st, err := e.Chats().Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: t.TempDir(), Resume: &ref})
	if err != nil {
		t.Fatal(err)
	}
	if b := blocked(); !strings.Contains(b, "open in a chat") {
		t.Fatalf("owned session: blocked = %q", b)
	}
	if err := e.Chats().Close(st.ChatID); err != nil {
		t.Fatal(err)
	}
	events.waitExit(t, st.ChatID)
	// The fake provider has no live detector, so the session may stay
	// blocked for that; only the chat's block must be gone.
	if b := blocked(); strings.Contains(b, "chat") {
		t.Fatalf("released session: blocked = %q", b)
	}
}

func (f *chatFixture) startAsking(t *testing.T) ChatState {
	t.Helper()
	st, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentClaude, Dir: f.dir, Questions: true})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestChatQuestionAnswered(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "question claude", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	if asked.ID == "" || asked.ToolCallID != "toolu_ask" || asked.Message != "Please answer the following questions." || asked.Outcome != "" {
		t.Fatalf("asked = %+v", asked)
	}
	// The fields go to the frontend as a list, in the order sent.
	b, err := json.Marshal(asked)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"fields":[{"key":"question_0","type":"string","title":"Approach","description":"Which approach?","oneOf":[{"const":"Extend existing module (Recommended)"`; !strings.Contains(string(b), want) {
		t.Fatalf("question JSON = %s", b)
	}
	var keys []string
	for _, fl := range asked.Fields {
		keys = append(keys, fl.Key)
	}
	if strings.Join(keys, " ") != "question_0 question_0_custom question_1 question_1_custom" {
		t.Fatalf("keys = %q", keys)
	}
	// A reloaded window finds the waiting question in the chat's state.
	if open := f.m.Chats(); len(open) != 1 || len(open[0].Questions) != 1 || open[0].Questions[0].ID != asked.ID || len(open[0].Questions[0].Fields) != 4 {
		t.Fatalf("chats = %+v", open)
	}

	// Answers that do not fit are refused, and the question stays.
	for _, bad := range []map[string]any{
		{"question_0": "Something else"},
		{"question_1": []any{"Unit tests", "Unit tests"}},
		{"question_7": "x"},
	} {
		if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationAccept, bad); err == nil {
			t.Fatalf("answer %v was accepted", bad)
		}
	}
	if err := f.m.Answer(st.ChatID, asked.ID, "maybe", nil); err == nil {
		t.Fatal("an unknown action was accepted")
	}
	if err := f.m.Answer(st.ChatID, "question-x", acp.ElicitationDecline, nil); !errors.Is(err, ErrQuestionNotFound) {
		t.Fatalf("unknown question = %v", err)
	}
	if open := f.m.Chats(); len(open[0].Questions) != 1 {
		t.Fatalf("refused answers resolved the question: %+v", open[0].Questions)
	}

	// Values arrive as decoded JSON from the frontend.
	answer := map[string]any{"question_0": "New module", "question_1": []any{"Documentation", "Unit tests"}, "question_1_custom": ""}
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationAccept, answer); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationAccept, answer); !errors.Is(err, ErrQuestionNotFound) {
		t.Fatalf("second answer = %v", err)
	}
	if end := f.events.wait(t, st.ChatID, kind(ChatTurnEnded)); end.StopReason != acp.StopEndTurn {
		t.Fatalf("turn end = %+v", end)
	}
	resolved := f.events.wait(t, st.ChatID, kind(ChatQuestionResolved)).Question
	if resolved.ID != asked.ID || resolved.Outcome != acp.ElicitationAccept || resolved.Content["question_0"] != "New module" || resolved.Content["question_1_custom"] != nil {
		t.Fatalf("resolved = %+v", resolved)
	}
	u, _ := f.events.of(st.ChatID)
	if got := chunks(u); len(got) != 1 || got[0] != `accept {"question_0":"New module","question_1":["Documentation","Unit tests"]}` {
		t.Fatalf("chunks = %q", got)
	}
	if open := f.m.Chats(); len(open[0].Questions) != 0 {
		t.Fatalf("answered question still listed: %+v", open[0].Questions)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatQuestionDeclined(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "question codex", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	if len(asked.Required) != 1 || !strings.Contains(string(asked.Meta), "autoResolutionMs") {
		t.Fatalf("asked = %+v", asked)
	}
	// Required fields do not stop a skip.
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationAccept, map[string]any{"confirm_note": "Only the tests"}); err == nil {
		t.Fatal("a missing required answer was accepted")
	}
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationDecline, nil); err != nil {
		t.Fatal(err)
	}
	f.events.wait(t, st.ChatID, kind(ChatTurnEnded))
	if r := f.events.wait(t, st.ChatID, kind(ChatQuestionResolved)).Question; r.Outcome != acp.ElicitationDecline || r.Content != nil {
		t.Fatalf("resolved = %+v", r)
	}
	if u, _ := f.events.of(st.ChatID); !slices.Equal(chunks(u), []string{"decline"}) {
		t.Fatalf("chunks = %q", chunks(u))
	}
}

// Without the start flag the agent is not offered questions and falls back.
func TestChatQuestionsNeedTheFlag(t *testing.T) {
	f := newChatFixture(t)
	st := f.start(t, nil)
	f.turn(t, st.ChatID, "question claude")
	u, _ := f.events.of(st.ChatID)
	if slices.ContainsFunc(u, kind(ChatQuestionAsked)) || !slices.Equal(chunks(u), []string{"no elicitation"}) {
		t.Fatalf("updates = %+v", u)
	}
}

func TestChatCancelAnswersQuestion(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "question", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	if err := f.m.Cancel(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if end := f.events.wait(t, st.ChatID, kind(ChatTurnEnded)); end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end = %+v", end)
	}
	if r := f.events.wait(t, st.ChatID, kind(ChatQuestionResolved)).Question; r.ID != asked.ID || r.Outcome != acp.ElicitationCancel {
		t.Fatalf("resolved = %+v", r)
	}
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationDecline, nil); !errors.Is(err, ErrQuestionNotFound) {
		t.Fatalf("answer after cancel = %v", err)
	}
}

func TestChatCloseDuringQuestion(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "question", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	closed := make(chan struct{})
	go func() {
		_ = f.m.Close(st.ChatID)
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(15 * time.Second):
		t.Fatal("Close blocked on a pending question")
	}
	u, x := f.events.of(st.ChatID)
	if len(x) != 1 || x[0].Reason != ChatExitClosed {
		t.Fatalf("exits = %+v", x)
	}
	r := u[slices.IndexFunc(u, kind(ChatQuestionResolved))]
	if r.Question.ID != asked.ID || r.Question.Outcome != acp.ElicitationCancel {
		t.Fatalf("resolved = %+v", r.Question)
	}
	f.events.checkSeq(t, st.ChatID)
}

func TestChatQuestionWithdrawn(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "withdraw 200", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	if r := f.events.wait(t, st.ChatID, kind(ChatQuestionResolved)).Question; r.ID != asked.ID || r.Outcome != ChatQuestionWithdrawn {
		t.Fatalf("resolved = %+v", r)
	}
	if end := f.events.wait(t, st.ChatID, kind(ChatTurnEnded)); end.StopReason != acp.StopEndTurn {
		t.Fatalf("turn end = %+v", end)
	}
	if err := f.m.Answer(st.ChatID, asked.ID, acp.ElicitationDecline, nil); !errors.Is(err, ErrQuestionNotFound) {
		t.Fatalf("answer after withdrawal = %v", err)
	}
	if u, _ := f.events.of(st.ChatID); !slices.Equal(chunks(u), []string{"withdrawn"}) {
		t.Fatalf("chunks = %q", chunks(u))
	}
}

func TestChatAgentExitAnswersQuestion(t *testing.T) {
	f := newChatFixture(t)
	st := f.startAsking(t)
	if _, err := f.m.Prompt(st.ChatID, "askexit", nil); err != nil {
		t.Fatal(err)
	}
	asked := f.events.wait(t, st.ChatID, kind(ChatQuestionAsked)).Question
	if x := f.events.waitExit(t, st.ChatID); x.Reason != ChatExitCrashed {
		t.Fatalf("exit = %+v", x)
	}
	u, _ := f.events.of(st.ChatID)
	r := u[slices.IndexFunc(u, kind(ChatQuestionResolved))]
	if r.Question.ID != asked.ID || r.Question.Outcome != acp.ElicitationCancel {
		t.Fatalf("resolved = %+v", r.Question)
	}
	f.events.checkSeq(t, st.ChatID)
}
