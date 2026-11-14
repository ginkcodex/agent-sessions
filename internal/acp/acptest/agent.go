// Package acptest is a scripted fake ACP agent for tests. The test binary
// starts itself as the agent: call Main first in TestMain, then start the
// agent with Command.
//
// A prompt's first word picks what the turn does:
//
//	stream N     N agent_message_chunk updates of one message, then end_turn
//	tool         a tool_call, its completed update and a plan
//	permission   a tool_call and a permission request; the answer is reported
//	             as a chunk "allowed:<id>" or "rejected:<id>", and a
//	             cancelled answer ends the turn cancelled
//	hang         waits for session/cancel, then ends cancelled
//	hold         waits for session/cancel (ends cancelled) or a steer (ends
//	             end_turn)
//	crash        writes "fake agent crashed" to stderr and exits with status 3
//	auth         fails with -32000 "Authentication required"
//	opened       one chunk naming how the session was opened: session/new,
//	             session/load or session/resume
//	spawn        starts a child that shares stdout and sleeps; the chunk is
//	             "child <pid>"
//	stubborn     from now on, ignores a closed stdin (SIGTERM still works)
//	immortal     from now on, ignores a closed stdin and SIGTERM
//	flood        from now on, ignores a closed stdin; writes one line longer
//	             than acp.DefaultMaxFrameBytes, then waits for session/cancel
//	blocks       one chunk listing the prompt's blocks, as describeBlocks
//	             does; also any prompt that does not start with text
//	question F   asks form F (QuestionForms; "claude" when left out) with
//	             elicitation/create; the answer is reported as a chunk
//	             "accept <content JSON>", "decline", "cancel" or "error
//	             <code>", and a cancel ends the turn cancelled. Without the
//	             client's form capability the chunk is "no elicitation"
//	withdraw MS  asks the "claude" form and withdraws it with
//	             $/cancel_request after MS milliseconds; the chunk is
//	             "withdrawn"
//	askexit      asks the "claude" form, then exits with status 3 without
//	             waiting for the answer
//	commands N   an available_commands_update with N commands (0 clears),
//	             then end_turn
//	anything     echoed back as one chunk
//
// session/new and session/load send a two-command catalog before replying;
// a load sends it among its replayed history.
//
// _session/steering during a turn reports the message as a chunk
// "steered:<text>", sent before the reply, and answers injected; with no
// turn running it answers promptRequired.
//
// Environment, set through Command's env:
//
//	ACPTEST_PROTOCOL=N       reply with protocol version N
//	ACPTEST_NO_RESUME=1      do not advertise session/resume
//	ACPTEST_NO_LOAD=1        do not advertise session/load
//	ACPTEST_STDERR_BYTES=N   write N bytes of numbered stderr lines at start
//	ACPTEST_TEXT_ONLY=1      advertise no image or embedded-context prompts
//	ACPTEST_NO_STEER=1       do not advertise or answer _session/steering
//
// Loading or resuming the session "in-use" fails as codex-acp does while
// another Codex process holds the session: -32600 with data.reason
// "thread_active_writer".
package acptest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
)

const (
	envAgent = "ACPTEST_AGENT"
	envChild = "ACPTEST_CHILD"
)

// Main runs the fake agent, or its child, when the test binary was started
// as one, and exits. Otherwise it returns at once.
func Main() {
	switch {
	case os.Getenv(envChild) != "":
		for {
			time.Sleep(time.Hour)
		}
	case os.Getenv(envAgent) != "":
		os.Exit(run())
	}
}

