package manage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/filelock"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

func repairFixture(t *testing.T) (*Manager, model.SessionMeta, []byte, string) {
	t.Helper()
	return repairFixtureIn(t, testRoots(t))
}

func repairFixtureIn(t *testing.T, roots paths.Roots) (*Manager, model.SessionMeta, []byte, string) {
	t.Helper()
	dir := filepath.Join(roots.Codex, "sessions", "2026", "11", "06")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-restored-"+uuidA+".jsonl")
	raw := []byte(`{"ordinal":0,"type":"session_meta","timestamp":"2026-08-21T13:52:35Z","payload":{"id":"` + uuidA + `","timestamp":"2026-08-21T13:52:35Z"}}` + "\n" + `{"ordinal":1,"type":"world_state","payload":{"unchanged":true}}` + "\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"state_4.sqlite", "state_5.sqlite"} {
		db, err := sql.Open("sqlite", filepath.Join(roots.Codex, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE threads (id TEXT,rollout_path TEXT,title TEXT);INSERT INTO threads VALUES (?,?,'preserve')`, uuidA, path); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entry, _ := json.Marshal(map[string]any{"id": uuidA, "rollout_path": path, "thread_name": path, "large": json.Number("9007199254740993")})
	if err := os.WriteFile(filepath.Join(roots.Codex, "session_index.jsonl"), append(entry, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ := newTestManager(t, roots)
	meta := model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentCodex, ID: uuidA}, SourcePath: path}
	return m, meta, raw, filepath.Join(dir, "rollout-2026-08-21T13-52-35-"+uuidA+".jsonl")
}
func repairDBPath(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("SELECT rollout_path FROM threads").Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestRepairCodexSessionBackupsAndNativeIndexes(t *testing.T) {
	m, meta, raw, target := repairFixture(t)
	result, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{meta})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != target {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(meta.SourcePath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists: %v", err)
	}
	saved, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(saved, raw) {
		t.Fatal("conversation bytes changed")
	}
	backup, err := os.ReadFile(filepath.Join(result.BackupPath, filepath.Base(meta.SourcePath)))
	if err != nil || !bytes.Equal(backup, raw) {
		t.Fatal("missing original backup")
	}
	for _, name := range []string{"state_4.sqlite", "state_5.sqlite"} {
		if got := repairDBPath(t, filepath.Join(m.codex.root, name)); got != target {
			t.Fatalf("index=%q", got)
		}
		if got := repairDBPath(t, filepath.Join(result.BackupPath, name)); got != meta.SourcePath {
			t.Fatalf("backup index=%q", got)
		}
	}
	index, err := os.ReadFile(filepath.Join(m.codex.root, "session_index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(index, []byte(`"rollout_path":"`+filepath.ToSlash(target)+`"`)) && !strings.Contains(string(index), "rollout-2026-08-21T13-52-35-") {
		t.Fatalf("index not repaired: %s", index)
	}
	if !bytes.Contains(index, []byte(`9007199254740993`)) {
		t.Fatal("native numeric metadata changed")
	}
	if _, err := os.Stat(filepath.Join(result.BackupPath, "session_index.jsonl")); err != nil {
		t.Fatal(err)
	}
}
func TestRepairCodexSessionGuards(t *testing.T) {
	for _, kind := range []string{"owned", "writer lock", "conflict", "metadata ID", "symlink index", "wrong index path", "running terminal"} {
		t.Run(kind, func(t *testing.T) {
			m, meta, _, target := repairFixture(t)
			switch kind {
			case "owned":
				m.owned = func(model.SessionRef) bool { return true }
			case "writer lock":
				lock := filepath.Join(m.codex.root, "thread-writer-locks", uuidA+".lock")
				if err := os.MkdirAll(filepath.Dir(lock), 0700); err != nil {
					t.Fatal(err)
				}
				held, err := filelock.TryLock(lock)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Unlock()
			case "conflict":
				if err := os.WriteFile(target, []byte("different"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata ID":
				raw, _ := os.ReadFile(meta.SourcePath)
				if err := os.WriteFile(meta.SourcePath, bytes.ReplaceAll(raw, []byte(uuidA), []byte(uuidB)), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink index":
				db := filepath.Join(m.codex.root, "state_4.sqlite")
				other := filepath.Join(t.TempDir(), "outside.sqlite")
				if err := os.Rename(db, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, db); err != nil {
					t.Skip(err)
				}
			case "wrong index path":
				db, err := sql.Open("sqlite", filepath.Join(m.codex.root, "state_4.sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE threads SET rollout_path='elsewhere'`); err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "running terminal":
				m.proc = fakeProc{argv: map[int][]string{1: {"codex", "resume", uuidA}}}
			}
			if _, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{meta}); err == nil {
				t.Fatal("unsafe repair allowed")
			}
			if _, err := os.Stat(meta.SourcePath); err != nil {
				t.Fatal("guard removed original")
			}
			matches, _ := filepath.Glob(filepath.Join(m.codex.root, "rollout-repair-backup-*"))
			if len(matches) != 0 {
				t.Fatal("guard ran mutation before validating")
			}
		})
	}
}
func TestRepairCodexSessionPartialIndexFailureCanRetry(t *testing.T) {
	m, meta, raw, target := repairFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(m.codex.root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_repair BEFORE UPDATE ON threads BEGIN SELECT RAISE(ABORT,'injected'); END;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	result, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{meta})
	if err == nil || result.BackupPath == "" {
		t.Fatal("expected partial failure with backup location")
	}
	a, _ := os.Stat(meta.SourcePath)
	b, _ := os.Stat(target)
	if a == nil || b == nil || !os.SameFile(a, b) {
		t.Fatal("partial failure removed a valid path")
	}
	if got := repairDBPath(t, filepath.Join(m.codex.root, "state_4.sqlite")); got != target {
		t.Fatal("expected first index committed")
	}
	db, err = sql.Open("sqlite", filepath.Join(m.codex.root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_repair`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{meta}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(saved, raw) {
		t.Fatal("retry altered saved history")
	}
	if _, err := os.Stat(meta.SourcePath); !os.IsNotExist(err) {
		t.Fatal("retry did not finish cleanup")
	}
}

func TestRepairCodexSessionBackupIncludesWAL(t *testing.T) {
	m, meta, _, _ := repairFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(m.codex.root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;UPDATE threads SET title='committed in WAL'`); err != nil {
		t.Fatal(err)
	}
	result, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{meta})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := sql.Open("sqlite", filepath.Join(result.BackupPath, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var title string
	if err := backup.QueryRow(`SELECT title FROM threads`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "committed in WAL" {
		t.Fatalf("backup lost WAL metadata: %q", title)
	}
}

// TestRepairCodexSessionBridgesRootSpellings covers provider roots behind a
// symlink or Windows 8.3 short name, like macOS temp dirs under the /var
// alias: the scan reports session paths under the symlink-resolved root while
// Codex's indexes store the root as configured. Repair must accept both
// spellings and report results the way the caller spells them.
func TestRepairCodexSessionBridgesRootSpellings(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	platform.Symlink(t, real, filepath.Join(base, "alias"))
	roots := testRoots(t)
	roots.Codex = filepath.Join(base, "alias", "codex")
	m, meta, raw, target := repairFixtureIn(t, roots)

	scanned, err := filepath.EvalSymlinks(meta.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if scanned == meta.SourcePath {
		t.Fatal("fixture root is not aliased")
	}
	result, err := m.RepairCodexSession(t.Context(), meta.Ref, []model.SessionMeta{{Ref: meta.Ref, SourcePath: scanned}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != want {
		t.Fatalf("result path=%q want the caller spelling %q", result.Path, want)
	}
	// Codex's indexes keep the configured spelling it writes and reads.
	for _, name := range []string{"state_4.sqlite", "state_5.sqlite"} {
		if got := repairDBPath(t, filepath.Join(m.codex.root, name)); got != target {
			t.Fatalf("index=%q want %q", got, target)
		}
	}
	saved, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(saved, raw) {
		t.Fatal("conversation bytes changed")
	}
	if _, err := os.Stat(meta.SourcePath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists: %v", err)
	}
}
