package app

import (
	"errors"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

// ErrChatRemote refuses a chat while the window shows a WSL distribution or
// an SSH host: chats run on this computer only, for now.
var ErrChatRemote = errors.New("chat works for this computer's sessions only; connected hosts do not support it yet")

// errChatUnavailable means the window has no local engine to run chats.
var errChatUnavailable = errors.New("chat is not available")

// localChats returns the chat manager of this window's local engine, unless
// the window shows a connected host.
func (a *App) localChats() (*engine.ChatManager, error) {
	if a.route().host != "" {
		return nil, ErrChatRemote
	}
	return a.ownChats()
}

// ownChats returns the local chat manager whatever the window shows. Ending
// a chat or a turn is always allowed.
func (a *App) ownChats() (*engine.ChatManager, error) {
	if a.localEngine == nil || a.localEngine.Chats() == nil {
		return nil, errChatUnavailable
	}
	return a.localEngine.Chats(), nil
}

// ChatAgents reports, for each agent that can run in a chat, whether its ACP
// adapter is installed and how to install it.
func (a *App) ChatAgents() ([]engine.ChatAgent, error) {
	m, err := a.localChats()
	if err != nil {
		return nil, err
	}
	return m.Agents(a.appCtx())
}

// ChatStart starts the agent's ACP adapter in req.Dir: a new session, or the
// session req.Resume names. Updates arrive as chat:update events and the end
// as chat:exit.
func (a *App) ChatStart(req engine.ChatStartRequest) (engine.ChatState, error) {
	m, err := a.localChats()
	if err != nil {
		return engine.ChatState{}, err
	}
	return m.Start(a.appCtx(), req)
}

// ChatPrompt sends text to the chat's agent. It returns once the turn has
// started; the turn's end arrives as a turn_end update.
func (a *App) ChatPrompt(chatID, text string) error {
	m, err := a.localChats()
	if err != nil {
		return err
	}
	return m.Prompt(chatID, text)
}

// ChatCancel cancels the chat's running turn and answers its pending
// permission requests cancelled.
func (a *App) ChatCancel(chatID string) error {
	m, err := a.ownChats()
	if err != nil {
		return err
	}
	return m.Cancel(chatID)
}

// ChatRespond answers a permission request with one of its options, or
// cancelled when optionID is empty.
func (a *App) ChatRespond(chatID, permissionID, optionID string) error {
	m, err := a.ownChats()
	if err != nil {
		return err
	}
	return m.Respond(chatID, permissionID, optionID)
}

// ChatSetOption sets one of the session's options (mode, model, effort) and
// returns them all as the agent reports them.
func (a *App) ChatSetOption(chatID, configID string, value any) ([]acp.ConfigOption, error) {
	m, err := a.localChats()
	if err != nil {
		return nil, err
	}
	return m.SetOption(a.appCtx(), chatID, configID, value)
}

// ChatClose ends the chat and its agent. Pending permission requests are
// answered cancelled.
func (a *App) ChatClose(chatID string) error {
	m, err := a.ownChats()
	if err != nil {
		return err
	}
	return m.Close(chatID)
}

// ChatList returns the window's open chats, so a reloaded frontend can find
// them again.
func (a *App) ChatList() []engine.ChatState {
	m, err := a.ownChats()
	if err != nil {
		return []engine.ChatState{}
	}
	return m.Chats()
}

// closeChats ends every chat of this window.
func (a *App) closeChats() {
	if m, err := a.ownChats(); err == nil {
		m.CloseAll()
	}
}
