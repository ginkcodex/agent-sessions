package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/chatattach"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

// ErrNoChatAttachments means the host's agent-sessions is too old to send
// attachments to its agents.
var ErrNoChatAttachments = errors.New("this host's agent-sessions cannot send attachments to agents; reconnect to update it")

// maxChatPromptBytes bounds a prompt's text. Prompts are typed by a person;
// this only keeps a peer from handing the agent a frame-sized one.
const maxChatPromptBytes = 1 << 20

// chatRPC holds the chats a connection started. They belong to it: other
// connections cannot reach them, their events go only to it, and they end
// when it does.
type chatRPC struct {
	mu      sync.Mutex
	enabled bool
	closed  bool
	owned   map[string]bool
}

// ChatPromptParams sends text and attachments to a chat's agent. A server
// without Capabilities.ChatAttachments ignores Attachments.
type ChatPromptParams struct {
	ChatID      string                  `json:"chatId"`
	Text        string                  `json:"text"`
	Attachments []chatattach.Attachment `json:"attachments,omitempty"`
}

// ChatParams names a chat.
type ChatParams struct {
	ChatID string `json:"chatId"`
}

// ChatRespondParams answers a permission request; an empty OptionID answers
// it cancelled.
type ChatRespondParams struct {
	ChatID       string `json:"chatId"`
	PermissionID string `json:"permissionId"`
	OptionID     string `json:"optionId,omitempty"`
}

// ChatSetOptionParams sets one of the session's options.
type ChatSetOptionParams struct {
	ChatID   string `json:"chatId"`
	ConfigID string `json:"configId"`
	Value    any    `json:"value"`
}

// chatManager returns the backend's chat manager, or nil.
func chatManager(backend engine.Backend) *engine.ChatManager {
	if b, ok := backend.(engine.ChatBackend); ok {
		return b.Chats()
	}
	return nil
}

