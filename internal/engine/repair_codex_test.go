package engine

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/manage"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

type repairTestProc struct{}

func (repairTestProc) Cmdlines() (map[int][]string, error) { return map[int][]string{}, nil }
func TestRepairCodexRefreshesCatalogPath(t *testing.T) {
	roots := paths.Roots{Codex: t.TempDir(), Config: t.TempDir()}
	const id = "11111111-1111-4111-8111-111111111111"
	dir := filepath.Join(roots.Codex, "sessions", "2026", "11", "06")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "rollout-restored-"+id+".jsonl")
	data, _ := json.Marshal(map[string]any{"ordinal": 0, "type": "session_meta", "timestamp": "2026-08-21T13:52:35Z", "payload": map[string]string{"id": id, "cwd": roots.Codex}})
	if err := os.WriteFile(old, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(roots.Codex, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE threads(id TEXT,rollout_path TEXT);INSERT INTO threads VALUES (?,?)`, id, old); err != nil {
		t.Fatal(err)
	}
	db.Close()
	e := NewEngine(WithRoots(roots), WithCacheEnabled(false))
	defer e.Close()
	manager, err := manage.New(roots, filepath.Join(roots.Config, "config.toml"), manage.WithProcFS(repairTestProc{}))
	if err != nil {
		t.Fatal(err)
	}
	e.manageOverride = manager
	if err := e.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	ref := model.SessionRef{Agent: model.AgentCodex, ID: id}
	result, err := e.RepairCodexSession(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := e.GetSessionMeta(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SourcePath != result.Path || meta.SourcePath == old {
		t.Fatalf("catalog path=%q result=%+v", meta.SourcePath, result)
	}
	if _, err := e.RepairCodexSession(t.Context(), ref); err == nil {
		t.Fatal("canonical sessions must not be repaired twice")
	}
}
