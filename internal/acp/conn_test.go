package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAgent is the agent end of a Conn over two in-process pipes.
type fakeAgent struct {
	t   *testing.T
	in  *bufio.Reader  // what the client writes
	out io.WriteCloser // what the client reads
	mu  sync.Mutex
}

func newPair(t *testing.T, opts ConnOptions) (*Conn, *fakeAgent) {
	t.Helper()
	clientRead, agentWrite := io.Pipe()
	agentRead, clientWrite := io.Pipe()
	c := NewConn(clientRead, clientWrite, opts)
	t.Cleanup(func() {
		_ = c.Close()
		_ = agentWrite.Close()
		_ = agentRead.Close()
	})
	return c, &fakeAgent{t: t, in: bufio.NewReader(agentRead), out: agentWrite}
}

// recv reads the client's next message.
func (a *fakeAgent) recv() message {
	a.t.Helper()
	line, err := a.in.ReadBytes('\n')
	if err != nil {
		a.t.Fatalf("agent read: %v", err)
	}
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		a.t.Fatalf("agent got invalid JSON %q: %v", line, err)
	}
	if m.JSONRPC != "2.0" {
		a.t.Fatalf("jsonrpc = %q in %q", m.JSONRPC, line)
	}
	return m
}

// recvLine reads the client's next message as raw text.
func (a *fakeAgent) recvLine() string {
	a.t.Helper()
	line, err := a.in.ReadString('\n')
	if err != nil {
		a.t.Fatalf("agent read: %v", err)
	}
	return strings.TrimSpace(line)
}

func (a *fakeAgent) send(line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := io.WriteString(a.out, line+"\n"); err != nil {
		a.t.Errorf("agent write: %v", err)
	}
}

func waitDone(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not end")
	}
}

func TestCallRoundTrip(t *testing.T) {
	c, agent := newPair(t, ConnOptions{})
	done := make(chan error, 1)
	var got InitializeResponse
	go func() {
		done <- c.Call(context.Background(), MethodInitialize,
			InitializeRequest{ProtocolVersion: ProtocolVersion, ClientInfo: &Implementation{Name: "agent-sessions"}}, &got)
	}()

	req := agent.recv()
	if req.Method != MethodInitialize {
		t.Fatalf("method = %q", req.Method)
	}
	if want := `{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"agent-sessions"}}`; string(req.Params) != want {
		t.Fatalf("params = %s, want %s", req.Params, want)
	}
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"resume":{},"list":{}}},"authMethods":[{"id":"opencode-login","name":"Login","description":"Run opencode auth login"}]}}`, req.ID))

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !got.AgentCapabilities.LoadSession || !got.AgentCapabilities.CanResume() {
		t.Fatalf("capabilities = %+v", got.AgentCapabilities)
	}
	if len(got.AuthMethods) != 1 || got.AuthMethods[0].Description != "Run opencode auth login" {
		t.Fatalf("auth methods = %+v", got.AuthMethods)
	}
}

func TestCallReturnsAgentError(t *testing.T) {
	c, agent := newPair(t, ConnOptions{})
	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), MethodSessionPrompt, PromptRequest{SessionID: "s", Prompt: []ContentBlock{TextBlock("hi")}}, nil)
	}()
	req := agent.recv()
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"Authentication required: provider authentication required","data":{}}}`, req.ID))

	err := <-done
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeAuthRequired {
		t.Fatalf("err = %v", err)
	}
	if !IsAuthRequired(err) {
		t.Fatal("IsAuthRequired = false")
	}
}