// Command starts the fake agent in dir. env adds KEY=VALUE settings.
func Command(dir string, env ...string) acp.Command {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return acp.Command{
		Argv: []string{exe, "-test.run=^$"},
		Dir:  dir,
		// A race-enabled agent would otherwise sleep a second at every exit.
		Env: append([]string{envAgent + "=1", "GORACE=" + strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0")}, env...),
	}
}

type agent struct {
	conn  *acp.Conn
	ready chan struct{} // closed once conn is set; handlers may run before

	mu       sync.Mutex
	next     int
	turns    map[string]context.CancelFunc
	holds    map[string]chan struct{} // a hold turn's session -> its steer signal
	opened   map[string]string        // session ID -> the method that opened it
	mode     string
	stubborn bool
	forms    bool // the client advertised form elicitation
}

// QuestionForms are the forms the "question" prompt asks, shaped like each
// adapter's (docs/plan/chat-questions-adapters.md), plus two the client
// must refuse.
var QuestionForms = map[string]map[string]any{
	"claude": {
		"mode": "form", "toolCallId": "toolu_ask", "message": "Please answer the following questions.",
		"requestedSchema": map[string]any{"type": "object", "properties": json.RawMessage(`{
			"question_0": {"type": "string", "title": "Approach", "description": "Which approach?",
				"oneOf": [{"const": "Extend existing module (Recommended)", "title": "Extend existing module (Recommended)", "description": "Least new code"},
					{"const": "New module", "title": "New module"}]},
			"question_0_custom": {"type": "string", "title": "Other", "description": "Type your own answer"},
			"question_1": {"type": "array", "title": "Extras", "description": "Extras to include?",
				"items": {"anyOf": [{"const": "Unit tests", "title": "Unit tests"}, {"const": "Documentation", "title": "Documentation"}]}},
			"question_1_custom": {"type": "string", "title": "Other", "description": "Type your own answer"}
		}`)},
	},
	"codex": {
		"mode": "form", "toolCallId": "item_1", "message": "Codex needs your input to continue.",
		"_meta": map[string]any{"codex": map[string]any{"autoResolutionMs": 60000}},
		"requestedSchema": map[string]any{"type": "object", "required": []string{"confirm"}, "properties": json.RawMessage(`{
			"confirm": {"type": "string", "title": "This will overwrite src/config.ts. Continue?", "description": "Confirm",
				"_meta": {"codex": {"isOther": true, "isSecret": false}},
				"oneOf": [{"const": "Yes", "title": "Yes"}, {"const": "No", "title": "No"},
					{"const": "Other", "title": "Other", "description": "Provide a different answer in the note field."}]},
			"confirm_note": {"type": "string", "title": "Additional answer or note",
				"_meta": {"codex": {"questionId": "confirm", "role": "user_note", "isSecret": false}}}
		}`)},
	},
	"opencode": {
		"mode": "form", "message": "Pick the tests to run",
		"requestedSchema": map[string]any{"type": "object", "required": []string{"tests", "retries"}, "properties": json.RawMessage(`{
			"tests": {"type": "array", "title": "Tests", "minItems": 1, "maxItems": 2,
				"items": {"anyOf": [{"const": "unit", "title": "Unit"}, {"const": "e2e", "title": "End to end"}, {"const": "lint", "title": "Lint"}]}},
			"tests_custom": {"type": "string", "title": "Tests (other)", "description": "Add your own answer", "maxLength": 20},
			"retries": {"type": "integer", "title": "Retries", "minimum": 0, "maximum": 3},
			"verbose": {"type": "boolean", "title": "Verbose", "default": false}
		}`)},
	},
	"url":   {"mode": "url", "message": "Log in", "url": "https://example.invalid/login", "elicitationId": "e1"},
	"email": {"mode": "form", "message": "Your email", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}}}},
}

func run() int {
	if n, _ := strconv.Atoi(os.Getenv("ACPTEST_STDERR_BYTES")); n > 0 {
		var b strings.Builder
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "stderr line %d\n", i)
		}
		_, _ = os.Stderr.WriteString(b.String())
	}
	a := &agent{turns: map[string]context.CancelFunc{}, holds: map[string]chan struct{}{}, opened: map[string]string{}, mode: "ask", ready: make(chan struct{})}
	a.conn = acp.NewConn(os.Stdin, os.Stdout, acp.ConnOptions{
		OnRequest:      a.request,
		OnNotification: a.notification,
	})
	close(a.ready)
	<-a.conn.Done()
	a.mu.Lock()
	stubborn := a.stubborn
	a.mu.Unlock()
	if stubborn {
		for {
			time.Sleep(time.Hour)
		}
	}
	return 0
}

func (a *agent) configOptions() []acp.ConfigOption {
	a.mu.Lock()
	defer a.mu.Unlock()
	return []acp.ConfigOption{{
		ID: "mode", Name: "Mode", Category: "mode", Type: "select",
		CurrentValue: json.RawMessage(strconv.Quote(a.mode)),
		Options: []acp.ConfigSelectOption{
			{Value: "ask", Name: "Ask"},
			{Value: "auto", Name: "Auto"},
		},
	}}
}

