package acp

import (
	"encoding/json"
	"testing"
)

// Payloads below are trimmed copies of what the adapters sent in the spike
// (docs/plan/acp-chat-spike.md).

func decodeUpdate(t *testing.T, params string) SessionNotification {
	t.Helper()
	var n SessionNotification
	if err := json.Unmarshal([]byte(params), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNewSessionRequestSendsEmptyMCPServers(t *testing.T) {
	b, err := json.Marshal(NewSessionRequest{Cwd: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"cwd":"/p","mcpServers":[]}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	b, err = json.Marshal(LoadSessionRequest{SessionID: "s", Cwd: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"sessionId":"s","cwd":"/p","mcpServers":[]}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestUnknownUpdateKeepsRaw(t *testing.T) {
	const raw = `{"sessionUpdate":"brand_new_kind","x":{"y":[1,2]},"_meta":{"vendor":true}}`
	n := decodeUpdate(t, `{"sessionId":"s","update":`+raw+`}`)
	if n.Update.Kind != "brand_new_kind" || string(n.Update.Raw) != raw {
		t.Fatalf("update = %+v", n.Update)
	}
	if _, err := n.Update.AsChunk(); err == nil {
		t.Fatal("AsChunk on an unknown kind succeeded")
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"sessionId":"s","update":` + raw + `}`; string(b) != want {
		t.Fatalf("re-encoded %s", b)
	}
}

func TestThoughtChunkMessageID(t *testing.T) {
	n := decodeUpdate(t, `{"sessionId":"ses_1","update":{"sessionUpdate":"agent_thought_chunk","messageId":"msg_1c8eed:reasoning:0","content":{"type":"text","text":"Thinking"}}}`)
	c, err := n.Update.AsChunk()
	if err != nil {
		t.Fatal(err)
	}
	if c.MessageID != "msg_1c8eed:reasoning:0" || c.Content.Text != "Thinking" {
		t.Fatalf("chunk = %+v", c)
	}
}

func TestToolCallShapes(t *testing.T) {
	// Claude: a Write with a diff whose oldText is null.
	n := decodeUpdate(t, `{"sessionId":"s","update":{"_meta":{"claudeCode":{"toolName":"Write"}},"toolCallId":"toolu_01","sessionUpdate":"tool_call","rawInput":{"file_path":"/p/a.txt"},"status":"pending","title":"Write a.txt","kind":"edit","content":[{"type":"diff","path":"/p/a.txt","oldText":null,"newText":"hi\n"}],"locations":[{"path":"/p/a.txt"}]}}`)
	tc, err := n.Update.AsToolCall()
	if err != nil {
		t.Fatal(err)
	}
	if tc.ToolCallID != "toolu_01" || *tc.Status != "pending" || *tc.Kind != "edit" {
		t.Fatalf("tool call = %+v", tc)
	}
	d := (*tc.Content)[0]
	if d.Type != "diff" || d.OldText != nil || d.NewText != "hi\n" || d.Path != "/p/a.txt" {
		t.Fatalf("diff = %+v", d)
	}

	// Codex: terminal content, output only in _meta.
	n = decodeUpdate(t, `{"sessionId":"s","update":{"sessionUpdate":"tool_call_update","toolCallId":"exec-1","status":"completed","_meta":{"terminal_output_delta":{"data":"a.txt\n"},"terminal_exit":{"exit_code":0}}}}`)
	tc, err = n.Update.AsToolCall()
	if err != nil {
		t.Fatal(err)
	}
	if tc.Title != nil || tc.Content != nil || tc.Locations != nil {
		t.Fatalf("absent fields decoded as present: %+v", tc)
	}
	var meta struct {
		Delta struct{ Data string } `json:"terminal_output_delta"`
		Exit  struct {
			Code int `json:"exit_code"`
		} `json:"terminal_exit"`
	}
	if err := json.Unmarshal(tc.Meta, &meta); err != nil || meta.Delta.Data != "a.txt\n" || meta.Exit.Code != 0 {
		t.Fatalf("meta = %s (%v)", tc.Meta, err)
	}

	// An empty content list is present, unlike an absent one.
	n = decodeUpdate(t, `{"sessionId":"s","update":{"sessionUpdate":"tool_call","toolCallId":"toolu_02","title":"Preparing file…","content":[],"locations":[]}}`)
	if tc, err = n.Update.AsToolCall(); err != nil || tc.Content == nil || len(*tc.Content) != 0 {
		t.Fatalf("tool call = %+v (%v)", tc, err)
	}
}

func TestConfigOptionUpdates(t *testing.T) {
	n := decodeUpdate(t, `{"sessionId":"s","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"mode","name":"Mode","category":"mode","type":"select","currentValue":"read-only","options":[{"value":"read-only","name":"Read-only"},{"value":"agent-full-access","name":"Full access"}]},{"id":"model","name":"Model","type":"select","currentValue":"m1","options":[{"group":"openai","name":"OpenAI","options":[{"value":"m1","name":"M1"}]}]}]}}`)
	u, err := n.Update.AsConfigOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(u.ConfigOptions) != 2 {
		t.Fatalf("options = %+v", u.ConfigOptions)
	}
	mode := u.ConfigOptions[0]
	if mode.ID != "mode" || string(mode.CurrentValue) != `"read-only"` || len(mode.Options) != 2 {
		t.Fatalf("mode = %+v", mode)
	}
	group := u.ConfigOptions[1].Options[0]
	if group.Group != "openai" || len(group.Options) != 1 || group.Options[0].Value != "m1" {
		t.Fatalf("group = %+v", group)
	}

	n = decodeUpdate(t, `{"sessionId":"s","update":{"sessionUpdate":"current_mode_update","currentModeId":"default"}}`)
	m, err := n.Update.AsCurrentMode()
	if err != nil || m.CurrentModeID != "default" {
		t.Fatalf("mode = %+v (%v)", m, err)
	}
}

func TestPlanUpdate(t *testing.T) {
	n := decodeUpdate(t, `{"sessionId":"s","update":{"sessionUpdate":"plan","entries":[{"content":"Read","priority":"high","status":"completed"},{"content":"Write","priority":"medium","status":"pending"}]}}`)
	p, err := n.Update.AsPlan()
	if err != nil || len(p.Entries) != 2 || p.Entries[1].Status != "pending" {
		t.Fatalf("plan = %+v (%v)", p, err)
	}
}

func TestPermissionRequest(t *testing.T) {
	var r RequestPermissionRequest
	err := json.Unmarshal([]byte(`{"toolCall":{"toolCallId":"call_1","title":"touch x","kind":"execute","status":"pending","rawInput":{"command":"touch x"}},"options":[{"optionId":"allow_once","name":"Allow","kind":"allow_once"},{"optionId":"accept_execpolicy_amendment","name":"Don't ask again","kind":"allow_always"},{"optionId":"cancel","name":"Cancel","kind":"reject_once"}],"sessionId":"s"}`), &r)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := r.Option("accept_execpolicy_amendment")
	if !ok || o.Kind != PermissionAllowAlways {
		t.Fatalf("option = %+v, %v", o, ok)
	}
	if _, ok := r.Option("nope"); ok {
		t.Fatal("unknown option found")
	}

	b, _ := json.Marshal(Cancelled())
	if string(b) != `{"outcome":{"outcome":"cancelled"}}` {
		t.Fatalf("cancelled = %s", b)
	}
}

func TestCanResume(t *testing.T) {
	var caps AgentCapabilities
	if caps.CanResume() {
		t.Fatal("empty capabilities can resume")
	}
	if err := json.Unmarshal([]byte(`{"sessionCapabilities":{"resume":null}}`), &caps); err != nil {
		t.Fatal(err)
	}
	if caps.CanResume() {
		t.Fatal("resume: null counts as supported")
	}
	if err := json.Unmarshal([]byte(`{"sessionCapabilities":{"resume":{}}}`), &caps); err != nil {
		t.Fatal(err)
	}
	if !caps.CanResume() {
		t.Fatal("resume: {} not supported")
	}
}

func TestPromptBlocks(t *testing.T) {
	for _, c := range []struct {
		block ContentBlock
		want  string
	}{
		{ImageBlock("image/png", "iVBORw0KGgo="), `{"type":"image","data":"iVBORw0KGgo=","mimeType":"image/png"}`},
		{ResourceBlock(TextResource{URI: "file:///p/a.go", MimeType: "text/x-go", Text: "package a\n"}), `{"type":"resource","resource":{"uri":"file:///p/a.go","mimeType":"text/x-go","text":"package a\n"}}`},
		// An empty file still sends its (empty) text.
		{ResourceBlock(TextResource{URI: "file:///p/empty.txt"}), `{"type":"resource","resource":{"uri":"file:///p/empty.txt","text":""}}`},
	} {
		b, err := json.Marshal(c.block)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != c.want {
			t.Errorf("got %s, want %s", b, c.want)
		}
	}
}