// The agent's request IDs start at 0 (seen with all three adapters) and must
// come back exactly as sent, as must string IDs.
func TestAgentRequestIDsEchoedExactly(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	_, agent := newPair(t, ConnOptions{
		OnRequest: func(_ context.Context, method string, params json.RawMessage) (any, error) {
			mu.Lock()
			methods = append(methods, method)
			mu.Unlock()
			var p RequestPermissionRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
			}
			return Selected(p.Options[0].OptionID), nil
		},
	})

	for _, id := range []string{`0`, `"perm-7"`} {
		agent.send(`{"jsonrpc":"2.0","id":` + id + `,"method":"session/request_permission","params":{"sessionId":"s","toolCall":{"toolCallId":"toolu_1"},"options":[{"optionId":"allow-once","name":"Yes","kind":"allow_once"}]}}`)
		want := `{"jsonrpc":"2.0","id":` + id + `,"result":{"outcome":{"outcome":"selected","optionId":"allow-once"}}}`
		if got := agent.recvLine(); got != want {
			t.Fatalf("reply = %s\nwant    %s", got, want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) != 2 {
		t.Fatalf("handler calls = %v", methods)
	}
}

func TestRequestsWithoutHandlerGetMethodNotFound(t *testing.T) {
	_, agent := newPair(t, ConnOptions{})
	agent.send(`{"jsonrpc":"2.0","id":3,"method":"fs/read_text_file","params":{"path":"/etc/passwd"}}`)
	m := agent.recv()
	if string(m.ID) != "3" || m.Error == nil || m.Error.Code != CodeMethodNotFound {
		t.Fatalf("reply = %+v (error %+v)", m, m.Error)
	}
}

func TestHandlerErrors(t *testing.T) {
	_, agent := newPair(t, ConnOptions{
		OnRequest: func(_ context.Context, method string, _ json.RawMessage) (any, error) {
			switch method {
			case "terminal/create":
				return nil, MethodNotFound(method)
			case "x/nil":
				return nil, nil
			}
			return nil, errors.New("boom")
		},
	})

	agent.send(`{"jsonrpc":"2.0","id":1,"method":"terminal/create","params":{}}`)
	if m := agent.recv(); m.Error == nil || m.Error.Code != CodeMethodNotFound {
		t.Fatalf("terminal/create reply error = %+v", m.Error)
	}
	agent.send(`{"jsonrpc":"2.0","id":2,"method":"x/fail","params":{}}`)
	if m := agent.recv(); m.Error == nil || m.Error.Code != CodeInternalError || m.Error.Message != "boom" {
		t.Fatalf("x/fail reply error = %+v", m.Error)
	}
	agent.send(`{"jsonrpc":"2.0","id":3,"method":"x/nil"}`)
	if got, want := agent.recvLine(), `{"jsonrpc":"2.0","id":3,"result":null}`; got != want {
		t.Fatalf("x/nil reply = %s, want %s", got, want)
	}
}

// Updates are delivered in order, and before the reply that follows them.
func TestNotificationsInOrderBeforeReply(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	c, agent := newPair(t, ConnOptions{
		OnNotification: func(method string, params json.RawMessage) {
			var n SessionNotification
			if err := json.Unmarshal(params, &n); err != nil {
				t.Errorf("notification %s: %v", method, err)
				return
			}
			chunk, err := n.Update.AsChunk()
			if err != nil {
				t.Errorf("chunk: %v", err)
				return
			}
			mu.Lock()
			seen = append(seen, chunk.Content.Text)
			mu.Unlock()
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), MethodSessionPrompt, PromptRequest{SessionID: "s"}, nil)
	}()
	req := agent.recv()
	const n = 200
	for i := range n {
		agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","messageId":"msg_1","content":{"type":"text","text":"%d"}}}}`, i))
	}
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}`, req.ID))
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != n {
		t.Fatalf("saw %d updates before the reply, want %d", len(seen), n)
	}
	for i, s := range seen {
		if s != fmt.Sprint(i) {
			t.Fatalf("update %d = %q", i, s)
		}
	}
}

// Many calls in both directions at once; replies come back in reverse order.
func TestConcurrentCallsBothDirections(t *testing.T) {
	c, agent := newPair(t, ConnOptions{
		OnRequest: func(ctx context.Context, _ string, params json.RawMessage) (any, error) {
			return json.RawMessage(params), nil
		},
	})
	const n = 50

	errs := make(chan error, n)
	for i := range n {
		go func() {
			var res struct{ N int }
			if err := c.Call(context.Background(), "x/echo", map[string]int{"n": i}, &res); err != nil {
				errs <- err
				return
			}
			if res.N != i*10 {
				errs <- fmt.Errorf("call %d got %d", i, res.N)
				return
			}
			errs <- nil
		}()
	}

	// Read the n requests, interleaved with the replies to the agent's own
	// requests, then answer the client in reverse order.
	var reqs []message
	for i := range n {
		agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"x/agent","params":{"i":%d}}`, 1000+i, i))
	}
	replies := 0
	for len(reqs) < n || replies < n {
		m := agent.recv()
		if m.Method != "" {
			reqs = append(reqs, m)
			continue
		}
		replies++
	}
	for i := len(reqs) - 1; i >= 0; i-- {
		var p struct{ N int }
		if err := json.Unmarshal(reqs[i].Params, &p); err != nil {
			t.Fatal(err)
		}
		agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"n":%d}}`, reqs[i].ID, p.N*10))
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMalformedMessagesAreReportedAndSkipped(t *testing.T) {
	var mu sync.Mutex
	var reported []error
	c, agent := newPair(t, ConnOptions{
		OnProtocolError: func(err error) {
			mu.Lock()
			reported = append(reported, err)
			mu.Unlock()
		},
	})
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), "x/ping", nil, nil) }()
	req := agent.recv()

	agent.send(`not json`)
	agent.send(``)
	agent.send(`   `)
	agent.send(`{"jsonrpc":"2.0"}`)
	agent.send(`{"jsonrpc":"2.0","id":999,"result":{}}`)
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, req.ID))

	if err := <-done; err != nil {
		t.Fatalf("call after malformed lines: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 3 {
		t.Fatalf("reported %d errors, want 3: %v", len(reported), reported)
	}
	if c.Err() != nil {
		t.Fatalf("connection ended: %v", c.Err())
	}
}

func TestOversizedFrameEndsConnection(t *testing.T) {
	c, agent := newPair(t, ConnOptions{MaxFrameBytes: 1024})
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), "x/ping", nil, nil) }()
	agent.recv()

	go agent.send(`{"jsonrpc":"2.0","method":"session/update","params":{"text":"` + strings.Repeat("x", 4096) + `"}}`)
	if err := <-done; !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("pending call err = %v, want ErrFrameTooLarge", err)
	}
	waitDone(t, c)
	if !errors.Is(c.Err(), ErrFrameTooLarge) {
		t.Fatalf("Err = %v", c.Err())
	}
	if err := c.Call(context.Background(), "x/ping", nil, nil); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("call after end = %v", err)
	}
}

func TestFrameAtLimitIsAccepted(t *testing.T) {
	line := `{"jsonrpc":"2.0","method":"x/n","params":"` + strings.Repeat("y", 100) + `"}`
	got := make(chan string, 1)
	_, agent := newPair(t, ConnOptions{
		MaxFrameBytes:  len(line),
		OnNotification: func(_ string, p json.RawMessage) { got <- string(p) },
	})
	agent.send(line)
	select {
	case p := <-got:
		if len(p) != 102 {
			t.Fatalf("params length %d", len(p))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification at the limit not delivered")
	}
}

func TestAgentExitFailsPendingCalls(t *testing.T) {
	c, agent := newPair(t, ConnOptions{})
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), MethodSessionPrompt, nil, nil) }()
	agent.recv()
	_ = agent.out.Close()

	if err := <-done; !errors.Is(err, ErrAgentGone) {
		t.Fatalf("pending call err = %v, want ErrAgentGone", err)
	}
	waitDone(t, c)
	if err := c.Notify(MethodSessionCancel, CancelNotification{SessionID: "s"}); !errors.Is(err, ErrAgentGone) {
		t.Fatalf("notify after exit = %v", err)
	}
}

// A last message without a trailing newline is still delivered at EOF.
func TestLastLineWithoutNewline(t *testing.T) {
	got := make(chan string, 1)
	c, agent := newPair(t, ConnOptions{OnNotification: func(m string, _ json.RawMessage) { got <- m }})
	_, _ = io.WriteString(agent.out, `{"jsonrpc":"2.0","method":"x/last"}`)
	_ = agent.out.Close()
	if m := <-got; m != "x/last" {
		t.Fatalf("method = %q", m)
	}
	waitDone(t, c)
}

func TestCallContextCancellation(t *testing.T) {
	var mu sync.Mutex
	var reported []error
	c, agent := newPair(t, ConnOptions{OnProtocolError: func(err error) {
		mu.Lock()
		reported = append(reported, err)
		mu.Unlock()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Call(ctx, "x/slow", nil, nil) }()
	req := agent.recv()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	// The late reply is reported, and the connection keeps working.
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, req.ID))
	go func() { done <- c.Call(context.Background(), "x/ping", nil, nil) }()
	next := agent.recv()
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, next.ID))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "unknown request id "+string(req.ID)) {
		t.Fatalf("reported = %v", reported)
	}
}

func TestCloseFailsPendingAndCancelsHandlers(t *testing.T) {
	handlerStarted := make(chan struct{})
	handlerDone := make(chan error, 1)
	c, agent := newPair(t, ConnOptions{
		OnRequest: func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
			close(handlerStarted)
			<-ctx.Done() // a permission request waiting for the user
			handlerDone <- ctx.Err()
			return Cancelled(), nil
		},
	})
	callDone := make(chan error, 1)
	go func() { callDone <- c.Call(context.Background(), MethodSessionPrompt, nil, nil) }()
	agent.recv()
	agent.send(`{"jsonrpc":"2.0","id":0,"method":"session/request_permission","params":{}}`)
	<-handlerStarted

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-callDone; !errors.Is(err, ErrClosed) {
		t.Fatalf("pending call err = %v, want ErrClosed", err)
	}
	if err := <-handlerDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("handler ctx err = %v", err)
	}
	// Closing the connection closes the agent's stdin.
	if _, err := agent.in.ReadBytes('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("agent read after close = %v, want EOF", err)
	}
	if !errors.Is(c.Err(), ErrClosed) {
		t.Fatalf("Err = %v", c.Err())
	}
	// Messages still arriving are drained, not delivered.
	agent.send(`{"jsonrpc":"2.0","method":"session/update","params":{}}`)
	_ = agent.out.Close()
	if err := c.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("second close: %v", err)
	}
}