func (a *agent) request(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case acp.MethodInitialize:
		var p acp.InitializeRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		a.mu.Lock()
		a.forms = p.ClientCapabilities.Elicitation != nil && p.ClientCapabilities.Elicitation.Form != nil
		a.mu.Unlock()
		version := acp.ProtocolVersion
		if v, err := strconv.Atoi(os.Getenv("ACPTEST_PROTOCOL")); err == nil {
			version = v
		}
		caps := map[string]json.RawMessage{"list": json.RawMessage("{}")}
		if os.Getenv("ACPTEST_NO_RESUME") == "" {
			caps["resume"] = json.RawMessage("{}")
		}
		var meta json.RawMessage
		if os.Getenv("ACPTEST_NO_STEER") == "" {
			meta = json.RawMessage(`{"steering":{"supported":true}}`)
		}
		return acp.InitializeResponse{
			Meta:            meta,
			ProtocolVersion: version,
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession:         os.Getenv("ACPTEST_NO_LOAD") == "",
				SessionCapabilities: caps,
				PromptCapabilities: acp.PromptCapabilities{
					Image:           os.Getenv("ACPTEST_TEXT_ONLY") == "",
					EmbeddedContext: os.Getenv("ACPTEST_TEXT_ONLY") == "",
				},
			},
			AuthMethods: []acp.AuthMethod{{ID: "fake-login", Name: "Login", Description: "Run `fake auth login` in the terminal"}},
			AgentInfo:   &acp.Implementation{Name: "acptest", Version: "1"},
		}, nil
	case acp.MethodSessionNew:
		a.mu.Lock()
		a.next++
		id := fmt.Sprintf("fake-%d-%d", os.Getpid(), a.next)
		a.opened[id] = method
		a.mu.Unlock()
		a.commands(id, 2)
		return acp.SessionSetup{SessionID: id, ConfigOptions: a.configOptions()}, nil
	case acp.MethodSessionLoad, acp.MethodSessionResume:
		var p acp.LoadSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		if p.SessionID == "in-use" {
			return nil, &acp.Error{Code: acp.CodeInvalidRequest, Message: "Invalid request: This Codex session is in use by another Codex client",
				Data: json.RawMessage(`{"reason":"thread_active_writer","threadId":"in-use","details":"thread in-use already has an active writer"}`)}
		}
		a.mu.Lock()
		a.opened[p.SessionID] = method
		a.mu.Unlock()
		if method == acp.MethodSessionLoad {
			a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateUserMessageChunk, "content": acp.TextBlock("earlier prompt")})
			a.commands(p.SessionID, 2)
			a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateAgentMessageChunk, "messageId": "msg_old", "content": acp.TextBlock("earlier reply")})
		}
		return acp.SessionSetup{ConfigOptions: a.configOptions()}, nil
	case acp.MethodSessionSetConfigOption:
		var p acp.SetConfigOptionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		v, _ := p.Value.(string)
		if p.ConfigID != "mode" || (v != "ask" && v != "auto") {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: "unknown option value"}
		}
		a.mu.Lock()
		a.mode = v
		a.mu.Unlock()
		opts := a.configOptions()
		a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateConfigOption, "configOptions": opts})
		return acp.SetConfigOptionResponse{ConfigOptions: opts}, nil
	case acp.MethodSessionPrompt:
		var p acp.PromptRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		return a.prompt(p)
	case acp.MethodSessionSteering:
		if os.Getenv("ACPTEST_NO_STEER") != "" {
			break
		}
		var p acp.SteerRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		return a.steer(p), nil
	}
	return nil, acp.MethodNotFound(method)
}

func (a *agent) steer(p acp.SteerRequest) acp.SteerResponse {
	a.mu.Lock()
	_, running := a.turns[p.SessionID]
	hold := a.holds[p.SessionID]
	a.mu.Unlock()
	if !running {
		if p.Meta.Steering.IdleBehavior != acp.SteerIdlePromptRequired {
			return acp.SteerResponse{Outcome: acp.SteerStartedNewTurn}
		}
		return acp.SteerResponse{Outcome: acp.SteerPromptRequired, Reason: "noRunningTurn"}
	}
	text := ""
	if len(p.Prompt) > 0 {
		text = p.Prompt[0].Text
	}
	a.chunk(p.SessionID, "msg_steered", "steered:"+text)
	if hold != nil {
		select {
		case hold <- struct{}{}:
		default:
		}
	}
	return acp.SteerResponse{Outcome: acp.SteerInjected}
}

func (a *agent) notification(method string, params json.RawMessage) {
	if method != acp.MethodSessionCancel {
		return
	}
	var p acp.CancelNotification
	if json.Unmarshal(params, &p) != nil {
		return
	}
	a.mu.Lock()
	if cancel := a.turns[p.SessionID]; cancel != nil {
		cancel()
	}
	a.mu.Unlock()
}

func (a *agent) c() *acp.Conn {
	<-a.ready
	return a.conn
}

func (a *agent) update(sessionID string, update map[string]any) {
	_ = a.c().Notify(acp.MethodSessionUpdate, map[string]any{"sessionId": sessionID, "update": update})
}

