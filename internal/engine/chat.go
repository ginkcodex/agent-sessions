package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// Chat events.
const (
	// ChatUpdateEvent carries a ChatUpdate.
	ChatUpdateEvent = "chat:update"
	// ChatExitEvent carries a ChatExit; it is a chat's last event.
	ChatExitEvent = "chat:exit"
)

// Kinds of ChatUpdate.
const (
	// ChatStarted carries the chat's State. It is the first event.
	ChatStarted = "started"
	// ChatSessionUpdate carries an ACP session/update as Update.
	ChatSessionUpdate = "session_update"
	// ChatPermissionAsked carries a pending Permission.
	ChatPermissionAsked = "permission"
	// ChatPermissionResolved carries the Permission's ID and its outcome.
	ChatPermissionResolved = "permission_resolved"
	// ChatTurnStarted is sent when a prompt is sent to the agent.
	ChatTurnStarted = "turn_start"
	// ChatTurnEnded carries the StopReason, or Error (and AuthMethods when
	// the agent needs a login).
	ChatTurnEnded = "turn_end"
	// ChatConfigOptions carries the session options after they changed.
	ChatConfigOptions = "config_options"
)

// Reasons a chat ended, in ChatExit.
const (
	ChatExitClosed  = "closed"  // the chat was closed
	ChatExitExited  = "exited"  // the agent exited with status 0
	ChatExitCrashed = "crashed" // the agent failed or was killed
)

var (
	// ErrChatNotFound means there is no open chat with that ID.
	ErrChatNotFound = errors.New("chat not found")
	// ErrChatBusy means the agent is still working on the previous prompt.
	ErrChatBusy = errors.New("the agent is still working on the last prompt")
	// ErrChatClosed means the chat is closing or its agent has exited.
	ErrChatClosed = errors.New("the chat is closed")
	// ErrChatCannotContinue means the agent offers neither session/load nor
	// session/resume, so a saved session cannot continue in a chat.
	ErrChatCannotContinue = errors.New("the agent's chat adapter cannot continue a saved session")
	// ErrPermissionNotFound means the permission request was already
	// answered, or was cancelled.
	ErrPermissionNotFound = errors.New("the permission request is no longer pending")
	// ErrSessionOwned is wrapped by SessionOwnedError.
	ErrSessionOwned = errors.New("the session is already open in a chat")
)

// SessionOwnedError refuses a second chat for a session.
type SessionOwnedError struct {
	Ref    model.SessionRef
	ChatID string
}

func (e *SessionOwnedError) Error() string {
	return fmt.Sprintf("%s: %s (chat %s)", ErrSessionOwned, e.Ref.Key(), e.ChatID)
}

func (e *SessionOwnedError) Unwrap() error { return ErrSessionOwned }

// AuthRequiredError means the agent needs the user to log in. Methods are the
// agent's ways to do it, from its initialize reply.
type AuthRequiredError struct {
	Agent   model.AgentID
	Methods []acp.AuthMethod
	Err     error
}

func (e *AuthRequiredError) Error() string {
	msg := string(e.Agent) + " needs you to log in"
	if len(e.Methods) > 0 {
		if m := e.Methods[0]; m.Description != "" {
			msg += ": " + m.Description
		} else {
			msg += ": " + m.Name
		}
	}
	return msg
}

func (e *AuthRequiredError) Unwrap() error { return e.Err }

// ChatStartRequest starts a chat in Dir: a new session, or Resume's.
type ChatStartRequest struct {
	Agent  model.AgentID     `json:"agent"`
	Dir    string            `json:"dir"`
	Resume *model.SessionRef `json:"resume,omitempty"`
	// Emitter, when set, takes this chat's events instead of the manager's
	// emitter: a server sends them to the connection that started the chat.
	Emitter Emitter `json:"-"`
}

// ChatState describes an open chat.
type ChatState struct {
	ChatID string        `json:"chatId"`
	Agent  model.AgentID `json:"agent"`
	Dir    string        `json:"dir"`
	// Session is the session the chat drives, named by the agent's
	// sessionId.
	Session       model.SessionRef    `json:"session"`
	AgentInfo     *acp.Implementation `json:"agentInfo,omitempty"`
	ConfigOptions []acp.ConfigOption  `json:"configOptions"`
	// Working is true while a prompt turn runs.
	Working bool `json:"working"`
	// Permissions are the requests waiting for an answer, oldest first, so
	// a reloaded window can show them again.
	Permissions []ChatPermission `json:"permissions,omitempty"`
}

