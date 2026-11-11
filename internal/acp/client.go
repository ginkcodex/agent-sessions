package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/proctree"
)

// Defaults for ClientOptions.
const (
	DefaultStderrBytes = 64 << 10
	DefaultGrace       = 3 * time.Second
)

// ErrNotSupported is returned for a session method the agent does not
// advertise.
var ErrNotSupported = errors.New("acp: the agent does not support this")

// Command is how to start an agent's ACP server.
type Command struct {
	Argv []string
	Dir  string
	// Env is added to the app's environment, as KEY=VALUE.
	Env []string
}

// ClientOptions configures a Client. Every field is optional.
type ClientOptions struct {
	ClientInfo Implementation
	// OnUpdate receives session/update notifications in order. It runs on
	// the connection's read loop and must not block for long.
	OnUpdate func(SessionNotification)
	// OnPermission asks the user. It returns Selected or Cancelled; any
	// answer naming an option the request did not offer is sent as
	// Cancelled. ctx ends when the turn is cancelled or the client closes,
	// and the request is then answered Cancelled whatever OnPermission
	// does. When nil, every request is answered Cancelled.
	OnPermission func(ctx context.Context, req RequestPermissionRequest) RequestPermissionResponse
	// OnElicitation shows a form and returns the user's answer. Setting it
	// advertises form elicitation, which is how agents ask the user
	// questions. Only session-scoped forms whose schema is Supported
	// reach it; others get invalid-params. ctx ends when the turn is
	// cancelled, the client closes or the agent withdraws the form
	// (context.Cause is then ErrWithdrawn), and the form is then answered
	// cancel. An accepted answer that does not Validate is sent as cancel.
	OnElicitation func(ctx context.Context, req CreateElicitationRequest) CreateElicitationResponse
	// OnProtocolError reports messages that were skipped.
	OnProtocolError func(error)
	// StderrBytes is how much of the agent's stderr is kept.
	StderrBytes int
	// Grace is how long each shutdown step waits: after stdin is closed,
	// and after SIGTERM, before the process tree is killed.
	Grace time.Duration
}

// Client drives one agent process over ACP.
type Client struct {
	opts   ClientOptions
	cmd    *exec.Cmd
	conn   *Conn
	stdout *os.File
	stderr *tailBuffer
	info   InitializeResponse

	treeMu   sync.Mutex
	tree     *proctree.Tree
	released bool

	exited  chan struct{}
	waitErr error // set before exited is closed

	mu       sync.Mutex
	closing  bool
	perms    map[*permWait]struct{}
	handlers sync.WaitGroup

	closeOnce sync.Once
}

type permWait struct {
	sessionID string
	cancel    context.CancelFunc
}

// ExitError is the error of a call whose agent went away.
type ExitError struct {
	Err    error  // why the connection ended
	Exit   error  // the process's exit error; nil for exit status 0
	Stderr string // the end of the agent's stderr
}

func (e *ExitError) Error() string {
	msg := "acp: agent exited"
	if e.Exit != nil {
		msg += " (" + e.Exit.Error() + ")"
	}
	if line := lastLine(e.Stderr); line != "" {
		msg += ": " + line
	}
	return msg
}

func (e *ExitError) Unwrap() error { return e.Err }