// commands sends a catalog of n commands: plugin:cmd1 (with a hint), cmd2…
func (a *agent) commands(sessionID string, n int) {
	list := []acp.AvailableCommand{}
	for i := 1; i <= n; i++ {
		c := acp.AvailableCommand{Name: fmt.Sprintf("cmd%d", i), Description: fmt.Sprintf("Command %d", i)}
		if i == 1 {
			c.Name, c.Input = "plugin:cmd1", &acp.AvailableCommandInput{Hint: "[args]"}
		}
		list = append(list, c)
	}
	a.update(sessionID, map[string]any{"sessionUpdate": acp.UpdateAvailableCommands, "availableCommands": list})
}

func (a *agent) chunk(sessionID, messageID, text string) {
	a.update(sessionID, map[string]any{"sessionUpdate": acp.UpdateAgentMessageChunk, "messageId": messageID, "content": acp.TextBlock(text)})
}

func (a *agent) prompt(p acp.PromptRequest) (any, error) {
	text := ""
	if len(p.Prompt) > 0 {
		text = p.Prompt[0].Text
	}
	fields := strings.Fields(text)
	word := ""
	if len(fields) > 0 {
		word = fields[0]
	}
	if len(p.Prompt) > 0 && p.Prompt[0].Type != "text" {
		word = "blocks"
	}
	if word == "crash" {
		_, _ = os.Stderr.WriteString("fake agent crashed\n")
		os.Exit(3)
	}
	if word == "askexit" {
		params := map[string]any{"sessionId": p.SessionID}
		for k, v := range QuestionForms["claude"] {
			params[k] = v
		}
		go func() { _ = a.c().Call(context.Background(), acp.MethodElicitationCreate, params, nil) }()
		time.Sleep(200 * time.Millisecond)
		os.Exit(3)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.mu.Lock()
	a.turns[p.SessionID] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.turns, p.SessionID)
		a.mu.Unlock()
	}()

	end := func(reason string) (any, error) { return acp.PromptResponse{StopReason: reason}, nil }
	switch word {
	case "commands":
		n := 0
		if len(fields) > 1 {
			n, _ = strconv.Atoi(fields[1])
		}
		a.commands(p.SessionID, n)
		return end(acp.StopEndTurn)
	case "stream":
		n := 3
		if len(fields) > 1 {
			n, _ = strconv.Atoi(fields[1])
		}
		for i := range n {
			a.chunk(p.SessionID, "msg_stream", fmt.Sprintf("%d ", i))
		}
		return end(acp.StopEndTurn)
	case "tool":
		a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateToolCall, "toolCallId": "call_1", "title": "List files", "kind": "execute", "status": "pending", "rawInput": map[string]string{"command": "ls"}})
		a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateToolCallUpdate, "toolCallId": "call_1", "status": "completed", "content": []any{map[string]any{"type": "content", "content": acp.TextBlock("a.txt")}}})
		a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdatePlan, "entries": []acp.PlanEntry{{Content: "List files", Priority: "high", Status: "completed"}}})
		return end(acp.StopEndTurn)
	case "permission":
		a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateToolCall, "toolCallId": "call_p", "title": "touch x", "kind": "execute", "status": "pending"})
		var r acp.RequestPermissionResponse
		err := a.c().Call(context.Background(), acp.MethodRequestPermission, map[string]any{
			"sessionId": p.SessionID,
			"toolCall":  map[string]any{"toolCallId": "call_p", "title": "touch x", "kind": "execute"},
			"options": []acp.PermissionOption{
				{OptionID: "allow-once", Name: "Yes", Kind: acp.PermissionAllowOnce},
				{OptionID: "allow-always", Name: "Always", Kind: acp.PermissionAllowAlways},
				{OptionID: "reject", Name: "No", Kind: acp.PermissionRejectOnce},
			},
		}, &r)
		if err != nil {
			return nil, err
		}
		switch {
		case r.Outcome.Outcome == "cancelled":
			return end(acp.StopCancelled)
		case strings.HasPrefix(r.Outcome.OptionID, "allow"):
			a.chunk(p.SessionID, "msg_perm", "allowed:"+r.Outcome.OptionID)
		default:
			a.chunk(p.SessionID, "msg_perm", "rejected:"+r.Outcome.OptionID)
		}
		return end(acp.StopEndTurn)
	case "opened":
		a.mu.Lock()
		how := a.opened[p.SessionID]
		a.mu.Unlock()
		a.chunk(p.SessionID, "msg_opened", how)
		return end(acp.StopEndTurn)
	case "hang":
		<-ctx.Done()
		return end(acp.StopCancelled)
	case "hold":
		steered := make(chan struct{}, 1)
		a.mu.Lock()
		a.holds[p.SessionID] = steered
		a.mu.Unlock()
		defer func() {
			a.mu.Lock()
			delete(a.holds, p.SessionID)
			a.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return end(acp.StopCancelled)
		case <-steered:
			return end(acp.StopEndTurn)
		}
	case "auth":
		return nil, &acp.Error{Code: acp.CodeAuthRequired, Message: "Authentication required"}
	case "spawn":
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		child := exec.Command(exe, "-test.run=^$")
		child.Env = append(os.Environ(), envChild+"=1")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			return nil, err
		}
		a.chunk(p.SessionID, "msg_spawn", fmt.Sprintf("child %d", child.Process.Pid))
		return end(acp.StopEndTurn)
	case "blocks":
		a.chunk(p.SessionID, "msg_blocks", describeBlocks(p.Prompt))
		return end(acp.StopEndTurn)
	case "question", "withdraw":
		return a.question(p.SessionID, word, fields[1:])
	case "flood":
		a.mu.Lock()
		a.stubborn = true
		a.mu.Unlock()
		line := make([]byte, acp.DefaultMaxFrameBytes+1)
		for i := range line {
			line[i] = 'x'
		}
		// Straight to stdout: not a message, only too long to be one.
		_, _ = os.Stdout.Write(append(line, '\n'))
		<-ctx.Done()
		return end(acp.StopCancelled)
	case "stubborn", "immortal":
		a.mu.Lock()
		a.stubborn = true
		a.mu.Unlock()
		if word == "immortal" {
			signal.Ignore(syscall.SIGTERM)
		}
		a.chunk(p.SessionID, "msg_mode", word)
		return end(acp.StopEndTurn)
	}
	a.chunk(p.SessionID, "msg_echo", text)
	return end(acp.StopEndTurn)
}