// ChatPermission is a permission request, or its outcome once resolved.
type ChatPermission struct {
	ID       string                 `json:"id"`
	ToolCall *acp.ToolCall          `json:"toolCall,omitempty"`
	Options  []acp.PermissionOption `json:"options,omitempty"`
	// Outcome is "selected" or "cancelled" once resolved.
	Outcome  string `json:"outcome,omitempty"`
	OptionID string `json:"optionId,omitempty"`
}

// ChatUpdate is the payload of chat:update. Seq counts a chat's events from
// 1, chat:exit included, in the order they were emitted.
type ChatUpdate struct {
	ChatID        string             `json:"chatId"`
	Seq           uint64             `json:"seq"`
	Kind          string             `json:"kind"`
	State         *ChatState         `json:"state,omitempty"`
	Update        *acp.SessionUpdate `json:"update,omitempty"`
	Permission    *ChatPermission    `json:"permission,omitempty"`
	StopReason    string             `json:"stopReason,omitempty"`
	Error         string             `json:"error,omitempty"`
	AuthMethods   []acp.AuthMethod   `json:"authMethods,omitempty"`
	ConfigOptions []acp.ConfigOption `json:"configOptions,omitempty"`
}

// ChatExit is the payload of chat:exit.
type ChatExit struct {
	ChatID     string `json:"chatId"`
	Seq        uint64 `json:"seq"`
	Reason     string `json:"reason"`
	Error      string `json:"error,omitempty"`
	StderrTail string `json:"stderrTail,omitempty"`
}

// ChatBackend is a backend that runs chats.
type ChatBackend interface {
	// Chats returns the chat manager; nil when chats cannot run.
	Chats() *ChatManager
}

// ChatManagerOptions configures a ChatManager.
type ChatManagerOptions struct {
	// Launch returns the command that starts agent's adapter in dir.
	Launch func(ctx context.Context, agent model.AgentID, dir string) (acp.Command, error)
	// Agents reports adapter availability.
	Agents func(ctx context.Context) ([]ChatAgent, error)
	// Refresh asks for a rescan of the agent's sessions. It must not block.
	Refresh    func(agent model.AgentID)
	Emitter    Emitter
	ClientInfo acp.Implementation
	// Grace is each shutdown step's wait (acp.ClientOptions.Grace).
	Grace time.Duration

	// owners replaces the process-wide registry in tests.
	owners *chatOwners
}

// chatOwners maps sessions to the chats that drive them. One registry
// serves the whole process, so two windows, or two connections to a
// server, cannot open chats on the same session.
type chatOwners struct {
	mu sync.Mutex
	m  map[string]string // SessionRef.Key() → chat ID
}

var processChatOwners = &chatOwners{m: map[string]string{}}

// claim makes chatID the owner of ref unless another chat owns it.
func (o *chatOwners) claim(ref model.SessionRef, chatID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if owner, ok := o.m[ref.Key()]; ok && owner != chatID {
		return &SessionOwnedError{Ref: ref, ChatID: owner}
	}
	o.m[ref.Key()] = chatID
	return nil
}

func (o *chatOwners) owner(ref model.SessionRef) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	id, ok := o.m[ref.Key()]
	return id, ok
}

// release drops every session chatID owns.
func (o *chatOwners) release(chatID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for key, owner := range o.m {
		if owner == chatID {
			delete(o.m, key)
		}
	}
}

func (o *chatOwners) len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.m)
}

// ChatManager runs chats: one ACP agent process per chat, each driving one
// session. A session has at most one chat.
type ChatManager struct {
	opts ChatManagerOptions

	emitterMu sync.RWMutex
	emitter   Emitter

	owners *chatOwners

	mu    sync.Mutex
	chats map[string]*chat
}

// NewChatManager returns a manager without chats.
func NewChatManager(opts ChatManagerOptions) *ChatManager {
	m := &ChatManager{opts: opts, chats: map[string]*chat{}, owners: opts.owners}
	if m.owners == nil {
		m.owners = processChatOwners
	}
	m.SetEmitter(opts.Emitter)
	return m
}

