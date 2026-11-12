package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func schema(t *testing.T, s string) *ElicitationSchema {
	t.Helper()
	var out ElicitationSchema
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

// Integer-like keys would be sorted by a map; the fields keep the order sent
// and write it back the same way.
func TestElicitationFieldsKeepOrder(t *testing.T) {
	s := schema(t, `{"type":"object","properties":{"b":{"type":"string"},"10":{"type":"boolean"},"2":{"type":"number","minimum":1}},"required":["10"]}`)
	var keys []string
	for _, f := range s.Properties {
		keys = append(keys, f.Key)
	}
	if strings.Join(keys, ",") != "b,10,2" {
		t.Fatalf("keys = %q", keys)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"type":"object","properties":{"b":{"type":"string"},"10":{"type":"boolean"},"2":{"type":"number","minimum":1}},"required":["10"]}`; string(b) != want {
		t.Fatalf("marshal = %s, want %s", b, want)
	}
	if err := json.Unmarshal([]byte(`{"properties":[1]}`), &ElicitationSchema{}); err == nil {
		t.Fatal("properties as a list were accepted")
	}
}

func TestElicitationSupported(t *testing.T) {
	for _, tc := range []struct {
		schema string
		err    string // "" when supported
	}{
		{`{"type":"object","properties":{}}`, ""},
		{`{"properties":{"a":{"type":"string","minLength":1,"maxLength":5,"pattern":"^[a-z]+$","default":"x"}}}`, ""},
		{`{"type":"object","properties":{"a":{"type":"string","oneOf":[{"const":"x","title":"X","_meta":{"k":1}}]},"b":{"type":"string","enum":["p","q"]}}}`, ""},
		{`{"type":"object","properties":{"a":{"type":"array","items":{"type":"string","enum":["x"]},"minItems":1,"maxItems":1}}}`, ""},
		{`{"type":"object","properties":{"a":{"type":"array","items":{"anyOf":[{"const":"x","title":"X"}]}}}}`, ""},
		{`{"type":"object","properties":{"a":{"type":"boolean"},"b":{"type":"integer","minimum":0},"c":{"type":"number"}}}`, ""},
		{`{"type":"array","properties":{}}`, "not an object"},
		{`{"type":"object","properties":{"a":{"type":"string","format":"email"}}}`, "format"},
		{`{"type":"object","properties":{"a":{"type":"string","pattern":"(?<=x)"}}}`, "pattern"},
		{`{"type":"object","properties":{"a":{"type":"object"}}}`, "not supported"},
		{`{"type":"object","properties":{"a":{"type":"string","properties":{}}}}`, "unknown keys"},
		{`{"type":"object","properties":{"a":{"type":"array","items":{"type":"string"}}}}`, "without options"},
		{`{"type":"object","properties":{"a":{"type":"array","items":{"type":"number","enum":["1"]}}}}`, "a list of"},
		{`{"type":"object","properties":{"a":{"type":"string","enum":["x","x"]}}}`, "twice"},
		{`{"type":"object","properties":{"a":{"type":"string"},"a":{"type":"string"}}}`, "appears twice"},
		{`{"type":"object","properties":{"":{"type":"string"}}}`, "no name"},
		{`{"type":"object","properties":{"a":{"type":"string"}},"required":["b"]}`, "requires"},
	} {
		err := schema(t, tc.schema).Supported()
		if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("%s: err = %v, want %q", tc.schema, err, tc.err)
		}
	}
	if err := (*ElicitationSchema)(nil).Supported(); err == nil {
		t.Fatal("a missing schema is supported")
	}
}

func TestElicitationValidate(t *testing.T) {
	s := schema(t, `{"type":"object","required":["choice","count"],"properties":{
		"choice":{"type":"string","title":"Approach","oneOf":[{"const":"a","title":"A"},{"const":"b","title":"B"}]},
		"plain":{"type":"string","enum":["x","y"]},
		"text":{"type":"string","minLength":2,"maxLength":3,"pattern":"^[a-zé]+$"},
		"many":{"type":"array","items":{"anyOf":[{"const":"p","title":"P"},{"const":"q","title":"Q"},{"const":"r","title":"R"}]},"minItems":1,"maxItems":2},
		"yes":{"type":"boolean"},
		"count":{"type":"integer","minimum":0,"maximum":10},
		"ratio":{"type":"number","maximum":1}
	}}`)
	for _, tc := range []struct {
		content string
		want    string // the validated content as JSON, or the error's text after "!"
	}{
		{`{"choice":"a","count":3}`, `{"choice":"a","count":3}`},
		{
			`{"choice":"b","count":0,"plain":"y","text":"éé","many":["q","p"],"yes":false,"ratio":0.5}`,
			`{"choice":"b","count":0,"many":["q","p"],"plain":"y","ratio":0.5,"text":"éé","yes":false}`,
		},
		{`{"choice":"a","count":3.0,"text":"","plain":null}`, `{"choice":"a","count":3}`},
		{`{"count":3}`, `!"Approach" needs an answer`},
		{`{"choice":"a"}`, `!"count" needs an answer`},
		{`{"choice":"c","count":1}`, `!"c" is not one of the options`},
		{`{"choice":"a","count":1,"plain":"z"}`, `!not one of the options`},
		{`{"choice":1,"count":1}`, `!not text`},
		{`{"choice":"a","count":1,"text":"a"}`, `!at least 2`},
		{`{"choice":"a","count":1,"text":"abcd"}`, `!at most 3`},
		{`{"choice":"a","count":1,"text":"A1"}`, `!expected form`},
		{`{"choice":"a","count":1,"many":[]}`, `!at least 1`},
		{`{"choice":"a","count":1,"many":["p","q","r"]}`, `!at most 2`},
		{`{"choice":"a","count":1,"many":["p","p"]}`, `!chosen twice`},
		{`{"choice":"a","count":1,"many":["s"]}`, `!not one of the options`},
		{`{"choice":"a","count":1,"many":"p"}`, `!not a list`},
		{`{"choice":"a","count":1,"yes":"yes"}`, `!yes or no`},
		{`{"choice":"a","count":1.5}`, `!whole number`},
		{`{"choice":"a","count":11}`, `!above 10`},
		{`{"choice":"a","count":-1}`, `!below 0`},
		{`{"choice":"a","count":1,"ratio":"1"}`, `!not a number`},
		{`{"choice":"a","count":1,"ratio":2}`, `!above 1`},
		{`{"choice":"a","count":1,"other":"x"}`, `!no field "other"`},
	} {
		var content map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tc.content), &content); err != nil {
			t.Fatal(err)
		}
		out, err := s.Validate(content)
		if msg, ok := strings.CutPrefix(tc.want, "!"); ok {
			if err == nil || !strings.Contains(err.Error(), msg) {
				t.Errorf("%s: err = %v, want %q", tc.content, err, msg)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.content, err)
			continue
		}
		if b, _ := json.Marshal(out); string(b) != tc.want {
			t.Errorf("%s: content = %s, want %s", tc.content, b, tc.want)
		}
	}
	// An integer is sent as a whole number, not 3.0 or 3e0.
	out, err := s.Validate(map[string]json.RawMessage{"choice": json.RawMessage(`"a"`), "count": json.RawMessage(`3e0`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["count"].(int64); !ok {
		t.Fatalf("count = %T", out["count"])
	}
}

// A request the agent withdraws ends the handler's context and is answered
// request-cancelled, whatever the handler returns.
func TestCancelRequestWithdrawsHandler(t *testing.T) {
	started := make(chan struct{})
	ended := make(chan error, 1)
	_, agent := newPair(t, ConnOptions{
		OnRequest: func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			ended <- context.Cause(ctx)
			return ElicitationCancelled(), nil
		},
	})
	agent.send(`{"jsonrpc":"2.0","id":"q-1","method":"elicitation/create","params":{}}`)
	<-started
	agent.send(`{"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":"q-1"}}`)
	if err := <-ended; !errors.Is(err, ErrWithdrawn) {
		t.Fatalf("handler ctx cause = %v", err)
	}
	if got, want := agent.recvLine(), fmt.Sprintf(`{"jsonrpc":"2.0","id":"q-1","error":{"code":%d,"message":"request cancelled"}}`, CodeRequestCancelled); got != want {
		t.Fatalf("reply = %s, want %s", got, want)
	}
}

// Withdrawing an unknown or finished request does nothing; a malformed one is
// reported.
func TestCancelRequestUnknown(t *testing.T) {
	var mu sync.Mutex
	var reported []error
	_, agent := newPair(t, ConnOptions{
		OnRequest: func(context.Context, string, json.RawMessage) (any, error) { return map[string]any{}, nil },
		OnProtocolError: func(err error) {
			mu.Lock()
			reported = append(reported, err)
			mu.Unlock()
		},
	})
	agent.send(`{"jsonrpc":"2.0","id":5,"method":"x/quick","params":{}}`)
	if got := agent.recvLine(); got != `{"jsonrpc":"2.0","id":5,"result":{}}` {
		t.Fatalf("reply = %s", got)
	}
	agent.send(`{"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":5}}`)
	agent.send(`{"jsonrpc":"2.0","method":"$/cancel_request","params":{}}`)
	agent.send(`{"jsonrpc":"2.0","id":6,"method":"x/quick","params":{}}`)
	if got := agent.recvLine(); got != `{"jsonrpc":"2.0","id":6,"result":{}}` {
		t.Fatalf("reply = %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "$/cancel_request") {
		t.Fatalf("reported = %v", reported)
	}
}

// CallWithdrawing sends $/cancel_request when its context ends, and the late
// reply is expected rather than reported.
func TestCallWithdrawing(t *testing.T) {
	var mu sync.Mutex
	var reported []error
	c, agent := newPair(t, ConnOptions{OnProtocolError: func(err error) {
		mu.Lock()
		reported = append(reported, err)
		mu.Unlock()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.CallWithdrawing(ctx, MethodElicitationCreate, nil, nil) }()
	req := agent.recv()
	cancel()
	// The pipe has no buffer: read the withdrawal before the call returns.
	if got, want := agent.recvLine(), fmt.Sprintf(`{"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":%s}}`, req.ID); got != want {
		t.Fatalf("withdrawal = %s, want %s", got, want)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":{"code":%d,"message":"request cancelled"}}`, req.ID, CodeRequestCancelled))
	// The connection keeps working and nothing was reported.
	go func() { done <- c.Call(context.Background(), "x/ping", nil, nil) }()
	next := agent.recv()
	agent.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, next.ID))
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ping did not return")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 0 {
		t.Fatalf("reported = %v", reported)
	}
}
