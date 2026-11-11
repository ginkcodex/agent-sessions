package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/codex"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

func TestGuardianScanTreeAndCounts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sessions", "2026", "11", "11")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const parent = "11111111-1111-1111-1111-111111111111"
	const guardian = "22222222-2222-2222-2222-222222222222"
	for _, id := range []string{parent, guardian} {
		extra := ""
		if id == guardian {
			extra = `,"parent_thread_id":"` + parent + `","source":{"subagent":{"other":"guardian"}},"thread_source":"guardian_review"`
		}
		data := fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"cwd\":%q%s}}\n", id, root, extra)
		if err := os.WriteFile(filepath.Join(dir, "rollout-2026-11-11T12-00-00-"+id+".jsonl"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY); CREATE TABLE thread_spawn_edges (parent_thread_id TEXT, child_thread_id TEXT);`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := codex.New(root, nil).Scan(context.Background(), provider.ScanState{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := scan.NewCatalog()
	catalog.Apply(res)
	svc := NewService(catalog, nil)
	svc.SetDataDir(t.TempDir())
	for _, tc := range []struct {
		filter FilterOpts
		want   int
	}{
		{FilterOpts{}, 2}, {FilterOpts{HideSubagents: true}, 1}, {FilterOpts{HasSubagents: true}, 1}, {FilterOpts{HideSubagents: true, HasSubagents: true}, 1},
	} {
		sessions, err := svc.ListSessions("", tc.filter, SortOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) != tc.want {
			t.Fatalf("filter %+v sessions=%+v", tc.filter, sessions)
		}
		counts, err := svc.AgentCounts(tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if counts["codex"] != tc.want {
			t.Fatalf("filter %+v counts=%v", tc.filter, counts)
		}
		// Directory and agent groups count main sessions, not nested reviews.
		for _, mode := range []GroupMode{GroupModeDirAgent, GroupModeAgentDir} {
			groups, err := svc.ListGroups(mode, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || groups[0].SessionCount != 1 {
				t.Fatalf("mode=%s filter=%+v groups=%+v", mode, tc.filter, groups)
			}
		}
		groups, err := svc.ListGroups(GroupModeFlat, tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].SessionCount != 1 || len(groups[0].Sessions) != tc.want {
			t.Fatalf("filter %+v groups=%+v", tc.filter, groups)
		}
		if !tc.filter.HideSubagents && !tc.filter.HasSubagents {
			if len(groups[0].Children) != 1 || groups[0].Children[0].SessionCount != 0 {
				t.Fatalf("Guardian not nested: %+v", groups[0])
			}
			for _, m := range sessions {
				if m.Ref.ID == guardian && m.ParentID != parent {
					t.Fatalf("Guardian parent missing: %+v", m)
				}
			}
		}
	}
}
