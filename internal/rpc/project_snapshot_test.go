package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

// snapshotBackend is a mock host that supports the optional snapshot method.
type snapshotBackend struct {
	mockBackend
	snapshot files.ListingSnapshot
	err      error
	block    bool
	entered  chan struct{}
	finished chan error
	mu       sync.Mutex
	dirs     []string
}

var (
	_ engine.ProjectSnapshotBackend = (*snapshotBackend)(nil)
	_ engine.ProjectSnapshotBackend = (*engine.Engine)(nil)
	_ engine.ProjectSnapshotBackend = (*engine.Service)(nil)
)

func (b *snapshotBackend) ProjectSnapshot(ctx context.Context, dir string) (files.ListingSnapshot, error) {
	b.mu.Lock()
	b.dirs = append(b.dirs, dir)
	b.mu.Unlock()
	if b.block {
		close(b.entered)
		<-ctx.Done()
		b.finished <- ctx.Err()
		return files.ListingSnapshot{}, ctx.Err()
	}
	if b.err != nil {
		return files.ListingSnapshot{}, b.err
	}
	// Each call returns a fresh copy, as a retained snapshot must not be shared.
	out := b.snapshot
	if out.Paths != nil {
		listing := *out.Paths
		listing.Files = append([]string(nil), listing.Files...)
		listing.Directories = append([]string(nil), listing.Directories...)
		out.Paths = &listing
	}
	return out, nil
}

func initSnapshotPair(t *testing.T, backend engine.Backend) (*Client, func()) {
	t.Helper()
	s, c, cleanup := setupPipePair(backend)
	s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return backend, Capabilities{}, paths.Roots{}, nil
	})
	if _, err := c.Initialize(t.Context(), InitializeRequest{}); err != nil {
		cleanup()
		t.Fatal(err)
	}
	return c, cleanup
}

func testSnapshot() files.ListingSnapshot {
	return files.ListingSnapshot{
		Paths:      &files.Listing{Root: "/repo", Files: []string{"a.go", "dir/b.go"}, Directories: []string{"dir"}, Git: true},
		Generation: "g-7",
		Unverified: true,
	}
}