// SetEmitter changes where events go.
func (m *ChatManager) SetEmitter(emitter Emitter) {
	if m == nil {
		return
	}
	if emitter == nil {
		emitter = NopEmitter{}
	}
	m.emitterMu.Lock()
	m.emitter = emitter
	m.emitterMu.Unlock()
}

func (m *ChatManager) emit(name string, payload any) {
	m.emitterMu.RLock()
	em := m.emitter
	m.emitterMu.RUnlock()
	em.Emit(name, payload)
}

// Agents reports, per agent with an adapter, whether a chat can start.
func (m *ChatManager) Agents(ctx context.Context) ([]ChatAgent, error) {
	if m.opts.Agents == nil {
		return []ChatAgent{}, nil
	}
	return m.opts.Agents(ctx)
}

// Owned returns the chat that drives ref, in this manager or another one
// in the process.
func (m *ChatManager) Owned(ref model.SessionRef) (chatID string, ok bool) {
	if m == nil {
		return "", false
	}
	return m.owners.owner(ref)
}

// Chats returns the open chats.
func (m *ChatManager) Chats() []ChatState {
	m.mu.Lock()
	chats := make([]*chat, 0, len(m.chats))
	for _, ch := range m.chats {
		chats = append(chats, ch)
	}
	m.mu.Unlock()
	out := make([]ChatState, 0, len(chats))
	for _, ch := range chats {
		if st, ok := ch.state(); ok {
			out = append(out, st)
		}
	}
	slices.SortFunc(out, func(a, b ChatState) int { return strings.Compare(a.ChatID, b.ChatID) })
	return out
}

// Start starts the agent and opens the session: a new one, or Resume's,
// continued with session/resume when the agent offers it and otherwise with
// session/load, whose replay is dropped (the transcript already shows it).
// The first event is ChatStarted.
func (m *ChatManager) Start(ctx context.Context, req ChatStartRequest) (ChatState, error) {
	if req.Resume != nil && (req.Resume.Agent != req.Agent || req.Resume.ID == "") {
		return ChatState{}, fmt.Errorf("chat: cannot continue %s with %s", req.Resume.Key(), req.Agent)
	}
	if m.opts.Launch == nil {
		return ChatState{}, ErrChatUnsupported
	}
	ch := &chat{
		m: m, id: newChatID(), agent: req.Agent, dir: req.Dir, emitter: req.Emitter,
		perms: map[string]*pendingPermission{}, done: make(chan struct{}),
	}
	ch.ctx, ch.cancel = context.WithCancel(context.Background())
	if req.Resume != nil {
		if err := m.owners.claim(*req.Resume, ch.id); err != nil {
			ch.cancel()
			return ChatState{}, err
		}
		ch.ref = *req.Resume
	}
	m.mu.Lock()
	m.chats[ch.id] = ch
	m.mu.Unlock()

	st, err := m.start(ctx, ch, req)
	if err != nil {
		ch.cancel()
		m.release(ch)
		close(ch.done)
		return ChatState{}, err
	}
	return st, nil
}

func (m *ChatManager) start(ctx context.Context, ch *chat, req ChatStartRequest) (ChatState, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch.mu.Lock()
	ch.startCancel = cancel
	ch.mu.Unlock()

	cmd, err := m.opts.Launch(ctx, req.Agent, req.Dir)
	if err != nil {
		return ChatState{}, err
	}
	client, err := acp.Start(ctx, cmd, acp.ClientOptions{
		ClientInfo:   m.opts.ClientInfo,
		OnUpdate:     ch.onUpdate,
		OnPermission: ch.onPermission,
		Grace:        m.opts.Grace,
	})
	if err != nil {
		return ChatState{}, ch.closedOr(err)
	}
	ch.mu.Lock()
	closing := ch.closing
	if !closing {
		ch.client = client
	}
	ch.mu.Unlock()
	if closing {
		_ = client.Close()
		return ChatState{}, ErrChatClosed
	}

	var setup acp.SessionSetup
	switch {
	case req.Resume == nil:
		setup, err = client.NewSession(ctx, req.Dir)
	case client.Info().AgentCapabilities.CanResume():
		setup, err = client.ResumeSession(ctx, req.Resume.ID, req.Dir)
	case !client.Info().AgentCapabilities.LoadSession:
		err = ErrChatCannotContinue
	default:
		// The replay arrives before the reply; drop it.
		ch.setReplaying(true)
		setup, err = client.LoadSession(ctx, req.Resume.ID, req.Dir)
		ch.setReplaying(false)
	}
	if err != nil {
		_ = client.Close()
		return ChatState{}, ch.closedOr(m.chatError(req.Agent, client, err))
	}

	ref := model.SessionRef{Agent: req.Agent, ID: setup.SessionID}
	if err := m.owners.claim(ref, ch.id); err != nil {
		_ = client.Close()
		return ChatState{}, err
	}

	ch.mu.Lock()
	ch.ref = ref
	ch.options = setup.ConfigOptions
	ch.started = true
	ch.mu.Unlock()

	st, _ := ch.state()
	ch.emit(ChatUpdate{Kind: ChatStarted, State: &st})
	go ch.watch(client)
	return st, nil
}

