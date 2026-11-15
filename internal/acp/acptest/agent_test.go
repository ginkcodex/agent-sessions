package acptest

import (
	"context"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/acp"
)

func TestHoldRegistrationAndSteering(t *testing.T) {
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	a := &agent{
		conn:  acp.NewConn(r, io.Discard, acp.ConnOptions{}),
		ready: make(chan struct{}),
		turns: map[string]context.CancelFunc{},
		holds: map[string]chan struct{}{},
	}
	close(a.ready)
	t.Cleanup(func() { _ = a.conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	for i := range 1000 {
		done := make(chan any, 1)
		go func() {
			result, _ := a.prompt(acp.PromptRequest{SessionID: "hold", Prompt: []acp.ContentBlock{acp.TextBlock("hold")}})
			done <- result
		}()
		// Race the prompt's registration, not just its already-waiting state.
		for {
			a.mu.Lock()
			cancelTurn := a.turns["hold"]
			hold := a.holds["hold"]
			a.mu.Unlock()
			if cancelTurn != nil {
				if hold == nil {
					cancelTurn()
					<-done
					t.Fatalf("iteration %d: running hold has no steering channel", i)
				}
				break
			}
			if err := ctx.Err(); err != nil {
				t.Fatal(err)
			}
			runtime.Gosched()
		}
		if out := a.steer(acp.SteerRequest{SessionID: "hold", Prompt: []acp.ContentBlock{acp.TextBlock("next")}}); out.Outcome != acp.SteerInjected {
			t.Fatalf("steer = %+v", out)
		}
		select {
		case result := <-done:
			if result != (acp.PromptResponse{StopReason: acp.StopEndTurn}) {
				t.Fatalf("prompt = %+v", result)
			}
		case <-ctx.Done():
			t.Fatal("injected steer did not end the hold turn")
		}
	}
}
