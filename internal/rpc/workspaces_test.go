package rpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func TestWorkspaceRPC(t *testing.T) {
	svc := engine.NewService(nil, nil)
	svc.SetDataDir(t.TempDir())
	eng := engine.NewEngine(engine.WithService(svc), engine.WithCacheEnabled(false))
	t.Cleanup(func() { _ = eng.Close() })
	server, client, cleanup := setupPipePair(nil)
	t.Cleanup(cleanup)
	server.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return eng, Capabilities{}, paths.Roots{}, nil
	})
	res, err := client.Initialize(t.Context(), InitializeRequest{})
	if err != nil || !res.Capabilities.Workspaces {
		t.Fatalf("initialize = %+v %v", res, err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := client.OpenWorkspace(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := client.ListWorkspaces(t.Context())
	if err != nil || len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("list = %v %v", dirs, err)
	}
	browse, err := client.BrowseWorkspace(t.Context(), dir)
	if err != nil || browse.Path != dir || len(browse.Directories) != 1 {
		t.Fatalf("browse = %+v %v", browse, err)
	}
	groups, err := client.ListGroups(t.Context(), engine.GroupModeDirAgent, engine.FilterOpts{})
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v %v", groups, err)
	}
	if sessions, err := client.ListSessions(t.Context(), groups[0].Key, engine.FilterOpts{}, engine.SortOpts{}); err != nil || len(sessions) != 0 {
		t.Fatalf("sessions = %v %v", sessions, err)
	}
	if _, err := client.OpenWorkspace(t.Context(), "relative"); err == nil {
		t.Fatal("accepted relative path")
	}
	if err := client.RemoveWorkspace(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	if dirs, err := client.ListWorkspaces(t.Context()); err != nil || len(dirs) != 0 {
		t.Fatalf("removed = %v %v", dirs, err)
	}
}

func TestWorkspaceOldRemoteUnsupported(t *testing.T) {
	server, client, cleanup := setupPipePair(nil)
	t.Cleanup(cleanup)
	_ = server
	if _, err := client.Initialize(t.Context(), InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OpenWorkspace(t.Context(), t.TempDir()); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("old host = %v", err)
	}
}