func (a *agent) question(sessionID, word string, args []string) (any, error) {
	end := func(reason string) (any, error) { return acp.PromptResponse{StopReason: reason}, nil }
	a.mu.Lock()
	forms := a.forms
	a.mu.Unlock()
	if !forms {
		a.chunk(sessionID, "msg_question", "no elicitation")
		return end(acp.StopEndTurn)
	}
	name := "claude"
	if word == "question" && len(args) > 0 {
		name = args[0]
	}
	form, ok := QuestionForms[name]
	if !ok {
		return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: "unknown form " + name}
	}
	params := map[string]any{"sessionId": sessionID}
	for k, v := range form {
		params[k] = v
	}
	var r acp.CreateElicitationResponse
	if word == "withdraw" {
		ms := 0
		if len(args) > 0 {
			ms, _ = strconv.Atoi(args[0])
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
		defer cancel()
		if err := a.c().CallWithdrawing(ctx, acp.MethodElicitationCreate, params, &r); err != nil {
			a.chunk(sessionID, "msg_question", "withdrawn")
			return end(acp.StopEndTurn)
		}
	} else if err := a.c().Call(context.Background(), acp.MethodElicitationCreate, params, &r); err != nil {
		var e *acp.Error
		if !errors.As(err, &e) {
			return nil, err
		}
		a.chunk(sessionID, "msg_question", fmt.Sprintf("error %d", e.Code))
		return end(acp.StopEndTurn)
	}
	switch r.Action {
	case acp.ElicitationAccept:
		b, _ := json.Marshal(r.Content)
		a.chunk(sessionID, "msg_question", "accept "+string(b))
	case acp.ElicitationCancel:
		a.chunk(sessionID, "msg_question", "cancel")
		return end(acp.StopCancelled)
	default:
		a.chunk(sessionID, "msg_question", r.Action)
	}
	return end(acp.StopEndTurn)
}

// describeBlocks lists a prompt's blocks, one per line: "text <text>",
// "image <mimeType> <bytes of base64>", "resource <uri> <mimeType> <text>",
// or the block's type.
func describeBlocks(blocks []acp.ContentBlock) string {
	lines := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			lines = append(lines, "text "+b.Text)
		case "image":
			lines = append(lines, fmt.Sprintf("image %s %d", b.MimeType, len(b.Data)))
		case "resource":
			var r acp.TextResource
			_ = json.Unmarshal(b.Resource, &r)
			lines = append(lines, fmt.Sprintf("resource %s %s %s", r.URI, r.MimeType, r.Text))
		default:
			lines = append(lines, b.Type)
		}
	}
	return strings.Join(lines, "\n")
}
