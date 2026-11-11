package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// DefaultMaxFrameBytes caps one incoming message, as internal/rpc does.
const DefaultMaxFrameBytes = 64 << 20

var (
	// ErrClosed is the error of calls on a connection the client closed.
	ErrClosed = errors.New("acp: connection closed")
	// ErrAgentGone ends a connection whose agent closed its output.
	ErrAgentGone = errors.New("acp: agent closed the connection")
	// ErrFrameTooLarge ends a connection that received an oversized message.
	ErrFrameTooLarge = errors.New("acp: message exceeds the frame limit")
)

// RequestHandler answers one of the agent's requests. A returned *Error is
// sent as is; any other error is sent as an internal error.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// NotificationHandler receives the agent's notifications in arrival order.
type NotificationHandler func(method string, params json.RawMessage)

// ConnOptions configures a Conn. Every field is optional.
type ConnOptions struct {
	// MaxFrameBytes caps an incoming message; 0 means DefaultMaxFrameBytes.
	MaxFrameBytes int
	// OnRequest answers the agent's requests, each in its own goroutine.
	// Its context ends when the connection ends, or when the agent
	// withdraws the request with $/cancel_request; the reply is then
	// error CodeRequestCancelled, whatever OnRequest returns. When nil,
	// every request gets method-not-found.
	OnRequest RequestHandler
	// OnNotification runs on the read loop, before any later message is
	// read, so it sees updates in order and before the reply that follows
	// them. It must not block for long.
	OnNotification NotificationHandler
	// OnProtocolError reports a message that was skipped: invalid JSON, a
	// reply to an unknown request, or a message that is neither request,
	// notification nor reply.
	OnProtocolError func(error)
}

// Conn is a two-way JSON-RPC connection to an agent, one message per line.
type Conn struct {
	w    io.Writer
	opts ConnOptions

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[string]chan reply
	// incoming are the agent's requests being answered, by ID.
	incoming map[string]*incomingRequest
	err      error // set once, when the connection ends

	done      chan struct{}
	closeOnce sync.Once
	// handlerCtx is cancelled when the connection ends, so handlers waiting
	// for the user stop.
	handlerCtx    context.Context
	cancelHandler context.CancelFunc
}

type incomingRequest struct {
	cancel    context.CancelCauseFunc
	withdrawn bool // under Conn.mu
}

type reply struct {
	result json.RawMessage
	err    error
}

// message is any JSON-RPC message. ID stays raw so the agent's IDs (which
// may be 0 or strings) are echoed exactly as received.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func (m *message) hasID() bool {
	return len(m.ID) > 0 && string(m.ID) != "null"
}

// NewConn starts reading r. Writes go to w; Close closes w when it is an
// io.Closer, which tells a stdio agent to exit.
func NewConn(r io.Reader, w io.Writer, opts ConnOptions) *Conn {
	if opts.MaxFrameBytes <= 0 {
		opts.MaxFrameBytes = DefaultMaxFrameBytes
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{
		w:             w,
		opts:          opts,
		pending:       map[string]chan reply{},
		incoming:      map[string]*incomingRequest{},
		done:          make(chan struct{}),
		handlerCtx:    ctx,
		cancelHandler: cancel,
	}
	go c.readLoop(r)
	return c
}

// Done is closed when the connection has ended.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err is why the connection ended: ErrClosed, ErrAgentGone,
// ErrFrameTooLarge or a read error. It is nil while the connection is open.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close ends the connection: pending calls fail with ErrClosed, handlers'
// contexts are cancelled, and w is closed. Messages still arriving are read
// and dropped until the agent closes its output.
func (c *Conn) Close() error {
	c.finish(ErrClosed)
	if cl, ok := c.w.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

// finish ends the connection once, with the first reason given.
func (c *Conn) finish(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		pending := c.pending
		c.pending = nil
		c.mu.Unlock()
		for _, ch := range pending {
			ch <- reply{err: err}
		}
		c.cancelHandler()
		close(c.done)
	})
}

// Call sends a request and decodes its result into result, which may be nil.
// It returns the agent's *Error, the connection's error, or ctx's error.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	return c.call(ctx, method, params, result, false)
}

func (c *Conn) call(ctx context.Context, method string, params, result any, withdraw bool) error {
	raw, err := encode(params)
	if err != nil {
		return fmt.Errorf("acp: encode %s params: %w", method, err)
	}
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	key := strconv.FormatInt(c.nextID, 10)
	ch := make(chan reply, 1)
	c.pending[key] = ch
	c.mu.Unlock()

	if err := c.write(message{ID: json.RawMessage(key), Method: method, Params: raw}); err != nil {
		c.forget(key)
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if result == nil || len(r.result) == 0 {
			return nil
		}
		if err := json.Unmarshal(r.result, result); err != nil {
			return fmt.Errorf("acp: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		if withdraw {
			// The entry stays until the other side's reply, which is then
			// expected and dropped; ch has room for it.
			_ = c.Notify(MethodCancelRequest, CancelRequestNotification{RequestID: json.RawMessage(key)})
		} else {
			c.forget(key)
		}
		return ctx.Err()
	}
}

// CallWithdrawing is Call, but when ctx ends first it also tells the other
// side to drop the request with $/cancel_request, and its late reply is not
// reported as a protocol error.
func (c *Conn) CallWithdrawing(ctx context.Context, method string, params, result any) error {
	return c.call(ctx, method, params, result, true)
}

func (c *Conn) forget(key string) {
	c.mu.Lock()
	if c.pending != nil {
		delete(c.pending, key)
	}
	c.mu.Unlock()
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	raw, err := encode(params)
	if err != nil {
		return fmt.Errorf("acp: encode %s params: %w", method, err)
	}
	return c.write(message{Method: method, Params: raw})
}

// encode returns v as JSON; nil stays absent.
func encode(v any) (json.RawMessage, error) {
	switch v := v.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return v, nil
	}
	return json.Marshal(v)
}

