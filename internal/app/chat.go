package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/chatattach"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

// ErrChatRemote refuses a chat on a connected host whose server does not
// offer chats (Capabilities.ACPChat): an older agent-sessions there.
var ErrChatRemote = errors.New("this host's agent-sessions does not support chat; reconnect to update it")

// errChatUnavailable means the window has no local engine to run chats.
var errChatUnavailable = errors.New("chat is not available")

// chatService runs the window's chats: the local engine's chat manager, or
// the connected host's server (rpc.Client), where the session's files and
// the agent are.
type chatService interface {
	ChatAgents(ctx context.Context) ([]engine.ChatAgent, error)
	ChatStart(ctx context.Context, req engine.ChatStartRequest) (engine.ChatState, error)
	ChatPrompt(ctx context.Context, chatID, text string, atts []chatattach.Attachment) (engine.ChatPromptResult, error)
	ChatSendNow(ctx context.Context, chatID, queuedID string) error
	ChatUnqueue(ctx context.Context, chatID, queuedID string) (engine.ChatDraft, error)
	ChatCancel(ctx context.Context, chatID string) error
	ChatRespond(ctx context.Context, chatID, permissionID, optionID string) error
	ChatAnswer(ctx context.Context, chatID, questionID, action string, content map[string]any) error
	ChatSetOption(ctx context.Context, chatID, configID string, value any) ([]acp.ConfigOption, error)
	ChatClose(ctx context.Context, chatID string) error
	ChatList(ctx context.Context) ([]engine.ChatState, error)
}

// localChatService is the local chat manager as a chatService.
type localChatService struct{ m *engine.ChatManager }

func (l localChatService) ChatAgents(ctx context.Context) ([]engine.ChatAgent, error) {
	return l.m.Agents(ctx)
}

func (l localChatService) ChatStart(ctx context.Context, req engine.ChatStartRequest) (engine.ChatState, error) {
	return l.m.Start(ctx, req)
}

func (l localChatService) ChatPrompt(_ context.Context, chatID, text string, atts []chatattach.Attachment) (engine.ChatPromptResult, error) {
	return l.m.Prompt(chatID, text, atts)
}

func (l localChatService) ChatSendNow(ctx context.Context, chatID, queuedID string) error {
	return l.m.SendNow(ctx, chatID, queuedID)
}

func (l localChatService) ChatUnqueue(_ context.Context, chatID, queuedID string) (engine.ChatDraft, error) {
	return l.m.Unqueue(chatID, queuedID)
}

func (l localChatService) ChatCancel(_ context.Context, chatID string) error {
	return l.m.Cancel(chatID)
}

func (l localChatService) ChatRespond(_ context.Context, chatID, permissionID, optionID string) error {
	return l.m.Respond(chatID, permissionID, optionID)
}

func (l localChatService) ChatAnswer(_ context.Context, chatID, questionID, action string, content map[string]any) error {
	return l.m.Answer(chatID, questionID, action, content)
}

func (l localChatService) ChatSetOption(ctx context.Context, chatID, configID string, value any) ([]acp.ConfigOption, error) {
	return l.m.SetOption(ctx, chatID, configID, value)
}

func (l localChatService) ChatClose(_ context.Context, chatID string) error {
	return l.m.Close(chatID)
}

func (l localChatService) ChatList(context.Context) ([]engine.ChatState, error) {
	return l.m.Chats(), nil
}

// chats returns where the window's chats run: this computer's engine, or
// the host the window shows. A host's chats end with its connection (the
// server closes a connection's chats), and this computer's end when the
// window connects to a host (connection.leaveLocal), so the window's chats
// are always on the host it shows.
func (a *App) chats() (chatService, error) {
	r := a.route()
	if r.host == "" {
		m, err := a.ownChats()
		if err != nil {
			return nil, err
		}
		return localChatService{m}, nil
	}
	if off, offline := r.backend.(offlineBackend); offline {
		return nil, off.err()
	}
	s, ok := r.backend.(chatService)
	if !ok {
		return nil, ErrChatRemote
	}
	return remoteChats{s}, nil
}

