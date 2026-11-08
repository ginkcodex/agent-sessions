//go:build !windows

package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

func TestAskpass_RoundTrip(t *testing.T) {
	var mu sync.Mutex
	var receivedID, receivedPrompt string
	promptArrived := make(chan struct{})

	broker, err := NewAskpassBroker(func(id, prompt string) {
		mu.Lock()
		receivedID = id
		receivedPrompt = prompt
		mu.Unlock()
		close(promptArrived)
	})
	if err != nil {
		t.Fatalf("NewAskpassBroker failed: %v", err)
	}
	defer broker.Close()

	// Verify permissions
	info, err := os.Stat(broker.dir)
	if err != nil {
		t.Fatalf("stat socket dir failed: %v", err)
	}
	if platform.ModeBits && info.Mode().Perm() != 0o700 {
		t.Errorf("socket dir perm = %o, want 0700", info.Mode().Perm())
	}

	testPrompt := "Enter passphrase for key '/id_ed25519':"
	testAnswer := "super-secret-passphrase-1234"

	var stdout, stderr bytes.Buffer
	helperDone := make(chan error, 1)

	go func() {
		err := RunAskpassHelper(
			context.Background(),
			broker.SocketPath(),
			broker.Token(),
			testPrompt,
			&stdout,
			&stderr,
		)
		helperDone <- err
	}()

	// Wait for prompt to arrive at broker handler
	select {
	case <-promptArrived:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt to arrive")
	}

	mu.Lock()
	if receivedPrompt != testPrompt {
		t.Errorf("got prompt %q, want %q", receivedPrompt, testPrompt)
	}
	idToReply := receivedID
	mu.Unlock()

	// Reply with answer
	if !broker.Reply(idToReply, testAnswer) {
		t.Fatalf("failed to reply to prompt %q", idToReply)
	}

	// Helper should complete
	select {
	case err := <-helperDone:
		if err != nil {
			t.Fatalf("RunAskpassHelper failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RunAskpassHelper to complete")
	}

	output := strings.TrimSpace(stdout.String())
	if output != testAnswer {
		t.Errorf("got stdout %q, want %q", output, testAnswer)
	}
}

func TestAskpass_ProbeBudgetPauses(t *testing.T) {
	for _, action := range []string{"reply", "cancel", "close"} {
		t.Run(action, func(t *testing.T) {
			arrived := make(chan string, 1)
			broker, err := NewAskpassBroker(func(id, _ string) { arrived <- id })
			if err != nil {
				t.Fatal(err)
			}
			defer broker.Close()
			// Start already open: subscribing must snapshot existing prompts.
			helperDone := make(chan error, 1)
			go func() {
				var out, diagnostic bytes.Buffer
				helperDone <- RunAskpassHelper(context.Background(), broker.SocketPath(), broker.Token(), "Password:", &out, &diagnostic)
			}()
			var id string
			select {
			case id = <-arrived:
			case <-time.After(2 * time.Second):
				t.Fatal("no prompt")
			}
			ctx, cancel := probeBudget(context.Background(), 150*time.Millisecond, broker)
			defer cancel()
			select {
			case <-ctx.Done():
				t.Fatal("open prompt consumed probe budget")
			case <-time.After(250 * time.Millisecond):
			}
			switch action {
			case "reply":
				broker.Reply(id, "answer")
			case "cancel":
				broker.Cancel(id)
			case "close":
				broker.Close()
			}
			select {
			case <-ctx.Done():
				t.Fatal("resume did not preserve remaining time")
			case <-time.After(50 * time.Millisecond):
			}
			select {
			case <-ctx.Done():
				if context.Cause(ctx) != context.DeadlineExceeded {
					t.Fatalf("cause = %v", context.Cause(ctx))
				}
			case <-time.After(time.Second):
				t.Fatal("closed prompt did not resume probe budget")
			}
			select {
			case <-helperDone:
			case <-time.After(time.Second):
				t.Fatal("helper did not finish")
			}
		})
	}
}

func TestAskpass_ProbeBudgetRemainingAndDisconnectedHelper(t *testing.T) {
	arrived := make(chan string, 2)
	broker, err := NewAskpassBroker(func(id, _ string) { arrived <- id })
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	ctx, cancel := probeBudget(context.Background(), 800*time.Millisecond, broker)
	defer cancel()
	time.Sleep(500 * time.Millisecond) // consume most of the active budget
	var conns []net.Conn
	for range 2 {
		conn, err := net.Dial("unix", broker.SocketPath())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conns = append(conns, conn)
		if err := json.NewEncoder(conn).Encode(AskpassRequest{Token: broker.Token(), Prompt: "Password:"}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-arrived:
		case <-time.After(time.Second):
			t.Fatal("no prompt")
		}
	}
	conns[0].Close()
	select {
	case <-ctx.Done():
		t.Fatal("one closed helper resumed budget while another prompt was open")
	case <-time.After(900 * time.Millisecond):
	}
	conns[1].Close()
	select {
	case <-ctx.Done():
	case <-time.After(650 * time.Millisecond):
		t.Fatal("helper disconnect did not resume remaining budget (or reset it)")
	}
}

func TestAskpass_InvalidToken(t *testing.T) {
	broker, err := NewAskpassBroker(nil)
	if err != nil {
		t.Fatalf("NewAskpassBroker failed: %v", err)
	}
	defer broker.Close()

	var stdout, stderr bytes.Buffer
	err = RunAskpassHelper(
		context.Background(),
		broker.SocketPath(),
		"wrong-token",
		"Enter password:",
		&stdout,
		&stderr,
	)
	if err == nil {
		t.Fatal("expected error with invalid token, got nil")
	}
	if !strings.Contains(err.Error(), "invalid askpass token") {
		t.Errorf("expected 'invalid askpass token', got: %v", err)
	}
}

func TestAskpass_Cancelled(t *testing.T) {
	promptArrived := make(chan string, 1)
	broker, err := NewAskpassBroker(func(id, prompt string) {
		promptArrived <- id
	})
	if err != nil {
		t.Fatalf("NewAskpassBroker failed: %v", err)
	}
	defer broker.Close()

	var stdout, stderr bytes.Buffer
	helperDone := make(chan error, 1)

	go func() {
		err := RunAskpassHelper(
			context.Background(),
			broker.SocketPath(),
			broker.Token(),
			"Host key confirmation:",
			&stdout,
			&stderr,
		)
		helperDone <- err
	}()

	var id string
	select {
	case id = <-promptArrived:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt")
	}

	// Cancel the prompt
	broker.Cancel(id)

	select {
	case err := <-helperDone:
		if err == nil {
			t.Fatal("expected error on cancelled prompt, got nil")
		}
		if !strings.Contains(err.Error(), "prompt cancelled") {
			t.Errorf("expected 'prompt cancelled', got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for helper after cancel")
	}
}
