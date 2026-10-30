package rpc

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

type languagesRPCBackend struct {
	*diagnosticsBackend
	disabled [][]string
}

func (b *languagesRPCBackend) AttachCodeDiagnosticsWithout(ctx context.Context, dir string, v int, disabled []string) (engine.CodeDiagnosticsAttachment, error) {
	b.disabled = append(b.disabled, disabled)
	return b.AttachCodeDiagnostics(ctx, dir, v)
}

func serveDiagnostics(t *testing.T, backend engine.Backend) *Client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s, c := NewServer(backend, inR, outW), NewClient(outR, inW)
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
	return c
}

func TestDiagnosticsRPCDisabledLanguages(t *testing.T) {
	b, _, _, cleanup := rpcDiagnosticsFixture(t)
	defer cleanup()
	ctx := context.Background()
	dir := b.root.Project().Root
	attach := func(c *Client, disabled []string) error {
		a, err := c.AttachCodeDiagnosticsWithout(ctx, dir, 1, disabled)
		if err == nil {
			_, err = c.DetachCodeDiagnostics(ctx, a.Context)
		}
		return err
	}

	// A server whose backend cannot apply the field does not advertise it, and
	// the client does not send it.
	old := serveDiagnostics(t, b)
	init, err := old.Initialize(ctx, InitializeRequest{})
	if err != nil || !init.Capabilities.CodeDiagnostics || init.Capabilities.CodeDiagnosticsLanguages {
		t.Fatal(init.Capabilities, err)
	}
	if err := attach(old, []string{"python"}); err != nil {
		t.Fatal(err)
	}

	lb := &languagesRPCBackend{diagnosticsBackend: b}
	c := serveDiagnostics(t, lb)
	init, err = c.Initialize(ctx, InitializeRequest{})
	if err != nil || !init.Capabilities.CodeDiagnosticsLanguages {
		t.Fatal(init.Capabilities, err)
	}
	// Absent means every language: the plain attach.
	if err := attach(c, nil); err != nil || len(lb.disabled) != 0 {
		t.Fatal(err, lb.disabled)
	}
	// Unknown names pass through; discovery ignores them.
	if err := attach(c, []string{"python", "cobol"}); err != nil || len(lb.disabled) != 1 || !slices.Equal(lb.disabled[0], []string{"python", "cobol"}) {
		t.Fatal(err, lb.disabled)
	}
	for _, bad := range [][]string{slices.Repeat([]string{"go"}, 17), {strings.Repeat("x", 33)}, {""}} {
		err := attach(c, bad)
		var failure *cd.Error
		if !errors.As(err, &failure) || failure.Code != "invalidArgument" {
			t.Fatalf("%d names: %v", len(bad), err)
		}
	}
	if len(lb.disabled) != 1 {
		t.Fatal("an invalid set reached the backend", lb.disabled)
	}
	sixteen := slices.Repeat([]string{strings.Repeat("x", 32)}, 16)
	if err := attach(c, sixteen); err != nil {
		t.Fatal("the largest valid set was refused", err)
	}
}