func (m *ChatManager) chatError(agent model.AgentID, client *acp.Client, err error) error {
	if acp.IsAuthRequired(err) {
		return &AuthRequiredError{Agent: agent, Methods: client.Info().AuthMethods, Err: err}
	}
	return err
}

func (m *ChatManager) get(chatID string) (*chat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.chats[chatID]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrChatNotFound, chatID)
}

// release forgets the chat and the sessions it owned.
func (m *ChatManager) release(ch *chat) {
	m.mu.Lock()
	if m.chats[ch.id] == ch {
		delete(m.chats, ch.id)
	}
	m.mu.Unlock()
	m.owners.release(ch.id)
}

// Prompt sends text as a new turn and returns once the turn has started.
// Its updates follow as events, then ChatTurnEnded.
func (m *ChatManager) Prompt(chatID, text string) error {
	ch, err := m.get(chatID)
	if err != nil {
		return err
	}
	ch.mu.Lock()
	switch {
	case ch.closing || ch.exited || ch.client == nil || !ch.started:
		ch.mu.Unlock()
		return ErrChatClosed
	case ch.working:
		ch.mu.Unlock()
		return ErrChatBusy
	}
	ch.working = true
	ch.work.Add(1)
	client, sessionID := ch.client, ch.ref.ID
	ch.mu.Unlock()

	ch.emit(ChatUpdate{Kind: ChatTurnStarted})
	go func() {
		defer ch.work.Done()
		// The turn runs until it ends or the agent goes away; Cancel stops it.
		stop, err := client.Prompt(context.Background(), sessionID, []acp.ContentBlock{acp.TextBlock(text)})
		ev := ChatUpdate{Kind: ChatTurnEnded, StopReason: stop}
		if err != nil {
			err = m.chatError(ch.agent, client, err)
			ev.Error = err.Error()
			var auth *AuthRequiredError
			if errors.As(err, &auth) {
				ev.AuthMethods = auth.Methods
			}
		}
		// The next turn's start must follow this end.
		ch.emitMu.Lock()
		ch.mu.Lock()
		ch.working = false
		ch.mu.Unlock()
		ch.emitLocked(ev)
		ch.emitMu.Unlock()
		if m.opts.Refresh != nil {
			m.opts.Refresh(ch.agent)
		}
	}()
	return nil
}

// Cancel asks the agent to stop the running turn. Pending permission
// requests are answered cancelled.
func (m *ChatManager) Cancel(chatID string) error {
	ch, err := m.get(chatID)
	if err != nil {
		return err
	}
	client, sessionID, err := ch.live()
	if err != nil {
		return err
	}
	return client.Cancel(sessionID)
}

// Respond answers a permission request with one of its options, or
// cancelled when optionID is empty.
func (m *ChatManager) Respond(chatID, permissionID, optionID string) error {
	ch, err := m.get(chatID)
	if err != nil {
		return err
	}
	ch.mu.Lock()
	p, ok := ch.perms[permissionID]
	if !ok {
		ch.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPermissionNotFound, permissionID)
	}
	answer := acp.Cancelled()
	if optionID != "" {
		if _, ok := p.req.Option(optionID); !ok {
			ch.mu.Unlock()
			return fmt.Errorf("chat: the permission request has no option %q", optionID)
		}
		answer = acp.Selected(optionID)
	}
	delete(ch.perms, permissionID)
	ch.mu.Unlock()
	p.answer <- answer
	return nil
}

