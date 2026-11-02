package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"

	"github.com/ginkcode/agent-sessions/internal/model"
)

func workspaceService(t *testing.T) *Service {
	t.Helper()
	s := NewService(nil, nil)
	s.SetDataDir(t.TempDir())
	return s
}

func TestWorkspaceRegistrationAuthorizationAndPersistence(t *testing.T) {
	s := workspaceService(t)
	dir, err := canonicalWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.knownDirectory(dir) {
		t.Fatal("unregistered directory authorized")
	}
	if _, err := s.OpenWorkspace("relative"); err == nil {
		t.Fatal("relative path accepted")
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{file, filepath.Join(dir, "missing")} {
		if _, err := s.OpenWorkspace(invalid); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	got, err := s.OpenWorkspace(filepath.Join(dir, "."))
	if err != nil || got != dir {
		t.Fatalf("open = %q %v", got, err)
	}
	if _, err := s.OpenWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	next := NewService(nil, nil)
	next.SetDataDir(s.DataDir())
	if dirs, err := next.listWorkspaces(); err != nil || !reflect.DeepEqual(dirs, []string{dir}) {
		t.Fatalf("persisted = %v %v", dirs, err)
	}
	if !next.knownDirectory(dir) || next.knownDirectory(filepath.Dir(dir)) || next.knownDirectory(filepath.Join(dir, "child")) {
		t.Fatal("root-only authorization incorrect")
	}
	eng := NewEngine(WithService(next), WithCacheEnabled(false))
	if _, err := eng.ReadFile(context.Background(), dir, "file.txt"); err != nil {
		t.Fatal(err)
	}
	if resolved, err := eng.ResolveTerminalFile(context.Background(), dir, "file.txt"); err != nil || resolved.Dir != dir || resolved.Path != "file.txt" {
		t.Fatalf("registered terminal path = %+v %v", resolved, err)
	}
	groups, err := next.ListGroups(GroupModeDirAgent, FilterOpts{})
	if err != nil || len(groups) != 1 || groups[0].Key != "dir:"+workspaceKey(dir) {
		t.Fatalf("groups = %+v %v", groups, err)
	}
	sessions, err := next.ListSessions(groups[0].Key, FilterOpts{}, SortOpts{})
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty workspace sessions = %v %v", sessions, err)
	}
	if _, err := next.ListSessions("dir:/not-registered", FilterOpts{}, SortOpts{}); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("unknown group = %v", err)
	}
	if err := next.RemoveWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	if next.knownDirectory(dir) {
		t.Fatal("removed directory authorized")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("unregister deleted contents", err)
	}
}

func TestWorkspaceNewSessionLaunch(t *testing.T) {
	s, _ := setupTestService(t)
	s.SetDataDir(t.TempDir())
	dir, err := s.OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cmd, err := s.NewSessionLaunch(model.AgentClaude, dir); err != nil || cmd.Dir != dir {
		t.Fatalf("launch = %+v %v", cmd, err)
	}
	if err := s.RemoveWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewSessionLaunch(model.AgentClaude, dir); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatal(err)
	}
}

func TestWorkspaceBrowseAndSymlinks(t *testing.T) {
	s := workspaceService(t)
	root, err := canonicalWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"visible", ".hidden", ".git"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "visible"), link); err == nil {
		got, err := s.OpenWorkspace(link)
		if err != nil || got != filepath.Join(root, "visible") {
			t.Fatalf("canonical symlink = %s %v", got, err)
		}
	}
	got, err := browseWorkspace(context.Background(), root)
	want := []string{filepath.Join(root, ".hidden"), filepath.Join(root, "visible")}
	if err != nil || got.Path != root || got.Parent != filepath.Dir(root) || !reflect.DeepEqual(got.Directories, want) {
		t.Fatalf("browse = %+v %v", got, err)
	}
	if s.knownDirectory(root) {
		t.Fatal("browsing authorized root")
	}
	empty, err := browseWorkspace(context.Background(), filepath.Join(root, "visible"))
	if err != nil || empty.Directories == nil || len(empty.Directories) != 0 {
		t.Fatalf("empty = %+v %v", empty, err)
	}
	if _, err := browseWorkspace(context.Background(), "relative"); err == nil {
		t.Fatal("relative browse accepted")
	}
	if err := s.RemoveWorkspace(filepath.Join(root, "visible")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveWorkspace(root); err != nil {
		t.Fatal("cannot remove stale root", err)
	}
}

func TestWorkspaceBrowseBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 1010; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("dir%04d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	got, err := browseWorkspace(context.Background(), root)
	if err != nil || len(got.Directories) != 1000 {
		t.Fatalf("bounded browse = %d %v", len(got.Directories), err)
	}
}

func TestWorkspaceConcurrentWritersAndSessionCompatibility(t *testing.T) {
	s := workspaceService(t)
	other := NewService(nil, nil)
	other.SetDataDir(s.DataDir())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		dir := t.TempDir()
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			writer := s
			if i%2 == 0 {
				writer = other
			}
			if _, err := writer.OpenWorkspace(dir); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	dirs, err := s.listWorkspaces()
	if err != nil || len(dirs) != 8 {
		t.Fatalf("concurrent roots = %v %v", dirs, err)
	}
	dir := dirs[0]
	s.catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "workspace-session"}, CWD: dir}}})
	groups, err := s.ListGroups(GroupModeDirAgent, FilterOpts{})
	if err != nil || len(groups) != 8 {
		t.Fatalf("deduplicated groups = %d %v", len(groups), err)
	}
	if err := s.RemoveWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	if !s.knownDirectory(dir) {
		t.Fatal("removing registration revoked session root")
	}
	sessions, err := s.ListSessions("dir:"+workspaceKey(dir), FilterOpts{}, SortOpts{})
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions after unregister = %v %v", sessions, err)
	}
}

func TestWorkspaceUsesEngineDataRoot(t *testing.T) {
	data := t.TempDir()
	eng := NewEngine(WithService(NewService(nil, nil)), WithRoots(paths.Roots{Data: data}), WithCacheEnabled(false))
	if _, err := eng.OpenWorkspace(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "workspaces.json")); err != nil {
		t.Fatal(err)
	}
}
