package app

import (
	"errors"
	"os"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

func TestWorkspaceApp(t *testing.T) {
	a, _ := newSessionApp(t)
	withPanel(t, a)
	root := t.TempDir()
	dir, err := a.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if dirs := a.ListWorkspaces(); len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("list = %v", dirs)
	}
	if result, err := a.BrowseWorkspace(dir); err != nil || result.Path != dir || result.Directories == nil {
		t.Fatalf("browse = %+v %v", result, err)
	}
	if _, err := a.PanelNewSession(model.AgentClaude, dir, TerminalSize{Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	if len(a.ListWorkspaces()) != 0 {
		t.Fatal("workspace not removed")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("workspace deleted", err)
	}
	if _, err := a.PanelNewSession(model.AgentClaude, dir, TerminalSize{Cols: 80, Rows: 24}); !errors.Is(err, engine.ErrUnknownDirectory) {
		t.Fatalf("unregistered launch = %v", err)
	}
}
