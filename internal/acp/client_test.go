package acp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
)

func TestMain(m *testing.M) {
	acptest.Main()
	os.Exit(m.Run())
}

// recorder collects updates in arrival order.
type recorder struct {
	mu      sync.Mutex
	updates []acp.SessionNotification
}

func (r *recorder) add(n acp.SessionNotification) {
	r.mu.Lock()
	r.updates = append(r.updates, n)
	r.mu.Unlock()
}

func (r *recorder) take() []acp.SessionNotification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.updates
	r.updates = nil
	return out
}

func (r *recorder) texts(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, n := range r.take() {
		if c, err := n.Update.AsChunk(); err == nil {
			out = append(out, c.Content.Text)
		}
	}
	return out
}

func start(t *testing.T, opts acp.ClientOptions, env ...string) (*acp.Client, *recorder) {
	t.Helper()
	rec := &recorder{}
	if opts.OnUpdate == nil {
		opts.OnUpdate = rec.add
	}
	if opts.Grace == 0 {
		opts.Grace = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := acp.Start(ctx, acptest.Command(t.TempDir(), env...), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, rec
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

func newSession(t *testing.T, c *acp.Client) string {
	t.Helper()
	s, err := c.NewSession(ctx(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s.SessionID
}

func prompt(t *testing.T, c *acp.Client, sessionID, text string) (string, error) {
	t.Helper()
	return c.Prompt(ctx(t), sessionID, []acp.ContentBlock{acp.TextBlock(text)})
}

func waitExited(t *testing.T, c *acp.Client) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("agent did not exit")
	}
}

func TestClientLifecycle(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	info := c.Info()
	if info.ProtocolVersion != 1 || !info.AgentCapabilities.LoadSession || !info.AgentCapabilities.CanResume() {
		t.Fatalf("info = %+v", info)
	}
	if len(info.AuthMethods) != 1 || !strings.Contains(info.AuthMethods[0].Description, "fake auth login") {
		t.Fatalf("auth methods = %+v", info.AuthMethods)
	}

	s, err := c.NewSession(ctx(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.SessionID == "" || len(s.ConfigOptions) != 1 || string(s.ConfigOptions[0].CurrentValue) != `"ask"` {
		t.Fatalf("session = %+v", s)
	}

	opts, err := c.SetConfigOption(ctx(t), s.SessionID, "mode", "auto")
	if err != nil || string(opts[0].CurrentValue) != `"auto"` {
		t.Fatalf("set option = %+v, %v", opts, err)
	}
	if _, err := c.SetConfigOption(ctx(t), s.SessionID, "mode", "nope"); err == nil {
		t.Fatal("invalid option value accepted")
	}
	rec.take()

	stop, err := prompt(t, c, s.SessionID, "stream 50")
	if err != nil || stop != acp.StopEndTurn {
		t.Fatalf("prompt = %q, %v", stop, err)
	}
	texts := rec.texts(t)
	if len(texts) != 50 {
		t.Fatalf("got %d chunks before the reply, want 50", len(texts))
	}
	for i, s := range texts {
		if s != strconv.Itoa(i)+" " {
			t.Fatalf("chunk %d = %q", i, s)
		}
	}

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	waitExited(t, c)
	if c.ExitErr() != nil {
		t.Fatalf("exit = %v\n%s", c.ExitErr(), c.StderrTail())
	}
	if _, err := prompt(t, c, s.SessionID, "x"); !errors.Is(err, acp.ErrClosed) {
		t.Fatalf("prompt after close = %v", err)
	}
}

func TestToolCallAndPlanUpdates(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	id := newSession(t, c)
	if _, err := prompt(t, c, id, "tool"); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, n := range rec.take() {
		kinds = append(kinds, n.Update.Kind)
	}
	if got := strings.Join(kinds, ","); got != "tool_call,tool_call_update,plan" {
		t.Fatalf("kinds = %s", got)
	}
}

func TestLoadAndResume(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	s, err := c.LoadSession(ctx(t), "old-1", t.TempDir())
	if err != nil || s.SessionID != "old-1" {
		t.Fatalf("load = %+v, %v", s, err)
	}
	var kinds []string
	for _, n := range rec.take() {
		kinds = append(kinds, n.Update.Kind)
	}
	if got := strings.Join(kinds, ","); got != "user_message_chunk,agent_message_chunk" {
		t.Fatalf("replay kinds = %s", got)
	}

	s, err = c.ResumeSession(ctx(t), "old-2", t.TempDir())
	if err != nil || s.SessionID != "old-2" {
		t.Fatalf("resume = %+v, %v", s, err)
	}
	if n := len(rec.take()); n != 0 {
		t.Fatalf("resume replayed %d updates", n)
	}

	c2, _ := start(t, acp.ClientOptions{}, "ACPTEST_NO_RESUME=1")
	if _, err := c2.ResumeSession(ctx(t), "old-2", t.TempDir()); !errors.Is(err, acp.ErrNotSupported) {
		t.Fatalf("resume without the capability = %v", err)
	}
}

func TestPermissionAnswers(t *testing.T) {
	for _, tc := range []struct {
		name string
		pick func(acp.RequestPermissionRequest) acp.RequestPermissionResponse
		stop string
		text string
	}{
		{"allow once", byKind(acp.PermissionAllowOnce), acp.StopEndTurn, "allowed:allow-once"},
		{"reject", byKind(acp.PermissionRejectOnce), acp.StopEndTurn, "rejected:reject"},
		{"cancelled", func(acp.RequestPermissionRequest) acp.RequestPermissionResponse { return acp.Cancelled() }, acp.StopCancelled, ""},
		// An option the request did not offer is never sent.
		{"unknown option", func(acp.RequestPermissionRequest) acp.RequestPermissionResponse { return acp.Selected("bogus") }, acp.StopCancelled, ""},
		{"empty answer", func(acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			return acp.RequestPermissionResponse{}
		}, acp.StopCancelled, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked []acp.RequestPermissionRequest
			c, rec := start(t, acp.ClientOptions{OnPermission: func(_ context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
				asked = append(asked, req)
				return tc.pick(req)
			}})
			id := newSession(t, c)
			stop, err := prompt(t, c, id, "permission")
			if err != nil || stop != tc.stop {
				t.Fatalf("prompt = %q, %v", stop, err)
			}
			if len(asked) != 1 || asked[0].SessionID != id || asked[0].ToolCall.ToolCallID != "call_p" || len(asked[0].Options) != 3 {
				t.Fatalf("asked = %+v", asked)
			}
			texts := rec.texts(t)
			if tc.text == "" && len(texts) != 0 || tc.text != "" && (len(texts) != 1 || texts[0] != tc.text) {
				t.Fatalf("chunks = %q, want %q", texts, tc.text)
			}
		})
	}
}

func byKind(kind string) func(acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	return func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		for _, o := range req.Options {
			if o.Kind == kind {
				return acp.Selected(o.OptionID)
			}
		}
		return acp.Cancelled()
	}
}

func TestNoPermissionHandlerAnswersCancelled(t *testing.T) {
	c, _ := start(t, acp.ClientOptions{})
	stop, err := prompt(t, c, newSession(t, c), "permission")
	if err != nil || stop != acp.StopCancelled {
		t.Fatalf("prompt = %q, %v", stop, err)
	}
}

// Cancel answers a pending permission request cancelled, even when the
// handler ignores its context.
func TestCancelAnswersPendingPermission(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, _ := start(t, acp.ClientOptions{OnPermission: func(_ context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		close(waiting)
		<-release // a user who never answers
		return acp.Selected(req.Options[0].OptionID)
	}})
	id := newSession(t, c)
	result := make(chan string, 1)
	go func() {
		stop, err := prompt(t, c, id, "permission")
		if err != nil {
			t.Error(err)
		}
		result <- stop
	}()
	<-waiting
	if err := c.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if stop := <-result; stop != acp.StopCancelled {
		t.Fatalf("stop = %q", stop)
	}
}

func TestCancelOnlyAffectsItsSession(t *testing.T) {
	var mu sync.Mutex
	ctxs := map[string]context.Context{}
	waiting := make(chan struct{}, 2)
	c, _ := start(t, acp.ClientOptions{OnPermission: func(ctx context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		mu.Lock()
		ctxs[req.SessionID] = ctx
		mu.Unlock()
		waiting <- struct{}{}
		<-ctx.Done()
		return acp.Cancelled()
	}})
	a, b := newSession(t, c), newSession(t, c)
	done := make(chan struct{}, 2)
	for _, id := range []string{a, b} {
		go func() {
			_, _ = prompt(t, c, id, "permission")
			done <- struct{}{}
		}()
	}
	<-waiting
	<-waiting
	if err := c.Cancel(a); err != nil {
		t.Fatal(err)
	}
	<-done
	mu.Lock()
	aErr, bErr := ctxs[a].Err(), ctxs[b].Err()
	mu.Unlock()
	if aErr == nil || bErr != nil {
		t.Fatalf("after cancelling a: a ctx %v, b ctx %v", aErr, bErr)
	}
	if err := c.Cancel(b); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestCancelTurn(t *testing.T) {
	c, _ := start(t, acp.ClientOptions{})
	id := newSession(t, c)
	result := make(chan string, 1)
	go func() {
		stop, _ := prompt(t, c, id, "hang")
		result <- stop
	}()
	// The cancel may reach the agent before the prompt; retry until it ends.
	for {
		if err := c.Cancel(id); err != nil {
			t.Fatal(err)
		}
		select {
		case stop := <-result:
			if stop != acp.StopCancelled {
				t.Fatalf("stop = %q", stop)
			}
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestCloseAnswersPendingPermission(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, _ := start(t, acp.ClientOptions{OnPermission: func(context.Context, acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		close(waiting)
		<-release
		return acp.Cancelled()
	}})
	id := newSession(t, c)
	result := make(chan error, 1)
	go func() {
		_, err := prompt(t, c, id, "permission")
		result <- err
	}()
	<-waiting
	closed := make(chan struct{})
	go func() {
		_ = c.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close blocked on a pending permission")
	}
	// The cancelled answer and stdin's close race the reply; either way the
	// turn does not succeed.
	if err := <-result; err == nil {
		t.Fatal("prompt succeeded after close")
	}
}

func TestAgentCrashMidTurn(t *testing.T) {
	c, _ := start(t, acp.ClientOptions{})
	_, err := prompt(t, c, newSession(t, c), "crash")
	var exit *acp.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("err = %v (%T)", err, err)
	}
	if !errors.Is(err, acp.ErrAgentGone) {
		t.Fatalf("err does not wrap ErrAgentGone: %v", err)
	}
	var status *exec.ExitError
	if !errors.As(exit.Exit, &status) || status.ExitCode() != 3 {
		t.Fatalf("exit = %v", exit.Exit)
	}
	if !strings.Contains(exit.Stderr, "fake agent crashed") || !strings.Contains(err.Error(), "fake agent crashed") {
		t.Fatalf("stderr = %q, err = %v", exit.Stderr, err)
	}
	waitExited(t, c)

	// Later calls report the exit the same way.
	if err := c.Cancel("x"); !errors.As(err, &exit) {
		t.Fatalf("cancel after exit = %v", err)
	}
}

func TestAuthRequired(t *testing.T) {
	c, _ := start(t, acp.ClientOptions{})
	_, err := prompt(t, c, newSession(t, c), "auth")
	if !acp.IsAuthRequired(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartFailures(t *testing.T) {
	_, err := acp.Start(ctx(t), acp.Command{Argv: []string{filepath.Join(t.TempDir(), "missing-agent")}}, acp.ClientOptions{})
	if err == nil {
		t.Fatal("missing agent started")
	}
	if _, err := acp.Start(ctx(t), acp.Command{}, acp.ClientOptions{}); err == nil {
		t.Fatal("empty command started")
	}
	_, err = acp.Start(ctx(t), acptest.Command(t.TempDir(), "ACPTEST_PROTOCOL=2"), acp.ClientOptions{Grace: time.Second})
	if err == nil || !strings.Contains(err.Error(), "protocol version 2") {
		t.Fatalf("protocol mismatch err = %v", err)
	}
}

func TestStderrTailIsBounded(t *testing.T) {
	c, _ := start(t, acp.ClientOptions{StderrBytes: 1024}, "ACPTEST_STDERR_BYTES=100000")
	newSession(t, c) // the agent wrote stderr before answering initialize
	tail := c.StderrTail()
	if len(tail) == 0 || len(tail) > 1024 || !strings.HasPrefix(tail, "stderr line ") {
		t.Fatalf("tail (%d bytes) = %.80q", len(tail), tail)
	}
}

func TestShutdownEscalates(t *testing.T) {
	for _, mode := range []string{"stubborn", "immortal"} {
		t.Run(mode, func(t *testing.T) {
			const grace = 300 * time.Millisecond
			c, _ := start(t, acp.ClientOptions{Grace: grace})
			if _, err := prompt(t, c, newSession(t, c), mode); err != nil {
				t.Fatal(err)
			}
			began := time.Now()
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			waitExited(t, c)
			took := time.Since(began)
			if took < grace {
				t.Fatalf("closed in %v; the agent should have ignored stdin", took)
			}
			if took > 10*grace+5*time.Second {
				t.Fatalf("close took %v", took)
			}
		})
	}
}

func TestCloseStopsDescendants(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	if _, err := prompt(t, c, newSession(t, c), "spawn"); err != nil {
		t.Fatal(err)
	}
	texts := rec.texts(t)
	if len(texts) != 1 || !strings.HasPrefix(texts[0], "child ") {
		t.Fatalf("chunks = %q", texts)
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(texts[0], "child "))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	waitExited(t, c)
	assertStopped(t, pid)
}

// An agent that exits by itself also takes its descendants with it.
func TestAgentExitStopsDescendants(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	id := newSession(t, c)
	if _, err := prompt(t, c, id, "spawn"); err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(rec.texts(t)[0], "child "))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prompt(t, c, id, "crash"); err == nil {
		t.Fatal("crash prompt succeeded")
	}
	waitExited(t, c)
	assertStopped(t, pid)
}

func TestQuestionsNeedOnElicitation(t *testing.T) {
	c, rec := start(t, acp.ClientOptions{})
	stop, err := prompt(t, c, newSession(t, c), "question")
	if err != nil || stop != acp.StopEndTurn {
		t.Fatalf("prompt = %q, %v", stop, err)
	}
	if texts := rec.texts(t); len(texts) != 1 || texts[0] != "no elicitation" {
		t.Fatalf("chunks = %q", texts)
	}
}

func TestQuestionAnswers(t *testing.T) {
	accept := func(content map[string]any) acp.CreateElicitationResponse {
		return acp.CreateElicitationResponse{Action: acp.ElicitationAccept, Content: content}
	}
	for _, tc := range []struct {
		name   string
		form   string
		answer acp.CreateElicitationResponse
		stop   string
		text   string
		asked  bool
	}{
		{
			"claude accept", "claude", accept(map[string]any{"question_0": "New module", "question_1": []string{"Unit tests"}, "question_1_custom": ""}),
			acp.StopEndTurn, `accept {"question_0":"New module","question_1":["Unit tests"]}`, true,
		},
		{
			"claude own answer", "claude", accept(map[string]any{"question_0_custom": "Rewrite it"}),
			acp.StopEndTurn, `accept {"question_0_custom":"Rewrite it"}`, true,
		},
		{
			"decline", "claude",
			acp.CreateElicitationResponse{Action: acp.ElicitationDecline, Content: map[string]any{"question_0": "New module"}},
			acp.StopEndTurn, "decline", true,
		},
		{"cancel", "claude", acp.ElicitationCancelled(), acp.StopCancelled, "cancel", true},
		{"unknown action", "claude", acp.CreateElicitationResponse{Action: "maybe"}, acp.StopCancelled, "cancel", true},
		// An answer that does not fit the form is never sent.
		{"not an option", "claude", accept(map[string]any{"question_0": "Something else"}), acp.StopCancelled, "cancel", true},
		{"unknown field", "claude", accept(map[string]any{"question_9": "x"}), acp.StopCancelled, "cancel", true},
		{
			"codex other with note", "codex", accept(map[string]any{"confirm": "Other", "confirm_note": "Only the tests"}),
			acp.StopEndTurn, `accept {"confirm":"Other","confirm_note":"Only the tests"}`, true,
		},
		{"codex missing required", "codex", accept(map[string]any{"confirm": "Yes"}), acp.StopCancelled, "cancel", true},
		{
			"opencode accept", "opencode", accept(map[string]any{"tests": []string{"unit", "lint"}, "retries": 2, "verbose": true}),
			acp.StopEndTurn, `accept {"retries":2,"tests":["unit","lint"],"verbose":true}`, true,
		},
		{"opencode too many", "opencode", accept(map[string]any{"tests": []string{"unit", "lint", "e2e"}, "retries": 2}), acp.StopCancelled, "cancel", true},
		{"opencode not whole", "opencode", accept(map[string]any{"tests": []string{"unit"}, "retries": 1.5}), acp.StopCancelled, "cancel", true},
		{"url mode refused", "url", acp.ElicitationCancelled(), acp.StopEndTurn, fmt.Sprintf("error %d", acp.CodeInvalidParams), false},
		{"unsupported format refused", "email", acp.ElicitationCancelled(), acp.StopEndTurn, fmt.Sprintf("error %d", acp.CodeInvalidParams), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked []acp.CreateElicitationRequest
			c, rec := start(t, acp.ClientOptions{OnElicitation: func(_ context.Context, req acp.CreateElicitationRequest) acp.CreateElicitationResponse {
				asked = append(asked, req)
				return tc.answer
			}})
			id := newSession(t, c)
			stop, err := prompt(t, c, id, "question "+tc.form)
			if err != nil || stop != tc.stop {
				t.Fatalf("prompt = %q, %v", stop, err)
			}
			if texts := rec.texts(t); len(texts) != 1 || texts[0] != tc.text {
				t.Fatalf("chunks = %q, want %q", texts, tc.text)
			}
			if !tc.asked {
				if len(asked) != 0 {
					t.Fatalf("asked = %+v", asked)
				}
				return
			}
			if len(asked) != 1 || asked[0].SessionID != id || asked[0].Mode != "form" || asked[0].RequestedSchema == nil {
				t.Fatalf("asked = %+v", asked)
			}
		})
	}
}

// The form reaches the callback with its fields in the order sent.
func TestQuestionFieldOrder(t *testing.T) {
	var got *acp.CreateElicitationRequest
	c, _ := start(t, acp.ClientOptions{OnElicitation: func(_ context.Context, req acp.CreateElicitationRequest) acp.CreateElicitationResponse {
		got = &req
		return acp.CreateElicitationResponse{Action: acp.ElicitationDecline}
	}})
	if _, err := prompt(t, c, newSession(t, c), "question claude"); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ToolCallID != "toolu_ask" || got.Message != "Please answer the following questions." {
		t.Fatalf("request = %+v", got)
	}
	var keys []string
	for _, f := range got.RequestedSchema.Properties {
		keys = append(keys, f.Key)
	}
	if want := "question_0 question_0_custom question_1 question_1_custom"; strings.Join(keys, " ") != want {
		t.Fatalf("keys = %q, want %q", keys, want)
	}
	if p, _ := got.RequestedSchema.Field("question_0"); len(p.OneOf) != 2 || p.OneOf[0].Description != "Least new code" {
		t.Fatalf("question_0 = %+v", p)
	}
}

// Cancel answers a pending question cancel, even when the handler ignores
// its context.
func TestCancelAnswersPendingQuestion(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, rec := start(t, acp.ClientOptions{OnElicitation: func(context.Context, acp.CreateElicitationRequest) acp.CreateElicitationResponse {
		close(waiting)
		<-release // a user who never answers
		return acp.CreateElicitationResponse{Action: acp.ElicitationAccept, Content: map[string]any{"question_0": "New module"}}
	}})
	id := newSession(t, c)
	result := make(chan string, 1)
	go func() {
		stop, err := prompt(t, c, id, "question")
		if err != nil {
			t.Error(err)
		}
		result <- stop
	}()
	<-waiting
	if err := c.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if stop := <-result; stop != acp.StopCancelled {
		t.Fatalf("stop = %q", stop)
	}
	if texts := rec.texts(t); len(texts) != 1 || texts[0] != "cancel" {
		t.Fatalf("chunks = %q", texts)
	}
}

func TestCloseAnswersPendingQuestion(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, _ := start(t, acp.ClientOptions{OnElicitation: func(context.Context, acp.CreateElicitationRequest) acp.CreateElicitationResponse {
		close(waiting)
		<-release
		return acp.ElicitationCancelled()
	}})
	id := newSession(t, c)
	result := make(chan error, 1)
	go func() {
		_, err := prompt(t, c, id, "question")
		result <- err
	}()
	<-waiting
	closed := make(chan struct{})
	go func() {
		_ = c.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close blocked on a pending question")
	}
	if err := <-result; err == nil {
		t.Fatal("prompt succeeded after close")
	}
}

// An agent that withdraws its question ends the callback's context, and the
// turn goes on.
func TestWithdrawnQuestion(t *testing.T) {
	ended := make(chan error, 1)
	c, rec := start(t, acp.ClientOptions{OnElicitation: func(ctx context.Context, _ acp.CreateElicitationRequest) acp.CreateElicitationResponse {
		<-ctx.Done()
		ended <- context.Cause(ctx)
		return acp.CreateElicitationResponse{Action: acp.ElicitationAccept, Content: map[string]any{"question_0": "New module"}}
	}})
	stop, err := prompt(t, c, newSession(t, c), "withdraw 100")
	if err != nil || stop != acp.StopEndTurn {
		t.Fatalf("prompt = %q, %v", stop, err)
	}
	select {
	case err := <-ended:
		if !errors.Is(err, acp.ErrWithdrawn) {
			t.Fatalf("ctx cause = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the withdrawn question's context did not end")
	}
	if texts := rec.texts(t); len(texts) != 1 || texts[0] != "withdrawn" {
		t.Fatalf("chunks = %q", texts)
	}
}