// remoteChats reads a host without the capability as ErrChatRemote.
type remoteChats struct{ chatService }

func remoteErr(err error) error {
	if errors.Is(err, engine.ErrUnsupportedAction) {
		return fmt.Errorf("%w (%w)", ErrChatRemote, err)
	}
	return err
}

func (r remoteChats) ChatAgents(ctx context.Context) ([]engine.ChatAgent, error) {
	out, err := r.chatService.ChatAgents(ctx)
	return out, remoteErr(err)
}

func (r remoteChats) ChatStart(ctx context.Context, req engine.ChatStartRequest) (engine.ChatState, error) {
	out, err := r.chatService.ChatStart(ctx, req)
	return out, remoteErr(err)
}

func (r remoteChats) ChatPrompt(ctx context.Context, chatID, text string, atts []chatattach.Attachment) (engine.ChatPromptResult, error) {
	out, err := r.chatService.ChatPrompt(ctx, chatID, text, atts)
	return out, remoteErr(err)
}

func (r remoteChats) ChatSendNow(ctx context.Context, chatID, queuedID string) error {
	return remoteErr(r.chatService.ChatSendNow(ctx, chatID, queuedID))
}

func (r remoteChats) ChatUnqueue(ctx context.Context, chatID, queuedID string) (engine.ChatDraft, error) {
	out, err := r.chatService.ChatUnqueue(ctx, chatID, queuedID)
	return out, remoteErr(err)
}

func (r remoteChats) ChatCancel(ctx context.Context, chatID string) error {
	return remoteErr(r.chatService.ChatCancel(ctx, chatID))
}

func (r remoteChats) ChatRespond(ctx context.Context, chatID, permissionID, optionID string) error {
	return remoteErr(r.chatService.ChatRespond(ctx, chatID, permissionID, optionID))
}

func (r remoteChats) ChatAnswer(ctx context.Context, chatID, questionID, action string, content map[string]any) error {
	return remoteErr(r.chatService.ChatAnswer(ctx, chatID, questionID, action, content))
}

func (r remoteChats) ChatSetOption(ctx context.Context, chatID, configID string, value any) ([]acp.ConfigOption, error) {
	out, err := r.chatService.ChatSetOption(ctx, chatID, configID, value)
	return out, remoteErr(err)
}

func (r remoteChats) ChatClose(ctx context.Context, chatID string) error {
	return remoteErr(r.chatService.ChatClose(ctx, chatID))
}

func (r remoteChats) ChatList(ctx context.Context) ([]engine.ChatState, error) {
	out, err := r.chatService.ChatList(ctx)
	return out, remoteErr(err)
}

// ownChats returns the local chat manager whatever the window shows.
func (a *App) ownChats() (*engine.ChatManager, error) {
	if a.localEngine == nil || a.localEngine.Chats() == nil {
		return nil, errChatUnavailable
	}
	return a.localEngine.Chats(), nil
}

// ChatAgents reports, for each agent that can run in a chat, whether its ACP
// adapter is installed on the host the window shows and how to install it.
func (a *App) ChatAgents() ([]engine.ChatAgent, error) {
	s, err := a.chats()
	if err != nil {
		return nil, err
	}
	return s.ChatAgents(a.appCtx())
}

// ChatStart starts the agent's ACP adapter in req.Dir: a new session, or the
// session req.Resume names. Updates arrive as chat:update events and the end
// as chat:exit.
func (a *App) ChatStart(req engine.ChatStartRequest) (engine.ChatState, error) {
	s, err := a.chats()
	if err != nil {
		return engine.ChatState{}, err
	}
	return s.ChatStart(a.appCtx(), req)
}

