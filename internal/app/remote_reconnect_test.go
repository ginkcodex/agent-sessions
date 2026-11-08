package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/remote"
	"github.com/ginkcode/agent-sessions/internal/rpc"
)

func TestConnection_SilentMasterCleanup(t *testing.T) {
	prev := reconnectBackoff
	reconnectBackoff = []time.Duration{time.Millisecond}
	defer func() { reconnectBackoff = prev }()
	for _, mode := range []string{"silent", "busy", "plain", "stop-fails", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			states := make(chan ConnectionState, 16)
			c := newConnection(engine.EmitterFunc(func(_ string, p any) {
				if st, ok := p.(ConnectionState); ok {
					states <- st
				}
			}))
			defer c.Shutdown()
			c.opts.ControlMaster = "auto"
			c.setBusy(func(string) bool { return mode == "busy" })
			stops := make(chan struct{}, 4)
			c.stopMaster = func(ctx context.Context, _ string, _ remote.SSHOptions) error {
				stops <- struct{}{}
				if mode == "disconnect" {
					c.mu.Lock()
					c.gen++
					if c.cancel != nil {
						c.cancel()
					}
					c.mu.Unlock()
				}
				return errors.New("stop failed")
			}
			r, w := io.Pipe()
			defer w.Close()
			cl := rpc.NewClient(r, io.Discard)
			sess := remote.NewTestSession("box", cl, nil)
			redial := make(chan struct{}, 1)
			first := true
			c.dial = func(ctx context.Context, _ string, _ remote.SSHOptions, _ map[string]string, _ engine.Emitter) (*remote.Session, error) {
				if first {
					first = false
					return sess, nil
				}
				redial <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			}
			if err := c.Connect(t.Context(), "box"); err != nil {
				t.Fatal(err)
			}
			for st := range states {
				if st.Phase == ConnConnected {
					break
				}
			}
			if mode == "plain" {
				_ = w.Close()
			} else {
				go cl.WatchLiveness(t.Context(), 5*time.Millisecond, 30*time.Millisecond, func() {})
			}
			if mode == "disconnect" {
				select {
				case <-stops:
				case <-time.After(time.Second):
					t.Fatal("no stop")
				}
				select {
				case <-redial:
					t.Fatal("old generation redialed")
				case <-time.After(40 * time.Millisecond):
				}
				return
			}
			select {
			case <-redial:
			case <-time.After(2 * time.Second):
				t.Fatal("no retry")
			}
			wantStop := mode != "plain" && mode != "busy"
			select {
			case <-stops:
				if !wantStop {
					t.Fatal("unexpected stop")
				}
			default:
				if wantStop {
					t.Fatal("missing stop before dial")
				}
			}
			want := "connection lost"
			if mode != "plain" {
				want = remote.SilentLinkMessage()
			}
			for {
				select {
				case st := <-states:
					if st.Phase == ConnReconnecting {
						if st.Error != want {
							t.Fatalf("error=%q", st.Error)
						}
						return
					}
				case <-time.After(time.Second):
					t.Fatal("no reconnect state")
				}
			}
		})
	}
}