// SetOption changes a session option and returns the options as they now
// are.
func (m *ChatManager) SetOption(ctx context.Context, chatID, configID string, value any) ([]acp.ConfigOption, error) {
	ch, err := m.get(chatID)
	if err != nil {
		return nil, err
	}
	client, sessionID, err := ch.live()
	if err != nil {
		return nil, err
	}
	opts, err := client.SetConfigOption(ctx, sessionID, configID, value)
	if err != nil {
		return nil, err
	}
	if len(opts) > 0 {
		ch.setOptions(opts)
	}
	st, _ := ch.state()
	return st.ConfigOptions, nil
}

// Close stops the chat's agent and waits until the chat is gone. Pending
// permission requests are answered cancelled.
func (m *ChatManager) Close(chatID string) error {
	ch, err := m.get(chatID)
	if err != nil {
		return err
	}
	ch.close()
	return nil
}

// CloseAll closes every chat and waits for them.
func (m *ChatManager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	chats := make([]*chat, 0, len(m.chats))
	for _, ch := range m.chats {
		chats = append(chats, ch)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, ch := range chats {
		wg.Go(ch.close)
	}
	wg.Wait()
}

type chat struct {
	m     *ChatManager
	id    string
	agent model.AgentID
	dir   string
	done  chan struct{} // closed once the chat is gone
	// emitter takes the chat's events; nil means the manager's.
	emitter Emitter

	// ctx ends when the agent has exited; permission waits use it.
	ctx    context.Context
	cancel context.CancelFunc

	// emitMu orders events; it may be taken before mu, never after.
	emitMu sync.Mutex
	seq    uint64

	mu          sync.Mutex
	client      *acp.Client
	startCancel context.CancelFunc
	ref         model.SessionRef
	options     []acp.ConfigOption
	started     bool
	working     bool
	closing     bool
	exited      bool
	replaying   bool
	perms       map[string]*pendingPermission
	nextPerm    int
	// work counts turns and permission waits; Add only under mu while
	// !exited.
	work sync.WaitGroup
}

type pendingPermission struct {
	n      int // the order asked
	req    acp.RequestPermissionRequest
	answer chan acp.RequestPermissionResponse // buffered; one send
}

func (ch *chat) send(name string, payload any) {
	if ch.emitter != nil {
		ch.emitter.Emit(name, payload)
		return
	}
	ch.m.emit(name, payload)
}

func newChatID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "chat-" + hex.EncodeToString(b[:])
}

func (ch *chat) state() (ChatState, bool) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if !ch.started {
		return ChatState{}, false
	}
	st := ChatState{
		ChatID: ch.id, Agent: ch.agent, Dir: ch.dir, Session: ch.ref,
		ConfigOptions: slices.Clone(ch.options), Working: ch.working,
	}
	if st.ConfigOptions == nil {
		st.ConfigOptions = []acp.ConfigOption{}
	}
	ids := make([]string, 0, len(ch.perms))
	for id := range ch.perms {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return ch.perms[a].n - ch.perms[b].n })
	for _, id := range ids {
		p := ch.perms[id]
		tool := p.req.ToolCall
		st.Permissions = append(st.Permissions, ChatPermission{ID: id, ToolCall: &tool, Options: slices.Clone(p.req.Options)})
	}
	if info := ch.client.Info().AgentInfo; info != nil {
		i := *info
		st.AgentInfo = &i
	}
	return st, true
}

func (ch *chat) live() (*acp.Client, string, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closing || ch.exited || ch.client == nil || !ch.started {
		return nil, "", ErrChatClosed
	}
	return ch.client, ch.ref.ID, nil
}

// closedOr reports a failure caused by Close as ErrChatClosed.
func (ch *chat) closedOr(err error) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closing {
		return ErrChatClosed
	}
	return err
}

func (ch *chat) emit(u ChatUpdate) {
	ch.emitMu.Lock()
	defer ch.emitMu.Unlock()
	ch.emitLocked(u)
}

func (ch *chat) emitLocked(u ChatUpdate) {
	ch.seq++
	u.ChatID, u.Seq = ch.id, ch.seq
	ch.send(ChatUpdateEvent, u)
}

func (ch *chat) setReplaying(v bool) {
	ch.mu.Lock()
	ch.replaying = v
	ch.mu.Unlock()
}

func (ch *chat) setOptions(opts []acp.ConfigOption) {
	ch.mu.Lock()
	ch.options = slices.Clone(opts)
	ch.mu.Unlock()
	ch.emit(ChatUpdate{Kind: ChatConfigOptions, ConfigOptions: slices.Clone(opts)})
}

