package rpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// From ID 1,000,000 a float64 prints as 1e+06 with %v. Replies were matched
// that way, so every call after the millionth waited until its context
// ended. Calls, pings and cancels across the boundary must still match.
func TestCallsMatchRepliesPastOneMillionRequests(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s, c := NewServer(&mockBackend{}, inR, outW), NewClient(outR, inW)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Serve(context.Background())
	}()
	t.Cleanup(func() {
		_ = c.Close()
		_ = s.Close()
		<-done
	})

	if _, err := c.Initialize(t.Context(), InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	atomic.StoreInt64(&c.seq, 999_995)
	for i := range 10 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		got, err := c.Ping(ctx, "x")
		cancel()
		if err != nil || !strings.Contains(got, "x") {
			t.Fatalf("call %d (ID %d): %q, %v", i, atomic.LoadInt64(&c.seq), got, err)
		}
	}

	// The liveness ping's reply clears pinging the same way.
	atomic.StoreInt64(&c.seq, 1_999_999)
	c.sendPing()
	deadline := time.Now().Add(2 * time.Second)
	for c.pinging.Load() {
		if time.Now().After(deadline) {
			t.Fatalf("the reply to liveness ping %d was not matched", c.pingID.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestResponseKey(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		key   string
		hasID bool
	}{
		{"", "", false},
		{"null", "", false},
		{" null ", "", false},
		{"1", "1", true},
		{"1000000", "1000000", true},
		{"1e6", "1000000", true},
		{"1e+06", "1000000", true},
		{"1000000.0", "1000000", true},
		{"9007199254740993", "9007199254740993", true},
		{"1.5", "1.5", true},
		{`"1000000"`, `"1000000"`, true},
	} {
		key, hasID := responseKey(json.RawMessage(tc.raw))
		if key != tc.key || hasID != tc.hasID {
			t.Errorf("responseKey(%q) = %q, %v; want %q, %v", tc.raw, key, hasID, tc.key, tc.hasID)
		}
	}
	for _, id := range []any{float64(1_000_000), int64(1_000_000), 1_000_000, json.Number("1000000")} {
		if got := idKey(id); got != "1000000" {
			t.Errorf("idKey(%T %v) = %q, want 1000000", id, id, got)
		}
	}
}

// The server's cancel registry keys a decoded float64 ID in decimal, so a
// $/cancelRequest for request 1,000,000 cancels it.
func TestCancelRegistryPastOneMillion(t *testing.T) {
	r := NewCancelRegistry()
	var cancelled atomic.Bool
	var req, params any
	if err := json.Unmarshal([]byte("1000000"), &req); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte("1e6"), &params); err != nil {
		t.Fatal(err)
	}
	r.Register(req, func() { cancelled.Store(true) })
	if !r.Cancel(params) || !cancelled.Load() {
		t.Fatal("cancel by ID 1,000,000 did not find the request")
	}
}
