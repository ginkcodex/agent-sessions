package manage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// RenameSession persists a native provider title. Catalog metadata is supplied by
// the owning host, never by an RPC caller. Renaming does not require enabling deletion.
func (m *Manager) RenameSession(ctx context.Context, ref model.SessionRef, title string, catalog []model.SessionMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 1000 || strings.ContainsFunc(title, unicode.IsControl) {
		return errors.New("rename: title must contain 1–1000 characters without control characters")
	}
	var meta model.SessionMeta
	found := false
	for _, candidate := range catalog {
		if candidate.Ref == ref {
			meta = candidate
			found = true
			break
		}
	}
	if !found {
		return errors.New("rename: unknown session")
	}
	switch ref.Agent {
	case model.AgentClaude:
		return m.renameClaude(ctx, meta, title)
	case model.AgentCodex:
		return m.renameCodex(ctx, meta, title)
	case model.AgentOpenCode:
		return m.renameOpenCode(ctx, meta, title)
	default:
		return ErrUnsupportedAction
	}
}

// This is the native SDK operation, not a transcript rewrite:
// https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/session_mutations.py
// Its custom-title records are append-only, last-wins, and have no timestamp.
func (m *Manager) renameClaude(ctx context.Context, meta model.SessionMeta, title string) error {
	cm := m.claude
	if cm == nil || meta.ParentID != "" || !uuidPattern.MatchString(meta.Ref.ID) {
		return fmt.Errorf("%w: Claude rename requires a main UUID session", ErrUnsupportedAction)
	}
	// Only projects/<project>/<uuid>.jsonl is supported, never a caller path or subagent.
	project := filepath.Base(filepath.Dir(meta.SourcePath))
	target := filepath.Join(cm.root, "projects", project, meta.Ref.ID+".jsonl")
	if filepath.Clean(meta.SourcePath) != target {
		return ErrPathOutsideRoot
	}
	mode, err := cm.ps.classify(target)
	if err != nil {
		return safePathError(err)
	}
	if !mode.IsRegular() {
		return ErrPathOutsideRoot
	}
	before, err := os.Lstat(target)
	if err != nil {
		return errors.New("rename: cannot inspect Claude session")
	}
	// Root-relative open prevents a concurrently replaced ancestor symlink from
	// redirecting the write outside this host's configured provider root.
	root, err := os.OpenRoot(cm.root)
	if err != nil {
		return errors.New("rename: cannot open Claude root")
	}
	defer root.Close()
	rel, err := filepath.Rel(cm.root, target)
	if err != nil {
		return ErrPathOutsideRoot
	}
	f, err := root.OpenFile(rel, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return errors.New("rename: cannot open Claude session")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() == 0 || !os.SameFile(before, opened) {
		return errors.New("rename: session changed or is empty")
	}
	var terminal [1]byte
	if n, err := f.ReadAt(terminal[:], opened.Size()-1); err != nil || n != 1 || terminal[0] != '\n' {
		return errors.New("rename: Claude session has an incomplete trailing record")
	}
	if err := cm.ps.ensureWithinRoot(target); err != nil {
		return safePathError(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record, _ := json.Marshal(struct {
		Type  string `json:"type"`
		Title string `json:"customTitle"`
		ID    string `json:"sessionId"`
	}{"custom-title", title, meta.Ref.ID})
	// One O_APPEND write avoids clobbering concurrent CLI transcript appends.
	record = append(record, '\n')
	n, err := f.Write(record)
	if err != nil || n != len(record) {
		return errors.New("rename: cannot append Claude title")
	}
	if err := f.Sync(); err != nil {
		return errors.New("rename: cannot sync Claude title")
	}
	return nil
}
