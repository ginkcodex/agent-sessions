// Package acp is a client for the Agent Client Protocol (ACP): JSON-RPC 2.0
// over an agent's stdin and stdout, one JSON message per line.
//
// Only the protocol v1 subset the chat mode uses is typed. Session updates
// keep their raw JSON, so kinds this package does not know, and the agents'
// _meta extensions, pass through untouched. docs/plan/acp-chat.md and
// docs/plan/acp-chat-spike.md describe what the adapters send.
package acp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ProtocolVersion is the ACP version the client asks for.
const ProtocolVersion = 1

// Methods the client calls, and the agent's requests and notifications.
const (
	MethodInitialize             = "initialize"
	MethodSessionNew             = "session/new"
	MethodSessionLoad            = "session/load"
	MethodSessionResume          = "session/resume"
	MethodSessionPrompt          = "session/prompt"
	MethodSessionCancel          = "session/cancel"
	MethodSessionSetConfigOption = "session/set_config_option"

	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// JSON-RPC error codes. CodeAuthRequired is ACP's "authentication required".
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeAuthRequired   = -32000
)

// Error is a JSON-RPC error, sent or received.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("acp: %s (code %d)", e.Message, e.Code)
}

// Reason is data.reason, which some adapters set to say why they refused a
// request (codex-acp: "thread_active_writer"), or "" without one.
func (e *Error) Reason() string {
	var d struct {
		Reason string `json:"reason"`
	}
	if len(e.Data) == 0 || json.Unmarshal(e.Data, &d) != nil {
		return ""
	}
	return d.Reason
}

// ReasonThreadActiveWriter is codex-acp's reason when another Codex process,
// usually Codex's background daemon, holds the session's writer lock.
const ReasonThreadActiveWriter = "thread_active_writer"

// IsSessionInUse reports whether err is the agent refusing a session that
// another of its own processes has open.
func IsSessionInUse(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason() == ReasonThreadActiveWriter
}

// MethodNotFound is the reply to a request the client does not handle.
func MethodNotFound(method string) *Error {
	return &Error{Code: CodeMethodNotFound, Message: "method not found: " + method}
}

// IsAuthRequired reports whether err is the agent saying it needs a login.
func IsAuthRequired(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == CodeAuthRequired
}

// Implementation names a client or an agent.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// ClientCapabilities leaves out fs and terminal on purpose: without them the
// agent must not call the client's file or terminal methods. Elicitation is
// set only when the client shows forms.
type ClientCapabilities struct {
	Elicitation *ElicitationCapabilities `json:"elicitation,omitempty"`
}

type InitializeRequest struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
}

type InitializeResponse struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []AuthMethod      `json:"authMethods,omitempty"`
	AgentInfo         *Implementation   `json:"agentInfo,omitempty"`
}

type AgentCapabilities struct {
	LoadSession        bool               `json:"loadSession,omitempty"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	// SessionCapabilities maps a capability (resume, list, close, ...) to its
	// options; a key that is present means the agent supports it.
	SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities,omitempty"`
}

// CanResume reports whether the agent advertises session/resume.
func (c AgentCapabilities) CanResume() bool {
	v, ok := c.SessionCapabilities["resume"]
	return ok && string(v) != "null"
}

type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// AuthMethod is a way to log in, such as "Run `opencode auth login` in the
// terminal".
type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// mcpServers is always sent, as [] when there are none.
type mcpServers []json.RawMessage

func (m mcpServers) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]json.RawMessage(m))
}

type NewSessionRequest struct {
	Cwd        string     `json:"cwd"`
	McpServers mcpServers `json:"mcpServers"`
}

// SessionSetup is what session/new, session/load and session/resume return.
type SessionSetup struct {
	SessionID     string         `json:"sessionId,omitempty"`
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// LoadSessionRequest is the params of session/load and of session/resume.
type LoadSessionRequest struct {
	SessionID  string     `json:"sessionId"`
	Cwd        string     `json:"cwd"`
	McpServers mcpServers `json:"mcpServers"`
}

// ConfigOption is one session setting such as mode, model or effort.
type ConfigOption struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Description  string               `json:"description,omitempty"`
	Category     string               `json:"category,omitempty"`
	Type         string               `json:"type"`
	CurrentValue json.RawMessage      `json:"currentValue,omitempty"`
	Options      []ConfigSelectOption `json:"options,omitempty"`
}

