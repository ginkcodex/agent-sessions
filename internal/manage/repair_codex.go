package manage

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ginkcode/agent-sessions/internal/filelock"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
)

// CodexRepairResult identifies the repaired file and its private backup folder.
type CodexRepairResult struct {
	Path       string `json:"path"`
	BackupPath string `json:"backupPath"`
}

var codexStateName = regexp.MustCompile(`^state_[0-9]+\.sqlite$`)

// RepairCodexSession repairs only the legacy filename emitted by our importer.
// Paths come from the host catalog, never from the RPC caller.
func (m *Manager) RepairCodexSession(ctx context.Context, ref model.SessionRef, catalog []model.SessionMeta) (result CodexRepairResult, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if ref.Agent != model.AgentCodex || !uuidPattern.MatchString(ref.ID) || m.codex == nil {
		return result, ErrUnsupportedAction
	}
	if err := m.ownedError(ref); err != nil {
		return result, errors.New("Close this session's chat before repairing it.")
	}
	var meta model.SessionMeta
	for _, candidate := range catalog {
		if candidate.Ref == ref {
			meta = candidate
			break
		}
	}
	if meta.Ref != ref {
		return result, errors.New("The session is no longer available. Refresh and try again.")
	}
	if filepath.Base(meta.SourcePath) != "rollout-restored-"+ref.ID+".jsonl" {
		return result, errors.New("This session does not need this compatibility repair.")
	}
	if meta.Live {
		return result, errors.New("Close this session in Codex before repairing it.")
	}
	live, liveErr := m.procLiveMap(ctx)
	if err := processSafetyError(live, liveErr, string(model.AgentCodex)); err != nil {
		return result, fmt.Errorf("Close Codex terminals before repairing: %w", err)
	}
	cm := m.codex
	if _, err := cm.ps.resolvedRoot(); err != nil {
		return result, err
	}
	old := filepath.Clean(meta.SourcePath)
	if err := cm.ps.ensureWithinRoot(old); err != nil {
		return result, err
	}
	info, err := os.Lstat(old)
	if err != nil || !info.Mode().IsRegular() {
		return result, errors.New("The saved session file is unavailable or unsafe.")
	}
	lockPath := filepath.Join(cm.root, ".agent-sessions-rollout-repair.lock")
	if err := cm.ps.ensureWithinRoot(lockPath); err != nil {
		return result, err
	}
	guard, err := filelock.TryLock(lockPath)
	if err != nil {
		return result, errors.New("Another session repair is running. Try again shortly.")
	}
	defer func() { _ = guard.Unlock() }()
	writer := filepath.Join(cm.root, "thread-writer-locks", ref.ID+".lock")
	if err := cm.ps.ensureWithinRoot(writer); err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(writer), 0700); err != nil {
		return result, err
	}
	writerGuard, err := filelock.TryLock(writer)
	if err != nil {
		return result, errors.New("Codex still has this session open. Close it in Codex, wait briefly, then retry Repair session.")
	}
	defer func() { _ = writerGuard.Unlock() }()
	header, err := codexRepairHeader(old, ref.ID)
	if err != nil {
		return result, err
	}
	name, err := restoredCodexRolloutName("", ref.ID, header, time.Time{})
	if err != nil {
		return result, err
	}
	target := filepath.Join(filepath.Dir(old), name)
	if err := cm.ps.ensureWithinRoot(target); err != nil {
		return result, err
	}
	if existing, statErr := os.Lstat(target); statErr == nil {
		if !os.SameFile(info, existing) {
			return result, errors.New("A different session file already occupies the repair destination.")
		}
	} else if !os.IsNotExist(statErr) {
		return result, statErr
	}
	paths, err := cm.derivePaths(ctx, meta)
	if err != nil {
		return result, err
	}
	for _, path := range paths {
		if path != old && path != target {
			return result, errors.New("More than one saved file matches this session. Repair cannot choose safely.")
		}
	}
	databases, err := cm.repairDatabases(ctx, ref.ID, old, target)
	if err != nil {
		return result, err
	}
	index := filepath.Join(cm.root, "session_index.jsonl")
	if err := codexRepairSafePath(cm.root, index); err != nil {
		return result, err
	}
	raw, err := os.ReadFile(index)
	hasIndex := err == nil
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	updated, err := codexRepairIndex(raw, old, target, ref.ID)
	if err != nil {
		return result, err
	}
	backup, err := os.MkdirTemp(cm.root, "rollout-repair-backup-")
	if err != nil {
		return result, err
	}
	result = CodexRepairResult{Path: target, BackupPath: backup}
	// After backups start, always identify recovery files in any error.
	defer func() {
		if err != nil {
			err = fmt.Errorf("Repair did not finish. Keep the saved files and retry. Backups: %s. %w", backup, err)
		}
	}()
	if err = copyCodexRepairFile(old, filepath.Join(backup, filepath.Base(old))); err != nil {
		return result, err
	}
	if hasIndex {
		if err = os.WriteFile(filepath.Join(backup, "session_index.jsonl"), raw, 0600); err != nil {
			return result, err
		}
	}
	for _, dbPath := range databases {
		if err = codexRepairSafeDB(cm.root, dbPath); err != nil {
			return result, err
		}
		db, openErr := openCodexRepairDB(ctx, dbPath, true)
		if openErr != nil {
			return result, openErr
		}
		_, err = db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(backup, filepath.Base(dbPath)))
		_ = db.Close()
		if err != nil {
			return result, err
		}
		if err = os.Chmod(filepath.Join(backup, filepath.Base(dbPath)), 0600); err != nil {
			return result, err
		}
	}
	manifest, _ := json.Marshal(map[string]string{"id": ref.ID, "old": old, "new": target})
	if err = os.WriteFile(filepath.Join(backup, "manifest.json"), manifest, 0600); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	current, err := os.Lstat(old)
	if err != nil {
		return result, err
	}
	if !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
		return result, errors.New("The session changed during repair. Close Codex and try again.")
	}
	if err = cm.ps.ensureWithinRoot(target); err != nil {
		return result, err
	}
	if existing, statErr := os.Lstat(target); statErr == nil {
		if !os.SameFile(current, existing) {
			return result, errors.New("The repair destination changed.")
		}
	} else if !os.IsNotExist(statErr) {
		return result, statErr
	} else if err = os.Link(old, target); err != nil {
		return result, err
	}
	// Both names remain until every index commits. Failure is safe to retry.
	for _, dbPath := range databases {
		if err = codexRepairSafeDB(cm.root, dbPath); err != nil {
			return result, err
		}
		db, openErr := openCodexRepairDB(ctx, dbPath, false)
		if openErr != nil {
			return result, openErr
		}
		err = func() error {
			defer func() { _ = db.Close() }()
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr != nil {
				return txErr
			}
			defer func() { _ = tx.Rollback() }()
			if err := validateCodexRepairRows(ctx, tx, ref.ID, old, target); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE threads SET rollout_path=? WHERE rollout_path=?`, target, old); err != nil {
				return err
			}
			return tx.Commit()
		}()
		if err != nil {
			return result, err
		}
	}
	if hasIndex && !bytes.Equal(raw, updated) {
		if err = codexRepairSafePath(cm.root, index); err != nil {
			return result, err
		}
		latest, readErr := os.ReadFile(index)
		if readErr != nil {
			return result, readErr
		}
		if !bytes.Equal(latest, raw) {
			return result, errors.New("Codex's session index changed. Close Codex and retry.")
		}
		if err = replaceCodexRepairIndex(index, updated); err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	a, err := os.Lstat(old)
	if err != nil {
		return result, err
	}
	b, err := os.Lstat(target)
	if err != nil {
		return result, err
	}
	if !a.Mode().IsRegular() || !b.Mode().IsRegular() || !os.SameFile(a, b) {
		return result, errors.New("The saved session paths changed during repair.")
	}
	if err = os.Remove(old); err != nil {
		return result, err
	}
	return result, nil
}

func codexRepairHeader(path, id string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				ID string `json:"id"`
			} `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			return nil, errors.New("The saved session contains an invalid record.")
		}
		if record.Type == "session_meta" {
			if record.Payload.ID != id {
				return nil, errors.New("The saved session ID does not match its filename.")
			}
			return bytes.Clone(scanner.Bytes()), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("The saved session has no session metadata.")
}

// Unlike general management paths, these exact paths deliberately include
// native SQLite indexes; still refuse symlinks at every level and special files.
func codexRepairSafePath(root, path string) error {
	if !contained(root, path) {
		return ErrPathOutsideRoot
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Repair refuses a symbolic link in the saved-session paths.")
		}
		if current == path && !info.Mode().IsRegular() {
			return errors.New("Repair requires regular saved-session files.")
		}
	}
	return nil
}
func codexRepairSafeDB(root, path string) error {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := codexRepairSafePath(root, path+suffix); err != nil {
			return err
		}
	}
	return nil
}
func openCodexRepairDB(ctx context.Context, path string, readonly bool) (*sql.DB, error) {
	mode := "rw"
	if readonly {
		mode = "ro"
	}
	values := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(2000)"}, "_txlock": {"immediate"}}
	if readonly {
		values.Set("_txlock", "deferred")
	}
	db, err := sql.Open("sqlite", pathutil.SQLiteURI(path, values))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

type codexRepairQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func validateCodexRepairRows(ctx context.Context, db codexRepairQuerier, id, old, target string) error {
	rows, err := db.QueryContext(ctx, `SELECT id,rollout_path FROM threads WHERE id=? OR rollout_path=?`, id, old)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rowID string
		var path sql.NullString
		if err := rows.Scan(&rowID, &path); err != nil {
			return err
		}
		if rowID != id || (path.Valid && path.String != "" && path.String != old && path.String != target) {
			return errors.New("The Codex index references a different saved-session path or ID. Repair stopped.")
		}
	}
	return rows.Err()
}
func (cm *codexManager) repairDatabases(ctx context.Context, id, old, target string) ([]string, error) {
	entries, err := os.ReadDir(cm.root)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if !codexStateName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(cm.root, entry.Name())
		if err := codexRepairSafeDB(cm.root, path); err != nil {
			return nil, err
		}
		db, err := openCodexRepairDB(ctx, path, true)
		if err != nil {
			return nil, err
		}
		eligible, err := func() (bool, error) {
			defer func() { _ = db.Close() }()
			rows, err := db.QueryContext(ctx, `PRAGMA table_info(threads)`)
			if err != nil {
				return false, err
			}
			columns := map[string]bool{}
			for rows.Next() {
				var n, notnull, pk int
				var name, typ string
				var def any
				if err := rows.Scan(&n, &name, &typ, &notnull, &def, &pk); err != nil {
					_ = rows.Close()
					return false, err
				}
				columns[name] = true
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return false, err
			}
			if !columns["id"] || !columns["rollout_path"] {
				return false, nil
			}
			return true, validateCodexRepairRows(ctx, db, id, old, target)
		}()
		if err != nil {
			return nil, err
		}
		if eligible {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
func codexRepairIndex(raw []byte, old, target, id string) ([]byte, error) {
	var result bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(raw))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if len(bytes.TrimSpace(line)) == 0 {
				result.Write(line)
			} else {
				var value any
				decoder := json.NewDecoder(bytes.NewReader(line))
				decoder.UseNumber()
				if e := decoder.Decode(&value); e != nil {
					return nil, e
				}
				changed := false
				var visit func(any) error
				visit = func(v any) error {
					switch v := v.(type) {
					case map[string]any:
						for key, item := range v {
							if key == "rollout_path" || key == "rolloutPath" {
								if path, ok := item.(string); ok {
									if path == old {
										v[key] = target
										changed = true
									} else if filepath.Base(path) == "rollout-restored-"+id+".jsonl" {
										return errors.New("The session index uses another path for this session.")
									}
								}
							} else if err := visit(item); err != nil {
								return err
							}
						}
					case []any:
						for _, item := range v {
							if err := visit(item); err != nil {
								return err
							}
						}
					}
					return nil
				}
				if err := visit(value); err != nil {
					return nil, err
				}
				if changed {
					encoded, e := json.Marshal(value)
					if e != nil {
						return nil, e
					}
					result.Write(encoded)
					result.WriteByte('\n')
				} else {
					result.Write(line)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return result.Bytes(), nil
}
func copyCodexRepairFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func replaceCodexRepairIndex(path string, raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".repair-index-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer func() { _ = os.Remove(temporary) }()
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, path)
}
