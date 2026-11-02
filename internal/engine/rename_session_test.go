package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func TestRenameSessionPersistsAndRefreshes(t *testing.T) {
	root := t.TempDir()
	id := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(root, "projects", "fixture", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"user\",\"sessionId\":\""+id+"\",\"message\":{\"content\":\"synthetic prompt\"}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(WithRoots(paths.Roots{Claude: root, Config: t.TempDir()}), WithCacheEnabled(false))
	defer e.Close()
	if err := e.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	ref := model.SessionRef{Agent: model.AgentClaude, ID: id}
	if err := e.RenameSession(t.Context(), ref, "native title"); err != nil {
		t.Fatal(err)
	}
	meta, ok := e.svc.Catalog().Get(ref)
	if !ok || meta.Title != "native title" {
		t.Fatalf("meta=%+v ok=%v", meta, ok)
	}
	if err := e.RenameSession(t.Context(), model.SessionRef{Agent: model.AgentClaude, ID: "unknown"}, "title"); err == nil {
		t.Fatal("accepted unknown")
	}
}