func (s *Server) dispatchChat(ctx context.Context, backend engine.Backend, req Request) (any, error) {
	c := &s.chats
	c.mu.Lock()
	enabled, closed := c.enabled, c.closed
	c.mu.Unlock()
	m := chatManager(backend)
	switch {
	case closed:
		return nil, ErrDisconnected
	case !enabled || m == nil:
		return nil, engine.ErrUnsupportedAction
	}
	switch req.Method {
	case "chatAgents":
		return m.Agents(ctx)
	case "chatList":
		var out []engine.ChatState
		for _, st := range m.Chats() {
			if s.ownsChat(st.ChatID) {
				out = append(out, st)
			}
		}
		if out == nil {
			out = []engine.ChatState{}
		}
		return out, nil
	case "chatStart":
		var p engine.ChatStartRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
		}
		p.Emitter = engine.EmitterFunc(s.chatEvent)
		st, err := m.Start(ctx, p)
		if err == nil && !s.ownsChat(st.ChatID) {
			// The connection ended while the chat started.
			_ = m.Close(st.ChatID)
			return nil, ErrDisconnected
		}
		return st, err
	}

	// The rest name a chat this connection must own.
	var p struct {
		ChatID       string                  `json:"chatId"`
		Text         string                  `json:"text"`
		Attachments  []chatattach.Attachment `json:"attachments"`
		PermissionID string                  `json:"permissionId"`
		OptionID     string                  `json:"optionId"`
		ConfigID     string                  `json:"configId"`
		Value        any                     `json:"value"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
	}
	if !s.ownsChat(p.ChatID) {
		return nil, fmt.Errorf("%w: %s", engine.ErrChatNotFound, p.ChatID)
	}
	switch req.Method {
	case "chatPrompt":
		if len(p.Text) > maxChatPromptBytes {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "the prompt is too long"}
		}
		// The engine checks the attachments' number and sizes.
		return nil, m.Prompt(p.ChatID, p.Text, p.Attachments)
	case "chatCancel":
		return nil, m.Cancel(p.ChatID)
	case "chatRespond":
		return nil, m.Respond(p.ChatID, p.PermissionID, p.OptionID)
	case "chatSetOption":
		return m.SetOption(ctx, p.ChatID, p.ConfigID, p.Value)
	default: // chatClose
		return nil, m.Close(p.ChatID)
	}
}

// chatEvent sends one of this connection's chat events to it. The started
// event, which Start emits before it returns, makes the chat ours; the exit,
// always the last event, forgets it.
func (s *Server) chatEvent(name string, payload any) {
	c := &s.chats
	switch ev := payload.(type) {
	case engine.ChatUpdate:
		if ev.Kind == engine.ChatStarted {
			c.mu.Lock()
			if !c.closed {
				c.owned[ev.ChatID] = true
			}
			c.mu.Unlock()
		}
	case engine.ChatExit:
		c.mu.Lock()
		delete(c.owned, ev.ChatID)
		c.mu.Unlock()
	}
	_ = s.Notify(name, payload)
}

func (s *Server) ownsChat(chatID string) bool {
	c := &s.chats
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.owned[chatID]
}

// closeChats ends this connection's chats and waits for their agents to
// exit. Their pending permission requests are answered cancelled. It runs
// once every handler has returned, so no chat starts after it.
func (s *Server) closeChats() {
	c := &s.chats
	c.mu.Lock()
	c.closed = true
	owned := c.owned
	c.owned = map[string]bool{}
	c.mu.Unlock()
	s.mu.Lock()
	m := chatManager(s.backend)
	s.mu.Unlock()
	if m == nil {
		return
	}
	var wg sync.WaitGroup
	for id := range owned {
		wg.Go(func() { _ = m.Close(id) })
	}
	wg.Wait()
}

// chatCall calls a chat method. A host that lacks it, though it claimed the
// capability, does not support chats.
func (c *Client) chatCall(ctx context.Context, method string, params, out any) error {
	err := c.call(ctx, method, params, out)
	var rpcErr *ResponseError
	if errors.As(err, &rpcErr) && rpcErr.Code == CodeMethodNotFound {
		return engine.ErrUnsupportedAction
	}
	return err
}

// ChatAgents reports, for each agent, whether its ACP adapter is installed
// on the host.
func (c *Client) ChatAgents(ctx context.Context) ([]engine.ChatAgent, error) {
	if !c.Capabilities().ACPChat {
		return nil, engine.ErrUnsupportedAction
	}
	var out []engine.ChatAgent
	return out, c.chatCall(ctx, "chatAgents", struct{}{}, &out)
}

// ChatStart starts a chat on the host. Its events arrive as chat:update and
// chat:exit notifications on this connection only.
func (c *Client) ChatStart(ctx context.Context, req engine.ChatStartRequest) (engine.ChatState, error) {
	var out engine.ChatState
	if !c.Capabilities().ACPChat {
		return out, engine.ErrUnsupportedAction
	}
	return out, c.chatCall(ctx, "chatStart", req, &out)
}

// ChatPrompt sends text and attachments to the chat's agent. A host that
// would drop the attachments refuses them with ErrNoChatAttachments.
func (c *Client) ChatPrompt(ctx context.Context, chatID, text string, atts []chatattach.Attachment) error {
	caps := c.Capabilities()
	if !caps.ACPChat {
		return engine.ErrUnsupportedAction
	}
	if len(atts) > 0 && !caps.ChatAttachments {
		return ErrNoChatAttachments
	}
	return c.chatCall(ctx, "chatPrompt", ChatPromptParams{ChatID: chatID, Text: text, Attachments: atts}, nil)
}

// ChatCancel cancels the chat's running turn.
func (c *Client) ChatCancel(ctx context.Context, chatID string) error {
	if !c.Capabilities().ACPChat {
		return engine.ErrUnsupportedAction
	}
	return c.chatCall(ctx, "chatCancel", ChatParams{ChatID: chatID}, nil)
}

// ChatRespond answers a permission request.
func (c *Client) ChatRespond(ctx context.Context, chatID, permissionID, optionID string) error {
	if !c.Capabilities().ACPChat {
		return engine.ErrUnsupportedAction
	}
	return c.chatCall(ctx, "chatRespond", ChatRespondParams{ChatID: chatID, PermissionID: permissionID, OptionID: optionID}, nil)
}

// ChatSetOption sets one of the session's options.
func (c *Client) ChatSetOption(ctx context.Context, chatID, configID string, value any) ([]acp.ConfigOption, error) {
	if !c.Capabilities().ACPChat {
		return nil, engine.ErrUnsupportedAction
	}
	var out []acp.ConfigOption
	return out, c.chatCall(ctx, "chatSetOption", ChatSetOptionParams{ChatID: chatID, ConfigID: configID, Value: value}, &out)
}

// ChatClose ends the chat and its agent.
func (c *Client) ChatClose(ctx context.Context, chatID string) error {
	if !c.Capabilities().ACPChat {
		return engine.ErrUnsupportedAction
	}
	return c.chatCall(ctx, "chatClose", ChatParams{ChatID: chatID}, nil)
}

// ChatList returns the chats this connection started that are still open.
func (c *Client) ChatList(ctx context.Context) ([]engine.ChatState, error) {
	if !c.Capabilities().ACPChat {
		return nil, engine.ErrUnsupportedAction
	}
	var out []engine.ChatState
	return out, c.chatCall(ctx, "chatList", struct{}{}, &out)
}