// Start starts the agent and initializes the connection. ctx bounds the
// start and the initialize call; the agent runs until Close.
func Start(ctx context.Context, command Command, opts ClientOptions) (*Client, error) {
	if len(command.Argv) == 0 {
		return nil, errors.New("acp: empty agent command")
	}
	if opts.StderrBytes <= 0 {
		opts.StderrBytes = DefaultStderrBytes
	}
	if opts.Grace <= 0 {
		opts.Grace = DefaultGrace
	}
	c := &Client{
		opts:   opts,
		stderr: newTailBuffer(opts.StderrBytes),
		exited: make(chan struct{}),
		perms:  map[*permWait]struct{}{},
	}

	cmd := exec.Command(command.Argv[0], command.Argv[1:]...)
	cmd.Dir = command.Dir
	if len(command.Env) > 0 {
		cmd.Env = append(os.Environ(), command.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// The client owns the read end of stdout, so the last messages are read
	// even after the agent has exited (exec's StdoutPipe closes on Wait).
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stdout = outW
	cmd.Stderr = c.stderr
	// Bounds stderr draining when a descendant keeps the pipe open.
	cmd.WaitDelay = opts.Grace
	tree, err := proctree.Start(cmd)
	_ = outW.Close()
	if err != nil {
		_ = outR.Close()
		_ = stdin.Close()
		return nil, fmt.Errorf("acp: start %s: %w", command.Argv[0], err)
	}
	c.cmd, c.tree, c.stdout = cmd, tree, outR
	c.conn = NewConn(outR, stdin, ConnOptions{
		OnRequest:       c.handleRequest,
		OnNotification:  c.handleNotification,
		OnProtocolError: opts.OnProtocolError,
	})
	go c.watch()

	info := opts.ClientInfo
	if info.Name == "" {
		info.Name = "agent-sessions"
	}
	var resp InitializeResponse
	init := InitializeRequest{ProtocolVersion: ProtocolVersion, ClientInfo: &info}
	if opts.OnElicitation != nil {
		init.ClientCapabilities.Elicitation = &ElicitationCapabilities{Form: &struct{}{}}
	}
	err = c.conn.Call(ctx, MethodInitialize, init, &resp)
	if err == nil && resp.ProtocolVersion != ProtocolVersion {
		err = fmt.Errorf("acp: the agent speaks protocol version %d, not %d", resp.ProtocolVersion, ProtocolVersion)
	}
	if err != nil {
		err = c.wrap(err)
		_ = c.Close()
		return nil, err
	}
	c.info = resp
	return c, nil
}

// Info is the agent's initialize reply.
func (c *Client) Info() InitializeResponse { return c.info }

// Done is closed when the agent process has exited.
func (c *Client) Done() <-chan struct{} { return c.exited }

// ExitErr is the process's exit error once Done is closed; nil for status 0.
func (c *Client) ExitErr() error {
	select {
	case <-c.exited:
		return c.waitErr
	default:
		return nil
	}
}

// StderrTail is the end of the agent's stderr.
func (c *Client) StderrTail() string { return c.stderr.String() }

// NewSession starts a session in cwd.
func (c *Client) NewSession(ctx context.Context, cwd string) (SessionSetup, error) {
	var s SessionSetup
	err := c.call(ctx, MethodSessionNew, NewSessionRequest{Cwd: cwd}, &s)
	if err == nil && s.SessionID == "" {
		err = errors.New("acp: session/new returned no sessionId")
	}
	return s, err
}

// LoadSession opens an existing session. The agent replays its history as
// updates before the call returns.
func (c *Client) LoadSession(ctx context.Context, sessionID, cwd string) (SessionSetup, error) {
	if !c.info.AgentCapabilities.LoadSession {
		return SessionSetup{}, ErrNotSupported
	}
	return c.openSession(ctx, MethodSessionLoad, sessionID, cwd)
}

// ResumeSession opens an existing session without replaying its history.
func (c *Client) ResumeSession(ctx context.Context, sessionID, cwd string) (SessionSetup, error) {
	if !c.info.AgentCapabilities.CanResume() {
		return SessionSetup{}, ErrNotSupported
	}
	return c.openSession(ctx, MethodSessionResume, sessionID, cwd)
}

func (c *Client) openSession(ctx context.Context, method, sessionID, cwd string) (SessionSetup, error) {
	var s SessionSetup
	if err := c.call(ctx, method, LoadSessionRequest{SessionID: sessionID, Cwd: cwd}, &s); err != nil {
		return SessionSetup{}, err
	}
	if s.SessionID == "" {
		s.SessionID = sessionID
	}
	return s, nil
}

// SetConfigOption changes a session option and returns the options as the
// agent now reports them.
func (c *Client) SetConfigOption(ctx context.Context, sessionID, configID string, value any) ([]ConfigOption, error) {
	var r SetConfigOptionResponse
	err := c.call(ctx, MethodSessionSetConfigOption, SetConfigOptionRequest{SessionID: sessionID, ConfigID: configID, Value: value}, &r)
	return r.ConfigOptions, err
}

// Prompt runs one turn and returns its stop reason. Updates arrive through
// OnUpdate meanwhile. Cancelling ctx stops waiting but not the turn; Cancel
// stops the turn.
func (c *Client) Prompt(ctx context.Context, sessionID string, prompt []ContentBlock) (string, error) {
	var r PromptResponse
	if err := c.call(ctx, MethodSessionPrompt, PromptRequest{SessionID: sessionID, Prompt: prompt}, &r); err != nil {
		return "", err
	}
	return r.StopReason, nil
}

// Cancel asks the agent to stop the session's turn and answers the
// session's pending permission requests Cancelled.
func (c *Client) Cancel(sessionID string) error {
	c.cancelPermissions(func(w *permWait) bool { return w.sessionID == sessionID })
	return c.wrap(c.conn.Notify(MethodSessionCancel, CancelNotification{SessionID: sessionID}))
}

// Close stops the agent and waits for it to exit. Pending permission
// requests are answered Cancelled first; then stdin is closed, then the tree
// is sent SIGTERM, then killed, each after the grace period.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closing = true
		c.mu.Unlock()
		c.cancelPermissions(func(*permWait) bool { return true })
		c.waitHandlers(c.opts.Grace)
		_ = c.conn.Close()
		if c.waitExit(c.opts.Grace) {
			return
		}
		c.signalTree((*proctree.Tree).Interrupt)
		if c.waitExit(c.opts.Grace) {
			return
		}
		c.signalTree((*proctree.Tree).Kill)
		<-c.exited
	})
	return nil
}