func (c *Conn) write(m message) error {
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.Err(); err != nil {
		return err
	}
	_, err = c.w.Write(b)
	return err
}

func (c *Conn) readLoop(r io.Reader) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := readLine(br, c.opts.MaxFrameBytes)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				err = ErrAgentGone
			case errors.Is(err, ErrFrameTooLarge):
			default:
				err = fmt.Errorf("acp: read: %w", err)
			}
			c.finish(err)
			return
		}
		if len(line) == 0 {
			continue
		}
		select {
		case <-c.done:
			continue // closed by the client: drain until the agent exits
		default:
		}
		c.dispatch(line)
	}
}

func (c *Conn) dispatch(line []byte) {
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		c.protocolError(fmt.Errorf("acp: invalid message %.200q: %w", line, err))
		return
	}
	switch {
	case m.Method != "" && m.hasID():
		// Registered before the next message is read, so a
		// $/cancel_request that follows finds it.
		ctx, cancel := context.WithCancelCause(c.handlerCtx)
		in := &incomingRequest{cancel: cancel}
		key := requestKey(m.ID)
		c.mu.Lock()
		if _, dup := c.incoming[key]; dup {
			key = "" // a reused ID cannot be withdrawn
		} else {
			c.incoming[key] = in
		}
		c.mu.Unlock()
		go c.handleRequest(ctx, key, in, m)
	case m.Method == MethodCancelRequest:
		c.withdraw(m.Params)
	case m.Method != "":
		if c.opts.OnNotification != nil {
			c.opts.OnNotification(m.Method, m.Params)
		}
	case m.hasID():
		c.handleReply(m)
	default:
		c.protocolError(fmt.Errorf("acp: message is not a request, notification or reply: %.200q", line))
	}
}

func (c *Conn) handleReply(m message) {
	key := string(m.ID)
	c.mu.Lock()
	ch, ok := c.pending[key]
	if ok {
		delete(c.pending, key)
	}
	c.mu.Unlock()
	if !ok {
		c.protocolError(fmt.Errorf("acp: reply to unknown request id %s", key))
		return
	}
	if m.Error != nil {
		ch <- reply{err: m.Error}
		return
	}
	ch <- reply{result: m.Result}
}

// withdraw cancels the request a $/cancel_request names.
func (c *Conn) withdraw(params json.RawMessage) {
	var p CancelRequestNotification
	if err := json.Unmarshal(params, &p); err != nil || len(p.RequestID) == 0 {
		c.protocolError(fmt.Errorf("acp: invalid %s: %.200q", MethodCancelRequest, params))
		return
	}
	c.mu.Lock()
	in := c.incoming[requestKey(p.RequestID)]
	if in != nil {
		in.withdrawn = true
	}
	c.mu.Unlock()
	if in != nil {
		in.cancel(ErrWithdrawn)
	}
}

// requestKey is a request ID as compact JSON, so 7 and 7.0 or "a" written
// with other spacing still match.
func requestKey(id json.RawMessage) string {
	var b bytes.Buffer
	if err := json.Compact(&b, id); err != nil {
		return string(id)
	}
	return b.String()
}

func (c *Conn) handleRequest(ctx context.Context, key string, in *incomingRequest, m message) {
	defer func() {
		in.cancel(nil)
		if key != "" {
			c.mu.Lock()
			delete(c.incoming, key)
			c.mu.Unlock()
		}
	}()
	var (
		result any
		err    error
	)
	if c.opts.OnRequest == nil {
		err = MethodNotFound(m.Method)
	} else {
		result, err = c.opts.OnRequest(ctx, m.Method, m.Params)
	}
	c.mu.Lock()
	withdrawn := in.withdrawn
	c.mu.Unlock()
	if withdrawn {
		result, err = nil, &Error{Code: CodeRequestCancelled, Message: "request cancelled"}
	}
	out := message{ID: m.ID}
	if err == nil {
		if out.Result, err = encode(result); err == nil && out.Result == nil {
			out.Result = json.RawMessage("null")
		}
	}
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = &Error{Code: CodeInternalError, Message: err.Error()}
		}
		out.Result, out.Error = nil, e
	}
	// A reply that cannot be sent because the connection ended is expected.
	if werr := c.write(out); werr != nil && c.Err() == nil {
		c.protocolError(fmt.Errorf("acp: reply to %s: %w", m.Method, werr))
	}
}

func (c *Conn) protocolError(err error) {
	if c.opts.OnProtocolError != nil {
		c.opts.OnProtocolError(err)
	}
}

// readLine reads one line of at most limit bytes, without its newline and
// surrounding space. A longer line returns ErrFrameTooLarge.
func readLine(br *bufio.Reader, limit int) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				return bytes.TrimSpace(buf), nil
			}
			return nil, err
		}
		if len(buf)+len(chunk) > limit {
			return nil, ErrFrameTooLarge
		}
		buf = append(buf, chunk...)
		if !isPrefix {
			return bytes.TrimSpace(buf), nil
		}
	}
}