// ConfigSelectOption is a value of a select option, or a named group of
// values when Group is set.
type ConfigSelectOption struct {
	Value       string               `json:"value,omitempty"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Group       string               `json:"group,omitempty"`
	Options     []ConfigSelectOption `json:"options,omitempty"`
}

type SetConfigOptionRequest struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     any    `json:"value"`
}

type SetConfigOptionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// ContentBlock is a prompt or message part. Type is text, image, audio,
// resource_link or resource.
type ContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Name     string          `json:"name,omitempty"`
	Resource json.RawMessage `json:"resource,omitempty"`
}

// TextBlock is a text content block.
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// ImageBlock is an image content block; data is the image in base64. Only
// for agents that advertise promptCapabilities.image.
func ImageBlock(mimeType, data string) ContentBlock {
	return ContentBlock{Type: "image", MimeType: mimeType, Data: data}
}

// TextResource is the text of an embedded resource.
type TextResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	// Text is sent even when empty: the protocol requires it.
	Text string `json:"text"`
}

// ResourceBlock embeds a text resource in a prompt. Only for agents that
// advertise promptCapabilities.embeddedContext.
func ResourceBlock(r TextResource) ContentBlock {
	raw, err := json.Marshal(r)
	if err != nil { // unreachable: strings only
		panic(err)
	}
	return ContentBlock{Type: "resource", Resource: raw}
}

type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// Stop reasons a prompt turn ends with.
const (
	StopEndTurn         = "end_turn"
	StopMaxTokens       = "max_tokens"
	StopMaxTurnRequests = "max_turn_requests"
	StopRefusal         = "refusal"
	StopCancelled       = "cancelled"
)

type PromptResponse struct {
	StopReason string `json:"stopReason"`
}

type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// Session update kinds (the sessionUpdate field).
const (
	UpdateUserMessageChunk  = "user_message_chunk"
	UpdateAgentMessageChunk = "agent_message_chunk"
	UpdateAgentThoughtChunk = "agent_thought_chunk"
	UpdateToolCall          = "tool_call"
	UpdateToolCallUpdate    = "tool_call_update"
	UpdatePlan              = "plan"
	UpdateAvailableCommands = "available_commands_update"
	UpdateCurrentMode       = "current_mode_update"
	UpdateConfigOption      = "config_option_update"
	UpdateSessionInfo       = "session_info_update"
	UpdateUsage             = "usage_update"
)

// SessionNotification is the params of session/update.
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate keeps the update's raw JSON next to its kind, so unknown
// kinds and extension fields survive. The As* methods decode known kinds.
type SessionUpdate struct {
	Kind string
	Raw  json.RawMessage
}

func (u *SessionUpdate) UnmarshalJSON(b []byte) error {
	var head struct {
		Kind string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return err
	}
	u.Kind = head.Kind
	u.Raw = append(json.RawMessage(nil), b...)
	return nil
}

func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	if len(u.Raw) == 0 {
		return json.Marshal(map[string]string{"sessionUpdate": u.Kind})
	}
	return u.Raw, nil
}

// ContentChunk is a user_message_chunk, agent_message_chunk or
// agent_thought_chunk.
type ContentChunk struct {
	Content   ContentBlock `json:"content"`
	MessageID string       `json:"messageId,omitempty"`
}

// AsChunk decodes a message or thought chunk.
func (u SessionUpdate) AsChunk() (ContentChunk, error) {
	var c ContentChunk
	err := u.decode(&c, UpdateUserMessageChunk, UpdateAgentMessageChunk, UpdateAgentThoughtChunk)
	return c, err
}

// ToolCall is a tool_call, or a tool_call_update in which every field but
// ToolCallID may be absent (nil).
type ToolCall struct {
	ToolCallID string             `json:"toolCallId"`
	Title      *string            `json:"title,omitempty"`
	Kind       *string            `json:"kind,omitempty"`
	Status     *string            `json:"status,omitempty"`
	Content    *[]ToolCallContent `json:"content,omitempty"`
	Locations  *[]ToolLocation    `json:"locations,omitempty"`
	RawInput   json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage    `json:"rawOutput,omitempty"`
	Meta       json.RawMessage    `json:"_meta,omitempty"`
}

// ToolCallContent is one of: content (a block), diff, or terminal.
type ToolCallContent struct {
	Type       string        `json:"type"`
	Content    *ContentBlock `json:"content,omitempty"`
	Path       string        `json:"path,omitempty"`
	OldText    *string       `json:"oldText,omitempty"`
	NewText    string        `json:"newText,omitempty"`
	TerminalID string        `json:"terminalId,omitempty"`
}

type ToolLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// AsToolCall decodes a tool_call or tool_call_update.
func (u SessionUpdate) AsToolCall() (ToolCall, error) {
	var t ToolCall
	err := u.decode(&t, UpdateToolCall, UpdateToolCallUpdate)
	return t, err
}

type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"`
	Status   string `json:"status,omitempty"`
}

type Plan struct {
	Entries []PlanEntry `json:"entries"`
}

// AsPlan decodes a plan update, which replaces the previous plan.
func (u SessionUpdate) AsPlan() (Plan, error) {
	var p Plan
	err := u.decode(&p, UpdatePlan)
	return p, err
}

type ConfigOptionUpdate struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// AsConfigOptions decodes a config_option_update.
func (u SessionUpdate) AsConfigOptions() (ConfigOptionUpdate, error) {
	var c ConfigOptionUpdate
	err := u.decode(&c, UpdateConfigOption)
	return c, err
}

type CurrentModeUpdate struct {
	CurrentModeID string `json:"currentModeId"`
}

// AsCurrentMode decodes a current_mode_update.
func (u SessionUpdate) AsCurrentMode() (CurrentModeUpdate, error) {
	var c CurrentModeUpdate
	err := u.decode(&c, UpdateCurrentMode)
	return c, err
}

func (u SessionUpdate) decode(v any, kinds ...string) error {
	for _, k := range kinds {
		if u.Kind == k {
			return json.Unmarshal(u.Raw, v)
		}
	}
	return fmt.Errorf("acp: session update %q is not %v", u.Kind, kinds)
}

// Permission option kinds.
const (
	PermissionAllowOnce    = "allow_once"
	PermissionAllowAlways  = "allow_always"
	PermissionRejectOnce   = "reject_once"
	PermissionRejectAlways = "reject_always"
)

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionRequest is the params of session/request_permission.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCall           `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// Option returns the option with the given ID.
func (r RequestPermissionRequest) Option(id string) (PermissionOption, bool) {
	for _, o := range r.Options {
		if o.OptionID == id {
			return o, true
		}
	}
	return PermissionOption{}, false
}

type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

type RequestPermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// Selected answers a permission request with one of its options.
func Selected(optionID string) RequestPermissionResponse {
	return RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: "selected", OptionID: optionID}}
}

// Cancelled answers a permission request whose turn was cancelled.
func Cancelled() RequestPermissionResponse {
	return RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: "cancelled"}}
}