func (c *Client) waitExit(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.exited:
		return true
	case <-t.C:
		return false
	}
}

func (c *Client) waitHandlers(d time.Duration) {
	done := make(chan struct{})
	go func() {
		c.handlers.Wait()
		close(done)
	}()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	}
}

func (c *Client) signalTree(f func(*proctree.Tree) error) {
	c.treeMu.Lock()
	defer c.treeMu.Unlock()
	if !c.released {
		_ = f(c.tree)
	}
}

// watch waits for the agent to exit, then stops what it left behind.
func (c *Client) watch() {
	err := c.cmd.Wait()
	c.signalTree((*proctree.Tree).Kill) // descendants that outlived the agent
	c.waitErr = err
	close(c.exited)
	c.cancelPermissions(func(*permWait) bool { return true })

	// Let the reader take what the agent wrote before exiting, then end it
	// even if something still holds the pipe.
	t := time.NewTimer(c.opts.Grace)
	select {
	case <-c.conn.Done():
	case <-t.C:
	}
	t.Stop()
	_ = c.stdout.Close()

	c.treeMu.Lock()
	c.released = true
	_ = c.tree.Release()
	c.treeMu.Unlock()
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	return c.wrap(c.conn.Call(ctx, method, params, result))
}

// wrap turns the error of a connection that ended under the client into an
// ExitError carrying the exit status and stderr.
func (c *Client) wrap(err error) error {
	if err == nil {
		return nil
	}
	var agentErr *Error
	if errors.Is(err, ErrClosed) || errors.As(err, &agentErr) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Either the connection ended, or a write failed because the agent is
	// gone (exec closes stdin once the process exits).
	if cerr := c.conn.Err(); cerr == nil || !errors.Is(err, cerr) {
		select {
		case <-c.exited:
			err = fmt.Errorf("%w: %w", ErrAgentGone, err)
		default:
			return err
		}
	}
	c.waitExit(c.opts.Grace)
	return &ExitError{Err: err, Exit: c.ExitErr(), Stderr: c.StderrTail()}
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	if method != MethodSessionUpdate || c.opts.OnUpdate == nil {
		return
	}
	var n SessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		if c.opts.OnProtocolError != nil {
			c.opts.OnProtocolError(fmt.Errorf("acp: invalid session/update: %w", err))
		}
		return
	}
	c.opts.OnUpdate(n)
}

