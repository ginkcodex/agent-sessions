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
//	crash        writes "fake agent crashed" to stderr and exits with status 3
//	auth         fails with -32000 "Authentication required"
//	spawn        starts a child that shares stdout and sleeps; the chunk is
//	             "child <pid>"
//	stubborn     from now on, ignores a closed stdin (SIGTERM still works)
//	immortal     from now on, ignores a closed stdin and SIGTERM
//	anything     echoed back as one chunk
//
// Environment, set through Command's env:
//
//	ACPTEST_PROTOCOL=N       reply with protocol version N
//	ACPTEST_NO_RESUME=1      do not advertise session/resume
//	ACPTEST_STDERR_BYTES=N   write N bytes of numbered stderr lines at start
package acptest

import (
	"context"
	"encoding/json"
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
	mode     string
	stubborn bool
}

func run() int {
	if n, _ := strconv.Atoi(os.Getenv("ACPTEST_STDERR_BYTES")); n > 0 {
		var b strings.Builder
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "stderr line %d\n", i)
		}
		_, _ = os.Stderr.WriteString(b.String())
	}
	a := &agent{turns: map[string]context.CancelFunc{}, mode: "ask", ready: make(chan struct{})}
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
		version := acp.ProtocolVersion
		if v, err := strconv.Atoi(os.Getenv("ACPTEST_PROTOCOL")); err == nil {
			version = v
		}
		caps := map[string]json.RawMessage{"list": json.RawMessage("{}")}
		if os.Getenv("ACPTEST_NO_RESUME") == "" {
			caps["resume"] = json.RawMessage("{}")
		}
		return acp.InitializeResponse{
			ProtocolVersion: version,
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession:         true,
				SessionCapabilities: caps,
			},
			AuthMethods: []acp.AuthMethod{{ID: "fake-login", Name: "Login", Description: "Run `fake auth login` in the terminal"}},
			AgentInfo:   &acp.Implementation{Name: "acptest", Version: "1"},
		}, nil
	case acp.MethodSessionNew:
		a.mu.Lock()
		a.next++
		id := fmt.Sprintf("fake-%d", a.next)
		a.mu.Unlock()
		return acp.SessionSetup{SessionID: id, ConfigOptions: a.configOptions()}, nil
	case acp.MethodSessionLoad, acp.MethodSessionResume:
		var p acp.LoadSessionRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		if method == acp.MethodSessionLoad {
			a.update(p.SessionID, map[string]any{"sessionUpdate": acp.UpdateUserMessageChunk, "content": acp.TextBlock("earlier prompt")})
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
	}
	return nil, acp.MethodNotFound(method)
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
	if word == "crash" {
		_, _ = os.Stderr.WriteString("fake agent crashed\n")
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
	case "hang":
		<-ctx.Done()
		return end(acp.StopCancelled)
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
