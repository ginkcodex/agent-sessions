package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func watchTestClient(t *testing.T, c *Client) {
	t.Helper()
	go c.WatchLiveness(t.Context(), 30*time.Millisecond, 250*time.Millisecond, func() {})
	t.Cleanup(func() { _ = c.Close() })
}

func TestLivenessSilentBlockedWrite(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	blocked, input := io.Pipe()
	defer blocked.Close()
	c := NewClient(r, input)
	watchTestClient(t, c)
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("blocked ping stopped watchdog")
	}
	if !errors.Is(c.Err(), ErrLinkSilent) || !errors.Is(c.Err(), ErrDisconnected) {
		t.Fatalf("Err = %v", c.Err())
	}
	if _, err := c.Ping(t.Context(), "after drop"); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Ping = %v", err)
	}
}

func TestLivenessOneUnansweredPing(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	inputR, inputW := io.Pipe()
	defer inputR.Close()
	c := NewClient(r, inputW)
	var count atomic.Int32
	go func() {
		fr := NewFrameReader(inputR)
		for {
			_, err := fr.ReadFrame()
			if err != nil {
				return
			}
			count.Add(1)
		}
	}()
	watchTestClient(t, c)
	<-c.Done()
	if got := count.Load(); got != 1 {
		t.Fatalf("pings=%d, want one outstanding", got)
	}
}

func TestLivenessOldServerErrorReply(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	inputR, inputW := io.Pipe()
	defer inputR.Close()
	c := NewClient(r, inputW)
	var count atomic.Int32
	go func() {
		fr := NewFrameReader(inputR)
		fw := NewFrameWriter(w)
		for {
			frame, err := fr.ReadFrame()
			if err != nil {
				return
			}
			var req Request
			_ = json.Unmarshal(frame, &req)
			count.Add(1)
			_ = fw.WriteFrame(Response{JSONRPC: JSONRPCVersion, ID: req.ID, Error: &ResponseError{Code: CodeMethodNotFound, Message: "old server"}})
		}
	}()
	watchTestClient(t, c)
	select {
	case <-c.Done():
		t.Fatalf("old server dropped: %v", c.Err())
	case <-time.After(700 * time.Millisecond):
	}
	if count.Load() < 2 {
		t.Fatal("did not probe repeatedly after replies")
	}
}

func TestLivenessSlowLargeFrame(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	c := NewClient(r, io.Discard)
	watchTestClient(t, c)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		text := `{"jsonrpc":"2.0","id":999,"result":"` + strings.Repeat("x", 400) + `"}` + "\n"
		for len(text) > 0 {
			n := min(20, len(text))
			if _, err := io.WriteString(w, text[:n]); err != nil {
				return
			}
			text = text[n:]
			time.Sleep(25 * time.Millisecond)
		}
	}()
	select {
	case <-c.Done():
		t.Fatalf("active chunks dropped: %v", c.Err())
	case <-finished:
	}
}

func TestLivenessPingDuringInitialize(t *testing.T) {
	s, c, cleanup := setupPipePair(nil)
	defer cleanup()
	entered, release := make(chan struct{}), make(chan struct{})
	s.SetPreparer(func(ctx context.Context) (PreparedBackend, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return PreparedBackend{Backend: &mockBackend{}}, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := c.call(ctx, MethodPing, nil, nil); err != nil {
		t.Fatalf("before initialize: %v", err)
	}
	result := make(chan error, 1)
	go func() { _, err := c.Initialize(ctx, InitializeRequest{}); result <- err }()
	<-entered
	err := c.call(ctx, MethodPing, nil, nil)
	close(release)
	if err != nil {
		t.Fatalf("during initialize: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestLivenessBlockedNotificationDoesNotBlockReader(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	c := NewClient(r, io.Discard)
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	c.OnNotification(func(string, json.RawMessage) { close(entered); <-release })
	watchTestClient(t, c)
	if err := NewFrameWriter(w).WriteFrame(Notification{JSONRPC: JSONRPCVersion, Method: "test"}); err != nil {
		t.Fatal(err)
	}
	<-entered
	for range 12 {
		if err := NewFrameWriter(w).WriteFrame(Response{JSONRPC: JSONRPCVersion, ID: 999, Result: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-c.Done():
			t.Fatalf("callback stalled reader: %v", c.Err())
		case <-time.After(30 * time.Millisecond):
		}
	}
}