// handleRequest answers the agent. Permission requests and, with
// OnElicitation, forms are handled; without the fs and terminal
// capabilities, everything else is method-not-found.
func (c *Client) handleRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case MethodRequestPermission:
		var req RequestPermissionRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
		}
		return c.askPermission(ctx, req), nil
	case MethodElicitationCreate:
		if c.opts.OnElicitation == nil {
			return nil, MethodNotFound(method)
		}
		var req CreateElicitationRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
		}
		switch {
		case req.Mode != "form":
			return nil, &Error{Code: CodeInvalidParams, Message: fmt.Sprintf("elicitation mode %q is not supported", req.Mode)}
		case req.SessionID == "":
			return nil, &Error{Code: CodeInvalidParams, Message: "only forms of a session are shown"}
		}
		if err := req.RequestedSchema.Supported(); err != nil {
			return nil, &Error{Code: CodeInvalidParams, Message: "the form cannot be shown: " + err.Error()}
		}
		return c.askForm(ctx, req), nil
	}
	return nil, MethodNotFound(method)
}

// wait registers a wait for the user on sessionID, which Cancel and Close
// end. ok is false when the client is closing.
func (c *Client) wait(ctx context.Context, sessionID string) (wctx context.Context, done func(), ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return nil, nil, false
	}
	wctx, cancel := context.WithCancel(ctx)
	w := &permWait{sessionID: sessionID, cancel: cancel}
	c.perms[w] = struct{}{}
	c.handlers.Add(1)
	return wctx, func() {
		c.mu.Lock()
		delete(c.perms, w)
		c.mu.Unlock()
		cancel()
		c.handlers.Done()
	}, true
}

func (c *Client) askForm(ctx context.Context, req CreateElicitationRequest) CreateElicitationResponse {
	fctx, done, ok := c.wait(ctx, req.SessionID)
	if !ok {
		return ElicitationCancelled()
	}
	defer done()
	answer := make(chan CreateElicitationResponse, 1)
	go func() { answer <- c.opts.OnElicitation(fctx, req) }()
	select {
	case r := <-answer:
		if fctx.Err() != nil {
			return ElicitationCancelled() // the turn ended while the user answered
		}
		switch r.Action {
		case ElicitationDecline:
			return CreateElicitationResponse{Action: ElicitationDecline}
		case ElicitationAccept:
			if content, err := req.RequestedSchema.ValidateValues(r.Content); err == nil {
				return CreateElicitationResponse{Action: ElicitationAccept, Content: content}
			}
		}
		return ElicitationCancelled()
	case <-fctx.Done():
		return ElicitationCancelled()
	}
}

func (c *Client) askPermission(ctx context.Context, req RequestPermissionRequest) RequestPermissionResponse {
	if c.opts.OnPermission == nil {
		return Cancelled()
	}
	pctx, done, ok := c.wait(ctx, req.SessionID)
	if !ok {
		return Cancelled()
	}
	defer done()

	answer := make(chan RequestPermissionResponse, 1)
	go func() { answer <- c.opts.OnPermission(pctx, req) }()
	select {
	case r := <-answer:
		if pctx.Err() != nil {
			return Cancelled() // the turn ended while the user answered
		}
		if r.Outcome.Outcome == "selected" {
			if _, ok := req.Option(r.Outcome.OptionID); ok {
				return r
			}
		}
		return Cancelled()
	case <-pctx.Done():
		return Cancelled()
	}
}

func (c *Client) cancelPermissions(match func(*permWait) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for w := range c.perms {
		if match(w) {
			w.cancel()
		}
	}
}