// ChatPrompt sends text and attachments to the chat's agent. It returns
// once the turn has started; the turn's end arrives as a turn_end update.
// While the agent works the message is queued, and the result names it.
func (a *App) ChatPrompt(chatID, text string, attachments []chatattach.Attachment) (engine.ChatPromptResult, error) {
	s, err := a.chats()
	if err != nil {
		return engine.ChatPromptResult{}, err
	}
	return s.ChatPrompt(a.appCtx(), chatID, text, attachments)
}

// ChatSendNow sends a queued message at once: it starts a turn when the
// agent is idle, or joins the running turn when the agent steers.
func (a *App) ChatSendNow(chatID, queuedID string) error {
	s, err := a.chats()
	if err != nil {
		return err
	}
	return s.ChatSendNow(a.appCtx(), chatID, queuedID)
}

// ChatUnqueue removes a queued message and returns it with its
// attachments, for the composer to edit.
func (a *App) ChatUnqueue(chatID, queuedID string) (engine.ChatDraft, error) {
	s, err := a.chats()
	if err != nil {
		return engine.ChatDraft{}, err
	}
	return s.ChatUnqueue(a.appCtx(), chatID, queuedID)
}

// ChatCancel cancels the chat's running turn and answers its pending
// permission requests cancelled.
func (a *App) ChatCancel(chatID string) error {
	s, err := a.chats()
	if err != nil {
		return err
	}
	return s.ChatCancel(a.appCtx(), chatID)
}

// ChatRespond answers a permission request with one of its options, or
// cancelled when optionID is empty.
func (a *App) ChatRespond(chatID, permissionID, optionID string) error {
	s, err := a.chats()
	if err != nil {
		return err
	}
	return s.ChatRespond(a.appCtx(), chatID, permissionID, optionID)
}

// ChatAnswer answers one of the agent's questions (a question update):
// action is accept, with content checked against the question's form, or
// decline (the user skipped it) or cancel.
func (a *App) ChatAnswer(chatID, questionID, action string, content map[string]any) error {
	s, err := a.chats()
	if err != nil {
		return err
	}
	return s.ChatAnswer(a.appCtx(), chatID, questionID, action, content)
}

// ChatSetOption sets one of the session's options (mode, model, effort) and
// returns them all as the agent reports them.
func (a *App) ChatSetOption(chatID, configID string, value any) ([]acp.ConfigOption, error) {
	s, err := a.chats()
	if err != nil {
		return nil, err
	}
	return s.ChatSetOption(a.appCtx(), chatID, configID, value)
}

// ChatClose ends the chat and its agent. Pending permission requests are
// answered cancelled.
func (a *App) ChatClose(chatID string) error {
	s, err := a.chats()
	if err != nil {
		return err
	}
	return s.ChatClose(a.appCtx(), chatID)
}

// ChatList returns the window's open chats, so a reloaded frontend can find
// them again.
func (a *App) ChatList() []engine.ChatState {
	s, err := a.chats()
	if err != nil {
		return []engine.ChatState{}
	}
	out, err := s.ChatList(a.appCtx())
	if err != nil || out == nil {
		return []engine.ChatState{}
	}
	return out
}

// SetChatsOpen tells the window's close guard whether the window has an
// open chat. A host's chats run on the host, which the guard cannot ask
// without waiting; the frontend knows them. Closing the window or quitting
// then asks first, as it does for running terminals.
func (a *App) SetChatsOpen(open bool) {
	a.chatsOpen.Store(open)
}

// windowBusy reports whether closing the window would end something that
// runs: a terminal or a chat. The close guard calls it on the UI thread, so
// it never waits. This computer's chats are read here too, so a window
// whose frontend has not reported a new chat yet still asks.
func (a *App) windowBusy() bool {
	if a.terminals != nil && a.terminals.AnyRunning() {
		return true
	}
	if a.chatsOpen.Load() {
		return true
	}
	m, err := a.ownChats()
	return err == nil && m.Any()
}

// closeChats ends this computer's chats of this window. A host's chats end
// with the connection to it.
func (a *App) closeChats() {
	if m, err := a.ownChats(); err == nil {
		m.CloseAll()
	}
}
