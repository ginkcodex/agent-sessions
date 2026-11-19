package manage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// AntigravityWarning says what agy keeps after its files go to the Trash
// (docs/plan/antigravity-cli-delete.md).
const AntigravityWarning = "Antigravity keeps this session's title and the start of its first prompt in its own index. Putting the files back from the Trash restores the session."

// antigravityManager moves an agy session's own files to the Trash:
// brain/<id>/, conversations/<id>.db and annotations/<id>.pbtxt. agy's
// index (conversation_summaries.db) is never written: agy rebuilds removed
// rows from jetbox_summaries_proto.pb, and hides rows without files.
type antigravityManager struct {
	ps    *pathSafety
	trash Trash
	now   NowFunc
}

func newAntigravityManager(root string, trash Trash, now NowFunc) (*antigravityManager, error) {
	ps, err := newPathSafety(root)
	if err != nil {
		return nil, err
	}
	return &antigravityManager{ps: ps, trash: trash, now: now}, nil
}

// plan derives one session's files from its ID, checks they are safe to
// move and that the session is idle.
func (am *antigravityManager) plan(ctx context.Context, m model.SessionMeta) ([]itemFile, error) {
	if am.trash == nil {
		return nil, errors.New("trash transport is unavailable on this platform")
	}
	if m.Ref.Agent != model.AgentAntigravity || !uuidPattern.MatchString(m.Ref.ID) {
		return nil, errors.New("invalid session ID format")
	}
	brain := filepath.Join(am.ps.root, "brain", m.Ref.ID)
	src := filepath.Clean(m.SourcePath)
	if filepath.Dir(src) != filepath.Join(brain, ".system_generated", "logs") {
		return nil, errors.New("session path does not match session ID")
	}
	if err := am.ps.checkTree(ctx.Err, brain); err != nil {
		return nil, safePathError(err)
	}
	paths := []string{brain}
	for _, p := range []string{
		filepath.Join(am.ps.root, "conversations", m.Ref.ID+".db"),
		filepath.Join(am.ps.root, "conversations", m.Ref.ID+".db-wal"),
		filepath.Join(am.ps.root, "conversations", m.Ref.ID+".db-shm"),
		filepath.Join(am.ps.root, "annotations", m.Ref.ID+".pbtxt"),
	} {
		if _, err := os.Lstat(p); err == nil {
			paths = append(paths, p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("cannot inspect session files")
		}
	}
	files, err := filesForPaths(am.ps, paths)
	if err != nil {
		return nil, safePathError(err)
	}
	if checker, ok := am.trash.(TrashPathChecker); ok {
		for _, f := range files {
			if err := checker.CheckTrashPath(f.Path); err != nil {
				return nil, err
			}
		}
	}
	// The transcript is inside brain/, whose own mtime does not follow it.
	if err := checkRecent(am.now(), m, append([]itemFile{{Path: src}}, files...)); err != nil {
		return nil, err
	}
	if m.Turn != "" && m.Turn != model.TurnDone {
		return nil, fmt.Errorf("%w: session is currently active", ErrLive)
	}
	return files, nil
}

func (m *Manager) planAntigravity(ctx context.Context, meta model.SessionMeta, idx *catalogIndex, procLive map[string]string, procErr error) (operation, error) {
	if m.antigravity == nil {
		return operation{}, ErrUnsupportedAction
	}
	// A TUI or print-mode agy may have any session open; agy acp is skipped
	// (agentServer), its sessions are guarded by ownership and recency.
	if err := processSafetyError(procLive, procErr, "agy"); err != nil {
		return operation{}, err
	}
	children, err := idx.descendants(meta.Ref)
	if err != nil {
		return operation{}, err
	}
	// idx.descendants yields postorder: subagents before their parent.
	op := operation{Item: Item{Ref: meta.Ref, Agent: meta.Ref.Agent, Title: meta.Title, Action: ActionTrash, Reversible: true, Warning: AntigravityWarning}}
	for _, s := range append(children, meta) {
		files, err := m.antigravity.plan(ctx, s)
		if err != nil {
			if s.Ref != meta.Ref {
				return operation{}, fmt.Errorf("%w: subagent cannot be safely deleted", err)
			}
			return operation{}, err
		}
		op.Files = append(op.Files, files...)
		for _, f := range files {
			op.Item.Paths = append(op.Item.Paths, f.Path)
			op.Item.Bytes += bytesOf(f.Path, f.Mode)
		}
		if s.Ref != meta.Ref {
			op.Descendants = append(op.Descendants, s.Ref)
		}
	}
	return op, nil
}