// RPC returns exactly what the in-process backend returns, and copies.
func TestProjectSnapshotParityAndImmutable(t *testing.T) {
	backend := &snapshotBackend{snapshot: testSnapshot()}
	c, cleanup := initSnapshotPair(t, backend)
	defer cleanup()
	if !c.Capabilities().ProjectSnapshot {
		t.Fatal("supporting host did not advertise projectSnapshot")
	}

	ctx := t.Context()
	remote, err := c.ProjectSnapshot(ctx, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	local, err := backend.ProjectSnapshot(ctx, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal(remote)
	l, _ := json.Marshal(local)
	if string(r) != string(l) || !remote.Unverified || remote.Generation != "g-7" || remote.Paths == nil || remote.Paths.Status != nil {
		t.Fatalf("remote %s, local %s", r, l)
	}
	if backend.dirs[0] != "/repo" {
		t.Fatalf("dir = %q", backend.dirs[0])
	}

	// Mutating one envelope does not leak into the next.
	remote.Paths.Files[0] = "changed"
	remote.Paths.Files = append(remote.Paths.Files, "extra")
	remote.Generation = "mutated"
	again, err := c.ProjectSnapshot(ctx, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(again)
	if string(a) != string(l) {
		t.Fatalf("second envelope %s, want %s", a, l)
	}
}

// An empty envelope means no snapshot, and stays distinct on the wire.
func TestProjectSnapshotEmptyEnvelopeAndJSON(t *testing.T) {
	c, cleanup := initSnapshotPair(t, &snapshotBackend{})
	defer cleanup()
	got, err := c.ProjectSnapshot(t.Context(), "/repo")
	if err != nil || got.Paths != nil || got.Unverified || got.Generation != "" {
		t.Fatalf("empty = %+v, %v", got, err)
	}
	b, err := json.Marshal(files.ListingSnapshot{})
	if err != nil || string(b) != `{"unverified":false}` {
		t.Fatalf("zero envelope JSON %s, %v", b, err)
	}
	b, _ = json.Marshal(Capabilities{})
	if strings.Contains(string(b), "projectSnapshot") {
		t.Fatalf("absent capability must be omitted: %s", b)
	}
}

// Old hosts: no capability means no request; a missing method is a zero envelope.
func TestProjectSnapshotOldHostFallback(t *testing.T) {
	for _, mode := range []string{"no_capability", "method_not_found"} {
		t.Run(mode, func(t *testing.T) {
			c, cleanup := initSnapshotPair(t, &mockBackend{})
			defer cleanup()
			if c.Capabilities().ProjectSnapshot {
				t.Fatal("old host advertised projectSnapshot")
			}
			if mode == "method_not_found" {
				c.mu.Lock()
				c.caps.ProjectSnapshot = true
				c.mu.Unlock()
			}
			got, err := c.ProjectSnapshot(t.Context(), "/repo")
			if err != nil || got.Paths != nil || got.Unverified || got.Generation != "" {
				t.Fatalf("fallback = %+v, %v", got, err)
			}
			if listing, err := c.ListFiles(t.Context(), "/repo"); err != nil || len(listing.Files) != 1 {
				t.Fatalf("authoritative listing: %+v, %v", listing, err)
			}
		})
	}
	// No transport is touched without the capability.
	if got, err := (&Client{}).ProjectSnapshot(t.Context(), "/repo"); err != nil || got.Paths != nil {
		t.Fatalf("no capability = %+v, %v", got, err)
	}
}

// A rejected root stays an error; it is not mistaken for "no snapshot".
func TestProjectSnapshotRejectedRoot(t *testing.T) {
	c, cleanup := initSnapshotPair(t, &snapshotBackend{err: engine.ErrUnknownDirectory})
	defer cleanup()
	got, err := c.ProjectSnapshot(t.Context(), "/unauthorized")
	if err == nil || !strings.Contains(err.Error(), engine.ErrUnknownDirectory.Error()) || got.Paths != nil {
		t.Fatalf("rejected root = %+v, %v", got, err)
	}
}

func TestProjectSnapshotCancellation(t *testing.T) {
	backend := &snapshotBackend{block: true, entered: make(chan struct{}), finished: make(chan error, 1)}
	c, cleanup := initSnapshotPair(t, backend)
	defer cleanup()
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, err := c.ProjectSnapshot(ctx, "/repo")
		errCh <- err
	}()
	waitFor(t, backend.entered, "snapshot call")
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled call = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not return after cancel")
	}
	select {
	case err := <-backend.finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("server context = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server handler was not cancelled")
	}
}

func TestProjectSnapshotDisconnect(t *testing.T) {
	backend := &snapshotBackend{block: true, entered: make(chan struct{}), finished: make(chan error, 1)}
	c2sReader, c2sWriter := io.Pipe()
	s2cReader, s2cWriter := io.Pipe()
	server := NewServer(backend, c2sReader, s2cWriter)
	server.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return backend, Capabilities{}, paths.Roots{}, nil
	})
	client := NewClient(s2cReader, c2sWriter)
	serveCtx, stop := context.WithCancel(t.Context())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = server.Serve(serveCtx)
	}()
	defer func() {
		stop()
		_ = client.Close()
		_ = server.Close()
		_ = c2sWriter.Close()
		_ = s2cWriter.Close()
		<-served
	}()
	if _, err := client.Initialize(t.Context(), InitializeRequest{}); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := client.ProjectSnapshot(t.Context(), "/repo")
		errCh <- err
	}()
	waitFor(t, backend.entered, "snapshot call")
	// The transport drops in both directions.
	_ = s2cWriter.CloseWithError(io.ErrClosedPipe)
	_ = c2sReader.CloseWithError(io.ErrClosedPipe)
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("disconnected call = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not return after disconnect")
	}
	if _, err := client.ProjectSnapshot(t.Context(), "/repo"); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("call after disconnect = %v", err)
	}
	select {
	case <-backend.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler was not cancelled after disconnect")
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