// onUpdate runs on the connection's read loop, so updates are emitted in
// the order the agent sent them.
func (ch *chat) onUpdate(n acp.SessionNotification) {
	ch.mu.Lock()
	if ch.replaying {
		ch.mu.Unlock()
		return
	}
	var options []acp.ConfigOption
	switch n.Update.Kind {
	case acp.UpdateConfigOption:
		if u, err := n.Update.AsConfigOptions(); err == nil {
			options = u.ConfigOptions
		}
	case acp.UpdateCurrentMode:
		if u, err := n.Update.AsCurrentMode(); err == nil {
			options = withMode(ch.options, u.CurrentModeID)
		}
	}
	ch.mu.Unlock()
	u := n.Update
	ch.emit(ChatUpdate{Kind: ChatSessionUpdate, Update: &u})
	if options != nil {
		ch.setOptions(options)
	}
}

// withMode returns options with the mode option set to mode, or nil when
// there is no mode option.
func withMode(options []acp.ConfigOption, mode string) []acp.ConfigOption {
	i := slices.IndexFunc(options, func(o acp.ConfigOption) bool { return o.Category == "mode" })
	if i < 0 {
		i = slices.IndexFunc(options, func(o acp.ConfigOption) bool { return o.ID == "mode" })
	}
	if i < 0 {
		return nil
	}
	out := slices.Clone(options)
	out[i].CurrentValue = json.RawMessage(strconv.Quote(mode))
	return out
}

// onPermission emits the request and waits for Respond, the turn's end or
// the agent's exit.
func (ch *chat) onPermission(ctx context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	ch.mu.Lock()
	if ch.closing || ch.exited {
		ch.mu.Unlock()
		return acp.Cancelled()
	}
	ch.nextPerm++
	id := "perm-" + strconv.Itoa(ch.nextPerm)
	p := &pendingPermission{n: ch.nextPerm, req: req, answer: make(chan acp.RequestPermissionResponse, 1)}
	ch.perms[id] = p
	ch.work.Add(1)
	ch.mu.Unlock()
	defer ch.work.Done()

	tool := req.ToolCall
	ch.emit(ChatUpdate{Kind: ChatPermissionAsked, Permission: &ChatPermission{ID: id, ToolCall: &tool, Options: req.Options}})
	var r acp.RequestPermissionResponse
	select {
	case r = <-p.answer:
	case <-ctx.Done():
	case <-ch.ctx.Done():
	}
	ch.mu.Lock()
	delete(ch.perms, id)
	ch.mu.Unlock()
	if r.Outcome.Outcome == "" || ctx.Err() != nil || ch.ctx.Err() != nil {
		r = acp.Cancelled() // what the client sends then
	}
	ch.emit(ChatUpdate{Kind: ChatPermissionResolved, Permission: &ChatPermission{ID: id, Outcome: r.Outcome.Outcome, OptionID: r.Outcome.OptionID}})
	return r
}

// close stops the agent, or the start in progress, and waits for the chat
// to be gone.
func (ch *chat) close() {
	ch.mu.Lock()
	ch.closing = true
	client, startCancel := ch.client, ch.startCancel
	ch.mu.Unlock()
	if startCancel != nil {
		startCancel()
	}
	if client != nil {
		_ = client.Close()
	}
	<-ch.done
}

// watch ends the chat when its agent exits: it waits for the turn and the
// permission waits to finish, releases the session and emits chat:exit.
func (ch *chat) watch(client *acp.Client) {
	<-client.Done()
	ch.mu.Lock()
	ch.exited = true
	closing := ch.closing
	ch.mu.Unlock()
	ch.cancel()
	ch.work.Wait()
	ch.m.release(ch)

	exit := ChatExit{Reason: ChatExitExited, StderrTail: client.StderrTail()}
	switch err := client.ExitErr(); {
	case closing:
		exit.Reason = ChatExitClosed
	case err != nil:
		exit.Reason, exit.Error = ChatExitCrashed, err.Error()
	}
	ch.emitMu.Lock()
	ch.seq++
	exit.ChatID, exit.Seq = ch.id, ch.seq
	ch.send(ChatExitEvent, exit)
	ch.emitMu.Unlock()
	close(ch.done)
}
